<template>
  <!--
    Workstream daily-fanfic-spotlight — Task 14 stub.

    MINIMAL COMPILING STUB: satisfies the vue-tsc `data` prop narrowing +
    the HeroSpotlightBlock.vue import so the type/token/dispatch plumbing
    lands cleanly. Task 15 replaces this body with the real card anatomy
    (fanfic excerpt, author credit, AI-generated badge, etc. per UI-SPEC).
  -->
  <SpotlightCardShell accent="pink" icon="sparkles" :kicker="t('spotlight.dailyFanfic.title')">
    {{ data.fanfic_title }}
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DailyFanficData } from '@/types/spotlight'
import SpotlightCardShell from '../SpotlightCardShell.vue'

defineProps<{ data: DailyFanficData }>()
const { t } = useI18n()
</script>
