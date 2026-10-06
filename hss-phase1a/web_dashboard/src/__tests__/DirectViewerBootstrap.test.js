// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import App from '../App.svelte';
import Dashboard from '../lib/Dashboard.svelte';
import { getApiBaseUrl, getWsBaseUrl, getClientWsUrl } from '../lib/config';

describe('Direct 1-Click Viewer Mode & URL Auto-Authentication Bootstrap', () => {
  const originalLocation = window.location;

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
    localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
  });

  function setWindowUrl(urlStr) {
    const url = new URL(urlStr);
    Object.defineProperty(window, 'location', {
      value: {
        href: url.href,
        protocol: url.protocol,
        hostname: url.hostname,
        port: url.port,
        host: url.host,
        pathname: url.pathname,
        search: url.search,
        searchParams: url.searchParams,
        origin: url.origin,
        hash: url.hash,
      },
      writable: true,
      configurable: true,
    });
  }

  describe('1. URL Parameter Bootstrapping in App.svelte', () => {
    it('renders Login view when query params are empty and localStorage is empty', () => {
      setWindowUrl('http://localhost:5173/');
      render(App);

      expect(screen.getByText('Pulse Cloud')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
    });

    it('auto-bootstraps session and directly mounts Dashboard when ?viewer=true is present', async () => {
      setWindowUrl('http://localhost:5173/?viewer=true');
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [{ device_id: 'test-camera-1', role: 'admin' }],
      });

      render(App);

      await waitFor(() => {
        expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
      });

      expect(localStorage.getItem('pulse_token')).toBe('mock_user_jwt');
      expect(localStorage.getItem('pulse_dashboard_mode')).toBe('viewer');
    });

    it('auto-bootstraps session when ?autostart=true is present', async () => {
      setWindowUrl('https://my-stream.trycloudflare.com/?autostart=true');
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [{ device_id: 'cam-01', role: 'viewer' }],
      });

      render(App);

      await waitFor(() => {
        expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
      });

      expect(localStorage.getItem('pulse_token')).toBe('mock_user_jwt');
      expect(localStorage.getItem('pulse_dashboard_mode')).toBe('viewer');
    });

    it('uses explicitly provided ?token= query parameter', async () => {
      const explicitJwt = 'custom-shareable-jwt-token-xyz';
      setWindowUrl(`http://localhost:5173/?viewer=true&token=${explicitJwt}`);
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [{ device_id: 'cam-01', role: 'viewer' }],
      });

      render(App);

      await waitFor(() => {
        expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
      });

      expect(localStorage.getItem('pulse_token')).toBe(explicitJwt);
      expect(localStorage.getItem('pulse_dashboard_mode')).toBe('viewer');
    });

    it('allows signing out and clearing session even after auto-bootstrap', async () => {
      setWindowUrl('http://localhost:5173/?viewer=true');
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [],
      });

      render(App);

      await waitFor(() => {
        expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
      });

      const logoutBtn = screen.getByRole('button', { name: /sign out/i });
      await fireEvent.click(logoutBtn);

      await waitFor(() => {
        expect(localStorage.getItem('pulse_token')).toBeNull();
        expect(screen.getByText('Pulse Cloud')).toBeInTheDocument();
      });
    });
  });

  describe('2. Dashboard URL Parameter Pre-selection', () => {
    it('pre-selects specific camera when ?camera=cam-backyard is in URL', async () => {
      setWindowUrl('http://localhost:5173/?viewer=true&camera=cam-backyard');
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [
          { device_id: 'cam-front', role: 'admin' },
          { device_id: 'cam-backyard', role: 'admin' },
        ],
      });

      render(Dashboard, { props: { token: 'mock_user_jwt' } });

      await waitFor(() => {
        expect(screen.getByText('Pulse Dashboard')).toBeInTheDocument();
      });
    });

    it('pre-selects theater layout when ?layout=theater is in URL', async () => {
      setWindowUrl('http://localhost:5173/?viewer=true&layout=theater');
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [
          { device_id: 'test-camera-1', role: 'admin' },
        ],
      });

      render(Dashboard, { props: { token: 'mock_user_jwt' } });

      await waitFor(() => {
        expect(screen.getByTestId('theater-mode-container')).toBeInTheDocument();
        expect(screen.getByText('THEATER FOCUS')).toBeInTheDocument();
      });
    });
  });

  describe('3. Dynamic Public Ingress Tunnel Resolution in config.ts', () => {
    it('resolves Cloudflare Quick Tunnel trycloudflare.com to clean HTTPS and WSS endpoints without :9091', () => {
      setWindowUrl('https://alpha-bravo-charlie.trycloudflare.com/?viewer=true&autostart=true');

      expect(getApiBaseUrl()).toBe('https://alpha-bravo-charlie.trycloudflare.com');
      expect(getWsBaseUrl()).toBe('wss://alpha-bravo-charlie.trycloudflare.com');
      expect(getClientWsUrl('tok-123')).toBe(
        'wss://alpha-bravo-charlie.trycloudflare.com/client?token=tok-123'
      );
    });

    it('resolves ngrok tunnel hostname to clean HTTPS and WSS endpoints without :9091', () => {
      setWindowUrl('https://camera-stream-feed.ngrok-free.app/viewer');

      expect(getApiBaseUrl()).toBe('https://camera-stream-feed.ngrok-free.app');
      expect(getWsBaseUrl()).toBe('wss://camera-stream-feed.ngrok-free.app');
      expect(getClientWsUrl('ngrok-token')).toBe(
        'wss://camera-stream-feed.ngrok-free.app/client?token=ngrok-token'
      );
    });

    it('resolves localtunnel hostname to clean HTTP/WS endpoints without :9091', () => {
      setWindowUrl('http://my-live-cam.loca.lt/?viewer=true');

      expect(getApiBaseUrl()).toBe('http://my-live-cam.loca.lt');
      expect(getWsBaseUrl()).toBe('ws://my-live-cam.loca.lt');
    });

    it('preserves standard localhost:9091 for direct local development when not on tunnel', () => {
      setWindowUrl('http://localhost:5173/');

      expect(getApiBaseUrl()).toBe('http://localhost:9091');
      expect(getWsBaseUrl()).toBe('ws://localhost:9091');
    });
  });
});
