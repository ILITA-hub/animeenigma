-- Admin users management (spec 2026-07-24).
--
-- 1. Add the Telegram display-identity columns (persisted on every Telegram
--    login; surfaced + searchable in the /admin/users page).
-- 2. Align the users table's legacy unique-constraint names with the names
--    GORM AutoMigrate expects. Migrations 000001-000003 created inline
--    `UNIQUE` constraints, which Postgres auto-named users_username_key /
--    users_public_id_key / users_telegram_id_key. The domain.User struct's
--    `uniqueIndex` tags make GORM expect uni_users_username /
--    uni_users_public_id / uni_users_telegram_id, so the next time AutoMigrate
--    has to ALTER this table (e.g. to add the columns above) it issues
--    `DROP CONSTRAINT uni_users_username`, which fails with 42704 and crashes
--    auth on startup (main.go log.Fatalw). Renaming removes the drift
--    permanently. The renames are guarded, so this is a no-op on databases
--    whose users table was first created by GORM AutoMigrate (already uni_*)
--    rather than by these SQL migrations.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_username_key' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT users_username_key TO uni_users_username;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_public_id_key' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT users_public_id_key TO uni_users_public_id;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_telegram_id_key' AND conrelid = 'users'::regclass) THEN
        ALTER TABLE users RENAME CONSTRAINT users_telegram_id_key TO uni_users_telegram_id;
    END IF;
END $$;

ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_username VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_first_name VARCHAR(128);
