-- 017_episode_storage.sql — storage-backend column + dual-presence unique key.
--
-- Adds `storage TEXT` to library_episodes (default 'minio' — every row that
-- exists today lives on local self-hosted MinIO) and to library_jobs
-- (default '' — the REQUESTED backend override at job creation; '' = the
-- class default. The resolved actual value is written back after upload by
-- a later task, mirroring how library_episodes.audio_lang/quality (016)
-- record the encoder's actual output rather than a request).
--
-- Swaps the library_episodes unique key from (shikimori_id, episode_number)
-- to (shikimori_id, episode_number, storage): the SAME episode may now have
-- one row per backend (a local MinIO copy AND an external S3 copy), which is
-- the whole point of the storage-service milestone. The old constraint is
-- dropped first (IF EXISTS — a no-op if it was already swapped by a prior
-- run of this migration); the new one is added inside a DO $$ ... EXCEPTION
-- WHEN duplicate_object block, matching every other constraint-add in this
-- package (see 002_library_episodes.sql) so re-running this migration across
-- restarts stays a safe no-op, per the package doc's stated invariant.
--
-- Idempotent — ADD COLUMN IF NOT EXISTS + DROP CONSTRAINT IF EXISTS +
-- DO $$ ... EXCEPTION guard. Must follow 001 (created library_jobs) and 002
-- (created library_episodes).

ALTER TABLE library_episodes ADD COLUMN IF NOT EXISTS storage TEXT NOT NULL DEFAULT 'minio';
ALTER TABLE library_jobs     ADD COLUMN IF NOT EXISTS storage TEXT NOT NULL DEFAULT '';

ALTER TABLE library_episodes DROP CONSTRAINT IF EXISTS library_episodes_shikimori_ep_uniq;

DO $$
BEGIN
    ALTER TABLE library_episodes
        ADD CONSTRAINT library_episodes_shikimori_ep_storage_uniq
        UNIQUE (shikimori_id, episode_number, storage);
EXCEPTION
    WHEN duplicate_object THEN NULL;
    WHEN duplicate_table THEN NULL;
END $$;
