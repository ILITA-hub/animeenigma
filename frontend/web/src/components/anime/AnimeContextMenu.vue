<template>
  <DropdownMenu
    :open="visible"
    :anchor-point="{ x, y }"
    align="start"
    side="right"
    :side-offset="4"
    @update:open="$emit('update:visible', $event)"
  >
    <div v-if="anime" class="p-3">
      <!-- Header: poster + info -->
      <div class="flex gap-3 mb-3">
        <!-- w=384 matches the card surfaces (PosterCard / WatchlistRow), so the
             menu thumb is a browser-cache hit instead of a full-size download. -->
        <PosterImage
          :src="anime.coverImage"
          :alt="localizedTitle"
          ratio="2/3"
          rounded="sm"
          :proxy-width="384"
          class="w-12 flex-shrink-0"
        />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-white line-clamp-2 leading-tight">{{ localizedTitle }}</p>
          <p class="text-xs text-white/40 mt-0.5">
            <span v-if="anime.releaseYear">{{ anime.releaseYear }}</span>
            <span v-if="anime.releaseYear && anime.episodes" class="mx-1">·</span>
            <span v-if="anime.episodes">{{ anime.episodes }} {{ $t('anime.episode') }}</span>
          </p>
          <!-- Scores -->
          <div class="flex items-center gap-2 mt-1">
            <span v-if="anime.rating" class="flex items-center gap-0.5 text-xs text-warning">
              <Star class="size-3" fill="currentColor" aria-hidden="true" />
              {{ anime.rating.toFixed(1) }}
            </span>
            <span v-if="siteRating && siteRating.total_reviews > 0" class="flex items-center gap-0.5 text-xs text-cyan-400">
              <ScoreDiamond class="size-3" />
              {{ siteRating.average_score.toFixed(1) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Auth-gated notice -->
      <p v-if="!authStore.isAuthenticated" class="text-xs text-white/30 px-2 py-2 border-t border-white/10">
        {{ $t('anime.loginToManage') }}
      </p>

      <!-- Menu items — Reka DropdownMenuItem provides roving focus + keyboard nav. -->
      <div class="border-t border-white/10 pt-2">
        <template v-for="action in actions" :key="action.key">
          <div v-if="action.dividerBefore" class="border-t border-white/10 my-1" aria-hidden="true" />
          <DropdownMenuItem
            :class="itemClasses(action)"
            @select="activate(action)"
          >
            <!-- icon -->
            <Check
              v-if="action.kind === 'status' && action.current"
              class="size-4 flex-shrink-0"
            />
            <span v-else-if="action.kind === 'status'" class="w-4 flex-shrink-0" />

            <Trash2
              v-else-if="action.kind === 'remove'"
              class="size-4 flex-shrink-0"
            />

            <Check
              v-else-if="action.kind === 'mark-next'"
              class="size-4 flex-shrink-0"
            />

            <ExternalLink
              v-else-if="action.kind === 'goto'"
              class="size-4 flex-shrink-0"
            />

            <ExternalLink
              v-else-if="action.kind === 'newtab'"
              class="size-4 flex-shrink-0"
            />

            <Download
              v-else-if="action.kind === 'download-season'"
              class="size-4 flex-shrink-0"
            />

            {{ action.label }}
          </DropdownMenuItem>
        </template>
      </div>
    </div>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ReferenceElement } from 'reka-ui'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Check, Trash2, ExternalLink, Star, Download } from 'lucide-vue-next'
