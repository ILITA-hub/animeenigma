---
phase: 01-instrumentation-baseline
plan: 01
subsystem: instrumentation/test-floor
tags: [tdd, red-tests, instrumentation, prometheus, playwright]
requires: []
provides:
  - "Compile-time / runtime contract for OverrideHandler (Wave 1 plan 01-02)"
  - "Compile-time contract for OptionalAuthMiddleware (Wave 1 plan 01-03)"
  - "Compile-time contract for metrics.ComboOverrideTotal & ComboResolveTotal (Wave 1 plan 01-02)"
  - "Runtime contract for anon-friendly ResolvePreference (Wave 1 plan 01-03)"
  - "Playwright E2E test stubs for the 7 override-tracking scenarios (Wave 2 plan 01-05)"
affects: []
tech-stack:
  added: []
  patterns:
    - "go testify + prometheus/client_golang/prometheus/testutil for counter delta assertions"
    - "go.uber.org/zap/zaptest/observer for structured log assertions"
    - "gorm.io/driver/sqlite in-memory + manual CREATE TABLE for resolver flow tests"
    - "Playwright test.describe + test.skip(true, ...) for scaffolded contract specs"
key-files:
  created:
    - services/player/internal/handler/override_test.go
    - services/player/internal/handler/preference_anon_test.go
    - services/player/internal/service/preference_resolve_combo_test.go
    - services/player/internal/transport/optional_auth_test.go
    - frontend/web/e2e/combo-override.spec.ts
  modified: []
decisions:
  - "Used testutil.ToFloat64 + InDelta(0.001) for counter assertions instead of value-equality, since the same Prometheus registry is shared across the test process — only deltas are reliable."
  - "Wired observer.New(zapcore.InfoLevel) directly into a logger.Logger struct literal for log-field introspection, avoiding the need to expose a logger constructor seam in libs/logger."
  - "Used a real *repo.PreferenceRepository over an in-memory SQLite instead of mocking — the resolver flow is short and the repo signatures are concrete, making a stub more brittle than the real thing."
  - "Set test.skip(true, '...') on every Playwright body rather than test.fixme — skip is unconditional and the message cites the unblocking plan (01-05), satisfying the acceptance criterion that the scaffold lists 7 enumerable test names without executing any."
metrics:
  duration: "single session"
  completed: "2026-04-27"
  tasks_completed: 3
  tasks_total: 3
  files_created: 5
  lines_added: 746
---

# Phase 1 Plan 01: Wave 0 Test Floor — Summary

Pre-wrote the verification contract for Phase 1's Wave 1 (plans 02 + 03) and Wave 2 (plans 04 + 05) before any production code is touched. Five test files freeze the behavior the next two waves must satisfy: 4 Go tests reference symbols that do not yet exist (RED until Wave 1 lands), and 1 Playwright spec enumerates 7 scenarios behind `test.skip(true, ...)` until Wave 2 wires the composable.

## Tasks Completed

| # | Task                                                            | Commit  | Files                                                                                                                                          |
| - | --------------------------------------------------------------- | ------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| 1 | Backend handler test scaffold for OverrideHandler               | b1e05ab | services/player/internal/handler/override_test.go                                                                                              |
| 2 | Middleware + service + anon-resolve test scaffolds              | 244f094 | services/player/internal/transport/optional_auth_test.go, services/player/internal/service/preference_resolve_combo_test.go, services/player/internal/handler/preference_anon_test.go |
| 3 | Playwright E2E spec scaffold (combo-override.spec.ts)           | 2f411a3 | frontend/web/e2e/combo-override.spec.ts                                                                                                        |

## Test Files Created

### Go unit tests (4 files — all RED)

#### services/player/internal/handler/override_test.go

8 test functions covering the OverrideHandler contract:
- `TestOverride_IncrementsCounter` — POST valid payload with JWT claims → 204, `metrics.ComboOverrideTotal` deltas by exactly 1.
- `TestOverride_AcceptsJWT` — JWT-authed request emits counter with `anon="false"`.
- `TestOverride_AcceptsAnonID` — `X-Anon-ID` request emits counter with `anon="true"`.
- `TestOverride_RejectsBothMissing` — neither JWT nor X-Anon-ID → 400, no counter increment (T-01-01 mitigation).
- `TestOverride_RejectsInvalidDimension` — `dimension="quality"` → 400, no counter increment (T-01-02 cardinality protection).
- `TestOverride_RejectsMissingAnimeID` — payload without `anime_id` → 400.
- `TestOverride_RejectsMissingLoadSessionID` — payload without `load_session_id` → 400.
- `TestOverride_LogsStructured` — observer-core asserts the `combo_override` log entry has the required fields and no PII (T-01-03: never log usernames or tokens).

#### services/player/internal/transport/optional_auth_test.go

