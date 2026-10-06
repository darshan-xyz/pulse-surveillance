export type CameraSource = "webcam" | "ip camera" | "rtsp";

export type DetectionType = "fire" | "smoke" | "weapon" | "person" | "animal";



export interface Camera {
  id: string;
  name: string;
  source: CameraSource;
  url?: string; // For IP Camera or RTSP/file
  detectionTypes: DetectionType[];
  isActive: boolean;
  createdAt: Date;
}

export interface CameraFormData {
  name: string;
  source: CameraSource;
  url?: string;
  detectionTypes: DetectionType[];
}




