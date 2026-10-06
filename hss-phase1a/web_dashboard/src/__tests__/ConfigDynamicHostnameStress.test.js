// @ts-nocheck
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getApiBaseUrl, getWsBaseUrl, getClientWsUrl } from '../lib/config';

describe('Empirical Challenge: Dynamic Hostnames & URL Resolution Stress Harness', () => {
  const originalLocation = window.location;

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  function setWindowLocation(protocol, hostname, port = '', pathname = '/') {
    Object.defineProperty(window, 'location', {
      value: {
        protocol,
        hostname,
        port,
        host: port ? `${hostname}:${port}` : hostname,
        pathname,
        origin: `${protocol}//${port ? `${hostname}:${port}` : hostname}`,
        search: '',
        hash: '',
      },
      writable: true,
      configurable: true,
    });
  }

  describe('1. Standard HTTP and Localhost Variants', () => {
    it('standard localhost on port 5173 resolves to port 9091', () => {
      setWindowLocation('http:', 'localhost', '5173');
      expect(getApiBaseUrl()).toBe('http://localhost:9091');
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });

    it('127.0.0.1 loopback IP resolves to 127.0.0.1:9091', () => {
      setWindowLocation('http:', '127.0.0.1', '5173');
      expect(getApiBaseUrl()).toBe('http://127.0.0.1:9091');
      expect(getWsBaseUrl()).toBe('ws://127.0.0.1:9091');
    });

    it('IPv6 loopback [::1] preserves bracketed hostname', () => {
      setWindowLocation('http:', '[::1]', '5173');
      expect(getApiBaseUrl()).toBe('http://[::1]:9091');
      expect(getWsBaseUrl()).toBe('ws://[::1]:9091');
    });

    it('standard localhost without port on frontend resolves to :9091', () => {
      setWindowLocation('http:', 'localhost', '');
      expect(getApiBaseUrl()).toBe('http://localhost:9091');
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });
  });

  describe('2. Remote Domains and HTTPS Protocol Switching', () => {
    it('HTTPS on remote domain viewer.pulse.domain.com resolves to wss:// and https://', () => {
      setWindowLocation('https:', 'viewer.pulse.domain.com', '443');
      expect(getApiBaseUrl()).toBe('https://viewer.pulse.domain.com:9091');
      expect(getWsBaseUrl()).toBe('wss://viewer.pulse.domain.com:9091');
    });

    it('HTTPS on multi-level subdomain with non-standard dev port', () => {
      setWindowLocation('https:', 'node-01.stage.edge.pulse.internal', '8443');
      expect(getApiBaseUrl()).toBe('https://node-01.stage.edge.pulse.internal:9091');
      expect(getWsBaseUrl()).toBe('wss://node-01.stage.edge.pulse.internal:9091');
    });

    it('URL containing deep path, search params, and hash isolates only hostname and protocol', () => {
      setWindowLocation('https:', 'cloud.pulse-defense.org', '443', '/dashboard/viewer/live?cam=front#fullscreen');
      expect(getApiBaseUrl()).toBe('https://cloud.pulse-defense.org:9091');
      expect(getWsBaseUrl()).toBe('wss://cloud.pulse-defense.org:9091');
      expect(getClientWsUrl('tok123')).toBe('wss://cloud.pulse-defense.org:9091/client?token=tok123');
    });
  });

  describe('3. LAN and Private IP Addresses', () => {
    const lanIPs = [
      '192.168.1.100',
      '192.168.0.1',
      '10.0.0.50',
      '10.200.45.12',
      '172.16.0.5',
      '172.31.255.254'
    ];

    lanIPs.forEach((ip) => {
      it(`resolves correctly for LAN IP ${ip}`, () => {
        setWindowLocation('http:', ip, '3000');
        expect(getApiBaseUrl()).toBe(`http://${ip}:9091`);
        expect(getWsBaseUrl()).toBe(`ws://${ip}:9091`);
        expect(getClientWsUrl('tok-lan')).toBe(`ws://${ip}:9091/client?token=tok-lan`);
      });
    });
  });

  describe('4. Port Overrides via VITE_API_PORT', () => {
    it('empty string VITE_API_PORT produces clean URLs without port or trailing colon for reverse proxy', () => {
      vi.stubEnv('VITE_API_PORT', '');
      setWindowLocation('https:', 'viewer.pulse.domain.com');

      expect(getApiBaseUrl()).toBe('https://viewer.pulse.domain.com');
      expect(getWsBaseUrl()).toBe('wss://viewer.pulse.domain.com');
      expect(getClientWsUrl('my-token')).toBe('wss://viewer.pulse.domain.com/client?token=my-token');
      // Verify no colon at end of host
      expect(getApiBaseUrl().split('//')[1]).not.toContain(':');
      expect(getWsBaseUrl().split('//')[1]).not.toContain(':');
    });

    it('custom port 8080 replaces default 9091', () => {
      vi.stubEnv('VITE_API_PORT', '8080');
      setWindowLocation('http:', '192.168.1.200');

      expect(getApiBaseUrl()).toBe('http://192.168.1.200:8080');
      expect(getWsBaseUrl()).toBe('ws://192.168.1.200:8080');
      expect(getClientWsUrl('abc')).toBe('ws://192.168.1.200:8080/client?token=abc');
    });

    it('custom port 443 over HTTPS', () => {
      vi.stubEnv('VITE_API_PORT', '443');
      setWindowLocation('https:', 'secure-gateway.pulse.org');

      expect(getApiBaseUrl()).toBe('https://secure-gateway.pulse.org:443');
      expect(getWsBaseUrl()).toBe('wss://secure-gateway.pulse.org:443');
    });
  });

  describe('5. Base URL Overrides via VITE_API_BASE_URL and VITE_WS_BASE_URL', () => {
    it('VITE_API_BASE_URL explicitly overrides getApiBaseUrl completely', () => {
      vi.stubEnv('VITE_API_BASE_URL', 'https://proxy.custom-edge.net/api/v2');
      setWindowLocation('http:', 'localhost', '5173');

      expect(getApiBaseUrl()).toBe('https://proxy.custom-edge.net/api/v2');
      // WS should still follow dynamic resolution when VITE_WS_BASE_URL is not set
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });

    it('VITE_WS_BASE_URL explicitly overrides getWsBaseUrl and getClientWsUrl completely', () => {
      vi.stubEnv('VITE_WS_BASE_URL', 'wss://signaling-mesh.pulse.io:9999/webrtc');
      setWindowLocation('http:', 'localhost', '5173');

      expect(getWsBaseUrl()).toBe('wss://signaling-mesh.pulse.io:9999/webrtc');
      expect(getClientWsUrl('mesh-auth-token')).toBe(
        'wss://signaling-mesh.pulse.io:9999/webrtc/client?token=mesh-auth-token'
      );
      // API should still follow dynamic resolution
      expect(getApiBaseUrl()).toBe('http://localhost:9091');
    });

    it('both VITE_API_BASE_URL and VITE_WS_BASE_URL can be overridden simultaneously', () => {
      vi.stubEnv('VITE_API_BASE_URL', 'https://api.staging.pulse.com');
      vi.stubEnv('VITE_WS_BASE_URL', 'wss://ws.staging.pulse.com');

      expect(getApiBaseUrl()).toBe('https://api.staging.pulse.com');
      expect(getWsBaseUrl()).toBe('wss://ws.staging.pulse.com');
      expect(getClientWsUrl('staging-key')).toBe('wss://ws.staging.pulse.com/client?token=staging-key');
    });
  });

  describe('6. Token Query Parameter Encoding & URL Parsing Oracle', () => {
    const testTokens = [
      { name: 'Standard JWT', token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.doNotLeak' },
      { name: 'Plus and Slash Signs', token: 'token+with+plus/and/slashes==' },
      { name: 'Query Separators (&, ?, =, #)', token: 'tok&param=value?foo=bar#fragment' },
      { name: 'Spaces and Line Breaks', token: 'token with spaces \n and \t tabs' },
      { name: 'Unicode and Emojis', token: 'token-🔑-cam-01-📹-日本語-üöä' },
      { name: 'Special Punctuation', token: 'jwt~!@#$%^&*()_+{}|:"<>?' },
      { name: 'Empty String Token', token: '' },
      { name: 'Long 2048-char Token', token: 'A'.repeat(2048) },
    ];

    testTokens.forEach(({ name, token }) => {
      it(`oracle test: correctly encodes and decodes token [${name}]`, () => {
        setWindowLocation('https:', 'viewer.pulse.domain.com', '9091');
        const wsUrl = getClientWsUrl(token);

        // Verify prefix
        expect(wsUrl.startsWith('wss://viewer.pulse.domain.com:9091/client?token=')).toBe(true);

        // Verify URL parseability using standard URL parser (using http protocol for URL parser compatibility)
        const httpEquivalent = wsUrl.replace('wss://', 'https://');
        const parsed = new URL(httpEquivalent);

        expect(parsed.pathname).toBe('/client');
        expect(parsed.searchParams.get('token')).toBe(token);
      });
    });
  });

  describe('7. Edge Cases: Empty Hostname, file: Protocol Fallback', () => {
    it('empty hostname string falls back to localhost', () => {
      setWindowLocation('http:', '');
      expect(getApiBaseUrl()).toBe('http://localhost:9091');
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });

    it('file:/// protocol defaults to http:// and ws://', () => {
      setWindowLocation('file:', '');
      expect(getApiBaseUrl()).toBe('http://localhost:9091');
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });
  });
});
