---
phase: 07-latest-news-refactor
plan: 07
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-LN-01, HSB-V11-LN-02, HSB-V11-LN-03, HSB-V11-LN-04]
blocked_by: [01]
status: ready
---

# Plan 07: LatestNewsCard refactor — type-coded entries with relative dates

## Goal

Give changelog entries visual hierarchy via type-icons (feat/fix/perf),
type pills, relative dates, and drop the fragile title-split regex in
favor of a simple character-count fallback.

## Tasks

### Task 1 — Backdrop + header with sparkles icon

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="gradient-mesh" accent="amber" />
  <div class="relative z-10 w-full h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8">
    <header class="flex items-baseline justify-between">
      <div class="flex items-center gap-2">
        <SpotlightIcon name="sparkles" class="w-5 h-5 text-amber-300" />
        <h3 class="text-lg md:text-xl font-semibold text-white">
          {{ t('spotlight.latestNews.title') }}
        </h3>
      </div>
      <button
        type="button"
        class="cta-text"
        @click="scrollToChangelog"
      >
        {{ t('spotlight.latestNews.readMore') }}
        <SpotlightIcon name="play" class="w-3 h-3 rotate-90" />
      </button>
    </header>
    <ul class="flex-1 grid grid-cols-1 md:grid-cols-3 gap-3 md:gap-4">
      <!-- entries -->
    </ul>
  </div>
</article>
```

### Task 2 — Type icons + type pills

```ts
const iconByType: Record<string, SpotlightIconName> = {
  feat:  'sparkles',
  fix:   'wrench',
  perf:  'lightning',
  docs:  'sparkles',  // fallback for less common types
}

const labelByType: Record<string, { i18nKey: string; accent: string }> = {
  feat:  { i18nKey: 'spotlight.latestNews.typeFeat',  accent: 'bg-cyan-500/20 text-cyan-200' },
  fix:   { i18nKey: 'spotlight.latestNews.typeFix',   accent: 'bg-green-500/20 text-green-200' },
  perf:  { i18nKey: 'spotlight.latestNews.typePerf',  accent: 'bg-amber-500/20 text-amber-200' },
}
```

i18n keys added:

```json
"typeFeat": "Новая фича",
"typeFix":  "Исправление",
"typePerf": "Улучшение"
```

(EN: "Feature", "Bug fix", "Performance"; JA per locale.)

### Task 3 — Relative dates via Intl.RelativeTimeFormat

```ts
function formatEntryDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  const diffDays = Math.round(diffMs / 86_400_000)
  try {
    const rtf = new Intl.RelativeTimeFormat(localeStr.value, { numeric: 'auto' })
    if (Math.abs(diffDays) < 1) return rtf.format(0, 'day')   // "today"
    if (Math.abs(diffDays) < 7) return rtf.format(diffDays, 'day')
    if (Math.abs(diffDays) < 30) return rtf.format(Math.round(diffDays / 7), 'week')
    return new Intl.DateTimeFormat(localeStr.value, { dateStyle: 'medium' }).format(d)
  } catch {
    return iso
  }
}
```

### Task 4 — Drop sentence-splitter regex

Replace the regex with a simple length-based truncation. The backend's
`latest_news` resolver emits `{date, type, message}` — frontend treats
message as plain body text:

```ts
function entryTitle(msg: string): string {
  return msg.length > 60 ? msg.slice(0, 60).trimEnd() + '…' : msg
}
function entryBody(_msg: string): string {
  // No body — title now carries the message. Body field deprecated.
  return ''
}
```

Optional backend follow-up (not in this phase): make resolver emit a
proper `title` + `body` schema so frontend can fully drop the truncation
heuristic. Tracked as v1.2 candidate.

### Task 5 — Entry render

```vue
<li
  v-for="(entry, idx) in data.entries.slice(0, 3)"
  :key="entry.date + ':' + idx"
  class="flex flex-col gap-2 p-3 rounded-xl bg-black/30 backdrop-blur-sm hover:bg-black/40 transition min-w-0"
>
  <div class="flex items-center justify-between gap-2">
    <SpotlightIcon
      :name="iconByType[entry.type] ?? 'sparkles'"
      class="w-4 h-4"
      :class="labelByType[entry.type]?.accent ?? 'text-gray-300'"
    />
    <span
      v-if="labelByType[entry.type]"
      class="text-[10px] font-semibold uppercase tracking-wider px-2 py-0.5 rounded"
      :class="labelByType[entry.type].accent"
    >
      {{ t(labelByType[entry.type].i18nKey) }}
    </span>
  </div>
  <p class="text-xs font-medium text-gray-400">{{ formatEntryDate(entry.date) }}</p>
  <p class="text-sm font-semibold text-white line-clamp-3 flex-1">
    {{ entryTitle(entry.message) }}
  </p>
</li>
```

### Task 6 — Spec updates

`LatestNewsCard.spec.ts`:

- `<SpotlightIcon>` renders with `name="wrench"` for entry where `type === 'fix'`.
- Type pill renders with `bg-green-500/20` for fix type.
- Date "2026-05-19" rendered as a non-ISO string (relative format applied).
- Title-split regex removed — assert no regex in component source (`grep -c '\\^(\\.+?\\[' LatestNewsCard.vue` → 0).

## Verification

- `bunx vitest run src/components/home/spotlight/cards/LatestNewsCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: cycle to LatestNews, confirm 3 entries each have type icon + pill + relative date.

## Metrics

`UXΔ = +2 (Better) · CDI = 0.03 * 5 · MVQ = Sprite 75%/80%`
