<template>
  <SpotlightCardShell
    accent="cyan"
    icon="chart"
    :kicker="t('spotlight.platformStats.title')"
    backdrop="none"
  >
  <!--
    Workstream hero-spotlight — DS alignment 2026-06-10, batch 1 (spec:
    2026-06-10-spotlight-ds-alignment-design.md). Two-column hero-stats
    layout rebuilt on SpotlightCardShell + Tailwind utilities: kicker row
    added (was the only card without one), font weights clamped to the DS
    medium/semibold scale, scoped CSS reduced to the bespoke gradient
    backdrop + decorative tile highlight. All data bindings unchanged.
  -->
    <template #background>
      <div class="stats-bg" aria-hidden="true" />
    </template>

    <div class="flex-1 min-h-0 grid md:grid-cols-[1.1fr_1fr] gap-5 md:gap-8 overflow-y-auto">
      <!-- LEFT column: status headline + uptime + vibe row + tagline -->
      <div class="flex flex-col justify-center gap-3 md:gap-4">
        <h2 class="font-display font-semibold text-3xl md:text-[44px] leading-[1.05] tracking-[-0.025em]">
          Работает:
          <span :class="workingOk ? 'text-success' : 'text-warning'">
            {{ workingOk ? 'ДА' : 'ТЕХНИЧЕСКИ ДА' }}
          </span>
        </h2>

        <p class="font-mono text-[13px] text-cyan-400 tracking-[0.04em]">
          Аптайм: {{ hero.uptime_quip
          }}<template v-if="hero.uptime_percent != null"> — {{ hero.uptime_percent }}%</template>
        </p>

        <p class="font-mono text-[11px] text-white/40 leading-snug">
          {{ hero.service }} — UXΔ {{ hero.ux_delta }} · CDI {{ hero.cdi }} · MVQ {{ hero.mvq }}
        </p>

        <blockquote class="italic text-sm text-white/70 border-l-2 border-cyan-400/60 pl-3.5 max-w-[380px]">
          «{{ hero.tagline }}»
        </blockquote>
      </div>

      <!-- RIGHT column: 2×2 stat tile grid -->
      <ul class="grid grid-cols-2 gap-3 content-center">
        <li
          v-for="tile in tiles"
          :key="tile.label"
          class="stat-tile relative p-4 rounded-lg bg-white/5 border border-white/10 overflow-hidden"
        >
          <p class="font-mono text-[10px] uppercase tracking-[0.12em] text-cyan-400 mb-1.5">
            {{ windowLabel(tile.window) }}
          </p>
          <p class="font-display font-semibold text-2xl md:text-[32px] leading-[1.05] tracking-[-0.02em]">
            {{ formatValue(tile) }}
          </p>
          <p class="text-xs text-muted-foreground mt-1.5">{{ tile.label }}</p>
        </li>
      </ul>
    </div>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
// Workstream hero-spotlight — PlatformStatsCard. All data bindings,
// computed fields, and type imports unchanged by the DS alignment.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { statusApi } from '@/api/client'
import type { PlatformStatsData, StatsTile } from '@/types/spotlight'
import SpotlightCardShell from '../SpotlightCardShell.vue'

const props = defineProps<{ data: PlatformStatsData }>()
const { t } = useI18n()

const hero = computed(() => props.data.hero)
const tiles = computed(() => props.data.tiles ?? [])

// The backend hero is cached for the whole UTC day, so its working_ok can
// freeze a transient redeploy blip into a day of «ТЕХНИЧЕСКИ ДА». Re-check
// the live /api/status aggregate (same source as the /status page) on mount
// and prefer its verdict; the cached value stays as the fallback.
const liveWorkingOk = ref<boolean | null>(null)

onMounted(async () => {
  try {
    const res = await statusApi.getStatus()
    const overall: string | undefined = res.data?.data?.overall
    if (overall) liveWorkingOk.value = overall === 'operational'
  } catch {
    // Status endpoint unreachable — keep the cached fallback.
  }
})

const workingOk = computed(() => liveWorkingOk.value ?? hero.value.working_ok)

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

<style scoped>
/* Bespoke keeps: the cyan radial-on-deep-blue gradient backdrop (transcribed
   from the design handoff) and the decorative per-tile highlight — neither
   maps to a token surface. */
.stats-bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(700px 400px at 20% 80%, rgba(0, 212, 255, 0.1), transparent 60%),
    linear-gradient(135deg, #0d2030 0%, #050a12 100%);
}
.stat-tile::after {
  content: "";
  position: absolute;
  right: -20px;
  top: -20px;
  width: 80px;
  height: 80px;
  border-radius: 999px;
  background: radial-gradient(circle, var(--accent-soft), transparent 70%);
  opacity: 0.7;
}
</style>
