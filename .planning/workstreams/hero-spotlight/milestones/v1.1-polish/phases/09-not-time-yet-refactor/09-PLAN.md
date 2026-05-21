---
phase: 09-not-time-yet-refactor
plan: 09
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-NT-01, HSB-V11-NT-02, HSB-V11-NT-03, HSB-V11-NT-04]
blocked_by: [01]
status: ready
---

# Plan 09: NotTimeYetCard refactor — amber nostalgia identity

## Goal

Make NotTimeYetCard distinct from AnimeOfDayCard via amber/clock theming,
a status pill (planned vs postponed), a "Last added X ago" timestamp, and
a direct-to-watch CTA. Requires a small backend pass-through.

## Tasks

### Task 1 — Backend pass-through for added_at

`services/catalog/internal/service/spotlight/cards/not_time_yet.go`:

`PlayerClient.FetchListByStatuses` already returns rows that include
`updated_at` from `anime_list`. Surface it as `AddedAt` in `NotTimeYetData`:

```go
type NotTimeYetData struct {
  Anime   AnimeRef   `json:"anime"`
  Status  string     `json:"status"`
  AddedAt *time.Time `json:"added_at,omitempty"`  // NEW
}
```

No new query, no schema change — just struct extension + JSON tag.

### Task 2 — Frontend: amber theming + clock icon

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="poster-blur" :poster-url="data.anime.poster_url" />
  <!-- Amber secondary overlay -->
  <div aria-hidden="true" class="absolute inset-0 bg-gradient-to-r from-amber-500/30 via-transparent to-transparent" />
  <div class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-6 lg:p-8 md:items-center">
    <!-- Poster -->
    <router-link
      :to="`/anime/${data.anime.id}`"
      class="flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-52 group"
    >
      <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-amber-500/20">
        <img
          :src="data.anime.poster_url || '/placeholder.svg'"
          :alt="title"
          class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
          loading="lazy"
        />
      </div>
    </router-link>

    <!-- Meta -->
    <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
      <div>
        <!-- Promoted header -->
        <div class="flex items-center gap-2 mb-3">
          <SpotlightIcon name="clock" class="w-5 h-5 text-amber-300" />
          <p class="text-amber-200 text-sm font-semibold uppercase tracking-[0.15em]">
            {{ t('spotlight.notTimeYet.title') }}
          </p>
        </div>

        <!-- Status pill -->
        <span class="inline-flex items-center gap-1 mb-2 px-2.5 py-1 rounded-md text-xs font-semibold" :class="statusPillClass">
          {{ statusLabel }}
        </span>

        <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>

        <p v-if="data.anime.episodes_count" class="mt-2 text-sm text-gray-400 font-medium">
          {{ t('spotlight.animeOfDay.episodesLabel', { n: data.anime.episodes_count }) }}
        </p>

        <p v-if="data.added_at" class="mt-1 text-xs text-amber-300/70 font-medium">
          {{ t('spotlight.notTimeYet.addedAt', { ago: formatAgo(data.added_at) }) }}
        </p>
      </div>

      <router-link :to="`/anime/${data.anime.id}/watch`" class="cta-hero" data-accent="amber">
        {{ t('spotlight.notTimeYet.watchCta') }}
        <SpotlightIcon name="play" class="w-4 h-4" />
      </router-link>
    </div>
  </div>
</article>
```

### Task 3 — Status pill mapping

```ts
const statusLabel = computed(() => data.status === 'planned'
  ? t('spotlight.notTimeYet.statusPlanned')
  : t('spotlight.notTimeYet.statusPostponed'))

const statusPillClass = computed(() => data.status === 'planned'
  ? 'bg-yellow-500/20 text-yellow-200'
  : 'bg-slate-500/20 text-slate-300')
```

i18n keys added:
- `statusPlanned`: "В планах" / "Planned" / "計画中"
- `statusPostponed`: "Отложено" / "Postponed" / "延期"
- `addedAt`: "Добавлено {ago}" / "Added {ago}" / "{ago}に追加"

### Task 4 — `formatAgo` helper

Reuse the same `Intl.RelativeTimeFormat` logic from Plan 07. Consider
extracting to `frontend/web/src/utils/time.ts` so both LatestNews and
NotTimeYet share it.

### Task 5 — Direct-to-watch CTA

CTA href is `/anime/{data.anime.id}/watch` (not `/anime/{id}`). Honors the
intent: the user has already bookmarked the anime; bring them straight to
the player.

### Task 6 — Spec updates

`NotTimeYetCard.spec.ts`:

- Renders `<SpotlightIcon name="clock">` in header.
- Status pill text === "В планах" when `status === 'planned'`, "Отложено" when `'postponed'`.
- Status pill class === `bg-yellow-500/20 text-yellow-200` for planned, `bg-slate-500/20 text-slate-300` for postponed.
- CTA href ends in `/watch`.
- Relative `added_at` ("2 недели назад") rendered when `data.added_at` provided.
- Backdrop secondary overlay class contains `from-amber-500/30`.

Backend test:

`not_time_yet_test.go` asserts `AddedAt` is populated when the player
client returns a non-zero `UpdatedAt`.

## Verification

- Backend: `cd services/catalog && go test ./internal/service/spotlight/cards/... -count=1 -race` — green.
- Frontend: `bunx vitest run src/components/home/spotlight/cards/NotTimeYetCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: log in as `ui_audit_bot` (seed planned/postponed entries
  via `scripts/seed-ui-audit-user.sh` if absent), cycle to NotTimeYet,
  confirm amber theming + status pill + addedAt + direct-watch CTA.

## Metrics

`UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Sprite 82%/80%`
