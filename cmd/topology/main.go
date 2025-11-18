package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud_project/internal/topology"
)

type upsertRequest struct {
	PeerID    string         `json:"peer_id"`
	Region    string         `json:"region"`
	RTTms     int            `json:"rtt_ms"`
	Neighbors []string       `json:"neighbors"`
	Metadata  map[string]any `json:"metadata"`
}

func main() {
	addr := env("TOPOLOGY_ADDR", ":8090")
	graph := topology.NewGraph()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req upsertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.PeerID == "" {
			http.Error(w, "peer_id required", http.StatusBadRequest)
			return
		}
		graph.Upsert(req.PeerID, req.Region, req.RTTms, req.Neighbors, req.Metadata)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/peers/", func(w http.ResponseWriter, r *http.Request) {
		peerID := strings.TrimPrefix(r.URL.Path, "/peers/")
		if peerID == "" {
			http.Error(w, "peer id required", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		graph.Remove(peerID)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
		topology.WriteJSON(w, http.StatusOK, graph.Snapshot())
	})
	mux.HandleFunc("/path", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from == "" || to == "" {
			http.Error(w, "from/to required", http.StatusBadRequest)
			return
		}
		path, err := graph.BFS(from, to)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		topology.WriteJSON(w, http.StatusOK, map[string]any{
			"path": path,
		})
	})

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	log.Printf("Topology manager listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("topology server error: %v", err)
	}
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
