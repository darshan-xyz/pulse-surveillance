import { supabase } from './supabaseClient';

export function subscribeToHistoryEvents(onNewEvent) {
  return supabase
    .channel('public:history')
    .on('postgres_changes', { event: 'INSERT', schema: 'public', table: 'history' }, payload => {
      onNewEvent(payload.new);
    })
    .subscribe();
} 