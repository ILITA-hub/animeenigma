---
phase: 01-instrumentation-baseline
verified: 2026-04-27T15:30:00Z
status: passed
score: 12/12
overrides_applied: 1
deferred:
  - truth: "A baseline override-rate snapshot (>= 24 hours of real traffic) is captured and recorded in PROJECT.md before Phase 6 starts"
    addressed_in: "Phase 6 gate"
    evidence: "ROADMAP.md SC3 explicitly states 'before Phase 6 starts' — not a Phase 1 task. STATE.md Phase 1 Follow-ups section carries the reminder with the PromQL one-liner. VALIDATION.md also classifies this as a manual-only phase-gate."
human_verified:
  - test: "Grafana panel 'Auto-Pick Override Rate (Phase 1 Baseline)' renders with all 5 sub-panels at https://animeenigma.ru/admin/grafana/d/preference-resolution-v1/preference-resolution"
    confirmed_by: user
    confirmed_at: 2026-04-27T13:50:00Z
    confirmation: "Grafana panels ok"
---

# Phase 1: Instrumentation Baseline — Verification Report

**Phase Goal:** Smart Watch Picker baseline instrumentation — emit `combo_override_total` and `combo_resolve_total` Prometheus metrics from production traffic, segmented by tier/player/language/anon, and surface them in a Grafana dashboard. Anon-friendly so anon users contribute to both numerator and denominator.
**Verified:** 2026-04-27T15:30:00Z
**Status:** passed (Grafana visual gate confirmed by user "Grafana panels ok" at 2026-04-27T13:50:00Z)
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| SC1 | `combo_override` event emitted when logged-in or anon user changes language/player/team/episode within 30s of player load, on any of the 4 players | VERIFIED | Live `/metrics` shows `combo_override_total{anon="true",dimension="language",...} 1`. All 4 player `.vue` files import `useOverrideTracker`; each has >= 3 `recordPickerEvent` call sites. Anime.vue tracks the `player` dimension via `onUserPickedProvider`. |
| SC2 | Grafana dashboard tile shows override rate segmented by tier, language, anonymous-vs-auth, and player, refreshing within 1 minute | HUMAN_NEEDED | Dashboard JSON is valid, row id=100 "Auto-Pick Override Rate (Phase 1 Baseline)" exists, 5 panels (ids 101-105) cover all 5 D-15 segmentations, PromQL targets both counter families. Visual rendering requires human confirmation. |
| SC3 | Baseline override-rate snapshot (>= 24h of real traffic) captured and recorded in PROJECT.md | DEFERRED | Explicitly a pre-Phase-6 gate per ROADMAP.md ("before Phase 6 starts"). STATE.md carries the reminder. See deferred section. |
| SC4 | Instrumentation deployed via `make redeploy-player` and verified live on production | VERIFIED | `make redeploy-player`, `make redeploy-gateway`, `make redeploy-web` all completed successfully. Live smoke tests: anon resolve → 200, anon override → 204, invalid dimension → 400, both-missing identity → 400. `/metrics` exposes both HELP lines. |

**Score (excluding deferred SC3):** 2 VERIFIED + 1 HUMAN_NEEDED + 1 DEFERRED = 11/12 total must-haves verified (see full breakdown below).

---

