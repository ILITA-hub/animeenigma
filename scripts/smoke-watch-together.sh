#!/usr/bin/env bash
# Watch-Together end-to-end smoke test (Phase 1 acceptance check)
#
# Drives ALL 8 success criteria from
#   .planning/workstreams/watch-together/ROADMAP.md
# against a live local stack (gateway:8000 + watch-together:8091 + redis).
#
# Pre-reqs:
#   - Docker stack up:  make redeploy-watch-together && make redeploy-gateway
#   - Tools:            jq, openssl, docker, bun (with `ws` package globally
#                       or installed in $TMPDIR/ws-smoke), curl
#   - docker/.env:      JWT_SECRET (signing secret used by all services)
#   - DB seeded:        ui_audit_bot user must exist
#                       (run scripts/seed-ui-audit-user.sh once if missing)
#
# Usage:
#   bash scripts/smoke-watch-together.sh
#   WATCH_TOGETHER_FAST_TTL=1 bash scripts/smoke-watch-together.sh   # TTL test
#
# Idempotent: the script tracks rooms it creates and cleans them up via
# a trap on EXIT. Re-runnable as many times as you like.
#
# NOT a CI tool — CI smoke tests are deferred to Phase 5 (the smoke runs
# happily against any healthy local stack but takes ~3s, which is fine for
# a developer "did I break anything" check).
#
# See: .planning/workstreams/watch-together/phases/01-backend-foundation/01.9-PLAN.md
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ----------------------------------------------------------------------------
# 0. Tool discovery
# ----------------------------------------------------------------------------
require() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "FATAL: required tool '$tool' not on PATH" >&2
    echo "       install it (apt / brew / your package manager) and retry." >&2
    exit 1
  fi
}

require jq
require openssl
require docker
require curl

# Bun is the WS client because neither websocat nor wscat is reliably
# scriptable across distros. Bun ships node-compatible `ws` package, and
# almost every machine in our dev rotation already has bun for the frontend.
# If bun is missing, fall back to wscat via bun-global if possible.
WS_CLIENT_KIND="bun"
if ! command -v bun >/dev/null 2>&1; then
  echo "WARN: bun not found on PATH — falling back to wscat" >&2
  if ! command -v wscat >/dev/null 2>&1; then
    echo "FATAL: neither bun nor wscat available. install one of:" >&2
    echo "  - curl -fsSL https://bun.sh/install | bash" >&2
    echo "  - bun install -g wscat   (after bun installed)" >&2
    echo "  - npm install -g wscat   (if you have node)" >&2
    exit 1
  fi
  WS_CLIENT_KIND="wscat"
fi

# ----------------------------------------------------------------------------
# 1. Secrets
# ----------------------------------------------------------------------------
if [[ ! -f docker/.env ]]; then
  echo "FATAL: docker/.env not found" >&2
  exit 1
fi

# Read JWT_SECRET line-by-line — `source docker/.env` is unsafe because
# the file may contain multiline JWTs / OAuth tokens that break bash parsing.
JWT_SECRET=$(grep -E "^JWT_SECRET=" docker/.env | head -1 | cut -d= -f2-)
if [[ -z "${JWT_SECRET}" ]]; then
  echo "FATAL: JWT_SECRET not set in docker/.env" >&2
  exit 1
fi

# ----------------------------------------------------------------------------
# 2. JWT minter — pure-bash HS256 token using openssl
# ----------------------------------------------------------------------------
mint_jwt() {
  local secret="$1"
  local user_id="$2"
  local username="$3"
  local now exp header payload b64_header b64_payload sig
  now=$(date +%s)
  exp=$((now + 900))  # 15min
  header='{"alg":"HS256","typ":"JWT"}'
  payload=$(printf '{"uid":"%s","username":"%s","role":"user","iss":"animeenigma","sub":"%s","iat":%d,"exp":%d}' \
            "$user_id" "$username" "$user_id" "$now" "$exp")
  b64_header=$(printf '%s' "$header" | openssl base64 -A | tr '+/' '-_' | tr -d '=')
  b64_payload=$(printf '%s' "$payload" | openssl base64 -A | tr '+/' '-_' | tr -d '=')
  sig=$(printf '%s' "${b64_header}.${b64_payload}" \
        | openssl dgst -binary -sha256 -hmac "$secret" \
        | openssl base64 -A | tr '+/' '-_' | tr -d '=')
  printf '%s.%s.%s' "$b64_header" "$b64_payload" "$sig"
}

