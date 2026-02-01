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
          <!-- Loading -->
          <div v-if="loading" class="flex justify-center py-12">
            <svg class="w-8 h-8 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          </div>

          <div v-else-if="watchlistEntries.length > 0" class="space-y-4">
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
                {{ filter.label }}
                <span class="ml-1 opacity-60">({{ filter.count }})</span>
              </button>
            </div>

            <!-- View Toggle -->
            <div class="flex justify-end gap-2">
              <button
                class="p-2 rounded-lg transition-colors"
                :class="viewMode === 'table' ? 'bg-cyan-500/20 text-cyan-400' : 'bg-white/5 text-white/60 hover:text-white'"
                @click="viewMode = 'table'"
                title="Table view"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
                </svg>
              </button>
              <button
                class="p-2 rounded-lg transition-colors"
                :class="viewMode === 'grid' ? 'bg-cyan-500/20 text-cyan-400' : 'bg-white/5 text-white/60 hover:text-white'"
                @click="viewMode = 'grid'"
                title="Grid view"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
                </svg>
              </button>
            </div>

            <!-- MAL-style Table View -->
            <div v-if="viewMode === 'table'" class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="text-left text-white/60 border-b border-white/10">
                    <th class="pb-3 pr-2 w-8">{{ $t('profile.table.number') }}</th>
                    <th class="pb-3 px-2 w-16">{{ $t('profile.table.image') }}</th>
                    <th class="pb-3 px-2">{{ $t('profile.table.title') }}</th>
                    <th class="pb-3 px-2 w-16 text-center">{{ $t('profile.table.score') }}</th>
                    <th class="pb-3 px-2 w-20">{{ $t('profile.table.type') }}</th>
                    <th class="pb-3 px-2 w-24">{{ $t('profile.table.progress') }}</th>
                    <th class="pb-3 px-2 hidden md:table-cell">{{ $t('profile.table.tags') }}</th>
                    <th class="pb-3 pl-2 w-24 text-center">{{ $t('profile.table.actions') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(anime, index) in filteredWatchlist"
                    :key="anime.id"
                    class="border-b border-white/5 hover:bg-white/5 transition-colors group"
                  >
                    <!-- Number -->
                    <td class="py-3 pr-2 text-white/40">{{ index + 1 }}</td>

                    <!-- Image -->
                    <td class="py-3 px-2">
                      <router-link :to="`/anime/${anime.id}`" class="block w-12 h-16 rounded overflow-hidden bg-surface">
                        <img
                          v-if="anime.coverImage && anime.coverImage !== '/placeholder.svg'"
                          :src="anime.coverImage"
                          :alt="anime.title"
                          class="w-full h-full object-cover"
                          @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'"
                        />
                        <div v-else class="w-full h-full flex items-center justify-center text-white/20">
                          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                          </svg>
                        </div>
                      </router-link>
                    </td>

                    <!-- Title -->
                    <td class="py-3 px-2">
                      <router-link :to="`/anime/${anime.id}`" class="text-white hover:text-cyan-400 transition-colors font-medium">
                        {{ anime.title }}
                      </router-link>
                      <div v-if="anime.isRewatching" class="text-xs text-cyan-400 mt-0.5">{{ $t('profile.rewatching') }}</div>
                    </td>

                    <!-- Score -->
                    <td class="py-3 px-2 text-center">
                      <span v-if="anime.score && anime.score > 0" class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-cyan-500/20 text-cyan-400 font-bold">
                        {{ anime.score }}
                      </span>
                      <span v-else class="text-white/30">-</span>
                    </td>

                    <!-- Type -->
                    <td class="py-3 px-2">
                      <span class="text-white/60 text-xs uppercase">{{ anime.animeType || '-' }}</span>
                    </td>

                    <!-- Progress -->
                    <td class="py-3 px-2">
                      <div class="flex items-center gap-1">
                        <span class="text-white">{{ anime.episodes || 0 }}</span>
                        <span class="text-white/40">/</span>
                        <span class="text-white/60">{{ anime.totalEpisodes || '?' }}</span>
                      </div>
                      <div v-if="anime.totalEpisodes && anime.totalEpisodes > 0" class="mt-1 h-1 w-16 bg-white/10 rounded-full overflow-hidden">
                        <div
                          class="h-full bg-cyan-500 transition-all"
                          :style="{ width: `${Math.min(100, ((anime.episodes || 0) / anime.totalEpisodes) * 100)}%` }"
                        />
                      </div>
                    </td>

                    <!-- Tags -->
                    <td class="py-3 px-2 hidden md:table-cell">
                      <div v-if="anime.tags" class="flex flex-wrap gap-1 max-w-xs">
                        <span
                          v-for="tag in anime.tags.split(',').slice(0, 3)"
                          :key="tag"
                          class="px-2 py-0.5 rounded-full bg-white/10 text-white/60 text-xs"
                        >
                          {{ tag.trim() }}
                        </span>
                        <span v-if="anime.tags.split(',').length > 3" class="text-white/40 text-xs">
                          +{{ anime.tags.split(',').length - 3 }}
                        </span>
                      </div>
                      <span v-else class="text-white/30 text-xs">-</span>
                    </td>

                    <!-- Actions -->
                    <td class="py-3 pl-2">
                      <div class="flex items-center justify-center gap-1">
                        <div class="w-28">
                          <Select
                            :model-value="anime.listStatus"
                            :options="statusOptions"
                            size="xs"
                            @change="(val: string | number) => updateAnimeStatus(anime.id, String(val))"
                          />
                        </div>
                        <button
                          @click="removeFromList(anime.id)"
                          class="p-1.5 rounded text-white/40 hover:text-pink-400 hover:bg-pink-500/10 transition-colors"
                          title="Remove from list"
                        >
                          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                          </svg>
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <!-- Grid View (original) -->
            <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4">
              <div
                v-for="anime in filteredWatchlist"
                :key="anime.id"
                class="relative group"
              >
                <router-link :to="`/anime/${anime.id}`" class="block">
                  <div class="aspect-[2/3] rounded-xl overflow-hidden bg-surface flex items-center justify-center">
                    <img
                      v-if="anime.coverImage && anime.coverImage !== '/placeholder.svg'"
                      :src="anime.coverImage"
                      :alt="anime.title"
                      class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                      @error="(e: Event) => (e.target as HTMLImageElement).style.display = 'none'"
                    />
                    <div v-else class="flex flex-col items-center justify-center text-white/30 p-4">
                      <svg class="w-12 h-12 mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                      </svg>
                      <span class="text-xs text-center">{{ $t('profile.noPoster') }}</span>
                    </div>
                  </div>
                  <h3 class="mt-2 text-sm font-medium text-white line-clamp-2">{{ anime.title }}</h3>
                  <div v-if="anime.score && anime.score > 0" class="mt-1 flex items-center gap-1 text-xs text-white/60">
                    <svg class="w-3 h-3 text-yellow-400" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                    </svg>
                    {{ anime.score }}
                  </div>
                </router-link>

                <!-- Status dropdown -->
                <div class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity z-10" @click.stop>
                  <div class="w-28">
                    <Select
                      :model-value="anime.listStatus"
                      :options="statusOptions"
                      size="xs"
                      @change="(val: string | number) => updateAnimeStatus(anime.id, String(val))"
                    />
                  </div>
                </div>

                <!-- Remove button -->
                <button
                  @click="removeFromList(anime.id)"
                  class="absolute top-2 left-2 opacity-0 group-hover:opacity-100 transition-opacity w-6 h-6 rounded-full bg-pink-500/80 flex items-center justify-center text-white hover:bg-pink-500"
                  title="Remove from list"
                >
                  <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <div v-else class="text-center py-12">
            <p class="text-white/50 mb-4">{{ $t('profile.empty.watchlist') }}</p>
            <Button variant="outline" @click="$router.push('/browse')">
              {{ $t('profile.actions.browseCatalog') }}
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
                <p class="text-white/50 text-sm">{{ $t('profile.history.episode') }} {{ item.episode }}</p>
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
            <p class="text-white/50">{{ $t('profile.history.empty') }}</p>
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
                    <p class="text-white/50 text-sm">{{ $t('profile.settings.languageDesc') }}</p>
                  </div>
                  <div class="w-32">
                    <Select
                      v-model="settings.language"
                      :options="languageOptions"
                      size="sm"
                    />
                  </div>
                </div>
                <div class="flex items-center justify-between">
                  <div>
                    <p class="text-white">{{ $t('profile.settings.reduceMotion') }}</p>
                    <p class="text-white/50 text-sm">{{ $t('profile.settings.reduceMotionDesc') }}</p>
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
                  <div class="w-28">
                    <Select
                      v-model="settings.defaultQuality"
                      :options="qualityOptions"
                      size="sm"
                    />
                  </div>
                </div>
              </div>
            </div>

            <!-- Import -->
            <div class="glass-card p-6">
              <h3 class="text-lg font-semibold text-white mb-4">{{ $t('profile.import.title') }}</h3>
              <div class="space-y-4">
                <div>
                  <label class="block text-white/60 text-sm mb-2">MyAnimeList</label>
                  <div class="flex gap-2">
                    <input
                      v-model="malUsername"
                      type="text"
                      :placeholder="$t('profile.import.malPlaceholder')"
                      class="flex-1 bg-white/10 border border-white/10 rounded-lg px-4 py-2 text-white placeholder-white/40 focus:outline-none focus:border-cyan-500"
                      :disabled="malImporting"
                    />
                    <Button
                      variant="primary"
                      :disabled="!malUsername || malImporting"
                      @click="importMAL"
                    >
                      <svg v-if="malImporting" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      {{ malImporting ? $t('profile.import.importing') : $t('profile.import.import') }}
                    </Button>
                  </div>
                  <p class="text-white/40 text-xs mt-2">
                    {{ $t('profile.import.malDescription') }}
                  </p>
                  <div v-if="malImportResult" class="mt-3 p-3 rounded-lg" :class="malImportResult.errors?.length ? 'bg-amber-500/20' : 'bg-green-500/20'">
                    <p class="text-sm" :class="malImportResult.errors?.length ? 'text-amber-400' : 'text-green-400'">
                      {{ $t('profile.import.imported') }}: {{ malImportResult.imported }} | {{ $t('profile.import.skipped') }}: {{ malImportResult.skipped }}
                    </p>
                  </div>
                  <div v-if="malImportError" class="mt-3 p-3 rounded-lg bg-pink-500/20">
                    <p class="text-sm text-pink-400">{{ malImportError }}</p>
                  </div>
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
import { Badge, Button, Tabs, Select, type SelectOption } from '@/components/ui'
import { userApi } from '@/api/client'

interface WatchlistEntry {
  id: string
  user_id: string
  anime_id: string
  anime_title: string
  anime_cover: string
  status: string
  score: number
  episodes: number
  notes: string
  tags: string
  is_rewatching: boolean
  priority: string
  anime_type: string
  anime_total_episodes: number
  mal_id: number | null
  created_at: string
  updated_at: string
}

interface Anime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  totalEpisodes?: number
  genres?: string[]
  listStatus?: string
  score?: number
  animeType?: string
  tags?: string
  isRewatching?: boolean
  priority?: string
  notes?: string
  malId?: number | null
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
const { t, locale } = useI18n()
const authStore = useAuthStore()

const activeTab = ref('watchlist')
const watchlistFilter = ref('all')
const viewMode = ref<'table' | 'grid'>('table')
const loading = ref(false)

const tabs = computed(() => [
  { value: 'watchlist', label: t('profile.tabs.watchlist') },
  { value: 'history', label: t('profile.tabs.history') },
  { value: 'settings', label: t('profile.tabs.settings') },
])

const settings = reactive({
  language: locale.value,
  reduceMotion: false,
  autoplay: false,
  defaultQuality: 'auto',
})

const statusOptions = computed<SelectOption[]>(() => [
  { value: 'watching', label: t('profile.watchlist.watching') },
  { value: 'plan_to_watch', label: t('profile.watchlist.planToWatch') },
  { value: 'completed', label: t('profile.watchlist.completed') },
  { value: 'on_hold', label: t('profile.watchlist.onHold') },
  { value: 'dropped', label: t('profile.watchlist.dropped') },
])

const languageOptions: SelectOption[] = [
  { value: 'ru', label: 'Русский' },
  { value: 'ja', label: '日本語' },
  { value: 'en', label: 'English' },
]

const qualityOptions: SelectOption[] = [
  { value: 'auto', label: 'Auto' },
  { value: '1080p', label: '1080p' },
  { value: '720p', label: '720p' },
  { value: '480p', label: '480p' },
]

const watchlistEntries = ref<WatchlistEntry[]>([])
const watchlist = ref<Anime[]>([])
const history = ref<HistoryItem[]>([])

// MAL Import
const malUsername = ref('')
const malImporting = ref(false)
const malImportResult = ref<{ imported: number; skipped: number; errors?: string[] } | null>(null)
const malImportError = ref<string | null>(null)

const statusLabels = computed(() => ({
  all: t('profile.watchlist.all'),
  watching: t('profile.watchlist.watching'),
  plan_to_watch: t('profile.watchlist.planToWatch'),
  completed: t('profile.watchlist.completed'),
  on_hold: t('profile.watchlist.onHold'),
  dropped: t('profile.watchlist.dropped'),
}))

const watchlistFilters = computed(() => {
  const counts: Record<string, number> = {
    all: watchlistEntries.value.length,
    watching: 0,
    plan_to_watch: 0,
    completed: 0,
    on_hold: 0,
    dropped: 0,
  }

  watchlistEntries.value.forEach(entry => {
    if (counts[entry.status] !== undefined) {
      counts[entry.status]++
    }
  })

  return Object.entries(statusLabels.value).map(([value, label]) => ({
    value,
    label,
    count: counts[value] || 0,
  }))
})

const filteredWatchlist = computed(() => {
  if (watchlistFilter.value === 'all') return watchlist.value
  return watchlist.value.filter(a => a.listStatus === watchlistFilter.value)
})

const userInitials = computed(() => {
  const username = authStore.user?.username
  if (!username) return '?'
  return username.slice(0, 2).toUpperCase()
})

const fetchWatchlist = async () => {
  if (!authStore.isAuthenticated) return

  loading.value = true
  try {
    const response = await userApi.getWatchlist()
    const entries: WatchlistEntry[] = response.data?.data || response.data || []
    watchlistEntries.value = entries

    // Use stored anime data from watchlist entries
    const animeList: Anime[] = entries.map(entry => ({
      id: entry.anime_id,
      title: entry.anime_title || `Anime ${entry.anime_id}`,
      coverImage: entry.anime_cover || '',
      listStatus: entry.status,
      score: entry.score,
      episodes: entry.episodes,
      totalEpisodes: entry.anime_total_episodes,
      animeType: entry.anime_type,
      tags: entry.tags,
      isRewatching: entry.is_rewatching,
      priority: entry.priority,
      notes: entry.notes,
      malId: entry.mal_id,
    }))

    watchlist.value = animeList
  } catch (err) {
    console.error('Failed to fetch watchlist:', err)
  } finally {
    loading.value = false
  }
}

const updateAnimeStatus = async (animeId: string, newStatus: string) => {
  try {
    await userApi.updateWatchlistStatus(animeId, newStatus)
    // Update local state
    const entry = watchlistEntries.value.find(e => e.anime_id === animeId)
    if (entry) {
      entry.status = newStatus
    }
    const anime = watchlist.value.find(a => a.id === animeId)
    if (anime) {
      anime.listStatus = newStatus
    }
  } catch (err) {
    console.error('Failed to update status:', err)
  }
}

const removeFromList = async (animeId: string) => {
  try {
    await userApi.removeFromWatchlist(animeId)
    watchlistEntries.value = watchlistEntries.value.filter(e => e.anime_id !== animeId)
    watchlist.value = watchlist.value.filter(a => a.id !== animeId)
  } catch (err) {
    console.error('Failed to remove from list:', err)
  }
}

const importMAL = async () => {
  if (!malUsername.value) return

  malImporting.value = true
  malImportResult.value = null
  malImportError.value = null

  try {
    const response = await userApi.importMAL(malUsername.value)
    const data = response.data?.data || response.data
    malImportResult.value = data
    // Refresh watchlist after import
    await fetchWatchlist()
  } catch (err: any) {
    malImportError.value = err.response?.data?.message || 'Failed to import list'
  } finally {
    malImporting.value = false
  }
}

const logout = () => {
  authStore.logout()
  router.push('/')
}

onMounted(async () => {
  if (!authStore.user) {
    await authStore.fetchUser()
  }
  await fetchWatchlist()
})
</script>
