import {
  Camera as CameraIcon,
  Activity,
  Settings,
  Plus,
  History,
  User,
  Mail,
  Phone,
  MapPin,
  Shield,
  Lock,
  Eye,
  Facebook,
  Twitter,
  Linkedin,
  Instagram,
  Award,
  MessageCircle,
  Send,
  Globe,
  ChevronDown,
  Star,
  Users2,
  Clock,
  Zap,
  Video,
  Power,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { CameraManagement } from "@/components/camera-management";
import { Link } from "react-router-dom";
import { CameraGrid } from "@/components/camera-grid";
import { ThemeToggle } from "@/components/theme-toggle";
import { SurveillanceLogo } from "@/components/surveillance-logo";
import { DetectionAlerts } from "@/components/detection-alerts";
import { SettingsPanel } from "@/components/settings-panel";
import { ProfileSettings } from "@/components/profile-settings";
import { VideoFeedCard } from "@/components/video-feed-card";
import { useCameras } from "@/hooks/use-cameras";
import { useSettings } from "@/hooks/use-settings";
import { useUserProfile } from "@/hooks/use-user-profile.tsx";
import { ParallaxBackground } from "@/components/ParallaxBackground";
import { ScrollToTopButton } from "@/components/ScrollToTopButton";
import React, { useState, useRef, useEffect } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { multiTenantClient, MultiTenantCamera } from '@/lib/multiTenantClient';
import { addCamera, fetchCameras, updateCamera, deleteCamera } from '@/lib/supabaseCameras';
import { Camera, DetectionType } from '@/lib/types';

import { VideoRecorderUpload } from '@/components/VideoRecorderUpload';
import { subscribeToHistoryEvents } from '@/lib/supabaseNotifications';
import { useSupabaseUserId } from '@/lib/useSupabaseUserId';
import { logSupabaseDiagnostics } from '@/lib/supabaseDiagnostics';
import { useNavigate } from 'react-router-dom';

// Helper function to convert MultiTenantCamera to Camera
function convertMultiTenantToCamera(multiTenantCamera: MultiTenantCamera): Camera {
  console.log('Converting camera:', multiTenantCamera.name, 'Detection types:', multiTenantCamera.detectionTypes);
  return {
    id: multiTenantCamera.id,
    name: multiTenantCamera.name,
    source: multiTenantCamera.source,
    url: multiTenantCamera.url,
    detectionTypes: (multiTenantCamera.detectionTypes || []) as DetectionType[],
    isActive: multiTenantCamera.isActive,
    createdAt: multiTenantCamera.createdAt,

  };
}

// LiveClock component
function LiveClock() {
  const [time, setTime] = useState(new Date());
  useEffect(() => {
    const interval = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(interval);
  }, []);
  return (
    <div className="flex items-center gap-1 text-blue-700 dark:text-cyan-300 font-mono text-lg font-semibold animate-in fade-in-0 slide-in-from-left-2 duration-300 delay-300">
      <Clock className="h-5 w-5 mr-1" />
      {time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
    </div>
  );
}

export default function Index() {
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isAddCameraOpen, setIsAddCameraOpen] = useState(false);
  const [isProfileOpen, setIsProfileOpen] = useState(false); // Ensure initialized to false
  const { getDisplayName, profile } = useUserProfile();
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [editingCamera, setEditingCamera] = useState(null);
  const [showSkeleton, setShowSkeleton] = useState(true);
  const [addWarning, setAddWarning] = useState('');
  const [diagnosticWarning, setDiagnosticWarning] = useState('');
  const { userId, restoring } = useSupabaseUserId();
  const navigate = useNavigate();
  const [showProfileModalOnce, setShowProfileModalOnce] = useState(() => localStorage.getItem('showProfileModalOnce') === 'true');

  useEffect(() => {
    let isMounted = true;
    
    async function loadCameras() {
      try {
        const cams = await fetchCameras(userId);
        if (isMounted) {
          // Only log in debug mode
          if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
            console.log('Loaded cameras:', cams);
          }
          setCameras(cams);
        }
      } catch (error) {
        console.error('Error loading cameras:', error);
      }
    }
    
    // Only load cameras if user is authenticated
    if (userId) {
      loadCameras();
    }
    
    return () => {
      isMounted = false;
    };
  }, [userId]); // Add userId as dependency

  useEffect(() => {
    logSupabaseDiagnostics().then((msg) => {
      if (msg) setDiagnosticWarning(msg);
    });
  }, []);

  useEffect(() => {
    if (diagnosticWarning === 'No Supabase user is logged in.') {
      navigate('/login');
    }
  }, [diagnosticWarning, navigate]);

  // ---
  // Supabase SQL for cameras table (run in SQL editor):
  //
  // create table if not exists cameras (
  //   id uuid primary key default uuid_generate_v4(),
  //   user_id uuid references auth.users not null,
  //   name text not null,
  //   source text not null,
  //   url text,
  //   detectionTypes text[],
  //   isActive boolean default true,
  //   created_at timestamp with time zone default timezone('utc', now())
  // );
  //
  // Enable RLS:
  // alter table cameras enable row level security;
  //
  // Policy (allow insert for own user):
  // create policy "Users can insert their own cameras" on cameras
  //   for insert using (auth.uid() = user_id);
  // ---

  const [adding, setAdding] = useState(false);

  const handleAddCamera = async (camera) => {
    setAdding(true);
    setAddWarning('');
    try {
      // Validate required fields
      if (!camera.name || !camera.source || !camera.detectionTypes) {
        alert('Please fill all required fields.');
        setAdding(false);
        return;
      }
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Adding camera with detection types:', camera.detectionTypes);
      }
      const newCamera = await addCamera(camera);
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Camera added successfully:', newCamera);
      }
      const cams = await fetchCameras(userId);
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Fetched cameras:', cams);
      }
      setCameras(cams);
      if (!cams || cams.length === 0) {
        setAddWarning('Camera was not added. Please check your Supabase table schema and RLS policy.');
      }
      setIsAddCameraOpen(false);
    } catch (error) {
      alert('Failed to add camera: ' + (error.message || error));
      setAddWarning('Failed to add camera: ' + (error.message || error));
      console.error('Supabase error:', error);
    } finally {
      setAdding(false);
    }
  };
  const handleUpdateCamera = async (id, updates) => {
    await updateCamera(id, updates);
    const cams = await fetchCameras(userId);
    setCameras(cams);
    setEditingCamera(null);
  };
  
  const handleToggleActive = async (camera, newState) => {
    // Store original state for rollback
    const originalState = camera.isActive;
    
    // Update local state immediately for better UX (optimistic update)
    setCameras(prevCameras => 
      prevCameras.map(cam => 
        cam.id === camera.id 
          ? { ...cam, isActive: newState }
          : cam
      )
    );
    
    try {
      // Retry logic for transient network errors (reduced retries and faster backoff)
      let lastError = null;
      const maxRetries = 2; // Reduced from 3 to 2 for faster failure
      let success = false;
      
      for (let attempt = 1; attempt <= maxRetries; attempt++) {
        try {
          await updateCamera(camera.id, { isActive: newState });
          success = true;
          break;
        } catch (error) {
          lastError = error;
          // Check if it's a transient error (ECONNRESET, network errors, proxy errors)
          const errorMessage = (error?.message || '').toLowerCase();
          const errorCode = error?.code || '';
          const isTransientError = 
            errorMessage.includes('econnreset') ||
            errorMessage.includes('network') ||
            errorMessage.includes('timeout') ||
            errorMessage.includes('connection') ||
            errorMessage.includes('proxy error') ||
            errorMessage.includes('failed to fetch') ||
            errorMessage.includes('service unavailable') ||
            errorCode === 'ECONNRESET' ||
            error?.isRetryable === true ||
            error?.status >= 500;
          
          if (isTransientError && attempt < maxRetries) {
            // Faster backoff: 200ms, 400ms (reduced from 500ms increments)
            const delay = attempt * 200;
            if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
              console.log(`Retrying camera update (attempt ${attempt + 1}/${maxRetries}) after ${delay}ms...`);
            }
            await new Promise(resolve => setTimeout(resolve, delay));
            continue;
          }
          // If it's not a transient error or we've exhausted retries, throw
          throw error;
        }
      }
      
      if (!success) {
        throw lastError;
      }
      
      // Don't refresh immediately - the optimistic update is sufficient
      // The next time cameras are loaded (e.g., on page refresh or when adding/removing),
      // they'll be in sync. This prevents race conditions and incorrect state overwrites.
    } catch (error) {
      console.error('Error toggling camera:', error);
      
      // Revert optimistic update on error
      setCameras(prevCameras => 
        prevCameras.map(cam => 
          cam.id === camera.id 
            ? { ...cam, isActive: originalState }
            : cam
        )
      );
      
      // Show user-friendly error message
      const errorMessage = error?.message || 'Unknown error';
      const isNetworkError = errorMessage.toLowerCase().includes('network') || 
                            errorMessage.toLowerCase().includes('connection') ||
                            errorMessage.toLowerCase().includes('econnreset');
      
      if (isNetworkError) {
        alert('Network error: Failed to toggle camera. Please check your connection and try again.');
      } else {
        alert('Failed to toggle camera: ' + errorMessage);
      }
    }
  };
  
  const handleToggleAllCameras = async () => {
    if (cameras.length === 0) {
      alert('No cameras available to toggle.');
      return;
    }
    
    // Determine target state: if all are on, turn all off; otherwise turn all on
    const allActive = cameras.every(cam => cam.isActive);
    const targetState = !allActive;
    
    // Store original states for rollback
    const originalStates = cameras.map(cam => ({ id: cam.id, isActive: cam.isActive }));
    
    // Optimistic update - update all cameras immediately
    setCameras(prevCameras => 
      prevCameras.map(cam => ({ ...cam, isActive: targetState }))
    );
    
    try {
      // Update all cameras in parallel with retry logic
      const updatePromises = cameras.map(async (camera) => {
        let lastError = null;
        const maxRetries = 2;
        let success = false;
        
        for (let attempt = 1; attempt <= maxRetries; attempt++) {
          try {
            await updateCamera(camera.id, { isActive: targetState });
            success = true;
            break;
          } catch (error) {
            lastError = error;
            const errorMessage = (error?.message || '').toLowerCase();
            const errorCode = error?.code || '';
            const isTransientError = 
              errorMessage.includes('econnreset') ||
              errorMessage.includes('network') ||
              errorMessage.includes('timeout') ||
              errorMessage.includes('connection') ||
              errorMessage.includes('proxy error') ||
              errorMessage.includes('failed to fetch') ||
              errorMessage.includes('service unavailable') ||
              errorCode === 'ECONNRESET' ||
              error?.isRetryable === true ||
              error?.status >= 500;
            
            if (isTransientError && attempt < maxRetries) {
              const delay = attempt * 200;
              await new Promise(resolve => setTimeout(resolve, delay));
              continue;
            }
            throw error;
          }
        }
        
        if (!success) {
          throw lastError;
        }
      });
      
      // Wait for all updates to complete
      await Promise.all(updatePromises);
      
      // Refresh from database to ensure consistency (only for bulk updates)
      // Validate the data before updating state
      try {
        const cams = await fetchCameras(userId);
        // Only update if we got valid data that matches expected count
        if (cams && Array.isArray(cams) && cams.length === cameras.length) {
          setCameras(cams);
        } else {
          // If data doesn't match, log but don't overwrite optimistic update
          console.warn('Camera count mismatch after bulk update. Keeping optimistic state.');
        }
      } catch (fetchError) {
        console.warn('Failed to refresh cameras after bulk update:', fetchError);
        // Don't revert - keep the optimistic update since the database updates succeeded
      }
    } catch (error) {
      console.error('Error toggling all cameras:', error);
      
      // Revert optimistic update on error
      setCameras(prevCameras => 
        prevCameras.map(cam => {
          const original = originalStates.find(s => s.id === cam.id);
          return original ? { ...cam, isActive: original.isActive } : cam;
        })
      );
      
      // Show user-friendly error message
      const errorMessage = error?.message || 'Unknown error';
      const isNetworkError = errorMessage.toLowerCase().includes('network') || 
                            errorMessage.toLowerCase().includes('connection') ||
                            errorMessage.toLowerCase().includes('econnreset');
      
      if (isNetworkError) {
        alert('Network error: Failed to toggle cameras. Please check your connection and try again.');
      } else {
        alert('Failed to toggle cameras: ' + errorMessage);
      }
    }
  };
  
  const handleRemoveCamera = async (id) => {
    try {
      await deleteCamera(id);
      const cams = await fetchCameras(userId);
      setCameras(cams);
    } catch (error) {
      console.error('Error removing camera:', error);
      alert('Failed to remove camera: ' + error.message);
    }
  };
  const startEditing = (camera) => setEditingCamera(camera);
  const stopEditing = () => setEditingCamera(null);

  useEffect(() => {
    const timer = setTimeout(() => setShowSkeleton(false), 1000);
    return () => clearTimeout(timer);
  }, []);

  // Add real-time notification effect
  useEffect(() => {
    if (!userId) return;
    const subscription = subscribeToHistoryEvents((event) => {
      
    });
    return () => {
      if (subscription && subscription.unsubscribe) subscription.unsubscribe();
    };
  }, [userId]);

  useEffect(() => {
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('showProfileModalOnce:', showProfileModalOnce, 'profile:', profile, 'isProfileOpen:', isProfileOpen);
    }
    // Only show the modal if the flag is set AND the profile is incomplete
    if (
      showProfileModalOnce &&
      (!profile.firstName || !profile.lastName || !profile.email || !profile.phone)
    ) {
      setIsProfileOpen(true);
      localStorage.removeItem('showProfileModalOnce');
      setShowProfileModalOnce(false);
    }
  }, [showProfileModalOnce, profile]);

  const handleCloseProfileModal = () => {
    setIsProfileOpen(false);
  };

  return (
    <div className="flex flex-col min-h-screen">
      {/* Move floating background elements here to avoid layout shift on first scroll */}
      <div className="absolute inset-0 pointer-events-none z-0">
        <div className="absolute top-20 left-10 w-4 h-4 bg-blue-400/30 rounded-full animate-bounce" style={{ animationDuration: "3s", animationDelay: "0s" }}></div>
        <div className="absolute top-40 right-20 w-2 h-2 bg-purple-400/40 rounded-full animate-bounce" style={{ animationDuration: "4s", animationDelay: "1s" }}></div>
        <div className="absolute bottom-32 left-1/4 w-3 h-3 bg-cyan-400/35 rounded-full animate-bounce" style={{ animationDuration: "5s", animationDelay: "2s" }}></div>
        <div className="absolute top-1/2 right-10 w-2 h-2 bg-indigo-400/30 rounded-full animate-bounce" style={{ animationDuration: "3.5s", animationDelay: "0.5s" }}></div>
        <div className="absolute bottom-20 right-1/3 w-4 h-4 bg-blue-300/25 rounded-full animate-bounce" style={{ animationDuration: "4.5s", animationDelay: "1.5s" }}></div>
      </div>
      <ParallaxBackground>
        {/* Header */}
        <header className="bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-b border-blue-200/50 dark:border-gray-700/50 sticky top-0 z-40 animate-in slide-in-from-top-4 duration-500">
          <div className="container mx-auto px-4 py-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3 animate-in slide-in-from-left-4 duration-700 ease-out">
                <div className="p-1 bg-gradient-to-br from-gray-900/50 to-gray-800/50 dark:from-gray-800/30 dark:to-gray-900/50 rounded-2xl hover:scale-110 transition-all duration-500 ease-out border border-cyan-500/20 dark:border-cyan-400/30 hover:shadow-lg hover:shadow-cyan-500/20 will-change-transform">
                  <SurveillanceLogo className="h-12 w-12" />
                </div>
                <div>
                  <h1 className="text-2xl font-black bg-gradient-to-r from-blue-800 to-purple-800 dark:from-blue-200 dark:to-purple-200 bg-clip-text text-transparent animate-in slide-in-from-left-2 duration-300 delay-200 tracking-wide">
                    METAVISION
                  </h1>
                  <p className="text-sm text-gray-600 dark:text-gray-400 animate-in slide-in-from-left-2 duration-300 delay-300">
                    Advanced Security & Surveillance Solutions
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-4 animate-in slide-in-from-right-4 duration-700 ease-out">
                <div className="animate-in slide-in-from-right-2 delay-200 duration-700 ease-out">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setIsProfileOpen(true)}
                    className="h-12 w-12 p-0 hover:bg-blue-100 dark:hover:bg-neon-cyan/10 hover:scale-125 transition-all duration-500 ease-out will-change-transform group"
                  >
                    <User className="h-6 w-6 transition-all duration-500 group-hover:scale-110 font-bold" strokeWidth={2.5} />
                  </Button>
                </div>
                <div className="animate-in slide-in-from-right-2 delay-300 duration-700 ease-out">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setIsSettingsOpen(true)}
                    className="h-12 w-12 p-0 hover:bg-blue-100 dark:hover:bg-neon-cyan/10 hover:scale-125 transition-all duration-500 ease-out will-change-transform group"
                  >
                    <Settings className="h-6 w-6 transition-all duration-500 group-hover:rotate-90 font-bold" strokeWidth={2.5} />
                  </Button>
                </div>
                <div className="animate-in slide-in-from-right-2 delay-400 duration-700 ease-out">
                  <div className="transition-all duration-500 hover:scale-125 will-change-transform">
                    <ThemeToggle />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </header>

        {/* Main Content */}
        {showSkeleton ? (
          <div className="container mx-auto px-4 py-8 space-y-8">
            {/* Header Skeleton - Bolder and Shinier */}
            <div className="flex items-center gap-6 animate-pulse">
              <Skeleton className="h-16 w-16 rounded-2xl shadow-lg" />
              <div className="space-y-3">
                <Skeleton className="h-8 w-64 rounded-xl" />
                <Skeleton className="h-5 w-48 rounded-lg" />
              </div>
            </div>
            {/* Camera Grid Skeleton - Bold, Full Width, Shiny */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="w-full h-96 flex flex-col bg-white/80 dark:bg-gray-800/80 rounded-3xl shadow-xl animate-pulse p-0 border-2 border-transparent bg-clip-padding relative overflow-hidden">
                  {/* Animated Gradient Border for Shine */}
                  <div className="absolute inset-0 rounded-3xl pointer-events-none z-10 animate-shimmer border-4 border-transparent bg-gradient-to-r from-blue-200/30 via-purple-200/40 to-cyan-200/30 dark:from-blue-900/30 dark:via-purple-900/40 dark:to-cyan-900/30" style={{maskImage:'linear-gradient(90deg,transparent,white 20%,white 80%,transparent)'}} />
                  <div className="relative z-20 p-6 h-full flex flex-col">
                    {/* Card Header Skeleton */}
                    <div className="flex items-center gap-4 mb-6">
                      <Skeleton className="h-8 w-8 rounded-lg" />
                      <Skeleton className="h-6 w-32 rounded" />
                    </div>
                    {/* Video Window Skeleton */}
                    <Skeleton className="aspect-[4/3] w-full min-h-[180px] rounded-2xl mb-6" />
                    {/* Info Section Skeleton */}
                    <div className="space-y-3 flex-1 flex flex-col justify-end">
                      <Skeleton className="h-5 w-28 rounded" />
                      <Skeleton className="h-5 w-40 rounded" />
                      <div className="flex gap-3 mt-3">
                        <Skeleton className="h-6 w-16 rounded-full" />
                        <Skeleton className="h-6 w-16 rounded-full" />
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
        <main className="container mx-auto px-4 py-8">
          <div className="space-y-8">
            {/* User Welcome Section */}
            <section className="animate-in slide-in-from-bottom-4 duration-700 ease-out">
              <div className="relative text-center mb-8 transform transition-all duration-1000 ease-out">
                {/* Glowing Background */}
                <div
                  className="absolute inset-0 bg-gradient-to-r from-blue-500/10 via-purple-500/10 to-blue-500/10 dark:from-blue-400/20 dark:via-purple-400/20 dark:to-blue-400/20 rounded-2xl blur-xl -z-10 animate-pulse"
                  style={{ animationDuration: "4s" }}
                ></div>
                <div
                  className="absolute inset-0 bg-gradient-to-r from-cyan-500/5 via-indigo-500/5 to-purple-500/5 dark:from-cyan-400/10 dark:via-indigo-400/10 dark:to-purple-400/10 rounded-xl -z-10 animate-pulse"
                  style={{ animationDuration: "6s", animationDelay: "1s" }}
                ></div>

                {/* Content */}
                <div className="relative bg-white/50 dark:bg-gray-900/50 backdrop-blur-sm border border-blue-200/30 dark:border-blue-700/30 rounded-xl p-6 shadow-lg transform transition-all duration-700 ease-out hover:scale-[1.02] hover:shadow-xl">
                  <h2 className="text-xl font-bold text-gray-900 dark:text-gray-100 mb-2 tracking-wide">
                    {getDisplayName()}'s Home Surveillance
                  </h2>
                  <p className="text-gray-600 dark:text-gray-400 font-medium">
                    Welcome back! Here's your security overview
                  </p>
                </div>
              </div>
            </section>

            {/* Camera Grid */}
            <section
              className="animate-in slide-in-from-bottom-4 duration-700 ease-out"
              style={{ animationDelay: "200ms" }}
            >
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h2 className="text-2xl font-bold text-gray-900 dark:text-gray-100 mb-2">
                    Camera Feeds
                  </h2>
                  <p className="text-gray-600 dark:text-gray-400">
                    Monitor your security cameras in real-time
                  </p>
                </div>
                <div className="flex items-center gap-4">
                  <Button
                    onClick={() => setIsAddCameraOpen(true)}
                    size="lg"
                    className="bg-blue-600 hover:bg-blue-700 dark:bg-neon-cyan/80 dark:hover:bg-neon-cyan dark:shadow-neon-cyan/30 hover:scale-110 hover:shadow-xl hover:shadow-blue-500/25 transition-all duration-500 ease-out will-change-transform group text-lg font-semibold px-6 py-3"
                  >
                    <Plus className="h-5 w-5 mr-2 transition-all duration-500 group-hover:rotate-180 group-hover:scale-125" strokeWidth={2.5} />
                    Add Camera
                  </Button>
                  <Button
                    variant="outline"
                    size="lg"
                    onClick={handleToggleAllCameras}
                    disabled={cameras.length === 0}
                    className="hover:bg-green-100 dark:hover:bg-green-900/20 dark:border-green-400/30 hover:scale-110 hover:shadow-lg hover:shadow-green-500/20 transition-all duration-500 ease-out will-change-transform group text-lg font-semibold px-6 py-3 disabled:opacity-50 disabled:cursor-not-allowed"
                    title={cameras.every(cam => cam.isActive) ? "Turn all cameras off" : "Turn all cameras on"}
                  >
                    <Power className={`h-5 w-5 mr-2 transition-all duration-500 ${cameras.every(cam => cam.isActive) ? 'text-green-600 dark:text-green-400' : 'text-gray-600 dark:text-gray-400'}`} strokeWidth={2.5} />
                    {cameras.every(cam => cam.isActive) ? 'Turn All Off' : 'Turn All On'}
                  </Button>
                  <Button
                    variant="outline"
                    size="lg"
                    onClick={() => (window.location.href = "/history")}
                    className="hover:bg-purple-100 dark:hover:bg-purple-900/20 dark:border-purple-400/30 hover:scale-110 hover:shadow-lg hover:shadow-purple-500/20 transition-all duration-500 ease-out will-change-transform group text-lg font-semibold px-6 py-3"
                  >
                    <History className="h-5 w-5 mr-2 transition-all duration-500 group-hover:rotate-12" strokeWidth={2.5} />
                    History
                  </Button>
                </div>
              </div>

              
                            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
                {cameras.map((camera, index) => (
                  <div key={camera.id} className="animate-in slide-in-from-bottom-4 fade-in-0" style={{ animationDelay: `${index * 150}ms` }}>
                    <VideoFeedCard
                      camera={camera}
                      onEdit={startEditing}
                      onRemove={handleRemoveCamera}
                      onToggleActive={handleToggleActive}
                    />
                  </div>
                ))}
              </div>
            </section>
            {addWarning && (
              <div className="bg-yellow-100 text-yellow-800 p-4 rounded shadow mt-4">
                {addWarning}
              </div>
            )}
            {diagnosticWarning && (
              <div className="bg-red-100 text-red-800 p-4 rounded shadow mb-4">
                {diagnosticWarning}
              </div>
            )}


          </div>
        </main>
        )}

        {/* Detection Alerts */}
        <DetectionAlerts cameras={cameras} />

        {/* Camera Management Modal */}
        {(isAddCameraOpen || editingCamera) && (
          <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in-0 duration-300">
            <div className="w-full max-w-2xl animate-in zoom-in-95 slide-in-from-bottom-8 duration-500">
              <CameraManagement
                onAddCamera={handleAddCamera}
                onUpdateCamera={handleUpdateCamera}
                editingCamera={editingCamera}
                onStopEditing={stopEditing}
                onClose={() => {
                  setIsAddCameraOpen(false);
                  stopEditing();
                }}
              />
            </div>
          </div>
        )}

        {/* Settings Panel */}
        <SettingsPanel
          isOpen={isSettingsOpen}
          onClose={() => setIsSettingsOpen(false)}
        />

        {/* Profile Settings */}
        {isProfileOpen && (() => { console.log('Rendering Profile Modal'); return <ProfileSettings onClose={handleCloseProfileModal} />; })()}

        {/* Footer Bottom Bar */}
        <ScrollToTopButton />
      </ParallaxBackground>
      <footer className="bg-white/80 dark:bg-gray-900/80 backdrop-blur-md border-t border-blue-200/50 dark:border-gray-700/50 mt-auto">
        <div className="container mx-auto px-4 pt-6 pb-6 animate-in slide-in-from-bottom-4 duration-1000 delay-700">
            <div className="flex flex-col lg:flex-row justify-between items-center gap-4">
              <div className="flex flex-col sm:flex-row items-center gap-4">
                <p className="text-gray-600 dark:text-gray-400 text-sm">
                © 2025 <span className="font-black">CYBER EAGLE</span>. All rights reserved.
                </p>
                <div className="flex items-center gap-2 px-3 py-1 bg-green-100 dark:bg-green-900/20 rounded-full">
                  <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
                  <span className="text-xs text-green-700 dark:text-green-300 font-medium">
                    System Operational
                  </span>
                </div>
              </div>
              <div className="flex items-center gap-4">
                {/* Language Selector */}
                <div className="flex items-center gap-2">
                  <Globe className="h-4 w-4 text-gray-500 dark:text-gray-400" />
                  <select className="text-sm bg-transparent border-none text-gray-600 dark:text-gray-400 cursor-pointer focus:outline-none">
                    <option value="en">English</option>
                    <option value="es">Español</option>
                    <option value="fr">Français</option>
                    <option value="de">Deutsch</option>
                  </select>
                </div>
              <Link
                to="/about"
                className="text-sm text-blue-600 dark:text-blue-400 hover:underline transition-colors duration-300 px-2 py-1 rounded-lg hover:bg-blue-50 dark:hover:bg-blue-900/20"
              >
                About Us
              </Link>
                <div className="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <Shield className="h-3 w-3" />
                  <span>Enterprise Grade Security</span>
              </div>
            </div>
          </div>
        </div>
      </footer>
    </div>
  );
}
