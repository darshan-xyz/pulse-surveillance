import React from "react";
import { useState } from "react";
import {
  Settings,
  Bell,
  AlertTriangle,
  Volume2,
  VolumeX,
  Camera,
  Mail,
  Phone,
  HardDrive,
  RotateCcw,
  X,
  LogOut,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import { useSettings } from "@/hooks/use-settings";
import { useUserProfile } from "@/hooks/use-user-profile.tsx";
import { supabase } from "@/lib/supabaseClient";

interface SettingsPanelProps {
  isOpen: boolean;
  onClose: () => void;
}

export function SettingsPanel({ isOpen, onClose }: SettingsPanelProps) {
  const { settings, updateSetting, resetSettings } = useSettings();
  const { clearProfile } = useUserProfile();
  const [newContact, setNewContact] = useState("");
  const [showSignOutConfirm, setShowSignOutConfirm] = useState(false);

  if (!isOpen) return null;

  const addEmergencyContact = () => {
    if (
      newContact.trim() &&
      !settings.emergencyContacts.includes(newContact.trim())
    ) {
      updateSetting("emergencyContacts", [
        ...settings.emergencyContacts,
        newContact.trim(),
      ]);
      setNewContact("");
    }
  };

  const removeEmergencyContact = (contact: string) => {
    updateSetting(
      "emergencyContacts",
      settings.emergencyContacts.filter((c) => c !== contact),
    );
  };

  const handleSignOut = () => {
    setShowSignOutConfirm(true);
  };

  const confirmSignOut = async () => {
    // Clear any stored authentication data
    localStorage.removeItem("surveillance-settings");
    // Clear profile data
    clearProfile();
    // Sign out from Supabase
    await supabase.auth.signOut();
    // Redirect to login page
    window.location.href = "/login";
  };

  const cancelSignOut = () => {
    setShowSignOutConfirm(false);
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in-0 duration-300">
      <Card className="w-full max-w-4xl max-h-[90vh] overflow-hidden bg-white dark:bg-gray-900 border-blue-200 dark:border-neon-purple/30 animate-in zoom-in-95 slide-in-from-bottom-8 duration-500">
        <CardHeader className="flex flex-row items-center justify-between border-b border-gray-200 dark:border-gray-700">
          <CardTitle className="flex items-center gap-2 text-xl">
            <Settings className="h-6 w-6 text-blue-600 dark:text-neon-cyan" />
            Surveillance Settings
          </CardTitle>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleSignOut}
              className="text-red-600 dark:text-red-400 border-red-200 dark:border-red-800 hover:bg-red-50 dark:hover:bg-red-900/20"
            >
              <LogOut className="h-4 w-4 mr-2" />
              Sign Out
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={resetSettings}
              className="text-gray-600 dark:text-gray-300"
            >
              <RotateCcw className="h-4 w-4 mr-2" />
              Reset
            </Button>
            <Button variant="ghost" size="sm" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>

        <CardContent className="p-6 overflow-y-auto max-h-[calc(90vh-100px)]">
          <div className="space-y-8">
            {/* Core Settings */}
            <section>
              <h3 className="text-lg font-semibold mb-4 flex items-center gap-2">
                <Bell className="h-5 w-5 text-blue-600 dark:text-neon-cyan" />
                Core Settings
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-4">
                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      <Bell className="h-4 w-4" />
                      <Label htmlFor="notifications">Notifications</Label>
                    </div>
                    <Switch
                      id="notifications"
                      checked={settings.notificationsEnabled}
                      onCheckedChange={(checked) =>
                        updateSetting("notificationsEnabled", checked)
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      <AlertTriangle className="h-4 w-4" />
                      <Label htmlFor="alerts">Security Alerts</Label>
                    </div>
                    <Switch
                      id="alerts"
                      checked={settings.alertsEnabled}
                      onCheckedChange={(checked) =>
                        updateSetting("alertsEnabled", checked)
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      {settings.soundEnabled ? (
                        <Volume2 className="h-4 w-4" />
                      ) : (
                        <VolumeX className="h-4 w-4" />
                      )}
                      <Label htmlFor="sound">Alert Sounds</Label>
                    </div>
                    <Switch
                      id="sound"
                      checked={settings.soundEnabled}
                      onCheckedChange={(checked) =>
                        updateSetting("soundEnabled", checked)
                      }
                    />
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      <Camera className="h-4 w-4" />
                      <Label htmlFor="autoSave">Auto-Save on Detection</Label>
                    </div>
                    <Switch
                      id="autoSave"
                      checked={settings.autoRecordOnDetection}
                      onCheckedChange={(checked) =>
                        updateSetting("autoRecordOnDetection", checked)
                      }
                    />
                  </div>
                </div>
              </div>
            </section>

            <Separator />

            {/* Recording Settings */}
            <section>
              <h3 className="text-lg font-semibold mb-4 flex items-center gap-2">
                <HardDrive className="h-5 w-5 text-blue-600 dark:text-neon-cyan" />
                Recording & Storage
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>Recording Quality</Label>
                    <Select
                      value={settings.recordingQuality}
                      onValueChange={(value) =>
                        updateSetting("recordingQuality", value as any)
                      }
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="720p">720p HD</SelectItem>
                        <SelectItem value="1080p">1080p Full HD</SelectItem>
                        <SelectItem value="4k">4K Ultra HD</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2">
                    <Label>Storage Location</Label>
                    <Input
                      value={settings.storageLocation}
                      onChange={(e) =>
                        updateSetting("storageLocation", e.target.value)
                      }
                      placeholder="/recordings"
                    />
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label>
                      Data Retention (Days): {settings.dataRetentionDays}
                    </Label>
                    <Slider
                      value={[settings.dataRetentionDays]}
                      onValueChange={([value]) =>
                        updateSetting("dataRetentionDays", value)
                      }
                      min={1}
                      max={365}
                      step={1}
                      className="w-full"
                    />
                  </div>
                </div>
              </div>
            </section>

            <Separator />

            {/* Communication Settings */}
            <section>
              <h3 className="text-lg font-semibold mb-4 flex items-center gap-2">
                <Mail className="h-5 w-5 text-blue-600 dark:text-neon-cyan" />
                Communication
              </h3>
              <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      <Mail className="h-4 w-4" />
                      <Label htmlFor="email">Email Notifications</Label>
                    </div>
                    <Switch
                      id="email"
                      checked={settings.emailNotifications}
                      onCheckedChange={(checked) =>
                        updateSetting("emailNotifications", checked)
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-800/50 rounded-lg">
                    <div className="flex items-center gap-3">
                      <Phone className="h-4 w-4" />
                      <Label htmlFor="sms">SMS Notifications</Label>
                    </div>
                    <Switch
                      id="sms"
                      checked={settings.smsNotifications}
                      onCheckedChange={(checked) =>
                        updateSetting("smsNotifications", checked)
                      }
                    />
                  </div>
                </div>

                <div className="space-y-3">
                  <Label>Emergency Contacts</Label>
                  <div className="flex gap-2">
                    <Input
                      value={newContact}
                      onChange={(e) => setNewContact(e.target.value)}
                      placeholder="Add email or phone number"
                      onKeyPress={(e) =>
                        e.key === "Enter" && addEmergencyContact()
                      }
                    />
                    <Button onClick={addEmergencyContact} variant="outline">
                      Add
                    </Button>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {settings.emergencyContacts.map((contact, index) => (
                      <Badge
                        key={index}
                        variant="secondary"
                        className="cursor-pointer hover:bg-red-100 dark:hover:bg-red-900/30"
                        onClick={() => removeEmergencyContact(contact)}
                      >
                        {contact} ×
                      </Badge>
                    ))}
                  </div>
                </div>
              </div>
            </section>
          </div>
        </CardContent>
      </Card>

      {/* Sign Out Confirmation Modal */}
      {showSignOutConfirm && (
        <div className="fixed inset-0 z-[60] bg-black/50 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in-0 duration-300">
          <div className="bg-white dark:bg-gray-900 rounded-xl p-6 max-w-md w-full mx-4 animate-in zoom-in-95 slide-in-from-bottom-8 duration-500 border border-red-200 dark:border-red-800/50">
            <div className="flex items-center gap-3 mb-4">
              <div className="p-2 bg-red-100 dark:bg-red-900/30 rounded-full">
                <LogOut className="h-5 w-5 text-red-600 dark:text-red-400" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                Sign Out
              </h3>
            </div>

            <p className="text-gray-600 dark:text-gray-300 mb-6">
              Are you sure you want to sign out? You will be redirected to the
              login page.
            </p>

            <div className="flex gap-3 justify-end">
              <Button
                variant="outline"
                onClick={cancelSignOut}
                className="hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                Cancel
              </Button>
              <Button
                onClick={confirmSignOut}
                className="bg-red-600 hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600 text-white"
              >
                Sign Out
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
