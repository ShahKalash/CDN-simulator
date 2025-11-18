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
	"time"
)

type PeerData struct {
	PeerID       string   `json:"peerId"`
	Addr         string   `json:"addr"`
	Segments     []string `json:"segments"`
	Region       string   `json:"region"`
	RTT          int      `json:"rtt"`
	Bandwidth    string   `json:"bandwidth"`
	LastSeen     int64    `json:"lastSeen"`
	Availability float64  `json:"availability"`
}

func main() {
	peerCount := 2000
	if len(os.Args) > 1 {
		if count, err := strconv.Atoi(os.Args[1]); err == nil {
			peerCount = count
		}
	}

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
		"canada", "brazil", "australia", "japan", "india"
	}

	bandwidthTiers := []struct {
		tier         string
		rttRange     [2]int
		availability float64
		segmentProb  float64
	}{
		{"fiber", [2]int{5, 15}, 0.95, 0.8},
		{"cable", [2]int{15, 40}, 0.85, 0.6},
		{"dsl", [2]int{40, 80}, 0.75, 0.4},
		{"mobile", [2]int{80, 200}, 0.65, 0.3},
	}

	fmt.Printf("🚀 Simulating %d peers for Rick Roll CDN...\n", peerCount)
	fmt.Println("📡 Registering peers with tracker...")

	successCount := 0
	errorCount := 0

	for i := 1; i <= peerCount; i++ {
		// Random peer characteristics
		region := regions[rand.Intn(len(regions))]
		bandwidth := bandwidthTiers[rand.Intn(len(bandwidthTiers))]
		rtt := rand.Intn(bandwidth.rttRange[1]-bandwidth.rttRange[0]) + bandwidth.rttRange[0]

		// Determine which segments this peer has (realistic distribution)
		var peerSegments []string
		for segIdx, segment := range segments {
			// Higher probability for earlier segments (more popular)
			probability := bandwidth.segmentProb * (1.0 - float64(segIdx)*0.1)
			if rand.Float64() < probability {
				peerSegments = append(peerSegments, segment)
			}
		}

		// Only register peers that have at least one segment
		if len(peerSegments) > 0 {
			peer := PeerData{
				PeerID:       fmt.Sprintf("peer-%s-%s-%d", region, bandwidth.tier, i),
				Addr:         signalingURL,
				Segments:     peerSegments,
				Region:       region,
				RTT:          rtt,
				Bandwidth:    bandwidth.tier,
				LastSeen:     time.Now().Unix(),
				Availability: bandwidth.availability,
			}

			if err := registerPeer(trackerURL, peer); err == nil {
				successCount++
				if successCount%200 == 0 {
					fmt.Printf("✅ Registered %d peers...\n", successCount)
				}
			} else {
				errorCount++
				if errorCount%100 == 0 {
					fmt.Printf("⚠️  %d registration errors so far...\n", errorCount)
				}
			}
		}

		// Small delay to avoid overwhelming the tracker
		if i%100 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	fmt.Println()
	fmt.Println("🎉 Peer Registration Complete!")
	fmt.Printf("✅ Successfully registered: %d peers\n", successCount)
	fmt.Printf("❌ Registration errors: %d\n", errorCount)
	fmt.Println()

	// Test peer distribution
	fmt.Println("📊 Testing peer distribution...")
	for _, segment := range segments {
		count := queryPeerCount("http://localhost:8090", segment, "us-east")
		fmt.Printf("🎵 %s : %d peers available\n", segment, count)
	}

	fmt.Println()
	fmt.Println("🌐 Geographic Distribution Test:")
	testRegions := []string{"us-east", "us-west", "eu-west", "asia-pacific", "canada"}
	for _, region := range testRegions {
		count := queryPeerCount("http://localhost:8090", segments[0], region)
		fmt.Printf("🌍 %s : %d peers with segment000\n", region, count)
	}

	fmt.Println()
	fmt.Println("🎊 Simulation Complete! Your CDN now has thousands of persistent peers!")
	fmt.Println("🌐 Visit http://localhost:8000/peers.html to explore the peer network")
	fmt.Println("🎵 Visit http://localhost:8000/index.html to test Rick Roll streaming")
}

func registerPeer(trackerURL string, peer PeerData) error {
	jsonData, err := json.Marshal(peer)
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

func queryPeerCount(trackerURL, segment, region string) int {
	url := fmt.Sprintf("%s/peers?seg=%s&count=50&region=%s", trackerURL, segment, region)
	resp, err := http.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var peers []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return 0
	}

	return len(peers)
}
