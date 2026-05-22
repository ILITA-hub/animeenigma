---
phase: 01-foundation
plan: 01
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-CC-01, HSB-V11-CC-02, HSB-V11-CC-03, HSB-V11-CC-04, HSB-V11-CC-05, HSB-V11-CC-06]
human_verified: "pending eyeball confirmation on animeenigma.ru"
key_files:
  created:
    - frontend/web/src/components/home/spotlight/tokens.ts
    - frontend/web/src/components/home/spotlight/tokens.spec.ts
    - frontend/web/src/components/home/spotlight/SpotlightIcon.vue
    - frontend/web/src/components/home/spotlight/SpotlightIcon.spec.ts
    - frontend/web/src/components/home/spotlight/SpotlightBackdrop.vue
    - frontend/web/src/components/home/spotlight/SpotlightBackdrop.spec.ts
    - frontend/web/src/components/home/spotlight/__snapshots__/SpotlightBackdrop.spec.ts.snap
    - frontend/web/e2e/spotlight-transition-lock.spec.ts
  modified:
    - frontend/web/src/styles/main.css
    - frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue
    - frontend/web/src/components/home/spotlight/CarouselControls.vue
    - frontend/web/src/components/home/spotlight/CarouselControls.spec.ts
commits:
  - 4fe43c4 feat(spotlight) add cardTokens map + SpotlightIcon sprite (HSB-V11-CC-02,03)
  - 23db0d4 feat(spotlight) add SpotlightBackdrop with poster-blur + gradient-mesh variants (HSB-V11-CC-01)
  - aeb6fab feat(spotlight) add cta-hero/cta-card/cta-text classes (HSB-V11-CC-04)
  - 56b2f21 fix(spotlight) lock transition during fade to prevent blank-card race (HSB-V11-CC-05)
  - 793b9b3 feat(spotlight) labeled-pill dot indicators with per-type icons (HSB-V11-CC-06)
  - dd7ec8e test(spotlight) e2e regression for rapid-click transition lock
metrics:
  metric_string: "UXΔ = +3 (Better) · CDI = 0.04 * 13 · MVQ = Griffin 88%/85%"
  completed_date: 2026-05-22
---

# Phase 01 Plan 01: Foundation — tokens, backdrop, icons, CTA, transition lock Summary

Shared primitives for the v1.1-polish 9-card refactor: per-card design tokens, two-variant decorative backdrop, inline-SVG icon sprite, three CTA hierarchies, transition lock fixing the Phase 03 UAT blank-card bug, and labeled-pill dot indicators with per-type accent colors.

## What shipped

### HSB-V11-CC-02 / HSB-V11-CC-03 — `tokens.ts` + `SpotlightIcon.vue`

- `cardTokens: Record<SpotlightCardType, CardToken>` with 9 entries, one per `SpotlightCard['type']` variant. Each entry carries `{accent, kickerKey, icon}`.
- `SpotlightAccent` union: `cyan | purple | sky | amber | teal | green` (6 accents).
- `SpotlightIconName` union: 9 names (`telegram`, `sparkles`, `chart`, `pulse`, `clock`, `play`, `shuffle`, `wrench`, `lightning`).
- `accentDotBg` helper: maps accent to Tailwind utility string for the labeled-pill active state.
- `SpotlightIcon.vue` renders inline SVG for any of the 9 named icons; forwards `class` onto the SVG root via `useAttrs()` + `inheritAttrs: false`. Each icon is a uniform 24×24 viewBox using stroke-based geometry with `currentColor` so accent color flows from the parent.

**Tests:** `tokens.spec.ts` (12 assertions, parity guard across the SpotlightCard discriminated union — adding a 10th variant trips this test even if tsc misses it). `SpotlightIcon.spec.ts` (12 assertions across all 9 icon names plus class-forwarding + single-svg-per-render checks).

### HSB-V11-CC-01 — `SpotlightBackdrop.vue`

Two-variant decorative backdrop:

