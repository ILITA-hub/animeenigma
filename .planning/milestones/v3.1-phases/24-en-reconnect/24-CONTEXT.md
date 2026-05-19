# Phase 24: EN Reconnect — Context

**Gathered:** 2026-05-19
**Status:** Ready for planning (`/gsd-plan-phase --phase 24`)
**Milestone:** v3.1 Scraper Self-Healing (REOPENED 2026-05-19)
**Spec:** `.planning/milestones/v3.1-REQUIREMENTS.md` (SCRAPER-HEAL-17..20)
**Supersedes standalone work:** `docs/superpowers/specs/2026-05-19-english-scraper-reconnect-design.md` + `docs/superpowers/plans/2026-05-19-english-scraper-reconnect.md` (Project A from the 2026-05-19 standalone brainstorm; absorbed into this phase via the supersedes note in `.planning/milestones/v3.1-REOPENING.md`).

<domain>
## Phase Boundary

A logged-out user opens an anime page, sees an English tab between RU and 18+, clicks it, sees the three-phase loader, then real video plays — restoring the user-facing surface that v3.1 Phase 21 originally shipped (SCRAPER-HEAL-08) and the v3.0 Phase 20 cutover over-rotation deleted on 2026-05-18.

**Concretely, this phase delivers:**

1. `frontend/web/src/components/player/EnglishPlayer.vue` restored from git history at commit `8424e99` (last good state, 1973 lines, three-phase loader + multi-source dropdown + scraperApi wiring + ReportButton + SubtitleOverlay + OtherSubsPanel). Any drift between the restored snapshot and current contracts (`scraperApi`, `useWatchPreferences`, `ReportButton`, `SubtitleOverlay`, `OtherSubsPanel`) is reconciled in-line during restoration — not deferred.
2. `frontend/web/src/views/Anime.vue` re-mounts EnglishPlayer behind an EN tab. The `VALID_LANGUAGES` whitelist grows `'en'`; `VALID_PROVIDERS` grows `'english'`; `switchLanguage` learns `'en'` (defaulting to `preferred_en_provider` from localStorage, falling back to `'english'`); the `videoProvider` save watcher writes `preferred_en_provider` when `videoLanguage === 'en'`; the `applyResolvedCombo` filter that stripped `'en'` / `'english'` is removed. The stale-localStorage sanitization from commit `ee4ed56` stays in place — it gracefully handles users whose localStorage still holds removed strings.
3. i18n keys re-added to all three locales: `videoTab.english`, `player.englishEmpty`, `player.englishUnavailable`, `player.serverPicker`, `player.categorySub`, `player.categoryDub`. Parity verified via `bun run lint:i18n`.
4. The cleanup-removed multi-source switcher keys (`tabEnglish`, `tabDebugSuffix`, `source`, `sourceUnhealthy`, `sourceSwitchFailed`, etc.) stay removed. The restored EnglishPlayer uses a single-tab + in-player dropdown — NOT a multi-tab UI. The dropdown's *infrastructure* comes back; it stays single-option until Phase 26 adds providers.
5. Backend allow-listing: `services/player/internal/handler/report.go::allowedPlayerTypes` and `services/player/internal/domain/preference.go::ValidPlayers` grow `"english": true` entries (verified during the 2026-05-19 standalone brainstorm to currently lack them — the original v3.0 Phase 16 plan added these, but the 2026-05-18 cleanup may have stripped them).
6. **Provider verification gate** before any of the above ships: gogoanime + animepahe + animekai (fall-through) each exercised end-to-end against Frieren (MAL 52991) via the curl pipeline in `docs/issues/scraper-provider-verification-2026-05-19.md` (the standalone Phase 0 task list, retained as the gate doc).

**Out of scope:**

- Adding new providers (Phase 26).
- Resolving the BLK-INT-01 hls3 host rotation (Phase 25 — Phase 24 ships even if BLK-INT-01 is still surfacing silent-200s; the affected path is the playback layer, not the player-component layer).
- `has_english` GORM column on `Anime` + browse-filter activation (deferred to Phase 26 once provider expansion guarantees enough rows for the filter to be useful).
- Health-aware tab hiding (`scraperApi.getHealth()` + 60s cache in Anime.vue) — deferred; the EN tab is shown unconditionally and an empty-state inside EnglishPlayer covers the "no providers responding" case.
- Source dropdown lighting up with multiple providers — single-option per Phase 26 dependency.

**Requirements covered:** SCRAPER-HEAL-17, SCRAPER-HEAL-18, SCRAPER-HEAL-19, SCRAPER-HEAL-20.

