#!/usr/bin/env pwsh
# Comprehensive CDN + P2P Simulation Script
# Compares P2P vs Edge performance with detailed metrics

param(
    [int]$PeerCount = 30,
    [string]$SegmentID = "nevergonnagiveyouup/128k/segment000.ts",
    [int]$TestIterations = 5
)

$ErrorActionPreference = "Continue"

Write-Host "================================================================================`n" -ForegroundColor Cyan
Write-Host "CDN + P2P PERFORMANCE COMPARISON SIMULATION`n" -ForegroundColor Cyan
Write-Host "================================================================================`n" -ForegroundColor Cyan

# Helper function to get clean JSON response
function Get-SegmentRequest {
    param(
        [string]$PeerName,
        [string]$SegmentID
    )
    
    try {
        $response = docker exec $PeerName wget -qO- "http://localhost:8080/request/$SegmentID" 2>&1
        if ($LASTEXITCODE -eq 0 -and $response) {
            # Try to parse JSON
            $json = $response | ConvertFrom-Json -ErrorAction SilentlyContinue
            if ($json) {
                return $json
            }
        }
        return $null
    } catch {
        return $null
    }
}

# Helper function to get path to edge
function Get-PathToEdge {
    param(
        [string]$PeerName,
        [string]$EdgeName
    )
    
    try {
        $response = Invoke-RestMethod -Uri "http://localhost:8090/path?from=$PeerName&to=$EdgeName" -ErrorAction SilentlyContinue
        if ($response) {
            return $response
        }
        return $null
    } catch {
        return $null
    }
}

# Helper function to get peer RTT measurements
function Get-PeerRTT {
    param([string]$PeerName)
    
    try {
        $response = docker exec $PeerName wget -qO- "http://localhost:8080/rtt" 2>&1
        if ($LASTEXITCODE -eq 0 -and $response) {
            $json = $response | ConvertFrom-Json -ErrorAction SilentlyContinue
            return $json
        }
        return $null
    } catch {
        return $null
    }
}

# Function to display segment request results cleanly
function Show-SegmentRequest {
    param(
        [string]$PeerName,
        [string]$SegmentID
    )
    
    Write-Host "`n[REQUEST] $PeerName -> $SegmentID" -ForegroundColor Yellow
    
    $result = Get-SegmentRequest -PeerName $PeerName -SegmentID $SegmentID
    if (-not $result) {
        Write-Host "  ERROR: Failed to get response" -ForegroundColor Red
        return $null
    }
    
    Write-Host "  Source:     $($result.source)" -ForegroundColor $(if ($result.source -eq "local") { "Green" } elseif ($result.source -eq "p2p") { "Cyan" } else { "Yellow" })
    
    if ($result.path -and $result.path.Count -gt 0) {
        $pathStr = $result.path -join " -> "
        Write-Host "  Path:       $pathStr" -ForegroundColor White
    } else {
        Write-Host "  Path:       (direct)" -ForegroundColor Gray
    }
    
    if ($result.hops) {
        Write-Host "  Hops:       $($result.hops)" -ForegroundColor White
    }
    
    if ($result.rtt_ms) {
        Write-Host "  RTT:        $($result.rtt_ms)ms" -ForegroundColor $(if ($result.rtt_ms -lt 50) { "Green" } elseif ($result.rtt_ms -lt 100) { "Yellow" } else { "Red" })
    }
    
    if ($result.est_rtt_ms) {
        Write-Host "  Est. RTT:   $($result.est_rtt_ms)ms" -ForegroundColor Gray
    }
    
    return $result
}

# Function to analyze path to edge
function Show-PathToEdge {
    param(
        [string]$PeerName,
        [string]$EdgeName
    )
    
    Write-Host "`n[PATH ANALYSIS] $PeerName -> $EdgeName" -ForegroundColor Magenta
    
    $pathInfo = Get-PathToEdge -PeerName $PeerName -EdgeName $EdgeName
    if (-not $pathInfo) {
        Write-Host "  ERROR: Failed to get path information" -ForegroundColor Red
        return $null
    }
    
    if ($pathInfo.path -and $pathInfo.path.Count -gt 0) {
        $pathStr = $pathInfo.path -join " -> "
        Write-Host "  Path:       $pathStr" -ForegroundColor White
    }
    
    if ($pathInfo.hops) {
        Write-Host "  Hops:       $($pathInfo.hops)" -ForegroundColor White
    }
    
    if ($pathInfo.estimated_rtt_ms) {
        Write-Host "  Est. RTT:   $($pathInfo.estimated_rtt_ms)ms" -ForegroundColor Gray
    }
    
    return $pathInfo
}

