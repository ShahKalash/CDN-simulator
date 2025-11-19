package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	cachepkg "cloud_project/internal/peer/cache"
	rttpkg "cloud_project/internal/peer/rtt"
	signalclient "cloud_project/internal/peer/signalling"
	trackerclient "cloud_project/internal/peer/tracker"
)

type peerConfig struct {
	Name              string
	Port              string
	Neighbors         []string
	TrackerURL        string
	TopologyURL       string
	SignalURL         string
	EdgeURLs          []string // List of edge server URLs
	Room              string
	Region            string
	RTTms             int
	HeartbeatInterval time.Duration
	CacheCapacity     int
}

type peerApp struct {
	cfg          peerConfig
	cache        *cachepkg.LRU
	tracker      *trackerclient.Client
	signal       *signalclient.Client
	server       *http.Server
	heartbeatTrg chan struct{}
	rttMeasurer  *rttpkg.Measurer
	httpClient   *http.Client
}

func newPeerApp(cfg peerConfig) *peerApp {
	app := &peerApp{
		cfg:          cfg,
		cache:        cachepkg.NewLRU(cfg.CacheCapacity),
		tracker:      trackerclient.NewClient(cfg.TrackerURL),
		heartbeatTrg: make(chan struct{}, 1),
		rttMeasurer:  rttpkg.NewMeasurer(),
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
	if cfg.SignalURL != "" {
		app.signal = signalclient.NewClient(cfg.SignalURL, cfg.Room, cfg.Name, cfg.Neighbors)
	}
	return app
}

func loadConfig() peerConfig {
	name := getenv("PEER_NAME", "peer")
	port := getenv("PEER_PORT", "8080")
	rawNeighbors := strings.TrimSpace(os.Getenv("PEER_NEIGHBORS"))
	var neighbors []string
	if rawNeighbors != "" {
		for _, n := range considerNeighborList(strings.Split(rawNeighbors, ",")) {
			neighbors = append(neighbors, n)
		}
	}
	trackerURL := getenv("TRACKER_URL", "http://localhost:7070")
	topologyURL := getenv("TOPOLOGY_URL", "http://localhost:8090")
	signalURL := getenv("SIGNAL_URL", "ws://localhost:7080/ws")
	rawEdgeURLs := strings.TrimSpace(os.Getenv("EDGE_URLS"))
	var edgeURLs []string
	if rawEdgeURLs != "" {
		edgeURLs = strings.Split(rawEdgeURLs, ",")
		for i := range edgeURLs {
			edgeURLs[i] = strings.TrimSpace(edgeURLs[i])
		}
	}
	room := getenv("PEER_ROOM", "default")
	region := getenv("PEER_REGION", "global")
	rtt := getenvInt("PEER_RTT_MS", 25)
	hbInterval := time.Duration(getenvInt("HEARTBEAT_INTERVAL_SEC", 30)) * time.Second
	cacheCap := getenvInt("CACHE_CAPACITY", 64)

	return peerConfig{
		Name:              name,
		Port:              port,
		Neighbors:         neighbors,
		TrackerURL:        trackerURL,
		TopologyURL:       topologyURL,
		SignalURL:         signalURL,
		EdgeURLs:          edgeURLs,
		Room:              room,
		Region:            region,
		RTTms:             rtt,
		HeartbeatInterval: hbInterval,
		CacheCapacity:     cacheCap,
	}
}

func getenv(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func getenvInt(key string, def int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}

func considerNeighborList(items []string) []string {
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (a *peerApp) startHTTP(ctx context.Context) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s: ok", a.cfg.Name)
	})
	mux.HandleFunc("/peers", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, strings.Join(a.cfg.Neighbors, ","))
	})
	mux.HandleFunc("/name", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, a.cfg.Name)
	})
	mux.HandleFunc("/segments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req storeSegmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.storeSegment(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.triggerHeartbeat()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/segments/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/segments/")
		if id == "" {
			http.Error(w, "segment id required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			// Measure RTT for this request
			start := time.Now()
			seg, ok := a.cache.Get(id)
			rtt := int(time.Since(start).Milliseconds())
			
			// Update RTT measurement for this peer (from requester's perspective)
			// The requester will measure the full round-trip
			if !ok {
				http.NotFound(w, r)
				return
			}
			resp := fetchSegmentResponse{
				ID:      seg.ID,
				Payload: base64.StdEncoding.EncodeToString(seg.Data),
			}
			writeJSON(w, resp)
			_ = rtt // RTT measured here is just processing time, not network RTT
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/rtt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		allRTTs := a.rttMeasurer.GetAll()
		writeJSON(w, map[string]any{
			"rtts":  allRTTs,
			"average": a.rttMeasurer.GetAverage(),
		})
	})
	mux.HandleFunc("/request/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		segmentID := strings.TrimPrefix(r.URL.Path, "/request/")
		if segmentID == "" {
			http.Error(w, "segment id required", http.StatusBadRequest)
			return
		}
		
		// Use the full routing logic
		result, err := a.requestSegment(r.Context(), segmentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		
		resp := map[string]any{
			"id":           segmentID,
			"payload":      base64.StdEncoding.EncodeToString(result.Data),
			"source":       result.Source,
			"path":         result.Path,
			"hops":         result.Hops,
			"rtt_ms":       result.RTTms,
			"est_rtt_ms":   result.EstRTTms,
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("/songs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		songID := strings.TrimPrefix(r.URL.Path, "/songs/")
		if songID == "" {
			http.Error(w, "song id required", http.StatusBadRequest)
			return
		}
		
		// Request entire song and distribute segments
		if err := a.requestSong(r.Context(), songID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		writeJSON(w, map[string]string{
			"status": "distributed",
			"song_id": songID,
		})
	})

	server := &http.Server{
		Addr:    ":" + a.cfg.Port,
		Handler: mux,
	}
	a.server = server

	go func() {
		log.Printf("[%s] HTTP listening on %s", a.cfg.Name, server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", a.cfg.Name, err)
		}
	}()
	return server
}

type storeSegmentRequest struct {
	ID      string `json:"id"`
	Payload string `json:"payload"`
}

type fetchSegmentResponse struct {
	ID      string `json:"id"`
	Payload string `json:"payload"`
}

func (a *peerApp) storeSegment(req storeSegmentRequest) error {
	if req.ID == "" || req.Payload == "" {
		return fmt.Errorf("id and payload required")
	}
	data, err := base64.StdEncoding.DecodeString(req.Payload)
	if err != nil {
		return fmt.Errorf("invalid base64 payload: %w", err)
	}
	a.cache.Put(cachepkg.Segment{ID: req.ID, Data: data})
	return nil
}

func (a *peerApp) triggerHeartbeat() {
	select {
	case a.heartbeatTrg <- struct{}{}:
	default:
	}
}

func (a *peerApp) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer ticker.Stop()
	a.emitAnnounce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.emitHeartbeat(ctx)
		case <-a.heartbeatTrg:
			a.emitHeartbeat(ctx)
		}
	}
}

