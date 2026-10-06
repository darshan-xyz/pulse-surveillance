/**
 * WebRTC SDP Mutator for Ultra-Low Latency Video Playout
 * Injects bitrate floors, bandwidth constraints, low-latency audio ptime,
 * and H.264 Constrained Baseline level-asymmetry parameters into RFC 4566 SDP descriptions.
 */

export interface SdpModifierOptions {
  minBitrateKbps?: number;     // x-google-min-bitrate (default: 1500)
  minBitrate?: number;
  maxBitrateKbps?: number;     // x-google-max-bitrate (default: 4000)
  maxBitrate?: number;
  startBitrateKbps?: number;   // x-google-start-bitrate (default: 2500)
  startBitrate?: number;
  cbr?: boolean | number;       // cbr=1 (default: 1)
  bandwidthKbps?: number;      // b=AS:<kbps> (default: 4000)
  bandwidthAS?: number;
  ptimeMs?: number;            // a=ptime:<ms> (default: 10)
  ptime?: number;
  maxptime?: number;           // a=maxptime:<ms> (default: 20)
  profileLevelId?: string;     // H.264 profile-level-id (default: '42e01f')
  levelAsymmetryAllowed?: boolean | number; // level-asymmetry-allowed (default: 1)
  packetizationMode?: number;  // packetization-mode (default: 1)
}

const DEFAULT_SDP_OPTIONS = {
  minBitrateKbps: 1500,
  maxBitrateKbps: 4000,
  startBitrateKbps: 2500,
  cbr: 1,
  bandwidthKbps: 4000,
  ptimeMs: 10,
  maxptime: 20,
  profileLevelId: '42e01f',
  levelAsymmetryAllowed: 1,
  packetizationMode: 1,
};

/**
 * Parses and updates semicolon-delimited fmtp parameters with key-value pairs.
 */
export function updateFmtpParameters(
  existingParams: string,
  newParams: Record<string, string | number>
): string {
  const paramMap = new Map<string, string>();

  if (existingParams && existingParams.trim().length > 0) {
    const tokens = existingParams.split(';');
    for (const token of tokens) {
      const trimmed = token.trim();
      if (!trimmed) continue;
      const eqIdx = trimmed.indexOf('=');
      if (eqIdx !== -1) {
        paramMap.set(trimmed.substring(0, eqIdx).trim(), trimmed.substring(eqIdx + 1).trim());
      } else {
        paramMap.set(trimmed, '');
      }
    }
  }

  for (const [key, value] of Object.entries(newParams)) {
    paramMap.set(key, String(value));
  }

  const entries: string[] = [];
  for (const [key, value] of paramMap.entries()) {
    if (value !== '') {
      entries.push(`${key}=${value}`);
    } else {
      entries.push(key);
    }
  }

  return entries.join(';');
}

/**
 * Normalizes options by resolving aliases.
 */
function normalizeOptions(userOptions?: SdpModifierOptions) {
  const minBitrate = userOptions?.minBitrate ?? userOptions?.minBitrateKbps ?? DEFAULT_SDP_OPTIONS.minBitrateKbps;
  const maxBitrate = userOptions?.maxBitrate ?? userOptions?.maxBitrateKbps ?? DEFAULT_SDP_OPTIONS.maxBitrateKbps;
  const startBitrate = userOptions?.startBitrate ?? userOptions?.startBitrateKbps ?? DEFAULT_SDP_OPTIONS.startBitrateKbps;
  const cbr = userOptions?.cbr === false ? 0 : 1;
  const bandwidthAS = userOptions?.bandwidthAS ?? userOptions?.bandwidthKbps ?? DEFAULT_SDP_OPTIONS.bandwidthKbps;
  const ptime = userOptions?.ptime ?? userOptions?.ptimeMs ?? DEFAULT_SDP_OPTIONS.ptimeMs;
  const maxptime = userOptions?.maxptime ?? DEFAULT_SDP_OPTIONS.maxptime;
  const profileLevelId = userOptions?.profileLevelId ?? DEFAULT_SDP_OPTIONS.profileLevelId;
  const levelAsymmetryAllowed = userOptions?.levelAsymmetryAllowed === false ? 0 : 1;
  const packetizationMode = userOptions?.packetizationMode ?? DEFAULT_SDP_OPTIONS.packetizationMode;

  return {
    minBitrate,
    maxBitrate,
    startBitrate,
    cbr,
    bandwidthAS,
    ptime,
    maxptime,
    profileLevelId,
    levelAsymmetryAllowed,
    packetizationMode,
  };
}

