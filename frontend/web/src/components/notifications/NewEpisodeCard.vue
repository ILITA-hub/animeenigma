<template>
  <div class="group relative flex items-start gap-3 p-3 hover:bg-white/5 transition-colors">
    <!-- Whole-row click target (excluding the dismiss button) -->
    <button
      type="button"
      class="flex items-start gap-3 flex-1 min-w-0 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded"
      @click="onClick"
    >
      <PosterImage
        v-if="payload.anime_poster_url"
        :src="payload.anime_poster_url"
        :alt="payload.anime_title"
        ratio="2/3"
        rounded="sm"
        :proxy-width="128"
        class="w-[52px] flex-shrink-0"
      />
      <div
        v-else
        class="w-[52px] h-[72px] rounded flex-shrink-0 bg-white/5 flex items-center justify-center text-white/30 text-[10px] uppercase"
      >
        {{ payload.anime_title?.slice(0, 2) || '?' }}
      </div>

      <div class="flex-1 min-w-0">
        <p class="text-white text-sm font-medium truncate">{{ payload.anime_title }}</p>
        <p class="text-cyan-400 text-xs mt-0.5">{{ rangeText }}</p>
        <p v-if="sourceText" class="text-white/50 text-xs mt-0.5 truncate">{{ sourceText }}</p>
        <p class="text-white/40 text-[10px] mt-1">{{ relativeTime }}</p>
      </div>
    </button>

    <NotificationRowActions :notification="notification" />
  </div>
</template>

<script setup lang="ts">
/**
 * Renderer for type === 'new_episode'. Layout: 52×72 poster, title,
 * "Episode N is out" or "Episodes N–M are out", source line, relative
 * timestamp, trailing action (dismiss × in the dropdown, delete bin in the
 * history modal). Click anywhere except that action routes to the /click
 * handler + navigates to the watch URL.
 *
 * Phase 3 — workstream: notifications.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import PosterImage from '@/components/anime/PosterImage.vue'
import NotificationRowActions from '@/components/notifications/NotificationRowActions.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { formatRelativeTime, type SupportedLocale } from '@/lib/relativeTime'
import type { UserNotification, NewEpisodePayload } from '@/types/notification'

const props = defineProps<{
  notification: UserNotification
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t, locale } = useI18n()
const router = useRouter()
const store = useNotificationsStore()

const payload = computed<NewEpisodePayload>(() => {
  // Safe cast: dispatched only when notification.type === 'new_episode'.
  return (props.notification.payload as NewEpisodePayload) || {
    anime_id: '',
    anime_title: '',
    first_unwatched_episode: 0,
    latest_available_episode: 0,
    player: '',
    language: '',
    watch_type: '',
    translation_id: '',
    watch_url: '',
  }
})

const rangeText = computed(() => {
  const first = payload.value.first_unwatched_episode
  const latest = payload.value.latest_available_episode
  if (!first || !latest || first === latest) {
    return t('notifications.newEpisode.singleEp', { n: latest || first || '?' })
  }
  return t('notifications.newEpisode.rangeEp', { n: first, m: latest })
})

const sourceText = computed(() => {
  const p = payload.value
  const team = p.translation_title || p.translation_id
  if (!team) return ''
  // Append a short language/type marker e.g. "(RU dub)" when present.
  const lang = (p.language || '').toUpperCase()
  const type = p.watch_type || ''
  const tag = lang && type ? ` (${lang} ${type})` : (lang ? ` (${lang})` : '')
  return t('notifications.newEpisode.via', { translation: team }) + tag
})

const relativeTime = computed(() => {
  return formatRelativeTime(
    props.notification.created_at,
    locale.value as SupportedLocale,
    t('notifications.time.justNow'),
  )
})

function onClick(): void {
  store.handleClick(props.notification, router)
  emit('close')
}
</script>