func (a *peerApp) emitAnnounce(ctx context.Context) {
	if a.tracker == nil {
		return
	}
	segments := a.cache.Keys()
	
	// Measure RTT to tracker
	trackerRTT := a.rttMeasurer.Get("tracker")
	if trackerRTT == 0 {
		// First time, measure it
		url := fmt.Sprintf("%s/healthz", a.cfg.TrackerURL)
		if rtt, err := a.rttMeasurer.MeasureHTTP(ctx, a.httpClient, http.MethodGet, url); err == nil {
			a.rttMeasurer.Update("tracker", rtt)
			trackerRTT = rtt
		} else {
			// Fallback to average or default
			avg := a.rttMeasurer.GetAverage()
			if avg == 0 {
				trackerRTT = a.cfg.RTTms // Use static fallback
			} else {
				trackerRTT = avg
			}
		}
	}
	
	// Use average RTT to neighbors as our representative RTT
	avgNeighborRTT := a.calculateAverageNeighborRTT()
	if avgNeighborRTT == 0 {
		avgNeighborRTT = trackerRTT
	}
	if avgNeighborRTT == 0 {
		avgNeighborRTT = a.cfg.RTTms // Final fallback
	}
	
	payload := trackerclient.AnnouncePayload{
		PeerID:    a.cfg.Name,
		Room:      a.cfg.Room,
		Region:    a.cfg.Region,
		RTTms:     avgNeighborRTT, // Use measured RTT instead of static
		Segments:  segments,
		Neighbors: a.cfg.Neighbors,
	}
	if err := a.tracker.Announce(ctx, payload); err != nil {
		log.Printf("[%s] tracker announce failed: %v", a.cfg.Name, err)
	} else {
		// Measure RTT to tracker after successful announce
		url := fmt.Sprintf("%s/healthz", a.cfg.TrackerURL)
		if rtt, err := a.rttMeasurer.MeasureHTTP(ctx, a.httpClient, http.MethodGet, url); err == nil {
			a.rttMeasurer.Update("tracker", rtt)
		}
	}
}

