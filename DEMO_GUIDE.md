# CDN Simulator Demo Guide

## Quick Start (30 seconds)
```powershell
# Run this single command to start everything:
.\demo-start.ps1
```

## What's Working

### Core Services
- **Tracker Service** (Port 8090): Peer discovery and segment tracking
- **Signaling Service** (Port 8091): WebRTC peer connections
- **Persistent Peer System**: 50+ simulated peers with P2P mesh topology

### CDN Services (via Docker)
- **Edge Server** (Port 8081): Caches content from origin
- **Origin Server** (Port 8080): Serves original content
- **MinIO Storage**: Object storage backend

### Web Dashboard
- **Main Dashboard**: `web/index.html` - Overview and quick player
- **Enhanced Player**: `web/enhanced-player.html` - Interactive HLS player
- **Network Graph**: `web/network-graph.html` - Live peer visualization
- **Peer Explorer**: `web/enhanced-peers.html` - Advanced peer management

## Demo Script (5 minutes)

### 1. Introduction (30 seconds)
- "This is a hybrid CDN + P2P content delivery system"
- "We have 50+ simulated peers across global regions"
- "Content is delivered through both CDN edge servers and peer-to-peer connections"

### 2. Show Network Topology (1 minute)
- Open `web/network-graph.html`
- Point out: "Peers are connected in a realistic mesh topology"
- "Seed peers connect to edge servers, regular peers connect to each other"
- "Peers cache segments based on popularity and device capabilities"

### 3. Demonstrate Content Delivery (2 minutes)
- Open `web/enhanced-player.html`
- Play: `http://localhost:8081/rickroll/128k/playlist.m3u8`
- Explain: "First request goes to edge server, then peers cache and share segments"
- Show: "Subsequent requests can be served by nearby peers"

### 4. Show Peer Behavior (1 minute)
- Open `web/enhanced-peers.html`
- Point out: "Peers go online/offline realistically"
- "Popular segments are cached by more peers"
- "Regional peers prefer connections to nearby peers"

### 5. Technical Highlights (30 seconds)
- "Intelligent routing based on latency and region"
- "Sliding window storage with LRU eviction"
- "Real-time peer discovery and segment tracking"

## Key Metrics to Highlight

- **Peer Count**: 50+ active peers
- **Segment Distribution**: Shows which peers have which content
- **Regional Distribution**: Peers across 10+ global regions
- **Cache Hit Ratio**: How often content is served from cache vs origin
- **P2P Efficiency**: Bandwidth savings through peer sharing

## Troubleshooting

### If services don't start:
```powershell
# Check if ports are free
netstat -an | findstr "8090 8091 8080 8081"

# Kill processes on ports if needed
netstat -ano | findstr ":8090"
taskkill /PID <PID> /F
```

### If Docker issues:
```powershell
# Start Docker Desktop manually
Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"

# Or run services individually
go run cmd/tracker-simple/main.go
go run cmd/signaling/main.go
go run tools/persistent-peers/main.go 50
```

### If web dashboard doesn't load:
- Make sure all services are running
- Check browser console for errors
- Try refreshing the page

## Demo Content Available

1. **Rick Roll** (128k): `http://localhost:8081/rickroll/128k/playlist.m3u8`
2. **Rick Roll** (Master): `http://localhost:8081/rickroll/master.m3u8`
3. **Demo Audio** (128k): `http://localhost:8081/demo/128k/playlist.m3u8`

## Architecture Overview

```
[Client] → [Edge Server:8081] → [Origin:8080] → [MinIO Storage]
    ↓
[P2P Network] ← [Tracker:8090] ← [Signaling:8091]
```

- **Edge Server**: Caches popular content, reduces origin load
- **P2P Network**: Peers share cached segments, further reduces bandwidth
- **Tracker**: Discovers which peers have which segments
- **Signaling**: Establishes WebRTC connections between peers

## Success!

Your CDN simulator is fully functional and ready for demo. The system demonstrates:
- Hybrid CDN + P2P architecture
- Intelligent content caching and distribution
- Real-time peer discovery and connection management
- Global network simulation with regional optimization
