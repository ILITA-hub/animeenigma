<template>
  <!-- Neon Tokyo .activity shell — mirrors ActivityFeed shell -->
  <div class="activity-shell flex flex-col relative">
    <!-- Header with tabs -->
    <div class="section-head">
      <h2 class="section-title">{{ $t('updates.title') }}</h2>

      <!-- Tab switcher -->
      <div class="tab-switcher">
        <button
          v-for="tab in tabs"
          :key="tab"
          @click="activeTab = tab"
          class="tab-btn"
          :class="{ 'tab-btn--active': activeTab === tab }"
        >
          {{ $t(`updates.${tab}`) }}
        </button>
      </div>
    </div>

    <!-- Changelog tab -->
    <div v-if="activeTab === 'changelog'" class="update-list custom-scrollbar flex-1 min-h-0 overflow-y-auto">
      <!-- Loading -->
      <div v-if="changelogLoading" class="update-list">
        <div v-for="i in 5" :key="i" class="update-skeleton">
          <div class="skeleton-badge" />
          <div class="skeleton-line" />
        </div>
      </div>

      <!-- Error -->
      <div v-else-if="changelogError" class="update-empty">
        {{ $t('updates.error') }}
      </div>

      <!-- Content -->
      <template v-else>
        <div v-if="limitedChangelog.length === 0" class="update-empty">
          {{ $t('updates.empty') }}
        </div>
        <div v-for="group in limitedChangelog" :key="group.date" class="changelog-group">
          <div class="changelog-date">{{ formatDate(group.date) }}</div>
          <button
            v-for="(entry, idx) in group.entries"
            :key="idx"
            type="button"
            @click="onEntryClick(group.date, idx)"
            :aria-expanded="isExpanded(group.date, idx)"
            class="update-row"
            :class="{ 'update-row--expanded': isExpanded(group.date, idx) }"
          >
            <span class="entry-badge" :class="typeBadgeClass(entry.type)">
              {{ $t(`updates.${entry.type}`) }}
            </span>
            <div
              class="entry-msg"
              :class="{ 'entry-msg--open': isExpanded(group.date, idx) }"
            >
              <div class="entry-msg__inner">
                <p class="entry-text">{{ entry.message }}</p>
              </div>
            </div>
            <svg
              class="entry-chevron"
              :class="{ 'entry-chevron--open': isExpanded(group.date, idx) }"
              fill="none" stroke="currentColor" viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
        </div>
      </template>
    </div>

    <!-- News tab — .update-row layout: thumbnail + body + timestamp -->
    <div v-else class="update-list custom-scrollbar flex-1 min-h-0 overflow-y-auto">
      <!-- Loading -->
      <div v-if="newsLoading" class="update-list">
        <div v-for="i in 5" :key="i" class="news-skeleton">
          <div class="skeleton-thumb" />
          <div class="skeleton-news-body">
            <div class="skeleton-line skel-w-full" />
            <div class="skeleton-line skel-w-2-3" />
            <div class="skeleton-line skel-w-1q" />
          </div>
        </div>
      </div>

      <!-- Error -->
      <div v-else-if="newsError" class="update-empty">
        {{ $t('updates.error') }}
      </div>

      <!-- Content -->
      <template v-else>
        <div v-if="news.length === 0" class="update-empty">
          {{ $t('updates.newsEmpty') }}
        </div>
        <!-- .update-row: poster-sm + body (title + sub) + when -->
        <a
          v-for="item in news"
          :key="item.id"
          :href="item.link"
          target="_blank"
          rel="noopener noreferrer"
          class="update-row"
        >
          <div
            class="update-thumb"
            :class="{ 'update-thumb--empty': !item.image_url }"
          >
            <img
              v-if="item.image_url"
              class="update-thumb-img"
              :src="item.image_url"
              alt=""
              loading="lazy"
            />
          </div>
          <div class="update-body">
            <div class="update-title">{{ item.text }}</div>
            <div class="update-sub" v-if="item.views">{{ item.views }}</div>
          </div>
          <div class="update-when">{{ formatRelativeTime(item.date) }}</div>
        </a>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
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

const MAX_CHANGELOG_ENTRIES = 20

const changelog = ref<ChangelogGroup[]>([])
const changelogLoading = ref(true)
const changelogError = ref(false)

const limitedChangelog = computed(() => {
  const result: ChangelogGroup[] = []
  let count = 0
  for (const group of changelog.value) {
    if (count >= MAX_CHANGELOG_ENTRIES) break
    const remaining = MAX_CHANGELOG_ENTRIES - count
    const entries = group.entries.slice(0, remaining)
    result.push({ date: group.date, entries })
    count += entries.length
  }
  return result
})

const news = ref<NewsItem[]>([])
const newsLoading = ref(true)
const newsError = ref(false)

const expandedKey = ref<string | null>(null)

function entryKey(date: string, idx: number): string {
  return `${date}-${idx}`
}

function isExpanded(date: string, idx: number): boolean {
  return expandedKey.value === entryKey(date, idx)
}

function toggleEntry(date: string, idx: number) {
  const key = entryKey(date, idx)
  expandedKey.value = expandedKey.value === key ? null : key
}

function onEntryClick(date: string, idx: number) {
  // Don't toggle if the user is selecting text inside the row
  const sel = window.getSelection()
  if (sel && sel.toString().length > 0) return
  toggleEntry(date, idx)
}

