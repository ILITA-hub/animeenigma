<!--
  Workstream watch-together — Phase 03 (player-sync) Plan 03.5.

  ConnectionStatusOverlay — renders a non-blocking banner over the player
  during transient disconnect states (reconnecting, closed). The
  useWatchTogetherRoom composable auto-reconnects with exponential
  backoff `[1s, 2s, 4s, 8s, 16s, 30s]`; this overlay just reassures the
  user that we know about the disconnect. Player continues playing
  locally; drift correction (Phase 3 §"Drift correction UX") pulls state
  back into sync on reconnect.

  Why only `reconnecting` + `closed` (and not `failed`)?
    - `idle` / `connecting`: pre-connect — WatchTogetherView's `loading`
      branch owns that UX (Plan 02.8).
    - `open`: happy path; nothing to show.
    - `reconnecting`: composable is actively retrying — show banner.
    - `closed`: defensive — composable transitions here on graceful close
      (room:closed frame), but the room-ended branch in
      WatchTogetherView is the canonical empty state. We render a banner
      here as a fallback so users see SOMETHING even if the parent view
      doesn't react fast enough.
    - `failed`: terminal — auth expired, capacity full, or room gone.
      WatchTogetherView routes to /auth or shows the dedicated empty
      state; THIS overlay deliberately stays silent so it doesn't double
      up with the parent UX.

  Layout: absolute top-2, centered horizontally. `pointer-events-none`
  on the container ensures clicks pass through to the player controls;
  the banner box itself is `pointer-events-auto` so screen readers + the
  AriaLive announcement work without interception.
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { ConnectionStatus } from '@/composables/useWatchTogetherRoom'

const props = defineProps<{
  status: ConnectionStatus
}>()

const { t } = useI18n()

const visible = computed(
  () => props.status === 'reconnecting' || props.status === 'closed',
)

const label = computed(() => {
  if (props.status === 'reconnecting') {
    return t('watch_together.reconnecting_indicator')
  }
  if (props.status === 'closed') {
    return t('watch_together.connection_status_closed')
  }
  return ''
})
</script>

<template>
  <div
    v-if="visible"
    class="absolute top-2 left-1/2 -translate-x-1/2 z-10 pointer-events-none"
    role="status"
    aria-live="polite"
  >
    <div
      class="flex items-center gap-2 px-3 py-1.5 rounded-md bg-black/70 text-white text-sm font-medium backdrop-blur-sm pointer-events-auto"
    >
      <span
        class="inline-block w-3 h-3 rounded-full border-2 border-white border-t-transparent animate-spin"
        aria-hidden="true"
      ></span>
      <span>{{ label }}</span>
    </div>
  </div>
</template>
