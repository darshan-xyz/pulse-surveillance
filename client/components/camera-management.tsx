import React from "react";
import { useState, useRef, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Plus, Edit3, Save, FolderOpen, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { CameraFormData, DetectionType, Camera } from "@/lib/types";

const cameraSchema = z.object({
  name: z.string().min(1, "Camera name is required"),
  source: z.enum(["webcam", "ip camera", "rtsp"]),
  url: z.string().optional(),
  detectionTypes: z
    .array(z.enum(["fire", "smoke", "weapon", "person", "animal"]))
    .min(1, "Select at least one detection type"),
});

interface CameraManagementProps {
  onAddCamera: (data: CameraFormData) => void;
  onUpdateCamera: (id: string, updates: Partial<Camera>) => void;
  editingCamera: Camera | null;
  onStopEditing: () => void;
  onClose?: () => void;
}



const detectionOptions: { value: DetectionType; label: string }[] = [
  { value: "fire", label: "Fire Detection" },
  { value: "smoke", label: "Smoke Detection" },
  { value: "weapon", label: "Weapon Detection" },
  { value: "person", label: "Person Detection" },
  { value: "animal", label: "Animal Detection" },
];

export function CameraManagement({
  onAddCamera,
  onUpdateCamera,
  editingCamera,
  onStopEditing,
  onClose,
}: CameraManagementProps) {
  const [selectedDetections, setSelectedDetections] = useState<DetectionType[]>(
    editingCamera?.detectionTypes || [],
  );

  const fileInputRef = useRef<HTMLInputElement>(null);

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    reset,
    formState: { errors },
  } = useForm<CameraFormData>({
    resolver: zodResolver(cameraSchema),
            defaultValues: editingCamera
          ? {
              name: editingCamera.name,
              source: editingCamera.source,
              url: editingCamera.url || "",
              detectionTypes: editingCamera.detectionTypes,
            }
          : {
              name: "",
              source: "webcam",
              url: "",
              detectionTypes: [],
            },
  });

  // Register the detectionTypes field
  register("detectionTypes");

  const sourceValue = watch("source");

  // Watch form data for detectionTypes
  const watchedData = watch();

  // Sync selectedDetections with form data
  useEffect(() => {
    setSelectedDetections(watchedData.detectionTypes || []);
  }, [watchedData.detectionTypes]);

  const handleDetectionChange = (
    detection: DetectionType,
    checked: boolean,
  ) => {
    const updated = checked
      ? [...selectedDetections, detection]
      : selectedDetections.filter((d) => d !== detection);

    setSelectedDetections(updated);
    setValue("detectionTypes", updated);
  };

  const handleSelectAllDetections = () => {
    if (selectedDetections.length === detectionOptions.length) {
      // If all are selected, deselect all
      setSelectedDetections([]);
      setValue("detectionTypes", []);
    } else {
      // If not all are selected, select all
      const allDetections = detectionOptions.map(option => option.value);
      setSelectedDetections(allDetections);
      setValue("detectionTypes", allDetections);
    }
  };

  const onSubmit = (data: CameraFormData) => {
    if (!data.source) {
      toast.error("Please select a camera source");
      return;
    }

    if ((data.source === "ip camera" || data.source === "rtsp") && !data.url) {
      toast.error("URL is required for IP Camera and RTSP sources");
      return;
    }

    const cameraData = {
      ...data,
      detectionTypes: data.detectionTypes,
    };

    if (editingCamera) {
      onUpdateCamera(editingCamera.id, {
        name: data.name,
        source: data.source,
        url: data.url,
        detectionTypes: data.detectionTypes,
      });
      onStopEditing();
      toast.success("Camera updated successfully");
    } else {
      onAddCamera(cameraData);
      toast.success("Camera added successfully");
    }

    reset({
      name: "",
      source: "webcam",
      url: "",
      detectionTypes: [],
    });
    setSelectedDetections([]);
  };



  return (
    <Card className="bg-gradient-to-br from-white/90 to-blue-50/50 dark:from-gray-900/90 dark:to-gray-800/50 border-blue-200/50 dark:border-neon-purple/20 hover:shadow-lg dark:hover:shadow-neon-purple/20 transition-all duration-300 animate-in slide-in-from-top-4 fade-in-0">
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2 text-blue-900 dark:text-blue-100 transition-colors duration-300">
            {editingCamera ? (
              <Edit3 className="h-5 w-5 animate-in spin-in-180 duration-300" />
            ) : (
              <Plus className="h-5 w-5 animate-in zoom-in-0 duration-300" />
            )}
            <span className="animate-in slide-in-from-left-2 duration-300">
              {editingCamera ? "Edit Camera" : "Add New Camera"}
            </span>
          </CardTitle>
          {onClose && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onClose}
              className="h-8 w-8 p-0"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
          <div className="space-y-2 animate-in slide-in-from-bottom-2 duration-300">
            <Label htmlFor="name">Camera Name</Label>
            <Input
              id="name"
              {...register("name")}
              placeholder="e.g., Front Door, Living Room"
              className="bg-white/80 dark:bg-gray-800/80 transition-all duration-200 focus:scale-[1.02] focus:shadow-md"
            />
            {errors.name && (
              <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200">
                {errors.name.message}
              </p>
            )}
          </div>

          <div className="space-y-2 animate-in slide-in-from-bottom-2 duration-300 delay-100">
            <Label htmlFor="source">Camera Source</Label>
            <Select
              value={sourceValue}
              onValueChange={(value) => setValue("source", value as any)}
            >
              <SelectTrigger className="bg-white/80 dark:bg-gray-800/80 transition-all duration-200 hover:bg-white dark:hover:bg-gray-800">
                <SelectValue placeholder="Select camera source" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="webcam">Webcam</SelectItem>
                <SelectItem value="ip camera">IP Camera</SelectItem>
                <SelectItem value="rtsp">RTSP/Video File</SelectItem>
              </SelectContent>
            </Select>
            {errors.source && (
              <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200">
                {errors.source.message}
              </p>
            )}
          </div>

          {(sourceValue === "ip camera" || sourceValue === "rtsp") && (
            <div className="space-y-2 animate-in slide-in-from-bottom-4 fade-in-0 duration-300">
              <Label htmlFor="url">
                {sourceValue === "ip camera"
                  ? "IP Camera URL"
                  : "RTSP URL or Video File"}
              </Label>
              <div className="flex gap-3">
                <Input
                  id="url"
                  {...register("url")}
                  placeholder={
                    sourceValue === "ip camera"
                      ? "http://192.168.1.100:4747/video"
                      : "rtsp://... or select a video file"
                  }
                  className="bg-white/80 dark:bg-gray-800/80 transition-all duration-200 focus:scale-[1.02] focus:shadow-md max-w-xs"
                />
                {sourceValue === "rtsp" && (
                  <>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => fileInputRef.current?.click()}
                      className="px-6 py-2 h-11 bg-white/80 dark:bg-gray-800/80 border-neon-cyan/30 dark:border-neon-cyan/50 hover:border-neon-cyan/60 dark:hover:border-neon-cyan/80 hover:bg-neon-cyan/5 dark:hover:bg-neon-cyan/10 transition-all duration-200 hover:scale-105"
                    >
                      <FolderOpen className="h-5 w-5 mr-2" />
                      Browse
                    </Button>
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept="video/*"
                      className="hidden"
                      onChange={(e) => {
                        const file = e.target.files?.[0];
                        if (file) {
                          setValue("url", file.name);
                        }
                      }}
                    />
                  </>
                )}
              </div>
            </div>
          )}

          <div className="space-y-3 animate-in slide-in-from-bottom-2 duration-300 delay-200">
            <Label>AI Detection Types</Label>
            <div className="w-full flex flex-col items-center gap-4">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleSelectAllDetections}
                className="transition-all duration-200 hover:scale-105"
              >
                {selectedDetections.length === detectionOptions.length ? 'Deselect All' : 'Select All'}
              </Button>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-x-6 gap-y-3 w-full max-w-md">
              {detectionOptions.map((option, index) => (
                <div
                  key={option.value}
                    className="flex items-center justify-center gap-2 p-2 rounded-lg hover:bg-white/50 dark:hover:bg-gray-800/50 transition-all duration-200 animate-in slide-in-from-left-2"
                    style={{ animationDelay: `${350 + index * 50}ms` }}
                >
                  <Checkbox
                    id={option.value}
                    checked={selectedDetections.includes(option.value)}
                    onCheckedChange={(checked) =>
                      handleDetectionChange(option.value, checked as boolean)
                    }
                    className="transition-all duration-200 hover:scale-110"
                  />
                  <Label
                    htmlFor={option.value}
                    className="text-sm font-medium cursor-pointer"
                  >
                    {option.label}
                  </Label>
                </div>
              ))}
              </div>
            </div>
            {errors.detectionTypes && (
              <p className="text-sm text-red-600 dark:text-red-400 animate-in slide-in-from-left-2 duration-200">
                {errors.detectionTypes.message}
              </p>
            )}
          </div>

          <div className="flex gap-3 pt-6 animate-in slide-in-from-bottom-2 duration-300 delay-300">
            <Button
              type="submit"
              className="bg-blue-600 hover:bg-blue-700 dark:bg-neon-cyan/80 dark:hover:bg-neon-cyan dark:shadow-neon-cyan/30 transition-all duration-200 hover:scale-105 hover:shadow-lg dark:hover:shadow-neon-cyan/50"
            >
              {editingCamera ? (
                <Save className="h-4 w-4 mr-2" />
              ) : (
                <Plus className="h-4 w-4 mr-2" />
              )}
              {editingCamera ? "Update Camera" : "Add Camera"}
            </Button>
            {editingCamera && (
              <Button
                type="button"
                variant="outline"
                onClick={onStopEditing}
                className="transition-all duration-200 hover:scale-105"
              >
                Cancel
              </Button>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
