---
phase: 3
slug: dynamic-cards-migration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-21
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for the 9-card spotlight rollout + migration.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Frameworks** | go test (stdlib) for backend; Vitest 4.x + Playwright 1.58 + @axe-core/playwright 4.11 for frontend |
| **Config file(s)** | `services/*/go.mod`; `frontend/web/{vitest.config.ts, playwright.config.ts}` |
| **Quick run (backend)** | `cd services/catalog && go test ./internal/service/spotlight/... ./internal/handler/spotlight* -count=1 -race -short && cd ../player && go test ./internal/handler/... ./internal/service/... -count=1 -short` |
| **Quick run (frontend)** | `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/composables/useSpotlight.spec.ts src/locales/__tests__/` |
| **Full suite** | Catalog `go test ./...`; Player `go test ./...`; Gateway `go test ./...`; Frontend tsc + eslint + vitest + `bunx playwright test spotlight` |
| **Estimated runtime** | ~60s backend quick, ~30s frontend quick, ~3min full |

---

## Sampling Rate

- **After every task commit:** Quick command for whichever service was touched
- **After every plan wave:** Full suite for the affected service(s)
- **Before `/gsd-verify-work`:** Full suite green + `make redeploy-catalog && make redeploy-player && make redeploy-web` clean + `make health` all green + `curl -s http://localhost:8000/api/home/spotlight` shows up to 4 cards anon and up to 6 cards as `ui_audit_bot` (some login-only cards depend on user data)
- **Max feedback latency:** ≤60s

---

## Per-Task Verification Map (high-level)

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | Status |
|---------|------|------|-------------|-----------|-------------------|--------|
| 3-PE-01 | 01 | 1 | HSB-BE-26 | unit | `cd services/player && go test ./internal/handler -run TestListByStatuses_Internal -count=1` | ⬜ |
| 3-PE-02 | 01 | 1 | HSB-NF-02 | unit | `cd services/player && go test ./internal/domain -run TestWatchProgress_IndexTag -count=1` | ⬜ |
| 3-OA-01 | 02 | 1 | HSB-BE-01 | unit | `cd services/catalog && go test ./internal/transport -run TestOptionalAuth -count=1` | ⬜ |
| 3-PC-01 | 02 | 1 | HSB-BE-23 | unit | `cd services/catalog && go test ./internal/service/spotlight/client -run TestPlayerClient -count=1` | ⬜ |
| 3-AD-01 | 02 | 1 | HSB-BE-30 | unit | `cd services/catalog && go test ./internal/service/spotlight -run TestAdaptiveSlice -count=1` | ⬜ |
| 3-RE-01 | 03 | 2 | HSB-BE-20 | unit | `go test ./internal/service/spotlight/cards -run TestPersonalPick -count=1` | ⬜ |
| 3-RE-02 | 03 | 2 | HSB-BE-21 | unit | `go test ./internal/service/spotlight/cards -run TestTelegramNews -count=1` | ⬜ |
| 3-RE-03 | 03 | 2 | HSB-BE-22 | unit | `go test ./internal/service/spotlight/cards -run TestNowWatching -count=1` | ⬜ |
| 3-RE-04 | 03 | 2 | HSB-BE-24 | unit | `go test ./internal/service/spotlight/cards -run TestNotTimeYet -count=1` | ⬜ |
| 3-RE-05 | 03 | 2 | HSB-BE-25 | unit | `go test ./internal/service/spotlight/cards -run TestContinueWatchingNew -count=1` | ⬜ |
| 3-AG-01 | 04 | 3 | HSB-BE-02 | unit | `go test ./internal/service/spotlight -run TestAggregator_NineCards -count=1` | ⬜ |
| 3-GW-01 | 04 | 3 | HSB-BE-06 | unit | `cd services/gateway && go test ./internal/transport -run TestRouter_InternalListNotProxied -count=1` | ⬜ |
| 3-FE-24 | 05 | 3 | HSB-FE-24 | unit | `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/PersonalPickCard.spec.ts` | ⬜ |
| 3-FE-25 | 05 | 3 | HSB-FE-25 | unit | `bunx vitest run src/components/home/spotlight/cards/NowWatchingCard.spec.ts` | ⬜ |
| 3-FE-26 | 05 | 3 | HSB-FE-26 | unit | `bunx vitest run src/components/home/spotlight/cards/TelegramNewsCard.spec.ts` | ⬜ |
| 3-FE-27 | 05 | 3 | HSB-FE-27 | unit | `bunx vitest run src/components/home/spotlight/cards/NotTimeYetCard.spec.ts` | ⬜ |
| 3-FE-28 | 05 | 3 | HSB-FE-28 | unit | `bunx vitest run src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts` | ⬜ |
| 3-I18-01 | 05 | 3 | HSB-FE-40 | unit | `bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts` | ⬜ |
| 3-DI-01 | 06 | 4 | HSB-BE-02 | unit | catalog smoke: `curl /api/home/spotlight \| jq '.cards[].type'` includes all 9 types when authenticated | ⬜ |
| 3-MI-01 | 07 | 4 | HSB-MIG-01 | unit | `grep -c "trendingRecs" frontend/web/src/views/Home.vue` returns 0 | ⬜ |
| 3-MI-02 | 07 | 4 | HSB-NF-05 | manual | `grep -q "Adding a Spotlight Card Type" CLAUDE.md` | ⬜ |
| 3-E2E-01 | 08 | 5 | HSB-FE-* | e2e | `bunx playwright test e2e/spotlight-full.spec.ts` (extended 9-card scenario) | ⬜ |
| 3-E2E-02 | 08 | 5 | HSB-FE-07 | e2e | axe-core 0 violations on 9-card mode | ⬜ |
| 3-E2E-03 | 08 | 5 | ROADMAP §9 | manual+e2e | logged-in as `ui_audit_bot` → spotlight shows ≥1 of personal_pick/continue_watching_new/not_time_yet | ⬜ |

