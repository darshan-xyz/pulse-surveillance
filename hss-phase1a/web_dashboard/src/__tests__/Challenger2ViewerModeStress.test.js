// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Dashboard from '../lib/Dashboard.svelte';
import CameraStream from '../lib/CameraStream.svelte';
import { getApiBaseUrl, getWsBaseUrl, getClientWsUrl } from '../lib/config';

describe('Empirical Challenger 2: Viewer Mode & WebRTC Comprehensive Stress Harness', () => {
  const testToken = 'challenger2-jwt-token-alpha-beta-gamma';
  const sampleDevices = [
    { device_id: 'cam-north-gate', role: 'edge', status: 'online' },
    { device_id: 'cam-south-perimeter', role: 'edge', status: 'online' },
    { device_id: 'cam-warehouse-vault', role: 'edge', status: 'online' },
    { device_id: 'cam-server-room', role: 'edge', status: 'online' },
    { device_id: 'cam-hq-lobby', role: 'edge', status: 'online' },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('Adversarial Test Suite 1: Mode Switcher, Persistence & State Transitions', () => {
    it('initializes to Home Mode when localStorage is empty, corrupt, or invalid', async () => {
      // Test with corrupt/unexpected string in localStorage
      localStorage.setItem('pulse_dashboard_mode', 'invalid_mode_string_123');

      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Enterprise Surveillance System')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Viewer Mode'
        );
      });
    });

    it('correctly initializes to Viewer Mode when localStorage is set to viewer', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByTestId('viewer-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('Remote View-Only Surveillance')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /toggle dashboard mode/i })).toHaveTextContent(
          'Switch to Home Mode'
        );
      });
    });

    it('handles rapid toggle stress (10 consecutive toggles) and maintains state consistency', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
      });

      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });

      for (let i = 0; i < 10; i++) {
        await fireEvent.click(modeBtn);
        const expectedMode = i % 2 === 0 ? 'viewer' : 'home';
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe(expectedMode);
      }

      // After 10 clicks (even number), we end on 'home'
      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe('home');
      });
    });

    it('verifies persistence updates when toggled back and forth multiple times', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
      });

      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        expect(screen.getByTestId('viewer-mode-badge')).toBeInTheDocument();
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe('viewer');
      });

      await fireEvent.click(modeBtn);

      await waitFor(() => {
        expect(screen.getByTestId('home-mode-badge')).toBeInTheDocument();
        expect(localStorage.getItem('pulse_dashboard_mode')).toBe('home');
      });
    });
  });

  describe('Adversarial Test Suite 2: UI Suppression & Camera Selection Integrity', () => {
    it('strictly hides Local Webcam from dropdown when in Viewer Mode', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);

      // In Home mode, local-webcam is present
      let optionValues = Array.from(select.querySelectorAll('option')).map((o) => o.value);
      expect(optionValues).toContain('local-webcam');

      // Toggle to Viewer mode
      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        const viewerOptionValues = Array.from(select.querySelectorAll('option')).map((o) => o.value);
        expect(viewerOptionValues).not.toContain('local-webcam');
        expect(viewerOptionValues).toEqual([
          'all',
          'cam-north-gate',
          'cam-south-perimeter',
          'cam-warehouse-vault',
          'cam-server-room',
          'cam-hq-lobby',
        ]);
      });
    });

    it('resets selectedCameraId from local-webcam to all upon switching to Viewer Mode', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);

      // Select Local Webcam
      await fireEvent.change(select, { target: { value: 'local-webcam' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Local Webcam' })).toBeInTheDocument();
      });

      // Switch to Viewer Mode
      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      // selectedCameraId must reset to 'all', rendering the grid of remote cameras
      await waitFor(() => {
        expect(screen.queryByRole('heading', { name: 'Local Webcam' })).not.toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-north-gate' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-south-perimeter' })).toBeInTheDocument();
        expect(select.value).toBe('all');
      });
    });

    it('preserves specific edge camera selection when toggling between Home and Viewer Mode', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByLabelText(/camera selection/i)).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);

      // Select specific edge camera: 'cam-warehouse-vault'
      await fireEvent.change(select, { target: { value: 'cam-warehouse-vault' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-warehouse-vault' })).toBeInTheDocument();
      });

      // Switch to Viewer Mode
      const modeBtn = screen.getByRole('button', { name: /toggle dashboard mode/i });
      await fireEvent.click(modeBtn);

      // Camera selection MUST remain 'cam-warehouse-vault' and NOT reset to 'all'
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-warehouse-vault' })).toBeInTheDocument();
        expect(select.value).toBe('cam-warehouse-vault');
        expect(screen.queryByRole('heading', { name: 'cam-north-gate' })).not.toBeInTheDocument();
      });

      // Switch back to Home Mode
      await fireEvent.click(modeBtn);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-warehouse-vault' })).toBeInTheDocument();
        expect(select.value).toBe('cam-warehouse-vault');
      });
    });
  });

  describe('Adversarial Test Suite 3: Multi-Camera Responsive Grid & Filtering', () => {
    it('dynamically adapts grid classes for 1, 2, 3, and 4 camera layouts in Viewer Mode', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByText('5 Active Nodes')).toBeInTheDocument();
      });

      const btn1Cam = screen.getByRole('button', { name: '1 Cam' });
      const btn2Cams = screen.getByRole('button', { name: '2 Cams' });
      const btn3Cams = screen.getByRole('button', { name: '3 Cams' });
      const btn4Cams = screen.getByRole('button', { name: '4 Cams' });

      // Default is 3 Cams
      expect(btn3Cams.className).toContain('bg-indigo-600');

      // Click 1 Cam
      await fireEvent.click(btn1Cam);
      expect(btn1Cam.className).toContain('bg-indigo-600');
      expect(btn3Cams.className).not.toContain('bg-indigo-600');

      // Click 2 Cams
      await fireEvent.click(btn2Cams);
      expect(btn2Cams.className).toContain('bg-indigo-600');
      expect(btn1Cam.className).not.toContain('bg-indigo-600');

      // Click 4 Cams
      await fireEvent.click(btn4Cams);
      expect(btn4Cams.className).toContain('bg-indigo-600');
      expect(btn2Cams.className).not.toContain('bg-indigo-600');
    });

    it('performs real-time case-insensitive search and provides Clear Filter recovery', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByText('5 Active Nodes')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText('Filter cameras...');

      // Search for uppercase 'VAULT'
      await fireEvent.input(searchInput, { target: { value: 'VAULT' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-warehouse-vault' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-north-gate' })).not.toBeInTheDocument();
        expect(screen.getByText('1 Active Node')).toBeInTheDocument();
      });

      // Search for non-existent camera
      await fireEvent.input(searchInput, { target: { value: 'non-existent-device-xyz' } });

      await waitFor(() => {
        expect(screen.getByText('No matching cameras')).toBeInTheDocument();
        expect(screen.getByText(/No devices matching/)).toBeInTheDocument();
        expect(screen.getByText('0 Active Nodes')).toBeInTheDocument();
      });

      // Click Clear Filter button
      const clearBtn = screen.getByRole('button', { name: 'Clear Filter' });
      await fireEvent.click(clearBtn);

      await waitFor(() => {
        expect(screen.getByText('5 Active Nodes')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-north-gate' })).toBeInTheDocument();
        const updatedSearchInput = screen.getByPlaceholderText('Filter cameras...');
        expect(updatedSearchInput.value).toBe('');
      });
    });

    it('hides search input and grid column selectors when a single camera is selected', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => sampleDevices,
      });

      render(Dashboard, { props: { token: testToken } });

      await waitFor(() => {
        expect(screen.getByPlaceholderText('Filter cameras...')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '1 Cam' })).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);
      await fireEvent.change(select, { target: { value: 'cam-north-gate' } });

      await waitFor(() => {
        expect(screen.queryByPlaceholderText('Filter cameras...')).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: '1 Cam' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: '2 Cams' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: '3 Cams' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: '4 Cams' })).not.toBeInTheDocument();
      });
    });
  });

  describe('Adversarial Test Suite 4: WebRTC Signaling, recvonly Transceiver & Edge Failures', () => {
    it('verifies WebRTC recvonly transceiver creation and SDP offer negotiation payload', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-test-recvonly', token: testToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBe(1);
      });

      const ws = globalThis.WebSocket.instances[0];
      expect(ws.url).toBe(`ws://localhost:9091/client?token=${encodeURIComponent(testToken)}`);

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBe(1);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];

      // CRITICAL CHECK: Exactly one video transceiver with direction: 'recvonly'
      expect(pc.transceivers.length).toBe(1);
      expect(pc.transceivers[0].direction).toBe('recvonly');

      // Verify SDP offer was sent over the WebSocket
      await waitFor(() => {
        const offerMsgs = ws.sentMessages.map((m) => JSON.parse(m)).filter((m) => m.type === 'offer');
        expect(offerMsgs.length).toBe(1);
        expect(offerMsgs[0]).toMatchObject({
          type: 'offer',
          device_id: 'cam-test-recvonly',
          camera_id: 'cam-test-recvonly',
        });
        expect(offerMsgs[0].sdp).toBeDefined();
      });
    });

    it('buffers trickle ICE candidates arriving before SDP answer and flushes upon answer', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-trickle-test', token: testToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBe(1);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const ws = globalThis.WebSocket.instances[0];

      const addCandidateSpy = vi.spyOn(pc, 'addIceCandidate');

      // Send 3 early ICE candidates BEFORE answer
      ws.simulateMessage({
        type: 'candidate',
        candidate: { candidate: 'candidate:1 1 UDP 2130706431 10.0.0.1 50001 typ host', sdpMid: '0', sdpMLineIndex: 0 },
      });
      ws.simulateMessage({
        type: 'candidate',
        candidate: { candidate: 'candidate:2 1 UDP 2130706431 10.0.0.2 50002 typ host', sdpMid: '0', sdpMLineIndex: 0 },
      });
      ws.simulateMessage({
        type: 'candidate',
        candidate: JSON.stringify({ candidate: 'candidate:3 1 UDP 2130706431 10.0.0.3 50003 typ host', sdpMid: '0', sdpMLineIndex: 0 }),
      });

      // No candidates should be added to peer connection yet since remoteDescription is not set
      expect(addCandidateSpy).not.toHaveBeenCalled();

      // Now send answer
      ws.simulateMessage({
        type: 'answer',
        sdp: 'v=0\r\no=- 222 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
      });

      // All 3 early candidates should be flushed and added
      await waitFor(() => {
        expect(addCandidateSpy).toHaveBeenCalledTimes(3);
      });
    });

    it('handles backend authentication/permission rejection (Forbidden error)', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-unauthorized', token: 'bad-token' },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBe(1);
      });

      const ws = globalThis.WebSocket.instances[0];

      // Simulate Forbidden error response from backend
      ws.simulateMessage({
        error: 'Forbidden',
      });

      await waitFor(() => {
        expect(screen.getByText('Access Denied')).toBeInTheDocument();
      });
    });

    it('handles WebSocket connection error and disconnection gracefully', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-disconnect-test', token: testToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBe(1);
      });

      const ws = globalThis.WebSocket.instances[0];

      // Simulate WebSocket error
      ws.simulateError(new Error('Network error'));

      await waitFor(() => {
        expect(screen.getByText('WebSocket Error')).toBeInTheDocument();
      });

      // Simulate WebSocket close
      ws.close();

      await waitFor(() => {
        expect(screen.getByText('Disconnected')).toBeInTheDocument();
      });
    });

    it('handles WebRTC connectionState transitions (connected, disconnected, failed)', async () => {
      render(CameraStream, {
        props: { deviceId: 'cam-state-test', token: testToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBe(1);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const ws = globalThis.WebSocket.instances[0];

      // Simulate answer and remote track
      ws.simulateMessage({
        type: 'answer',
        sdp: 'v=0\r\no=- 333 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
      });

      pc.simulateConnectionState('connected');

      await waitFor(() => {
        expect(screen.getByText('LIVE')).toBeInTheDocument();
      });

      // Simulate connection lost / failed
      pc.simulateConnectionState('failed');

      await waitFor(() => {
        expect(screen.getByText('Connection Lost')).toBeInTheDocument();
      });
    });
  });

  describe('Adversarial Test Suite 5: Dynamic Hostnames & Tunnel Protocol Matrix', () => {
    it('properly resolves HTTPS / WSS for secure remote origins and cloud tunnels', () => {
      const origLocation = window.location;
      delete window.location;
      window.location = {
        protocol: 'https:',
        hostname: 'viewer.pulse-surveillance.cloud',
        host: 'viewer.pulse-surveillance.cloud:9091',
        port: '9091',
        pathname: '/',
        search: '',
        hash: '',
        assign: vi.fn(),
      };

      const apiBase = getApiBaseUrl();
      const wsBase = getWsBaseUrl();
      const clientWs = getClientWsUrl('token123');

      expect(apiBase).toBe('https://viewer.pulse-surveillance.cloud:9091');
      expect(wsBase).toBe('wss://viewer.pulse-surveillance.cloud:9091');
      expect(clientWs).toBe('wss://viewer.pulse-surveillance.cloud:9091/client?token=token123');

      window.location = origLocation;
    });

    it('properly resolves standard HTTP / WS without ports when port is empty', () => {
      const origLocation = window.location;
      const origEnv = import.meta.env.VITE_API_PORT;
      import.meta.env.VITE_API_PORT = '';

      delete window.location;
      window.location = {
        protocol: 'http:',
        hostname: '192.168.1.150',
        host: '192.168.1.150',
        port: '',
        pathname: '/',
        search: '',
        hash: '',
        assign: vi.fn(),
      };

      const apiBase = getApiBaseUrl();
      const wsBase = getWsBaseUrl();
      const clientWs = getClientWsUrl('token456');

      expect(apiBase).toBe('http://192.168.1.150');
      expect(wsBase).toBe('ws://192.168.1.150');
      expect(clientWs).toBe('ws://192.168.1.150/client?token=token456');

      window.location = origLocation;
      import.meta.env.VITE_API_PORT = origEnv;
    });
  });
});
