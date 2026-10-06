import React, { useRef, useEffect, useState, useCallback } from "react";
import {
  Maximize2,
  Edit,
  Trash2,
  Camera,
  Wifi,
  WifiOff,
  Video,
  AlertTriangle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Camera as CameraType } from "@/lib/types";



interface VideoFeedCardProps {
  camera: CameraType;
  onEdit: (camera: CameraType) => void;
  onRemove: (id: string) => void;
  onToggleActive?: (camera: CameraType, newState: boolean) => void;
  color?: string;
}



const sourceIcons = {
  webcam: Camera,
  "ip camera": Wifi,
  rtsp: WifiOff,
};

// Array of beautiful gradient colors for video feed cards - 24 unique combinations
const cardColors = [
  'from-blue-400/80 to-cyan-400/80',
  'from-purple-400/80 to-pink-400/80',
  'from-green-400/80 to-emerald-400/80',
  'from-orange-400/80 to-red-400/80',
  'from-indigo-400/80 to-purple-400/80',
  'from-teal-400/80 to-blue-400/80',
  'from-pink-400/80 to-rose-400/80',
  'from-yellow-400/80 to-orange-400/80',
  'from-violet-400/80 to-purple-400/80',
  'from-cyan-400/80 to-blue-400/80',
  'from-emerald-400/80 to-teal-400/80',
  'from-rose-400/80 to-pink-400/80',
  'from-sky-400/80 to-blue-400/80',
  'from-lime-400/80 to-green-400/80',
  'from-amber-400/80 to-yellow-400/80',
  'from-red-400/80 to-pink-400/80',
  'from-fuchsia-400/80 to-purple-400/80',
  'from-slate-400/80 to-gray-400/80',
  'from-zinc-400/80 to-neutral-400/80',
  'from-stone-400/80 to-slate-400/80',
  'from-neutral-400/80 to-stone-400/80',
  'from-gray-400/80 to-zinc-400/80',
  'from-slate-400/80 to-sky-400/80',
  'from-cyan-400/80 to-teal-400/80',
  'from-emerald-400/80 to-lime-400/80'
];

// Function to get color for detection type badges
const getDetectionTypeColor = (type: string) => {
  switch (type) {
    case 'fire':
      return 'bg-red-500 hover:bg-red-600 text-white';
    case 'smoke':
      return 'bg-gray-500 hover:bg-gray-600 text-white';
    case 'weapon':
      return 'bg-orange-500 hover:bg-orange-600 text-white';
    case 'person':
      return 'bg-blue-500 hover:bg-blue-600 text-white';
    case 'animal':
      return 'bg-green-500 hover:bg-green-600 text-white';
    default:
      return 'bg-purple-500 hover:bg-purple-600 text-white';
  }
};

