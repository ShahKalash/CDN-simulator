package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type edgeConfig struct {
	Name        string
	Port        string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	OriginURL   string
	TopologyURL string
	ConnectedPeers []string // Peers connected to this edge
}

type edgeApp struct {
	cfg    edgeConfig
	db     *sql.DB
	server *http.Server
	client *http.Client
}

func loadConfig() edgeConfig {
	rawPeers := strings.TrimSpace(os.Getenv("CONNECTED_PEERS"))
	var connectedPeers []string
	if rawPeers != "" {
		connectedPeers = strings.Split(rawPeers, ",")
		for i := range connectedPeers {
			connectedPeers[i] = strings.TrimSpace(connectedPeers[i])
		}
	}
	
	return edgeConfig{
		Name:        getenv("EDGE_NAME", "edge-1"),
		Port:        getenv("EDGE_PORT", "8082"),
		DBHost:      getenv("DB_HOST", "localhost"),
		DBPort:      getenv("DB_PORT", "5432"),
		DBUser:      getenv("DB_USER", "media"),
		DBPassword:  getenv("DB_PASSWORD", "media_pass"),
		DBName:      getenv("DB_NAME", "hls"),
		OriginURL:   getenv("ORIGIN_URL", "http://origin:8081"),
		TopologyURL: getenv("TOPOLOGY_URL", "http://topology:8090"),
		ConnectedPeers: connectedPeers,
	}
}

func getenv(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func (a *edgeApp) initDB(ctx context.Context) error {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		a.cfg.DBHost, a.cfg.DBPort, a.cfg.DBUser, a.cfg.DBPassword, a.cfg.DBName)
	
	var err error
	a.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Retry connection with timeout
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if err := a.db.PingContext(ctx); err == nil {
			break
		}
		if i < maxRetries-1 {
			log.Printf("[%s] Database not ready, retrying in 2 seconds... (%d/%d)", a.cfg.Name, i+1, maxRetries)
			time.Sleep(2 * time.Second)
		} else {
			return fmt.Errorf("failed to ping database after %d retries: %w", maxRetries, err)
		}
	}

	// Create segments table (same as origin)
	createTable := `
	CREATE TABLE IF NOT EXISTS segments (
		id VARCHAR(255) PRIMARY KEY,
		song_id VARCHAR(255) NOT NULL,
		bitrate VARCHAR(50),
		segment_index INTEGER,
		data BYTEA NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_song_id ON segments(song_id);
	CREATE INDEX IF NOT EXISTS idx_segment_id ON segments(id);
	`
	
	if _, err := a.db.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	log.Printf("[%s] Database initialized", a.cfg.Name)
	return nil
}

func (a *edgeApp) fetchFromOrigin(ctx context.Context, segmentID string) ([]byte, error) {
	url := fmt.Sprintf("%s/segments/%s", a.cfg.OriginURL, segmentID)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("origin returned status %d", resp.StatusCode)
	}

	var segResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&segResp); err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(segResp["payload"])
	if err != nil {
		return nil, fmt.Errorf("invalid base64 payload: %w", err)
	}

	// Store in edge database (unlimited cache)
	parts := strings.Split(segmentID, "/")
	songID := ""
	bitrate := ""
	segmentIndex := 0
	
	if len(parts) >= 3 {
		songID = parts[0]
		bitrate = parts[1]
		// Extract index from segment name (e.g., segment000.ts -> 0)
		segName := parts[2]
		if strings.HasPrefix(segName, "segment") && strings.HasSuffix(segName, ".ts") {
			fmt.Sscanf(segName, "segment%d.ts", &segmentIndex)
		}
	}

	_, err = a.db.ExecContext(ctx,
		"INSERT INTO segments (id, song_id, bitrate, segment_index, data) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data",
		segmentID, songID, bitrate, segmentIndex, data)
	if err != nil {
		log.Printf("[%s] Warning: failed to cache segment %s: %v", a.cfg.Name, segmentID, err)
	}

	log.Printf("[%s] Fetched and cached segment %s from origin", a.cfg.Name, segmentID)
	return data, nil
}

func (a *edgeApp) fetchSongFromOrigin(ctx context.Context, songID string) error {
	// Fetch all segments for a song from origin
	url := fmt.Sprintf("%s/songs/%s", a.cfg.OriginURL, songID)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("origin returned status %d", resp.StatusCode)
	}

	var songResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&songResp); err != nil {
		return err
	}

	segments, ok := songResp["segments"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid response format")
	}

	log.Printf("[%s] Fetching %d segments for song %s from origin...", a.cfg.Name, len(segments), songID)

	// Fetch each segment
	for _, seg := range segments {
		segMap, ok := seg.(map[string]interface{})
		if !ok {
			continue
		}
		segmentID, ok := segMap["id"].(string)
		if !ok {
			continue
		}
		_, err := a.fetchFromOrigin(ctx, segmentID)
		if err != nil {
			log.Printf("[%s] Warning: failed to fetch segment %s: %v", a.cfg.Name, segmentID, err)
		}
	}

	log.Printf("[%s] Successfully cached all segments for song %s", a.cfg.Name, songID)
	return nil
}

