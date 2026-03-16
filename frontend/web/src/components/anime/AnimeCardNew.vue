<template>
  <router-link
    :to="`/anime/${anime.id}`"
    class="group block"
  >
    <div class="card-hover rounded-xl overflow-hidden bg-white/5 border border-white/10">
      <!-- Poster Container -->
      <div class="relative aspect-[2/3] overflow-hidden bg-surface">
        <!-- Placeholder (visible until image loads) -->
        <div class="absolute inset-0 bg-gradient-to-b from-white/5 to-white/10 animate-pulse" />
        <!-- Lazy Loaded Image (fades in over placeholder) -->
        <img
          :src="anime.coverImage"
          :alt="localizedTitle"
          class="absolute inset-0 w-full h-full object-cover transition-[opacity,transform] duration-300 group-hover:scale-110"
          :class="imageLoaded ? 'opacity-100' : 'opacity-0'"
          loading="lazy"
          @load="imageLoaded = true"
          @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.coverImage) } }"
        />

        <!-- Overlay Gradient -->
        <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />

        <!-- Top Badges -->
        <div class="absolute top-2 left-2 right-2 flex justify-between items-start">
          <!-- Quality Badge -->
          <Badge v-if="anime.quality" variant="default" size="sm">
            {{ anime.quality }}
          </Badge>
          <span v-else />

          <!-- Rating Badge -->
          <Badge
            v-if="anime.rating"
            :variant="ratingVariant"
            size="sm"
            class="flex items-center gap-1"
          >
            <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {{ anime.rating.toFixed(1) }}
          </Badge>
        </div>

        <!-- Watchlist Status Badge -->
        <div v-if="listStatus" class="absolute bottom-2 left-2">
          <span :class="listBadgeClasses">
            {{ listStatusLabel }}
          </span>
        </div>

        <!-- Play Button Overlay -->
        <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
          <div class="w-14 h-14 rounded-full bg-cyan-500/90 flex items-center justify-center shadow-[0_0_20px_rgba(0,212,255,0.5)]">
            <svg class="w-6 h-6 text-white ml-1" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
        </div>
      </div>

      <!-- Card Content -->
      <div class="p-3">
        <!-- Title -->
        <h3 class="font-medium text-white line-clamp-2 mb-1 group-hover:text-cyan-400 transition-colors">
          {{ localizedTitle }}
        </h3>

        <!-- Meta Info -->
        <div class="flex items-center gap-2 text-xs text-white/50">
          <span v-if="anime.releaseYear">{{ anime.releaseYear }}</span>
          <span v-if="anime.releaseYear && anime.episodes" class="text-white/30">•</span>
          <span v-if="anime.episodes">{{ anime.episodes }} {{ $t('anime.episode') }}</span>
          <span v-if="anime.episodes && primaryGenre" class="text-white/30">•</span>
          <span v-if="primaryGenre">{{ primaryGenre }}</span>
        </div>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from '@/components/ui/Badge.vue'
import { getLocalizedTitle, getLocalizedGenre } from '@/utils/title'
import { getImageFallbackUrl } from '@/composables/useImageProxy'

const { t, locale } = useI18n()

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
  rawGenres?: { name?: string; nameRu?: string }[]
  quality?: string
}

const props = defineProps<{
  anime: Anime
  listStatus?: string | null
}>()

const imageLoaded = ref(false)

const localizedTitle = computed(() => {
  if (props.anime.name || props.anime.nameRu || props.anime.nameJp) {
    // Force reactivity on locale change
    void locale.value
    return getLocalizedTitle(props.anime.name, props.anime.nameRu, props.anime.nameJp)
  }
  return props.anime.title
})

const localizedGenre = computed(() => {
  if (props.anime.rawGenres?.length) {
    void locale.value
    return getLocalizedGenre(props.anime.rawGenres[0].name, props.anime.rawGenres[0].nameRu)
  }
  return props.anime.genres?.[0] ?? ''
})

const ratingVariant = computed(() => {
  if (!props.anime.rating) return 'default'
  if (props.anime.rating >= 8) return 'rating'
  if (props.anime.rating >= 6) return 'default'
  return 'warning'
})

const primaryGenre = computed(() => {
  return localizedGenre.value
})

const statusToI18nKey: Record<string, string> = {
  watching: 'profile.watchlist.watching',
  plan_to_watch: 'profile.watchlist.planToWatch',
  completed: 'profile.watchlist.completed',
  on_hold: 'profile.watchlist.onHold',
  dropped: 'profile.watchlist.dropped',
}

const statusColors: Record<string, string> = {
  watching: 'bg-cyan-500/80 text-white',
  plan_to_watch: 'bg-white/20 text-white/90 backdrop-blur-sm',
  completed: 'bg-emerald-500/80 text-white',
  on_hold: 'bg-amber-500/80 text-white',
  dropped: 'bg-red-500/80 text-white',
}

const listStatusLabel = computed(() => {
  if (!props.listStatus) return ''
  return t(statusToI18nKey[props.listStatus] || props.listStatus)
})

const listBadgeClasses = computed(() => {
  const color = statusColors[props.listStatus || ''] || 'bg-white/20 text-white/90'
  return `inline-flex items-center px-2 py-0.5 text-xs font-medium rounded ${color}`
})

</script>
