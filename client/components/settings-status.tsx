import React from "react";
import { Bell, AlertTriangle, Volume2, VolumeX } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { useSettings } from "@/hooks/use-settings";

export function SettingsStatus() {
  const { settings } = useSettings();

  const statusItems = [
    {
      enabled: settings.notificationsEnabled,
      icon: Bell,
      label: "Notifications",
      color:
        "bg-blue-100 dark:bg-neon-cyan/10 text-blue-700 dark:text-neon-cyan",
      disabledColor:
        "bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400",
    },
    {
      enabled: settings.alertsEnabled,
      icon: AlertTriangle,
      label: "Alerts",
      color: "bg-red-100 dark:bg-red-900/20 text-red-700 dark:text-red-300",
      disabledColor:
        "bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400",
    },
    {
      enabled: settings.soundEnabled,
      icon: settings.soundEnabled ? Volume2 : VolumeX,
      label: "Sound",
      color:
        "bg-green-100 dark:bg-neon-green/10 text-green-700 dark:text-neon-green",
      disabledColor:
        "bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400",
    },
  ];

  return (
    <div className="flex gap-2">
      {statusItems.map((item) => {
        const Icon = item.icon;
        return (
          <Badge
            key={item.label}
            variant="secondary"
            className={`flex items-center gap-1 px-2 py-1 text-xs ${
              item.enabled ? item.color : item.disabledColor
            } border-0 ${!item.enabled ? "opacity-60" : ""}`}
          >
            <Icon className="h-3 w-3" />
            {item.label}
          </Badge>
        );
      })}
    </div>
  );
}
