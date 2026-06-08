# Phase 06 — Deferred Items

## Out-of-scope pre-existing failures (NOT caused by this phase)

- **`animepahe-resolver` container Up 2 weeks (unhealthy)** — started 2026-05-19, long
  before this plan. A `docker compose up -d` during the cutover refused to (re)start a
  sibling that `depends_on` it, but did not disturb any already-running service. Unrelated
  to the observability consolidation. Pre-existing.
- **`meilisearch` / `notifications` containers unhealthy** — pre-existing long-running
  conditions, untouched by this phase.

## Optional post-cutover cleanup (06-03, Open Q2)

- **MinIO `tempo` bucket** — DEFERRED. `mc` is not available inside the minio image
  without alias setup, so reclaiming the orphaned bucket is not trivial. It is a harmless
  orphan (Tempo no longer writes to it). The bulk of reclaimable space (the `docker_tempo_data`
  and `docker_loki_data` named volumes) WAS reclaimed during 06-03 Task 4.
