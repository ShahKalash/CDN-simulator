package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Song struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Duration    float64   `json:"duration"`
	Bitrates    []string  `json:"bitrates"`
	Segments    []string  `json:"segments"`
	UploadTime  time.Time `json:"uploadTime"`
	Status      string    `json:"status"` // "processing", "ready", "error"
	PlaylistURL string    `json:"playlistUrl"`
}

type SongManager struct {
	songs map[string]*Song
	mu    sync.RWMutex
}

var songManager = &SongManager{
	songs: make(map[string]*Song),
}

func (sm *SongManager) AddSong(song *Song) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.songs[song.ID] = song
}

func (sm *SongManager) GetSong(id string) *Song {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.songs[id]
}

func (sm *SongManager) GetAllSongs() map[string]*Song {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	songsCopy := make(map[string]*Song, len(sm.songs))
	for id, song := range sm.songs {
		songsCopy[id] = song
	}
	return songsCopy
}

func (sm *SongManager) UpdateSongStatus(id, status string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if song, exists := sm.songs[id]; exists {
		song.Status = status
	}
}

func (sm *SongManager) UpdateSongSegments(id string, segments []string, bitrates []string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if song, exists := sm.songs[id]; exists {
		song.Segments = segments
		song.Bitrates = bitrates
		song.Status = "ready"
	}
}

func processAudioFile(file multipart.File, header *multipart.FileHeader, title, artist string) (*Song, error) {
	// Create unique song ID
	songID := fmt.Sprintf("song_%d", time.Now().UnixNano())

	// Create song directory
	songDir := filepath.Join("assets", "songs", songID)
	if err := os.MkdirAll(songDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create song directory: %w", err)
	}

	// Save uploaded file
	uploadPath := filepath.Join(songDir, header.Filename)
	dst, err := os.Create(uploadPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return nil, fmt.Errorf("failed to save file: %w", err)
	}

	// Create song object
	song := &Song{
		ID:         songID,
		Title:      title,
		Artist:     artist,
		UploadTime: time.Now(),
		Status:     "processing",
	}

	// Process audio in background
	go func() {
		if err := processAudioToHLS(uploadPath, songDir, songID); err != nil {
			log.Printf("Error processing audio for song %s: %v", songID, err)
			songManager.UpdateSongStatus(songID, "error")
			return
		}

		// Generate segments list
		segments := []string{}
		for i := 0; i < 8; i++ { // Assuming 8 segments
			segments = append(segments, fmt.Sprintf("segment%03d.ts", i))
		}

		bitrates := []string{"128k", "192k"}
		songManager.UpdateSongSegments(songID, segments, bitrates)

		// Update network topology with new segments
		updateNetworkTopology(songID, segments)

		log.Printf("✅ Song %s processed successfully", songID)
	}()

	return song, nil
}

func processAudioToHLS(inputPath, outputDir, songID string) error {
	bitrates := []string{"128k", "192k"}

	for _, bitrate := range bitrates {
		bitrateDir := filepath.Join(outputDir, bitrate)
		if err := os.MkdirAll(bitrateDir, 0755); err != nil {
			return fmt.Errorf("failed to create bitrate directory: %w", err)
		}

		playlistFile := filepath.Join(bitrateDir, "playlist.m3u8")
		segmentPattern := filepath.Join(bitrateDir, "segment%03d.ts")

		// Use FFmpeg to create HLS segments
		// It is expected that ffmpeg is installed in the system which is running this.
		cmd := exec.Command("ffmpeg",
			"-i", inputPath,
			"-c:a", "aac",
			"-b:a", bitrate,
			"-hls_time", "4",
			"-hls_playlist_type", "vod",
			"-hls_segment_filename", segmentPattern,
			playlistFile,
		)

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("FFmpeg failed for %s: %w", bitrate, err)
		}
	}

	return nil
}

func updateNetworkTopology(songID string, segments []string) {
	// Add new segments to origin server
	for _, segment := range segments {
		reqBody := map[string]string{
			"nodeId":    "origin-1",
			"segmentId": segment,
		}

		jsonData, _ := json.Marshal(reqBody)
		resp, err := http.Post("http://localhost:8092/add-segment", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Failed to add segment %s to origin: %v", segment, err)
		} else {
			resp.Body.Close()
		}
	}

	// Distribute some segments to edge servers
	edgeServers := []string{"edge-1", "edge-2", "edge-3", "edge-4"}
	for i, segment := range segments {
		if i < len(edgeServers) {
			edgeID := edgeServers[i]
			reqBody := map[string]string{
				"nodeId":    edgeID,
				"segmentId": segment,
			}

			jsonData, _ := json.Marshal(reqBody)
			resp, err := http.Post("http://localhost:8092/add-segment", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				log.Printf("Failed to add segment %s to edge %s: %v", segment, edgeID, err)
			} else {
				resp.Body.Close()
			}
		}
	}
}

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

func main() {
	port := getenv("PORT", "8093")

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(corsMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Get all songs
	r.Get("/songs", func(w http.ResponseWriter, r *http.Request) {
		songs := songManager.GetAllSongs()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(songs)
	})

	// Get specific song
	r.Get("/songs/{id}", func(w http.ResponseWriter, r *http.Request) {
		songID := chi.URLParam(r, "id")
		song := songManager.GetSong(songID)
		if song == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(song)
	})

	// Upload new song
	r.Post("/upload", func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form
		err := r.ParseMultipartForm(32 << 20) // 32 MB max
		if err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// Get form values
		title := r.FormValue("title")
		artist := r.FormValue("artist")
		if title == "" || artist == "" {
			http.Error(w, "Title and artist are required", http.StatusBadRequest)
			return
		}

		// Get uploaded file
		file, header, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "No audio file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Check file type
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".mp3") &&
			!strings.HasSuffix(strings.ToLower(header.Filename), ".wav") &&
			!strings.HasSuffix(strings.ToLower(header.Filename), ".m4a") {
			http.Error(w, "Only MP3, WAV, and M4A files are supported", http.StatusBadRequest)
			return
		}

		// Process the file
		song, err := processAudioFile(file, header, title, artist)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to process audio: %v", err), http.StatusInternalServerError)
			return
		}

		// Add to song manager
		songManager.AddSong(song)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(song)
	})

	// Serve HLS files
	r.Handle("/hls/*", http.StripPrefix("/hls/", http.FileServer(http.Dir("assets/songs/"))))

	log.Printf("Song Manager service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
