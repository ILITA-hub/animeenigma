<template>
  <div
    v-if="visible"
    role="status"
    class="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-muted/50 border border-border text-muted-foreground text-xs flex-wrap"
  >
    <!-- watching -->
    <template v-if="kind === 'watching' && (finishedEpisode ?? 0) > 0">
      <Check class="size-3.5 text-brand-cyan flex-shrink-0" aria-hidden="true" />
      <span>{{ t('anime.resume.justFinished', { n: finishedEpisode }) }}</span>
    </template>

    <!-- not-yet-aired -->
    <template v-else-if="kind === 'not-yet-aired' && nextEpisodeNumber">
      <Clock class="size-3.5 text-warning/70 flex-shrink-0" aria-hidden="true" />
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
        <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-brand-pink opacity-75"></span>
        <span class="relative inline-flex rounded-full h-2 w-2 bg-brand-pink"></span>
      </span>
      <span>{{ t('anime.resume.episodeNotLoaded', { n: nextEpisodeNumber, ago: airedAgoLabel }) }}</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Clock } from 'lucide-vue-next'

type ResumeKind = 'first-time' | 'watching' | 'finished' | 'not-yet-aired' | 'episode-not-loaded-yet'

const props = defineProps<{
  kind: ResumeKind
  finishedEpisode?: number
  nextEpisodeNumber?: number
  nextEpisodeEtaLabel?: string
  /** episode-not-loaded-yet only: localized "aired N ago" label (e.g. "2 часа назад"). */
  airedAgoLabel?: string
}>()

const { t } = useI18n()

// 'finished' renders nothing — the player still mounts on the last episode and
// the state machine keeps classifying it 'finished' (so no wrong "watching"
// breadcrumb shows), but there is no finished surface here anymore.
const visible = computed(() => {
  if (props.kind === 'watching') return (props.finishedEpisode ?? 0) > 0
  if (props.kind === 'not-yet-aired' || props.kind === 'episode-not-loaded-yet') {
    return !!props.nextEpisodeNumber
  }
  return false
})
</script>
