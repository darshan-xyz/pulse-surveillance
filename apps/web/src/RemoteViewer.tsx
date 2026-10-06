import React, { useEffect, useRef, useState } from 'react';
import { Camera, Wifi, WifiOff } from 'lucide-react';

interface RemoteViewerProps {
  deviceId: string;
  cameraId: string;
  token: string; // Used for Auth if necessary
}

export const RemoteViewer: React.FC<RemoteViewerProps> = ({ deviceId, cameraId, token }) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<string>('Connecting to Cloud Relay...');
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    // 1. Establish WebSocket Connection to Central Cloud Signaling Relay
    const sessionId = `web-session-${Math.random().toString(36).substring(7)}`;
    const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${wsProto}//${window.location.host}/ws/client?session_id=${sessionId}&device_id=${deviceId}&token=${token}`);
    
    // 2. Initialize WebRTC PeerConnection
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    });

    // Attach stream to video element
    pc.ontrack = (event) => {
      setStatus('Live');
      setIsConnected(true);
      if (videoRef.current) {
        videoRef.current.srcObject = event.streams[0];
      }
    };

    pc.oniceconnectionstatechange = () => {
      if (pc.iceConnectionState === 'disconnected' || pc.iceConnectionState === 'failed') {
        setStatus('Connection Lost');
        setIsConnected(false);
      } else {
        setStatus(`ICE State: ${pc.iceConnectionState}`);
      }
    };

    // Send ICE candidates to Cloud Relay -> HSS
    pc.onicecandidate = (event) => {
      if (event.candidate && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          type: 'candidate',
          session_id: sessionId,
          camera_id: cameraId,
          candidate: JSON.stringify(event.candidate)
        }));
      }
    };

    ws.onopen = async () => {
      setStatus('Relay Connected. Negotiating WebRTC...');
      
      // Create Offer
      pc.addTransceiver('video', { direction: 'recvonly' });
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      // Send Offer to Cloud Relay -> HSS
      ws.send(JSON.stringify({
        type: 'offer',
        session_id: sessionId,
        camera_id: cameraId,
        sdp: offer.sdp
      }));
    };

    ws.onmessage = async (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'answer') {
        setStatus('Received Answer. Establishing P2P stream...');
        await pc.setRemoteDescription(new RTCSessionDescription({
          type: 'answer',
          sdp: msg.sdp
        }));
      }
    };

    ws.onerror = () => {
      setStatus('WebSocket Connection Failed');
      setIsConnected(false);
    };

    return () => {
      ws.close();
      pc.close();
    };
  }, [deviceId, cameraId]);

  return (
    <div className="flex flex-col bg-gray-900 border border-gray-800 rounded-xl shadow-2xl overflow-hidden w-full max-w-4xl">
      <div className="flex justify-between items-center bg-gray-950 px-4 py-3 border-b border-gray-800">
        <div className="flex items-center gap-2">
          <Camera className="w-5 h-5 text-gray-400" />
          <span className="font-medium text-gray-200">{cameraId}</span>
        </div>
        <div className="flex items-center gap-2 text-sm">
          {isConnected ? (
            <Wifi className="w-4 h-4 text-emerald-400" />
          ) : (
            <WifiOff className="w-4 h-4 text-red-400" />
          )}
          <span className={isConnected ? 'text-emerald-400' : 'text-gray-400'}>{status}</span>
        </div>
      </div>
      
      <div className="relative w-full aspect-video bg-black flex items-center justify-center">
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="absolute inset-0 w-full h-full object-contain"
        />
        {!isConnected && (
          <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/80 z-10">
            <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-blue-500 mb-4"></div>
            <p className="text-gray-400 text-sm">{status}</p>
          </div>
        )}
      </div>
    </div>
  );
};
