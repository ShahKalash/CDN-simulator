package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// PeerContainer represents a user's phone/device with persistent storage
type PeerContainer struct {
	ID             string                 `json:"peerId"`
	Region         string                 `json:"region"`
	DeviceType     string                 `json:"deviceType"`
	Bandwidth      string                 `json:"bandwidth"`
	RTT            int                    `json:"rtt"`
	Availability   float64                `json:"availability"`
	Storage        *SlidingWindowStorage  `json:"-"`
	LastSeen       int64                  `json:"lastSeen"`
	IsOnline       bool                   `json:"isOnline"`
	TotalUploads   int64                  `json:"totalUploads"`
	TotalDownloads int64                  `json:"totalDownloads"`
	
	// P2P Network Properties
	IsSeedPeer     bool     `json:"isSeedPeer"`     // Connects to edge server
	ConnectedPeers []string `json:"connectedPeers"` // Other peers this peer is connected to
	MaxConnections int      `json:"maxConnections"` // Max peer connections based on bandwidth
	UploadSlots    int      `json:"uploadSlots"`    // Available upload slots
}

// SlidingWindowStorage simulates phone storage with limited capacity
type SlidingWindowStorage struct {
	segments    map[string]*SegmentInfo
	capacity    int // Max segments to store
	accessOrder []string // LRU tracking
	mutex       sync.RWMutex
}

type SegmentInfo struct {
	SegmentID    string    `json:"segmentId"`
	Size         int64     `json:"size"`
	DownloadTime time.Time `json:"downloadTime"`
	AccessCount  int       `json:"accessCount"`
	LastAccess   time.Time `json:"lastAccess"`
}

type PeerAnnouncement struct {
	PeerID         string   `json:"peerId"`
	Addr           string   `json:"addr"`
	Segments       []string `json:"segments"`
	Region         string   `json:"region"`
	RTT            int      `json:"rtt"`
	Bandwidth      string   `json:"bandwidth"`
	LastSeen       int64    `json:"lastSeen"`
	Availability   float64  `json:"availability"`
	DeviceType     string   `json:"deviceType"`
	IsSeedPeer     bool     `json:"isSeedPeer"`
	ConnectedPeers []string `json:"connectedPeers"`
	MaxConnections int      `json:"maxConnections"`
	UploadSlots    int      `json:"uploadSlots"`
}

func NewSlidingWindowStorage(capacity int) *SlidingWindowStorage {
	return &SlidingWindowStorage{
		segments:    make(map[string]*SegmentInfo),
		capacity:    capacity,
		accessOrder: make([]string, 0),
	}
}

func (s *SlidingWindowStorage) AddSegment(segmentID string, size int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	now := time.Now()
	
	// If segment already exists, update access
	if existing, exists := s.segments[segmentID]; exists {
		existing.AccessCount++
		existing.LastAccess = now
		s.updateAccessOrder(segmentID)
		return
	}
	
	// If at capacity, remove least recently used
	if len(s.segments) >= s.capacity {
		s.evictLRU()
	}
	
	// Add new segment
	s.segments[segmentID] = &SegmentInfo{
		SegmentID:    segmentID,
		Size:         size,
		DownloadTime: now,
		AccessCount:  1,
		LastAccess:   now,
	}
	
	s.accessOrder = append(s.accessOrder, segmentID)
}

func (s *SlidingWindowStorage) HasSegment(segmentID string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	if info, exists := s.segments[segmentID]; exists {
		// Update access (in a real system, this would be done on actual access)
		info.AccessCount++
		info.LastAccess = time.Now()
		return true
	}
	return false
}

func (s *SlidingWindowStorage) GetSegments() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	segments := make([]string, 0, len(s.segments))
	for segmentID := range s.segments {
		segments = append(segments, segmentID)
	}
	return segments
}

func (s *SlidingWindowStorage) evictLRU() {
	if len(s.accessOrder) == 0 {
		return
	}
	
	// Remove least recently used
	lruSegment := s.accessOrder[0]
	delete(s.segments, lruSegment)
	s.accessOrder = s.accessOrder[1:]
}

