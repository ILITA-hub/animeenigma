#!/bin/bash
set -euo pipefail

# =============================================================================
# Migrate 3 PostgreSQL databases into single "animeenigma" database
# =============================================================================
#
# This script consolidates data from:
#   - animeenigma_auth    (users)
#   - animeenigma_catalog (animes, genres, anime_genres, videos, pinned_translations)
#   - animeenigma_player  (watch_progress, anime_list, watch_history, reviews)
#
# And scheduler tables already in animeenigma_catalog:
#   - mal_export_jobs, anime_load_tasks, mal_shikimori_mapping
#
# Into the single "animeenigma" database (already exists via POSTGRES_DB).
#
# Usage:
#   docker exec -i animeenigma-postgres bash < docker/postgres/migrate-to-single-db.sh
#
# Prerequisites:
#   - Stop services first: docker compose stop auth catalog player scheduler
#   - The "animeenigma" database must exist (created by POSTGRES_DB env var)

DB_USER="${POSTGRES_USER:-postgres}"
TARGET_DB="animeenigma"
DUMP_DIR="/tmp/db-migration"

# Color helpers
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()   { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check if source databases exist
db_exists() {
    psql -U "$DB_USER" -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$1'" 2>/dev/null | grep -q 1
}

mkdir -p "$DUMP_DIR"

echo "============================================="
echo "  Database Consolidation Migration"
echo "============================================="
echo ""

# Enable uuid-ossp in target database
log "Enabling uuid-ossp extension in $TARGET_DB..."
psql -U "$DB_USER" -d "$TARGET_DB" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' 2>/dev/null

# ─── Step 1: Export from source databases ────────────────────────────────────

log "Step 1: Exporting data from source databases..."

# Auth tables (full dump with schema + data)
if db_exists "animeenigma_auth"; then
    log "  Dumping animeenigma_auth..."
    pg_dump -U "$DB_USER" -d animeenigma_auth \
        --no-owner --no-privileges --no-comments \
        > "$DUMP_DIR/auth.sql" 2>/dev/null || true
    log "  Done: auth tables"
else
    warn "  animeenigma_auth does not exist, skipping"
    touch "$DUMP_DIR/auth.sql"
fi

# Catalog tables (full dump with schema + data)
if db_exists "animeenigma_catalog"; then
    log "  Dumping animeenigma_catalog..."
    pg_dump -U "$DB_USER" -d animeenigma_catalog \
        --no-owner --no-privileges --no-comments \
        > "$DUMP_DIR/catalog.sql" 2>/dev/null || true
    log "  Done: catalog tables"
else
    warn "  animeenigma_catalog does not exist, skipping"
    touch "$DUMP_DIR/catalog.sql"
fi

# Player tables (full dump with schema + data)
if db_exists "animeenigma_player"; then
    log "  Dumping animeenigma_player..."
    pg_dump -U "$DB_USER" -d animeenigma_player \
        --no-owner --no-privileges --no-comments \
        > "$DUMP_DIR/player.sql" 2>/dev/null || true
    log "  Done: player tables"
else
    warn "  animeenigma_player does not exist, skipping"
    touch "$DUMP_DIR/player.sql"
fi

# ─── Step 2: Import into target database ─────────────────────────────────────

log "Step 2: Importing data into $TARGET_DB..."
log "  Order: auth → catalog → player (for FK integrity)"

# Import auth first (users table, referenced by player tables)
if [ -s "$DUMP_DIR/auth.sql" ]; then
    log "  Importing auth data..."
    psql -U "$DB_USER" -d "$TARGET_DB" -f "$DUMP_DIR/auth.sql" 2>/dev/null || {
        warn "  Some auth records may have failed (duplicates?)"
    }
fi

# Import catalog second (animes table, referenced by player tables)
if [ -s "$DUMP_DIR/catalog.sql" ]; then
    log "  Importing catalog data..."
    psql -U "$DB_USER" -d "$TARGET_DB" -f "$DUMP_DIR/catalog.sql" 2>/dev/null || {
        warn "  Some catalog records may have failed (duplicates?)"
    }
fi

# Import player last (references both users and animes)
if [ -s "$DUMP_DIR/player.sql" ]; then
    log "  Importing player data..."
    psql -U "$DB_USER" -d "$TARGET_DB" -f "$DUMP_DIR/player.sql" 2>/dev/null || {
        warn "  Some player records may have failed (duplicates?)"
    }
fi

# ─── Step 3: Clean orphan records ────────────────────────────────────────────

log "Step 3: Cleaning orphan records..."

psql -U "$DB_USER" -d "$TARGET_DB" <<'SQL'
-- Delete player records referencing non-existent users
DELETE FROM watch_progress WHERE user_id NOT IN (SELECT id FROM users);
DELETE FROM anime_list WHERE user_id NOT IN (SELECT id FROM users);
DELETE FROM watch_history WHERE user_id NOT IN (SELECT id FROM users);
DELETE FROM reviews WHERE user_id NOT IN (SELECT id FROM users);

-- Delete player records referencing non-existent anime
DELETE FROM watch_progress WHERE anime_id NOT IN (SELECT id FROM animes);
DELETE FROM anime_list WHERE anime_id NOT IN (SELECT id FROM animes);
DELETE FROM watch_history WHERE anime_id NOT IN (SELECT id FROM animes);
DELETE FROM reviews WHERE anime_id NOT IN (SELECT id FROM animes);

-- Delete scheduler records referencing non-existent users
DELETE FROM mal_export_jobs WHERE user_id NOT IN (SELECT id FROM users);
DELETE FROM anime_load_tasks WHERE user_id NOT IN (SELECT id FROM users);

-- Nullify scheduler references to non-existent anime (keep history)
UPDATE anime_load_tasks SET resolved_anime_id = NULL
    WHERE resolved_anime_id IS NOT NULL
    AND resolved_anime_id NOT IN (SELECT id FROM animes);
UPDATE mal_shikimori_mapping SET anime_id = NULL
    WHERE anime_id IS NOT NULL
    AND anime_id NOT IN (SELECT id FROM animes);
SQL

log "  Orphan cleanup complete"

# ─── Step 4: Drop stale "anime" table (singular) if it exists ────────────────

log "Step 4: Dropping stale 'anime' table if it exists..."
psql -U "$DB_USER" -d "$TARGET_DB" -c "DROP TABLE IF EXISTS anime CASCADE;" 2>/dev/null || true

# ─── Step 5: Add FK constraints ──────────────────────────────────────────────

log "Step 5: Adding foreign key constraints..."

psql -U "$DB_USER" -d "$TARGET_DB" <<'SQL'
-- watch_progress
ALTER TABLE watch_progress
    ADD CONSTRAINT fk_watch_progress_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE watch_progress
    ADD CONSTRAINT fk_watch_progress_anime
    FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;

-- anime_list
ALTER TABLE anime_list
    ADD CONSTRAINT fk_anime_list_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE anime_list
    ADD CONSTRAINT fk_anime_list_anime
    FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;

-- watch_history
ALTER TABLE watch_history
    ADD CONSTRAINT fk_watch_history_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE watch_history
    ADD CONSTRAINT fk_watch_history_anime
    FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;

-- reviews
ALTER TABLE reviews
    ADD CONSTRAINT fk_reviews_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE reviews
    ADD CONSTRAINT fk_reviews_anime
    FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;

-- mal_export_jobs
ALTER TABLE mal_export_jobs
    ADD CONSTRAINT fk_mal_export_jobs_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- anime_load_tasks
ALTER TABLE anime_load_tasks
    ADD CONSTRAINT fk_anime_load_tasks_user
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE anime_load_tasks
    ADD CONSTRAINT fk_anime_load_tasks_anime
    FOREIGN KEY (resolved_anime_id) REFERENCES animes(id) ON DELETE SET NULL;

-- mal_shikimori_mapping
ALTER TABLE mal_shikimori_mapping
    ADD CONSTRAINT fk_mal_shikimori_mapping_anime
    FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE SET NULL;
SQL

log "  FK constraints added"

# ─── Step 6: Verify ──────────────────────────────────────────────────────────

log "Step 6: Verification..."

echo ""
log "Table row counts in $TARGET_DB:"
psql -U "$DB_USER" -d "$TARGET_DB" -c "
SELECT
    schemaname || '.' || relname AS table_name,
    n_live_tup AS row_count
FROM pg_stat_user_tables
ORDER BY relname;
"

echo ""
log "Foreign key constraints:"
psql -U "$DB_USER" -d "$TARGET_DB" -c "
SELECT
    conname AS constraint_name,
    conrelid::regclass AS table_name,
    confrelid::regclass AS references_table
FROM pg_constraint
WHERE contype = 'f'
ORDER BY conrelid::regclass::text, conname;
"

# Cleanup temp files
rm -rf "$DUMP_DIR"

echo ""
echo "============================================="
log "Migration completed successfully!"
echo "============================================="
echo ""
log "Next steps:"
echo "  1. Update service configs to use DB_NAME=animeenigma"
echo "  2. Redeploy services: make redeploy-auth && make redeploy-catalog && make redeploy-player && make redeploy-scheduler"
echo "  3. Verify: make health"
echo "  4. After verification, optionally drop old databases:"
echo "     psql -U postgres -c 'DROP DATABASE IF EXISTS animeenigma_auth;'"
echo "     psql -U postgres -c 'DROP DATABASE IF EXISTS animeenigma_catalog;'"
echo "     psql -U postgres -c 'DROP DATABASE IF EXISTS animeenigma_player;'"
