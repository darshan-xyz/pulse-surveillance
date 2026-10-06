import { useEffect, useState } from 'react';
import { supabase } from './supabaseClient';

export function useSupabaseUserId() {
  const [userId, setUserId] = useState<string | null>(null);
  const [restoring, setRestoring] = useState(true);

  useEffect(() => {
    supabase.auth.getUser().then(({ data }) => {
      setUserId(data?.user?.id ?? null);
      setRestoring(false);
    });
    const { data: listener } = supabase.auth.onAuthStateChange((_event, session) => {
      setUserId(session?.user?.id ?? null);
      setRestoring(false);
    });
    return () => {
      listener?.subscription.unsubscribe();
    };
  }, []);

  return { userId, restoring };
} 