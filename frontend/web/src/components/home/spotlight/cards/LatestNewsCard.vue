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
            class="w-5 h-5 text-amber-300"
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
          class="flex flex-col gap-2 p-3 rounded-xl bg-black/30 backdrop-blur-sm hover:bg-black/40 transition min-w-0"
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
          <p class="text-xs font-medium text-gray-400">
            {{ formatEntryDate(entry.date) }}
          </p>
          <p
            class="text-sm font-semibold text-white line-clamp-3 flex-1"
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
  return badge ? badge.accent : 'text-gray-300'
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

// Title = first 60 chars + ellipsis if longer. The Phase 02
// sentence-splitter regex is gone (see SUMMARY for the rationale) —
// it produced confusing splits on emoji and ASCII colons. A proper
// backend title/body split lives in the v1.2 backlog.
function entryTitle(msg: string): string {
  return msg.length > 60 ? msg.slice(0, 60).trimEnd() + '…' : msg
}
</script>
