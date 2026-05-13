<template>
  <!-- Phase 11 / UX-24 — System-status banner. Renders nothing when there
       is no active incident, or when the currently visible incident has
       already been dismissed in localStorage (per-incident key:
       `sys_status_dismissed_{id}`). Mounted permanently at the top of
       Home; safe because the composable returns an empty array when the
       gateway env is off. -->
  <div
    v-if="visibleIncident"
    role="alert"
    class="bg-red-500/90 text-white text-sm px-4 py-2 flex items-center justify-between gap-3"
  >
    <span class="flex-1 text-center">{{ visibleIncident.title }}</span>
    <button
      type="button"
      class="text-white/80 hover:text-white p-1 rounded transition-colors"
      :aria-label="$t('system.statusBanner.dismiss')"
      @click="dismiss"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useSystemStatus } from '@/composables/useSystemStatus'

// Component owns its own composable subscription — no props, no emits.
// Poll every 60s so a banner flip on the backend (operator toggles
// SYSTEM_BANNER_ACTIVE) propagates within a minute without a page reload.
const { incidents } = useSystemStatus(60000)

// dismissedTick increments on each dismiss click so the `visibleIncident`
// computed re-evaluates without us needing to mutate the incidents array.
const dismissedTick = ref(0)

function isDismissed(id: string): boolean {
  if (typeof localStorage === 'undefined') return false
  return localStorage.getItem(`sys_status_dismissed_${id}`) === '1'
}

const visibleIncident = computed(() => {
  // Touch dismissedTick so the computed re-runs after dismiss().
  void dismissedTick.value
  return incidents.value.find((i) => !isDismissed(i.id)) ?? null
})

function dismiss() {
  const inc = visibleIncident.value
  if (!inc || typeof localStorage === 'undefined') return
  localStorage.setItem(`sys_status_dismissed_${inc.id}`, '1')
  dismissedTick.value++
}
</script>
