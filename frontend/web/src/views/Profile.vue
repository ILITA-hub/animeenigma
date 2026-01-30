<template>
  <div class="min-h-screen pb-20 md:pb-0">
    <!-- Profile Header -->
    <div class="relative overflow-hidden">
      <!-- Background -->
      <div class="absolute inset-0 bg-gradient-to-b from-cyan-500/10 to-transparent" />

      <div class="relative max-w-4xl mx-auto px-4 pt-24 pb-8">
        <div class="flex flex-col sm:flex-row items-center sm:items-end gap-6">
          <!-- Avatar -->
          <div class="relative">
            <div class="w-28 h-28 sm:w-32 sm:h-32 rounded-full overflow-hidden ring-4 ring-cyan-500/30 bg-surface">
              <img
                v-if="authStore.user?.avatar"
                :src="authStore.user.avatar"
                :alt="authStore.user.username"
                class="w-full h-full object-cover"
              />
              <div
                v-else
                class="w-full h-full flex items-center justify-center text-4xl font-bold text-cyan-400 bg-cyan-500/10"
              >
                {{ userInitials }}
              </div>
            </div>
            <button class="absolute bottom-0 right-0 w-8 h-8 rounded-full bg-cyan-500 flex items-center justify-center text-white shadow-lg hover:bg-cyan-400 transition-colors">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
              </svg>
            </button>
          </div>

          <!-- User Info -->
          <div class="text-center sm:text-left flex-1">
            <h1 class="text-2xl sm:text-3xl font-bold text-white mb-1">
              {{ authStore.user?.username || 'User' }}
            </h1>
            <p class="text-white/60 mb-2">{{ authStore.user?.email }}</p>
            <div class="flex flex-wrap items-center justify-center sm:justify-start gap-2">
              <Badge variant="primary" size="sm">{{ authStore.user?.role || 'Member' }}</Badge>
              <span class="text-white/40 text-sm">{{ $t('profile.memberSince') }} 2024</span>
            </div>
          </div>

          <!-- Edit Profile Button -->
          <Button variant="ghost" size="sm" class="hidden sm:flex">
            {{ $t('profile.editProfile') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="max-w-4xl mx-auto px-4">
      <Tabs v-model="activeTab" :tabs="tabs" variant="underline" full-width class="mb-6">
        <template #watchlist>
          <div v-if="watchlist.length > 0" class="space-y-4">
            <!-- Filter Pills -->
            <div class="flex gap-2 overflow-x-auto pb-2 scrollbar-hide">
              <button
                v-for="filter in watchlistFilters"
                :key="filter.value"
                class="flex-shrink-0 px-4 py-2 rounded-full text-sm font-medium transition-colors"
                :class="watchlistFilter === filter.value
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30'
                  : 'bg-white/5 text-white/60 border border-transparent hover:text-white'"
                @click="watchlistFilter = filter.value"
              >
                {{ $t(`profile.watchlist.${filter.value}`) }}
                <span class="ml-1 opacity-60">({{ filter.count }})</span>
              </button>
            </div>

            <!-- Watchlist Grid -->
            <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
              <AnimeCardNew
                v-for="anime in filteredWatchlist"
                :key="anime.id"
                :anime="anime"
              />
            </div>
          </div>
          <div v-else class="text-center py-12">
            <p class="text-white/50 mb-4">{{ $t('profile.watchlist.empty') || 'Your watchlist is empty' }}</p>
            <Button variant="outline" @click="$router.push('/browse')">
              {{ $t('nav.catalog') }}
            </Button>
          </div>
        </template>

        <template #history>
          <div v-if="history.length > 0" class="space-y-3">
            <div
              v-for="item in history"
              :key="item.id"
              class="flex gap-4 p-4 rounded-xl bg-white/5 border border-white/10 hover:bg-white/10 transition-colors"
            >
              <router-link :to="`/anime/${item.animeId}`" class="flex-shrink-0 w-16 aspect-[2/3] rounded-lg overflow-hidden">
                <img :src="item.coverImage" :alt="item.title" class="w-full h-full object-cover" />
              </router-link>
              <div class="flex-1 min-w-0">
                <router-link :to="`/anime/${item.animeId}`" class="font-medium text-white hover:text-cyan-400 transition-colors">
                  {{ item.title }}
                </router-link>
                <p class="text-white/50 text-sm">Episode {{ item.episode }}</p>
                <div class="mt-2 h-1 bg-white/10 rounded-full overflow-hidden">
                  <div class="h-full bg-cyan-400" :style="{ width: `${item.progress}%` }" />
                </div>
              </div>
              <div class="text-right text-sm text-white/40">
                {{ item.watchedAt }}
              </div>
            </div>
          </div>
          <div v-else class="text-center py-12">
            <p class="text-white/50">{{ $t('profile.history.empty') || 'No watch history yet' }}</p>
          </div>
        </template>

        <template #favorites>
          <div v-if="favorites.length > 0" class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
            <AnimeCardNew
              v-for="anime in favorites"
              :key="anime.id"
              :anime="anime"
            />
          </div>
          <div v-else class="text-center py-12">
            <p class="text-white/50">{{ $t('profile.favorites.empty') || 'No favorites yet' }}</p>
          </div>
        </template>

        <template #settings>
          <div class="space-y-6">
            <!-- Appearance -->
            <div class="glass-card p-6">
              <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.appearance') }}</h3>
              <div class="space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <p class="text-white">{{ $t('profile.settings.language') }}</p>
                    <p class="text-white/50 text-sm">{{ $t('profile.settings.languageDesc') || 'Interface language' }}</p>
                  </div>
                  <select
                    v-model="settings.language"
                    class="bg-white/10 border border-white/10 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-cyan-500"
                  >
                    <option value="ru">Русский</option>
                    <option value="ja">日本語</option>
                    <option value="en">English</option>
                  </select>
                </div>
                <div class="flex items-center justify-between">
                  <div>
                    <p class="text-white">{{ $t('profile.settings.reduceMotion') }}</p>
                    <p class="text-white/50 text-sm">{{ $t('profile.settings.reduceMotionDesc') || 'Reduce animations' }}</p>
                  </div>
                  <button
                    class="w-12 h-7 rounded-full transition-colors relative"
                    :class="settings.reduceMotion ? 'bg-cyan-500' : 'bg-white/20'"
                    @click="settings.reduceMotion = !settings.reduceMotion"
                  >
                    <span
                      class="absolute top-1 w-5 h-5 rounded-full bg-white transition-transform"
                      :class="settings.reduceMotion ? 'left-6' : 'left-1'"
                    />
                  </button>
                </div>
              </div>
            </div>

            <!-- Playback -->
            <div class="glass-card p-6">
              <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.playback') }}</h3>
              <div class="space-y-4">
                <div class="flex items-center justify-between">
                  <div>
                    <p class="text-white">{{ $t('profile.settings.autoplay') }}</p>
                  </div>
                  <button
                    class="w-12 h-7 rounded-full transition-colors relative"
                    :class="settings.autoplay ? 'bg-cyan-500' : 'bg-white/20'"
                    @click="settings.autoplay = !settings.autoplay"
                  >
                    <span
                      class="absolute top-1 w-5 h-5 rounded-full bg-white transition-transform"
                      :class="settings.autoplay ? 'left-6' : 'left-1'"
                    />
                  </button>
                </div>
                <div class="flex items-center justify-between">
                  <div>
                    <p class="text-white">{{ $t('profile.settings.defaultQuality') }}</p>
                  </div>
                  <select
                    v-model="settings.defaultQuality"
                    class="bg-white/10 border border-white/10 rounded-lg px-4 py-2 text-white focus:outline-none focus:border-cyan-500"
                  >
                    <option value="auto">Auto</option>
                    <option value="1080p">1080p</option>
                    <option value="720p">720p</option>
                    <option value="480p">480p</option>
                  </select>
                </div>
              </div>
            </div>

            <!-- Account -->
            <div class="glass-card p-6">
              <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.account') }}</h3>
              <div class="space-y-4">
                <Button variant="ghost" full-width class="justify-start">
                  {{ $t('profile.settings.changePassword') }}
                </Button>
                <Button variant="secondary" full-width @click="logout">
                  {{ $t('profile.settings.signOut') }}
                </Button>
              </div>
            </div>
          </div>
        </template>
      </Tabs>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { Badge, Button, Tabs } from '@/components/ui'
import { AnimeCardNew } from '@/components/anime'

interface Anime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  genres?: string[]
  status?: string
}

interface HistoryItem {
  id: string
  animeId: string
  title: string
  coverImage: string
  episode: number
  progress: number
  watchedAt: string
}

const router = useRouter()
const { locale } = useI18n()
const authStore = useAuthStore()

const activeTab = ref('watchlist')
const watchlistFilter = ref('all')

const tabs = [
  { value: 'watchlist', label: 'Watchlist' },
  { value: 'history', label: 'History' },
  { value: 'favorites', label: 'Favorites' },
  { value: 'settings', label: 'Settings' },
]

const settings = reactive({
  language: locale.value,
  reduceMotion: false,
  autoplay: false,
  defaultQuality: 'auto',
})

// Mock data
const watchlist = ref<Anime[]>([])
const history = ref<HistoryItem[]>([])
const favorites = ref<Anime[]>([])

const watchlistFilters = computed(() => [
  { value: 'all', count: watchlist.value.length },
  { value: 'watching', count: 0 },
  { value: 'planToWatch', count: 0 },
  { value: 'completed', count: 0 },
  { value: 'dropped', count: 0 },
])

const filteredWatchlist = computed(() => {
  if (watchlistFilter.value === 'all') return watchlist.value
  return watchlist.value.filter((a: Anime) => (a as Anime & { status?: string }).status === watchlistFilter.value)
})

const userInitials = computed(() => {
  const username = authStore.user?.username
  if (!username) return '?'
  return username.slice(0, 2).toUpperCase()
})

const logout = () => {
  authStore.logout()
  router.push('/')
}

onMounted(async () => {
  if (!authStore.user) {
    await authStore.fetchUser()
  }
})
</script>
