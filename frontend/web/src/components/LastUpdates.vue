<template>
  <div class="glass-card rounded-2xl p-5">
    <!-- Header with tabs -->
    <div class="flex items-center justify-between mb-5">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-purple-500 to-indigo-500 flex items-center justify-center">
          <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 20H5a2 2 0 01-2-2V6a2 2 0 012-2h10a2 2 0 012 2v1m2 13a2 2 0 01-2-2V7m2 13a2 2 0 002-2V9a2 2 0 00-2-2h-2m-4-3H9M7 16h6M7 8h6v4H7V8z" />
          </svg>
        </div>
        <h2 class="text-xl font-bold text-white">{{ $t('updates.title') }}</h2>
      </div>

      <!-- Tab switcher -->
      <div class="flex gap-1 bg-white/5 rounded-lg p-0.5">
        <button
          v-for="tab in tabs"
          :key="tab"
          @click="activeTab = tab"
          class="px-3 py-1 text-xs font-medium rounded-md transition-all"
          :class="activeTab === tab
            ? 'bg-white/10 text-white'
            : 'text-gray-400 hover:text-gray-300'"
        >
          {{ $t(`updates.${tab}`) }}
        </button>
      </div>
    </div>

    <!-- Changelog tab -->
    <div v-if="activeTab === 'changelog'" class="space-y-2 max-h-[600px] overflow-y-auto custom-scrollbar">
      <!-- Loading -->
      <div v-if="changelogLoading" class="space-y-3">
        <div v-for="i in 5" :key="i" class="animate-pulse flex gap-3 p-2">
          <div class="w-12 h-4 bg-white/10 rounded flex-shrink-0"></div>
          <div class="flex-1 space-y-2">
            <div class="h-4 bg-white/10 rounded w-3/4"></div>
          </div>
        </div>
      </div>

      <!-- Error -->
      <div v-else-if="changelogError" class="text-center py-8 text-gray-400">
        {{ $t('updates.error') }}
      </div>

      <!-- Content -->
      <template v-else>
        <div v-if="changelog.length === 0" class="text-center py-8 text-gray-400">
          {{ $t('updates.empty') }}
        </div>
        <div v-for="group in changelog" :key="group.date" class="mb-3">
          <div class="text-xs text-gray-500 font-medium mb-1.5 px-2">{{ formatDate(group.date) }}</div>
          <div
            v-for="(entry, idx) in group.entries"
            :key="idx"
            class="flex items-start gap-2.5 p-2 rounded-xl hover:bg-white/5 transition-colors"
          >
            <span
              class="text-[10px] font-bold uppercase px-1.5 py-0.5 rounded flex-shrink-0 mt-0.5"
              :class="typeBadgeClass(entry.type)"
            >
              {{ $t(`updates.${entry.type}`) }}
            </span>
            <p class="text-sm text-gray-300 line-clamp-2">{{ entry.message }}</p>
          </div>
        </div>
      </template>
    </div>

    <!-- News tab -->
    <div v-else class="space-y-2 max-h-[600px] overflow-y-auto custom-scrollbar">
      <!-- Loading -->
      <div v-if="newsLoading" class="space-y-3">
        <div v-for="i in 5" :key="i" class="animate-pulse p-2">
          <div class="flex gap-3">
            <div class="w-16 h-16 bg-white/10 rounded-lg flex-shrink-0"></div>
            <div class="flex-1 space-y-2">
              <div class="h-4 bg-white/10 rounded w-full"></div>
              <div class="h-4 bg-white/10 rounded w-2/3"></div>
              <div class="h-3 bg-white/10 rounded w-1/4"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- Error -->
      <div v-else-if="newsError" class="text-center py-8 text-gray-400">
        {{ $t('updates.error') }}
      </div>

      <!-- Content -->
      <template v-else>
        <div v-if="news.length === 0" class="text-center py-8 text-gray-400">
          {{ $t('updates.newsEmpty') }}
        </div>
        <a
          v-for="item in news"
          :key="item.id"
          :href="item.link"
          target="_blank"
          rel="noopener noreferrer"
          class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors group"
        >
          <img
            v-if="item.image_url"
            :src="item.image_url"
            alt=""
            class="w-16 h-16 rounded-lg object-cover flex-shrink-0"
            loading="lazy"
          />
          <div class="flex-1 min-w-0">
            <p class="text-sm text-white group-hover:text-purple-400 transition-colors line-clamp-3">
              {{ item.text }}
            </p>
            <p class="text-xs text-gray-500 mt-1">
              {{ formatRelativeTime(item.date) }}
              <span v-if="item.views" class="ml-2">{{ item.views }}</span>
            </p>
          </div>
        </a>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { animeApi } from '@/api/client'

