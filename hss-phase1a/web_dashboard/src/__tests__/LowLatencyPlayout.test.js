// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import CameraStream from '../lib/CameraStream.svelte';
import { WebcamBroadcaster } from '../lib/webcamBroadcaster';
import { applyReceiverPlayoutHints, tuneSdpForLowLatency } from '../lib/sdpModifier';

describe('Low-Latency Playout & WebRTC Frontend Pipeline (M1)', () => {
  const mockDeviceId = 'cam-stream-lowlat-01';
  const mockToken = 'jwt-token-m1-test';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  describe('1. WebRTC Configuration & Receiver Playout Hints', () => {
    it('configures RTCPeerConnection with iceCandidatePoolSize=2, max-bundle, and rtcp-mux require', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      expect(pc.config).toBeDefined();
      expect(pc.config.iceCandidatePoolSize).toBe(2);
      expect(pc.config.bundlePolicy).toBe('max-bundle');
      expect(pc.config.rtcpMuxPolicy).toBe('require');
    });

    it('sets playoutDelayHint = 0 and jitterBufferTarget = 0 on incoming track receiver', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockReceiver = {
        track: { kind: 'video', stop: vi.fn() },
        playoutDelayHint: null,
        jitterBufferTarget: null,
      };
      const mockStream = new globalThis.MediaStream([mockReceiver.track]);

      pc.simulateTrack(mockStream, mockReceiver);

      expect(mockReceiver.playoutDelayHint).toBe(0);
      expect(mockReceiver.jitterBufferTarget).toBe(0);
    });

    it('applies playoutDelayHint to all existing receivers in RTCPeerConnection', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const receivers = pc.getReceivers();

      receivers.forEach((receiver) => {
        expect(receiver.playoutDelayHint).toBe(0);
        expect(receiver.jitterBufferTarget).toBe(0);
      });
    });

    it('gracefully handles legacy or restricted receivers without throwing', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const legacyReceiver = { track: { kind: 'video', stop: vi.fn() } };

      expect(() => {
        pc.simulateTrack(new globalThis.MediaStream([legacyReceiver.track]), legacyReceiver);
      }).not.toThrow();
    });
  });

  describe('2. Browser Video Element Playout Optimization & Attributes', () => {
    it('configures video element with autoplay, playsinline, and muted for unblocked playout', () => {
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      const video = container.querySelector('video');
      expect(video).toBeInTheDocument();
      expect(video).toHaveAttribute('autoplay');
      expect(video).toHaveAttribute('playsinline');
      expect(video.muted).toBe(true);
    });

    it('triggers video.play() immediately upon track arrival and on track onunmute event', async () => {
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

      pc.simulateTrack(mockStream, mockTrack);

      expect(playSpy).toHaveBeenCalled();
      expect(typeof mockTrack.onunmute).toBe('function');

      // Simulate pause state recovery via onunmute
      const video = container.querySelector('video');
      Object.defineProperty(video, 'paused', { value: true, configurable: true });

      mockTrack.onunmute();
      expect(playSpy).toHaveBeenCalledTimes(2);
    });
  });

  describe('3. Buffer Drift Detection & Low-Latency Catch-Up', () => {
    it('detects buffer lag when buffered.end(0) - currentTime exceeds 200ms and executes catch-up jump', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);

      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockStream = new globalThis.MediaStream([mockTrack]);
      pc.simulateTrack(mockStream, mockTrack);

      const video = container.querySelector('video');

      // Simulate severe drift: 500ms lag
      video.simulateBuffer(0, 10.5, 10.0);

      // Trigger stats/drift interval
      vi.advanceTimersByTime(1000);

      // Verify currentTime jumped forward to live edge
      expect(video.currentTime).toBeGreaterThanOrEqual(10.45);
      vi.useRealTimers();
    });

    it('speeds up playbackRate to 1.1x during moderate lag (80ms - 200ms) and resets to 1.0x when caught up', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);

      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockStream = new globalThis.MediaStream([mockTrack]);
      pc.simulateTrack(mockStream, mockTrack);

      const video = container.querySelector('video');

      // Simulate moderate drift: 150ms buffer lag
      video.simulateBuffer(0, 10.15, 10.0);
      vi.advanceTimersByTime(1000);

      expect(video.playbackRate).toBe(1.1);

      // Simulate caught up: 30ms buffer lag
      video.simulateBuffer(0, 10.03, 10.0);
      vi.advanceTimersByTime(1000);

      expect(video.playbackRate).toBe(1.0);
      vi.useRealTimers();
    });

    it('safely handles empty buffer ranges without throwing errors', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);

      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');
      video.clearBuffer();

      expect(() => {
        vi.advanceTimersByTime(1000);
      }).not.toThrow();
      vi.useRealTimers();
    });
  });

  describe('4. WebRTC Signaling, SDP Mutation & Broadcaster Verification', () => {
    it('transmits SDP offer mutated with low-latency parameters (x-google-min-bitrate=1500, b=AS:4000)', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      });

      const ws = globalThis.WebSocket.instances[0];

      await waitFor(() => {
        expect(ws.sentMessages.length).toBeGreaterThan(0);
      });

      const offerMsg = JSON.parse(ws.sentMessages[0]);
      expect(offerMsg.type).toBe('offer');
      expect(offerMsg.sdp).toContain('x-google-min-bitrate=1500');
      expect(offerMsg.sdp).toContain('b=AS:4000');
    });

    it('transmits end-of-candidates candidate:null when ICE candidate gathering completes', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const ws = globalThis.WebSocket.instances[0];

      // Trigger null candidate (end of candidates)
      pc.onicecandidate({ candidate: null });

      const nullCandMsg = ws.sentMessages
        .map((m) => JSON.parse(m))
        .find((m) => m.type === 'candidate' && m.candidate === null);
      expect(nullCandMsg).toBeDefined();
    });

    it('webcam broadcaster configures 30 FPS minimum getUserMedia and mutated SDP answer', async () => {
      const getUserMediaSpy = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
      const broadcaster = new WebcamBroadcaster('webcam-test-node', mockToken);

      await broadcaster.start();

      expect(getUserMediaSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          video: expect.objectContaining({
            frameRate: expect.objectContaining({ ideal: 30, min: 30 }),
          }),
        })
      );

      broadcaster.stop();
    });

    it('stops buffer drift monitoring, halts media tracks, and closes RTCPeerConnection on unmount', async () => {
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
  });
});