3 test functions covering the OptionalAuthMiddleware passthrough behavior:
- `TestOptionalAuth_NoAuthHeader_PassesThroughWithoutClaims` — missing Authorization → next handler called, no claims attached, 200 OK.
- `TestOptionalAuth_ValidJWT_AttachesClaims` — valid Bearer token → next handler called, claims attached with correct UserID.
- `TestOptionalAuth_MalformedJWT_PassesThroughWithoutClaims` — bad token → next handler called WITHOUT claims (NOT 401 — that's the inversion vs `AuthMiddleware`).

#### services/player/internal/service/preference_resolve_combo_test.go

1 test function covering the resolver counter contract:
- `TestResolve_IncrementsComboCounter` — `PreferenceService.Resolve` must increment `metrics.ComboResolveTotal` by exactly 1 per call. This is the denominator for the `combo_override / combo_resolve` Grafana panel.

#### services/player/internal/handler/preference_anon_test.go

1 test function covering the anon-friendly ResolvePreference contract:
- `TestResolve_AcceptsAnon` — POST `/preferences/resolve` with NO claims and NO X-Anon-ID returns 200 (currently 401). Wave 1 plan 01-03 lifts the early `httputil.Unauthorized` return and falls through to `userID := ""`.

### Playwright E2E spec (1 file — 7 skipped tests)

#### frontend/web/e2e/combo-override.spec.ts

7 test cases inside `test.describe('Combo Override Tracking', ...)`. All bodies are `test.skip(true, 'Wave 2 plan 01-05 wires composable + player components')` with detailed comment plans describing the scenario the Wave 2 implementer must satisfy:

1. **auth user — language change within 10s of player load fires POST `/api/preferences/override`** (login flow + page.route capture + body assertions)
2. **anon user — team change includes `X-Anon-ID` header, no `Authorization` header**
3. **debounce — two clicks within 250ms coalesce to one POST** (uses `page.clock.install()` for deterministic timing)
4. **30s window — click after 31s emits no POST**
5. **first per dimension only — second team click in same session is ignored**
6. **ignores auto-advance — programmatic episode change emits no POST** (cites Pitfall 1: auto-advance must bypass `selectEpisode`)
7. **records `original_combo` and `new_combo` on POST body**

Verification: `bunx playwright test combo-override.spec.ts --list` enumerates 21 entries (7 tests × 3 projects: chromium, firefox, Mobile Chrome).

## RED State — Wave 1 Acceptance Gate

`go test ./...` inside `services/player/` reports the following undefined symbols, exactly as planned:

| Symbol                           | Created in        | First reference                                              |
| -------------------------------- | ----------------- | ------------------------------------------------------------ |
| `handler.NewOverrideHandler`     | plan 01-02        | services/player/internal/handler/override_test.go:61         |
| `metrics.ComboOverrideTotal`     | plan 01-02        | services/player/internal/handler/override_test.go:91         |
| `metrics.ComboResolveTotal`      | plan 01-02        | services/player/internal/service/preference_resolve_combo_test.go:98 |
| `transport.OptionalAuthMiddleware` | plan 01-03      | services/player/internal/transport/optional_auth_test.go:59  |

**For Wave 1 plans (01-02 and 01-03):** Going green is your acceptance gate. When `go test ./internal/handler ./internal/service ./internal/transport -run "TestOverride|TestOptionalAuth|TestResolve_IncrementsComboCounter|TestResolve_AcceptsAnon"` passes from inside `services/player/`, the contract you implemented matches the contract Wave 0 froze.

`preference_anon_test.go` does NOT have a compile-time RED dependency — it compiles today but FAILS at runtime (gets 401 instead of expected 200). Wave 1 plan 01-03 turns it green by lifting the early `httputil.Unauthorized` return in `services/player/internal/handler/preference.go:42-44`.

## Deviations from Plan

None — plan executed exactly as written. The acceptance criteria for all three tasks were met on the first pass:

- Task 1: 8 `TestOverride_*` functions, testutil + metrics imports, ≥ 3 references to `metrics.ComboOverrideTotal` (actual: 14), `go vet` reports the planned undefined symbols.
- Task 2: 3 `TestOptionalAuth_*` functions, ≥ 1 reference to `metrics.ComboResolveTotal` (actual: 4), `TestResolve_AcceptsAnon` defined, "Wave 0 RED test" marker present in all three files.
- Task 3: 1 `test.describe('Combo Override Tracking', ...)`, 7 `test('...')` blocks, 7 `test.skip(true, ...)` calls, 7 distinct exact-match test names, ≥ 7 `page.route|clock.install` citations (actual: 10).

## Threat Model Coverage

The 5 test files together verify the Wave 0 disposition for the threat register documented in 01-01-PLAN.md:

| Threat ID | Verified by |
|-----------|-------------|
| T-01-01 — DoS via headerless flood | `TestOverride_RejectsBothMissing` enforces the gate that requires either JWT OR X-Anon-ID. |
| T-01-02 — Cardinality explosion via free-form labels | `TestOverride_RejectsInvalidDimension` enforces the strict whitelist `{language|player|team|episode}`. |
| T-01-03 — PII leak via structured `combo_override` log line | `TestOverride_LogsStructured` asserts the field set and verifies the username does NOT appear in the entry. |

No new threat surface introduced by Wave 0 (it is test-only).

## Self-Check: PASSED

All 5 created files exist on disk:
- FOUND: services/player/internal/handler/override_test.go
- FOUND: services/player/internal/handler/preference_anon_test.go
- FOUND: services/player/internal/service/preference_resolve_combo_test.go
- FOUND: services/player/internal/transport/optional_auth_test.go
- FOUND: frontend/web/e2e/combo-override.spec.ts

All 3 task commits exist in git:
- FOUND: b1e05ab — `test(01-01): add RED test scaffold for OverrideHandler`
- FOUND: 244f094 — `test(01-01): add RED scaffolds for optional auth, resolve counter, anon resolve`
- FOUND: 2f411a3 — `test(01-01): add Playwright E2E spec scaffold for combo-override tracking`
