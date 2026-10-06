Write-Host "Waiting for Docker Engine to initialize..." -ForegroundColor Yellow
$maxRetries = 60
$retryCount = 0
$dockerRunning = $false

while (-Not $dockerRunning -and $retryCount -lt $maxRetries) {
    docker info 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
        $dockerRunning = $true
        Write-Host "Docker is up!" -ForegroundColor Green
    } else {
        Start-Sleep -Seconds 2
        $retryCount++
    }
}

if ($dockerRunning) {
    Write-Host "Executing start_server.ps1..."
    .\start_server.ps1
} else {
    Write-Host "Docker failed to start after 120 seconds." -ForegroundColor Red
}
