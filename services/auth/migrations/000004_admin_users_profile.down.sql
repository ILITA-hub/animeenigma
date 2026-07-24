ALTER TABLE users DROP COLUMN IF EXISTS telegram_first_name;
ALTER TABLE users DROP COLUMN IF EXISTS telegram_username;

-- Revert the constraint-name alignment (guarded — no-op if already reverted or
-- if the table used GORM names from the start).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uni_users_username' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT uni_users_username TO users_username_key;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uni_users_public_id' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT uni_users_public_id TO users_public_id_key;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uni_users_telegram_id' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT uni_users_telegram_id TO users_telegram_id_key;
    END IF;
END $$;
