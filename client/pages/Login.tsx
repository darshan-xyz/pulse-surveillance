import React, { useState, useEffect, useCallback } from 'react';
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import {
  ArrowLeft,
  Mail,
  Smartphone,
  User,
  ArrowRight,
  Eye,
  EyeOff,
  Lock,
} from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from 'react-router-dom';

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { SurveillanceLogo } from "@/components/surveillance-logo";
import { ThemeToggle } from "@/components/theme-toggle";
import { Dialog, DialogContent } from "@/components/ui/dialog";

import { PhoneInput } from "@/components/auth/phone-input";
import { PasswordInput } from "@/components/auth/password-input";
import { GoogleSignIn } from "@/components/auth/google-signin";
import { AnimatedCheckbox } from "@/components/auth/animated-checkbox";
import { PhoneOTPInput } from "@/components/auth/phone-otp-input";

import {
  LoginMethod,
  AuthMode,
  phoneSchema,
  emailSchema,
  signUpSchema,
  PhoneLoginData,
  EmailLoginData,
  SignUpData,
} from "@/lib/auth-utils";
import { supabase, phoneEmailSignUp, phoneEmailLogin, linkPhoneToEmail, validateLoginCredentials } from "@/lib/supabaseClient";
import { logAuthCallbackUrlDiagnostics } from '@/lib/supabaseDiagnostics';
import { useUserProfile } from "@/hooks/use-user-profile.tsx";

