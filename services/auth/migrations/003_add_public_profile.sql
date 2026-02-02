-- +migrate Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_id VARCHAR(32) UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_statuses TEXT[] DEFAULT '{watching,completed,plan_to_watch}';

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_public_id ON users(public_id) WHERE deleted_at IS NULL;

-- Generate public_id for existing users
UPDATE users SET public_id = 'user' || FLOOR(RANDOM() * 9000000 + 1000000)::INT
WHERE public_id IS NULL;

-- Make public_id NOT NULL after populating
ALTER TABLE users ALTER COLUMN public_id SET NOT NULL;

-- +migrate Down
ALTER TABLE users ALTER COLUMN public_id DROP NOT NULL;
DROP INDEX IF EXISTS idx_users_public_id;
ALTER TABLE users DROP COLUMN IF EXISTS public_statuses;
ALTER TABLE users DROP COLUMN IF EXISTS public_id;
