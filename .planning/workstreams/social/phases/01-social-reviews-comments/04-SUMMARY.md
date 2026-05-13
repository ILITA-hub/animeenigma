---
phase: 1
workstream: social
plan: 4
subsystem: services/player + services/gateway (comments wiring + proxy)
tags:
  - wiring
  - http-routes
  - gateway-proxy
  - route-ordering
  - wave-4
requirements:
  - SOCIAL-04
dependency_graph:
  requires:
    - "Plan 03: production CommentRepository / CommentService / CommentHandler with rate-limit + activity emission + soft-delete (commits 8f8f1a4, 2a3655a, 30b4c34)"
    - "Plan 01: domain.Comment AutoMigrate + comments table + indexes live in Postgres"
  provides:
    - "Live HTTP endpoints: GET /api/anime/{animeId}/comments (public), POST/PATCH/DELETE under AuthMiddleware"
    - "Gateway proxy routes for all 4 verbs, registered BEFORE the /anime/* catch-all to catalog (RESEARCH.md Pitfall 1 mitigation)"
    - "End-to-end CRUD path validated against live infra (201/200/204/400/401/429 all produced in expected places)"
  affects:
    - "services/player/cmd/player-api/main.go (commentRepo + commentService + commentHandler constructed; threaded into NewRouter after reviewHandler)"
    - "services/player/internal/transport/router.go (NewRouter signature gains *handler.CommentHandler; 4 routes mounted inside /anime/{animeId} group)"
    - "services/gateway/internal/transport/router.go (4 new proxy routes registered between the reviews block and the /anime/* catch-all)"
tech-stack:
  added: []
  patterns:
    - "chi route ordering — specific verb-bound routes BEFORE generic HandleFunc catch-alls (mirrors the existing reviews wiring exactly)"
    - "Public/protected partition via two parallel chi groups inside one r.Route — GET outside the AuthMiddleware sub-group, mutations inside"
key-files:
  created:
    - ".planning/workstreams/social/phases/01-social-reviews-comments/04-SUMMARY.md"
  modified:
    - "services/player/cmd/player-api/main.go"
    - "services/player/internal/transport/router.go"
    - "services/gateway/internal/transport/router.go"
decisions:
  - "Comment routes mount inside the existing `/anime/{animeId}` chi route group — no top-level `/comments` group. This keeps the URL shape consistent with reviews and means the gateway only needs to register 4 explicit routes, not a 5th group with its own prefix."
  - "Gateway routes are 4 separate verb-bound entries (Get/Post/Patch/Delete) rather than `HandleFunc(\"/anime/{animeId}/comments/*\", ...)`. Matches the existing reviews wiring style exactly (6 verb-bound reviews routes; not a catch-all). Slightly more verbose, but the gateway already reads as a route table — keeps the diff scoped to what's actually being added."
  - "POST `/comments` lives at `/anime/{animeId}/comments` (no commentId on the path); PATCH/DELETE live at `/anime/{animeId}/comments/{commentId}`. Plan 03's handler already chi.URLParam(\"commentId\")-extracts the id; gateway proxies both URI shapes verbatim."
  - "Per Plan 02's documented pre-existing API-key auth bug (gateway → auth resolveApiKey 401s on /anime/{animeId}/* routes even though /api/users/* works fine with the same ak_ key), the checkpoint smoke ran with a real JWT obtained via password login as `ui_audit_bot`. This is NOT a regression introduced by Plan 04 — the same 401 reproduces for the existing review POST that's been in production since Plan 02. Tracked as a follow-up; out of scope for SOCIAL-04."
metrics:
  duration_minutes: 6
  completed_date: "2026-05-13"
  tasks_completed: 4
  files_created: 1
  files_modified: 3
  commits: 3
---

# Phase 1 Plan 04 (Workstream `social`): Comments Wiring + Gateway Proxy Summary

**One-liner:** Three small edits — main.go wiring, player chi router
mount, gateway proxy ordering — make the dormant Plan-03 comment handlers
reachable over HTTP. Functional smoke against live infra produces
201/200/204/400/401/429 in the expected places; gateway route ordering
verified to send `/api/anime/{animeId}/comments*` to player (NOT
catalog), unblocking SOCIAL-04.

