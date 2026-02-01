-- +migrate Up
ALTER TABLE anime ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_anime_hidden ON anime(hidden) WHERE hidden = true;

-- +migrate Down
DROP INDEX IF EXISTS idx_anime_hidden;
ALTER TABLE anime DROP COLUMN IF EXISTS hidden;