func (s *SlidingWindowStorage) updateAccessOrder(segmentID string) {
	// Move to end (most recently used)
	for i, id := range s.accessOrder {
		if id == segmentID {
			s.accessOrder = append(s.accessOrder[:i], s.accessOrder[i+1:]...)
			break
		}
	}
	s.accessOrder = append(s.accessOrder, segmentID)
}

func main() {
	fmt.Println("🚀 Starting Persistent Peer Container System...")
	
	peerCount := 2000
	if len(os.Args) > 1 {
		if count, err := strconv.Atoi(os.Args[1]); err == nil {
			peerCount = count
		}
	}
	
	fmt.Printf("📱 Target peer count: %d\n", peerCount)

	trackerURL := "http://localhost:8090/announce"
	signalingURL := "ws://localhost:8091/ws"

	segments := []string{
		"rickroll/128k/segment000.ts",
		"rickroll/128k/segment001.ts",
		"rickroll/128k/segment002.ts",
		"rickroll/128k/segment003.ts",
		"rickroll/128k/segment004.ts",
	}

	regions := []string{
		"us-east", "us-west", "us-central",
		"eu-west", "eu-central", "eu-north",
		"asia-pacific", "asia-southeast", "asia-northeast",
		"canada", "brazil", "australia", "japan", "india",
	}

	deviceTypes := []string{"smartphone", "tablet", "laptop", "desktop"}
	
	bandwidthTiers := []struct {
		tier         string
		rttRange     [2]int
		availability float64
		storageCapacity int
	}{
		{"fiber", [2]int{5, 25}, 0.95, 15},      // Can store 15 segments
		{"cable", [2]int{20, 50}, 0.88, 12},    // Can store 12 segments
		{"4g", [2]int{30, 80}, 0.82, 8},        // Can store 8 segments
		{"3g", [2]int{80, 200}, 0.70, 5},       // Can store 5 segments
		{"wifi", [2]int{15, 40}, 0.90, 10},     // Can store 10 segments
	}

	fmt.Printf("🚀 Creating %d persistent peer containers (simulating user phones)...\n", peerCount)
	fmt.Println("📱 Each peer has sliding window storage and stays online permanently")
	
	// Check if tracker is accessible
	fmt.Println("🔍 Testing tracker connectivity...")
	if resp, err := http.Get("http://localhost:8090/health"); err != nil {
		fmt.Printf("❌ Cannot connect to tracker: %v\n", err)
		fmt.Println("💡 Make sure Docker services are running: docker-compose up")
		return
	} else {
		resp.Body.Close()
		fmt.Println("✅ Tracker is accessible")
	}

	var peers []*PeerContainer
	var wg sync.WaitGroup
	successCount := 0
	errorCount := 0
	
	// Create peer containers with P2P topology
	seedPeerCount := max(1, peerCount/15) // ~7% are seed peers
	fmt.Printf("🌱 Creating %d seed peers (connect to edge server)\n", seedPeerCount)
	fmt.Printf("👥 Creating %d regular peers (connect to other peers)\n", peerCount-seedPeerCount)
	
	for i := 1; i <= peerCount; i++ {
		region := regions[rand.Intn(len(regions))]
		deviceType := deviceTypes[rand.Intn(len(deviceTypes))]
		bandwidth := bandwidthTiers[rand.Intn(len(bandwidthTiers))]
		rtt := rand.Intn(bandwidth.rttRange[1]-bandwidth.rttRange[0]) + bandwidth.rttRange[0]

		// Determine max connections based on bandwidth and device type
		maxConnections := getMaxConnections(bandwidth.tier, deviceType)
		uploadSlots := getUploadSlots(bandwidth.tier, deviceType)
		
		peer := &PeerContainer{
			ID:             fmt.Sprintf("peer-%s-%s-%s-%d", region, deviceType, bandwidth.tier, i),
			Region:         region,
			DeviceType:     deviceType,
			Bandwidth:      bandwidth.tier,
			RTT:            rtt,
			Availability:   bandwidth.availability,
			Storage:        NewSlidingWindowStorage(bandwidth.storageCapacity),
			LastSeen:       time.Now().Unix(),
			IsOnline:       true,
			IsSeedPeer:     i <= seedPeerCount, // First peers are seed peers
			ConnectedPeers: make([]string, 0),
			MaxConnections: maxConnections,
			UploadSlots:    uploadSlots,
		}

		// Simulate realistic segment distribution
		// Popular segments are more likely to be cached
		for segIdx, segment := range segments {
			// Probability decreases for later segments (less popular)
			probability := 0.8 - float64(segIdx)*0.15
			
			// Adjust based on device type and bandwidth
			switch deviceType {
			case "smartphone":
				probability *= 0.7 // Phones have less storage
			case "tablet":
				probability *= 0.85
			case "laptop":
				probability *= 0.95
			case "desktop":
				probability *= 1.0
			}
			
			if rand.Float64() < probability {
				segmentSize := int64(rand.Intn(50000) + 30000) // 30-80KB segments
				peer.Storage.AddSegment(segment, segmentSize)
			}
		}

		peers = append(peers, peer)
	}

	// Create P2P mesh topology
	fmt.Println("🕸️  Creating P2P mesh topology...")
	createP2PMesh(peers)

	fmt.Printf("📡 Registering %d peer containers with tracker...\n", len(peers))

	// Register all peers concurrently
	for _, peer := range peers {
		wg.Add(1)
		go func(p *PeerContainer) {
			defer wg.Done()
			
			if err := registerPeerContainer(trackerURL, signalingURL, p); err == nil {
				successCount++
				if successCount%100 == 0 {
					fmt.Printf("✅ Registered %d peer containers...\n", successCount)
				}
			} else {
				errorCount++
			}
		}(peer)
		
		// Small delay to avoid overwhelming tracker
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()

	fmt.Println()
	fmt.Println("🎉 Peer Container System Initialized!")
	fmt.Printf("✅ Successfully registered: %d peer containers\n", successCount)
	fmt.Printf("❌ Registration errors: %d\n", errorCount)
	fmt.Println()

	// Start persistent peer behavior simulation
	fmt.Println("🔄 Starting persistent peer behavior simulation...")
	startPeerBehaviorSimulation(peers, trackerURL, signalingURL)

	// Keep the program running
	fmt.Println("📱 Peer containers are now running permanently...")
	fmt.Println("Press Ctrl+C to stop")
	
	// Periodic status updates
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			showPeerStatus(peers)
		}
	}
}

