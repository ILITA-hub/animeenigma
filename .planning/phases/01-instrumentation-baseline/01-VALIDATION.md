---
phase: 1
slug: instrumentation-baseline
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-27
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | `go test` + `testify v1.8.4` (existing in `services/player/go.mod`) |
| **Frontend unit framework** | Vitest 1.x (NEW — Wave 0 install: `bun add -d vitest @vue/test-utils happy-dom`) |
| **Frontend E2E framework** | Playwright 1.58.0 (existing — `frontend/web/package.json`) |
| **Backend test config** | `services/player/go.mod` — no separate config file |
| **Frontend unit config** | `frontend/web/vitest.config.ts` (NEW — Wave 0 gap) |
| **Quick run command — backend** | `cd services/player && go test ./internal/handler/... -run Override` |
| **Quick run command — frontend unit** | `cd frontend/web && bunx vitest run src/composables/useOverrideTracker.test.ts` |
| **Quick run command — frontend E2E** | `cd frontend/web && bunx playwright test combo-override` |
| **Full suite — backend** | `cd services/player && go test ./...` |
| **Full suite — frontend** | `cd frontend/web && bunx vitest run && bunx playwright test` |
| **Phase gate** | `make health` all-green; PromQL probe shows both `combo_override_total` AND `combo_resolve_total` present |
| **Estimated runtime** | ~30s backend, ~10s frontend unit, ~60s frontend E2E |

---

## Sampling Rate

- **After every task commit:** Run the unit suite for the touched layer
  - Composable changes: `bunx vitest run src/composables/useOverrideTracker.test.ts`
  - Backend handler changes: `cd services/player && go test ./internal/handler/...`
  - Metrics changes: `cd libs/metrics && go test ./...`
- **After every plan wave:** Full unit suite + lint
  - Backend: `cd services/player && go test ./...`
  - Frontend: `cd frontend/web && bunx vitest run && bunx eslint src/ && bunx tsc --noEmit`
- **Before `/gsd-verify-work`:** Full suite must be green + Playwright E2E green
- **Max feedback latency:** 30 seconds (per-task), 90 seconds (per-wave)

---

## Per-Task Verification Map