---

## Wave 0 Requirements

- [ ] `services/catalog/internal/service/spotlight/adaptive_slice.go` + `_test.go` (generic 1-2-3 helper)
- [ ] `services/catalog/internal/service/spotlight/client/player_client.go` + `_test.go`
- [ ] `services/catalog/internal/service/spotlight/cards/{personal_pick,telegram_news,now_watching,not_time_yet,continue_watching_new}.go` + tests
- [ ] `services/catalog/internal/transport/optional_auth.go` + `optional_auth_test.go` (or inline in router)
- [ ] `services/player/internal/handler/list_internal.go` + `_test.go` (new internal endpoint)
- [ ] `services/player/internal/transport/router.go` (extend with /internal/users/{id}/list route)
- [ ] `services/gateway/internal/transport/router_internal_list_test.go` (defense-in-depth: NOT proxied)
- [ ] `frontend/web/src/components/home/spotlight/cards/{PersonalPick,NowWatching,TelegramNews,NotTimeYet,ContinueWatchingNew}Card.vue` + co-located specs
- [ ] `frontend/web/src/locales/{en,ru,ja}.json` extended with 5 new sub-namespaces (parity test from Phase 2 catches drift)
- [ ] `frontend/web/e2e/spotlight-full.spec.ts` (extended 9-card scenarios)
- [ ] `frontend/web/src/views/Home.vue` diff: trendingRecs block removed + its setup-script state
- [ ] `CLAUDE.md` extended with "Adding a Spotlight Card Type" section under Common Tasks

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Login + see ≥1 of {personal_pick, continue_watching_new, not_time_yet} | ROADMAP §9 | Requires authenticated session against running infra | curl with `Authorization: Bearer $UI_AUDIT_API_KEY`; verify response cards include at least one of the three login-only types |
| `idx_watch_progress_updated_at` exists after deploy | HSB-NF-02 | DB introspection | `docker compose exec postgres psql -U postgres -d animeenigma -c "\d watch_progress"` shows the new index |
| `now_watching` card appears within ≤30s of a real user watching | HSB-BE-22 | Requires a second authenticated session generating real watch_progress events | Sign in as ui_audit_bot, start watching anime; in another session/browser, curl /api/home/spotlight and confirm now_watching card includes the ui_audit_bot session |
| Trending row truly gone in production | HSB-MIG-01 | Final visual check | Open https://animeenigma.ru/ after `make redeploy-web`; confirm 3-column grid is now directly below the spotlight block (no trendingRecs row) |

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] No 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency ≤60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