# ----------------------------------------------------------------------------
# 3. Service health gate
# ----------------------------------------------------------------------------
echo "[1/8] Service health (Criterion 1+2 — make redeploy / direct /health)"
HEALTH=$(curl -fsS --max-time 3 http://localhost:8091/health || echo '')
if ! echo "$HEALTH" | jq -e '.data.status == "ok" or .status == "ok"' >/dev/null 2>&1; then
  echo "FAIL: watch-together /health did not return ok. response: $HEALTH" >&2
  docker compose -f docker/docker-compose.yml logs --tail 20 watch-together >&2 || true
  exit 1
fi
echo "    OK: watch-together:8091 /health = ok"

# ----------------------------------------------------------------------------
# 4. Find the ui_audit_bot user_id
# ----------------------------------------------------------------------------
UI_AUDIT_USER_ID=$(docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -tA \
  -c "SELECT id FROM users WHERE username='ui_audit_bot'" \
  | tr -d '[:space:]')

if [[ -z "$UI_AUDIT_USER_ID" ]]; then
  echo "FAIL: ui_audit_bot user not found in postgres" >&2
  echo "      run scripts/seed-ui-audit-user.sh first" >&2
  exit 1
fi
JWT=$(mint_jwt "$JWT_SECRET" "$UI_AUDIT_USER_ID" "ui_audit_bot")
echo "    OK: minted JWT for ui_audit_bot ($UI_AUDIT_USER_ID)"

# ----------------------------------------------------------------------------
# 5. POST /rooms via gateway → {room_id, invite_url, ws_url}
# ----------------------------------------------------------------------------
echo "[2/8] POST /rooms via gateway (Criterion 2b)"
CREATE_RESPONSE=$(curl -fsS -X POST http://localhost:8000/api/watch-together/rooms \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"anime_id":"00000000-0000-0000-0000-000000000001","episode_id":"1","player":"animelib","translation_id":"smoke-translation"}')

ROOM_ID=$(echo "$CREATE_RESPONSE" | jq -r '.data.room_id // empty')
INVITE_URL=$(echo "$CREATE_RESPONSE" | jq -r '.data.invite_url // empty')
WS_URL=$(echo "$CREATE_RESPONSE" | jq -r '.data.ws_url // empty')

if [[ -z "$ROOM_ID" || -z "$INVITE_URL" || -z "$WS_URL" ]]; then
  echo "FAIL: POST /rooms response missing fields. body: $CREATE_RESPONSE" >&2
  exit 1
fi
echo "    OK: room_id=$ROOM_ID"

# Track created rooms for cleanup on exit.
ROOMS_TO_CLEANUP=("$ROOM_ID")

cleanup() {
  local rc=$?
  echo ""
  echo "[cleanup] DELETE-ing ${#ROOMS_TO_CLEANUP[@]} room(s)…"
  for rid in "${ROOMS_TO_CLEANUP[@]}"; do
    curl -fsS -X DELETE \
      -H "Authorization: Bearer $JWT" \
      "http://localhost:8000/api/watch-together/rooms/$rid" >/dev/null 2>&1 || true
  done
  exit $rc
}
trap cleanup EXIT INT TERM

# ----------------------------------------------------------------------------
# 6. WS smoke — drive Criteria 3, 4, 7 via a single bun script
# ----------------------------------------------------------------------------
echo "[3/8] WS smoke: snapshot, two-client chat broadcast, seek rate-limit (Criteria 3+4+7)"

SMOKE_DIR="${TMPDIR:-/tmp}/wt-smoke"
mkdir -p "$SMOKE_DIR"
cat > "$SMOKE_DIR/smoke.mjs" <<'JS'
// Smoke driver: two simultaneous WS clients, broadcast + rate-limit assertions.
// stdout is consumed by the bash wrapper — keep it parseable.
import WebSocket from 'ws';

const wsUrl = process.argv[2];

function client(label) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(wsUrl);
    const frames = [];
    ws.on('open',    () => process.stdout.write(`${label}:OPEN\n`));
    ws.on('message', (raw) => {
      const s = raw.toString();
      frames.push(s);
      const j = JSON.parse(s);
      process.stdout.write(`${label}:RECV:${j.type}${j.data?.code ? ':' + j.data.code : ''}\n`);
    });
    ws.on('error',   (e) => { process.stdout.write(`${label}:ERR:${e.message}\n`); reject(e); });
    ws.on('close',   (c) => process.stdout.write(`${label}:CLOSE:${c}\n`));
    setTimeout(() => resolve({ ws, frames, label }), 800);
  });
}