| # | Plan | Wave | Requirement | Behavior | Test Type | Automated Command | File Exists |
|---|------|------|-------------|----------|-----------|-------------------|-------------|
| 1 | TBD | 0 | M-01 | Vitest config + dev deps installed | unit | `cd frontend/web && bunx vitest --version` | ❌ Wave 0 |
| 2 | TBD | 1 | M-01 | Composable detects user-initiated language change within 30s window, emits POST | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "language dimension"` | ❌ Wave 0 |
| 3 | TBD | 1 | M-01 | Composable detects user-initiated team change | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "team dimension"` | ❌ Wave 0 |
| 4 | TBD | 1 | M-01 | Composable detects user-initiated episode change | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "episode dimension"` | ❌ Wave 0 |
| 5 | TBD | 2 | M-01 | Composable detects user-initiated player change (Anime.vue level) | unit | `bunx vitest run src/views/Anime.test.ts -t "player override"` | ❌ Wave 0 |
| 6 | TBD | 1 | M-01 | Composable IGNORES auto-advance (Pitfall 1) | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "ignores auto-advance"` | ❌ Wave 0 |
| 7 | TBD | 1 | M-01 | Composable DEBOUNCES double-clicks within 250ms to one event | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "debounces"` | ❌ Wave 0 |
| 8 | TBD | 1 | M-01 | Composable IGNORES second change to same dimension after first emitted | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "first per dimension only"` | ❌ Wave 0 |
| 9 | TBD | 1 | M-01 | Composable IGNORES changes after 30s window | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "30s window expires"` | ❌ Wave 0 |
| 10 | TBD | 1 | M-01 | Window starts when resolvedCombo APPLIED, not on mount (Pitfall 2) | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "window starts on apply"` | ❌ Wave 0 |
| 11 | TBD | 1 | M-01 | Backend handler increments `ComboOverrideTotal` with correct labels | unit | `cd services/player && go test ./internal/handler -run TestOverride_IncrementsCounter` | ❌ Wave 0 |
| 12 | TBD | 1 | M-01 | Backend handler emits `combo_override` log line with structured fields | unit | `cd services/player && go test ./internal/handler -run TestOverride_LogsStructured` | ❌ Wave 0 |
| 13 | TBD | 1 | M-01 | Backend handler validates `dimension` against whitelist (Pitfall 3) | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsInvalidDimension` | ❌ Wave 0 |
| 14 | TBD | 1 | M-01 | Backend handler accepts JWT-authenticated requests | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsJWT` | ❌ Wave 0 |
| 15 | TBD | 1 | M-01 | Backend handler accepts X-Anon-ID requests | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsAnonID` | ❌ Wave 0 |
| 16 | TBD | 1 | M-01 | Backend handler rejects requests with neither JWT nor X-Anon-ID | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsBothMissing` | ❌ Wave 0 |
| 17 | TBD | 1 | M-01 | OptionalAuthMiddleware decodes JWT when present, no-op when absent | unit | `cd services/player && go test ./internal/transport -run TestOptionalAuth` | ❌ Wave 0 |
| 18 | TBD | 1 | M-01 | Resolver also emits `ComboResolveTotal` with same label set | unit | `cd services/player && go test ./internal/service -run TestResolve_IncrementsComboCounter` | ❌ Wave 0 |
| 19 | TBD | 2 | M-01 | E2E auth-user override flow end-to-end | integration | `bunx playwright test combo-override.spec.ts -t "auth user language change"` | ❌ Wave 0 |
| 20 | TBD | 2 | M-01 | E2E anon-user override flow end-to-end | integration | `bunx playwright test combo-override.spec.ts -t "anon user team change"` | ❌ Wave 0 |
| 21 | TBD | 2 | M-02 | Grafana dashboard panel JSON parses (no syntax errors) | unit | `cd docker/grafana/dashboards && jq . preference-resolution.json > /dev/null` | ✅ |
| 22 | TBD | 3 | M-02 | PromQL `rate(combo_override_total[5m])` returns non-empty after smoke tests | manual-semi | `curl -s 'https://admin.animeenigma.ru/prometheus/api/v1/query?query=combo_override_total' \| jq '.data.result \| length > 0'` | manual |
| 23 | TBD | 3 | M-02 | Grafana panel renders with "Auto-Pick Override Rate" title | manual | Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1` | manual |
| 24 | TBD | 3 | M-02 | Baseline snapshot ≥ 24h captured and recorded in PROJECT.md | manual | `grep -A 10 "Baseline override rate" .planning/PROJECT.md` | manual (Phase gate before Phase 6) |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

*Note: The `Plan` column will be filled in by the planner with the actual plan IDs (e.g., `01-01`, `01-02`, …). Wave numbers may be revised by the planner based on dependency analysis.*

---

## Wave 0 Requirements

These must be created in Wave 0 before any implementation work begins:

- [ ] `frontend/web/vitest.config.ts` — Vitest config; install via `bun add -d vitest @vue/test-utils happy-dom`
- [ ] `frontend/web/src/composables/useOverrideTracker.test.ts` — covers M-01 composable detection logic
- [ ] `frontend/web/src/views/Anime.test.ts` — covers M-01 player-dimension tracking
- [ ] `frontend/web/tests/e2e/combo-override.spec.ts` — covers M-01 E2E flow (auth + anon)
- [ ] `services/player/internal/handler/override_test.go` — covers M-01 backend handler (counter + log + validation)
- [ ] `services/player/internal/transport/optional_auth_test.go` — covers M-01 middleware
- [ ] `services/player/internal/service/preference_resolve_combo_test.go` — assert `ComboResolveTotal` increments

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Production PromQL probe non-zero after live traffic | M-02 | Requires real production traffic | After deploy, trigger 1+ override on prod, then `curl -s 'https://admin.animeenigma.ru/prometheus/api/v1/query?query=combo_override_total' \| jq '.data.result \| length'` returns ≥ 1 |
| Grafana panel renders correctly with all 5 segmentations | M-02 | Requires browser visual confirmation | Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1`, confirm "Auto-Pick Override Rate" panel renders with tier / language / anon-vs-auth / player / dimension segmentations |
| 24h baseline captured and written to PROJECT.md | M-02, success criterion 3 | Requires 24+ hours of real traffic | After 24h soak: extract `rate(combo_override_total[5m]) / rate(combo_resolve_total[5m])` per segment, write to `.planning/PROJECT.md` § "Baseline override rate" |
| Live deployment via `make redeploy-player` healthy | success criterion 4 | Requires production environment | `make redeploy-player && make health` returns all-green; both counters appear in `/metrics` exposition |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (7 files)
- [ ] No watch-mode flags (`vitest run`, not `vitest`; `playwright test`, not `playwright test --ui`)
- [ ] Feedback latency < 30s per task
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
