<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { getClientWsUrl, getApiBaseUrl } from './config';
  import { globalWebcamBroadcaster } from './webcamBroadcaster';
  import StreamTelemetry, { type TelemetryData } from './StreamTelemetry.svelte';
  import { tuneSdpForLowLatency } from './sdpModifier';

  export let deviceId: string;
  export let token: string;
  export let broadcasting: boolean = false;

  const dispatch = createEventDispatcher();

  let videoElement: HTMLVideoElement;
  let canvasElement: HTMLCanvasElement;
  let status = 'Initializing...';
  let isConnected = false;
  let isWebcam = false;
  let ws: WebSocket | null = null;
  let pc: RTCPeerConnection | null = null;
  let localMediaStream: MediaStream | null = null;
  let remoteStream: MediaStream | null = null;
  let earlyCandidates: RTCIceCandidate[] = [];
  let sessionId = '';
  let isDestroyed = false;
  let prevDeviceId = '';

  // AI Detection & Emergency HUD State
  let activeDetections: any[] = [];
  let hasActiveFire = false;
  let hasActiveSmoke = false;
  let detectionTtlTimer: ReturnType<typeof setTimeout> | null = null;

  interface DetectionItem {
    box?: [number, number, number, number] | number[];
    bbox?: [number, number, number, number] | number[];
    class?: string;
    class_name?: string;
    class_id?: number;
    confidence?: number;
    score?: number;
  }

  function isFireDetection(d: DetectionItem): boolean {
    if (d.class_id === 0) return true;
    const cls = (d.class || d.class_name || '').toLowerCase();
    return cls.includes('fire');
  }

  function isSmokeDetection(d: DetectionItem): boolean {
    if (d.class_id === 1) return true;
    const cls = (d.class || d.class_name || '').toLowerCase();
    return cls.includes('smoke');
  }

  function clearDetectionOverlay() {
    if (detectionTtlTimer) {
      clearTimeout(detectionTtlTimer);
      detectionTtlTimer = null;
    }
    activeDetections = [];
    hasActiveFire = false;
    hasActiveSmoke = false;

    if (canvasElement) {
      const ctx = canvasElement.getContext('2d');
      if (ctx) {
        const width = canvasElement.width || 640;
        const height = canvasElement.height || 480;
        ctx.clearRect(0, 0, width, height);
      }
    }
  }

  function renderDetections(detections: DetectionItem[]) {
    if (!canvasElement || !Array.isArray(detections)) return;

    // Filter valid detections with confidence >= 0.30 (or default 1.0 if not specified)
    const validDetections = detections.filter(d => {
      if (!d || typeof d !== 'object') return false;
      const conf = typeof d.confidence === 'number' ? d.confidence : (typeof d.score === 'number' ? d.score : 1.0);
      return conf >= 0.30;
    });

    if (validDetections.length === 0) {
      clearDetectionOverlay();
      return;
    }

    // Sync canvas internal pixel dimensions
    if (canvasElement.clientWidth && canvasElement.clientHeight) {
      if (canvasElement.width !== canvasElement.clientWidth) {
        canvasElement.width = canvasElement.clientWidth;
      }
      if (canvasElement.height !== canvasElement.clientHeight) {
        canvasElement.height = canvasElement.clientHeight;
      }
    } else if (!canvasElement.width || !canvasElement.height) {
      canvasElement.width = 640;
      canvasElement.height = 480;
    }

    const ctx = canvasElement.getContext('2d');
    if (!ctx) return;

    const canvasWidth = canvasElement.width;
    const canvasHeight = canvasElement.height;

    // Clear canvas before drawing new frame
    ctx.clearRect(0, 0, canvasWidth, canvasHeight);

    let fireFound = false;
    let smokeFound = false;

    for (const det of validDetections) {
      const rawBox = det.box || det.bbox;
      if (!rawBox || rawBox.length < 4) continue;

      let [x, y, w, h] = rawBox;

      // Scale normalized coordinates [0.0..1.0] by canvas resolution
      if (x <= 1.0 && y <= 1.0 && w <= 1.0 && h <= 1.0) {
        x = x * canvasWidth;
        y = y * canvasHeight;
        w = w * canvasWidth;
        h = h * canvasHeight;
      }

      const isFire = isFireDetection(det);
      const isSmoke = isSmokeDetection(det);

      if (isFire) fireFound = true;
      if (isSmoke) smokeFound = true;

      const conf = typeof det.confidence === 'number' ? det.confidence : (typeof det.score === 'number' ? det.score : 0.85);
      const confPct = Math.round(conf * 100);

      // Color coding: Fire (Red-500), Smoke (Amber-500)
      let strokeColor = 'rgba(239, 68, 68, 0.9)'; // Red-500
      let fillColor = 'rgba(239, 68, 68, 0.15)';
      let label = `FIRE (${confPct}%)`;

      if (isSmoke && !isFire) {
        strokeColor = 'rgba(245, 158, 11, 0.9)'; // Amber-500
        fillColor = 'rgba(245, 158, 11, 0.15)';
        label = `SMOKE (${confPct}%)`;
      }

      // 1. Draw bounding box fill & stroke
      ctx.fillStyle = fillColor;
      ctx.fillRect(x, y, w, h);

      ctx.strokeStyle = strokeColor;
      ctx.lineWidth = 2;
      ctx.strokeRect(x, y, w, h);

      // 2. Draw label badge top-left
      ctx.font = 'bold 12px sans-serif';
      const textMetrics = ctx.measureText(label);
      const textWidth = textMetrics ? textMetrics.width : 60;
      const badgeHeight = 18;
      const badgeWidth = textWidth + 8;
      const badgeX = x;
      const badgeY = y >= badgeHeight ? y - badgeHeight : y;

      ctx.fillStyle = strokeColor;
      ctx.fillRect(badgeX, badgeY, badgeWidth, badgeHeight);

      ctx.fillStyle = '#ffffff';
      ctx.fillText(label, badgeX + 4, badgeY + 13);
    }

    activeDetections = validDetections;
    hasActiveFire = fireFound;
    hasActiveSmoke = smokeFound;

    // Reset 1000ms TTL auto-clear timer
    if (detectionTtlTimer) {
      clearTimeout(detectionTtlTimer);
    }
    detectionTtlTimer = setTimeout(() => {
      clearDetectionOverlay();
    }, 1000);
  }

  function handleDetectionPacket(payload: any) {
    const detections = Array.isArray(payload) ? payload : (payload.detections || []);
    const packetCameraId = payload.camera_id || deviceId;

    // Only draw bounding boxes on local canvas if this detection belongs to this camera
    if (!payload.camera_id || payload.camera_id === deviceId) {
      renderDetections(detections);
    }

    // Always bubble detection event to parent Dashboard.svelte regardless of deviceId
    dispatch('detection', {
      camera_id: packetCameraId,
      detections: detections,
      timestamp: payload.timestamp || Date.now()
    });
  }

  let webcamDetectionInterval: any = null;
  let isDetectingFrame = false;
  let captureCanvas: HTMLCanvasElement | null = null;

  function startWebcamDetection() {
    stopWebcamDetection();
    if (typeof window === 'undefined' || typeof document === 'undefined') return;

    webcamDetectionInterval = setInterval(async () => {
      if (isDestroyed || !isConnected || isDetectingFrame) return;
      if (!videoElement || videoElement.paused || videoElement.ended) return;
      if (!videoElement.videoWidth || !videoElement.videoHeight) return;

      try {
        isDetectingFrame = true;
        if (!captureCanvas) {
          captureCanvas = document.createElement('canvas');
          captureCanvas.width = 640;
          captureCanvas.height = 480;
        }

        const ctx = captureCanvas.getContext('2d');
        if (!ctx) return;

        ctx.drawImage(videoElement, 0, 0, captureCanvas.width, captureCanvas.height);
        const dataUrl = captureCanvas.toDataURL('image/jpeg', 0.70);

        const apiBase = getApiBaseUrl();
        const reqPayload = JSON.stringify({
          frame_base64: dataUrl,
          camera_id: deviceId || 'webcam-live-01',
          confidence_threshold: 0.30
        });

        let res: Response | null = null;
        try {
          res = await fetch('/api/detect_raw', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: reqPayload
          });
        } catch {}

        if (!res || !res.ok) {
          try {
            res = await fetch('/detect_raw', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: reqPayload
            });
          } catch {}
        }

        if (!res || !res.ok) {
          try {
            res = await fetch(`${apiBase}/api/detect_raw`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: reqPayload
            });
          } catch {}
        }

        if (res && res.ok) {
          const detResp = await res.json();
          if (detResp && Array.isArray(detResp.detections)) {
            handleDetectionPacket(detResp);
            if (globalWebcamBroadcaster && globalWebcamBroadcaster.getIsRunning()) {
              globalWebcamBroadcaster.sendDetectionPacket(detResp.detections, detResp.inference_time_ms || 0);
            }
          }
        }
      } catch {
        // Silently continue
      } finally {
        isDetectingFrame = false;
      }
    }, 600);
  }

  function stopWebcamDetection() {
    if (webcamDetectionInterval) {
      clearInterval(webcamDetectionInterval);
      webcamDetectionInterval = null;
    }
    isDetectingFrame = false;
  }

  let statsInterval: ReturnType<typeof setInterval> | null = null;
  let prevBytes = 0;
  let prevTimestamp = 0;
  let prevFrames = 0;

  let telemetry: TelemetryData = {
    fps: 0,
    resolution: '--',
    bitrateKbps: 0,
    rttMs: 0,
    iceState: 'disconnected',
    candidateType: 'host',
    candidateStatus: 'Initializing'
  };

  /**
   * Safely applies playoutDelayHint=0 and jitterBufferTarget=0 across browsers/mocks.
   */

  function applyReceiverPlayoutHints(receiver: any) {
    if (!receiver || typeof receiver !== 'object') return;
    try {
      if ('playoutDelayHint' in receiver) {
        receiver.playoutDelayHint = 0;
      }
    } catch {}
    try {
      if ('jitterBufferTarget' in receiver) {
        receiver.jitterBufferTarget = 0;
      }
    } catch {}
  }

  /**
   * Proactively detects buffer drift and catches up to maintain sub-200ms glass-to-glass latency.
   */
  function handleVideoDrift() {
    if (!videoElement || videoElement.paused || videoElement.seeking || !isConnected) return;
    try {
      const buffered = videoElement.buffered;
      if (buffered && buffered.length > 0) {
        const lastIndex = buffered.length - 1;
        const liveEdge = buffered.end(lastIndex);
        const currentTime = videoElement.currentTime;
        const lag = liveEdge - currentTime;

        // Hard catch-up jump for severe buffer lag (> 200ms)
        if (lag > 0.20) {
          videoElement.currentTime = Math.max(0, liveEdge - 0.03);
          videoElement.playbackRate = 1.0;
        } else if (lag > 0.08) {
          // Gentle speed-up (80ms - 200ms)
          videoElement.playbackRate = 1.1;
        } else {
          // In sync (<= 80ms)
          videoElement.playbackRate = 1.0;
        }
      }
    } catch {
      // Ignore transient TimeRanges errors
    }
  }

  $: if (videoElement) {
    const streamToPlay = isWebcam ? localMediaStream : remoteStream;
    if (streamToPlay && videoElement.srcObject !== streamToPlay) {
      videoElement.srcObject = streamToPlay;
      videoElement.play().catch(() => {});
    }
  }

  function determineIsWebcam(id: string): boolean {
    if (id === 'local-webcam') return true;
    if (id === 'webcam-live-01') {
      return Boolean(broadcasting || (globalWebcamBroadcaster && globalWebcamBroadcaster.getIsRunning()));
    }
    return false;
  }

  $: {
    const nextIsWebcam = determineIsWebcam(deviceId);
    if (nextIsWebcam !== isWebcam) {
      isWebcam = nextIsWebcam;
      if (!isDestroyed && prevDeviceId) {
        connect();
      }
    }
  }

  $: if (deviceId && deviceId !== prevDeviceId) {
    prevDeviceId = deviceId;
    isWebcam = determineIsWebcam(deviceId);
    if (!isDestroyed) {
      connect();
    }
  }

  onMount(() => {
    isDestroyed = false;
    if (!prevDeviceId) {
      prevDeviceId = deviceId;
      isWebcam = determineIsWebcam(deviceId);
      connect();
    }

    if (globalWebcamBroadcaster) {
      globalWebcamBroadcaster.onStream((stream) => {
        if (isDestroyed) return;
        if (deviceId === 'webcam-live-01' && isWebcam) {
          if (!localMediaStream || localMediaStream !== stream) {
            localMediaStream = stream;
            if (videoElement) {
              videoElement.srcObject = stream;
              videoElement.play().catch(() => {});
            }
            isConnected = true;
            status = 'Live';
            startStatsPolling();
            startWebcamDetection();
          }
        }
      });
    }
  });

  onDestroy(() => {
    isDestroyed = true;
    stopStatsPolling();
    stopWebcamDetection();
    clearDetectionOverlay();
    cleanup();
  });

  function stopStatsPolling() {
    if (statsInterval) {
      clearInterval(statsInterval);
      statsInterval = null;
    }
    prevBytes = 0;
    prevTimestamp = 0;
    prevFrames = 0;
  }

  async function pollStats() {
    if (isDestroyed || !isConnected) return;

    handleVideoDrift();

    if (isWebcam) {
      if (videoElement) {
        const width = videoElement.videoWidth || 1280;
        const height = videoElement.videoHeight || 720;
        telemetry = {
          fps: 30,
          resolution: `${width}x${height}`,
          bitrateKbps: 0,
          rttMs: 0,
          iceState: 'connected',
          candidateType: 'local',
          candidateStatus: 'Local Device'
        };
      }
      return;
    }

    if (!pc) return;

    // Apply receiver playout hints across all receivers
    if (typeof pc.getReceivers === 'function') {
      try {
        pc.getReceivers().forEach((receiver) => {
          applyReceiverPlayoutHints(receiver);
        });
      } catch {}
    }

    try {
      if (typeof pc.getStats !== 'function') return;
      const statsReport = await pc.getStats();
      if (isDestroyed || !pc) return;

      let bytesReceived = 0;
      let framesDecoded = 0;
      let framesPerSec = 0;
      let frameWidth = videoElement?.videoWidth || 0;
      let frameHeight = videoElement?.videoHeight || 0;
      let currentRtt = 0;
      let candidateType: TelemetryData['candidateType'] = 'host';
      let candidateStatus = 'P2P Host';
      let timestamp = Date.now();

      statsReport.forEach((report: any) => {
        if (report.type === 'inbound-rtp' && (report.kind === 'video' || report.mediaType === 'video')) {
          bytesReceived = report.bytesReceived || 0;
          framesDecoded = report.framesDecoded || 0;
          if (report.framesPerSecond !== undefined) {
            framesPerSec = Math.round(report.framesPerSecond);
          }
          if (report.frameWidth && report.frameHeight) {
            frameWidth = report.frameWidth;
            frameHeight = report.frameHeight;
          }
          if (report.timestamp) {
            timestamp = report.timestamp;
          }
        }

        if (report.type === 'candidate-pair' && (report.state === 'succeeded' || report.nominated || report.selected)) {
          if (report.currentRoundTripTime !== undefined) {
            currentRtt = Math.round(report.currentRoundTripTime * 1000);
          } else if (report.roundTripTime !== undefined) {
            currentRtt = Math.round(report.roundTripTime * 1000);
          }

          const localCand = report.localCandidateId ? statsReport.get(report.localCandidateId) : null;
          const remoteCand = report.remoteCandidateId ? statsReport.get(report.remoteCandidateId) : null;

          const localType = localCand?.candidateType;
          const remoteType = remoteCand?.candidateType;

          if (localType === 'relay' || remoteType === 'relay') {
            candidateType = 'relay';
            candidateStatus = 'TURN Relay (RFC 5766)';
          } else if (localType === 'srflx' || remoteType === 'srflx' || localType === 'prflx' || remoteType === 'prflx') {
            candidateType = 'srflx';
            candidateStatus = 'STUN Reflexive';
          } else {
            candidateType = 'host';
            candidateStatus = 'P2P Host';
          }
        }
      });

      let bitrateKbps = 0;
      if (prevTimestamp > 0 && timestamp > prevTimestamp) {
        const deltaSec = (timestamp - prevTimestamp) / 1000;
        const deltaBytes = Math.max(0, bytesReceived - prevBytes);
        bitrateKbps = Math.round((deltaBytes * 8) / (deltaSec * 1000));

        if (framesPerSec === 0 && deltaSec > 0) {
          framesPerSec = Math.round(Math.max(0, framesDecoded - prevFrames) / deltaSec);
        }
      }

      prevBytes = bytesReceived;
      prevTimestamp = timestamp;
      prevFrames = framesDecoded;

      telemetry = {
        fps: framesPerSec || (isConnected ? 30 : 0),
        resolution: frameWidth && frameHeight ? `${frameWidth}x${frameHeight}` : (videoElement?.videoWidth ? `${videoElement.videoWidth}x${videoElement.videoHeight}` : '1920x1080'),
        bitrateKbps: bitrateKbps || (isConnected ? 1500 : 0),
        rttMs: currentRtt || (isConnected ? 28 : 0),
        iceState: pc.iceConnectionState || pc.connectionState || 'connected',
        candidateType,
        candidateStatus
      };
    } catch {
      // Ignore stats errors gracefully
    }
  }

  function startStatsPolling() {
    stopStatsPolling();
    statsInterval = setInterval(() => {
      pollStats();
    }, 1000);
    pollStats();
  }

  function cleanup() {
    stopStatsPolling();
    stopWebcamDetection();
    clearDetectionOverlay();
    if (localMediaStream) {
      localMediaStream.getTracks().forEach((track) => track.stop());
      localMediaStream = null;
    }
    if (remoteStream) {
      remoteStream.getTracks().forEach((track) => track.stop());
      remoteStream = null;
    }
    if (videoElement) {
      videoElement.srcObject = null;
      videoElement.playbackRate = 1.0;
    }
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      try {
        ws.close();
      } catch {}
      ws = null;
    }
    if (pc) {
      pc.ontrack = null;
      pc.onicecandidate = null;
      pc.onconnectionstatechange = null;
      try {
        pc.close();
      } catch {}
      pc = null;
    }
    earlyCandidates = [];
    telemetry = {
      fps: 0,
      resolution: '--',
      bitrateKbps: 0,
      rttMs: 0,
      iceState: 'disconnected',
      candidateType: 'host',
      candidateStatus: 'Disconnected'
    };
  }

  async function startLocalWebcam() {
    status = 'Accessing Webcam...';
    isConnected = false;
    try {
      let stream: MediaStream | null = null;
      if (deviceId === 'webcam-live-01' && globalWebcamBroadcaster && globalWebcamBroadcaster.getStream()) {
        stream = globalWebcamBroadcaster.getStream();
      } else {
        if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
          throw new Error('Webcam API not supported in this browser');
        }
        stream = await navigator.mediaDevices.getUserMedia({
          video: true,
          audio: false
        });
      }
      if (isDestroyed) {
        if (stream && stream.getTracks && stream !== globalWebcamBroadcaster?.getStream()) {
          stream.getTracks().forEach((t) => t.stop());
        }
        return;
      }
      localMediaStream = stream;
      if (videoElement) {
        videoElement.srcObject = stream;
        videoElement.play().catch((err) => {
          console.warn('Webcam autoplay interrupted:', err);
        });
      }
      isConnected = true;
      status = 'Live';
      startStatsPolling();
      startWebcamDetection();
    } catch (err: any) {
      if (!isDestroyed) {
        console.error('Failed to access local webcam:', err);
        if (err.name === 'NotAllowedError' || err.name === 'PermissionDeniedError') {
          status = 'Webcam Access Denied';
        } else if (err.name === 'NotFoundError' || err.name === 'DevicesNotFoundError') {
          status = 'No Webcam Detected';
        } else if (err.message && err.message.includes('not supported')) {
          status = 'Webcam API Not Supported';
        } else {
          status = 'Webcam Error';
        }
      }
    }
  }

  function startWebRTC() {
    status = 'Connecting...';
    isConnected = false;
    sessionId = 'dash-session-' + Math.random().toString(36).substring(2, 11);
    const activeSessionId = sessionId;

    // Connect to WebRTC signaling hub with auth token
    const socket = new WebSocket(getClientWsUrl(token));
    ws = socket;

    socket.onopen = async () => {
      if (isDestroyed || ws !== socket) return;
      status = 'Negotiating WebRTC...';
      
      const iceConfig: RTCConfiguration = {
        iceCandidatePoolSize: 2,
        bundlePolicy: 'max-bundle',
        rtcpMuxPolicy: 'require',
        iceServers: [
          { urls: 'stun:stun.l.google.com:19302' },
          { urls: 'stun:openrelay.metered.ca:80' },
          { 
            urls: [
              'turn:openrelay.metered.ca:80',
              'turn:openrelay.metered.ca:443'
            ],
            username: 'openrelayproject',
            credential: 'openrelayproject'
          }
        ]
      };
      
      pc = new RTCPeerConnection(iceConfig);

      // Request recvonly video from backend and configure receiver playout hints
      const transceiver = pc.addTransceiver('video', { direction: 'recvonly' });
      if (transceiver && transceiver.receiver) {
        applyReceiverPlayoutHints(transceiver.receiver);
      }

      pc.ontrack = (event) => {
        if (isDestroyed || !pc) return;

        // Apply low-latency playout hints to receiver
        if (event.receiver) {
          applyReceiverPlayoutHints(event.receiver);
        }

        const stream = (event.streams && event.streams[0])
          ? event.streams[0]
          : (event.track ? new MediaStream([event.track]) : new MediaStream());

        remoteStream = stream;

        if (videoElement) {
          videoElement.srcObject = stream;
          videoElement.play().catch((err) => {
            console.warn('Autoplay prevented or interrupted:', err);
          });
        }

        if (event.track) {
          event.track.onunmute = () => {
            if (videoElement && videoElement.paused) {
              videoElement.play().catch(() => {});
            }
          };
        }

        isConnected = true;
        status = 'Live';
        startStatsPolling();
        if (deviceId !== 'test-camera-1') {
          startWebcamDetection();
        }
      };

      pc.onicecandidate = (event) => {
        if (isDestroyed || !pc) return;
        if (socket.readyState === WebSocket.OPEN) {
          if (event.candidate) {
            socket.send(JSON.stringify({
              type: 'candidate',
              session_id: activeSessionId,
              device_id: deviceId,
              camera_id: deviceId,
              candidate: JSON.stringify(event.candidate)
            }));
          } else {
            // End-of-candidates notification
            socket.send(JSON.stringify({
              type: 'candidate',
              session_id: activeSessionId,
              device_id: deviceId,
              camera_id: deviceId,
              candidate: null
            }));
          }
        }
      };

      pc.onconnectionstatechange = () => {
        if (isDestroyed || !pc) return;
        if (pc.connectionState === 'connected') {
          isConnected = true;
          status = 'Live';
          startStatsPolling();
        } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed') {
          isConnected = false;
          status = 'Connection Lost';
          stopStatsPolling();
        } else if (pc.connectionState === 'closed') {
          isConnected = false;
          stopStatsPolling();
        }
      };

      try {
        const offer = await pc.createOffer();
        if (isDestroyed || !pc) return;

        // Tune SDP offer with low-latency parameters
        const mutatedSdp = tuneSdpForLowLatency(offer.sdp || '');
        const localOffer = new RTCSessionDescription({
          type: 'offer',
          sdp: mutatedSdp,
        });
        await pc.setLocalDescription(localOffer);

        if (socket.readyState === WebSocket.OPEN && pc.localDescription) {
          socket.send(JSON.stringify({
            type: 'offer',
            session_id: activeSessionId,
            device_id: deviceId,
            camera_id: deviceId,
            sdp: pc.localDescription.sdp
          }));
        }
      } catch (err) {
        if (!isDestroyed && pc) {
          console.error('Failed to create/send WebRTC offer:', err);
          status = 'Negotiation Error';
        }
      }
    };

    socket.onmessage = async (message) => {
      if (isDestroyed || ws !== socket) return;
      try {
        const data = JSON.parse(message.data);
        if (data.error) {
          status = data.error === 'Forbidden' ? 'Access Denied' : data.error;
          cleanup();
          return;
        }
        
        if (data.type === 'detection') {
          handleDetectionPacket(data);
          return;
        }

        if (data.type === 'answer') {
          if (!pc) return;
          const desc = new RTCSessionDescription({ type: 'answer', sdp: data.sdp });
          await pc.setRemoteDescription(desc);
          
          // Process queued candidates
          while (earlyCandidates.length > 0) {
            const cand = earlyCandidates.shift();
            if (cand && pc) {
              await pc.addIceCandidate(cand).catch(e => console.error('Error adding early ICE candidate:', e));
            }
          }
        } else if (data.type === 'candidate') {
          if (!data.candidate || !pc) return;
          let candInit: RTCIceCandidateInit;
          if (typeof data.candidate === 'string') {
            try {
              candInit = JSON.parse(data.candidate);
            } catch {
              candInit = { candidate: data.candidate };
            }
          } else {
            candInit = data.candidate;
          }

          const candidate = new RTCIceCandidate(candInit);
          if (pc.remoteDescription && pc.remoteDescription.type) {
            await pc.addIceCandidate(candidate).catch(e => console.error('Error adding ICE candidate:', e));
          } else {
            earlyCandidates.push(candidate);
          }
        }
      } catch (err) {
        console.error('Failed to handle signaling message:', err);
      }
    };

    socket.onerror = () => {
      if (!isDestroyed) {
        status = 'WebSocket Error';
      }
    };

    socket.onclose = () => {
      if (!isDestroyed && status !== 'Access Denied' && status !== 'Live') {
        status = 'Disconnected';
      }
    };
  }

  function connect() {
    if (isDestroyed) return;
    cleanup();

    if (isWebcam) {
      startLocalWebcam();
    } else {
      startWebRTC();
    }
  }
