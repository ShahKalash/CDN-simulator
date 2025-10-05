#!/usr/bin/env pwsh
# Quick P2P Network Setup - Strategic Segment Distribution

param(
    [int]$PeerCount = 800,
    [switch]$Clean
)

Write-Host "🚀 Strategic P2P Network Setup" -ForegroundColor Green
Write-Host "==============================" -ForegroundColor Green

if ($Clean) {
    Write-Host "🧹 Cleaning existing peers..." -ForegroundColor Yellow
    Get-Process | Where-Object {$_.ProcessName -eq "go"} | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

# Check Docker services
Write-Host "🔍 Checking Docker services..." -ForegroundColor Yellow
$dockerServices = docker ps --format "{{.Names}}" | Where-Object {$_ -like "cdn-simulator*"}
if ($dockerServices.Count -lt 5) {
    Write-Host "🐳 Starting Docker services..." -ForegroundColor Yellow
    docker-compose up -d
    Start-Sleep -Seconds 8
}

# Check web server
try {
    Invoke-WebRequest -Uri "http://localhost:8000" -UseBasicParsing -TimeoutSec 2 | Out-Null
    Write-Host "✅ Web server running" -ForegroundColor Green
} catch {
    Write-Host "🌐 Starting web server..." -ForegroundColor Yellow
    Start-Process powershell -ArgumentList "-Command", "cd '$PWD\web'; python -m http.server 8000" -WindowStyle Hidden
    Start-Sleep -Seconds 3
}

Write-Host ""
Write-Host "📊 Strategic Distribution Plan:" -ForegroundColor Cyan
Write-Host "   segment000.ts: 70% of peers (HIGH P2P availability)" -ForegroundColor Green
Write-Host "   segment001.ts: 35% of peers (MEDIUM P2P availability)" -ForegroundColor Yellow  
Write-Host "   segment002.ts: 12% of peers (LOW P2P availability)" -ForegroundColor Orange
Write-Host "   segment003.ts: 4% of peers (VERY LOW P2P availability)" -ForegroundColor Red
Write-Host "   segment004.ts: 0% of peers (NO P2P - Edge/Origin only)" -ForegroundColor Magenta
Write-Host ""

# Create strategic peer distribution
Write-Host "🏗️  Creating $PeerCount strategic peers..." -ForegroundColor Yellow

# Update the existing peer system to use strategic distribution
$strategicPeerCode = @'
// Strategic segment distribution - replace the existing segment assignment logic
for segIdx, segment := range segments {
    var probability float64
    switch segIdx {
    case 0: // segment000.ts - High P2P availability
        probability = 0.70
    case 1: // segment001.ts - Medium P2P availability  
        probability = 0.35
    case 2: // segment002.ts - Low P2P availability
        probability = 0.12
    case 3: // segment003.ts - Very low P2P availability
        probability = 0.04
    case 4: // segment004.ts - No P2P availability
        probability = 0.00
    default:
        probability = 0.20
    }
    
    // Adjust based on device type and bandwidth for realism
    adjustedProbability := probability
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
    
    if rand.Float64() < adjustedProbability {
        segmentSize := int64(rand.Intn(50000) + 30000)
        peer.Storage.AddSegment(segment, segmentSize)
    }
}
'@

# Create a temporary modified version of the peer system
$originalFile = Get-Content "tools/persistent-peers/main.go" -Raw
$modifiedFile = $originalFile -replace 'for segIdx, segment := range segments \{[^}]+\}[^}]+\}', $strategicPeerCode

$modifiedFile | Out-File -FilePath "tools/strategic-peers-temp.go" -Encoding UTF8

Write-Host "📝 Created strategic distribution logic" -ForegroundColor Green

# Build the strategic version
Write-Host "🔨 Building strategic peer system..." -ForegroundColor Yellow
go build -o bin/strategic-peers.exe ./tools/strategic-peers-temp.go

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Build failed, using original peer system..." -ForegroundColor Yellow
    Write-Host "🚀 Starting original peer system..." -ForegroundColor Green
    Start-Process powershell -ArgumentList "-Command", "cd '$PWD'; go run ./tools/persistent-peers $PeerCount" -WindowStyle Normal
} else {
    Write-Host "🚀 Starting strategic peer system..." -ForegroundColor Green
    Start-Process -FilePath ".\bin\strategic-peers.exe" -ArgumentList "$PeerCount" -WindowStyle Normal
}

