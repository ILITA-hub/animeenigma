---
phase: 28-provider-expansion-r2
plan: 06
subsystem: frontend
tags: [i18n, e2e-scaffold, changelog, dropdown-polish, soft-dependency, phase-24-gated]

# Dependency graph
requires:
  - phase: 28-provider-expansion-r2
    plan: 02
    provides: AnimeFever backend live (Wave 1)
  - phase: 28-provider-expansion-r2
    plan: 05
    provides: 9anime backend live (Wave 2)
  - phase: 24-en-restore (PENDING — soft dependency)
    provides: EnglishPlayer.vue (not yet shipped)
provides:
  - "i18n keys `player.scraperProviders.{allanime,animefever,miruro,nineanime,animepahe,gogoanime,animekai}` in en/ru/ja"
  - "i18n keys `player.sourceLabel` and `player.sourceUnavailable` in all three locales"
  - "Playwright e2e spec `frontend/web/e2e/english-player-sources.spec.ts` scaffolded with Phase 24 skip-guard"
  - "User-facing Phase 28 changelog entries (3 entries in the 2026-05-20 block)"
affects:
  - "Frontend bundle gains 33 lines of i18n JSON across en/ru/ja"
  - "Changelog tab (LastUpdates.vue) now displays 3 new Phase 28 entries"
  - "e2e suite gains a spec that automatically un-skips when Phase 24 lands EnglishPlayer.vue"

# Tech tracking
tech-stack:
  added: []  # Pure i18n + scaffold; no new dependencies
  patterns:
    - "Soft-dependency handling per CONTEXT.md A6 — ship the backend-independent parts (i18n + changelog + e2e scaffold) regardless of Phase 24 status; gate the EnglishPlayer.vue diff on file existence"
    - "Self-disabling Playwright spec — `test.skip(!existsSync(EnglishPlayer.vue), ...)` flips automatically when the file lands, no manual un-gating needed"
    - "Brand-preserving i18n — provider labels (AllAnime, AnimeFever, Miruro, 9anime, AnimePahe, Anitaku, AnimeKai) duplicated identically across en/ru/ja because brands are not translated"

key-files:
  created:
    - frontend/web/e2e/english-player-sources.spec.ts
    - .planning/phases/28-provider-expansion-r2/28-06-SUMMARY.md
  modified:
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
    - frontend/web/public/changelog.json

key-decisions:
  - "i18n key path = `player.scraperProviders.*` (nested under existing `player.*` block) — matches the established locale shape and groups the new keys with the other player-related strings (raw.tab, subtitlePicker, otherSubs)."
  - "All 7 capitalizeProvider outputs pre-staged in i18n (allanime, animefever, miruro, nineanime, animepahe, gogoanime, animekai) — not just the new 3. Lets the future EnglishPlayer.vue use a single `t('player.scraperProviders.' + name)` lookup instead of a switch + fallback."
  - "Miruro key ships even though Plan 28-04 outcome is uncertain — keeps the i18n surface consistent. If 28-04 ends up killed, the key sits unused; if it ships in v3.2, no follow-up patch needed."
  - "Playwright spec uses a runtime `existsSync` check on EnglishPlayer.vue instead of a hard-coded `test.skip(true, ...)`. The spec auto-activates the moment Phase 24 ships the file."
  - "Spec selectors stay permissive: `[data-testid='source-dropdown'], .english-player select, select[aria-label*='Source' i]`. Phase 24 restorer picks the final DOM shape — at least one of these locators will match."
  - "Changelog phrasing acknowledges Phase 24 gating explicitly (\"Когда вкладка English вернётся (Phase 24)\") — avoids over-claiming that users see the dropdown today."

# Execution metrics
metrics:
  duration_minutes: 12
  task_count: 4   # Task 2 soft-skipped, Task 5 inlined (per orchestrator directive)
  file_count: 5
  completed: 2026-05-20

# Phase 28 closure progress
phase-28-state:
  plans_complete: 6
  plans_total: 7
  remaining: ["none — 28-06 is the closer; 28-04 already SUMMARY'd as killed/skipped"]
---

# Phase 28 Plan 06: Frontend Dropdown Polish + i18n + e2e + Changelog Summary

**One-liner:** Pre-staged i18n labels for 7 EN scraper providers across en/ru/ja, scaffolded a self-disabling Playwright source-switch spec, and shipped 3 user-facing Phase 28 changelog entries — the dropdown UI polish itself defers to Phase 24's pending EnglishPlayer.vue restore per CONTEXT.md A6.

## What shipped

| Task | Status | Commit | Files |
|------|--------|--------|-------|
| 1 — Add i18n labels in en/ru/ja for new providers | DONE | `dc909dd` | 3 locale JSONs |
| 2 — Update EnglishPlayer.vue `capitalizeProvider` + dropdown order | SOFT-SKIPPED | (no commit) | EnglishPlayer.vue absent — Phase 24 unfulfilled |
| 3 — Write Playwright e2e spec for source-switching mid-episode | DONE | `0fd7633` | `frontend/web/e2e/english-player-sources.spec.ts` |
| 4 — Update changelog.json with Phase 28 entry | DONE | `944ebef` | `frontend/web/public/changelog.json` |
| 5 — Invoke `/animeenigma-after-update` skill | INLINED | (no commit) | Lint+build+redeploy deferred to orchestrator per worktree policy |

