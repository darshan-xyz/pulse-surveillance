// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Dashboard from '../lib/Dashboard.svelte';
import CameraStream from '../lib/CameraStream.svelte';

describe('Challenger 2 Empirical Stress: Off-Screen Alert Bubbling & Multi-View Robustness', () => {
  const mockToken = 'jwt-challenger2-alert-stress-token';
  const testDevices = [
    { device_id: 'cam-lobby-01', role: 'edge', status: 'online' },
    { device_id: 'cam-warehouse-02', role: 'edge', status: 'online' },
    { device_id: 'cam-server-room-03', role: 'edge', status: 'online' },
    { device_id: 'cam-perimeter-04', role: 'edge', status: 'online' },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  describe('1. Theater View Mode Off-Screen Alert Bubbling', () => {
    it('bubbles off-screen camera fire alert to Emergency Warning Banner while viewing a different camera in Theater Mode', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      // Switch to Theater Mode
      const theaterBtn = screen.getByTestId('theater-mode-btn');
      await fireEvent.click(theaterBtn);

      await waitFor(() => {
        expect(screen.getByTestId('theater-mode-container')).toBeInTheDocument();
        expect(screen.getByText('THEATER FOCUS')).toBeInTheDocument();
      });

      // Initially active theater camera is cam-lobby-01 (or local-webcam if in home mode)
      // Switch theater main player explicitly to cam-lobby-01
      const thumbLobby = screen.getByTestId('thumbnail-cam-lobby-01');
      await fireEvent.click(thumbLobby);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-lobby-01' })).toBeInTheDocument();
      });

      // Emergency banner should NOT be present initially
      expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();

      // Find the active WebSocket on the mounted theater main player
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      const ws = globalThis.WebSocket.instances[globalThis.WebSocket.instances.length - 1];

      // Send fire detection packet for off-screen camera 'cam-server-room-03'
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-server-room-03',
        timestamp: Date.now(),
        detections: [
          {
            class: 'fire',
            class_id: 0,
            confidence: 0.92,
            box: [0.15, 0.25, 0.35, 0.45],
          },
        ],
      });

      // Verify that Emergency Warning Banner activates globally in Dashboard
      await waitFor(() => {
        const banner = screen.getByTestId('emergency-warning-banner');
        expect(banner).toBeInTheDocument();
        expect(banner).toHaveTextContent(/Emergency Alert: Fire Detected/i);
        expect(banner).toHaveTextContent('cam-server-room-03');
        expect(banner).toHaveTextContent('92%');
      });

      // Verify "Focus Camera" action shifts Theater view directly to cam-server-room-03
      const focusBtn = screen.getByRole('button', { name: /Focus Camera \(cam-server-room-03\)/i });
      expect(focusBtn).toBeInTheDocument();
      await fireEvent.click(focusBtn);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-server-room-03' })).toBeInTheDocument();
      });

      // Verify dismiss removes banner
      const dismissBtn = screen.getByTitle('Dismiss Alert');
      await fireEvent.click(dismissBtn);

      await waitFor(() => {
        expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();
      });
    });

    it('ensures off-screen detections do NOT render bounding boxes on the main theater player canvas', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      const { container } = render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      // Switch to Theater mode
      const theaterBtn = screen.getByTestId('theater-mode-btn');
      await fireEvent.click(theaterBtn);

      const thumbLobby = screen.getByTestId('thumbnail-cam-lobby-01');
      await fireEvent.click(thumbLobby);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-lobby-01' })).toBeInTheDocument();
      });

      const canvas = container.querySelector('canvas');
      expect(canvas).not.toBeNull();
      const ctx = canvas.getContext('2d');
      ctx._calls = [];

      const ws = globalThis.WebSocket.instances[globalThis.WebSocket.instances.length - 1];

      // Simulate off-screen camera detection for cam-warehouse-02
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-warehouse-02',
        timestamp: Date.now(),
        detections: [
          {
            class: 'fire',
            class_id: 0,
            confidence: 0.95,
            box: [0.1, 0.1, 0.5, 0.5],
          },
        ],
      });

      // Emergency banner should appear
      await waitFor(() => {
        expect(screen.getByTestId('emergency-warning-banner')).toBeInTheDocument();
      });

      // But the local canvas on cam-lobby-01 should NOT have drawn strokeRect for this detection
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBe(0);

      // Now send a detection for the ON-SCREEN camera cam-lobby-01
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-lobby-01',
        timestamp: Date.now(),
        detections: [
          {
            class: 'fire',
            class_id: 0,
            confidence: 0.88,
            box: [0.2, 0.2, 0.4, 0.4],
          },
        ],
      });

      await waitFor(() => {
        const onScreenStrokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
        expect(onScreenStrokeCalls.length).toBeGreaterThan(0);
      });
    });
  });

  describe('2. Single Camera View Mode Off-Screen Alert Bubbling', () => {
    it('bubbles off-screen smoke detection to Emergency Warning Banner while viewing a single camera', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      // Select single camera cam-lobby-01
      const select = screen.getByLabelText(/camera selection/i);
      await fireEvent.change(select, { target: { value: 'cam-lobby-01' } });

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-lobby-01' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-warehouse-02' })).not.toBeInTheDocument();
      });

      const ws = globalThis.WebSocket.instances[globalThis.WebSocket.instances.length - 1];

      // Dispatch smoke alert from off-screen camera cam-perimeter-04
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-perimeter-04',
        timestamp: Date.now(),
        detections: [
          {
            class: 'smoke',
            class_id: 1,
            confidence: 0.77,
            box: [0.3, 0.3, 0.3, 0.3],
          },
        ],
      });

      // Emergency banner activates with smoke details
      await waitFor(() => {
        const banner = screen.getByTestId('emergency-warning-banner');
        expect(banner).toBeInTheDocument();
        expect(banner).toHaveTextContent(/Emergency Alert: Smoke Detected/i);
        expect(banner).toHaveTextContent('cam-perimeter-04');
        expect(banner).toHaveTextContent('77%');
      });

      // Focus camera shifts single view to cam-perimeter-04
      const focusBtn = screen.getByRole('button', { name: /Focus Camera \(cam-perimeter-04\)/i });
      await fireEvent.click(focusBtn);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-perimeter-04' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-lobby-01' })).not.toBeInTheDocument();
        expect(select.value).toBe('cam-perimeter-04');
      });
    });
  });

  describe('3. Multi-Camera Grid Layouts Robustness & Layout Switching', () => {
    it('seamlessly switches between 1-Cam, 2-Cam, 3-Cam, and 4-Cam grid layouts while alerts arrive', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      const btn1Cam = screen.getByRole('button', { name: '1 Cam' });
      const btn2Cam = screen.getByRole('button', { name: '2 Cams' });
      const btn3Cam = screen.getByRole('button', { name: '3 Cams' });
      const btn4Cam = screen.getByRole('button', { name: '4 Cams' });

      // Test 1 Cam grid layout
      await fireEvent.click(btn1Cam);
      expect(btn1Cam.className).toContain('bg-indigo-600');

      // Test 2 Cams grid layout
      await fireEvent.click(btn2Cam);
      expect(btn2Cam.className).toContain('bg-indigo-600');

      // Test 4 Cams grid layout
      await fireEvent.click(btn4Cam);
      expect(btn4Cam.className).toContain('bg-indigo-600');

      // Deliver alert from cam-warehouse-02
      const ws = globalThis.WebSocket.instances[0];
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-warehouse-02',
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.89, box: [0.1, 0.1, 0.2, 0.2] },
        ],
      });

      await waitFor(() => {
        expect(screen.getByTestId('emergency-warning-banner')).toBeInTheDocument();
      });

      // Switch back to 3 Cams grid layout while banner is active
      await fireEvent.click(btn3Cam);
      expect(btn3Cam.className).toContain('bg-indigo-600');
      expect(screen.getByTestId('emergency-warning-banner')).toBeInTheDocument();
    });
  });

  describe('4. Viewer Mode Off-Screen Alert Bubbling', () => {
    it('activates Emergency Warning Banner and supports Focus Camera in Viewer Mode', async () => {
      localStorage.setItem('pulse_dashboard_mode', 'viewer');

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByTestId('viewer-mode-badge')).toBeInTheDocument();
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      // In viewer mode, switch to theater mode
      const theaterBtn = screen.getByTestId('theater-mode-btn');
      await fireEvent.click(theaterBtn);

      await waitFor(() => {
        expect(screen.getByTestId('theater-mode-container')).toBeInTheDocument();
      });

      const ws = globalThis.WebSocket.instances[globalThis.WebSocket.instances.length - 1];

      // Send fire detection packet for cam-perimeter-04
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-perimeter-04',
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.96, box: [0.1, 0.2, 0.3, 0.4] },
        ],
      });

      await waitFor(() => {
        const banner = screen.getByTestId('emergency-warning-banner');
        expect(banner).toBeInTheDocument();
        expect(banner).toHaveTextContent(/Fire Detected/i);
        expect(banner).toHaveTextContent('cam-perimeter-04');
        expect(banner).toHaveTextContent('96%');
      });

      // Focus camera in viewer mode
      const focusBtn = screen.getByRole('button', { name: /Focus Camera/i });
      await fireEvent.click(focusBtn);

      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-perimeter-04' })).toBeInTheDocument();
      });
    });
  });

  describe('5. Confidence Threshold & Malformed Payload Adversarial Filtering', () => {
    it('strictly rejects detections with confidence below 0.30 threshold', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      const ws = globalThis.WebSocket.instances[0];

      // Sub-threshold detection (0.28 < 0.30)
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-lobby-01',
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.28, box: [0.1, 0.1, 0.2, 0.2] },
          { class: 'smoke', class_id: 1, confidence: 0.25, box: [0.3, 0.3, 0.2, 0.2] },
        ],
      });

      // Emergency banner should NOT appear
      expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();

      // Borderline passing detection (0.30)
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-lobby-01',
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.30, box: [0.1, 0.1, 0.2, 0.2] },
        ],
      });

      await waitFor(() => {
        expect(screen.getByTestId('emergency-warning-banner')).toBeInTheDocument();
      });
    });

    it('safely handles malformed and edge-case detection packets without throwing', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      const ws = globalThis.WebSocket.instances[0];

      const malformedPackets = [
        { type: 'detection' },
        { type: 'detection', detections: null },
        { type: 'detection', detections: 'invalid-string' },
        { type: 'detection', detections: [] },
        { type: 'detection', detections: [{}] },
        { type: 'detection', detections: [{ class: 'person', confidence: 0.99 }] },
        { type: 'detection', detections: [{ class_id: 99, confidence: 0.99 }] },
        { type: 'detection', camera_id: null, detections: [{ class_id: 0, confidence: 0.85 }] },
      ];

      for (const pkt of malformedPackets) {
        expect(() => ws.simulateMessage(pkt)).not.toThrow();
      }
    });
  });

  describe('6. High-Frequency Stress Burst & Deduplication', () => {
    it('handles high-frequency burst (50 detection packets) and maintains deduplicated activeAlerts', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await waitFor(() => {
        expect(screen.getByText('4 Active Nodes')).toBeInTheDocument();
      });

      const ws = globalThis.WebSocket.instances[0];

      // Rapid burst of 50 packets across 2 cameras
      for (let i = 0; i < 50; i++) {
        const camId = i % 2 === 0 ? 'cam-lobby-01' : 'cam-warehouse-02';
        const conf = 0.50 + (i % 40) * 0.01;
        ws.simulateMessage({
          type: 'detection',
          camera_id: camId,
          timestamp: Date.now() + i,
          detections: [
            { class: 'fire', class_id: 0, confidence: conf, box: [0.1, 0.1, 0.2, 0.2] },
          ],
        });
      }

      await waitFor(() => {
        const banner = screen.getByTestId('emergency-warning-banner');
        expect(banner).toBeInTheDocument();
        // Should list both cameras in activeAlerts without crashing or duplicating
        expect(banner).toHaveTextContent('cam-lobby-01');
        expect(banner).toHaveTextContent('cam-warehouse-02');
      });
    });
  });

  describe('7. Alert TTL Expiration', () => {
    it('automatically removes expired alerts after TTL (3000ms)', async () => {
      vi.useFakeTimers();

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => testDevices,
      });

      render(Dashboard, { props: { token: mockToken } });

      await vi.advanceTimersByTimeAsync(50);

      const ws = globalThis.WebSocket.instances[0];
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-lobby-01',
        timestamp: Date.now(),
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.91, box: [0.1, 0.1, 0.2, 0.2] },
        ],
      });

      // Run microtasks
      await vi.advanceTimersByTimeAsync(100);

      const banner = screen.getByTestId('emergency-warning-banner');
      expect(banner).toBeInTheDocument();

      // Fast forward past 3000ms TTL + 500ms cleanup interval
      await vi.advanceTimersByTimeAsync(3600);

      expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();
    });
  });
});
