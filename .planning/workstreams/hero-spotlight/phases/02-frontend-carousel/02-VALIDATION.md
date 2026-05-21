---
phase: 2
slug: frontend-carousel
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-21
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution of
> hero-spotlight's frontend carousel.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.x (unit/component) + Playwright 1.58 (e2e) + axe-core 4.11 (a11y) |
| **Config file** | `frontend/web/vitest.config.ts` + `frontend/web/playwright.config.ts` |
| **Quick run command** | `cd frontend/web && bunx vitest run src/components/home/spotlight src/composables/useSpotlight.spec.ts` |
| **Full suite command** | `cd frontend/web && bunx tsc --noEmit && bunx eslint src/ && bunx vitest run && bunx playwright test spotlight` |
| **Estimated runtime** | ~15s quick (Vitest), ~120s full (incl. Playwright + axe) |

---

## Sampling Rate

- **After every task commit:** Run quick command (Vitest only, ~15s)
- **After every plan wave:** Run full suite (tsc + eslint + vitest + playwright)
- **Before `/gsd-verify-work`:** Full suite green + `bun run build` clean + `make redeploy-web` + visual check on `https://animeenigma.ru/`
- **Max feedback latency:** ≤30s

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | HSB-FE-09 | — | TypeScript types compile; `SpotlightCard` union matches Phase 1 JSON | unit | `cd frontend/web && bunx tsc --noEmit` | ❌ W0 | ⬜ pending |
| 2-01-02 | 01 | 1 | HSB-FE-09 | T-2-01 (flag bypass) | `VITE_HERO_SPOTLIGHT_ENABLED=false` → block does not mount | unit | `bunx vitest run src/composables/useSpotlight.spec.ts -t "flag-off"` | ❌ W0 | ⬜ pending |
| 2-02-01 | 02 | 2 | HSB-FE-02 | — | `useSpotlight()` fetches `/api/home/spotlight`; returns cards/loading/error refs | unit | `bunx vitest run src/composables/useSpotlight.spec.ts` | ❌ W0 | ⬜ pending |
| 2-02-02 | 02 | 2 | HSB-FE-02 | T-2-02 (5xx) | On 5xx → cards stays empty, block hides | unit | `bunx vitest run src/composables/useSpotlight.spec.ts -t "5xx"` | ❌ W0 | ⬜ pending |
| 2-03-01 | 03 | 2 | HSB-FE-03 | — | 7s auto-cycle interval starts on mount when motion allowed | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "interval"` | ❌ W0 | ⬜ pending |
| 2-03-02 | 03 | 2 | HSB-FE-03 | — | Hover/focus pauses auto-cycle; mouseleave/blur resumes | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "pause"` | ❌ W0 | ⬜ pending |
| 2-03-03 | 03 | 2 | HSB-FE-04 | — | ArrowLeft/Right and dot clicks seek; wraps around | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "nav"` | ❌ W0 | ⬜ pending |
| 2-03-04 | 03 | 2 | HSB-FE-05 | — | Initial slide index is random within `[0, cards.length-1]` | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "random init"` | ❌ W0 | ⬜ pending |
| 2-03-05 | 03 | 2 | HSB-FE-06 | — | `prefers-reduced-motion: reduce` → no auto-cycle, no transition class | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "reduced motion"` | ❌ W0 | ⬜ pending |
| 2-03-06 | 03 | 2 | HSB-FE-07 | — | a11y attributes present: role=region, aria-roledescription=carousel, slide aria-label "N of M" | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "aria"` | ❌ W0 | ⬜ pending |
| 2-03-07 | 03 | 2 | HSB-FE-08 | — | Loading state renders skeleton with matching height (no layout shift) | unit | `bunx vitest run src/components/home/spotlight/HeroSpotlightBlock.spec.ts -t "skeleton"` | ❌ W0 | ⬜ pending |
| 2-04-01 | 04 | 2 | HSB-FE-20 | — | AnimeOfDayCard renders poster, title, score, episodes, genres, Watch + Add CTAs | unit | `bunx vitest run src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` | ❌ W0 | ⬜ pending |
| 2-04-02 | 04 | 2 | HSB-FE-21 | — | LatestNewsCard renders 1..3 changelog entries with links to /changelog | unit | `bunx vitest run src/components/home/spotlight/cards/LatestNewsCard.spec.ts` | ❌ W0 | ⬜ pending |
| 2-04-03 | 04 | 2 | HSB-FE-22 | — | PlatformStatsCard renders 1..3 metric chips; hides null metrics | unit | `bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts` | ❌ W0 | ⬜ pending |
| 2-04-04 | 04 | 2 | HSB-FE-23 | — | RandomTailCard renders single poster + meta + "Random pick" header | unit | `bunx vitest run src/components/home/spotlight/cards/RandomTailCard.spec.ts` | ❌ W0 | ⬜ pending |
| 2-05-01 | 05 | 3 | HSB-FE-40 | — | `spotlight.*` keys exist in en.json AND ru.json with identical key sets | unit | `bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` | ❌ W0 | ⬜ pending |
| 2-06-01 | 06 | 3 | HSB-FE-01 | — | `<HeroSpotlightBlock />` mounts in Home.vue above trendingRecs | e2e | `bunx playwright test spotlight.spec.ts -g "mounts above legacy"` | ❌ W0 | ⬜ pending |
| 2-06-02 | 06 | 3 | HSB-FE-03 | — | E2E: block cycles every 7s; hover pauses | e2e | `bunx playwright test spotlight.spec.ts -g "auto-cycle"` | ❌ W0 | ⬜ pending |
| 2-06-03 | 06 | 3 | HSB-FE-06 | — | E2E: reduced-motion emulation disables auto, manual nav works | e2e | `bunx playwright test spotlight.spec.ts -g "reduced motion"` | ❌ W0 | ⬜ pending |
| 2-06-04 | 06 | 3 | HSB-FE-04 | — | E2E mobile viewport 375×667: poster cards stack vertically | e2e | `bunx playwright test spotlight.spec.ts -g "mobile"` | ❌ W0 | ⬜ pending |
| 2-06-05 | 06 | 3 | HSB-FE-07 | — | E2E + axe-core: zero a11y violations on the block | e2e | `bunx playwright test spotlight.spec.ts -g "axe"` | ❌ W0 | ⬜ pending |
| 2-06-06 | 06 | 3 | HSB-FE-09 | — | E2E with `VITE_HERO_SPOTLIGHT_ENABLED=false` rebuild: block absent, trendingRecs visible | manual+e2e | `bunx playwright test spotlight.spec.ts -g "flag off"` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `frontend/web/src/types/spotlight.ts` — `SpotlightCard` discriminated union matching Phase 1 JSON envelope
- [ ] `frontend/web/src/composables/useSpotlight.ts` + `useSpotlight.spec.ts`
- [ ] `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` + `HeroSpotlightBlock.spec.ts`
- [ ] `frontend/web/src/components/home/spotlight/CarouselControls.vue` + `CarouselControls.spec.ts`
- [ ] `frontend/web/src/components/home/spotlight/cards/{AnimeOfDayCard,LatestNewsCard,PlatformStatsCard,RandomTailCard}.vue` + co-located `.spec.ts` files
- [ ] `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — assert en/ru key parity
- [ ] `frontend/web/e2e/spotlight.spec.ts` — Playwright spec
- [ ] `@axe-core/playwright@4.11.3` dev-dep installed via `bun add -d`
- [ ] `VITE_HERO_SPOTLIGHT_ENABLED=true` documented in `frontend/web/.env.example` (project's only committed env file — see RESEARCH.md A3)

All test files are new — no existing files mocked.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visual smoke on production-style build | HSB-FE-01..09 | Live render with real /api/home/spotlight is the truth | After `make redeploy-web`, open https://animeenigma.ru/ in a desktop browser. Confirm block mounts above 3-column grid, cycles, hover pauses. Confirm legacy trendingRecs still visible below (Phase 2 is additive). |
| Random initial slide across F5 reloads | HSB-FE-05 | Statistical observation over multiple loads | Reload 10 times. Initial slide should differ ≥7/10 with cards.length=4 (75% chance not picking same). |
| Real `prefers-reduced-motion: reduce` via OS setting | HSB-FE-06 | OS-level toggle differs from DevTools emulation | On macOS: System Settings → Accessibility → Display → Reduce Motion. Reload. Confirm no auto-cycle, manual nav still works. |
| Visual on real mobile device | HSB-FE-04 | DevTools emulation ≠ real device viewport behavior | Open https://animeenigma.ru/ on iPhone/Android. Confirm poster cards stack vertically, taps work for dots + chevrons. |
| Feature-flag-off rebuild | HSB-FE-09 | Requires Docker rebuild with env override | Stop web container, set `VITE_HERO_SPOTLIGHT_ENABLED=false` in `.env`, `make redeploy-web`, refresh. Confirm block absent; legacy trendingRecs still visible. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency ≤30s quick / ≤120s full
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
