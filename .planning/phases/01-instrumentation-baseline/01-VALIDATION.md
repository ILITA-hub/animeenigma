---
phase: 1
slug: instrumentation-baseline
status: approved
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-27
updated: 2026-04-27
test_approach: playwright-only
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
>
> **Test approach (planner-decided):** Playwright-only. Vitest is NOT installed in repo and adding it for ~6 unit tests is yak-shaving for a phase whose explicit goal is "wiring existing infrastructure together." Backend Go tests use existing `go test`; frontend tests use Playwright with `clock.install()` for deterministic timing — covers debounce / 30s window / dimension lock / auto-advance suppression as integration assertions.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | `go test` + `testify v1.8.4` (existing in `services/player/go.mod`) |
| **Frontend E2E framework** | Playwright 1.58.0 (existing — `frontend/web/package.json`) |
| **Frontend unit framework** | None (Playwright covers timing edges via `clock.install()`) |
| **Backend test config** | `services/player/go.mod` — no separate config file |
| **Frontend E2E directory** | `frontend/web/e2e/` (NOT `frontend/web/tests/e2e/`) |
| **Quick run command — backend** | `cd services/player && go test ./internal/handler/... -run Override` |
| **Quick run command — frontend E2E** | `cd frontend/web && bunx playwright test combo-override` |
| **Full suite — backend** | `cd services/player && go test ./...` |
| **Full suite — frontend** | `cd frontend/web && bunx playwright test` |
| **Phase gate** | `make health` all-green; PromQL probe shows both `combo_override_total` AND `combo_resolve_total` present |
| **Estimated runtime** | ~30s backend, ~60s frontend E2E |

---

## Sampling Rate

- **After every task commit:** Run the unit suite for the touched layer
  - Backend handler changes: `cd services/player && go test ./internal/handler/...`
  - Backend transport changes: `cd services/player && go test ./internal/transport/...`
  - Backend service changes: `cd services/player && go test ./internal/service/...`
  - Frontend changes: `cd frontend/web && bunx tsc --noEmit && bunx eslint src/`
- **After every plan wave:** Full unit suite + lint
  - Backend: `cd services/player && go test ./...`
  - Frontend: `cd frontend/web && bunx eslint src/ && bunx tsc --noEmit`
- **Before `/gsd-verify-work`:** Backend full suite green + Playwright E2E `combo-override` spec green
- **Max feedback latency:** 30 seconds (per-task), 90 seconds (per-wave)

---

## Per-Task Verification Map

