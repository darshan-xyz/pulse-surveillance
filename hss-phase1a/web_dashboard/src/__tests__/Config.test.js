// @ts-nocheck
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getApiBaseUrl, getWsBaseUrl, getClientWsUrl } from '../lib/config';

describe('Dynamic Configuration & URL Resolvers', () => {
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

  describe('getApiBaseUrl', () => {
    it('returns default http://localhost:9091 in default jsdom environment', () => {
      const url = getApiBaseUrl();
      expect(url).toBe('http://localhost:9091');
    });

    it('dynamically adapts to remote hostname over HTTP', () => {
      Object.defineProperty(window, 'location', {
        value: {
          protocol: 'http:',
          hostname: '192.168.1.55',
          port: '5173',
          host: '192.168.1.55:5173',
        },
        writable: true,
        configurable: true,
      });

      const url = getApiBaseUrl();
      expect(url).toBe('http://192.168.1.55:9091');
    });

    it('dynamically uses https: protocol when loaded over HTTPS', () => {
      Object.defineProperty(window, 'location', {
        value: {
          protocol: 'https:',
          hostname: 'viewer.pulse-cloud.internal',
          port: '',
          host: 'viewer.pulse-cloud.internal',
        },
        writable: true,
        configurable: true,
      });

      const url = getApiBaseUrl();
      expect(url).toBe('https://viewer.pulse-cloud.internal:9091');
    });

    it('respects VITE_API_BASE_URL env override if provided', () => {
      vi.stubEnv('VITE_API_BASE_URL', 'https://api.custom-domain.com/v1');

      const url = getApiBaseUrl();
      expect(url).toBe('https://api.custom-domain.com/v1');
    });

    it('respects VITE_API_PORT env override if provided', () => {
      vi.stubEnv('VITE_API_PORT', '8080');

      const url = getApiBaseUrl();
      expect(url).toBe('http://localhost:8080');
    });
  });

  describe('getWsBaseUrl', () => {
    it('returns ws://localhost:9091 in default jsdom environment', () => {
      const url = getWsBaseUrl();
      expect(url).toBe('ws://localhost:9091');
    });

    it('dynamically switches to wss: when loaded over HTTPS', () => {
      Object.defineProperty(window, 'location', {
        value: {
          protocol: 'https:',
          hostname: 'remote-camera.pulse.io',
          port: '443',
          host: 'remote-camera.pulse.io',
        },
        writable: true,
        configurable: true,
      });

      const url = getWsBaseUrl();
      expect(url).toBe('wss://remote-camera.pulse.io:9091');
    });

    it('dynamically adapts to custom LAN IP hostname over ws:', () => {
      Object.defineProperty(window, 'location', {
        value: {
          protocol: 'http:',
          hostname: '10.0.0.120',
          port: '3000',
          host: '10.0.0.120:3000',
        },
        writable: true,
        configurable: true,
      });

      const url = getWsBaseUrl();
      expect(url).toBe('ws://10.0.0.120:9091');
    });

    it('respects VITE_WS_BASE_URL env override if provided', () => {
      vi.stubEnv('VITE_WS_BASE_URL', 'wss://stream.custom-domain.com');

      const url = getWsBaseUrl();
      expect(url).toBe('wss://stream.custom-domain.com');
    });
  });

  describe('getClientWsUrl', () => {
    it('constructs correct /client WebSocket URL with encoded token', () => {
      const token = 'jwt.header.payload+signature/with=special?chars&more';
      const wsUrl = getClientWsUrl(token);

      expect(wsUrl).toBe(
        `ws://localhost:9091/client?token=${encodeURIComponent(token)}`
      );
      expect(wsUrl).toContain('token=jwt.header.payload%2Bsignature%2Fwith%3Dspecial%3Fchars%26more');
    });

    it('constructs correct wss /client URL on remote HTTPS domain', () => {
      Object.defineProperty(window, 'location', {
        value: {
          protocol: 'https:',
          hostname: 'pulse.production.com',
          port: '',
          host: 'pulse.production.com',
        },
        writable: true,
        configurable: true,
      });

      const token = 'secure-session-token-123';
      const wsUrl = getClientWsUrl(token);

      expect(wsUrl).toBe('wss://pulse.production.com:9091/client?token=secure-session-token-123');
    });
  });
});
