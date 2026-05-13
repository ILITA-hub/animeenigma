---
phase: 1
slug: social-reviews-comments
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-13
workstream: social
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Derived from `01-RESEARCH.md` § Validation Architecture.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework (Go)** | `testing` stdlib + `github.com/stretchr/testify v1.8.4` |
| **Framework (Frontend e2e)** | `@playwright/test 1.58.0` |
| **Config file (Go)** | `services/player/.golangci.yml` (lint) — no separate test config; tests run via `go test ./...` |
| **Config file (Playwright)** | `frontend/web/playwright.config.ts` |
| **Quick run command (Go, package)** | `cd services/player && go test ./internal/{handler,service,repo}/... -short` |
| **Full suite command (Go, player)** | `cd services/player && go test ./... -race -cover` |
| **Full suite command (frontend e2e)** | `cd frontend/web && bunx playwright test` |
| **Estimated quick runtime** | ~30 seconds |
| **Estimated full runtime** | ~3 minutes (Go race+cover) + ~2 minutes (Playwright comments suite) |

---

## Sampling Rate

- **After every task commit:** `cd services/player && go test ./internal/{handler,service,repo}/... -short` (≤ 30s)
- **After every plan wave:** `cd services/player && go test ./... -race -cover` + `cd frontend/web && bunx vue-tsc --noEmit && bunx playwright test e2e/comments.spec.ts`
- **Before `/gsd-verify-work`:** Full suite must be green + manual smoke checks (DB inspection, golden-file diff)
- **Max feedback latency:** 30 seconds (quick) / 5 minutes (full)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 01-Wave0-01 | 00 | 0 | SOCIAL-04..06 | — | N/A (scaffolding) | scaffold | `find services/player/internal -name 'comment*.go' \| wc -l` ≥ 6 | ❌ W0 | ⬜ pending |
| 01-Wave0-02 | 00 | 0 | SOCIAL-06 | — | N/A (scaffolding) | scaffold | `test -f frontend/web/e2e/comments.spec.ts` | ❌ W0 | ⬜ pending |
| 01-Schema-01 | 01 | 1 | SOCIAL-01 | T-1-V5 | New columns nullable-safe; `DEFAULT ''` prevents NULL inject on legacy rows | smoke (boot) | `make redeploy-player && docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d anime_list" \| grep -E "review_text\|username"` | ✅ (post-redeploy) | ⬜ pending |
| 01-Migrate-01 | 01 | 1 | SOCIAL-02 | T-1-Migration | Idempotent guard prevents double-copy on retry | unit | `cd services/player && go test ./cmd/player-api/... -run TestSocialMigration_Idempotent -v` | ❌ W0 | ⬜ pending |
| 01-Migrate-02 | 01 | 1 | SOCIAL-02 / SOCIAL-NF-02 | T-1-Migration | Migration logs `complete` once; nothing on second run | smoke (boot logs) | `make logs-player \| grep "social migration"` shows "complete" on first run, NOTHING on second run | ✅ (post-redeploy) | ⬜ pending |
| 01-Reviews-01 | 02 | 2 | SOCIAL-03 / SOCIAL-NF-01 | T-1-V13 | All six review endpoints preserve byte-identical JSON shape; handler projection prevents leaking `AnimeListEntry` private fields | unit | `cd services/player && go test ./internal/handler -run TestReviewHandler_ -v` | ✅ (existing pattern, augment) | ⬜ pending |
| 01-Reviews-02 | 02 | 2 | SOCIAL-04 | T-1-V11 | Imported `score=8` row appears in reviews list (no extra code) | unit | `cd services/player && go test ./internal/repo -run TestReviewRepo_ListByAnime_IncludesImported -v` | ❌ W0 | ⬜ pending |
| 01-Comment-01 | 03 | 2 | SOCIAL-04a | T-1-V5 | Body 1-2000 UTF-8 runes; trimmed; non-empty | unit | `cd services/player && go test ./internal/handler -run TestCommentHandler_CreateComment_HappyPath -v` | ❌ W0 | ⬜ pending |
| 01-Comment-02 | 03 | 2 | SOCIAL-04b | T-1-V5 | Empty / whitespace body → 400 | unit | `cd services/player && go test ./internal/handler -run TestCommentHandler_CreateComment_EmptyBody -v` | ❌ W0 | ⬜ pending |
| 01-Comment-03 | 03 | 2 | SOCIAL-04c | T-1-V4 | PATCH by non-owner → 403 (admin override allowed) | unit | `cd services/player && go test ./internal/handler -run TestCommentHandler_UpdateComment_NotOwner -v` | ❌ W0 | ⬜ pending |
| 01-Comment-04 | 03 | 2 | SOCIAL-04d | T-1-V4 | Soft-delete: `deleted_at` set; row remains; excluded from GET | unit | `cd services/player && go test ./internal/repo -run TestCommentRepo_SoftDelete -v` | ❌ W0 | ⬜ pending |
| 01-Comment-05 | 03 | 2 | SOCIAL-04e | T-1-V5 | Cursor pagination: page 2 returns next 50; cursor base64 round-trips | unit | `cd services/player && go test ./internal/repo -run TestCommentRepo_ListByAnime_Cursor -v` | ❌ W0 | ⬜ pending |
| 01-Comment-06 | 03 | 2 | SOCIAL-04f | T-1-V11 | 11th POST in an hour → 429 | unit | `cd services/player && go test ./internal/service -run TestCommentService_RateLimit -v` | ❌ W0 | ⬜ pending |
| 01-Activity-01 | 04 | 2 | SOCIAL-05 | — | Each comment-create writes one `activity_events` row (type='comment'); content preview ≤ 300 runes + ellipsis | unit | `cd services/player && go test ./internal/service -run TestCommentService_EmitsActivity -v` | ❌ W0 | ⬜ pending |
| 01-Gateway-01 | 05 | 3 | SOCIAL-04 | — | Gateway routes `/anime/{id}/comments` to player BEFORE the `/anime/*` catch-all to catalog | smoke | `curl -s http://localhost:8000/api/anime/test-id/comments \| jq '.error // empty'` returns nothing or known error from player, NOT a catalog error | ✅ (post-redeploy) | ⬜ pending |
| 01-FE-API-01 | 06 | 3 | SOCIAL-04 | T-1-V13 | `commentApi` JSON contract matches backend | type-check | `cd frontend/web && bunx vue-tsc --noEmit` | ✅ (existing pattern) | ⬜ pending |
| 01-FE-Locales | 07 | 3 | SOCIAL-06 | — | All 24 `anime.ugc.*` keys translated in EN/JA/RU; no missing-key warnings | type-check | `cd frontend/web && bunx vue-tsc --noEmit` + boot dev server and grep console for `[intlify]` warnings | ⚠️ Manual (dev console) | ⬜ pending |
| 01-FE-Tabs-01 | 08 | 4 | SOCIAL-06a | — | `?ugc=comments` deep-link mounts Comments tab on first paint | Playwright e2e | `cd frontend/web && bunx playwright test e2e/comments.spec.ts -g "deep-link"` | ❌ W0 | ⬜ pending |
| 01-FE-Tabs-02 | 08 | 4 | SOCIAL-06b | — | Tab click updates URL via `router.replace` | Playwright e2e | `cd frontend/web && bunx playwright test e2e/comments.spec.ts -g "URL persists"` | ❌ W0 | ⬜ pending |
| 01-FE-Tabs-03 | 08 | 4 | SOCIAL-06c | T-1-V4 | Anonymous user on Comments tab sees login prompt, no textarea | Playwright e2e | `cd frontend/web && bunx playwright test e2e/comments.spec.ts -g "anon login prompt"` | ❌ W0 | ⬜ pending |
| 01-FE-Tabs-04 | 08 | 4 | SOCIAL-06d | T-1-V4 | Logged-in user can post / edit / delete own comment | Playwright e2e | `cd frontend/web && bunx playwright test e2e/comments.spec.ts -g "logged-in CRUD"` | ❌ W0 | ⬜ pending |
| 01-FE-Feed-01 | 09 | 4 | SOCIAL-05 | — | ActivityFeed renders `comment` events with new locale key (no `[intlify]` warning) | type-check + manual smoke | `bunx vue-tsc --noEmit`; dev server boots and renders feed with a real comment event | ⚠️ Manual (dev console) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky / manual*

