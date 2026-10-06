/**
 * Pulse 1-Click All-in-One Orchestrator
 * 
 * Boots all required components in a single execution:
 *  1. AI Inference Service (FastAPI + YOLO11 Fire & Smoke model best.pt)
 *  2. Pulse Cloud Backend (Go signaling hub + auth + API on :9091)
 *  3. Home Surveillance Server / HSS Daemon (Go camera ingest + AI detector)
 *  4. Web Dashboard (Vite dev server on :5173 with proxy)
 *  5. Cloudflare Ingress Tunnel (Generates instant public HTTPS/WSS URL)
 *  6. Automatically opens the browser to the local or remote viewer URL
 */

const fs = require('fs');
const path = require('path');
const os = require('os');
const net = require('net');
const { spawn, execSync } = require('child_process');

const ANSI = {
  reset: '\x1b[0m',
  bold: '\x1b[1m',
  cyan: '\x1b[36m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  magenta: '\x1b[35m',
  red: '\x1b[31m',
  gray: '\x1b[90m',
  white: '\x1b[97m',
  bgRed: '\x1b[41m',
};

const HSS_DIR = path.resolve(__dirname, '..');
const ROOT_DIR = path.resolve(HSS_DIR, '..');
const AI_DIR = path.resolve(HSS_DIR, 'ai_service');
const DASHBOARD_DIR = path.resolve(HSS_DIR, 'web_dashboard');

const spawnedChildren = [];

// Clean process exit handler
function registerExitHandlers() {
  const cleanup = () => {
    console.log(`\n${ANSI.yellow}[Pulse 1-Click] Terminating all background services cleanly...${ANSI.reset}`);
    for (const child of spawnedChildren) {
      try {
        if (!child.killed) {
          if (process.platform === 'win32' && child.pid) {
            execSync(`taskkill /pid ${child.pid} /T /F`, { stdio: 'ignore' });
          } else {
            child.kill('SIGTERM');
          }
        }
      } catch (e) {
        // ignore
      }
    }
    process.exit(0);
  };

  process.on('SIGINT', cleanup);
  process.on('SIGTERM', cleanup);
  process.on('exit', () => {
    for (const child of spawnedChildren) {
      try {
        if (!child.killed) child.kill('SIGTERM');
      } catch (e) {}
    }
  });
}

function checkPort(port, host = '127.0.0.1', timeout = 500) {
  return new Promise((resolve) => {
    const socket = new net.Socket();
    socket.setTimeout(timeout);
    socket.once('connect', () => {
      socket.destroy();
      resolve(true);
    });
    socket.once('timeout', () => {
      socket.destroy();
      resolve(false);
    });
    socket.once('error', () => {
      socket.destroy();
      resolve(false);
    });
    try {
      socket.connect(port, host);
    } catch {
      resolve(false);
    }
  });
}

async function waitForPort(port, host = '127.0.0.1', maxRetries = 40, delayMs = 500) {
  for (let i = 0; i < maxRetries; i++) {
    const isListening = await checkPort(port, host, 400);
    if (isListening) return true;
    await new Promise((r) => setTimeout(r, delayMs));
  }
  return false;
}

function getPythonExecutable() {
  const candidates = [
    'python',
    'python3',
    'C:\\Users\\KV\\AppData\\Local\\Programs\\Python\\Python311\\python.exe',
  ];
  for (const cmd of candidates) {
    try {
      execSync(`"${cmd}" -c "import sys; sys.exit(0)"`, { stdio: 'ignore' });
      return cmd;
    } catch {
      continue;
    }
  }
  return 'python';
}

function spawnProcess(name, command, args, cwd, color = ANSI.cyan) {
  console.log(`${color}[${name}] Spawning: ${command} ${args.join(' ')}${ANSI.reset}`);
  
  const isWin = process.platform === 'win32';
  const targetCmd = isWin && /[\s()]/.test(command) ? `"${command}"` : command;
  const child = spawn(targetCmd, args, {
    cwd,
    shell: isWin,
    windowsHide: true,
    env: { ...process.env, PYTHONUNBUFFERED: '1', FORCE_COLOR: '1' },
    stdio: ['ignore', 'pipe', 'pipe']
  });

  child.stdout.on('data', (data) => {
    const lines = data.toString().split('\n');
    for (const line of lines) {
      if (line.trim()) {
        console.log(`${color}[${name}]${ANSI.reset} ${line.trimEnd()}`);
      }
    }
  });

  child.stderr.on('data', (data) => {
    const lines = data.toString().split('\n');
    for (const line of lines) {
      if (line.trim()) {
        console.log(`${ANSI.gray}[${name}:stderr]${ANSI.reset} ${line.trimEnd()}`);
      }
    }
  });

  child.on('error', (err) => {
    console.error(`${ANSI.red}[${name}] Error: ${err.message}${ANSI.reset}`);
  });

  spawnedChildren.push(child);
  return child;
}

async function main() {
  registerExitHandlers();

  console.clear();
  console.log(`${ANSI.bold}${ANSI.cyan}================================================================${ANSI.reset}`);
  console.log(`${ANSI.bold}${ANSI.cyan}       PULSE SURVEILLANCE & AI FIRE DETECTION — 1-CLICK BOOT    ${ANSI.reset}`);
  console.log(`${ANSI.bold}${ANSI.cyan}================================================================${ANSI.reset}\n`);

  // 1. Start AI Inference Service
  const pyCmd = getPythonExecutable();
  const aiPort = 8000;
  const isAiRunning = await checkPort(aiPort);
  if (!isAiRunning) {
    console.log(`${ANSI.yellow}[1/5] Starting YOLO11 Fire & Smoke Inference Service (:8000)...${ANSI.reset}`);
    spawnProcess('Pulse-AI', pyCmd, ['main.py', '--host', '127.0.0.1', '--port', `${aiPort}`], AI_DIR, ANSI.magenta);
    const aiReady = await waitForPort(aiPort, '127.0.0.1', 90, 500);
    if (aiReady) {
      console.log(`${ANSI.green}  ✓ AI Inference Engine is ONLINE (:8000)${ANSI.reset}`);
    } else {
      console.log(`${ANSI.yellow}  ! AI Inference Service deferred/fallback mode active${ANSI.reset}`);
    }
  } else {
    console.log(`${ANSI.green}[1/5] AI Inference Service is already active on :${aiPort}${ANSI.reset}`);
  }

  // 2. Start Pulse Cloud Backend
  const cloudPort = 9091;
  const isCloudRunning = await checkPort(cloudPort);
  if (!isCloudRunning) {
    console.log(`\n${ANSI.yellow}[2/5] Starting Pulse Cloud Backend & Signaling Hub (:9091)...${ANSI.reset}`);
    const cloudBin = path.resolve(HSS_DIR, 'bin', process.platform === 'win32' ? 'pulse-cloud.exe' : 'pulse-cloud');
    if (fs.existsSync(cloudBin)) {
      spawnProcess('Cloud-Hub', cloudBin, [], HSS_DIR, ANSI.cyan);
    } else {
      spawnProcess('Cloud-Hub', 'go', ['run', '-mod=readonly', './cmd/pulse-cloud/main.go'], HSS_DIR, ANSI.cyan);
    }
    const cloudReady = await waitForPort(cloudPort, '127.0.0.1', 30, 500);
    if (cloudReady) {
      console.log(`${ANSI.green}  ✓ Pulse Cloud Signaling & Auth Hub is ONLINE (:9091)${ANSI.reset}`);
    }
  } else {
    console.log(`\n${ANSI.green}[2/5] Pulse Cloud Backend is already active on :${cloudPort}${ANSI.reset}`);
  }

  // 3. Start HSS Edge Daemon
  const hssPort = 8080;
  const isHssRunning = await checkPort(hssPort);
  if (!isHssRunning) {
    console.log(`\n${ANSI.yellow}[3/5] Starting HSS Edge Camera Supervisor & AI Detector (:8080)...${ANSI.reset}`);
    const hssBin = path.resolve(HSS_DIR, 'bin', process.platform === 'win32' ? 'hss.exe' : 'hss');
    if (fs.existsSync(hssBin)) {
      spawnProcess('HSS-Edge', hssBin, [], HSS_DIR, ANSI.yellow);
    } else {
      spawnProcess('HSS-Edge', 'go', ['run', '-mod=readonly', './cmd/hss/main.go'], HSS_DIR, ANSI.yellow);
    }
    await waitForPort(hssPort, '127.0.0.1', 20, 500);
    console.log(`${ANSI.green}  ✓ Edge Camera Supervisor & Fire Detection is ONLINE${ANSI.reset}`);
  } else {
    console.log(`\n${ANSI.green}[3/5] HSS Edge Daemon is already active on :${hssPort}${ANSI.reset}`);
  }

  // 4. Start Web Dashboard
  const vitePort = 5173;
  const isViteRunning = await checkPort(vitePort);
  if (!isViteRunning) {
    console.log(`\n${ANSI.yellow}[4/5] Starting Web Dashboard Dev Server (:5173)...${ANSI.reset}`);
    spawnProcess('Dashboard', 'npm', ['run', 'dev', '--', '--host', '--port', `${vitePort}`], DASHBOARD_DIR, ANSI.green);
    const viteReady = await waitForPort(vitePort, '127.0.0.1', 30, 500);
    if (viteReady) {
      console.log(`${ANSI.green}  ✓ Svelte Web Dashboard is ONLINE (:5173)${ANSI.reset}`);
    }
  } else {
    console.log(`\n${ANSI.green}[4/5] Web Dashboard is already active on :${vitePort}${ANSI.reset}`);
  }

  // 5. Start Cloudflare Tunnel Provisioner
  console.log(`\n${ANSI.yellow}[5/5] Launching Cloudflare Quick Tunnel Provisioner...${ANSI.reset}`);
  const tunnelScript = path.resolve(HSS_DIR, 'scripts', 'provision_tunnel.cjs');
  const tunnelChild = spawn('node', [tunnelScript, '--port', `${vitePort}`, '--cloud-port', `${cloudPort}`], {
    cwd: HSS_DIR,
    stdio: 'inherit'
  });
  spawnedChildren.push(tunnelChild);

  // Automatically open browser after 3.5 seconds
  setTimeout(() => {
    const localUrl = `http://localhost:${vitePort}/`;
    console.log(`\n${ANSI.bold}${ANSI.green}[Browser] Opening local dashboard: ${localUrl}${ANSI.reset}`);
    try {
      if (process.platform === 'win32') {
        spawn('cmd', ['/c', 'start', localUrl], { detached: true, stdio: 'ignore' });
      } else if (process.platform === 'darwin') {
        spawn('open', [localUrl], { detached: true, stdio: 'ignore' });
      } else {
        spawn('xdg-open', [localUrl], { detached: true, stdio: 'ignore' });
      }
    } catch {
      // ignore
    }
  }, 3500);
}

main().catch((err) => {
  console.error(`${ANSI.red}Fatal 1-click error: ${err.message}${ANSI.reset}`);
  process.exit(1);
});
