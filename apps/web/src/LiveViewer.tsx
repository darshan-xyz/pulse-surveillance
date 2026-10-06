import React, { useEffect, useRef, useState } from 'react';

interface LiveViewerProps {
  cameraId: string;
}

export const LiveViewer: React.FC<LiveViewerProps> = ({ cameraId }) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<string>('Connecting...');

  useEffect(() => {
    // 1. Establish WebSocket Connection to HSS Local API
    const ws = new WebSocket(`ws://localhost:8080/ws`);
    
    // 2. Initialize WebRTC PeerConnection
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    });

    // Attach stream to video element
    pc.ontrack = (event) => {
      setStatus('Live');
      if (videoRef.current) {
        videoRef.current.srcObject = event.streams[0];
      }
    };

    pc.oniceconnectionstatechange = () => {
      setStatus(`ICE State: ${pc.iceConnectionState}`);
    };

    // Send ICE candidates to HSS
    pc.onicecandidate = (event) => {
      if (event.candidate) {
        ws.send(JSON.stringify({
          type: 'candidate',
          session_id: 'session-123',
          camera_id: cameraId,
          candidate: JSON.stringify(event.candidate)
        }));
      }
    };

    ws.onopen = async () => {
      setStatus('WebSocket Connected. Creating Offer...');
      
      // Create Offer
      pc.addTransceiver('video', { direction: 'recvonly' });
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      // Send Offer to HSS
      ws.send(JSON.stringify({
        type: 'offer',
        session_id: 'session-123',
        camera_id: cameraId,
        sdp: offer.sdp
      }));
    };

    ws.onmessage = async (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'answer') {
        setStatus('Received Answer. Establishing connection...');
        await pc.setRemoteDescription(new RTCSessionDescription({
          type: 'answer',
          sdp: msg.sdp
        }));
      }
    };

    return () => {
      ws.close();
      pc.close();
    };
  }, [cameraId]);

  return (
    <div className="flex flex-col items-center justify-center p-4 bg-gray-900 rounded-lg shadow-xl w-full max-w-4xl mx-auto">
      <div className="flex justify-between w-full mb-2 text-gray-300 text-sm font-medium">
        <span>Camera: {cameraId}</span>
        <span className={status === 'Live' ? 'text-green-400' : 'text-yellow-400'}>{status}</span>
      </div>
      
      <div className="relative w-full aspect-video bg-black rounded overflow-hidden">
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="w-full h-full object-cover"
        />
        {status !== 'Live' && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/50">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-white"></div>
          </div>
        )}
      </div>
    </div>
  );
};