func (a *edgeApp) startHTTP(ctx context.Context) *http.Server {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "%s: ok", a.cfg.Name)
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

		// Try to fetch from edge cache first
		var data []byte
		err := a.db.QueryRowContext(r.Context(),
			"SELECT data FROM segments WHERE id = $1", segmentID).Scan(&data)
		
		if err == sql.ErrNoRows {
			// Not in cache, fetch from origin
			log.Printf("[%s] Segment %s not in cache, fetching from origin...", a.cfg.Name, segmentID)
			data, err = a.fetchFromOrigin(r.Context(), segmentID)
			if err != nil {
				log.Printf("[%s] Failed to fetch segment %s from origin: %v", a.cfg.Name, segmentID, err)
				http.Error(w, "segment not found", http.StatusNotFound)
				return
			}
		} else if err != nil {
			log.Printf("[%s] Error fetching segment %s: %v", a.cfg.Name, segmentID, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Return segment as base64 JSON
		resp := map[string]string{
			"id":      segmentID,
			"payload": base64.StdEncoding.EncodeToString(data),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/songs/", func(w http.ResponseWriter, r *http.Request) {
		// Fetch entire song from origin and cache it, then return segment list
		songID := strings.TrimPrefix(r.URL.Path, "/songs/")
		if songID == "" {
			http.Error(w, "song id required", http.StatusBadRequest)
			return
		}

		// First, fetch from origin to ensure we have it
		if err := a.fetchSongFromOrigin(r.Context(), songID); err != nil {
			log.Printf("[%s] Failed to fetch song %s: %v", a.cfg.Name, songID, err)
			http.Error(w, "failed to fetch song", http.StatusInternalServerError)
			return
		}

		// Return list of segments
		rows, err := a.db.QueryContext(r.Context(),
			"SELECT id, segment_index FROM segments WHERE song_id = $1 ORDER BY segment_index", songID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var segments []map[string]interface{}
		for rows.Next() {
			var id string
			var index int
			if err := rows.Scan(&id, &index); err != nil {
				continue
			}
			segments = append(segments, map[string]interface{}{
				"id":    id,
				"index": index,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "cached",
			"song_id":  songID,
			"segments": segments,
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

func main() {
	cfg := loadConfig()
	app := &edgeApp{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize database
	if err := app.initDB(ctx); err != nil {
		log.Fatalf("[%s] Failed to initialize database: %v", cfg.Name, err)
	}

	// Start HTTP server
	app.startHTTP(context.Background())

	// Register with topology service
	go app.registerWithTopology(context.Background())

	// Wait for shutdown
	select {}
}

func (a *edgeApp) registerWithTopology(ctx context.Context) {
	// Wait a bit for topology service to be ready
	time.Sleep(2 * time.Second)
	
	if a.cfg.TopologyURL == "" {
		return
	}
	
	// Retry registration with exponential backoff
	maxRetries := 5
	client := &http.Client{Timeout: 5 * time.Second}
	
	for i := 0; i < maxRetries; i++ {
		// Use connected peers from config
		payload := map[string]interface{}{
			"peer_id":   a.cfg.Name,
			"region":    "global",
			"neighbors": a.cfg.ConnectedPeers,
		}
		
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TopologyURL+"/edges", strings.NewReader(string(body)))
		if err != nil {
			log.Printf("[%s] Failed to create topology request: %v", a.cfg.Name, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := client.Do(req)
		if err != nil {
			if i < maxRetries-1 {
				waitTime := time.Duration(1<<uint(i)) * time.Second // Exponential backoff: 1s, 2s, 4s, 8s
				log.Printf("[%s] Failed to register with topology (attempt %d/%d), retrying in %v: %v", a.cfg.Name, i+1, maxRetries, waitTime, err)
				time.Sleep(waitTime)
				continue
			}
			log.Printf("[%s] Failed to register with topology after %d attempts: %v", a.cfg.Name, maxRetries, err)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
			log.Printf("[%s] Registered with topology service", a.cfg.Name)
			return
		}
		
		if i < maxRetries-1 {
			waitTime := time.Duration(1<<uint(i)) * time.Second
			log.Printf("[%s] Topology returned status %d (attempt %d/%d), retrying in %v", a.cfg.Name, resp.StatusCode, i+1, maxRetries, waitTime)
			time.Sleep(waitTime)
		}
	}
}

