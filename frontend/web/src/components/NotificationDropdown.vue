<template>
  <div
    role="dialog"
    aria-modal="false"
    :aria-label="$t('notifications.dropdown.title')"
    class="bg-gray-950/95 backdrop-blur-xl border border-white/10 shadow-2xl rounded-xl w-[380px] max-w-[calc(100vw-1.5rem)] overflow-hidden flex flex-col"
  >
    <!-- Body: list region (scrolls) -->
    <div class="max-h-[420px] overflow-y-auto" role="region">
      <!-- Empty state -->
      <div
        v-if="store.notifications.length === 0"
        class="flex flex-col items-center justify-center text-center py-10 px-6 text-white/40"
      >
        <svg class="w-10 h-10 mb-3 text-white/20" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 17h5l-1.4-1.4A2 2 0 0118 14.2V11a6 6 0 10-12 0v3.2a2 2 0 01-.6 1.4L4 17h5m6 0a3 3 0 11-6 0" />
        </svg>
        <p class="text-sm">{{ $t('notifications.dropdown.empty') }}</p>
      </div>

      <!-- Notification list -->
      <ul v-else class="divide-y divide-white/5">
        <li v-for="n in store.notifications" :key="n.id">
          <component
            :is="resolveRenderer(n.type)"
            :notification="n"
            @close="$emit('close')"
          />
        </li>
      </ul>
    </div>

    <!-- Footer: mark-all-read (hidden when nothing unread) -->
    <div
      v-if="store.unreadCount > 0"
      class="border-t border-white/10 bg-white/[0.02] px-3 py-2 flex items-center justify-end"
    >
      <button
        type="button"
        class="text-xs text-cyan-400 hover:text-cyan-300 transition-colors px-2 py-1 rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400"
        @click="onMarkAllRead"
      >
        {{ $t('notifications.dropdown.markAllRead') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Notification dropdown body — opened from NotificationBell. Renders an
 * empty state when nothing is queued, else a virtualized-vertical list
 * of renderer components keyed by notification.type. Sticky footer
 * holds the mark-all-read action.
 *
 * Outside-click + Esc close are handled by the parent NotificationBell
 * wrapper (so closing also flips the wrapper's `open` ref to drive the
 * Transition cleanly).
 *
 * Phase 3 — workstream: notifications.
 */
import { useNotificationsStore } from '@/stores/notifications'
import { resolveRenderer } from '@/lib/notification-renderers'

const store = useNotificationsStore()

const emit = defineEmits<{
  (e: 'close'): void
}>()

async function onMarkAllRead(): Promise<void> {
  try {
    await store.markAllRead()
  } catch {
    /* optimistic rollback handled in store */
  }
  emit('close')
}
</script>
