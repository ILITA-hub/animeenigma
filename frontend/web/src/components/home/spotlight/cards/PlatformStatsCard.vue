<template>
  <article class="platform-stats-hero">
    <!-- Neon Tokyo two-column hero-stats layout (feat/homepage-neon-tokyo-redesign).
         Transcribed from .hero-stats, .hero-stats .left, .hero-stats .grid,
         .stat-tile etc. in design_handoff_homepage_redesign/styles.css.
         All data bindings are unchanged — only presentation/layout changed. -->

    <!-- LEFT column: status headline + uptime + vibe row + tagline -->
    <div class="stats-left">
      <h2 class="stats-headline">
        Работает:
        <span :class="hero.working_ok ? 'stats-ok' : 'stats-warn'">
          {{ hero.working_ok ? 'ДА' : 'ТЕХНИЧЕСКИ ДА' }}
        </span>
      </h2>

      <p class="stats-uptime">
        Аптайм: {{ hero.uptime_quip
        }}<template v-if="hero.uptime_percent != null"> — {{ hero.uptime_percent }}%</template>
      </p>

      <p class="stats-vibe">
        {{ hero.service }} — UXΔ {{ hero.ux_delta }} · CDI {{ hero.cdi }} · MVQ {{ hero.mvq }}
      </p>

      <blockquote class="stats-quote">«{{ hero.tagline }}»</blockquote>
    </div>

    <!-- RIGHT column: 2×2 stat tile grid -->
    <ul class="stats-grid">
      <li
        v-for="tile in tiles"
        :key="tile.label"
        class="stat-tile"
      >
        <p class="stat-tile-label">{{ windowLabel(tile.window) }}</p>
        <p class="stat-tile-value">{{ formatValue(tile) }}</p>
        <p class="stat-tile-sub">{{ tile.label }}</p>
      </li>
    </ul>
  </article>
</template>

<script setup lang="ts">
// Workstream hero-spotlight — Neon Tokyo restyle of PlatformStatsCard
// (feat/homepage-neon-tokyo-redesign, Task 5). Layout updated to the
// two-column hero-stats variant from the design handoff. All data
// bindings, computed fields, and type imports are unchanged.
// SINGLE-ROOT <article>, NO top-level v-if (Transition mode="out-in" safety).
import { computed } from 'vue'
import type { PlatformStatsData, StatsTile } from '@/types/spotlight'

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

<style scoped>
/* Neon Tokyo two-column hero-stats layout.
   Transcribed from .hero-stats, .hero-stats .left, .hero-stats .grid,
   .stat-tile, .stat-tile::after in design_handoff_homepage_redesign/styles.css. */

.platform-stats-hero {
  position: absolute;
  inset: 0;
  padding: 40px 48px;
  display: grid;
  grid-template-columns: 1.1fr 1fr;
  gap: 32px;
  background:
    radial-gradient(700px 400px at 20% 80%, rgba(0, 212, 255, 0.1), transparent 60%),
    linear-gradient(135deg, #0d2030 0%, #050a12 100%);
  overflow: hidden;
}

/* LEFT column */
.stats-left {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 16px;
}

.stats-headline {
  font-family: var(--f-display);
  font-size: 48px;
  font-weight: 800;
  line-height: 1.05;
  letter-spacing: -0.025em;
  color: var(--foreground);
}

.stats-ok {
  /* success green per handoff (--color-success) */
  color: var(--color-success);
}

.stats-warn {
  color: var(--color-warning);
}

.stats-uptime {
  font-family: var(--f-mono);
  font-size: 13px;
  color: var(--brand-cyan);
  letter-spacing: 0.04em;
}

.stats-vibe {
  font-family: var(--f-mono);
  font-size: 11px;
  color: var(--ink-4);
  letter-spacing: 0;
  line-height: 1.4;
}

.stats-quote {
  font-style: italic;
  font-size: 14px;
  color: var(--ink-2);
  border-left: 2px solid var(--brand-cyan);
  padding-left: 14px;
  max-width: 380px;
}

/* RIGHT column: 2×2 tile grid */
.stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  align-content: center;
}

.stat-tile {
  position: relative;
  padding: 18px 18px 16px;
  border-radius: var(--r-lg);
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--line);
  overflow: hidden;
}

.stat-tile-label {
  font-family: var(--f-mono);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: var(--brand-cyan);
  margin-bottom: 6px;
}

.stat-tile-value {
  font-family: var(--f-display);
  font-size: 32px;
  font-weight: 700;
  line-height: 1.05;
  letter-spacing: -0.02em;
  color: var(--foreground);
}

.stat-tile-sub {
  font-size: 12px;
  color: var(--muted-foreground);
  margin-top: 6px;
}

/* Top-right radial highlight per tile */
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

/* Mobile: stack columns vertically */
@media (max-width: 768px) {
  .platform-stats-hero {
    grid-template-columns: 1fr;
    padding: 24px;
    gap: 20px;
    overflow-y: auto;
  }
  .stats-headline {
    font-size: 32px;
  }
  .stat-tile-value {
    font-size: 24px;
  }
}
</style>