function handleEsc(e: KeyboardEvent) {
  if (e.key === 'Escape' && expandedKey.value) {
    expandedKey.value = null
  }
}

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
    case 'feature': return 'badge-feature'
    case 'fix': return 'badge-fix'
    case 'perf': return 'badge-perf'
    default: return 'badge-other'
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
  window.addEventListener('keydown', handleEsc)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleEsc)
})
</script>

<style scoped>
/* Neon Tokyo .activity shell */
.activity-shell {
  background: rgba(255, 255, 255, 0.025);
  border: 1px solid var(--line);
  border-radius: var(--r-xl);
  padding: 18px;
}

/* Section header */
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  gap: 12px;
}

.section-title {
  font-family: var(--f-display);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--ink);
  flex-shrink: 0;
}

/* Tab switcher */
.tab-switcher {
  display: flex;
  gap: 2px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: var(--r-sm);
  padding: 2px;
}

.tab-btn {
  padding: 4px 12px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 6px;
  color: var(--ink-3);
  transition: background 0.15s ease, color 0.15s ease;
  cursor: pointer;
}

.tab-btn:hover {
  color: var(--ink-2);
}

.tab-btn--active {
  background: rgba(255, 255, 255, 0.08);
  color: var(--ink);
}

/* Update list */
.update-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

/* .update-row from handoff — shared by changelog entries and news items */
.update-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px;
  border-radius: 10px;
  transition: background 0.15s ease;
  cursor: pointer;
  width: 100%;
  text-align: left;
  text-decoration: none;
  color: inherit;
  background: transparent;
  border: none;
}

.update-row:hover,
.update-row--expanded {
  background: rgba(255, 255, 255, 0.03);
}

/* News thumbnail — .poster-sm from handoff: 36×48px */
.update-thumb {
  position: relative;
  width: 36px;
  height: 48px;
  border-radius: 6px;
  flex-shrink: 0;
  overflow: hidden;
}

/* Actual image — fills the container cleanly; no CSS url() injection */
.update-thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.update-thumb--empty {
  background: linear-gradient(135deg, #0e7490 0%, #4c1d95 100%);
}

/* Body (title + sub) */
.update-body {
  flex: 1;
  min-width: 0;
}

.update-title {
  font-size: 13px;
  font-weight: 500;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  white-space: normal;
  color: var(--ink);
}

.update-sub {
  font-size: 11px;
  color: var(--ink-3);
  margin-top: 2px;
}

/* Right-aligned mono timestamp */
.update-when {
  font-size: 11px;
  color: var(--ink-4);
  font-family: var(--f-mono);
  flex-shrink: 0;
}

/* Empty + error states */
.update-empty {
  text-align: center;
  padding: 32px 0;
  color: var(--ink-4);
  font-size: 13px;
}

/* Changelog group */
.changelog-group {
  margin-bottom: 8px;
}

.changelog-date {
  font-size: 11px;
  color: var(--ink-4);
  font-weight: 500;
  margin-bottom: 4px;
  padding: 0 10px;
  font-family: var(--f-mono);
}

/* Changelog entry badge */
.entry-badge {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  padding: 2px 6px;
  border-radius: 4px;
  flex-shrink: 0;
  margin-top: 1px;
}

.badge-feature { background: rgba(16, 185, 129, 0.2); color: #34d399; }
.badge-fix { background: rgba(245, 158, 11, 0.2); color: #fbbf24; }
.badge-perf { background: rgba(14, 165, 233, 0.2); color: #38bdf8; }
.badge-other { background: rgba(107, 114, 128, 0.2); color: #9ca3af; }

/* Entry message — inline expand/collapse */
.entry-msg {
  display: grid;
  grid-template-rows: 2.5rem;
  transition: grid-template-rows 280ms cubic-bezier(0.4, 0, 0.2, 1);
  flex: 1;
  min-width: 0;
}

.entry-msg--open {
  grid-template-rows: 1fr;
}

.entry-msg__inner {
  overflow: hidden;
}

.entry-text {
  font-size: 13px;
  color: var(--ink-3);
  transition: color 0.15s ease;
  white-space: pre-wrap;
  word-break: break-words;
  user-select: text;
}

.update-row:hover .entry-text {
  color: var(--ink);
}

/* Chevron */
.entry-chevron {
  width: 16px;
  height: 16px;
  color: var(--ink-4);
  flex-shrink: 0;
  margin-top: 4px;
  transition: transform 200ms ease;
}

.entry-chevron--open {
  transform: rotate(180deg);
}

/* Loading skeletons */
.update-skeleton {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-badge {
  width: 48px;
  height: 18px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.skeleton-line {
  height: 12px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  flex: 1;
}

.news-skeleton {
  display: flex;
  gap: 12px;
  padding: 10px;
  animation: pulse 1.5s ease-in-out infinite;
}

.skeleton-thumb {
  width: 36px;
  height: 48px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.skeleton-news-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
  justify-content: center;
}

/* Skeleton-only width helpers — prefixed skel- to avoid shadowing Tailwind utilities */
.skel-w-full { width: 100%; }
.skel-w-2-3  { width: 66%; }
.skel-w-1q   { width: 25%; }

/* Scrollbar */
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

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
