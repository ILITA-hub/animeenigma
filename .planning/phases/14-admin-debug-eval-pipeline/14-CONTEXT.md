# Phase 14: Admin Debug Page & Eval Pipeline - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated with locked decisions from design spec §9 + Phase 13 hand-off (autonomous mode)

<domain>
## Phase Boundary

Land the full `/admin/recs/:user_id` debug page (per-signal contribution table, S5 TF-IDF term breakdown on row expand, S11 filter audit), the force-recompute endpoint, the frontend `rec_click` / `rec_watched` events tagged with the top contributor signal, and the Prometheus `rec_signal_ctr` per-signal CTR metric. After this phase ships, every ranking decision is auditable and v2.1 weight tuning has data.

In scope:
- **Backend admin endpoint (`GET /api/admin/recs/:user_id`):** returns a top-50 breakdown for the target user. Per-row payload: rank, anime ID/name/poster, final score, weight × normalized contribution per signal (s1, s2, s3, s4, s5), top contributor signal ID, S5 TF-IDF term breakdown (e.g. `{studio: "Madhouse", tf: 0.41, idf: 2.30}`), S6 cascade source ("local" / "shikimori_similar" / "score_5_fallback") when applicable, plus a sibling `filtered_out` array listing anime that S11 removed with reason (`status=completed`, `status=dropped`, `hidden=true`).
- **Backend force-recompute endpoint (`POST /api/admin/recs/:user_id/recompute`):** invalidates `recs:user:{user_id}:topN` Redis cache, calls `userOrchestrator.RunForUser(ctx, userID)` synchronously, returns `{computed_at, top_n_count, latency_ms}`.
- **Both endpoints are admin-only.** Auth check: JWT must be present AND `user.role == 'admin'`. Use existing `authz` middleware patterns; if no admin-only middleware exists yet, add it (look for prior precedent — themes service has admin sync endpoints).
- **Frontend admin route:** `/admin/recs/:user_id` Vue route guarded by `useAuthStore.isAdmin`. New view: `frontend/web/src/views/admin/AdminRecs.vue` (or wherever the project's admin-view convention lives — verify in plan-phase). Layout: top-50 table with columns rank | anime poster + name | final | s1 | s2 | s3 | s4 | s5 | top contributor; expanding a row reveals the S5 TF-IDF term breakdown plus S6 cascade source; separate "Filter audit" panel below the table; force-recompute button at the top.
- **Frontend telemetry events (REC-EVAL-01):**
  - `rec_click`: emitted when the user clicks a card in any recs row (Trending now / Up Next for you, anonymous OR logged-in). Payload: `{event: "rec_click", anime_id, rank, top_contributor, pinned, pin_source, pin_seed_anime_id, source_route}`. Use the existing analytics emit path (look for prior precedent in `frontend/web/src/utils/analytics.ts` or similar; if no precedent, send to a new `POST /api/events/rec` endpoint).
  - `rec_watched`: emitted when the player crosses the 20-minute auto-mark threshold for an anime that originated from a `rec_click` (the click ID is correlated by anime_id + recent timestamp; if no recent click, the event is not emitted). Same payload shape.
- **Backend Prometheus metric (REC-EVAL-02):**
  - `rec_signal_ctr` — gauge OR summary, labeled by `signal_id`. Computed as `rec_watched_total{signal_id=X} / rec_click_total{signal_id=X}` over the last hour.
  - Underlying counters: `rec_click_total{signal_id, pinned}` and `rec_watched_total{signal_id, pinned}` exposed via existing `libs/metrics` package on `/metrics` of the player service (or wherever the events handler lives). Both are tagged with `signal_id` (the top contributor at click time, or `"s6_pin"` if `pinned: true`) and `pinned` (boolean string).
- **Grafana dashboard:** new "Rec engine" dashboard row (or panel set) with the per-signal CTR plus daily click/watch totals. Panels query the new metrics. **Dashboard provisioning:** add a JSON file under `infra/grafana/dashboards/` (or wherever the project's Grafana dashboard JSON lives; verify in plan-phase) and reference in `infra/grafana/provisioning/dashboards.yaml` if that pattern exists.

Out of scope (deferred to v2.1 / v3.0):
- Editable signal weights with "preview" button → v2.1+
- Per-row neighbor expansion for S1 (showing the top-K neighbors and their scores) → v2.1
- S6 seed history (past 30 days of qualifying completions) → v2.1
- Diversification re-rank (S12) → v2.1
- A/B testing framework for weight tuning → v3.0
- Click→watch attribution beyond simple anime_id timestamp matching (e.g., session-based attribution) → v2.1

</domain>

<decisions>
## Implementation Decisions

### Backend Admin Endpoints (Decision §C1)

- **File:** `services/player/internal/handler/admin_recs.go` + `admin_recs_test.go`. Lives under `services/player/internal/handler/` rather than a new `admin/` subdirectory — Go convention is flat unless a package needs isolation.
- **Auth middleware:** require admin role. If `services/player/internal/middleware/admin.go` (or similar) doesn't exist, add it: extracts claims via `authz.ClaimsFromContext`, asserts `claims.Role == "admin"`, returns `403 Forbidden` otherwise. Mount on the `/api/admin/recs/*` routes only — do not affect existing routes.
- **Routing:** add to `services/player/internal/transport/router.go`:
  ```
  r.Route("/api/admin/recs", func(r chi.Router) {
      r.Use(adminMiddleware)
      r.Get("/{user_id}", h.GetAdminRecs)
      r.Post("/{user_id}/recompute", h.ForceRecompute)
  })
  ```
- **Gateway routing:** add `/api/admin/recs/*` route to the gateway (`services/gateway/internal/transport/router.go`) pointing to player:8083. The gateway's existing admin routes proxy to other services (catalog, themes, streaming) — follow that pattern.
- **GetAdminRecs handler shape:**
  ```go
  type AdminRecRow struct {
      Rank             int                       `json:"rank"`
      Anime            RecAnimePayload           `json:"anime"`
      Final            float64                   `json:"final"`
      Breakdown        map[string]float64        `json:"breakdown"`         // {s1, s2, s3, s4, s5}
      Weights          map[string]float64        `json:"weights"`           // ditto
      TopContributor   string                    `json:"top_contributor"`
      ContributorDetail map[string]interface{}   `json:"contributor_detail,omitempty"` // S5 TF-IDF terms; S6 cascade source
      Pinned           bool                      `json:"pinned,omitempty"`
      PinReason        string                    `json:"pin_reason,omitempty"`
      PinSource        string                    `json:"pin_source,omitempty"`
      PinSeedAnimeID   string                    `json:"pin_seed_anime_id,omitempty"`
  }
  type FilteredOutEntry struct {
      AnimeID string `json:"anime_id"`
      Reason  string `json:"reason"`  // "status=completed" | "status=dropped" | "hidden=true"
  }
  type AdminRecsResponse struct {
      Recs            []AdminRecRow      `json:"recs"`
      FilteredOut     []FilteredOutEntry `json:"filtered_out"`
      ComputedAt      string             `json:"computed_at"`
      SignalVersions  map[string]string  `json:"signal_versions"`  // {s1: "v1.0", ...}
      UserID          string             `json:"user_id"`
  }
  ```
- **Implementation:** GetAdminRecs reuses `RecsHandler.computeFreshForUser` BUT extends ensemble.Rank to surface per-signal raw + normalized + weight × normalized for every candidate (not just final). The cleanest path: add a new `Ensemble.RankWithBreakdown(ctx, userID, candidates) ([]RankedRowWithBreakdown, error)` method that returns the breakdown alongside the final score. Existing `Rank` keeps its narrow shape for the public handler.

### Force-Recompute (Decision §C2)

- **File:** same `admin_recs.go` handler.
- **Implementation:**
  1. Parse `user_id` from the route param.
  2. `cache.Del(ctx, "recs:user:{user_id}:topN")` — fire-and-forget OK; if cache layer fails, log and continue.
  3. `start := time.Now()`; `userOrchestrator.RunForUser(ctx, userID)` (synchronous); `latency := time.Since(start)`.
  4. Re-fetch the just-computed top-N (call `computeFreshForUser` to materialize fresh recs).
  5. Return `{computed_at: time.Now().UTC().Format(RFC3339), top_n_count: len(top), latency_ms: latency.Milliseconds()}`.
- **Auth:** same admin middleware. No special CSRF / nonce — admin endpoints are JWT-protected and the operation is idempotent (running it twice produces the same result).
- **Latency:** expected 100ms-2s depending on cron complexity for that user (S1 + S2 + S5 precompute); Phase 12 SUMMARY data showed ~1.2s for ui_audit_bot's full precompute. Acceptable for a manual debug action.

### Frontend Admin View (Decision §C3)

- **Route:** `/admin/recs/:user_id` in `frontend/web/src/router/index.ts`. Add a route guard that calls `useAuthStore.isAdmin.value` — redirect to home (or 403 page if one exists) if non-admin.
- **View file:** `frontend/web/src/views/admin/AdminRecs.vue` (creating the `views/admin/` subdirectory if it doesn't exist). New composable: `frontend/web/src/composables/useAdminRecs.ts` — fetches `GET /api/admin/recs/:user_id`, exposes `{rows, filteredOut, computedAt, isLoading, error, refresh, recompute}`.
- **Layout:**
  - Top: page title, user_id breadcrumb, force-recompute button.
  - Main: top-50 table. Sticky header. Each row clickable to expand the contributor_detail row beneath it. Rank, poster (small), name (with link to `/anime/:id`), final, then 5 columns for s1-s5 contributions, top_contributor pill.
  - Below table: "Filter audit" panel — a small box listing each filtered_out entry with anime name (lookup) and reason badge.
- **Styling:** matches existing project conventions (Tailwind + the existing card component patterns from Phase 11/13). Pin highlighting (border + badge from Phase 13) carries through if a row has `pinned: true`.

### Telemetry Events (Decision §C4)

- **Frontend emit path:** add `frontend/web/src/utils/recsAnalytics.ts` (new file) with two functions:
  - `emitRecClick(payload: RecClickPayload)`
  - `emitRecWatched(payload: RecWatchedPayload)`
- **Click correlation for rec_watched:** when `emitRecClick` fires, store `{anime_id, signal_id, pinned, timestamp}` in `localStorage.recentRecClicks` (FIFO, keep last 50 entries, expire entries > 1h old). When `MarkEpisodeWatched` succeeds AND it's the user's first auto-mark of the day for that anime AND the anime ID matches a recent click within 1 hour → call `emitRecWatched` with the matching click's signal_id.
- **Backend endpoint:** new `POST /api/events/rec` route (player service, JWT-optional — anonymous events are valid for the public trending row's CTR data).
  - Request body: `{event_type: "rec_click" | "rec_watched", anime_id, signal_id, pinned, ...}`.
  - Handler: `services/player/internal/handler/rec_events.go`. Increments the relevant Prometheus counter. Optionally persists to a new `rec_events` table for raw audit (Decision: persist for v2.1 weight tuning data — small table, append-only).
- **Schema (NEW):** `rec_events` table with columns: `id (UUID PK)`, `event_type (string)`, `user_id (UUID nullable)`, `anime_id (UUID)`, `signal_id (string)`, `pinned (bool)`, `pin_source (string nullable)`, `pin_seed_anime_id (UUID nullable)`, `created_at (timestamp default now())`. Indexed on `(user_id, created_at desc)` and `(signal_id, event_type, created_at)`. **Auto-migrated** alongside existing player tables.

### Prometheus Metric (Decision §C5)

- **File:** `libs/metrics/recs.go` (new — extends the existing libs/metrics package).
- **Metrics exposed:**
  - `rec_click_total{signal_id, pinned}` Counter
  - `rec_watched_total{signal_id, pinned}` Counter
  - `rec_signal_ctr` is computed via Grafana / Prometheus query, NOT a separate metric: `rate(rec_watched_total[1h]) / rate(rec_click_total[1h])` per `signal_id`. This avoids needing a custom metric type and works with the standard Prometheus rate function.
- **Wire-up:** the new `/api/events/rec` handler increments the counters using existing `libs/metrics` patterns. Player service `/metrics` endpoint surfaces them automatically.

### Grafana Dashboard (Decision §C6)

- **Dashboard JSON:** `infra/grafana/dashboards/rec-engine.json`. New panel: "Per-signal CTR (1h rate)" with a Prometheus query `rate(rec_watched_total[1h]) / rate(rec_click_total[1h])` grouped by `signal_id`. Other panels: "Click rate by signal", "Watch rate by signal", "Pin CTR (s6_pin signal_id)", "Top-clicked anime (last 24h)".
- **Provisioning:** check `infra/grafana/provisioning/dashboards.yaml` (or equivalent); if dashboards are auto-loaded from `dashboards/` directory, just drop the JSON in. Otherwise add to the YAML.
- **No custom Prometheus rules needed** — the recording rule (`rec_signal_ctr`) is a Grafana panel query, not a stored metric.

### Locked from spec §13 / Phase 9-13 hand-offs (do not relitigate)

- Per-signal CTR formula: `rec_watched_total / rec_click_total` (spec §11.5)
- Top contributor signal ID determined at click time (spec §11.5)
- S6 pin events tagged with signal_id `s6_pin` (Phase 13 SUMMARY hand-off)
- Pin source surfacing in admin: "local" / "shikimori_similar" / "score_5_fallback" (Phase 13 SUMMARY hand-off)
- S11 filter audit categories: `status=completed`, `status=dropped`, `hidden=true` (Phase 11 + Phase 13 already implement these)

### Claude's Discretion

- Whether the admin middleware lives in `services/player/internal/middleware/admin.go` or inline in router.go — small enough either way; pick the cleaner option after inspecting existing middleware layout.
- Whether the click-correlation `localStorage` key is `recentRecClicks` or namespaced (e.g., `aenima:recent-rec-clicks`); pick whichever matches existing localStorage conventions.
- Whether the admin view's table uses Tailwind directly or a dedicated table component — match existing project pattern (look at how the Anime list / Browse views render tables).
- Whether `RankWithBreakdown` is a new method on `Ensemble` or a separate `EnsembleDebugger` type — both work; pick the cleaner Go API.
- Whether the per-row expand UI uses a `<details>` element, a Vue transition, or a permanent column. Match existing project UX.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/handler/recs.go` — Phase 11/12/13 logged-in branch. The admin debug endpoint reuses much of `computeFreshForUser` but with breakdown surfacing.
- `services/player/internal/service/recs/ensemble.go` — Phase 9. Add `RankWithBreakdown` method beside the existing `Rank`.
- `services/player/internal/service/recs/user_orchestrator.go` — Phase 11. The force-recompute endpoint calls `RunForUser`.
- `libs/cache.Del` — Phase 11 already used for cache invalidation in handlers.
- `libs/metrics` — existing Prometheus integration. Phase 14 extends with new counters.
- `libs/authz.ClaimsFromContext` — already extracts JWT claims; admin middleware reads `claims.Role`.
- `frontend/web/src/stores/auth.ts:48` — `isAdmin = computed(() => user.value?.role === 'admin')`. Used for view guards.
- `frontend/web/src/router/index.ts` — existing route definitions; admin route slots in here.
- `services/gateway/internal/transport/router.go:77,137,203,221` — established admin-route pattern (proxy to specific service); Phase 14 adds an admin/recs/* line.

### Established Patterns
- **Admin endpoints:** themes service has `POST /admin/sync` and `GET /admin/sync/status` per the gateway routing. Look at how themes' admin handler enforces role; mirror it.
- **Cron-triggered recompute:** Phase 11/13 orchestrators already call `RunForUser` synchronously in tests. Force-recompute uses the same path.
- **Vue admin views:** none yet — Phase 14 introduces the first `views/admin/` subdirectory.
- **Prometheus counters in libs/metrics:** existing per-service metrics (gateway, catalog, etc.) follow `metrics.HTTPRequests*` pattern. New `metrics.RecClicks` / `metrics.RecWatched` follow the same shape.
- **Click → watched correlation via localStorage:** the existing report-button + watch-history flow has prior art for client-side event correlation; check `frontend/web/src/utils/diagnostics.ts` from Phase 11 SUMMARY for the pattern.

### Integration Points
- **Add admin middleware** to player routes (or use existing one if it exists).
- **Wire admin route in gateway** (one-liner in services/gateway/internal/transport/router.go).
- **Add new route to player:** `r.Route("/api/admin/recs", ...)`.
- **Auto-migrate `rec_events` table** in `services/player/cmd/player-api/main.go` alongside existing tables.
- **Wire counters** in `libs/metrics/recs.go`.
- **Grafana dashboard JSON** dropped into `infra/grafana/dashboards/`.
- **No new env vars.**
- **No new dependencies** — stdlib + existing prom client + GORM.

</code_context>

<specifics>
## Specific Ideas

- **Spec ref:** `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §9 (admin debug page) + §11.5 (eval pipeline) are the binding contracts.
- **Existing admin role check:** `frontend/web/src/stores/auth.ts:48` exposes `isAdmin`. Backend uses `claims.Role`. Both need to align; verify in plan-phase that the JWT actually carries the role claim (look at `services/auth/...` for the JWT generation path).
- **CTR data needs at least 1 week** of click + watch events to produce useful per-signal CTR numbers. Phase 14 ships the infrastructure; the actual weight tuning happens in v2.1 once data accumulates.
- **Per CLAUDE.md UI Audit Test User:** `ui_audit_bot` (with role=test, NOT admin) is the analytics target user for admin-page testing — admin can fetch ui_audit_bot's debug page after redeploy. Production verification: log in as a real admin, navigate to `/admin/recs/{ui_audit_bot_user_id}`, verify the table renders with all 5 signal columns + the S5 TF-IDF expansion + a sample S11 filter audit.
- **S6 pin in admin view:** when ui_audit_bot's seed is set (Phase 13 verification artifact), the admin page should surface `recs[0].pinned=true`, `pin_source=...`, `pin_seed_anime_id=...` distinctly from the regular ensemble rows.
- **Background AniList tag backfill is still running** — by the time Phase 14 completes, tag coverage will likely be 50-80%. The admin page will show progressively richer S5 TF-IDF term breakdowns as tags fill in. No special handling needed.
- **Phase 13 hand-off explicitly required** these Phase 14 deliverables (per `13-01-SUMMARY.md`):
  - Admin debug page surfacing `pin_source`
  - `rec_click` events tagging `pin_seed_anime_id` when applicable
  - Force-recompute endpoint
  - Filter audit panel with `s6_cascade_dropped_by_s11` reason category (advisory; can be added if cheap)
- **Per CLAUDE.md:** all new copy goes to BOTH EN and RU. Admin labels (column headers, button labels, modal text) — EN + RU + JA mirror EN. New i18n keys: `admin.recs.title`, `admin.recs.forceRecompute`, `admin.recs.column.rank`, `admin.recs.column.final`, `admin.recs.filterAudit`, etc.

</specifics>

<deferred>
## Deferred Ideas

- Editable signal weights with "preview" button → v2.1 (after CTR data exists)
- Per-row S1 nearest-neighbor expansion → v2.1
- S6 seed history (past 30 days) → v2.1
- Diversification re-rank (S12) → v2.1
- A/B testing framework → v3.0
- Session-based click→watch attribution (vs the simple anime_id+timestamp match) → v2.1
- Real-time CTR dashboard updates (currently the 1h rate is the freshness floor) → v3.0
- Per-anime CTR breakdown (which specific anime are over- or under-performing) → v2.1
- Click-vs-impression rate (impressions = how many times an anime was rendered in a recs row) → v2.1; v1 only tracks click-vs-watch

</deferred>
