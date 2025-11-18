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
	signalclient "cloud_project/internal/peer/signalling"
	trackerclient "cloud_project/internal/peer/tracker"
)

type peerConfig struct {
	Name              string
	Port              string
	Neighbors         []string
	TrackerURL        string
	SignalURL         string
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
}

func newPeerApp(cfg peerConfig) *peerApp {
	app := &peerApp{
		cfg:          cfg,
		cache:        cachepkg.NewLRU(cfg.CacheCapacity),
		tracker:      trackerclient.NewClient(cfg.TrackerURL),
		heartbeatTrg: make(chan struct{}, 1),
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
	signalURL := getenv("SIGNAL_URL", "ws://localhost:7080/ws")
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
		SignalURL:         signalURL,
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
			seg, ok := a.cache.Get(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			resp := fetchSegmentResponse{
				ID:      seg.ID,
				Payload: base64.StdEncoding.EncodeToString(seg.Data),
			}
			writeJSON(w, resp)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
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
	payload := trackerclient.AnnouncePayload{
		PeerID:    a.cfg.Name,
		Room:      a.cfg.Room,
		Region:    a.cfg.Region,
		RTTms:     a.cfg.RTTms,
		Segments:  segments,
		Neighbors: a.cfg.Neighbors,
	}
	if err := a.tracker.Announce(ctx, payload); err != nil {
		log.Printf("[%s] tracker announce failed: %v", a.cfg.Name, err)
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
	}
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
				resp, err := client.Get(url)
				if err != nil {
					log.Printf("[%s] neighbor %s unreachable: %v", a.cfg.Name, neighbor, err)
					continue
				}
				resp.Body.Close()
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
