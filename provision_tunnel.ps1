Write-Host "================================================================" -ForegroundColor Cyan
Write-Host "         Pulse Cloudflare Remote Ingress Provisioner            " -ForegroundColor Cyan
Write-Host "================================================================" -ForegroundColor Cyan

# Create storage data directories
if (-Not (Test-Path "./data/postgres")) { New-Item -ItemType Directory -Force -Path "./data/postgres" | Out-Null }
if (-Not (Test-Path "./data/valkey")) { New-Item -ItemType Directory -Force -Path "./data/valkey" | Out-Null }

Write-Host "`n[1/2] Ensuring Backend Infrastructure is Active..." -ForegroundColor Yellow
docker compose -f docker-compose.local.yml up -d postgres valkey pulse-cloud

Write-Host "`n[2/2] Launching and Negotiating Cloudflare Quick Tunnel..." -ForegroundColor Yellow
docker compose -f docker-compose.local.yml stop cloudflared 2>$null | Out-Null
docker stop pulse-tunnel 2>$null | Out-Null
docker rm pulse-tunnel 2>$null | Out-Null
docker compose -f docker-compose.local.yml up -d cloudflared | Out-Null

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
    Write-Host "  PROVISION SUCCESS: Cloudflare Ingress Tunnel is Online" -ForegroundColor Green
    Write-Host "================================================================" -ForegroundColor Green
    
    Write-Host "`n>>> 1-CLICK INSTANT VIEWER LINK <<<" -ForegroundColor White
    Write-Host "  $publicUrl/?viewer=true&autostart=true" -ForegroundColor Magenta
    
    Write-Host "`n--- Public Web Dashboard ---" -ForegroundColor Gray
    Write-Host "  $publicUrl/" -ForegroundColor Cyan
    
    Write-Host "`n--- Local LAN 1-Click Viewer ---" -ForegroundColor Gray
    Write-Host "  http://${localIp}:5173/?viewer=true&autostart=true" -ForegroundColor Yellow
    Write-Host "================================================================`n" -ForegroundColor Green
} else {
    Write-Host "`nFailed to capture public URL from tunnel logs." -ForegroundColor Red
    docker logs pulse-tunnel
}