| # | Plan | Wave | Requirement | Behavior | Test Type | Automated Command | File Exists |
|---|------|------|-------------|----------|-----------|-------------------|-------------|
| 1 | 01-01 | 0 | M-01 | Backend handler test scaffold + middleware test scaffold exist (RED) | unit | `cd services/player && go test ./internal/handler -run TestOverride 2>&1 \| grep -E "FAIL\|undefined\|no test files"` | ❌ Wave 0 |
| 2 | 01-01 | 0 | M-01 | Resolver `ComboResolveTotal` test scaffold exists (RED) | unit | `cd services/player && go test ./internal/service -run TestResolve_IncrementsComboCounter 2>&1 \| grep -E "FAIL\|undefined"` | ❌ Wave 0 |
| 3 | 01-01 | 0 | M-01 | Playwright spec stub `e2e/combo-override.spec.ts` exists (test.skip ok) | e2e | `test -f frontend/web/e2e/combo-override.spec.ts && grep -c "test.describe.*Combo Override" frontend/web/e2e/combo-override.spec.ts` returns >=1 | ❌ Wave 0 |
| 4 | 01-02 | 1 | M-01 | `libs/metrics/watch.go` defines `ComboOverrideTotal` and `ComboResolveTotal` | unit | `cd libs/metrics && go build ./... && grep -c "ComboOverrideTotal\\\|ComboResolveTotal" watch.go` returns 2 | ✅ |
| 5 | 01-02 | 1 | M-01 | Backend handler increments `ComboOverrideTotal` for valid auth request | unit | `cd services/player && go test ./internal/handler -run TestOverride_IncrementsCounter` exits 0 | ✅ |
| 6 | 01-02 | 1 | M-01 | Backend handler accepts JWT requests (`anon=false`) | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsJWT` exits 0 | ✅ |
| 7 | 01-02 | 1 | M-01 | Backend handler accepts X-Anon-ID requests (`anon=true`) | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsAnonID` exits 0 | ✅ |
| 8 | 01-02 | 1 | M-01 | Backend handler rejects requests with neither JWT nor X-Anon-ID | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsBothMissing` exits 0 | ✅ |
| 9 | 01-02 | 1 | M-01 | Backend handler validates `dimension` whitelist (cardinality protection) | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsInvalidDimension` exits 0 | ✅ |
| 10 | 01-02 | 1 | M-01 | Backend handler emits `combo_override` log line with structured fields | unit | `cd services/player && go test ./internal/handler -run TestOverride_LogsStructured` exits 0 | ✅ |
| 11 | 01-03 | 1 | M-01 | OptionalAuthMiddleware decodes JWT when present, no-op when absent | unit | `cd services/player && go test ./internal/transport -run TestOptionalAuth` exits 0 | ✅ |
| 12 | 01-03 | 1 | M-01 | Resolver also emits `ComboResolveTotal` with same label set | unit | `cd services/player && go test ./internal/service -run TestResolve_IncrementsComboCounter` exits 0 | ✅ |
| 13 | 01-03 | 1 | M-01 | `ResolvePreference` handler accepts anon requests via OptionalAuth | unit | `cd services/player && go test ./internal/handler -run TestResolve_AcceptsAnon` exits 0 | ✅ |
| 14 | 01-03 | 1 | M-01 | Player router registers `/api/preferences/override` outside JWT group | unit | `grep -A 5 'r.Route("/preferences"' services/player/internal/transport/router.go \| grep -c "OptionalAuthMiddleware"` returns >=1 | ✅ |
| 15 | 01-03 | 1 | M-01 | Gateway router proxies `/api/preferences/*` outside JWT group | unit | `grep -B 2 'preferences/\*"' services/gateway/internal/transport/router.go \| grep -v "JWTValidation"` non-empty | ✅ |
| 16 | 01-04 | 2 | M-01 | `useOverrideTracker.ts` exports composable factory | unit | `grep -c "export function useOverrideTracker" frontend/web/src/composables/useOverrideTracker.ts` returns 1 | ✅ |
| 17 | 01-04 | 2 | M-01 | `anonId.ts` provides idempotent `getOrCreateAnonId` | unit | `grep -c "export function getOrCreateAnonId" frontend/web/src/utils/anonId.ts` returns 1 | ✅ |
| 18 | 01-04 | 2 | M-01 | `apiClient` interceptor sets `X-Anon-ID` when no JWT | unit | `grep -c "X-Anon-ID" frontend/web/src/api/client.ts` returns >=1 | ✅ |
| 19 | 01-04 | 2 | M-01 | `useWatchPreferences.ts` no longer short-circuits on anon | unit | `grep -c "!authStore.isAuthenticated" frontend/web/src/composables/useWatchPreferences.ts` returns 0 | ✅ |
| 20 | 01-05 | 2 | M-01 | All four player components import `useOverrideTracker` | unit | `grep -l "useOverrideTracker" frontend/web/src/components/player/{Kodik,AnimeLib,HiAnime,Consumet}Player.vue \| wc -l` returns 4 | ✅ |
| 21 | 01-05 | 2 | M-01 | `Anime.vue` invokes tracker for `player` dimension on user-driven `videoProvider` change | unit | `grep -c "useOverrideTracker\\\|recordPickerEvent" frontend/web/src/views/Anime.vue` returns >=2 | ✅ |
| 22 | 01-05 | 2 | M-01 | E2E auth-user override flow end-to-end | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "auth user"` exits 0 | ✅ |
| 23 | 01-05 | 2 | M-01 | E2E anon-user override flow end-to-end (X-Anon-ID header set) | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "anon user"` exits 0 | ✅ |
| 24 | 01-05 | 2 | M-01 | E2E debounce — 250ms double-click coalesces to 1 POST | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "debounce"` exits 0 | ✅ |
| 25 | 01-05 | 2 | M-01 | E2E 30s window expires — click after 31s emits no POST | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "30s window"` exits 0 | ✅ |
| 26 | 01-05 | 2 | M-01 | E2E first-per-dimension lock — 2nd team click ignored | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "first per dimension"` exits 0 | ✅ |
| 27 | 01-05 | 2 | M-01 | E2E auto-advance suppression — programmatic episode change emits no POST | integration | `cd frontend/web && bunx playwright test combo-override.spec.ts -g "ignores auto-advance"` exits 0 | ✅ |
| 28 | 01-06 | 3 | M-02 | Grafana dashboard panel JSON parses (no syntax errors) | unit | `jq . docker/grafana/dashboards/preference-resolution.json > /dev/null` exits 0 | ✅ |
| 29 | 01-06 | 3 | M-02 | Dashboard contains "Auto-Pick Override Rate" panel/row | unit | `jq '.panels \| map(select(.title \| test("Override Rate"))) \| length' docker/grafana/dashboards/preference-resolution.json` returns >=1 | ✅ |
| 30 | 01-07 | 3 | M-02 | Player + gateway redeployed via `make redeploy-*` | manual-semi | `make health` all-green AND `curl -s http://localhost:8083/metrics \| grep -c combo_override_total` returns >=1 | ✅ |
| 31 | 01-07 | 3 | M-02 | PromQL probe — counters present after smoke override | manual-semi | `curl -s 'http://localhost:9090/api/v1/query?query=combo_override_total' \| jq '.data.result \| length > 0'` returns true | manual |
| 32 | 01-07 | 3 | M-02 | Grafana panel renders with "Auto-Pick Override Rate" title | manual | Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1` and confirm panel visible | manual |
| 33 | 01-07 | 3 | M-02 | `animeenigma-after-update` skill ran — changelog updated, commit + push completed | manual-semi | `jq '.entries[0].title' frontend/web/public/changelog.json \| grep -i "override\\\|baseline\\\|instrumentation"` non-empty AND `git log -1 --pretty=%s` matches phase-1 instrumentation message | ✅ |
| 34 | post-phase | — | M-02 | Baseline snapshot ≥ 24h captured and recorded in PROJECT.md (PHASE GATE before Phase 6) | manual | `grep -A 10 "Baseline override rate" .planning/PROJECT.md` non-empty | manual (Phase gate) |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

These must be created in Wave 0 before any implementation work begins (RED state — tests reference future code, so they fail now):

- [ ] `services/player/internal/handler/override_test.go` — backend handler tests (covers M-01 counter, log, validation, JWT, anon, both-missing rejection). RED until handler exists.
- [ ] `services/player/internal/transport/optional_auth_test.go` — middleware test (JWT decode + no-op when absent). RED until middleware exists.
- [ ] `services/player/internal/service/preference_resolve_combo_test.go` — assert `ComboResolveTotal` increments. RED until service edit lands.
- [ ] `services/player/internal/handler/preference_anon_test.go` — assert ResolvePreference accepts anon (no JWT). RED until handler edit lands.
- [ ] `frontend/web/e2e/combo-override.spec.ts` — Playwright spec stubs covering 7 E2E scenarios; `test.skip()` body until backend + frontend land. Becomes active in Wave 2.

Note: Wave 0 commits compile-failing tests. Wave 1 brings them green by introducing the production code. The Nyquist contract is satisfied because every `<verify>` references a Wave 0 file path.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Production PromQL probe non-zero after live traffic | M-02 | Requires real production traffic | After deploy, trigger 1+ override on prod, then `curl -s 'https://admin.animeenigma.ru/prometheus/api/v1/query?query=combo_override_total' \| jq '.data.result \| length'` returns ≥ 1 |
| Grafana panel renders correctly with all 5 segmentations | M-02 | Requires browser visual confirmation | Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1`, confirm "Auto-Pick Override Rate" panel renders with tier / language / anon-vs-auth / player / dimension segmentations |
| 24h baseline captured and written to PROJECT.md | M-02, success criterion 3 | Requires 24+ hours of real traffic | After 24h soak: extract `rate(combo_override_total[5m]) / rate(combo_resolve_total[5m])` per segment, write to `.planning/PROJECT.md` § "Baseline override rate" |
| Live deployment via `make redeploy-player` healthy | success criterion 4 | Requires production environment | `make redeploy-player && make redeploy-gateway && make health` returns all-green; both counters appear in `/metrics` exposition |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (5 files)
- [x] No watch-mode flags (`go test`, `playwright test`, no `--ui`)
- [x] Feedback latency < 30s per task
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved — Approach B (Playwright-only) chosen by planner.
