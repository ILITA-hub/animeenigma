-- Phase 03 (workstream raw-jp / v0.2): library_jobs schema
--
-- Source of truth for the job-queue enums + the partial index that
-- powers FOR UPDATE SKIP LOCKED scans. Wrapped in DO $$ ... $$
-- blocks so this migration is idempotent: running it twice in a
-- row against the same database must succeed without error.
--
-- The Go domain model in internal/domain/job.go mirrors this schema
-- 1:1. GORM AutoMigrate is NOT used here because we need control over
-- enum types and the partial index expression, which AutoMigrate
-- cannot reproduce.

-- job_source: which provider seeded the magnet.
DO $$
BEGIN
    CREATE TYPE job_source AS ENUM ('nyaa', 'animetosho', 'manual');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- job_status: state machine documented in 03-CONTEXT.md.
-- Phase 3 implements transitions through 'downloading' and stops at
-- the 'encoding' boundary. Phase 4 picks up at 'encoding' and drives
-- 'uploading' / 'done'.
DO $$
BEGIN
    CREATE TYPE job_status AS ENUM (
        'queued',
        'downloading',
        'encoding',
        'uploading',
        'done',
        'failed',
        'cancelled'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS library_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source        job_source   NOT NULL,
    magnet        TEXT         NOT NULL,
    title         TEXT         NOT NULL,
    uploader      TEXT,
    quality       TEXT,
    size_bytes    BIGINT       NOT NULL DEFAULT 0,
    shikimori_id  TEXT,
    status        job_status   NOT NULL DEFAULT 'queued',
    progress_pct  INT          NOT NULL DEFAULT 0,
    error_text    TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ
);

-- Partial index covering the queue-scan path: workers only ever
-- look at non-terminal rows ordered by created_at. Excluding
-- 'done' and 'cancelled' keeps the index small as the table grows.
CREATE INDEX IF NOT EXISTS idx_library_jobs_status
    ON library_jobs (status, created_at)
    WHERE status NOT IN ('done', 'cancelled');
