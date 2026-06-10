<template>
  <div
    class="group relative flex items-start gap-3 p-3 hover:bg-white/5 transition-colors"
    :class="{ 'opacity-70': isRead }"
  >
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
        <svg class="w-6 h-6" fill="none" stroke="currentColor" stroke-width="2"
          stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24" aria-hidden="true">
          <path v-for="(d, i) in stage.iconPaths" :key="i" :d="d" />
        </svg>
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

    <!-- Dismiss × -->
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
 * Renderer for the three feedback triage stages (AUTO-417):
 * `feedback_created` / `feedback_in_progress` / `feedback_ai_done`.
 * One component for all three — the stage drives icon, accent color and
 * i18n strings. Click marks read (no navigation: there is no user-facing
 * report page). Dismiss × mirrors NewEpisodeCard.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

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

const isRead = computed(() => Boolean(props.notification.read_at))

interface StageMeta {
  /** Inline lucide path data (bot / check-check / message-square-plus). */
  iconPaths: string[]
  iconClass: string
  key: 'created' | 'inProgress' | 'aiDone'
}

// lucide "bot"
const BOT_PATHS = [
  'M12 8V4H8',
  'M2 14h2',
  'M20 14h2',
  'M15 13v2',
  'M9 13v2',
  'M5 8 h14 a2 2 0 0 1 2 2 v8 a2 2 0 0 1 -2 2 H5 a2 2 0 0 1 -2 -2 v-8 a2 2 0 0 1 2 -2 z',
]
// lucide "check-check"
const CHECK_CHECK_PATHS = ['M18 6 7 17l-5-5', 'm22 10-7.5 7.5L13 16']
// lucide "message-square-plus"
const MESSAGE_PLUS_PATHS = [
  'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z',
  'M12 7v6',
  'M9 10h6',
]

const stage = computed<StageMeta>(() => {
  switch (props.notification.type) {
    case 'feedback_in_progress':
      return { iconPaths: BOT_PATHS, iconClass: 'text-info', key: 'inProgress' }
    case 'feedback_ai_done':
      return { iconPaths: CHECK_CHECK_PATHS, iconClass: 'text-success', key: 'aiDone' }
    default:
      return { iconPaths: MESSAGE_PLUS_PATHS, iconClass: 'text-cyan-400', key: 'created' }
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

async function onDismiss(): Promise<void> {
  try {
    await store.dismiss(props.notification.id)
  } catch {
    // Optimistic rollback already happened inside the store action.
  }
}
</script>
