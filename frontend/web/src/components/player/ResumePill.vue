<template>
  <div
    v-if="banner.kind !== 'none'"
    role="status"
    class="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-muted/50 border border-border text-muted-foreground text-xs flex-wrap"
  >
    <template v-if="banner.kind === 'just-finished'">
      <Check class="size-3.5 text-brand-cyan flex-shrink-0" aria-hidden="true" />
      <span>{{ t('anime.resume.justFinished', { n: banner.episode }) }}</span>
    </template>

    <template v-else-if="banner.kind === 'next-unavailable'">
      <Clock class="size-3.5 text-warning/70 flex-shrink-0" aria-hidden="true" />
      <span v-if="banner.etaLabel">
        {{ t('anime.resume.notYetAvailableEta', { n: banner.episode, when: banner.etaLabel }) }}
      </span>
      <span v-else>
        {{ t('anime.resume.notYetAvailable', { n: banner.episode }) }}
      </span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Clock } from 'lucide-vue-next'
import type { ResumeBanner } from '@/composables/watchState'

defineProps<{ banner: ResumeBanner }>()

const { t } = useI18n()
</script>
