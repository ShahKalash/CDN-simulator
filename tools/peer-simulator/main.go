package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// PeerContainer represents a simulated mobile device
type PeerContainer struct {
	ID           string            `json:"id"`
	Region       string            `json:"region"`
	DeviceType   string            `json:"deviceType"`
	Storage      map[string]bool   `json:"storage"`      // Segments this peer has
	Memory       int64             `json:"memory"`       // Current memory usage
	MaxMemory    int64             `json:"maxMemory"`    // Maximum memory capacity
	Connections  []string          `json:"connections"`  // Connected peer IDs
	IsOnline     bool              `json:"isOnline"`
	LastSeen     time.Time         `json:"lastSeen"`
	RequestCount int64             `json:"requestCount"`
	UploadCount  int64             `json:"uploadCount"`
}

// PeerRequest represents a request from this peer
type PeerRequest struct {
	RequestID string `json:"requestId"`
	SegmentID string `json:"segmentId"`
	FromPeer  string `json:"fromPeer"`
	Timestamp time.Time `json:"timestamp"`
}

// PeerResponse represents a response to a request
type PeerResponse struct {
	RequestID string `json:"requestId"`
	Success   bool   `json:"success"`
	Source    string `json:"source"`
	Latency   int    `json:"latency"`
	Hops      int    `json:"hops"`
}

type PeerSimulator struct {
	peers        []*PeerContainer
	networkAPI   string
	trackerAPI   string
	mu           sync.RWMutex
	requestCount int64
}

func NewPeerSimulator(networkAPI, trackerAPI string) *PeerSimulator {
	return &PeerSimulator{
		networkAPI: networkAPI,
		trackerAPI: trackerAPI,
		peers:      make([]*PeerContainer, 0),
	}
}

func (ps *PeerSimulator) CreatePeers(count int) {
	regions := []string{"us-east", "us-west", "us-central", "eu-west", "eu-central", "asia-pacific", "asia-southeast", "canada", "australia", "japan", "india", "brazil"}
	deviceTypes := []string{"smartphone", "tablet", "laptop"}
	
	fmt.Printf("📱 Creating %d peer containers...\n", count)
	
	for i := 0; i < count; i++ {
		region := regions[rand.Intn(len(regions))]
		deviceType := deviceTypes[rand.Intn(len(deviceTypes))]
		
		// Memory capacity based on device type
		maxMemory := int64(50000000) // 50MB default
		switch deviceType {
		case "smartphone":
			maxMemory = 30000000 // 30MB
		case "tablet":
			maxMemory = 80000000 // 80MB
		case "laptop":
			maxMemory = 100000000 // 100MB
		}
		
		peer := &PeerContainer{
			ID:          fmt.Sprintf("peer-%d", i+1),
			Region:      region,
			DeviceType:  deviceType,
			Storage:     make(map[string]bool),
			Memory:      0,
			MaxMemory:   maxMemory,
			Connections: make([]string, 0),
			IsOnline:    true,
			LastSeen:    time.Now(),
		}
		
		// Give peer some random segments (1-3 segments)
		segmentCount := rand.Intn(3) + 1
		for j := 0; j < segmentCount; j++ {
			segmentID := fmt.Sprintf("segment%03d.ts", rand.Intn(8))
			peer.Storage[segmentID] = true
			peer.Memory += 5000000 // 5MB per segment
		}
		
		ps.peers = append(ps.peers, peer)
	}
	
	// Create P2P connections
	ps.createP2PConnections()
	
	fmt.Printf("✅ Created %d peer containers\n", len(ps.peers))
}

