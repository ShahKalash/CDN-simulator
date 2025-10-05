package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type AnnounceRequest struct {
	PeerID         string   `json:"peerId"`
	Addr           string   `json:"addr"`
	Segments       []string `json:"segments"`
	Region         string   `json:"region"`
	RTT            int      `json:"rtt"`
	Bandwidth      string   `json:"bandwidth"`
	LastSeen       int64    `json:"lastSeen"`
	Availability   float64  `json:"availability"`
	DeviceType     string   `json:"deviceType"`
	IsSeedPeer     bool     `json:"isSeedPeer"`
	ConnectedPeers []string `json:"connectedPeers"`
	MaxConnections int      `json:"maxConnections"`
	UploadSlots    int      `json:"uploadSlots"`
}

type PeerInfo struct {
	PeerID         string   `json:"peerId"`
	Addr           string   `json:"addr"`
	Region         string   `json:"region"`
	RTT            int      `json:"rtt"`
	Bandwidth      string   `json:"bandwidth"`
	LastSeen       int64    `json:"lastSeen"`
	Availability   float64  `json:"availability"`
	DeviceType     string   `json:"deviceType"`
	IsSeedPeer     bool     `json:"isSeedPeer"`
	ConnectedPeers []string `json:"connectedPeers"`
	MaxConnections int      `json:"maxConnections"`
	UploadSlots    int      `json:"uploadSlots"`
}

type InMemoryTracker struct {
	mu      sync.RWMutex
	peers   map[string]*PeerInfo
	segments map[string]map[string]bool // segment -> peerId -> true
	ttl     time.Duration
}

func NewInMemoryTracker(ttl time.Duration) *InMemoryTracker {
	t := &InMemoryTracker{
		peers:    make(map[string]*PeerInfo),
		segments: make(map[string]map[string]bool),
		ttl:      ttl,
	}
	
	// Start cleanup goroutine
	go t.cleanup()
	
	return t
}

func (t *InMemoryTracker) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now().Unix()
		t.mu.Lock()
		
		// Remove expired peers
		for peerID, peer := range t.peers {
			if now-peer.LastSeen > int64(t.ttl.Seconds()) {
				delete(t.peers, peerID)
				// Remove from segment mappings
				for segment, peerMap := range t.segments {
					delete(peerMap, peerID)
					if len(peerMap) == 0 {
						delete(t.segments, segment)
					}
				}
			}
		}
		
		t.mu.Unlock()
	}
}

func (t *InMemoryTracker) Announce(peer *AnnounceRequest) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	now := time.Now().Unix()
	peerInfo := &PeerInfo{
		PeerID:         peer.PeerID,
		Addr:           peer.Addr,
		Region:         peer.Region,
		RTT:            peer.RTT,
		Bandwidth:      peer.Bandwidth,
		LastSeen:       now,
		Availability:   peer.Availability,
		DeviceType:     peer.DeviceType,
		IsSeedPeer:     peer.IsSeedPeer,
		ConnectedPeers: peer.ConnectedPeers,
		MaxConnections: peer.MaxConnections,
		UploadSlots:    peer.UploadSlots,
	}
	
	t.peers[peer.PeerID] = peerInfo
	
	// Update segment mappings
	for _, segment := range peer.Segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if t.segments[segment] == nil {
			t.segments[segment] = make(map[string]bool)
		}
		t.segments[segment][peer.PeerID] = true
	}
}

func (t *InMemoryTracker) GetPeers(segment string, region string, count int) []PeerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	peerMap, exists := t.segments[segment]
	if !exists {
		return []PeerInfo{}
	}
	
	peers := make([]PeerInfo, 0, len(peerMap))
	for peerID := range peerMap {
		if peer, exists := t.peers[peerID]; exists {
			peers = append(peers, *peer)
		}
	}
	
	// Sort by region match (desc), then RTT (asc), then peerID (asc)
	sort.Slice(peers, func(i, j int) bool {
		rim := boolToInt(peers[i].Region == region)
		rjm := boolToInt(peers[j].Region == region)
		if rim != rjm {
			return rim > rjm
		}
		if peers[i].RTT != peers[j].RTT {
			return peers[i].RTT < peers[j].RTT
		}
		return peers[i].PeerID < peers[j].PeerID
	})
	
	if len(peers) > count {
		peers = peers[:count]
	}
	
	return peers
}

func main() {
	httpAddr := getenv("HTTP_ADDR", ":8090")
	ttlSeconds := getenvInt("TRACKER_TTL_SECONDS", 120)
	
	tracker := NewInMemoryTracker(time.Duration(ttlSeconds) * time.Second)
	
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.Post("/announce", func(w http.ResponseWriter, req *http.Request) {
		var body AnnounceRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.PeerID == "" || len(body.Segments) == 0 {
			http.Error(w, "peerId and segments required", http.StatusBadRequest)
			return
		}
		
		tracker.Announce(&body)
		w.WriteHeader(http.StatusNoContent)
	})

	r.Get("/peers", func(w http.ResponseWriter, req *http.Request) {
		seg := req.URL.Query().Get("seg")
		if seg == "" {
			http.Error(w, "seg required", http.StatusBadRequest)
			return
		}
		wantCount, _ := strconv.Atoi(req.URL.Query().Get("count"))
		if wantCount <= 0 {
			wantCount = 10
		}
		region := req.URL.Query().Get("region")

		peers := tracker.GetPeers(seg, region, wantCount)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(peers)
	})

	log.Printf("tracker listening on %s (in-memory)", httpAddr)
	log.Fatal(http.ListenAndServe(httpAddr, r))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Simple permissive CORS for demo
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
