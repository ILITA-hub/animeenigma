---
phase: 03-random-tail-refactor
plan: 03
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-RT-01, HSB-V11-RT-02, HSB-V11-RT-03, HSB-V11-RT-04]
human_verified: "pending eyeball confirmation on animeenigma.ru after orchestrator merge + redeploy"
key_files:
  created: []
  modified:
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
    - frontend/web/src/styles/main.css
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue
    - frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts
commits:
  - f439dba feat(spotlight) add randomTail.taglines i18n + 3-locale parity (HSB-V11-RT-03)
  - 3d11872 feat(spotlight) add shuffle-deck keyframe animation for RandomTailCard (HSB-V11-RT-04)
  - 5f86b29 refactor(spotlight) RandomTailCard discovery identity (HSB-V11-RT-01..04)
metrics:
  metric_string: "UXΔ = +2 (Better) · CDI = 0.03 * 5 · MVQ = Sprite 78%/82%"
  completed_date: 2026-05-24
---

# Phase 03 Plan 03: RandomTailCard discovery refactor Summary

Refactored `RandomTailCard.vue` away from being a cyan-tinted AnimeOfDayCard clone into a distinct purple "discovery" surface — purple backdrop overlay, shuffle-icon kicker, 4-candidate rotating tagline per locale, mount-time shuffle-deck animation (gated on `prefers-reduced-motion`), and a purple `.cta-hero` CTA with an inline shuffle icon. The Phase 02 cinematic-backdrop pattern (relative wrapper + SpotlightBackdrop + `z-10` content) is reused verbatim so the two sibling cards now share the structural treatment while the discovery one wears its own paint.

## What shipped

### HSB-V11-RT-03 — i18n taglines + parity coverage

- Added a `spotlight.randomTail.taglines: string[]` array of 4 entries to each of `en.json`, `ru.json`, `ja.json`. The 4 candidates per locale were drafted to read as distinct discovery prompts rather than re-phrasings of the same line:
  - **EN** — "Discover something new", "Have you seen this one?", "A random gem from the vault", "Maybe it'll be love"
  - **RU** — "Откройте что-то новое", "А вы это смотрели?", "Случайная находка из закромов", "Может, это любовь"
  - **JA** — "何か新しいものを発見", "見ましたか？", "倉庫から掘り出した一作", "もしかしたら運命の出会い"
- The pre-existing scalar `spotlight.randomTail.subtitle` key was kept unchanged in all three locales so the component has a defensive fallback when a future locale ships without the array.
- Extended `spotlight-keys.spec.ts`:
  - Loaded `ja.json` for the first time (was en+ru only).
  - Added a 3-locale parametrized assertion that `taglines` is a 4-element array of non-empty strings in each locale.
  - Added a length-parity assertion across en/ru/ja so the component's random index pick can never resolve to a missing tagline in another locale.

### HSB-V11-RT-04 — `.shuffle-deck` + `.shuffle-card` + `@keyframes shuffle`

Added a new `/* Spotlight RandomTail */` section to `frontend/web/src/styles/main.css`:

```css
.shuffle-deck { position: relative; width: 144px; height: 200px; }
.shuffle-card {
  position: absolute; inset: 0;
  background: linear-gradient(135deg, #8b5cf6, #06b6d4);
  border-radius: 12px;
  opacity: 0;
  animation: shuffle 800ms cubic-bezier(0.4, 0, 0.2, 1) var(--delay) forwards;
}
@keyframes shuffle {
  0%   { opacity: 0; transform: translateY(-100px) rotate(-15deg); }
  60%  { opacity: 1; transform: translateY(0) rotate(0); }
  100% { opacity: 0; transform: translateY(20px) rotate(5deg); }
}
```

Five gradient cards (purple → cyan) stack at the same inset, each fed a `--delay` custom property by the consuming SFC. The animation peaks at 60% (opaque, centered, unrotated) and decays out, so the deck self-removes visually; the SFC then removes the wrapper from the DOM at +1000ms via a tracked `setTimeout`.