## What Was Built

### Player main.go wiring (Task 4.1)

Three new lines inserted immediately after the existing review-service
construction:

```go
commentRepo := repo.NewCommentRepository(db.DB)
commentService := service.NewCommentService(commentRepo, activityRepo, log)
commentHandler := handler.NewCommentHandler(commentService, log)
```

The `transport.NewRouter(...)` call site gains `commentHandler` as a new
argument immediately after `reviewHandler`. The `db.AutoMigrate(...)`
block is unchanged — `&domain.Comment{}` was already added in Plan 01.

### Player chi router (Task 4.2)

`NewRouter` signature gains `commentHandler *handler.CommentHandler`
immediately after `reviewHandler`. Inside the existing
`r.Route("/anime/{animeId}", ...)` block, four routes added:

```go
// Public (outside the AuthMiddleware-protected group)
r.Get("/comments", commentHandler.ListComments)

// Inside the existing protected group
r.Group(func(r chi.Router) {
    r.Use(AuthMiddleware(jwtConfig))
    // ... existing review routes ...
    r.Post("/comments", commentHandler.CreateComment)
    r.Patch("/comments/{commentId}", commentHandler.UpdateComment)
    r.Delete("/comments/{commentId}", commentHandler.DeleteComment)
})
```

GET `/comments` is unauthenticated per CONTEXT.md (anonymous readers can
fetch the feed). POST/PATCH/DELETE require a valid bearer token; the
service layer applies ownership/admin check + 10/hour/(user,anime) rate
limit on top.

### Gateway proxy (Task 4.3)

Four explicit routes inserted between the reviews proxy block and the
`/anime/*` catch-all that proxies to catalog:

```go
// Player service routes - comments (must be before /anime/* catch-all)
r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)

// Catalog service routes (public)
r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)  // <-- catch-all
```

Without this ordering, chi's match-in-registration-order semantics would
route every `/anime/{id}/comments*` request to catalog, which 404s
because no such handlers exist. The functional smoke (`GET /comments`
returning the player's `{"comments":[], "has_more":false}` shape, NOT a
catalog response) verifies the ordering is working.

### Comments route map (final)

| Verb   | URL                                                | Auth | Handler                       |
|--------|---------------------------------------------------|------|-------------------------------|
| GET    | `/api/anime/{animeId}/comments`                   | None | `commentHandler.ListComments` |
| POST   | `/api/anime/{animeId}/comments`                   | JWT  | `commentHandler.CreateComment`|
| PATCH  | `/api/anime/{animeId}/comments/{commentId}`       | JWT  | `commentHandler.UpdateComment`|
| DELETE | `/api/anime/{animeId}/comments/{commentId}`       | JWT  | `commentHandler.DeleteComment`|

## Verification

### Static checks (all PASS)

| Check | Result |
|---|---|
| `cd services/player && go build ./...` | exits 0 |
| `cd services/player && go test ./... -count=1` | all packages PASS |
| `cd services/gateway && go build ./...` | exits 0 |
| `cd services/gateway && go test ./... -count=1` | all packages PASS |
| `grep -c 'commentRepo := repo.NewCommentRepository' services/player/cmd/player-api/main.go` | `1` |
| `grep -c 'commentService := service.NewCommentService' services/player/cmd/player-api/main.go` | `1` |
| `grep -c 'commentHandler := handler.NewCommentHandler' services/player/cmd/player-api/main.go` | `1` |
| `grep -E 'transport\.NewRouter\(.*commentHandler' services/player/cmd/player-api/main.go` | matches (NewRouter gets commentHandler) |
| `grep -c 'commentHandler.ListComments' services/player/internal/transport/router.go` | `1` |
| `grep -c 'commentHandler.CreateComment' services/player/internal/transport/router.go` | `1` |
| `grep -c 'commentHandler.UpdateComment' services/player/internal/transport/router.go` | `1` |
| `grep -c 'commentHandler.DeleteComment' services/player/internal/transport/router.go` | `1` |
| GET `/comments` line precedes inner-group AuthMiddleware line | PASS (line 181 < line 185) |
| `grep -c '/anime/{animeId}/comments' services/gateway/internal/transport/router.go` | `4` |
| Delete-comments line precedes `/anime/*` catch-all line | PASS (4 new comments lines before catch-all at line 159) |
| Comments lines using `ProxyToPlayer` | `4` |
| Comments lines using `ProxyToCatalog` | `0` |

