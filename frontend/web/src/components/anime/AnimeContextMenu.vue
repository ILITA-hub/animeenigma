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

      <!-- Status Actions -->
      <div class="border-t border-white/10 pt-2">
        <template v-if="authStore.isAuthenticated">
          <button
            v-for="status in statusOptions"
            :key="status.value"
            class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm transition-colors"
            :class="listStatus === status.value
              ? 'bg-cyan-500/20 text-cyan-400'
              : 'text-white/70 hover:bg-white/5 hover:text-white'"
            @click="setStatus(status.value)"
          >
            <svg v-if="listStatus === status.value" class="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
            </svg>
            <span v-else class="w-4" />
            {{ $t(status.i18nKey) }}
          </button>

          <!-- Remove from list -->
          <button
            v-if="listStatus"
            class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm text-pink-400 hover:bg-pink-500/10 transition-colors mt-1"
            @click="removeFromList"
          >
            <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
            {{ $t('profile.actions.removeFromList') }}
          </button>
        </template>

        <p v-else class="text-xs text-white/30 px-2 py-2">
          {{ $t('anime.loginToManage') }}
        </p>
      </div>

      <!-- Go to page / Open in new tab -->
      <div class="border-t border-white/10 pt-2 mt-1">
        <button
          class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm text-white/70 hover:bg-white/5 hover:text-white transition-colors"
          @click="goToPage"
        >
          <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
          </svg>
          {{ $t('contextMenu.goToPage') }}
        </button>
        <button
          class="w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-sm text-white/70 hover:bg-white/5 hover:text-white transition-colors"
          @click="openInNewTab"
        >
          <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 3h7v7m0-7L10 14m-4-4H4a1 1 0 00-1 1v9a1 1 0 001 1h9a1 1 0 001-1v-2" />
          </svg>
          {{ $t('contextMenu.openInNewTab') }}
        </button>
      </div>
    </div>
  </ContextMenu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
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

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  anime: Anime | null
  listStatus: string | null
  siteRating?: { average_score: number; total_reviews: number } | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'statusChange': [animeId: string, status: string]
  'removeFromList': [animeId: string]
}>()

const router = useRouter()
const { locale } = useI18n()
const authStore = useAuthStore()
const watchlistStore = useWatchlistStore()

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
  emit('update:visible', false)
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
  emit('update:visible', false)
}

function goToPage() {
  if (!props.anime) return
  router.push(`/anime/${props.anime.id}`)
  emit('update:visible', false)
}

function openInNewTab() {
  if (!props.anime) return
  window.open(`/anime/${props.anime.id}`, '_blank', 'noopener,noreferrer')
  emit('update:visible', false)
}
</script>
