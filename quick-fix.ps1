#!/usr/bin/env pwsh
# Quick Fix for P2P CDN Network Issues

Write-Host "Quick Fix for P2P CDN Network" -ForegroundColor Green
Write-Host "==============================" -ForegroundColor Green

# Stop everything
Write-Host "Stopping all services..." -ForegroundColor Yellow
docker-compose down 2>$null
Get-Process | Where-Object {$_.ProcessName -eq "go" -or $_.ProcessName -eq "strategic-peers"} | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process | Where-Object {$_.ProcessName -eq "python"} | Stop-Process -Force -ErrorAction SilentlyContinue

Start-Sleep -Seconds 3

# Start fresh
Write-Host "Starting Docker services..." -ForegroundColor Yellow
docker-compose up -d

Write-Host "Waiting for services to start..." -ForegroundColor Yellow
Start-Sleep -Seconds 15

# Test services
Write-Host "Testing services..." -ForegroundColor Yellow

# Test edge server with netstat approach
$edgePort = netstat -an | Select-String ":8081.*LISTENING"
if ($edgePort) {
    Write-Host "✅ Edge server port 8081 is listening" -ForegroundColor Green
} else {
    Write-Host "❌ Edge server port 8081 not listening" -ForegroundColor Red
}

# Test tracker
$trackerPort = netstat -an | Select-String ":8090.*LISTENING"
if ($trackerPort) {
    Write-Host "✅ Tracker port 8090 is listening" -ForegroundColor Green
} else {
    Write-Host "❌ Tracker port 8090 not listening" -ForegroundColor Red
}

# Start web server
Write-Host "Starting web server..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-Command", "cd '$PWD\web'; python -m http.server 8000" -WindowStyle Hidden
Start-Sleep -Seconds 3

# Test web server
$webPort = netstat -an | Select-String ":8000.*LISTENING"
if ($webPort) {
    Write-Host "✅ Web server port 8000 is listening" -ForegroundColor Green
} else {
    Write-Host "❌ Web server port 8000 not listening" -ForegroundColor Red
}

# Start strategic peers with smaller count for testing
Write-Host "Starting 200 strategic peers for testing..." -ForegroundColor Yellow
Start-Process -FilePath ".\bin\strategic-peers.exe" -ArgumentList "200" -WindowStyle Normal

Write-Host ""
Write-Host "Services Status:" -ForegroundColor Cyan
Write-Host "=================" -ForegroundColor Cyan
docker ps --format "table {{.Names}}\t{{.Status}}"

Write-Host ""
Write-Host "Network Access:" -ForegroundColor Green
Write-Host "Dashboard: http://localhost:8000" -ForegroundColor Blue
Write-Host "Enhanced Peers: http://localhost:8000/enhanced-peers.html" -ForegroundColor Blue
Write-Host "Enhanced Player: http://localhost:8000/enhanced-player.html" -ForegroundColor Blue

Write-Host ""
Write-Host "If you still see connection errors, try:" -ForegroundColor Yellow
Write-Host "1. Restart Docker Desktop" -ForegroundColor White
Write-Host "2. Check Windows Firewall settings" -ForegroundColor White
Write-Host "3. Try running as Administrator" -ForegroundColor White