func registerPeerContainer(trackerURL, signalingURL string, peer *PeerContainer) error {
	announcement := PeerAnnouncement{
		PeerID:         peer.ID,
		Addr:           signalingURL,
		Segments:       peer.Storage.GetSegments(),
		Region:         peer.Region,
		RTT:            peer.RTT,
		Bandwidth:      peer.Bandwidth,
		LastSeen:       peer.LastSeen,
		Availability:   peer.Availability,
		DeviceType:     peer.DeviceType,
		IsSeedPeer:     peer.IsSeedPeer,
		ConnectedPeers: peer.ConnectedPeers,
		MaxConnections: peer.MaxConnections,
		UploadSlots:    peer.UploadSlots,
	}

	jsonData, err := json.Marshal(announcement)
	if err != nil {
		return err
	}

	resp, err := http.Post(trackerURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("tracker returned status %d", resp.StatusCode)
	}

	return nil
}

func startPeerBehaviorSimulation(peers []*PeerContainer, trackerURL, signalingURL string) {
	// Simulate peer behavior: downloading new segments, going online/offline, etc.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		
		segments := []string{
			"rickroll/128k/segment000.ts",
			"rickroll/128k/segment001.ts",
			"rickroll/128k/segment002.ts",
			"rickroll/128k/segment003.ts",
			"rickroll/128k/segment004.ts",
		}
		
		for {
			select {
			case <-ticker.C:
				// Simulate some peers downloading new segments
				for _, peer := range peers {
					if !peer.IsOnline {
						continue
					}
					
					// 10% chance to download a new segment
					if rand.Float64() < 0.1 {
						segment := segments[rand.Intn(len(segments))]
						if !peer.Storage.HasSegment(segment) {
							segmentSize := int64(rand.Intn(50000) + 30000)
							peer.Storage.AddSegment(segment, segmentSize)
							peer.TotalDownloads++
							
							// Re-announce to tracker with updated segments
							go registerPeerContainer(trackerURL, signalingURL, peer)
						}
					}
					
					// Small chance to go offline temporarily (simulate real users)
					if rand.Float64() < 0.02 { // 2% chance
						peer.IsOnline = false
						go func(p *PeerContainer) {
							// Come back online after 30-120 seconds
							offlineTime := time.Duration(rand.Intn(90)+30) * time.Second
							time.Sleep(offlineTime)
							p.IsOnline = true
							p.LastSeen = time.Now().Unix()
							registerPeerContainer(trackerURL, signalingURL, p)
						}(peer)
					}
					
					// Update last seen
					peer.LastSeen = time.Now().Unix()
				}
			}
		}
	}()
	
	// Periodic re-announcement to keep peers alive in tracker
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				for _, peer := range peers {
					if peer.IsOnline {
						go registerPeerContainer(trackerURL, signalingURL, peer)
					}
				}
			}
		}
	}()
}

