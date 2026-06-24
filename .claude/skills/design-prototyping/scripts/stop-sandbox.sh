#!/usr/bin/env bash
# Stop a design sandbox started by launch-sandbox.sh.
# Usage: stop-sandbox.sh [--name <session>]
set -euo pipefail

NAME="design"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --name) NAME="$2"; shift 2;;
    *) echo "unknown arg: $1" >&2; exit 2;;
  esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SESSION_DIR="$REPO_ROOT/.superpowers/brainstorm/$NAME"
PID_LOG="$SESSION_DIR/state/server.log"

# server.cjs logs its port; find the node process serving this session dir and kill it.
PIDS="$(pgrep -f "server.cjs" 2>/dev/null || true)"
killed=0
for pid in $PIDS; do
  if tr '\0' ' ' < "/proc/$pid/environ" 2>/dev/null | grep -q "BRAINSTORM_DIR=$SESSION_DIR"; then
    kill "$pid" 2>/dev/null && { echo "stopped sandbox '$NAME' (pid $pid)"; killed=1; }
  fi
done
[[ "$killed" == 0 ]] && echo "no running sandbox found for '$NAME'"
exit 0
