<template>
  <div class="min-h-screen pb-20 md:pb-0">
    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center items-center min-h-screen">
      <svg class="w-12 h-12 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="flex flex-col items-center justify-center min-h-screen px-4">
      <svg class="w-16 h-16 text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
      <p class="text-white/60 text-lg">{{ error }}</p>
      <router-link to="/" class="mt-4 text-cyan-400 hover:text-cyan-300">
        На главную
      </router-link>
    </div>

    <template v-else-if="user">
      <!-- Profile Header -->
      <div class="relative overflow-hidden">
        <div class="absolute inset-0 bg-gradient-to-b from-cyan-500/10 to-transparent" />

        <div class="relative max-w-4xl mx-auto px-4 pt-24 pb-8">
          <div class="flex flex-col sm:flex-row items-center sm:items-end gap-6">
            <!-- Avatar -->
            <div class="w-28 h-28 sm:w-32 sm:h-32 rounded-full overflow-hidden ring-4 ring-cyan-500/30 bg-surface flex items-center justify-center text-4xl font-bold text-cyan-400 bg-cyan-500/10">
              {{ userInitials }}
            </div>

            <!-- User Info -->
            <div class="text-center sm:text-left flex-1">
              <h1 class="text-2xl sm:text-3xl font-bold text-white mb-2">
                {{ user.username }}
              </h1>
              <div class="flex flex-wrap items-center justify-center sm:justify-start gap-2">
                <span class="text-white/40 text-sm">Профиль пользователя</span>
              </div>
            </div>

            <!-- Share Button -->
            <button
              @click="copyProfileLink"
              class="flex items-center gap-2 px-4 py-2 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
              </svg>
              <span>{{ copied ? 'Скопировано!' : 'Поделиться' }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- Watchlist -->
      <div class="max-w-4xl mx-auto px-4 py-6">
        <!-- Filter Pills -->
        <div v-if="user.public_statuses?.length > 1" class="flex gap-2 overflow-x-auto pb-4 scrollbar-hide">
          <button
            class="flex-shrink-0 px-4 py-2 rounded-full text-sm font-medium transition-colors"
            :class="activeFilter === 'all'
              ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30'
              : 'bg-white/5 text-white/60 border border-transparent hover:text-white'"
            @click="activeFilter = 'all'"
          >
            Все ({{ watchlist.length }})
          </button>
          <button
            v-for="status in user.public_statuses"
            :key="status"
            class="flex-shrink-0 px-4 py-2 rounded-full text-sm font-medium transition-colors"
            :class="activeFilter === status
              ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30'
              : 'bg-white/5 text-white/60 border border-transparent hover:text-white'"
            @click="activeFilter = status"
          >
            {{ statusLabels[status] }} ({{ getCountByStatus(status) }})
          </button>
        </div>

        <!-- Loading Watchlist -->
        <div v-if="loadingWatchlist" class="flex justify-center py-12">
          <svg class="w-8 h-8 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        </div>

        <!-- Empty State -->
        <div v-else-if="filteredWatchlist.length === 0" class="text-center py-12">
          <svg class="w-16 h-16 mx-auto text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <p class="text-white/50">Список пуст</p>
        </div>

        <!-- Watchlist Grid -->
        <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
          <router-link
            v-for="anime in filteredWatchlist"
            :key="anime.anime_id"
            :to="`/anime/${anime.anime_id}`"
            class="group"
          >
            <div class="aspect-[2/3] rounded-lg overflow-hidden bg-surface relative">
              <img
                v-if="anime.anime_cover"
                :src="anime.anime_cover"
                :alt="anime.anime_title"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
              />
              <div v-else class="w-full h-full flex items-center justify-center text-white/20">
                <svg class="w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                </svg>
              </div>

              <!-- Score Badge -->
              <div v-if="anime.score" class="absolute top-2 right-2 px-2 py-1 rounded bg-black/60 text-yellow-400 text-sm font-bold">
                {{ anime.score }}
              </div>

              <!-- Status Badge -->
              <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/80 to-transparent">
                <span class="text-xs px-2 py-0.5 rounded-full" :class="statusColors[anime.status]">
                  {{ statusLabels[anime.status] }}
                </span>
              </div>
            </div>

            <h3 class="mt-2 text-sm text-white group-hover:text-cyan-400 transition-colors line-clamp-2">
              {{ anime.anime_title }}
            </h3>

            <p class="text-xs text-white/50 mt-1">
              {{ anime.episodes || 0 }} / {{ anime.anime_total_episodes || '?' }} эп.
            </p>
          </router-link>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { publicApi } from '@/api/client'

interface PublicUser {
  id: string
  username: string
  public_id: string
  public_statuses: string[]
}

interface WatchlistEntry {
  anime_id: string
  anime_title: string
  anime_cover: string
  status: string
  score: number
  episodes: number
  anime_total_episodes: number
}

const route = useRoute()

const user = ref<PublicUser | null>(null)
const watchlist = ref<WatchlistEntry[]>([])
const loading = ref(true)
const loadingWatchlist = ref(false)
const error = ref<string | null>(null)
const activeFilter = ref('all')
const copied = ref(false)

const statusLabels: Record<string, string> = {
  watching: 'Смотрю',
  completed: 'Просмотрено',
  plan_to_watch: 'В планах',
  on_hold: 'Отложено',
  dropped: 'Брошено'
}

const statusColors: Record<string, string> = {
  watching: 'bg-green-500/80 text-white',
  completed: 'bg-blue-500/80 text-white',
  plan_to_watch: 'bg-purple-500/80 text-white',
  on_hold: 'bg-yellow-500/80 text-black',
  dropped: 'bg-red-500/80 text-white'
}

const userInitials = computed(() => {
  if (!user.value?.username) return '?'
  return user.value.username.slice(0, 2).toUpperCase()
})

const filteredWatchlist = computed(() => {
  if (activeFilter.value === 'all') return watchlist.value
  return watchlist.value.filter(a => a.status === activeFilter.value)
})

const getCountByStatus = (status: string) => {
  return watchlist.value.filter(a => a.status === status).length
}

const copyProfileLink = async () => {
  try {
    await navigator.clipboard.writeText(window.location.href)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch (err) {
    console.error('Failed to copy:', err)
  }
}

const fetchProfile = async () => {
  const publicId = route.params.publicId as string
  if (!publicId) {
    error.value = 'Пользователь не найден'
    loading.value = false
    return
  }

  try {
    const response = await publicApi.getUserProfile(publicId)
    user.value = response.data

    // Fetch watchlist
    if (user.value) {
      loadingWatchlist.value = true
      const watchlistResponse = await publicApi.getPublicWatchlist(
        user.value.id,
        user.value.public_statuses
      )
      watchlist.value = watchlistResponse.data?.data || watchlistResponse.data || []
    }
  } catch (err: any) {
    console.error('Failed to load profile:', err)
    if (err.response?.status === 404) {
      error.value = 'Пользователь не найден'
    } else {
      error.value = 'Не удалось загрузить профиль'
    }
  } finally {
    loading.value = false
    loadingWatchlist.value = false
  }
}

onMounted(fetchProfile)
</script>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
