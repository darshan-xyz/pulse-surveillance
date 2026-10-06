import React, { useState } from 'react';
import { Settings, Edit, Video, VideoOff, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Camera } from '@/lib/types';

interface CameraSettingsProps {
  camera: Camera;
  isOpen: boolean;
  onClose: () => void;
  onEdit: (camera: Camera) => void;

}

export function CameraSettings({
  camera,
  isOpen,
  onClose,
  onEdit,

}: CameraSettingsProps) {


  const handleEditCamera = () => {
    onEdit(camera);
    onClose();
  };



  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in-0 duration-300">
      <div className="w-full max-w-md animate-in zoom-in-95 slide-in-from-bottom-8 duration-500">
        <Card className="border-2 border-blue-200/50 dark:border-blue-700/50 shadow-2xl">
          <CardHeader className="pb-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-blue-100 dark:bg-blue-900/30 rounded-full">
                  <Settings className="h-5 w-5 text-blue-600 dark:text-blue-400" />
                </div>
                <div>
                  <CardTitle className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                    Camera Settings
                  </CardTitle>
                  <CardDescription className="text-sm text-gray-600 dark:text-gray-400">
                    Configure {camera.name}
                  </CardDescription>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={onClose}
                className="h-8 w-8 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>

          <CardContent className="space-y-6">
            {/* Camera Info */}
            <div className="space-y-3">
              <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                <div>
                  <div className="font-medium text-gray-900 dark:text-gray-100">
                    {camera.name}
                  </div>
                  <div className="text-sm text-gray-500 dark:text-gray-400 capitalize">
                    {camera.source} camera
                  </div>
                </div>
                <Badge 
                  variant={camera.isActive ? "default" : "secondary"}
                  className={camera.isActive ? "bg-green-500" : "bg-gray-500"}
                >
                  {camera.isActive ? 'Active' : 'Inactive'}
                </Badge>
              </div>
            </div>

            {/* Settings Options */}
            <div className="space-y-4">
              {/* Edit Camera */}
              <Button
                variant="outline"
                onClick={handleEditCamera}
                className="w-full justify-start h-12 text-left"
              >
                <Edit className="h-4 w-4 mr-3 text-blue-600" />
                <div>
                  <div className="font-medium">Edit Camera</div>
                  <div className="text-sm text-gray-500">Modify camera settings and configuration</div>
                </div>
              </Button>


            </div>

            {/* Camera Details */}
            <div className="space-y-2 pt-4 border-t border-gray-200 dark:border-gray-700">
              <div className="text-sm text-gray-600 dark:text-gray-400">
                <span className="font-medium">Source:</span> {camera.source}
              </div>
              {camera.url && (
                <div className="text-sm text-gray-600 dark:text-gray-400">
                  <span className="font-medium">URL:</span> 
                  <div className="text-xs text-gray-500 mt-1 break-all">
                    {camera.url}
                  </div>
                </div>
              )}
              {camera.detectionTypes && camera.detectionTypes.length > 0 && (
                <div className="text-sm text-gray-600 dark:text-gray-400">
                  <span className="font-medium">Detection Types:</span>
                  <div className="flex flex-wrap gap-1 mt-1">
                    {camera.detectionTypes.map((type) => (
                      <Badge key={type} variant="secondary" className="text-xs">
                        {type}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}

            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
} 