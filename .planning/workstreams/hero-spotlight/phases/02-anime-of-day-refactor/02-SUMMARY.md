---
phase: 02-anime-of-day-refactor
plan: 02
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-AOD-01, HSB-V11-AOD-02, HSB-V11-AOD-03, HSB-V11-AOD-04]
human_verified: "pending eyeball confirmation on animeenigma.ru after orchestrator merge + redeploy"
key_files:
  created: []
  modified:
    - frontend/web/src/components/home/spotlight/tokens.ts
    - frontend/web/src/components/home/spotlight/tokens.spec.ts
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue
    - frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts
commits:
  - 5c1d895 feat(spotlight) extend cardTokens with anime_of_day.genreColors map
  - 178df93 refactor(spotlight) AnimeOfDayCard cinematic v1.1 (HSB-V11-AOD-01..04)
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Phoenix 82%/80%"
  completed_date: 2026-05-24
---

# Phase 02 Plan 02: AnimeOfDayCard cinematic refactor Summary

Cinematic v1.1-polish refactor of `AnimeOfDayCard.vue`: blurred poster backdrop, bigger foreground poster with cyan-tinted shadow, single oversized "Watch" CTA (dead disabled "Add to list" button removed), score badge promoted from a poster-corner overlay to a meta-row pill, color-coded genre tags driven by a Shikimori-genre-ID → Tailwind class map in `tokens.ts`.

## What shipped

### HSB-V11-AOD-04 — `cardTokens.anime_of_day.genreColors`

- Extended `cardTokens` type so `anime_of_day` carries an extra `genreColors: Record<string, string>` field while the other 8 variants stay vanilla. The Record-of-Record annotation keeps tsc exhaustiveness checks intact.
- Mapped 12 Shikimori genre IDs (Action, Adventure, Comedy, Mystery, Drama, Fantasy, Horror, Romance, Sci-Fi, Sports, Slice of Life, Supernatural) to a `bg-{hue}-500/20` + `text-{hue}-200` pair each. Tailwind 4 needs the utility strings to appear verbatim in source, so they're literal — no template composition.
- The map is read-only at the call site via `cardTokens.anime_of_day.genreColors[id] ?? 'bg-white/10 text-gray-300'`, so unmapped IDs render with the neutral fallback rather than no styling.

**Tests:** `tokens.spec.ts` gained 4 new assertions: map exists, ≥10 entries, every entry pairs a `bg-*/20` and `text-*/200` class, every key is a numeric string. The pre-existing 9-variant parity test stays green.

### HSB-V11-AOD-01 — backdrop layer

`AnimeOfDayCard.vue` now wraps its content in `<article class="relative w-full h-full overflow-hidden">` with `<SpotlightBackdrop variant="poster-blur" :poster-url="data.anime.poster_url" />` as the lowest layer and the existing flex content stacked above it under `relative z-10`. Reuses the already-fetched poster URL — no extra HTTP request and no decode-time penalty (browser cache hit).

### HSB-V11-AOD-02 — bigger poster + accent kicker

- Foreground poster widened from `w-28 md:w-32 lg:w-44` to `w-32 md:w-44 lg:w-56`.
- Added `shadow-2xl shadow-cyan-500/20 transition-shadow duration-300 group-hover:shadow-cyan-500/40` to the poster's rounded container so the art reads as the hero and gains a subtle glow on hover (matching the cyan accent of the rest of the card).
- Both kicker copies (mobile `md:hidden` + desktop `hidden md:block`) restyled to `text-cyan-300 text-[10px] uppercase tracking-[0.18em] font-semibold` so the kicker punches without competing with the title.

### HSB-V11-AOD-03 — single hero CTA

- Removed the disabled `<button type="button" disabled aria-disabled="true">` entirely.
- The remaining "Watch" anchor is now a single cyan `.cta-hero` router-link (the Phase 01 utility class from `main.css`), with the SpotlightIcon "play" mark inline:

