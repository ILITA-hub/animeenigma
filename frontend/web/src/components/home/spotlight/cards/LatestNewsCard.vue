<template>
  <SpotlightCardShell
    accent="violet"
    icon="sparkles"
    :kicker="t('spotlight.latestNews.title')"
  >
    <ul
      class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4 min-h-0"
    >
      <li
        v-for="(entry, idx) in data.entries.slice(0, 3)"
        :key="entry.date + ':' + idx"
        class="news-tile flex flex-col gap-2 p-3 rounded-xl bg-white/5 border border-white/10 hover:bg-white/10 backdrop-blur-sm transition min-w-0"
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
        <p class="text-xs font-medium text-muted-foreground">
          {{ formatEntryDate(entry.date) }}
        </p>
        <p
          class="text-sm font-semibold text-white news-msg flex-1"
        >
          {{ entryTitle(entry.message) }}
        </p>
      </li>
    </ul>

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
// Workstream hero-spotlight — DS alignment 2026-06-10: SpotlightCardShell
// kicker (violet), Button link-variant CTA pinned bottom-left, tile
// surfaces on bg-white/5 + border-white/10 utilities. Type pills keep the
// tokens.ts class strings (tinted-inline badges on a glass surface).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowDown } from 'lucide-vue-next'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
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
