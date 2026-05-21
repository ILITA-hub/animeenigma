<template>
  <div class="flex items-start gap-3 p-3 hover:bg-white/5 transition-colors">
    <div
      class="w-[52px] h-[72px] rounded flex-shrink-0 bg-white/5 flex items-center justify-center text-white/30"
      aria-hidden="true"
    >
      <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    </div>
    <div class="flex-1 min-w-0">
      <p class="text-white/70 text-sm">{{ $t('notifications.unknown.title') }}</p>
      <p class="text-white/40 text-[10px] mt-1">{{ relativeTime }}</p>
    </div>
    <button
      type="button"
      class="text-white/40 hover:text-white text-lg leading-none p-1 -mr-1 flex-shrink-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded"
      :aria-label="$t('notifications.toast.dismissAria')"
      @click.stop="onDismiss"
    >
      ×
    </button>
  </div>
</template>

<script setup lang="ts">
/**
 * Graceful fallback renderer for unrecognized notification types.
 * Shipped alongside NewEpisodeCard so a v1.1 `new_comment` or
 * `system_announcement` row from a newer backend doesn't crash an older
 * frontend — the user sees "New notification — view in dropdown"
 * instead of a blank slot or a Vue render error.
 *
 * Toast layer suppresses unknown types entirely (per Plan §NOTIF-UI-06)
 * so this renderer only ever appears in the dropdown list.
 *
 * Phase 3 — workstream: notifications.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { useNotificationsStore } from '@/stores/notifications'
import { formatRelativeTime, type SupportedLocale } from '@/lib/relativeTime'
import type { UserNotification } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t, locale } = useI18n()
const store = useNotificationsStore()

const relativeTime = computed(() =>
  formatRelativeTime(
    props.notification.created_at,
    locale.value as SupportedLocale,
    t('notifications.time.justNow'),
  ),
)

async function onDismiss(): Promise<void> {
  try {
    await store.dismiss(props.notification.id)
  } catch {
    /* optimistic rollback handled inside store */
  }
}
</script>