- `variant="poster-blur"` renders a blurred + tinted `<img>` from `posterUrl` with `filter: blur(40px) saturate(1.2); opacity: 0.4` (specified literally in the plan). Reuses the card's existing poster URL — browser cache hit, no extra HTTP request.
- `variant="gradient-mesh"` renders an `accent`-tinted dual-radial-gradient mesh (no HTTP request). 6 accent meshes are declared as static Tailwind class strings so the v4 build pipeline extracts them.
- Both variants share a right-edge vignette (`bg-gradient-to-r from-transparent via-black/30 to-black/60`) so foreground text on the left/center stays AA-readable.
- If `variant="poster-blur"` is requested with an empty `posterUrl`, falls back to `gradient-mesh` of the same accent — never paints against transparent.
- All decorative layers carry `aria-hidden="true"` + `pointer-events-none`.

**Tests:** `SpotlightBackdrop.spec.ts` (12 assertions including 7 Vitest snapshots — 1 for poster-blur with mock URL and 6 for gradient-mesh × 6 accents). Class-distinctness guard ensures the 6 accent mesh class strings never collapse to a single string.

### HSB-V11-CC-04 — CTA classes in `main.css`

Three CTA hierarchies the card refactor will reuse:

- `.cta-hero` — Primary, accent-filled bold button (e.g. "Watch →"). Default cyan, switchable via `data-accent="purple|amber|green|sky|teal"`.
- `.cta-card` — Secondary, glass-pill (`bg-white/10 hover:bg-white/20`).
- `.cta-text` — Tertiary, inline-text link (no background). Accent-switchable via the same `data-accent` attribute.

All composed via Tailwind `@apply` so the v4 class extractor sees the source utilities.

Also hoisted the `spotlight-fade` 0.4s magic number to the `--spotlight-fade-ms: 400ms` CSS var so the upcoming transition lock watchdog matches the actual fade window. The same var fuels `.spotlight-fade-enter-active` and `.spotlight-fade-leave-active` transitions.

### HSB-V11-CC-05 — Transition lock in `HeroSpotlightBlock.vue`

The Phase 03 UAT surfaced a "blank-card" bug: 10× rapid ArrowRight presses outraced the 400 ms cross-fade and left the carousel stuck in `leave-to opacity:0` with no card visible.

- `isTransitioning: Ref<boolean>` flips to `true` on `@before-leave`, to `false` on `@after-leave`.
- `next()` / `prev()` / `goTo()` early-return when `isTransitioning.value` is true.
- 600 ms watchdog `setTimeout` force-clears the lock if `@after-leave` never fires (e.g. transition cancelled by a route change). Cleared on unmount to prevent HMR leaks.
- Lock release fires on `@after-leave` (NOT `@after-enter`) — see Deviation #1 for the rationale.

### HSB-V11-CC-06 — Labeled-pill dot indicators in `CarouselControls.vue`

Replaced the row of 6 grey dots with labeled-pill buttons:

- Each pill: `w-8 h-8 rounded-full` with the card's icon from `SpotlightIcon`.
- `aria-label` + `title` tooltip = i18n kicker key (`spotlight.{cardType}.title`).
- `aria-current="true"` on the active dot, `"false"` on siblings.
- Active pill picks up `accentDotBg[token.accent]` (e.g. `bg-purple-500/30 text-purple-100`) and scales up 10%. Inactive pills stay glass-on-glass.
- Chevron tap targets enlarged from 40×40 to 44×44 (WCAG 2.5.5).

API change: `CarouselControls` now takes `cards: SpotlightCard[]` instead of `cardCount: number` so each dot can read its card's type/icon/accent. `HeroSpotlightBlock.vue` updated to pass `:cards="cards"`.

Forward-compat: if the backend ships an unknown card type the frontend doesn't yet know about, `tokenFor()` returns a `FALLBACK_TOKEN` so the dot still renders rather than throwing on undefined cardTokens access. Mirrors the existing `HeroSpotlightBlock` "unknown card type" contract.

**Tests:** `CarouselControls.spec.ts` rewritten against the new API (10 assertions, up from 7 in the v1.0 spec). E2E regression test at `e2e/spotlight-transition-lock.spec.ts` (2 scenarios: 10 rapid ArrowRight + rapid dot clicks).

## Verification

### Unit tests (verbatim)

```
$ bunx vitest run src/components/home/spotlight/tokens.spec.ts \
                  src/components/home/spotlight/SpotlightBackdrop.spec.ts \
                  src/components/home/spotlight/SpotlightIcon.spec.ts

 Test Files  3 passed (3)
      Tests  40 passed (40)
   Duration  1.16s
```

