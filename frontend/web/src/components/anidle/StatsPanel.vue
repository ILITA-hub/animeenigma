<template>
  <div>
    <h2 class="text-lg font-semibold text-white mb-4">{{ $t('anidle.stats_title') }}</h2>

    <!-- Guest notice -->
    <div v-if="!isAuthenticated" class="rounded-xl border border-white/10 bg-white/5 p-4">
      <p class="text-sm text-muted-foreground">
        {{ $t('anidle.stats_guest_notice') }}
        <router-link to="/auth" class="text-white underline hover:no-underline ml-1">
          {{ $t('nav.login') }}
        </router-link>
      </p>
    </div>

    <!-- Loading state -->
    <LoadingState v-else-if="!stats" />

    <!-- Stats content -->
    <div v-else class="space-y-4">
      <!-- 4 stat boxes in 2×2 grid -->
      <div class="grid grid-cols-2 gap-3">
        <Card padding="sm" class="text-center">
          <p class="text-2xl font-semibold text-white">{{ stats.games_played }}</p>
          <p class="text-xs text-muted-foreground mt-1">{{ $t('anidle.stats_games_played') }}</p>
        </Card>
        <Card padding="sm" class="text-center">
          <p class="text-2xl font-semibold text-white">{{ stats.games_won }}</p>
          <p class="text-xs text-muted-foreground mt-1">{{ $t('anidle.stats_games_won') }}</p>
        </Card>
        <Card padding="sm" class="text-center">
          <p class="text-2xl font-semibold text-white">{{ stats.current_streak }}</p>
          <p class="text-xs text-muted-foreground mt-1">{{ $t('anidle.stats_streak_current') }}</p>
        </Card>
        <Card padding="sm" class="text-center">
          <p class="text-2xl font-semibold text-white">{{ stats.max_streak }}</p>
          <p class="text-xs text-muted-foreground mt-1">{{ $t('anidle.stats_streak_max') }}</p>
        </Card>
      </div>

      <!-- Guess distribution histogram -->
      <div v-if="Object.keys(stats.guess_distribution).length > 0">
        <h3 class="text-sm font-medium text-muted-foreground mb-2">
          {{ $t('anidle.stats_distribution_title') }}
        </h3>
        <div class="space-y-1.5">
          <div
            v-for="(count, attempt) in sortedDistribution"
            :key="attempt"
            class="flex items-center gap-2 text-sm"
          >
            <span class="text-muted-foreground w-4 text-right flex-shrink-0">{{ attempt }}</span>
            <div class="flex-1 h-4 rounded bg-white/5 overflow-hidden">
              <div
                class="h-full rounded bg-success transition-all duration-500"
                :style="{ width: barWidth(count) }"
              />
            </div>
            <span class="text-muted-foreground w-4 text-left flex-shrink-0">{{ count }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { UserStats } from '@/api/anidle'
import Card from '@/components/ui/Card.vue'
import LoadingState from '@/components/ui/LoadingState.vue'

const props = defineProps<{
  stats: UserStats | null
  isAuthenticated: boolean
}>()

const sortedDistribution = computed(() => {
  if (!props.stats) return {}
  const dist = props.stats.guess_distribution
  // Sort by attempt number (key is a string)
  return Object.fromEntries(
    Object.entries(dist).sort(([a], [b]) => Number(a) - Number(b)),
  )
})

const maxCount = computed(() => {
  if (!props.stats) return 1
  const vals = Object.values(props.stats.guess_distribution)
  return Math.max(1, ...vals)
})

function barWidth(count: number): string {
  return `${Math.round((count / maxCount.value) * 100)}%`
}
</script>