/**
 * Injects video bitrates into H.264 and video fmtp lines.
 */
export function setVideoBitrates(
  sdp: string,
  options: { minBitrate?: number; maxBitrate?: number; startBitrate?: number; cbr?: boolean | number }
): string {
  if (!sdp || typeof sdp !== 'string') return sdp;
  return modifySdp(sdp, {
    minBitrate: options.minBitrate,
    maxBitrate: options.maxBitrate,
    startBitrate: options.startBitrate,
    cbr: options.cbr,
    bandwidthAS: 0,
  });
}

/**
 * Injects or updates bandwidth modifier under video media sections.
 */
export function setMediaBandwidth(sdp: string, bandwidthAS: number = 4000): string {
  if (!sdp || typeof sdp !== 'string') return sdp;
  return modifySdp(sdp, { bandwidthAS });
}

/**
 * Injects or updates packetization delay attributes (a=ptime, a=maxptime).
 */
export function setPacketizationDelay(sdp: string, ptime: number = 10, maxptime: number = 20): string {
  if (!sdp || typeof sdp !== 'string') return sdp;
  return modifySdp(sdp, { ptime, maxptime, bandwidthAS: 0 });
}

/**
 * Prioritizes H.264 in video m-line and enforces Constrained Baseline profile.
 */
export function prioritizeH264Profile(sdp: string, profileLevelId: string = '42e01f'): string {
  if (!sdp || typeof sdp !== 'string') return sdp;
  return modifySdp(sdp, { profileLevelId, bandwidthAS: 0 });
}

/**
 * Modifies an SDP string to tune video and audio parameters for sub-200ms latency.
 */
