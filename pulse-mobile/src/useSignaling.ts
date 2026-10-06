import { useEffect, useState, useRef } from 'react';
import {
  RTCPeerConnection,
  RTCSessionDescription,
  RTCIceCandidate,
  MediaStream,
  MediaStreamTrack,
} from 'react-native-webrtc';

export function useSignaling(cloudUrl: string, userToken: string, deviceId: string) {
  const [stream, setStream] = useState<MediaStream | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string>('Connecting to Cloud...');
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const ws = new WebSocket(`${cloudUrl}?token=${userToken}`);
    wsRef.current = ws;

    ws.onopen = async () => {
      setStatus('Negotiating WebRTC...');
      
      const pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      });
      pcRef.current = pc;

      // When the edge daemon sends its video track
      pc.ontrack = (event) => {
        if (event.streams && event.streams[0]) {
          setStream(event.streams[0]);
          setStatus('Live');
        } else {
          const inboundStream = new MediaStream([event.track as MediaStreamTrack]);
          setStream(inboundStream);
          setStatus('Live');
        }
      };

      pc.onicecandidate = (event) => {
        if (event.candidate) {
          ws.send(
            JSON.stringify({
              type: 'candidate',
              device_id: deviceId,
              candidate: JSON.stringify(event.candidate),
            })
          );
        }
      };

      // Create a recvonly transceiver for video
      pc.addTransceiver('video', { direction: 'recvonly' });

      try {
        const offer = await pc.createOffer({});
        await pc.setLocalDescription(offer);

        ws.send(
          JSON.stringify({
            type: 'offer',
            device_id: deviceId,
            session_id: Math.random().toString(36).substring(7),
            sdp: JSON.stringify(offer),
          })
        );
      } catch (err) {
        setError('Failed to create offer');
        console.error(err);
      }
    };

    ws.onmessage = async (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.error) {
          setError(msg.error);
          setStatus('Failed');
          return;
        }
        
        if (msg.type === 'answer' && pcRef.current) {
          const answerSdp = JSON.parse(msg.sdp);
          await pcRef.current.setRemoteDescription(new RTCSessionDescription(answerSdp));
        } else if (msg.type === 'candidate' && pcRef.current) {
          const candidate = JSON.parse(msg.candidate);
          await pcRef.current.addIceCandidate(new RTCIceCandidate(candidate));
        }
      } catch (err) {
        console.error('Error parsing message', err);
      }
    };

    ws.onerror = () => {
      setError('WebSocket disconnected');
      setStatus('Failed');
    };

    ws.onclose = () => {
      setStatus('Disconnected');
    };

    return () => {
      ws.close();
      if (pcRef.current) {
        pcRef.current.close();
      }
    };
  }, [cloudUrl, userToken, deviceId]);

  return { stream, error, status };
}
