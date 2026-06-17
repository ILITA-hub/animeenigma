<template>
  <div class="min-h-screen">
    <!-- Phase 11 / UX-24 — System status banner. Permanent mount at the
         very top; renders nothing when no incident is active (gateway env
         SYSTEM_BANNER_ACTIVE=false, the default) or when the user has
         dismissed the current incident. Above <h1 sr-only> so screen
         readers see the alert before the page title. -->
    <SystemStatusBanner />

    <!-- Search Bar -->
    <h1 class="sr-only">AnimeEnigma</h1>
    <div class="pt-24 px-4 lg:px-8 max-w-7xl mx-auto mb-8">
      <div class="search-row relative z-40">
        <!-- Search wrapper — Neon Tokyo .search shell wrapping the SearchAutocomplete -->
        <div class="search-shell">
          <SearchAutocomplete
            v-model="searchQuery"
            listbox-id="home-search"
            @submit="goToSearch"
          />
          <!-- OS-aware shortcut hint: ⌘K on Mac, Ctrl K everywhere else -->
          <kbd class="search-kbd hidden sm:inline-flex" aria-hidden="true">{{ searchHint }}</kbd>
        </div>
        <!-- Schedule button — .btn-ghost-accent -->
        <router-link
          to="/schedule"
          :aria-label="$t('nav.scheduleLink')"
          class="btn-ghost-accent"
        >
          <Calendar class="size-5 flex-shrink-0" aria-hidden="true" />
          <span class="hidden sm:inline">{{ $t('nav.schedule') }}</span>
        </router-link>
      </div>
    </div>

    <!-- Phase 2 (HSB-FE-01) — HeroSpotlightBlock. Mounted between the
         search bar and the 3-column grid, per design doc §2. Self-gated on
         flag + cards.length > 0 + non-error state; silent self-hide
         otherwise. -->
    <HeroSpotlightBlock />

    <!-- Continue-Watching row (Phase 8 / UX-15 / UA-061). Promoted above
         the 3-column grid (Neon Tokyo redesign Task 6) so in-progress
         content appears at the top for logged-in users. Hidden when
         anonymous OR when the logged-in user has no in-progress
         watch_progress rows. The component owns its own v-if gate so
         we just always mount it here. -->
    <ContinueWatchingRow />

    <!-- Recommendations rail (notebook 12:04:05). "Trending now" for anonymous,
         "Up Next for you" once logged in. Self-gating: hidden when there are no
         recs, so it sits quietly below Continue-Watching. -->
    <RecsRow />

    <!-- Three Columns Layout -->
    <div class="px-4 lg:px-8 max-w-7xl mx-auto mb-6">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">

        <!-- Ongoing Column -->
        <HomeColumn
          :title="$t('home.ongoing')"
          :sub="ongoingUpdatedAt && !loadingOngoing ? $t('home.updated', { time: formatUpdatedAt(ongoingUpdatedAt) }) : undefined"
          icon-tone="green"
          see-all-to="/browse?status=ongoing"
          :loading="loadingOngoing"
        >
          <PosterRow
            v-for="anime in ongoingAnime"
            :key="anime.id"
            :model="homeCardModel(anime)"
            variant="ongoing"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e: TouchEvent) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <EmptyState v-if="ongoingAnime.length === 0" size="sm">
            {{ $t('home.noOngoing') }}
          </EmptyState>
        </HomeColumn>

        <!-- Top Anime Column -->
        <HomeColumn
          :title="$t('home.topAnime')"
          :sub="$t('home.topAnimeSub')"
          icon-tone="gold"
          see-all-to="/browse?sort=rating"
          :loading="loadingTop"
        >
          <PosterRow
            v-for="(anime, index) in topAnime"
            :key="anime.id"
            :model="homeCardModel(anime)"
            variant="top"
            :rank="index + 1"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e: TouchEvent) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <EmptyState v-if="topAnime.length === 0" size="sm">
            {{ $t('home.noData') }}
          </EmptyState>
        </HomeColumn>

        <!-- Announcements Column -->
        <HomeColumn
          :title="$t('home.announcements')"
          :sub="$t('home.announcementsSub')"
          icon-tone="blue"
          see-all-to="/browse?status=announced"
          :loading="loadingAnnounced"
        >
          <PosterRow
            v-for="anime in announcedAnime"
            :key="anime.id"
            :model="fromHomeAnime(anime)"
            variant="announced"
            :season="anime.season"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e: TouchEvent) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <EmptyState v-if="announcedAnime.length === 0" size="sm">
            {{ $t('home.noAnnounced') }}
          </EmptyState>
        </HomeColumn>

      </div>
    </div>

    <!-- Phase 17 (UX-33) — admin-curated editorial collections. Self-gated
         on items.length === 0 so the row hides entirely when no
         collections have been published yet. -->
    <CollectionsRow />

    <!-- Activity Feed + Last Updates -->
    <div id="changelog" class="px-4 lg:px-8 max-w-7xl mx-auto pb-12 scroll-mt-24">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <ActivityFeed />
        <LastUpdates />
      </div>
    </div>
  </div>

  <!-- Context menu for the three home columns -->
  <AnimeContextMenu
    :visible="contextMenu.visible"
    :x="contextMenu.x"
    :y="contextMenu.y"
    :anchor-el="contextMenu.anchorEl"
    :anime="contextMenu.anime"
    :list-status="contextMenu.listStatus"
    :site-rating="contextMenu.siteRating"
    @update:visible="contextMenu.visible = $event"
    @status-change="watchlistStore.fetchStatuses(true)"
    @remove-from-list="watchlistStore.fetchStatuses(true)"
  />
