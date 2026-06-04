---
phase: 01-clickhouse-foundation-eventstore-swap
plan: 01
subsystem: infra
tags: [clickhouse, docker-compose, prometheus, backup, observability, analytics, event-store]

# Dependency graph
requires:
  - phase: none
    provides: "first plan of the v4.0 Activity Register milestone"
provides:
  - "Live single-host ClickHouse 26.3.12.3 service (animeenigma-clickhouse) on the shared docker compose project"
  - "Native ClickHouse Prometheus self-metrics on :9363, scraped by the 'clickhouse' Prometheus job"
  - "clickhouse-backup 2.7.0 sidecar (animeenigma-clickhouse-backup) with a verified create/list/restore procedure"
  - "Pinned, reproducible image tags for 01-02 and 01-03 to build on"
  - "127.0.0.1-only port bindings + dedicated CH user/password (no empty-password default)"
affects: [01-02-eventstore-go-impl, 01-03-dual-write-migration, analytics, grafana, prometheus]

# Tech tracking
tech-stack:
  added:
    - "clickhouse/clickhouse-server:26.3.12.3 (26.3 stable line, user-confirmed)"
    - "altinity/clickhouse-backup:2.7.0"
  patterns:
    - "Containerized CH must bind listen_host 0.0.0.0/:: via config.d (stock image is loopback-only); host exposure stays locked by compose 127.0.0.1: port maps"
    - "clickhouse-backup sidecar pattern: sleep infinity entrypoint, driven via docker exec, shares the CH data volume, authenticates with CLICKHOUSE_USERNAME/PASSWORD"
    - "Backup dir lives INSIDE the data volume (hardlink requires same filesystem)"

key-files:
  created:
    - "docker/clickhouse/config.d/prometheus.xml"
    - "docker/clickhouse/config.d/listen.xml"
    - "docker/clickhouse/BACKUP-RESTORE.md"
  modified:
    - "docker/docker-compose.yml"
    - "docker/prometheus/prometheus.yml"
    - "docker/.env.example"

key-decisions:
  - "Pinned clickhouse-server:26.3.12.3 (user chose the 26.3 stable line, explicitly declined the 25.8 LTS line) and clickhouse-backup:2.7.0 — exact patch tags, never latest/lts (T-01-03)"
  - "Backups persist inside the shared clickhouse_data volume rather than a separate volume, because clickhouse-backup hardlinks shadow/->backup/ and cross-device links fail"
  - "CH binds 0.0.0.0 internally; host-side security is enforced solely by the compose 127.0.0.1: port bindings (T-01-01)"

patterns-established:
  - "Stateful 3rd-party data store in compose mirrors the tempo block: pinned image, named volume, :ro config.d mount, wget-spider healthcheck, 127.0.0.1-bound ports"
  - "Backup/restore proof-of-procedure dry-run: backup a sentinel table, then restore --schema into a scratch DB via --restore-database-mapping to avoid Atomic-engine UUID store-dir conflicts"

requirements-completed: [AR-STORE-01]

# Metrics
duration: ~25min
completed: 2026-06-04
---

# Phase 1 Plan 01: ClickHouse Foundation Summary

**Stood up a live, host-bound ClickHouse 26.3.12.3 instance with native Prometheus self-metrics on :9363 and a clickhouse-backup 2.7.0 sidecar whose backup/restore procedure was dry-run-verified end-to-end — satisfying AR-STORE-01 as the storage foundation for the v4 Activity Register.**

## Performance

- **Duration:** ~25 min (resumed from the Task-1 image-pin checkpoint)
- **Completed:** 2026-06-04
- **Tasks:** 3/3 (Task 1 checkpoint resolved by user; Tasks 2 & 3 auto)
- **Files created:** 3 · **Files modified:** 3

## Accomplishments
- Live `animeenigma-clickhouse` container (healthy) on the shared docker compose project, all ports (8123 / 9100->9000 / 9363) bound to 127.0.0.1 only.
- Prometheus `clickhouse` scrape target confirmed **UP**, pulling 8055 `ClickHouse_*` series including `ClickHouse_Info{version="26.3.12.3"}`.
- Backup `dryrun-2026-06-04` created + listed; schema restore dry-run into a scratch DB succeeded (`operation=restore_schema done`) — the AR-STORE-01 acceptance artifact, recorded in `BACKUP-RESTORE.md`.

## Task Commits

1. **Task 1: Confirm + pin the ClickHouse image tag** — checkpoint (no commit). User confirmed `clickhouse-server:26.3.12.3` (26.3 stable line, NOT 25.8 LTS) + `clickhouse-backup:2.7.0`; both verified to pull and report the expected version before pinning.
2. **Task 2: Add clickhouse + clickhouse-backup services, config.d, Prometheus scrape** — `fdf55f0c` (feat)
3. **Task 3: Bring ClickHouse up, verify scrape, dry-run backup/restore** — `adf921dc` (feat; includes 3 deviation fixes)

_Plan metadata commit follows this SUMMARY._

