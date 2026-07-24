<template>
  <div class="flex items-start gap-3 p-3 hover:bg-white/5 transition-colors">
    <div
      class="w-[52px] h-[72px] rounded flex-shrink-0 bg-white/5 flex items-center justify-center text-white/30"
      aria-hidden="true"
    >
      <Info class="size-6" aria-hidden="true" />
    </div>
    <div class="flex-1 min-w-0">
      <p class="text-white/70 text-sm">{{ $t('notifications.unknown.title') }}</p>
      <p class="text-white/40 text-[10px] mt-1">{{ relativeTime }}</p>
    </div>
    <NotificationRowActions :notification="notification" />
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
import { Info } from 'lucide-vue-next'

import NotificationRowActions from '@/components/notifications/NotificationRowActions.vue'
import { formatRelativeTime, type SupportedLocale } from '@/lib/relativeTime'
import type { UserNotification } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t, locale } = useI18n()

const relativeTime = computed(() =>
  formatRelativeTime(
    props.notification.created_at,
    locale.value as SupportedLocale,
    t('notifications.time.justNow'),
  ),
)
</script>
