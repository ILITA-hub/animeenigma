<template>
  <article
    class="w-full h-full flex flex-col gap-4 p-4 md:p-4 lg:p-6"
  >
    <header class="flex items-baseline justify-between">
      <h3 class="text-lg md:text-xl font-semibold text-white">
        {{ t('spotlight.latestNews.title') }}
      </h3>
      <a
        href="#changelog"
        class="text-sm font-medium text-cyan-400 hover:text-cyan-300 transition-colors"
        @click.prevent="scrollToChangelog"
      >
        {{ t('spotlight.latestNews.readMore') }}
      </a>
    </header>

    <ul
      class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4"
    >
      <li
        v-for="(entry, idx) in data.entries.slice(0, 3)"
        :key="entry.date + ':' + idx"
        class="flex flex-col p-3 rounded-xl bg-white/5 hover:bg-white/10 transition-colors min-w-0"
      >
        <p class="text-xs font-medium text-gray-500 mb-1">
          {{ formatEntryDate(entry.date) }}
        </p>
        <p
          class="text-sm md:text-base font-semibold text-white line-clamp-2 mb-1"
        >
          {{ entryTitle(entry.message) }}
        </p>
        <p
          v-if="entryBody(entry.message)"
          class="text-xs text-gray-400 line-clamp-3 flex-1 font-medium"
        >
          {{ entryBody(entry.message) }}
        </p>
      </li>
    </ul>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LatestNewsData } from '@/types/spotlight'

defineProps<{ data: LatestNewsData }>()
const { t, locale: i18nLocale } = useI18n()

const localeStr = computed<string>(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

// Locale-aware medium date format: "May 21, 2026" / "21 мая 2026 г." /
// "2026年5月21日". Falls back to the raw ISO string if the input is
// malformed (defensive — the backend emits YYYY-MM-DD).
function formatEntryDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  try {
    return new Intl.DateTimeFormat(localeStr.value, { dateStyle: 'medium' }).format(d)
  } catch {
    return iso
  }
}

function scrollToChangelog(): void {
  const el = document.getElementById('changelog')
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// The backend's Phase 1 resolver emits `{date, type, message}` per entry —
// no separate title/summary fields. We split `message` at the first sentence
// boundary so the visual hierarchy in UI-SPEC (line-clamp-2 title +
// line-clamp-3 summary) still has two distinct strings to render.
//
// "Title" = first sentence (up to and including the first period, question
// mark, or em dash). "Body" = the remainder. If no sentence break exists,
// the whole message is the title and the body is empty.
function splitMessage(msg: string): { title: string; body: string } {
  const match = msg.match(/^(.+?[.!?——–:](?:\s|$))(.*)$/s)
  if (!match) return { title: msg.trim(), body: '' }
  return { title: match[1].trim(), body: match[2].trim() }
}

function entryTitle(msg: string): string {
  return splitMessage(msg).title
}

function entryBody(msg: string): string {
  return splitMessage(msg).body
}
</script>
