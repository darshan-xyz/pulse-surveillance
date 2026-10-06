/**
 * Pulse Web Dashboard Dynamic Configuration
 * Provides dynamic URL resolution for HTTP API and WebSocket signaling endpoints
 * based on current browser window location (protocol, hostname, port) with
 * environment variable override support for development and reverse proxy tunnels.
 */

/**
 * Returns the base URL for HTTP REST API requests.
 * Evaluates window.location to determine protocol (http/https), hostname, and port.
 */
export function getApiBaseUrl(): string {
  if (typeof window === 'undefined') {
    return 'http://localhost:9091';
  }

  // Allow explicit override via Vite env variable
  if (import.meta.env && import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL;
  }

  const isHttps = window.location.protocol === 'https:';
  const protocol = isHttps ? 'https:' : 'http:';
  const hostname = window.location.hostname || 'localhost';

  // If custom API port is set via env, use it
  if (import.meta.env && import.meta.env.VITE_API_PORT !== undefined) {
    const port = import.meta.env.VITE_API_PORT;
    return port ? `${protocol}//${hostname}:${port}` : `${protocol}//${hostname}`;
  }

  // Detect public ingress tunnels (e.g. Cloudflare Quick Tunnels, ngrok, localtunnel)
  // Public tunnels expose standard HTTPS (443) and multiplex API/WS routes via reverse proxy
  const isTunnel = hostname.includes('trycloudflare.com') ||
    hostname.includes('ngrok') ||
    hostname.includes('localtunnel') ||
    hostname.includes('loca.lt');

  if (isTunnel) {
    const port = window.location.port;
    return port && port !== '80' && port !== '443'
      ? `${protocol}//${hostname}:${port}`
      : `${protocol}//${hostname}`;
  }

  // Default backend port is 9091 across environments
  return `${protocol}//${hostname}:9091`;
}

/**
 * Returns the base URL for WebSocket signaling connections.
 * Automatically chooses wss:// for HTTPS origins and ws:// for HTTP origins.
 */
export function getWsBaseUrl(): string {
  if (typeof window === 'undefined') {
    return 'ws://localhost:9091';
  }

  // Allow explicit override via Vite env variable
  if (import.meta.env && import.meta.env.VITE_WS_BASE_URL) {
    return import.meta.env.VITE_WS_BASE_URL;
  }

  const isHttps = window.location.protocol === 'https:';
  const protocol = isHttps ? 'wss:' : 'ws:';
  const hostname = window.location.hostname || 'localhost';

  // If custom API/WS port is set via env, use it
  if (import.meta.env && import.meta.env.VITE_API_PORT !== undefined) {
    const port = import.meta.env.VITE_API_PORT;
    return port ? `${protocol}//${hostname}:${port}` : `${protocol}//${hostname}`;
  }

  // Detect public ingress tunnels (e.g. Cloudflare Quick Tunnels, ngrok, localtunnel)
  const isTunnel = hostname.includes('trycloudflare.com') ||
    hostname.includes('ngrok') ||
    hostname.includes('localtunnel') ||
    hostname.includes('loca.lt');

  if (isTunnel) {
    const port = window.location.port;
    return port && port !== '80' && port !== '443'
      ? `${protocol}//${hostname}:${port}`
      : `${protocol}//${hostname}`;
  }

  // Default backend port is 9091 across environments
  return `${protocol}//${hostname}:9091`;
}

/**
 * Returns the fully qualified WebSocket endpoint URL for viewer client connections.
 * @param token JWT authentication token for the viewer session
 */
export function getClientWsUrl(token: string): string {
  const wsBase = getWsBaseUrl();
  return `${wsBase}/client?token=${encodeURIComponent(token)}`;
}
