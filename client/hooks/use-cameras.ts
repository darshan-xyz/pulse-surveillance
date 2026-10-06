import { useState, useCallback, useEffect } from "react";
import { Camera, CameraFormData } from "@/lib/types";

const CAMERAS_STORAGE_KEY = "surveillanceCameras";

export function useCameras() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [editingCamera, setEditingCamera] = useState<Camera | null>(null);

  // Load cameras from localStorage on mount
  useEffect(() => {
    const savedCameras = localStorage.getItem(CAMERAS_STORAGE_KEY);
    if (savedCameras) {
      try {
        const parsed = JSON.parse(savedCameras);
        const camerasWithDates = parsed.map((camera: any) => ({
          ...camera,
          createdAt: new Date(camera.createdAt),
        }));
        setCameras(camerasWithDates);
      } catch (error) {
        console.error("Failed to parse cameras from localStorage:", error);
      }
    }
  }, []);

  // Save cameras to localStorage whenever cameras change
  useEffect(() => {
    localStorage.setItem(CAMERAS_STORAGE_KEY, JSON.stringify(cameras));
  }, [cameras]);

  const addCamera = useCallback((formData: CameraFormData) => {
    const newCamera: Camera = {
      id: Date.now().toString(),
      name: formData.name,
      source: formData.source,
      url: formData.url,
      detectionTypes: formData.detectionTypes,
      isActive: true,
      createdAt: new Date(),
    };
    setCameras((prev) => [...prev, newCamera]);
  }, []);

  const updateCamera = useCallback((id: string, updates: Partial<Camera>) => {
    setCameras((prev) =>
      prev.map((camera) =>
        camera.id === id ? { ...camera, ...updates } : camera,
      ),
    );
  }, []);

  const removeCamera = useCallback((id: string) => {
    setCameras((prev) => prev.filter((camera) => camera.id !== id));
  }, []);

  const startEditing = useCallback((camera: Camera) => {
    setEditingCamera(camera);
  }, []);

  const stopEditing = useCallback(() => {
    setEditingCamera(null);
  }, []);

  return {
    cameras,
    editingCamera,
    addCamera,
    updateCamera,
    removeCamera,
    startEditing,
    stopEditing,
  };
}
