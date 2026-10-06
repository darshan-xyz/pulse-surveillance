import React from "react";
import { AlertTriangle, Flame, Wind, Zap, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Camera } from "@/lib/types";

interface DetectionAlertsProps {
  cameras: Camera[];
}

const detectionIcons = {
  fire: Flame,
  smoke: Wind,
  weapon: Zap,
  person: AlertTriangle,
  animal: AlertTriangle,
};

const detectionColors = {
  fire: "bg-red-500 text-white",
  smoke: "bg-gray-500 text-white",
  weapon: "bg-orange-500 text-white",
  person: "bg-blue-500 text-white",
  animal: "bg-green-500 text-white",
};

export function DetectionAlerts({ cameras }: DetectionAlertsProps) {
  const [isVisible, setIsVisible] = React.useState(true);

  if (!isVisible) return null;

  // Get all unique detection types from all cameras
  const allDetectionTypes = Array.from(
    new Set(
      cameras
        .flatMap((camera) => camera.detectionTypes || [])
        .filter(Boolean)
    )
  );

  if (allDetectionTypes.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-50 animate-in slide-in-from-right-2 duration-500">
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg p-4 max-w-sm">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
            AI Detection Active
          </h3>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setIsVisible(false)}
            className="h-6 w-6 p-0 hover:bg-gray-100 dark:hover:bg-gray-700"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
        
        <div className="space-y-2">
          <p className="text-xs text-gray-600 dark:text-gray-400">
            Monitoring {cameras.length} camera{cameras.length !== 1 ? 's' : ''} for:
          </p>
          
          <div className="flex flex-wrap gap-2">
            {allDetectionTypes.map((type) => {
              const Icon = detectionIcons[type as keyof typeof detectionIcons] || AlertTriangle;
              const colorClass = detectionColors[type as keyof typeof detectionColors] || "bg-gray-500 text-white";
              
              return (
                <Badge
                  key={type}
                  className={`${colorClass} text-xs flex items-center gap-1 animate-pulse`}
                  style={{ animationDuration: "2s" }}
                >
                  <Icon className="h-3 w-3" />
                  {type.charAt(0).toUpperCase() + type.slice(1)}
                </Badge>
              );
            })}
          </div>
          
          <div className="text-xs text-gray-500 dark:text-gray-400 mt-3 pt-2 border-t border-gray-200 dark:border-gray-700">
            Detection system is configured and ready
          </div>
        </div>
      </div>
    </div>
  );
}