func (ps *PeerSimulator) createP2PConnections() {
	fmt.Println("🕸️  Creating P2P mesh connections...")
	
	// Only 3-4 peers connect directly to edge servers
	edgeConnections := 0
	maxEdgeConnections := 4
	
	for _, peer := range ps.peers {
		// 6% chance to connect to edge server
		if rand.Float64() < 0.06 && edgeConnections < maxEdgeConnections {
			// This peer will connect to edge server (handled by network topology)
			edgeConnections++
		}
		
		// Connect to 2-5 other peers
		peerConnections := rand.Intn(4) + 2
		connected := 0
		
		for _, otherPeer := range ps.peers {
			if otherPeer.ID != peer.ID && connected < peerConnections {
				// Higher chance to connect to peers in same region
				connectProb := 0.3
				if peer.Region == otherPeer.Region {
					connectProb = 0.8
				}
				
				if rand.Float64() < connectProb {
					peer.Connections = append(peer.Connections, otherPeer.ID)
					otherPeer.Connections = append(otherPeer.Connections, peer.ID)
					connected++
				}
			}
		}
	}
	
	fmt.Printf("✅ P2P mesh created with %d edge connections\n", edgeConnections)
}

func (ps *PeerSimulator) StartSimulation() {
	fmt.Println("🚀 Starting peer simulation...")
	
	// Register all peers with network topology
	ps.registerPeers()
	
	// Start request simulation
	go ps.simulateRequests()
	
	// Start periodic status updates
	go ps.periodicStatusUpdate()
	
	// Keep running
	select {}
}

func (ps *PeerSimulator) registerPeers() {
	fmt.Println("📡 Registering peers with network topology...")
	
	for _, peer := range ps.peers {
		// Register peer with network topology
		peerData := map[string]interface{}{
			"id":       peer.ID,
			"type":     "peer",
			"region":   peer.Region,
			"storage":  peer.Storage,
			"memory":   peer.Memory,
			"maxMemory": peer.MaxMemory,
			"isOnline": peer.IsOnline,
		}
		
		jsonData, _ := json.Marshal(peerData)
		http.Post(ps.networkAPI+"/add-peer", "application/json", bytes.NewBuffer(jsonData))
		
		// Register segments with tracker
		for segmentID := range peer.Storage {
			ps.registerSegment(peer.ID, segmentID)
		}
	}
	
	fmt.Println("✅ All peers registered")
}

func (ps *PeerSimulator) registerSegment(peerID, segmentID string) {
	segmentData := map[string]string{
		"nodeId":    peerID,
		"segmentId": segmentID,
	}
	
	jsonData, _ := json.Marshal(segmentData)
	http.Post(ps.networkAPI+"/add-segment", "application/json", bytes.NewBuffer(jsonData))
}

func (ps *PeerSimulator) simulateRequests() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		// Randomly select a peer to make a request
		if len(ps.peers) == 0 {
			continue
		}
		
		peer := ps.peers[rand.Intn(len(ps.peers))]
		if !peer.IsOnline {
			continue
		}
		
		// Request a random segment (try both formats)
		segmentNum := rand.Intn(8)
		segmentID := fmt.Sprintf("segment%03d.ts", segmentNum)
		songID := fmt.Sprintf("song_%03d", segmentNum)
		
		// Check if peer already has this segment (either format)
		if peer.Storage[segmentID] || peer.Storage[songID] {
			continue
		}
		
		// Make request through network topology
		ps.makeRequest(peer, segmentID)
	}
}

