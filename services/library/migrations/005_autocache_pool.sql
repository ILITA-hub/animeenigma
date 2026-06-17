-- Phase 07 (workstream auto-torrent-population / v4.1): autocache pool foundation
--
-- POOL-01 (unified layout) + POOL-03 (per-row accounting ledger). This migration
-- introduces two Postgres enum types and five new columns on library_episodes so
-- every first-party RAW object can be metered under one budget. The Go domain model
-- in internal/domain/episode.go mirrors this schema 1:1. GORM AutoMigrate is NOT
-- used (it cannot reproduce enum types) — same convention as job_source/job_status
-- in 001_library_jobs.sql.
--
-- Idempotency contract (re-running across restarts must be a no-op):
--   * enum CREATE wrapped in DO $$ ... EXCEPTION WHEN duplicate_object THEN NULL
--   * column adds use ADD COLUMN IF NOT EXISTS
--   * the backfill is guarded by WHERE downloaded_at IS NULL AND source = 'admin'
--     so re-runs touch nothing AND can never rewrite a future autocache row
--
-- D2 (locked): the episode_track enum carries 'sub' / 'dub' so the schema reserves
-- them, but they are NEVER written in v1 — RAW only.
-- size_bytes already exists (added by 002_library_episodes.sql, POOL-03) — NOT re-added.

-- episode_source: distinguishes admin uploads from autocache-downloaded content.
-- D6 — the path is uniform (aeProvider/.../RAW/...); THIS column is the discriminator.
DO $$
BEGIN
    CREATE TYPE episode_source AS ENUM ('admin', 'autocache');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- episode_track: RAW only in v1. 'sub' / 'dub' are reserved (D2) — present in the
-- enum so future tracks need no schema change, but never written in this milestone.
DO $$
BEGIN
    CREATE TYPE episode_track AS ENUM ('raw', 'sub', 'dub');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Five new accounting-ledger columns. ADD COLUMN IF NOT EXISTS is the column-add
-- analog of 004's idempotent ADD VALUE IF NOT EXISTS. downloaded_at stays nullable
-- so the ADD on a populated table succeeds; the backfill below fills existing rows.
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS source        episode_source NOT NULL DEFAULT 'admin';
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS track         episode_track  NOT NULL DEFAULT 'raw';
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS downloaded_at TIMESTAMPTZ;
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS last_fetch_at TIMESTAMPTZ;          -- nullable: the fetch signal, written in Phase 8
ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS fetch_count   BIGINT NOT NULL DEFAULT 0;

-- One-time backfill: every pre-existing row is admin-sourced RAW content whose
-- download date is its creation date. The ADD COLUMN ... NOT NULL DEFAULT above
-- already populated source='admin'/track='raw' on every pre-existing row, so the
-- only real work here is anchoring downloaded_at. Scope to source='admin' so a
-- re-run on every boot can NEVER touch a future autocache row: once Phase 8 starts
-- inserting autocache content, a NULL-downloaded_at autocache row must NOT be
-- force-rewritten to admin (D6 — source is the single admin-vs-autocache truth).
UPDATE library_episodes
   SET downloaded_at = created_at
 WHERE downloaded_at IS NULL
   AND source = 'admin';
