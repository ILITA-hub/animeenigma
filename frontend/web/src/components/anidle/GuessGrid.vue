<template>
  <div v-if="guesses.length > 0">
    <!-- Desktop: horizontal-scroll table (≥md) -->
    <div class="hidden md:block overflow-x-auto">
      <table class="w-full text-sm border-collapse min-w-[560px]">
        <thead>
          <tr class="border-b border-white/10">
            <th class="text-left py-2 px-3 text-muted-foreground font-medium min-w-[160px]">
              Anime
            </th>
            <th
              v-for="col in columns"
              :key="col.key"
              class="py-2 px-2 text-muted-foreground font-medium text-center w-[80px]"
            >
              {{ $t(col.labelKey) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="guess in reversed"
            :key="guess.attempt"
            class="border-b border-white/5"
          >
            <td class="py-2 px-3">
              <div class="flex items-center gap-2">
                <img
                  :src="posterFor(guess)"
                  :alt="guess.anime.name_ru"
                  class="w-8 h-11 rounded object-cover flex-shrink-0 bg-white/10"
                />
                <span class="text-white font-medium truncate max-w-[120px]">{{ guess.anime.name_ru }}</span>
              </div>
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.genres.status"
                :value="genreNames(guess)"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.studios.status"
                :value="studioNames(guess)"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.year.status"
                :value="guess.anime.year"
                :hint="guess.result.year.hint"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.episodes.status"
                :value="guess.anime.episodes"
                :hint="guess.result.episodes.hint"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.score.status"
                :value="guess.anime.score"
                :hint="guess.result.score.hint"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.status.status"
                :value="guess.anime.status"
              />
            </td>
            <td class="py-2 px-2 text-center">
              <GuessCell
                :status="guess.result.rating.status"
                :value="guess.anime.rating"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Mobile: card stack (<md) -->
    <div class="block md:hidden space-y-3">
      <GuessCard
        v-for="guess in reversed"
        :key="guess.attempt"
        :guess="guess"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import type { GuessOutcome } from '@/api/anidle'
import GuessCell from './GuessCell.vue'
import GuessCard from './GuessCard.vue'

const props = defineProps<{
  guesses: GuessOutcome[]
}>()

// Newest guess at the top
const reversed = computed(() => props.guesses.slice().reverse())

const columns = [
  { key: 'genres', labelKey: 'anidle.column_genres' },
  { key: 'studios', labelKey: 'anidle.column_studios' },
  { key: 'year', labelKey: 'anidle.column_year' },
  { key: 'episodes', labelKey: 'anidle.column_episodes' },
  { key: 'score', labelKey: 'anidle.column_score' },
  { key: 'status', labelKey: 'anidle.column_status' },
  { key: 'rating', labelKey: 'anidle.column_rating' },
] as const

function posterFor(guess: GuessOutcome) {
  return cardPosterUrl(guess.anime.poster_url, 128)
}

// Cap genres/studios to keep the fixed-width cells uniform and readable.
function genreNames(guess: GuessOutcome) {
  return guess.anime.genres.slice(0, 2).map(g => g.name).join(', ') || '—'
}

function studioNames(guess: GuessOutcome) {
  return guess.anime.studios.slice(0, 1).map(s => s.name).join(', ') || '—'
}
</script>
