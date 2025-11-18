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

	"cloud_project/internal/tracker"
	"github.com/redis/go-redis/v9"
)

func main() {
	addr := env("TRACKER_ADDR", ":7070")
	redisAddr := env("REDIS_ADDR", "localhost:6379")
	ttlSeconds := envInt("TRACKER_TTL_SECONDS", 120)
	topologyURL := env("TOPOLOGY_URL", "http://localhost:8090")

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}

	service := tracker.NewService(rdb, tracker.Config{
		TTL:         time.Duration(ttlSeconds) * time.Second,
		TopologyURL: topologyURL,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.StartReaper(ctx)

<<<<<<< HEAD
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/announce", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req tracker.AnnounceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
=======
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	r.Post("/announce", func(w http.ResponseWriter, req *http.Request) {
		var body AnnounceRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
>>>>>>> 19b5dca (p2p network creation, song segmentation + upload, segment distribution, simulation, visualisation)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := service.HandleAnnounce(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req tracker.HeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := service.HandleHeartbeat(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/segments/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		segmentID := strings.TrimPrefix(r.URL.Path, "/segments/")
		if segmentID == "" {
			http.Error(w, "segment id required", http.StatusBadRequest)
			return
		}
		region := r.URL.Query().Get("region")
		resp, err := service.LookupSegment(r.Context(), segmentID, region)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	log.Printf("Tracker listening on %s (redis=%s, ttl=%ds)", addr, redisAddr, ttlSeconds)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("tracker server error: %v", err)
	}
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func envInt(key string, def int) int {
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
