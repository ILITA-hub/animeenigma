<template>
  <article
    class="w-full h-full flex flex-col gap-4 p-4 md:p-4 lg:p-6"
  >
    <header>
      <h3 class="text-lg md:text-xl font-semibold text-white">
        {{ t('spotlight.platformStats.title') }}
      </h3>
    </header>

    <ul
      class="flex-1 grid gap-3 md:gap-4"
      :class="{
        'grid-cols-1': data.metrics.length === 1,
        'grid-cols-1 md:grid-cols-2': data.metrics.length === 2,
        'grid-cols-1 md:grid-cols-3': data.metrics.length >= 3,
      }"
    >
      <li
        v-for="m in data.metrics"
        :key="m.key"
        class="flex flex-col items-start p-4 rounded-xl bg-white/5"
      >
        <p
          class="text-xs font-medium text-gray-400 uppercase tracking-wider"
        >
          {{ t(`spotlight.platformStats.${camelize(m.key)}`) }}
        </p>
        <p
          class="mt-1 text-3xl md:text-4xl font-semibold text-white tabular-nums leading-none"
        >
          {{ m.value.toLocaleString(localeStr) }}
        </p>
        <p
          v-if="typeof m.delta === 'number' && m.delta > 0"
          class="mt-1 text-xs font-medium text-cyan-400 tabular-nums"
        >
          {{ t('spotlight.platformStats.deltaPositive', { n: m.delta }) }}
        </p>
        <p
          v-else-if="m.delta === 0 || m.delta == null"
          class="mt-1 text-xs font-medium text-gray-500"
        >
          {{ t('spotlight.platformStats.noChange') }}
        </p>
      </li>
    </ul>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlatformStatsData } from '@/types/spotlight'

defineProps<{ data: PlatformStatsData }>()
const { t, locale: i18nLocale } = useI18n()

// Backend emits metric keys in snake_case (`anime_added_7d`); UI-SPEC's
// Copywriting Contract declares the matching i18n keys in camelCase
// (`spotlight.platformStats.animeAdded7d`). Convert here so Plan 02-05's
// locale JSON can stay camelCase-only.
function camelize(snake: string): string {
  return snake.replace(/_([a-z0-9])/g, (_, c) => (c as string).toUpperCase())
}

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})
</script>
