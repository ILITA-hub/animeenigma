<template>
  <div class="font-mono text-sm space-y-1">
    <p class="font-semibold text-white">Anidle {{ date }} — {{ guesses.length }} {{ $t('anidle.result_attempts', { n: guesses.length }) }}</p>
    <div class="space-y-0.5">
      <p v-for="(guess, i) in reversedGuesses" :key="i" class="tracking-widest">
        <span v-for="(col, j) in columns" :key="j">{{ emojiFor(guess, col) }}</span>
      </p>
      <p v-if="!solved" class="text-muted-foreground text-xs">❌</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { GuessOutcome, GuessComparison } from '@/api/anidle'

const props = defineProps<{
  guesses: GuessOutcome[]
  date: string
  solved: boolean
}>()

const columns: Array<keyof GuessComparison> = [
  'genres', 'studios', 'year', 'episodes', 'score', 'status', 'rating', 'tags',
]

const reversedGuesses = computed(() => props.guesses.slice().reverse())

function emojiFor(guess: GuessOutcome, col: keyof GuessComparison): string {
  const s = guess.result[col]?.status
  if (s === 'correct') return '🟩'
  if (s === 'partial') return '🟨'
  return '⬜'
}
</script>
