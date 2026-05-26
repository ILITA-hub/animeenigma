<!--
  Workstream watch-together — Phase 03 (player-sync) Plan 03.5.

  SyncToastStack — narrates remote playback events with fadeable toasts at
  the bottom of the player. Subscribes to `room.onPlaybackEvent` (which the
  composable has already pre-filtered for own-user echoes — see Phase 2
  echo-guard contract in useWatchTogetherRoom.ts §"Re-emission guard").
  Each toast: 1.5s display + 0.5s fade, max 3 stacked vertically.

  Mounted inside the WatchTogetherView player column (relative container);
  the absolute positioning anchors to that wrapper. Pointer-events-none so
  it doesn't intercept clicks on player controls (matches the existing
  ReactionBurstOverlay pattern from Plan 02.5).

  Design notes:
    - Username lookup goes through `room.members.value` by `by_user_id`;
      members may have left mid-event so we fall back to "someone" if the
      lookup misses. Translators don't get a generic "someone" key — it's
      a deliberate verbatim fallback for the edge case where the remote
      member fired the event before the member-left frame arrived.
    - Time formatted as mm:ss in the component (not the locale) so the
      locale strings only carry the `{time}` slot. No hours — anime
      episodes are typically < 60 min.
    - The transition-group provides the CSS fade-out. Lifetime is 2000ms
      total; the leave transition (opacity → 0 over 500ms) starts when
      setTimeout removes the toast from the array, so the visual budget
      is ~1500ms display + ~500ms fade.
    - max 3 stacked: when a 4th event arrives we drop the oldest via
      `.slice(-MAX_STACK)`. This is rare in practice — events arrive at
      human cadence (clicks/seeks ~1s apart) but is needed for the
      drift-correction storm case where a sync-recovery triggers several
      seek events in rapid succession.
-->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

interface Toast {
  id: number
  kind: 'play' | 'pause' | 'seek'
  username: string
  time: number
  createdAt: number
}

const props = defineProps<{
  room: WatchTogetherRoomHandle
}>()

const { t } = useI18n()

const toasts = ref<Toast[]>([])
let nextId = 1
let unsub: (() => void) | null = null

const MAX_STACK = 3
// 1.5s display + 0.5s fade — the CSS transition duration is 500ms, so the
// leave transition runs out as the element is removed at the 2000ms mark.
const TOAST_LIFETIME_MS = 2000

function lookupUsername(userId: string): string {
  const member = props.room.members.value.find((m) => m.user_id === userId)
  return member?.meta.username ?? 'someone'
}

function formatTime(sec: number): string {
  const s = Math.max(0, Math.floor(sec))
  const mm = Math.floor(s / 60).toString().padStart(2, '0')
  const ss = (s % 60).toString().padStart(2, '0')
  return `${mm}:${ss}`
}

function labelFor(toast: Toast): string {
  if (toast.kind === 'play') {
    return t('watch_together.sync_toast_played', { username: toast.username })
  }
  if (toast.kind === 'pause') {
    return t('watch_together.sync_toast_paused', { username: toast.username })
  }
  return t('watch_together.sync_toast_seeked', {
    username: toast.username,
    time: formatTime(toast.time),
  })
}

function enqueue(kind: 'play' | 'pause' | 'seek', userId: string, time: number) {
  const toast: Toast = {
    id: nextId++,
    kind,
    username: lookupUsername(userId),
    time,
    createdAt: Date.now(),
  }
  // Append + cap at MAX_STACK (oldest dropped).
  toasts.value = [...toasts.value, toast].slice(-MAX_STACK)
  window.setTimeout(() => {
    toasts.value = toasts.value.filter((existing) => existing.id !== toast.id)
  }, TOAST_LIFETIME_MS)
}

onMounted(() => {
  unsub = props.room.onPlaybackEvent((e) => {
    enqueue(e.kind, e.by_user_id, e.time)
  })
})

onBeforeUnmount(() => {
  unsub?.()
  unsub = null
})

const visibleToasts = computed(() => toasts.value)
</script>

<template>
  <div
    class="absolute bottom-16 left-1/2 -translate-x-1/2 flex flex-col items-center gap-2 pointer-events-none z-10"
    aria-live="polite"
    role="status"
  >
    <transition-group name="sync-toast" tag="div" class="flex flex-col items-center gap-2">
      <div
        v-for="toast in visibleToasts"
        :key="toast.id"
        class="px-3 py-1.5 rounded-md bg-black/70 text-white text-sm font-medium backdrop-blur-sm"
      >
        {{ labelFor(toast) }}
      </div>
    </transition-group>
  </div>
</template>

<style scoped>
.sync-toast-enter-active,
.sync-toast-leave-active {
  transition: opacity 0.5s ease, transform 0.5s ease;
}
.sync-toast-enter-from,
.sync-toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
