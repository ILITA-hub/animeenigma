-- +migrate Up
ALTER TABLE anime ADD COLUMN IF NOT EXISTS episodes_aired INTEGER DEFAULT 0;

-- +migrate Down
ALTER TABLE anime DROP COLUMN IF EXISTS episodes_aired;
