/**
 * WebcamBroadcaster
 * Captures local physical webcam via getUserMedia and establishes a multi-tenant
 * WebRTC publisher node connected to the Pulse Cloud Signaling Hub.
 * Handles incoming viewer SDP Offers, creates peer connections, and streams
 * live webcam video tracks to any subscribing remote viewer.
 */

import { getWsBaseUrl } from './config';
import { tuneSdpForLowLatency } from './sdpModifier';

export interface BroadcastStatus {
  isBroadcasting: boolean;
  viewerCount: number;
  error: string | null;
}

export class WebcamBroadcaster {
  private cameraId: string;
  private token: string;
  private ws: WebSocket | null = null;
  private stream: MediaStream | null = null;
  private peerConnections: Map<string, RTCPeerConnection> = new Map();
  private isRunning: boolean = false;
  private onStatusChange?: (status: BroadcastStatus) => void;
  private streamListeners: Array<(stream: MediaStream) => void> = [];

  constructor(cameraId: string = 'webcam-live-01', token: string = '') {
    this.cameraId = cameraId;
    this.token = token;
  }

  public setToken(token: string) {
    this.token = token;
  }

  public setStatusCallback(cb: (status: BroadcastStatus) => void) {
    this.onStatusChange = cb;
  }

  public onStream(cb: (stream: MediaStream) => void) {
    this.streamListeners.push(cb);
    if (this.stream) {
      try {
        cb(this.stream);
      } catch {}
    }
  }

  private notifyStreamListeners(stream: MediaStream) {
    for (const listener of this.streamListeners) {
      try {
        listener(stream);
      } catch {}
    }
  }

  private notifyStatus(error: string | null = null) {
    if (this.onStatusChange) {
      this.onStatusChange({
        isBroadcasting: this.isRunning,
        viewerCount: this.peerConnections.size,
        error
      });
    }
  }

