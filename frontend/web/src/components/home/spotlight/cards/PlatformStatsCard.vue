<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop variant="gradient-mesh" accent="teal" />
    <div
      aria-hidden="true"
      class="absolute inset-0 opacity-5"
      style="background-image: repeating-linear-gradient(0deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px), repeating-linear-gradient(90deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px);"
    />
    <div
      class="relative z-10 w-full h-full grid md:grid-cols-[2fr_3fr] gap-6 p-4 md:p-6 lg:p-8"
    >
      <!-- Hero (left) -->
      <div class="flex flex-col justify-center min-w-0">
        <div class="flex items-center gap-2 mb-3">
          <SpotlightIcon name="chart" class="w-5 h-5 text-teal-300" />
          <h3 class="text-base font-semibold text-white">Как дела у платформы</h3>
        </div>

        <p class="text-2xl md:text-3xl font-semibold text-white leading-tight">
          Работает:
          <span :class="hero.working_ok ? 'text-teal-300' : 'text-amber-300'">
            {{ hero.working_ok ? 'ДА' : 'ТЕХНИЧЕСКИ ДА' }}
          </span>
        </p>

        <p class="mt-2 text-lg font-medium text-teal-200">
          Аптайм: {{ hero.uptime_quip
          }}<template v-if="hero.uptime_percent != null"> — {{ hero.uptime_percent }}%</template>
        </p>

        <p class="mt-3 text-sm font-medium text-gray-200 break-words">
          {{ hero.service }} — UXΔ {{ hero.ux_delta }} · CDI {{ hero.cdi }} · MVQ {{ hero.mvq }}
        </p>

        <p class="mt-4 text-base md:text-lg font-medium text-white/90 italic">
          «{{ hero.tagline }}»
        </p>
      </div>

      <!-- Tiles (right, 2×2) -->
      <ul class="grid grid-cols-2 gap-3 content-center min-w-0">
        <li
          v-for="tile in tiles"
          :key="tile.label"
          class="flex flex-col p-3 rounded-lg bg-white/5 backdrop-blur-sm"
        >
          <span class="text-[10px] font-medium text-teal-300 uppercase tracking-wider">
            {{ windowLabel(tile.window) }}
          </span>
          <p class="mt-1 text-2xl font-semibold text-white tabular-nums">
            {{ formatValue(tile) }}
          </p>
          <p class="text-[11px] font-medium text-gray-400 truncate">{{ tile.label }}</p>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
// Workstream hero-spotlight — Trump-style joke rewrite of platform_stats.
// SINGLE-ROOT <article>, NO top-level v-if (Transition mode="out-in" safety).
// Deliberately i18n-free: chrome is fixed Russian and the joke content is
// rendered verbatim from the backend payload (everyone sees the same).
import { computed } from 'vue'
import type { PlatformStatsData, StatsTile } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'

const props = defineProps<{ data: PlatformStatsData }>()

const hero = computed(() => props.data.hero)
const tiles = computed(() => props.data.tiles ?? [])

function windowLabel(w: StatsTile['window']): string {
  switch (w) {
    case 'day':
      return 'ЗА ДЕНЬ'
    case 'week':
      return 'ЗА НЕДЕЛЮ'
    default:
      return 'ЗА ВСЁ ВРЕМЯ'
  }
}

function formatValue(tile: StatsTile): string {
  // Defensive: a non-finite or negative value (e.g. an upstream error
  // sentinel) should never render as "NaN Б" / "-1 КБ".
  if (!Number.isFinite(tile.value) || tile.value < 0) return '—'
  if (tile.format === 'bytes') return formatBytes(tile.value)
  if (tile.format === 'seconds') return `${tile.value.toFixed(2)} с`
  return Math.round(tile.value).toLocaleString('ru')
}

function formatBytes(n: number): string {
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}
</script>
