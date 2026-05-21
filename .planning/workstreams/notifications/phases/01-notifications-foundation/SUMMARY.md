---
phase: 01-notifications-foundation
plan: 01
workstream: notifications
milestone: v1.0
status: complete
completed: 2026-05-21
executor_branch: worktree-agent-ac54bed54dd03a2c1 (merged to main via no-ff)
score:
  UXΔ: 0 (Ambiguous — pure infra; user-visible UX lands in Phase 3)
  CDI: 0.05 × 5
  MVQ: Sprite 80%/92%
commits:
  - 4c8d241  # Task 1 — scaffold service skeleton + domain models + Dockerfile
  - 712c648  # Task 2 — repo layer (CRUD + partial indexes + read-only views) + service
  - fc78747  # Task 3 — HTTP handlers + transport router + main.go wiring
  - f69191a  # Task 4 — gateway proxy /api/notifications/* to notifications:8090
  - 7e3c7ec  # Task 5 — docker-compose + env + CLAUDE.md + seed script + Makefile
  - 45e96be  # Task 6 — sibling Dockerfile COPY auto-fix (Rule 3)
requirements_resolved:
  - NOTIF-FOUND-01
  - NOTIF-FOUND-02
  - NOTIF-FOUND-03
  - NOTIF-FOUND-04
  - NOTIF-FOUND-05
  - NOTIF-FOUND-06
  - NOTIF-FOUND-07
  - NOTIF-FOUND-08
  - NOTIF-NF-04  # partial — service-ports row + gateway-routing row
---

# Phase 1 — Notifications Service Foundation Summary

**One-liner:** Standalone notifications microservice on port **8090** (not 8087 — see D-PORT below), full CRUD + UPSERT-producer + gateway proxy + idempotent seed script. Two service-owned tables + two partial indexes (`uk_user_dedupe`, `idx_user_unread`) in the shared `animeenigma` Postgres. Phase 2's detector can call `POST /internal/notifications` right now; Phase 3's frontend can develop against `/api/notifications/*` immediately.

## Verification matrix (live, 2026-05-21)

| SC  | Description | Result |
| --- | --- | --- |
| SC1 | `make health` includes `✓ notifications:8090` | **PASS** |
| SC2 | `/health` on 8090 + `/api/notifications` via gateway both return 200 with expected JSON | **PASS** |
| SC3 | Tables `user_notifications` + `parser_episode_snapshots` exist; partial indexes `uk_user_dedupe` + `idx_user_unread` exist with `WHERE (dismissed_at IS NULL)` | **PASS** |
| SC4 | Seed → list → dismiss → unread-count cycle (full `new_episode` payload roundtrip via `scripts/seed-notification-for-ui-audit-user.sh`) | **PASS** |
| SC5 | `/internal/notifications` returns 404 via gateway, returns 200 via `docker compose exec` inside Docker network | **PASS** |
| SC6 | Re-run seed script 3× — active-row count stays at 1 (UPSERT semantics via `(user_id, dedupe_key) WHERE dismissed_at IS NULL`) | **PASS** |

Full verbatim commands + output captured in the executor agent transcript (worktree branch `worktree-agent-ac54bed54dd03a2c1`).

## Deviations from plan

1. **D-PORT** (Rule 3 auto-fix) — Service ships on port **8090**, not the plan-specified 8087. Materialized risk R2: the host-native `services/maintenance/` binary (pid 38335) was already bound to `*:8087`, and `MAINTENANCE_URL=http://host-gateway:8087` is hard-wired into player, scheduler, and the Grafana webhook integration. Same blocker that pushed the `library` service to 8089 during v0.2. All references updated consistently: `docker-compose.yml` exposes 8090, CLAUDE.md Service Ports row lists 8090, gateway env default updated, `.env.example` updated, Makefile health probe updated, this workstream's PROJECT/REQUIREMENTS/ROADMAP all updated post-hoc.

2. **D-DOCKERFILE** (Rule 3 auto-fix, commit `45e96be`) — Added `COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/` to all 10 sibling service Dockerfiles (auth, catalog, gateway, library, player, rooms, scheduler, scraper, streaming, themes). Required because `go.work` now lists `./services/notifications` and every multi-stage Dockerfile that runs `go mod download` fails without the new module's go.mod present. Same pattern documented in MEMORY.md "Adding New libs/ Module" applied to services/.

## Risks materialized

- **R2** (port 8087 collision): YES → resolved via D-PORT (port 8090).
- **R5** (Makefile whitelist): NO — `redeploy-%` wildcard target is unconditional. Minor note: `deploy/scripts/redeploy.sh` SERVICE_PORTS map doesn't include notifications; non-blocking since the Make wildcard works.
- **R7** (`gorm.io/datatypes` pinning): NO — `v1.2.5` resolved cleanly against existing `gorm.io/gorm v1.30.0`.

