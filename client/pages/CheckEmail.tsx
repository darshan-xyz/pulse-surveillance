import React from "react";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { SurveillanceLogo } from "@/components/surveillance-logo";
import { Button } from "@/components/ui/button";
import { useEffect } from "react";
import { useUserProfile } from "@/hooks/use-user-profile.tsx";
import { supabase } from "@/lib/supabaseClient";
import { useNavigate } from "react-router-dom";

const GmailLogo = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" className="inline-block mr-2 align-middle">
    <rect width="24" height="24" rx="4" fill="#fff"/>
    <path d="M20 6.5V17.5C20 18.3284 19.3284 19 18.5 19H5.5C4.67157 19 4 18.3284 4 17.5V6.5C4 5.67157 4.67157 5 5.5 5H18.5C19.3284 5 20 5.67157 20 6.5Z" fill="#EA4335"/>
    <path d="M4 7.5L12 13.5L20 7.5V17.5C20 18.3284 19.3284 19 18.5 19H5.5C4.67157 19 4 18.3284 4 17.5V7.5Z" fill="#fff"/>
    <path d="M4 6.5L12 12.5L20 6.5" stroke="#EA4335" strokeWidth="1.5" strokeLinejoin="round"/>
  </svg>
);

export default function CheckEmail() {
  const { updateProfile } = useUserProfile();
  const navigate = useNavigate();
  useEffect(() => {
    // On mount, check if user is already authenticated (handles direct visit after confirmation)
    supabase.auth.getUser().then(({ data }) => {
      if (data?.user?.email) {
        updateProfile({ email: data.user.email });
        navigate("/", { replace: true });
      }
    });
    // Listen for auth state change (handles confirmation in same tab)
    const { data: listener } = supabase.auth.onAuthStateChange((event, session) => {
      if (event === "SIGNED_IN" && session?.user) {
        updateProfile({ email: session.user.email });
        navigate("/", { replace: true });
      }
    });
    return () => {
      listener?.subscription.unsubscribe();
    };
  }, [updateProfile, navigate]);
  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 p-4 relative overflow-hidden">
      {/* Soft background orbs */}
      <div className="absolute inset-0 pointer-events-none z-0">
        <div className="absolute top-1/4 left-1/4 w-80 h-80 bg-green-200/40 rounded-full blur-3xl" style={{ filter: 'blur(60px)' }} />
        <div className="absolute top-3/4 right-1/4 w-96 h-96 bg-emerald-200/40 rounded-full blur-3xl" style={{ filter: 'blur(80px)' }} />
        <div className="absolute bottom-1/4 left-1/3 w-96 h-96 bg-teal-200/40 rounded-full blur-3xl" style={{ filter: 'blur(80px)' }} />
        <div className="absolute top-10 right-10 w-32 h-32 bg-yellow-100/40 rounded-full blur-2xl" style={{ filter: 'blur(40px)' }} />
        <div className="absolute bottom-10 left-10 w-40 h-40 bg-blue-100/40 rounded-full blur-2xl" style={{ filter: 'blur(40px)' }} />
        {/* Subtle grid pattern */}
        <div className="absolute inset-0 opacity-10">
          <div className="absolute inset-0 bg-[linear-gradient(rgba(34,197,94,0.15)_1px,transparent_1px),linear-gradient(90deg,rgba(34,197,94,0.15)_1px,transparent_1px)] bg-[size:60px_60px]" />
        </div>
      </div>
      <Card className="w-full max-w-lg relative z-10 overflow-hidden border-0 shadow-2xl bg-white/90 dark:bg-gray-900/90 backdrop-blur-xl animate-in zoom-in-90 slide-in-from-bottom-8 duration-1000 ease-out border border-white/20 dark:border-gray-700/20 will-change-transform">
        <CardHeader className="text-center space-y-6 pb-2">
          <div className="flex justify-center">
            <div className="p-4 bg-gradient-to-br from-green-600 to-emerald-600 dark:from-green-500 dark:to-emerald-500 rounded-2xl shadow-lg">
              <SurveillanceLogo className="h-14 w-14" />
            </div>
          </div>
          <div>
            <h1 className="text-3xl font-black bg-gradient-to-r from-green-700 to-emerald-700 dark:from-green-300 dark:to-emerald-300 bg-clip-text text-transparent tracking-wide mb-4">
              Check Your Email
            </h1>
            <p className="text-lg text-gray-700 dark:text-gray-300 font-medium mb-4">
              We've sent a confirmation link to your email address.<br />
              <span className="font-semibold text-green-700 dark:text-green-300">Please check your inbox and click the link to activate your account.</span>
            </p>
            <div className="flex justify-center">
              <div className="w-full max-w-xl">
                <div className="text-center">
                  <span className="inline-block mb-2 px-4 py-2 text-base font-extrabold uppercase tracking-wider bg-red-600 text-white rounded-full shadow-sm border-2 border-red-700">Important</span>
                  <span className="block text-lg font-bold text-white bg-red-600 border-2 border-red-700 rounded-xl px-4 py-2 mb-4 shadow-lg">
                    Please authorize the email on the same device where you signed up
                  </span>
                </div>
              </div>
            </div>
            <Button
              asChild
              className="mt-4 text-lg px-8 py-3 bg-[#1A73E8] hover:bg-[#1558b0] text-white font-bold rounded-xl shadow-lg transition-all duration-200 flex items-center justify-center gap-2"
            >
              <a href="https://mail.google.com" target="_blank" rel="noopener noreferrer">
                <GmailLogo />Open Gmail
              </a>
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-6 text-center"></CardContent>
      </Card>
    </div>
  );
} 