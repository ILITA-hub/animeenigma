<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop variant="gradient-mesh" accent="amber" />

    <div
      class="relative z-10 w-full h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8"
    >
      <header class="flex items-baseline justify-between">
        <div class="flex items-center gap-2">
          <SpotlightIcon
            name="sparkles"
            class="w-5 h-5 text-warning"
          />
          <h3
            class="text-lg md:text-xl font-semibold text-white"
          >
            {{ t('spotlight.latestNews.title') }}
          </h3>
        </div>
        <a
          href="#changelog"
          class="text-sm font-medium text-cyan-400 hover:text-cyan-300 transition-colors inline-flex items-center gap-1"
          @click.prevent="scrollToChangelog"
        >
          {{ t('spotlight.latestNews.readMore') }}
          <SpotlightIcon
            name="play"
            class="w-3 h-3 rotate-90"
          />
        </a>
      </header>

      <ul
        class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4"
      >
        <li
          v-for="(entry, idx) in data.entries.slice(0, 3)"
          :key="entry.date + ':' + idx"
          class="news-tile flex flex-col gap-2 p-3 rounded-xl backdrop-blur-sm transition min-w-0"
        >
          <div class="flex items-center justify-between gap-2">
            <SpotlightIcon
              :name="iconFor(entry.type)"
              class="w-4 h-4"
              :class="iconAccentClassFor(entry.type)"
            />
            <span
              v-if="badgeFor(entry.type)"
              class="text-[10px] font-semibold uppercase tracking-wider px-2 py-0.5 rounded"
              :class="badgeFor(entry.type)!.accent"
            >
              {{ t(badgeFor(entry.type)!.i18nKey) }}
            </span>
          </div>
          <p class="text-xs font-medium news-date">
            {{ formatEntryDate(entry.date) }}
          </p>
          <p
            class="text-sm font-semibold text-white news-msg flex-1"
          >
            {{ entryTitle(entry.message) }}
          </p>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import {
  cardTokens,
  type LatestNewsTypeBadge,
  type SpotlightIconName,
} from '../tokens'
import type { LatestNewsData } from '@/types/spotlight'

defineProps<{ data: LatestNewsData }>()
const { t, locale: i18nLocale } = useI18n()

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const latestNewsToken = cardTokens.latest_news

// Per-type helpers. Each accepts `string | undefined` because the
// backend ChangelogEntry.type is optional and the per-entry payload may
// legitimately omit it (very old entries). Unknown types fall back to a
// sparkles icon with gray accent and no pill (badgeFor returns null).
function iconFor(type?: string): SpotlightIconName {
  if (type && latestNewsToken.iconByType[type]) {
    return latestNewsToken.iconByType[type]
  }
  return 'sparkles'
}

function badgeFor(type?: string): LatestNewsTypeBadge | null {
  if (!type) return null
  return latestNewsToken.labelByType[type] ?? null
}

function iconAccentClassFor(type?: string): string {
  const badge = badgeFor(type)
  return badge ? badge.accent : 'text-muted-foreground'
}

// Locale-aware relative date. Falls back to a medium absolute date when
// the entry is older than ~30 days, and to the raw ISO string when the
// date is unparseable (defensive: backend emits YYYY-MM-DD).
function formatEntryDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  const diffDays = Math.round(diffMs / 86_400_000)
  try {
    const rtf = new Intl.RelativeTimeFormat(localeStr.value, { numeric: 'auto' })
    if (Math.abs(diffDays) < 1) return rtf.format(0, 'day') // "today"
    if (Math.abs(diffDays) < 7) return rtf.format(diffDays, 'day')
    if (Math.abs(diffDays) < 30) return rtf.format(Math.round(diffDays / 7), 'week')
    return new Intl.DateTimeFormat(localeStr.value, { dateStyle: 'medium' }).format(d)
  } catch {
    return iso
  }
}

function scrollToChangelog(): void {
  const el = document.getElementById('changelog')
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// Show the full changelog message; visual truncation (with an ellipsis) is
// handled by the CSS line-clamp on the tile, which is sized to the tile's
// height. The previous hard 60-char slice cropped the 150–210 char changelog
// entries down to ~2 lines even though each tile has room for ~11 — that was
// the "cropped too much" the slice caused. A proper backend title/body split
// is still in the v1.2 backlog; until then the whole message is the teaser.
function entryTitle(msg: string): string {
  return msg || ''
}
</script>

<style scoped>
/* Neon Tokyo token replacements (feat/homepage-neon-tokyo-redesign). */
.news-tile {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid var(--line);
}
.news-tile:hover {
  background: rgba(255, 255, 255, 0.07);
}
.news-date { color: var(--muted-foreground); }

/* Show up to 7 lines of the changelog message, then truncate with an
   ellipsis. Defined here (not via a Tailwind line-clamp-[7] utility) because
   Tailwind v4 did not emit that arbitrary value — the explicit rule is
   guaranteed. Each tile is tall (~11 lines fit); 7 shows the full 150–210 char
   entries while keeping the three tiles visually balanced. */
.news-msg {
  display: -webkit-box;
  -webkit-line-clamp: 7;
  line-clamp: 7;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
