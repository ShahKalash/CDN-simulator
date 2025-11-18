# Interactive CDN + P2P Demo Guide

## Overview
This demo showcases a realistic Content Delivery Network (CDN) with Peer-to-Peer (P2P) capabilities, simulating how platforms like Spotify distribute music globally.

## Quick Start

### 1. Start All Services
```powershell
# Terminal 1: Network Topology Service
go run cmd/network-topology/main.go

# Terminal 2: Peer Simulator (50 peers)
go run tools/peer-simulator/main.go 50

# Terminal 3: Song Manager Service
go run cmd/song-manager/main.go

# Terminal 4: Web Server
python -m http.server 8000
```

### 2. Open Web Interfaces
- **Main Dashboard**: http://localhost:8000/web/index.html
- **Interactive Network**: http://localhost:8000/web/interactive-network.html
- **Song Upload (Spotify HQ)**: http://localhost:8000/web/song-upload.html
- **Enhanced Player**: http://localhost:8000/web/enhanced-player.html

## Demo Scenarios

### Scenario 1: Upload New Song (Spotify HQ)
1. Open **Song Upload** interface
2. Fill in song details (title, artist)
3. Upload an MP3/WAV/M4A file
4. Watch the processing status
5. See the song distributed across the CDN network

### Scenario 2: Interactive Network Visualization
1. Open **Interactive Network** interface
2. Select any peer node as client
3. Choose a segment to request
4. Click "Simulate Request" to see:
   - P2P routing (green path)
   - Edge server fallback (blue path)
   - Origin server fallback (red path)
5. Use "Auto Simulate" for continuous demo

### Scenario 3: Real-time P2P Distribution
1. Watch the peer simulator logs
2. See peers sharing segments with each other:
   ```
   peer-12 received segment006.ts from peer peer-11 (P2P, 1 hop, 20ms)
   ```
3. Observe how segments spread through the P2P network

## Network Architecture

### Origin Server
- **Location**: Center of network
- **Content**: All song segments
- **Connections**: Only to edge servers
- **Purpose**: Master content repository

### Edge Servers (4 servers)
- **Locations**: Distributed globally (US-East, US-West, EU-West, Asia-Pacific)
- **Content**: 70% of segments (randomly distributed)
- **Connections**: Origin + some peers
- **Purpose**: Regional content caching

### Peer Nodes (50 peers)
- **Locations**: Distributed globally
- **Content**: 1-3 segments (limited storage)
- **Connections**: P2P mesh + some edge connections
- **Purpose**: User devices, P2P sharing

## Request Routing Logic

### 1. P2P First (Fastest)
- Check connected peers (1 hop)
- Check peers' peers (2 hops)
- Check peers' peers' peers (3 hops)
- **Latency**: 20-60ms

### 2. Edge Server (Fallback)
- If not found in P2P network
- Check nearest edge server
- **Latency**: 50-100ms

### 3. Origin Server (Last Resort)
- If not found in edge servers
- Request from origin via edge
- Edge caches for future requests
- **Latency**: 100-250ms

## Key Features Demonstrated

### 1. Intelligent Routing
- **P2P Priority**: Always try peers first
- **Geographic Awareness**: Prefer nearby peers/edges
- **Caching Strategy**: Edges learn from origin requests

### 2. Real-time Visualization
- **Network Topology**: See all nodes and connections
- **Request Paths**: Animated routing decisions
- **Segment Distribution**: Which nodes have which content
- **Live Statistics**: P2P vs Edge vs Origin requests

### 3. Content Management
- **Upload System**: Spotify HQ-style interface
- **HLS Processing**: Automatic segment generation
- **Multi-bitrate**: 128k and 192k versions
- **Global Distribution**: Automatic CDN population

### 4. P2P Mesh Network
- **Dynamic Connections**: Peers connect to each other
- **Segment Sharing**: Peers share content they have
- **Memory Management**: LRU eviction when full
- **Regional Clustering**: Prefer same-region peers

## Interactive Controls

### Network Visualization
- **Click Nodes**: Select as client
- **Zoom/Pan**: Navigate the network
- **Auto Simulate**: Continuous random requests
- **Reset**: Clear visualizations

### Song Upload
- **Drag & Drop**: Upload audio files
- **Real-time Status**: Processing updates
- **Song Library**: View all uploaded songs
- **Direct Play**: Test uploaded songs

## Performance Metrics

### Request Sources
- **P2P Requests**: Fastest, reduces server load
- **Edge Requests**: Good performance, cached content
- **Origin Requests**: Slowest, but always available

### Network Efficiency
- **Bandwidth Savings**: P2P reduces origin load
- **Latency Optimization**: Geographic distribution
- **Scalability**: More peers = better performance

## Technical Details

### Services
- **Network Topology** (Port 8092): Manages all nodes and routing
- **Peer Simulator** (Port 8090): Simulates peer behavior
- **Song Manager** (Port 8093): Handles uploads and processing
- **Web Server** (Port 8000): Serves the dashboard

### Technologies
- **Backend**: Go (Golang)
- **Frontend**: HTML5, JavaScript, D3.js
- **Audio Processing**: FFmpeg
- **Network Simulation**: Custom P2P + CDN logic

## Demo Tips

### For Presenters
1. **Start with Upload**: Show how easy it is to add content
2. **Show P2P in Action**: Point out peer-to-peer sharing in logs
3. **Interactive Network**: Let audience select nodes and segments
4. **Explain Routing**: Walk through the 3-tier routing logic
5. **Show Scaling**: Mention how more peers improve performance

### For Developers
1. **Check Logs**: Monitor peer simulator for P2P activity
2. **Network API**: Use http://localhost:8092/topology for data
3. **Upload API**: Use http://localhost:8093/songs for song data
4. **Custom Songs**: Upload your own audio files
5. **Modify Parameters**: Change peer count, segment distribution, etc.

## Troubleshooting

### Services Not Starting
- Check if ports are available
- Kill existing Go processes: `Get-Process go | Stop-Process -Force`
- Restart services in order

### Upload Issues
- Ensure FFmpeg is available
- Check file format (MP3, WAV, M4A)
- Verify file size (max 32MB)

### Network Visualization Issues
- Refresh the page
- Check browser console for errors
- Ensure all services are running

## Success Indicators

### Working Demo Shows:
- Peers sharing segments with each other
- Visual request paths in network graph
- Songs processing and distributing
- P2P requests faster than edge/origin
- Interactive node selection and simulation

This demo effectively simulates how modern content delivery works at scale, combining the efficiency of P2P networks with the reliability of traditional CDN infrastructure.