### Live smoke (autonomous checkpoint 4.4)

Both services redeployed via `make redeploy-player` + `make
redeploy-gateway`; both reported `healthy`. Smoke run against
`http://localhost:8000` with `ANIME_ID=8e913af8-580b-4ae6-b6ee-eb4e9ca71e1f`
(first row in `animes`).

| Step | What | Status | Notes |
|------|------|--------|-------|
| 3 | `GET /api/anime/$ANIME_ID/comments` | `HTTP/1.1 200 OK` | Body: `{"success":true,"data":{"comments":[],"has_more":false}}`. Shape matches the player's `CommentsListResponse` — NOT a catalog response — proving the gateway routes correctly. |
| 4 | Anonymous `POST /comments` | `HTTP 401` | Auth middleware engaged before reaching the handler. |
| 5 | Authenticated `POST /comments` (real JWT) | `HTTP 201 Created` | Returned: `{"id":"b603e509-...","user_id":"5ea77649-...","anime_id":"8e913af8-...","username":"ui_audit_bot","body":"smoke test comment from plan 04 executor","created_at":"2026-05-13T03:25:11.378028Z","updated_at":"2026-05-13T03:25:11.378028Z"}`. |
| 6 | `POST /comments` with empty body `"   "` | `HTTP 400` | `validateBody` trim+empty check fires before service rate-limit. |
| 7 | Rate-limit: 10 successful posts (steps 5+9 more), 11th hits limit | `9× HTTP 201`, then `HTTP 429` | Sliding window of 10 per (userID, animeID) per hour enforced (RESEARCH.md Pattern 5). |
| 8 | Activity events emitted | `comment_events = 10` | `SELECT count(*) FROM activity_events WHERE type='comment' AND user_id=... AND anime_id=...` returned `10` — one row per comment, no per-day dedup (matches Plan 03 SUMMARY decision). |
| 9 | Soft delete | `HTTP 204` + `deleted_at` set | `deleted_at = 2026-05-13 03:25:32.873724+00`. GET `/comments` after the DELETE returned 9 visible rows (was 10); deleted id NOT in the list. |
| 10 | Cleanup | 9 rows soft-deleted, 10 activity events purged | `GET /comments` after cleanup returns 0 visible comments. |

### Gateway routing — Pitfall 1 verification

The whole point of putting the comments routes BEFORE the `/anime/*`
catch-all is to make sure they reach player, not catalog. Tested by
comparing response shapes:

```
GET /api/anime/{id}        → top-level keys: ["data", "success"]
GET /api/anime/{id}/comments → data keys:     ["comments", "has_more"]
```

The `comments` + `has_more` keys are only emitted by the player's
`domain.CommentsListResponse` type; catalog has no such shape. Routing
verified correct.

### Authenticated test method note

Per Plan 02's SUMMARY documenting a pre-existing API-key resolution bug
on the `/anime/{animeId}/*` route family (gateway's
`JWTValidationMiddleware` 401s on `ak_` keys for these routes even
though `/api/users/*` works fine with the same key), the checkpoint
smoke ran with a real JWT obtained via password login as `ui_audit_bot`
(password set in CLAUDE.md, valid against the current production
database). The same 401 reproduces for the existing review POST with
the same API key — this is NOT a regression introduced by Plan 04.
Tracked as a follow-up; out of scope for SOCIAL-04.

## Commits

| Task | Commit  | Message |
|------|---------|---------|
| 4.1  | `df9829b` | `feat(1-4): wire CommentRepository + CommentService + CommentHandler in player main.go` |
| 4.2  | `5f42857` | `feat(1-4): mount comment routes in player chi router (1 public + 3 protected)` |
| 4.3  | `a232ebd` | `feat(1-4): proxy /anime/{animeId}/comments* to player BEFORE /anime/* catch-all` |

(4.4 is the human-verify checkpoint — no separate commit.)

## Deviations from Plan

### Auto-fixed Issues

None. Plan 04 was a straightforward three-edit plan and every step
executed exactly as written.

### Notes (not deviations)

