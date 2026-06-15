<template>
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <!-- Page header -->
      <h1 class="text-3xl font-semibold text-white mb-1">{{ $t('anidle.page_title') }}</h1>
      <p class="text-muted-foreground text-sm mb-6">{{ $t('anidle.page_subtitle') }}</p>

      <!-- Loading / error states -->
      <LoadingState v-if="isLoading" :label="$t('anidle.loading')" />
      <Alert v-else-if="error" variant="destructive">{{ error }}</Alert>

      <template v-else>
        <!-- Mode tabs -->
        <ModeTabs v-model="mode" class="mb-6" />

        <!-- Daily mode -->
        <template v-if="mode === 'daily'">
          <!-- Already played notice (solved but modal closed) -->
          <Alert
            v-if="(dailySolved || dailyGaveUp) && !showResult"
            variant="default"
            class="mb-4"
          >
            {{ $t('anidle.daily_complete_played') }}
          </Alert>

          <!-- Search + Give Up row (only when game is active) -->
          <div v-if="!dailySolved && !dailyGaveUp" class="flex gap-3 mb-6">
            <AnidleSearch
              :disabled="isGuessing"
              class="flex-1"
              @select="onDailyGuess"
            />
            <button
              type="button"
              :disabled="isGuessing"
              class="px-4 py-2 rounded-lg border border-white/20 text-white/70 hover:text-white hover:border-white/40 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex-shrink-0"
              @click="onGiveUp"
            >
              {{ $t('anidle.give_up_button') }}
            </button>
          </div>

          <!-- Guess grid -->
          <GuessGrid :guesses="dailyGuesses" class="mb-6" />
        </template>

        <!-- Endless mode -->
        <template v-else-if="mode === 'endless'">
          <div v-if="!endlessToken" class="flex justify-center py-8">
            <button
              type="button"
              class="px-6 py-3 rounded-xl bg-white/10 hover:bg-white/20 text-white font-medium transition-colors"
              @click="startEndless"
            >
              {{ $t('anidle.endless_new_round') }}
            </button>
          </div>

          <template v-else>
            <div v-if="!endlessSolved" class="flex gap-3 mb-6">
              <AnidleSearch
                :disabled="isGuessing"
                class="flex-1"
                @select="onEndlessGuess"
              />
            </div>

            <div v-if="endlessSolved" class="mb-4">
              <Alert variant="default">
                {{ $t('anidle.endless_win_title') }}
              </Alert>
            </div>

            <GuessGrid :guesses="endlessGuesses" class="mb-6" />

            <div v-if="endlessSolved" class="flex justify-center mt-4">
              <button
                type="button"
                class="px-6 py-3 rounded-xl bg-white/10 hover:bg-white/20 text-white font-medium transition-colors"
                @click="startEndless"
              >
                {{ $t('anidle.endless_new_round') }}
              </button>
            </div>
          </template>
        </template>

        <!-- Stats + Leaderboard (below the game area) -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mt-10">
          <StatsPanel :stats="stats" :is-authenticated="isAuthenticated" />
          <Leaderboard :entries="leaderboard" :loading="loadingLeaderboard" />
        </div>
      </template>
    </div>

    <!-- Result modal (daily solve or give-up) -->
    <ResultModal
      v-if="dailyAnswer && showResult"
      :open="showResult"
      :answer="dailyAnswer"
      :guesses="dailyGuesses"
      :date="dailyDate"
      :solved="dailySolved"
      @close="showResult = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAnidle } from '@/composables/useAnidle'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/ui/LoadingState.vue'
import Alert from '@/components/ui/Alert.vue'
import ModeTabs from '@/components/anidle/ModeTabs.vue'
import AnidleSearch from '@/components/anidle/AnidleSearch.vue'
import GuessGrid from '@/components/anidle/GuessGrid.vue'
import ResultModal from '@/components/anidle/ResultModal.vue'
import StatsPanel from '@/components/anidle/StatsPanel.vue'
import Leaderboard from '@/components/anidle/Leaderboard.vue'

const auth = useAuthStore()
const isAuthenticated = auth.isAuthenticated

const {
  mode,
  dailyDate,
  dailyGuesses,
  dailySolved,
  dailyGaveUp,
  dailyAnswer,
  endlessToken,
  endlessGuesses,
  endlessSolved,
  isLoading,
  isGuessing,
  error,
  stats,
  leaderboard,
  submitDailyGuess,
  submitGiveUp,
  startEndless,
  submitEndlessGuess,
  fetchStats,
  fetchLeaderboard,
} = useAnidle()

const showResult = ref(false)
const loadingLeaderboard = ref(false)

// Auto-open result modal when game ends
watch([dailySolved, dailyGaveUp], ([solved, gaveUp]) => {
  if (solved || gaveUp) {
    showResult.value = true
  }
})

// Fetch stats and leaderboard on mount (non-blocking)
async function onDailyGuess(id: string) {
  await submitDailyGuess(id)
  if (dailySolved.value || dailyGaveUp.value) {
    showResult.value = true
  }
}

async function onGiveUp() {
  await submitGiveUp()
  showResult.value = true
}

async function onEndlessGuess(id: string) {
  await submitEndlessGuess(id)
}

// Load stats + leaderboard
;(async () => {
  if (auth.isAuthenticated) {
    void fetchStats()
  }
  loadingLeaderboard.value = true
  try {
    await fetchLeaderboard(dailyDate.value)
  } finally {
    loadingLeaderboard.value = false
  }
})()
</script>
