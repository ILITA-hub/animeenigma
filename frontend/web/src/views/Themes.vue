<template>
  <div class="min-h-screen pt-24 pb-16 px-4">
    <div class="max-w-4xl mx-auto">
      <!-- Header -->
      <div class="flex items-center justify-between mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">{{ $t('themes.title') }}</h1>
          <p class="text-white/50 text-sm mt-1">
            {{ seasonLabel }} {{ currentYear }}
          </p>
        </div>

        <!-- Admin sync button -->
        <button
          v-if="authStore.isAdmin"
          class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="syncing
            ? 'bg-yellow-500/20 text-yellow-400 cursor-wait'
            : 'bg-cyan-500/20 text-cyan-400 hover:bg-cyan-500/30'"
          :disabled="syncing"
          @click="triggerSync"
        >
          <svg v-if="syncing" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
          </svg>
          <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ syncing ? $t('themes.syncing', { progress: syncProgress }) : $t('themes.syncButton') }}
        </button>
      </div>

      <!-- Filters -->
      <div class="flex flex-wrap items-center gap-3 mb-6">
        <!-- Type filter -->
        <div class="flex rounded-lg overflow-hidden border border-white/10">
          <button
            v-for="opt in typeOptions"
            :key="opt.value"
            class="px-4 py-2 text-sm font-medium transition-colors"
            :class="typeFilter === opt.value
              ? 'bg-cyan-500/20 text-cyan-400'
              : 'text-white/50 hover:text-white hover:bg-white/5'"
            @click="typeFilter = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>

        <!-- Sort -->
        <select
          v-model="sortBy"
          class="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white/70 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
        >
          <option value="rating">{{ $t('themes.sortRating') }}</option>
          <option value="name">{{ $t('themes.sortName') }}</option>
          <option value="newest">{{ $t('themes.sortNewest') }}</option>
        </select>

        <!-- Season selector -->
        <select
          v-model="selectedSeason"
          class="bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white/70 focus:outline-none focus:ring-1 focus:ring-cyan-500/50"
        >
          <option v-for="s in seasonOptions" :key="s.value" :value="s.value">{{ s.label }}</option>
        </select>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex justify-center py-20">
        <svg class="w-10 h-10 animate-spin text-cyan-400" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
        </svg>
      </div>

      <!-- Error -->
      <div v-else-if="error" class="text-center py-20">
        <p class="text-white/60">{{ error }}</p>
        <button class="mt-4 text-cyan-400 hover:text-cyan-300" @click="fetchThemes">Retry</button>
      </div>

      <!-- Empty state -->
      <div v-else-if="themes.length === 0" class="text-center py-20">
        <svg class="w-16 h-16 mx-auto text-white/10 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
        </svg>
        <p class="text-white/40">{{ $t('themes.noThemes') }}</p>
        <p v-if="authStore.isAdmin" class="text-white/30 text-sm mt-2">{{ $t('themes.syncHint') }}</p>
      </div>

      <!-- Theme Cards Grid -->
      <div v-else class="space-y-3">
        <ThemeCard
          v-for="theme in themes"
          :key="theme.id"
          :theme="theme"
          @rate="(score) => rateTheme(theme.id, score)"
          @unrate="unrateTheme(theme.id)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { themesApi, adminThemesApi } from '@/api/client'
import ThemeCard from '@/components/themes/ThemeCard.vue'

const { t } = useI18n()
const authStore = useAuthStore()

interface Theme {
  id: string
  anime_name: string
  anime_slug: string
  anime_id?: string
  poster_url: string
  theme_type: string
  slug: string
  song_title: string
  artist_name: string
  video_basename: string
  avg_score: number
  vote_count: number
  user_score?: number | null
}

const loading = ref(false)
const error = ref<string | null>(null)
const themes = ref<Theme[]>([])
let componentActive = true
const syncing = ref(false)
const syncProcessed = ref(0)
const syncTotal = ref(0)
let syncPollInterval: ReturnType<typeof setInterval> | null = null

onBeforeUnmount(() => {
  componentActive = false
})

onUnmounted(() => {
  if (syncPollInterval) {
    clearInterval(syncPollInterval)
    syncPollInterval = null
  }
})

// Current season detection
const now = new Date()
const currentYear = now.getFullYear()
const currentMonth = now.getMonth() + 1
const currentSeasonName = currentMonth <= 3 ? 'winter' : currentMonth <= 6 ? 'spring' : currentMonth <= 9 ? 'summer' : 'fall'

