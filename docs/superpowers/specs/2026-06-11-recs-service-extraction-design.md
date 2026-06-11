# Recommendation Engine: Service Extraction + Quality Ride-Alongs

**Date:** 2026-06-11
**Status:** Approved design, pending implementation plan
**Owner decision trail:** extract-first > internal-HTTP trigger seam > re-point existing URLs > ride-alongs: ISS-026 + S7 dropped-penalty + S12 diversification

## Goal

Move the recommendation engine out of `services/player` into a new `services/recs` microservice (port **8094**), then improve recommendation quality in three measured steps: close the conversion-measurement loop (ISS-026), add a negative "dropped-penalty" signal (S7), and add a diversification re-rank (S12).

## Current State (v2.0, Phases 9–14)

The engine lives entirely inside `services/player`:

- **Core:** `services/player/internal/service/recs/` — ensemble (weighted sum over min-max-normalized signals), signals S1–S6, S11 candidate filter, PopulationOrchestrator (60min cron), UserOrchestrator (6h cron + on-write debounce), co-occurrence materializer.
- **Handlers:** `internal/handler/recs.go` (`GET /api/users/recs`, optional JWT), `admin_recs.go` (`/api/admin/recs/*`, admin debug + force recompute), `rec_events.go` (`POST /api/users/rec-events`, click/watched telemetry).
- **Persistence:** `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` (GORM AutoMigrate in player's main.go). Redis: `recs:public:trending:topN` and `recs:user:{uid}:topN:v2`, both 6h TTL, plus `recs:user:{uid}:debounce` SetNX lock.
- **Weights (logged-in, sum 1.0):** S1 score-cluster 0.30, S2 genre-Jaccard 0.20, S3 trending 0.20, S4 recency 0.10, S5 TF-IDF attribute 0.20. Anonymous: S3 0.20 + S4 0.10 only. S6 "Because you finished X" is a post-rank pin.
- **Key fact enabling a cheap move:** all core services share the single `animeenigma` Postgres database, so the new service can read `anime_list` / `watch_history` / `animes` / `anime_genres` / `anime_tags` / `anime_studios` directly. No data-replication seam is required.

The only true in-process coupling is the **on-write debounce**: player's watch-history insert path triggers a user-signal recompute in the same process. That seam is the one piece that must become cross-service.

## Phasing (Approach A — staged)

Each phase deploys independently. Ranking-output changes are isolated to the phases that intend them.

| Phase | Deliverable | Behavior change | UXΔ | CDI | MVQ |
|-------|-------------|-----------------|-----|-----|-----|
| ① | Mechanical extraction to `services/recs` | **None (byte-identical)** | 0 (Ambiguous) | 0.06 * 21 | Basilisk 90%/85% |
| ② | ISS-026 watched-event instrumentation | None (telemetry only) | 0 (Ambiguous) | 0.01 * 3 | Sprite 80%/85% |
| ③ | S7 dropped-penalty signal | Ranking change (demotes) | +2 (Better) | 0.02 * 5 | Griffin 85%/80% |
| ④ | S12 diversification re-rank | Ranking change (reorders) | +3 (Better) | 0.02 * 8 | Phoenix 85%/75% |

Rationale for the order: ② lands measurement (per-signal CTR, watch-rate, pin-CTR Grafana panels) **before** ③/④ change rankings, so quality changes are observable. ① first gives a byte-identical checkpoint — any later ranking diff is attributable to ③/④, never to the move.

## Phase ① — Extraction to `services/recs`

### Service skeleton

Standard layout per CLAUDE.md (`cmd/recs-api/main.go`, `internal/{config,domain,handler,service,repo,transport}`, Dockerfile, go.mod). Port **8094** (8093 is reserved for gacha). Registered in `go.work`; per the libs-module rule, no new `libs/` module is created so no Dockerfile fan-out is needed.

### What moves (near-verbatim)

| From `services/player` | To `services/recs` |
|---|---|
| `internal/service/recs/**` (types, ensemble, normalizer, signals s1–s6 + s11, both orchestrators, co_occurrence) | `internal/service/recs/**` |
| `internal/handler/recs.go`, `admin_recs.go`, `rec_events.go` | `internal/handler/` |
| `internal/domain/recs.go` | `internal/domain/` |
| `internal/repo/recs.go` | `internal/repo/` |
| AutoMigrate of `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` | recs main.go |
| Unit tests co-located with the above | move with their code |

Player-side copies (code, routes, AutoMigrate calls, orchestrator startup) are **deleted in the same phase** — no dual-serving window.

### Data access

- Recs reads `anime_list`, `watch_history`, `animes`, `anime_genres`, `anime_tags`, `anime_studios` **read-only** via the shared `animeenigma` DB (GORM, `libs/database`). Player remains the owner/writer of `anime_list` and `watch_history`.
- Recs owns and writes the three `rec_*` tables. Schema unchanged; ownership handover is a no-op (AutoMigrate is idempotent).
- S6's Shikimori-similar fallback calls catalog over HTTP: `CATALOG_URL` (default `http://catalog:8081`) — same pattern as notifications.

### Trigger seam (replaces in-process debounce)

- New internal endpoint on recs: `POST /internal/recs/recompute-hint` body `{"user_id": "<uuid>"}`. **Docker-network-only; the gateway does not proxy `/internal/*`** (same rule as `/internal/notifications`, `/internal/effects`).
- Player's watch-history insert path fires the hint via a **non-blocking, drop-on-full producer** (clone of the analytics `/internal/effects` producer pattern): buffered channel + single sender goroutine; on full buffer or recs outage, hints are dropped and a WARN is logged. A recs outage can never slow or fail a watch request.
- Env on player: `RECS_INTERNAL_URL` (default `http://recs:8094`).
- The Redis SetNX debounce (`recs:user:{uid}:debounce`) moves into recs, keyed off the hint endpoint. Worst case if hints are dropped: user signals refresh on the existing 6h cron instead of within the debounce window — graceful degradation, identical to today's behavior when the debounce loses a race.

### Gateway re-pointing (zero frontend changes)

Carve-outs re-pointed from player to `recs:8094`:

- `GET /api/users/recs` — optional-JWT middleware; **must stay registered before the protected `/users/*` group** (chi longest-prefix precedence; carry the existing comment over).
- `POST /api/users/rec-events` — same carve-out family.
- `GET /api/admin/recs/{user_id}` and `POST /api/admin/recs/{user_id}/recompute` — JWT + AdminRole, carved out of the `/api/admin/* → catalog` default.

URL shapes are unchanged; `useRecs.ts` and the admin page need no edits in this phase.

### Ops wiring

- `docker/docker-compose.yml`: new `recs` service (standard `DB_*`, `REDIS_*`, `JWT_SECRET`, `CATALOG_URL`, `ANALYTICS_INTERNAL_URL`), healthcheck, depends_on postgres/redis.
- Prometheus scrape target for `recs:8094/metrics`. The existing `libs/metrics` recs counters (`rec_click_total`, `rec_watched_total`, etc.) now exported by recs — Grafana panels keep working because they query by metric name, not job, but panel job filters must be checked and updated if they pin `job="player"`.
- `Makefile`: `redeploy-recs`, `restart-recs`, `logs-recs` come for free if targets are pattern-based; verify.
- CLAUDE.md updates: Service Ports table (+`recs | 8094`), Gateway Routing list, recs env vars.
- Activity Register: recs ships egress effects via `ANALYTICS_INTERNAL_URL` like catalog/scraper/streaming; background orchestrators must `tracing.SeedBaggage(ctx, "scheduled_job:<name>", "")` (existing calls move with the code — verify they survive the move).

### Phase ① acceptance (byte-identical gate)

1. Before the move: capture `GET /api/admin/recs/{ui_audit_bot}` JSON and `GET /api/users/recs` (as `ui_audit_bot` and anonymous) to files.
2. After deploy: re-capture and diff — per-signal raw/normalized/weighted scores and final ranking must be identical (`generated_at`/`cache_hit` fields excluded).
3. `make health` green across all services; `rec_click_total` still increments on a real click; recompute admin endpoint returns sane latency.
4. Player image no longer contains recs code; gateway routes resolve to recs.

## Phase ② — ISS-026: emit the `watched` rec-event (frontend)

**Problem:** `rec_watched_total` has zero series because the frontend never POSTs `{type:"watched"}`; 3 Grafana panels (per-signal CTR, watch-rate by signal, pin CTR) are permanently blank, so rec conversion is unmeasurable.

**Design:**

- `useRecs.ts` already writes `recentRecClicks` to localStorage on card click (anime_id, signal/top_contributor, timestamp).
- Add a check in the watch-completion path (where the player auto-marks an episode complete and where the manual complete button fires): if the anime ID matches a `recentRecClicks` entry younger than **7 days**, POST `POST /api/users/rec-events` with `{type:"watched", anime_id, signal_id}` and then remove the entry (fire at most once per rec-click).
- Entries older than 7 days are pruned on read.
- No backend changes — the endpoint and counter already work (`services/player/internal/handler/rec_events.go:81` today; lives in recs after Phase ①).

**Acceptance:** click a rec card with `ui_audit_bot`, watch/auto-complete an episode, observe `rec_watched_total` series appear in Prometheus and the 3 panels render data.

## Phase ③ — S7 dropped-penalty signal

**Intuition:** things similar to what a user explicitly dropped should rank lower. Dropping is noisy (pacing, mood, life), so the signal **demotes, never buries**.

**Design (mirrors S2's stateless request-time pattern, inverted):**

- **Seeds:** `anime_list` rows with `status='dropped'`, **excluding** rows the user scored ≥7 (dropped-but-liked).
- **Cold-start guard:** fewer than 2 eligible seeds → signal returns all-zero (silent).
- **Raw score:** per candidate, `max` over seeds of Jaccard similarity computed on the union of genre IDs and tag IDs (`genre:{id}`, `tag:{id}` namespaced into one set). Range [0,1].
- **Weight:** existing positive weights unchanged (S1 0.30 / S2 0.20 / S3 0.20 / S4 0.10 / S5 0.20, sum 1.0). S7 enters the same weighted sum at **−0.15** over the min-max-normalized score. The ensemble already supports arbitrary weights; normalization keeps the penalty bounded.
- **No persistence, no orchestrator work** — computed per-request like S2. Applies to logged-in flow only (anonymous users have no list).
- **Cache key bump:** `recs:user:{uid}:topN:v2` → `:v3` in this phase, in all three places that reference it (handler, user orchestrator, admin recompute), so stale v2 rankings don't serve alongside new ones. (Spotlight cache-shape lesson: bump or flush, never assume.)
- **Admin debug:** S7 appears in the `/api/admin/recs/{user_id}` per-signal breakdown with raw/normalized/weighted values, like every other signal.
- **Tests:** co-located table-driven tests with handwritten fakes (house style): liked-drop exclusion, cold-start guard, max-Jaccard math, negative contribution direction, weight constant.

**Tuning path:** −0.15 is a starting constant; Phase ② panels (per-signal CTR / watch-rate) are the feedback loop for adjusting it later.

## Phase ④ — S12 diversification re-rank

**Problem:** a weighted-sum ensemble happily fills the row with 20 near-identical cards (same genre cluster, same studio, sequels).

**Design (greedy MMR, post-rank, pre-slice):**

- Operates on the ranked top-50 (logged-in) / top-20 (anonymous) **before** slicing to the served row.
- Greedy selection: repeatedly pick the candidate maximizing `final − λ · max_sim(candidate, already_picked)`, with `λ = 0.3`.
- `sim(a, b)` = Jaccard over the namespaced union of genre IDs + studio IDs.
- **Hard cap:** at most **3** picked items sharing an identical genre-ID set (catches same-franchise sequels, which share exact genre sets; there is no `franchise` column on `animes` — Shikimori franchise backfill is explicitly out of scope, noted as a future improvement).
- **S6 pin exemption:** the "Because you finished X" pin stays at rank 1 and is excluded from MMR; it does count as "already picked" for similarity purposes so the row doesn't open with three clones of the pin.
- Applies to **both** logged-in and anonymous rows (the trending row is the most genre-monotone).
- `top_contributor` / per-signal telemetry fields are computed before re-rank and carried through unchanged; `rank` reflects the post-S12 order.
- **Cache key bumps:** `recs:user:{uid}:topN:v3` → `:v4` and `recs:public:trending:topN` → `recs:public:trending:topN:v2`.
- **Admin debug:** breakdown response gains a `pre_s12_rank` field per item so reordering is inspectable.
- **Tests:** MMR picks lower-final-score but diverse item over near-duplicate; hard cap enforced; pin exempt but similarity-counted; λ=0 degenerates to identity order.

## Out of Scope (recorded for the next milestone)

- Anonymous personalization via `X-Anon-ID` (REC-V21-01).
- Adult-content filter in S11 (needs per-user setting).
- Shikimori `franchise` backfill + true franchise-aware dedup in S12.
- Further signals from unused data: time-decay watch recency, themes/OP-ED ratings affinity, watch-completion-ratio, clickstream sequence signal, seasonal affinity.
- Weight tuning of S1–S5/S7 and λ against the Phase ② conversion panels.
- Multi-instance cache-coherence (single player/recs instance assumed, as today).

## Risks

| Risk | Mitigation |
|---|---|
| Extraction silently changes rankings | Byte-identical diff gate on admin breakdown before/after (Phase ① acceptance) |
| Dropped recompute hints (recs down) | Graceful: 6h cron still refreshes; producer logs WARN; identical to today's debounce-miss behavior |
| Stale cache serves old-shape/old-ranking payloads | Explicit cache-key version bumps in ③/④; spotlight-lesson applied |
| Grafana panels pin `job="player"` | Check/repoint panel job filters in Phase ① |
| Chi route precedence regression on `/api/users/recs` | Carve-out registered before protected `/users/*` group; comment carried over; smoke anonymous + authed after deploy |
| S7 over-penalizes (noisy drop data) | Conservative −0.15, liked-drop exclusion, ≥2-seed guard, tunable via ② metrics |
| MMR makes the row feel "worse-ranked" | λ=0.3 conservative; hard cap only at 3 identical-genre-set items; admin `pre_s12_rank` for inspection |

## Verification Summary

- Per-phase: unit tests move/added (`go test ./... -race` in `services/recs`), `make health`, gateway-path smoke as anonymous + `ui_audit_bot`, in-browser check of the home rec row (desktop + mobile per DS-NF-06 — the row is a rendered surface even when only ranking changes).
- Phase ① additionally: admin-breakdown byte-diff gate.
- Phase ② additionally: Prometheus series + Grafana panel check.
- `/animeenigma-after-update` after each phase (lint, redeploy, changelog, commit, push).
