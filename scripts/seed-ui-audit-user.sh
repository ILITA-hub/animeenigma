#!/usr/bin/env bash
# Seed the permanent ui_audit_bot test account.
# Idempotent — safe to re-run. See:
#   /root/.claude/projects/-data-animeenigma/memory/project_test_user_pattern.md
#   /root/.claude/plans/kind-wibbling-pelican.md
#
# Usage: ./scripts/seed-ui-audit-user.sh
# Run from project root.

set -euo pipefail

cd "$(dirname "$0")/.."

DC="docker compose -f docker/docker-compose.yml exec -T postgres"
TEST_USERNAME="ui_audit_bot"
TEST_PUBLIC_ID="ui-audit-bot"

# ============================================================================
# Step 0b: Create test user (idempotent)
# ============================================================================
$DC psql -U postgres -d animeenigma -c "
  INSERT INTO users (username, public_id, password_hash, public_statuses, avatar, role)
  VALUES (
    '${TEST_USERNAME}',
    '${TEST_PUBLIC_ID}',
    '\$2a\$10\$DISABLED.DISABLED.DISABLED.DISABLED.DISABLED.DISABLED..',
    ARRAY['watching','completed','plan_to_watch']::text[],
    '',
    'user'
  )
  ON CONFLICT (username) DO NOTHING;
"
echo "--- Step 0b done: user created (or already existed) ---"

# ============================================================================
# Step 0c: Mint API key (refuses to overwrite)
# ============================================================================
EXISTING=$($DC psql -U postgres -d animeenigma -tAc \
  "SELECT api_key_hash FROM users WHERE username = '${TEST_USERNAME}' AND api_key_hash IS NOT NULL;" \
  | tr -d '[:space:]')

if [ -n "$EXISTING" ]; then
  echo "!!! api_key_hash already set for ${TEST_USERNAME}."
  echo "!!! Reusing existing key (not shown — must be retrieved from your records or docker/.env)."
  echo "!!! To rotate, first NULL the key:"
  echo "!!!   $DC psql -U postgres -d animeenigma -c \"UPDATE users SET api_key_hash = NULL WHERE username = '${TEST_USERNAME}';\""
  echo "!!! Then re-run this script."
else
  RAW_KEY="ak_$(openssl rand -hex 32)"
  HASH=$(echo -n "$RAW_KEY" | sha256sum | awk '{print $1}')
  $DC psql -U postgres -d animeenigma \
    -c "UPDATE users SET api_key_hash = '${HASH}' WHERE username = '${TEST_USERNAME}';" \
    > /dev/null
  echo ""
  echo "=========================================="
  echo "API KEY (save to docker/.env as UI_AUDIT_API_KEY — shown only once):"
  echo "${RAW_KEY}"
  echo "=========================================="
  echo ""
fi

# ============================================================================
# Step 0d: Seed sample data (idempotent via count check)
# ============================================================================
USER_ID=$($DC psql -U postgres -d animeenigma -tAc \
  "SELECT id FROM users WHERE username = '${TEST_USERNAME}';" \
  | tr -d '[:space:]')

echo "Seeding for user_id=${USER_ID}"

WH_COUNT=$($DC psql -U postgres -d animeenigma -tAc \
  "SELECT COUNT(*) FROM watch_history WHERE user_id = '${USER_ID}'::uuid;" \
  | tr -d '[:space:]')

if [ "${WH_COUNT}" -gt 0 ]; then
  echo "--- watch_history already seeded (${WH_COUNT} rows). Skipping seed step. ---"
else
  $DC psql -U postgres -d animeenigma <<SQL