export default function Login() {
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [loginMethod, setLoginMethod] = useState<LoginMethod | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [showPhonePassword, setShowPhonePassword] = useState(false);
  const [showEmailPassword, setShowEmailPassword] = useState(false);
  const [showSignUpPassword, setShowSignUpPassword] = useState(false);
  const [showPhoneOTP, setShowPhoneOTP] = useState(false);
  const [phoneNumber, setPhoneNumber] = useState('');
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [restoringSession, setRestoringSession] = useState(true);
  const [debugInfo, setDebugInfo] = useState({ session: null, user: null, error: null, localStorage: {}, cookies: '' });
  const [agreeToTermsPhone, setAgreeToTermsPhone] = useState(false);
  const [phoneTermsError, setPhoneTermsError] = useState("");
  const [showGoogleTerms, setShowGoogleTerms] = useState(false);
  const [agreeToTermsGoogle, setAgreeToTermsGoogle] = useState(false);
  const [googleTermsError, setGoogleTermsError] = useState("");
  // Add state for phone sign up email
  const [phoneSignupEmail, setPhoneSignupEmail] = useState("");
  const [phoneSignupEmailError, setPhoneSignupEmailError] = useState("");
  const [emailLoginError, setEmailLoginError] = useState<string>("");
  const [signUpError, setSignUpError] = useState<string>(""); // New state for sign up errors
  const { updateProfile } = useUserProfile();
  const navigate = useNavigate();

  function getDebugStorage() {
    let localStorageData = {};
    try {
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        localStorageData[key] = localStorage.getItem(key);
      }
    } catch (e) {}
    return {
      localStorage: localStorageData,
      cookies: document.cookie,
    };
  }

  useEffect(() => {
    logAuthCallbackUrlDiagnostics();
    // Wait for Supabase session restoration
    const checkSession = async () => {
      const { data, error } = await supabase.auth.getSession();
      const storage = getDebugStorage();
      setDebugInfo({ session: data?.session, user: data?.session?.user, error, ...storage });
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Supabase session on login page mount:', data, error, storage);
      }
      setRestoringSession(false);
    };
    checkSession();
    const { data: listener } = supabase.auth.onAuthStateChange((event, session) => {
      const storage = getDebugStorage();
      setDebugInfo({ session, user: session?.user, error: null, ...storage });
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Supabase auth state change:', event, session, storage);
      }
    });
    return () => {
      listener?.subscription.unsubscribe();
    };
  }, []);

  // Phone login form
  const {
    register: registerPhone,
    watch: watchPhone,
    setValue: setPhoneValue,
    handleSubmit: handlePhoneSubmit,
    formState: { errors: phoneErrors },
  } = useForm<PhoneLoginData>({
    resolver: zodResolver(phoneSchema),
    defaultValues: {
      countryCode: "+1",
      number: "",
      password: "",
    },
  });

  // Email login form
  const {
    register: registerEmail,
    handleSubmit: handleEmailSubmit,
    formState: { errors: emailErrors },
    watch: watchEmail,
  } = useForm<EmailLoginData>({
    resolver: zodResolver(emailSchema),
  });

  // Sign up form
  const {
    register: registerSignUp,
    handleSubmit: handleSignUpSubmit,
    formState: { errors: signUpErrors },
    watch: watchSignUp,
    setValue: setSignUpValue,
  } = useForm<SignUpData>({
    resolver: zodResolver(signUpSchema),
    defaultValues: {
      agreeToTerms: false,
    },
  });

  const phoneData = watchPhone();
  const emailPassword = watchEmail("password") || "";
  const signUpPassword = watchSignUp("password") || "";
  const agreeToTerms = watchSignUp("agreeToTerms") || false;

  const handlePhoneLogin = async (data: PhoneLoginData) => {
    setPhoneTermsError("");
    setPhoneSignupEmailError("");
    if (authMode === "signup" && !agreeToTermsPhone) {
      setPhoneTermsError("You must agree to the Terms & Conditions and Privacy Policy.");
      setIsLoading(false);
      return;
    }
    if (authMode === "signup" && !phoneSignupEmail) {
      setPhoneSignupEmailError("Email is required for phone sign up.");
      setIsLoading(false);
      return;
    }
    setIsLoading(true);
    try {
      if (authMode === "signup") {
        // Format phone number properly (extract numeric country code)
        let countryCode = data.countryCode.replace(/^\+/, '').split('-')[0].replace(/\D/g, '');
        const phone = `+${countryCode}-${data.number}`;
        // 1. Create Supabase Auth user
        const { data: authData, error: authError } = await supabase.auth.signUp({
          email: phoneSignupEmail,
          password: data.password,
        });
        if (authError) {
          setPhoneSignupEmailError(authError.message || "Sign up failed. Please try again.");
          setIsLoading(false);
          return;
        }
        // 2. Insert phone/email mapping (no password)
        const { error } = await phoneEmailSignUp({ phone, email: phoneSignupEmail });
        if (error) {
          if (error.message && (error.message.includes('duplicate key') || error.message.includes('already exists'))) {
            setPhoneSignupEmailError('This phone or email is already registered. Please log in or use a different phone/email.');
          } else {
            setPhoneSignupEmailError(error.message || "Sign up failed. Please try again.");
          }
          setIsLoading(false);
          return;
        }
        // 3. Link phone to email
        const { error: linkError } = await linkPhoneToEmail({ email: phoneSignupEmail, phone });
        if (linkError) {
          toast.error("Failed to link phone to email: " + linkError.message);
        } else {
          toast.success("Phone and email linked. You can now log in with either.");
        }
        // Update profile and redirect to profile page
        updateProfile({ email: phoneSignupEmail, phone });
        localStorage.setItem('showProfileModalOnce', 'true');
        window.location.href = "/";
        setIsLoading(false);
        return;
      }
      if (authMode === "login") {
        // Format phone number properly (extract numeric country code)
        let countryCode = data.countryCode.replace(/^\+/, '').split('-')[0].replace(/\D/g, '');
        const phone = `+${countryCode}-${data.number}`;
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log("Attempting phone login with:", phone, data.password);
        }
        const { data: user, error } = await phoneEmailLogin({ phone });
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log("phoneEmailLogin result:", { user, error });
        }
        if (error || !user) {
          setPhoneSignupEmailError(error?.message || "Phone or password is incorrect.");
          toast.error("No account found. Please sign up first.");
          setAuthMode("signup");
          setLoginMethod("phone");
          setIsLoading(false);
          return;
        }
        if (!user.email) {
          setPhoneSignupEmailError("No email found for this phone user.");
          toast.error("No email found for this phone user.");
          setIsLoading(false);
          return;
        }
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log("Attempting Supabase Auth login with:", user.email, data.password);
        }
        const { error: authError, data: authData } = await supabase.auth.signInWithPassword({
          email: user.email,
          password: data.password,
        });
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log("Supabase Auth login result:", { authError, authData });
        }
        if (authError) {
          setPhoneSignupEmailError(authError.message || "Login failed. Please try again.");
          toast.error("Supabase Auth login failed: " + authError.message);
          setIsLoading(false);
          return;
        }
      window.location.href = "/";
        setIsLoading(false);
        return;
      }
    } catch (error) {
      toast.error("Login failed. Please try again.");
    }
    setIsLoading(false);
  };

  const handleEmailLogin = useCallback(async (data: EmailLoginData) => {
    setIsLoading(true);
    setEmailLoginError(""); // Clear previous errors

    try {
      const { data: authData, error, userExists, isWrongPassword, needsGuidance } = await validateLoginCredentials(
        data.email,
        data.password
      );

      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Login validation result:', {
          authData,
          error,
          userExists,
          isWrongPassword,
          needsGuidance,
          errorMessage: error?.message
        });
      }

      if (error) {
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log('Login error details:', {
            error: error.message,
            userExists,
            isWrongPassword,
            needsGuidance
          });
        }

        if (userExists === false) {
          // Only log in debug mode
          if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
            console.log('User does not exist, redirecting to signup');
          }
          // User doesn't exist - redirect to signup
          setEmailLoginError('No account found. Please sign up first.');
          toast.error('No account found. Please sign up first.');
          setAuthMode('signup');
          setLoginMethod('email');
          setIsLoading(false);
          return;
        }

        if (needsGuidance) {
          // Only log in debug mode
          if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
            console.log('Providing user guidance for ambiguous case');
          }
          // For ambiguous cases, show the error and provide guidance
          setEmailLoginError(error.message);
          toast.error(error.message);
          setIsLoading(false);
          return;
        }

        // Other errors
        setEmailLoginError(error.message || "Login failed. Please try again.");
        toast.error(error.message || "Login failed. Please try again.");
        setIsLoading(false);
        return;
      }

      // Login successful (only log in debug mode)
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Login successful, redirecting to home');
      }
      toast.success("Login successful!");
      window.location.href = "/";
    } catch (error) {
      console.error('Login validation failed:', error);
      setEmailLoginError("Login failed. Please try again.");
      toast.error("Login failed. Please try again.");
    }
    setIsLoading(false);
  }, []);

  const handleSignUp = async (data: SignUpData) => {
    setIsLoading(true);
    setSignUpError(""); // Clear previous errors
    
    try {
      const { data: authData, error } = await supabase.auth.signUp({
        email: data.email,
        password: data.password,
      });
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Supabase sign up result:', { authData, error });
      }

      if (error) {
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log('Signup error details:', error.message);
        }
        const errorMessage = error.message.toLowerCase();
        
        // Check for various "user already exists" error messages
        if (
          errorMessage.includes('user already registered') ||
          errorMessage.includes('already registered') ||
          errorMessage.includes('user already exists') ||
          errorMessage.includes('email already exists') ||
          errorMessage.includes('already exists') ||
          errorMessage.includes('duplicate key') ||
          errorMessage.includes('unique constraint')
        ) {
          // Only log in debug mode
          if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
            console.log('User already exists, showing error message');
          }
          setSignUpError("An account with this email already exists. Please sign in instead.");
          toast.error("An account with this email already exists. Please sign in instead.");
          setIsLoading(false);
          return;
        }
        
        // Handle specific database errors
        if (errorMessage.includes('database error saving new user')) {
          setSignUpError("Account creation failed due to database setup. Please contact support or try again later.");
          toast.error("Account creation failed due to database setup. Please contact support or try again later.");
          console.error('Database setup issue:', error);
        } else {
          // Generic error handling
          setSignUpError(error.message || "Sign up failed. Please try again.");
          toast.error(error.message || "Sign up failed. Please try again.");
        }
        setIsLoading(false);
        return;
      }

      // Check if email confirmation is required
      if (authData.user && !authData.user.email_confirmed_at) {
        toast.success("Account created successfully! Check your email for a confirmation link.");
        localStorage.setItem('showProfileModalOnce', 'true');
        window.location.href = "/check-email";
      } else {
        // User is already confirmed (some providers auto-confirm)
        toast.success("Account created successfully! You can now sign in.");
        setAuthMode('login');
        setLoginMethod('email');
      }
    } catch (error) {
      console.error('Signup failed with exception:', error);
      setSignUpError("Sign up failed. Please try again.");
      toast.error("Sign up failed. Please try again.");
    }
    setIsLoading(false);
  };

  const handleGoogleAuth = async () => {
    // For sign up, require terms acceptance
    if (authMode === "signup") {
      setShowGoogleTerms(true);
      return;
    }
    try {
      setIsLoading(true);
      const redirectTo = window.location.origin;
      const { error } = await supabase.auth.signInWithOAuth({
        provider: 'google',
        options: { redirectTo },
      });
      if (error) {
        console.error('Google OAuth error:', error.message);
        toast.error("Google sign-in failed. Please try again.");
        setIsLoading(false);
      }
      // On success, Supabase will redirect to provider; no further action here
    } catch (e) {
      console.error('Google OAuth exception:', e);
      toast.error("Google sign-in failed. Please try again.");
      setIsLoading(false);
    }
  };

  const handlePhoneOTP = (data: PhoneLoginData) => {
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Phone data received:', data);
    }
    
    // Validate phone number is not empty
    if (!data.number || data.number.trim().length === 0) {
      toast.error('Please enter a valid phone number');
      return;
    }
    
    // Extract only the numeric country code (remove country name like "IN")
    // countryCode format is like "91-IN" or just "91" or "+91"
    let countryCode = data.countryCode;
    
    // Remove leading + if present
    countryCode = countryCode.replace(/^\+/, '');
    
    if (countryCode.includes('-')) {
      // Split by "-" and take only the numeric part
      countryCode = countryCode.split('-')[0];
    }
    
    // Remove any non-numeric characters from country code
    countryCode = countryCode.replace(/\D/g, '');
    
    // Validate country code is not empty
    if (!countryCode || countryCode.length === 0) {
      toast.error('Please select a valid country code');
      return;
    }
    
    // Format phone number properly for Firebase
    let phone = countryCode + data.number;
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Combined phone:', phone);
    }
    
    // Remove any spaces, dashes, or parentheses
    phone = phone.replace(/[\s\-\(\)]/g, '');
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('After cleaning:', phone);
    }
    
    // Validate minimum length (country code + at least 6 digits)
    if (phone.length < 8) {
      toast.error('Please enter a complete phone number');
      return;
    }
    
    // Ensure it starts with +
    if (!phone.startsWith('+')) {
      phone = '+' + phone;
    }
    
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Final formatted phone for OTP:', phone);
    }
    setPhoneNumber(phone);
    setShowPhoneOTP(true);
  };

  const handleOTPSuccess = () => {
    setShowPhoneOTP(false);
    toast.success("Phone number verified successfully!");
    window.location.href = "/";
  };

  const handleOTPError = (error: string) => {
    toast.error(error);
  };

  const handleOTPBack = () => {
    setShowPhoneOTP(false);
    setPhoneNumber('');
  };

  const handleForgotPassword = () => {
    navigate('/forgot-password');
  };

  const resetToMethodSelection = () => {
    setLoginMethod(null);
  };

  // Loading spinner overlay
  const LoadingOverlay = () => (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm animate-in fade-in-0 duration-300">
      <div className="flex flex-col items-center gap-4 p-8 bg-white/90 dark:bg-gray-900/90 rounded-2xl shadow-2xl">
        <div className="w-10 h-10 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
        <span className="text-lg font-semibold text-blue-700 dark:text-blue-300">Signing in...</span>
      </div>
    </div>
  );

  if (restoringSession) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
        <span className="ml-4 text-blue-700">Restoring session...</span>
      </div>
    );
  }

  return (
    <div className="min-h-screen relative overflow-hidden bg-gradient-to-br from-green-100 via-emerald-200 to-teal-100 dark:from-slate-900 dark:via-green-900 dark:to-slate-900 flex items-center justify-center p-4">
      {isLoading && <LoadingOverlay />}
      {/* Animated Background Elements */}
      <div className="absolute inset-0 overflow-hidden">
        {/* Light Mode Background */}
        <div className="absolute inset-0 dark:hidden">
          {/* Light floating orbs */}
          <div
            className="absolute top-1/4 left-1/4 w-72 h-72 bg-green-200/40 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDuration: "4s",
              animationTimingFunction: "ease-in-out",
            }}
          />
          <div
            className="absolute top-3/4 right-1/4 w-96 h-96 bg-emerald-200/40 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDelay: "1.5s",
              animationDuration: "5s",
              animationTimingFunction: "ease-in-out",
            }}
          />
          <div
            className="absolute bottom-1/4 left-1/3 w-80 h-80 bg-teal-200/40 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDelay: "3s",
              animationDuration: "4.5s",
              animationTimingFunction: "ease-in-out",
            }}
          />

          {/* Light grid pattern */}
          <div className="absolute inset-0 opacity-30">
            <div className="absolute inset-0 bg-[linear-gradient(rgba(34,197,94,0.2)_1px,transparent_1px),linear-gradient(90deg,rgba(34,197,94,0.2)_1px,transparent_1px)] bg-[size:60px_60px]" />
          </div>

          {/* Light moving dots */}
          <div className="absolute inset-0">
            <div className="absolute top-20 left-20 w-2 h-2 bg-green-500 rounded-full animate-bounce delay-100" />
            <div className="absolute top-40 right-32 w-1 h-1 bg-emerald-500 rounded-full animate-bounce delay-300" />
            <div className="absolute bottom-32 left-1/2 w-1.5 h-1.5 bg-teal-500 rounded-full animate-bounce delay-500" />
            <div className="absolute top-1/2 right-20 w-1 h-1 bg-lime-500 rounded-full animate-bounce delay-700" />
          </div>

          {/* Light scanning lines */}
          <div className="absolute inset-0 pointer-events-none">
            <div className="absolute top-0 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-green-500/40 to-transparent animate-pulse duration-3000" />
            <div className="absolute bottom-0 right-0 w-full h-0.5 bg-gradient-to-l from-transparent via-emerald-500/40 to-transparent animate-pulse duration-3000 delay-1500" />
          </div>
        </div>

        {/* Dark Mode Background */}
        <div className="absolute inset-0 hidden dark:block">
          {/* Dark floating orbs */}
          <div
            className="absolute top-1/4 left-1/4 w-72 h-72 bg-green-500/20 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDuration: "4s",
              animationTimingFunction: "ease-in-out",
            }}
          />
          <div
            className="absolute top-3/4 right-1/4 w-96 h-96 bg-emerald-500/20 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDelay: "1.5s",
              animationDuration: "5s",
              animationTimingFunction: "ease-in-out",
            }}
          />
          <div
            className="absolute bottom-1/4 left-1/3 w-80 h-80 bg-lime-500/20 rounded-full blur-3xl animate-pulse will-change-transform"
            style={{
              animationDelay: "3s",
              animationDuration: "4.5s",
              animationTimingFunction: "ease-in-out",
            }}
          />

          {/* Dark grid pattern */}
          <div className="absolute inset-0 opacity-10">
            <div className="absolute inset-0 bg-[linear-gradient(rgba(34,197,94,0.3)_1px,transparent_1px),linear-gradient(90deg,rgba(34,197,94,0.3)_1px,transparent_1px)] bg-[size:50px_50px]" />
          </div>

          {/* Dark moving dots */}
          <div className="absolute inset-0">
            <div className="absolute top-20 left-20 w-2 h-2 bg-green-400 rounded-full animate-bounce delay-100" />
            <div className="absolute top-40 right-32 w-1 h-1 bg-emerald-400 rounded-full animate-bounce delay-300" />
            <div className="absolute bottom-32 left-1/2 w-1.5 h-1.5 bg-lime-400 rounded-full animate-bounce delay-500" />
            <div className="absolute top-1/2 right-20 w-1 h-1 bg-teal-400 rounded-full animate-bounce delay-700" />
          </div>

          {/* Dark scanning lines */}
          <div className="absolute inset-0 pointer-events-none">
            <div className="absolute top-0 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-green-400/50 to-transparent animate-pulse duration-3000" />
            <div className="absolute bottom-0 right-0 w-full h-0.5 bg-gradient-to-l from-transparent via-emerald-400/50 to-transparent animate-pulse duration-3000 delay-1500" />
          </div>
        </div>
      </div>

      {/* Theme Toggle - Fixed Position */}
      <div className="fixed top-6 right-6 z-50">
        <div className="p-2 bg-white/10 dark:bg-gray-900/10 backdrop-blur-lg rounded-full border border-white/20 dark:border-gray-700/20">
          <ThemeToggle />
        </div>
      </div>

      <Card className="w-full max-w-md relative overflow-hidden border-0 shadow-2xl bg-white/90 dark:bg-gray-900/90 backdrop-blur-xl animate-in zoom-in-90 slide-in-from-bottom-8 duration-1000 ease-out border border-white/20 dark:border-gray-700/20 will-change-transform">
        {/* Header */}
        <CardHeader className="text-center space-y-4 pb-2">
          <div className="flex justify-center">
            <div className="p-3 bg-gradient-to-br from-green-600 to-emerald-600 dark:from-green-500 dark:to-emerald-500 rounded-2xl shadow-lg">
              <SurveillanceLogo className="h-10 w-10" />
            </div>
          </div>
          <div>
            <h1 className="text-2xl font-black bg-gradient-to-r from-green-700 to-emerald-700 dark:from-green-300 dark:to-emerald-300 bg-clip-text text-transparent tracking-wide">
              CYBER EAGLE
            </h1>
            <p className="text-gray-700 dark:text-gray-300 text-sm font-medium">
              {authMode === "login"
                ? "Welcome back to your security portal"
                : "Join the next generation of security"}
            </p>
          </div>
        </CardHeader>

        <CardContent className="space-y-6">
          {/* Method Selection */}
          {!loginMethod && (
            <div className="space-y-4 animate-in slide-in-from-right-4 duration-700 ease-out">
              <GoogleSignIn
                onSuccess={handleGoogleAuth}
                disabled={isLoading}
                text={
                  authMode === "login"
                    ? "Continue with Google"
                    : "Sign up with Google"
                }
              />

              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-gray-300 dark:border-gray-600" />
                </div>
                <div className="relative flex justify-center text-sm">
                  <span className="px-2 bg-white dark:bg-gray-900 text-gray-500">
                    or
                  </span>
                </div>
              </div>

              <div className="grid gap-3">
                <Button
                  variant="outline"
                  onClick={() => setLoginMethod("phone")}
                  className="w-full justify-start gap-3 h-12 hover:scale-[1.02] transition-all duration-300 ease-out hover:shadow-lg group border-gray-300 dark:border-gray-600 will-change-transform hover:border-green-400 dark:hover:border-green-500"
                >
                  <Smartphone className="h-5 w-5 text-green-600 dark:text-green-400 transition-transform duration-200 group-hover:scale-110" />
                  <span>Continue with Phone</span>
                  <ArrowRight className="h-4 w-4 ml-auto opacity-0 group-hover:opacity-100 transition-all duration-300 transform translate-x-1 group-hover:translate-x-0" />
                </Button>

                <Button
                  variant="outline"
                  onClick={() => setLoginMethod("email")}
                  className="w-full justify-start gap-3 h-12 hover:scale-[1.02] transition-all duration-300 ease-out hover:shadow-lg group border-gray-300 dark:border-gray-600 will-change-transform hover:border-green-400 dark:hover:border-green-500"
                >
                  <Mail className="h-5 w-5 text-green-600 dark:text-green-400 transition-transform duration-200 group-hover:scale-110" />
                  <span>Continue with Email</span>
                  <ArrowRight className="h-4 w-4 ml-auto opacity-0 group-hover:opacity-100 transition-all duration-300 transform translate-x-1 group-hover:translate-x-0" />
                </Button>
              </div>

              <div className="text-center pt-4">
                <button
                  onClick={() =>
                    setAuthMode(authMode === "login" ? "signup" : "login")
                  }
                  className="text-sm text-green-600 dark:text-green-400 hover:underline transition-colors duration-200"
                >
                  {authMode === "login"
                    ? "Don't have an account? Sign up"
                    : "Already have an account? Sign in"}
                </button>
              </div>
            </div>
          )}

          {/* Phone Login */}
          {loginMethod === "phone" && (
            <div className="space-y-6 animate-in slide-in-from-right-4 duration-500">
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={resetToMethodSelection}
                  className="p-2"
                >
                  <ArrowLeft className="h-4 w-4" />
                </Button>
                <h2 className="text-lg font-semibold">
                  {authMode === "login" ? "Sign in" : "Sign up"} with Phone
                </h2>
              </div>

              <form
                onSubmit={handlePhoneSubmit(handlePhoneLogin)}
                className="space-y-4"
              >
                <PhoneInput
                  value={phoneData}
                  onChange={(value) => {
                    setPhoneValue("countryCode", value.countryCode);
                    setPhoneValue("number", value.number);
                  }}
                  error={
                    phoneErrors.number?.message ||
                    phoneErrors.countryCode?.message
                  }
                  disabled={isLoading}
                />

                {authMode === "signup" && (
                  <div className="space-y-2">
                    <Label htmlFor="phone-signup-email">Email Address</Label>
                    <div className="relative">
                      <Input
                        id="phone-signup-email"
                        type="email"
                        placeholder="Enter your email"
                        value={phoneSignupEmail}
                        onChange={e => setPhoneSignupEmail(e.target.value)}
                        className="pl-10"
                        disabled={isLoading}
                      />
                      <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                    </div>
                    {phoneSignupEmailError && (
                      <div 
                        key={`phone-error-${phoneSignupEmailError}`}
                        className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg"
                      >
                        <p className="text-sm text-red-600 dark:text-red-400">
                          {phoneSignupEmailError}
                        </p>
                        
                        {/* Show signup button for non-existing users */}
                        {(phoneSignupEmailError.includes('No email found') || 
                          phoneSignupEmailError.includes('Phone or password is incorrect')) && (
                          <div className="mt-2 pt-2 border-t border-red-200 dark:border-red-800">
                            <button
                              type="button"
                              onClick={() => setAuthMode('signup')}
                              className="text-sm text-green-600 dark:text-green-400 hover:underline transition-colors duration-200"
                            >
                              → Create new account
                            </button>
                          </div>
                        )}
                        
                        {/* Show login button for existing users */}
                        {(phoneSignupEmailError.includes('already registered') || 
                          phoneSignupEmailError.includes('This phone or email is already registered')) && (
                          <div className="mt-2 pt-2 border-t border-red-200 dark:border-red-800">
                            <button
                              type="button"
                              onClick={() => setAuthMode('login')}
                              className="text-sm text-blue-600 dark:text-blue-400 hover:underline transition-colors duration-200"
                            >
                              → Sign in instead
                            </button>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )}

                <div className="space-y-2">
                  <Label htmlFor="phone-password">Password</Label>
                  <div className="relative">
                    <Input
                      id="phone-password"
                      type={showPhonePassword ? "text" : "password"}
                      placeholder="Enter your password"
                      {...registerPhone("password")}
                      className="pl-10 pr-12"
                      disabled={isLoading}
                    />
                    <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowPhonePassword(!showPhonePassword)}
                      className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
                      disabled={isLoading}
                    >
                      {showPhonePassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  {phoneErrors.password && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {phoneErrors.password.message}
                    </p>
                  )}
                </div>

                {authMode === "signup" && (
                  <div className="space-y-2">
                    <AnimatedCheckbox
                      checked={agreeToTermsPhone}
                      onChange={setAgreeToTermsPhone}
                      error={phoneTermsError}
                      disabled={isLoading}
                      label={
                        <span className="text-gray-700 dark:text-gray-300">
                          I agree to the {" "}
                          <a
                            href="/terms"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-green-600 dark:text-green-400 hover:underline"
                          >
                            Terms & Conditions
                          </a>{" "}
                          and {" "}
                          <a
                            href="/privacy"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-green-600 dark:text-green-400 hover:underline"
                          >
                            Privacy Policy
                          </a>
                        </span>
                      }
                    />
                  </div>
                )}

                <div className="space-y-3">
                <Button
                  type="submit"
                  className="w-full bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-700 hover:to-emerald-700 dark:from-green-500 dark:to-emerald-500 dark:hover:from-green-400 dark:hover:to-emerald-400 transition-all duration-200 hover:scale-[1.02] hover:shadow-lg shadow-lg"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <div className="flex items-center gap-2">
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                      Processing...
                    </div>
                  ) : (
                    `${authMode === "login" ? "Sign In" : "Sign Up"} with Phone`
                  )}
                </Button>
                  
                  <div className="relative">
                    <div className="absolute inset-0 flex items-center">
                      <span className="w-full border-t border-gray-300 dark:border-gray-600" />
                    </div>
                    <div className="relative flex justify-center text-xs uppercase">
                      <span className="bg-white dark:bg-gray-900 px-2 text-gray-500 dark:text-gray-400">
                        Or
                      </span>
                    </div>
                  </div>
                  
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full border-blue-300 dark:border-blue-600 hover:border-blue-400 dark:hover:border-blue-500 transition-all duration-200 hover:scale-[1.02] hover:shadow-lg"
                    onClick={() => handlePhoneOTP(phoneData)}
                    disabled={isLoading}
                  >
                    <Smartphone className="w-4 h-4 mr-2" />
                    Verify with SMS Code
                  </Button>
                </div>
              </form>
            </div>
          )}

          {/* Phone OTP Verification */}
          {showPhoneOTP && (
            <div className="space-y-6 animate-in slide-in-from-right-4 duration-500">
              <PhoneOTPInput
                phoneNumber={phoneNumber}
                onBack={handleOTPBack}
                onSuccess={handleOTPSuccess}
                onError={handleOTPError}
              />
            </div>
          )}

          {/* Email Login */}
          {loginMethod === "email" && authMode === "login" && (
            <div className="space-y-6 animate-in slide-in-from-right-4 duration-500">
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={resetToMethodSelection}
                  className="p-2"
                >
                  <ArrowLeft className="h-4 w-4" />
                </Button>
                <h2 className="text-lg font-semibold">Sign in with Email</h2>
              </div>

              <form
                onSubmit={handleEmailSubmit(handleEmailLogin)}
                className="space-y-4"
              >
                <div className="space-y-2">
                  <Label htmlFor="email">Email Address</Label>
                  <div className="relative">
                    <Input
                      id="email"
                      type="email"
                      placeholder="Enter your email"
                      {...registerEmail("email")}
                      className="pl-10"
                      disabled={isLoading}
                    />
                    <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                  </div>
                  {emailErrors.email && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {emailErrors.email.message}
                    </p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="login-password">Password</Label>
                  <div className="relative">
                    <Input
                      id="login-password"
                      type={showEmailPassword ? "text" : "password"}
                      placeholder="Enter your password"
                      {...registerEmail("password")}
                      className="pl-10 pr-12"
                      disabled={isLoading}
                    />
                    <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowEmailPassword(!showEmailPassword)}
                      className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
                      disabled={isLoading}
                    >
                      {showEmailPassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  {emailErrors.password && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {emailErrors.password.message}
                    </p>
                  )}
                </div>

                {/* Display login error */}
                {emailLoginError && (
                  <div 
                    key={`email-error-${emailLoginError}`}
                    className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg"
                  >
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {emailLoginError}
                    </p>
                    
                    {/* Show guidance for ambiguous cases */}
                    {emailLoginError.includes('Invalid login credentials. Please check your email and password, or sign up if you don\'t have an account.') && (
                      <div className="mt-3 pt-3 border-t border-red-200 dark:border-red-800">
                        <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
                          💡 Not sure what to do?
                        </p>
                        <div className="flex gap-2">
                          <button
                            type="button"
                            onClick={() => {
                              setEmailLoginError("");
                              // Keep user on login form to retry
                            }}
                            className="text-sm text-blue-600 dark:text-blue-400 hover:underline transition-colors duration-200"
                          >
                            Try again
                          </button>
                          <span className="text-gray-400">or</span>
                          <button
                            type="button"
                            onClick={() => setAuthMode('signup')}
                            className="text-sm text-green-600 dark:text-green-400 hover:underline transition-colors duration-200"
                          >
                            Create new account
                          </button>
                        </div>
                      </div>
                    )}
                    
                    {/* Show signup button for non-existing users */}
                    {emailLoginError.includes('No account found') && (
                      <div className="mt-2 pt-2 border-t border-red-200 dark:border-red-800">
                        <button
                          type="button"
                          onClick={() => setAuthMode('signup')}
                          className="text-sm text-green-600 dark:text-green-400 hover:underline transition-colors duration-200"
                        >
                          → Create new account
                        </button>
                      </div>
                    )}
                  </div>
                )}

                <div className="flex justify-end">
                  <button
                    type="button"
                    className="text-sm text-green-600 dark:text-green-400 hover:underline transition-colors duration-200"
                    onClick={handleForgotPassword}
                  >
                    Forgot Password?
                  </button>
                </div>

                <Button
                  type="submit"
                  className="w-full bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-700 hover:to-emerald-700 dark:from-green-500 dark:to-emerald-500 dark:hover:from-green-400 dark:hover:to-emerald-400 transition-all duration-200 hover:scale-[1.02] hover:shadow-lg shadow-lg"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <div className="flex items-center gap-2">
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                      Signing In...
                    </div>
                  ) : (
                    "Sign In"
                  )}
                </Button>
              </form>
            </div>
          )}

          {/* Email Sign Up */}
          {loginMethod === "email" && authMode === "signup" && (
            <div className="space-y-6 animate-in slide-in-from-right-4 duration-500">
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={resetToMethodSelection}
                  className="p-2"
                >
                  <ArrowLeft className="h-4 w-4" />
                </Button>
                <h2 className="text-lg font-semibold">Create Account</h2>
              </div>

              <form
                onSubmit={handleSignUpSubmit(handleSignUp)}
                className="space-y-4"
              >
                <div className="space-y-2">
                  <Label htmlFor="name">Full Name</Label>
                  <div className="relative">
                    <Input
                      id="name"
                      type="text"
                      placeholder="Enter your full name"
                      {...registerSignUp("name")}
                      className="pl-10"
                      disabled={isLoading}
                    />
                    <User className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                  </div>
                  {signUpErrors.name && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {signUpErrors.name.message}
                    </p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="signup-email">Email Address</Label>
                  <div className="relative">
                    <Input
                      id="signup-email"
                      type="email"
                      placeholder="Enter your email"
                      {...registerSignUp("email")}
                      className="pl-10"
                      disabled={isLoading}
                    />
                    <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                  </div>
                  {signUpErrors.email && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {signUpErrors.email.message}
                    </p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="signup-password">Password</Label>
                  <div className="relative">
                    <Input
                      id="signup-password"
                      type={showSignUpPassword ? "text" : "password"}
                  placeholder="Create a strong password"
                      {...registerSignUp("password")}
                      className="pl-10 pr-12"
                  disabled={isLoading}
                    />
                    <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowSignUpPassword(!showSignUpPassword)}
                      className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
                      disabled={isLoading}
                    >
                      {showSignUpPassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  {signUpErrors.password && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {signUpErrors.password.message}
                    </p>
                  )}
                </div>

                <div className="space-y-2">
                  <Label htmlFor="confirm-password">Confirm Password</Label>
                  <div className="relative">
                    <Input
                      id="confirm-password"
                      type={showConfirmPassword ? "text" : "password"}
                  placeholder="Confirm your password"
                      {...registerSignUp("confirmPassword")}
                      className="pl-10 pr-12"
                  disabled={isLoading}
                    />
                    <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                      className="absolute right-1 top-1/2 h-8 w-8 -translate-y-1/2 p-0 hover:bg-gray-100 dark:hover:bg-gray-800"
                      disabled={isLoading}
                    >
                      {showConfirmPassword ? (
                        <EyeOff className="h-4 w-4" />
                      ) : (
                        <Eye className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                  {signUpErrors.confirmPassword && (
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {signUpErrors.confirmPassword.message}
                    </p>
                  )}
                </div>

                <AnimatedCheckbox
                  checked={agreeToTerms}
                  onChange={(checked) =>
                    setSignUpValue("agreeToTerms", checked)
                  }
                  error={signUpErrors.agreeToTerms?.message}
                  disabled={isLoading}
                  label={
                    <span className="text-gray-700 dark:text-gray-300">
                      I agree to the {" "}
                      <a
                        href="/terms"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-green-600 dark:text-green-400 hover:underline"
                      >
                        Terms & Conditions
                      </a>{" "}
                      and {" "}
                      <a
                        href="/privacy"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-green-600 dark:text-green-400 hover:underline"
                      >
                        Privacy Policy
                      </a>
                    </span>
                  }
                />

                {/* Display signup error */}
                {signUpError && (
                  <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
                    <p className="text-sm text-red-600 dark:text-red-400">
                      {signUpError}
                    </p>
                    
                    {/* Show login button for existing users */}
                    {signUpError.includes('already exists') && (
                      <div className="mt-2 pt-2 border-t border-red-200 dark:border-red-800">
                        <button
                          type="button"
                          onClick={() => setAuthMode('login')}
                          className="text-sm text-blue-600 dark:text-blue-400 hover:underline transition-colors duration-200"
                        >
                          → Sign in instead
                        </button>
                      </div>
                    )}
                  </div>
                )}

                <Button
                  type="submit"
                  className="w-full bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-700 hover:to-emerald-700 dark:from-green-500 dark:to-emerald-500 dark:hover:from-green-400 dark:hover:to-emerald-400 transition-all duration-200 hover:scale-[1.02] hover:shadow-lg shadow-lg"
                  disabled={isLoading}
                >
                  {isLoading ? (
                    <div className="flex items-center gap-2">
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                      Creating Account...
                    </div>
                  ) : (
                    "Create Account"
                  )}
                </Button>
              </form>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Google Terms Modal */}
      <Dialog open={showGoogleTerms} onOpenChange={setShowGoogleTerms}>
        <DialogContent className="max-w-lg">
          <h2 className="text-xl font-bold mb-4">Accept Terms & Privacy</h2>
          <AnimatedCheckbox
            checked={agreeToTermsGoogle}
            onChange={setAgreeToTermsGoogle}
            error={googleTermsError}
            label={
              <span className="text-gray-700 dark:text-gray-300">
                I agree to the {" "}
                <a
                  href="/terms"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-green-600 dark:text-green-400 hover:underline"
                >
                  Terms & Conditions
                </a>{" "}
                and {" "}
                <a
                  href="/privacy"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-green-600 dark:text-green-400 hover:underline"
                >
                  Privacy Policy
                </a>
              </span>
            }
          />
          <Button
            className="w-full mt-4"
            onClick={() => {
              if (!agreeToTermsGoogle) {
                setGoogleTermsError("You must agree to the Terms & Conditions and Privacy Policy.");
                return;
              }
              setShowGoogleTerms(false);
              toast.success("Google authentication successful!");
              window.location.href = "/";
            }}
          >
            Continue
          </Button>
        </DialogContent>
      </Dialog>
    </div>
  );
}
