import { createClient } from '@supabase/supabase-js';

// Environment-based configuration with fallback mechanism
const getSupabaseUrl = () => {
  // First try environment variable
  if (import.meta.env.VITE_SUPABASE_URL) {
    return import.meta.env.VITE_SUPABASE_URL;
  }
  
  // Last resort fallback
  return 'https://bujskrxtswtogpamnflh.supabase.co';
};

const supabaseUrl = getSupabaseUrl();
const supabaseAnonKey = import.meta.env.VITE_SUPABASE_ANON_KEY || 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImJ1anNrcnh0c3d0b2dwYW1uZmxoIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NTMxNjMzMjMsImV4cCI6MjA2ODczOTMyM30.0ToD4XmfdZOWZEJCH5wWLKCTBm7QhQv7-YnMuNMK5oQ';

// Log current environment (only in development and if debug is enabled)
const isDevelopment = import.meta.env.DEV;
if (isDevelopment && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
console.log(`🚀 Supabase client initialized in ${isDevelopment ? 'DEVELOPMENT' : 'PRODUCTION'} mode`);
console.log('Supabase URL:', supabaseUrl);
console.log('Using proxy for Supabase:', isDevelopment);
}

// Proxy health tracking - skip proxy if it's consistently failing
let proxyFailureCount = 0;
const MAX_PROXY_FAILURES = 2; // Reduced from 3 to 2 for faster proxy disabling
let proxyDisabled = false;
const PROXY_DISABLE_DURATION = 30000; // Disable proxy for 30 seconds after failures (reduced from 1 minute)
let proxyDisabledUntil = 0;

// Check if proxy should be used
const shouldUseProxy = () => {
  if (!isDevelopment) return false;
  if (proxyDisabled) {
    // Re-enable proxy after cooldown period
    if (Date.now() > proxyDisabledUntil) {
      proxyDisabled = false;
      proxyFailureCount = 0;
      return true;
    }
    return false;
  }
  return true;
};

// Mark proxy as failed
const markProxyFailure = () => {
  proxyFailureCount++;
  if (proxyFailureCount >= MAX_PROXY_FAILURES) {
    proxyDisabled = true;
    proxyDisabledUntil = Date.now() + PROXY_DISABLE_DURATION;
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_PROXY === 'true') {
      console.warn(`Proxy disabled due to ${proxyFailureCount} failures. Will retry in ${PROXY_DISABLE_DURATION / 1000} seconds. Using direct connection.`);
    }
  }
};

// Mark proxy as successful (reset failure count)
const markProxySuccess = () => {
  if (proxyFailureCount > 0) {
    proxyFailureCount = 0;
  }
};

