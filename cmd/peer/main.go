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