Full spotlight suite (no regressions):

```
$ bunx vitest run src/components/home/spotlight/

 Test Files  14 passed (14)
      Tests  140 passed (140)
   Duration  3.69s
```

### Type-check + lint (verbatim — empty output is clean)

```
$ bunx tsc --noEmit
[no output]

$ bunx eslint src/components/home/spotlight/ e2e/spotlight-transition-lock.spec.ts
[no output]
```

### E2E (Playwright)

New spec — both green:

```
$ BASE_URL=http://localhost:3002 bunx playwright test spotlight-transition-lock --project=chromium

  ✓ 10 rapid ArrowRight presses settle without leaving a card stuck mid-fade (2.5s)
  ✓ lock honors goTo: rapid dot clicks do not produce stuck states (3.0s)

  2 passed (4.3s)
```

Existing specs (`spotlight.spec.ts`, `spotlight-full.spec.ts`) ran with 14 passing and 1 flaky (the `reduced-motion preference disables auto-cycle` test passed on retry). Three tests failed identically on **both my changes and the pre-Phase-01 baseline (commit `4689dd1`)** when run against a vite dev server:

  - `spotlight.spec.ts:38` — "mounts above the legacy trending row"
  - `spotlight.spec.ts:202` — "axe-core reports zero a11y violations"
  - `spotlight-full.spec.ts:207` — "axe-core reports 0 violations on the 9-card spotlight"

