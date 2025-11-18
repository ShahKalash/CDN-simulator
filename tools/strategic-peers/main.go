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

type PeerContainer struct {
	ID             string   `json:"peerId"`
	Region         string   `json:"region"`
	DeviceType     string   `json:"deviceType"`
	Bandwidth      string   `json:"bandwidth"`
	RTT            int      `json:"rtt"`
	Availability   float64  `json:"availability"`
	LastSeen       int64    `json:"lastSeen"`
	IsOnline       bool     `json:"isOnline"`
	IsSeedPeer     bool     `json:"isSeedPeer"`
	ConnectedPeers []string `json:"connectedPeers"`
	MaxConnections int      `json:"maxConnections"`
	UploadSlots    int      `json:"uploadSlots"`
	Segments       []string `json:"segments"`
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

func main() {
	peerCount := 800
	if len(os.Args) > 1 {
		if count, err := strconv.Atoi(os.Args[1]); err == nil {
			peerCount = count
		}
	}

	fmt.Printf("🚀 Creating %d strategic P2P peers for CDN testing...\n", peerCount)
	fmt.Println("📊 Strategic segment distribution:")
	fmt.Println("   segment000.ts: 70% of peers (HIGH P2P availability)")
	fmt.Println("   segment001.ts: 35% of peers (MEDIUM P2P availability)")
	fmt.Println("   segment002.ts: 12% of peers (LOW P2P availability)")
	fmt.Println("   segment003.ts: 4% of peers (VERY LOW P2P availability)")
	fmt.Println("   segment004.ts: 0% of peers (NO P2P - Edge/Origin only)")
	fmt.Println()

	trackerURL := "http://localhost:8090/announce"
	signalingURL := "ws://localhost:8091/ws"

	// Check tracker connectivity
	fmt.Println("🔍 Testing tracker connectivity...")
	if resp, err := http.Get("http://localhost:8090/health"); err != nil {
		fmt.Printf("❌ Cannot connect to tracker: %v\n", err)
		fmt.Println("💡 Make sure Docker services are running: docker-compose up")
		return
	} else {
		resp.Body.Close()
		fmt.Println("✅ Tracker is accessible")
	}

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
	}{
		{"fiber", [2]int{5, 25}, 0.95},
		{"cable", [2]int{20, 50}, 0.88},
		{"4g", [2]int{30, 80}, 0.82},
		{"3g", [2]int{80, 200}, 0.70},
		{"wifi", [2]int{15, 40}, 0.90},
	}

	// Strategic segment distribution probabilities
	segmentProbabilities := []float64{
		0.70, // segment000.ts - High P2P availability
		0.35, // segment001.ts - Medium P2P availability
		0.12, // segment002.ts - Low P2P availability
		0.04, // segment003.ts - Very low P2P availability
		0.00, // segment004.ts - No P2P availability (Edge/Origin only)
	}

	var peers []*PeerContainer
	seedPeerCount := max(1, peerCount/15) // ~7% are seed peers

	fmt.Printf("🌱 Creating %d seed peers (connect to edge servers)\n", seedPeerCount)
	fmt.Printf("👥 Creating %d regular peers (P2P mesh)\n", peerCount-seedPeerCount)

	// Create peers with strategic segment distribution
	for i := 1; i <= peerCount; i++ {
		region := regions[rand.Intn(len(regions))]
		deviceType := deviceTypes[rand.Intn(len(deviceTypes))]
		bandwidth := bandwidthTiers[rand.Intn(len(bandwidthTiers))]
		rtt := rand.Intn(bandwidth.rttRange[1]-bandwidth.rttRange[0]) + bandwidth.rttRange[0]

		peer := &PeerContainer{
			ID:             fmt.Sprintf("peer-%s-%s-%s-%d", region, deviceType, bandwidth.tier, i),
			Region:         region,
			DeviceType:     deviceType,
			Bandwidth:      bandwidth.tier,
			RTT:            rtt,
			Availability:   bandwidth.availability,
			LastSeen:       time.Now().Unix(),
			IsOnline:       true,
			IsSeedPeer:     i <= seedPeerCount,
			ConnectedPeers: make([]string, 0),
			MaxConnections: getMaxConnections(bandwidth.tier, deviceType),
			UploadSlots:    getUploadSlots(bandwidth.tier, deviceType),
			Segments:       make([]string, 0),
		}

		// Strategic segment assignment
		for segIdx, segment := range segments {
			baseProbability := segmentProbabilities[segIdx]

			// Adjust based on device type for realism
			adjustedProbability := baseProbability
			switch deviceType {
			case "smartphone":
				adjustedProbability *= 0.8 // Phones have less storage
			case "tablet":
				adjustedProbability *= 0.9
			case "laptop":
				adjustedProbability *= 1.0
			case "desktop":
				adjustedProbability *= 1.1 // Desktops store more
			}

			// High bandwidth users are more likely to have segments
			switch bandwidth.tier {
			case "fiber":
				adjustedProbability *= 1.2
			case "cable":
				adjustedProbability *= 1.1
			case "4g":
				adjustedProbability *= 0.9
			case "3g":
				adjustedProbability *= 0.7
			}

			// Ensure probability doesn't exceed 1.0
			if adjustedProbability > 1.0 {
				adjustedProbability = 1.0
			}

			if rand.Float64() < adjustedProbability {
				peer.Segments = append(peer.Segments, segment)
			}
		}

		peers = append(peers, peer)
	}

	// Create P2P mesh topology
	fmt.Println("🕸️  Creating P2P mesh topology...")
	createP2PMesh(peers)

	// Register peers with tracker
	fmt.Printf("📡 Registering %d peers with tracker...\n", len(peers))

	var wg sync.WaitGroup
	successCount := 0
	errorCount := 0

	for _, peer := range peers {
		wg.Add(1)
		go func(p *PeerContainer) {
			defer wg.Done()
			if err := registerPeer(trackerURL, signalingURL, p); err == nil {
				successCount++
				if successCount%100 == 0 {
					fmt.Printf("✅ Registered %d peers...\n", successCount)
				}
			} else {
				errorCount++
			}
		}(peer)

		// Small delay to avoid overwhelming tracker
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()

	fmt.Println()
	fmt.Println("🎉 Strategic P2P Network Initialized!")
	fmt.Printf("✅ Successfully registered: %d peers\n", successCount)
	fmt.Printf("❌ Registration errors: %d\n", errorCount)
	fmt.Println()

	// Print actual distribution
	fmt.Println("📊 Actual Segment Distribution:")
	segmentCounts := make([]int, len(segments))
	for _, peer := range peers {
		for segIdx, segment := range segments {
			for _, peerSegment := range peer.Segments {
				if peerSegment == segment {
					segmentCounts[segIdx]++
					break
				}
			}
		}
	}

	for i, segment := range segments {
		segmentName := segment[len(segment)-15:] // Last part of segment name
		percentage := float64(segmentCounts[i]) / float64(len(peers)) * 100
		fmt.Printf("   %s: %d peers (%.1f%%)\n", segmentName, segmentCounts[i], percentage)
	}

	fmt.Println()
	fmt.Println("🌐 Access your strategic P2P network:")
	fmt.Println("   Dashboard: http://localhost:8000")
	fmt.Println("   Enhanced Peers: http://localhost:8000/enhanced-peers.html")
	fmt.Println("   Enhanced Player: http://localhost:8000/enhanced-player.html")
	fmt.Println()
	fmt.Println("🧪 Test routing scenarios:")
	fmt.Println("   • segment000.ts: Should prefer P2P (high availability)")
	fmt.Println("   • segment001.ts: P2P vs Edge competition")
	fmt.Println("   • segment002.ts: Should prefer Edge (low P2P availability)")
	fmt.Println("   • segment003.ts: Edge heavily preferred")
	fmt.Println("   • segment004.ts: Edge/Origin only (no P2P)")
	fmt.Println()

	// Keep the program running with periodic status updates
	fmt.Println("📱 Strategic P2P network is running... Press Ctrl+C to stop")

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Printf("📊 Network status: %d peers online\n", len(peers))
		}
	}
}

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

	switch deviceType {
	case "smartphone":
		return max(2, base-2)
	case "tablet":
		return base
	case "laptop":
		return base + 2
	case "desktop":
		return base + 4
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func createP2PMesh(peers []*PeerContainer) {
	// Create realistic P2P mesh where regular peers connect to each other
	// and seed peers form a backbone
	
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
	fmt.Printf("   👥 %d regular peers will form P2P mesh\n", len(regularPeers))

	// Connect regular peers to each other (preferably in same region)
	for _, peer := range regularPeers {
		connectionsNeeded := min(peer.MaxConnections, 6) // Limit connections for performance

		// Try to connect to at least one seed peer for content discovery
		if len(seedPeers) > 0 && connectionsNeeded > 0 {
			seedPeer := seedPeers[rand.Intn(len(seedPeers))]
			if len(seedPeer.ConnectedPeers) < seedPeer.MaxConnections {
				peer.ConnectedPeers = append(peer.ConnectedPeers, seedPeer.ID)
				seedPeer.ConnectedPeers = append(seedPeer.ConnectedPeers, peer.ID)
				connectionsNeeded--
			}
		}

		// Connect to other regular peers
		attempts := 0
		maxAttempts := min(len(regularPeers), 50) // Limit attempts for performance

		for connectionsNeeded > 0 && attempts < maxAttempts {
			attempts++
			targetPeer := regularPeers[rand.Intn(len(regularPeers))]

			if targetPeer.ID == peer.ID {
				continue
			}

			if len(targetPeer.ConnectedPeers) >= targetPeer.MaxConnections {
				continue
			}

			// Check if already connected
			alreadyConnected := false
			for _, connectedID := range peer.ConnectedPeers {
				if connectedID == targetPeer.ID {
					alreadyConnected = true
					break
				}
			}

			if !alreadyConnected {
				// Prefer same region connections
				connectProbability := 0.3
				if peer.Region == targetPeer.Region {
					connectProbability = 0.8
				}

				if rand.Float64() < connectProbability {
					peer.ConnectedPeers = append(peer.ConnectedPeers, targetPeer.ID)
					targetPeer.ConnectedPeers = append(targetPeer.ConnectedPeers, peer.ID)
					connectionsNeeded--
				}
			}
		}
	}

	// Connect seed peers to each other (backbone)
	for i, seedPeer := range seedPeers {
		connectionsNeeded := min(seedPeer.MaxConnections-len(seedPeer.ConnectedPeers), 3)

		for j, otherSeed := range seedPeers {
			if i != j && connectionsNeeded > 0 && len(otherSeed.ConnectedPeers) < otherSeed.MaxConnections {
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

	// Calculate mesh statistics
	totalConnections := 0
	for _, peer := range peers {
		totalConnections += len(peer.ConnectedPeers)
	}
	totalConnections /= 2 // Each connection is counted twice

	fmt.Printf("   ✅ P2P mesh created: %d total connections\n", totalConnections)
	fmt.Printf("   📊 Average connections per peer: %.1f\n", float64(totalConnections*2)/float64(len(peers)))
}

func registerPeer(trackerURL, signalingURL string, peer *PeerContainer) error {
	announcement := PeerAnnouncement{
		PeerID:         peer.ID,
		Addr:           signalingURL,
		Segments:       peer.Segments,
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

	return nil
}