// Custom fetch for development to use Vite proxy with better error handling
const customFetch = isDevelopment ? async (url: string, options?: RequestInit) => {
  // Convert Supabase URL to use proxy paths
  const supabaseBaseUrl = 'https://bujskrxtswtogpamnflh.supabase.co';
  
  if (url.startsWith(supabaseBaseUrl)) {
    // Parse URL properly to separate path and query string
    try {
      const urlObj = new URL(url);
      const path = urlObj.pathname; // Just the path, without query string
      const queryString = urlObj.search; // Includes the '?' if present, empty string if not
      
      // Helper function to handle proxy requests with timeout and error handling
      // Increased timeout for better reliability on slow networks (10 seconds default)
      const fetchWithTimeout = async (fetchUrl: string, fetchOptions?: RequestInit, timeout = 10000): Promise<Response> => {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeout);
        
        try {
          const response = await fetch(fetchUrl, {
            ...fetchOptions,
            signal: controller.signal,
          });
          clearTimeout(timeoutId);
          
        // Check if response is valid (only log if debug enabled)
        if (!response.ok && response.status >= 500) {
          if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_PROXY === 'true') {
            console.warn(`Proxy returned error status ${response.status}, trying direct connection`);
          }
          // Create a proper error that can be caught and retried
          const error = new Error(`Proxy error: ${response.status}`);
          (error as any).status = response.status;
          (error as any).isRetryable = true;
          throw error;
        }
          
          return response;
        } catch (error: any) {
          clearTimeout(timeoutId);
          if (error.name === 'AbortError') {
            const timeoutError = new Error('Request timeout');
            (timeoutError as any).isRetryable = true;
            (timeoutError as any).isTimeout = true;
            throw timeoutError;
          }
          // Mark network errors as retryable
          if (error.message && (
            error.message.includes('Failed to fetch') ||
            error.message.includes('NetworkError') ||
            error.message.includes('ECONNRESET') ||
            error.message.includes('network')
          )) {
            (error as any).isRetryable = true;
          }
          throw error;
        }
      };
      
      // Route auth requests through /auth proxy with fast fallback and health tracking
    if (path.startsWith('/auth')) {
        // Skip proxy if it's disabled due to failures
        if (!shouldUseProxy()) {
          // Proxy is disabled, go directly to Supabase
          return await fetchWithTimeout(url, options, 10000);
        }
        
        const proxyUrl = `${window.location.origin}${path}${queryString}`;
        // Only log in development and for errors
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_PROXY === 'true') {
      console.log('Proxying auth request:', proxyUrl);
        }
        try {
          // Use reasonable timeout for proxy (5 seconds) for faster fallback while still being reliable
          const response = await fetchWithTimeout(proxyUrl, options, 5000);
          markProxySuccess(); // Proxy worked, reset failure count
          return response;
        } catch (error: any) {
          markProxyFailure(); // Mark proxy as failed
          // Fallback to direct connection if proxy fails (10 second timeout for better reliability)
          try {
            return await fetchWithTimeout(url, options, 10000);
          } catch (directError: any) {
            // Both proxy and direct failed - check if it's a network/DNS error
            const isNetworkError = 
              directError.message?.includes('NetworkError') ||
              directError.message?.includes('Failed to fetch') ||
              directError.message?.includes('ENOTFOUND') ||
              directError.message?.includes('CORS') ||
              directError.message?.includes('timeout') ||
              directError.message?.includes('Request timeout') ||
              directError.isTimeout;
            
            // Only log if it's not a common network error (these are expected when offline/DNS fails)
            if (!isNetworkError) {
              console.error('Both proxy and direct connection failed:', directError.message);
            }
            
            // Return a proper error response instead of throwing
            return new Response(
              JSON.stringify({ 
                error: isNetworkError ? 'Network error' : 'Connection failed', 
                message: isNetworkError 
                  ? 'Unable to connect to server. Please check your internet connection.'
                  : (directError.message || 'Failed to connect to authentication server')
              }), 
              { 
                status: 503, 
                statusText: 'Service Unavailable',
                headers: { 'Content-Type': 'application/json' }
              }
            );
          }
        }
      }
      
      // Route rest requests through /rest proxy with fast fallback and health tracking
      if (path.startsWith('/rest')) {
        // Skip proxy if it's disabled due to failures
        if (!shouldUseProxy()) {
          // Proxy is disabled, go directly to Supabase
          return await fetchWithTimeout(url, options, 10000);
        }
        
        const proxyUrl = `${window.location.origin}${path}${queryString}`;
        // Only log in development and for errors
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_PROXY === 'true') {
      console.log('Proxying rest request:', proxyUrl);
        }
        try {
          // Use reasonable timeout for proxy (5 seconds) for faster fallback while still being reliable
          const response = await fetchWithTimeout(proxyUrl, options, 5000);
          markProxySuccess(); // Proxy worked, reset failure count
          return response;
        } catch (error: any) {
          markProxyFailure(); // Mark proxy as failed
          // Fallback to direct connection if proxy fails (10 second timeout for better reliability)
          try {
            return await fetchWithTimeout(url, options, 10000);
          } catch (directError: any) {
            // Both proxy and direct failed - check if it's a network/DNS error
            const isNetworkError = 
              directError.message?.includes('NetworkError') ||
              directError.message?.includes('Failed to fetch') ||
              directError.message?.includes('ENOTFOUND') ||
              directError.message?.includes('CORS') ||
              directError.message?.includes('timeout') ||
              directError.message?.includes('Request timeout') ||
              directError.isTimeout;
            
            // Only log if it's not a common network error (these are expected when offline/DNS fails)
            if (!isNetworkError) {
              console.error('Both proxy and direct connection failed:', directError.message);
            }
            
            return new Response(
              JSON.stringify({ 
                error: isNetworkError ? 'Network error' : 'Connection failed', 
                message: isNetworkError 
                  ? 'Unable to connect to server. Please check your internet connection.'
                  : (directError.message || 'Failed to connect to database server')
              }), 
              { 
                status: 503, 
                statusText: 'Service Unavailable',
                headers: { 'Content-Type': 'application/json' }
              }
            );
          }
        }
      }
      
      // For other paths, use the proxy base with fallback and health tracking
      if (!shouldUseProxy()) {
        // Proxy is disabled, go directly to Supabase
        return await fetchWithTimeout(url, options, 10000);
      }
      
      const proxyUrl = `${window.location.origin}${path}${queryString}`;
      try {
        const response = await fetchWithTimeout(proxyUrl, options, 5000);
        markProxySuccess(); // Proxy worked, reset failure count
        return response;
      } catch (error: any) {
        markProxyFailure(); // Mark proxy as failed
        // Fallback to direct connection if proxy fails
        try {
          return await fetchWithTimeout(url, options, 10000);
        } catch (directError: any) {
          // Both proxy and direct failed - check if it's a network/DNS error
          const isNetworkError = 
            directError.message?.includes('NetworkError') ||
            directError.message?.includes('Failed to fetch') ||
            directError.message?.includes('ENOTFOUND') ||
            directError.message?.includes('CORS') ||
            directError.message?.includes('timeout') ||
            directError.message?.includes('Request timeout') ||
            directError.isTimeout;
          
          // Only log if it's not a common network error (these are expected when offline/DNS fails)
          if (!isNetworkError) {
            console.error('Both proxy and direct connection failed:', directError.message);
          }
          
          return new Response(
            JSON.stringify({ 
              error: isNetworkError ? 'Network error' : 'Connection failed', 
              message: isNetworkError 
                ? 'Unable to connect to server. Please check your internet connection.'
                : (directError.message || 'Failed to connect to server')
            }), 
            { 
              status: 503, 
              statusText: 'Service Unavailable',
              headers: { 'Content-Type': 'application/json' }
            }
          );
        }
      }
    } catch (parseError) {
      // If URL parsing fails, fall back to direct connection
      // Only log if it's not a NetworkError (which is expected when offline/DNS fails)
      if (!(parseError instanceof TypeError && parseError.message?.includes('NetworkError'))) {
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_PROXY === 'true') {
          console.warn('Failed to parse URL, using direct connection:', parseError);
        }
      }
      // Try direct connection, but catch CORS errors gracefully
      try {
        return await fetch(url, options);
      } catch (directError: any) {
        // If direct connection also fails (CORS, network, etc.), return a proper error response
        const isNetworkError = 
          directError.message?.includes('NetworkError') ||
          directError.message?.includes('Failed to fetch') ||
          directError.message?.includes('CORS');
        
        if (isNetworkError) {
          // Return a proper error response for network/CORS issues
          return new Response(
            JSON.stringify({ 
              error: 'Network error', 
              message: 'Unable to connect to server. Please check your internet connection.' 
            }), 
            { 
              status: 503, 
              statusText: 'Service Unavailable',
              headers: { 'Content-Type': 'application/json' }
            }
          );
        }
        throw directError;
      }
    }
  }
  
  // For non-Supabase URLs, use normal fetch
  return fetch(url, options);
} : undefined;

