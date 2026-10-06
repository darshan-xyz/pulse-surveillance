<script lang="ts">
  import CameraStream from './CameraStream.svelte';

  export let deviceId: string;
  export let token: string;
</script>

<CameraStream {deviceId} {token} />
