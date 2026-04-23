<template>
  <ContextMenu
    :visible="visible"
    :x="x"
    :y="y"
    @update:visible="$emit('update:visible', $event)"
  >
    <div v-if="anime" class="p-3">
      <!-- Header: poster + info -->
      <div class="flex gap-3 mb-3">
        <img
          :src="anime.coverImage"
          :alt="localizedTitle"
          class="w-12 h-16 rounded object-cover flex-shrink-0"
          @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime!.coverImage) } }"
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
            <span v-if="anime.rating" class="flex items-center gap-0.5 text-xs text-amber-400">
              <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
              </svg>
              {{ anime.rating.toFixed(1) }}
            </span>
            <span v-if="siteRating && siteRating.total_reviews > 0" class="flex items-center gap-0.5 text-xs text-cyan-400">
              <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
              </svg>
              {{ siteRating.average_score.toFixed(1) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Auth-gated notice -->
      <p v-if="!authStore.isAuthenticated" class="text-xs text-white/30 px-2 py-2 border-t border-white/10">
        {{ $t('anime.loginToManage') }}
      </p>

      <!-- Menu items (single v-for so roving tabindex stays dense) -->
      <div class="border-t border-white/10 pt-2">
        <button
          v-for="(action, i) in actions"
          :key="action.key"
          :ref="(el: any) => setItemRef(el, i)"
          role="menuitem"
          :tabindex="focusedIndex === i ? 0 : -1"
          :class="itemClasses(action)"
          @click="activate(action)"
          @keydown="onItemKeydown"
        >
          <!-- icon -->
          <svg
            v-if="action.kind === 'status' && action.current"
            class="w-4 h-4 flex-shrink-0"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
              clip-rule="evenodd"
            />
          </svg>
          <span v-else-if="action.kind === 'status'" class="w-4 flex-shrink-0" />

          <svg
            v-else-if="action.kind === 'remove'"
            class="w-4 h-4 flex-shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
            />
          </svg>

          <svg
            v-else-if="action.kind === 'mark-next'"
            class="w-4 h-4 flex-shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>

          <svg
            v-else-if="action.kind === 'goto'"
            class="w-4 h-4 flex-shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
            />
          </svg>

          <svg
            v-else-if="action.kind === 'newtab'"
            class="w-4 h-4 flex-shrink-0"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M14 3h7v7m0-7L10 14m-4-4H4a1 1 0 00-1 1v9a1 1 0 001 1h9a1 1 0 001-1v-2"
            />
          </svg>

          {{ action.label }}
        </button>
      </div>
    </div>
  </ContextMenu>
</template>

<script setup lang="ts">
import { computed, ref, inject, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ContextMenu from '@/components/ui/ContextMenu.vue'
import { useAuthStore } from '@/stores/auth'
import { useWatchlistStore } from '@/stores/watchlist'
import { userApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageFallbackUrl } from '@/composables/useImageProxy'

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

type ActionKind = 'status' | 'remove' | 'mark-next' | 'goto' | 'newtab'

interface MenuAction {
  key: string
  kind: ActionKind
  label: string
  current?: boolean
  danger?: boolean
  onActivate: () => void | Promise<void>
}

const props = defineProps<{
  visible: boolean
  x: number
  y: number
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

// Provided by ContextMenu wrapper so we can mark close-reason='item' on activation.
const closeWithReason = inject<(reason: 'item') => void>('ctxMenuClose', () => emit('update:visible', false))

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
  if (authStore.isAuthenticated) {
    for (const s of statusOptions) {
      out.push({
        key: `status-${s.value}`,
        kind: 'status',
        label: t(s.i18nKey),
        current: props.listStatus === s.value,
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
  out.push({ key: 'goto', kind: 'goto', label: t('contextMenu.goToPage'), onActivate: goToPage })
  out.push({ key: 'newtab', kind: 'newtab', label: t('contextMenu.openInNewTab'), onActivate: openInNewTab })
  return out
})

function itemClasses(action: MenuAction) {
  const base = 'w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors text-left'
  if (action.kind === 'status' && action.current) {
    return `${base} bg-cyan-500/20 text-cyan-400`
  }
  if (action.kind === 'remove' || action.danger) {
    return `${base} text-pink-400 hover:bg-pink-500/10 mt-1`
  }
  return `${base} text-white/70 hover:bg-white/5 hover:text-white`
}

// --- Roving tabindex ---
const itemEls = ref<HTMLButtonElement[]>([])
const focusedIndex = ref(0)

function setItemRef(el: any, i: number) {
  if (el) itemEls.value[i] = el as HTMLButtonElement
}

function moveFocus(delta: number) {
  const items = itemEls.value.filter(Boolean)
  if (!items.length) return
  focusedIndex.value = (focusedIndex.value + delta + items.length) % items.length
  items[focusedIndex.value]?.focus()
}

function onItemKeydown(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      moveFocus(1)
      break
    case 'ArrowUp':
      e.preventDefault()
      moveFocus(-1)
      break
    case 'Home':
      e.preventDefault()
      focusedIndex.value = 0
      itemEls.value[0]?.focus()
      break
    case 'End': {
      e.preventDefault()
      const last = itemEls.value.filter(Boolean).length - 1
      focusedIndex.value = last
      itemEls.value[last]?.focus()
      break
    }
  }
}

watch(() => props.visible, (now) => {
  if (now) {
    focusedIndex.value = 0
  } else {
    itemEls.value = []
  }
})

// Reset roving index when the action set itself changes (e.g. listStatus flips
// while menu is open and the remove/mark-next items appear/disappear).
watch(() => actions.value.length, () => {
  if (focusedIndex.value >= actions.value.length) focusedIndex.value = 0
})

async function activate(action: MenuAction) {
  await action.onActivate()
}

async function setStatus(status: string) {
  if (!props.anime) return
  const animeId = String(props.anime.id)
  try {
    if (props.listStatus) {
      await userApi.updateWatchlistStatus(animeId, status)
    } else {
      await userApi.addToWatchlist(animeId, status)
    }
    watchlistStore.invalidate()
    await watchlistStore.fetchWatchlist(true)
    emit('statusChange', animeId, status)
  } catch (e) {
    console.error('Failed to update watchlist status:', e)
  }
  closeWithReason('item')
}

async function removeFromList() {
  if (!props.anime) return
  const animeId = String(props.anime.id)
  try {
    await userApi.removeFromWatchlist(animeId)
    watchlistStore.invalidate()
    await watchlistStore.fetchWatchlist(true)
    emit('removeFromList', animeId)
  } catch (e) {
    console.error('Failed to remove from watchlist:', e)
  }
  closeWithReason('item')
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
  closeWithReason('item')
}

function goToPage() {
  if (!props.anime) return
  router.push(`/anime/${props.anime.id}`)
  closeWithReason('item')
}

function openInNewTab() {
  if (!props.anime) return
  window.open(`/anime/${props.anime.id}`, '_blank', 'noopener,noreferrer')
  closeWithReason('item')
}
</script>