---

## Wave 0 Requirements

Wave 0 scaffolds the test files needed for SOCIAL-04..06 verification. No new test framework is introduced — `testify` + SQLite-in-memory is the project standard (per `services/player/internal/handler/sync_test.go:22 setupSyncTestDB`).

- [ ] `services/player/internal/domain/comment.go` — new Comment domain struct + `TableName()`
- [ ] `services/player/internal/repo/comment.go` — new CommentRepository (Create, GetByID, ListByAnime, Update, SoftDelete)
- [ ] `services/player/internal/repo/comment_test.go` — stubs for SOCIAL-04d (soft delete), SOCIAL-04e (cursor pagination)
- [ ] `services/player/internal/service/comment.go` — new CommentService (Create, Update, Delete, List with rate-limit bucket)
- [ ] `services/player/internal/service/comment_test.go` — stubs for SOCIAL-04f (rate limit), SOCIAL-05 (activity emit)
- [ ] `services/player/internal/handler/comment.go` — new CommentHandler (4 endpoints)
- [ ] `services/player/internal/handler/comment_test.go` — stubs for SOCIAL-04a (happy path), SOCIAL-04b (empty body), SOCIAL-04c (non-owner PATCH)
- [ ] `services/player/cmd/player-api/main_test.go` — migration idempotency test (`TestSocialMigration_Idempotent`)
- [ ] `frontend/web/e2e/comments.spec.ts` — e2e stubs for SOCIAL-06a (deep-link), 06b (URL persists), 06c (anon login prompt), 06d (logged-in CRUD)

