---
phase: 1
workstream: social
plan: 4
type: execute
wave: 4
depends_on: [2, 3]
files_modified:
  - services/player/cmd/player-api/main.go
  - services/player/internal/transport/router.go
  - services/gateway/internal/transport/router.go
autonomous: false
requirements:
  - SOCIAL-04

must_haves:
  truths:
    - "main.go constructs commentRepo, commentService, commentHandler and passes commentHandler into NewRouter."
    - "Player router exposes GET /api/anime/{animeId}/comments publicly and POST/PATCH/DELETE under the AuthMiddleware-protected sub-group."
    - "Gateway router proxies /anime/{animeId}/comments* (4 verbs) to player BEFORE the `/anime/*` catch-all that routes to catalog."
    - "curl GET http://localhost:8000/api/anime/<any-id>/comments returns HTTP 200 (possibly empty list) — NOT a catalog 404."
    - "curl POST http://localhost:8000/api/anime/<any-id>/comments without Authorization returns 401 (auth middleware engaged)."
    - "curl POST with valid Bearer token and body {body:hi} returns 201 and a Comment JSON object."
  artifacts:
    - path: "services/player/cmd/player-api/main.go"
      provides: "Comment wiring — repo + service + handler constructed; passed to NewRouter; AutoMigrate still includes &domain.Comment{} (already added in plan 01)"
      contains: "NewCommentHandler"
    - path: "services/player/internal/transport/router.go"
      provides: "Comment routes mounted under /anime/{animeId}; GET public, POST/PATCH/DELETE protected; NewRouter signature accepts *handler.CommentHandler"
      contains: "commentHandler.ListComments"
    - path: "services/gateway/internal/transport/router.go"
      provides: "Four explicit comments proxy routes BEFORE the /anime/* catch-all"
      contains: "/anime/{animeId}/comments"
  key_links:
    - from: "services/player/cmd/player-api/main.go"
      to: "services/player/internal/transport/router.go (NewRouter)"
      via: "transport.NewRouter(..., commentHandler, ...)"
      pattern: "transport.NewRouter\\("
    - from: "services/gateway/internal/transport/router.go"
      to: "player service via ProxyToPlayer"
      via: "r.Get/Post/Patch/Delete on /anime/{animeId}/comments[/{commentId}]"
      pattern: "/anime/\\{animeId\\}/comments"
---

<objective>
Wire the dormant comment handlers from plan 03 into the live HTTP stack. Three small but high-leverage edits: (1) main.go constructs the comment pipeline and passes it to NewRouter; (2) the player chi router mounts four routes (1 public + 3 protected) inside the existing `/anime/{animeId}` group; (3) the gateway router proxies the four routes to player BEFORE the `/anime/*` catch-all that goes to catalog. Without the gateway routes (RESEARCH.md Pitfall 3), every comment request returns a catalog 404.

Purpose: SOCIAL-04 — the CRUD endpoints exist in code but until this plan they aren't reachable from the network. After this plan, the frontend can hit `/api/anime/:id/comments` and receive real responses.

Output: comments endpoints are live HTTP endpoints in the local dev environment; functional smoke (curl) confirms 200/201/401/403/429.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-02-SUMMARY.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-03-SUMMARY.md
@services/player/cmd/player-api/main.go
@services/player/internal/transport/router.go
@services/gateway/internal/transport/router.go
@services/player/internal/handler/comment.go

<interfaces>
Public surface from plan 03 that this plan wires up.

From services/player/internal/repo/comment.go:
- NewCommentRepository(db *gorm.DB) *CommentRepository

