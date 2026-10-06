#!/usr/bin/env node
/**
 * Pulse Automated Ingress Tunnel Provisioner
 * 
 * Provides zero-configuration public ingress tunneling via Cloudflare Quick Tunnels,
 * dynamically extracting the assigned domain and generating one-click direct viewer
 * URLs with automatic authenticated session bootstrapping and offline LAN fallback.
 */

const fs = require('fs');
const path = require('path');
const os = require('os');
const http = require('http');
const https = require('https');
const net = require('net');
const { spawn, execSync } = require('child_process');

// Regular expressions for tunnel discovery
const ANSI_REGEX = /\x1B\[[0-9;]*[a-zA-Z]/g;
const TRYCLOUDFLARE_REGEX = /https:\/\/[a-zA-Z0-9-]+\.trycloudflare\.com/i;
const GENERIC_TUNNEL_REGEX = /https:\/\/[a-zA-Z0-9-.]+\.(?:trycloudflare\.com|ngrok-free\.app|ngrok\.app|loca\.lt)/i;

/**
 * Strips ANSI escape sequences from text.
 */
function stripAnsi(text) {
  if (typeof text !== 'string') return '';
  return text.replace(ANSI_REGEX, '');
}

/**
 * Returns the expected cloudflared executable binary name for the current platform.
 */
function getBinaryName(platform = process.platform) {
  return platform === 'win32' ? 'cloudflared.exe' : 'cloudflared';
}

/**
 * Resolves the official Cloudflare GitHub release download URL for a given OS and architecture.
 */
function getBinaryDownloadUrl(platform = process.platform, arch = process.arch) {
  const baseUrl = 'https://github.com/cloudflare/cloudflared/releases/latest/download';
  
  if (platform === 'win32') {
    if (arch === 'x64' || arch === 'arm64') {
      return `${baseUrl}/cloudflared-windows-amd64.exe`;
    }
    return `${baseUrl}/cloudflared-windows-386.exe`;
  }
  
  if (platform === 'darwin') {
    if (arch === 'arm64') {
      return `${baseUrl}/cloudflared-darwin-arm64`;
    }
    return `${baseUrl}/cloudflared-darwin-amd64`;
  }
  
  if (platform === 'linux') {
    if (arch === 'x64') {
      return `${baseUrl}/cloudflared-linux-amd64`;
    }
    if (arch === 'arm64') {
      return `${baseUrl}/cloudflared-linux-arm64`;
    }
    if (arch === 'arm') {
      return `${baseUrl}/cloudflared-linux-arm`;
    }
    if (arch === 'ia32') {
      return `${baseUrl}/cloudflared-linux-386`;
    }
  }

  throw new Error(`Unsupported platform/architecture for cloudflared: ${platform}/${arch}`);
}

/**
 * Searches for an existing cloudflared binary in PATH and local candidate directories.
 */
function findCloudflaredBinary(customSearchDirs = []) {
  const binaryName = getBinaryName();
  
  // 1. Check custom / project local bin directories
  const defaultBinDir = path.resolve(__dirname, '..', 'bin');
  const searchDirs = [...customSearchDirs, defaultBinDir, process.cwd(), path.join(process.cwd(), 'bin')];

  for (const dir of searchDirs) {
    if (!dir) continue;
    const candidate = path.join(dir, binaryName);
    if (fs.existsSync(candidate)) {
      try {
        fs.accessSync(candidate, fs.constants.X_OK || fs.constants.R_OK);
        return candidate;
      } catch {
        // Continue searching
      }
    }
  }

  // 2. Check system PATH
  try {
    const cmd = process.platform === 'win32' ? `where ${binaryName}` : `which ${binaryName}`;
    const result = execSync(cmd, { stdio: ['pipe', 'pipe', 'ignore'], encoding: 'utf-8' }).trim();
    if (result) {
      const firstPath = result.split(/\r?\n/)[0].trim();
      if (fs.existsSync(firstPath)) {
        return firstPath;
      }
    }
  } catch {
    // Not found in PATH
  }

  return null;
}

/**
 * Downloads a file with automatic redirect following.
 */
async function downloadFileWithRedirects(url, destPath, onProgress) {
  return new Promise((resolve, reject) => {
    function get(currentUrl, redirectCount = 0) {
      if (redirectCount > 10) {
        return reject(new Error('Too many HTTP redirects'));
      }

      const client = currentUrl.startsWith('https:') ? https : http;
      const request = client.get(currentUrl, { headers: { 'User-Agent': 'pulse-tunnel-provisioner/1.0' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          const redirectUrl = new URL(res.headers.location, currentUrl).href;
          return get(redirectUrl, redirectCount + 1);
        }

        if (res.statusCode !== 200) {
          return reject(new Error(`Failed to download binary: HTTP ${res.statusCode} ${res.statusMessage || ''}`));
        }

        const totalBytes = parseInt(res.headers['content-length'] || '0', 10);
        let downloadedBytes = 0;

        const fileStream = fs.createWriteStream(destPath);

        res.on('data', (chunk) => {
          downloadedBytes += chunk.length;
          if (onProgress && totalBytes > 0) {
            onProgress(downloadedBytes, totalBytes);
          }
        });

        res.pipe(fileStream);

        fileStream.on('finish', () => {
          fileStream.close(() => resolve(destPath));
        });

        fileStream.on('error', (err) => {
          fs.unlink(destPath, () => {});
          reject(err);
        });
      });

      request.on('error', (err) => {
        fs.unlink(destPath, () => {});
        reject(err);
      });
    }

    get(url);
  });
}

/**
 * Downloads the cloudflared binary into the specified directory if not present.
 */
async function downloadCloudflared(targetDir = path.resolve(__dirname, '..', 'bin'), logger = console) {
  if (!fs.existsSync(targetDir)) {
    fs.mkdirSync(targetDir, { recursive: true });
  }

  const binaryName = getBinaryName();
  const targetPath = path.join(targetDir, binaryName);
  const tempPath = path.join(targetDir, `${binaryName}.downloading`);
  const downloadUrl = getBinaryDownloadUrl();

  logger.log(`[*] Downloading cloudflared binary from: ${downloadUrl}`);
  logger.log(`[*] Target destination: ${targetPath}`);

  let lastPercent = -1;
  await downloadFileWithRedirects(downloadUrl, tempPath, (downloaded, total) => {
    const percent = Math.floor((downloaded / total) * 100);
    if (percent !== lastPercent && percent % 10 === 0) {
      lastPercent = percent;
      logger.log(`    ... ${percent}% downloaded (${(downloaded / 1024 / 1024).toFixed(1)} MB / ${(total / 1024 / 1024).toFixed(1)} MB)`);
    }
  });

  if (fs.existsSync(targetPath)) {
    try { fs.unlinkSync(targetPath); } catch {}
  }
  fs.renameSync(tempPath, targetPath);

  // Set executable permissions on Unix/macOS
  if (process.platform !== 'win32') {
    try {
      fs.chmodSync(targetPath, 0o755);
    } catch (err) {
      logger.warn(`[!] Failed to set chmod +x on ${targetPath}: ${err.message}`);
    }
  }

  logger.log(`[✓] cloudflared downloaded and cached successfully.`);
  return targetPath;
}

/**
 * Ensures cloudflared binary is available either in PATH, cached locally, or downloaded on demand.
 */
async function ensureCloudflared(binDir = path.resolve(__dirname, '..', 'bin'), logger = console) {
  const existing = findCloudflaredBinary([binDir]);
  if (existing) {
    logger.log(`[✓] Found cloudflared binary at: ${existing}`);
    return existing;
  }

  logger.log(`[*] cloudflared not found in PATH or local cache. Initiating automatic zero-config download...`);
  return await downloadCloudflared(binDir, logger);
}

/**
 * Parses stdout or stderr stream chunks from cloudflared to extract the assigned tunnel URL.
 */
function parseTunnelUrl(text) {
  if (!text) return null;
  const clean = stripAnsi(text);
  
  // Check trycloudflare regex first
  const tryCloudflareMatch = clean.match(TRYCLOUDFLARE_REGEX);
  if (tryCloudflareMatch) {
    return tryCloudflareMatch[0].trim();
  }

  // Check generic tunnel regex
  const genericMatch = clean.match(GENERIC_TUNNEL_REGEX);
  if (genericMatch) {
    return genericMatch[0].trim();
  }

  return null;
}

/**
 * Constructs a one-click direct viewer URL with required query parameters.
 */
function constructViewerUrl(baseUrl, options = {}) {
  if (!baseUrl) return '';
  let urlStr = baseUrl.trim();
  if (!urlStr.startsWith('http://') && !urlStr.startsWith('https://')) {
    urlStr = `https://${urlStr}`;
  }

  const url = new URL(urlStr);
  url.searchParams.set('viewer', 'true');
  url.searchParams.set('autostart', 'true');

  if (options.token) {
    url.searchParams.set('token', options.token);
  }
  if (options.camera) {
    url.searchParams.set('camera', options.camera);
  }

  return url.toString();
}

/**
 * Constructs an admin console URL from base URL.
 */
function constructAdminUrl(baseUrl) {
  if (!baseUrl) return '';
  let urlStr = baseUrl.trim();
  if (!urlStr.startsWith('http://') && !urlStr.startsWith('https://')) {
    urlStr = `https://${urlStr}`;
  }

  const url = new URL(urlStr);
  return `${url.protocol}//${url.host}/`;
}

/**
 * Retrieves non-internal IPv4 LAN addresses of the current host, ranked by interface priority.
 */
function getLanIpAddresses() {
  const interfaces = os.networkInterfaces();
  const scoredIps = [];

  for (const [name, addrs] of Object.entries(interfaces)) {
    if (!addrs) continue;
    for (const iface of addrs) {
      if (iface.family === 'IPv4' && !iface.internal && iface.address) {
        let score = 50;
        const lowerName = name.toLowerCase();
        
        // Penalize virtual / switch / container adapters first
        if (lowerName.includes('vethernet') || lowerName.includes('wsl') || lowerName.includes('virtual') || lowerName.includes('docker') || lowerName.includes('vbox') || lowerName.includes('vmware') || lowerName.includes('hyper-v')) {
          score = 10;
        } else if (lowerName.includes('wi-fi') || lowerName.includes('wlan') || lowerName.includes('wifi') || lowerName.includes('wireless')) {
          score = 100;
        } else if (lowerName.includes('eth') || lowerName.includes('ethernet') || lowerName.includes('en0') || lowerName.includes('lan')) {
          score = 90;
        }

        // Exclude link-local (169.254.x.x) or loopback
        if (!iface.address.startsWith('169.254.') && !iface.address.startsWith('127.')) {
          scoredIps.push({ ip: iface.address, score, ifaceName: name });
        }
      }
    }
  }

  // Sort descending by score
  scoredIps.sort((a, b) => b.score - a.score);
  return scoredIps.map(item => item.ip);
}

/**
 * Constructs local LAN fallback viewer URLs.
 */
function getLanViewerUrls(port = 5173, options = {}) {
  const ips = getLanIpAddresses();
  if (ips.length === 0) {
    return [constructViewerUrl(`http://localhost:${port}`, options)];
  }
  return ips.map(ip => constructViewerUrl(`http://${ip}:${port}`, options));
}

/**
 * Checks if a specific TCP port is listening.
 */
function checkPortListening(port, host = '127.0.0.1', timeout = 800) {
  return new Promise((resolve) => {
    const socket = new net.Socket();
    let resolved = false;

    function cleanupAndResolve(val) {
      if (!resolved) {
        resolved = true;
        socket.removeAllListeners();
        try { socket.destroy(); } catch {}
        try { socket.unref(); } catch {}
        resolve(val);
      }
    }

    socket.setTimeout(timeout);
    socket.once('connect', () => cleanupAndResolve(true));
    socket.once('timeout', () => cleanupAndResolve(false));
    socket.once('error', () => cleanupAndResolve(false));

    try {
      socket.connect(port, host);
    } catch {
      cleanupAndResolve(false);
    }
  });
}

/**
 * Checks the status of local frontend and backend services.
 */
async function checkLocalServices(options = {}) {
  const frontendPort = options.port || 5173;
  const backendPort = options.cloudPort || 9091;

  const [frontendUp, backendUp] = await Promise.all([
    checkPortListening(frontendPort, '127.0.0.1', 800),
    checkPortListening(backendPort, '127.0.0.1', 800)
  ]);

  return { frontendUp, backendUp, frontendPort, backendPort };
}

/**
 * Formats and prints the terminal banner.
 */
function renderBanner(data, logger = console) {
  const {
    tunnelUrl,
    viewerUrl,
    adminUrl,
    lanViewerUrls,
    serviceStatus,
    isOfflineFallback
  } = data;

  const width = 80;
  const separator = '='.repeat(width);
  const subSeparator = '-'.repeat(width);

  logger.log('');
  logger.log(separator);
  logger.log('          PULSE STREAMING - ZERO-CONFIGURATION INGRESS PROVISIONER');
  logger.log(separator);

  if (isOfflineFallback) {
    logger.log('  Mode: OFFLINE / LAN FALLBACK MODE (Cloudflare Ingress bypassed/unavailable)');
  } else {
    logger.log('  Mode: PUBLIC INGRESS TUNNEL ACTIVE (Cloudflare Quick Tunnel)');
    logger.log('  Ingress Protocol: Encrypted HTTPS / WSS Edge Proxy');
  }

  if (serviceStatus) {
    const feIcon = serviceStatus.frontendUp ? '[✓]' : '[!]';
    const beIcon = serviceStatus.backendUp ? '[✓]' : '[!]';
    const feMsg = serviceStatus.frontendUp ? 'ACTIVE' : 'NOT RUNNING (Run "npm run dev" in web_dashboard)';
    const beMsg = serviceStatus.backendUp ? 'ACTIVE' : 'NOT RUNNING (Run "go run ./cmd/pulse-cloud")';
    logger.log(`  ${feIcon} Frontend Dev Server (:${serviceStatus.frontendPort}): ${feMsg}`);
    logger.log(`  ${beIcon} Pulse Cloud Backend (:${serviceStatus.backendPort}): ${beMsg}`);
  }

  logger.log(subSeparator);
  logger.log('  >>> SINGLE CLICKABLE VIEWER LINK (Share with any mobile or external device):');
  logger.log(`      ${viewerUrl}`);
  logger.log(subSeparator);

  if (adminUrl && !isOfflineFallback) {
    logger.log('  [*] Public Admin Console:');
    logger.log(`      ${adminUrl}`);
    logger.log('');
  }

  if (lanViewerUrls && lanViewerUrls.length > 0) {
    logger.log('  [*] Local LAN Viewer Link(s) (Same Wi-Fi / Local Network):');
    for (const lanUrl of lanViewerUrls) {
      logger.log(`      ${lanUrl}`);
    }
    logger.log('');
  }

  logger.log('  [*] Local Reverse Proxy:');
  logger.log(`      http://localhost:${serviceStatus ? serviceStatus.frontendPort : 5173} (Multiplexes UI + /api + /client)`);

  logger.log(separator);
  logger.log('  Tunnel is active. Press Ctrl+C to terminate cleanly.');
  logger.log(separator);
  logger.log('');
}

/**
 * Spawns cloudflared tunnel process and captures the dynamically assigned tunnel domain.
 */
async function spawnTunnel(binaryPath, options = {}) {
  const port = options.port || 5173;
  const timeoutMs = options.timeout || 30000;
  const verbose = !!options.verbose;
  const logger = options.logger || console;

  const targetUrl = `http://127.0.0.1:${port}`;
  const args = ['tunnel', '--url', targetUrl, '--http-host-header', `localhost:${port}`];

  return new Promise((resolve, reject) => {
    logger.log(`[*] Spawning cloudflared tunnel targeting ${targetUrl}...`);

    const child = spawn(binaryPath, args, {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: process.env,
      windowsHide: true
    });

    let detectedUrl = null;
    let fullOutput = '';
    let timeoutHandle = null;

    function handleData(chunk) {
      const text = chunk.toString();
      fullOutput += text;

      if (verbose) {
        process.stdout.write(text);
      }

      if (!detectedUrl) {
        const found = parseTunnelUrl(text);
        if (found) {
          detectedUrl = found;
          if (timeoutHandle) clearTimeout(timeoutHandle);

          const viewerUrl = constructViewerUrl(detectedUrl, options);
          const adminUrl = constructAdminUrl(detectedUrl);
          const lanViewerUrls = getLanViewerUrls(port, options);

          resolve({
            url: detectedUrl,
            viewerUrl,
            adminUrl,
            lanViewerUrls,
            child,
            fullOutput
          });
        }
      }
    }

    child.stdout.on('data', handleData);
    child.stderr.on('data', handleData);

    child.on('error', (err) => {
      if (timeoutHandle) clearTimeout(timeoutHandle);
      reject(new Error(`Failed to spawn cloudflared: ${err.message}`));
    });

    child.on('exit', (code, signal) => {
      if (timeoutHandle) clearTimeout(timeoutHandle);
      if (!detectedUrl) {
        reject(new Error(`cloudflared exited prematurely (code: ${code}, signal: ${signal}). Output:\n${fullOutput}`));
      }
    });

    timeoutHandle = setTimeout(() => {
      if (!detectedUrl) {
        try { child.kill(); } catch {}
        reject(new Error(`Timed out after ${timeoutMs}ms waiting for cloudflared tunnel URL. Output:\n${fullOutput}`));
      }
    }, timeoutMs);
  });
}

/**
 * Gracefully terminates a spawned child process.
 */
function terminateChild(child) {
  if (!child || child.killed) return;
  try {
    if (process.platform === 'win32' && child.pid) {
      try {
        execSync(`taskkill /pid ${child.pid} /t /f`, { stdio: 'ignore' });
      } catch {
        child.kill('SIGTERM');
      }
    } else {
      child.kill('SIGTERM');
    }
  } catch {
    // Ignore cleanup errors
  }
}

/**
 * Parses command line arguments.
 */
function parseCliArgs(argv = process.argv.slice(2)) {
  const options = {
    port: 5173,
    cloudPort: 9091,
    token: '',
    camera: '',
    lanOnly: false,
    dryRun: false,
    noServices: false,
    timeout: 30000,
    verbose: false,
    help: false
  };

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--port' || arg === '-p') {
      options.port = parseInt(argv[++i], 10) || 5173;
    } else if (arg === '--cloud-port') {
      options.cloudPort = parseInt(argv[++i], 10) || 9091;
    } else if (arg === '--token' || arg === '-t') {
      options.token = argv[++i] || '';
    } else if (arg === '--camera' || arg === '-c') {
      options.camera = argv[++i] || '';
    } else if (arg === '--lan-only' || arg === '--offline') {
      options.lanOnly = true;
    } else if (arg === '--dry-run') {
      options.dryRun = true;
    } else if (arg === '--no-services') {
      options.noServices = true;
    } else if (arg === '--timeout') {
      options.timeout = parseInt(argv[++i], 10) || 30000;
    } else if (arg === '--verbose' || arg === '-v') {
      options.verbose = true;
    } else if (arg === '--help' || arg === '-h') {
      options.help = true;
    }
  }

  return options;
}

