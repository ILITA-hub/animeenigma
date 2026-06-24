#!/usr/bin/env bash
# Launch an AnimeEnigma design sandbox: the superpowers brainstorm visual-companion
# server, pinned to a fixed port so the owner's SSH tunnel reaches it, with the
# owner-PID watchdog disabled so it survives across conversation turns.
#
# Usage: launch-sandbox.sh [--name <session>] [--port <port>] [--idle-min <n>]
#   --name      session/content dir name under .superpowers/brainstrm/ (default: design)
#   --port      bind port; MUST match the owner's tunnel -L 3000:localhost:<port> (default: 58363)
#   --idle-min  idle-timeout minutes before self-exit (default: 120)
#
# Prints the owner-facing URL. Idempotent: if a server already listens on the port,
# it reprints the existing URL instead of starting a competitor.
set -euo pipefail

NAME="design"; PORT="58363"; IDLE_MIN="120"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) NAME="$2"; shift 2;;
    --port) PORT="$2"; shift 2;;
    --idle-min) IDLE_MIN="$2"; shift 2;;
    *) echo "unknown arg: $1" >&2; exit 2;;
  esac
done

# Resolve repo root (this script lives at .claude/skills/design-prototyping/scripts/)
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SESSION_DIR="$REPO_ROOT/.superpowers/brainstorm/$NAME"
mkdir -p "$SESSION_DIR/content" "$SESSION_DIR/state"

# Keep scratch out of git
if ! grep -qE '^\.superpowers/?$' "$REPO_ROOT/.gitignore" 2>/dev/null; then
  printf '\n# Design sandbox scratch\n.superpowers/\n' >> "$REPO_ROOT/.gitignore"
fi

# Token: reuse if present so an already-open browser tab stays valid across restarts
TOKEN_FILE="$SESSION_DIR/.token"
if [[ -s "$TOKEN_FILE" ]]; then TOKEN="$(cat "$TOKEN_FILE")"; else
  TOKEN="$(openssl rand -hex 16 2>/dev/null || head -c16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  printf '%s' "$TOKEN" > "$TOKEN_FILE"
fi

owner_url() { echo "http://localhost:${PORT}/?key=${TOKEN}  (owner opens http://localhost:3000/?key=${TOKEN} via the -L 3000:localhost:${PORT} tunnel)"; }

# Idempotent: already listening on this port?
if ss -ltn 2>/dev/null | grep -q ":${PORT}\b"; then
  echo "sandbox already running on :${PORT}"
  echo "URL: $(owner_url)"
  echo "content dir: $SESSION_DIR/content  (write <name>-vN.html; newest wins)"
  exit 0
fi

# Newest installed brainstorm server
SCRIPTS_DIR="$(ls -d /root/.claude/plugins/cache/claude-plugins-official/superpowers/*/skills/brainstorming/scripts 2>/dev/null | sort -V | tail -1)"
if [[ -z "${SCRIPTS_DIR:-}" || ! -f "$SCRIPTS_DIR/server.cjs" ]]; then
  echo "ERROR: brainstorm server.cjs not found under superpowers plugin cache" >&2; exit 1
fi

SERVER_ID="$(openssl rand -hex 24 2>/dev/null || head -c24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
cd "$SCRIPTS_DIR"
# NOTE: BRAINSTORM_OWNER_PID deliberately UNSET — disables the self-terminate
# watchdog (server.cjs:635 `if (!ownerPid) return true`) so the server outlives
# the launching shell. Idle timeout is the only auto-shutdown.
nohup env \
  BRAINSTORM_DIR="$SESSION_DIR" \
  BRAINSTORM_HOST="127.0.0.1" \
  BRAINSTORM_URL_HOST="localhost" \
  BRAINSTORM_PORT="$PORT" \
  BRAINSTORM_TOKEN="$TOKEN" \
  BRAINSTORM_IDLE_TIMEOUT_MS="$(( IDLE_MIN * 60 * 1000 ))" \
  node server.cjs "--brainstorm-server-id=$SERVER_ID" > "$SESSION_DIR/state/server.log" 2>&1 &
disown || true

# Wait for startup
for _ in $(seq 1 50); do
  grep -q '"type":"server-started"' "$SESSION_DIR/state/server.log" 2>/dev/null && break
  sleep 0.1
done

if ! ss -ltn 2>/dev/null | grep -q ":${PORT}\b"; then
  echo "ERROR: server failed to bind :${PORT}. Log:" >&2
  tail -5 "$SESSION_DIR/state/server.log" >&2 || true
  exit 1
fi

echo "sandbox up on :${PORT} (idle-exit ${IDLE_MIN}m)"
echo "URL: $(owner_url)"
echo "content dir: $SESSION_DIR/content  (write ${NAME}-vN.html; newest wins)"