</domain>

<decisions>
## Implementation Decisions

### D1 — Restore from git, do NOT rewrite from scratch

The standalone 2026-05-19 plan proposed writing a new ~700-line EnglishPlayer.vue from scratch. That was the right call before knowing the original 1973-line file was recoverable. Now that we have it (commit `8424e99`), restoring keeps all the polish accumulated through 16-06 + 18-04 + 21-04 (sub/dub toggle, fullscreen handler precedence fix, three-phase loader, multi-source dropdown infrastructure, ARIA semantics, rollback-on-fail UX, late-arriving preferredCombo fix from commit `6409510`).

Risk: the restored snapshot may have drifted from current contracts. Mitigation: each drift gets fixed inline by the restoration plan (one task per drift), not deferred or punted to a follow-up phase.

### D2 — Provider verification is a Wave-0 hard gate

The operator explicitly demanded "test each provider" before any other work. Phase 24's first task is a provider verification pass against Frieren (MAL 52991) with results committed to `docs/issues/scraper-provider-verification-2026-05-19.md`. If gogoanime or animepahe fails (animekai is expected to fall-through and is allowed to fail-as-disabled), Phase 24 frontend tasks pause until the provider is recovered or formally disabled via `SCRAPER_DEGRADED_PROVIDERS`. This is non-negotiable.

### D3 — Single tab + in-player dropdown (NOT multi-tab)

The 2026-05-12 design landed one "English" tab with an in-player Source dropdown. The cleanup correctly removed the multi-tab complexity; Phase 24 restores the single-tab pattern, NOT the multi-tab world the older i18n keys hinted at. The dropdown's infrastructure (the `<select>` element + `selectedServer` ref + per-server preference persistence via `useWatchPreferences.preferredScraperProvider`) comes back, but stays single-option until Phase 26.

### D4 — Defer browse-filter activation to Phase 26

`has_english` column + `BrowseSidebar` filter row are not in Phase 24. Reason: with only gogoanime + animepahe live and the filter populated opportunistically per-anime-view, the filter would match approximately zero rows on first ship and look broken to users. Phase 26's AllAnime lift roughly doubles candidate coverage, making the filter useful enough to ship.

### D5 — Defer health-aware tab hiding to "future polish, not v3.1"

The standalone plan proposed `scraperApi.getHealth()` on `Anime.vue` mount with a 60s cache to hide the tab when all providers are DOWN. Phase 24 ships without it: the EN tab is shown unconditionally and EnglishPlayer's own empty-state ("No English episodes available — try Kodik or AnimeLib") covers the failure mode. Reason: adds a code path that's exercised only during full outages, and the empty-state is functionally equivalent. If users complain about "EN tab that does nothing," revisit in a follow-up cycle.

### D6 — Restored EnglishPlayer keeps existing component contracts; no React-style "ground up"

The 2026-05-19 plan proposed `hls.js` directly (no Video.js). The restored snapshot used `hls.js` already (per commit `27c6cd5` and earlier). Decision: trust the restored snapshot. If the restored code uses Video.js OR plain `hls.js` OR Safari-native, that's the canonical implementation. The restoration task does NOT re-pick the HLS library.

### D7 — Provider verification log lives in `docs/issues/`, not `.planning/`

The verification log is a one-time pre-flight check, not a planning artifact. Following the existing convention (e.g., `docs/issues/ui-audit-2026-05-12.md`), it lives in `docs/issues/scraper-provider-verification-2026-05-19.md`. Phase 25 (SCRAPER-HEAL-21 self-healing) may refresh it; otherwise it's a one-off.

</decisions>

<open_questions>
None — all design questions resolved during the 2026-05-19 standalone brainstorm and absorbed into D1-D7 above. The remaining open items are implementation discoveries that emerge during plan-writing (e.g., "does the restored EnglishPlayer's `useI18n()` import match the current shape?") and are owned by `/gsd-plan-phase`.
</open_questions>

<risks>
## Risks specific to this phase

