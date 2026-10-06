// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import CameraStream from '../lib/CameraStream.svelte';
import Dashboard from '../lib/Dashboard.svelte';

describe('Empirical Challenge & Stress Test Suite', () => {
  const mockToken = 'mock-jwt-token-xyz';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  describe('1. Camera Selection Dropdown & Multi-View Transitions', () => {
    it('handles switching from All Cameras (Grid View) to Local Webcam to individual edge camera and back', async () => {
      const mockDevices = [
        { device_id: 'cam-alpha', role: 'edge' },
        { device_id: 'cam-beta', role: 'edge' },
        { device_id: 'cam-gamma', role: 'edge' },
      ];

      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockDevices,
      });

      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
      navigator.mediaDevices.getUserMedia = vi.fn().mockResolvedValue(mockWebcamStream);

      const { container } = render(Dashboard, { props: { token: mockToken } });

      // Initial state: Grid view with 3 active cameras
      await waitFor(() => {
        expect(screen.getByText('3 Active Nodes')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-alpha' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-beta' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-gamma' })).toBeInTheDocument();
      });

      const select = screen.getByLabelText(/camera selection/i);

      // Step 1: Switch to Local Webcam
      await fireEvent.change(select, { target: { value: 'local-webcam' } });
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'Local Webcam' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-alpha' })).not.toBeInTheDocument();
      });

      // Step 2: Switch to individual camera 'cam-beta'
      await fireEvent.change(select, { target: { value: 'cam-beta' } });
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-beta' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-alpha' })).not.toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'Local Webcam' })).not.toBeInTheDocument();
      });

      // Step 3: Switch to individual camera 'cam-gamma'
      await fireEvent.change(select, { target: { value: 'cam-gamma' } });
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-gamma' })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'cam-beta' })).not.toBeInTheDocument();
      });

      // Step 4: Return to 'all' Grid View
      await fireEvent.change(select, { target: { value: 'all' } });
      await waitFor(() => {
        expect(screen.getByRole('heading', { name: 'cam-alpha' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-beta' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'cam-gamma' })).toBeInTheDocument();
      });
    });
  });

  describe('2. Rapid Switching & Race Conditions', () => {
    it('handles rapid sequential switching of deviceId on CameraStream without throwing or leaking', async () => {
      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
      let resolveWebcam;
      navigator.mediaDevices.getUserMedia = vi.fn().mockImplementation(() => {
        return new Promise((resolve) => {
          resolveWebcam = () => resolve(mockWebcamStream);
        });
      });

      const { rerender } = render(CameraStream, {
        props: { deviceId: 'cam-01', token: mockToken },
      });

      // Rapid switches between remote cameras and webcam
      await rerender({ deviceId: 'cam-02', token: mockToken });
      await rerender({ deviceId: 'local-webcam', token: mockToken });
      await rerender({ deviceId: 'cam-03', token: mockToken });
      await rerender({ deviceId: 'cam-04', token: mockToken });

      // If delayed getUserMedia resolves late after deviceId has switched to cam-04
      if (resolveWebcam) resolveWebcam();

      await waitFor(() => {
        expect(screen.getByText('cam-04')).toBeInTheDocument();
        expect(screen.getByText('Edge Node via WebRTC')).toBeInTheDocument();
      });
    });

    it('handles component unmount during in-flight getUserMedia resolution', async () => {
      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
      let resolveWebcam;
      navigator.mediaDevices.getUserMedia = vi.fn().mockImplementation(() => {
        return new Promise((resolve) => {
          resolveWebcam = () => resolve(mockWebcamStream);
        });
      });

      const { unmount } = render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      // Unmount before getUserMedia resolves
      unmount();

      // Now resolve getUserMedia
      if (resolveWebcam) resolveWebcam();

      // Wait a tick and verify tracks are stopped and no unhandled error
      await new Promise((r) => setTimeout(r, 20));
      expect(mockTrack.stop).toHaveBeenCalled();
    });

    it('handles component unmount during in-flight WebRTC negotiation', async () => {
      const { unmount } = render(CameraStream, {
        props: { deviceId: 'cam-stress-01', token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      });

      const ws = globalThis.WebSocket.instances[0];

      // Unmount immediately
      unmount();

      // Simulate incoming message to closed websocket
      expect(() => {
        ws.simulateMessage({
          type: 'answer',
          sdp: 'v=0\r\ns=-\r\n',
        });
        ws.simulateMessage({
          type: 'candidate',
          candidate: 'candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host',
        });
      }).not.toThrow();
    });
  });

  describe('3. Permission & Hardware Rejection Error States', () => {
    it('handles NotAllowedError (Permission Denied)', async () => {
      const err = new Error('Permission Denied by user');
      err.name = 'NotAllowedError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Webcam Access Denied')).toBeInTheDocument();
      });
    });

    it('handles PermissionDeniedError (Legacy Permission Denied)', async () => {
      const err = new Error('Permission Denied legacy');
      err.name = 'PermissionDeniedError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Webcam Access Denied')).toBeInTheDocument();
      });
    });

    it('handles NotFoundError (No camera hardware found)', async () => {
      const err = new Error('No device found');
      err.name = 'NotFoundError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('No Webcam Detected')).toBeInTheDocument();
      });
    });

    it('handles DevicesNotFoundError (Legacy no camera hardware found)', async () => {
      const err = new Error('No device found');
      err.name = 'DevicesNotFoundError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('No Webcam Detected')).toBeInTheDocument();
      });
    });

    it('handles NotReadableError (Camera in use by another application)', async () => {
      const err = new Error('Device in use');
      err.name = 'NotReadableError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Webcam Error')).toBeInTheDocument();
      });
    });

    it('handles OverconstrainedError (Requested resolution/constraints impossible)', async () => {
      const err = new Error('Constraint could not be satisfied');
      err.name = 'OverconstrainedError';
      navigator.mediaDevices.getUserMedia = vi.fn().mockRejectedValue(err);

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Webcam Error')).toBeInTheDocument();
      });
    });

    it('handles missing navigator.mediaDevices (Insecure HTTP origin / unsupported browser)', async () => {
      const originalMediaDevices = navigator.mediaDevices;
      try {
        Object.defineProperty(navigator, 'mediaDevices', {
          value: undefined,
          writable: true,
          configurable: true,
        });

        render(CameraStream, {
          props: { deviceId: 'local-webcam', token: mockToken },
        });

        await waitFor(() => {
          expect(screen.getByText('Webcam API Not Supported')).toBeInTheDocument();
        });
      } finally {
        Object.defineProperty(navigator, 'mediaDevices', {
          value: originalMediaDevices,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe('4. Track Cleanup Verification', () => {
    it('stops all tracks in local MediaStream upon component destroy', async () => {
      const track1 = { kind: 'video', stop: vi.fn() };
      const track2 = { kind: 'video', stop: vi.fn() };
      const stream = new globalThis.MediaStream([track1, track2]);
      navigator.mediaDevices.getUserMedia = vi.fn().mockResolvedValue(stream);

      const { unmount } = render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Local Webcam')).toBeInTheDocument();
      });

      unmount();

      expect(track1.stop).toHaveBeenCalled();
      expect(track2.stop).toHaveBeenCalled();
    });

    it('stops previous tracks when switching from webcam to remote camera and back to webcam', async () => {
      const track1 = { kind: 'video', stop: vi.fn() };
      const stream1 = new globalThis.MediaStream([track1]);

      const track2 = { kind: 'video', stop: vi.fn() };
      const stream2 = new globalThis.MediaStream([track2]);

      let callCount = 0;
      navigator.mediaDevices.getUserMedia = vi.fn().mockImplementation(() => {
        callCount++;
        return Promise.resolve(callCount === 1 ? stream1 : stream2);
      });

      const { rerender } = render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(screen.getByText('Local Webcam')).toBeInTheDocument();
      });

      // Switch to edge camera
      await rerender({ deviceId: 'cam-east-01', token: mockToken });
      await waitFor(() => {
        expect(track1.stop).toHaveBeenCalled();
        expect(screen.getByText('cam-east-01')).toBeInTheDocument();
      });

      // Switch back to local webcam
      await rerender({ deviceId: 'local-webcam', token: mockToken });
      await waitFor(() => {
        expect(screen.getByText('Local Webcam')).toBeInTheDocument();
      });
    });
  });

  describe('5. Browser Autoplay & Attribute Compliance', () => {
    it('ensures video element has autoplay, playsinline, and muted attributes', () => {
      const { container } = render(CameraStream, {
        props: { deviceId: 'cam-test-attributes', token: mockToken },
      });

      const video = container.querySelector('video');
      expect(video).toBeInTheDocument();
      expect(video.hasAttribute('autoplay')).toBe(true);
      expect(video.hasAttribute('playsinline')).toBe(true);
      expect(video.muted).toBe(true);
    });

    it('handles play() promise rejection gracefully (e.g. policy restriction without crashing)', async () => {
      const playRejection = new Error('The play() request was interrupted by a call to pause().');
      playRejection.name = 'AbortError';
      const playSpy = vi.spyOn(HTMLVideoElement.prototype, 'play').mockRejectedValue(playRejection);

      const mockTrack = { kind: 'video', stop: vi.fn() };
      const mockWebcamStream = new globalThis.MediaStream([mockTrack]);
      navigator.mediaDevices.getUserMedia = vi.fn().mockResolvedValue(mockWebcamStream);

      const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      render(CameraStream, {
        props: { deviceId: 'local-webcam', token: mockToken },
      });

      await waitFor(() => {
        expect(playSpy).toHaveBeenCalled();
        expect(warnSpy).toHaveBeenCalledWith(
          expect.stringContaining('Webcam autoplay interrupted:'),
          expect.any(Error)
        );
      });
    });
  });
});