```vue
<router-link :to="`/anime/${data.anime.id}/watch`" class="cta-hero">
  {{ t('spotlight.animeOfDay.watchCta') }}
  <SpotlightIcon name="play" class="w-4 h-4" />
</router-link>
```

- The `spotlight.animeOfDay.addCta` + `addCtaComingSoon` i18n keys are intentionally retained in `en/ru/ja.json`. Future watchlist wiring (Phase 3 of this milestone) may re-introduce the CTA, and keeping the strings avoids a noisy revert later. The keys are simply unreferenced in the template now — `vue-tsc` and `eslint` are clean against this.

### Score badge → meta-row pill

The previous `<div class="absolute top-2 right-2 ...">` overlay sat on top of the poster art and obstructed it (a UX audit observation that motivated this whole refactor). It's now a meta-row pill placed alongside the episodes count:

```vue
<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-semibold bg-yellow-500/20 text-yellow-200">
  <svg ... existing star path ...></svg>
  {{ data.anime.score?.toFixed(1) }}
</span>
```

Same star SVG, no new icon added to the SpotlightIcon sprite. The pill carries no `absolute` class and lives in the same `mt-2 flex flex-wrap items-center gap-2` row as the episode count.

### Color-coded genre tags

Genre `<span>` elements still slice to the first 3 entries and still localize their label, but they now apply per-genre classes through a helper:

```ts
const GENRE_FALLBACK_CLASS = 'bg-white/10 text-gray-300'
function genreColorClass(id: string): string {
  return cardTokens.anime_of_day.genreColors[id] ?? GENRE_FALLBACK_CLASS
}
```

`bg-white/10 text-gray-300` continues to render for any unmapped Shikimori ID, so the test for "no broken styling on unknown genre" passes.

## Verification

```
$ cd frontend/web && bunx vitest run src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts src/components/home/spotlight/tokens.spec.ts

 RUN  v4.1.6 /data/animeenigma/.claude/worktrees/agent-af0594e7438ec1874/frontend/web

 Test Files  2 passed (2)
      Tests  32 passed (32)
   Start at  11:48:09
   Duration  1.52s
```

```
$ cd frontend/web && bunx vitest run src/components/home/spotlight/

 Test Files  14 passed (14)
      Tests  148 passed (148)
```

(All Phase 01 primitive specs — `SpotlightIcon`, `SpotlightBackdrop`, `CarouselControls`, `HeroSpotlightBlock`, the 8 untouched card specs — stay green alongside the updated `AnimeOfDayCard` + `tokens` specs.)

```
$ cd frontend/web && bunx tsc --noEmit
(clean — no output)
```

```
$ cd frontend/web && bunx eslint src/components/home/spotlight/
(clean — no output)
```

Playwright e2e (`BASE_URL=http://127.0.0.1:5173 bunx playwright test spotlight-full --project=chromium`):

