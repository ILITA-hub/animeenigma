<template>
  <div class="glass-card rounded-2xl p-5">
    <div class="flex items-center gap-3 mb-5">
      <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-green-500 to-teal-500 flex items-center justify-center">
        <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white">{{ $t('updates.title') }}</h2>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading" class="space-y-3">
      <div v-for="i in 5" :key="i" class="animate-pulse flex gap-3 p-2">
        <div class="w-16 h-4 bg-white/10 rounded flex-shrink-0"></div>
        <div class="flex-1 space-y-2">
          <div class="h-4 bg-white/10 rounded w-3/4"></div>
          <div class="h-3 bg-white/10 rounded w-1/3"></div>
        </div>
      </div>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="text-center py-8 text-gray-400">
      {{ $t('updates.error') }}
    </div>

    <!-- Commits list -->
    <div v-else class="space-y-2 max-h-[600px] overflow-y-auto custom-scrollbar">
      <a
        v-for="commit in commits"
        :key="commit.sha"
        :href="commit.url"
        target="_blank"
        rel="noopener noreferrer"
        class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors group"
      >
        <code class="text-xs text-cyan-400 font-mono flex-shrink-0 mt-0.5">{{ commit.shortSha }}</code>
        <div class="flex-1 min-w-0">
          <p class="text-sm text-white group-hover:text-purple-400 transition-colors line-clamp-1">
            {{ commit.message }}
          </p>
          <p class="text-xs text-gray-500 mt-1">
            {{ commit.author }} &middot; {{ formatRelativeTime(commit.date) }}
          </p>
        </div>
      </a>

      <!-- Empty state -->
      <div v-if="commits.length === 0" class="text-center py-8 text-gray-400">
        {{ $t('updates.empty') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

interface Commit {
  sha: string
  shortSha: string
  message: string
  author: string
  date: string
  url: string
}

const CACHE_KEY = 'lastUpdates'
const CACHE_TTL = 5 * 60 * 1000 // 5 minutes

const { t, locale } = useI18n()
const commits = ref<Commit[]>([])
const loading = ref(true)
const error = ref(false)

const getCached = (): Commit[] | null => {
  try {
    const raw = sessionStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const { data, ts } = JSON.parse(raw)
    if (Date.now() - ts > CACHE_TTL) {
      sessionStorage.removeItem(CACHE_KEY)
      return null
    }
    return data
  } catch {
    return null
  }
}

const setCache = (data: Commit[]) => {
  try {
    sessionStorage.setItem(CACHE_KEY, JSON.stringify({ data, ts: Date.now() }))
  } catch {
    // sessionStorage full or unavailable
  }
}

const fetchCommits = async () => {
  const cached = getCached()
  if (cached) {
    commits.value = cached
    loading.value = false
    return
  }

  try {
    const res = await fetch('https://api.github.com/repos/ILITA-hub/animeenigma/commits?per_page=10')
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const parsed: Commit[] = data.map((item: any) => ({
      sha: item.sha,
      shortSha: item.sha.slice(0, 7),
      message: (item.commit?.message || '').split('\n')[0],
      author: item.commit?.author?.name || item.author?.login || 'Unknown',
      date: item.commit?.author?.date || '',
      url: item.html_url,
    }))
    commits.value = parsed
    setCache(parsed)
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
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
  fetchCommits()
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

.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
