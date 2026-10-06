// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Dashboard from '../lib/Dashboard.svelte';
import CameraStream from '../lib/CameraStream.svelte';

describe('Milestone M1 — Viewer Mode & Navigation Verification', () => {
  const mockToken = 'mock-viewer-jwt-token-999';
  const mockDevices = [
    { device_id: 'cam-gate-01', role: 'edge' },
    { device_id: 'cam-lobby-02', role: 'edge' },
    { device_id: 'cam-dock-03', role: 'edge' },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  describe('R1: Mode Switcher & Navigation Toggle', () => {
    it('initializes in Home Mode by default and displays Home Console badge', async () => {
      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Home Console')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Viewer Mode'
        );
      });
    });

    it('toggles to Viewer Mode when mode switcher button is clicked', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
      });

      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        expect(screen.getByTestId('viewer-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Viewer Mode')).toBeInTheDocument();
        expect(screen.getByText('Remote View-Only Surveillance')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Home Mode'
        );
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe('viewer');
      });

      // Toggle back to Home Mode
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Home Console')).toBeInTheDocument();
        expect(screen.getByText('Enterprise Surveillance System')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Viewer Mode'
        );
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe('home');
      });
    });

    it('restores Viewer Mode on initialization if saved in localStorage', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByTestId('viewer-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Viewer Mode')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Home Mode'
        );
      });
    });
  });

  describe('R3: Clean Viewer UI & Local Control Suppression', () => {
    it('shows Local Webcam option in Home Mode and suppresses it in Viewer Mode', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);
      let options = Array.from(select.querySelectorAll('option')).map((o) => o.textContent);
      expect(options).toContain('Local Webcam');
      expect(options).toContain('cam-gate-01');

      // Switch to Viewer Mode
      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        const updatedOptions = Array.from(select.querySelectorAll('option')).map(
          (o) => o.textContent
        );
        expect(updatedOptions).not.toContain('Local Webcam');
        expect(updatedOptions).toContain('All Cameras (Grid View)');
        expect(updatedOptions).toContain('cam-gate-01');
        expect(updatedOptions).toContain('cam-lobby-02');
        expect(updatedOptions).toContain('cam-dock-03');
      });
    });

    it('automatically resets selectedCameraId to "all" if switching to Viewer Mode with local-webcam active', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);

      // Select Local Webcam in Home Mode
      await fireEvent.change(select, { target: { value: 'local-webcam' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Local Webcam' })).toBeInTheDocument();
      });

      // Switch to Viewer Mode
      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      // Verify selectedCameraId was reset to 'all' (grid view rendered)
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-gate-01' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-lobby-02' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-dock-03' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'Local Webcam' })).not.toBeInTheDocument();
      });
    });

    it('preserves multi-camera grid layout selection and search filtering in Viewer Mode', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('3 Active Nodes')).toBeInTheDocument();
      });

      // Test Grid Layout Selector (1 Cam, 2 Cams, 3 Cams, 4 Cams)
      const btn2Cam = screen.getByRole('button', { name: '2 Cams' });
      await fireEvent.click(btn2Cam);
      expect(btn2Cam.className).toContain('bg-indigo-600');

      // Test Search filtering
      const searchInput = screen.getByPlaceholderText('Filter cameras...');
      await fireEvent.input(searchInput, { target: { value: 'lobby' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-lobby-02' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-gate-01' })).not.toBeInTheDocument();
        expect(screen.getByText('1 Active Node')).toBeInTheDocument();
      });
    });
  });

  describe('R2: Viewer WebRTC Connection & Recvonly Transceiver', () => {
    it('initiates /client WebSocket with dynamic URL and registers strictly recvonly transceiver', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-remote-view', token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBe(1);
      });

      const ws = globalThis.WebSocket.instances[0];
      expect(ws.url).toBe(`ws://localhost:9091/client?token=${encodeURIComponent(mockToken)}`);

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBe(1);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      expect(pc.transceivers.length).toBe(1);
      expect(pc.transceivers[0].direction).toBe('recvonly');
    });

    it('establishes remote video stream and renders LIVE badge', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-remote-view', token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBe(1);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const ws = globalThis.WebSocket.instances[0];

      // Simulate answer from cloud gateway
      ws.simulateMessage({
        type: 'answer',
        sdp: 'v=0\r\no=- 111 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
      });

      // Simulate remote video track arrival
      const mockTrack = { kind: 'video', stop: vi.fn(), onunmute: null };
      const mockStream = new globalThis.MediaStream([mockTrack]);
      pc.simulateTrack(mockStream);

      await waitFor(() => {
        expect(screen.getByText('LIVE')).toBeInTheDocument();
      });
    });
  });
});
