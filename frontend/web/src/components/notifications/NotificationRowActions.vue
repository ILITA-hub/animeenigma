<template>
  <button
    v-if="showButton"
    type="button"
    class="text-white/40 hover:text-white p-1 -mr-1 flex-shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded"
    :aria-label="$t(ariaKey)"
    :title="isHistory ? $t(ariaKey) : undefined"
    @click.stop="run"
  >
    <Trash2 v-if="isHistory" class="size-4" aria-hidden="true" />
    <span v-else class="text-lg leading-none" aria-hidden="true">×</span>
  </button>
</template>

<script setup lang="ts">
/**
 * Trailing action for a notification card, chosen by the ambient surface:
 *   - bell dropdown → dismiss × ("clear from the bell, keep in history")
 *   - history modal → delete bin ("remove from history too")
 *
 * The surface is injected (provided by NotificationHistoryModal) rather than
 * passed as a prop, so it need not thread through NotificationList's
 * `<component :is>` renderer dispatch. Every card drops one of these in its
 * action slot and gets the right control for free.
 *
 * Both actions are optimistic with store-side rollback; this component
 * intentionally does NOT close the parent dropdown/modal (the user may act on
 * several notifications in one session). An already-actioned row (dismissed in
 * the dropdown, deleted in history) renders no button.
 */
import { computed, inject } from 'vue'
import { Trash2 } from 'lucide-vue-next'

import { useNotificationsStore } from '@/stores/notifications'
import { notificationSurfaceKey } from '@/components/notifications/surface'
import type { UserNotification } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

const store = useNotificationsStore()
// The surface is fixed for a mounted card, so a plain const (not a computed)
// is enough.
const isHistory = inject(notificationSurfaceKey, 'dropdown') === 'history'

const ariaKey = computed(() =>
  isHistory ? 'notifications.history.deleteAria' : 'notifications.toast.dismissAria',
)
const showButton = computed(() =>
  isHistory ? !props.notification.deleted_at : !props.notification.dismissed_at,
)

async function run(): Promise<void> {
  try {
    if (isHistory) await store.delete(props.notification.id)
    else await store.dismiss(props.notification.id)
  } catch {
    // Optimistic rollback already happened inside the store action; the row
    // reappears. No further UI feedback needed.
  }
}
</script>
