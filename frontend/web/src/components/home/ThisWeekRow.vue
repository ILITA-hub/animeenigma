<template>
  <!-- Phase 16 (UX-32). This Week row — today + tomorrow's airing episodes
       by hour. Sourced from the same fetchSchedule composable that powers
       /schedule, filtered client-side to a 2-day window. Self-gates on
       items.length === 0 so users with no airing anime in the next 48h
       see no degraded affordance and the Continue-Watching row below
       remains the top anchor. Mount pattern matches ContinueWatchingRow. -->
  <div v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl md:text-2xl font-bold text-white">
        {{ $t('home.thisWeek') }}
      </h2>
      <router-link
        to="/schedule"
        class="text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
      >
        {{ $t('home.seeAll') }}
      </router-link>
    </div>
    <div class="flex gap-3 overflow-x-auto scrollbar-hide pb-2 -mx-1 px-1">
      <router-link
        v-for="item in items"
        :key="item.id + ':' + (item.next_episode_at ?? '')"
        :to="`/anime/${item.id}?episode=${(item.episodes_aired || 0) + 1}`"
        class="flex-shrink-0 w-32 md:w-40 lg:w-48 group"
      >
        <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] mb-2">
          <img
            :src="item.poster_url || '/placeholder.svg'"
            alt=""
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
          <!-- Day chip (Today / Tomorrow) — top-left, cyan for "today" so
               the eye lands there first when scrolling. -->
          <span
            class="absolute top-2 left-2 px-2 py-0.5 rounded text-[10px] font-bold tracking-wider text-white"
            :class="item._isToday ? 'bg-cyan-500/90' : 'bg-white/20 backdrop-blur-sm'"
          >
            {{ item._isToday ? $t('home.thisWeekToday') : $t('home.thisWeekTomorrow') }}
          </span>
          <!-- Episode badge — top-right, matches the Continue-Watching pattern. -->
          <div class="absolute top-2 right-2 px-2 py-1 rounded-md bg-black/70 backdrop-blur-sm text-xs font-semibold text-white">
            {{ $t('home.continueWatchingEpisode', { n: (item.episodes_aired || 0) + 1 }) }}
          </div>
          <!-- Time badge — bottom-left, cyan time over translucent bar. -->
          <div class="absolute bottom-0 left-0 right-0 px-2 py-1 bg-black/60 backdrop-blur-sm flex items-center justify-between">
            <span class="text-xs font-medium text-cyan-400">{{ item._timeLabel }}</span>
          </div>
        </div>
        <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">
          {{ getLocalizedTitle(item.name, item.name_ru, item.name_jp) }}
        </h3>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
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

// Decorated item we render — schedule entry plus the precomputed
// today/tomorrow flag and formatted time label.
interface ThisWeekItem extends ScheduleAnime {
  _isToday: boolean
  _timeLabel: string
}

const { locale } = useI18n()
const { fetchSchedule } = useAnime()
const schedule = ref<ScheduleAnime[]>([])

// Compare two Date instances by calendar day in the user's local
// time zone — schedule items use UTC ISO strings, and getDate()/getMonth()/
// getFullYear() implicitly project into local time.
function sameLocalDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

function formatLocalTime(date: Date): string {
  const localeMap: Record<string, string> = { ru: 'ru-RU', en: 'en-US', ja: 'ja-JP' }
  // 24-hour HH:MM in the browser's local time zone — schedule.vue forces
  // Europe/Moscow, but for the Home row we want the user's actual local
  // hour so the "Today 18:00" badge maps to the user's clock.
  return date.toLocaleTimeString(localeMap[locale.value] || 'en-US', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const items = computed<ThisWeekItem[]>(() => {
  if (!schedule.value.length) return []
  const now = new Date()
  const tomorrow = new Date(now)
  tomorrow.setDate(tomorrow.getDate() + 1)

  const out: ThisWeekItem[] = []
  for (const anime of schedule.value) {
    if (!anime.next_episode_at) continue
    const date = new Date(anime.next_episode_at)
    if (Number.isNaN(date.getTime())) continue
    const isToday = sameLocalDay(date, now)
    const isTomorrow = sameLocalDay(date, tomorrow)
    if (!isToday && !isTomorrow) continue
    // Skip episodes whose air time has already passed today — they're not
    // upcoming any more and would confuse "Today 14:00" when the wall clock
    // already reads 18:00.
    if (isToday && date.getTime() < now.getTime()) continue
    out.push({
      ...anime,
      _isToday: isToday,
      _timeLabel: formatLocalTime(date),
    })
  }
  // Sort ascending by airtime so the next-up episode is leftmost.
  out.sort((a, b) => {
    const ta = new Date(a.next_episode_at ?? 0).getTime()
    const tb = new Date(b.next_episode_at ?? 0).getTime()
    return ta - tb
  })
  return out
})

let active = true

onMounted(async () => {
  try {
    const data = await fetchSchedule()
    if (active) schedule.value = data
  } catch (err) {
    // Non-fatal — the row simply stays hidden when fetch fails. Don't
    // surface an error UI on a discovery row above the fold.
    console.error('Failed to fetch this-week schedule:', err)
  }
})

onBeforeUnmount(() => {
  active = false
})
</script>
