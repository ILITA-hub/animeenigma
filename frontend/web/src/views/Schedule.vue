<template>
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8">
      <h1 class="text-3xl font-bold text-white mb-8">{{ $t('schedule.title') }}</h1>

      <!-- Loading -->
      <div v-if="loading" class="flex justify-center py-12">
        <div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>
      </div>

      <!-- Schedule by day -->
      <div v-else-if="scheduleByDay.length > 0" class="space-y-8">
        <div v-for="day in scheduleByDay" :key="day.dayName" class="glass-card p-6">
          <h2 class="text-xl font-semibold text-white mb-4 flex items-center gap-2">
            <span class="w-3 h-3 rounded-full" :class="day.isToday ? 'bg-cyan-500' : 'bg-white/30'"></span>
            {{ day.dayName }}
            <span v-if="day.isToday" class="text-sm text-cyan-400 font-normal">{{ $t('schedule.today') }}</span>
          </h2>

          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            <router-link
              v-for="anime in day.animes"
              :key="anime.id"
              :to="`/anime/${anime.id}`"
              class="flex gap-3 p-3 rounded-lg bg-white/5 hover:bg-white/10 transition-colors"
            >
              <img
                :src="anime.poster_url || '/placeholder.png'"
                :alt="getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp)"
                class="w-16 h-24 object-cover rounded"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-white font-medium truncate">{{ getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp) }}</h3>
                <p class="text-white/60 text-sm mt-1">
                  {{ $t('schedule.episode', { n: (anime.episodes_aired || 0) + 1 }) }}
                </p>
                <p class="text-cyan-400 text-sm mt-1">
                  {{ formatTime(anime.next_episode_at ?? '') }}
                </p>
              </div>
            </router-link>
          </div>
        </div>
      </div>

      <!-- No schedule -->
      <div v-else class="text-center py-12">
        <p class="text-white/60">{{ $t('schedule.noData') }}</p>
        <p class="text-white/40 text-sm mt-2">{{ $t('schedule.hint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAnime } from '@/composables/useAnime'
import { getLocalizedTitle } from '@/utils/title'

interface ScheduleAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  next_episode_at?: string | null
  episodes_aired?: number
}

const { t, locale } = useI18n()
const { fetchSchedule, loading } = useAnime()
const schedule = ref<ScheduleAnime[]>([])

const dayNames = computed(() => [t('schedule.days.sunday'), t('schedule.days.monday'), t('schedule.days.tuesday'), t('schedule.days.wednesday'), t('schedule.days.thursday'), t('schedule.days.friday'), t('schedule.days.saturday')])

const scheduleByDay = computed(() => {
  if (!schedule.value.length) return []

  const today = new Date()
  const todayDay = today.getDay()

  // Group by day of week
  const grouped: Record<number, ScheduleAnime[]> = {}

  for (const anime of schedule.value) {
    if (!anime.next_episode_at) continue
    const date = new Date(anime.next_episode_at)
    const day = date.getDay()
    if (!grouped[day]) grouped[day] = []
    grouped[day].push(anime)
  }

  // Sort each day's anime by time
  for (const day in grouped) {
    grouped[day].sort((a, b) =>
      new Date(a.next_episode_at ?? 0).getTime() - new Date(b.next_episode_at ?? 0).getTime()
    )
  }

  // Create ordered list starting from today
  const result = []
  for (let i = 0; i < 7; i++) {
    const dayIndex = (todayDay + i) % 7
    if (grouped[dayIndex] && grouped[dayIndex].length > 0) {
      result.push({
        dayIndex,
        dayName: dayNames.value[dayIndex],
        isToday: i === 0,
        animes: grouped[dayIndex]
      })
    }
  }

  return result
})

const formatTime = (dateStr: string) => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const localeMap: Record<string, string> = { ru: 'ru-RU', en: 'en-US', ja: 'ja-JP' }
  const timeStr = date.toLocaleTimeString(localeMap[locale.value] || 'en-US', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Moscow'
  })
  return t('schedule.timeMsk', { time: timeStr })
}

onMounted(async () => {
  try {
    schedule.value = await fetchSchedule()
  } catch (err) {
    console.error('Failed to fetch schedule:', err)
  }
})
</script>
