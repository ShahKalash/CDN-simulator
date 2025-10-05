package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	redis "github.com/redis/go-redis/v9"
)

type AnnounceRequest struct {
	PeerID   string   `json:"peerId"`
	Segments []string `json:"segments"`
	Region   string   `json:"region"`
	RTT      int      `json:"rtt"`
	LastSeen int64    `json:"lastSeen"`
}

type PeerInfo struct {
	PeerID string `json:"peerId"`
	Region string `json:"region"`
	RTT    int    `json:"rtt"`
}

func main() {
	httpAddr := getenv("HTTP_ADDR", ":8090")
	redisAddr := getenv("REDIS_ADDR", "127.0.0.1:6379")
	ttlSeconds := getenvInt("TRACKER_TTL_SECONDS", 120)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx := context.Background()

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
		now := time.Now().Unix()
		pi := PeerInfo{PeerID: body.PeerID, Region: body.Region, RTT: body.RTT}
		peerKey := "peer:" + body.PeerID
		_ = rdb.HSet(ctx, peerKey, map[string]interface{}{
			"region": pi.Region,
			"rtt":    pi.RTT,
			"ts":     now,
		}).Err()
		_ = rdb.Expire(ctx, peerKey, time.Duration(ttlSeconds)*time.Second).Err()
		// For each segment, add peer to set seg:<id>
		for _, seg := range body.Segments {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			setKey := "seg:" + seg
			_ = rdb.SAdd(ctx, setKey, body.PeerID).Err()
			_ = rdb.Expire(ctx, setKey, time.Duration(ttlSeconds)*time.Second).Err()
		}
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

		ids, err := rdb.SMembers(ctx, "seg:"+seg).Result()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Fetch peer infos and apply simple deterministic sort by (region match desc, rtt asc, id)
		peers := make([]PeerInfo, 0, len(ids))
		for _, id := range ids {
			h, err := rdb.HGetAll(ctx, "peer:"+id).Result()
			if err != nil || len(h) == 0 {
				continue
			}
			rtt, _ := strconv.Atoi(h["rtt"])
			peers = append(peers, PeerInfo{PeerID: id, Region: h["region"], RTT: rtt})
		}
		// Simple stable selection
		sortFunc := func(i, j int) bool {
			rim := boolToInt(peers[i].Region == region)
			rjm := boolToInt(peers[j].Region == region)
			if rim != rjm {
				return rim > rjm
			}
			if peers[i].RTT != peers[j].RTT {
				return peers[i].RTT < peers[j].RTT
			}
			return peers[i].PeerID < peers[j].PeerID
		}
		for i := 0; i < len(peers); i++ { // insertion sort for simplicity and determinism
			for j := i; j > 0 && sortFunc(j, j-1); j-- {
				peers[j], peers[j-1] = peers[j-1], peers[j]
			}
		}
		if len(peers) > wantCount {
			peers = peers[:wantCount]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(peers)
	})

	log.Printf("tracker listening on %s (redis %s)", httpAddr, redisAddr)
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
