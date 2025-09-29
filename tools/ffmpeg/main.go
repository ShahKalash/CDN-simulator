package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Checksum struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func run(cmd string, args ...string) error {
	c := exec.CommandContext(context.Background(), cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// This tool expects ffmpeg installed on PATH.
// Usage:
//
//	ffmpeg-tool <input_audio_or_video> <output_dir> <bitrates_csv>
//
// Example:
//
//	ffmpeg-tool assets/input/song.mp3 assets/hls/song "128k,192k,256k"
func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: ffmpeg-tool <input> <out_dir> <bitrates_csv>\n")
		os.Exit(2)
	}
	in := os.Args[1]
	outDir := os.Args[2]
	bitrates := strings.Split(os.Args[3], ",")

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	checksums := make([]Checksum, 0, 128)

	// For each bitrate, create HLS ladder directory and segments
	for _, br := range bitrates {
		br = strings.TrimSpace(br)
		variantDir := filepath.Join(outDir, br)
		if err := os.MkdirAll(variantDir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Produce HLS segments: 2s segments, fMP4
		// master.m3u8 will be assembled later; here we produce variant playlist
		// Example ffmpeg command for fMP4 HLS:
		// ffmpeg -i in -c:a aac -b:a 128k -hls_time 2 -hls_playlist_type vod \
		//   -hls_segment_type fmp4 -master_pl_name master.m3u8 -var_stream_map "a:0,name:audio" \
		//   -f hls 128k/index.m3u8
		variantIndex := filepath.Join(variantDir, "index.m3u8")
		args := []string{
			"-y",
			"-i", in,
			"-c:a", "aac",
			"-b:a", br,
			"-hls_time", "2",
			"-hls_playlist_type", "vod",
			"-hls_segment_type", "fmp4",
			"-f", "hls",
			variantIndex,
		}
		if err := run("ffmpeg", args...); err != nil {
			fmt.Fprintf(os.Stderr, "ffmpeg failed for %s: %v\n", br, err)
			os.Exit(1)
		}

		// Hash all files in variantDir
		if err := filepath.WalkDir(variantDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			sum, err := hashFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(outDir, path)
			checksums = append(checksums, Checksum{Path: rel, SHA256: sum})
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "hash walk failed for %s: %v\n", br, err)
			os.Exit(1)
		}
	}

	// Write master playlist that references all variants by bitrate directory
	master := "#EXTM3U\n"
	for _, br := range bitrates {
		br = strings.TrimSpace(br)
		master += fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%s\n%s/index.m3u8\n", strings.TrimSuffix(br, "k")+"000", br)
	}
	if err := os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte(master), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Write checksums
	b, _ := json.MarshalIndent(checksums, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "checksums.json"), b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
