ALTER TABLE anime ADD COLUMN IF NOT EXISTS next_episode_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE anime ADD COLUMN IF NOT EXISTS aired_on DATE;

CREATE INDEX IF NOT EXISTS idx_anime_next_episode ON anime(next_episode_at) WHERE next_episode_at IS NOT NULL AND status = 'ongoing';
