<template>
  <div class="min-h-screen bg-gradient-to-b from-gray-900 via-gray-900 to-black">
    <!-- Search Bar -->
    <h1 class="sr-only">AnimeEnigma</h1>
    <div class="pt-24 px-4 lg:px-8 max-w-7xl mx-auto mb-8">
      <div class="flex items-center gap-3 relative z-[60]">
        <div class="flex-1">
          <SearchAutocomplete
            v-model="searchQuery"
            listbox-id="home-search"
            @submit="goToSearch"
          />
        </div>
        <router-link
          to="/schedule"
          class="flex items-center gap-2 px-4 py-4 rounded-xl bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
          <span class="hidden sm:inline">{{ $t('nav.schedule') }}</span>
        </router-link>
      </div>
    </div>

    <!-- Three Columns Layout -->
    <div class="px-4 lg:px-8 max-w-7xl mx-auto pb-12">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">

        <!-- Ongoing Column -->
        <div class="glass-card rounded-2xl p-5">
          <div class="flex items-center gap-3 mb-5">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-green-500 to-emerald-500 flex items-center justify-center">
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div class="flex-1">
              <h2 class="text-xl font-bold text-white">{{ $t('home.ongoing') }}</h2>
              <p v-if="ongoingUpdatedAt && !loadingOngoing" class="text-xs text-gray-500">
                {{ $t('home.updated', { time: formatUpdatedAt(ongoingUpdatedAt) }) }}
              </p>
            </div>
            <router-link
              to="/browse?status=ongoing"
              class="ml-auto text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
            >
              {{ $t('home.seeAll') }}
            </router-link>
          </div>

          <div v-if="loadingOngoing" class="space-y-3">
            <div v-for="i in 5" :key="i" class="animate-pulse">
              <div class="flex gap-3">
                <div class="w-16 h-20 bg-white/10 rounded-lg"></div>
                <div class="flex-1 space-y-2">
                  <div class="h-4 bg-white/10 rounded w-3/4"></div>
                  <div class="h-3 bg-white/10 rounded w-1/2"></div>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="space-y-3 max-h-[600px] overflow-y-auto custom-scrollbar">
            <router-link
              v-for="anime in ongoingAnime"
              :key="anime.id"
              :to="`/anime/${anime.id}`"
              class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors group"
            >
              <img
                :src="anime.poster_url || '/placeholder.svg'"
                :alt="getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp)"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp) }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.episodes_count ? $t('home.episodeCount', { count: anime.episodes_count }) : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span class="text-xs px-2 py-0.5 rounded-full bg-green-500/20 text-green-400">
                    {{ $t('home.airing') }}
                  </span>
                  <span v-if="anime.score" class="text-xs text-yellow-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ anime.score.toFixed(1) }}
                  </span>
                  <span v-if="siteRatings[anime.id]?.total_reviews > 0" class="text-xs text-cyan-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ siteRatings[anime.id].average_score.toFixed(1) }}
                  </span>
                </div>
                <!-- Next episode info -->
                <p v-if="anime.next_episode_at" class="text-xs text-cyan-400 mt-1 flex items-center gap-1">
                  <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  {{ $t('schedule.episode', { n: (anime.episodes_aired || 0) + 1 }) }}: {{ formatNextEpisode(anime.next_episode_at) }}
                </p>
              </div>
            </router-link>

            <div v-if="ongoingAnime.length === 0" class="text-center py-8 text-gray-400">
              {{ $t('home.noOngoing') }}
            </div>
          </div>
        </div>

        <!-- Top Anime Column -->
        <div class="glass-card rounded-2xl p-5">
          <div class="flex items-center gap-3 mb-5">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-yellow-500 to-orange-500 flex items-center justify-center">
              <svg class="w-5 h-5 text-white" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
              </svg>
            </div>
            <h2 class="text-xl font-bold text-white">{{ $t('home.topAnime') }}</h2>
          </div>

          <div v-if="loadingTop" class="space-y-3">
            <div v-for="i in 5" :key="i" class="animate-pulse">
              <div class="flex gap-3">
                <div class="w-8 h-8 bg-white/10 rounded-full"></div>
                <div class="w-16 h-20 bg-white/10 rounded-lg"></div>
                <div class="flex-1 space-y-2">
                  <div class="h-4 bg-white/10 rounded w-3/4"></div>
                  <div class="h-3 bg-white/10 rounded w-1/2"></div>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="space-y-3 max-h-[600px] overflow-y-auto custom-scrollbar">
            <router-link
              v-for="(anime, index) in topAnime"
              :key="anime.id"
              :to="`/anime/${anime.id}`"
              class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors group"
            >
              <div class="w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 font-bold text-sm"
                   :class="getRankClass(index)">
                {{ index + 1 }}
              </div>
              <img
                :src="anime.poster_url || '/placeholder.svg'"
                :alt="getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp)"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp) }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.year }} {{ anime.episodes_count ? `/ ${$t('home.episodeCount', { count: anime.episodes_count })}` : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span v-if="anime.score" class="text-xs text-yellow-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ anime.score.toFixed(1) }}
                  </span>
                  <span v-if="siteRatings[anime.id]?.total_reviews > 0" class="text-xs text-cyan-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ siteRatings[anime.id].average_score.toFixed(1) }}
                  </span>
                  <span class="text-xs px-2 py-0.5 rounded-full bg-purple-500/20 text-purple-400">
                    {{ anime.status ? $t(`anime.status.${anime.status}`) : '' }}
                  </span>
                </div>
              </div>
            </router-link>

            <div v-if="topAnime.length === 0" class="text-center py-8 text-gray-400">
              {{ $t('home.noData') }}
            </div>
          </div>
        </div>

        <!-- Announcements Column -->
        <div class="glass-card rounded-2xl p-5">
          <div class="flex items-center gap-3 mb-5">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-blue-500 to-cyan-500 flex items-center justify-center">
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
            </div>
            <h2 class="text-xl font-bold text-white">{{ $t('home.announcements') }}</h2>
            <router-link
              to="/browse?status=announced"
              class="ml-auto text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
            >
              {{ $t('home.seeAll') }}
            </router-link>
          </div>

          <div v-if="loadingAnnounced" class="space-y-3">
            <div v-for="i in 5" :key="i" class="animate-pulse">
              <div class="flex gap-3">
                <div class="w-16 h-20 bg-white/10 rounded-lg"></div>
                <div class="flex-1 space-y-2">
                  <div class="h-4 bg-white/10 rounded w-3/4"></div>
                  <div class="h-3 bg-white/10 rounded w-1/2"></div>
                </div>
              </div>
            </div>
          </div>

          <div v-else class="space-y-3 max-h-[600px] overflow-y-auto custom-scrollbar">
            <router-link
              v-for="anime in announcedAnime"
              :key="anime.id"
              :to="`/anime/${anime.id}`"
              class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors group"
            >
              <img
                :src="anime.poster_url || '/placeholder.svg'"
                :alt="getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp)"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp) }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.year }} {{ anime.season ? `/ ${$t(`seasons.${anime.season}`)}` : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span class="text-xs px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-400">
                    {{ $t('home.announced') }}
                  </span>
                </div>
              </div>
            </router-link>

            <div v-if="announcedAnime.length === 0" class="text-center py-8 text-gray-400">
              {{ $t('home.noAnnounced') }}
            </div>
          </div>
        </div>
      </div>

      <!-- Activity Feed + Last Updates -->
      <div class="mt-6 grid grid-cols-1 md:grid-cols-2 gap-6">
        <ActivityFeed />
        <LastUpdates />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { getLocalizedTitle } from '@/utils/title'
import { useHomeStore } from '@/stores/home'
import { SearchAutocomplete } from '@/components/ui'
import ActivityFeed from '@/components/ActivityFeed.vue'
import LastUpdates from '@/components/LastUpdates.vue'

const router = useRouter()
const { t } = useI18n()
const homeStore = useHomeStore()

const searchQuery = ref('')

const {
  announcedAnime,
  ongoingAnime,
  topAnime,
  siteRatings,
  ongoingUpdatedAt,
  loadingAnnounced,
  loadingOngoing,
  loadingTop,
} = storeToRefs(homeStore)

const goToSearch = () => {
  if (searchQuery.value.trim()) {
    router.push({ path: '/browse', query: { q: searchQuery.value.trim() } })
  }
}

const getRankClass = (index: number) => {
  if (index === 0) return 'bg-gradient-to-br from-yellow-400 to-yellow-600 text-black'
  if (index === 1) return 'bg-gradient-to-br from-gray-300 to-gray-500 text-black'
  if (index === 2) return 'bg-gradient-to-br from-amber-600 to-amber-800 text-white'
  return 'bg-white/10 text-gray-400'
}

const formatNextEpisode = (dateStr: string) => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  const timeStr = date.toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Europe/Moscow'
  })

  if (diffDays === 0) {
    return t('home.todayAt', { time: timeStr })
  } else if (diffDays === 1) {
    return t('home.tomorrowAt', { time: timeStr })
  } else if (diffDays > 1 && diffDays < 7) {
    const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
    return t('home.dayAt', { day: t(`schedule.daysShort.${dayKeys[date.getDay()]}`), time: timeStr })
  } else {
    return date.toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'short'
    })
  }
}

const formatUpdatedAt = (dateStr: string) => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMinutes = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffMinutes < 1) {
    return t('time.justNow')
  } else if (diffMinutes < 60) {
    return t('time.minutesAgo', { n: diffMinutes })
  } else if (diffHours < 24) {
    return t('time.hoursAgo', { n: diffHours })
  } else if (diffDays === 1) {
    return t('common.yesterday')
  } else if (diffDays < 7) {
    return t('time.daysAgo', { n: diffDays })
  } else {
    return date.toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'short'
    })
  }
}

onMounted(() => {
  homeStore.fetchAll()
})
</script>

<style scoped>
.glass-card {
  background: rgba(255, 255, 255, 0.03);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.05);
}

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.2);
}

.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
