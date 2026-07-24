<template>
  <button
    v-if="!notification.dismissed_at"
    type="button"
    class="text-white/40 hover:text-white text-lg leading-none p-1 -mr-1 flex-shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded"
    :aria-label="$t('notifications.toast.dismissAria')"
    @click.stop="onDismiss"
  >
    ×
  </button>
</template>

<script setup lang="ts">
/**
 * Shared dismiss × for notification card renderers. Owns the
 * already-dismissed guard (history rows render without a live ×) and the
 * optimistic store.dismiss call, so every card — including future types —
 * gets identical dismiss behavior for free.
 *
 * Intentionally does NOT close the parent dropdown/modal: the user may
 * want to dismiss several notifications in one session.
 */
import { useNotificationsStore } from '@/stores/notifications'
import type { UserNotification } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

const store = useNotificationsStore()

async function onDismiss(): Promise<void> {
  try {
    await store.dismiss(props.notification.id)
  } catch {
    // Optimistic rollback already happened inside the store action; the
    // user sees the row reappear. No further UI feedback needed.
  }
}
</script>