// Suppress WebSocket connection errors in development (they're expected during HMR)
if (isDevelopment && typeof window !== 'undefined') {
  const originalConsoleError = console.error;
  console.error = (...args: any[]) => {
    // Suppress Supabase Realtime WebSocket connection interruption errors in development
    const message = args[0]?.toString() || '';
    if (
      message.includes('The connection to wss://') &&
      message.includes('supabase.co/realtime') &&
      (message.includes('was interrupted') || message.includes('interrupted while'))
    ) {
      // Silently ignore - these are expected during HMR and page reloads
      return;
    }
    // Pass through all other errors
    originalConsoleError.apply(console, args);
  };
}

// Create Supabase client instance
// Note: During HMR, this module may be reloaded, which can cause the "Multiple GoTrueClient instances" warning.
// This is expected in development and doesn't affect functionality. The warning can be safely ignored.
// WebSocket connection interruptions are also expected during development and don't affect functionality.
export const supabase = createClient(supabaseUrl, supabaseAnonKey, {
  auth: {
    storage: localStorage,
    persistSession: true,
    autoRefreshToken: true,
    detectSessionInUrl: true,
    flowType: 'pkce',
    // Use a consistent storage key to minimize conflicts
    storageKey: 'supabase.auth.token',
  },
  global: {
    fetch: customFetch,
  },
  realtime: {
    // Configure Realtime to be more resilient in development
    params: {
      eventsPerSecond: 10,
    },
    // Suppress connection errors in development
    log_level: isDevelopment ? 'error' : 'info',
  },
});

// Enhanced authentication helper
export async function ensureAuthenticated() {
  const { data: { user }, error } = await supabase.auth.getUser();
  if (error || !user) {
    throw new Error('User not authenticated');
  }
  return user;
}

// Enhanced session management
export async function getCurrentSession() {
  const { data: { session }, error } = await supabase.auth.getSession();
  if (error) {
    console.error('Error getting session:', error);
    return null;
  }
  return session;
}

// Phone/email sign up (upsert) - Enhanced with better error handling
export async function phoneEmailSignUp({ phone, email }: { phone: string, email: string }) {
  try {
    const { data, error } = await supabase.from('phone_users').upsert([{ phone, email }], { onConflict: 'phone' });
    if (error) {
      console.error('Phone/email signup error:', error);
      throw error;
    }
    return { data, error: null };
  } catch (error) {
    console.error('Phone/email signup failed:', error);
    return { data: null, error };
  }
}