From services/player/internal/service/comment.go:
- NewCommentService(commentRepo *repo.CommentRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *CommentService

From services/player/internal/handler/comment.go:
- NewCommentHandler(commentService *service.CommentService, log *logger.Logger) *CommentHandler
- ListComments / CreateComment / UpdateComment / DeleteComment (GET public; POST/PATCH/DELETE auth-required)

From services/player/internal/transport/router.go current signature:
- NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, overrideHandler, recsHandler, adminRecsHandler, recEventsHandler, cfg.JWT, log, metricsCollector) http.Handler
- (commentHandler will be added as a parameter immediately after reviewHandler)
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 4.1: Wire commentRepo + commentService + commentHandler in main.go; thread into NewRouter</name>
  <files>services/player/cmd/player-api/main.go</files>
  <read_first>
    - services/player/cmd/player-api/main.go (lines 215-300 — the section where reviewRepo / reviewService / reviewHandler are constructed and passed to NewRouter)
    - services/player/internal/transport/router.go (NewRouter signature)
  </read_first>
  <action>
    Edit services/player/cmd/player-api/main.go. After the activityRepo construction line, add three new lines: commentRepo := repo.NewCommentRepository(db.DB), commentService := service.NewCommentService(commentRepo, activityRepo, log), commentHandler := handler.NewCommentHandler(commentService, log). Place these immediately after the existing review pipeline construction so wiring reads top-to-bottom: review then comment. Modify the transport.NewRouter(...) call site to pass commentHandler as a new argument inserted immediately after reviewHandler in the argument list. Do NOT touch db.AutoMigrate — plan 01 already added &domain.Comment{}. Do NOT remove the runSocialMigration call.
  </action>
  <verify>
    <automated>cd services/player && go build ./... && grep -c 'NewCommentHandler' services/player/cmd/player-api/main.go</automated>
  </verify>
  <acceptance_criteria>
    - `cd services/player && go build ./...` exits 0 (this task must be performed in lockstep with 4.2; if compiled in isolation it will fail because router.go's NewRouter signature doesn't yet accept commentHandler — that's expected).
    - `grep -c 'commentRepo := repo.NewCommentRepository' services/player/cmd/player-api/main.go` outputs `1`.
    - `grep -c 'commentService := service.NewCommentService' services/player/cmd/player-api/main.go` outputs `1`.
    - `grep -c 'commentHandler := handler.NewCommentHandler' services/player/cmd/player-api/main.go` outputs `1`.
    - The transport.NewRouter call line contains commentHandler. Verify: `grep -E 'transport\.NewRouter\(.*commentHandler' services/player/cmd/player-api/main.go` exits 0.
  </acceptance_criteria>
  <done>main.go constructs the full comment pipeline and threads the handler into NewRouter.</done>
</task>

<task type="auto">
  <name>Task 4.2: Add comment routes to player chi router (1 public + 3 protected)</name>
  <files>services/player/internal/transport/router.go</files>
  <read_first>
    - services/player/internal/transport/router.go (full file — focus on lines 160-180, the existing /anime/{animeId} group + protected sub-group)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pattern 3, lines 414-448)
    - services/player/internal/handler/comment.go (method signatures)
  </read_first>
  <action>
    Edit services/player/internal/transport/router.go.

    Step 1: Update NewRouter(...) signature to accept commentHandler *handler.CommentHandler as a new parameter. Insert it immediately after reviewHandler in the parameter list to match the main.go call site (task 4.1).

    Step 2: Inside the existing r.Route("/anime/{animeId}", func(r chi.Router) { ... }) block (currently lines 166-178), add four routes:
    - Add r.Get("/comments", commentHandler.ListComments) as a PUBLIC route, immediately after the existing r.Get("/reviews", ...) line and BEFORE the r.Group(...AuthMiddleware...) block. Public comment listing must NOT require auth (per CONTEXT.md + RESEARCH.md Pitfall 4).
    - Inside the protected r.Group(func(r chi.Router) { r.Use(AuthMiddleware(jwtConfig)); ... }) block, add three lines after the existing review-protected routes:
      - r.Post("/comments", commentHandler.CreateComment)
      - r.Patch("/comments/{commentId}", commentHandler.UpdateComment)
      - r.Delete("/comments/{commentId}", commentHandler.DeleteComment)

    Step 3: Do NOT introduce a separate top-level /comments route group — keep everything nested under /anime/{animeId}. Do NOT modify any existing review or activity routes.
  </action>
  <verify>
    <automated>cd services/player && go build ./... && grep -c 'commentHandler.ListComments' services/player/internal/transport/router.go && grep -c 'commentHandler.CreateComment' services/player/internal/transport/router.go && grep -c 'commentHandler.UpdateComment' services/player/internal/transport/router.go && grep -c 'commentHandler.DeleteComment' services/player/internal/transport/router.go</automated>
  </verify>
  <acceptance_criteria>
    - `cd services/player && go build ./...` exits 0.
    - `grep -c 'commentHandler.ListComments' services/player/internal/transport/router.go` outputs `1`.
    - `grep -c 'commentHandler.CreateComment' services/player/internal/transport/router.go` outputs `1`.
    - `grep -c 'commentHandler.UpdateComment' services/player/internal/transport/router.go` outputs `1`.
    - `grep -c 'commentHandler.DeleteComment' services/player/internal/transport/router.go` outputs `1`.
    - The GET /comments route appears BEFORE the AuthMiddleware-protected group. Verify by line number: `awk '/r.Get\("\/comments"/{a=NR} /AuthMiddleware\(jwtConfig\)/{b=NR} END{exit (a < b)?0:1}' services/player/internal/transport/router.go` exits 0 (GET line precedes the AuthMiddleware line).
    - `cd services/player && go test ./...` exits 0 (no regressions; comment handler tests from plan 03 still pass).
  </acceptance_criteria>
  <done>Player chi router mounts the four comment routes with the correct public/protected partition.</done>
