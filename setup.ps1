#!/usr/bin/env pwsh
# Quick CDN P2P Network Setup

param(
    [int]$Peers = 800,
    [switch]$Clean
)

Write-Host "CDN P2P Network Setup" -ForegroundColor Green
Write-Host "=====================" -ForegroundColor Green

if ($Clean) {
    Write-Host "Cleaning up..." -ForegroundColor Yellow
    Get-Process | Where-Object {$_.ProcessName -eq "go"} | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

# Start Docker services
Write-Host "Starting Docker services..." -ForegroundColor Yellow
docker-compose up -d
Start-Sleep -Seconds 8

# Start web server
Write-Host "Starting web server..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-Command", "cd '$PWD\web'; python -m http.server 8000" -WindowStyle Hidden
Start-Sleep -Seconds 3

# Build strategic peer system
Write-Host "Building strategic peer system..." -ForegroundColor Yellow
go build -o bin/strategic-peers.exe ./tools/strategic-peers

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# Start strategic peers
Write-Host "Starting $Peers strategic P2P peers..." -ForegroundColor Green
Start-Process -FilePath ".\bin\strategic-peers.exe" -ArgumentList "$Peers" -WindowStyle Normal

# Wait for setup
Write-Host "Waiting for network setup..." -ForegroundColor Yellow
Start-Sleep -Seconds 15

Write-Host ""
Write-Host "P2P CDN Network Ready!" -ForegroundColor Green
Write-Host ""
Write-Host "Access Points:" -ForegroundColor Cyan
Write-Host "   Dashboard: http://localhost:8000" -ForegroundColor Blue
Write-Host "   Enhanced Peers: http://localhost:8000/enhanced-peers.html" -ForegroundColor Blue
Write-Host "   Enhanced Player: http://localhost:8000/enhanced-player.html" -ForegroundColor Blue
Write-Host ""
Write-Host "Test Scenarios:" -ForegroundColor Yellow
Write-Host "   segment000.ts: HIGH P2P availability (should route P2P)" -ForegroundColor Green
Write-Host "   segment001.ts: MEDIUM P2P availability (P2P vs Edge)" -ForegroundColor Yellow
Write-Host "   segment002.ts: LOW P2P availability (should route Edge)" -ForegroundColor DarkYellow
Write-Host "   segment003.ts: VERY LOW P2P availability (Edge preferred)" -ForegroundColor Red
Write-Host "   segment004.ts: NO P2P availability (Edge/Origin only)" -ForegroundColor Magenta
Write-Host ""
Write-Host "This network tests all CDN routing scenarios!" -ForegroundColor Green
