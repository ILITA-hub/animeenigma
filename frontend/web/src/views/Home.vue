<template>
  <div class="min-h-screen bg-gradient-to-b from-gray-900 via-gray-900 to-black">
    <!-- Header -->
    <div class="pt-8 pb-6 px-4 lg:px-8 max-w-7xl mx-auto">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-3xl md:text-4xl font-bold text-white mb-2">
            AnimeEnigma
          </h1>
          <p class="text-gray-400">
            Смотри аниме онлайн бесплатно
          </p>
        </div>
        <router-link
          to="/schedule"
          class="flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
          <span class="hidden sm:inline">Расписание</span>
        </router-link>
      </div>
    </div>

    <!-- Search Bar -->
    <div class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
      <div class="relative">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Поиск аниме..."
          class="w-full bg-white/5 backdrop-blur-sm border border-white/10 rounded-xl px-5 py-4 text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-purple-500/50 focus:border-transparent transition-all"
          @keyup.enter="goToSearch"
        />
        <button
          @click="goToSearch"
          class="absolute right-3 top-1/2 -translate-y-1/2 p-2 text-gray-400 hover:text-white transition-colors"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Three Columns Layout -->
    <div class="px-4 lg:px-8 max-w-7xl mx-auto pb-12">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">

        <!-- Announcements Column -->
        <div class="glass-card rounded-2xl p-5">
          <div class="flex items-center gap-3 mb-5">
            <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-blue-500 to-cyan-500 flex items-center justify-center">
              <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
            </div>
            <h2 class="text-xl font-bold text-white">Анонсы</h2>
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
                :alt="anime.name_ru || anime.name"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ anime.name_ru || anime.name }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.year }} {{ anime.season ? `/ ${translateSeason(anime.season)}` : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span class="text-xs px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-400">
                    Анонс
                  </span>
                </div>
              </div>
            </router-link>

            <div v-if="announcedAnime.length === 0" class="text-center py-8 text-gray-400">
              Нет анонсов
            </div>
          </div>
        </div>

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
              <h2 class="text-xl font-bold text-white">Онгоинги</h2>
              <p v-if="ongoingUpdatedAt && !loadingOngoing" class="text-xs text-gray-500">
                Обновлено {{ formatUpdatedAt(ongoingUpdatedAt) }}
              </p>
            </div>
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
                :alt="anime.name_ru || anime.name"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ anime.name_ru || anime.name }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.episodes_count ? `${anime.episodes_count} эп.` : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span class="text-xs px-2 py-0.5 rounded-full bg-green-500/20 text-green-400">
                    Выходит
                  </span>
                  <span v-if="anime.score" class="text-xs text-yellow-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ anime.score.toFixed(1) }}
                  </span>
                </div>
                <!-- Next episode info -->
                <p v-if="anime.next_episode_at" class="text-xs text-cyan-400 mt-1 flex items-center gap-1">
                  <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  Эп. {{ (anime.episodes_aired || 0) + 1 }}: {{ formatNextEpisode(anime.next_episode_at) }}
                </p>
              </div>
            </router-link>

            <div v-if="ongoingAnime.length === 0" class="text-center py-8 text-gray-400">
              Нет онгоингов
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
            <h2 class="text-xl font-bold text-white">Топ аниме</h2>
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
                :alt="anime.name_ru || anime.name"
                class="w-16 h-20 object-cover rounded-lg flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium text-white group-hover:text-purple-400 transition-colors line-clamp-2">
                  {{ anime.name_ru || anime.name }}
                </h3>
                <p class="text-xs text-gray-400 mt-1">
                  {{ anime.year }} {{ anime.episodes_count ? `/ ${anime.episodes_count} эп.` : '' }}
                </p>
                <div class="flex items-center gap-2 mt-1">
                  <span v-if="anime.score" class="text-xs text-yellow-400 flex items-center gap-1">
                    <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ anime.score.toFixed(1) }}
                  </span>
                  <span class="text-xs px-2 py-0.5 rounded-full bg-purple-500/20 text-purple-400">
                    {{ translateStatus(anime.status) }}
                  </span>
                </div>
              </div>
            </router-link>

            <div v-if="topAnime.length === 0" class="text-center py-8 text-gray-400">
              Нет данных
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { animeApi } from '@/api/client'

interface Anime {
  id: string
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  season?: string
  next_episode_at?: string
  updated_at?: string
}

const router = useRouter()

const searchQuery = ref('')
const announcedAnime = ref<Anime[]>([])
const ongoingAnime = ref<Anime[]>([])
const topAnime = ref<Anime[]>([])
const ongoingUpdatedAt = ref<string | null>(null)

const loadingAnnounced = ref(true)
const loadingOngoing = ref(true)
const loadingTop = ref(true)

const goToSearch = () => {
  if (searchQuery.value.trim()) {
    router.push({ path: '/browse', query: { q: searchQuery.value.trim() } })
  }
}

const translateSeason = (season: string) => {
  const seasons: Record<string, string> = {
    winter: 'Зима',
    spring: 'Весна',
    summer: 'Лето',
    fall: 'Осень'
  }
  return seasons[season] || season
}

const translateStatus = (status?: string) => {
  if (!status) return ''
  const statuses: Record<string, string> = {
    released: 'Вышло',
    ongoing: 'Выходит',
    anons: 'Анонс'
  }
  return statuses[status] || status
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
    return `Сегодня ${timeStr}`
  } else if (diffDays === 1) {
    return `Завтра ${timeStr}`
  } else if (diffDays > 1 && diffDays < 7) {
    const dayNames = ['Вс', 'Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб']
    return `${dayNames[date.getDay()]} ${timeStr}`
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
    return 'только что'
  } else if (diffMinutes < 60) {
    return `${diffMinutes} мин. назад`
  } else if (diffHours < 24) {
    return `${diffHours} ч. назад`
  } else if (diffDays === 1) {
    return 'вчера'
  } else if (diffDays < 7) {
    return `${diffDays} дн. назад`
  } else {
    return date.toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'short'
    })
  }
}

onMounted(async () => {
  // Fetch announced anime
  try {
    const response = await animeApi.getAnnounced(15)
    announcedAnime.value = response.data?.data || []
  } catch (err) {
    console.error('Failed to load announced anime:', err)
  } finally {
    loadingAnnounced.value = false
  }

  // Fetch ongoing anime
  try {
    const response = await animeApi.getOngoing(15)
    const animes = response.data?.data || []
    ongoingAnime.value = animes
    // Find the most recent updated_at
    if (animes.length > 0) {
      const maxUpdated = animes.reduce((max: string | null, anime: Anime) => {
        if (!anime.updated_at) return max
        if (!max) return anime.updated_at
        return new Date(anime.updated_at) > new Date(max) ? anime.updated_at : max
      }, null as string | null)
      ongoingUpdatedAt.value = maxUpdated
    }
  } catch (err) {
    console.error('Failed to load ongoing anime:', err)
  } finally {
    loadingOngoing.value = false
  }

  // Fetch top anime
  try {
    const response = await animeApi.getTop(15)
    topAnime.value = response.data?.data || []
  } catch (err) {
    console.error('Failed to load top anime:', err)
  } finally {
    loadingTop.value = false
  }
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