### Full Must-Have Truths (All Plans)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | ComboOverrideTotal and ComboResolveTotal exist as Prometheus CounterVecs with the labels documented in CONTEXT D-04/D-05 | VERIFIED | `libs/metrics/watch.go`: both `promauto.NewCounterVec` definitions exist. Labels: `["tier","dimension","language","anon","player"]` (override) and `["tier","language","anon","player"]` (resolve). |
| 2 | OverrideHandler accepts JWT-bearing requests and X-Anon-ID-only requests | VERIFIED | `services/player/internal/handler/override.go` exists; defines `OverrideHandler`, `NewOverrideHandler`, `RecordOverride`. Live smoke 4c (anon override) → 204. |
| 3 | OverrideHandler rejects requests with neither identity (400, no counter increment — T-01-01) | VERIFIED | `validDimensions` whitelist enforced. Live smoke 4e (no JWT, no X-Anon-ID) → 400 with "X-Anon-ID required for unauthenticated requests". |
| 4 | OverrideHandler rejects invalid `dimension` values (400 — T-01-02) | VERIFIED | Live smoke 4d (dimension=INVALID) → 400. |
| 5 | OverrideHandler emits one Prometheus increment AND one structured `combo_override` log line per accepted request | VERIFIED | `metrics.ComboOverrideTotal.WithLabelValues(...).Inc()` present (grep count=1). `h.log.Infow("combo_override", ...)` present (grep count=1). Live metrics show increment after smoke. |
| 6 | Log line contains user_id OR anon_id but never username/token/PII (T-01-03) | VERIFIED | PII grep on override.go (excluding comments) returns 0 matches for username/authorization/bearer. |
| 7 | OptionalAuthMiddleware exists, attaches claims when JWT present, no-ops when absent | VERIFIED | `services/player/internal/transport/optional_auth.go` defines `OptionalAuthMiddleware`. Does NOT call `httputil.Unauthorized` (grep=0). |
| 8 | POST /api/preferences/override registered outside JWT group, behind OptionalAuthMiddleware | VERIFIED | `r.Route("/preferences"...)` exists in player router. `OptionalAuthMiddleware(jwtConfig)` present in group. Route is outside `/users` JWT group. |
| 9 | POST /api/preferences/resolve moved out of /users/* JWT group, behind OptionalAuthMiddleware | VERIFIED | `awk '/r.Route\("\/users"/,/^\s+\}\)/'` of router.go shows preferences/resolve count=0 inside /users. New /preferences route group confirmed. |
| 10 | ResolvePreference handler accepts callers with empty userID (anon path) | VERIFIED | `httputil.Unauthorized` count=0 inside ResolvePreference function. `claims.UserID` fallback present. BL-01 fix (d098f0a) also fixed envelope unwrap in useWatchPreferences, confirmed via live smoke 4b → 200. |
| 11 | PreferenceService.Resolve increments ComboResolveTotal alongside existing PreferenceResolutionTotal | VERIFIED | `metrics.ComboResolveTotal.WithLabelValues` count=1 in preference.go. `metrics.PreferenceResolutionTotal.WithLabelValues` count=1. `labelOrUnknownService` helper present. Live metrics show `combo_resolve_total{anon="true",...} 2` and `combo_resolve_total{anon="false",...} 2`. |
| 12 | Gateway proxies /api/preferences/* without JWT validation (Critical Finding 1) | VERIFIED | `r.HandleFunc("/preferences/*", proxyHandler.ProxyToPlayer)` present in gateway router (count=1). Outside any JWTValidation group (group-awk count=0). |
| 13 | getOrCreateAnonId() returns the same UUIDv4 across calls within a browsing session (localStorage-persisted) | VERIFIED | `frontend/web/src/utils/anonId.ts` exists. Exports `getOrCreateAnonId`. Module-scoped `let cached` present. `crypto.randomUUID()` used. `aenig_anon_id` key (STORAGE_KEY const). |
| 14 | axios client always attaches X-Anon-ID header (always-set, not else-branched) | VERIFIED | `grep -c "X-Anon-ID" client.ts` = 4. `grep -B2 "X-Anon-ID" client.ts | grep -c "else if"` = 0. `getOrCreateAnonId` import present. |
| 15 | useOverrideTracker composable exists with documented API: 30s window, 250ms debounce, per-dimension lock, best-effort catch | VERIFIED | File exists. `export function useOverrideTracker` present. `WINDOW_MS = 30_000` + window check present. `DEBOUNCE_MS = 250` present. `emittedDimensions.add/has` present (count=3). `onUnmounted` present. No `watch(props/currentEpisode)` anti-pattern (count=0). |
| 16 | useWatchPreferences no longer short-circuits on !authStore.isAuthenticated | VERIFIED | `grep -c "!authStore.isAuthenticated" useWatchPreferences.ts` = 0. `available.length === 0` guard remains. WR-14 fix also drops auth gate in Anime.vue initPreferences. |
| 17 | userApi.recordOverride and userApi.resolvePreference target /preferences/* (NOT /users/preferences/*) | VERIFIED | `grep -c "post.*'/preferences/resolve'" client.ts` = 1. `grep -c "post.*'/users/preferences/resolve'" client.ts` = 0. `recordOverride:` post to `/preferences/override` = 1. |
| 18 | All 4 player components import useOverrideTracker and call recordPickerEvent in click handlers | VERIFIED | All 4 players found by `grep -l "useOverrideTracker"`. recordPickerEvent counts: Kodik=3, AnimeLib=5, HiAnime=6, Consumet=5. No player calls recordPickerEvent from a watch() block (count=0). |
| 19 | HiAnime + Consumet auto-advance bypass via _advanceServer/_advanceEpisode siblings | VERIFIED | HiAnime: 17 `_advanceServer/_advanceEpisode` references. Consumet: 9 references. |
| 20 | DEV-only force-advance window hooks in HiAnime + Consumet, gated by import.meta.env.DEV | VERIFIED | `__aenigForceAdvanceHiAnime` count=2 in HiAnimePlayer. `__aenigForceAdvanceConsumet` count=2 in ConsumetPlayer. Both gated by `import.meta.env.DEV` (5-line lookback confirms). |
| 21 | Anime.vue tracks player dimension via useOverrideTracker; 4 tracked-provider buttons use onUserPickedProvider | VERIFIED | `import.*useOverrideTracker` count=1. `@click="onUserPickedProvider` count=4. Direct `@click="videoProvider = 'kodik/animelib/hianime/consumet'"` count=0. |
| 22 | E2E spec has 7 real test bodies, no test.skip(true) / test.fixme | VERIFIED | `test.skip(true` count=0. `test.fixme` count=0. `bunx playwright test combo-override --list` = 21 tests (7×3 browsers). `__aenigForceAdvanceHiAnime/__aenigForceAdvanceConsumet` referenced in spec (count=6). |
| 23 | Grafana dashboard JSON is valid with row id=100 "Auto-Pick Override Rate (Phase 1 Baseline)" and 5 panels (ids 101-105) | VERIFIED | `jq .` exits 0. Row title confirmed. 5 non-row panels confirmed. IDs unique (true). combo_override_total refs=5, combo_resolve_total refs=4. All 5 D-15 segmentations: tier, player, language+anon, dimension all present. |
| 24 | Grafana panel renders correctly in production (visual confirmation) | HUMAN_NEEDED | Dashboard JSON verified programmatically. Visual rendering requires human confirmation (Task 2 checkpoint). Live evidence provided states user visually confirmed all 5 sub-panels rendered. |
| 25 | PROJECT.md documents Loki 7d retention constraint | VERIFIED | `grep -ic "loki retention" PROJECT.md` = 3. `grep -cE "168h\|7 days\|7d" PROJECT.md` = 4. 31d documentation error corrected. Phase 5 escape hatch documented. |
| 26 | STATE.md contains Phase 1 follow-up marker for 24h baseline capture before Phase 6 | VERIFIED | `grep -c "Phase 1 follow-up" STATE.md` = 1. `grep -c "rate(combo_override_total" STATE.md` = 1. Phase 6 gating noted. |
| 27 | changelog.json has entry describing Phase 1 instrumentation | VERIFIED | `jq -r '.[0].entries[].message' changelog.json | grep -iE "override\|baseline\|instrumentation"` returns match: "Grafana-панель «Auto-Pick Override Rate»...". |
| 28 | git commit + push completed with 3 required co-authors | VERIFIED | Commit 2ccdb67 has Co-Authored-By: Claude Opus 4.6, 0neymik0, NANDIorg. `git status --porcelain` = 0 lines. `git rev-list HEAD..origin/main --count` = 0. |

**Score:** 11/12 truths verified (SC3 deferred; SC2/Grafana visual is the human_needed item).

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `libs/metrics/watch.go` | ComboOverrideTotal + ComboResolveTotal CounterVecs | VERIFIED | Both promauto.NewCounterVec definitions present with correct label sets |
| `services/player/internal/handler/override.go` | OverrideHandler.RecordOverride | VERIFIED | 139 lines, all acceptance greps pass, PII-clean |
| `services/player/internal/transport/optional_auth.go` | OptionalAuthMiddleware | VERIFIED | Exported, no Unauthorized call, inverted control flow |
| `services/player/internal/transport/router.go` | /preferences route group + OverrideHandler wiring | VERIFIED | /preferences route with OptionalAuth, both endpoints wired |
| `services/player/internal/handler/preference.go` | Anon-friendly ResolvePreference | VERIFIED | No Unauthorized in ResolvePreference; userID fallback present |
| `services/player/internal/service/preference.go` | ComboResolveTotal emit | VERIFIED | ComboResolveTotal.WithLabelValues + labelOrUnknownService present |
| `services/player/cmd/player-api/main.go` | OverrideHandler instantiation | VERIFIED | `NewOverrideHandler` count=1 |
| `services/gateway/internal/transport/router.go` | /preferences/* proxy outside JWT group | VERIFIED | HandleFunc line present, outside JWT group confirmed |
| `frontend/web/src/utils/anonId.ts` | getOrCreateAnonId() | VERIFIED | Exported function, module cache, localStorage key |
| `frontend/web/src/composables/useOverrideTracker.ts` | useOverrideTracker composable | VERIFIED | All invariants (D-07/D-08/D-09/D-10) implemented |
| `frontend/web/src/api/client.ts` | X-Anon-ID interceptor + recordOverride | VERIFIED | Always-set header, migrated path, new endpoint |
| `frontend/web/src/composables/useWatchPreferences.ts` | Anon-friendly resolve composable | VERIFIED | Short-circuit removed, envelope unwrap applied (BL-01 fix) |
| `frontend/web/src/components/player/KodikPlayer.vue` | Override tracking wired | VERIFIED | useOverrideTracker imported, 3 recordPickerEvent calls |
| `frontend/web/src/components/player/AnimeLibPlayer.vue` | Override tracking wired | VERIFIED | useOverrideTracker imported, 5 recordPickerEvent calls |
| `frontend/web/src/components/player/HiAnimePlayer.vue` | Override tracking + auto-advance bypass | VERIFIED | useOverrideTracker, 6 recordPickerEvent, 17 _advance* refs, DEV hook |
| `frontend/web/src/components/player/ConsumetPlayer.vue` | Override tracking + auto-advance bypass | VERIFIED | useOverrideTracker, 5 recordPickerEvent, 9 _advance* refs, DEV hook |
| `frontend/web/src/views/Anime.vue` | Player-dimension tracking | VERIFIED | useOverrideTracker imported, onUserPickedProvider wired (4 buttons) |
| `frontend/web/e2e/combo-override.spec.ts` | 7 E2E tests, no skip/fixme | VERIFIED | 21 tests listed (7×3 browsers), 0 skip, 0 fixme |
| `docker/grafana/dashboards/preference-resolution.json` | Override-rate panel row | VERIFIED | Valid JSON, row id=100, 5 panels ids 101-105, all segmentations present |
| `.planning/PROJECT.md` | Loki 7d retention subsection | VERIFIED | Section present with 31d correction and Phase 5 escape hatch |
| `.planning/STATE.md` | Phase 1 follow-up marker | VERIFIED | Marker present with PromQL and Phase 6 gating |
| `frontend/web/public/changelog.json` | User-facing changelog entry | VERIFIED | Russian-language entry mentioning Grafana override-rate panel |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `services/player/internal/handler/override.go` | `libs/metrics.ComboOverrideTotal` | `metrics.ComboOverrideTotal.WithLabelValues(...).Inc()` | VERIFIED | grep count=1 |
| `services/player/internal/handler/override.go` | `libs/logger.Infow` | `h.log.Infow("combo_override", ...)` | VERIFIED | grep count=1 |
| `services/player/internal/transport/router.go` | `handler.OverrideHandler` | `overrideHandler.RecordOverride` | VERIFIED | Route registration confirmed |
| `services/player/internal/transport/router.go` | `transport.OptionalAuthMiddleware` | `OptionalAuthMiddleware(jwtConfig)` | VERIFIED | grep count=2 in router.go |
| `services/gateway/internal/transport/router.go` | `/api/preferences/*` | `proxyHandler.ProxyToPlayer outside JWT group` | VERIFIED | HandleFunc line present, outside JWT group |
| `services/player/internal/service/preference.go` | `libs/metrics.ComboResolveTotal` | `metrics.ComboResolveTotal.WithLabelValues(...).Inc()` | VERIFIED | grep count=1 |
| `frontend/web/src/api/client.ts` | `frontend/web/src/utils/anonId.ts` | `getOrCreateAnonId() in axios interceptor` | VERIFIED | Import and usage confirmed |
| `frontend/web/src/composables/useOverrideTracker.ts` | `frontend/web/src/api/client.ts` | `userApi.recordOverride` | VERIFIED | `recordOverride` called in emit() |
| `{Kodik,AnimeLib,HiAnime,Consumet}Player.vue` | `useOverrideTracker.ts` | `tracker.recordPickerEvent in click handlers` | VERIFIED | All 4 players wired |
| `frontend/web/src/views/Anime.vue` | `useOverrideTracker.ts` | `recordPickerEvent('player', ...)` | VERIFIED | onUserPickedProvider calls recordPickerEvent |

---

## Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `useWatchPreferences.ts` | `resolvedCombo` | `userApi.resolvePreference` → `/api/preferences/resolve` → PreferenceService.Resolve | Yes — DB-backed resolver (Tier 1-5), live smoke confirmed 200 response with `{data:{resolved:{...}}}` body. BL-01 fix (d098f0a) unwraps the envelope correctly. | FLOWING |
| `combo_override_total` Prometheus counter | counter value | `OverrideHandler.RecordOverride` via POST from `useOverrideTracker.emit()` | Yes — live metrics show `combo_override_total{anon="true",...} 1` after smoke | FLOWING |
| `combo_resolve_total` Prometheus counter | counter value | `PreferenceService.Resolve` on every resolve call | Yes — live metrics show `combo_resolve_total{anon="true",...} 2` and `combo_resolve_total{anon="false",...} 2` | FLOWING |
| Grafana panels 101-105 | override rate ratio | PromQL `rate(combo_override_total[5m]) / rate(combo_resolve_total[5m])` | Yes — Prometheus is scraping player:8083/metrics where both counters are live | FLOWING (visual confirmation needed) |

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Both metric HELP lines exposed at /metrics | `curl -s http://localhost:8083/metrics \| grep -cE "^# HELP combo_(override\|resolve)_total"` | 2 | PASS |
| Anon resolve returns 200 | POST /api/preferences/resolve with X-Anon-ID | 200 | PASS |
| Anon override returns 204 | POST /api/preferences/override with X-Anon-ID | 204 | PASS |
| Invalid dimension returns 400 (T-01-02) | POST /api/preferences/override with dimension=INVALID | 400 | PASS |
| Both-missing identity returns 400 (T-01-01) | POST /api/preferences/override with no JWT and no X-Anon-ID | 400 | PASS |
| Player service builds cleanly | `cd services/player && go build ./...` | exit 0 | PASS |
| Gateway builds cleanly | `cd services/gateway && go build ./...` | exit 0 | PASS |
| Playwright spec lists 21 tests, 0 skip | `bunx playwright test combo-override --list` | 21 tests, 0 skip/fixme | PASS |
| Git is clean and in sync with remote | `git status --porcelain`, `git rev-list HEAD..origin/main` | 0, 0 | PASS |

---

## Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|---------------|-------------|--------|----------|
| M-01 | 01-01 through 01-05, 01-07 | Emit `combo_override` event when user changes language/player/team/episode within 30s of player load | SATISFIED | Counter emitting live; all 4 players wired; anon-friendly endpoint; 7 E2E tests in spec |
| M-02 | 01-06, 01-07 | Grafana dashboard tile for override-rate, segmented by tier/language/anon/player | SATISFIED (code) / HUMAN_NEEDED (visual) | Dashboard JSON verified; visual rendering needs human confirmation |

No orphaned requirements — REQUIREMENTS.md maps only M-01 and M-02 to Phase 1. Both are addressed.

---

## Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `useOverrideTracker.ts` | 30s window check fires at call time, POST goes out 250ms later (WR-01 from REVIEW.md) | Warning | Clicks at t≈29.95s pass the gate; POST fires at t≈30.2s. Minor timing inaccuracy, not a blocker for phase goal. |
| `services/player/internal/handler/override.go` | `tier` and `player` labels are NOT whitelisted (WR-04 from REVIEW.md) | Warning | Hostile clients can explode cardinality via these free-form labels. `labelOrUnknown` bounds empty strings but not arbitrary values. Documented in REVIEW.md for follow-up. |
| `services/player/internal/handler/override.go` | `OriginalCombo`/`NewCombo` typed as `map[string]interface{}` and logged verbatim (WR-05 from REVIEW.md) | Warning | Clients can smuggle arbitrary keys into log lines. Documented in REVIEW.md. |
| `services/player/internal/transport/optional_auth.go` | Expired JWT treated as anon silently (WR-06 from REVIEW.md) | Warning | Real users with broken sessions become silent anon increments instead of getting explicit 401. Documented in REVIEW.md. |

None of the above are blockers for Phase 1's goal (instrumentation baseline). All are documented in REVIEW.md for Phase 3+ follow-up.

---

## Human Verification Required

### 1. Grafana Panel Visual Confirmation

**Test:** Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1` (or find via Dashboards > "Preference Resolution"). Expand the "Auto-Pick Override Rate (Phase 1 Baseline)" row. Verify all 5 panels render:

- a. "Override Rate (last 5m)" — stat panel; may show small value from smoke tests
- b. "Override Rate by Tier" — timeseries
- c. "Override Rate by Player" — timeseries
- d. "Override Rate by Language and Auth State" — timeseries
- e. "Overrides by Dimension (24h count)" — barchart; should show at minimum `language: 1` from smoke test

Also confirm: hover stat panel description says "fresh resolves only"; click Edit on any panel and confirm PromQL references `combo_override_total` and `combo_resolve_total`.

Optional: in Grafana Explore, run LogQL `{service="player"} |= "combo_override" | json` over last 1h — verify NO `username` or `Authorization` field in the structured log line (T-01-03 manual confirmation).

**Expected:** All 5 panels render with no "No data" errors on the stat panel (smoke test fired at least 1 override), correct metric names visible in panel editor.

**Why human:** Grafana panel rendering is a visual/browser verification that cannot be reliably automated via CLI. Plan 07 Task 2 is a blocking `checkpoint:human-verify` gate. The live_evidence provided with this verification request states "Grafana panel 'Auto-Pick Override Rate (Phase 1 Baseline)' was visually verified by user as rendering with all 5 sub-panels" — if that confirmation is accepted as equivalent to this human verification, status can be promoted to `passed`.

---

## Deferred Items

Items not yet met but explicitly addressed in later milestone phases.

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | Baseline override-rate snapshot (>= 24h traffic) captured in PROJECT.md | Phase 6 gate | ROADMAP.md SC3 explicitly states "before Phase 6 starts". STATE.md carries PromQL reminder. VALIDATION.md classifies as manual-only phase-gate. The 24h accumulation window starts from deploy (2026-04-27); capture date is 2026-04-28+. |

---

## Gaps Summary

No blocking gaps identified. All Phase 1 must-haves are either VERIFIED (code artifacts exist, patterns present, live behavior confirmed) or deferred/human_needed with documented rationale.

The only item requiring resolution before `passed` can be declared is the Grafana visual confirmation (human_needed). Per live_evidence provided: "Grafana panel 'Auto-Pick Override Rate (Phase 1 Baseline)' was visually verified by user as rendering with all 5 sub-panels." If this statement is accepted as the human verification signal, status upgrades to `passed` with the SC3 baseline deferral intact.

---

_Verified: 2026-04-27T15:30:00Z_
_Verifier: Claude (gsd-verifier)_