</template>

<script setup lang="ts">
import { Calendar } from 'lucide-vue-next'
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { getLocalizedTitle } from '@/utils/title'
import { useHomeStore } from '@/stores/home'
import { useWatchlistStore } from '@/stores/watchlist'
import { useContextMenu } from '@/composables/useContextMenu'
import { SearchAutocomplete, EmptyState } from '@/components/ui'
import { AnimeContextMenu } from '@/components/anime'
import ActivityFeed from '@/components/ActivityFeed.vue'
import LastUpdates from '@/components/LastUpdates.vue'
import ContinueWatchingRow from '@/components/home/ContinueWatchingRow.vue'
import RecsRow from '@/components/home/RecsRow.vue'
// Phase 17 (UX-33) — admin-curated editorial collections home row.
import CollectionsRow from '@/components/home/CollectionsRow.vue'
import SystemStatusBanner from '@/components/home/SystemStatusBanner.vue'
import HeroSpotlightBlock from '@/components/home/spotlight/HeroSpotlightBlock.vue'
import HomeColumn from '@/components/home/HomeColumn.vue'
import { PosterRow } from '@/components/anime'
import { fromHomeAnime } from '@/utils/toCardModel'
import type { HomeAnime } from '@/stores/home'

const router = useRouter()
const { t } = useI18n()
const homeStore = useHomeStore()
const watchlistStore = useWatchlistStore()

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

const {
  contextMenu,
  openAtElement: openHomeCtx,
  onTouchstart: onHomeCtxTouchstart,
  onTouchmove: onHomeTouchmove,
  onTouchend: onHomeTouchend,
} = useContextMenu()

function ctxAnimeFromHome(anime: HomeAnime) {
  return {
    id: anime.id,
    title: getLocalizedTitle(anime.name, anime.name_ru, anime.name_jp) || 'Anime',
    name: anime.name,
    nameRu: anime.name_ru,
    nameJp: anime.name_jp,
    coverImage: anime.poster_url || '',
    rating: anime.score,
    releaseYear: anime.year,
    episodes: anime.episodes_count,
    status: anime.status,
  }
}

function homeCardModel(anime: HomeAnime) {
  const sr = siteRatings.value[anime.id]
  return fromHomeAnime(anime, {
    siteScore: sr && sr.total_reviews > 0 ? sr.average_score : undefined,
  })
}

function homeOpts(anime: HomeAnime) {
  return {
    listStatus: watchlistStore.getStatus(anime.id),
    siteRating: siteRatings.value[anime.id] ?? null,
  }
}

function openHomeMenuAt(el: HTMLElement, anime: HomeAnime) {
  openHomeCtx(el, ctxAnimeFromHome(anime), homeOpts(anime))
}

function onHomeTouchstart(event: TouchEvent, anime: HomeAnime) {
  onHomeCtxTouchstart(event, ctxAnimeFromHome(anime), homeOpts(anime))
}

const searchHint = computed(() =>
  /Mac|iPhone|iPod|iPad/.test(navigator.platform) ? '⌘K' : 'Ctrl K'
)

const goToSearch = () => {
  if (searchQuery.value.trim()) {
    router.push({ path: '/browse', query: { q: searchQuery.value.trim() } })
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
  // Best-effort load of watchlist statuses so the kebab menu can pre-select
  // the user's status. Anonymous users are no-op'd by the store.
  watchlistStore.fetchStatuses()
})
</script>

<style scoped>
/* Neon Tokyo search row — grid 1fr auto, gap 12px */
.search-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 12px;
  align-items: stretch;
}

/* Shell around SearchAutocomplete — .search look from handoff */
.search-shell {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  height: 56px;
  background: var(--white-a4);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  color: var(--muted-foreground);
  transition: border 0.15s ease, background 0.15s ease;
  /* overflow: hidden removed — it clipped the inner input's focus ring;
     the dropdown is absolutely-positioned so it is unaffected */
}

.search-shell:hover,
.search-shell:focus-within {
  border-color: var(--accent-line);
  background: var(--white-a4);
}

/* Allow SearchAutocomplete to flex-fill the shell */
.search-shell :deep(> *:first-child) {
  flex: 1;
  min-width: 0;
}

/* Merge inner <input> visually into the shell so there is no nested border/
   background peeking through; the shell's focus-within border-color change
   remains the only visible focus affordance. */
.search-shell :deep(input) {
  background: transparent;
  border: none;
  box-shadow: none;
}

.search-shell :deep(input:focus) {
  box-shadow: none;
  outline: none;
}

/* ⌘K decorative kbd hint */
.search-kbd {
  font-family: var(--font-mono);
  font-size: 11px;
  padding: 3px 6px;
  border-radius: 4px;
  border: 1px solid var(--line-strong);
  color: var(--muted-foreground);
  flex-shrink: 0;
  white-space: nowrap;
}

/* Schedule link — .btn-ghost-accent from handoff */
.btn-ghost-accent {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 56px;
  padding: 0 20px;
  background: var(--accent-soft);
  border: 1px solid var(--accent-line);
  border-radius: var(--r-lg);
  color: var(--brand-cyan);
  font-size: 14px;
  font-weight: 600;
  transition: background 0.15s ease;
  white-space: nowrap;
  text-decoration: none;
}

.btn-ghost-accent:hover {
  background: var(--cyan-a20);
}

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 2px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--white-a20);
}
</style>
