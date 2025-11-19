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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type originConfig struct {
	Port         string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	SongPath     string
	SegmentDir   string
}

type originApp struct {
	cfg    originConfig
	db     *sql.DB
	server *http.Server
}

func loadConfig() originConfig {
	return originConfig{
		Port:         getenv("ORIGIN_PORT", "8081"),
		DBHost:       getenv("DB_HOST", "localhost"),
		DBPort:       getenv("DB_PORT", "5432"),
		DBUser:       getenv("DB_USER", "media"),
		DBPassword:   getenv("DB_PASSWORD", "media_pass"),
		DBName:       getenv("DB_NAME", "hls"),
		SongPath:     getenv("SONG_PATH", "Rick-Roll-Sound-Effect.mp3"),
		SegmentDir:   getenv("SEGMENT_DIR", "./segments"),
	}
}

func getenv(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (a *originApp) initDB(ctx context.Context) error {
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
			log.Printf("[origin] Database not ready, retrying in 2 seconds... (%d/%d)", i+1, maxRetries)
			time.Sleep(2 * time.Second)
		} else {
			return fmt.Errorf("failed to ping database after %d retries: %w", maxRetries, err)
		}
	}

	// Create segments table
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

	log.Printf("[origin] Database initialized")
	return nil
}

func (a *originApp) segmentSong(ctx context.Context) error {
	songPath := a.cfg.SongPath
	// Try multiple possible paths
	possiblePaths := []string{
		songPath,
		filepath.Join(".", songPath),
		filepath.Join("/home/origin", songPath),
		"Rick-Roll-Sound-Effect.mp3",
		filepath.Join(".", "Rick-Roll-Sound-Effect.mp3"),
	}
	
	var actualPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			actualPath = path
			break
		}
	}
	
	if actualPath == "" {
		return fmt.Errorf("song file not found in any of: %v", possiblePaths)
	}
	
	songPath = actualPath

	// Create segment directory
	if err := os.MkdirAll(a.cfg.SegmentDir, 0755); err != nil {
		return fmt.Errorf("failed to create segment dir: %w", err)
	}

	songID := "rickroll"
	bitrate := "128k"
	outputDir := filepath.Join(a.cfg.SegmentDir, songID, bitrate)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// Check if segments already exist in DB
	var count int
	err := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM segments WHERE song_id = $1", songID).Scan(&count)
	if err == nil && count > 0 {
		log.Printf("[origin] Segments for %s already exist in database (%d segments)", songID, count)
		return nil
	}

	log.Printf("[origin] Segmenting song: %s (found at: %s)", a.cfg.SongPath, songPath)
	
	// Find ffmpeg executable
	ffmpegPath := "ffmpeg"
	if portableFFmpeg := filepath.Join("ffmpeg-portable", "ffmpeg-8.0-essentials_build", "bin", "ffmpeg.exe"); 
		fileExists(portableFFmpeg) {
		ffmpegPath = portableFFmpeg
	} else if portableFFmpegLinux := filepath.Join("ffmpeg-portable", "ffmpeg-8.0-essentials_build", "bin", "ffmpeg");
		fileExists(portableFFmpegLinux) {
		ffmpegPath = portableFFmpegLinux
	}
	
	// Use ffmpeg to create HLS segments
	playlistPath := filepath.Join(outputDir, "playlist.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment%03d.ts")
	
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-i", songPath,
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "hls",
		"-hls_time", "10",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Read and store segments in database
	segmentFiles, err := filepath.Glob(filepath.Join(outputDir, "segment*.ts"))
	if err != nil {
		return fmt.Errorf("failed to glob segments: %w", err)
	}

	log.Printf("[origin] Found %d segments, storing in database...", len(segmentFiles))

	for i, segFile := range segmentFiles {
		data, err := os.ReadFile(segFile)
		if err != nil {
			log.Printf("[origin] Warning: failed to read segment %s: %v", segFile, err)
			continue
		}

		// Generate segment ID: song_id/bitrate/segment_filename
		segName := filepath.Base(segFile)
		segmentID := fmt.Sprintf("%s/%s/%s", songID, bitrate, segName)

		// Store in database
		_, err = a.db.ExecContext(ctx,
			"INSERT INTO segments (id, song_id, bitrate, segment_index, data) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data",
			segmentID, songID, bitrate, i, data)
		if err != nil {
			log.Printf("[origin] Warning: failed to store segment %s: %v", segmentID, err)
			continue
		}
	}

	log.Printf("[origin] Successfully stored %d segments in database", len(segmentFiles))
	return nil
}

func (a *originApp) startHTTP(ctx context.Context) *http.Server {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "origin: ok")
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

		// Fetch segment from database
		var data []byte
		err := a.db.QueryRowContext(r.Context(),
			"SELECT data FROM segments WHERE id = $1", segmentID).Scan(&data)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("[origin] Error fetching segment %s: %v", segmentID, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Return segment as base64 JSON (same format as peers)
		resp := map[string]string{
			"id":      segmentID,
			"payload": base64.StdEncoding.EncodeToString(data),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/songs/", func(w http.ResponseWriter, r *http.Request) {
		// Get all segments for a song
		songID := strings.TrimPrefix(r.URL.Path, "/songs/")
		if songID == "" {
			http.Error(w, "song id required", http.StatusBadRequest)
			return
		}

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
		log.Printf("[origin] HTTP listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[origin] server error: %v", err)
		}
	}()

	return server
}

func main() {
	cfg := loadConfig()
	app := &originApp{cfg: cfg}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize database
	if err := app.initDB(ctx); err != nil {
		log.Fatalf("[origin] Failed to initialize database: %v", err)
	}

	// Segment song and store in database
	if err := app.segmentSong(ctx); err != nil {
		log.Fatalf("[origin] Failed to segment song: %v", err)
	}

	// Start HTTP server
	app.startHTTP(context.Background())

	// Wait for shutdown
	select {}
}