## Files Created/Modified
- `docker/docker-compose.yml` — added `clickhouse` + `clickhouse-backup` services and the `clickhouse_data` volume.
- `docker/clickhouse/config.d/prometheus.xml` — enables the native `<prometheus>` endpoint on :9363.
- `docker/clickhouse/config.d/listen.xml` — binds CH to 0.0.0.0/:: inside the container (deviation fix).
- `docker/prometheus/prometheus.yml` — added the `clickhouse` scrape job (`clickhouse:9363`).
- `docker/.env.example` — added `CLICKHOUSE_USER` / `CLICKHOUSE_PASSWORD`.
- `docker/clickhouse/BACKUP-RESTORE.md` — backup/restore runbook with the recorded dry-run result.

## Pinned Image Tags (for 01-02 / 01-03 / future phases)
| Image | Pinned tag |
|-------|-----------|
| `clickhouse/clickhouse-server` | **`26.3.12.3`** (26.3 stable line) |
| `altinity/clickhouse-backup` | **`2.7.0`** |

CH service DNS in-network: `clickhouse:8123` (HTTP) / `clickhouse:9000` (native) / `clickhouse:9363` (metrics). DB `analytics`, user `${CLICKHOUSE_USER:-analytics}`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ClickHouse only bound to loopback inside the container**
- **Found during:** Task 3 (live `:8123/ping` from host → connection reset; in-network peers unreachable)
- **Issue:** The stock image sets `<listen_host>127.0.0.1</listen_host>` / `::1`, so docker-proxy (forwarding to the container eth0 IP) and in-Docker-network peers (Prometheus, the future analytics store) could not connect.
- **Fix:** Added `docker/clickhouse/config.d/listen.xml` binding `0.0.0.0` + `::` with `listen_try`. Host-side exposure stays locked by the compose `127.0.0.1:` port maps (T-01-01 preserved).
- **Files modified:** `docker/clickhouse/config.d/listen.xml` (new)
- **Commit:** `adf921dc`

**2. [Rule 2 - Missing critical] Backup sidecar had no username → auth failed (code 516)**
- **Found during:** Task 3 (`clickhouse-backup create`/`list` hung; logs showed `default: Authentication failed`)
- **Issue:** The CH entrypoint creates `CLICKHOUSE_USER` *instead of* `default` (we run `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT`). The sidecar set only `CLICKHOUSE_PASSWORD`, so it tried the non-existent `default` user.
- **Fix:** Added `CLICKHOUSE_USERNAME: ${CLICKHOUSE_USER:-analytics}` to the sidecar.
- **Files modified:** `docker/docker-compose.yml`
- **Commit:** `adf921dc`

**3. [Rule 1 - Bug] Separate backups volume caused `invalid cross-device link`**
- **Found during:** Task 3 (`clickhouse-backup create` failed moving shadow→backup)
- **Issue:** The plan mounted `clickhouse_backups:/var/lib/clickhouse/backup` as a separate volume. `clickhouse-backup` hardlinks parts from `shadow/` into `backup/`, and hardlinks cannot cross filesystems (separate volume = separate device).
- **Fix:** Removed the separate `clickhouse_backups` volume; backups now live under `/var/lib/clickhouse/backup` inside the shared `clickhouse_data` volume. Dropped the now-unused volume declaration.
- **Files modified:** `docker/docker-compose.yml`
- **Commit:** `adf921dc`

> All three are correctness fixes within the plan's own scope (the new CH service). No architectural (Rule 4) decisions were required. The plan's volume/credential/listen assumptions were refined to what `clickhouse-backup` and the containerized CH image actually require.

## Authentication Gates
None.

## Verification Evidence
- `docker compose config` validates.
- `docker compose ps` → `animeenigma-clickhouse` **healthy**.
- `curl http://127.0.0.1:8123/ping` → `Ok.`
- `curl http://127.0.0.1:9363/metrics` → 8055 `ClickHouse_*` series, `version="26.3.12.3"`.
- Prometheus `clickhouse` target health = **up**.
- `clickhouse-backup list` → `dryrun-2026-06-04 ... local ... regular`.
- `restore --schema --restore-database-mapping analytics:restore_scratch` → `done, operation=restore_schema`; restored DDL matched; scratch DB dropped.
- All Task verification gates returned `COMPOSE_OK`/`CONFIG_OK` and `AR_STORE_01_OK`.

## Known Stubs
None. No placeholder/empty-data stubs introduced. The `analytics` DB intentionally ships with no tables — the real event schema lands in plans 01-02 / 01-03. The sentinel table used for the backup dry-run was a transient verification artifact and was removed.

## Notes for 01-02 / 01-03
- Do NOT add `depends_on: clickhouse` to the `analytics` service yet — that wiring belongs to 01-03 once the Go store is implemented.
- A separate `clickhouse_backups` volume is intentionally absent; if off-host backup replication is added later, use `clickhouse-backup`'s remote upload config, not a second local volume over `backup/`.

## Self-Check: PASSED

All 7 created/modified files exist on disk; both task commits (`fdf55f0c`, `adf921dc`) are present in git history.
