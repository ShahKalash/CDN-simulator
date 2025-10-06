package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"time"
)

type Peer struct {
	PeerID       string  `json:"peerId"`
	Region       string  `json:"region"`
	RTT          int     `json:"rtt"`
	Bandwidth    string  `json:"bandwidth"`
	Availability float64 `json:"availability"`
}

type RoutingDecision struct {
	RequestedSegment string `json:"requestedSegment"`
	ClientRegion     string `json:"clientRegion"`
	Decision         string `json:"decision"`
	Source           string `json:"source"`
	Reason           string `json:"reason"`
	RTT              int    `json:"rtt"`
	Peers            []Peer `json:"availablePeers"`
	Timestamp        int64  `json:"timestamp"`
}

func main() {
	trackerURL := "http://localhost:8090"
	edgeURL := "http://localhost:8081"

	segments := []string{
		"rickroll/128k/segment000.ts",
		"rickroll/128k/segment001.ts",
		"rickroll/128k/segment002.ts",
		"rickroll/128k/segment003.ts",
		"rickroll/128k/segment004.ts",
	}

	regions := []string{"us-east", "us-west", "eu-west", "eu-central", "asia-pacific"}

	fmt.Println("🎯 Intelligent CDN Routing Demo")
	fmt.Println("===============================")

	// Simulate 20 different client requests
	for i := 0; i < 20; i++ {
		segment := segments[rand.Intn(len(segments))]
		clientRegion := regions[rand.Intn(len(regions))]

		decision := makeRoutingDecision(trackerURL, edgeURL, segment, clientRegion)

		fmt.Printf("\n📱 Request #%d:\n", i+1)
		fmt.Printf("   Segment: %s\n", decision.RequestedSegment)
		fmt.Printf("   Client Region: %s\n", decision.ClientRegion)
		fmt.Printf("   🎯 DECISION: %s\n", decision.Decision)
		fmt.Printf("   📡 Source: %s\n", decision.Source)
		fmt.Printf("   💡 Reason: %s\n", decision.Reason)
		fmt.Printf("   ⚡ RTT: %dms\n", decision.RTT)
		fmt.Printf("   👥 Available Peers: %d\n", len(decision.Peers))

		if len(decision.Peers) > 0 && decision.Decision == "P2P" {
			fmt.Printf("   🏆 Best Peer: %s (RTT: %dms, %s)\n",
				decision.Peers[0].PeerID, decision.Peers[0].RTT, decision.Peers[0].Bandwidth)
		}

		time.Sleep(500 * time.Millisecond) // Simulate processing time
	}

	fmt.Println("\n🎉 Routing Demo Complete!")
	fmt.Println("This demonstrates how the CDN intelligently chooses between:")
	fmt.Println("  • P2P (fastest, lowest cost)")
	fmt.Println("  • Edge Cache (reliable, medium cost)")
	fmt.Println("  • Origin Server (fallback, highest cost)")
}

func makeRoutingDecision(trackerURL, edgeURL, segment, clientRegion string) RoutingDecision {
	decision := RoutingDecision{
		RequestedSegment: segment,
		ClientRegion:     clientRegion,
		Timestamp:        time.Now().Unix(),
	}

	// Step 1: Query tracker for available peers
	peers := queryTracker(trackerURL, segment, clientRegion)
	decision.Peers = peers

	// Step 2: Apply intelligent routing logic
	if len(peers) > 0 {
		// Sort peers by performance score (RTT + availability + region preference)
		sortedPeers := rankPeers(peers, clientRegion)
		bestPeer := sortedPeers[0]

		// Decision logic: Use P2P if we have good peers
		if bestPeer.RTT < 100 && bestPeer.Availability > 0.7 {
			decision.Decision = "P2P"
			decision.Source = bestPeer.PeerID
			decision.RTT = bestPeer.RTT
			decision.Reason = fmt.Sprintf("Fast peer available (RTT: %dms, Availability: %.1f%%)",
				bestPeer.RTT, bestPeer.Availability*100)
		} else {
			decision.Decision = "EDGE_CACHE"
			decision.Source = "edge-server"
			decision.RTT = estimateEdgeRTT(clientRegion)
			decision.Reason = "Peers available but edge cache is more reliable"
		}
	} else {
		// No peers available, check edge cache
		if checkEdgeCache(edgeURL, segment) {
			decision.Decision = "EDGE_CACHE"
			decision.Source = "edge-server"
			decision.RTT = estimateEdgeRTT(clientRegion)
			decision.Reason = "No peers available, serving from edge cache"
		} else {
			decision.Decision = "ORIGIN_FALLBACK"
			decision.Source = "origin-server"
			decision.RTT = estimateOriginRTT(clientRegion)
			decision.Reason = "Content not in edge cache, fetching from origin"
		}
	}

	return decision
}

func queryTracker(trackerURL, segment, region string) []Peer {
	url := fmt.Sprintf("%s/peers?seg=%s&count=10&region=%s", trackerURL, segment, region)

	resp, err := http.Get(url)
	if err != nil {
		return []Peer{}
	}
	defer resp.Body.Close()

	var peers []Peer
	json.NewDecoder(resp.Body).Decode(&peers)
	return peers
}

func rankPeers(peers []Peer, clientRegion string) []Peer {
	// Create a copy to avoid modifying original
	rankedPeers := make([]Peer, len(peers))
	copy(rankedPeers, peers)

	sort.Slice(rankedPeers, func(i, j int) bool {
		scoreI := calculatePeerScore(rankedPeers[i], clientRegion)
		scoreJ := calculatePeerScore(rankedPeers[j], clientRegion)
		return scoreI > scoreJ // Higher score is better
	})

	return rankedPeers
}

func calculatePeerScore(peer Peer, clientRegion string) float64 {
	score := 100.0

	// RTT penalty (lower RTT = higher score)
	score -= float64(peer.RTT) * 0.5

	// Availability bonus
	score += peer.Availability * 50

	// Region preference bonus
	if peer.Region == clientRegion {
		score += 20
	} else if isNearbyRegion(peer.Region, clientRegion) {
		score += 10
	}

	// Bandwidth tier bonus
	switch peer.Bandwidth {
	case "fiber":
		score += 15
	case "cable":
		score += 10
	case "dsl":
		score += 5
	}

	return score
}

func isNearbyRegion(peerRegion, clientRegion string) bool {
	nearby := map[string][]string{
		"us-east":      {"us-west", "canada"},
		"us-west":      {"us-east", "canada"},
		"eu-west":      {"eu-central"},
		"eu-central":   {"eu-west"},
		"asia-pacific": {"asia-southeast", "japan", "australia"},
	}

	for _, region := range nearby[clientRegion] {
		if region == peerRegion {
			return true
		}
	}
	return false
}

func checkEdgeCache(edgeURL, segment string) bool {
	url := fmt.Sprintf("%s/%s", edgeURL, segment)
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	return resp.StatusCode == 200
}

func estimateEdgeRTT(region string) int {
	// Simulate edge server RTT based on region
	baseRTT := map[string]int{
		"us-east":      25,
		"us-west":      30,
		"eu-west":      45,
		"eu-central":   40,
		"asia-pacific": 80,
	}

	if rtt, exists := baseRTT[region]; exists {
		return rtt + rand.Intn(10) // Add some variance
	}
	return 60 + rand.Intn(20)
}

func estimateOriginRTT(region string) int {
	// Origin server is typically farther away
	return estimateEdgeRTT(region) + 50 + rand.Intn(30)
}
