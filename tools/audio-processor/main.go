package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type AudioSegment struct {
	SegmentID    string    `json:"segmentId"`
	FilePath     string    `json:"filePath"`
	Size         int64     `json:"size"`
	Duration     float64   `json:"duration"`
	SHA256       string    `json:"sha256"`
	Bitrate      string    `json:"bitrate"`
	SegmentIndex int       `json:"segmentIndex"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AudioManifest struct {
	SongID       string         `json:"songId"`
	Title        string         `json:"title"`
	TotalDuration float64       `json:"totalDuration"`
	Bitrates     []string       `json:"bitrates"`
	Segments     []AudioSegment `json:"segments"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: audio-processor <input_audio_file> [bitrates]")
		fmt.Println("Example: audio-processor song.mp3 128k,192k,256k")
		os.Exit(1)
	}

	inputFile := os.Args[1]
	bitrates := []string{"128k", "192k", "256k"}
	
	if len(os.Args) > 2 {
		bitrates = strings.Split(os.Args[2], ",")
		for i, br := range bitrates {
			bitrates[i] = strings.TrimSpace(br)
		}
	}

	fmt.Printf("🎵 Processing audio file: %s\n", inputFile)
	fmt.Printf("📊 Bitrates: %v\n", bitrates)

	// Create output directories
	outputDir := "assets/audio-segments"
	os.MkdirAll(outputDir, 0755)

	// Process each bitrate
	for _, bitrate := range bitrates {
		fmt.Printf("\n🔄 Processing %s bitrate...\n", bitrate)
		processBitrate(inputFile, bitrate, outputDir)
	}

	fmt.Println("\n✅ Audio processing complete!")
	fmt.Println("📁 Segments stored in:", outputDir)
}

func processBitrate(inputFile, bitrate, outputDir string) {
	bitrateDir := filepath.Join(outputDir, bitrate)
	os.MkdirAll(bitrateDir, 0755)

	// Create HLS segments using FFmpeg
	playlistFile := filepath.Join(bitrateDir, "playlist.m3u8")
	segmentPattern := filepath.Join(bitrateDir, "segment%03d.ts")

	cmd := exec.Command("ffmpeg-portable/ffmpeg-8.0-essentials_build/bin/ffmpeg.exe",
		"-i", inputFile,
		"-c:a", "aac",
		"-b:a", bitrate,
		"-hls_time", "4", // 4-second segments
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segmentPattern,
		"-f", "hls",
		playlistFile,
		"-y", // Overwrite output files
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("FFmpeg failed for %s: %v", bitrate, err)
	}

	// Process generated segments
	processSegments(bitrateDir, bitrate)
}

func processSegments(bitrateDir, bitrate string) {
	files, err := filepath.Glob(filepath.Join(bitrateDir, "segment*.ts"))
	if err != nil {
		log.Printf("Error finding segments: %v", err)
		return
	}

	fmt.Printf("📦 Found %d segments for %s\n", len(files), bitrate)

	for i, file := range files {
		segment := AudioSegment{
			SegmentID:    generateSegmentID(filepath.Base(file)),
			FilePath:     file,
			Bitrate:      bitrate,
			SegmentIndex: i,
			CreatedAt:    time.Now(),
		}

		// Get file size
		if stat, err := os.Stat(file); err == nil {
			segment.Size = stat.Size()
		}

		// Calculate SHA256
		if hash, err := calculateSHA256(file); err == nil {
			segment.SHA256 = hash
		}

		// Estimate duration (4 seconds per segment)
		segment.Duration = 4.0

		fmt.Printf("  📄 %s (%.2f KB, %s)\n", 
			segment.SegmentID, 
			float64(segment.Size)/1024, 
			segment.SHA256[:8])
	}
}

func generateSegmentID(filename string) string {
	// Extract song name and segment number
	base := strings.TrimSuffix(filename, ".ts")
	parts := strings.Split(base, "segment")
	if len(parts) == 2 {
		return fmt.Sprintf("song_%s", parts[1])
	}
	return fmt.Sprintf("song_%s", base)
}

func calculateSHA256(filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
