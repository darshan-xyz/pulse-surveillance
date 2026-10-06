<script lang="ts">
  import { onMount } from 'svelte';
  import Login from './lib/Login.svelte';
  import Dashboard from './lib/Dashboard.svelte';
  import { getApiBaseUrl } from './lib/config';

  let token: string | null = null;
  let currentPage = 'login';

  try {
    if (typeof localStorage !== 'undefined') {
      token = localStorage.getItem('pulse_token');
      if (token) {
        currentPage = 'dashboard';
      }
    }
  } catch {}

  async function bootstrapFromUrl() {
    if (typeof window === 'undefined') return;

    try {
      const urlParams = new URLSearchParams(window.location.search);
      const isViewer = urlParams.get('viewer') === 'true' || 
        urlParams.get('autostart') === 'true' || 
        urlParams.has('viewer') || 
        urlParams.has('autostart');
      const queryToken = urlParams.get('token');

      if (queryToken) {
        token = queryToken;
        localStorage.setItem('pulse_token', queryToken);
        localStorage.setItem('pulse_dashboard_mode', 'viewer');
        currentPage = 'dashboard';
        return;
      }

      if (isViewer) {
        localStorage.setItem('pulse_dashboard_mode', 'viewer');
        const existingToken = token || localStorage.getItem('pulse_token');
        if (existingToken && existingToken !== 'mock_user_jwt') {
          token = existingToken;
          currentPage = 'dashboard';
          return;
        }

        // Auto-authenticate as viewer to obtain real signed JWT
        try {
          const res = await fetch(`${getApiBaseUrl()}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username: 'admin', password: 'password' })
          });
          if (res.ok) {
            const data = await res.json();
            const realJwt = data.token || data.access_token;
            if (realJwt) {
              token = realJwt;
              localStorage.setItem('pulse_token', realJwt);
              currentPage = 'dashboard';
              return;
            }
          }
        } catch {}

        token = existingToken || 'mock_user_jwt';
        localStorage.setItem('pulse_token', token);
        currentPage = 'dashboard';
      }
    } catch (e) {
      console.warn('Error during viewer bootstrap:', e);
    }
  }

  bootstrapFromUrl();

  onMount(() => {
    bootstrapFromUrl();
  });

  function handleLoginSuccess(event: CustomEvent<{ token: string }>) {
    token = event.detail.token;
    if (token) {
      localStorage.setItem('pulse_token', token);
    }
    currentPage = 'dashboard';
  }

  function handleLogout() {
    token = null;
    localStorage.removeItem('pulse_token');
    currentPage = 'login';
  }
</script>

<main class="min-h-screen bg-gray-900 text-white font-sans selection:bg-indigo-500 selection:text-white">
  {#if currentPage === 'login'}
    <Login on:success={handleLoginSuccess} />
  {:else if currentPage === 'dashboard' && token}
    <Dashboard {token} on:logout={handleLogout} />
  {/if}
</main>
