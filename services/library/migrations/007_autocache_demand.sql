-- Phase 08 (workstream auto-torrent-population / v4.1): autocache_demand intake table
--
-- SERVE-03 backfill-demand sink. When the ae serve path MISSES (the requested
-- RAW episode is absent from the pool), the resolver fires a backfill demand so
-- Phase 9's Planner can later download it. This durable table (preferred over an
-- in-memory queue) lets the Planner drain wanted episodes ACROSS restarts.
--
-- Schema is deliberately minimal: one row per wanted (mal_id, episode). The
-- composite PRIMARY KEY (mal_id, episode) is the dedup key — concurrent demand
-- for the same episode collapses to a single row, and Record() refreshes
-- requested_at via ON CONFLICT DO UPDATE so the row always reflects most-recent
-- want. mal_id == shikimori_id (CONTEXT line 42) and is an internal short
-- identifier, never raw user input.
--
-- D (locked): the autocache_demand_reason enum carries 'next_ep' AND 'backfill'
-- so the schema reserves both, but Phase 8 ONLY ever writes 'backfill' —
-- 'next_ep' is reserved for Phase 9's next-episode predictor, mirroring how
-- migration 005 reserved the episode_track sub/dub values.
--
-- Idempotent (re-running across restarts must be a no-op): the enum CREATE is
-- wrapped in a DO $$ ... EXCEPTION WHEN duplicate_object block, and the table is
-- CREATE TABLE IF NOT EXISTS with no seed rows. The Go domain model in
-- internal/domain/autocache_demand.go mirrors this schema 1:1; GORM AutoMigrate
-- is NOT used (it cannot reproduce the enum type).

DO $$
BEGIN
    CREATE TYPE autocache_demand_reason AS ENUM ('next_ep', 'backfill');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS autocache_demand (
    mal_id       TEXT                    NOT NULL,   -- == shikimori_id (CONTEXT line 42)
    episode      INT                     NOT NULL,
    reason       autocache_demand_reason NOT NULL DEFAULT 'backfill',
    requested_at TIMESTAMPTZ             NOT NULL DEFAULT now(),
    PRIMARY KEY (mal_id, episode)                    -- dedup key: one row per wanted episode
);
