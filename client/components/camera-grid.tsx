import React from "react";
import { VideoFeedCard } from "./video-feed-card";
import { Camera } from "@/lib/types";

interface CameraGridProps {
  cameras: Camera[];
  onEditCamera: (camera: Camera) => void;
  onRemoveCamera: (id: string) => void;
  onToggleActive?: (camera: Camera, newState: boolean) => void;
}

export function CameraGrid({
  cameras,
  onEditCamera,
  onRemoveCamera,
  onToggleActive,
}: CameraGridProps) {
  if (cameras.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center animate-in fade-in-0 slide-in-from-bottom-8 duration-700">
        <div className="w-28 h-28 mx-auto mb-6 bg-gradient-to-br from-blue-100 to-purple-100 dark:from-blue-900/20 dark:to-purple-900/20 rounded-full flex items-center justify-center animate-pulse hover:scale-105 transition-transform duration-300">
          <svg
            className="w-14 h-14 text-blue-600 dark:text-blue-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
            />
          </svg>
        </div>
        <h3 className="text-xl font-medium text-gray-900 dark:text-gray-100 mb-3 animate-in slide-in-from-bottom-4 duration-500 delay-200">
          No cameras added yet
        </h3>
        <p className="text-gray-600 dark:text-gray-400 max-w-md animate-in slide-in-from-bottom-4 duration-500 delay-300">
          Add your first camera using the form above to start monitoring your
          smart home with AI-powered detection.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
      {cameras.map((camera, index) => (
        <div
          key={camera.id}
          className="animate-in slide-in-from-bottom-4 fade-in-0"
          style={{ animationDelay: `${index * 150}ms` }}
        >
          <VideoFeedCard
            camera={camera}
            onEdit={onEditCamera}
            onRemove={onRemoveCamera}
            onToggleActive={onToggleActive}
          />
        </div>
      ))}
    </div>
  );
}
