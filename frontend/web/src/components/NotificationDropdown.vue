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
      <NotificationList
        :notifications="store.notifications"
        @close="$emit('close')"
      />
    </div>

    <!-- Footer: view-older (always) + mark-all-read (hidden when nothing unread) -->
    <div class="border-t border-white/10 bg-white/[0.02] px-3 py-2 flex flex-col gap-2">
      <Button
        v-if="store.unreadCount > 0"
        variant="link"
        size="xs"
        class="self-end"
        @click="onMarkAllRead"
      >
        {{ $t('notifications.dropdown.markAllRead') }}
      </Button>
      <Button
        variant="soft"
        size="sm"
        full-width
        @click="onViewOlder"
      >
        <History aria-hidden="true" />
        {{ $t('notifications.history.viewOlder') }}
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
import { History } from 'lucide-vue-next'
import NotificationList from '@/components/NotificationList.vue'
import { useNotificationsStore } from '@/stores/notifications'
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

/** Close whichever surface hosts the dropdown, then open the history modal
 *  (hosted in App.vue so it outlives both the bell dropdown and the
 *  mobile-drawer Modal). */
function onViewOlder(): void {
  emit('close')
  store.openHistory()
}
</script>
