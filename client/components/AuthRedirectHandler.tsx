import { useEffect } from "react";
import { supabase } from "@/lib/supabaseClient";
import { useNavigate, useLocation } from "react-router-dom";

export function AuthRedirectHandler() {
  // Don't use useUserProfile here to avoid context dependency issues during HMR
  // Instead, use Supabase directly and update profile separately if needed
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    // Handle auth redirects without depending on UserProfile context
    // This makes the component more resilient during HMR and error recovery
    const checkAuth = async () => {
      try {
        const { data } = await supabase.auth.getUser();
      const user = data?.user;
        
        // Redirect authenticated users away from auth pages
        if (user?.email && user?.email_confirmed_at) {
          // If user is on login/signup page, redirect to home
          if (location.pathname === "/login" || location.pathname === "/signup") {
        navigate("/", { replace: true });
      }
        } else if (!user && location.pathname !== "/login" && location.pathname !== "/signup" && location.pathname !== "/reset-password" && location.pathname !== "/check-email" && location.pathname !== "/forgot-password") {
          // If user is not authenticated and not on an auth page, redirect to login
          // But don't redirect from public pages like /about, /terms, /privacy
          const publicPages = ["/about", "/terms", "/privacy"];
          if (!publicPages.includes(location.pathname)) {
            navigate("/login", { replace: true });
          }
        }
      } catch (error) {
        // Silently handle errors - this component shouldn't break the app
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.warn('AuthRedirectHandler: Error checking auth', error);
        }
      }
    };
    
    checkAuth();
    
    // Listen for auth state changes
    const { data: listener } = supabase.auth.onAuthStateChange((event, session) => {
      const user = session?.user;
      
      // Handle sign in events
      if (event === "SIGNED_IN" && user?.email && user?.email_confirmed_at) {
        // Redirect to home if on auth pages
        if (location.pathname === "/login" || location.pathname === "/signup") {
        navigate("/", { replace: true });
      }
      }
      
      // Handle sign out events
      if (event === "SIGNED_OUT") {
        // Redirect to login if not already there
        if (location.pathname !== "/login" && location.pathname !== "/signup") {
          navigate("/login", { replace: true });
        }
      }
    });
    
    return () => {
      listener?.subscription.unsubscribe();
    };
  }, [navigate, location.pathname]);

  return null;
} 