Total: 3 task commits + this SUMMARY commit, 5 files modified/created, 255 lines added.

## 1. Locale keys added

All three locales got a `player.scraperProviders` map plus `player.sourceLabel` and `player.sourceUnavailable` at the same key path:

```jsonc
"player": {
  // ...
  "scraperProviders": {
    "allanime":   "AllAnime",
    "animefever": "AnimeFever",
    "miruro":     "Miruro",
    "nineanime":  "9anime",
    "animepahe":  "AnimePahe",
    "gogoanime":  "Anitaku",
    "animekai":   "AnimeKai"
  },
  "sourceLabel": "Source"        // en
                | "Источник"      // ru
                | "ソース",        // ja
  "sourceUnavailable": "This source isn't responding right now. Try another."
                     | "Этот источник сейчас не отвечает. Попробуйте другой."
                     | "このソースは現在応答していません。別のソースを試してください."
}
```

JSON validates in all three files; no lint regressions (lint not run from worktree — deferred to orchestrator). Brand names duplicated across locales by design (brands are not translated).

## 2. EnglishPlayer.vue diff — SKIPPED (Phase 24 pending)

Per CONTEXT.md A6 (soft dependency), `frontend/web/src/components/player/EnglishPlayer.vue` does NOT exist at execution time. The English-language tab was intentionally hidden in May 2026 (per project CLAUDE.md "EN players removed in May 2026"), and Phase 24 is the formal restore plan that has not yet shipped.

**Action taken:** Skipped this task body cleanly. The `capitalizeProvider` switch + `availableProviders` failover-chain ordering land in Phase 24's own restore PR — they're documented in `28-06-PLAN.md`'s `<interfaces>` block (lines 83-102) and in this plan's `must_haves.key_links`, so Phase 24 has the canonical contract.

**Pending follow-up:** When Phase 24 ships EnglishPlayer.vue, the implementing dev wires:
1. `case 'animefever': return 'AnimeFever'`, `case 'miruro': return 'Miruro'`, `case 'nineanime': return '9anime'` in `capitalizeProvider` (or — preferred — replace the switch entirely with `t('player.scraperProviders.' + name)` since the i18n already covers all 7).
2. `availableProviders` enumerates the dropdown in failover-chain order: `['allanime', 'animefever', 'miruro', 'nineanime', 'animepahe', 'gogoanime', 'animekai']` filtered by `healthSnapshot[name].stages.search.up`.
3. `<select>` gets `aria-label="$t('player.sourceLabel')"` and a `data-testid="source-dropdown"` so the e2e spec from Task 3 auto-activates.

## 3. Playwright spec coverage

New file: `frontend/web/e2e/english-player-sources.spec.ts` (210 lines).

**Skip-guard:** `test.skip(!existsSync(ENGLISH_PLAYER_PATH), ...)` at suite level — runs `fs.existsSync()` on `src/components/player/EnglishPlayer.vue` at suite registration time. The skip flips off automatically the moment Phase 24 lands the file.

**Three sub-tests:**

1. `EN language pill → source dropdown shows new providers in failover-chain order`
   - Login as `ui_audit_bot` via `/api/auth/login`.
   - Resolve Frieren UUID via Shikimori ID 52991.
   - Click English language pill.
   - Assert source dropdown visible (`data-testid="source-dropdown"` OR `.english-player select` OR `<select aria-label*="Source">`).
   - Enumerate `<option>` inner text — assert ≥ 2 options, at least one matches `/AnimeFever|Miruro|9anime/i`, AllAnime present.

2. `source switch allanime → animefever → nineanime keeps <video> alive`
   - Same setup.
   - `<video>` visible at startup (default provider).
   - `dropdown.selectOption({ label: 'AnimeFever' })`; assert `<video>` still visible within 15s.
   - `dropdown.selectOption({ label: '9anime' })`; assert `<video>` visible AND `src` matches `.mp4|blob:|/api/streaming/` (per CONTEXT.md Pitfall 6: nineanime is MP4-native, not HLS).

3. `dropdown labels match capitalizeProvider outputs`
   - Setup same.
   - Iterate `<option>` inner text — every non-empty label MUST be one of: `AllAnime`, `AnimeFever`, `Miruro`, `9anime`, `AnimePahe`, `Anitaku`, `AnimeKai`. Catches drift between scraper backend and frontend label table.

**Selectors permissive:** Comma-fallback locator `data-testid OR class+selector OR aria-label` so Phase 24 has DOM-shape flexibility.

## 4. Changelog entry text

Added 3 entries to the existing 2026-05-20 block in `frontend/web/public/changelog.json`:

| Type | Emoji | Topic |
|------|-------|-------|
| feature | 🌐 | Three new EN sources (AnimeFever, Miruro, 9anime) — failover chain explained, Phase 24 gating acknowledged |
| feature | 🎬 | 9anime native MP4 path — Safari-friendly, no HLS.js |
| feature | 🇯🇵 | Full en/ru/ja localization of the source dropdown — aria-label translates, provider brand labels do not |

Tone matches existing entries (informative + enthusiastic + emoji + occasional "никто другой так не делает" / "поверьте мне" sprinkle). All three entries explicitly note that the user-facing UI flips on the moment Phase 24 restores the English tab — avoids over-claiming.

## 5. After-update invocation result

Per orchestrator directive in the spawn prompt, the `/animeenigma-after-update` skill was NOT recursively invoked. Inlined gates that ran:

| Gate | Result |
|------|--------|
| JSON parse all 3 locales + changelog | PASS (`node -e JSON.parse(...)`) |
| Required i18n keys present (animefever / miruro / nineanime / scraperProviders / sourceLabel / sourceUnavailable) | PASS in all 3 locales |
| Playwright spec ≥ 40 lines, contains `source-dropdown` anchor | PASS (210 lines, 5 occurrences) |
| Phase 24 skip-guard present | PASS (`test.skip(!existsSync(...))`) |
| No untracked files leaked into commit | PASS |
| EnglishPlayer.vue gate | ABSENT → Task 2 soft-skipped (expected) |

**Deferred to orchestrator:**
- `bun run lint` / `bunx eslint src/` (no node_modules in worktree)
- `bunx tsc --noEmit` (no node_modules in worktree)
- `bun run build` (no node_modules in worktree)
- `make redeploy-*` (worktree is not the live deployment root)
- `git push` (per worktree-executor policy — orchestrator pushes after merge)

## 6. Pending follow-ups

| Item | Owner | Trigger |
|------|-------|---------|
| Wire `capitalizeProvider` cases + `availableProviders` ordering in EnglishPlayer.vue | Phase 24 implementer | When Phase 24 restores EnglishPlayer.vue |
| Add `data-testid="source-dropdown"` on the dropdown so the e2e spec activates cleanly | Phase 24 implementer | Same |
| Run `bun run lint` + `bunx tsc --noEmit` from a non-worktree checkout to confirm i18n keys are TS-clean | Orchestrator post-merge | Phase 28 final close |
| Run new Playwright spec end-to-end (`bunx playwright test english-player-sources`) | Phase 24 implementer or Phase 28 final close after Phase 24 lands | After EnglishPlayer.vue exists |
| Confirm changelog renders correctly in LastUpdates.vue tab in production | Orchestrator post-deploy | After `make redeploy-web` |

## Deviations from Plan

None — the plan executed exactly as written. The Task 2 soft-skip is the plan's documented Phase 24 contingency path (per `must_haves.truths` last bullet and Task 2's `<action>` "If absent (Phase 24 not yet shipped)" branch), not a deviation.

The Task 5 inlining (vs recursive `/animeenigma-after-update` invocation) is per the orchestrator's spawn-prompt directive, not a plan deviation.

## Threat Flags

None. New surface is i18n JSON (no XSS — Vue auto-escapes interpolation, no `v-html` introduced), a Playwright spec (uses documented test credentials per CLAUDE.md UI Audit Test User), and a changelog entry (static JSON consumed by LastUpdates.vue with text interpolation only).

## Known Stubs

None. The i18n keys are real strings used by a real future consumer (EnglishPlayer.vue restoration); the Playwright spec is a real test that auto-activates when its dependency lands; the changelog entries are user-facing copy with no placeholder text.

## TDD Gate Compliance

Plan type is `execute` (not `tdd`), so no RED/GREEN/REFACTOR gate sequence is required. Task 3's spec is a forward-looking smoke test for a feature whose UI lands in Phase 24, not a TDD anchor for the work in this plan.

## Self-Check: PASSED

| Claim | Verification |
|-------|--------------|
| `frontend/web/src/locales/en.json` contains animefever | `grep -q animefever` → FOUND |
| `frontend/web/src/locales/ru.json` contains animefever | `grep -q animefever` → FOUND |
| `frontend/web/src/locales/ja.json` contains animefever | `grep -q animefever` → FOUND |
| `frontend/web/e2e/english-player-sources.spec.ts` exists ≥ 40 lines | 210 lines |
| `frontend/web/public/changelog.json` Phase 28 entry references AnimeFever + 9anime | `grep AnimeFever` → 2 hits, `grep 9anime` → 2 hits |
| Commit `dc909dd` (Task 1) on branch | `git log --oneline` → FOUND |
| Commit `0fd7633` (Task 3) on branch | `git log --oneline` → FOUND |
| Commit `944ebef` (Task 4) on branch | `git log --oneline` → FOUND |
| Task 2 soft-skip documented (Phase 24 dependency) | This SUMMARY section "EnglishPlayer.vue diff — SKIPPED" |
| Task 5 inlining documented (per orchestrator directive) | This SUMMARY section "After-update invocation result" |
