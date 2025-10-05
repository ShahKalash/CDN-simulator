# Test script for song upload functionality
Write-Host "🎵 Testing Song Upload System" -ForegroundColor Green

# Check if services are running
Write-Host "Checking services..." -ForegroundColor Yellow

try {
    $response = Invoke-RestMethod -Uri "http://localhost:8093/health" -Method Get
    Write-Host "✅ Song Manager service is running" -ForegroundColor Green
} catch {
    Write-Host "❌ Song Manager service is not running. Please start it first." -ForegroundColor Red
    Write-Host "Run: go run cmd/song-manager/main.go" -ForegroundColor Yellow
    exit 1
}

try {
    $response = Invoke-RestMethod -Uri "http://localhost:8092/health" -Method Get
    Write-Host "✅ Network Topology service is running" -ForegroundColor Green
} catch {
    Write-Host "❌ Network Topology service is not running. Please start it first." -ForegroundColor Red
    Write-Host "Run: go run cmd/network-topology/main.go" -ForegroundColor Yellow
    exit 1
}

# Check if we have a test audio file
$testFile = "Rick-Roll-Sound-Effect.mp3"
if (Test-Path $testFile) {
    Write-Host "✅ Found test audio file: $testFile" -ForegroundColor Green
    
    # Upload the test file
    Write-Host "Uploading test song..." -ForegroundColor Yellow
    
    $boundary = [System.Guid]::NewGuid().ToString()
    $LF = "`r`n"
    
    $bodyLines = (
        "--$boundary",
        "Content-Disposition: form-data; name=`"title`"",
        "",
        "Test Song - Rick Roll",
        "--$boundary",
        "Content-Disposition: form-data; name=`"artist`"",
        "",
        "Test Artist",
        "--$boundary",
        "Content-Disposition: form-data; name=`"audio`"; filename=`"$testFile`"",
        "Content-Type: audio/mpeg",
        "",
        [System.IO.File]::ReadAllBytes($testFile),
        "--$boundary--"
    )
    
    $body = $bodyLines -join $LF
    
    try {
        $response = Invoke-RestMethod -Uri "http://localhost:8093/upload" -Method Post -Body $body -ContentType "multipart/form-data; boundary=$boundary"
        Write-Host "✅ Song uploaded successfully!" -ForegroundColor Green
        Write-Host "Song ID: $($response.id)" -ForegroundColor Cyan
        Write-Host "Title: $($response.title)" -ForegroundColor Cyan
        Write-Host "Artist: $($response.artist)" -ForegroundColor Cyan
        Write-Host "Status: $($response.status)" -ForegroundColor Cyan
        
        Write-Host "`n🎵 You can now:" -ForegroundColor Yellow
        Write-Host "1. Open http://localhost:8000/web/song-upload.html to see the upload interface" -ForegroundColor White
        Write-Host "2. Open http://localhost:8000/web/interactive-network.html to see the network visualization" -ForegroundColor White
        Write-Host "3. The song will be processed in the background and distributed to the CDN" -ForegroundColor White
        
    } catch {
        Write-Host "❌ Upload failed: $($_.Exception.Message)" -ForegroundColor Red
    }
} else {
    Write-Host "❌ Test audio file not found: $testFile" -ForegroundColor Red
    Write-Host "Please ensure you have an MP3 file to test with." -ForegroundColor Yellow
}

Write-Host "`n🚀 Demo is ready! Open the web interfaces to test the full system." -ForegroundColor Green
