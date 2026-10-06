// @ts-nocheck
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
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

describe('Challenger 1: Frontend Playout, SDP Modifiers & Signaling Adversarial Test Suite', () => {
  const mockDeviceId = 'cam-stream-challenger-01';
  const mockToken = 'jwt-token-challenger-adversarial';

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.RTCPeerConnection.instances = [];
    globalThis.WebSocket.instances = [];
  });

  // =========================================================================
  // CHALLENGE AREA 1: SDP MUTATOR EDGE CASES & FUZZING
  // =========================================================================
  describe('1. SDP Mutator Edge Cases & Fuzzing', () => {
    const baseSdpOffer = [
      'v=0',
      'o=- 1234567890 2 IN IP4 127.0.0.1',
      's=-',
      't=0 0',
      'a=group:BUNDLE 0 1',
      'm=audio 9 UDP/TLS/RTP/SAVPF 111',
      'c=IN IP4 0.0.0.0',
      'a=rtpmap:111 opus/48000/2',
      'a=fmtp:111 minptime=10;useinbandfec=1',
      'a=mid:0',
      'm=video 9 UDP/TLS/RTP/SAVPF 96 97 98',
      'c=IN IP4 0.0.0.0',
      'a=rtpmap:96 H264/90000',
      'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
      'a=rtpmap:97 rtx/90000',
      'a=fmtp:97 apt=96',
      'a=rtpmap:98 VP8/90000',
      'a=mid:1',
      '',
    ].join('\r\n');

    it('Edge Case 1.1: Malformed SDP lines (empty lines, invalid verbs, missing tokens, non-SDP text)', () => {
      const hostileSdpInputs = [
        '',
        '   ',
        '\r\n\r\n\r\n',
        'm=',
        'm=video',
        'm=video ',
        'm=video 9',
        'm=video 9 UDP/TLS/RTP/SAVPF',
        'a=rtpmap:',
        'a=rtpmap:96',
        'a=fmtp:',
        'a=fmtp:96',
        'b=',
        'b=AS:',
        'c=',
        'c=IN IP4',
        'NOT AN SDP AT ALL: 1234567890!@#$%^&*()',
        '{"type": "offer", "sdp": "bogus"}',
        '<sdp><media>video</media></sdp>',
        'v=0\r\nINVALID\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nGARBAGE\r\n',
      ];

      for (const input of hostileSdpInputs) {
        expect(() => modifySdp(input)).not.toThrow();
        expect(() => tuneSdpForLowLatency(input)).not.toThrow();
        expect(() => modifySdpOffer(input)).not.toThrow();
        expect(() => modifySdpAnswer(input)).not.toThrow();
      }
    });

    it('Edge Case 1.2: Missing a=fmtp for video codecs (synthesizes compliant fmtp for H.264 and VP8)', () => {
      const sdpMissingFmtp = [
        'v=0',
        'o=- 11111 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96 98',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=rtpmap:98 VP8/90000',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(sdpMissingFmtp);

      // Verify fmtp:96 synthesized with H.264 profile, packetization-mode, level-asymmetry, and bitrates
      expect(mutated).toMatch(/a=fmtp:96 [^\r\n]*x-google-min-bitrate=1500/);
      expect(mutated).toMatch(/a=fmtp:96 [^\r\n]*profile-level-id=42e01f/);
      expect(mutated).toMatch(/a=fmtp:96 [^\r\n]*packetization-mode=1/);
      expect(mutated).toMatch(/a=fmtp:96 [^\r\n]*level-asymmetry-allowed=1/);
      expect(mutated).toMatch(/a=fmtp:96 [^\r\n]*cbr=1/);

      // Verify fmtp:98 synthesized for VP8 with bitrates
      expect(mutated).toMatch(/a=fmtp:98 [^\r\n]*x-google-min-bitrate=1500/);
      expect(mutated).toMatch(/a=fmtp:98 [^\r\n]*cbr=1/);
    });

    it('Edge Case 1.3: Conflicting bitrate limits and extreme custom options', () => {
      // User specifies custom options where minBitrate > maxBitrate or bandwidthAS is 0
      const mutatedCustom = modifySdp(baseSdpOffer, {
        minBitrate: 3000,
        maxBitrate: 6000,
        startBitrate: 4500,
        cbr: false,
        bandwidthAS: 6000,
        ptime: 5,
        maxptime: 10,
        profileLevelId: '42001f',
        packetizationMode: 0,
        levelAsymmetryAllowed: false,
      });

      expect(mutatedCustom).toContain('x-google-min-bitrate=3000');
      expect(mutatedCustom).toContain('x-google-max-bitrate=6000');
      expect(mutatedCustom).toContain('x-google-start-bitrate=4500');
      expect(mutatedCustom).toContain('cbr=0');
      expect(mutatedCustom).toContain('b=AS:6000');
      expect(mutatedCustom).toContain('a=ptime:5');
      expect(mutatedCustom).toContain('a=maxptime:10');
      expect(mutatedCustom).toContain('profile-level-id=42001f');
      expect(mutatedCustom).toContain('packetization-mode=0');
      expect(mutatedCustom).toContain('level-asymmetry-allowed=0');
    });

    it('Edge Case 1.4: Multiple video m-lines (multi-camera / simulcast matrix)', () => {
      const multiVideoSdp = [
        'v=0',
        'o=- 22222 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'a=group:BUNDLE 0 1 2',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        'a=mid:0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=mid:1',
        'm=video 10 UDP/TLS/RTP/SAVPF 97',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:97 H264/90000',
        'a=mid:2',
        'm=video 11 UDP/TLS/RTP/SAVPF 98',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:98 VP8/90000',
        'a=mid:3',
        '',
      ].join('\r\n');

      const mutated = tuneSdpForLowLatency(multiVideoSdp);

      // Verify all 3 video sections receive b=AS:4000
      const bAsMatches = mutated.match(/b=AS:4000/g);
      expect(bAsMatches).toHaveLength(3);

      // Verify all 3 video fmtp lines exist and have bitrates
      expect(mutated).toContain('a=fmtp:96');
      expect(mutated).toContain('a=fmtp:97');
      expect(mutated).toContain('a=fmtp:98');
      expect(mutated).toContain('a=ptime:10');
    });

    it('Edge Case 1.5: Audio-Only and DataChannel-Only streams without video', () => {
      const audioOnlySdp = [
        'v=0',
        'o=- 33333 2 IN IP4 127.0.0.1',
        's=AudioOnly',
        't=0 0',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        '',
      ].join('\r\n');

      const mutatedAudio = tuneSdpForLowLatency(audioOnlySdp);
      expect(mutatedAudio).toContain('a=ptime:10');
      expect(mutatedAudio).not.toContain('b=AS:');
      expect(mutatedAudio).not.toContain('x-google-min-bitrate');
      expect(mutatedAudio).not.toContain('profile-level-id');

      const dataChannelSdp = [
        'v=0',
        'o=- 44444 2 IN IP4 127.0.0.1',
        's=DataOnly',
        't=0 0',
        'm=application 9 UDP/DTLS/SCTP webrtc-datachannel',
        'c=IN IP4 0.0.0.0',
        'a=sctp-port:5000',
        '',
      ].join('\r\n');

      const mutatedData = tuneSdpForLowLatency(dataChannelSdp);
      expect(mutatedData).not.toContain('b=AS:');
      expect(mutatedData).not.toContain('a=ptime:');
      expect(mutatedData).not.toContain('x-google-min-bitrate');
      expect(mutatedData).toContain('m=application 9 UDP/DTLS/SCTP webrtc-datachannel');
    });

    it('Edge Case 1.6: Codec reordering and H.264 prioritization with multiple H.264 payload types', () => {
      // SDP offer where VP8 (98) and VP9 (100) are listed before H.264 (96, 102)
      const mixedCodecSdp = [
        'v=0',
        'o=- 55555 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 98 100 96 97 102 103',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:98 VP8/90000',
        'a=rtpmap:100 VP9/90000',
        'a=rtpmap:96 H264/90000',
        'a=fmtp:96 profile-level-id=42e01f;packetization-mode=1',
        'a=rtpmap:97 rtx/90000',
        'a=fmtp:97 apt=96',
        'a=rtpmap:102 H264/90000',
        'a=fmtp:102 profile-level-id=42e01f;packetization-mode=0',
        'a=rtpmap:103 rtx/90000',
        'a=fmtp:103 apt=102',
        '',
      ].join('\r\n');

      const mutated = prioritizeH264Profile(mixedCodecSdp);

      // Verify m=video line starts with H.264 payload types (96, 102) and their RTX (97, 103)
      const videoMLine = mutated.split('\r\n').find((l) => l.startsWith('m=video '));
      expect(videoMLine).toBeDefined();

      const pts = videoMLine.split(' ').slice(3);
      // H264 codecs (96, 102) and RTX (97, 103) must come before VP8/VP9 (98, 100)
      expect(pts.indexOf('96')).toBeLessThan(pts.indexOf('98'));
      expect(pts.indexOf('102')).toBeLessThan(pts.indexOf('98'));
      expect(pts.indexOf('96')).toBeLessThan(pts.indexOf('100'));
      expect(pts.indexOf('102')).toBeLessThan(pts.indexOf('100'));
    });

    it('Edge Case 1.7: Idempotency under 100 consecutive mutation cycles', () => {
      let current = baseSdpOffer;
      const firstPass = tuneSdpForLowLatency(current);
      current = firstPass;

      for (let i = 0; i < 100; i++) {
        const nextPass = tuneSdpForLowLatency(current);
        expect(nextPass).toBe(firstPass);
        current = nextPass;
      }
    });
  });

  // =========================================================================
  // CHALLENGE AREA 2: BUFFER DRIFT CATCH-UP UNDER HIGH JITTER & TIME JUMPS
  // =========================================================================
  describe('2. Buffer Drift Catch-up Under High Jitter & Time Jumps', () => {
    it('Drift Scenario 2.1: Severe Time Jump (> 10s tab backgrounding) triggers instant hard catch-up jump', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Simulate 30 seconds of buffered drift
      video.simulateBuffer(0, 45.0, 15.0); // 30s buffer lag

      // Trigger stats/drift polling
      vi.advanceTimersByTime(1000);

      // Must snap to liveEdge - 0.03 = 44.97
      expect(video.currentTime).toBeCloseTo(44.97, 2);
      expect(video.playbackRate).toBe(1.0);

      vi.useRealTimers();
    });

    it('Drift Scenario 2.2: High Jitter Oscillations (rapidly transitioning across 50ms, 120ms, 250ms thresholds)', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Step A: In Sync (40ms lag)
      video.simulateBuffer(0, 10.04, 10.00);
      vi.advanceTimersByTime(1000);
      expect(video.playbackRate).toBe(1.0);
      expect(video.currentTime).toBe(10.00);

      // Step B: Moderate Jitter Spike (150ms lag) -> Speed-up to 1.1x
      video.simulateBuffer(0, 10.15, 10.00);
      vi.advanceTimersByTime(1000);
      expect(video.playbackRate).toBe(1.1);
      expect(video.currentTime).toBe(10.00);

      // Step C: Severe Jitter Spike (350ms lag) -> Hard jump to liveEdge - 0.03
      video.simulateBuffer(0, 10.35, 10.00);
      vi.advanceTimersByTime(1000);
      expect(video.currentTime).toBeCloseTo(10.32, 2);
      expect(video.playbackRate).toBe(1.0);

      // Step D: Recovery (20ms lag) -> Remain at 1.0x
      video.simulateBuffer(0, 10.34, 10.32);
      vi.advanceTimersByTime(1000);
      expect(video.playbackRate).toBe(1.0);

      vi.useRealTimers();
    });

    it('Drift Scenario 2.3: Negative Drift Anomaly (currentTime > liveEdge) handles safely without seek or panic', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Set currentTime ahead of buffer end (negative lag = -1.5s)
      video.simulateBuffer(0, 10.0, 11.5);
      vi.advanceTimersByTime(1000);

      // Must remain at rate 1.0 and not jump backward
      expect(video.playbackRate).toBe(1.0);
      expect(video.currentTime).toBe(11.5);

      vi.useRealTimers();
    });

    it('Drift Scenario 2.4: Multiple disjoint TimeRanges (packet loss / recovery gap)', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Multiple disjoint ranges: [0..5] and [10..20]
      video._bufferedRanges = [[0, 5], [10, 20]];
      video._currentTime = 19.4; // 600ms lag from liveEdge 20.0

      vi.advanceTimersByTime(1000);

      // Must inspect the last range ([10, 20]) and snap to 20.0 - 0.03 = 19.97
      expect(video.currentTime).toBeCloseTo(19.97, 2);
      expect(video.playbackRate).toBe(1.0);

      vi.useRealTimers();
    });

    it('Drift Scenario 2.5: Invariants (paused video, seeking video, disconnected state)', async () => {
      vi.useFakeTimers();
      const { container } = render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      vi.advanceTimersByTime(10);
      const pc = globalThis.RTCPeerConnection.instances[0];
      const mockTrack = { kind: 'video', stop: vi.fn() };
      pc.simulateTrack(new globalThis.MediaStream([mockTrack]), mockTrack);

      const video = container.querySelector('video');

      // Case A: Paused video
      video.pause();
      video.simulateBuffer(0, 50.0, 10.0);
      vi.advanceTimersByTime(1000);
      expect(video.currentTime).toBe(10.0);

      // Unpause
      video.play();

      // Case B: Seeking video
      video.seeking = true;
      video.simulateBuffer(0, 80.0, 10.0);
      vi.advanceTimersByTime(1000);
      expect(video.currentTime).toBe(10.0);
      video.seeking = false;

      // Case C: Disconnected state
      pc.connectionState = 'disconnected';
      if (pc.onconnectionstatechange) pc.onconnectionstatechange();
      video.simulateBuffer(0, 100.0, 10.0);
      vi.advanceTimersByTime(1000);
      expect(video.currentTime).toBe(10.0);

      vi.useRealTimers();
    });
  });

  // =========================================================================
  // CHALLENGE AREA 3: SIGNALING & PEER CONNECTION ROBUSTNESS
  // =========================================================================
  describe('3. Signaling Protocol & Candidate Management', () => {
    it('Signaling Scenario 3.1: Handles candidate flood before remote description arrival (early candidate queuing)', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const ws = globalThis.WebSocket.instances[0];
      const pc = globalThis.RTCPeerConnection.instances[0];
      const addIceSpy = vi.spyOn(pc, 'addIceCandidate');

      // Flood 20 candidates BEFORE answer / remoteDescription arrives
      for (let i = 0; i < 20; i++) {
        ws.simulateMessage({
          type: 'candidate',
          candidate: JSON.stringify({
            candidate: `candidate:${i} 1 UDP 2130706431 127.0.0.1 ${50000 + i} typ host`,
            sdpMid: '1',
            sdpMLineIndex: 1,
          }),
        });
      }

      // Candidates should be queued in earlyCandidates, not added yet
      expect(addIceSpy).not.toHaveBeenCalled();

      // Now send answer
      ws.simulateMessage({
        type: 'answer',
        sdp: 'v=0\r\ns=-\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n',
      });

      await waitFor(() => {
        // All 20 queued candidates must have been drained and added
        expect(addIceSpy).toHaveBeenCalledTimes(20);
      });
    });

    it('Signaling Scenario 3.2: Handles malformed candidate payloads without throwing or breaking signaling loop', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.WebSocket.instances.length).toBeGreaterThan(0);
      });

      const ws = globalThis.WebSocket.instances[0];

      const malformedCandidates = [
        null,
        '',
        '{}',
        '{"invalid": true}',
        'not a json string at all',
        12345,
        { candidate: '' },
      ];

      for (const badCand of malformedCandidates) {
        expect(() => {
          ws.simulateMessage({
            type: 'candidate',
            candidate: badCand,
          });
        }).not.toThrow();
      }
    });

    it('Signaling Scenario 3.3: End-of-candidates notification (null candidate) handled cleanly', async () => {
      render(CameraStream, {
        props: { deviceId: mockDeviceId, token: mockToken },
      });

      await waitFor(() => {
        expect(globalThis.RTCPeerConnection.instances.length).toBeGreaterThan(0);
      });

      const pc = globalThis.RTCPeerConnection.instances[0];
      const ws = globalThis.WebSocket.instances[0];

      // Simulate local ICE gathering completion
      pc.onicecandidate({ candidate: null });

      const nullMsg = ws.sentMessages
        .map((m) => JSON.parse(m))
        .find((m) => m.type === 'candidate' && m.candidate === null);

      expect(nullMsg).toBeDefined();
      expect(nullMsg.session_id).toMatch(/^dash-session-/);
    });
  });
});
