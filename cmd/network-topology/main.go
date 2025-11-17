package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kalash/CDN-simulator/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// NetworkNode represents any node in the network (Origin, Edge, or Peer)
type NetworkNode struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"` // "origin", "edge", "peer"
	Region      string          `json:"region"`
	Connections []string        `json:"connections"` // Connected node IDs
	Storage     map[string]bool `json:"storage"`     // Segments this node has
	Memory      int64           `json:"memory"`      // Available memory in bytes
	MaxMemory   int64           `json:"maxMemory"`   // Maximum memory capacity
	Latency     map[string]int  `json:"latency"`     // Latency to other nodes (ms)
	IsOnline    bool            `json:"isOnline"`
	LastSeen    time.Time       `json:"lastSeen"`
}

// NetworkTopology manages the entire network structure
type NetworkTopology struct {
	mu    sync.RWMutex
	nodes map[string]*NetworkNode
	edges map[string][]string // Adjacency list for routing
}

type statusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.Status = code
	r.ResponseWriter.WriteHeader(code)
}

// Request represents a content request
type Request struct {
	RequestID string    `json:"requestId"`
	FromNode  string    `json:"fromNode"`
	SegmentID string    `json:"segmentId"`
	Timestamp time.Time `json:"timestamp"`
	Hops      int       `json:"hops"`
	Path      []string  `json:"path"`
	Source    string    `json:"source"` // "peer", "edge", "origin"
}

