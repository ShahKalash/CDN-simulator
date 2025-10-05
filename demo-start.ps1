# CDN Simulator Demo Startup Script
# Run this script to start all services for your demo

Write-Host "🚀 Starting CDN Simulator Demo..." -ForegroundColor Green
Write-Host ""

# Function to check if a port is in use
function Test-Port {
    param([int]$Port)
    try {
        $connection = New-Object System.Net.Sockets.TcpClient
        $connection.Connect("localhost", $Port)
        $connection.Close()
        return $true
    }
    catch {
        return $false
    }
}

# Function to start service in background
function Start-Service {
    param([string]$Name, [string]$Command, [int]$Port)
    
    if (Test-Port $Port) {
        Write-Host "✅ $Name is already running on port $Port" -ForegroundColor Green
        return
    }
    
    Write-Host "🔄 Starting $Name..." -ForegroundColor Yellow
    Start-Process powershell -ArgumentList "-Command", "cd '$PWD'; $Command" -WindowStyle Minimized
    Start-Sleep -Seconds 2
    
    if (Test-Port $Port) {
        Write-Host "✅ $Name started successfully on port $Port" -ForegroundColor Green
    } else {
        Write-Host "❌ Failed to start $Name on port $Port" -ForegroundColor Red
    }
}

# Start all services
Write-Host "📡 Starting Core Services..." -ForegroundColor Cyan
Start-Service "Tracker" "go run cmd/tracker-simple/main.go" 8090
Start-Service "Signaling" "go run cmd/signaling/main.go" 8091

Write-Host ""
Write-Host "🌐 Starting CDN Services..." -ForegroundColor Cyan

# Check if Docker is available
try {
    docker ps | Out-Null
    Write-Host "🐳 Docker is available, starting CDN services..." -ForegroundColor Green
    Start-Process powershell -ArgumentList "-Command", "cd '$PWD'; docker compose up" -WindowStyle Minimized
    Start-Sleep -Seconds 5
    Write-Host "✅ CDN services starting via Docker..." -ForegroundColor Green
} catch {
    Write-Host "⚠️  Docker not available, you'll need to start CDN services manually" -ForegroundColor Yellow
    Write-Host "   Run: docker-compose up" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "👥 Starting Peer Simulation..." -ForegroundColor Cyan
Start-Service "Peer Simulator" "go run tools/persistent-peers/main.go 50" 0

Write-Host ""
Write-Host "🎉 Demo Setup Complete!" -ForegroundColor Green
Write-Host ""
Write-Host "📋 Demo URLs:" -ForegroundColor Cyan
Write-Host "   Dashboard:     file:///$PWD/web/index.html" -ForegroundColor White
Write-Host "   Tracker:       http://localhost:8090/health" -ForegroundColor White
Write-Host "   Signaling:     ws://localhost:8091/ws" -ForegroundColor White
Write-Host "   Edge Server:   http://localhost:8081" -ForegroundColor White
Write-Host "   Origin:        http://localhost:8080" -ForegroundColor White
Write-Host ""
Write-Host "🎬 Demo Content:" -ForegroundColor Cyan
Write-Host "   Rick Roll:     http://localhost:8081/rickroll/128k/playlist.m3u8" -ForegroundColor White
Write-Host "   Demo Audio:    http://localhost:8081/demo/128k/playlist.m3u8" -ForegroundColor White
Write-Host ""
Write-Host "💡 Tips for Demo:" -ForegroundColor Yellow
Write-Host "   1. Open the dashboard in your browser" -ForegroundColor White
Write-Host "   2. Show the network graph with live peer connections" -ForegroundColor White
Write-Host "   3. Play content and explain P2P + CDN hybrid delivery" -ForegroundColor White
Write-Host "   4. Show how peers cache and share segments" -ForegroundColor White
Write-Host ""
Write-Host "Press any key to open the dashboard..." -ForegroundColor Green
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")

# Open dashboard
Start-Process "file:///$PWD/web/index.html"
