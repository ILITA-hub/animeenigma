<template>
  <SpotlightCardShell
    accent="violet"
    icon="code"
    :kicker="t('spotlight.latestNews.title')"
  >
  <!--
    Workstream hero-spotlight — v4 E-1 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Terminal changelog:
    `$ animeenigma --updates` prompt, entries as mono lines with colored
    [FEAT]/[FIX]/[PERF] prefixes (semantic tokens: cyan/success/warning),
    blinking cursor. Replaces the three-tile "wall of text" layout —
    instantly distinguishable from TelegramNewsCard.
  -->
    <div
      class="flex-1 min-h-0 bg-black/45 border border-white/10 rounded-xl px-5 py-4 font-mono text-[12.5px] leading-[1.75] overflow-hidden"
      data-testid="terminal"
    >
      <p class="text-muted-foreground">$ animeenigma --updates</p>
      <ul class="mt-2">
        <li
          v-for="(entry, idx) in data.entries.slice(0, 3)"
          :key="entry.date + ':' + idx"
          class="news-msg"
          data-testid="terminal-line"
        >
          <span class="text-muted-foreground">{{ formatEntryDate(entry.date) }}</span>
          {{ ' ' }}
          <span :class="prefixFor(entry.type).cls">[{{ prefixFor(entry.type).label }}]</span>
          {{ ' ' }}{{ entry.message }}
        </li>
      </ul>
      <p class="mt-2">
        <span class="text-muted-foreground">$</span>
        <span class="cursor-blink" aria-hidden="true" />
      </p>
    </div>

    <template #cta>
      <a
        href="#changelog"
        :class="[buttonVariants({ variant: 'link', size: 'sm' }), 'text-sm']"
        @click.prevent="scrollToChangelog"
      >
        {{ t('spotlight.latestNews.readMore') }}
        <ArrowDown class="w-3.5 h-3.5" aria-hidden="true" />
      </a>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowDown } from 'lucide-vue-next'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import type { LatestNewsData } from '@/types/spotlight'

defineProps<{ data: LatestNewsData }>()
const { t, locale: i18nLocale } = useI18n()

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// Terminal prefixes — the changelog `type` field mapped to mono labels
// in semantic accents. Unknown/missing types render a muted [INFO].
const PREFIXES: Record<string, { label: string; cls: string }> = {
  feature: { label: 'FEAT', cls: 'text-cyan-400' },
  fix: { label: 'FIX', cls: 'text-success' },
  perf: { label: 'PERF', cls: 'text-warning' },
}
const FALLBACK_PREFIX = { label: 'INFO', cls: 'text-muted-foreground' }

function prefixFor(type?: string): { label: string; cls: string } {
  return (type && PREFIXES[type]) || FALLBACK_PREFIX
}

// Compact dd.MM date for the terminal gutter; raw string fallback when
// unparseable (backend emits YYYY-MM-DD).
function formatEntryDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  try {
    return new Intl.DateTimeFormat(localeStr.value, { day: '2-digit', month: '2-digit' }).format(d)
  } catch {
    return iso
  }
}

function scrollToChangelog(): void {
  const el = document.getElementById('changelog')
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>

<style scoped>
/* Two terminal lines per entry, then ellipsis. Explicit rule (not a
   line-clamp-[2] utility) — Tailwind v4 did not reliably emit the
   arbitrary value here (same precedent as the old 7-line clamp). */
.news-msg {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
/* Blinking block cursor after the trailing prompt. steps(1) = hard
   on/off like a real terminal, no fade. */
.cursor-blink {
  display: inline-block;
  width: 8px;
  height: 15px;
  margin-left: 6px;
  vertical-align: text-bottom;
  background: var(--brand-cyan);
  animation: cursor-blink 1.1s steps(1) infinite;
}
@keyframes cursor-blink {
  50% { opacity: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .cursor-blink { animation: none; }
}
</style>