- The `cmd/player-api/` directory is gitignored at the repo level
  (the .gitignore pattern matches the compiled binary name); commits
  to `main.go` under that path require `git add -f`. Same pattern
  Plan 01/02 noted.
- The plan said the new comments-routes block should sit "Immediately
  AFTER `r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)`
  and BEFORE `r.HandleFunc("/anime", ...)`". The edit landed at exactly
  that location, with the canonical one-line comment header
  `// Player service routes - comments (must be before /anime/* catch-all)`
  matching the reviews-block style.
- The autonomous checkpoint policy converted the originally-blocking
  `checkpoint:human-verify` step into automated curl-based verification.
  All 9 verification steps in the plan's `<how-to-verify>` ran;
  results captured in the table above. Cleanup also executed (test
  comments + activity events purged).
- Step 7 in the plan's how-to-verify said "post 10 valid comments
  then assert the 11th returns 429." My run posted 1 in step 5 + 9
  more in the rate-limit loop = 10 total, then the 11th returned 429.
  Identical math; just split across the steps for clarity.

## Handoff

Plan 05 (frontend store + composables for comments) and Plan 06
(`CommentsPanel.vue` component) can now assume:

- `GET /api/anime/{id}/comments` returns 200 with paginated body
  `{"comments":[...],"has_more":bool,"next_cursor":"..."}` (response
  envelope wraps it under `.data`).
- `POST /api/anime/{id}/comments` requires `Authorization: Bearer
  <jwt>`, accepts `{"body":"..."}`, returns 201 with the full Comment
  JSON. Empty body → 400. >10/hr/anime → 429.
- `PATCH /api/anime/{id}/comments/{cid}` and `DELETE
  /api/anime/{id}/comments/{cid}` enforce owner-or-admin at the
  service layer; non-owner non-admin → 403.
- All four endpoints route through the gateway at port 8000 — frontend
  uses the existing `/api/...` baseURL with no new env vars.

## Known Stubs

None. The full comments CRUD stack is now live HTTP. No hardcoded
empty data, no placeholders, no TODO/FIXME in the wiring code.

## Threat Flags

None new. The plan's threat register has three items, all addressed:

- **T-1-V13 (gateway routing precedence)**: mitigated. Functional
  smoke step 3 + the response-shape comparison both confirm
  `/api/anime/{id}/comments` reaches the player. The four new
  gateway routes are registered BEFORE the `/anime/*` catch-all
  (line numbers asserted by awk in the verification table).
- **T-1-V4 (access control)**: mitigated. Anonymous POST returns 401
  (step 4); authenticated POST returns 201 (step 5). The
  AuthMiddleware-protected sub-group in the player router holds the
  three mutation routes; the GET route lives outside it.
- **Route precedence regression**: mitigated by the awk-based line-order
  acceptance check + the response-shape sanity check.

## Self-Check: PASSED

**Files verified to exist (modified):**

- `services/player/cmd/player-api/main.go` — FOUND; `grep -c 'NewCommentHandler' main.go` = 1
- `services/player/internal/transport/router.go` — FOUND; 4 `commentHandler.*` references
- `services/gateway/internal/transport/router.go` — FOUND; 4 `/anime/{animeId}/comments` lines, all `ProxyToPlayer`

**Commits verified in `git log --all`:**

- `df9829b` — FOUND (Task 4.1 — main.go wiring)
- `5f42857` — FOUND (Task 4.2 — player router routes)
- `a232ebd` — FOUND (Task 4.3 — gateway proxy)

**Live infra checks:**

- `make redeploy-player` succeeded; player container reported `healthy` — VERIFIED
- `make redeploy-gateway` succeeded; gateway container reported `healthy` — VERIFIED
- GET 200 with player response shape — VERIFIED
- Anonymous POST 401 — VERIFIED
- Authenticated POST 201 with full Comment JSON — VERIFIED
- Empty body 400 — VERIFIED
- 10 successful posts + 11th 429 — VERIFIED
- 10 activity_events rows of `type='comment'` — VERIFIED
- DELETE 204 + `deleted_at` non-NULL + omitted from list — VERIFIED
- Test data cleaned up (9 soft-deletes + 10 activity event deletes) — VERIFIED