These are pre-existing environmental failures (dev-server-vs-prod differences in page-level heading order + Home-view DOM layout) and **not regressions from this phase**. The `spotlight-full.spec.ts:142` "cycles through all 9 card types via next-chevron" test, which originally regressed after Task 4, was restored to green by switching the lock release to `@after-leave` (see Deviation #1).

### CSS lint note

`bunx eslint src/styles/main.css` reports a `Parsing error: Expression expected` on every CSS file because the project's ESLint config has no CSS parser. This is **pre-existing** — reproduced on the unmodified file by stashing changes and re-running. Logged as a deferred item; not actionable inside this phase.

## Deviations from plan

### 1. [Rule 3 — Blocking issue] Released transition lock on `@after-leave` instead of `@after-enter`

- **Found during:** Task 6 (e2e test pass) — the existing `e2e/spotlight-full.spec.ts:142` "cycles through all 9 card types via next-chevron" test regressed under the Task 4 lock as initially specified.
- **Plan said:** `@after-enter="isTransitioning = false"`.
- **Issue:** With `mode="out-in"`, `@after-enter` fires only after BOTH the leave (~400ms) and enter (~400ms) phases complete — total no-input window ~800ms. The existing 9-card e2e test waits 450 ms between chevron clicks and expects all 8 clicks to advance the carousel through all 9 distinct slides. With an 800 ms lock window the bulk of those clicks were swallowed, leaving the carousel stuck on slide 1 (seen-set size 1 of 9).
- **Fix:** Release the lock on `@after-leave` (after the outgoing card's fade-out completes) instead. At that point the new card is already in the DOM mid-enter, so a subsequent navigation interrupts the enter rather than producing a blank frame. Lock window is now ~400ms (matching `--spotlight-fade-ms`), legitimate rapid clicks pass through, and the Phase 03 blank-card bug stays fixed because the LEAVE phase — where the bug originates — is still fully protected.
- **Verification:** New e2e spec (`spotlight-transition-lock.spec.ts`) confirms 10 rapid ArrowRight presses produce no `.spotlight-fade-leave-active` stuck states. Existing `cycles through all 9` test passes.
- **Files modified:** `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`
- **Commit:** `dd7ec8e`

### 2. [Rule 2 — Critical functionality] Added watchdog cleanup on unmount

- **Found during:** Task 4 implementation review.
- **Plan said:** "Defensive 600ms watchdog … when `isTransitioning` flips to true, `setTimeout(() => { isTransitioning.value = false }, 600)`".
- **Issue:** The plan didn't specify cleanup. Without an `onBeforeUnmount` clearTimeout, the watchdog timer would leak between HMR cycles and could fire against a stale ref in the next mount.
- **Fix:** Added `onBeforeUnmount(() => clearTimeout(watchdogTimer))` so the timer is GC'd cleanly.
- **Files modified:** `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`
- **Commit:** `56b2f21`

### 3. [Rule 2 — Critical functionality] Forward-compat fallback for unknown card types in `CarouselControls`

- **Found during:** Task 5 — the existing `HeroSpotlightBlock.spec.ts:337` "renders without console.error when an unknown card type is encountered" test failed after my CarouselControls rewrite.
- **Issue:** New dots dereferenced `cardTokens[card.type].accent` directly. When the backend ships a forward-compat variant the frontend doesn't yet know about (`type: 'unknown'`), `cardTokens['unknown']` is `undefined` and accessing `.accent` throws — breaking the section render and emitting console errors.
- **Fix:** Added a `tokenFor(type)` helper that returns `cardTokens[type] ?? FALLBACK_TOKEN`, where `FALLBACK_TOKEN` uses a neutral cyan/sparkles/`spotlight.regionLabel` set. Mirrors the existing `HeroSpotlightBlock` contract.
- **Files modified:** `frontend/web/src/components/home/spotlight/CarouselControls.vue`
- **Commit:** `793b9b3`

### 4. [Rule 2 — Critical functionality] Empty-`alt` instead of removed `alt` on backdrop `<img>`

- **Found during:** Task 2 implementation.
- **Plan said:** `<img :src="posterUrl" aria-hidden="true" ... />` with no `alt` attribute.
- **Issue:** Per HTML spec, decorative `<img>` elements MUST have either `alt=""` or `role="presentation"` to be skipped by screen readers; omitting `alt` entirely is a WCAG failure even with `aria-hidden`.
- **Fix:** Added `alt=""` to the poster-blur `<img>`.
- **Files modified:** `frontend/web/src/components/home/spotlight/SpotlightBackdrop.vue`
- **Commit:** `23db0d4`

## Threat surface mitigations applied

| Threat | Mitigation status |
|---|---|
| **T-V11-01** — Backdrop image leaks an extra HTTP request per card | Mitigated. `SpotlightBackdrop` reuses the card's existing `posterUrl` (no separate fetch); browser cache hit on the second render. |
| **T-V11-02** — Inline `<svg>` sprite ballooning bundle | Mitigated. `SpotlightIcon.vue` is one file with 9 small icons, each a stroke-based 24×24 viewBox. Per-icon path is ~50 bytes; total component well under the 3 KB gzipped budget specified in the plan. |
| **T-V11-03** — Transition lock deadlock if `@after-enter` never fires | Mitigated AND adjusted. 600 ms watchdog timer force-clears `isTransitioning` (with `onBeforeUnmount` cleanup). Lock release switched to `@after-leave` per Deviation #1 — lock window is now ~400 ms matching `--spotlight-fade-ms`, so the watchdog window of 600 ms has ~50% slack. |

## Known stubs

None. No stubs introduced — every primitive shipped in this phase is fully wired (the cards that will consume them ship in Phases 02–10).

## Self-Check: PASSED

Verified all claimed files + commits exist in this worktree.

```
$ for f in \
    frontend/web/src/components/home/spotlight/tokens.ts \
    frontend/web/src/components/home/spotlight/tokens.spec.ts \
    frontend/web/src/components/home/spotlight/SpotlightIcon.vue \
    frontend/web/src/components/home/spotlight/SpotlightIcon.spec.ts \
    frontend/web/src/components/home/spotlight/SpotlightBackdrop.vue \
    frontend/web/src/components/home/spotlight/SpotlightBackdrop.spec.ts \
    frontend/web/src/components/home/spotlight/__snapshots__/SpotlightBackdrop.spec.ts.snap \
    frontend/web/e2e/spotlight-transition-lock.spec.ts \
    frontend/web/src/styles/main.css \
    frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue \
    frontend/web/src/components/home/spotlight/CarouselControls.vue \
    frontend/web/src/components/home/spotlight/CarouselControls.spec.ts; do
  [ -f "$f" ] && echo "FOUND: $f" || echo "MISSING: $f"
done
```

All 12 files FOUND.

Commits present in HEAD log:
- `4fe43c4` (Task 1)
- `23db0d4` (Task 2)
- `aeb6fab` (Task 3)
- `56b2f21` (Task 4)
- `793b9b3` (Task 5)
- `dd7ec8e` (Task 6)