/**
 * Prints CLI usage instructions.
 */
function printHelp() {
  console.log(`
Pulse Automated Ingress Tunnel Provisioner
==========================================
Usage: node scripts/provision_tunnel.cjs [options]

Options:
  -p, --port <num>       Frontend target port (default: 5173)
  --cloud-port <num>     Pulse Cloud backend port (default: 9091)
  -t, --token <jwt>      Append JWT authentication token to viewer URL
  -c, --camera <id>      Specify default camera ID in viewer URL
  --lan-only             Skip cloudflared and generate local LAN URLs only
  --dry-run              Simulate tunnel creation without spawning cloudflared
  --no-services          Skip verifying local service port availability
  --timeout <ms>         Tunnel discovery timeout in milliseconds (default: 30000)
  -v, --verbose          Print verbose cloudflared logging output
  -h, --help             Show this help message
`);
}

/**
 * Main execution routine.
 */
async function main(argv = process.argv.slice(2)) {
  const options = parseCliArgs(argv);

  if (options.help) {
    printHelp();
    if (require.main === module) process.exit(0);
    return;
  }

  console.log('[*] Initializing Pulse Ingress Tunnel Provisioner...');

  // Check local services
  let serviceStatus = null;
  if (!options.noServices) {
    serviceStatus = await checkLocalServices(options);
  }

  // Handle LAN-only mode
  if (options.lanOnly) {
    const lanViewerUrls = getLanViewerUrls(options.port, options);
    const viewerUrl = lanViewerUrls[0] || constructViewerUrl(`http://localhost:${options.port}`, options);

    renderBanner({
      tunnelUrl: null,
      viewerUrl,
      adminUrl: `http://localhost:${options.port}/`,
      lanViewerUrls,
      serviceStatus,
      isOfflineFallback: true
    });
    if (require.main === module) process.exit(0);
    return;
  }

  // Handle dry-run mode
  if (options.dryRun) {
    const mockDomain = 'https://pulse-preview-mock.trycloudflare.com';
    const viewerUrl = constructViewerUrl(mockDomain, options);
    const adminUrl = constructAdminUrl(mockDomain);
    const lanViewerUrls = getLanViewerUrls(options.port, options);

    renderBanner({
      tunnelUrl: mockDomain,
      viewerUrl,
      adminUrl,
      lanViewerUrls,
      serviceStatus,
      isOfflineFallback: false
    });
    if (require.main === module) process.exit(0);
    return;
  }

  let activeChild = null;

  function cleanup() {
    if (activeChild) {
      console.log('\n[*] Stopping cloudflared tunnel...');
      terminateChild(activeChild);
      activeChild = null;
      console.log('[✓] Tunnel terminated.');
    }
  }

  process.on('SIGINT', () => { cleanup(); process.exit(0); });
  process.on('SIGTERM', () => { cleanup(); process.exit(0); });
  process.on('exit', () => { cleanup(); });

  try {
    // 1. Ensure binary exists or download
    const binaryPath = await ensureCloudflared();

    // 2. Spawn tunnel and extract domain
    const result = await spawnTunnel(binaryPath, options);
    activeChild = result.child;

    // 3. Render banner
    renderBanner({
      tunnelUrl: result.url,
      viewerUrl: result.viewerUrl,
      adminUrl: result.adminUrl,
      lanViewerUrls: result.lanViewerUrls,
      serviceStatus,
      isOfflineFallback: false
    });

    // Keep process alive while child runs
    await new Promise((resolve) => {
      activeChild.on('exit', resolve);
    });
  } catch (err) {
    console.error(`\n[!] Error during tunnel provisioning: ${err.message}`);
    console.log('[*] Falling back to Local LAN URL...');

    const lanViewerUrls = getLanViewerUrls(options.port, options);
    const viewerUrl = lanViewerUrls[0] || constructViewerUrl(`http://localhost:${options.port}`, options);

    renderBanner({
      tunnelUrl: null,
      viewerUrl,
      adminUrl: `http://localhost:${options.port}/`,
      lanViewerUrls,
      serviceStatus,
      isOfflineFallback: true
    });
  }
}

// Module exports for programmatic testing and integration
module.exports = {
  stripAnsi,
  getBinaryName,
  getBinaryDownloadUrl,
  findCloudflaredBinary,
  downloadCloudflared,
  ensureCloudflared,
  parseTunnelUrl,
  constructViewerUrl,
  constructAdminUrl,
  getLanIpAddresses,
  getLanViewerUrls,
  checkPortListening,
  checkLocalServices,
  renderBanner,
  spawnTunnel,
  terminateChild,
  parseCliArgs,
  main
};

// Execute if run directly from CLI
if (require.main === module) {
  main().catch((err) => {
    console.error('Fatal error in provisioner:', err);
    process.exit(1);
  });
}
