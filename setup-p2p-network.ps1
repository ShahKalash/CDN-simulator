#!/usr/bin/env pwsh
# P2P CDN Network Setup Script
# Creates a realistic P2P network with strategic segment distribution for testing

param(
    [int]$PeerCount = 1000,
    [string]$ContentName = "rickroll",
    [switch]$Verbose
)

Write-Host "🚀 P2P CDN Network Setup" -ForegroundColor Green
Write-Host "=========================" -ForegroundColor Green
Write-Host ""

# Check prerequisites
Write-Host "🔍 Checking prerequisites..." -ForegroundColor Yellow

# Check if Docker services are running
try {
    $dockerStatus = docker ps --format "table {{.Names}}\t{{.Status}}" | Select-String "cdn-simulator"
    if ($dockerStatus.Count -lt 5) {
        Write-Host "❌ Docker services not running. Starting them..." -ForegroundColor Red
        Write-Host "   Running: docker-compose up -d" -ForegroundColor Gray
        docker-compose up -d
        Start-Sleep -Seconds 10
    } else {
        Write-Host "✅ Docker services are running" -ForegroundColor Green
    }
} catch {
    Write-Host "❌ Docker not available. Please start Docker Desktop and run 'docker-compose up -d'" -ForegroundColor Red
    exit 1
}

# Check if web server is running
try {
    $webTest = Invoke-WebRequest -Uri "http://localhost:8000" -UseBasicParsing -TimeoutSec 2
    Write-Host "✅ Web server is running" -ForegroundColor Green
} catch {
    Write-Host "🌐 Starting web server..." -ForegroundColor Yellow
    Start-Process powershell -ArgumentList "-Command", "cd '$PWD\web'; python -m http.server 8000" -WindowStyle Hidden
    Start-Sleep -Seconds 3
}

Write-Host ""
Write-Host "📊 Network Distribution Strategy" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan

# Define segment distribution strategy
$segments = @(
    "rickroll/128k/segment000.ts",
    "rickroll/128k/segment001.ts", 
    "rickroll/128k/segment002.ts",
    "rickroll/128k/segment003.ts",
    "rickroll/128k/segment004.ts"
)

$distributionStrategy = @{
    # Segment 0: High P2P availability (80% of peers have it)
    "segment000" = @{
        name = "segment000.ts"
        peerAvailability = 0.80
        edgeAvailability = $true
        originAvailability = $true
        testScenario = "P2P preferred (high availability)"
    }
    
    # Segment 1: Medium P2P availability (40% of peers)
    "segment001" = @{
        name = "segment001.ts" 
        peerAvailability = 0.40
        edgeAvailability = $true
        originAvailability = $true
        testScenario = "P2P vs Edge competition"
    }
    
    # Segment 2: Low P2P availability (15% of peers)
    "segment002" = @{
        name = "segment002.ts"
        peerAvailability = 0.15
        edgeAvailability = $true
        originAvailability = $true
        testScenario = "Edge preferred (low P2P availability)"
    }
    
    # Segment 3: Very low P2P availability (5% of peers)
    "segment003" = @{
        name = "segment003.ts"
        peerAvailability = 0.05
        edgeAvailability = $true
        originAvailability = $true
        testScenario = "Edge heavily preferred"
    }
    
    # Segment 4: NO P2P availability (0% of peers) - Edge/Origin only
    "segment004" = @{
        name = "segment004.ts"
        peerAvailability = 0.00
        edgeAvailability = $true
        originAvailability = $true
        testScenario = "Edge/Origin only (no P2P)"
    }
}

foreach ($segKey in $distributionStrategy.Keys) {
    $seg = $distributionStrategy[$segKey]
    Write-Host "   📁 $($seg.name): $($seg.testScenario)" -ForegroundColor White
    Write-Host "      P2P: $($seg.peerAvailability * 100)% | Edge: $($seg.edgeAvailability) | Origin: $($seg.originAvailability)" -ForegroundColor Gray
}

Write-Host ""
Write-Host "🏗️  Creating P2P Network..." -ForegroundColor Yellow

# Kill any existing peer processes
Get-Process | Where-Object {$_.ProcessName -eq "go" -and $_.CommandLine -like "*persistent-peers*"} | Stop-Process -Force -ErrorAction SilentlyContinue

# Create custom peer distribution script
$peerScript = @"
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
	ID             string   ``json:"peerId"``
	Region         string   ``json:"region"``
	DeviceType     string   ``json:"deviceType"``
	Bandwidth      string   ``json:"bandwidth"``
	RTT            int      ``json:"rtt"``
	Availability   float64  ``json:"availability"``
	LastSeen       int64    ``json:"lastSeen"``
	IsOnline       bool     ``json:"isOnline"``
	IsSeedPeer     bool     ``json:"isSeedPeer"``
	ConnectedPeers []string ``json:"connectedPeers"``
	MaxConnections int      ``json:"maxConnections"``
	UploadSlots    int      ``json:"uploadSlots"``
	Segments       []string ``json:"segments"``
}