func (a *peerApp) emitHeartbeat(ctx context.Context) {
	if a.tracker == nil {
		return
	}
	segments := a.cache.Keys()
	payload := trackerclient.HeartbeatPayload{
		PeerID:    a.cfg.Name,
		Segments:  segments,
		Neighbors: a.cfg.Neighbors,
	}
	if err := a.tracker.Heartbeat(ctx, payload); err != nil {
		log.Printf("[%s] tracker heartbeat failed: %v", a.cfg.Name, err)
	} else {
		// Measure RTT to tracker after successful heartbeat
		url := fmt.Sprintf("%s/healthz", a.cfg.TrackerURL)
		if rtt, err := a.rttMeasurer.MeasureHTTP(ctx, a.httpClient, http.MethodGet, url); err == nil {
			a.rttMeasurer.Update("tracker", rtt)
		}
	}
}

// calculateAverageNeighborRTT calculates the average RTT to all neighbors
func (a *peerApp) calculateAverageNeighborRTT() int {
	if len(a.cfg.Neighbors) == 0 {
		return 0
	}
	sum := 0
	count := 0
	for _, neighbor := range a.cfg.Neighbors {
		rtt := a.rttMeasurer.Get(neighbor)
		if rtt > 0 {
			sum += rtt
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

// fetchSegmentFromPeer fetches a segment from another peer and measures RTT
// Returns the segment data, RTT in milliseconds, and any error
func (a *peerApp) fetchSegmentFromPeer(ctx context.Context, peerID, segmentID string) ([]byte, int, error) {
	url := fmt.Sprintf("http://%s:%s/segments/%s", peerID, a.cfg.Port, segmentID)
	
	// Measure RTT while fetching
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rtt := int(time.Since(start).Milliseconds())
	
	// Update RTT measurement for this peer
	a.rttMeasurer.Update(peerID, rtt)
	
	if resp.StatusCode != http.StatusOK {
		return nil, rtt, fmt.Errorf("peer returned status %d", resp.StatusCode)
	}
	
	var segResp fetchSegmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&segResp); err != nil {
		return nil, rtt, err
	}
	
	data, err := base64.StdEncoding.DecodeString(segResp.Payload)
	if err != nil {
		return nil, rtt, fmt.Errorf("invalid base64 payload: %w", err)
	}
	
	return data, rtt, nil
}

// fetchSegmentFromEdge fetches a segment from an edge server
func (a *peerApp) fetchSegmentFromEdge(ctx context.Context, edgeURL, segmentID string) ([]byte, int, error) {
	url := fmt.Sprintf("%s/segments/%s", edgeURL, segmentID)
	
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rtt := int(time.Since(start).Milliseconds())
	
	// Update RTT measurement for this edge
	a.rttMeasurer.Update(edgeURL, rtt)
	
	if resp.StatusCode != http.StatusOK {
		return nil, rtt, fmt.Errorf("edge returned status %d", resp.StatusCode)
	}
	
	var segResp fetchSegmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&segResp); err != nil {
		return nil, rtt, err
	}
	
	data, err := base64.StdEncoding.DecodeString(segResp.Payload)
	if err != nil {
		return nil, rtt, fmt.Errorf("invalid base64 payload: %w", err)
	}
	
	return data, rtt, nil
}

// findBestEdge finds the best edge server based on shortest path and RTT
func (a *peerApp) findBestEdge(ctx context.Context) (string, error) {
	if len(a.cfg.EdgeURLs) == 0 {
		return "", fmt.Errorf("no edge servers configured")
	}
	
	// If only one edge, return it
	if len(a.cfg.EdgeURLs) == 1 {
		return a.cfg.EdgeURLs[0], nil
	}
	
	// Measure RTT to all edges and find best
	bestEdge := a.cfg.EdgeURLs[0]
	bestRTT := a.rttMeasurer.Get(bestEdge)
	if bestRTT == 0 {
		// Measure it
		url := fmt.Sprintf("%s/health", bestEdge)
		if rtt, err := a.rttMeasurer.MeasureHTTP(ctx, a.httpClient, http.MethodGet, url); err == nil {
			a.rttMeasurer.Update(bestEdge, rtt)
			bestRTT = rtt
		}
	}
	
	for _, edgeURL := range a.cfg.EdgeURLs[1:] {
		rtt := a.rttMeasurer.Get(edgeURL)
		if rtt == 0 {
			// Measure it
			url := fmt.Sprintf("%s/health", edgeURL)
			if measuredRTT, err := a.rttMeasurer.MeasureHTTP(ctx, a.httpClient, http.MethodGet, url); err == nil {
				a.rttMeasurer.Update(edgeURL, measuredRTT)
				rtt = measuredRTT
			}
		}
		if rtt > 0 && (bestRTT == 0 || rtt < bestRTT) {
			bestRTT = rtt
			bestEdge = edgeURL
		}
	}
	
	return bestEdge, nil
}

// segmentRequestResult holds the result of a segment request including path information
type segmentRequestResult struct {
	Data      []byte
	Source    string
	Path      []string
	Hops      int
	RTTms     int
	EstRTTms  int
}

// requestSegment handles the full routing logic: P2P → Edge → Origin
// Returns segment data, source type, path info, and error
func (a *peerApp) requestSegment(ctx context.Context, segmentID string) (*segmentRequestResult, error) {
	// Step 1: Check local cache
	if seg, ok := a.cache.Get(segmentID); ok {
		return &segmentRequestResult{
			Data:     seg.Data,
			Source:   "local",
			Path:     []string{a.cfg.Name},
			Hops:     0,
			RTTms:    0,
			EstRTTms: 0,
		}, nil
	}
	
	// Step 2: Try P2P - query tracker
	type trackerPeer struct {
		PeerID string `json:"peer_id"`
		Region string `json:"region"`
		RTTms  int    `json:"rtt_ms"`
	}
	
	type trackerResponse struct {
		Segment string        `json:"segment"`
		Peers   []trackerPeer `json:"peers"`
	}
	
	trackerURL := fmt.Sprintf("%s/segments/%s?region=%s", a.cfg.TrackerURL, segmentID, a.cfg.Region)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trackerURL, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := a.httpClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		var trackerResp trackerResponse
		if err := json.NewDecoder(resp.Body).Decode(&trackerResp); err == nil {
			resp.Body.Close()
			
			// Try fetching from best peer
			for _, peer := range trackerResp.Peers {
				// Get path information to peer
				pathURL := fmt.Sprintf("%s/path?from=%s&to=%s", a.cfg.TopologyURL, a.cfg.Name, peer.PeerID)
				pathReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pathURL, nil)
				var pathInfo map[string]interface{}
				if err == nil {
					pathResp, err := a.httpClient.Do(pathReq)
					if err == nil && pathResp.StatusCode == http.StatusOK {
						json.NewDecoder(pathResp.Body).Decode(&pathInfo)
						pathResp.Body.Close()
					}
				}
				
				data, rtt, err := a.fetchSegmentFromPeer(ctx, peer.PeerID, segmentID)
				if err == nil {
					// Store in cache
					a.cache.Put(cachepkg.Segment{ID: segmentID, Data: data})
					a.triggerHeartbeat()
					
					// Extract path information
					path := []string{a.cfg.Name, peer.PeerID}
					hops := 1
					estRTT := rtt
					if pathInfo != nil {
						if p, ok := pathInfo["path"].([]interface{}); ok {
							path = make([]string, 0, len(p))
							for _, node := range p {
								if s, ok := node.(string); ok {
									path = append(path, s)
								}
							}
						}
						if h, ok := pathInfo["hops"].(float64); ok {
							hops = int(h)
						}
						if r, ok := pathInfo["estimated_rtt_ms"].(float64); ok {
							estRTT = int(r)
						}
					}
					
					log.Printf("[%s] Fetched segment %s from P2P peer %s | Path: %v | Hops: %d | Est. RTT: %dms | Actual RTT: %dms", 
						a.cfg.Name, segmentID, peer.PeerID, path, hops, estRTT, rtt)
					
					return &segmentRequestResult{
						Data:     data,
						Source:   "p2p",
						Path:     path,
						Hops:     hops,
						RTTms:    rtt,
						EstRTTms: estRTT,
					}, nil
				}
			}
		} else {
			resp.Body.Close()
		}
	}
	
	// Step 3: Try Edge servers
	edgeURL, err := a.findBestEdge(ctx)
	if err == nil {
		// Get path information to edge
		edgeName := edgeURL
		if strings.Contains(edgeURL, "://") {
			parts := strings.Split(strings.TrimPrefix(edgeURL, "http://"), ":")
			edgeName = parts[0]
		}
		
		// Query topology for path information
		pathURL := fmt.Sprintf("%s/path?from=%s&to=%s", a.cfg.TopologyURL, a.cfg.Name, edgeName)
		pathReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pathURL, nil)
		var pathInfo map[string]interface{}
		if err == nil {
			pathResp, err := a.httpClient.Do(pathReq)
			if err == nil && pathResp.StatusCode == http.StatusOK {
				json.NewDecoder(pathResp.Body).Decode(&pathInfo)
				pathResp.Body.Close()
			}
		}
		
		data, rtt, err := a.fetchSegmentFromEdge(ctx, edgeURL, segmentID)
		if err == nil {
			// Store in cache
			a.cache.Put(cachepkg.Segment{ID: segmentID, Data: data})
			a.triggerHeartbeat()
			
			// Extract path information
			path := []string{a.cfg.Name, edgeName}
			hops := 1
			estRTT := rtt
			if pathInfo != nil {
				if p, ok := pathInfo["path"].([]interface{}); ok {
					path = make([]string, 0, len(p))
					for _, node := range p {
						if s, ok := node.(string); ok {
							path = append(path, s)
						}
					}
				}
				if h, ok := pathInfo["hops"].(float64); ok {
					hops = int(h)
				}
				if r, ok := pathInfo["estimated_rtt_ms"].(float64); ok {
					estRTT = int(r)
				}
			}
			
			log.Printf("[%s] Fetched segment %s from edge %s | Path: %v | Hops: %d | Est. RTT: %dms | Actual RTT: %dms", 
				a.cfg.Name, segmentID, edgeURL, path, hops, estRTT, rtt)
			
			return &segmentRequestResult{
				Data:     data,
				Source:   "edge",
				Path:     path,
				Hops:     hops,
				RTTms:    rtt,
				EstRTTms: estRTT,
			}, nil
		}
		
		// Try other edges if first one failed
		for _, otherEdge := range a.cfg.EdgeURLs {
			if otherEdge == edgeURL {
				continue
			}
			// Get path to other edge
			otherEdgeName := otherEdge
			if strings.Contains(otherEdge, "://") {
				parts := strings.Split(strings.TrimPrefix(otherEdge, "http://"), ":")
				otherEdgeName = parts[0]
			}
			pathURL := fmt.Sprintf("%s/path?from=%s&to=%s", a.cfg.TopologyURL, a.cfg.Name, otherEdgeName)
			pathReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pathURL, nil)
			var otherPathInfo map[string]interface{}
			if err == nil {
				pathResp, err := a.httpClient.Do(pathReq)
				if err == nil && pathResp.StatusCode == http.StatusOK {
					json.NewDecoder(pathResp.Body).Decode(&otherPathInfo)
					pathResp.Body.Close()
				}
			}
			
			data, rtt, err := a.fetchSegmentFromEdge(ctx, otherEdge, segmentID)
			if err == nil {
				a.cache.Put(cachepkg.Segment{ID: segmentID, Data: data})
				a.triggerHeartbeat()
				
				path := []string{a.cfg.Name, otherEdgeName}
				hops := 1
				estRTT := rtt
				if otherPathInfo != nil {
					if p, ok := otherPathInfo["path"].([]interface{}); ok {
						path = make([]string, 0, len(p))
						for _, node := range p {
							if s, ok := node.(string); ok {
								path = append(path, s)
							}
						}
					}
					if h, ok := otherPathInfo["hops"].(float64); ok {
						hops = int(h)
					}
					if r, ok := otherPathInfo["estimated_rtt_ms"].(float64); ok {
						estRTT = int(r)
					}
				}
				
				log.Printf("[%s] Fetched segment %s from edge %s | Path: %v | Hops: %d | Est. RTT: %dms | Actual RTT: %dms", 
					a.cfg.Name, segmentID, otherEdge, path, hops, estRTT, rtt)
				
				return &segmentRequestResult{
					Data:     data,
					Source:   "edge",
					Path:     path,
					Hops:     hops,
					RTTms:    rtt,
					EstRTTms: estRTT,
				}, nil
			}
		}
	}
	
	return nil, fmt.Errorf("segment not found in P2P network, edge servers, or origin")
}

