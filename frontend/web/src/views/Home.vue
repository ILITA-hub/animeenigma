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
      <div class="flex items-center gap-3 relative z-40">
        <div class="flex-1">
          <SearchAutocomplete
            v-model="searchQuery"
            listbox-id="home-search"
            @submit="goToSearch"
          />
        </div>
        <router-link
          to="/schedule"
          :aria-label="$t('nav.scheduleLink')"
          class="flex items-center gap-2 px-4 py-4 rounded-xl bg-cyan-500/10 backdrop-blur-xl border border-cyan-500/20 text-cyan-400 hover:bg-cyan-500/20 transition-colors"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
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
          <ColumnItem
            v-for="anime in ongoingAnime"
            :key="anime.id"
            :anime="anime"
            variant="ongoing"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            :site-rating="siteRatings[anime.id] ?? null"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <div v-if="ongoingAnime.length === 0" class="text-center py-8 text-gray-400">
            {{ $t('home.noOngoing') }}
          </div>
        </HomeColumn>

        <!-- Top Anime Column -->
        <HomeColumn
          :title="$t('home.topAnime')"
          :sub="$t('home.topAnimeSub')"
          icon-tone="gold"
          see-all-to="/browse?sort=rating"
          :loading="loadingTop"
        >
          <ColumnItem
            v-for="(anime, index) in topAnime"
            :key="anime.id"
            :anime="anime"
            variant="top"
            :rank="index + 1"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            :site-rating="siteRatings[anime.id] ?? null"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <div v-if="topAnime.length === 0" class="text-center py-8 text-gray-400">
            {{ $t('home.noData') }}
          </div>
        </HomeColumn>

        <!-- Announcements Column -->
        <HomeColumn
          :title="$t('home.announcements')"
          icon-tone="blue"
          see-all-to="/browse?status=announced"
          :loading="loadingAnnounced"
        >
          <ColumnItem
            v-for="anime in announcedAnime"
            :key="anime.id"
            :anime="anime"
            variant="announced"
            :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
            @open-menu="(el) => openHomeMenuAt(el, anime)"
            @touchstart="(e) => onHomeTouchstart(e, anime)"
            @touchmove="onHomeTouchmove"
            @touchend="onHomeTouchend"
          />
          <div v-if="announcedAnime.length === 0" class="text-center py-8 text-gray-400">
            {{ $t('home.noAnnounced') }}
          </div>
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
    :anime="contextMenu.anime"
    :list-status="contextMenu.listStatus"
    :site-rating="contextMenu.siteRating"
    @update:visible="contextMenu.visible = $event"
    @status-change="watchlistStore.fetchStatuses(true)"
    @remove-from-list="watchlistStore.fetchStatuses(true)"
  />
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { getLocalizedTitle } from '@/utils/title'
import { useHomeStore } from '@/stores/home'
import { useWatchlistStore } from '@/stores/watchlist'
import { useContextMenu } from '@/composables/useContextMenu'
import { SearchAutocomplete } from '@/components/ui'
import { AnimeContextMenu } from '@/components/anime'
import ActivityFeed from '@/components/ActivityFeed.vue'
import LastUpdates from '@/components/LastUpdates.vue'
import ContinueWatchingRow from '@/components/home/ContinueWatchingRow.vue'
// Phase 17 (UX-33) — admin-curated editorial collections home row.
import CollectionsRow from '@/components/home/CollectionsRow.vue'
import SystemStatusBanner from '@/components/home/SystemStatusBanner.vue'
import HeroSpotlightBlock from '@/components/home/spotlight/HeroSpotlightBlock.vue'
import HomeColumn from '@/components/home/HomeColumn.vue'
import ColumnItem from '@/components/home/ColumnItem.vue'

interface HomeAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  episodes_count?: number
  episodes_aired?: number // Phase 9 (UX-17) — used to construct ?episode={N+1}
  next_episode_at?: string // Phase 9 (UX-17) — gates the episode-aware URL
  year?: number
  status?: string
}

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
