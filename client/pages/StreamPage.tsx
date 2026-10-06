import React, { useState } from 'react';
import { VideoStream } from '../components/VideoStream';
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import { Loader2, Play, Square } from 'lucide-react';

// When running locally with vite proxy or the built server, we just use relative path
const API_BASE = '';

export default function StreamPage() {
  const [isStreaming, setIsStreaming] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const { toast } = useToast();

  const handleStartStream = async () => {
    setIsLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/stream/start`);
      if (res.ok) {
        setIsStreaming(true);
        toast({
          title: "Stream Started",
          description: "Connecting to video source...",
        });
      } else {
        throw new Error("Failed to start stream");
      }
    } catch (error) {
      toast({
        title: "Error",
        description: "Could not start the stream",
        variant: "destructive"
      });
    } finally {
      setIsLoading(false);
    }
  };

  const handleStopStream = async () => {
    setIsLoading(true);
    try {
      const res = await fetch(`${API_BASE}/api/stream/stop`);
      if (res.ok) {
        setIsStreaming(false);
        toast({
          title: "Stream Stopped",
          description: "Video stream has been terminated.",
        });
      }
    } catch (error) {
      toast({
        title: "Error",
        description: "Could not stop the stream",
        variant: "destructive"
      });
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="container mx-auto p-6 max-w-5xl space-y-6 mt-12">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Surveillance Stream</h1>
        <p className="text-muted-foreground mt-2">
          View the live video feed from the connected camera.
        </p>
      </div>

      <div className="flex gap-4 items-center mb-4">
        {!isStreaming ? (
          <Button onClick={handleStartStream} disabled={isLoading} className="gap-2">
            {isLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
            Start Stream
          </Button>
        ) : (
          <Button onClick={handleStopStream} variant="destructive" disabled={isLoading} className="gap-2">
            {isLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Square className="w-4 h-4" />}
            Stop Stream
          </Button>
        )}
      </div>

      {isStreaming ? (
        <div className="w-full shadow-lg border rounded-lg p-2 bg-card">
          {/* A unique key forces remounting if you stop and restart, clearing old buffers */}
          <VideoStream key={Date.now()} streamUrl={`${API_BASE}/hls/stream.m3u8`} />
        </div>
      ) : (
        <div className="w-full aspect-video bg-muted border-2 border-dashed rounded-lg flex items-center justify-center">
          <p className="text-muted-foreground">Stream is currently inactive.</p>
        </div>
      )}
    </div>
  );
}
