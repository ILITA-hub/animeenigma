DROP INDEX IF EXISTS idx_anime_next_episode;
ALTER TABLE anime DROP COLUMN IF EXISTS next_episode_at;
ALTER TABLE anime DROP COLUMN IF EXISTS aired_on;