type PeerAnnouncement struct {
	PeerID         string   ``json:"peerId"``
	Addr           string   ``json:"addr"``
	Segments       []string ``json:"segments"``
	Region         string   ``json:"region"``
	RTT            int      ``json:"rtt"``
	Bandwidth      string   ``json:"bandwidth"``
	LastSeen       int64    ``json:"lastSeen"``
	Availability   float64  ``json:"availability"``
	DeviceType     string   ``json:"deviceType"``
	IsSeedPeer     bool     ``json:"isSeedPeer"``
	ConnectedPeers []string ``json:"connectedPeers"``
	MaxConnections int      ``json:"maxConnections"``
	UploadSlots    int      ``json:"uploadSlots"``
}

func main() {
	peerCount := $PeerCount
	if len(os.Args) > 1 {
		if count, err := strconv.Atoi(os.Args[1]); err == nil {
			peerCount = count
		}
	}

	fmt.Printf("🚀 Creating %d strategic P2P peers...\n", peerCount)
	
	trackerURL := "http://localhost:8090/announce"
	signalingURL := "ws://localhost:8091/ws"

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

	// Strategic segment distribution
	segmentDistribution := map[string]float64{
		"rickroll/128k/segment000.ts": 0.80, // High P2P availability
		"rickroll/128k/segment001.ts": 0.40, // Medium P2P availability  
		"rickroll/128k/segment002.ts": 0.15, // Low P2P availability
		"rickroll/128k/segment003.ts": 0.05, // Very low P2P availability
		"rickroll/128k/segment004.ts": 0.00, // No P2P availability
	}

	var peers []*PeerContainer
	seedPeerCount := max(1, peerCount/15) // ~7% are seed peers
	
	fmt.Printf("🌱 %d seed peers (connect to edge)\n", seedPeerCount)
	fmt.Printf("👥 %d regular peers (P2P mesh)\n", peerCount-seedPeerCount)

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
		for segment, probability := range segmentDistribution {
			if rand.Float64() < probability {
				peer.Segments = append(peer.Segments, segment)
			}
		}

		peers = append(peers, peer)
	}

	// Create P2P mesh
	fmt.Println("🕸️  Creating P2P mesh topology...")
	createP2PMesh(peers)

	// Register peers
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
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()

	fmt.Printf("\n🎉 P2P Network Ready!\n")
	fmt.Printf("✅ Successfully registered: %d peers\n", successCount)
	fmt.Printf("❌ Registration errors: %d\n", errorCount)
	
	// Print distribution summary
	fmt.Printf("\n📊 Segment Distribution Summary:\n")
	for segment, probability := range segmentDistribution {
		expectedPeers := int(float64(successCount) * probability)
		fmt.Printf("   %s: ~%d peers (%.0f%%)\n", 
			segment[len(segment)-15:], expectedPeers, probability*100)
	}
	
	fmt.Printf("\n🌐 Access your P2P network at: http://localhost:8000\n")
	fmt.Printf("📊 Enhanced Peers: http://localhost:8000/enhanced-peers.html\n")
	fmt.Printf("🎬 Enhanced Player: http://localhost:8000/enhanced-player.html\n")
	
	// Keep running
	fmt.Printf("\n📱 P2P network is running... Press Ctrl+C to stop\n")
	select {}
}

func getMaxConnections(bandwidth, deviceType string) int {
	base := map[string]int{
		"fiber": 12, "cable": 8, "4g": 6, "3g": 3, "wifi": 10,
	}[bandwidth]
	
	switch deviceType {
	case "smartphone": return max(2, base-2)
	case "tablet": return base
	case "laptop": return base + 2
	case "desktop": return base + 4
	}
	return base
}

func getUploadSlots(bandwidth, deviceType string) int {
	base := map[string]int{
		"fiber": 6, "cable": 4, "4g": 2, "3g": 1, "wifi": 3,
	}[bandwidth]
	
	if deviceType == "smartphone" {
		return max(1, base-1)
	}
	return base
}

func max(a, b int) int {
	if a > b { return a }
	return b
}

