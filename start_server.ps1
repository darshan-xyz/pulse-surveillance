Write-Host "================================================================" -ForegroundColor Cyan
Write-Host "   Pulse Live WebRTC Surveillance & Remote Streaming Pipeline   " -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan

# Create data directories if they don't exist
if (-Not (Test-Path "./data/postgres")) { New-Item -ItemType Directory -Force -Path "./data/postgres" | Out-Null }
if (-Not (Test-Path "./data/valkey")) { New-Item -ItemType Directory -Force -Path "./data/valkey" | Out-Null }

Write-Host "`n[1/3] Starting Database and Cloud Backend Services..." -ForegroundColor Yellow
docker compose -f docker-compose.local.yml up -d --build postgres valkey pulse-cloud

Write-Host "`n[2/3] Establishing Zero-Configuration Cloudflare Tunnel..." -ForegroundColor Yellow
# Restart the tunnel container to obtain a fresh public HTTPS ingress tunnel
docker compose -f docker-compose.local.yml stop cloudflared 2>$null | Out-Null
docker stop pulse-tunnel 2>$null | Out-Null
docker rm pulse-tunnel 2>$null | Out-Null
docker compose -f docker-compose.local.yml up -d cloudflared | Out-Null

Write-Host "`n[3/3] Discovering Public & Local Streaming Ingress Endpoints..." -ForegroundColor Yellow

# Poll tunnel logs for up to 20 seconds to reliably extract assigned trycloudflare URL
$timeout = 20
$elapsed = 0
$publicUrl = $null

while (-not $publicUrl -and $elapsed -lt $timeout) {
    Start-Sleep -Seconds 1
    $elapsed++
    $tunnelLogs = docker logs pulse-tunnel 2>&1
    if ($tunnelLogs -match '(https://[a-zA-Z0-9-]+\.trycloudflare\.com)') {
        $publicUrl = $matches[1]
    }
}

# Resolve local LAN IPv4 address for local network viewing
$localIp = "localhost"
try {
    $foundIp = (Get-NetIPAddress -AddressFamily IPv4 | Where-Object { 
        $_.IPAddress -notmatch '^127\.' -and 
        $_.IPAddress -notmatch '^169\.254\.' -and 
        $_.InterfaceAlias -notmatch 'vEthernet|Loopback|Docker|WSL' 
    } | Select-Object -First 1).IPAddress
    if ($foundIp) { $localIp = $foundIp }
} catch {
    $localIp = "localhost"
}

if ($publicUrl) {
    Write-Host "`n================================================================" -ForegroundColor Green
    Write-Host "  SUCCESS! Remote Streaming Pipeline is Active and Online" -ForegroundColor Green
    Write-Host "================================================================" -ForegroundColor Green
    
    Write-Host "`n>>> 1-CLICK INSTANT VIEWER LINK (Share Anywhere / Scan on Mobile) <<<" -ForegroundColor White
    Write-Host "  $publicUrl/?viewer=true&autostart=true" -ForegroundColor Magenta
    
    Write-Host "`n--- Public Admin Console ---" -ForegroundColor Gray
    Write-Host "  $publicUrl/" -ForegroundColor Cyan
    
    Write-Host "`n--- Local LAN 1-Click Viewer ---" -ForegroundColor Gray
    Write-Host "  http://${localIp}:5173/?viewer=true&autostart=true" -ForegroundColor Yellow
    
    Write-Host "`n--- Local LAN Admin Console ---" -ForegroundColor Gray
    Write-Host "  http://${localIp}:5173/" -ForegroundColor Yellow
    Write-Host "================================================================`n" -ForegroundColor Green
} else {
    Write-Host "`nWarning: Timed out waiting for Cloudflare public URL. Checking tunnel logs directly:" -ForegroundColor Red
    docker logs pulse-tunnel
    Write-Host "`nLocal streaming endpoints are still available:" -ForegroundColor Yellow
    Write-Host "  Local Viewer: http://${localIp}:5173/?viewer=true&autostart=true" -ForegroundColor Yellow
    Write-Host "  Local Console: http://${localIp}:5173/" -ForegroundColor Yellow
}