import { DropdownMenu, DropdownMenuItem, ScoreDiamond } from '@/components/ui'
import PosterImage from '@/components/anime/PosterImage.vue'
import { useAuthStore } from '@/stores/auth'
import { useWatchlistStore } from '@/stores/watchlist'
import { userApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import { getLocalizedTitle } from '@/utils/title'
import { offlineDownloadsEnabled } from '@/offline/flag'
import { openSeasonDownload } from '@/offline/seasonDownloadFlow'
import { useStandaloneDisplay, installHintKey } from '@/pwa/standalone'

interface Anime {
  id: string | number
  title: string
  name?: string
  nameRu?: string
  nameJp?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  status?: string
  genres?: string[]
}

type ActionKind = 'status' | 'remove' | 'mark-next' | 'goto' | 'newtab' | 'download-season'

interface MenuAction {
  key: string
  kind: ActionKind
  label: string
  current?: boolean
  danger?: boolean
  dividerBefore?: boolean
  onActivate: () => void | Promise<void>
}

const props = defineProps<{
  visible: boolean
  // x/y retained for back-compat with the existing view bindings; positioning
  // now flows through anchorEl (Reka anchored mode).
  x: number
  y: number
  anchorEl?: ReferenceElement | null
  anime: Anime | null
  listStatus: string | null
  siteRating?: { average_score: number; total_reviews: number } | null
  episodesWatched?: number | null
  episodesTotal?: number | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'statusChange': [animeId: string, status: string]
  'removeFromList': [animeId: string]
  'episodesChange': [animeId: string, episodes: number]
}>()

const router = useRouter()
const { t, locale } = useI18n()
const authStore = useAuthStore()
const watchlistStore = useWatchlistStore()
const toast = useToast()
const isStandalone = useStandaloneDisplay()

// Close the anchored DropdownMenu after an action (replaces the old
// ContextMenu `closeWithReason('item')` provide/inject contract).
function closeMenu() {
  emit('update:visible', false)
}

const localizedTitle = computed(() => {
  if (!props.anime) return ''
  if (props.anime.name || props.anime.nameRu || props.anime.nameJp) {
    void locale.value
    return getLocalizedTitle(props.anime.name, props.anime.nameRu, props.anime.nameJp)
  }
  return props.anime.title
})

const statusOptions = [
  { value: 'watching', i18nKey: 'profile.watchlist.watching' },
  { value: 'plan_to_watch', i18nKey: 'profile.watchlist.planToWatch' },
  { value: 'completed', i18nKey: 'profile.watchlist.completed' },
  { value: 'on_hold', i18nKey: 'profile.watchlist.onHold' },
  { value: 'dropped', i18nKey: 'profile.watchlist.dropped' },
]

const actions = computed<MenuAction[]>(() => {
  const out: MenuAction[] = []
  // C-top: open-in-new-tab pinned first, its own navigation group.
  out.push({ key: 'newtab', kind: 'newtab', label: t('contextMenu.openInNewTab'), onActivate: openInNewTab })
  out.push({ key: 'goto', kind: 'goto', label: t('contextMenu.goToPage'), onActivate: goToPage })
  if (offlineDownloadsEnabled) {
    out.push({
      key: 'download-season',
      kind: 'download-season',
      // Downloads are app-only: from a browser tab the item points at the app.
      label: isStandalone.value ? t('contextMenu.downloadSeason') : t('downloads.inAppOnly'),
      onActivate: downloadSeason,
    })
  }
  if (authStore.isAuthenticated) {
    for (const [i, s] of statusOptions.entries()) {
      out.push({
        key: `status-${s.value}`,
        kind: 'status',
        label: t(s.i18nKey),
        current: props.listStatus === s.value,
        dividerBefore: i === 0,
        onActivate: () => setStatus(s.value),
      })
    }
    if (props.listStatus) {
      out.push({
        key: 'remove',
        kind: 'remove',
        label: t('profile.actions.removeFromList'),
        danger: true,
        onActivate: removeFromList,
      })
    }
    // Replaces the removed Profile-grid `+` button. Visible only when the
    // user is actively watching and there is a next episode to mark.
    const ep = props.episodesWatched ?? 0
    const total = props.episodesTotal ?? 0
    if (props.listStatus === 'watching' && (total === 0 || ep < total)) {
      out.push({
        key: 'mark-next',
        kind: 'mark-next',
        label: t('contextMenu.markNextWatched', { n: ep + 1 }),
        onActivate: markNextWatched,
      })
    }
  }
  return out
})

function itemClasses(action: MenuAction) {
  const base = 'w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors text-left cursor-pointer outline-none data-[highlighted]:bg-white/5'
  if (action.kind === 'status' && action.current) {
    return `${base} bg-cyan-500/20 text-cyan-400`
  }
  if (action.kind === 'remove' || action.danger) {
    return `${base} text-pink-400 hover:bg-pink-500/10 data-[highlighted]:bg-pink-500/10 mt-1`
  }
  return `${base} text-white/70 hover:bg-white/5 hover:text-white data-[highlighted]:text-white`
}

async function activate(action: MenuAction) {
  await action.onActivate()
}

async function setStatus(status: string) {
  if (!props.anime) return
  const animeId = String(props.anime.id)
  // Optimistic: mutate store first so any subscribed UI flips immediately.
  // Emit before the await so callers (cards/grids) sync their local mirrors.
  emit('statusChange', animeId, status)
  closeMenu()
  try {
    await watchlistStore.setStatusOptimistic(animeId, status)
  } catch (e) {
    console.error('Failed to update watchlist status:', e)
    toast.push(t('watchlist.errors.updateFailed'))
  }
}

async function removeFromList() {
  if (!props.anime) return
  const animeId = String(props.anime.id)
  // Optimistic: drop locally + emit so parent grids re-render before the
  // network round-trip resolves.
  emit('removeFromList', animeId)
  closeMenu()
  try {
    await watchlistStore.removeEntryOptimistic(animeId)
  } catch (e) {
    console.error('Failed to remove from watchlist:', e)
    toast.push(t('watchlist.errors.removeFailed'))
  }
}

async function markNextWatched() {
  if (!props.anime || !props.listStatus) return
  const animeId = String(props.anime.id)
  const next = (props.episodesWatched ?? 0) + 1
  try {
    await userApi.updateWatchlistEntry({
      anime_id: animeId,
      status: props.listStatus,
      episodes: next,
    })
    watchlistStore.invalidate()
    await watchlistStore.fetchWatchlist(true)
    emit('episodesChange', animeId, next)
  } catch (e) {
    console.error('Failed to mark next episode:', e)
  }
  closeMenu()
}

function goToPage() {
  if (!props.anime) return
  router.push(`/anime/${props.anime.id}`)
  closeMenu()
}

function openInNewTab() {
  if (!props.anime) return
  window.open(`/anime/${props.anime.id}`, '_blank', 'noopener,noreferrer')
  closeMenu()
}

function downloadSeason() {
  if (!props.anime) return
  closeMenu()
  if (!isStandalone.value) {
    toast.push(t(installHintKey()), 'info', 6000)
    return
  }
  const req = {
    animeId: String(props.anime.id),
    title: localizedTitle.value,
    poster: props.anime.coverImage,
  }
  // Fire-and-forget: the global <SeasonDownloadHost /> renders the flow's
  // dialog/toasts; this menu is already closed by then.
  void openSeasonDownload(req, locale.value)
}
</script>
