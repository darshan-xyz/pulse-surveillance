import React from "react";
import { Navigate } from "react-router-dom";
import { useSupabaseUserId } from "@/lib/useSupabaseUserId";

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { userId, restoring } = useSupabaseUserId();
  if (restoring) {
    // Show a loading spinner or nothing while restoring session
    return <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><div className="w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" /></div>;
  }
  if (!userId) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
} 