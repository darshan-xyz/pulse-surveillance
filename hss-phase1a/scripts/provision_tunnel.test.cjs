/**
 * Pulse Ingress Tunnel Provisioner Unit & Integration Test Suite
 */

const { test, describe } = require('node:test');
const assert = require('node:assert/strict');
const http = require('http');
const net = require('net');
const { execSync } = require('child_process');
const path = require('path');

const {
  stripAnsi,
  getBinaryName,
  getBinaryDownloadUrl,
  parseTunnelUrl,
  constructViewerUrl,
  constructAdminUrl,
  getLanIpAddresses,
  getLanViewerUrls,
  checkPortListening,
  checkLocalServices,
  renderBanner,
  parseCliArgs
} = require('./provision_tunnel.cjs');

describe('Pulse Tunnel Provisioner - Unit Tests', () => {

  describe('stripAnsi', () => {
    test('removes color and style escape codes', () => {
      const colored = '\x1B[32mhttps://pulse-preview-123.trycloudflare.com\x1B[0m';
      assert.equal(stripAnsi(colored), 'https://pulse-preview-123.trycloudflare.com');
    });

    test('handles empty or non-string inputs safely', () => {
      assert.equal(stripAnsi(''), '');
      assert.equal(stripAnsi(null), '');
      assert.equal(stripAnsi(undefined), '');
    });
  });

  describe('getBinaryName', () => {
    test('returns cloudflared.exe on Windows', () => {
      assert.equal(getBinaryName('win32'), 'cloudflared.exe');
    });

    test('returns cloudflared on Linux and Darwin', () => {
      assert.equal(getBinaryName('linux'), 'cloudflared');
      assert.equal(getBinaryName('darwin'), 'cloudflared');
    });
  });

  describe('getBinaryDownloadUrl', () => {
    test('returns correct download URL for Windows x64', () => {
      const url = getBinaryDownloadUrl('win32', 'x64');
      assert.equal(url, 'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe');
    });

    test('returns correct download URL for Windows x86 (ia32)', () => {
      const url = getBinaryDownloadUrl('win32', 'ia32');
      assert.equal(url, 'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-386.exe');
    });

    test('returns correct download URL for macOS arm64 (Apple Silicon)', () => {
      const url = getBinaryDownloadUrl('darwin', 'arm64');
      assert.equal(url, 'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-arm64');
    });

    test('returns correct download URL for Linux x64 and arm64', () => {
      assert.equal(
        getBinaryDownloadUrl('linux', 'x64'),
        'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64'
      );
      assert.equal(
        getBinaryDownloadUrl('linux', 'arm64'),
        'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64'
      );
    });

    test('throws for unsupported architectures', () => {
      assert.throws(() => getBinaryDownloadUrl('freebsd', 'mips'), /Unsupported platform/);
    });
  });

  describe('parseTunnelUrl', () => {
    test('extracts domain from Cloudflare ASCII banner box', () => {
      const banner = `
2026-08-31T04:30:00Z INF +--------------------------------------------------------------------------------------------+
2026-08-31T04:30:00Z INF |  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |
2026-08-31T04:30:00Z INF |  https://richard-tampa-chile-subscribers.trycloudflare.com                                 |
2026-08-31T04:30:00Z INF +--------------------------------------------------------------------------------------------+
`;
      assert.equal(parseTunnelUrl(banner), 'https://richard-tampa-chile-subscribers.trycloudflare.com');
    });

    test('extracts domain from registered connection log line', () => {
      const log = '2026-08-31T05:00:12Z INF Registered tunnel connection connIndex=0 location=ORD url=https://quick-pulse-stream-99.trycloudflare.com';
      assert.equal(parseTunnelUrl(log), 'https://quick-pulse-stream-99.trycloudflare.com');
    });

    test('extracts domain when wrapped with ANSI codes', () => {
      const coloredLog = '\x1B[34m[INF]\x1B[0m \x1B[1mhttps://fast-secure-tunnel-12.trycloudflare.com\x1B[0m ready';
      assert.equal(parseTunnelUrl(coloredLog), 'https://fast-secure-tunnel-12.trycloudflare.com');
    });

    test('supports generic tunnel fallbacks (ngrok / loca.lt)', () => {
      assert.equal(parseTunnelUrl('Tunnel online: https://abc-123.ngrok-free.app/'), 'https://abc-123.ngrok-free.app');
      assert.equal(parseTunnelUrl('Forwarding: https://pulse-demo.loca.lt/'), 'https://pulse-demo.loca.lt');
    });

    test('returns null when no tunnel URL is present in text', () => {
      assert.equal(parseTunnelUrl('Starting cloudflared version 2026.8.0...'), null);
      assert.equal(parseTunnelUrl(''), null);
      assert.equal(parseTunnelUrl(null), null);
    });
  });

  describe('constructViewerUrl', () => {
    test('builds standard one-click viewer URL with viewer=true and autostart=true', () => {
      const url = constructViewerUrl('https://my-stream.trycloudflare.com');
      assert.equal(url, 'https://my-stream.trycloudflare.com/?viewer=true&autostart=true');
    });

    test('prepends https protocol if missing', () => {
      const url = constructViewerUrl('my-stream.trycloudflare.com');
      assert.equal(url, 'https://my-stream.trycloudflare.com/?viewer=true&autostart=true');
    });

    test('appends token query parameter when supplied', () => {
      const url = constructViewerUrl('https://my-stream.trycloudflare.com', { token: 'mock-jwt-token-123' });
      assert.equal(url, 'https://my-stream.trycloudflare.com/?viewer=true&autostart=true&token=mock-jwt-token-123');
    });

    test('appends camera query parameter when supplied', () => {
      const url = constructViewerUrl('https://my-stream.trycloudflare.com', { camera: 'cam-garden-01' });
      assert.equal(url, 'https://my-stream.trycloudflare.com/?viewer=true&autostart=true&camera=cam-garden-01');
    });

    test('appends both token and camera query parameters', () => {
      const url = constructViewerUrl('https://my-stream.trycloudflare.com', {
        token: 'sample-jwt',
        camera: 'cam-front'
      });
      assert.equal(url, 'https://my-stream.trycloudflare.com/?viewer=true&autostart=true&token=sample-jwt&camera=cam-front');
    });

    test('handles LAN URL origins cleanly', () => {
      const url = constructViewerUrl('http://192.168.1.150:5173');
      assert.equal(url, 'http://192.168.1.150:5173/?viewer=true&autostart=true');
    });

    test('returns empty string if baseUrl is empty or null', () => {
      assert.equal(constructViewerUrl(''), '');
      assert.equal(constructViewerUrl(null), '');
    });
  });

  describe('constructAdminUrl', () => {
    test('returns root origin URL with trailing slash', () => {
      assert.equal(constructAdminUrl('https://my-stream.trycloudflare.com'), 'https://my-stream.trycloudflare.com/');
      assert.equal(constructAdminUrl('https://my-stream.trycloudflare.com/deep/path?q=1'), 'https://my-stream.trycloudflare.com/');
    });
  });

  describe('getLanIpAddresses & getLanViewerUrls', () => {
    test('returns an array of valid IPv4 addresses', () => {
      const ips = getLanIpAddresses();
      assert.ok(Array.isArray(ips));
      for (const ip of ips) {
        assert.match(ip, /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/);
        assert.ok(!ip.startsWith('127.'));
        assert.ok(!ip.startsWith('169.254.'));
      }
    });

    test('constructs LAN viewer URLs with configured port', () => {
      const lanUrls = getLanViewerUrls(5173, { token: 't1' });
      assert.ok(lanUrls.length > 0);
      for (const url of lanUrls) {
        assert.ok(url.includes(':5173'));
        assert.ok(url.includes('viewer=true'));
        assert.ok(url.includes('autostart=true'));
        assert.ok(url.includes('token=t1'));
      }
    });
  });

  describe('checkPortListening & checkLocalServices', () => {
    test('detects active TCP port listener', async () => {
      const server = net.createServer((socket) => {
        socket.on('error', () => {});
      });
      await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
      const port = server.address().port;

      const isListening = await checkPortListening(port, '127.0.0.1', 1000);
      assert.equal(isListening, true);

      if (server.closeAllConnections) server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    });

    test('detects inactive TCP port', async () => {
      const isListening = await checkPortListening(59999, '127.0.0.1', 300);
      assert.equal(isListening, false);
    });

    test('checks both frontend and backend services', async () => {
      const server = net.createServer((socket) => {
        socket.on('error', () => {});
      });
      await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
      const activePort = server.address().port;

      const status = await checkLocalServices({ port: activePort, cloudPort: 59998 });
      assert.equal(status.frontendUp, true);
      assert.equal(status.backendUp, false);
      assert.equal(status.frontendPort, activePort);
      assert.equal(status.backendPort, 59998);

      if (server.closeAllConnections) server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    });
  });

  describe('parseCliArgs', () => {
    test('parses defaults when no arguments provided', () => {
      const opts = parseCliArgs([]);
      assert.equal(opts.port, 5173);
      assert.equal(opts.cloudPort, 9091);
      assert.equal(opts.token, '');
      assert.equal(opts.camera, '');
      assert.equal(opts.lanOnly, false);
      assert.equal(opts.dryRun, false);
    });

    test('parses custom CLI options and flags', () => {
      const opts = parseCliArgs([
        '--port', '8080',
        '--cloud-port', '9095',
        '-t', 'custom-jwt-token',
        '-c', 'camera-2',
        '--dry-run',
        '--lan-only',
        '--verbose'
      ]);
      assert.equal(opts.port, 8080);
      assert.equal(opts.cloudPort, 9095);
      assert.equal(opts.token, 'custom-jwt-token');
      assert.equal(opts.camera, 'camera-2');
      assert.equal(opts.dryRun, true);
      assert.equal(opts.lanOnly, true);
      assert.equal(opts.verbose, true);
    });
  });

  describe('renderBanner', () => {
    test('renders banner output without crashing', () => {
      const logs = [];
      const mockLogger = { log: (msg) => logs.push(msg) };

      renderBanner({
        tunnelUrl: 'https://preview-123.trycloudflare.com',
        viewerUrl: 'https://preview-123.trycloudflare.com/?viewer=true&autostart=true',
        adminUrl: 'https://preview-123.trycloudflare.com/',
        lanViewerUrls: ['http://192.168.1.50:5173/?viewer=true&autostart=true'],
        serviceStatus: { frontendUp: true, backendUp: true, frontendPort: 5173, backendPort: 9091 },
        isOfflineFallback: false
      }, mockLogger);

      const combined = logs.join('\n');
      assert.ok(combined.includes('PULSE STREAMING'));
      assert.ok(combined.includes('https://preview-123.trycloudflare.com/?viewer=true&autostart=true'));
      assert.ok(combined.includes('ACTIVE'));
      assert.ok(combined.includes('http://192.168.1.50:5173'));
    });
  });

  describe('CLI Integration & Execution', () => {
    test('executes --help cleanly with exit code 0', () => {
      const scriptPath = path.resolve(__dirname, 'provision_tunnel.cjs');
      const output = require('child_process').execFileSync(process.execPath, [scriptPath, '--help'], { encoding: 'utf-8', windowsHide: true });
      assert.ok(output.includes('Pulse Automated Ingress Tunnel Provisioner'));
      assert.ok(output.includes('--port'));
    });

    test('executes --dry-run cleanly and displays public viewer link', () => {
      const scriptPath = path.resolve(__dirname, 'provision_tunnel.cjs');
      const output = require('child_process').execFileSync(process.execPath, [scriptPath, '--dry-run', '--no-services'], { encoding: 'utf-8', windowsHide: true });
      assert.ok(output.includes('https://pulse-preview-mock.trycloudflare.com/?viewer=true&autostart=true'));
    });

    test('executes --lan-only cleanly and displays local LAN fallback link', () => {
      const scriptPath = path.resolve(__dirname, 'provision_tunnel.cjs');
      const output = require('child_process').execFileSync(process.execPath, [scriptPath, '--lan-only', '--no-services'], { encoding: 'utf-8', windowsHide: true });
      assert.ok(output.includes('OFFLINE / LAN FALLBACK MODE'));
      assert.ok(output.includes('?viewer=true&autostart=true'));
    });
  });
});
