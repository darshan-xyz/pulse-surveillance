import { useState, useEffect } from "react";

export interface SurveillanceSettings {
  // Core toggles
  notificationsEnabled: boolean;
  alertsEnabled: boolean;
  soundEnabled: boolean;

      // Recording settings
    autoRecordOnDetection: boolean;
    recordingQuality: "720p" | "1080p" | "4k";
  storageLocation: string;
  dataRetentionDays: number;

  // Emergency settings
  emergencyContacts: string[];
  emailNotifications: boolean;
  smsNotifications: boolean;
}

const defaultSettings: SurveillanceSettings = {
  notificationsEnabled: true,
  alertsEnabled: true,
  soundEnabled: true,
  autoRecordOnDetection: true,
  recordingQuality: "1080p",
  storageLocation: "/recordings",
  dataRetentionDays: 30,
  emergencyContacts: [],
  emailNotifications: false,
  smsNotifications: false,
};

export function useSettings() {
  const [settings, setSettings] =
    useState<SurveillanceSettings>(defaultSettings);

  // Load settings from localStorage on mount
  useEffect(() => {
    const savedSettings = localStorage.getItem("surveillance-settings");
    if (savedSettings) {
      try {
        const parsed = JSON.parse(savedSettings);
        setSettings({ ...defaultSettings, ...parsed });
      } catch (error) {
        console.error("Failed to parse saved settings:", error);
      }
    }
  }, []);

  // Save settings to localStorage whenever they change
  useEffect(() => {
    localStorage.setItem("surveillance-settings", JSON.stringify(settings));
  }, [settings]);

  const updateSetting = <K extends keyof SurveillanceSettings>(
    key: K,
    value: SurveillanceSettings[K],
  ) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  };

  const resetSettings = () => {
    setSettings(defaultSettings);
  };

  return {
    settings,
    updateSetting,
    resetSettings,
  };
}
