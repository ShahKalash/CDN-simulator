package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type peerConfig struct {
	Name      string
	Port      string
	Neighbors []string
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
	return peerConfig{
		Name:      name,
		Port:      port,
		Neighbors: neighbors,
	}
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

func getenv(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func startHTTPServer(cfg peerConfig, healthCh chan<- struct{}) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s: ok", cfg.Name)
	})
	mux.HandleFunc("/peers", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, strings.Join(cfg.Neighbors, ","))
	})
	mux.HandleFunc("/name", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, cfg.Name)
	})

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		log.Printf("[%s] HTTP listening on %s", cfg.Name, server.Addr)
		healthCh <- struct{}{}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", cfg.Name, err)
		}
	}()
	return server
}

func startPeerDialer(ctx context.Context, cfg peerConfig) {
	ticker := time.NewTicker(5 * time.Second)
	client := http.Client{Timeout: 3 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, neighbor := range cfg.Neighbors {
				url := fmt.Sprintf("http://%s:%s/health", neighbor, cfg.Port)
				resp, err := client.Get(url)
				if err != nil {
					log.Printf("[%s] neighbor %s unreachable: %v", cfg.Name, neighbor, err)
					continue
				}
				resp.Body.Close()
				log.Printf("[%s] neighbor %s reachable", cfg.Name, neighbor)
			}
		}
	}
}

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	ready := make(chan struct{})
	server := startHTTPServer(cfg, ready)

	go func() {
		<-ready
		wg.Add(1)
		go func() {
			defer wg.Done()
			startPeerDialer(ctx, cfg)
		}()
	}()

	waitForShutdown(server, cancel, &wg, cfg.Name)
}

func waitForShutdown(server *http.Server, cancel context.CancelFunc, wg *sync.WaitGroup, name string) {
	quit := make(chan os.Signal, 1)
	// In BusyBox/Alpine we may not have full signal set; rely on context cancel via SIGTERM.
	signalNotify(quit)
	s := <-quit
	log.Printf("[%s] shutting down (%v)", name, s)
	cancel()
	shutdownCtx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelTimeout()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[%s] graceful shutdown failed: %v", name, err)
	}
	wg.Wait()
}

// signalNotify isolated for build tags where syscall is allowed.
func signalNotify(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
}