WITH picks AS (
  SELECT id, ROW_NUMBER() OVER (ORDER BY score DESC NULLS LAST) AS rn
  FROM animes
  WHERE deleted_at IS NULL AND poster_url IS NOT NULL AND poster_url != ''
  LIMIT 8
)
INSERT INTO anime_list (user_id, anime_id, status, score, episodes)
SELECT
  '${USER_ID}'::uuid,
  id,
  CASE rn
    WHEN 1 THEN 'watching'
    WHEN 2 THEN 'watching'
    WHEN 3 THEN 'watching'
    WHEN 4 THEN 'completed'
    WHEN 5 THEN 'completed'
    WHEN 6 THEN 'plan_to_watch'
    WHEN 7 THEN 'plan_to_watch'
    WHEN 8 THEN 'dropped'
  END,
  CASE rn WHEN 4 THEN 9 WHEN 5 THEN 8 ELSE 0 END,
  CASE rn WHEN 4 THEN 12 WHEN 5 THEN 24 WHEN 8 THEN 3 ELSE 0 END
FROM picks
ON CONFLICT (user_id, anime_id) DO NOTHING;

WITH picks AS (
  SELECT anime_id, ROW_NUMBER() OVER (ORDER BY anime_id) AS rn
  FROM anime_list
  WHERE user_id = '${USER_ID}'::uuid AND status = 'watching'
  LIMIT 4
)
INSERT INTO watch_history (user_id, anime_id, episode_number, player, language, watch_type, duration_watched, watched_at)
SELECT
  '${USER_ID}'::uuid,
  anime_id,
  rn::bigint,
  'kodik',
  'ru',
  'sub',
  (600 + rn::int * 30)::bigint,
  NOW() - (rn::int || ' hours')::interval
FROM picks;

-- UX-08 (Phase 3): watch_progress rows are backfilled UNCONDITIONALLY in
-- Step 0e below — kept out of this block so re-running the script on an
-- existing user still refreshes /api/users/progress.

INSERT INTO theme_ratings (user_id, theme_id, score)
SELECT '${USER_ID}'::uuid, id, (7 + (RANDOM() * 3)::int)::bigint
FROM anime_themes
WHERE deleted_at IS NULL
ORDER BY id
LIMIT 3
ON CONFLICT (user_id, theme_id) DO NOTHING;
SQL
  echo "--- Step 0d done: seeded ---"
fi

# ============================================================================
# Step 0e: Backfill watch_progress for existing seeded users (UA-111 / UX-08)
#
# Runs UNCONDITIONALLY (idempotent via ON CONFLICT). The Step 0d block only
# fires when watch_history is empty — so for users seeded BEFORE the Phase 3
# fix landed, the new watch_progress rows wouldn't be added without this
# follow-up. Re-running the seed script on an existing user will populate the
# missing rows and refresh /api/users/progress responses.
# ============================================================================
$DC psql -U postgres -d animeenigma <<SQL
WITH picks AS (
  SELECT wh.anime_id, MAX(wh.episode_number) AS ep_max
  FROM watch_history wh
  WHERE wh.user_id = '${USER_ID}'::uuid
  GROUP BY wh.anime_id
)
INSERT INTO watch_progress (user_id, anime_id, episode_number, progress, duration, completed, last_watched_at)
SELECT
  '${USER_ID}'::uuid,
  picks.anime_id,
  gs.ep::bigint,
  (600 + gs.ep * 30)::bigint,
  1440::bigint,
  TRUE,
  NOW() - (gs.ep || ' hours')::interval
FROM picks
CROSS JOIN LATERAL generate_series(1, picks.ep_max::int) AS gs(ep)
ON CONFLICT (user_id, anime_id, episode_number) DO NOTHING;
SQL
echo "--- Step 0e done: watch_progress backfilled ---"

# ============================================================================
# Verification
# ============================================================================
echo ""
echo "=== Verification counts ==="
$DC psql -U postgres -d animeenigma -c "
  SELECT
    (SELECT COUNT(*) FROM anime_list WHERE user_id = '${USER_ID}') AS list_entries,
    (SELECT COUNT(*) FROM watch_history WHERE user_id = '${USER_ID}') AS history_entries,
    (SELECT COUNT(*) FROM watch_progress WHERE user_id = '${USER_ID}') AS progress_entries,
    (SELECT COUNT(*) FROM theme_ratings WHERE user_id = '${USER_ID}') AS theme_ratings;
"
