#!/usr/bin/env pwsh
# Test concurrent request handling on a peer
# Demonstrates goroutine concurrency

param(
    [string]$PeerName = "peer-1",
    [string]$SegmentID = "nevergonnagiveyouup/128k/segment000.ts",
    [int]$ConcurrentRequests = 20,
    [int]$TotalRequests = 50
)

$ErrorActionPreference = "Continue"

Write-Host "================================================================================`n" -ForegroundColor Cyan
Write-Host "CONCURRENT REQUEST SIMULATION`n" -ForegroundColor Cyan
Write-Host "================================================================================`n" -ForegroundColor Cyan

Write-Host "Configuration:" -ForegroundColor Yellow
Write-Host "  Peer:              $PeerName" -ForegroundColor White
Write-Host "  Segment:           $SegmentID" -ForegroundColor White
Write-Host "  Concurrent:        $ConcurrentRequests requests" -ForegroundColor White
Write-Host "  Total:             $TotalRequests requests`n" -ForegroundColor White

# Function to make a single request and measure time
function Invoke-PeerRequest {
    param(
        [string]$Peer,
        [string]$Segment,
        [int]$RequestID
    )
    
    $startTime = Get-Date
    try {
        $response = docker exec $Peer wget -qO- "http://localhost:8080/request/$Segment" 2>&1
        $endTime = Get-Date
        $duration = ($endTime - $startTime).TotalMilliseconds
        
        $json = $response | ConvertFrom-Json -ErrorAction SilentlyContinue
        if ($json) {
            return [PSCustomObject]@{
                RequestID = $RequestID
                Success = $true
                Source = $json.source
                Hops = $json.hops
                RTTms = $json.rtt_ms
                EstRTTms = $json.est_rtt_ms
                Duration = [Math]::Round($duration, 2)
                Path = if ($json.path) { $json.path -join " -> " } else { "direct" }
            }
        } else {
            return [PSCustomObject]@{
                RequestID = $RequestID
                Success = $false
                Error = "Failed to parse response"
                Duration = [Math]::Round($duration, 2)
            }
        }
    } catch {
        $endTime = Get-Date
        $duration = ($endTime - $startTime).TotalMilliseconds
        return [PSCustomObject]@{
            RequestID = $RequestID
            Success = $false
            Error = $_.Exception.Message
            Duration = [Math]::Round($duration, 2)
        }
    }
}

# Test 1: Sequential requests (baseline)
Write-Host "`n[TEST 1] Sequential Requests (Baseline)" -ForegroundColor Green
Write-Host "Sending $ConcurrentRequests requests one after another...`n" -ForegroundColor Gray

$sequentialResults = @()
$seqStart = Get-Date
for ($i = 1; $i -le $ConcurrentRequests; $i++) {
    Write-Host "  Request $i..." -NoNewline
    $result = Invoke-PeerRequest -Peer $PeerName -Segment $SegmentID -RequestID $i
    $sequentialResults += $result
    if ($result.Success) {
        Write-Host " ✓ (${result.Duration}ms, source: $($result.Source))" -ForegroundColor Green
    } else {
        Write-Host " ✗ (${result.Duration}ms)" -ForegroundColor Red
    }
}
$seqEnd = Get-Date
$seqTotal = ($seqEnd - $seqStart).TotalSeconds

Write-Host "`nSequential Results:" -ForegroundColor Yellow
Write-Host "  Total Time:       $([Math]::Round($seqTotal, 2))s" -ForegroundColor White
Write-Host "  Avg per Request:  $([Math]::Round($seqTotal / $ConcurrentRequests, 2))s" -ForegroundColor White
Write-Host "  Successful:       $(($sequentialResults | Where-Object { $_.Success }).Count) / $ConcurrentRequests" -ForegroundColor White

# Test 2: Concurrent requests (goroutines)
Write-Host "`n`n[TEST 2] Concurrent Requests (Goroutines)" -ForegroundColor Green
Write-Host "Sending $ConcurrentRequests requests simultaneously...`n" -ForegroundColor Gray

$concurrentResults = @()
$conStart = Get-Date

# Create jobs for concurrent execution
$jobs = @()
for ($i = 1; $i -le $ConcurrentRequests; $i++) {
    $job = Start-Job -ScriptBlock {
        param($Peer, $Segment, $ID)
        $response = docker exec $Peer wget -qO- "http://localhost:8080/request/$Segment" 2>&1
        $json = $response | ConvertFrom-Json -ErrorAction SilentlyContinue
        if ($json) {
            return @{
                RequestID = $ID
                Success = $true
                Source = $json.source
                Hops = $json.hops
                RTTms = $json.rtt_ms
                EstRTTms = $json.est_rtt_ms
                Path = if ($json.path) { $json.path -join " -> " } else { "direct" }
            }
        } else {
            return @{
                RequestID = $ID
                Success = $false
            }
        }
    } -ArgumentList $PeerName, $SegmentID, $i
    $jobs += $job
}

# Wait for all jobs and collect results
$completed = 0
while ($completed -lt $jobs.Count) {
    $completed = ($jobs | Where-Object { $_.State -eq "Completed" }).Count
    Write-Host "  Progress: $completed / $ConcurrentRequests requests completed" -NoNewline -ForegroundColor Gray
    Start-Sleep -Milliseconds 100
    Write-Host "`r" -NoNewline
}

Write-Host "`n"

foreach ($job in $jobs) {
    $result = Receive-Job -Job $job
    Remove-Job -Job $job
    if ($result) {
        $concurrentResults += [PSCustomObject]$result
    }
}

$conEnd = Get-Date
$conTotal = ($conEnd - $conStart).TotalSeconds

