import { supabase } from './supabaseClient';

export async function logSupabaseDiagnostics() {
  // Check current user (silently, only log errors if debug enabled)
  const { data: userData, error: userError } = await supabase.auth.getUser();
  
  if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
    console.log('Supabase current user:', userData, userError);
  }

  // Try to fetch cameras table metadata (will fail if RLS blocks it)
  const { data: cameras, error: camerasError } = await supabase
    .from('cameras')
    .select('*')
    .limit(1);
    
  if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
    console.log('Supabase cameras table test fetch:', cameras, camerasError);
  }

  if (!userData?.user?.id) {
    return 'No Supabase user is logged in.';
  }
  if (camerasError) {
    // Only log actual errors
    if (camerasError.code !== 'PGRST116') { // PGRST116 is "no rows returned" which is fine
      console.error('Cameras table fetch error:', camerasError);
    }
    return 'Cameras table fetch failed: ' + camerasError.message;
  }
  if (!cameras) {
    return 'Cameras table fetch returned no data.';
  }
  return null;
}

export function logAuthCallbackUrlDiagnostics() {
  // Only log in debug mode
  if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SUPABASE === 'true') {
    const currentUrl = window.location.origin;
    console.log('Current window.location.origin:', currentUrl);
    // You must add this URL to your Supabase Auth > Settings > Redirect URLs
    // Example: http://localhost:5173 or your deployed domain
    // Check your Supabase dashboard for the allowed URLs
  }
} 