// Filters
const typeFilter = ref('')
const sortBy = ref('rating')
const selectedSeason = ref(`${currentYear}-${currentSeasonName}`)

const typeOptions = [
  { value: '', label: 'All' },
  { value: 'OP', label: 'Openings' },
  { value: 'ED', label: 'Endings' },
]

const seasonOptions = computed(() => {
  const options = []
  const seasons = ['winter', 'spring', 'summer', 'fall']
  // Current year + previous year
  for (const y of [currentYear, currentYear - 1]) {
    for (const s of seasons) {
      options.push({
        value: `${y}-${s}`,
        label: `${s.charAt(0).toUpperCase() + s.slice(1)} ${y}`,
      })
    }
  }
  return options
})

const seasonLabel = computed(() => {
  const [, season] = selectedSeason.value.split('-')
  return season ? season.charAt(0).toUpperCase() + season.slice(1) : ''
})

const syncProgress = computed(() => {
  if (syncTotal.value > 0) return `${syncProcessed.value}/${syncTotal.value}`
  return '...'
})

const fetchThemes = async () => {
  loading.value = true
  error.value = null
  try {
    const [yearStr, season] = selectedSeason.value.split('-')
    const resp = await themesApi.list({
      year: parseInt(yearStr),
      season,
      type: typeFilter.value || undefined,
      sort: sortBy.value,
    })
    if (componentActive) themes.value = resp.data?.data || resp.data || []
  } catch (err: unknown) {
    if (componentActive) {
      error.value = t('themes.loadError')
      console.error('Failed to fetch themes:', err)
    }
  } finally {
    if (componentActive) loading.value = false
  }
}

const rateTheme = async (themeId: string, score: number) => {
  try {
    const theme = themes.value.find(t => t.id === themeId)
    if (!theme) return

    const oldScore = theme.user_score
    const wasRated = oldScore != null && oldScore > 0

    // Optimistic local update
    theme.user_score = score
    if (wasRated) {
      // Changing existing vote — recalculate avg
      theme.avg_score = theme.vote_count > 0
        ? (theme.avg_score * theme.vote_count - oldScore + score) / theme.vote_count
        : score
    } else {
      // New vote
      theme.avg_score = (theme.avg_score * theme.vote_count + score) / (theme.vote_count + 1)
      theme.vote_count += 1
    }

    await themesApi.rate(themeId, score)
  } catch (err) {
    console.error('Failed to rate theme:', err)
    await fetchThemes() // Rollback on error
  }
}

const unrateTheme = async (themeId: string) => {
  try {
    const theme = themes.value.find(t => t.id === themeId)
    if (!theme) return

    const oldScore = theme.user_score ?? 0

    // Optimistic local update
    theme.user_score = null
    if (theme.vote_count > 1) {
      theme.avg_score = (theme.avg_score * theme.vote_count - oldScore) / (theme.vote_count - 1)
    } else {
      theme.avg_score = 0
    }
    theme.vote_count = Math.max(0, theme.vote_count - 1)

    await themesApi.unrate(themeId)
  } catch (err) {
    console.error('Failed to unrate theme:', err)
    await fetchThemes() // Rollback on error
  }
}

const triggerSync = async () => {
  syncing.value = true
  syncProcessed.value = 0
  syncTotal.value = 0
  try {
    await adminThemesApi.sync()
    // Poll sync status
    syncPollInterval = setInterval(async () => {
      try {
        const resp = await adminThemesApi.syncStatus()
        const status = resp.data?.data || resp.data
        syncProcessed.value = status.processed || 0
        syncTotal.value = status.total || 0
        if (!status.running) {
          if (syncPollInterval) clearInterval(syncPollInterval)
          syncPollInterval = null
          syncing.value = false
          await fetchThemes()
        }
      } catch {
        if (syncPollInterval) clearInterval(syncPollInterval)
        syncPollInterval = null
        syncing.value = false
      }
    }, 1500)
  } catch (err) {
    syncing.value = false
    console.error('Failed to trigger sync:', err)
  }
}

// Watch filters and refetch
watch([typeFilter, sortBy, selectedSeason], () => {
  fetchThemes()
})

onMounted(() => {
  fetchThemes()
})
</script>
