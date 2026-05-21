<template>
  <Transition name="notif-toast">
    <div
      v-if="currentNotification"
      role="status"
      aria-live="polite"
      :class="[
        'fixed z-50 pointer-events-auto',
        isDesktop
          ? 'bottom-6 right-6 w-[360px]'
          : 'top-16 left-3 right-3',
      ]"
      @mouseenter="onHoverStart"
      @mouseleave="onHoverEnd"
    >
      <div
        class="bg-gray-950/95 backdrop-blur-xl border border-cyan-400/30 shadow-2xl rounded-xl overflow-hidden"
      >
        <component
          :is="resolveRenderer(currentNotification.type)"
          :notification="currentNotification"
          @close="onDismiss"
        />
        <!-- Click target overlay so the whole toast routes (the card's
             internal click target only fires on the poster+text area —
             the toast wraps it with a transparent action). Pointer
             events still pass through to the dismiss × since that
             button sits visually on top in the renderer. -->
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
/**
 * Slide-in toast for the latest undismissed notification. Mounts at
 * App-root level so it survives route transitions. Auto-hides at 8s;
 * pauses on hover. Suppressed entirely when the user is already on the
 * matching anime route (route.params.id === payload.anime_id) — they
 * don't need a toast for content they're staring at.
 *
 * Unknown types are suppressed entirely (isKnownType()); the dropdown
 * still renders them via UnknownNotificationCard.
 *
 * Phase 3 — workstream: notifications.
 */
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useMediaQuery } from '@vueuse/core'

import { useNotificationsStore } from '@/stores/notifications'
import { resolveRenderer, isKnownType } from '@/lib/notification-renderers'
import type { UserNotification, NewEpisodePayload } from '@/types/notification'

const AUTO_HIDE_MS = 8_000

const store = useNotificationsStore()
const route = useRoute()
const isDesktop = useMediaQuery('(min-width: 768px)')

const paused = ref(false)
let hideTimer: ReturnType<typeof setTimeout> | null = null

/**
 * Route-suppression: when the user is already viewing the matching anime
 * detail page, swallow the toast (the dropdown badge still shows up).
 * Route param is matched against `payload.anime_id` per NOTIF-UI-04.
 */
const suppressedByRoute = computed<boolean>(() => {
  const candidate = store.latestUndismissedToast
  if (!candidate || candidate.type !== 'new_episode') return false
  const payload = candidate.payload as NewEpisodePayload | null
  if (!payload?.anime_id) return false
  const routeAnimeId = route.params.id as string | undefined
  return routeAnimeId === payload.anime_id
})

/**
 * The notification that should currently be on-screen. Filters out:
 *   - unknown types (Plan §NOTIF-UI-06)
 *   - notifications matching the current route (Plan §NOTIF-UI-04)
 */
const currentNotification = computed<UserNotification | null>(() => {
  const n = store.latestUndismissedToast
  if (!n) return null
  if (!isKnownType(n.type)) return null
  if (suppressedByRoute.value) return null
  return n
})

function clearTimer(): void {
  if (hideTimer !== null) {
    clearTimeout(hideTimer)
    hideTimer = null
  }
}

function scheduleAutoHide(): void {
  clearTimer()
  if (paused.value) return
  hideTimer = setTimeout(() => {
    const n = currentNotification.value
    if (n) {
      // Session-only suppression — keeps the row in the dropdown but
      // stops the toast from re-appearing for the same notification
      // until next page-load (or store.stop()).
      store.markToastShown(n.id)
    }
  }, AUTO_HIDE_MS)
}

function onHoverStart(): void {
  paused.value = true
  clearTimer()
}

function onHoverEnd(): void {
  paused.value = false
  scheduleAutoHide()
}

function onDismiss(): void {
  const n = currentNotification.value
  if (n) store.markToastShown(n.id)
  clearTimer()
}

// React to the toast-target changing (new notification appears, current
// one expires, route-suppression flips, user dismisses). Each time the
// target identity changes, restart the auto-hide timer.
watch(
  () => currentNotification.value?.id ?? null,
  (id, prevId) => {
    if (id !== prevId) {
      if (id !== null) {
        scheduleAutoHide()
      } else {
        clearTimer()
      }
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.notif-toast-enter-active,
.notif-toast-leave-active {
  transition: opacity 200ms ease-out, transform 200ms ease-out;
}
.notif-toast-enter-from,
.notif-toast-leave-to {
  opacity: 0;
  transform: translateY(20px);
}
</style>
