#!/usr/bin/env bash
# Seed a sample new_episode notification for the permanent ui_audit_bot
# test user via the internal producer endpoint.
#
# Idempotent — re-running upserts on (user_id, dedupe_key) per the partial
# uk_user_dedupe unique index. The same script can therefore be used as a
# smoke test for the UPSERT semantics (run twice, count stays at 1).
#
# Reference:
#   - .planning/workstreams/notifications/phases/01-notifications-foundation/PLAN.md
#   - docs/superpowers/specs/2026-05-11-notifications-engine-design.md
#
# Usage:
#   ./scripts/seed-notification-for-ui-audit-user.sh
#
# Requires:
#   - docker compose stack is running
#   - notifications service is healthy on 8090
#   - ui_audit_bot user exists (run ./scripts/seed-ui-audit-user.sh first)

set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE="docker compose -f docker/docker-compose.yml"

# Resolve ui_audit_bot's UUID from the shared `animeenigma` DB. Strip
# whitespace + trailing newline so the resulting variable is a clean UUID.
USER_ID=$($COMPOSE exec -T postgres psql -U postgres -d animeenigma -tAc \
    "SELECT id FROM users WHERE username = 'ui_audit_bot'" | tr -d '[:space:]')

if [ -z "$USER_ID" ]; then
    echo "ERROR: ui_audit_bot user does not exist." >&2
    echo "       Run ./scripts/seed-ui-audit-user.sh first." >&2
    exit 1
fi

echo "Resolved ui_audit_bot user_id: $USER_ID"

# Build the request body. The shikimori_id 57466 = Frieren: Beyond Journey's
# End (popular ongoing anime); the seed-anime-uuid is a stable fake UUID so
# the dedupe key stays stable across runs.
read -r -d '' BODY <<EOF || true
{
  "user_id": "${USER_ID}",
  "type": "new_episode",
  "dedupe_key": "new_episode:seed-anime-uuid:animelib:ru:dub:9999",
  "payload": {
    "anime_id": "seed-anime-uuid",
    "shikimori_id": "57466",
    "anime_title": "Frieren (seed)",
    "anime_poster_url": "https://shikimori.one/system/animes/original/57466.jpg",
    "first_unwatched_episode": 14,
    "latest_available_episode": 16,
    "player": "animelib",
    "language": "ru",
    "watch_type": "dub",
    "translation_id": "9999",
    "translation_title": "AniLibria",
    "watch_url": "/anime/seed-anime-uuid/watch?player=animelib&episode=14&translation=9999"
  }
}
EOF

# POST to the internal producer endpoint from INSIDE the Docker network
# (the gateway never proxies /internal/*; that's the entire point of the
# D-05 security model). `exec -T` keeps wget output streaming back to us.
echo "POST http://notifications:8090/internal/notifications (from inside docker network) ..."
RESPONSE=$($COMPOSE exec -T notifications wget -qO- \
    --post-data="$BODY" \
    --header='Content-Type: application/json' \
    http://localhost:8090/internal/notifications)

echo "Response: $RESPONSE"
echo ""
echo "Seeded notification for ui_audit_bot."
echo "Re-running this script will UPSERT (no duplicate row)."
echo ""
echo "Verify via gateway:"
echo "  curl -s -H \"Authorization: Bearer \$UI_AUDIT_API_KEY\" http://localhost:8000/api/notifications | jq"