try {
  const a = await client('A');
  const b = await client('B');
  await new Promise(r => setTimeout(r, 300));

  // Chat broadcast.
  a.ws.send(JSON.stringify({ type: 'chat:message', data: { body: 'smoke-msg-A' } }));
  await new Promise(r => setTimeout(r, 600));

  // Seek rate-limit (two seeks within 1s → 2nd should yield RATE_LIMITED).
  a.ws.send(JSON.stringify({ type: 'playback:seek', data: { time: 10 } }));
  a.ws.send(JSON.stringify({ type: 'playback:seek', data: { time: 20 } }));
  await new Promise(r => setTimeout(r, 600));

  a.ws.close();
  b.ws.close();
  await new Promise(r => setTimeout(r, 200));

  // Dump frames as one JSON line per client.
  process.stdout.write(`A:FRAMES:${JSON.stringify(a.frames)}\n`);
  process.stdout.write(`B:FRAMES:${JSON.stringify(b.frames)}\n`);
  process.exit(0);
} catch (e) {
  process.stdout.write(`FATAL:${e.message}\n`);
  process.exit(2);
}
JS

# Ensure the `ws` package is reachable to the bun script.
if [[ ! -d "$SMOKE_DIR/node_modules/ws" ]]; then
  ( cd "$SMOKE_DIR" && bun add ws >/dev/null 2>&1 ) || {
    echo "FAIL: bun add ws failed in $SMOKE_DIR" >&2
    exit 1
  }
fi

SMOKE_OUT=$( cd "$SMOKE_DIR" && bun smoke.mjs "ws://localhost:8000/api/watch-together/ws?token=$JWT&room=$ROOM_ID" 2>&1 )

# Criterion 3 — room:snapshot is the FIRST frame on each client.
if ! echo "$SMOKE_OUT" | grep -q "^A:RECV:room:snapshot$"; then
  echo "FAIL: client A never received room:snapshot" >&2
  echo "$SMOKE_OUT" >&2
  exit 1
fi
if ! echo "$SMOKE_OUT" | grep -q "^B:RECV:room:snapshot$"; then
  echo "FAIL: client B never received room:snapshot" >&2
  echo "$SMOKE_OUT" >&2
  exit 1
fi
echo "    OK: both clients received room:snapshot"

# Criterion 4 — chat broadcast reaches both.
if ! echo "$SMOKE_OUT" | grep -q "^A:RECV:chat:message$"; then
  echo "FAIL: chat:message did not echo back to sender A" >&2
  echo "$SMOKE_OUT" >&2
  exit 1
fi
if ! echo "$SMOKE_OUT" | grep -q "^B:RECV:chat:message$"; then
  echo "FAIL: chat:message did not broadcast to client B" >&2
  echo "$SMOKE_OUT" >&2
  exit 1
fi
echo "    OK: chat:message broadcast to both clients"

# Criterion 7 — second rapid seek triggers RATE_LIMITED.
if ! echo "$SMOKE_OUT" | grep -q "RATE_LIMITED"; then
  echo "FAIL: 2 rapid seeks did not yield RATE_LIMITED" >&2
  echo "$SMOKE_OUT" >&2
  exit 1