Framework install: none — `testify` is already in `services/player/go.mod`; Playwright is already wired in `frontend/web/`.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Schema columns appear post-migration | SOCIAL-01 | DB shape inspection after Docker redeploy is not run by `go test`; requires `psql \d` | `make redeploy-player && docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d anime_list"`; verify `review_text` and `username` columns appear; `\d reviews` reports "Did not find any relation" |
| Migration idempotency (boot log) | SOCIAL-NF-02 | Boot-log inspection is process-side, not test-side | `make logs-player \| grep "social migration"`; first redeploy prints "complete"; second redeploy prints nothing |
| Pre/post golden-file diff of 6 reviews endpoints | SOCIAL-NF-01 | Requires capturing JSON pre-deploy from the live API; cannot run inside the unit test boundary | (1) Before merge: `bash scripts/capture-reviews-fixtures.sh > tmp/reviews-pre.json`. (2) After redeploy: same script `> tmp/reviews-post.json`. (3) `diff tmp/reviews-pre.json tmp/reviews-post.json` → empty (shape-identical). The capture script is created in Wave 0. |
| `[intlify]` missing-key warnings in dev console | SOCIAL-06 / SOCIAL-08 | vue-i18n missing-key warnings show only at runtime, not at `vue-tsc` time | Boot `bun run dev`, open `/anime/<any-id>?ugc=comments` in all three locales, watch DevTools console for `[intlify] Not found 'anime.ugc.*'` warnings — must be empty |
| ActivityFeed renders `comment` events | SOCIAL-05 | Activity feed integration is best verified end-to-end visually | Post a comment as test user; open Profile / Activity Feed; verify the comment event appears with the new locale string (no fallback to `[review_event_template]`) |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify command or Wave 0 dependencies (per table above)
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify (✅ — each non-Wave-0 task has a `go test -run` or `playwright test -g` command)
- [ ] Wave 0 covers all `❌` references in the per-task table
- [ ] No watch-mode flags (✅ — no `--watch` anywhere; `-short` is one-shot)
- [ ] Feedback latency < 30s for the quick command
- [ ] `nyquist_compliant: true` set in frontmatter once Wave 0 completes

**Approval:** pending — flip `nyquist_compliant: true` and add `approved: 2026-05-13` after `/gsd-execute-phase 1 --ws social` lands the Wave-0 scaffolding.