# Main simulation
Write-Host "Step 1: Checking available peers..." -ForegroundColor Green
$peers = docker ps --filter "name=peer-" --format "{{.Names}}" | Where-Object { $_ -match "peer-\d+" }
$peerList = $peers | Sort-Object

if ($peerList.Count -eq 0) {
    Write-Host "ERROR: No peer containers found. Please deploy the network first." -ForegroundColor Red
    exit 1
}

Write-Host "Found $($peerList.Count) peers: $($peerList -join ', ')" -ForegroundColor Green

Write-Host "`nStep 2: Analyzing shortest paths to edge nodes..." -ForegroundColor Green
$edge1Path = @{}
$edge2Path = @{}

foreach ($peer in $peerList) {
    $path1 = Show-PathToEdge -PeerName $peer -EdgeName "edge-1"
    $path2 = Show-PathToEdge -PeerName $peer -EdgeName "edge-2"
    
    if ($path1) {
        $edge1Path[$peer] = @{
            Hops = $path1.hops
            EstRTT = $path1.estimated_rtt_ms
            Path = $path1.path
        }
    }
    
    if ($path2) {
        $edge2Path[$peer] = @{
            Hops = $path2.hops
            EstRTT = $path2.estimated_rtt_ms
            Path = $path2.path
        }
    }
    
    Start-Sleep -Milliseconds 100
}

Write-Host "`nStep 3: Testing segment requests and comparing P2P vs Edge..." -ForegroundColor Green

# Select test peers
$testPeers = $peerList | Select-Object -First [Math]::Min(10, $peerList.Count)
$results = @()

foreach ($peer in $testPeers) {
    Write-Host "`n--- Testing $peer ---" -ForegroundColor Cyan
    
    # Clear cache first (by requesting a different segment or waiting)
    Write-Host "  Clearing cache (requesting different segment)..." -ForegroundColor Gray
    docker exec $peer wget -qO- "http://localhost:8080/request/rickroll/128k/segment999.ts" 2>&1 | Out-Null
    
    Start-Sleep -Milliseconds 500
    
    # Test request
    $result = Show-SegmentRequest -PeerName $peer -SegmentID $SegmentID
    
    if ($result) {
        $edgeInfo = $null
        if ($edge1Path.ContainsKey($peer)) {
            $edgeInfo = $edge1Path[$peer]
        } elseif ($edge2Path.ContainsKey($peer)) {
            $edgeInfo = $edge2Path[$peer]
        }
        
        $results += [PSCustomObject]@{
            Peer = $peer
            Source = $result.source
            Hops = $result.hops
            RTT = $result.rtt_ms
            EstRTT = $result.est_rtt_ms
            PathToEdgeHops = if ($edgeInfo) { $edgeInfo.Hops } else { $null }
            PathToEdgeRTT = if ($edgeInfo) { $edgeInfo.EstRTT } else { $null }
            Path = if ($result.path) { ($result.path -join " -> ") } else { "direct" }
        }
    }
    
    Start-Sleep -Milliseconds 200
}

Write-Host "`n================================================================================`n" -ForegroundColor Cyan
Write-Host "PERFORMANCE COMPARISON SUMMARY`n" -ForegroundColor Cyan
Write-Host "================================================================================`n" -ForegroundColor Cyan

if ($results.Count -eq 0) {
    Write-Host "No results to display." -ForegroundColor Red
    exit 1
}

# Display results table
Write-Host ("{0,-10} {1,-8} {2,-6} {3,-8} {4,-10} {5,-8} {6,-10}" -f "Peer", "Source", "Hops", "RTT(ms)", "Est.RTT", "EdgeHops", "EdgeRTT") -ForegroundColor Yellow
Write-Host ("-" * 70) -ForegroundColor Gray

