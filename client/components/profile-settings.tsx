import React from "react";
import { useState } from "react";
import {
  User,
  Mail,
  Phone,
  Bell,
  Users,
  Save,
  Plus,
  Trash2,
  Camera,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { useUserProfile } from "@/hooks/use-user-profile.tsx";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";
import { useSupabaseUserId } from "@/lib/useSupabaseUserId";

interface ProfileSettingsProps {
  onClose: () => void;
}

export function ProfileSettings({ onClose }: ProfileSettingsProps) {
  const {
    profile,
    updateProfile,
    updatePreferences,
    addEmergencyContact,
    removeEmergencyContact,
    getInitials,
  } = useUserProfile();
  
  const { userId } = useSupabaseUserId();
  const userSchema = userId ? `user_${userId.substring(0, 8)}` : '';

  const [formData, setFormData] = useState(profile);
  // Keep formData in sync with profile (for pre-filling after sign up)
  React.useEffect(() => {
    setFormData(profile);
  }, [profile]);
  const [newContact, setNewContact] = useState({
    name: "",
    phone: "",
    relationship: "",
  });
  const [showAddContact, setShowAddContact] = useState(false);
  const navigate = useNavigate();

  const handleSave = async () => {
    // Require all fields to be filled
    if (!formData.firstName || !formData.lastName || !formData.email || !formData.phone) {
      toast.error("Please fill in all required fields (First Name, Last Name, Email, Phone).");
      return;
    }
    // updateProfile now returns a promise
    const result = await updateProfile(formData);
    // Only show success toast if no error was shown
    if (!result?.error) {
      toast.success("Profile updated successfully!");
      onClose(); // Automatically close after saving
      navigate("/"); // Redirect to home page after closing
    }
  };

  const handlePreferenceChange = (key: string, value: boolean) => {
    const newPreferences = { ...formData.preferences, [key]: value };
    setFormData({ ...formData, preferences: newPreferences });
    updatePreferences({ [key]: value });
  };

  const handleAddContact = () => {
    if (newContact.name && newContact.phone) {
      addEmergencyContact(newContact);
      setNewContact({ name: "", phone: "", relationship: "" });
      setShowAddContact(false);
      toast.success("Emergency contact added!");
    }
  };

  const handleRemoveContact = (id: string) => {
    removeEmergencyContact(id);
    toast.success("Emergency contact removed!");
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="w-full max-w-4xl max-h-[90vh] overflow-y-auto">
        <Card className="bg-white dark:bg-gray-900">
          <CardHeader className="flex flex-row items-center justify-between pb-4">
            <CardTitle className="flex items-center gap-2">
              <User className="h-5 w-5" />
              Profile Settings
            </CardTitle>
            <Button variant="ghost" size="sm" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
          </CardHeader>
          <CardContent className="space-y-6">
            {/* Profile Information */}
            <div className="space-y-4">
              <h3 className="text-lg font-semibold">Personal Information</h3>

              {/* Avatar Section */}
              <div className="flex items-center gap-4">
                <div className="h-20 w-20 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white text-xl font-bold">
                  {getInitials()}
                </div>
                <div>
                  <Button variant="outline" size="sm">
                    <Camera className="h-4 w-4 mr-2" />
                    Change Avatar
                  </Button>
                  <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">
                    Upload a profile picture
                  </p>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="firstName">First Name</Label>
                  <Input
                    id="firstName"
                    value={formData.firstName}
                    onChange={(e) =>
                      setFormData({ ...formData, firstName: e.target.value })
                    }
                    placeholder="Enter your first name"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="lastName">Last Name</Label>
                  <Input
                    id="lastName"
                    value={formData.lastName}
                    onChange={(e) =>
                      setFormData({ ...formData, lastName: e.target.value })
                    }
                    placeholder="Enter your last name"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="email">Email</Label>
                  <Input
                    id="email"
                    type="email"
                    value={formData.email || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, email: e.target.value })
                    }
                    placeholder="Enter your email"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="phone">Phone Number</Label>
                  <Input
                    id="phone"
                    type="tel"
                    value={formData.phone || ""}
                    onChange={(e) =>
                      setFormData({ ...formData, phone: e.target.value })
                    }
                    placeholder="Enter your phone number"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="schema">User ID</Label>
                  <Input
                    id="schema"
                    value={userSchema}
                    readOnly
                    className="bg-gray-50 dark:bg-gray-800 cursor-not-allowed"
                    placeholder="Your unique user identifier"
                  />
                  <p className="text-xs text-gray-500 dark:text-gray-400">
                    Your unique identifier in the system
                  </p>
                </div>
              </div>
            </div>

            {/* Notification Preferences */}
            <div className="space-y-4">
              <h3 className="text-lg font-semibold flex items-center gap-2">
                <Bell className="h-5 w-5" />
                Notification Preferences
              </h3>
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <Label htmlFor="notifications">Push Notifications</Label>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Receive real-time detection alerts
                    </p>
                  </div>
                  <Switch
                    id="notifications"
                    checked={formData.preferences.notifications}
                    onCheckedChange={(checked) =>
                      handlePreferenceChange("notifications", checked)
                    }
                  />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label htmlFor="emailAlerts">Email Alerts</Label>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Get alerts via email
                    </p>
                  </div>
                  <Switch
                    id="emailAlerts"
                    checked={formData.preferences.emailAlerts}
                    onCheckedChange={(checked) =>
                      handlePreferenceChange("emailAlerts", checked)
                    }
                  />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <Label htmlFor="smsAlerts">SMS Alerts</Label>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Receive critical alerts via SMS
                    </p>
                  </div>
                  <Switch
                    id="smsAlerts"
                    checked={formData.preferences.smsAlerts}
                    onCheckedChange={(checked) =>
                      handlePreferenceChange("smsAlerts", checked)
                    }
                  />
                </div>
              </div>
            </div>

            {/* Emergency Contacts */}
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold flex items-center gap-2">
                  <Users className="h-5 w-5" />
                  Emergency Contacts
                </h3>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowAddContact(true)}
                >
                  <Plus className="h-4 w-4 mr-2" />
                  Add Contact
                </Button>
              </div>

              {/* Existing Contacts */}
              <div className="space-y-2">
                {profile.emergencyContacts.map((contact) => (
                  <div
                    key={contact.id}
                    className="flex items-center justify-between p-3 border rounded-lg"
                  >
                    <div>
                      <div className="font-medium">{contact.name}</div>
                      <div className="text-sm text-gray-600 dark:text-gray-400">
                        {contact.phone} • {contact.relationship}
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRemoveContact(contact.id)}
                      className="text-red-600 hover:text-red-700"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}

                {profile.emergencyContacts.length === 0 && (
                  <p className="text-gray-600 dark:text-gray-400 text-center py-4">
                    No emergency contacts added yet
                  </p>
                )}
              </div>

              {/* Add Contact Form */}
              {showAddContact && (
                <Card className="bg-gray-50 dark:bg-gray-800">
                  <CardContent className="p-4 space-y-3">
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                      <Input
                        placeholder="Contact name"
                        value={newContact.name}
                        onChange={(e) =>
                          setNewContact({ ...newContact, name: e.target.value })
                        }
                      />
                      <Input
                        placeholder="Phone number"
                        value={newContact.phone}
                        onChange={(e) =>
                          setNewContact({
                            ...newContact,
                            phone: e.target.value,
                          })
                        }
                      />
                      <Select
                        value={newContact.relationship}
                        onValueChange={(value) =>
                          setNewContact({ ...newContact, relationship: value })
                        }
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="Relationship" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="family">Family</SelectItem>
                          <SelectItem value="friend">Friend</SelectItem>
                          <SelectItem value="neighbor">Neighbor</SelectItem>
                          <SelectItem value="colleague">Colleague</SelectItem>
                          <SelectItem value="emergency">
                            Emergency Service
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="flex gap-2">
                      <Button size="sm" onClick={handleAddContact}>
                        Add Contact
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setShowAddContact(false)}
                      >
                        Cancel
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>

            {/* Save Button */}
            <div className="flex justify-end gap-3 pt-4 border-t">
              <Button variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button
                onClick={handleSave}
                className="bg-blue-600 hover:bg-blue-700"
              >
                <Save className="h-4 w-4 mr-2" />
                Save Changes
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
