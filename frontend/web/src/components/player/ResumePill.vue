<template>
  <div
    v-if="visible"
    role="status"
    class="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/5 border border-white/10 text-white/70 text-xs flex-wrap"
  >
    <!-- watching -->
    <template v-if="kind === 'watching' && (finishedEpisode ?? 0) > 0">
      <svg class="w-3.5 h-3.5 text-cyan-400/70 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
      </svg>
      <span>{{ t('anime.resume.justFinished', { n: finishedEpisode }) }}</span>
    </template>

    <!-- finished -->
    <template v-else-if="kind === 'finished'">
      <svg class="w-3.5 h-3.5 text-success/70 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
      </svg>
      <span>{{ t('anime.resume.youFinishedThis') }}</span>
      <span class="text-white/30" aria-hidden="true">—</span>
      <button
        type="button"
        @click="$emit('rewatch')"
        class="text-cyan-400 hover:text-cyan-300 underline-offset-2 hover:underline transition-colors"
      >
        {{ t('anime.resume.rewatch') }}
      </button>
      <template v-if="canMarkCompleteInList">
        <span class="text-white/30" aria-hidden="true">·</span>
        <button
          type="button"
          @click="$emit('mark-complete-in-list')"
          class="text-cyan-400 hover:text-cyan-300 underline-offset-2 hover:underline transition-colors"
        >
          {{ t('anime.resume.markCompleteInList') }}
        </button>
      </template>
      <template v-if="findSimilarRoute">
        <span class="text-white/30" aria-hidden="true">·</span>
        <router-link
          :to="findSimilarRoute"
          class="text-cyan-400 hover:text-cyan-300 underline-offset-2 hover:underline transition-colors"
        >
          {{ t('anime.resume.findSimilar') }}
        </router-link>
      </template>
    </template>

    <!-- not-yet-aired -->
    <template v-else-if="kind === 'not-yet-aired' && nextEpisodeNumber">
      <svg class="w-3.5 h-3.5 text-warning/70 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <span v-if="nextEpisodeEtaLabel">
        {{ t('anime.resume.notYetAvailableEta', { n: nextEpisodeNumber, when: nextEpisodeEtaLabel }) }}
      </span>
      <span v-else>
        {{ t('anime.resume.notYetAvailable', { n: nextEpisodeNumber }) }}
      </span>
    </template>

    <!-- episode-not-loaded-yet: aired (air time passed) but not in our sources yet -->
    <template v-else-if="kind === 'episode-not-loaded-yet' && nextEpisodeNumber">
      <span class="relative flex h-2 w-2 flex-shrink-0" aria-hidden="true">
        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-pink-400/60 opacity-75"></span>
        <span class="relative inline-flex rounded-full h-2 w-2 bg-pink-400/70"></span>
      </span>
      <span>{{ t('anime.resume.episodeNotLoaded', { n: nextEpisodeNumber, ago: airedAgoLabel }) }}</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RouteLocationRaw } from 'vue-router'

type ResumeKind = 'first-time' | 'watching' | 'finished' | 'not-yet-aired' | 'episode-not-loaded-yet'

const props = defineProps<{
  kind: ResumeKind
  finishedEpisode?: number
  nextEpisodeNumber?: number
  nextEpisodeEtaLabel?: string
  /** episode-not-loaded-yet only: localized "aired N ago" label (e.g. "2 часа назад"). */
  airedAgoLabel?: string
  canMarkCompleteInList?: boolean
  findSimilarRoute?: RouteLocationRaw
}>()

defineEmits<{
  (e: 'rewatch'): void
  (e: 'mark-complete-in-list'): void
}>()

const { t } = useI18n()

const visible = computed(() => {
  if (props.kind === 'first-time') return false
  if (props.kind === 'watching') return (props.finishedEpisode ?? 0) > 0
  if (props.kind === 'not-yet-aired' || props.kind === 'episode-not-loaded-yet') {
    return !!props.nextEpisodeNumber
  }
  return true
})
</script>