foreach ($r in $results) {
    $sourceColor = switch ($r.Source) {
        "local" { "Green" }
        "p2p" { "Cyan" }
        "edge" { "Yellow" }
        default { "White" }
    }
    
    Write-Host ("{0,-10} {1,-8} {2,-6} {3,-8} {4,-10} {5,-8} {6,-10}" -f `
        $r.Peer, `
        $r.Source, `
        $(if ($r.Hops) { $r.Hops } else { "-" }), `
        $(if ($r.RTT) { $r.RTT } else { "-" }), `
        $(if ($r.EstRTT) { $r.EstRTT } else { "-" }), `
        $(if ($r.PathToEdgeHops) { $r.PathToEdgeHops } else { "-" }), `
        $(if ($r.PathToEdgeRTT) { $r.PathToEdgeRTT } else { "-" }))
}

# Calculate statistics
Write-Host "`n--- Statistics ---" -ForegroundColor Cyan

$p2pResults = $results | Where-Object { $_.Source -eq "p2p" }
$edgeResults = $results | Where-Object { $_.Source -eq "edge" }
$localResults = $results | Where-Object { $_.Source -eq "local" }

Write-Host "`nBy Source Type:" -ForegroundColor Yellow
Write-Host "  Local cache: $($localResults.Count) requests" -ForegroundColor Green
Write-Host "  P2P:         $($p2pResults.Count) requests" -ForegroundColor Cyan
Write-Host "  Edge:        $($edgeResults.Count) requests" -ForegroundColor Yellow

if ($p2pResults.Count -gt 0) {
    $avgP2PRTT = ($p2pResults | Where-Object { $_.RTT } | Measure-Object -Property RTT -Average).Average
    $avgP2PHops = ($p2pResults | Where-Object { $_.Hops } | Measure-Object -Property Hops -Average).Average
    Write-Host "`nP2P Performance:" -ForegroundColor Cyan
    Write-Host "  Average RTT:  $([Math]::Round($avgP2PRTT, 2))ms" -ForegroundColor White
    Write-Host "  Average Hops: $([Math]::Round($avgP2PHops, 2))" -ForegroundColor White
}

if ($edgeResults.Count -gt 0) {
    $avgEdgeRTT = ($edgeResults | Where-Object { $_.RTT } | Measure-Object -Property RTT -Average).Average
    $avgEdgeHops = ($edgeResults | Where-Object { $_.PathToEdgeHops } | Measure-Object -Property PathToEdgeHops -Average).Average
    Write-Host "`nEdge Performance:" -ForegroundColor Yellow
    Write-Host "  Average RTT:  $([Math]::Round($avgEdgeRTT, 2))ms" -ForegroundColor White
    Write-Host "  Average Hops to Edge: $([Math]::Round($avgEdgeHops, 2))" -ForegroundColor White
}

# Comparison
if ($p2pResults.Count -gt 0 -and $edgeResults.Count -gt 0) {
    Write-Host "`n--- P2P vs Edge Comparison ---" -ForegroundColor Magenta
    
    $p2pAvgRTT = ($p2pResults | Where-Object { $_.RTT } | Measure-Object -Property RTT -Average).Average
    $edgeAvgRTT = ($edgeResults | Where-Object { $_.RTT } | Measure-Object -Property RTT -Average).Average
    
    if ($p2pAvgRTT -and $edgeAvgRTT) {
        $improvement = (($edgeAvgRTT - $p2pAvgRTT) / $edgeAvgRTT) * 100
        Write-Host "  P2P RTT:     $([Math]::Round($p2pAvgRTT, 2))ms" -ForegroundColor Cyan
        Write-Host "  Edge RTT:    $([Math]::Round($edgeAvgRTT, 2))ms" -ForegroundColor Yellow
        
        if ($improvement -gt 0) {
            Write-Host "  P2P is $([Math]::Round($improvement, 1))% faster!" -ForegroundColor Green
        } else {
            Write-Host "  Edge is $([Math]::Round(-$improvement, 1))% faster" -ForegroundColor Yellow
        }
    }
}

$separator = "=" * 80
Write-Host ""
Write-Host $separator -ForegroundColor Cyan
Write-Host ""
Write-Host "Simulation complete!" -ForegroundColor Green