func (ps *PeerSimulator) makeRequest(peer *PeerContainer, segmentID string) {
	ps.mu.Lock()
	ps.requestCount++
	ps.mu.Unlock()
	
	// First try P2P - check connected peers
	for _, connectedPeerID := range peer.Connections {
		connectedPeer := ps.findPeer(connectedPeerID)
		if connectedPeer != nil {
			// Check both segment formats for the requested segment
			// Extract segment number from segmentID (e.g., "segment003.ts" -> 3)
			var segmentNum int
			if len(segmentID) >= 12 && segmentID[:8] == "segment" {
				fmt.Sscanf(segmentID, "segment%03d.ts", &segmentNum)
			}
			songIDCheck := fmt.Sprintf("song_%03d", segmentNum)
			
			if connectedPeer.Storage[segmentID] || connectedPeer.Storage[songIDCheck] {
				// Found in P2P network
				peer.Storage[segmentID] = true
				peer.Memory += 5000000 // 5MB per segment
				peer.RequestCount++
				
				// Register new segment with tracker
				ps.registerSegment(peer.ID, segmentID)
				
				// If memory is full, remove oldest segment
				if peer.Memory > peer.MaxMemory {
					ps.evictOldestSegment(peer)
				}
				
				fmt.Printf("✅ %s received %s from peer %s (P2P, 1 hop, 20ms)\n", 
					peer.ID, segmentID, connectedPeerID)
				return
			}
		}
	}
	
	// If not found in P2P, try through network topology (edge/origin)
	request := PeerRequest{
		RequestID: fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), ps.requestCount),
		SegmentID: segmentID,
		FromPeer:  peer.ID,
		Timestamp: time.Now(),
	}
	
	jsonData, _ := json.Marshal(request)
	resp, err := http.Post(ps.networkAPI+"/request", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var response PeerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Failed to decode response: %v", err)
		return
	}
	
	if response.Success {
		// Peer received the segment
		peer.Storage[segmentID] = true
		peer.Memory += 5000000 // 5MB per segment
		peer.RequestCount++
		
		// Register new segment with tracker
		ps.registerSegment(peer.ID, segmentID)
		
		// If memory is full, remove oldest segment
		if peer.Memory > peer.MaxMemory {
			ps.evictOldestSegment(peer)
		}
		
		fmt.Printf("✅ %s received %s from %s (%d hops, %dms)\n", 
			peer.ID, segmentID, response.Source, response.Hops, response.Latency)
	} else {
		fmt.Printf("❌ %s failed to get %s\n", peer.ID, segmentID)
	}
}

func (ps *PeerSimulator) findPeer(peerID string) *PeerContainer {
	for _, peer := range ps.peers {
		if peer.ID == peerID {
			return peer
		}
	}
	return nil
}

func (ps *PeerSimulator) evictOldestSegment(peer *PeerContainer) {
	// Simple LRU: remove first segment found
	for segmentID := range peer.Storage {
		delete(peer.Storage, segmentID)
		peer.Memory -= 5000000
		break
	}
}

func (ps *PeerSimulator) periodicStatusUpdate() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		ps.printStatus()
	}
}

func (ps *PeerSimulator) printStatus() {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	onlinePeers := 0
	totalSegments := 0
	segmentCounts := make(map[string]int)
	
	for _, peer := range ps.peers {
		if peer.IsOnline {
			onlinePeers++
			for segmentID := range peer.Storage {
				totalSegments++
				segmentCounts[segmentID]++
			}
		}
	}
	
	fmt.Printf("\n📊 Peer Status Update:\n")
	fmt.Printf("   Online Peers: %d/%d\n", onlinePeers, len(ps.peers))
	fmt.Printf("   Total Segments: %d\n", totalSegments)
	fmt.Printf("   Total Requests: %d\n", ps.requestCount)
	
	if len(segmentCounts) > 0 {
		fmt.Printf("   Segment Distribution:\n")
		for segmentID, count := range segmentCounts {
			fmt.Printf("     %s: %d peers\n", segmentID, count)
		}
	}
	fmt.Println()
}

func main() {
	networkAPI := getenv("NETWORK_API", "http://localhost:8092")
	trackerAPI := getenv("TRACKER_API", "http://localhost:8090")
	
	peerCount := 50
	if len(os.Args) > 1 {
		if count, err := strconv.Atoi(os.Args[1]); err == nil {
			peerCount = count
		}
	}
	
	fmt.Printf("🚀 Starting Peer Simulator with %d peers\n", peerCount)
	fmt.Printf("🌐 Network API: %s\n", networkAPI)
	fmt.Printf("📡 Tracker API: %s\n", trackerAPI)
	
	simulator := NewPeerSimulator(networkAPI, trackerAPI)
	simulator.CreatePeers(peerCount)
	simulator.StartSimulation()
}

func getenv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
