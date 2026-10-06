<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import CameraStream from './CameraStream.svelte';
  import { getApiBaseUrl } from './config';
  import { globalWebcamBroadcaster } from './webcamBroadcaster';

  export let token: string;
  const dispatch = createEventDispatcher();
  
  let devices: any[] = [];
  let errorMsg = '';
  let loading = true;
  let gridColumns = 3; // Default 3-column multi-camera grid layout
  let searchQuery = '';
  let selectedCameraId = 'all';
  let viewLayout: 'grid' | 'theater' = 'grid';
  let activeTheaterCameraId = '';
  let isBroadcasting = false;
  let broadcastViewerCount = 0;
  let broadcastError: string | null = null;

  // Dashboard mode: 'home' (camera management & local webcam enabled) | 'viewer' (clean view-only)
  function getInitialMode(): 'home' | 'viewer' {
    try {
      if (typeof localStorage !== 'undefined' && localStorage.getItem('pulse_dashboard_mode') === 'viewer') {
        return 'viewer';
      }
    } catch {}
    return 'home';
  }

  let mode: 'home' | 'viewer' = getInitialMode();

  function setMode(newMode: 'home' | 'viewer') {
    mode = newMode;
    if (typeof localStorage !== 'undefined') {
      try {
        localStorage.setItem('pulse_dashboard_mode', mode);
      } catch {}
    }
    if (mode === 'viewer') {
      if (selectedCameraId === 'local-webcam') {
        selectedCameraId = 'all';
      }
      if (activeTheaterCameraId === 'local-webcam') {
        activeTheaterCameraId = filteredDevices[0]?.device_id || '';
      }
    }
  }

  function toggleMode() {
    setMode(mode === 'home' ? 'viewer' : 'home');
  }

  // Update active theater camera ID if needed when layout or devices change
  $: if (viewLayout === 'theater') {
    const availableIds = [
      ...(mode === 'home' ? ['local-webcam'] : []),
      ...filteredDevices.map(d => d.device_id)
    ];
    if (!activeTheaterCameraId || !availableIds.includes(activeTheaterCameraId)) {
      activeTheaterCameraId = availableIds[0] || '';
    }
  }

  // Emergency Fire & Smoke Warning State
  interface EmergencyAlert {
    cameraId: string;
    cameraName: string;
    type: 'fire' | 'smoke';
    confidence: number;
    timestamp: number;
    expiresAt: number;
  }

  let activeAlerts: EmergencyAlert[] = [];
  let alertCleanupInterval: ReturnType<typeof setInterval> | null = null;

  function handleCameraDetection(event: CustomEvent<{ camera_id?: string; detections?: any[]; timestamp?: number }>) {
    const { camera_id, detections, timestamp } = event.detail || {};
    if (!detections || !Array.isArray(detections) || detections.length === 0) return;

    const validDetections = detections.filter((d: any) => {
      const conf = typeof d.confidence === 'number' ? d.confidence : (typeof d.score === 'number' ? d.score : 1.0);
      return conf >= 0.30;
    });

    const fireDet = validDetections.find((d: any) => {
      if (d.class_id === 0) return true;
      const cls = (d.class || d.class_name || '').toLowerCase();
      return cls.includes('fire');
    });

    const smokeDet = validDetections.find((d: any) => {
      if (d.class_id === 1) return true;
      const cls = (d.class || d.class_name || '').toLowerCase();
      return cls.includes('smoke');
    });

    const camId = camera_id || 'Camera';
    const camName = camId === 'local-webcam' ? 'Local Webcam' : camId;
    const now = Date.now();

    if (fireDet) {
      const conf = typeof fireDet.confidence === 'number' ? fireDet.confidence : (typeof fireDet.score === 'number' ? fireDet.score : 0.85);
      updateEmergencyAlert({
        cameraId: camId,
        cameraName: camName,
        type: 'fire',
        confidence: conf,
        timestamp: timestamp || now,
        expiresAt: now + 3000
      });
    } else if (smokeDet) {
      const conf = typeof smokeDet.confidence === 'number' ? smokeDet.confidence : (typeof smokeDet.score === 'number' ? smokeDet.score : 0.72);
      updateEmergencyAlert({
        cameraId: camId,
        cameraName: camName,
        type: 'smoke',
        confidence: conf,
        timestamp: timestamp || now,
        expiresAt: now + 3000
      });
    }
  }

  function updateEmergencyAlert(newAlert: EmergencyAlert) {
    const idx = activeAlerts.findIndex(a => a.cameraId === newAlert.cameraId && a.type === newAlert.type);
    if (idx >= 0) {
      activeAlerts[idx] = newAlert;
      activeAlerts = [...activeAlerts];
    } else {
      activeAlerts = [newAlert, ...activeAlerts];
    }
  }

  function focusCamera(camId: string) {
    selectedCameraId = camId;
    if (viewLayout === 'theater') {
      activeTheaterCameraId = camId;
    }
  }

  function dismissAlerts() {
    activeAlerts = [];
  }

  async function fetchDevices() {
    loading = true;
    errorMsg = '';
    try {
      const res = await fetch(`${getApiBaseUrl()}/api/devices`, {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      
      if (res && res.ok) {
        devices = await res.json();
      } else {
        if (res && res.status === 401) {
          logout();
          return;
        }
        errorMsg = 'Failed to load devices';
        devices = [];
      }
    } catch (err: any) {
      errorMsg = 'Failed to load devices';
      devices = [];
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    try {
      if (typeof window !== 'undefined' && window.location.search) {
        const params = new URLSearchParams(window.location.search);
        const cam = params.get('camera') || params.get('camera_id') || params.get('cam') || params.get('deviceId');
        if (cam) {
          selectedCameraId = cam;
          activeTheaterCameraId = cam;
        }
        if (params.get('layout') === 'theater') {
          viewLayout = 'theater';
        }
        const cols = parseInt(params.get('cols') || params.get('columns') || '0', 10);
        if ([1, 2, 3, 4].includes(cols)) {
          gridColumns = cols;
        }
      }
    } catch {}
    fetchDevices();

    alertCleanupInterval = setInterval(() => {
      const now = Date.now();
      if (activeAlerts.length > 0) {
        activeAlerts = activeAlerts.filter(a => a.expiresAt > now);
      }
    }, 500);
  });

  function logout() {
    dispatch('logout');
  }

  function setGridColumns(cols: number) {
    gridColumns = cols;
  }

  $: if (mode === 'home' && !isBroadcasting && typeof window !== 'undefined' && navigator && navigator.mediaDevices) {
    autoStartBroadcast();
  }

  async function autoStartBroadcast() {
    try {
      globalWebcamBroadcaster.setToken(token);
      globalWebcamBroadcaster.setStatusCallback((status) => {
        isBroadcasting = status.isBroadcasting;
        broadcastViewerCount = status.viewerCount;
        broadcastError = status.error;
      });
      await globalWebcamBroadcaster.start();
      isBroadcasting = true;
    } catch {}
  }

  async function toggleWebcamBroadcast() {
    if (isBroadcasting) {
      await globalWebcamBroadcaster.stop();
      isBroadcasting = false;
      await fetchDevices();
    } else {
      try {
        globalWebcamBroadcaster.setToken(token);
        globalWebcamBroadcaster.setStatusCallback((status) => {
          isBroadcasting = status.isBroadcasting;
          broadcastViewerCount = status.viewerCount;
          broadcastError = status.error;
        });
        await globalWebcamBroadcaster.start();
        isBroadcasting = true;
        await fetchDevices();
      } catch (err: any) {
        isBroadcasting = false;
      }
    }
  }

  onDestroy(() => {
    if (alertCleanupInterval) {
      clearInterval(alertCleanupInterval);
      alertCleanupInterval = null;
    }
  });

  $: filteredDevices = Array.isArray(devices) ? devices.filter(d => 
    !searchQuery || (d && d.device_id && d.device_id.toLowerCase().includes(searchQuery.toLowerCase()))
  ) : [];
</script>

<div class="min-h-screen bg-gray-950 text-gray-100 flex flex-col font-sans">
  <!-- Top Navigation Bar -->
  <nav class="bg-gray-900/90 backdrop-blur-md border-b border-gray-800 px-6 py-3.5 flex justify-between items-center sticky top-0 z-50 shadow-lg">
    <div class="flex items-center space-x-3.5">
      <div class="w-9 h-9 rounded-xl bg-gradient-to-tr from-indigo-600 to-indigo-500 flex items-center justify-center shadow-md shadow-indigo-500/20">
        <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"></path>
        </svg>
      </div>
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-xl font-bold text-white tracking-tight leading-none">Pulse Dashboard</h1>
          {#if mode === 'viewer'}
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-semibold bg-cyan-950/80 text-cyan-400 border border-cyan-700/60 shadow-sm" data-testid="viewer-mode-badge">
              <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
              </svg>
              <span>Viewer Mode</span>
            </span>
          {:else}
            <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-semibold bg-indigo-950/80 text-indigo-300 border border-indigo-700/50 shadow-sm" data-testid="home-mode-badge">
              <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
              </svg>
              <span>Home Console</span>
            </span>
          {/if}
        </div>
        <span class="text-xs text-indigo-400 font-medium">
          {mode === 'viewer' ? 'Remote View-Only Surveillance' : 'Enterprise Surveillance System'}
        </span>
      </div>
    </div>

    <!-- Navigation Right Controls -->
    <div class="flex items-center space-x-3">
      <!-- Mode Switcher Toggle Button -->
      <button 
        type="button"
        on:click={toggleMode}
        aria-label="Toggle Dashboard Mode"
        class="px-3 py-1.5 text-xs font-semibold rounded-lg border transition-all shadow-sm flex items-center gap-1.5 cursor-pointer {
          mode === 'viewer' 
            ? 'bg-cyan-950/60 hover:bg-cyan-900/80 text-cyan-300 border-cyan-700/60 hover:border-cyan-500' 
            : 'bg-indigo-950/60 hover:bg-indigo-900/80 text-indigo-300 border-indigo-700/60 hover:border-indigo-500'
        }"
        title={mode === 'viewer' ? 'Switch to Home Mode' : 'Switch to Viewer Mode'}
      >
        {#if mode === 'viewer'}
          <svg class="w-3.5 h-3.5 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
          </svg>
          <span>Switch to Home Mode</span>
        {:else}
          <svg class="w-3.5 h-3.5 text-indigo-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
          </svg>
          <span>Switch to Viewer Mode</span>
        {/if}
      </button>

      <div class="hidden sm:flex items-center space-x-1.5 px-3 py-1 bg-green-950/60 border border-green-800/40 rounded-full text-xs text-green-400 font-medium">
        <span class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></span>
        <span>System Operational</span>
      </div>

      <button 
        on:click={logout} 
        class="px-4 py-2 text-sm font-semibold text-gray-200 hover:text-white bg-gray-800 hover:bg-gray-700 active:bg-gray-900 border border-gray-700/80 rounded-lg transition-all shadow-sm flex items-center gap-1.5"
      >
        <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
        </svg>
        <span>Sign Out</span>
      </button>
    </div>
  </nav>

  <!-- Main Content Area -->
  <main class="flex-1 p-6 max-w-7xl w-full mx-auto space-y-6">
    <!-- Top-Level Emergency Fire & Smoke Warning Banner -->
    {#if activeAlerts.length > 0}
      <div data-testid="emergency-warning-banner" class="rounded-xl overflow-hidden bg-gradient-to-r from-red-600 via-red-500 to-amber-600 p-4 text-white shadow-xl animate-pulse border border-red-400 flex flex-col md:flex-row md:items-center md:justify-between gap-3">
        <div class="flex items-center space-x-3">
          <div class="p-2 bg-white/20 rounded-lg backdrop-blur-sm">
            <svg class="w-6 h-6 text-white animate-bounce" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
          </div>
          <div>
            <h4 class="text-base font-extrabold tracking-wide uppercase flex items-center gap-2">
              <span>⚠️ Emergency Alert: {activeAlerts.some(a => a.type === 'fire') ? 'Fire' : 'Smoke'} Detected</span>
            </h4>
            <div class="text-xs text-red-100 mt-0.5 flex flex-wrap gap-2">
              {#each activeAlerts as alert}
                <span class="inline-flex items-center gap-1 bg-black/30 px-2 py-0.5 rounded font-mono">
                  <strong>{alert.cameraName || alert.cameraId}</strong> ({alert.type.toUpperCase()}: {Math.round(alert.confidence * 100)}%)
                </span>
              {/each}
            </div>
          </div>
        </div>
        <div class="flex items-center space-x-2">
          {#if activeAlerts[0]}
            <button
              type="button"
              on:click={() => focusCamera(activeAlerts[0].cameraId)}
              class="px-3.5 py-1.5 bg-white text-red-700 font-bold text-xs rounded-lg shadow hover:bg-gray-100 transition-colors flex items-center space-x-1 cursor-pointer"
            >
              <span>Focus Camera ({activeAlerts[0].cameraName || activeAlerts[0].cameraId})</span>
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" /></svg>
            </button>
          {/if}
          <button
            type="button"
            on:click={dismissAlerts}
            class="p-1.5 text-white/80 hover:text-white hover:bg-white/20 rounded-lg transition-colors cursor-pointer"
            title="Dismiss Alert"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
      </div>
    {/if}

    {#if loading}
      <div class="flex flex-col justify-center items-center h-80 space-y-4">
        <div class="animate-spin rounded-full h-12 w-12 border-3 border-indigo-500/20 border-t-indigo-500"></div>
        <p class="text-sm text-gray-400 font-medium">Discovering and initializing camera feeds...</p>
      </div>
    {:else if errorMsg}
      <div class="bg-red-950/60 border border-red-800/80 text-red-200 p-6 rounded-2xl shadow-xl max-w-xl mx-auto text-center space-y-4">
        <div class="w-12 h-12 bg-red-900/40 rounded-full flex items-center justify-center mx-auto text-red-400">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        </div>
        <div>
          <h3 class="text-lg font-bold text-white">Connection Error</h3>
          <p class="text-sm text-red-300 mt-1">{errorMsg}</p>
        </div>
        <div class="flex items-center justify-center gap-3 pt-2">
          <button 
            type="button" 
            on:click={fetchDevices} 
            class="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold rounded-lg shadow transition cursor-pointer"
          >
            Retry Connection
          </button>
          <button 
            type="button" 
            on:click={logout} 
            class="px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs font-semibold rounded-lg border border-gray-700 transition cursor-pointer"
          >
            Return to Login
          </button>
        </div>
      </div>
    {:else}
      <!-- Header Bar with Camera Selection, Search & Multi-Camera Controls -->
      <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 pb-2 border-b border-gray-800/60">
        <div>
          <h2 class="text-2xl font-bold text-white tracking-tight">Live Overview</h2>
          <p class="text-sm text-gray-400 mt-1">Real-time low-latency surveillance</p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <!-- Camera Selection Dropdown -->
          <div class="flex items-center gap-2">
            <label for="camera-select" class="text-xs text-gray-400 font-medium hidden sm:inline">Camera:</label>
            <select
              id="camera-select"
              aria-label="Camera Selection"
              bind:value={selectedCameraId}
              class="bg-gray-900 border border-gray-800 text-xs text-white rounded-lg px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-indigo-500 cursor-pointer font-medium"
            >
              <option value="all">All Cameras (Grid View)</option>
              {#if mode === 'home'}
                <option value="local-webcam">Local Webcam</option>
              {/if}
              {#if Array.isArray(devices) && devices.length > 0}
                <optgroup label="Edge Cameras">
                  {#each devices as device (device.device_id)}
                    <option value={device.device_id}>{device.device_id}</option>
                  {/each}
                </optgroup>
              {/if}
            </select>
          </div>

          <!-- Broadcast Webcam Button (Home Console Feature) -->
          {#if mode === 'home'}
            <button
              type="button"
              on:click={toggleWebcamBroadcast}
              class="text-xs px-3 py-1.5 rounded-lg border font-semibold flex items-center gap-1.5 transition-all cursor-pointer select-none {
                isBroadcasting
                  ? 'bg-red-950/80 border-red-500 text-red-300 shadow-md shadow-red-500/20 ring-1 ring-red-500'
                  : 'bg-gray-900 hover:bg-gray-800 border-gray-700 text-gray-300 hover:text-white'
              }"
              title={isBroadcasting ? 'Broadcasting live to remote viewers' : 'Broadcast physical webcam to remote viewers'}
              data-testid="broadcast-webcam-btn"
            >
              <span class="w-2 h-2 rounded-full {isBroadcasting ? 'bg-red-500 animate-pulse' : 'bg-gray-500'}"></span>
              <span>{isBroadcasting ? `Broadcasting (${broadcastViewerCount} Viewers)` : 'Broadcast Webcam'}</span>
            </button>
          {/if}

          <!-- Search / Filter Input (only relevant in grid view or theater thumbnail filter) -->
          {#if selectedCameraId === 'all'}
            <div class="relative">
              <input 
                type="text"
                bind:value={searchQuery}
                placeholder="Filter cameras..."
                class="w-36 sm:w-48 pl-8 pr-3 py-1.5 bg-gray-900 border border-gray-800 rounded-lg text-xs text-white placeholder-gray-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 transition-all"
              />
              <svg class="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
            </div>
          {/if}

          <!-- Active Node Count -->
          <div class="text-xs px-3 py-1.5 bg-green-950/50 text-green-400 border border-green-800/50 rounded-full font-semibold flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-green-400 animate-pulse"></span>
            <span>{filteredDevices.length} Active Node{filteredDevices.length !== 1 ? 's' : ''}</span>
          </div>

          <!-- Multi-Camera Grid & Theater Layout Selector (only relevant when selectedCameraId === 'all') -->
          {#if selectedCameraId === 'all'}
            <div class="flex items-center bg-gray-900 border border-gray-800 rounded-lg p-0.5" data-testid="layout-controls">
              <button 
                type="button"
                on:click={() => { viewLayout = 'grid'; setGridColumns(1); }}
                class="px-2.5 py-1 text-xs font-semibold rounded-md transition {viewLayout === 'grid' && gridColumns === 1 ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}"
                title="1-Camera Focus View"
              >
                1 Cam
              </button>
              <button 
                type="button"
                on:click={() => { viewLayout = 'grid'; setGridColumns(2); }}
                class="px-2.5 py-1 text-xs font-semibold rounded-md transition {viewLayout === 'grid' && gridColumns === 2 ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}"
                title="2-Camera Dual View"
              >
                2 Cams
              </button>
              <button 
                type="button"
                on:click={() => { viewLayout = 'grid'; setGridColumns(3); }}
                class="px-2.5 py-1 text-xs font-semibold rounded-md transition {viewLayout === 'grid' && gridColumns === 3 ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}"
                title="3-Camera Grid View"
              >
                3 Cams
              </button>
              <button 
                type="button"
                on:click={() => { viewLayout = 'grid'; setGridColumns(4); }}
                class="px-2.5 py-1 text-xs font-semibold rounded-md transition {viewLayout === 'grid' && gridColumns === 4 ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}"
                title="4-Camera Quad View"
              >
                4 Cams
              </button>
              <div class="w-px h-4 bg-gray-800 mx-1"></div>
              <button 
                type="button"
                on:click={() => { viewLayout = 'theater'; }}
                class="px-2.5 py-1 text-xs font-semibold rounded-md transition flex items-center gap-1.5 cursor-pointer {viewLayout === 'theater' ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}"
                title="Theater Mode (16:9 Main Player + Thumbnail Switcher)"
                data-testid="theater-mode-btn"
              >
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
                <span>Theater</span>
              </button>
            </div>
          {/if}

          <!-- Quick Refresh Button -->
          <button 
            type="button"
            on:click={fetchDevices} 
            class="p-1.5 bg-gray-900 hover:bg-gray-800 text-gray-300 hover:text-white border border-gray-800 rounded-lg transition cursor-pointer"
            title="Refresh Cameras"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>
      
      <!-- Video Stream / Grid / Theater Content Area -->
      {#if selectedCameraId === 'local-webcam'}
        <div class="max-w-4xl mx-auto w-full">
          <CameraStream deviceId="local-webcam" {token} broadcasting={isBroadcasting} on:detection={handleCameraDetection} />
        </div>
      {:else if selectedCameraId !== 'all'}
        <div class="max-w-4xl mx-auto w-full">
          <CameraStream deviceId={selectedCameraId} {token} broadcasting={isBroadcasting} on:detection={handleCameraDetection} />
        </div>
      {:else if filteredDevices.length === 0 && (mode === 'viewer' || viewLayout !== 'theater')}
        <div class="text-center py-24 bg-gray-900/60 rounded-2xl border border-gray-800/80 shadow-xl max-w-2xl mx-auto">
          <div class="w-16 h-16 bg-gray-800 rounded-2xl flex items-center justify-center mx-auto mb-4 border border-gray-700">
            <svg class="w-8 h-8 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          </div>
          <h3 class="text-xl font-bold text-white tracking-tight">{searchQuery ? 'No matching cameras' : 'No cameras found'}</h3>
          <p class="mt-2 text-sm text-gray-400 max-w-sm mx-auto">
            {searchQuery ? `No devices matching "${searchQuery}".` : 'Your account does not have access to any devices.'}
          </p>
          <button on:click={() => { searchQuery = ''; fetchDevices(); }} class="mt-6 px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-semibold rounded-lg shadow-md transition cursor-pointer">
            {searchQuery ? 'Clear Filter' : 'Refresh Devices'}
          </button>
        </div>
      {:else if viewLayout === 'theater'}
        <!-- Theater Mode Layout -->
        <div class="space-y-6" data-testid="theater-mode-container">
          <!-- Dominant 16:9 Main Player -->
          {#if activeTheaterCameraId}
            <div class="max-w-5xl mx-auto w-full">
              <div class="mb-2 flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="px-2 py-0.5 bg-indigo-950/80 text-indigo-400 border border-indigo-800/60 rounded text-xs font-mono font-semibold">
                    THEATER FOCUS
                  </span>
                  <span class="text-sm font-bold text-white">
                    {activeTheaterCameraId === 'local-webcam' ? 'Local Webcam' : activeTheaterCameraId}
                  </span>
                </div>
              </div>
              <CameraStream deviceId={activeTheaterCameraId} {token} broadcasting={isBroadcasting} on:detection={handleCameraDetection} />
            </div>
          {:else}
            <div class="text-center py-16 bg-gray-900/60 rounded-2xl border border-gray-800/80 max-w-2xl mx-auto">
              <p class="text-gray-400">Select a camera below to view in Theater Mode</p>
            </div>
          {/if}

          <!-- Responsive Thumbnail Camera Switcher Strip -->
          <div class="border-t border-gray-800/80 pt-4 max-w-5xl mx-auto">
            <div class="flex items-center justify-between mb-3">
              <h4 class="text-xs font-bold text-gray-400 uppercase tracking-wider">Camera Switcher</h4>
              <span class="text-xs text-gray-500">Click to switch main display</span>
            </div>
            <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3">
              {#if mode === 'home'}
                <button
                  type="button"
                  on:click={() => activeTheaterCameraId = 'local-webcam'}
                  class="text-left p-2 rounded-xl border transition-all relative overflow-hidden group cursor-pointer {
                    activeTheaterCameraId === 'local-webcam'
                      ? 'bg-indigo-950/60 border-indigo-500 shadow-md shadow-indigo-500/20 ring-1 ring-indigo-500'
                      : 'bg-gray-900/80 border-gray-800 hover:border-gray-700 hover:bg-gray-800/60'
                  }"
                  data-testid="thumbnail-local-webcam"
                >
                  <div class="aspect-video bg-gray-950 rounded-lg mb-2 flex items-center justify-center border border-gray-800 group-hover:border-gray-700">
                    <svg class="w-5 h-5 {activeTheaterCameraId === 'local-webcam' ? 'text-indigo-400' : 'text-gray-500'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                    </svg>
                  </div>
                  <div class="text-xs font-bold truncate text-white">Local Webcam</div>
                  <div class="text-[10px] text-gray-400 font-mono truncate">MediaStream</div>
                </button>
              {/if}

              {#each filteredDevices as device (device.device_id)}
                <button
                  type="button"
                  on:click={() => activeTheaterCameraId = device.device_id}
                  class="text-left p-2 rounded-xl border transition-all relative overflow-hidden group cursor-pointer {
                    activeTheaterCameraId === device.device_id
                      ? 'bg-indigo-950/60 border-indigo-500 shadow-md shadow-indigo-500/20 ring-1 ring-indigo-500'
                      : 'bg-gray-900/80 border-gray-800 hover:border-gray-700 hover:bg-gray-800/60'
                  }"
                  data-testid={`thumbnail-${device.device_id}`}
                >
                  <div class="aspect-video bg-gray-950 rounded-lg mb-2 flex items-center justify-center border border-gray-800 group-hover:border-gray-700">
                    <svg class="w-5 h-5 {activeTheaterCameraId === device.device_id ? 'text-indigo-400' : 'text-gray-500'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                    </svg>
                  </div>
                  <div class="text-xs font-bold truncate text-white">{device.device_id}</div>
                  <div class="text-[10px] text-gray-400 font-mono truncate">Edge WebRTC</div>
                </button>
              {/each}
            </div>
          </div>
        </div>
      {:else}
        <!-- Multi-Camera Grid Container -->
        <div class="grid gap-6 transition-all duration-300 {
          gridColumns === 1 ? 'grid-cols-1 max-w-4xl mx-auto' :
          gridColumns === 2 ? 'grid-cols-1 md:grid-cols-2' :
          gridColumns === 4 ? 'grid-cols-1 md:grid-cols-2 lg:grid-cols-4' :
          'grid-cols-1 md:grid-cols-2 lg:grid-cols-3'
        }">
          {#each filteredDevices as device (device.device_id)}
            <CameraStream deviceId={device.device_id} {token} broadcasting={isBroadcasting} on:detection={handleCameraDetection} />
          {/each}
        </div>
      {/if}
    {/if}
  </main>

  <!-- Footer -->
  <footer class="mt-auto border-t border-gray-900 bg-gray-950/80 px-6 py-4 text-xs text-gray-500 flex flex-col sm:flex-row justify-between items-center gap-2">
    <div>
      © 2026 <span class="font-semibold text-gray-400">Pulse Cloud Architecture</span>. Multi-tenant WebRTC Engine.
    </div>
    <div class="flex items-center gap-4">
      <span class="flex items-center gap-1.5">
        <span class="w-1.5 h-1.5 rounded-full bg-green-500"></span>
        <span>Low-Latency RTP</span>
      </span>
      <span>•</span>
      <span>End-to-End Encrypted</span>
    </div>
  </footer>
</div>
