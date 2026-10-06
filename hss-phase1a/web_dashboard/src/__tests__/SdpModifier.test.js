// @ts-nocheck
import { describe, it, expect } from 'vitest';
import {
  modifySdp,
  modifySdpOffer,
  modifySdpAnswer,
  tuneSdpForLowLatency,
  setVideoBitrates,
  setPacketizationDelay,
  prioritizeH264Profile,
  setMediaBandwidth,
} from '../lib/sdpModifier';

describe('WebRTC SDP Modifier — Unit & Edge Case Suite (M1)', () => {
  // Reference Standard WebRTC SDP Offer
  const standardSdpOffer = [
    'v=0',
    'o=- 4611731400430051336 2 IN IP4 127.0.0.1',
    's=-',
    't=0 0',
    'a=group:BUNDLE 0 1',
    'a=msid-semantic: WMS',
    'm=audio 9 UDP/TLS/RTP/SAVPF 111 103 104',
    'c=IN IP4 0.0.0.0',
    'a=rtpmap:111 opus/48000/2',
    'a=fmtp:111 minptime=10;useinbandfec=1',
    'a=mid:0',
    'm=video 9 UDP/TLS/RTP/SAVPF 96 97 98 100',
    'c=IN IP4 0.0.0.0',
    'a=rtpmap:96 H264/90000',
    'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
    'a=rtpmap:97 rtx/90000',
    'a=fmtp:97 apt=96',
    'a=rtpmap:98 VP8/90000',
    'a=rtpmap:100 VP9/90000',
    'a=mid:1',
    '',
  ].join('\r\n');

  describe('1. Bitrate & Bandwidth Parameter Injection', () => {
    it('injects x-google min, max, start bitrates and cbr=1 into H.264 fmtp line', () => {
      const modified = setVideoBitrates(standardSdpOffer, {
        minBitrate: 1500,
        maxBitrate: 4000,
        startBitrate: 2500,
        cbr: true,
      });

      expect(modified).toContain('x-google-min-bitrate=1500');
      expect(modified).toContain('x-google-max-bitrate=4000');
      expect(modified).toContain('x-google-start-bitrate=2500');
      expect(modified).toContain('cbr=1');
      expect(modified).toMatch(/a=fmtp:96 .*x-google-min-bitrate=1500/);
    });

    it('injects b=AS:4000 bandwidth modifier under video m-line', () => {
      const modified = setMediaBandwidth(standardSdpOffer, 4000);
      expect(modified).toContain('b=AS:4000');

      // Verify b=AS appears in video section after m=video line
      const videoSection = modified.substring(modified.indexOf('m=video'));
      expect(videoSection).toMatch(/^m=video[^\r\n]*\r\n(c=[^\r\n]*\r\n)?b=AS:4000/m);
    });

    it('replaces existing b=AS line without creating duplicates', () => {
      const sdpWithExistingBandwidth = standardSdpOffer.replace(
        'm=video 9 UDP/TLS/RTP/SAVPF 96 97 98 100\r\n',
        'm=video 9 UDP/TLS/RTP/SAVPF 96 97 98 100\r\nb=AS:1000\r\n'
      );

      const modified = setMediaBandwidth(sdpWithExistingBandwidth, 4000);
      const matches = modified.match(/b=AS:\d+/g);
      expect(matches).toHaveLength(1);
      expect(matches[0]).toBe('b=AS:4000');
    });
  });

  describe('2. Packetization Delay & Frame Tuning', () => {
    it('injects a=ptime:10 and a=maxptime:20 into SDP', () => {
      const modified = setPacketizationDelay(standardSdpOffer, 10, 20);
      expect(modified).toContain('a=ptime:10');
      expect(modified).toContain('a=maxptime:20');
    });

    it('updates pre-existing a=ptime and a=maxptime lines', () => {
      const sdpWithPtime = standardSdpOffer + 'a=ptime:20\r\na=maxptime:40\r\n';
      const modified = setPacketizationDelay(sdpWithPtime, 10, 20);

      const ptimeMatches = modified.match(/a=ptime:\d+/g);
      const maxPtimeMatches = modified.match(/a=maxptime:\d+/g);

      expect(ptimeMatches).toEqual(['a=ptime:10']);
      expect(maxPtimeMatches).toEqual(['a=maxptime:20']);
    });
  });

  describe('3. Codec Prioritization & H.264 Profile Mutation', () => {
    it('reorders payload types in m=video line to prioritize H.264 (PT 96) as primary codec', () => {
      // SDP where VP8 (PT 98) is listed first
      const vp8FirstSdp = standardSdpOffer.replace(
        'm=video 9 UDP/TLS/RTP/SAVPF 96 97 98 100',
        'm=video 9 UDP/TLS/RTP/SAVPF 98 100 96 97'
      );

      const modified = prioritizeH264Profile(vp8FirstSdp);
      expect(modified).toMatch(/m=video 9 UDP\/TLS\/RTP\/SAVPF 96 97 98 100/);
    });

    it('enforces Constrained Baseline profile (42e01f) and packetization-mode=1 on H.264', () => {
      const highProfileSdp = standardSdpOffer.replace(
        'profile-level-id=42e01f',
        'profile-level-id=640028;packetization-mode=0'
      );

      const modified = prioritizeH264Profile(highProfileSdp);
      expect(modified).toContain('profile-level-id=42e01f');
      expect(modified).toContain('packetization-mode=1');
      expect(modified).toContain('level-asymmetry-allowed=1');
      expect(modified).not.toContain('profile-level-id=640028');
    });
  });

  describe('4. Full Pipeline: modifySdpOffer and modifySdpAnswer', () => {
    it('applies full low-latency profile transformations on WebRTC offer', () => {
      const modified = modifySdpOffer(standardSdpOffer, {
        minBitrate: 1500,
        maxBitrate: 4000,
        startBitrate: 2500,
        cbr: true,
        bandwidthAS: 4000,
        ptime: 10,
        maxptime: 20,
      });

      expect(modified).toContain('b=AS:4000');
      expect(modified).toContain('x-google-min-bitrate=1500');
      expect(modified).toContain('x-google-max-bitrate=4000');
      expect(modified).toContain('x-google-start-bitrate=2500');
      expect(modified).toContain('cbr=1');
      expect(modified).toContain('a=ptime:10');
      expect(modified).toContain('a=maxptime:20');
      expect(modified.endsWith('\r\n')).toBe(true);
    });

    it('applies matching mutations on WebRTC answer', () => {
      const standardAnswer = [
        'v=0',
        'o=- 888888 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f',
        '',
      ].join('\r\n');

      const modified = modifySdpAnswer(standardAnswer);
      expect(modified).toContain('b=AS:4000');
      expect(modified).toContain('x-google-min-bitrate=1500');
      expect(modified).toContain('cbr=1');
    });
  });

  describe('5. Comprehensive Edge Cases', () => {
    it('Edge Case 1: Audio-Only SDP (No m=video line) modifies audio without crashing or injecting video attributes', () => {
      const audioOnlySdp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=audio 9 UDP/TLS/RTP/SAVPF 111',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:111 opus/48000/2',
        '',
      ].join('\r\n');

      const modified = modifySdpOffer(audioOnlySdp);
      expect(modified).toContain('a=ptime:10');
      expect(modified).not.toContain('b=AS:4000');
      expect(modified).not.toContain('x-google-min-bitrate');
      expect(modified).toContain('m=audio 9 UDP/TLS/RTP/SAVPF 111');
    });

    it('Edge Case 2: Multiple video m-lines (dual-camera / simulcast) modifies every video section', () => {
      const multiVideoSdp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        'a=mid:video1',
        'm=video 10 UDP/TLS/RTP/SAVPF 97',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:97 H264/90000',
        'a=mid:video2',
        '',
      ].join('\r\n');

      const modified = modifySdpOffer(multiVideoSdp);
      const asMatches = modified.match(/b=AS:4000/g);
      expect(asMatches).toHaveLength(2);
      expect(modified).toContain('a=fmtp:96');
      expect(modified).toContain('a=fmtp:97');
    });

    it('Edge Case 3: Missing a=fmtp line synthesizes new fmtp line with all parameters', () => {
      const sdpMissingFmtp = [
        'v=0',
        'o=- 12345 2 IN IP4 127.0.0.1',
        's=-',
        't=0 0',
        'm=video 9 UDP/TLS/RTP/SAVPF 96',
        'c=IN IP4 0.0.0.0',
        'a=rtpmap:96 H264/90000',
        '',
      ].join('\r\n');

      const modified = modifySdpOffer(sdpMissingFmtp);
      expect(modified).toMatch(/a=fmtp:96 [^\r\n]*x-google-min-bitrate=1500/);
      expect(modified).toContain('profile-level-id=42e01f');
      expect(modified).toContain('packetization-mode=1');
      expect(modified).toContain('cbr=1');
    });

    it('Edge Case 4: Handles malformed, empty, or non-string inputs safely without throwing', () => {
      expect(modifySdpOffer('')).toBe('');
      expect(modifySdpOffer(null)).toBe(null);
      expect(modifySdpOffer(undefined)).toBe(undefined);
      expect(modifySdpOffer('v=0')).toContain('v=0');
      expect(modifySdpOffer('invalid-random-text')).toBe('invalid-random-text');
    });

    it('Edge Case 5: Preserves strict RFC 4566 CRLF (\\r\\n) line endings even if input had mixed \\n', () => {
      const lfOnlySdp = standardSdpOffer.replace(/\r\n/g, '\n');
      const modified = modifySdpOffer(lfOnlySdp);

      // Verify no solitary \n without preceding \r
      const bareLf = modified.replace(/\r\n/g, '').includes('\n');
      expect(bareLf).toBe(false);
      expect(modified).toContain('\r\n');
    });

    it('is idempotent when applied repeatedly', () => {
      const firstPass = tuneSdpForLowLatency(standardSdpOffer);
      const secondPass = tuneSdpForLowLatency(firstPass);
      expect(secondPass).toBe(firstPass);
    });
  });
});
