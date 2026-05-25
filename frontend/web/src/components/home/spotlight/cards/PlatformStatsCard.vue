<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop variant="gradient-mesh" accent="teal" />
    <!-- Faint grid pattern overlay for chart context -->
    <div
      aria-hidden="true"
      class="absolute inset-0 opacity-5"
      style="background-image: repeating-linear-gradient(0deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px), repeating-linear-gradient(90deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px);"
    />
    <div
      class="relative z-10 w-full h-full grid md:grid-cols-[2fr_3fr] gap-6 p-4 md:p-6 lg:p-8"
    >
      <!-- Hero stat (left) -->
      <div v-if="heroMetric" class="flex flex-col justify-center min-w-0">
        <div class="flex items-center gap-2 mb-2">
          <SpotlightIcon name="chart" class="w-5 h-5 text-teal-300" />
          <h3 class="text-base font-semibold text-white">
            {{ t('spotlight.platformStats.title') }}
          </h3>
        </div>
        <p
          class="text-xs font-medium text-teal-200 uppercase tracking-wider mb-2"
        >
          {{ t(`spotlight.platformStats.${camelize(heroMetric.key)}`) }}
        </p>
        <p
          class="text-7xl md:text-8xl font-semibold text-white tabular-nums leading-none"
        >
          {{ heroMetric.value.toLocaleString(localeStr) }}
        </p>
        <!-- Delta chip -->
        <div class="mt-3 flex items-center gap-2">
          <DeltaChip
            :current="heroMetric.value"
            :previous="heroMetric.previous_value"
          />
          <span class="text-xs text-gray-400">{{
            t('spotlight.platformStats.vsPriorWeek')
          }}</span>
        </div>
        <!-- Sparkline -->
        <Sparkline
          v-if="heroMetric.series && heroMetric.series.length >= 2"
          :data="heroMetric.series"
          class="mt-3 h-10 w-full text-teal-300"
        />
      </div>

      <!-- Micro-grid (right, 2×2) -->
      <ul class="grid grid-cols-2 gap-3 content-center min-w-0">
        <li
          v-for="m in supportingMetrics"
          :key="m.key"
          class="flex flex-col p-3 rounded-lg bg-white/5 backdrop-blur-sm"
        >
          <p
            class="text-[10px] font-medium text-gray-400 uppercase tracking-wider truncate"
          >
            {{ t(`spotlight.platformStats.${camelize(m.key)}`) }}
          </p>
          <p class="mt-1 text-xl font-semibold text-white tabular-nums">
            {{ m.value.toLocaleString(localeStr) }}
          </p>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
// v1.1-polish Phase 08 (platform-stats-refactor): hero-stat (left, oversized)
// + 2×2 supporting micro-grid (right) + 7-day sparkline + delta chip.
//
// SINGLE-ROOT <article> (project rule): the template root is the bare
// <article> with NO leading comment and NO top-level v-if, so Vue 3
// <Transition mode="out-in"> never wedges. The hero block is guarded by
// v-if="heroMetric" INSIDE the root for defensive safety even though the
// backend's eligibility check guarantees ≥1 metric.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlatformStatsData } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import Sparkline from '../Sparkline.vue'
import DeltaChip from '../DeltaChip.vue'

const props = defineProps<{ data: PlatformStatsData }>()
const { t, locale: i18nLocale } = useI18n()

// Hero = first metric; supporting = next four. Backend's emission order is
// the stable display contract (Phase 1 ships only anime_added_7d, so
// supportingMetrics is empty until more metrics land — the micro-grid then
// renders nothing, which is fine).
const heroMetric = computed(() => props.data.metrics[0])
const supportingMetrics = computed(() => props.data.metrics.slice(1, 5))

// Backend emits metric keys in snake_case (`anime_added_7d`); UI-SPEC's
// Copywriting Contract declares the matching i18n keys in camelCase
// (`spotlight.platformStats.animeAdded7d`). Convert here so the locale JSON
// can stay camelCase-only.
function camelize(snake: string): string {
  return snake.replace(/_([a-z0-9])/g, (_, c) => (c as string).toUpperCase())
}

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})
</script>
