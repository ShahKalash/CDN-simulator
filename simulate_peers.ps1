# Simulate thousands of persistent peers for Rick Roll CDN
param(
    [int]$PeerCount = 5000,
    [string]$TrackerUrl = "http://localhost:8090",
    [string]$SignalingUrl = "ws://localhost:8091/ws"
)

Write-Host "🚀 Simulating $PeerCount peers for Rick Roll CDN..." -ForegroundColor Green

# Rick Roll segments (we have 5 segments: 000-004)
$segments = @(
    "rickroll/128k/segment000.ts",
    "rickroll/128k/segment001.ts", 
    "rickroll/128k/segment002.ts",
    "rickroll/128k/segment003.ts",
    "rickroll/128k/segment004.ts"
)

# Regions for geographic distribution
$regions = @("us-east", "us-west", "eu-west", "eu-central", "asia-pacific", "asia-southeast", "canada", "brazil", "australia", "japan")

# Bandwidth tiers (affects RTT and availability)
$bandwidthTiers = @(
    @{tier="fiber"; rtt=@(5,15); availability=0.95; segmentProb=0.8},
    @{tier="cable"; rtt=@(15,40); availability=0.85; segmentProb=0.6}, 
    @{tier="dsl"; rtt=@(40,80); availability=0.75; segmentProb=0.4},
    @{tier="mobile"; rtt=@(80,200); availability=0.65; segmentProb=0.3}
)

$successCount = 0
$errorCount = 0

Write-Host "📡 Registering peers with tracker..." -ForegroundColor Yellow

for ($i = 1; $i -le $PeerCount; $i++) {
    try {
        # Random peer characteristics
        $region = $regions | Get-Random
        $bandwidth = $bandwidthTiers | Get-Random
        $rtt = Get-Random -Minimum $bandwidth.rtt[0] -Maximum $bandwidth.rtt[1]
        
        # Determine which segments this peer has (realistic distribution)
        $peerSegments = @()
        foreach ($segment in $segments) {
            # Higher probability for earlier segments (more popular)
            $segmentIndex = [array]::IndexOf($segments, $segment)
            $probability = $bandwidth.segmentProb * (1.0 - ($segmentIndex * 0.1))
            
            if ((Get-Random -Minimum 0.0 -Maximum 1.0) -lt $probability) {
                $peerSegments += $segment
            }
        }
        
        # Only register peers that have at least one segment
        if ($peerSegments.Count -gt 0) {
            $peerData = @{
                peerId = "peer-$region-$($bandwidth.tier)-$i"
                addr = $SignalingUrl
                segments = $peerSegments
                region = $region
                rtt = $rtt
                bandwidth = $bandwidth.tier
                lastSeen = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
                availability = $bandwidth.availability
            } | ConvertTo-Json -Depth 3
            
            $response = Invoke-WebRequest -Uri "$TrackerUrl/announce" -Method POST -Body $peerData -ContentType "application/json" -UseBasicParsing -TimeoutSec 5
            
            if ($response.StatusCode -eq 204) {
                $successCount++
                if ($successCount % 100 -eq 0) {
                    Write-Host "✅ Registered $successCount peers..." -ForegroundColor Green
                }
            }
        }
    }
    catch {
        $errorCount++
        if ($errorCount % 50 -eq 0) {
            Write-Host "⚠️  $errorCount registration errors so far..." -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "🎉 Peer Registration Complete!" -ForegroundColor Green
Write-Host "✅ Successfully registered: $successCount peers" -ForegroundColor Green
Write-Host "❌ Registration errors: $errorCount" -ForegroundColor Red
Write-Host ""
Write-Host "📊 Testing peer distribution..." -ForegroundColor Cyan

# Test peer distribution for each segment
foreach ($segment in $segments) {
    try {
        $response = Invoke-WebRequest -Uri "$TrackerUrl/peers?seg=$segment&count=50&region=us-east" -UseBasicParsing -TimeoutSec 5
        $peers = $response.Content | ConvertFrom-Json
        Write-Host "🎵 $segment : $($peers.Count) peers available" -ForegroundColor White
    }
    catch {
        Write-Host "❌ Error checking $segment" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "🌐 Geographic Distribution Test:" -ForegroundColor Cyan
foreach ($region in $regions[0..4]) {  # Test first 5 regions
    try {
        $response = Invoke-WebRequest -Uri "$TrackerUrl/peers?seg=rickroll/128k/segment000.ts&count=20&region=$region" -UseBasicParsing -TimeoutSec 5
        $peers = $response.Content | ConvertFrom-Json
        Write-Host "🌍 $region : $($peers.Count) peers with segment000" -ForegroundColor White
    }
    catch {
        Write-Host "❌ Error checking region $region" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "🎊 Simulation Complete! Your CDN now has thousands of persistent peers!" -ForegroundColor Green
Write-Host "🌐 Visit http://localhost:8000/peers.html to explore the peer network" -ForegroundColor Cyan
Write-Host "🎵 Visit http://localhost:8000/index.html to test Rick Roll streaming" -ForegroundColor Cyan