</task>

<task type="auto">
  <name>Task 4.3: Add four explicit comments proxy routes in the gateway BEFORE the /anime/* catch-all</name>
  <files>services/gateway/internal/transport/router.go</files>
  <read_first>
    - services/gateway/internal/transport/router.go (lines 140-160 — the existing reviews proxy block + the /anime/* catch-all to catalog)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pitfall 3, lines 638-651)
  </read_first>
  <action>
    Edit services/gateway/internal/transport/router.go.

    Locate the reviews proxy block (currently lines 143-149: r.Post("/anime/ratings/batch", ...), r.Get("/anime/{animeId}/reviews", ...), etc.). Immediately AFTER the line `r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)` and BEFORE the line `r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)` (the catalog catch-all begins at line 152), insert four new lines:

    - r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
    - r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
    - r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
    - r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)

    Add a one-line comment above the four new entries: `// Player service routes - comments (must be before /anime/* catch-all)` matching the convention of the existing reviews comment header.

    Do NOT touch any other gateway routes.
  </action>
  <verify>
    <automated>cd services/gateway && go build ./... && grep -c '/anime/{animeId}/comments' services/gateway/internal/transport/router.go</automated>
  </verify>
  <acceptance_criteria>
    - `cd services/gateway && go build ./...` exits 0.
    - `grep -c '/anime/{animeId}/comments' services/gateway/internal/transport/router.go` outputs `4` (one per HTTP verb).
    - The four new lines appear BEFORE the /anime/* catch-all. Verify via line numbers: `awk '/r\.Delete\("\/anime\/\{animeId\}\/comments/{a=NR} /r\.HandleFunc\("\/anime\/\*"/{b=NR} END{exit (a < b && a > 0)?0:1}' services/gateway/internal/transport/router.go` exits 0.
    - All four new lines use `proxyHandler.ProxyToPlayer` (not ProxyToCatalog). Verify: `grep '/anime/{animeId}/comments' services/gateway/internal/transport/router.go | grep -c ProxyToCatalog` outputs `0` and `grep '/anime/{animeId}/comments' services/gateway/internal/transport/router.go | grep -c ProxyToPlayer` outputs `4`.
  </acceptance_criteria>
  <done>Gateway routes comments to player before the catalog catch-all can claim them. Without this, every comment request would 404.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Checkpoint 4.4: Deploy player + gateway; functional smoke against live endpoints</name>
  <what-built>
    Live HTTP endpoints for the four comments operations. Frontend can hit them; auth + rate limit + soft delete + cursor pagination all enforced at runtime.
  </what-built>
  <action>Manual verification gate — implementer pauses execution and the human runs the steps in &lt;how-to-verify&gt; below, then types the resume signal. No automated work in this task.</action>
  <how-to-verify>
    1. Deploy both services: `make redeploy-player` then `make redeploy-gateway`. Wait for `make health` to return all-green.

    2. Pick a known anime ID (any from the seeded ui_audit_bot data, e.g. via `docker compose exec postgres psql -U postgres -d animeenigma -c "SELECT id FROM animes LIMIT 1"`). Export as `ANIME_ID=<value>`.

    3. Public listing — should return 200 with an empty list (no comments yet):
       `curl -s -i http://localhost:8000/api/anime/$ANIME_ID/comments | head -5`
       MUST show `HTTP/1.1 200 OK`. Body MUST be valid JSON like `{"comments":[], "has_more": false}` or `{"comments":null,...}` (either is acceptable — frontend handles both). MUST NOT be a catalog error body.

    4. Anonymous POST — should return 401:
       `curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8000/api/anime/$ANIME_ID/comments -H "Content-Type: application/json" -d '{"body":"hi"}'`
       MUST print `401`.

    5. Authenticated POST happy path — use the ui_audit_bot API key from docker/.env:
       `curl -s -i -X POST http://localhost:8000/api/anime/$ANIME_ID/comments -H "Authorization: Bearer $UI_AUDIT_API_KEY" -H "Content-Type: application/json" -d '{"body":"smoke test comment"}'`
       MUST show `HTTP/1.1 201 Created`. Body is a JSON Comment with non-empty `id`, `body == "smoke test comment"`, `username == "ui_audit_bot"`.

    6. Empty body 400:
       `curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8000/api/anime/$ANIME_ID/comments -H "Authorization: Bearer $UI_AUDIT_API_KEY" -H "Content-Type: application/json" -d '{"body":"   "}'`
       MUST print `400`.

    7. Rate limit 429 — post 10 valid comments in a tight loop, then assert the 11th returns 429:
       ```
       for i in $(seq 1 10); do curl -s -o /dev/null -X POST http://localhost:8000/api/anime/$ANIME_ID/comments -H "Authorization: Bearer $UI_AUDIT_API_KEY" -H "Content-Type: application/json" -d "{\"body\":\"rate-limit test $i\"}"; done
       curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8000/api/anime/$ANIME_ID/comments -H "Authorization: Bearer $UI_AUDIT_API_KEY" -H "Content-Type: application/json" -d '{"body":"11th"}'
       ```
       MUST print `429`.

    8. Activity event emitted — verify in postgres:
       `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "SELECT id, user_id, anime_id, type, content FROM activity_events WHERE type='comment' ORDER BY created_at DESC LIMIT 5;"`
       MUST show at least 11 recent rows of `type=comment` matching the comments posted in steps 5 + 7.

    9. Soft delete — get a commentId from the POST response in step 5; DELETE it; GET the comments list; assert it's not in the list.
       `curl -s -X DELETE http://localhost:8000/api/anime/$ANIME_ID/comments/<commentId> -H "Authorization: Bearer $UI_AUDIT_API_KEY"`
       MUST return 204. Subsequent GET MUST not show this comment. Postgres row check: `SELECT id, deleted_at FROM comments WHERE id='<commentId>';` MUST show non-NULL deleted_at.

    10. Cleanup: delete the 11 test comments via direct postgres or DELETE endpoint so they don't pollute the audit user's data.
  </how-to-verify>
  <resume-signal>Type "approved" if all 9 verification steps (5+all checks) pass. If any step returns a wrong status code or the gateway routes to catalog, describe the failure and the planner will produce a revision.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client → gateway (port 8000) | Untrusted; rate-limited at gateway by IP for general protection |
| gateway → player (port 8083) | Internal Docker network; trusted by mTLS/network policy |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-V13 | API & Web Service | gateway proxy routing | mitigate | Explicit routes for /anime/{animeId}/comments* registered BEFORE the /anime/* catch-all → catalog. Without this ordering, comments would 404 through catalog (RESEARCH.md Pitfall 3). Functional test in checkpoint 4.4 step 3 verifies. |
| T-1-V4 | Access control | chi router protected sub-group | mitigate | POST/PATCH/DELETE comments mounted inside the AuthMiddleware-protected sub-group; GET mounted outside. Functional test in checkpoint 4.4 steps 4-5 verifies. |
| Route precedence regression | Tampering | gateway router | mitigate | The four new comments routes are inserted directly above the catch-all line; awk-based acceptance check enforces that line ordering. |
</threat_model>

<verification>
- `cd services/player && go build ./...` exits 0
- `cd services/gateway && go build ./...` exits 0
- `cd services/player && go test ./...` exits 0
- `make redeploy-player` + `make redeploy-gateway` complete; `make health` returns all-green
- Curl smoke from checkpoint 4.4 produces 200/201/204/400/401/429 in the expected places
- `SELECT count(*) FROM activity_events WHERE type='comment'` shows ≥ 11 after the rate-limit test loop
</verification>

<success_criteria>
SOCIAL-04 fully shipped: comments CRUD live, rate-limited, soft-deleted, activity-event-emitting. Gateway routes correctly. Plan 05 + 06 (frontend) can now consume these endpoints.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-04-SUMMARY.md` documenting: the new chi route map for /anime/{animeId}, the four gateway routes, the live curl outputs from checkpoint 4.4 (status codes and a sample JSON body for each).
</output>
