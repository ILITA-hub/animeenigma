-- Phase 07 (workstream raw-jp / autocache pool): autocache_config
--
-- Singleton configuration table holding the live-editable autocache
-- tunables (spec §3.5 defaults). Exactly ONE row exists — the
-- singleton invariant is enforced by a PK fixed at 1 plus a CHECK
-- constraint, so any attempt to insert a second row fails. An admin
-- edits the values via PATCH /api/library/autocache/config (no
-- redeploy needed); the future downloader/evictor (Phases 8-10) reads
-- the master `enabled` switch and the freshness/budget windows here.
--
-- Idempotent: CREATE TABLE IF NOT EXISTS + a seed via
-- INSERT ... ON CONFLICT (id) DO NOTHING so re-running across restarts
-- is a no-op (the NOT NULL DEFAULTs fill the seeded row with §3.5
-- values). The Go domain model in internal/domain/autocache_config.go
-- mirrors this schema 1:1; GORM AutoMigrate is NOT used.

CREATE TABLE IF NOT EXISTS autocache_config (
    id                       INT         PRIMARY KEY DEFAULT 1,
    enabled                  BOOLEAN     NOT NULL DEFAULT true,
    budget_bytes             BIGINT      NOT NULL DEFAULT 107374182400,
    auto_fresh_download_days INT         NOT NULL DEFAULT 10,
    auto_fresh_fetch_days    INT         NOT NULL DEFAULT 3,
    admin_fresh_days         INT         NOT NULL DEFAULT 30,
    active_watcher_days      INT         NOT NULL DEFAULT 30,
    quality_cap              INT         NOT NULL DEFAULT 1080,
    min_seeders              INT         NOT NULL DEFAULT 3,
    sweep_interval_min       INT         NOT NULL DEFAULT 20,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT autocache_config_singleton CHECK (id = 1)
);

-- Seed the one-and-only row. The NOT NULL DEFAULTs above supply the
-- §3.5 values; ON CONFLICT keeps the migration idempotent.
INSERT INTO autocache_config (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
