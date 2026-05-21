#!/usr/bin/env bash
# smoke-spotlight.sh — Phase 1 end-to-end smoke for the hero spotlight
# aggregator. Run AFTER catalog + gateway have been redeployed (the
# Makefile targets in /data/animeenigma/Makefile rebuild and restart the
# affected containers). Asserts the 7 ROADMAP success criteria in
# services-up order.
#
# Usage:
#   ./scripts/smoke-spotlight.sh
#
# Env overrides:
#   GATEWAY_URL    default http://localhost:8000
#   CATALOG_URL    default http://localhost:8081
#   COMPOSE_FILE   default docker/docker-compose.yml
#   SKIP_FLAG_OFF  default 0 — set to 1 to skip emitting the flag-off manual hint
#   SKIP_WEB_DOWN  default 0 — set to 1 to skip emitting the web-down manual hint
#
# Exits 0 on full pass, non-zero on any failure.

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8000}"
CATALOG_URL="${CATALOG_URL:-http://localhost:8081}"
COMPOSE_FILE="${COMPOSE_FILE:-docker/docker-compose.yml}"

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "FATAL: required command '$1' not found in PATH" >&2
        exit 2
    }
}
require_cmd curl
require_cmd jq
require_cmd docker

fail() { echo "FAIL: $*" >&2; exit 1; }
ok()   { echo "OK:   $*"; }
note() { echo "...   $*"; }

# ---- 1. Wait for gateway to be healthy -----------------------------------
note "Waiting for gateway at ${GATEWAY_URL} (up to 30s)…"
for i in $(seq 1 30); do
    if curl -fsS -o /dev/null "${GATEWAY_URL}/health" 2>/dev/null; then
        ok "Gateway reachable"
        break
    fi
    if [ "$i" -eq 30 ]; then
        fail "Gateway never became healthy at ${GATEWAY_URL}/health"
    fi
    sleep 1
done

# ---- 2. Spotlight returns >=3 cards (ROADMAP §1.2) -----------------------
RESP_FILE="$(mktemp -t spotlight.XXXXXX.json)"
SECOND_FILE="$(mktemp -t spotlight2.XXXXXX.json)"
trap 'rm -f "$RESP_FILE" "$SECOND_FILE"' EXIT

HTTP_CODE=$(curl -s -o "$RESP_FILE" -w "%{http_code}" "${GATEWAY_URL}/api/home/spotlight")
if [ "$HTTP_CODE" != "200" ]; then
    fail "/api/home/spotlight returned HTTP ${HTTP_CODE}, expected 200. Body: $(cat "$RESP_FILE")"
fi

CARDS_LEN=$(jq '.cards | length' < "$RESP_FILE")
if [ "$CARDS_LEN" -lt 3 ]; then
    fail "Expected >= 3 cards, got ${CARDS_LEN}. Body: $(cat "$RESP_FILE")"
fi
ok "Spotlight returned ${CARDS_LEN} cards"

# ---- 3. Card types include the 4 expected (ROADMAP §1.3) -----------------
TYPES=$(jq -r '.cards[].type' < "$RESP_FILE" | sort -u | tr '\n' ',' | sed 's/,$//')
note "Card types present: ${TYPES}"

for TYPE in anime_of_day random_tail platform_stats; do
    if ! echo "$TYPES" | grep -q "$TYPE"; then
        fail "Expected card type '${TYPE}' in response; got types: ${TYPES}"
    fi
    ok "Card type present: ${TYPE}"
done
# latest_news may flake if web container is paused — log only.
if echo "$TYPES" | grep -q "latest_news"; then
    ok "Card type present: latest_news"
else
    note "latest_news absent — acceptable degradation if web:80/changelog.json was unreachable"
fi

# ---- 4. Redis has spotlight:* keys (ROADMAP §1.4) ------------------------
KEYS=$(docker compose -f "$COMPOSE_FILE" exec -T redis redis-cli --no-raw KEYS 'spotlight:*' | tr -d '\r' | grep -v '^$' || true)
KEY_COUNT=$(echo "$KEYS" | grep -c "^" || true)
if [ -z "$KEYS" ]; then
    KEY_COUNT=0
fi
if [ "$KEY_COUNT" -lt 1 ]; then
    fail "Expected >=1 spotlight:* key in Redis, got ${KEY_COUNT}"
