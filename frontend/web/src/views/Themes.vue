<template>
  <div class="min-h-screen pt-4 pb-16 px-4">
    <div class="max-w-4xl mx-auto">
      <!-- Header -->
      <div class="flex items-center justify-between mb-6">
        <div>
          <h1 class="text-2xl font-semibold text-white">{{ $t('themes.title') }}</h1>
          <p class="text-white/50 text-sm mt-1">
            {{ seasonLabel }} {{ currentYear }}
          </p>
        </div>

        <!-- Admin sync button -->
        <!-- KEPT bespoke: borderless two-state soft-bg pill (idle cyan-soft / syncing warning-soft, no border). The closest variant `ghost` adds `border border-white/10` + a `bg-white/5` base that tailwind-merge can't strip cleanly, introducing a visible border diff. Behavior (@click, :disabled, inline svg/spinner, label) stays as-is. -->
        <button
          v-if="authStore.isAdmin"
          class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors"
          :class="syncing
            ? 'bg-warning/20 text-warning cursor-wait'
            : 'bg-cyan-500/20 text-cyan-400 hover:bg-cyan-500/30'"
          :disabled="syncing"
          @click="triggerSync"
        >
          <Spinner v-if="syncing" size="sm" tone="mono" />
          <RefreshCw v-else class="size-4" />
          {{ syncing ? $t('themes.syncing', { progress: syncProgress }) : $t('themes.syncButton') }}
        </button>
      </div>

      <!-- Filters -->
      <div class="flex flex-wrap items-center gap-3 mb-6">
        <!-- Type filter -->
        <!-- UA-075 (UX-12 Phase 5): ButtonGroup wraps the type-filter row. -->
        <ButtonGroup
          :label="$t('themes.typeFilterLabel')"
          container-class="flex rounded-lg overflow-hidden border border-white/10"
        >
          <!-- KEPT bespoke: segmented-control item inside <ButtonGroup> (overflow-hidden border container). Items must be square-cornered/seamless; every Button variant forces rounded-lg/rounded-xl, breaking the seamless segmented look. :aria-pressed + two-state bg preserved as-is. -->
          <button
            v-for="opt in typeOptions"
            :key="opt.value"
            class="px-4 py-2 text-sm font-medium transition-colors"
            :class="typeFilter === opt.value
              ? 'bg-cyan-500/20 text-cyan-400'
              : 'text-white/50 hover:text-white hover:bg-white/5'"
            :aria-pressed="typeFilter === opt.value"
            @click="typeFilter = opt.value"
          >
            {{ opt.label }}
          </button>
        </ButtonGroup>

        <!-- Sort -->
        <Select
          v-model="sortBy"
          size="sm"
          :options="sortOptions"
          :aria-label="$t('themes.sortAriaLabel')"
          trigger-class="w-auto"
        />

        <!-- Season selector -->
        <Select
          v-model="selectedSeason"
          size="sm"
          :options="seasonOptions"
          :aria-label="$t('themes.seasonAriaLabel')"
          trigger-class="w-auto"
        />
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex justify-center py-20">
        <Spinner size="lg" />
      </div>

      <!-- Error -->
      <div v-else-if="error" class="text-center py-20">
        <p class="text-white/60">{{ error }}</p>
        <!-- KEPT bespoke: bare text-link affordance (cyan text, no bg/border). Button has no text-only variant; `ghost`/`outline` would add a filled/bordered box — a visible diff. -->
        <button class="mt-4 text-cyan-400 hover:text-cyan-300" @click="fetchThemes">Retry</button>
      </div>

      <!-- Empty state -->
      <div v-else-if="themes.length === 0" class="text-center py-20">
        <Music class="size-16 mx-auto text-white/10 mb-4" />
        <p class="text-white/60">{{ $t('themes.noThemes') }}</p>
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
import { Music, RefreshCw } from 'lucide-vue-next'
import { ref, computed, watch, onMounted, onUnmounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { themesApi, adminThemesApi } from '@/api/client'
import { ButtonGroup, Select, Spinner } from '@/components/ui'
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

const sortOptions = computed(() => [
  { value: 'rating', label: t('themes.sortRating') },
  { value: 'name', label: t('themes.sortName') },
  { value: 'newest', label: t('themes.sortNewest') },
])

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
    if (componentActive) {
      // Strict Array.isArray check guards against API shape drift (e.g. backend
      // returning {data: null, success: true} on empty seasons — UA-024).
      const raw = resp.data?.data
      themes.value = Array.isArray(raw) ? raw : []
    }
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