func createP2PMesh(peers []*PeerContainer) {
	// Simple mesh creation - connect peers within regions preferentially
	for _, peer := range peers {
		if peer.IsSeedPeer { continue }
		
		connectionsNeeded := min(peer.MaxConnections, 5)
		for _, otherPeer := range peers {
			if connectionsNeeded <= 0 { break }
			if peer.ID == otherPeer.ID { continue }
			if len(otherPeer.ConnectedPeers) >= otherPeer.MaxConnections { continue }
			
			// Prefer same region connections
			if peer.Region == otherPeer.Region || rand.Float64() < 0.3 {
				peer.ConnectedPeers = append(peer.ConnectedPeers, otherPeer.ID)
				otherPeer.ConnectedPeers = append(otherPeer.ConnectedPeers, peer.ID)
				connectionsNeeded--
			}
		}
	}
}

func min(a, b int) int {
	if a < b { return a }
	return b
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
	if err != nil { return err }

	resp, err := http.Post(trackerURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil { return err }
	defer resp.Body.Close()

	return nil
}
"@

# Write the custom peer script
$peerScript | Out-File -FilePath "tools/strategic-peers.go" -Encoding UTF8

Write-Host "📝 Created strategic peer distribution script" -ForegroundColor Green

# Build and run the strategic peer system
Write-Host "🔨 Building strategic peer system..." -ForegroundColor Yellow
go build -o bin/strategic-peers.exe ./tools/strategic-peers.go

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Failed to build peer system" -ForegroundColor Red
    exit 1
}

Write-Host "🚀 Starting strategic P2P network..." -ForegroundColor Green
Write-Host ""

# Start the peer system in background
Start-Process -FilePath ".\bin\strategic-peers.exe" -WindowStyle Normal

# Wait for peers to register
Write-Host "⏳ Waiting for peers to register..." -ForegroundColor Yellow
Start-Sleep -Seconds 15

# Test the network
Write-Host ""
Write-Host "🧪 Testing Network Distribution" -ForegroundColor Cyan
Write-Host "===============================" -ForegroundColor Cyan

foreach ($segKey in $distributionStrategy.Keys) {
    $seg = $distributionStrategy[$segKey]
    $segmentPath = "rickroll/128k/$($seg.name)"
    
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8090/peers?seg=$segmentPath&count=100" -UseBasicParsing -TimeoutSec 5
        $peers = $response.Content | ConvertFrom-Json
        $peerCount = $peers.Count
        
        Write-Host "📁 $($seg.name):" -ForegroundColor White
        Write-Host "   👥 $peerCount peers have this segment" -ForegroundColor Green
        Write-Host "   🎯 $($seg.testScenario)" -ForegroundColor Gray
        
        if ($peerCount -eq 0 -and $seg.peerAvailability -gt 0) {
            Write-Host "   ⚠️  Expected some peers, but found none" -ForegroundColor Yellow
        } elseif ($peerCount -gt 0 -and $seg.peerAvailability -eq 0) {
            Write-Host "   ⚠️  Expected no peers, but found some" -ForegroundColor Yellow
        } else {
            Write-Host "   ✅ Distribution matches strategy" -ForegroundColor Green
        }
        
        Write-Host ""
    } catch {
        Write-Host "   ❌ Could not query segment $($seg.name)" -ForegroundColor Red
        Write-Host ""
    }
}

Write-Host "🎉 P2P Network Setup Complete!" -ForegroundColor Green
Write-Host ""
Write-Host "🌐 Access Points:" -ForegroundColor Cyan
Write-Host "   Dashboard: http://localhost:8000" -ForegroundColor White
Write-Host "   Enhanced Peers: http://localhost:8000/enhanced-peers.html" -ForegroundColor White  
Write-Host "   Enhanced Player: http://localhost:8000/enhanced-player.html" -ForegroundColor White
Write-Host "   Network Graph: http://localhost:8000/network-graph.html" -ForegroundColor White
Write-Host ""
Write-Host "🧪 Test Scenarios:" -ForegroundColor Yellow
Write-Host "   • segment000.ts: High P2P availability (should prefer P2P)" -ForegroundColor Gray
Write-Host "   • segment001.ts: Medium P2P availability (P2P vs Edge)" -ForegroundColor Gray
Write-Host "   • segment002.ts: Low P2P availability (should prefer Edge)" -ForegroundColor Gray
Write-Host "   • segment003.ts: Very low P2P availability (Edge preferred)" -ForegroundColor Gray
Write-Host "   • segment004.ts: No P2P availability (Edge/Origin only)" -ForegroundColor Gray
Write-Host ""
Write-Host "💡 The network will test all routing scenarios:" -ForegroundColor Green
Write-Host "   🔗 P2P routing (peer-to-peer)" -ForegroundColor Green
Write-Host "   🏢 Edge routing (from edge cache)" -ForegroundColor Green  
Write-Host "   🌍 Origin routing (from origin via edge)" -ForegroundColor Green
