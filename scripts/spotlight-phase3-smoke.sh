#!/usr/bin/env bash
# spotlight-phase3-smoke.sh — Phase 3 end-to-end smoke for the 9-card hero
# spotlight aggregator. Run AFTER catalog + player + gateway + web have been
# redeployed (the Makefile targets in /data/animeenigma/Makefile rebuild and
# restart the affected containers). Asserts the Phase 3 9-card contract for
# both anonymous and authenticated (ui_audit_bot API key) paths.
#
# Usage:
#   ./scripts/spotlight-phase3-smoke.sh
#
# Env overrides:
#   GATEWAY_URL    default http://localhost:8000
#   COMPOSE_FILE   default /data/animeenigma/docker/docker-compose.yml
#
# Exits 0 on full pass, non-zero on any failure. WARN lines are acceptable
# (e.g. seed-data eligibility hasn't populated all 3 login-only cards).

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8000}"
COMPOSE_FILE="${COMPOSE_FILE:-/data/animeenigma/docker/docker-compose.yml}"
API="${GATEWAY_URL}/api/home/spotlight"

# ---- helpers --------------------------------------------------------------
ok()   { printf '\033[0;32mOK:\033[0m   %s\n' "$*"; }
fail() { printf '\033[0;31mFAIL:\033[0m %s\n' "$*" >&2; exit 1; }
note() { printf '\033[0;34m...\033[0m   %s\n' "$*"; }
warn() { printf '\033[0;33mWARN:\033[0m %s\n' "$*"; }

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "FATAL: required command '$1' not found in PATH" >&2
        exit 2
    }
}
require_cmd curl
require_cmd jq
require_cmd docker

# ---- 0. Load UI_AUDIT_API_KEY from docker/.env ----------------------------
# We DELIBERATELY grep one line (UI_AUDIT_API_KEY=ak_...) rather than
# `set -a; . docker/.env` — the .env file contains JWT-shaped tokens with
# dots / colons that POSIX-sourcing chokes on. Defense-in-depth: never echo
# the value, only inject it into the Authorization header.
ENV_FILE="/data/animeenigma/docker/.env"
if [ ! -f "$ENV_FILE" ]; then
    fail "docker/.env not found at $ENV_FILE"
fi
UI_AUDIT_API_KEY="$(grep -E '^UI_AUDIT_API_KEY=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
if [ -z "$UI_AUDIT_API_KEY" ]; then
    fail "UI_AUDIT_API_KEY not set in $ENV_FILE — see CLAUDE.md 'UI Audit Test User'"
fi
note "Loaded UI_AUDIT_API_KEY (prefix=${UI_AUDIT_API_KEY:0:5}..., len=${#UI_AUDIT_API_KEY})"

# ---- Check 1: anonymous response shape ------------------------------------
note "Check 1: anonymous GET ${API}"
ANON_RESP=$(curl -fsS "$API") || fail "anonymous request to ${API} failed"
ANON_COUNT=$(jq '.cards | length' <<<"$ANON_RESP")
if [ "$ANON_COUNT" -lt 1 ]; then
    fail "anon cards count = $ANON_COUNT (want >= 1). Body: $ANON_RESP"
fi
ok "anonymous returned $ANON_COUNT cards"
ANON_TYPES=$(jq -r '.cards[].type' <<<"$ANON_RESP" | sort -u | tr '\n' ',' | sed 's/,$//')
note "anonymous types: $ANON_TYPES"

# ---- Check 2: anonymous never has login-only cards ------------------------
note "Check 2: anonymous MUST NOT include not_time_yet or continue_watching_new"
NOT_LOGIN=$(jq '[.cards[] | select(.type == "not_time_yet" or .type == "continue_watching_new")] | length' <<<"$ANON_RESP")
if [ "$NOT_LOGIN" != "0" ]; then
    fail "anon response leaked login-only cards (count=$NOT_LOGIN). Types: $ANON_TYPES"
fi
ok "anonymous response has 0 login-only cards"

# ---- Check 3: bare envelope, no success/data wrapper (DIVERGENCE 3 guard) -
note "Check 3: response is a bare envelope (no .success/.data wrapper)"
HAS_WRAPPER=$(jq 'has("success") or has("data")' <<<"$ANON_RESP")
if [ "$HAS_WRAPPER" != "false" ]; then
    fail "response wrapped in success/data envelope (regression of DIVERGENCE 3)"
