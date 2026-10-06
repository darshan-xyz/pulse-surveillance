import { supabase } from './supabaseClient';

export async function fetchHistory(userId) {
  const { data, error } = await supabase
    .from('history')
    .select('*')
    .eq('user_id', userId)
    .order('timestamp', { ascending: false });
  if (error) throw error;
  return data;
}

export async function addHistory(event) {
  const { data, error } = await supabase.from('history').insert([event]);
  if (error) throw error;
  return data;
} 