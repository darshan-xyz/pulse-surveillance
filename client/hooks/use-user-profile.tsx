import React, { useState, useEffect, useContext, createContext, ReactNode } from "react";
import { linkPhoneToEmail, supabase } from "@/lib/supabaseClient";
import { toast } from "sonner";

export interface UserProfile {
  firstName: string;
  lastName: string;
  email: string;
  phone?: string;
  avatar?: string;
  preferences: {
    notifications: boolean;
    emailAlerts: boolean;
    smsAlerts: boolean;
    theme: "light" | "dark" | "system";
  };
  emergencyContacts: Array<{
    id: string;
    name: string;
    phone: string;
    relationship: string;
  }>;
}

const DEFAULT_PROFILE: UserProfile = {
  firstName: "",
  lastName: "",
  email: "",
  phone: "",
  avatar: "",
  preferences: {
    notifications: true,
    emailAlerts: true,
    smsAlerts: false,
    theme: "system",
  },
  emergencyContacts: [],
};

const USER_PROFILE_KEY = "surveillanceUserProfile";

interface UserProfileContextType {
  profile: UserProfile;
  isLoading: boolean;
  updateProfile: (updates: Partial<UserProfile>) => Promise<{ error?: any }>;
  updatePreferences: (preferences: Partial<UserProfile["preferences"]>) => void;
  addEmergencyContact: (contact: Omit<UserProfile["emergencyContacts"][0], "id">) => void;
  removeEmergencyContact: (id: string) => void;
  updateEmergencyContact: (id: string, updates: Partial<UserProfile["emergencyContacts"][0]>) => void;
  getDisplayName: () => string;
  getInitials: () => string;
  clearProfile: () => void;
  clearAllProfileData: () => void;
}

const UserProfileContext = createContext<UserProfileContextType | undefined>(undefined);

export function UserProfileProvider({ children }: { children: ReactNode }) {
  const [profile, setProfile] = useState<UserProfile>(DEFAULT_PROFILE);
  const [isLoading, setIsLoading] = useState(true);
  const [currentUserId, setCurrentUserId] = useState<string | null>(null);

  // Get current user ID from Supabase
  useEffect(() => {
    const getUser = async () => {
      const { data: { user } } = await supabase.auth.getUser();
      setCurrentUserId(user?.id || null);
    };
    getUser();

    const { data: { subscription } } = supabase.auth.onAuthStateChange((_event, session) => {
      setCurrentUserId(session?.user?.id || null);
    });

    return () => subscription.unsubscribe();
  }, []);

  // Load profile from localStorage on mount and when user changes
  useEffect(() => {
    if (!currentUserId) {
      setProfile(DEFAULT_PROFILE);
      setIsLoading(false);
      return;
    }

    const userSpecificKey = `${USER_PROFILE_KEY}_${currentUserId}`;
    let savedProfile = localStorage.getItem(userSpecificKey);
    
    // Migration: Check for old profile data and migrate it
    if (!savedProfile) {
      const oldProfile = localStorage.getItem(USER_PROFILE_KEY);
      if (oldProfile) {
        try {
          const parsed = JSON.parse(oldProfile);
          // Save to new user-specific key
          localStorage.setItem(userSpecificKey, oldProfile);
          // Remove old key
          localStorage.removeItem(USER_PROFILE_KEY);
          savedProfile = oldProfile;
        } catch (error) {
          console.error("Failed to migrate old profile data:", error);
        }
      }
    }
    
    if (savedProfile) {
      try {
        const parsed = JSON.parse(savedProfile);
        setProfile({ ...DEFAULT_PROFILE, ...parsed });
      } catch (error) {
        console.error("Failed to parse user profile:", error);
      }
    }
    setIsLoading(false);
  }, [currentUserId]);

  // Save profile to localStorage whenever it changes
  useEffect(() => {
    if (!isLoading && currentUserId) {
      const userSpecificKey = `${USER_PROFILE_KEY}_${currentUserId}`;
      localStorage.setItem(userSpecificKey, JSON.stringify(profile));
    }
  }, [profile, isLoading, currentUserId]);

  const updateProfile = async (updates: Partial<UserProfile>) => {
    // Only call backend if phone or email is present and changed
    const prevEmail = profile.email;
    const prevPhone = profile.phone;
    const newEmail = updates.email ?? prevEmail;
    const newPhone = updates.phone ?? prevPhone;
    setProfile((prev) => ({ ...prev, ...updates }));
    if (newEmail && newPhone && (newEmail !== prevEmail || newPhone !== prevPhone)) {
      try {
        const { error } = await linkPhoneToEmail({ email: newEmail, phone: newPhone });
        if (error) {
          if (error.message && error.message.includes('duplicate key value violates unique constraint')) {
            toast.error('This phone number is already linked to another account.');
          } else {
            toast.error("Failed to link phone to email: " + error.message);
          }
          return { error };
        } else {
          toast.success("Phone number linked to email.");
        }
      } catch (e) {
        toast.error("Failed to link phone to email.");
        return { error: e };
      }
    }
    return {};
  };

  const updatePreferences = (preferences: Partial<UserProfile["preferences"]>) => {
    setProfile((prev) => ({
      ...prev,
      preferences: { ...prev.preferences, ...preferences },
    }));
  };

  const addEmergencyContact = (contact: Omit<UserProfile["emergencyContacts"][0], "id">) => {
    const newContact = {
      ...contact,
      id: Date.now().toString(),
    };
    setProfile((prev) => ({
      ...prev,
      emergencyContacts: [...prev.emergencyContacts, newContact],
    }));
  };

  const removeEmergencyContact = (id: string) => {
    setProfile((prev) => ({
      ...prev,
      emergencyContacts: prev.emergencyContacts.filter((contact) => contact.id !== id),
    }));
  };

  const updateEmergencyContact = (id: string, updates: Partial<UserProfile["emergencyContacts"][0]>) => {
    setProfile((prev) => ({
      ...prev,
      emergencyContacts: prev.emergencyContacts.map((contact) =>
        contact.id === id ? { ...contact, ...updates } : contact
      ),
    }));
  };

  const getDisplayName = () => {
    if (profile.firstName && profile.lastName) {
      return `${profile.firstName} ${profile.lastName}`;
    }
    if (profile.firstName) {
      return profile.firstName;
    }
    return "User";
  };

  const getInitials = () => {
    const firstName = profile.firstName || "U";
    const lastName = profile.lastName || "";
    return `${firstName.charAt(0)}${lastName.charAt(0)}`.toUpperCase();
  };



  const clearProfile = () => {
    setProfile(DEFAULT_PROFILE);
    // Clear all profile data from localStorage
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && key.startsWith(USER_PROFILE_KEY)) {
        localStorage.removeItem(key);
      }
    }
  };

  const clearAllProfileData = () => {
    // Clear all profile-related data
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && (key.startsWith(USER_PROFILE_KEY) || key === "surveillanceUserProfile")) {
        localStorage.removeItem(key);
      }
    }
    setProfile(DEFAULT_PROFILE);
  };

  return (
    <UserProfileContext.Provider value={{ profile, isLoading, updateProfile, updatePreferences, addEmergencyContact, removeEmergencyContact, updateEmergencyContact, getDisplayName, getInitials, clearProfile, clearAllProfileData }}>
      {children}
    </UserProfileContext.Provider>
  );
}

export function useUserProfile() {
  const context = useContext(UserProfileContext);
  if (!context) {
    throw new Error("useUserProfile must be used within a UserProfileProvider");
  }
  return context;
}