// requestSong requests an entire song and distributes segments along the path
// This handles the initial request case where all caches are empty
func (a *peerApp) requestSong(ctx context.Context, songID string) error {
	// Find best edge
	edgeURL, err := a.findBestEdge(ctx)
	if err != nil {
		return fmt.Errorf("no edge servers available: %w", err)
	}
	
	// Extract edge name from URL (e.g., "http://edge-1:8082" -> "edge-1")
	edgeName := edgeURL
	if strings.Contains(edgeURL, "://") {
		parts := strings.Split(strings.TrimPrefix(edgeURL, "http://"), ":")
		edgeName = parts[0]
	}
	
	// Get path to edge using topology service
	pathURL := fmt.Sprintf("%s/path?from=%s&to=%s", a.cfg.TopologyURL, a.cfg.Name, edgeName)
	pathReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pathURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create path request: %w", err)
	}
	
	pathResp, err := a.httpClient.Do(pathReq)
	if err != nil {
		return fmt.Errorf("failed to query topology for path: %w", err)
	}
	defer pathResp.Body.Close()
	
	if pathResp.StatusCode != http.StatusOK {
		return fmt.Errorf("topology returned status %d for path query", pathResp.StatusCode)
	}
	
	var pathData map[string]interface{}
	if err := json.NewDecoder(pathResp.Body).Decode(&pathData); err != nil {
		return fmt.Errorf("failed to decode path response: %w", err)
	}
	
	path, ok := pathData["path"].([]interface{})
	if !ok || len(path) == 0 {
		return fmt.Errorf("invalid or empty path from topology")
	}
	
	// Extract path information
	pathStr := make([]string, 0, len(path))
	for _, p := range path {
		if s, ok := p.(string); ok {
			pathStr = append(pathStr, s)
		}
	}
	
	hops := 0
	if h, ok := pathData["hops"].(float64); ok {
		hops = int(h)
	}
	estRTT := 0
	if r, ok := pathData["estimated_rtt_ms"].(float64); ok {
		estRTT = int(r)
	}
	
	log.Printf("[%s] Requesting song %s from edge %s | Path: %v | Hops: %d | Est. RTT: %dms", 
		a.cfg.Name, songID, edgeURL, pathStr, hops, estRTT)
	
	// Get all segments for the song from edge
	segmentsURL := fmt.Sprintf("%s/songs/%s", edgeURL, songID)
	segReq, err := http.NewRequestWithContext(ctx, http.MethodGet, segmentsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create segments request: %w", err)
	}
	
	segResp, err := a.httpClient.Do(segReq)
	if err != nil {
		return fmt.Errorf("failed to fetch segments from edge: %w", err)
	}
	defer segResp.Body.Close()
	
	if segResp.StatusCode != http.StatusOK {
		return fmt.Errorf("edge returned status %d for segments", segResp.StatusCode)
	}
	
	var songData map[string]interface{}
	if err := json.NewDecoder(segResp.Body).Decode(&songData); err != nil {
		return fmt.Errorf("failed to decode song data: %w", err)
	}
	
	segments, ok := songData["segments"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid segments data from edge")
	}
	
	segmentCount := len(segments)
	pathLength := len(pathStr)
	
	if pathLength == 0 {
		return fmt.Errorf("empty path, cannot distribute segments")
	}
	
	if segmentCount == 0 {
		return fmt.Errorf("no segments found for song %s", songID)
	}
	
	// Distribute segments: segmentCount / pathLength per node
	segmentsPerNode := segmentCount / pathLength
	if segmentsPerNode == 0 {
		segmentsPerNode = 1
	}
	
	log.Printf("[%s] Distributing %d segments along path of length %d (%d segments per node)", 
		a.cfg.Name, segmentCount, pathLength, segmentsPerNode)
	
	// Distribute segments to each node in path (excluding ourselves from intermediate distribution)
	segmentIndex := 0
	for _, nodeID := range pathStr {
		if nodeID == a.cfg.Name {
			// Skip ourselves in the distribution loop - we'll get remaining segments at the end
			continue
		}
		
		// Assign segmentsPerNode segments to this node
		for j := 0; j < segmentsPerNode && segmentIndex < segmentCount; j++ {
			if seg, ok := segments[segmentIndex].(map[string]interface{}); ok {
				if segID, ok := seg["id"].(string); ok {
					// Fetch segment from edge and send to intermediate peer
					data, _, err := a.fetchSegmentFromEdge(ctx, edgeURL, segID)
					if err == nil {
						// Send segment to intermediate peer for caching
						if sendErr := a.sendSegmentToPeer(ctx, nodeID, segID, data); sendErr != nil {
							log.Printf("[%s] Warning: failed to send segment %s to %s: %v", a.cfg.Name, segID, nodeID, sendErr)
						}
					} else {
						log.Printf("[%s] Warning: failed to fetch segment %s from edge: %v", a.cfg.Name, segID, err)
					}
				}
			}
			segmentIndex++
		}
	}
	
	// Requesting peer gets all remaining segments
	for segmentIndex < segmentCount {
		if seg, ok := segments[segmentIndex].(map[string]interface{}); ok {
			if segID, ok := seg["id"].(string); ok {
				data, _, err := a.fetchSegmentFromEdge(ctx, edgeURL, segID)
				if err == nil {
					a.cache.Put(cachepkg.Segment{ID: segID, Data: data})
				} else {
					log.Printf("[%s] Warning: failed to fetch segment %s from edge: %v", a.cfg.Name, segID, err)
				}
			}
		}
		segmentIndex++
	}
	
	// Calculate how many segments were distributed vs cached locally
	distributedCount := (pathLength - 1) * segmentsPerNode // Excluding ourselves
	localCount := segmentCount - distributedCount
	
	log.Printf("[%s] Successfully distributed song %s: %d segments cached locally, %d segments distributed to %d intermediate nodes", 
		a.cfg.Name, songID, localCount, distributedCount, pathLength-1)
	
	a.triggerHeartbeat()
	return nil
}