## D-01..D-05 honored

- **D-01** (shared DB): `DB_NAME=animeenigma` in `services/notifications/internal/config/config.go`; `repo/views.go` uses the same `*gorm.DB` handle to LEFT-JOIN across `animes` / `anime_list` / `watch_history`.
- **D-02** (no cron in Phase 1): `services/notifications/internal/job/doc.go` is the only file in `job/` — no `cron.Cron` instance, no scheduler boot wiring.
- **D-03** (JWT context): handlers extract userID via `authz.UserIDFromContext(r.Context())`; zero references to any `X-User-ID` header anywhere in the new service.
- **D-04** (`gorm.io/datatypes`): `v1.2.5` in `services/notifications/go.mod`; `datatypes.JSON` is the type for `UserNotification.Payload` field.
- **D-05** (gateway-non-routing security): `services/gateway/internal/transport/router.go` proxies only `/api/notifications/*`; `services/notifications/internal/transport/router.go` mounts `/internal/notifications` + `/internal/health` at root with no middleware. SC5 verified the 404-via-gateway behavior.

## Process correction (recorded for posterity)

Initial Task 1 commit briefly landed on `main` due to a silent `cd /data/animeenigma && ...` pattern that drifted Bash invocations out of the worktree. Caught immediately: cherry-picked the orphaned commit onto the worktree branch (now `4c8d241`) and `git reset --hard 93dd1b9` on main to restore its prior HEAD. All subsequent tasks used explicit `git -C <worktree-path>` — no further drift. No work lost, no concurrent agent affected.

## Touched files summary

**New (services/notifications/):** 13 Go files (cmd/, config/, domain/{notification,snapshot}.go, handler/{internal,notification}.go, job/doc.go, repo/{indexes,notification,snapshot,views}.go, service/notification.go, transport/router.go) + Dockerfile + go.mod + go.sum.

**Modified (gateway):** internal/config/config.go (NotificationsURL field), internal/handler/proxy.go, internal/service/proxy.go, internal/transport/router.go (proxy block under authMiddleware).

**Modified (infra):** docker/docker-compose.yml (new notifications block), docker/.env.example, Makefile, go.work, go.work.sum.

**Modified (10 sibling Dockerfiles):** auth, catalog, gateway, library, player, rooms, scheduler, scraper, streaming, themes — each gained one `COPY services/notifications/go.mod ...` line.

**Docs:** CLAUDE.md (Service Ports row + Gateway Routing line + env-var sub-block).

**Scripts:** `scripts/seed-notification-for-ui-audit-user.sh` (executable, idempotent UPSERT seeder via `docker compose exec`).

## Next — Phase 2 ready

- **CRUD surface live** → Phase 3 frontend unblocked to develop against `/api/notifications/*` immediately.
- **Producer endpoint live** (`POST /internal/notifications`) → Phase 2 detector calls this for free UPSERT semantics with `(user_id, dedupe_key) WHERE dismissed_at IS NULL` partial-unique handling.
- **Both tables exist** → `parser_episode_snapshots` already has its composite unique index `uk_combo`, ready for Phase 2's `BulkUpsert` from the detector job.
- **`job/` package reserved** → Phase 2's plan diff stays "add 4 files" rather than "create dir + add 4 files".
- **Single-DB pattern locked** (D-01) → NOTIF-DET-02's "cross-DB query" wording resolved to "single GORM handle"; Phase 2 plan can drop the second-connection contingency.

## Score (per project convention)

- **UXΔ:** **0 (Ambiguous)** — pure backend infra, no user-visible UX yet. The visible-UX delivery is Phase 3 (Bell + Toast).
- **CDI:** `0.05 × 5` — Spread: backend (1 new service, 1 gateway change, 1 docker-compose change, 10 sibling Dockerfile patches, 1 CLAUDE.md doc). Shift: low (additive only, no schema changes on existing tables, no API breakage). Effort: 5 Fibonacci (single wave of 6 tasks, well-scoped from a detailed plan).
- **MVQ:** **Sprite 80%/92%** — Sprite (small, quick, supportive) is the right shape for foundation work: invisible to users but enables the rest. 80% match — could have been a Gnome (sturdy infra spirit) but the snappy 6-commit cadence + worktree isolation feels Sprite-shaped. 92% slop-resistance — UPSERT idempotency + partial-unique semantics + 6/6 live verification + auto-fix discipline made accidental defects extremely unlikely to land.
