#!/bin/bash
# with-deploy-lock.sh — run a command under a host-wide exclusive deploy lock.
#
# WHY: On 2026-06-22 the host froze twice (manual power cycles) from OOM
# death-spirals. Root cause: several `make redeploy-*` running at once (multiple
# agents + the maintenance bot) spawned ~87 concurrent Go `compile` workers
# (~6 GB) on a 16 GB box, exhausting RAM + the full 8 GB swap. Serializing build
# /redeploy so only ONE runs at a time caps the build storm at a single machine's
# worth of compilers instead of N×.
#
# Usage:   deploy/scripts/with-deploy-lock.sh <command> [args...]
# Env:
#   ANIMEENIGMA_DEPLOY_LOCK          lock file path (default /var/lock/animeenigma-deploy.lock)
#   ANIMEENIGMA_DEPLOY_LOCK_TIMEOUT  max seconds to wait for the lock (default 1800)
#   ANIMEENIGMA_DEPLOY_LOCK_HELD     set internally once held; re-entrant no-op when present
set -euo pipefail

LOCK_FILE="${ANIMEENIGMA_DEPLOY_LOCK:-/var/lock/animeenigma-deploy.lock}"
LOCK_TIMEOUT="${ANIMEENIGMA_DEPLOY_LOCK_TIMEOUT:-1800}"

if [ "$#" -eq 0 ]; then
    echo "[deploy-lock] usage: $0 <command> [args...]" >&2
    exit 2
fi

# Already inside the lock (nested redeploy.sh -> make -> ...): just run.
if [ -n "${ANIMEENIGMA_DEPLOY_LOCK_HELD:-}" ]; then
    exec "$@"
fi

# flock is part of util-linux; if it is somehow unavailable, degrade to running
# unserialized rather than blocking deploys entirely.
if ! command -v flock >/dev/null 2>&1; then
    echo "[deploy-lock] WARN: flock not found — running WITHOUT serialization" >&2
    exec env ANIMEENIGMA_DEPLOY_LOCK_HELD=1 "$@"
fi

# Best-effort: if the default /var/lock is not writable, fall back to a temp path.
if ! { : >>"$LOCK_FILE"; } 2>/dev/null; then
    LOCK_FILE="${TMPDIR:-/tmp}/animeenigma-deploy.lock"
fi

echo "[deploy-lock] acquiring exclusive deploy lock ($LOCK_FILE, timeout ${LOCK_TIMEOUT}s)…" >&2
# flock holds the lock for the lifetime of the command; releases on exit/crash.
exec env ANIMEENIGMA_DEPLOY_LOCK_HELD=1 \
    flock -w "$LOCK_TIMEOUT" "$LOCK_FILE" "$@"