// Phone/email login (lookup only) - Enhanced with better error handling
export async function phoneEmailLogin({ phone }: { phone: string }) {
  try {
    let query = supabase.from('phone_users').select('*').eq('phone', phone);
    const { data, error } = await query.single();
    if (error) {
      console.error('Phone/email login error:', error);
      throw error;
    }
    return { data, error: null };
  } catch (error) {
    console.error('Phone/email login failed:', error);
    return { data: null, error };
  }
}

// Link phone to email (upsert) - Enhanced with better error handling
export async function linkPhoneToEmail({ email, phone }: { email: string, phone: string }) {
  try {
    // Upsert: if a row with this email or phone exists, update; else insert
    const { data, error } = await supabase.from('phone_users').upsert([{ email, phone }], { onConflict: 'email' });
    if (error) {
      console.error('Link phone to email error:', error);
      throw error;
    }
    return { data, error: null };
  } catch (error) {
    console.error('Link phone to email failed:', error);
    return { data: null, error };
  }
}

// Enhanced login validation with clear user guidance and better error handling
export async function validateLoginCredentials(email: string, password: string) {
  try {
    // Only log in debug mode
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
    console.log('Starting login validation for:', email);
    }
    
    // Try to sign in directly with Supabase with timeout
    const loginPromise = supabase.auth.signInWithPassword({
      email,
      password,
    });
    
    // Add timeout to prevent hanging
    const timeoutPromise = new Promise((_, reject) => {
      setTimeout(() => reject(new Error('Login request timeout. Please check your connection and try again.')), 30000);
    });
    
    let result;
    try {
      result = await Promise.race([loginPromise, timeoutPromise]) as any;
    } catch (raceError: any) {
      console.error('Login request failed or timed out:', raceError);
      return {
        data: null,
        error: { 
          message: raceError.message || 'Connection error. Please check your internet connection and try again.' 
        },
        userExists: null,
        isWrongPassword: null,
        needsGuidance: false
      };
    }
    
    const { data, error } = result;
    
    if (error) {
      // Only log in debug mode
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Login failed with error:', error.message);
      }
      const errorMessage = error.message.toLowerCase();
      
      // Handle JSON parse errors or connection errors
      if (
        errorMessage.includes('json.parse') ||
        errorMessage.includes('unexpected end of data') ||
        errorMessage.includes('failed to fetch') ||
        errorMessage.includes('network error') ||
        errorMessage.includes('connection') ||
        errorMessage.includes('timeout')
      ) {
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
          console.log('Connection or parsing error detected');
        }
        return {
          data: null,
          error: { 
            message: 'Connection error. Please check your internet connection and try again. If the problem persists, the server may be temporarily unavailable.' 
          },
          userExists: null,
          isWrongPassword: null,
          needsGuidance: false
        };
      }
      
      // Supabase returns "Invalid login credentials" for both wrong password and non-existent user
      // Since we can't reliably determine which case it is, we'll provide clear guidance
      
      if (errorMessage.includes('invalid login credentials')) {
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('Invalid login credentials - providing user guidance');
        }
        return { 
          data: null, 
          error: { message: 'Invalid login credentials. Please check your email and password, or sign up if you don\'t have an account.' }, 
          userExists: null, 
          isWrongPassword: null,
          needsGuidance: true
        };
      }
      
      // Handle other specific errors
      if (
        errorMessage.includes('user not found') ||
        errorMessage.includes('email not found') ||
        errorMessage.includes('no user found')
      ) {
        // Only log in debug mode
        if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
        console.log('User not found error detected');
        }
        return { 
          data: null, 
          error: { message: 'No account found. Please sign up first.' }, 
          userExists: false, 
          isWrongPassword: false 
        };
      }
      
      // Other errors (only log in debug mode)
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
      console.log('Other error detected:', error.message);
      }
      return { 
        data: null, 
        error, 
        userExists: null, 
        isWrongPassword: null 
      };
    }
    
    // Login successful (only log in debug mode)
    if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
    console.log('Login successful');
    }
    return { data, error: null, userExists: true, isWrongPassword: false };
  } catch (error: any) {
    console.error('Login validation failed with exception:', error);
    const errorMessage = error?.message?.toLowerCase() || '';
    
    // Handle specific error types
    if (
      errorMessage.includes('json.parse') ||
      errorMessage.includes('unexpected end of data') ||
      errorMessage.includes('failed to fetch') ||
      errorMessage.includes('network error')
    ) {
      return {
        data: null,
        error: { 
          message: 'Connection error. Please check your internet connection and try again.' 
        },
        userExists: null,
        isWrongPassword: null,
        needsGuidance: false
      };
    }
    
    return { 
      data: null, 
      error: { message: error?.message || 'Login failed. Please try again.' }, 
      userExists: null, 
      isWrongPassword: null 
    };
  }
}