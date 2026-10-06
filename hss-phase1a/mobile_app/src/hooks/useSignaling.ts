import { useEffect, useState, useRef, useCallback } from 'react';
import * as WebRTC from 'react-native-webrtc';

export interface UseSignalingReturn {
  stream: any | null;
  error: string | null;
  status: string;
  reconnect: () => void;
  disconnect: () => void;
}

export function useSignaling(
  wsBaseUrl: string,
  userToken: string,
  deviceId: string,
  enabled: boolean = true
): UseSignalingReturn {
  const [stream, setStream] = useState<any | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string>('Disconnected');
  const [reconnectCounter, setReconnectCounter] = useState<number>(0);

  const pcRef = useRef<any | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const pendingCandidates = useRef<any[]>([]);
  const isRemoteDescriptionSet = useRef<boolean>(false);

  const reconnect = useCallback(() => {
    setReconnectCounter((c) => c + 1);
  }, []);

  const disconnect = useCallback(() => {
    setStatus('Disconnected');
    if (wsRef.current) {
      try {
        wsRef.current.close();
      } catch (_) {}
      wsRef.current = null;
    }
    if (pcRef.current) {
      try {
        pcRef.current.close();
      } catch (_) {}
      pcRef.current = null;
    }
    setStream(null);
  }, []);

  useEffect(() => {
    if (!enabled || !wsBaseUrl || !userToken || !deviceId) {
      setStatus('Disconnected');
      return;
    }

    let isMounted = true;
    isRemoteDescriptionSet.current = false;
    pendingCandidates.current = [];

    // Clean up any previous session
    if (wsRef.current) {
      try {
        wsRef.current.close();
      } catch (_) {}
      wsRef.current = null;
    }
    if (pcRef.current) {
      try {
        pcRef.current.close();
      } catch (_) {}
      pcRef.current = null;
    }

    setStatus('Connecting to Cloud...');
    setError(null);

    const cleanWsUrl = wsBaseUrl.replace(/\/+$/, '');
    const wsTarget = cleanWsUrl.includes('/client')
      ? cleanWsUrl + '?token=' + encodeURIComponent(userToken)
      : cleanWsUrl + '/client?token=' + encodeURIComponent(userToken);

    let ws: WebSocket;
    try {
      ws = new WebSocket(wsTarget);
      wsRef.current = ws;
    } catch (wsErr: any) {
      if (isMounted) {
        setError('Failed to create WebSocket: ' + wsErr.message);
        setStatus('Failed');
      }
      return;
    }

    const sessionId = 'sess_' + Date.now() + '_' + Math.random().toString(36).substring(2, 7);

    ws.onopen = async () => {
      if (!isMounted) return;
      setStatus('Negotiating WebRTC...');

      try {
        // Instantiate RTCPeerConnection safely
        const RTCPeerConnection = WebRTC.RTCPeerConnection;
        if (!RTCPeerConnection) {
          throw new Error('WebRTC RTCPeerConnection not supported in this environment');
        }

        const pc = new RTCPeerConnection({
          iceServers: [
            { urls: 'stun:stun.l.google.com:19302' },
            { urls: 'stun:stun1.l.google.com:19302' },
          ],
        });
        pcRef.current = pc;

        pc.ontrack = (event: any) => {
          if (!isMounted) return;
          if (event.streams && event.streams[0]) {
            setStream(event.streams[0]);
            setStatus('Live');
          } else if (event.track) {
            const MediaStream = WebRTC.MediaStream;
            if (MediaStream) {
              const inboundStream = new MediaStream([event.track]);
              setStream(inboundStream);
            } else {
              setStream({ toURL: () => 'track_stream_url' });
            }
            setStatus('Live');
          }
        };

        pc.onicecandidate = (event: any) => {
          if (event.candidate && ws.readyState === WebSocket.OPEN) {
            const candStr =
              typeof event.candidate === 'string'
                ? event.candidate
                : JSON.stringify(event.candidate);
            ws.send(
              JSON.stringify({
                type: 'candidate',
                session_id: sessionId,
                device_id: deviceId,
                camera_id: deviceId,
                candidate: candStr,
              })
            );
          }
        };

        pc.onconnectionstatechange = () => {
          if (!isMounted || !pcRef.current) return;
          const state = pcRef.current.connectionState;
          if (state === 'connected') {
            setStatus('Live');
          } else if (state === 'disconnected' || state === 'failed') {
            setStatus('Disconnected');
          }
        };

        // Add recvonly video transceiver
        if (pc.addTransceiver) {
          pc.addTransceiver('video', { direction: 'recvonly' });
        }

        // Generate SDP offer
        const offer = await pc.createOffer({});
        await pc.setLocalDescription(offer);

        const rawSdp = offer.sdp || (typeof offer === 'string' ? offer : JSON.stringify(offer));

        ws.send(
          JSON.stringify({
            type: 'offer',
            session_id: sessionId,
            device_id: deviceId,
            camera_id: deviceId,
            sdp: rawSdp,
          })
        );
      } catch (err: any) {
        if (isMounted) {
          setError(err.message || 'Failed to negotiate WebRTC offer');
          setStatus('Failed');
        }
      }
    };

    ws.onmessage = async (e: MessageEvent) => {
      if (!isMounted) return;
      try {
        const msg = JSON.parse(e.data);
        if (msg.error) {
          setError(msg.error);
          setStatus('Failed');
          return;
        }

        const pc = pcRef.current;
        if (!pc) return;

        if (msg.type === 'answer') {
          let sdpString = msg.sdp;
          if (typeof sdpString === 'object') {
            sdpString = sdpString.sdp || JSON.stringify(sdpString);
          } else if (typeof sdpString === 'string' && sdpString.startsWith('{')) {
            try {
              const parsed = JSON.parse(sdpString);
              if (parsed.sdp) sdpString = parsed.sdp;
            } catch (_) {}
          }

          const RTCSessionDescription = WebRTC.RTCSessionDescription;
          const desc = RTCSessionDescription
            ? new RTCSessionDescription({ type: 'answer', sdp: sdpString })
            : { type: 'answer', sdp: sdpString };

          await pc.setRemoteDescription(desc);
          isRemoteDescriptionSet.current = true;

          // Drain queued ICE candidates
          const RTCIceCandidate = WebRTC.RTCIceCandidate;
          while (pendingCandidates.current.length > 0) {
            const queued = pendingCandidates.current.shift();
            try {
              const candObj = RTCIceCandidate ? new RTCIceCandidate(queued) : queued;
              await pc.addIceCandidate(candObj);
            } catch (drainErr) {
              console.warn('Error adding queued ICE candidate:', drainErr);
            }
          }
        } else if (msg.type === 'candidate') {
          let candidateObj = msg.candidate;
          if (typeof candidateObj === 'string') {
            try {
              candidateObj = JSON.parse(candidateObj);
            } catch (_) {
              candidateObj = { candidate: candidateObj };
            }
          }

          if (isRemoteDescriptionSet.current) {
            const RTCIceCandidate = WebRTC.RTCIceCandidate;
            const cand = RTCIceCandidate ? new RTCIceCandidate(candidateObj) : candidateObj;
            await pc.addIceCandidate(cand);
          } else {
            pendingCandidates.current.push(candidateObj);
          }
        }
      } catch (err: any) {
        console.warn('Signaling message error:', err);
      }
    };

    ws.onerror = (e: any) => {
      if (isMounted) {
        setError('WebSocket connection error');
        setStatus('Failed');
      }
    };

    ws.onclose = () => {
      if (isMounted && status !== 'Failed') {
        setStatus('Disconnected');
      }
    };

    return () => {
      isMounted = false;
      try {
        ws.close();
      } catch (_) {}
      if (pcRef.current) {
        try {
          pcRef.current.close();
        } catch (_) {}
        pcRef.current = null;
      }
    };
  }, [wsBaseUrl, userToken, deviceId, enabled, reconnectCounter]);

  return {
    stream,
    error,
    status,
    reconnect,
    disconnect,
  };
}