func showPeerStatus(peers []*PeerContainer) {
	onlinePeers := 0
	totalSegments := 0
	
	segmentCounts := make(map[string]int)
	
	for _, peer := range peers {
		if peer.IsOnline {
			onlinePeers++
			segments := peer.Storage.GetSegments()
			totalSegments += len(segments)
			
			for _, segment := range segments {
				segmentCounts[segment]++
			}
		}
	}
	
	fmt.Printf("\n📊 Peer Container Status:\n")
	fmt.Printf("   Online Peers: %d/%d\n", onlinePeers, len(peers))
	fmt.Printf("   Total Cached Segments: %d\n", totalSegments)
	fmt.Printf("   Segment Distribution:\n")
	
	for segment, count := range segmentCounts {
		segmentName := segment[len(segment)-15:] // Last part of segment name
		fmt.Printf("     %s: %d peers\n", segmentName, count)
	}
}

// Helper functions for P2P network topology
func getMaxConnections(bandwidth, deviceType string) int {
	base := 0
	switch bandwidth {
	case "fiber":
		base = 12
	case "cable":
		base = 8
	case "4g":
		base = 6
	case "3g":
		base = 3
	case "wifi":
		base = 10
	}
	
	// Adjust based on device type
	switch deviceType {
	case "smartphone":
		return max(2, base-2) // Phones have fewer connections
	case "tablet":
		return base
	case "laptop":
		return base + 2
	case "desktop":
		return base + 4 // Desktops can handle more connections
	}
	return base
}

