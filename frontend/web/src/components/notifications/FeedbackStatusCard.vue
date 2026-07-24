<template>
  <div class="group relative flex items-start gap-3 p-3 hover:bg-white/5 transition-colors">
    <!-- Whole-row click target (excluding the dismiss button) -->
    <button
      type="button"
      class="flex items-start gap-3 flex-1 min-w-0 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded"
      @click="onClick"
    >
      <div
        class="w-[52px] h-[72px] rounded flex-shrink-0 bg-white/5 flex items-center justify-center"
        :class="stage.iconClass"
        aria-hidden="true"
      >
        <component :is="stage.icon" class="size-6" aria-hidden="true" />
      </div>

      <div class="flex-1 min-w-0">
        <p class="text-white text-sm font-medium">{{ titleText }}</p>
        <p class="text-xs mt-0.5" :class="stage.iconClass">{{ bodyText }}</p>
        <p v-if="payload.description" class="text-white/50 text-xs mt-0.5 truncate">
          «{{ payload.description }}»
        </p>
        <p class="text-white/40 text-[10px] mt-1">{{ relativeTime }}</p>
      </div>
    </button>

    <NotificationRowActions :notification="notification" />
  </div>
</template>

<script setup lang="ts">
/**
 * Renderer for the three feedback triage stages (AUTO-417):
 * `feedback_created` / `feedback_in_progress` / `feedback_ai_done`.
 * One component for all three — the stage drives icon, accent color and
 * i18n strings. Click marks read (no navigation: there is no user-facing
 * report page). Trailing action (dismiss × / delete bin) mirrors NewEpisodeCard.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Bot, CheckCheck, MessageSquarePlus, type LucideIcon } from 'lucide-vue-next'

import NotificationRowActions from '@/components/notifications/NotificationRowActions.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { formatRelativeTime, type SupportedLocale } from '@/lib/relativeTime'
import type { UserNotification, FeedbackStatusPayload } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t, locale } = useI18n()
const router = useRouter()
const store = useNotificationsStore()

const payload = computed<FeedbackStatusPayload>(() => {
  return (props.notification.payload as FeedbackStatusPayload) || {
    report_id: '',
    status: 'created',
  }
})

interface StageMeta {
  icon: LucideIcon
  iconClass: string
  key: 'created' | 'inProgress' | 'aiDone'
}

const stage = computed<StageMeta>(() => {
  switch (props.notification.type) {
    case 'feedback_in_progress':
      return { icon: Bot, iconClass: 'text-info', key: 'inProgress' }
    case 'feedback_ai_done':
      return { icon: CheckCheck, iconClass: 'text-success', key: 'aiDone' }
    default:
      return { icon: MessageSquarePlus, iconClass: 'text-cyan-400', key: 'created' }
  }
})

const titleText = computed(() => t(`notifications.feedback.${stage.value.key}.title`))
const bodyText = computed(() => t(`notifications.feedback.${stage.value.key}.body`))

const relativeTime = computed(() => {
  return formatRelativeTime(
    props.notification.created_at,
    locale.value as SupportedLocale,
    t('notifications.time.justNow'),
  )
})

function onClick(): void {
  // handleClick records telemetry + marks read; it only navigates for
  // new_episode payloads, so feedback cards just collapse the badge.
  store.handleClick(props.notification, router)
  emit('close')
}
</script>