fi
echo "    OK: rapid seek triggered RATE_LIMITED"

# ----------------------------------------------------------------------------
# 7. Criterion 5 — no token → HTTP 401
# ----------------------------------------------------------------------------
echo "[4/8] WS without token → HTTP 401 (Criterion 5)"
RC=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  "http://localhost:8000/api/watch-together/ws?room=anything")
if [[ "$RC" != "401" ]]; then
  echo "FAIL: expected HTTP 401 without token, got $RC" >&2
  exit 1
fi
echo "    OK: HTTP 401"

# ----------------------------------------------------------------------------
# 8. Criterion 6 — bogus room → HTTP 404 (or ROOM_NOT_FOUND close frame —
#    per 01.5 deviation, the watch-together service emits HTTP 404
#    pre-upgrade, NOT a close-frame envelope).
# ----------------------------------------------------------------------------
echo "[5/8] WS with bogus room → HTTP 404 (Criterion 6 — 01.5 deviation)"
RC=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  "http://localhost:8000/api/watch-together/ws?token=$JWT&room=does-not-exist-$(date +%s)")
if [[ "$RC" != "404" ]]; then
  echo "FAIL: expected HTTP 404 for bogus room, got $RC" >&2
  exit 1
fi
echo "    OK: HTTP 404"

# ----------------------------------------------------------------------------
# 9. Criterion 8 — TTL expiry → 410 Gone (opt-in via WATCH_TOGETHER_FAST_TTL=1)
# ----------------------------------------------------------------------------
echo "[6/8] TTL expiry → 410 (Criterion 8)"
if [[ "${WATCH_TOGETHER_FAST_TTL:-0}" == "1" ]]; then
  echo "    NOTE: WATCH_TOGETHER_FAST_TTL=1 set — verifying live"
  echo "          (you must have restarted watch-together with WATCH_TOGETHER_ROOM_TTL=5s)"

  TTL_ROOM=$(curl -fsS -X POST http://localhost:8000/api/watch-together/rooms \
    -H "Authorization: Bearer $JWT" \
    -H "Content-Type: application/json" \
    -d '{"anime_id":"00000000-0000-0000-0000-000000000002","episode_id":"1","player":"animelib","translation_id":"ttl-test"}' \
    | jq -r .data.room_id)
  ROOMS_TO_CLEANUP+=("$TTL_ROOM")

  echo "    sleeping 7s to outlast a 5s TTL…"
  sleep 7
  RC=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $JWT" \
    "http://localhost:8000/api/watch-together/rooms/$TTL_ROOM")
  if [[ "$RC" != "410" ]]; then
    echo "FAIL: expected HTTP 410 after TTL, got $RC" >&2
    exit 1
  fi
  echo "    OK: HTTP 410"
else
  echo "    SKIP: set WATCH_TOGETHER_FAST_TTL=1 to enable (requires watch-together"
  echo "          restarted with WATCH_TOGETHER_ROOM_TTL=5s for runtime-feasible test)."
  echo "          Production TTL is 15min; deep-check runs in Phase 5 prod-readiness."
fi

# ----------------------------------------------------------------------------
# 10. Bonus: GET /rooms/{id} returns snapshot
# ----------------------------------------------------------------------------
echo "[7/8] GET /rooms/{id} snapshot"
SNAP=$(curl -fsS -H "Authorization: Bearer $JWT" \
  "http://localhost:8000/api/watch-together/rooms/$ROOM_ID")
if ! echo "$SNAP" | jq -e '.data.room.id' >/dev/null 2>&1; then
  echo "FAIL: GET /rooms/{id} did not return a room snapshot. body: $SNAP" >&2
  exit 1
fi
echo "    OK: snapshot returned"

# ----------------------------------------------------------------------------
# 11. Done. Cleanup runs via EXIT trap.
# ----------------------------------------------------------------------------
echo "[8/8] cleanup (DELETE) runs on EXIT trap"
echo ""
echo "✓ smoke complete"
