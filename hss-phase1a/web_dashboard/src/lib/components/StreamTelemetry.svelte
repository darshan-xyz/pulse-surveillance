<script lang="ts">
  export interface TelemetryData {
    fps: number;
    resolution: string;
    bitrateKbps: number;
    rttMs: number;
    iceState: string;
    candidateType: 'relay' | 'host' | 'srflx' | 'prflx' | 'local';
    candidateStatus: string; // 'TURN Relay (RFC 5766)' | 'P2P Host' | 'STUN Reflexive' | 'Local Device'
  }

  export let telemetry: TelemetryData = {
    fps: 0,
    resolution: '--',
    bitrateKbps: 0,
    rttMs: 0,
    iceState: 'new',
    candidateType: 'host',
    candidateStatus: 'Initializing'
  };

  export let isConnected: boolean = false;
  let isExpanded: boolean = true;

  function toggleExpand() {
    isExpanded = !isExpanded;
  }
</script>

<div class="absolute top-3 right-3 z-30 flex flex-col items-end pointer-events-auto" data-testid="stream-telemetry-hud">
  <!-- Minimalist HUD Toggle Button -->
  <button 
    type="button"
    on:click={toggleExpand}
    class="px-2 py-1 bg-gray-950/80 hover:bg-gray-900 border border-gray-700/70 backdrop-blur-md rounded-md text-[11px] font-mono text-gray-300 flex items-center gap-1.5 shadow-lg transition cursor-pointer"
    title="Toggle Telemetry HUD"
    data-testid="telemetry-toggle-btn"
  >
    <span class="w-1.5 h-1.5 rounded-full {isConnected ? 'bg-green-400 animate-pulse' : 'bg-yellow-500'}"></span>
    <span class="font-semibold tracking-wider">HUD</span>
    <svg class="w-3 h-3 transition-transform {isExpanded ? 'rotate-180' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
    </svg>
  </button>

  <!-- Expanded Metrics Panel -->
  {#if isExpanded}
    <div class="mt-1.5 bg-gray-950/90 backdrop-blur-md border border-gray-800 rounded-lg p-2 shadow-2xl text-[11px] font-mono text-gray-300 min-w-[190px] space-y-1.5 transition-all">
      <!-- Resolution & FPS Row -->
      <div class="flex justify-between items-center border-b border-gray-800/80 pb-1">
        <span class="text-gray-400">RES / FPS</span>
        <span class="font-bold text-white flex items-center gap-1.5">
          <span data-testid="telemetry-resolution">{telemetry.resolution}</span>
          <span class="text-gray-500">@</span>
          <span class="{telemetry.fps >= 25 ? 'text-green-400' : telemetry.fps >= 15 ? 'text-yellow-400' : 'text-red-400'}" data-testid="telemetry-fps">
            {telemetry.fps} fps
          </span>
        </span>
      </div>

      <!-- Bitrate Row -->
      <div class="flex justify-between items-center border-b border-gray-800/80 pb-1">
        <span class="text-gray-400">BITRATE</span>
        <span class="font-bold text-cyan-400" data-testid="telemetry-bitrate">
          {telemetry.bitrateKbps >= 1000 ? `${(telemetry.bitrateKbps / 1000).toFixed(2)} Mbps` : `${telemetry.bitrateKbps} kbps`}
        </span>
      </div>

      <!-- Latency RTT Row -->
      <div class="flex justify-between items-center border-b border-gray-800/80 pb-1">
        <span class="text-gray-400">LATENCY (RTT)</span>
        <span class="font-bold {telemetry.rttMs <= 60 ? 'text-green-400' : telemetry.rttMs <= 150 ? 'text-yellow-400' : 'text-red-400'}" data-testid="telemetry-rtt">
          {telemetry.rttMs} ms
        </span>
      </div>

      <!-- ICE Candidate & Transport Status -->
      <div class="flex justify-between items-center pt-0.5">
        <span class="text-gray-400">ICE STATUS</span>
        <span 
          class="px-1.5 py-0.5 rounded text-[10px] font-bold tracking-tight {
            telemetry.candidateType === 'relay' || (telemetry.candidateStatus && telemetry.candidateStatus.includes('Relay'))
              ? 'bg-amber-950/80 text-amber-300 border border-amber-700/60' 
              : telemetry.candidateType === 'host' || (telemetry.candidateStatus && telemetry.candidateStatus.includes('Host'))
              ? 'bg-emerald-950/80 text-emerald-300 border border-emerald-700/60'
              : 'bg-indigo-950/80 text-indigo-300 border border-indigo-700/60'
          }"
          data-testid="telemetry-ice-status"
        >
          {telemetry.candidateStatus}
        </span>
      </div>
    </div>
  {/if}
</div>
