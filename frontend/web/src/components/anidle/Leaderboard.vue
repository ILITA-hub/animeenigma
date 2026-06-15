<template>
  <div>
    <h2 class="text-lg font-semibold text-white mb-4">{{ $t('anidle.leaderboard_title') }}</h2>

    <LoadingState v-if="loading" />

    <div v-else-if="entries.length === 0" class="rounded-xl border border-white/10 bg-white/5 p-4">
      <p class="text-sm text-muted-foreground text-center">{{ $t('anidle.leaderboard_empty') }}</p>
    </div>

    <div v-else class="rounded-xl border border-white/10 bg-white/5 overflow-hidden">
      <div
        v-for="(entry, index) in entries"
        :key="entry.username"
        :class="[
          'flex items-center gap-3 px-4 py-3 text-sm',
          index < entries.length - 1 ? 'border-b border-white/5' : '',
        ]"
      >
        <span class="text-muted-foreground font-medium w-6 flex-shrink-0">
          {{ $t('anidle.leaderboard_rank', { n: index + 1 }) }}
        </span>
        <span class="text-white font-medium flex-1 truncate">{{ entry.username }}</span>
        <span class="text-muted-foreground">
          {{ $t('anidle.leaderboard_attempts', { n: entry.attempts }) }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { LeaderEntry } from '@/api/anidle'
import LoadingState from '@/components/ui/LoadingState.vue'

defineProps<{
  entries: LeaderEntry[]
  loading: boolean
}>()
</script>
