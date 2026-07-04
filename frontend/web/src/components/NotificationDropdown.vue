<template>
  <div
    :role="flat ? undefined : 'dialog'"
    :aria-modal="flat ? undefined : 'false'"
    :aria-label="flat ? undefined : $t('notifications.dropdown.title')"
    class="overflow-hidden flex flex-col"
    :class="flat
      ? 'w-full'
      : 'bg-background/95 backdrop-blur-xl border border-white/10 shadow-2xl rounded-xl w-[380px] max-w-[calc(100vw-1.5rem)]'"
  >
    <!-- Body: list region (scrolls). Flat mode caps against the viewport
         so the list never nests a second scroller inside the Modal's own
         max-h-[90vh] scroll on short (landscape-phone) viewports. -->
    <div
      class="overflow-y-auto"
      :class="flat ? 'max-h-[min(420px,60svh)]' : 'max-h-[420px]'"
      role="region"
    >
      <!-- Empty state -->
      <EmptyState v-if="store.notifications.length === 0" size="sm" class="text-sm">
        <template #icon><Bell class="size-10" /></template>
        {{ $t('notifications.dropdown.empty') }}
      </EmptyState>

      <!-- Notification list. Read rows stay visible but tinted — the
           single point of control for the read/unread visual split, so
           the card renderers stay presentation-agnostic. -->
      <ul v-else class="divide-y divide-white/5">
        <li
          v-for="n in store.notifications"
          :key="n.id"
          :class="{ 'opacity-70': n.read_at }"
        >
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
      <Button
        variant="link"
        size="xs"
        @click="onMarkAllRead"
      >
        {{ $t('notifications.dropdown.markAllRead') }}
      </Button>
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
 * `flat` embeds the list in a host that already provides the panel
 * chrome and dialog semantics (the mobile-drawer Modal) — it drops the
 * standalone width/background/border and contributes no landmark of its
 * own (no role/aria-label; the host's labeled dialog covers it).
 *
 * Phase 3 — workstream: notifications.
 */
import { Bell } from 'lucide-vue-next'
import EmptyState from '@/components/ui/EmptyState.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { resolveRenderer } from '@/lib/notification-renderers'
import Button from '@/components/ui/Button.vue'

defineProps<{ flat?: boolean }>()

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