export function VideoFeedCard({
  camera,
  onEdit,
  onRemove,
  onToggleActive,
  color,
}: VideoFeedCardProps) {
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [imageError, setImageError] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const [webcamError, setWebcamError] = useState<string | null>(null);
  const fullscreenVideoRef = useRef<HTMLVideoElement>(null);
  const [fullscreenWebcamError, setFullscreenWebcamError] = useState<string | null>(null);
  const fullscreenModalRef = useRef<HTMLDivElement>(null);

  const SourceIcon = sourceIcons[camera.source] || Camera;
  
  // Generate a consistent color based on camera ID or use provided color
  const cardColor = color || cardColors[camera.id.charCodeAt(0) % cardColors.length];



  useEffect(() => {
    let stream: MediaStream | null = null;
    let isActive = true; // Track if this effect is still active
    
    if (camera.source === "webcam" && camera.isActive && videoRef.current) {
      navigator.mediaDevices.getUserMedia({ video: true })
        .then((mediaStream) => {
          if (!isActive) {
            // Component unmounted or effect cleaned up, stop the stream
            mediaStream.getTracks().forEach((track) => track.stop());
            return;
          }
          stream = mediaStream;
          if (videoRef.current) {
            videoRef.current.srcObject = mediaStream;
            videoRef.current.play().catch(err => {
              console.error('Error playing video:', err);
            });
          }
        })
        .catch((err) => {
          if (!isActive) return;
          console.error('Webcam access error for camera:', camera.name, err);
          setWebcamError("Webcam access denied or unavailable.");
        });
    }
    
    return () => {
      isActive = false;
      if (stream) {
        stream.getTracks().forEach((track) => track.stop());
      }
    };
  }, [camera.source, camera.isActive, camera.name]);

  // Fullscreen webcam effect - reuse existing stream to avoid permission requests in fullscreen
  useEffect(() => {
    if (isFullscreen && camera.source === "webcam" && camera.isActive && fullscreenVideoRef.current) {
      // Reuse the existing stream from the main video element to avoid requesting permissions in fullscreen
      if (videoRef.current?.srcObject instanceof MediaStream) {
        const existingStream = videoRef.current.srcObject as MediaStream;
        if (fullscreenVideoRef.current) {
          fullscreenVideoRef.current.srcObject = existingStream;
          fullscreenVideoRef.current.play().catch(err => {
            console.error('Error playing fullscreen video:', err);
          });
        }
      } else {
        // If no existing stream, show error (this shouldn't happen normally)
        setFullscreenWebcamError("No video stream available.");
      }
    }
    
    // Don't stop the stream here - it's managed by the main video element effect
    return () => {
      if (fullscreenVideoRef.current) {
        fullscreenVideoRef.current.srcObject = null;
      }
    };
  }, [isFullscreen, camera.source, camera.isActive]);

  useEffect(() => {
    if (isFullscreen && fullscreenModalRef.current) {
      // Request browser fullscreen
      const requestFullscreen = async () => {
        try {
          if (fullscreenModalRef.current) {
      if (fullscreenModalRef.current.requestFullscreen) {
              await fullscreenModalRef.current.requestFullscreen();
      } else if ((fullscreenModalRef.current as any).webkitRequestFullscreen) {
              await (fullscreenModalRef.current as any).webkitRequestFullscreen();
      } else if ((fullscreenModalRef.current as any).msRequestFullscreen) {
              await (fullscreenModalRef.current as any).msRequestFullscreen();
            }
          }
        } catch (err) {
          console.error('Error requesting fullscreen:', err);
        }
      };
      requestFullscreen();
    }
    
    // Listen for fullscreen change to exit modal if user exits via browser controls
    const handleFsChange = () => {
      if (!document.fullscreenElement) {
        setIsFullscreen(false);
      }
    };
    
    document.addEventListener("fullscreenchange", handleFsChange);
    document.addEventListener("webkitfullscreenchange", handleFsChange);
    document.addEventListener("msfullscreenchange", handleFsChange);
    
    return () => {
      document.removeEventListener("fullscreenchange", handleFsChange);
      document.removeEventListener("webkitfullscreenchange", handleFsChange);
      document.removeEventListener("msfullscreenchange", handleFsChange);
    };
  }, [isFullscreen]);

  const handleRemove = () => {
    setShowDeleteConfirm(true);
  };

  const confirmDelete = () => {
    onRemove(camera.id);
    setShowDeleteConfirm(false);
  };

  const cancelDelete = () => {
    setShowDeleteConfirm(false);
  };

  const handleFullscreen = () => {
    setIsFullscreen(true);
  };

  const closeFullscreen = async () => {
    // Exit fullscreen first, then update state
    if (document.fullscreenElement) {
      try {
      if (document.exitFullscreen) {
          await document.exitFullscreen();
      } else if ((document as any).webkitExitFullscreen) {
          await (document as any).webkitExitFullscreen();
      } else if ((document as any).msExitFullscreen) {
          await (document as any).msExitFullscreen();
        }
      } catch (err) {
        console.error('Error exiting fullscreen:', err);
      }
    }
    // Update state after exiting fullscreen to avoid "element removed" warning
    setIsFullscreen(false);
  };

  return (
    <>
      <Card className={`group overflow-hidden bg-gradient-to-br ${cardColor} border-2 border-white/20 hover:shadow-xl hover:shadow-blue-500/10 hover:scale-[1.02] transition-all duration-700 ease-out animate-in fade-in-0 slide-in-from-bottom-4 will-change-transform w-full max-w-md min-w-[260px] h-[420px] md:h-[440px] flex flex-col`}>
        <CardHeader className="pb-3 flex-shrink-0">
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-2">
              <SourceIcon className="h-4 w-4 text-gray-600 dark:text-gray-300 transition-colors duration-300" />
              <h3 className="font-medium text-gray-900 dark:text-gray-100 truncate transition-colors duration-300">
                {camera.name}
              </h3>
            </div>
            <div className="flex gap-1 items-center">
              {/* On/Off Toggle */}
              {typeof onToggleActive === 'function' && (
                <Button
                  variant={camera.isActive ? "default" : "outline"}
                  size="sm"
                  onClick={() => onToggleActive(camera, !camera.isActive)}
                  className={`h-8 px-3 py-1 rounded-full font-semibold text-xs transition-all duration-200 ${camera.isActive ? 'bg-green-500 text-white hover:bg-green-600' : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-300 dark:hover:bg-gray-600'}`}
                  aria-pressed={camera.isActive}
                  style={{ minWidth: 48 }}
                >
                  {camera.isActive ? 'On' : 'Off'}
                </Button>
              )}
            <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-all duration-500 ease-out transform translate-x-3 group-hover:translate-x-0">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onEdit(camera)}
                className="h-8 w-8 p-0 hover:bg-blue-100 dark:hover:bg-blue-900/50 hover:scale-110 transition-all duration-300 ease-out will-change-transform hover:shadow-md"
              >
                <Edit className="h-3 w-3 transition-transform duration-200" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={handleRemove}
                className="h-8 w-8 p-0 hover:bg-red-100 dark:hover:bg-red-900/50 hover:scale-110 transition-all duration-300 ease-out will-change-transform hover:shadow-md"
              >
                <Trash2 className="h-3 w-3 transition-transform duration-200" />
              </Button>
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-4 flex-1 flex flex-col justify-between min-h-0">
          <div className="relative aspect-[4/3] w-full min-h-[180px] bg-black rounded-xl border-3 border-black overflow-hidden shadow-lg hover:shadow-xl transition-all duration-500 ease-out will-change-transform">




            {camera.source === "webcam" && camera.isActive ? (
              webcamError ? (
                <div className="w-full h-full flex items-center justify-center bg-gray-900 transition-all duration-500 ease-out">
                  <div className="text-center text-red-400 animate-pulse" style={{ animationDuration: "2s" }}>
                    <Camera className="h-12 w-12 mx-auto mb-3" />
                    <p className="text-sm">{webcamError}</p>
                  </div>
                </div>
              ) : (
                <>
                  <video
                    ref={videoRef}
                    className="w-full h-full object-cover"
                    autoPlay
                    muted
                    playsInline
                    style={{ background: "#000" }}
                  />

                </>
              )
            ) : (
              <div className="w-full h-full flex items-center justify-center bg-gray-900 transition-all duration-500 ease-out">
                <div
                  className="text-center text-gray-400 animate-pulse"
                  style={{ animationDuration: "2s" }}
                >
                  <Camera className="h-12 w-12 mx-auto mb-3 transition-transform duration-500 ease-out group-hover:scale-110 will-change-transform" />
                  <p className="text-sm">{camera.isActive ? 'Camera Active' : 'Camera Inactive'}</p>
                  <p className="text-xs mt-1 opacity-75">{camera.source}</p>
                </div>
              </div>
            )}

            <Button
              variant="ghost"
              size="sm"
              onClick={handleFullscreen}
              className="absolute top-3 right-3 h-9 w-9 p-0 bg-black/50 hover:bg-black/80 text-white opacity-0 group-hover:opacity-100 transition-all duration-500 ease-out transform scale-90 group-hover:scale-100 backdrop-blur-sm will-change-transform hover:shadow-lg"
            >
              <Maximize2 className="h-4 w-4 transition-transform duration-200 hover:rotate-45" />
            </Button>

            {camera.isActive && (
              <div className="absolute top-3 left-3 flex items-center gap-1.5 px-3 py-1.5 bg-green-500 dark:bg-neon-green/80 text-white text-xs rounded-full animate-in slide-in-from-left-2 duration-500 shadow-lg dark:shadow-neon-green/50">
                <div className="w-2 h-2 bg-white rounded-full animate-pulse dark:shadow-white/50" />
                <span className="font-medium">LIVE</span>
              </div>
            )}
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between text-sm transition-all duration-300 hover:bg-white/50 dark:hover:bg-gray-800/50 rounded-lg p-2 -m-2">
              <span className="font-semibold text-gray-700 dark:text-gray-300">Source:</span>
              <span className="font-medium text-gray-900 dark:text-gray-100 capitalize">
                {camera.source}
              </span>
            </div>

            {camera.url && (
              <div className="text-xs text-gray-500 dark:text-gray-400 truncate bg-gray-50 dark:bg-gray-800/50 p-2 rounded-lg animate-in slide-in-from-bottom-2 duration-300">
                {camera.url}
              </div>
            )}

            <div className="space-y-2">
              <span className="text-sm font-semibold text-gray-700 dark:text-gray-300">
                AI Detection:
              </span>
              <div className="flex flex-wrap gap-1.5">
                {camera.detectionTypes && camera.detectionTypes.length > 0 ? (
                  camera.detectionTypes.map((type, index) => (
                    <Badge
                      key={type}
                      className={`text-xs border-0 hover:scale-105 transition-all duration-300 ease-out animate-in slide-in-from-bottom-3 shadow-sm dark:shadow-lg will-change-transform hover:shadow-md ${getDetectionTypeColor(type)}`}
                      style={{
                        animationDelay: `${index * 150}ms`,
                        animationDuration: "600ms",
                      }}
                    >
                      {type}
                    </Badge>
                  ))
                ) : (
                  <span className="text-xs text-gray-500 dark:text-gray-400 italic">
                    No detection types configured
                  </span>
                )}
              </div>
            </div>


          </div>
        </CardContent>
      </Card>

      {/* Fullscreen Modal */}
      {isFullscreen && (
        <div
          ref={fullscreenModalRef}
          className="fixed inset-0 z-50 bg-black/95 backdrop-blur-sm flex items-center justify-center animate-in fade-in-0 duration-300"
          onClick={(e) => {
            // Only close if clicking on the background, not the video element
            if (e.target === e.currentTarget) {
              closeFullscreen();
            }
          }}
        >
          <div className="relative w-full h-full animate-in zoom-in-95 slide-in-from-bottom-8 duration-500">
            {camera.source === "webcam" && camera.isActive ? (
              fullscreenWebcamError ? (
                <div className="w-full h-full flex items-center justify-center bg-gray-900 transition-all duration-500 ease-out">
                  <div className="text-center text-red-400 animate-pulse" style={{ animationDuration: "2s" }}>
                    <Camera className="h-12 w-12 mx-auto mb-3" />
                    <p className="text-sm">{fullscreenWebcamError}</p>
                  </div>
                </div>
              ) : (
                <video
                  ref={fullscreenVideoRef}
                  className="w-full h-full object-contain border-2 border-white/30 rounded-xl shadow-2xl"
                  autoPlay
                  muted
                  playsInline
                  style={{ background: "#000" }}
                />
              )
            ) : (
            <img
              src={`/video_feed/${camera.id}`}
              alt={`${camera.name} feed`}
              className="w-full h-full object-contain border-2 border-white/30 rounded-xl shadow-2xl"
            />
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={closeFullscreen}
              className="absolute top-4 right-4 text-white hover:bg-white/20 backdrop-blur-sm h-10 w-10 rounded-full transition-all duration-200 hover:scale-110"
            >
              ✕
            </Button>
            <div className="absolute bottom-4 left-4 text-white bg-black/50 backdrop-blur-sm rounded-lg p-4 animate-in slide-in-from-left-4 duration-500">
              <h3 className="text-xl font-medium mb-1">{camera.name}</h3>
              <p className="text-sm opacity-90 capitalize flex items-center gap-2">
                <SourceIcon className="h-4 w-4" />
                {camera.source}
              </p>
            </div>
            {camera.isActive && (
              <div className="absolute top-4 left-4 flex items-center gap-2 px-4 py-2 bg-green-500 text-white text-sm rounded-full animate-in slide-in-from-top-4 duration-500">
                <div className="w-2 h-2 bg-white rounded-full animate-pulse" />
                <span className="font-medium">LIVE FEED</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in-0 duration-300">
          <div className="bg-white dark:bg-gray-900 rounded-xl p-6 max-w-md w-full mx-4 animate-in zoom-in-95 slide-in-from-bottom-8 duration-500 border border-red-200 dark:border-red-800/50">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-full">
                <AlertTriangle className="h-5 w-5 text-red-600 dark:text-red-400" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                Remove Camera
              </h3>
            </div>

            <p className="text-gray-600 dark:text-gray-300 mb-6">
              Are you sure you want to remove "{camera.name}"? This action
              cannot be undone.
            </p>

            <div className="flex gap-3 justify-end">
              <Button
                variant="outline"
                onClick={cancelDelete}
                className="hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                Cancel
              </Button>
              <Button
                onClick={confirmDelete}
                className="bg-red-600 hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600 text-white"
              >
                Remove Camera
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