func getUploadSlots(bandwidth, deviceType string) int {
	base := 0
	switch bandwidth {
	case "fiber":
		base = 6
	case "cable":
		base = 4
	case "4g":
		base = 2
	case "3g":
		base = 1
	case "wifi":
		base = 3
	}
	
	// Mobile devices typically have fewer upload slots
	if deviceType == "smartphone" {
		return max(1, base-1)
	}
	return base
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// createP2PMesh creates realistic P2P connections between peers
func createP2PMesh(peers []*PeerContainer) {
	fmt.Printf("   🔗 Connecting peers in realistic P2P mesh...\n")
	
	// Separate seed peers and regular peers
	var seedPeers []*PeerContainer
	var regularPeers []*PeerContainer
	
	for _, peer := range peers {
		if peer.IsSeedPeer {
			seedPeers = append(seedPeers, peer)
		} else {
			regularPeers = append(regularPeers, peer)
		}
	}
	
	fmt.Printf("   🌱 %d seed peers will connect to edge servers\n", len(seedPeers))
	fmt.Printf("   👥 %d regular peers will connect to other peers\n", len(regularPeers))
	
	// Connect regular peers to seed peers and other peers
	for _, peer := range regularPeers {
		connectionsNeeded := peer.MaxConnections
		
		// Try to connect to at least one seed peer (for content discovery)
		if len(seedPeers) > 0 && connectionsNeeded > 0 {
			seedPeer := seedPeers[rand.Intn(len(seedPeers))]
			if canConnect(peer, seedPeer) {
				peer.ConnectedPeers = append(peer.ConnectedPeers, seedPeer.ID)
				seedPeer.ConnectedPeers = append(seedPeer.ConnectedPeers, peer.ID)
				connectionsNeeded--
			}
		}
		
		// Connect to other regular peers (preferably in same region)
		attempts := 0
		maxAttempts := len(regularPeers) * 2
		
		for connectionsNeeded > 0 && attempts < maxAttempts {
			attempts++
			targetPeer := regularPeers[rand.Intn(len(regularPeers))]
			
			if targetPeer.ID == peer.ID {
				continue // Don't connect to self
			}
			
			// Check if already connected
			alreadyConnected := false
			for _, connectedID := range peer.ConnectedPeers {
				if connectedID == targetPeer.ID {
					alreadyConnected = true
					break
				}
			}
			
			if !alreadyConnected && canConnect(peer, targetPeer) {
				// Prefer peers in same region (80% chance) or nearby regions
				regionMatch := peer.Region == targetPeer.Region
				nearbyRegion := areRegionsNearby(peer.Region, targetPeer.Region)
				
				connectProbability := 0.3 // Base probability
				if regionMatch {
					connectProbability = 0.8
				} else if nearbyRegion {
					connectProbability = 0.5
				}
				
				if rand.Float64() < connectProbability {
					peer.ConnectedPeers = append(peer.ConnectedPeers, targetPeer.ID)
					targetPeer.ConnectedPeers = append(targetPeer.ConnectedPeers, peer.ID)
					connectionsNeeded--
				}
			}
		}
	}
	
	// Connect seed peers to each other (they form a well-connected backbone)
	for i, seedPeer := range seedPeers {
		connectionsNeeded := min(seedPeer.MaxConnections-len(seedPeer.ConnectedPeers), 3)
		
		for j, otherSeed := range seedPeers {
			if i != j && connectionsNeeded > 0 && canConnect(seedPeer, otherSeed) {
				// Check if already connected
				alreadyConnected := false
				for _, connectedID := range seedPeer.ConnectedPeers {
					if connectedID == otherSeed.ID {
						alreadyConnected = true
						break
					}
				}
				
				if !alreadyConnected {
					seedPeer.ConnectedPeers = append(seedPeer.ConnectedPeers, otherSeed.ID)
					otherSeed.ConnectedPeers = append(otherSeed.ConnectedPeers, seedPeer.ID)
					connectionsNeeded--
				}
			}
		}
	}
	
	// Print mesh statistics
	totalConnections := 0
	for _, peer := range peers {
		totalConnections += len(peer.ConnectedPeers)
	}
	totalConnections /= 2 // Each connection is counted twice
	
	fmt.Printf("   ✅ P2P mesh created: %d total connections\n", totalConnections)
	fmt.Printf("   📊 Average connections per peer: %.1f\n", float64(totalConnections*2)/float64(len(peers)))
}

func canConnect(peer1, peer2 *PeerContainer) bool {
	return len(peer1.ConnectedPeers) < peer1.MaxConnections && 
		   len(peer2.ConnectedPeers) < peer2.MaxConnections
}

func areRegionsNearby(region1, region2 string) bool {
	nearbyRegions := map[string][]string{
		"us-east":         {"us-central", "us-west", "canada"},
		"us-west":         {"us-central", "us-east"},
		"us-central":      {"us-east", "us-west", "canada"},
		"eu-west":         {"eu-central", "eu-north"},
		"eu-central":      {"eu-west", "eu-north"},
		"eu-north":        {"eu-west", "eu-central"},
		"asia-pacific":    {"asia-southeast", "asia-northeast", "australia"},
		"asia-southeast":  {"asia-pacific", "asia-northeast"},
		"asia-northeast":  {"asia-pacific", "asia-southeast", "japan"},
		"canada":          {"us-east", "us-central"},
		"australia":       {"asia-pacific"},
		"japan":           {"asia-northeast"},
		"brazil":          {},
		"india":           {"asia-southeast"},
	}
	
	if nearby, exists := nearbyRegions[region1]; exists {
		for _, region := range nearby {
			if region == region2 {
				return true
			}
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
