import React, { useRef, useState } from 'react';
import { supabase } from '@/lib/supabaseClient';

export function VideoRecorderUpload({ userId }) {
  const [recording, setRecording] = useState(false);
  const [mediaRecorder, setMediaRecorder] = useState(null);
  const [videoUrl, setVideoUrl] = useState('');
  const videoRef = useRef(null);
  const chunks = useRef([]);

  const startRecording = async () => {
    const stream = await navigator.mediaDevices.getUserMedia({ video: true });
    videoRef.current.srcObject = stream;
    const recorder = new window.MediaRecorder(stream);
    recorder.ondataavailable = (e) => chunks.current.push(e.data);
    recorder.onstop = async () => {
      const blob = new Blob(chunks.current, { type: 'video/webm' });
      setVideoUrl(URL.createObjectURL(blob));
      // Upload to Supabase Storage
      const fileName = `${userId}/${Date.now()}.webm`;
      await supabase.storage.from('videos').upload(fileName, blob);
      chunks.current = [];
    };
    setMediaRecorder(recorder);
    recorder.start();
    setRecording(true);
  };

  const stopRecording = () => {
    mediaRecorder.stop();
    setRecording(false);
    if (videoRef.current && videoRef.current.srcObject) {
      const tracks = videoRef.current.srcObject.getTracks();
      tracks.forEach((track) => track.stop());
      videoRef.current.srcObject = null;
    }
  };

  return (
    <div>
      <video ref={videoRef} autoPlay style={{ width: '100%' }} />
      <div className="flex gap-2 mt-2">
        {!recording ? (
          <button onClick={startRecording} className="bg-green-500 text-white px-4 py-2 rounded">Record</button>
        ) : (
          <button onClick={stopRecording} className="bg-red-500 text-white px-4 py-2 rounded">Stop</button>
        )}
      </div>
      {videoUrl && <video src={videoUrl} controls style={{ width: '100%' }} />}
    </div>
  );
} 