export function modifySdp(sdp: string, userOptions?: SdpModifierOptions): string {
  if (!sdp || typeof sdp !== 'string') {
    return sdp;
  }

  const opts = normalizeOptions(userOptions);
  const isExplicitBandwidthZero = userOptions && (userOptions.bandwidthAS === 0 || userOptions.bandwidthKbps === 0);

  // Split lines
  const lines = sdp.split(/\r\n|\n/);
  const resultLines: string[] = [];

  let currentMedia: 'session' | 'video' | 'audio' | 'application' = 'session';
  let videoBandwidthInserted = false;
  let audioPtimeInserted = false;
  let audioMaxptimeInserted = false;

  const h264PayloadTypes: string[] = [];
  const rtxForH264Types: string[] = [];
  const videoPayloadTypes: string[] = [];

  // Pass 1: Parse codecs
  for (const line of lines) {
    if (line.startsWith('m=video ')) {
      const parts = line.split(' ');
      for (let i = 3; i < parts.length; i++) {
        if (parts[i]) videoPayloadTypes.push(parts[i]);
      }
    } else if (line.startsWith('a=rtpmap:')) {
      const match = line.match(/^a=rtpmap:(\d+)\s+([A-Za-z0-9\-_]+)\//);
      if (match) {
        const pt = match[1];
        const codec = match[2].toUpperCase();
        if (codec === 'H264') {
          h264PayloadTypes.push(pt);
        }
      }
    } else if (line.startsWith('a=fmtp:')) {
      const match = line.match(/^a=fmtp:(\d+)\s+apt=(\d+)/);
      if (match) {
        const rtxPt = match[1];
        const aptPt = match[2];
        if (h264PayloadTypes.includes(aptPt)) {
          rtxForH264Types.push(rtxPt);
        }
      }
    }
  }

  // Pass 2: Reconstruct and mutate SDP
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    if (line.startsWith('m=video ')) {
      currentMedia = 'video';
      videoBandwidthInserted = false;

      // Reorder payload types to prioritize H.264 if present
      if (h264PayloadTypes.length > 0) {
        const parts = line.split(' ');
        const prefix = parts.slice(0, 3);
        const pts = parts.slice(3).filter(Boolean);

        const prioritizedPts: string[] = [];
        for (const pt of h264PayloadTypes) {
          if (pts.includes(pt) && !prioritizedPts.includes(pt)) {
            prioritizedPts.push(pt);
          }
        }
        for (const pt of rtxForH264Types) {
          if (pts.includes(pt) && !prioritizedPts.includes(pt)) {
            prioritizedPts.push(pt);
          }
        }
        for (const pt of pts) {
          if (!prioritizedPts.includes(pt)) {
            prioritizedPts.push(pt);
          }
        }

        resultLines.push([...prefix, ...prioritizedPts].join(' '));
      } else {
        resultLines.push(line);
      }
      continue;
    } else if (line.startsWith('m=audio ')) {
      currentMedia = 'audio';
      audioPtimeInserted = false;
      audioMaxptimeInserted = false;
      resultLines.push(line);
      continue;
    } else if (line.startsWith('m=')) {
      currentMedia = 'application';
      resultLines.push(line);
      continue;
    }

    // Global check for a=ptime and a=maxptime anywhere in SDP
    if (line.startsWith('a=ptime:')) {
      if (opts.ptime > 0) {
        resultLines.push(`a=ptime:${opts.ptime}`);
        audioPtimeInserted = true;
      }
      continue;
    }
    if (line.startsWith('a=maxptime:')) {
      if (opts.maxptime > 0) {
        resultLines.push(`a=maxptime:${opts.maxptime}`);
        audioMaxptimeInserted = true;
      }
      continue;
    }

    if (currentMedia === 'video') {
      // Handle existing bandwidth lines
      if (line.startsWith('b=AS:') || line.startsWith('b=TIAS:')) {
        if (!isExplicitBandwidthZero && opts.bandwidthAS > 0 && !videoBandwidthInserted) {
          resultLines.push(`b=AS:${opts.bandwidthAS}`);
          videoBandwidthInserted = true;
        }
        continue;
      }

      // If we encounter connection line c=
      if (line.startsWith('c=')) {
        resultLines.push(line);
        if (!isExplicitBandwidthZero && opts.bandwidthAS > 0 && !videoBandwidthInserted) {
          resultLines.push(`b=AS:${opts.bandwidthAS}`);
          videoBandwidthInserted = true;
        }
        continue;
      }

      // If we encounter an attribute line before b=AS was inserted
      if (!videoBandwidthInserted && !isExplicitBandwidthZero && opts.bandwidthAS > 0 && line.startsWith('a=')) {
        resultLines.push(`b=AS:${opts.bandwidthAS}`);
        videoBandwidthInserted = true;
      }

      // Handle rtpmap lines: inject fmtp if missing
      if (line.startsWith('a=rtpmap:')) {
        resultLines.push(line);
        const rtpmapMatch = line.match(/^a=rtpmap:(\d+)\s+([A-Za-z0-9\-_]+)\//);
        if (rtpmapMatch) {
          const pt = rtpmapMatch[1];
          const codec = rtpmapMatch[2].toUpperCase();
          const hasExistingFmtp = lines.some(l => l.startsWith(`a=fmtp:${pt}`));
          if (!hasExistingFmtp && (codec === 'H264' || codec === 'VP8' || codec === 'VP9')) {
            const paramsToInject: Record<string, string | number> = {
              'x-google-min-bitrate': opts.minBitrate,
              'x-google-max-bitrate': opts.maxBitrate,
              'x-google-start-bitrate': opts.startBitrate,
              'cbr': opts.cbr ? 1 : 0,
            };
            if (codec === 'H264') {
              paramsToInject['profile-level-id'] = opts.profileLevelId;
              paramsToInject['packetization-mode'] = opts.packetizationMode;
              paramsToInject['level-asymmetry-allowed'] = opts.levelAsymmetryAllowed ? 1 : 0;
            }
            const fmtpStr = updateFmtpParameters('', paramsToInject);
            resultLines.push(`a=fmtp:${pt} ${fmtpStr}`);
          }
        }
        continue;
      }

      // Handle existing fmtp attributes
      if (line.startsWith('a=fmtp:')) {
        const match = line.match(/^a=fmtp:(\d+)\s*(.*)$/);
        if (match) {
          const pt = match[1];
          const existingParams = match[2] || '';
          const isH264 = h264PayloadTypes.includes(pt) || (videoPayloadTypes.includes(pt) && h264PayloadTypes.length === 0);

          const paramsToInject: Record<string, string | number> = {
            'x-google-min-bitrate': opts.minBitrate,
            'x-google-max-bitrate': opts.maxBitrate,
            'x-google-start-bitrate': opts.startBitrate,
            'cbr': opts.cbr ? 1 : 0,
          };

          if (isH264) {
            paramsToInject['profile-level-id'] = opts.profileLevelId;
            paramsToInject['packetization-mode'] = opts.packetizationMode;
            paramsToInject['level-asymmetry-allowed'] = opts.levelAsymmetryAllowed ? 1 : 0;
          }

          const updatedParams = updateFmtpParameters(existingParams, paramsToInject);
          resultLines.push(`a=fmtp:${pt} ${updatedParams}`);
          continue;
        }
      }

      resultLines.push(line);
      continue;
    }

    resultLines.push(line);
  }

  // Ensure b=AS was added if video section existed without attributes
  if (currentMedia === 'video' && !videoBandwidthInserted && !isExplicitBandwidthZero && opts.bandwidthAS > 0) {
    resultLines.push(`b=AS:${opts.bandwidthAS}`);
  }

  // Handle audio ptime / maxptime injection if audio section exists and attributes were omitted
  const hasAudioSection = lines.some(l => l.startsWith('m=audio '));
  if (hasAudioSection && !audioPtimeInserted && opts.ptime > 0) {
    const audioIdx = resultLines.findIndex(l => l.startsWith('m=audio '));
    if (audioIdx !== -1) {
      let insertOffset = 1;
      if (audioIdx + insertOffset < resultLines.length && resultLines[audioIdx + insertOffset].startsWith('c=')) {
        insertOffset++;
      }
      resultLines.splice(audioIdx + insertOffset, 0, `a=ptime:${opts.ptime}`);
      insertOffset++;
      if (!audioMaxptimeInserted && opts.maxptime > 0 && userOptions?.maxptime !== undefined) {
        resultLines.splice(audioIdx + insertOffset, 0, `a=maxptime:${opts.maxptime}`);
      }
    }
  }

  // Join with CRLF per RFC 4566
  let finalSdp = resultLines.join('\r\n');

  if (sdp.endsWith('\r\n') || sdp.endsWith('\n')) {
    if (!finalSdp.endsWith('\r\n')) {
      finalSdp += '\r\n';
    }
  }

  return finalSdp;
}

/**
 * Modifies an SDP offer description.
 */
export function modifySdpOffer(sdp: string, options?: SdpModifierOptions): string {
  return modifySdp(sdp, options);
}

/**
 * Modifies an SDP answer description.
 */
export function modifySdpAnswer(sdp: string, options?: SdpModifierOptions): string {
  return modifySdp(sdp, options);
}

/**
 * Convenience helper to tune an SDP description for real-time low-latency surveillance.
 */
export function tuneSdpForLowLatency(sdp: string, customOptions?: SdpModifierOptions): string {
  return modifySdp(sdp, customOptions);
}