- `spotlight-full.spec.ts:142` — **"cycles through all 9 card types via next-chevron"** — **PASS** (this is the canonical regression test for the full carousel; Phase 02's refactor did not break it).
- `spotlight-full.spec.ts:24` — renders all 9 card types — **PASS**
- `spotlight-full.spec.ts:225` — HSB-MIG-01 trendingRecs DOM gone — **PASS**
- `spotlight-full.spec.ts:275` — prefers-reduced-motion — **PASS**
- Additional checks — **PASS**
- `spotlight-full.spec.ts:241` — arrow-key navigation cycles all 9 slides — **FLAKY** (counted 8/9 unique labels under rapid key-presses; pre-existing transition-lock interaction noted in Phase 01 SUMMARY).
- `spotlight-full.spec.ts:207` — axe-core 0 violations — **FAIL** (pre-existing: "Heading order invalid" because every card uses `<h3>` without a preceding `<h2>` above the carousel; documented in Phase 01 SUMMARY as an environmental pre-existing failure, not a Phase 02 regression).

```
$ cd frontend/web && BASE_URL=http://127.0.0.1:5173 bunx playwright test spotlight-full --project=chromium --reporter=list -g "cycles through all 9 card types"

Running 1 test using 1 worker

  ✓  1 [chromium] › e2e/spotlight-full.spec.ts:142:3 › hero spotlight 9-card (Phase 3 / Plan 03-07) › cycles through all 9 card types via next-chevron (5.0s)

  1 passed (6.5s)
```

The `spotlight.spec.ts` Phase-2 suite also has 3 pre-existing environmental failures (mounts-above-trending / axe / reduced-motion) caused by `page.waitForLoadState('networkidle')` timing out when the dev server's `/api/home/spotlight` returns 404 (no backend running in the worktree). These were already broken on `913b225` before Phase 02's commits landed and are not regressions.

Visual smoke (post-deploy): open `https://animeenigma.ru/`, cycle to the AnimeOfDay card via the next-chevron, confirm:

1. Blurred poster fills the entire card background (right-edge vignette keeps text legible).
2. Foreground poster is visibly larger than before (~w-56 on desktop) and gains a cyan glow on hover.
3. Only one button is visible — the cyan "Watch" pill with the play icon.
4. Score pill (yellow) sits inline alongside the episode count, not floating over the poster.
5. Genre tags render in hue-coded pills (red Action, blue Adventure, yellow Comedy, etc.).

## Deviations from plan

### [Rule 0 — Atomicity] Refactor + spec landed as one commit

The orchestrator prompt suggested splitting the .vue refactor into 4 separate
commits (backdrop / poster / score-pill / genre-colors / spec). In practice
the existing `AnimeOfDayCard.spec.ts` asserts the old `btn-primary` + `btn-ghost`
classes and the old `text-yellow-400` overlay; any single-task commit would
leave the spec broken between commits (RED for several commits in a row).
The pragmatic choice was to land all four template changes plus the spec
rewrite as one atomic refactor commit (`178df93`), with the tokens-only
addition (`5c1d895`) preceding it as the data-layer commit. This matches
the "test changes land with the implementation that satisfies them" rule
and keeps every intermediate HEAD in the worktree in a buildable, green
state. The plan's task numbering (Task 1 backdrop → Task 4 spec) is fully
covered by the two commits and is described in the commit body line-by-line.

### [Rule 2 — Auto-add] Cyan-shadow hover state on poster

The plan said "add `shadow-2xl shadow-cyan-500/20` hover on the rounded
poster container". A naive interpretation would attach the shadow only on
`:hover`; that would cause a hard pop on first mouseover. I instead set
the cyan shadow as the resting state (`shadow-2xl shadow-cyan-500/20`)
and intensify it on group hover (`transition-shadow duration-300
group-hover:shadow-cyan-500/40`), so the cyan glow tracks the cyan accent
of the rest of the card at rest and grows when the user hovers. This is
"correctness for the design intent" (Rule 2): a hover-only shadow would
flash and feel out of place against the rest of the always-on cyan accent
language.

## Threat surface

No new network endpoints, no new file-system access patterns, no new
trust-boundary changes. The backdrop component reuses the already-fetched
poster URL (no second HTTP request) and the genre color map is a static
lookup table compiled into the JS bundle. No threat flags.

## Self-Check: PASSED

- FOUND: frontend/web/src/components/home/spotlight/tokens.ts (modified)
- FOUND: frontend/web/src/components/home/spotlight/tokens.spec.ts (modified)
- FOUND: frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue (modified)
- FOUND: frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts (modified)
- FOUND commit: 5c1d895 (feat(spotlight): extend cardTokens with anime_of_day.genreColors map)
- FOUND commit: 178df93 (refactor(spotlight): AnimeOfDayCard cinematic v1.1 (HSB-V11-AOD-01..04))