- **Drift between restored snapshot and current contracts**: the snapshot is 6 days old. `scraperApi`, `useWatchPreferences`, `ReportButton`, `SubtitleOverlay`, `OtherSubsPanel`, the i18n key set, and `Anime.vue`'s mount props may all have shifted underneath. Mitigation: restoration plan includes one explicit "diff and reconcile" task per import surface. Compiles via `bunx tsc --noEmit` are the green-light gate before mount.
- **Provider verification turns up a real outage**: if gogoanime or animepahe is down on the day Phase 24 starts, the hard gate blocks. Mitigation: D2 — the gate is the right behavior; we ship a working surface or we don't ship. Operator may temporarily disable the failing provider via `SCRAPER_DEGRADED_PROVIDERS` to unblock if the outage is upstream-side and the orchestrator's fall-through is verified.
- **Players service `ValidPlayers` map regression**: the 2026-05-18 cleanup may have stripped `"english": true` from `services/player/internal/domain/preference.go::ValidPlayers` and `services/player/internal/handler/report.go::allowedPlayerTypes`. If so, the restored EnglishPlayer's ReportButton + preference-save calls will 422. Mitigation: explicit task in the plan to re-add both entries and verify via curl before frontend redeploy.
- **The restored 1973-line file is heavier than the current `AnimeLibPlayer.vue` (869 lines)**: this is a known artifact of accumulated polish (sub/dub toggle, multi-source dropdown, three-phase loader, ARIA semantics, rollback UX). Decision is to keep all of it (D1). If maintenance burden becomes a real problem later, that's a follow-up refactor — not a Phase 24 concern.
- **`stale-localStorage sanitization fix from ee4ed56` interacts with the re-widened whitelists**: users currently storing `'english'` in localStorage will start seeing the EN tab as their default after Phase 24 ships, which is correct behavior. Users storing `'hianime'` / `'consumet'` will fall back to `'kodik'` cleanly (existing sanitization). No expected surprise.
</risks>

<dependencies>
## Phase Dependencies

- **Hard dependency on:** v3.0 Phase 15-19 (scraper microservice + providers operational), v3.0 Phase 20 (cutover that triggered the regression), v3.1 Phases 21-23 (the regressed surface this phase restores).
- **No dependency on:** Phase 25 (audit findings — independent), Phase 26 (provider expansion — independent; Phase 24 ships with single-option dropdown).
- **Blocked by:** nothing — Phase 24 is the next-actionable phase.
- **Blocks:** v3.0 Phase 20 cutover's "≥ 7 days clean prod traffic on EnglishPlayer" soak gate cannot start until Phase 24 ships. (Phase 20 itself is already shipped — the cutover landed — but the SOAK gate for the dead-code-deletion success criteria was structurally undefined while EnglishPlayer was missing.)
</dependencies>

<plan_sketch>
## Plan Sketch (for `/gsd-plan-phase` to flesh out)

**Wave 0 — Provider Verification (HARD GATE)**

- 24-00-PLAN.md: `docs/issues/scraper-provider-verification-2026-05-19.md` — exercise gogoanime + animepahe + animekai-fall-through end-to-end against Frieren (MAL 52991). Curl pipeline + verdict matrix. SCRAPER-HEAL-20.

**Wave 1 — Backend allow-list (independent, parallel with Wave 2)**

- 24-01-PLAN.md: `services/player/internal/domain/preference.go::ValidPlayers` + `services/player/internal/handler/report.go::allowedPlayerTypes` grow `"english": true`. `go build ./... && go test ./...` green. `make redeploy-player`. (Skipped if grep shows entries already present.)

**Wave 2 — Restore + Reconnect (parallel with Wave 1)**

- 24-02-PLAN.md: Restore `EnglishPlayer.vue` from `git show 8424e99:frontend/web/src/components/player/EnglishPlayer.vue`. Diff against current `scraperApi` / `useWatchPreferences` / `ReportButton` / `SubtitleOverlay` / `OtherSubsPanel` / vue-i18n / TypeScript-strict contracts; reconcile inline. `bunx tsc --noEmit && bunx eslint` green. SCRAPER-HEAL-17.
- 24-03-PLAN.md: i18n keys (6 keys × 3 locales). `bun run lint:i18n` shows `Missing keys: 0`. SCRAPER-HEAL-19.
- 24-04-PLAN.md: `Anime.vue` re-mount — type-union widening, EN tab button, v-else-if branch, `switchLanguage` `'en'` branch, `videoProvider` save watcher `'en'` branch, `applyResolvedCombo` filter removal. `bunx tsc --noEmit && bunx eslint` green. SCRAPER-HEAL-18.

**Wave 3 — Deploy + E2E + after-update**

- 24-05-PLAN.md: `make redeploy-web` + manual smoke (logged-out + ui_audit_bot) + new Playwright spec `frontend/web/tests/e2e/english-player.spec.ts` + `/animeenigma-after-update`. Done = EN tab visible on production, episode plays, changelog updated, commit on `origin/main`.
</plan_sketch>
