-- Phase 04 (workstream raw-jp / v0.2): library_episodes schema
--
-- One row per successfully-encoded episode: shikimori_id + episode_number
-- compose a unique key; minio_path is the bucket-relative prefix (ends
-- with `/`) under which playlist.m3u8 + segment_NNN.ts files live.
--
-- Idempotent — re-running this migration is a no-op:
--   * CREATE TABLE IF NOT EXISTS
--   * UNIQUE constraint wrapped in DO $$ ... EXCEPTION WHEN duplicate_object
--   * CREATE INDEX IF NOT EXISTS
--
-- The Go domain model in internal/domain/episode.go mirrors this schema
-- 1:1.

CREATE TABLE IF NOT EXISTS library_episodes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shikimori_id    TEXT         NOT NULL,
    episode_number  INT          NOT NULL,
    job_id          UUID         REFERENCES library_jobs(id),
    minio_path      TEXT         NOT NULL,
    duration_sec    INT,
    size_bytes      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- UNIQUE(shikimori_id, episode_number) — enforced via a named
-- constraint inside a DO $$ EXCEPTION block so this migration is
-- safely re-applicable.
DO $$
BEGIN
    ALTER TABLE library_episodes
        ADD CONSTRAINT library_episodes_shikimori_ep_uniq
        UNIQUE (shikimori_id, episode_number);
EXCEPTION
    WHEN duplicate_object THEN NULL;
    WHEN duplicate_table THEN NULL;
END $$;

-- Lookup index for GET /api/library/episodes/{shikimori_id}/{episode}.
-- Covers the primary read path; the UNIQUE constraint above already
-- provides a btree but a named index makes intent explicit.
CREATE INDEX IF NOT EXISTS idx_library_episodes_shikimori
    ON library_episodes (shikimori_id, episode_number);
