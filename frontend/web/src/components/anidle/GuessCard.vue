<template>
  <div class="rounded-xl border border-white/10 bg-white/5 p-4 space-y-3">
    <!-- Anime info row -->
    <div class="flex items-center gap-3">
      <img
        :src="posterSrc"
        :alt="guess.anime.name_ru"
        class="w-12 h-16 rounded-lg object-cover flex-shrink-0 bg-white/10"
      />
      <div class="min-w-0">
        <p class="font-semibold text-white truncate">{{ guess.anime.name_ru }}</p>
        <p class="text-sm text-muted-foreground truncate">{{ guess.anime.name_en }}</p>
      </div>
    </div>

    <!-- 2×4 chip grid -->
    <div class="grid grid-cols-4 gap-1">
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_genres') }}</p>
        <GuessCell
          :status="guess.result.genres.status"
          :value="genreNames"
          :full="allGenres"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_studios') }}</p>
        <GuessCell
          :status="guess.result.studios.status"
          :value="studioNames"
          :full="allStudios"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_year') }}</p>
        <GuessCell
          :status="guess.result.year.status"
          :value="guess.anime.year"
          :hint="guess.result.year.hint"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_episodes') }}</p>
        <GuessCell
          :status="guess.result.episodes.status"
          :value="guess.anime.episodes"
          :hint="guess.result.episodes.hint"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_score') }}</p>
        <GuessCell
          :status="guess.result.score.status"
          :value="guess.anime.score"
          :hint="guess.result.score.hint"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_status') }}</p>
        <GuessCell
          :status="guess.result.status.status"
          :value="guess.anime.status"
        />
      </div>
      <div class="space-y-1">
        <p class="text-xs text-muted-foreground text-center truncate">{{ $t('anidle.column_rating') }}</p>
        <GuessCell
          :status="guess.result.rating.status"
          :value="guess.anime.rating"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import type { GuessOutcome } from '@/api/anidle'
import GuessCell from './GuessCell.vue'

const props = defineProps<{
  guess: GuessOutcome
}>()

const posterSrc = computed(() => cardPosterUrl(props.guess.anime.poster_url, 128))
const genreNames = computed(() => props.guess.anime.genres.slice(0, 2).map(g => g.name).join(', ') || '—')
const studioNames = computed(() => props.guess.anime.studios.slice(0, 1).map(s => s.name).join(', ') || '—')
const allGenres = computed(() => props.guess.anime.genres.map(g => g.name).join(', '))
const allStudios = computed(() => props.guess.anime.studios.map(s => s.name).join(', '))
</script>