### HSB-V11-RT-01..04 — `RandomTailCard.vue` template + script rewrite

- **HSB-V11-RT-01 — backdrop + purple overlay.** The bare `<article>` is now wrapped in `<article class="relative w-full h-full overflow-hidden">` hosting:
  1. `<SpotlightBackdrop variant="poster-blur" :poster-url="data.anime.poster_url" />` at the bottom — reuses the already-fetched poster image (browser cache hit, zero new HTTP).
  2. A purple secondary overlay (`absolute inset-0 bg-gradient-to-r from-purple-500/30 via-transparent to-transparent`, `aria-hidden`) layered above it — this is the visual differentiator from the cyan AnimeOfDay sibling.
  3. The content under `relative z-10`.

- **HSB-V11-RT-02 — promoted kicker with shuffle icon.** Both mobile (`md:hidden`) and desktop (`hidden md:flex`) kicker copies were restyled to lead with `<SpotlightIcon name="shuffle" class="w-4 h-4 text-purple-300" />` followed by the label in `text-purple-200 text-[10px] uppercase tracking-[0.18em] font-semibold`. Matches the Phase 02 AnimeOfDay kicker treatment, but with the purple accent + shuffle mark.

- **HSB-V11-RT-03 — rotating tagline.** A new `tagline` ref is populated inside `onMounted` by reading the array via `tm('spotlight.randomTail.taglines')` and picking `arr[Math.floor(Math.random() * arr.length)]`. If `tm()` returns a non-array (locale missing the key, malformed translation), the SFC falls back to `t('spotlight.randomTail.subtitle')` so the card never renders a raw key string. The tagline element carries `hidden md:block` so it only shows on tablet+.

- **HSB-V11-RT-04 — shuffle-deck mount animation.** Mounted as a sibling under the article wrapper at `z-20` absolute-inset-0, behind a `v-if="!reducedMotion && showShuffle"` guard:
  - `reducedMotion` uses `useMediaQuery('(prefers-reduced-motion: reduce)')` (the same hook Hero.vue, Carousel.vue, and HeroSpotlightBlock.vue already use).
  - `showShuffle` is set false immediately if reduced motion is active; otherwise a `setTimeout(…, 1000)` flips it false. The timer reference is cleared in `onBeforeUnmount` so it can never fire on a destroyed instance.
  - The deck renders `v-for="n in 5"` `.shuffle-card` divs, each given a `--delay: ${n * 60}ms` inline so the V-shaped stagger reads as one continuous fan.

- **Purple `.cta-hero` CTA.** Replaced the legacy `class="btn btn-primary text-sm md:text-base"` chain with `class="cta-hero" data-accent="purple"`, which Phase 01's `main.css` resolves to `bg-purple-500 hover:bg-purple-400 shadow-purple-500/30`. An inline `<SpotlightIcon name="shuffle" class="w-4 h-4" />` was added after the label so the discovery affordance shows up on the action itself.

- **Phase 02 parity touch-ups** (consistency with the sibling card; not in the original plan task list but treated as part of "make this card a peer, not a clone"):
  - Foreground poster widened from `w-28/w-32/w-44` to `w-32/w-44/w-56`, matching the Phase 02 AnimeOfDay refactor.
  - Poster gains a purple-tinted shadow (`shadow-2xl shadow-purple-500/20`) that intensifies on `group-hover:shadow-purple-500/40` — same hover treatment AnimeOfDay uses with cyan.
  - The score chip was promoted from a `class="absolute top-2 right-2"` overlay (which obstructed the poster art) into a meta-row pill using the same `bg-yellow-500/20 text-yellow-200` treatment AnimeOfDay's Phase 02 refactor adopted. See **Deviations** below for why this happened.

### Spec rewrite (`RandomTailCard.spec.ts`) — 14 assertions

Mirrors the assertion style of the Phase 02 `AnimeOfDayCard.spec.ts`:

1. SpotlightBackdrop is mounted with `variant="poster-blur"` and the correct `posterUrl` prop.
2. The purple secondary overlay is present (`from-purple-500/30`).
3. `<SpotlightIcon name="shuffle">` appears at least twice (two header copies; CTA stub may swallow).
4. Localized title renders via `getLocalizedTitle`.
5. The picked tagline is one of the 4 mocked candidates (random pick verified via membership check).
6. The tagline element carries `hidden md:block`.
7. With `tm()` mocked to return a non-array, the tagline falls back to `spotlight.randomTail.subtitle`.
8. Exactly one `.cta-hero` with `data-accent="purple"` is rendered.
9. The legacy `btn-primary` / `btn-ghost` classes and any "addCta" key never leak into rendered output.
10. With `prefers-reduced-motion: reduce` → `[data-testid="shuffle-deck"]` does NOT exist and the `.shuffle-card` class never appears.
11. With motion allowed → the deck exists with exactly 5 `.shuffle-card` children, each with the correct `--delay: ${(i+1)*60}ms`.
12. Only `font-medium` / `font-semibold` typography weights are used.
13. Tablet padding is `p-4` (never `p-5`).
14. No raw English copy leaks — every label flows through `t()` (echo mock makes the keys visible as text).

The `@vueuse/core` mock matches the established HeroSpotlightBlock.spec pattern: a partial mock that returns a controllable `Ref<boolean>` for the `(prefers-reduced-motion)` query and `ref(false)` for everything else. The `vue-i18n` mock additionally exposes `tm()` so the component's array lookup can be exercised end-to-end.

## Verification

```
$ cd frontend/web && bunx vitest run src/components/home/spotlight/cards/RandomTailCard.spec.ts src/locales/__tests__/spotlight-keys.spec.ts

 Test Files  2 passed (2)
      Tests  90 passed (90)
```

```
$ cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts

 Test Files  15 passed (15)
      Tests  232 passed (232)
```

(Every Phase 01 primitive spec, every other card spec, the locales parity spec — all stay green alongside the new 14-test RandomTailCard suite.)

```
$ cd frontend/web && bunx tsc --noEmit
(clean — no output)
```

```
$ cd frontend/web && bunx eslint src/components/home/spotlight/ src/locales/
(clean — no output)
```

Playwright e2e (`BASE_URL=http://localhost:3003 bunx playwright test spotlight-full --project=chromium --workers=1`):

- `spotlight-full.spec.ts:142` — **"cycles through all 9 card types via next-chevron"** — **PASS** (the canonical 9-card regression test for the whole carousel; this Phase 03 refactor does not break it).
- `spotlight-full.spec.ts:24` — renders all 9 card types — **PASS**
- `spotlight-full.spec.ts:225` — HSB-MIG-01 trendingRecs DOM artifacts gone — **PASS**
- `spotlight-full.spec.ts:275` — `prefers-reduced-motion: reduce` disables auto-cycle on 9-card payload — **PASS**
- `spotlight-full.spec.ts:241` — arrow-key navigation cycles all 9 slides — **FLAKY** (pre-existing transition-lock interaction documented in Phase 01 + Phase 02 SUMMARYs).
- `spotlight-full.spec.ts:207` — axe-core 0 violations — **FAIL** (pre-existing: "heading-order" — every card uses `<h3>` without an intervening `<h2>` above the carousel. Documented in Phase 01 and Phase 02 SUMMARYs as an environmental pre-existing failure, not a Phase 03 regression).
- `spotlight-transition-lock.spec.ts` (both tests) — **PASS** (rapid-click transition-lock regression test stays green).

Visual smoke (post-deploy): open `https://animeenigma.ru/`, cycle to the RandomTail card via the next-chevron, confirm on the first visit per session:

1. Blurred poster + a purple wash fill the entire card background (right-edge vignette keeps text legible).
2. Foreground poster shows the larger size (~w-56 on desktop) with a soft purple glow.
3. Kicker reads "Random pick" (or locale equivalent) with a small purple shuffle icon to its left.
4. Tagline (desktop only) reads one of the 4 candidates — refresh to confirm rotation.
5. A short shuffle-deck animation fans in and out at mount (≤1s), then disappears.
6. The only button is the purple "Open" pill with the shuffle icon.
7. Toggling `prefers-reduced-motion: reduce` in DevTools and re-cycling to RandomTail shows the same card but with NO shuffle-deck animation.

## Deviations from plan

### [Rule 2 — Auto-add] Phase 02 backdrop pattern adopted for sibling parity

The plan's task list specified the backdrop + overlay + kicker + tagline + shuffle-deck + CTA changes, but did not explicitly call out the bigger foreground poster, the purple-tinted poster shadow, or the score-chip promotion into a meta-row pill. Those three changes are inherited from the Phase 02 AnimeOfDayCard refactor that landed on `main` between the worktree spawn and this execution. Without them, RandomTailCard would still read as a half-clone — same small poster, same absolute-positioned score overlay obscuring the art, no shadow story.

I treated the missing-from-plan deltas as **Rule 2 (auto-add missing critical functionality)** — "make this card a peer, not a clone" is the plan's stated goal, and shipping it without those three parity touches would leave the discovery identity half-implemented. None of the three required new i18n, new tokens, or new TypeScript types — they're pure template adjustments mirroring code that already merged into `main` in `178df93`.

### [Rule 3 — Auto-fix] Worktree HEAD was behind `main` at spawn time

The worktree branch was created from `87c75f8` (before Phase 02 merged), so `AnimeOfDayCard.vue` on disk was the pre-refactor version. The plan's `<code_context>` block explicitly named "Phase 02 AnimeOfDayCard pattern" as the visual reference, so I fast-forward-merged `main` into the worktree branch (`Updating 87c75f8..2ab41f9 Fast-forward`, no conflicts) before starting any code changes. This pulled in Phase 02's commits as ancestors of every new commit on this branch, so the final merge back to `main` will be a clean fast-forward of just my three task commits + the metadata commit.

### Per-task commit granularity

The orchestrator's commit protocol suggested 3-5 commits. I landed exactly three task commits ordered by data → asset → behavior:

- `f439dba` — i18n additions + parity-test extension (data layer; everything else depends on the array existing in en/ru/ja).
- `3d11872` — `main.css` keyframes (CSS asset; consumed by the SFC but tested independently as a regression-free addition).
- `5f86b29` — `RandomTailCard.vue` template + script + spec rewrite (behavior layer; depends on both prior commits, and the spec depends on the SFC, so splitting them would leave a RED test pointing at an unfinished SFC for one commit).

Each intermediate HEAD in the worktree was buildable + spec-green.

## Threat surface

No new network endpoints, no new file-system access patterns, no new trust-boundary changes. The backdrop component reuses the already-fetched poster URL (no second HTTP request). The shuffle-deck animation runs entirely client-side as CSS keyframes on inline-styled `<div>` children — no JS animation loop, no `requestAnimationFrame`, no event listeners attached. The taglines are static-translated strings compiled into the locale JSONs at build time. No threat flags.

## Self-Check: PASSED

- FOUND: frontend/web/src/locales/en.json (modified)
- FOUND: frontend/web/src/locales/ru.json (modified)
- FOUND: frontend/web/src/locales/ja.json (modified)
- FOUND: frontend/web/src/locales/__tests__/spotlight-keys.spec.ts (modified)
- FOUND: frontend/web/src/styles/main.css (modified)
- FOUND: frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue (modified)
- FOUND: frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts (modified)
- FOUND commit: f439dba (feat(spotlight): add randomTail.taglines i18n + 3-locale parity (HSB-V11-RT-03))
- FOUND commit: 3d11872 (feat(spotlight): add shuffle-deck keyframe animation for RandomTailCard (HSB-V11-RT-04))
- FOUND commit: 5f86b29 (refactor(spotlight): RandomTailCard discovery identity (HSB-V11-RT-01..04))
