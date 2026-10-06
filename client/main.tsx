import "./global.css";

import { Toaster } from "@/components/ui/toaster";
import { createRoot } from "react-dom/client";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { lazy, Suspense } from "react";
import Index from "./pages/Index";
import NotFound from "./pages/NotFound";
import About from "./pages/about";
import StreamPage from "./pages/StreamPage";
import { UserProfileProvider } from "@/hooks/use-user-profile.tsx";
import React, { useEffect } from "react";
import CheckEmail from "./pages/CheckEmail";
import Terms from "./pages/Terms";
import Privacy from "./pages/Privacy";
import { ProfileSettings } from "@/components/profile-settings";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { supabase } from "@/lib/supabaseClient";
import { useNavigate } from "react-router-dom";
import { useUserProfile } from "@/hooks/use-user-profile.tsx";
import { AuthRedirectHandler } from "@/components/AuthRedirectHandler";
import ForgotPassword from './pages/forgot-password';
import ResetPassword from './pages/reset-password';


import ErrorBoundary from "./components/ErrorBoundary";

const Login = lazy(() => import("./pages/Login"));

const queryClient = new QueryClient();

const App = () => (
  <UserProfileProvider>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Toaster />
        <Sonner />
        <BrowserRouter>
          <AuthRedirectHandler />
          <Routes>
            <Route path="/" element={<Index />} />
            <Route path="/about" element={<About />} />
            <Route path="/check-email" element={<CheckEmail />} />
            <Route path="/terms" element={<Terms />} />
            <Route path="/privacy" element={<Privacy />} />
            <Route path="/stream" element={<StreamPage />} />
            <Route path="/forgot-password" element={<ForgotPassword />} />
            <Route path="/reset-password" element={<ResetPassword />} />
              <Route
                path="/login"
                element={
                  <Suspense
                    fallback={
                      <div className="min-h-screen bg-gradient-to-br from-blue-50 via-indigo-50 to-purple-100 dark:from-gray-900 dark:via-gray-800 dark:to-gray-900 flex items-center justify-center">
                        <div className="text-center">
                          <div className="w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full animate-spin mx-auto mb-4"></div>
                          <p className="text-gray-600 dark:text-gray-400">
                            Loading...
                          </p>
                        </div>
                      </div>
                    }
                  >
                    <Login />
                  </Suspense>
                }
              />
            <Route
              path="/history"
              element={
                <Suspense
                  fallback={
                    <div className="min-h-screen bg-gradient-to-br from-blue-50 via-indigo-50 to-purple-100 dark:from-gray-900 dark:via-gray-800 dark:to-gray-900 flex items-center justify-center">
                      <div className="text-center">
                        <div className="w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full animate-spin mx-auto mb-4"></div>
                        <p className="text-gray-600 dark:text-gray-400">
                          Loading...
                        </p>
                      </div>
                    </div>
                  }
                >
          
                </Suspense>
              }
            />
            {/* ADD ALL CUSTOM ROUTES ABOVE THE CATCH-ALL "*" ROUTE */}
            <Route path="*" element={<NotFound />} />
          </Routes>
        </BrowserRouter>
      </TooltipProvider>
    </QueryClientProvider>
  </UserProfileProvider>
);

// Prevent multiple root creations during HMR
const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("Root element not found");
}

// Check if root already exists (during HMR) - use a more reliable approach
let root = (window as any).__react_root__;
if (!root) {
  root = createRoot(rootElement);
  (window as any).__react_root__ = root;
}

// Render the app with error boundary
// Note: ErrorBoundary should handle HMR errors gracefully
try {
  root.render(
  <ErrorBoundary>
    <App />
  </ErrorBoundary>
);
} catch (error) {
  // If rendering fails, reload the page for a clean state
  console.error('Failed to render app:', error);
  if (import.meta.env.DEV) {
    // In development, reload on render errors to recover from HMR issues
    setTimeout(() => window.location.reload(), 1000);
  }
}