// RequestResponse represents the response to a request
type RequestResponse struct {
	RequestID string    `json:"requestId"`
	Success   bool      `json:"success"`
	Source    string    `json:"source"`
	Latency   int       `json:"latency"`
	Hops      int       `json:"hops"`
	Path      []string  `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

var metricsObj *metrics.Metrics

func NewNetworkTopology() *NetworkTopology {
	return &NetworkTopology{
		nodes: make(map[string]*NetworkNode),
		edges: make(map[string][]string),
	}
}

func (nt *NetworkTopology) AddNode(node *NetworkNode) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	nt.nodes[node.ID] = node
	nt.edges[node.ID] = make([]string, 0)
}

func (nt *NetworkTopology) ConnectNodes(node1ID, node2ID string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	if node1, exists := nt.nodes[node1ID]; exists {
		node1.Connections = append(node1.Connections, node2ID)
		nt.edges[node1ID] = append(nt.edges[node1ID], node2ID)
	}

	if node2, exists := nt.nodes[node2ID]; exists {
		node2.Connections = append(node2.Connections, node1ID)
		nt.edges[node2ID] = append(nt.edges[node2ID], node1ID)
	}
}

func (nt *NetworkTopology) FindShortestPath(from, to string) ([]string, int) {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	// Handle same node case
	if from == to {
		return []string{from}, 0
	}

	// Simple BFS for shortest path
	queue := []string{from}
	visited := make(map[string]bool)
	parent := make(map[string]string)

	visited[from] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == to {
			// Reconstruct path
			path := []string{}
			node := to
			for node != "" {
				path = append([]string{node}, path...)
				node = parent[node]
			}
			hops := len(path) - 1
			if hops < 0 {
				hops = 0 // Same node = 0 hops
			}
			return path, hops
		}

		for _, neighbor := range nt.edges[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				parent[neighbor] = current
				queue = append(queue, neighbor)
			}
		}
	}

	return nil, -1
}

func (nt *NetworkTopology) FindSegment(segmentID, fromNode string) *RequestResponse {
	nt.mu.RLock()
	defer nt.mu.RUnlock()
	final_src := ""

	// Strategy: Check P2P peers first, then edge servers, then origin
	response := &RequestResponse{
		RequestID: fmt.Sprintf("req_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
	}

	defer func() {
		metricsObj.SegmentResponseTime.
			WithLabelValues(final_src).
			Observe(float64(response.Latency) / 1000.0) // histograms expects seconds
		// we hack the latency coversion which looks more like in ms
	}()

	// Convert segmentID to alternative format for checking
	var altSegmentID string
	if len(segmentID) >= 12 && segmentID[:8] == "segment" {
		// Convert "segment003.ts" to "song_003"
		var segmentNum int
		fmt.Sscanf(segmentID, "segment%03d.ts", &segmentNum)
		altSegmentID = fmt.Sprintf("song_%03d", segmentNum)
	} else if len(segmentID) >= 6 && segmentID[:4] == "song" {
		// Convert "song_003" to "segment003.ts"
		var segmentNum int
		fmt.Sscanf(segmentID, "song_%03d", &segmentNum)
		altSegmentID = fmt.Sprintf("segment%03d.ts", segmentNum)
	}

	// 1. Check P2P peers (within 3 hops) - check both formats
	peerNodes := nt.findPeersWithSegment(segmentID, fromNode, 3)
	if len(peerNodes) == 0 && altSegmentID != "" {
		peerNodes = nt.findPeersWithSegment(altSegmentID, fromNode, 3)
	}
	if len(peerNodes) > 0 {
		// Find closest peer
		closestPeer := nt.findClosestNode(fromNode, peerNodes)
		if closestPeer != "" {
			final_src = "peer"
			metricsObj.NodeRequests.WithLabelValues("peer", "hit").Inc()
			path, hops := nt.FindShortestPath(fromNode, closestPeer)
			response.Success = true
			response.Source = "peer"
			response.Hops = hops
			response.Path = path
			response.Latency = nt.calculateLatency(path)
			return response
		}
	}
	metricsObj.NodeRequests.WithLabelValues("peer", "miss").Inc()

	// 2. Check edge servers - check both formats
	edgeNodes := nt.findEdgeServersWithSegment(segmentID)
	if len(edgeNodes) == 0 && altSegmentID != "" {
		edgeNodes = nt.findEdgeServersWithSegment(altSegmentID)
	}
	if len(edgeNodes) > 0 {
		closestEdge := nt.findClosestNode(fromNode, edgeNodes)
		if closestEdge != "" {
			final_src = "edge"
			metricsObj.NodeRequests.WithLabelValues("edge", "hit").Inc()
			path, hops := nt.FindShortestPath(fromNode, closestEdge)
			response.Success = true
			response.Source = "edge"
			response.Hops = hops
			response.Path = path
			response.Latency = nt.calculateLatency(path)
			return response
		}
	}
	metricsObj.NodeRequests.WithLabelValues("edge", "miss").Inc()

	// 3. Check origin server - check both formats
	originNodes := nt.findOriginServersWithSegment(segmentID)
	if len(originNodes) == 0 && altSegmentID != "" {
		originNodes = nt.findOriginServersWithSegment(altSegmentID)
	}
	if len(originNodes) > 0 {
		closestOrigin := nt.findClosestNode(fromNode, originNodes)
		if closestOrigin != "" {
			final_src = "origin"
			metricsObj.NodeRequests.WithLabelValues("origin", "hit").Inc()
			path, hops := nt.FindShortestPath(fromNode, closestOrigin)
			response.Success = true
			response.Source = "origin"
			response.Hops = hops
			response.Path = path
			response.Latency = nt.calculateLatency(path)
			return response
		}
	}
	metricsObj.NodeRequests.WithLabelValues("origin", "miss").Inc()

	response.Success = false
	return response
}

func (nt *NetworkTopology) findPeersWithSegment(segmentID, fromNode string, maxHops int) []string {
	var peers []string

	// BFS to find peers within maxHops
	queue := []string{fromNode}
	visited := make(map[string]bool)
	hops := make(map[string]int)

	visited[fromNode] = true
	hops[fromNode] = 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if hops[current] >= maxHops {
			continue
		}

		if node, exists := nt.nodes[current]; exists && node.Type == "peer" {
			// Check both segment formats
			if node.Storage[segmentID] {
				peers = append(peers, current)
			} else {
				// Check alternative format
				var altSegmentID string
				if len(segmentID) >= 12 && segmentID[:8] == "segment" {
					// Convert "segment003.ts" to "song_003"
					var segmentNum int
					fmt.Sscanf(segmentID, "segment%03d.ts", &segmentNum)
					altSegmentID = fmt.Sprintf("song_%03d", segmentNum)
				} else if len(segmentID) >= 6 && segmentID[:4] == "song" {
					// Convert "song_003" to "segment003.ts"
					var segmentNum int
					fmt.Sscanf(segmentID, "song_%03d", &segmentNum)
					altSegmentID = fmt.Sprintf("segment%03d.ts", segmentNum)
				}

				if altSegmentID != "" && node.Storage[altSegmentID] {
					peers = append(peers, current)
				}
			}
		}

		for _, neighbor := range nt.edges[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				hops[neighbor] = hops[current] + 1
				queue = append(queue, neighbor)
			}
		}
	}

	return peers
}

func (nt *NetworkTopology) findEdgeServersWithSegment(segmentID string) []string {
	var edges []string

	for _, node := range nt.nodes {
		if node.Type == "edge" && node.Storage[segmentID] {
			edges = append(edges, node.ID)
		}
	}

	return edges
}

func (nt *NetworkTopology) findOriginServers() []string {
	var origins []string

	for _, node := range nt.nodes {
		if node.Type == "origin" {
			origins = append(origins, node.ID)
		}
	}

	return origins
}

func (nt *NetworkTopology) findOriginServersWithSegment(segmentID string) []string {
	var origins []string

	for _, node := range nt.nodes {
		if node.Type == "origin" && node.Storage[segmentID] {
			origins = append(origins, node.ID)
		}
	}

	return origins
}

func (nt *NetworkTopology) findClosestNode(fromNode string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	closest := candidates[0]
	minHops := 999

	for _, candidate := range candidates {
		_, hops := nt.FindShortestPath(fromNode, candidate)
		if hops < minHops && hops > 0 {
			minHops = hops
			closest = candidate
		}
	}

	return closest
}

func (nt *NetworkTopology) calculateLatency(path []string) int {
	totalLatency := 0
	for i := 0; i < len(path)-1; i++ {
		if node, exists := nt.nodes[path[i]]; exists {
			if latency, exists := node.Latency[path[i+1]]; exists {
				totalLatency += latency
			} else {
				totalLatency += 50 // Default latency
			}
		}
	}
	return totalLatency
}

func main() {
	port := getenv("PORT", "8092")

	topology := NewNetworkTopology()
	metricsObj = metrics.NewMetrics()
	reg := prometheus.NewRegistry()
	if err := metricsObj.Register(reg); err != nil {
		log.Fatalf("Failed to register metrics: %v", err)
	}

	// Create realistic network topology
	createRealisticTopology(topology)

	// Start HTTP server
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// API endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		metricsObj.HTTPRequestTotal.WithLabelValues(r.Method, "200", "/health").Inc()
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/topology", func(w http.ResponseWriter, req *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, Status: 200}

		topology.mu.RLock()
		defer topology.mu.RUnlock()

		rec.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(rec).Encode(topology.nodes)
		if err != nil {
			http.Error(rec, err.Error(), http.StatusInternalServerError)
		}

		metricsObj.HTTPRequestTotal.WithLabelValues(
			req.Method,
			strconv.Itoa(rec.Status),
			"/topology",
		).Inc()
	})

	r.Post("/request", func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, Status: 200}

		var req Request

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(rec, err.Error(), http.StatusBadRequest)
			metricsObj.HTTPRequestTotal.WithLabelValues(
				r.Method,
				strconv.Itoa(rec.Status),
				"/request",
			).Inc()
			return
		}

		response := topology.FindSegment(req.SegmentID, req.FromNode)

		rec.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(rec).Encode(response); err != nil {
			http.Error(rec, err.Error(), http.StatusInternalServerError)
		}

		metricsObj.HTTPRequestTotal.WithLabelValues(
			r.Method,
			strconv.Itoa(rec.Status),
			"/request",
		).Inc()
	})

	r.Post("/add-segment", func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			NodeID    string `json:"nodeId"`
			SegmentID string `json:"segmentId"`
		}

		rec := &statusRecorder{ResponseWriter: w, Status: 200}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(rec, err.Error(), http.StatusBadRequest)

			metricsObj.HTTPRequestTotal.WithLabelValues(
				r.Method,
				strconv.Itoa(rec.Status),
				"/add-segment",
			).Inc()

			return
		}

		topology.mu.Lock()
		if node, exists := topology.nodes[data.NodeID]; exists {
			node.Storage[data.SegmentID] = true
		}
		topology.mu.Unlock()

		metricsObj.HTTPRequestTotal.WithLabelValues(
			r.Method,
			strconv.Itoa(rec.Status),
			"/add-segment",
		).Inc()
	})

	r.Post("/seed-demo", func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, Status: 200}

		var data struct {
			SegmentID string `json:"segmentId"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(rec, err.Error(), http.StatusBadRequest)

			metricsObj.HTTPRequestTotal.WithLabelValues(
				r.Method,
				strconv.Itoa(rec.Status),
				"/seed-demo",
			).Inc()

			return
		}

		if data.SegmentID == "" {
			http.Error(rec, "segmentId required", http.StatusBadRequest)

			metricsObj.HTTPRequestTotal.WithLabelValues(
				r.Method,
				strconv.Itoa(rec.Status),
				"/seed-demo",
			).Inc()

			return
		}

		topology.mu.Lock()
		for i := 1; i <= 5; i++ {
			id := fmt.Sprintf("peer-%d", i)
			if node, ok := topology.nodes[id]; ok && node.Type == "peer" {
				node.Storage[data.SegmentID] = true
			}
		}
		topology.mu.Unlock()

		metricsObj.HTTPRequestTotal.WithLabelValues(
			r.Method,
			strconv.Itoa(rec.Status),
			"/seed-demo",
		).Inc()
	})

	log.Printf("Network topology service listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func createRealisticTopology(topology *NetworkTopology) {
	// Create origin server
	origin := &NetworkNode{
		ID:        "origin-1",
		Type:      "origin",
		Region:    "global",
		Storage:   make(map[string]bool),
		Memory:    1000000000, // 1GB
		MaxMemory: 1000000000,
		Latency:   make(map[string]int),
		IsOnline:  true,
		LastSeen:  time.Now(),
	}

	// Add all segments to origin (both formats for compatibility)
	for i := 0; i < 8; i++ {
		origin.Storage[fmt.Sprintf("segment%03d.ts", i)] = true
		origin.Storage[fmt.Sprintf("song_%03d", i)] = true
	}

	topology.AddNode(origin)

	// Create edge servers
	edgeRegions := []string{"us-east", "us-west", "eu-west", "asia-pacific"}
	for i, region := range edgeRegions {
		edge := &NetworkNode{
			ID:        fmt.Sprintf("edge-%d", i+1),
			Type:      "edge",
			Region:    region,
			Storage:   make(map[string]bool),
			Memory:    100000000, // 100MB
			MaxMemory: 100000000,
			Latency:   make(map[string]int),
			IsOnline:  true,
			LastSeen:  time.Now(),
		}

		// Edge servers have partial content (random segments, both formats)
		for j := 0; j < 8; j++ {
			if rand.Float64() < 0.7 { // 70% chance to have each segment
				edge.Storage[fmt.Sprintf("segment%03d.ts", j)] = true
				edge.Storage[fmt.Sprintf("song_%03d", j)] = true
			}
		}

		topology.AddNode(edge)
		topology.ConnectNodes(origin.ID, edge.ID)
	}

	// Create peer nodes (50 peers)
	peerRegions := []string{"us-east", "us-west", "us-central", "eu-west", "eu-central", "asia-pacific", "asia-southeast", "canada", "australia", "japan", "india", "brazil"}

	for i := 0; i < 50; i++ {
		region := peerRegions[rand.Intn(len(peerRegions))]
		peer := &NetworkNode{
			ID:        fmt.Sprintf("peer-%d", i+1),
			Type:      "peer",
			Region:    region,
			Storage:   make(map[string]bool),
			Memory:    50000000, // 50MB
			MaxMemory: 50000000,
			Latency:   make(map[string]int),
			IsOnline:  true,
			LastSeen:  time.Now(),
		}

		// Peers have very limited content (random 1-3 segments)
		segmentCount := rand.Intn(3) + 1
		for j := 0; j < segmentCount; j++ {
			segmentID := fmt.Sprintf("song_%03d", rand.Intn(10))
			peer.Storage[segmentID] = true
		}

		topology.AddNode(peer)
	}

	// Connect peers to each other (P2P mesh)
	// Only 3-4 peers connect directly to edge servers
	edgeConnections := 0
	for _, peer := range topology.nodes {
		if peer.Type == "peer" {
			// 6% chance to connect to edge server
			if rand.Float64() < 0.06 && edgeConnections < 4 {
				// Connect to random edge server
				for _, edge := range topology.nodes {
					if edge.Type == "edge" {
						topology.ConnectNodes(peer.ID, edge.ID)
						edgeConnections++
						break
					}
				}
			}

			// Connect to 2-5 other peers
			peerConnections := rand.Intn(4) + 2
			connected := 0

			for _, otherPeer := range topology.nodes {
				if otherPeer.Type == "peer" && otherPeer.ID != peer.ID && connected < peerConnections {
					// Higher chance to connect to peers in same region
					connectProb := 0.3
					if peer.Region == otherPeer.Region {
						connectProb = 0.8
					}

					if rand.Float64() < connectProb {
						topology.ConnectNodes(peer.ID, otherPeer.ID)
						connected++
					}
				}
			}
		}
	}

	log.Printf("Created network topology with %d nodes", len(topology.nodes))
}

func getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