// sendSegmentToPeer sends a segment to another peer for caching
func (a *peerApp) sendSegmentToPeer(ctx context.Context, peerID, segmentID string, data []byte) error {
	url := fmt.Sprintf("http://%s:%s/segments", peerID, a.cfg.Port)
	payload := base64.StdEncoding.EncodeToString(data)
	
	body := map[string]string{
		"id":      segmentID,
		"payload": payload,
	}
	
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}
	
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("peer returned status %d", resp.StatusCode)
	}
	
	return nil
}

func (a *peerApp) startNeighborProbe(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	client := http.Client{Timeout: 3 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, neighbor := range a.cfg.Neighbors {
				url := fmt.Sprintf("http://%s:%s/health", neighbor, a.cfg.Port)
				// Measure RTT to neighbor
				rtt, err := a.rttMeasurer.MeasureHTTP(ctx, &client, http.MethodGet, url)
				if err != nil {
					log.Printf("[%s] neighbor %s unreachable: %v", a.cfg.Name, neighbor, err)
					continue
				}
				// Update RTT measurement for this neighbor
				a.rttMeasurer.Update(neighbor, rtt)
				log.Printf("[%s] neighbor %s RTT: %dms", a.cfg.Name, neighbor, rtt)
			}
		}
	}
}

func (a *peerApp) writeSignal(ctx context.Context) {
	if a.signal == nil {
		return
	}
	if err := a.signal.Connect(ctx); err != nil {
		log.Printf("[%s] signalling connect failed: %v", a.cfg.Name, err)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func main() {
	cfg := loadConfig()
	app := newPeerApp(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	app.startHTTP(ctx)
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.heartbeatLoop(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.startNeighborProbe(ctx)
	}()
	go app.writeSignal(ctx)

	waitForShutdown(app, cancel, &wg)
}

func waitForShutdown(app *peerApp, cancel context.CancelFunc, wg *sync.WaitGroup) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	s := <-quit
	log.Printf("[%s] shutting down (%v)", app.cfg.Name, s)
	cancel()
	if app.server != nil {
		shutdownCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelTimeout()
		app.server.Shutdown(shutdownCtx)
	}
	if app.signal != nil {
		app.signal.Close()
	}
	wg.Wait()
}
