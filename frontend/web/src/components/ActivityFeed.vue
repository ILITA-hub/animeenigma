<template>
  <div class="glass-card rounded-2xl p-5">
    <div class="flex items-center gap-3 mb-5">
      <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-purple-500 to-pink-500 flex items-center justify-center">
        <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white">{{ $t('activity.title') }}</h2>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && events.length === 0" class="space-y-3">
      <div v-for="i in 4" :key="i" class="animate-pulse flex gap-3 p-2">
        <div class="w-12 h-16 bg-white/10 rounded-lg flex-shrink-0"></div>
        <div class="flex-1 space-y-2">
          <div class="h-3 bg-white/10 rounded w-1/4"></div>
          <div class="h-4 bg-white/10 rounded w-3/4"></div>
          <div class="h-3 bg-white/10 rounded w-1/3"></div>
        </div>
      </div>
    </div>

    <!-- Events list -->
    <div v-else class="space-y-2">
      <div
        v-for="event in events"
        :key="event.id"
        class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors"
      >
        <!-- Anime poster -->
        <router-link
          :to="`/anime/${event.anime_id}`"
          class="flex-shrink-0"
        >
          <img
            :src="event.anime?.poster_url || '/placeholder.svg'"
            :alt="getLocalizedTitle(event.anime?.name, event.anime?.name_ru) || ''"
            class="w-12 h-16 object-cover rounded-lg"
          />
        </router-link>

        <!-- Event info -->
        <div class="flex-1 min-w-0">
          <p class="text-xs text-gray-400">
            {{ event.username }}
          </p>
          <p class="text-sm text-white mt-0.5">
            <span>{{ actionText(event) }}</span>
            <router-link
              :to="`/anime/${event.anime_id}`"
              class="text-purple-400 hover:text-purple-300 transition-colors"
            >
              {{ animeName(event) }}
            </router-link>
          </p>
          <p class="text-xs text-gray-500 mt-1">
            {{ formatRelativeTime(event.created_at) }}
          </p>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="events.length === 0 && !loading" class="text-center py-8 text-gray-400">
        {{ $t('activity.empty') }}
      </div>

      <!-- Load more button -->
      <button
        v-if="hasMore"
        @click="loadMore"
        :disabled="loading"
        class="w-full mt-3 py-2.5 text-sm text-gray-400 hover:text-white bg-white/5 hover:bg-white/10 rounded-xl transition-colors disabled:opacity-50"
      >
        {{ loading ? $t('common.loading') : $t('activity.loadMore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { activityApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'

interface ActivityEvent {
  id: string
  user_id: string
  username: string
  anime_id: string
  anime?: {
    id: string
    name: string
    name_ru?: string
    poster_url?: string
  }
  type: string
  old_value: string
  new_value: string
  created_at: string
}

const { t, locale } = useI18n()
const events = ref<ActivityEvent[]>([])
const hasMore = ref(false)
const loading = ref(true)

const loadFeed = async (before?: string) => {
  loading.value = true
  try {
    const response = await activityApi.getFeed(10, before)
    const data = response.data?.data || response.data
    const newEvents: ActivityEvent[] = data?.events || []
    if (before) {
      events.value.push(...newEvents)
    } else {
      events.value = newEvents
    }
    hasMore.value = data?.has_more || false
  } catch (err) {
    console.error('Failed to load activity feed:', err)
  } finally {
    loading.value = false
  }
}

const loadMore = () => {
  if (events.value.length > 0) {
    const lastEvent = events.value[events.value.length - 1]
    loadFeed(lastEvent.id)
  }
}

const actionText = (event: ActivityEvent): string => {
  if (event.type === 'score') {
    return t('activity.score', { score: event.new_value })
  }
  return t(`activity.status.${event.new_value}`) + ' '
}

const animeName = (event: ActivityEvent): string => {
  if (!event.anime) return t('home.noData')
  return getLocalizedTitle(event.anime.name, event.anime.name_ru) || t('home.noData')
}

const formatRelativeTime = (dateStr: string): string => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMinutes = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffMinutes < 1) return t('time.justNow')
  if (diffMinutes < 60) return t('time.minutesAgo', { n: diffMinutes })
  if (diffHours < 24) return t('time.hoursAgo', { n: diffHours })
  if (diffDays === 1) return t('common.yesterday')
  if (diffDays < 7) return t('time.daysAgo', { n: diffDays })
  return date.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US', { day: 'numeric', month: 'short' })
}

onMounted(() => {
  loadFeed()
})
</script>

<style scoped>
.glass-card {
  background: rgba(255, 255, 255, 0.03);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.05);
}
</style>