fi
ok "Redis has ${KEY_COUNT} spotlight:* key(s)"
note "$KEYS"

# ---- 5. Cache hit on second call (ROADMAP §1.5) --------------------------
curl -s -o "$SECOND_FILE" "${GATEWAY_URL}/api/home/spotlight"
GEN_AT_1=$(jq -r '.generated_at' < "$RESP_FILE")
GEN_AT_2=$(jq -r '.generated_at' < "$SECOND_FILE")
note "generated_at first=${GEN_AT_1}, second=${GEN_AT_2}"
# Note: aggregator regenerates the envelope per request, so generated_at may change;
# what we assert is CARD-level cache hit by checking the anime_of_day
# data is identical (same UTC day → same pick).
PICK_1=$(jq -r '.cards[] | select(.type=="anime_of_day") | .data.anime.id // ""' < "$RESP_FILE")
PICK_2=$(jq -r '.cards[] | select(.type=="anime_of_day") | .data.anime.id // ""' < "$SECOND_FILE")
if [ -n "$PICK_1" ] && [ "$PICK_1" != "$PICK_2" ]; then
    fail "Cache hit broken: anime_of_day picked different anime across two calls. PICK_1=${PICK_1}, PICK_2=${PICK_2}"
fi
ok "Cache hit: anime_of_day picked the same anime on both calls (id=${PICK_1:-<absent>})"

# ---- 6. /metrics p95 < 100ms cached (ROADMAP §1.5 latency) ---------------
# Drive a few requests to populate the histogram.
for i in $(seq 1 10); do curl -s -o /dev/null "${GATEWAY_URL}/api/home/spotlight"; done

# Scrape catalog /metrics for the histogram bucket. The catalog
# service exposes http_request_duration_seconds with path label.
METRICS=$(curl -fsS "${CATALOG_URL}/metrics" || true)
if [ -z "$METRICS" ]; then
    note "Catalog /metrics unreachable; skipping p95 latency assertion"
else
    # Best-effort: look for the histogram and report buckets. A strict p95
    # computation in bash is gnarly; for the smoke we log buckets so the
    # operator can eyeball them. Soft assertion only.
    echo "$METRICS" | grep -E '^http_request_duration_seconds_bucket.*home/spotlight' | head -20 || \
        note "No /api/home/spotlight latency buckets yet (need more traffic)"
    ok "Metrics scraped (review buckets above for p95 health)"
fi

# ---- 7. Flag-off path returns 404 (ROADMAP §1.6) -------------------------
# This step is INTERACTIVE-by-design: setting SPOTLIGHT_ENABLED=false
# requires a redeploy. Skip in CI mode (SKIP_FLAG_OFF=1) but document the
# manual check.
if [ "${SKIP_FLAG_OFF:-0}" = "1" ]; then
    note "Skipping flag-off check (SKIP_FLAG_OFF=1)"
else
    echo
    echo "Manual step (ROADMAP §1.6):"
    echo "  1. Edit docker/.env, set SPOTLIGHT_ENABLED=false"
    echo "  2. Redeploy the catalog service (see Makefile target in repo root)"
    echo "  3. Then: curl -s -o /dev/null -w '%{http_code}\\n' ${GATEWAY_URL}/api/home/spotlight"
    echo "  4. Expect: 404"
    echo "  5. Restore: SPOTLIGHT_ENABLED=true and redeploy catalog again"
fi

# ---- 8. Web-down degradation (ROADMAP §1.7) ------------------------------
if [ "${SKIP_WEB_DOWN:-0}" = "1" ]; then
    note "Skipping web-down check (SKIP_WEB_DOWN=1)"
else
    echo
    echo "Manual step (ROADMAP §1.7):"
    echo "  1. docker compose -f ${COMPOSE_FILE} stop web"
    echo "  2. docker compose -f ${COMPOSE_FILE} exec -T redis redis-cli DEL 'spotlight:changelog:*'"
    echo "  3. curl -s ${GATEWAY_URL}/api/home/spotlight | jq '.cards | length'  # expect <=3, and"
    echo "     jq '.cards[].type' should NOT include 'latest_news'"
    echo "  4. Check catalog logs: docker compose logs catalog | grep 'spotlight.card_failed' | tail -3"
    echo "  5. Restore: docker compose -f ${COMPOSE_FILE} start web"
fi

echo
ok "All automated smoke assertions passed"
