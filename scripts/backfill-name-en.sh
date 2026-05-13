#!/usr/bin/env bash
# Backfill anime.name_en from Shikimori's `english` GraphQL field.
#
# One-off after the Anime schema gained name_en (2026-05-13). Iterates over
# every row missing name_en, batches by 50 (the Shikimori `animes(ids:...)`
# limit), fetches the English title, and UPDATEs the row. Idempotent — re-runs
# only touch rows still missing an English title.
#
# Runs inside the catalog container (it can reach https://shikimori.one and
# already has psql credentials available via env). Rate-limits to ~2 req/sec
# (one batch every 500ms) to stay well under Shikimori's 5 RPS cap.

set -euo pipefail

DB_HOST="${DB_HOST:-postgres}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-animeenigma}"
DB_PASSWORD="${DB_PASSWORD:-postgres}"
SHIKIMORI_URL="${SHIKIMORI_URL:-https://shikimori.io/api/graphql}"
SHIKIMORI_UA="${SHIKIMORI_UA:-AnimeEnigma/1.0}"
BATCH_SIZE=50
SLEEP_MS=500

export PGPASSWORD="$DB_PASSWORD"

total=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -tA -c \
  "SELECT COUNT(*) FROM animes WHERE shikimori_id IS NOT NULL AND shikimori_id != '' AND (name_en IS NULL OR name_en = '') AND deleted_at IS NULL;")
echo "Animes needing backfill: $total"
[ "$total" = "0" ] && { echo "Nothing to do."; exit 0; }

updated_total=0
empty_total=0
batch_n=0

while :; do
  # Pull a batch of shikimori_ids that still need backfill.
  ids=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -tA -c \
    "SELECT shikimori_id FROM animes WHERE shikimori_id IS NOT NULL AND shikimori_id != '' AND (name_en IS NULL OR name_en = '') AND deleted_at IS NULL ORDER BY shikimori_id LIMIT $BATCH_SIZE;")

  [ -z "$ids" ] && break

  batch_n=$((batch_n + 1))
  csv=$(echo "$ids" | paste -sd, -)

  # Shikimori GraphQL: animes(ids: "...", limit: 50) { id english }
  query=$(printf '{"query":"{ animes(ids: \\"%s\\", limit: %d) { id english } }"}' "$csv" "$BATCH_SIZE")

  resp=$(curl -sSL -X POST "$SHIKIMORI_URL" \
    -H "Content-Type: application/json" \
    -H "User-Agent: $SHIKIMORI_UA" \
    --max-time 15 \
    -d "$query" 2>&1) || { echo "Batch $batch_n: HTTP error, skipping"; sleep 1; continue; }

  # Build UPDATE statements per id whose english is non-empty. SQL-escape
  # the title by doubling single quotes.
  updates=$(echo "$resp" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    animes = d.get('data', {}).get('animes', [])
except Exception as e:
    print('--ERR', file=sys.stderr); sys.exit(1)
for a in animes:
    eng = (a.get('english') or '').strip()
    sid = a.get('id', '')
    if not eng or not sid: continue
    esc = eng.replace(\"'\", \"''\")
    print(f\"UPDATE animes SET name_en = '{esc}' WHERE shikimori_id = '{sid}';\")
")

  if [ -n "$updates" ]; then
    n=$(echo "$updates" | grep -c '^UPDATE')
    echo "$updates" | psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -q
    updated_total=$((updated_total + n))
    empty_in_batch=$((BATCH_SIZE - n))
  else
    empty_in_batch=$BATCH_SIZE
  fi

  # Mark animes whose Shikimori response had no `english` value as
  # backfill-attempted by writing a sentinel so the WHERE clause skips them
  # next pass. Use a single space — short, non-empty, easy to detect later.
  if [ "$empty_in_batch" -gt 0 ]; then
    sentinel_updates=$(echo "$resp" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    animes = d.get('data', {}).get('animes', [])
except: sys.exit(0)
for a in animes:
    if (a.get('english') or '').strip(): continue
    sid = a.get('id', '')
    if not sid: continue
    print(f\"UPDATE animes SET name_en = ' ' WHERE shikimori_id = '{sid}' AND (name_en IS NULL OR name_en = '');\")
")
    if [ -n "$sentinel_updates" ]; then
      echo "$sentinel_updates" | psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -q
      empty_total=$((empty_total + empty_in_batch))
    fi
  fi

  echo "Batch $batch_n: updated=$updated_total empty=$empty_total"

  # Politeness
  python3 -c "import time; time.sleep($SLEEP_MS/1000)" 2>/dev/null || sleep 1
done

echo ""
echo "Done. Filled name_en for $updated_total anime; $empty_total had no Shikimori english title (sentinel-marked)."