Write-Host "Concurrent Results:" -ForegroundColor Yellow
Write-Host "  Total Time:       $([Math]::Round($conTotal, 2))s" -ForegroundColor White
Write-Host "  Avg per Request:  $([Math]::Round($conTotal / $ConcurrentRequests, 2))s" -ForegroundColor White
Write-Host "  Successful:       $(($concurrentResults | Where-Object { $_.Success }).Count) / $ConcurrentRequests" -ForegroundColor White

# Test 3: Burst of many requests
Write-Host "`n`n[TEST 3] Burst Test ($TotalRequests requests)" -ForegroundColor Green
Write-Host "Sending $TotalRequests requests in batches of $ConcurrentRequests...`n" -ForegroundColor Gray

$burstResults = @()
$burstStart = Get-Date

$batches = [Math]::Ceiling($TotalRequests / $ConcurrentRequests)
for ($batch = 0; $batch -lt $batches; $batch++) {
    $startID = ($batch * $ConcurrentRequests) + 1
    $endID = [Math]::Min(($batch + 1) * $ConcurrentRequests, $TotalRequests)
    $batchSize = $endID - $startID + 1
    
    Write-Host "  Batch $($batch + 1)/$batches (requests $startID-$endID)..." -ForegroundColor Gray
    
    $batchJobs = @()
    for ($i = $startID; $i -le $endID; $i++) {
        $job = Start-Job -ScriptBlock {
            param($Peer, $Segment, $ID)
            $start = Get-Date
            $response = docker exec $Peer wget -qO- "http://localhost:8080/request/$Segment" 2>&1
            $end = Get-Date
            $duration = ($end - $start).TotalMilliseconds
            
            $json = $response | ConvertFrom-Json -ErrorAction SilentlyContinue
            if ($json) {
                return @{
                    RequestID = $ID
                    Success = $true
                    Source = $json.source
                    Hops = $json.hops
                    RTTms = $json.rtt_ms
                    Duration = [Math]::Round($duration, 2)
                }
            } else {
                return @{
                    RequestID = $ID
                    Success = $false
                    Duration = [Math]::Round($duration, 2)
                }
            }
        } -ArgumentList $PeerName, $SegmentID, $i
        $batchJobs += $job
    }
    
    # Wait for batch to complete
    $batchJobs | Wait-Job | Out-Null
    foreach ($job in $batchJobs) {
        $result = Receive-Job -Job $job
        Remove-Job -Job $job
        if ($result) {
            $burstResults += [PSCustomObject]$result
        }
    }
}

$burstEnd = Get-Date
$burstTotal = ($burstEnd - $burstStart).TotalSeconds

Write-Host "`nBurst Test Results:" -ForegroundColor Yellow
Write-Host "  Total Time:       $([Math]::Round($burstTotal, 2))s" -ForegroundColor White
Write-Host "  Requests/sec:     $([Math]::Round($TotalRequests / $burstTotal, 2))" -ForegroundColor White
Write-Host "  Successful:       $(($burstResults | Where-Object { $_.Success }).Count) / $TotalRequests" -ForegroundColor White

# Statistics
Write-Host "`n================================================================================`n" -ForegroundColor Cyan
Write-Host "STATISTICS`n" -ForegroundColor Cyan
Write-Host "================================================================================`n" -ForegroundColor Cyan

if ($concurrentResults.Count -gt 0) {
    $successful = $concurrentResults | Where-Object { $_.Success }
    if ($successful.Count -gt 0) {
        Write-Host "Concurrent Request Analysis:" -ForegroundColor Yellow
        Write-Host "  Source Distribution:" -ForegroundColor White
        $sourceGroups = $successful | Group-Object Source
        foreach ($group in $sourceGroups) {
            Write-Host "    $($group.Name): $($group.Count) requests" -ForegroundColor Cyan
        }
        
        if ($successful | Where-Object { $_.RTTms }) {
            $avgRTT = ($successful | Where-Object { $_.RTTms } | Measure-Object -Property RTTms -Average).Average
            $minRTT = ($successful | Where-Object { $_.RTTms } | Measure-Object -Property RTTms -Minimum).Minimum
            $maxRTT = ($successful | Where-Object { $_.RTTms } | Measure-Object -Property RTTms -Maximum).Maximum
            Write-Host "`n  RTT Statistics:" -ForegroundColor White
            Write-Host "    Average: $([Math]::Round($avgRTT, 2))ms" -ForegroundColor Green
            Write-Host "    Min:     $minRTT ms" -ForegroundColor Green
            Write-Host "    Max:     $maxRTT ms" -ForegroundColor $(if ($maxRTT -gt 100) { "Yellow" } else { "Green" })
        }
    }
}

# Performance comparison
Write-Host "`nPerformance Comparison:" -ForegroundColor Yellow
if ($seqTotal -gt 0 -and $conTotal -gt 0) {
    $speedup = $seqTotal / $conTotal
    Write-Host "  Sequential:  $([Math]::Round($seqTotal, 2))s" -ForegroundColor White
    Write-Host "  Concurrent:  $([Math]::Round($conTotal, 2))s" -ForegroundColor White
    Write-Host "  Speedup:     ${speedup}x faster with goroutines!" -ForegroundColor Green
}

Write-Host "`n================================================================================`n" -ForegroundColor Cyan
Write-Host "Test Complete!`n" -ForegroundColor Green
Write-Host "This demonstrates how the peer's HTTP server handles concurrent requests" -ForegroundColor Gray
Write-Host "using Go goroutines - each request is handled in parallel.`n" -ForegroundColor Gray


