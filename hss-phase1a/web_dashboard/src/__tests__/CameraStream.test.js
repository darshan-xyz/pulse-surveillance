// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CameraStream from '../lib/CameraStream.svelte';

describe('CameraStream Component', () => {
  const mockDeviceId = 'cam-front-01';
  const mockToken = 'mock-jwt-token-abc';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  it('renders camera card with device ID and reconnect button', () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    expect(screen.getByText(mockDeviceId)).toBeInTheDocument();
    expect(screen.getByText('Edge Node via WebRTC')).toBeInTheDocument();
    expect(screen.getByTitle('Reconnect Stream')).toBeInTheDocument();
  });

  it('initializes WebSocket and RTCPeerConnection and transmits SDP offer', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      const ws = globalThis.WebSocket.instances[0];
      expect(ws.url).toBe(
        `ws://localhost:9091/client?token=${encodeURIComponent(mockToken)}`
      );
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      const pc = globalThis.RTCPeerConnection.instances[0];
      expect(pc.localDescription).not.toBeNull();
      expect(pc.transceivers.length).toBe(1);
      expect(pc.transceivers[0].direction).toBe('recvonly');

      const ws = globalThis.WebSocket.instances[0];
      expect(ws.sentMessages.length).toBeGreaterThan(0);
      const offerMsg = JSON.parse(ws.sentMessages[0]);
      expect(offerMsg.type).toBe('offer');
      expect(offerMsg.device_id).toBe(mockDeviceId);
      expect(offerMsg.sdp).toBeDefined();
    });
  });

  it('handles incoming SDP answer and establishes remote description', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const ws = globalThis.WebSocket.instances[0];

    const answerSdp = 'v=0\r\no=- 999 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n';
    ws.simulateMessage({
      type: 'answer',
      sdp: answerSdp,
    });

    await waitFor(() => {
      expect(pc.remoteDescription).not.toBeNull();
      expect(pc.remoteDescription.type).toBe('answer');
      expect(pc.remoteDescription.sdp).toBe(answerSdp);
    });
  });

  it('handles incoming ICE candidate exchange', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const ws = globalThis.WebSocket.instances[0];

    // First set answer so remoteDescription is populated
    ws.simulateMessage({
      type: 'answer',
      sdp: 'v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
    });

    const addIceSpy = vi.spyOn(pc, 'addIceCandidate');

    ws.simulateMessage({
      type: 'candidate',
      candidate: JSON.stringify({
        candidate: 'candidate:1 1 UDP 2130706431 10.0.0.1 50000 typ host',
        sdpMid: '0',
        sdpMLineIndex: 0,
      }),
    });

    await waitFor(() => {
      expect(addIceSpy).toHaveBeenCalled();
    });
  });

  it('displays LIVE badge and video active state when track is received', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const mockTrack = { kind: 'video', stop: vi.fn() };
    const mockStream = new globalThis.MediaStream([mockTrack]);

    pc.simulateTrack(mockStream);

    await waitFor(() => {
      expect(screen.getByText('LIVE')).toBeInTheDocument();
    });
  });

  it('handles Forbidden error by showing Access Denied and cleaning up resources', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });

    const ws = globalThis.WebSocket.instances[0];
    ws.simulateMessage({
      error: 'Forbidden',
    });

    await waitFor(() => {
      expect(screen.getByText('Access Denied')).toBeInTheDocument();
    });
  });

  it('re-initiates connection when reconnect button is clicked', async () => {
    render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBe(1);
    });

    const reconnectBtn = screen.getByTitle('Reconnect Stream');
    await fireEvent.click(reconnectBtn);

    await waitFor(() => {
      expect(globalThis.WebSocket.instances.length).toBe(2);
    });
  });

  it('cleans up resources on component unmount', async () => {
    const { unmount } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const ws = globalThis.WebSocket.instances[0];
    const pcCloseSpy = vi.spyOn(pc, 'close');
    const wsCloseSpy = vi.spyOn(ws, 'close');

    unmount();

    expect(pcCloseSpy).toHaveBeenCalled();
    expect(wsCloseSpy).toHaveBeenCalled();
  });

  it('renders video element with required attributes (autoplay, playsinline, muted)', () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    const video = container.querySelector('video');
    expect(video).toBeInTheDocument();
    expect(video).toHaveAttribute('autoplay');
    expect(video).toHaveAttribute('playsinline');
    expect(video.muted).toBe(true);
    expect(video.className).toContain('w-full');
    expect(video.className).toContain('h-full');
    expect(video.className).toContain('object-cover');
  });

  it('binds MediaStream to video srcObject and triggers play on track arrival', async () => {
    const playSpy = vi.spyOn(HTMLVideoElement.prototype, 'play');
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const mockTrack = { kind: 'video', stop: vi.fn(), onunmute: null };
    const mockStream = new globalThis.MediaStream([mockTrack]);

    pc.ontrack({
      streams: [mockStream],
      track: mockTrack,
    });

    const video = container.querySelector('video');
    expect(video).toBeInTheDocument();
    expect(video.srcObject).toBe(mockStream);
    expect(playSpy).toHaveBeenCalled();
    expect(typeof mockTrack.onunmute).toBe('function');

    // Verify onunmute triggers play when video is paused
    Object.defineProperty(video, 'paused', { value: true, configurable: true });
    mockTrack.onunmute();
    expect(playSpy).toHaveBeenCalledTimes(2);
  });

  it('creates new MediaStream from event.track if event.streams is missing', async () => {
    const { container } = render(CameraStream, {
      props: { deviceId: mockDeviceId, token: mockToken },
    });

    await waitFor(() => {
      expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
    });

    const pc = globalThis.RTCPeerConnection.instances[0];
    const mockTrack = { kind: 'video', stop: vi.fn(), onunmute: null };

    pc.ontrack({
      streams: [],
      track: mockTrack,
    });

    const video = container.querySelector('video');
    expect(video.srcObject).toBeInstanceOf(globalThis.MediaStream);
    expect(video.srcObject.getTracks()).toContain(mockTrack);
  });

  it('supports local-webcam mode by invoking navigator.mediaDevices.getUserMedia and attaching stream', async () => {
    const mockTrack = { kind: 'video', stop: vi.fn() };
    const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
    const getUserMediaSpy = vi.fn().mockResolvedValue(mockWebcamStream);
    navigator.mediaDevices.getUserMedia = getUserMediaSpy;

    const playSpy = vi.spyOn(HTMLVideoElement.prototype, 'play');

    const { container } = render(CameraStream, {
      props: { deviceId: 'local-webcam', token: mockToken },
    });

    await waitFor(() => {
      expect(getUserMediaSpy).toHaveBeenCalledWith({
        video: true,
        audio: false,
      });
      const video = container.querySelector('video');
      expect(video.srcObject).toBe(mockWebcamStream);
      expect(playSpy).toHaveBeenCalled();
      expect(screen.getByText('Local Webcam')).toBeInTheDocument();
      expect(screen.getByText('Browser MediaStream')).toBeInTheDocument();
      expect(screen.getByText('LIVE')).toBeInTheDocument();
    });

    // Should NOT create WebSocket or RTCPeerConnection for local webcam
    expect(globalThis.WebSocket.instances.length).toBe(0);
    expect(globalThis.RTCPeerConnection.instances.length).toBe(0);
  });

  it('handles webcam access denied (NotAllowedError) by displaying Webcam Access Denied', async () => {
    const notAllowedError = new Error('Permission denied');
    notAllowedError.name = 'NotAllowedError';
    navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(notAllowedError);

    render(CameraStream, {
      props: { deviceId: 'local-webcam', token: mockToken },
    });

    await waitFor(() => {
      expect(screen.getByText('Webcam Access Denied')).toBeInTheDocument();
    });
  });

  it('handles webcam not found (NotFoundError) by displaying No Webcam Detected', async () => {
    const notFoundError = new Error('Device not found');
    notFoundError.name = 'NotFoundError';
    navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(notFoundError);

    render(CameraStream, {
      props: { deviceId: 'local-webcam', token: mockToken },
    });

    await waitFor(() => {
      expect(screen.getByText('No Webcam Detected')).toBeInTheDocument();
    });
  });

  it('stops webcam tracks on unmount', async () => {
    const mockTrack = { kind: 'video', stop: vi.fn() };
    const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
    navigator.mediaDevices.getUserMedia = vi.fn().mockResolvedValue(mockWebcamStream);

    const { unmount } = render(CameraStream, {
      props: { deviceId: 'local-webcam', token: mockToken },
    });

    await waitFor(() => {
      expect(screen.getByText('Local Webcam')).toBeInTheDocument();
    });

    unmount();
    expect(mockTrack.stop).toHaveBeenCalled();
  });

  it('switches cleanly from local-webcam to edge camera and stops webcam tracks', async () => {
    const mockTrack = { kind: 'video', stop: vi.fn() };
    const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
    navigator.mediaDevices.getUserMedia = vi.fn().mockResolvedValue(mockWebcamStream);

    const { rerender } = render(CameraStream, {
      props: { deviceId: 'local-webcam', token: mockToken },
    });

    await waitFor(() => {
      expect(screen.getByText('Local Webcam')).toBeInTheDocument();
    });

    // Switch deviceId to remote edge camera
    await rerender({ deviceId: 'cam-dock-02', token: mockToken });

    await waitFor(() => {
      expect(mockTrack.stop).toHaveBeenCalled();
      expect(screen.getByText('cam-dock-02')).toBeInTheDocument();
      expect(screen.getByText('Edge Node via WebRTC')).toBeInTheDocument();
      expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
    });
  });
});