  public async start(): Promise<MediaStream> {
    if (this.isRunning && this.stream) {
      return this.stream;
    }

    try {
      // 1. Acquire Local Physical Webcam with high-throughput 30 FPS constraints
      this.stream = await navigator.mediaDevices.getUserMedia({
        video: {
          width: { ideal: 1280, min: 640 },
          height: { ideal: 720, min: 480 },
          frameRate: { ideal: 30, min: 30 }
        },
        audio: false
      });

      this.notifyStreamListeners(this.stream);

      // 2. Connect to Central Signaling Hub as Device Node
      const wsBase = getWsBaseUrl();
      const wsUrl = `${wsBase}/signaling?camera_id=${encodeURIComponent(this.cameraId)}&token=${encodeURIComponent(this.token)}`;

      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.isRunning = true;
        this.notifyStatus(null);
      };

      this.ws.onmessage = async (event) => {
        try {
          const msg = JSON.parse(event.data);
          await this.handleSignalingMessage(msg);
        } catch (err: any) {
          console.error('[WebcamBroadcaster] Error handling message:', err);
        }
      };

      this.ws.onerror = (err) => {
        console.error('[WebcamBroadcaster] WebSocket error:', err);
        this.notifyStatus('Signaling WebSocket error');
      };

      this.ws.onclose = () => {
        if (this.isRunning) {
          // Reconnect if unexpectedly dropped
          setTimeout(() => {
            if (this.isRunning) this.reconnectSignaling();
          }, 3000);
        }
      };

      this.isRunning = true;
      this.notifyStatus(null);
      return this.stream;
    } catch (err: any) {
      this.stop();
      const msg = err instanceof Error ? err.message : String(err);
      this.notifyStatus(msg);
      throw err;
    }
  }

  private reconnectSignaling() {
    if (!this.isRunning) return;
    const wsBase = getWsBaseUrl();
    const wsUrl = `${wsBase}/signaling?camera_id=${encodeURIComponent(this.cameraId)}&token=${encodeURIComponent(this.token)}`;
    this.ws = new WebSocket(wsUrl);
    this.ws.onopen = () => this.notifyStatus(null);
    this.ws.onmessage = async (event) => {
      try {
        const msg = JSON.parse(event.data);
        await this.handleSignalingMessage(msg);
      } catch {}
    };
  }

  private async handleSignalingMessage(msg: any) {
    const { type, session_id, sdp, candidate, turn_server, turn_username, turn_password } = msg;

    if (!session_id) return;

    if (type === 'offer') {
      // Viewer is requesting stream -> Create RTCPeerConnection & send Answer
      const iceServers: RTCIceServer[] = [
        { urls: 'stun:stun.l.google.com:19302' },
        { 
          urls: 'turn:openrelay.metered.ca:80',
          username: 'openrelayproject',
          credential: 'openrelayproject'
        },
      ];

      const pc = new RTCPeerConnection({
        iceServers,
        iceCandidatePoolSize: 2,
        bundlePolicy: 'max-bundle',
        rtcpMuxPolicy: 'require'
      });

      // Clean up any existing PC for this session
      if (this.peerConnections.has(session_id)) {
        this.peerConnections.get(session_id)?.close();
      }
      this.peerConnections.set(session_id, pc);
      this.notifyStatus(null);

      // Add Local Webcam Tracks
      if (this.stream) {
        this.stream.getTracks().forEach((track) => {
          pc.addTrack(track, this.stream!);
        });
      }

      // Handle Trickle ICE Candidates with End-of-Candidates transmission
      pc.onicecandidate = (event) => {
        if (this.ws?.readyState === WebSocket.OPEN) {
          if (event.candidate) {
            this.ws.send(JSON.stringify({
              type: 'candidate',
              session_id: session_id,
              candidate: JSON.stringify(event.candidate),
            }));
          } else {
            this.ws.send(JSON.stringify({
              type: 'candidate',
              session_id: session_id,
              candidate: null,
            }));
          }
        }
      };

      pc.onconnectionstatechange = () => {
        if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed' || pc.connectionState === 'closed') {
          pc.close();
          this.peerConnections.delete(session_id);
          this.notifyStatus(null);
        }
      };

      // Set Remote Description (Offer) & Create Local Tuned Answer
      await pc.setRemoteDescription(new RTCSessionDescription({ type: 'offer', sdp }));
      const answer = await pc.createAnswer();
      const tunedAnswerSdp = tuneSdpForLowLatency(answer.sdp || '');
      const localAnswer = new RTCSessionDescription({
        type: 'answer',
        sdp: tunedAnswerSdp,
      });
      await pc.setLocalDescription(localAnswer);

      // Send Answer back over signaling
      if (this.ws?.readyState === WebSocket.OPEN && pc.localDescription) {
        this.ws.send(JSON.stringify({
          type: 'answer',
          session_id: session_id,
          sdp: pc.localDescription.sdp,
        }));
      }
    } else if (type === 'candidate' && candidate) {
      const pc = this.peerConnections.get(session_id);
      if (pc) {
        try {
          const candObj = typeof candidate === 'string' ? JSON.parse(candidate) : candidate;
          if (candObj && candObj.candidate !== '') {
            await pc.addIceCandidate(candObj);
          }
        } catch {
          // Ignore invalid candidate format
        }
      }
    }
  }

  public stop() {
    this.isRunning = false;

    // Close all viewer peer connections
    this.peerConnections.forEach((pc) => pc.close());
    this.peerConnections.clear();

    // Close WebSocket
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    // Stop webcam hardware tracks
    if (this.stream) {
      this.stream.getTracks().forEach((t) => t.stop());
      this.stream = null;
    }
    this.streamListeners = [];

    this.notifyStatus(null);
  }

  public getStream(): MediaStream | null {
    return this.stream;
  }

  public sendDetectionPacket(detections: any[], inferenceTimeMs: number = 0) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    try {
      const payload = {
        type: 'detection',
        camera_id: this.cameraId,
        timestamp: Date.now(),
        detections: detections,
        inference_time_ms: inferenceTimeMs
      };
      this.ws.send(JSON.stringify(payload));
    } catch {
      // ignore
    }
  }

  public getIsRunning(): boolean {
    return this.isRunning;
  }
}

// Global singleton for Home Console webcam broadcast
export const globalWebcamBroadcaster = new WebcamBroadcaster('webcam-live-01');