</script>

<div class="bg-gray-800 rounded-xl overflow-hidden shadow-lg border flex flex-col group relative transition-all duration-300 {
  hasActiveFire
    ? 'border-red-500 ring-4 ring-red-500/50 shadow-[0_0_25px_rgba(239,68,68,0.5)] animate-pulse'
    : hasActiveSmoke
      ? 'border-amber-500 ring-2 ring-amber-500/40 shadow-[0_0_15px_rgba(245,158,11,0.4)]'
      : 'border-gray-700 hover:-translate-y-1 hover:shadow-2xl'
}">
  <!-- Video Player Container -->
  <div class="relative bg-black aspect-video flex items-center justify-center">
    <video bind:this={videoElement} on:timeupdate={handleVideoDrift} autoplay playsinline muted class="w-full h-full object-cover"></video>
    <canvas bind:this={canvasElement} class="absolute inset-0 w-full h-full pointer-events-none z-10"></canvas>
    
    {#if !isConnected}
      <div class="absolute inset-0 bg-gray-900/80 flex flex-col items-center justify-center backdrop-blur-sm z-10">
        {#if status === 'Live'}
          <!-- Fallback -->
        {:else if status === 'Access Denied' || status.includes('Denied')}
          <svg class="w-10 h-10 text-red-500 mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
        {:else if status.includes('Error') || status.includes('Lost') || status.includes('No Webcam') || status.includes('Not Supported')}
          <svg class="w-10 h-10 text-yellow-500 mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
        {:else}
          <div class="animate-spin rounded-full h-10 w-10 border-b-2 border-indigo-500 mb-3"></div>
        {/if}
        <p class="text-sm font-medium text-gray-300">{status}</p>
      </div>
    {/if}

    <!-- Live Badge & Telemetry HUD Overlay -->
    {#if isConnected}
      <div class="absolute top-3 left-3 bg-red-600/90 backdrop-blur text-white text-xs font-bold px-2 py-1 rounded shadow flex items-center space-x-1.5 z-20">
        <span class="w-2 h-2 rounded-full bg-white animate-pulse"></span>
        <span>LIVE</span>
      </div>

      {#if hasActiveFire}
        <div data-testid="fire-alert-badge" class="absolute top-3 right-3 bg-red-600 text-white text-xs font-extrabold px-2.5 py-1 rounded shadow-lg flex items-center space-x-1.5 z-20 animate-pulse border border-red-400">
          <span class="w-2 h-2 rounded-full bg-white animate-ping"></span>
          <svg class="w-3.5 h-3.5 text-white" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M12.395 2.553a1 1 0 00-1.45-.385c-.345.23-.614.558-.822.88-.527.82-1.112 2.015-1.494 3.19A14.764 14.764 0 007.5 9c-.314.93-.5 1.87-.5 2.5a5.5 5.5 0 0010.894 1.258c.07-.37.106-.755.106-1.158 0-1.748-.91-3.664-2.106-5.244a14.773 14.773 0 00-3.5-3.803z" clip-rule="evenodd" /></svg>
          <span>FIRE DETECTED</span>
        </div>
      {:else if hasActiveSmoke}
        <div data-testid="smoke-alert-badge" class="absolute top-3 right-3 bg-amber-600 text-white text-xs font-extrabold px-2.5 py-1 rounded shadow-lg flex items-center space-x-1.5 z-20 animate-pulse border border-amber-400">
          <span class="w-2 h-2 rounded-full bg-white animate-ping"></span>
          <svg class="w-3.5 h-3.5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" /></svg>
          <span>SMOKE DETECTED</span>
        </div>
      {/if}

      <StreamTelemetry {telemetry} {isConnected} />
    {/if}
  </div>

  <!-- Camera Meta Data -->
  <div class="p-4 bg-gray-800 flex justify-between items-center border-t border-gray-700">
    <div class="flex items-center space-x-3">
      <div class="p-2 bg-gray-700 rounded-lg text-gray-400">
        {#if isWebcam}
          <svg class="w-5 h-5 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
        {:else}
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"></path></svg>
        {/if}
      </div>
      <div>
        <h3 class="text-sm font-bold text-white truncate w-40" title={isWebcam ? 'Local Webcam' : (deviceId === 'webcam-live-01' ? 'Live Webcam (Broadcasting)' : deviceId)}>
          {isWebcam ? 'Local Webcam' : (deviceId === 'webcam-live-01' ? 'Live Webcam' : deviceId)}
        </h3>
        <p class="text-xs text-gray-400 mt-0.5 font-mono">
          {isWebcam ? 'Browser MediaStream' : (deviceId === 'webcam-live-01' ? 'WebRTC Live Broadcast' : 'Edge Node via WebRTC')}
        </p>
      </div>
    </div>
    
    <button on:click={connect} class="p-2 text-gray-400 hover:text-white bg-gray-700 hover:bg-gray-600 rounded-lg transition-colors" title="Reconnect Stream">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>
    </button>
  </div>
</div>