# Wait for registration
Write-Host "⏳ Waiting for peer registration..." -ForegroundColor Yellow
Start-Sleep -Seconds 20

# Test the distribution
Write-Host ""
Write-Host "🧪 Testing Strategic Distribution" -ForegroundColor Cyan
Write-Host "=================================" -ForegroundColor Cyan

$segments = @(
    @{name="segment000.ts"; expected="HIGH"; color="Green"},
    @{name="segment001.ts"; expected="MEDIUM"; color="Yellow"},
    @{name="segment002.ts"; expected="LOW"; color="DarkYellow"},
    @{name="segment003.ts"; expected="VERY LOW"; color="Red"},
    @{name="segment004.ts"; expected="NONE"; color="Magenta"}
)

foreach ($seg in $segments) {
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8090/peers?seg=rickroll/128k/$($seg.name)&count=200" -UseBasicParsing -TimeoutSec 5
        $peers = $response.Content | ConvertFrom-Json
        $count = $peers.Count
        
        Write-Host "📁 $($seg.name): " -NoNewline -ForegroundColor White
        Write-Host "$count peers " -NoNewline -ForegroundColor $seg.color
        Write-Host "($($seg.expected) availability)" -ForegroundColor Gray
        
        # Show sample peer info
        if ($count -gt 0) {
            $samplePeer = $peers[0]
            Write-Host "   👤 Sample: $($samplePeer.peerId) ($($samplePeer.region), $($samplePeer.bandwidth))" -ForegroundColor DarkGray
        }
        
    } catch {
        Write-Host "📁 $($seg.name): " -NoNewline -ForegroundColor White
        Write-Host "Query failed" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "🎉 Strategic P2P Network is Ready!" -ForegroundColor Green
Write-Host ""
Write-Host "🌐 Test Your Network:" -ForegroundColor Cyan
Write-Host "   Dashboard: " -NoNewline -ForegroundColor White
Write-Host "http://localhost:8000" -ForegroundColor Blue
Write-Host "   Enhanced Peers: " -NoNewline -ForegroundColor White  
Write-Host "http://localhost:8000/enhanced-peers.html" -ForegroundColor Blue
Write-Host "   Enhanced Player: " -NoNewline -ForegroundColor White
Write-Host "http://localhost:8000/enhanced-player.html" -ForegroundColor Blue
Write-Host ""
Write-Host "🧪 Routing Test Scenarios:" -ForegroundColor Yellow
Write-Host "   🟢 segment000.ts → Should route via P2P (many peers)" -ForegroundColor Green
Write-Host "   🟡 segment001.ts → P2P vs Edge competition" -ForegroundColor Yellow
Write-Host "   🟠 segment002.ts → Should prefer Edge (few peers)" -ForegroundColor DarkYellow
Write-Host "   🔴 segment003.ts → Edge heavily preferred" -ForegroundColor Red
Write-Host "   🟣 segment004.ts → Edge/Origin only (no P2P)" -ForegroundColor Magenta
Write-Host ""
Write-Host "💡 This tests all CDN routing scenarios:" -ForegroundColor Green
Write-Host "   • P2P routing (peer-to-peer transfer)" -ForegroundColor White
Write-Host "   • Edge caching (from edge server)" -ForegroundColor White
Write-Host "   • Origin fallback (from origin via edge)" -ForegroundColor White

# Cleanup temp file
Remove-Item "tools/strategic-peers-temp.go" -ErrorAction SilentlyContinue
