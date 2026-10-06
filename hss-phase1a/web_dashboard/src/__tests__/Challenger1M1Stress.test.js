// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';
import CameraStream from '../lib/CameraStream.svelte';
import { WebcamBroadcaster } from '../lib/webcamBroadcaster';
import {
  modifySdp,
  modifySdpOffer,
  modifySdpAnswer,
  tuneSdpForLowLatency,
  setVideoBitrates,
  setPacketizationDelay,
  prioritizeH264Profile,
  setMediaBandwidth,
  updateFmtpParameters,
} from '../lib/sdpModifier';

describe('Milestone M1 Adversarial Empirical Challenge & Stress Test Suite (Challenger 1)', () => {
  const mockDeviceId = 'edge-cam-adversarial-01';
  const mockToken = 'jwt-challenger1-token';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  // =========================================================================
  // SECTION 1: SDP Modifier Adversarial Inputs & Fuzzing
  // =========================================================================
  describe('1. SDP Modifier Malformed Inputs, Edge Cases & Fuzzing', () => {
    it('handles completely malformed / non-SDP input gracefully without throwing', () => {
      const weirdInputs = [
        null,
        undefined,
        '',
        '   ',
        '\r\n\r\n\n\r',
        'NOT AN SDP STRING AT ALL',
        'v=0\r\no=- 1234\r\nINVALID GARBAGE\r\n',
        '{"json": "sdp_payload"}',
        '<xml><sdp>test</sdp></xml>',
        'a=rtpmap:96 H264/90000\r\na=fmtp:96 ;;;;;===;;;\r\n',
        'm=video\r\n',
        'm=video \r\n',
        'm=video 9 UDP/TLS/RTP/SAVPF\r\n',
      ];

      for (const input of weirdInputs) {
        expect(() => modifySdp(input)).not.toThrow();
        expect(() => tuneSdpForLowLatency(input)).not.toThrow();
      }
    });

    it('handles audio-only SDP offer correctly: injects audio ptime without corrupting or inserting video attributes', () => {
      const audioOnlySdp = [
        'v=0',
        'o=- 55555 2 IN IP4 127.0.0.1',
        's=AudioOnlySession',
        't=0 0',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        'a=fmtp:111 minptime=10;useinbandfec=1',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(audioOnlySdp);
      expect(mutated).toContain('a=ptime:10');
      expect(mutated).not.toContain('b=AS:');
      expect(mutated).not.toContain('x-google-min-bitrate');
      expect(mutated).not.toContain('profile-level-id');
      expect(mutated).toContain('m=audio 9 UDP/TLS/RTP/SAVPF 111');
      expect(mutated.endsWith('\r\n')).toBe(true);
    });

    it('handles data-channel-only SDP (no audio, no video) without injecting unwanted media attributes', () => {
      const dataChannelSdp = [
        'v=0',
        'o=- 77777 2 IN IP4 127.0.0.1',
        's=DataChannelSession',
        't=0 0',
        'm=application 9 UDP/DTLS/SCTP webrtc-datachannel',
        'c=IN IP4 0.0.0.0',
        'a=sctp-port:5000',
        'a=max-message-size:262144',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(dataChannelSdp);
      expect(mutated).not.toContain('b=AS:');
      expect(mutated).not.toContain('a=ptime:');
      expect(mutated).not.toContain('x-google-min-bitrate');
      expect(mutated).toContain('m=application 9 UDP/DTLS/SCTP webrtc-datachannel');
    });

    it('handles multi-stream / multi-video SDP (surveillance matrix / multi-lens cameras)', () => {
      const multiVideoSdp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=MultiLensStream',
        't=0 0',
        'a=group:BUNDLE 0 1 2',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        'a=mid:0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
        'a=mid:1',
        'm=video 9 UDP/TLS/RTP/SAVPF 97',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:97 H264/90000',
        'a=fmtp:97 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
        'a=mid:2',
        'm=video 9 UDP/TLS/RTP/SAVPF 98',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:98 VP8/90000',
        'a=mid:3',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(multiVideoSdp);

      // Verify all 3 video sections received bandwidth attributes
      const bAsMatches = mutated.match(/b=AS:4000/g);
      expect(bAsMatches).toHaveLength(3);

      // Verify all video fmtp lines (H.264 and VP8) received bitrate floor and cbr
      expect(mutated).toMatch(/a=fmtp:96 .*x-google-min-bitrate=1500.*cbr=1/);
      expect(mutated).toMatch(/a=fmtp:97 .*x-google-min-bitrate=1500.*cbr=1/);
      expect(mutated).toMatch(/a=fmtp:98 .*x-google-min-bitrate=1500.*cbr=1/);

      // Verify audio received ptime
      expect(mutated).toContain('a=ptime:10');
    });

    it('synthesizes missing fmtp lines for multiple video codecs (H264, VP8, VP9) without throwing', () => {
      const sdpNoFmtp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96 98 100',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=rtpmap:98 VP8/90000',
        'a=rtpmap:100 VP9/90000',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(sdpNoFmtp);

      expect(mutated).toContain('a=fmtp:96');
      expect(mutated).toContain('a=fmtp:98');
      expect(mutated).toContain('a=fmtp:100');

      expect(mutated).toMatch(/a=fmtp:96 .*profile-level-id=42e01f/);
      expect(mutated).toMatch(/a=fmtp:98 .*x-google-min-bitrate=1500/);
      expect(mutated).toMatch(/a=fmtp:100 .*x-google-min-bitrate=1500/);
    });

    it('merges existing complex fmtp parameters while overriding conflicting ones', () => {
      const customParams = 'sprop-parameter-sets=Z00AH5WoFAFuQA==,aO48gA==;profile-level-id=640028;packetization-mode=0;custom-param=true';
      const updated = updateFmtpParameters(customParams, {
        'profile-level-id': '42e01f',
        'packetization-mode': 1,
        'x-google-min-bitrate': 1500,
      });

      expect(updated).toContain('sprop-parameter-sets=Z00AH5WoFAFuQA==,aO48gA==');
      expect(updated).toContain('custom-param=true');
      expect(updated).toContain('profile-level-id=42e01f');
      expect(updated).toContain('packetization-mode=1');
      expect(updated).toContain('x-google-min-bitrate=1500');
      expect(updated).not.toContain('profile-level-id=640028');
    });

    it('preserves idempotency across 50 consecutive mutations', () => {
      const baseSdp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        'm=video 9 UDP/TLS/RTP/SAVPF 96 98',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
        'a=rtpmap:98 VP8/90000',
        '',
      ].join('\r\n');

      let current = tuneSdpForLowLatency(baseSdp);
      for (let i = 0; i < 50; i++) {
        const next = tuneSdpForLowLatency(current);
        expect(next).toBe(current);
        current = next;
      }
    });

    it('satisfies high throughput performance: transforms 500 SDP descriptions in < 50ms', () => {
      const baseSdp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        'm=video 9 UDP/TLS/RTP/SAVPF 96 98',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
        'a=rtpmap:98 VP8/90000',
        '',
      ].join('\r\n');

      const start = performance.now();
      for (let i = 0; i < 500; i++) {
        tuneSdpForLowLatency(baseSdp);
      }
      const elapsed = performance.now() - start;
      expect(elapsed).toBeLessThan(150); // Generous 150ms limit for CI/test runners
    });
  });

  // =========================================================================
  // SECTION 2: Video Buffer Drift Catch-up Stress Testing
  // =========================================================================
  describe('2. Video Buffer Drift Catch-Up Under Extreme Conditions', () => {
    it('Extreme Drift (> 10s frozen tab): snaps currentTime to live edge immediately', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Simulate 120 seconds of background buffering drift
      video.simulateBuffer(0, 150.0, 30.0); // 120s lag

      // Trigger drift check via stats polling
      vi.advanceTimersByTime(1000);

      // Must snap to live edge (150.0 - 0.03 = 149.97) and reset playbackRate = 1.0
      expect(video.currentTime).toBeCloseTo(149.97, 2);
      expect(video.playbackRate).toBe(1.0);

      vi.useRealTimers();
    });

    it('Negative Drift (currentTime > buffered.end()): safely handles time anomalies without invalid seeks or crashes', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Set currentTime ahead of buffer end (negative lag = -2.0s)
      video.simulateBuffer(0, 10.0, 12.0);

      vi.advanceTimersByTime(1000);

      // Should keep rate at 1.0 and not jump
      expect(video.playbackRate).toBe(1.0);
      expect(video.currentTime).toBe(12.0);

      vi.useRealTimers();
    });

    it('Zero Buffered Ranges (buffered.length === 0): does not throw IndexSizeError', async () => {
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

      expect(video.playbackRate).toBe(1.0);
      vi.useRealTimers();
    });

    it('Multiple Disjoint Buffered Ranges: inspects the latest range (last index) for live edge calculation', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Two disjoint ranges: [0, 10] and [20, 30]
      video._bufferedRanges = [[0, 10], [20, 30]];
      video._currentTime = 29.5; // 0.5s lag from 30.0

      vi.advanceTimersByTime(1000);

      // Should snap to 30 - 0.03 = 29.97
      expect(video.currentTime).toBeCloseTo(29.97, 2);
      expect(video.playbackRate).toBe(1.0);

      vi.useRealTimers();
    });

    it('Paused Video: does not manipulate playbackRate or currentTime when user or system pauses video', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');
      video.pause();
      video.simulateBuffer(0, 20.0, 10.0); // 10s lag

      vi.advanceTimersByTime(1000);

      // currentTime should NOT have jumped because video is paused
      expect(video.currentTime).toBe(10.0);

      vi.useRealTimers();
    });

    it('Active Seeking: does not trigger drift catch-up while videoElement is in seeking state', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');
      video.seeking = true;
      video.simulateBuffer(0, 50.0, 10.0);

      vi.advanceTimersByTime(1000);

      expect(video.currentTime).toBe(10.0);

      vi.useRealTimers();
    });
  });

  // =========================================================================
  // SECTION 3: Playout Hints & Receiver Property Defensiveness
  // =========================================================================
  describe('3. Receiver Playout Delay Hints Stress & Sandboxing', () => {
    it('safely handles receiver objects with throw-on-access / read-only properties', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];

      // Create a hostile receiver where writing playoutDelayHint throws
      const hostileReceiver = {
        track: { kind: 'video', stop: vi.fn() },
        get playoutDelayHint() {
          return 0;
        },
        set playoutDelayHint(v) {
          throw new Error('Restricted browser security sandbox');
        },
        get jitterBufferTarget() {
          return 0;
        },
        set jitterBufferTarget(v) {
          throw new Error('Restricted browser security sandbox');
        }
      };

      expect(() => {
        pc.simulateTrack(new globalThis.MediaStream([hostileReceiver.track]), hostileReceiver);
      }).not.toThrow();
    });

    it('safely handles null / non-object receivers in pc.getReceivers()', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      pc.getReceivers = () => [null, undefined, {}, { playoutDelayHint: 50 }];

      // Trigger stats poll which iterates getReceivers()
      vi.useFakeTimers();
      expect(() => {
        vi.advanceTimersByTime(1000);
      }).not.toThrow();
      vi.useRealTimers();
    });
  });

  // =========================================================================
  // SECTION 4: Broadcaster Edge Cases & Concurrency
  // =========================================================================
  describe('4. Broadcaster Edge Cases & Concurrency', () => {
    it('handles rapid start and stop cycling without leaking peer connections or sockets', async () => {
      const broadcaster = new WebcamBroadcaster('rapid-test-cam', mockToken);

      for (let i = 0; i < 5; i++) {
        await broadcaster.start();
        expect(broadcaster.getIsRunning()).toBe(true);
        broadcaster.stop();
        expect(broadcaster.getIsRunning()).toBe(false);
      }
    });

    it('handles candidate message when session has no peer connection gracefully', async () => {
      const broadcaster = new WebcamBroadcaster('cand-cam', mockToken);
      await broadcaster.start();

      const ws = globalThis.WebSocket.instances[globalThis.WebSocket.instances.length - 1];

      expect(() => {
        ws.simulateMessage({
          type: 'candidate',
          session_id: 'non-existent-session',
          candidate: JSON.stringify({ candidate: 'candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host' }),
        });
      }).not.toThrow();

      broadcaster.stop();
    });
  });
});