interface ChangelogEntry {
  type: string
  message: string
}

interface ChangelogGroup {
  date: string
  entries: ChangelogEntry[]
}

interface NewsItem {
  id: string
  text: string
  image_url?: string
  date: string
  link: string
  views?: string
}

const CHANGELOG_CACHE_KEY = 'changelog'
const NEWS_CACHE_KEY = 'newsItems'
const CACHE_TTL = 5 * 60 * 1000

const tabs = ['changelog', 'news'] as const
type Tab = typeof tabs[number]

const { t, locale } = useI18n()
const activeTab = ref<Tab>('changelog')

const changelog = ref<ChangelogGroup[]>([])
const changelogLoading = ref(true)
const changelogError = ref(false)

const news = ref<NewsItem[]>([])
const newsLoading = ref(true)
const newsError = ref(false)

// --- Caching helpers ---
function getCached<T>(key: string): T | null {
  try {
    const raw = sessionStorage.getItem(key)
    if (!raw) return null
    const { data, ts } = JSON.parse(raw)
    if (Date.now() - ts > CACHE_TTL) {
      sessionStorage.removeItem(key)
      return null
    }
    return data
  } catch {
    return null
  }
}

function setCache<T>(key: string, data: T) {
  try {
    sessionStorage.setItem(key, JSON.stringify({ data, ts: Date.now() }))
  } catch { /* sessionStorage full or unavailable */ }
}

// --- Data fetching ---
async function fetchChangelog() {
  const cached = getCached<ChangelogGroup[]>(CHANGELOG_CACHE_KEY)
  if (cached) {
    changelog.value = cached
    changelogLoading.value = false
    return
  }

  try {
    const res = await fetch('/changelog.json')
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data: ChangelogGroup[] = await res.json()
    changelog.value = data
    setCache(CHANGELOG_CACHE_KEY, data)
  } catch {
    changelogError.value = true
  } finally {
    changelogLoading.value = false
  }
}

async function fetchNews() {
  const cached = getCached<NewsItem[]>(NEWS_CACHE_KEY)
  if (cached) {
    news.value = cached
    newsLoading.value = false
    return
  }

  try {
    const res = await animeApi.getNews()
    const items: NewsItem[] = res.data?.data || res.data || []
    news.value = items
    setCache(NEWS_CACHE_KEY, items)
  } catch {
    newsError.value = true
  } finally {
    newsLoading.value = false
  }
}

// --- Formatting ---
function typeBadgeClass(type: string): string {
  switch (type) {
    case 'feature': return 'bg-emerald-500/20 text-emerald-400'
    case 'fix': return 'bg-amber-500/20 text-amber-400'
    case 'perf': return 'bg-sky-500/20 text-sky-400'
    default: return 'bg-gray-500/20 text-gray-400'
  }
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return date.toLocaleDateString(loc, { day: 'numeric', month: 'short', year: 'numeric' })
}

function formatRelativeTime(dateStr: string): string {
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
  const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
  return date.toLocaleDateString(loc, { day: 'numeric', month: 'short' })
}

onMounted(() => {
  fetchChangelog()
  fetchNews()
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

.line-clamp-3 {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
