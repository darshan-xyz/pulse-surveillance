import { Router } from 'express';
import ffmpeg from 'fluent-ffmpeg';
import ffmpegStatic from 'ffmpeg-static';
import path from 'path';
import fs from 'fs';

const router = Router();
let ffmpegProcess: ffmpeg.FfmpegCommand | null = null;

router.get('/start', (req, res) => {
  if (ffmpegProcess) {
    res.status(200).json({ message: 'Stream is already running' });
    return;
  }

  // Ensure public/hls directory exists
  const hlsDir = path.join(process.cwd(), 'public', 'hls');
  if (!fs.existsSync(hlsDir)) {
    fs.mkdirSync(hlsDir, { recursive: true });
  }

  // The RTSP stream or fallback video URL
  // For demo purposes, we provide a default that is likely not working unless replaced
  const sourceUrl = process.env.STREAM_SOURCE_URL || 'rtsp://camera.local/stream';
  const outputPath = path.join(hlsDir, 'stream.m3u8');

  if (ffmpegStatic) {
    ffmpeg.setFfmpegPath(ffmpegStatic);
  }

  ffmpegProcess = ffmpeg(sourceUrl)
    .addOptions([
      '-profile:v baseline',
      '-level 3.0',
      '-start_number 0',
      '-hls_time 10',       // 10 seconds per segment
      '-hls_list_size 6',   // keep only the last 6 segments (live rolling playlist)
      '-hls_flags delete_segments', // clean up old segments automatically
      '-f hls'
    ])
    .output(outputPath)
    .on('start', () => {
      console.log('FFmpeg started generating HLS stream');
    })
    .on('end', () => {
      console.log('FFmpeg stream generation ended');
      ffmpegProcess = null;
    })
    .on('error', (err) => {
      console.error('FFmpeg error:', err);
      ffmpegProcess = null;
    });

  ffmpegProcess.run();

  res.status(200).json({ message: 'Stream started successfully', source: sourceUrl });
});

router.get('/stop', (req, res) => {
  if (ffmpegProcess) {
    ffmpegProcess.kill('SIGKILL');
    ffmpegProcess = null;
    res.status(200).json({ message: 'Stream stopped' });
    return;
  }
  res.status(200).json({ message: 'No stream is currently running' });
});

export default router;
