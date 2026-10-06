// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CameraStream from '../lib/CameraStream.svelte';
import Dashboard from '../lib/Dashboard.svelte';

describe('CameraStream Canvas Overlay & Emergency HUD Suite', () => {
  const mockDeviceId = 'cam-front-01';
  const mockToken = 'mock-jwt-token-xyz';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders HTML5 canvas overlay element directly over the video player with correct classes', () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    const canvas = container.querySelector('canvas');
    expect(canvas).toBeInTheDocument();
    expect(canvas).toHaveClass('absolute', 'inset-0', 'w-full', 'h-full', 'pointer-events-none', 'z-10');

    const video = container.querySelector('video');
    expect(video).toBeInTheDocument();
  });

  it('receives WebSocket fire detection message and draws red bounding box and FIRE badge', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const canvas = container.querySelector('canvas');
    expect(canvas).not.toBeNull();
    const ctx = canvas.getContext('2d');
    expect(ctx).not.toBeNull();

    // Reset calls before dispatching detection
    ctx._calls = [];

    // Send fire detection packet with normalized coordinates
    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      timestamp: Date.now(),
      detections: [
        {
          class: 'fire',
          class_id: 0,
          confidence: 0.85,
          box: [0.1, 0.2, 0.3, 0.4], // [x, y, w, h] normalized
        },
      ],
    });

    await waitFor(() => {
      // Check strokeRect was called
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBeGreaterThan(0);

      const strokeCall = strokeCalls[0];
      expect(strokeCall.strokeStyle).toBe('rgba(239, 68, 68, 0.9)'); // Red-500

      // Check fillRect for bounding box
      const fillCalls = ctx._calls.filter((c) => c.method === 'fillRect');
      expect(fillCalls.length).toBeGreaterThanOrEqual(2); // 1 for bbox fill, 1 for badge background

      const boxFill = fillCalls.find((c) => c.fillStyle === 'rgba(239, 68, 68, 0.15)');
      expect(boxFill).toBeDefined();

      // Check text label badge
      const textCalls = ctx._calls.filter((c) => c.method === 'fillText');
      expect(textCalls.length).toBeGreaterThan(0);
      expect(textCalls[0].args[0]).toBe('FIRE (85%)');
      expect(textCalls[0].fillStyle).toBe('#ffffff');
    });
  });

  it('receives WebSocket smoke detection message and draws amber bounding box and SMOKE badge', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const canvas = container.querySelector('canvas');
    const ctx = canvas.getContext('2d');
    ctx._calls = [];

    // Send smoke detection packet
    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      timestamp: Date.now(),
      detections: [
        {
          class: 'smoke',
          class_id: 1,
          confidence: 0.72,
          box: [0.25, 0.35, 0.4, 0.5],
        },
      ],
    });

    await waitFor(() => {
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBeGreaterThan(0);

      const strokeCall = strokeCalls[0];
      expect(strokeCall.strokeStyle).toBe('rgba(245, 158, 11, 0.9)'); // Amber-500

      const fillCalls = ctx._calls.filter((c) => c.method === 'fillRect');
      const boxFill = fillCalls.find((c) => c.fillStyle === 'rgba(245, 158, 11, 0.15)');
      expect(boxFill).toBeDefined();

      const textCalls = ctx._calls.filter((c) => c.method === 'fillText');
      expect(textCalls.length).toBeGreaterThan(0);
      expect(textCalls[0].args[0]).toBe('SMOKE (72%)');
    });
  });

  it('accurately calculates and scales normalized bounding box coordinates to canvas pixel resolution', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const canvas = container.querySelector('canvas');
    canvas.width = 1280;
    canvas.height = 720;
    const ctx = canvas.getContext('2d');
    ctx._calls = [];

    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        {
          class: 'fire',
          class_id: 0,
          confidence: 0.95,
          box: [0.1, 0.2, 0.5, 0.25],
        },
      ],
    });

    await waitFor(() => {
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBeGreaterThan(0);

      const [x, y, w, h] = strokeCalls[0].args;
      expect(x).toBeCloseTo(0.1 * 1280, 1);
      expect(y).toBeCloseTo(0.2 * 720, 1);
      expect(w).toBeCloseTo(0.5 * 1280, 1);
      expect(h).toBeCloseTo(0.25 * 720, 1);
    });
  });

  it('filters out detections with confidence below threshold (< 0.40)', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const canvas = container.querySelector('canvas');
    const ctx = canvas.getContext('2d');
    ctx._calls = [];

    // Send sub-threshold detection
    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        {
          class: 'fire',
          class_id: 0,
          confidence: 0.25, // < 0.40
          box: [0.1, 0.2, 0.3, 0.4],
        },
      ],
    });

    await waitFor(() => {
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBe(0);
      expect(screen.queryByTestId('fire-alert-badge')).not.toBeInTheDocument();
    });
  });

  it('displays emergency HUD badge and pulsating red border when fire is detected', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const pc = globalThis.RTCPeerConnection.instances[0];
    pc.simulateConnectionState('connected');

    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        {
          class: 'fire',
          class_id: 0,
          confidence: 0.88,
          box: [0.2, 0.2, 0.3, 0.3],
        },
      ],
    });

    await waitFor(() => {
      const badge = screen.getByTestId('fire-alert-badge');
      expect(badge).toBeInTheDocument();
      expect(badge).toHaveTextContent('FIRE DETECTED');

      // Outer card has red pulsating border
      const card = container.firstElementChild;
      expect(card.className).toContain('border-red-500');
      expect(card.className).toContain('ring-4');
      expect(card.className).toContain('animate-pulse');
    });
  });

  it('displays smoke HUD badge and amber border when smoke is detected', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const pc = globalThis.RTCPeerConnection.instances[0];
    pc.simulateConnectionState('connected');

    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        {
          class: 'smoke',
          class_id: 1,
          confidence: 0.65,
          box: [0.2, 0.2, 0.3, 0.3],
        },
      ],
    });

    await waitFor(() => {
      const badge = screen.getByTestId('smoke-alert-badge');
      expect(badge).toBeInTheDocument();
      expect(badge).toHaveTextContent('SMOKE DETECTED');

      const card = container.firstElementChild;
      expect(card.className).toContain('border-amber-500');
    });
  });

  it('auto-clears canvas overlay and HUD badges after 1000ms TTL when detections cease', async () => {
    vi.useFakeTimers();

    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    // Wait for WS connection using fake timer step
    await vi.advanceTimersByTimeAsync(20);

    const ws = globalThis.WebSocket.instances[0];
    expect(ws).toBeDefined();

    const canvas = container.querySelector('canvas');
    const ctx = canvas.getContext('2d');
    ctx._calls = [];

    // Send fire detection
    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        {
          class: 'fire',
          class_id: 0,
          confidence: 0.9,
          box: [0.1, 0.1, 0.2, 0.2],
        },
      ],
    });

    expect(ctx._calls.some((c) => c.method === 'strokeRect')).toBe(true);

    // Fast-forward past 1000ms TTL
    ctx._calls = [];
    await vi.advanceTimersByTimeAsync(1100);

    // Canvas should have been cleared
    const clearCalls = ctx._calls.filter((c) => c.method === 'clearRect');
    expect(clearCalls.length).toBeGreaterThan(0);
  });

  it('handles multiple concurrent detections in the same frame', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const canvas = container.querySelector('canvas');
    const ctx = canvas.getContext('2d');
    ctx._calls = [];

    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      detections: [
        { class: 'fire', class_id: 0, confidence: 0.92, box: [0.1, 0.1, 0.2, 0.3] },
        { class: 'smoke', class_id: 1, confidence: 0.78, box: [0.5, 0.5, 0.3, 0.3] },
      ],
    });

    await waitFor(() => {
      const strokeCalls = ctx._calls.filter((c) => c.method === 'strokeRect');
      expect(strokeCalls.length).toBe(2);

      const redStroke = strokeCalls.find((c) => c.strokeStyle === 'rgba(239, 68, 68, 0.9)');
      const amberStroke = strokeCalls.find((c) => c.strokeStyle === 'rgba(245, 158, 11, 0.9)');
      expect(redStroke).toBeDefined();
      expect(amberStroke).toBeDefined();

      const textCalls = ctx._calls.filter((c) => c.method === 'fillText');
      expect(textCalls.some((c) => c.args[0] === 'FIRE (92%)')).toBe(true);
      expect(textCalls.some((c) => c.args[0] === 'SMOKE (78%)')).toBe(true);
    });
  });

  it('dispatches detection CustomEvent with payload to parent components', async () => {
    const detectionHandler = vi.fn();
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
      events: { detection: detectionHandler },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    const mockDetections = [
      { class: 'fire', class_id: 0, confidence: 0.89, box: [0.2, 0.3, 0.4, 0.5] },
    ];

    ws.simulateMessage({
      type: 'detection',
      camera_id: mockDeviceId,
      timestamp: 1772465000000,
      detections: mockDetections,
    });

    await waitFor(() => {
      expect(detectionHandler).toHaveBeenCalledTimes(1);
      const eventDetail = detectionHandler.mock.calls[0][0].detail;
      expect(eventDetail.camera_id).toBe(mockDeviceId);
      expect(eventDetail.detections).toEqual(mockDetections);
      expect(eventDetail.timestamp).toBe(1772465000000);
    });
  });

  it('renders top-level Emergency Warning Banner in Dashboard when fire detection alert occurs', async () => {
    const mockDevices = [
      { device_id: 'cam-lobby-01', role: 'edge' },
      { device_id: 'cam-warehouse-02', role: 'edge' },
    ];

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockDevices,
    });

    render(Dashboard, { props: { token: mockToken } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'cam-lobby-01' })).toBeInTheDocument();
    });

    // Warning banner should initially be absent
    expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();

    // Trigger detection through active CameraStream WebSockets
    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    globalThis.WebSocket.instances.forEach((ws) => {
      ws.simulateMessage({
        type: 'detection',
        camera_id: 'cam-lobby-01',
        timestamp: Date.now(),
        detections: [
          { class: 'fire', class_id: 0, confidence: 0.94, box: [0.3, 0.3, 0.2, 0.2] },
        ],
      });
    });

    await waitFor(() => {
      const banner = screen.getByTestId('emergency-warning-banner');
      expect(banner).toBeInTheDocument();
      expect(banner).toHaveTextContent(/Fire Detected/i);
      expect(banner).toHaveTextContent('cam-lobby-01');
      expect(banner).toHaveTextContent('94%');
    });

    // Test Quick Focus Action
    const focusBtn = screen.getByRole('button', { name: /Focus Camera/i });
    expect(focusBtn).toBeInTheDocument();
    await fireEvent.click(focusBtn);

    // After focus, selected camera dropdown is updated to focused camera
    const cameraSelect = screen.getByLabelText('Camera Selection');
    expect(cameraSelect.value).toBe('cam-lobby-01');

    // Test Dismiss Alert
    const dismissBtn = screen.getByTitle('Dismiss Alert');
    await fireEvent.click(dismissBtn);

    expect(screen.queryByTestId('emergency-warning-banner')).not.toBeInTheDocument();
  });
});