fi
ok "response is a bare {cards, generated_at} envelope"

# ---- Check 4: authenticated response shape --------------------------------
note "Check 4: authenticated GET (Authorization: Bearer ak_...)"
AUTH_RESP=$(curl -fsS -H "Authorization: Bearer $UI_AUDIT_API_KEY" "$API") || \
    fail "authenticated request failed"
AUTH_COUNT=$(jq '.cards | length' <<<"$AUTH_RESP")
if [ "$AUTH_COUNT" -lt "$ANON_COUNT" ]; then
    fail "authenticated cards ($AUTH_COUNT) < anon ($ANON_COUNT) — login should add cards, never remove"
fi
ok "authenticated returned $AUTH_COUNT cards (anon=$ANON_COUNT)"
AUTH_TYPES=$(jq -r '.cards[].type' <<<"$AUTH_RESP" | sort -u | tr '\n' ',' | sed 's/,$//')
note "authenticated types: $AUTH_TYPES"

# ---- Check 5: authenticated SHOULD have >=1 login-only card (warn) --------
note "Check 5: authenticated SHOULD include personal_pick / not_time_yet / continue_watching_new"
LOGIN_ONLY=$(jq '[.cards[] | select(.type == "personal_pick" or .type == "not_time_yet" or .type == "continue_watching_new")] | length' <<<"$AUTH_RESP")
if [ "$LOGIN_ONLY" = "0" ]; then
    warn "authenticated response has 0 login-only cards — seed data may need refresh (./scripts/seed-ui-audit-user.sh)"
else
    ok "authenticated response includes $LOGIN_ONLY login-only card(s)"
fi

# ---- Check 6: gateway does NOT proxy /internal/users/.../list -------------
note "Check 6: gateway MUST NOT proxy /internal/users/.../list (HSB-BE-26 defense-in-depth)"
INTERNAL_STATUS=$(curl -s -o /dev/null -w '%{http_code}' "${GATEWAY_URL}/internal/users/u1/list?status=watching")
if [ "$INTERNAL_STATUS" != "404" ]; then
    fail "gateway proxied internal endpoint (got $INTERNAL_STATUS, want 404)"
fi
ok "gateway returns 404 for /internal/users/u1/list (defense-in-depth)"

# ---- Check 7: Redis spotlight:* keys present ------------------------------
note "Check 7: Redis has spotlight:* keys after the requests above"
KEYS_LIST=$(docker compose -f "$COMPOSE_FILE" exec -T redis redis-cli --no-raw KEYS 'spotlight:*' | tr -d '\r' | grep -v '^$' || true)
KEY_COUNT=$(echo "$KEYS_LIST" | grep -c '^' || true)
if [ -z "$KEYS_LIST" ]; then
    KEY_COUNT=0
fi
if [ "$KEY_COUNT" -lt 1 ]; then
    fail "no spotlight:* Redis keys after requests"
fi
ok "Redis has $KEY_COUNT spotlight:* key(s)"
note "$KEYS_LIST"

# ---- Check 8: watch_progress.updated_at index exists ----------------------
note "Check 8: idx_watch_progress_updated_at exists on watch_progress (HSB-BE-30)"
HAS_IDX=$(docker compose -f "$COMPOSE_FILE" exec -T postgres psql -U postgres -d animeenigma -tAc \
    "SELECT 1 FROM pg_indexes WHERE tablename='watch_progress' AND indexname='idx_watch_progress_updated_at';" \
    | tr -d '\r' | head -1)
if [ "$HAS_IDX" != "1" ]; then
    fail "idx_watch_progress_updated_at missing — Plan 03-01 GORM tag did not migrate"
fi
ok "idx_watch_progress_updated_at index present"

# ---- Final card-count summary ---------------------------------------------
echo
echo "----------------------------------------"
echo "Phase 3 smoke summary"
echo "----------------------------------------"
echo "Anonymous:     $ANON_COUNT cards — $ANON_TYPES"
echo "Authenticated: $AUTH_COUNT cards — $AUTH_TYPES"
echo "Redis keys:    $KEY_COUNT spotlight:* entries"
echo "----------------------------------------"
echo "Phase 3 smoke: PASSED"
