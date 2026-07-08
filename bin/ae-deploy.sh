#!/usr/bin/env bash
# ae-deploy.sh — fast-forward the shared tree to origin/main, redeploy the given
# services, and verify each container is up. Replaces the manual ff-dance +
# `make redeploy-*` + full `make health` (14 services) with terse output.
#
# Usage: bin/ae-deploy.sh <service> [<service> ...]     e.g.  web   |   catalog gateway
#   Run AFTER bin/ae-land.sh — your commit must already be on origin/main, since
#   `make redeploy-*` builds from the shared tree (/data/animeenigma), not a worktree.
set -uo pipefail

SHARED="/data/animeenigma"
[ "$#" -ge 1 ] || { echo "usage: $(basename "$0") <service> [<service> ...]"; exit 1; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# --- ff the shared tree to origin/main (exactly what the autosync cron does) --
echo "== ff shared tree ($SHARED) to origin/main =="
git -C "$SHARED" fetch origin -q || { echo "FAIL: fetch"; exit 1; }
if ! git -C "$SHARED" merge --ff-only origin/main >"$TMP/ff" 2>&1; then
  echo "FAIL: shared tree is NOT fast-forwardable (dirty or diverged):"
  cat "$TMP/ff"
  echo "The shared tree must be clean + on main. NOT force-syncing — resolve it manually."
  exit 1
fi
echo "  shared tree now at: $(git -C "$SHARED" log --oneline -1)"

# --- redeploy each service + container up-check -------------------------------
for svc in "$@"; do
  echo "== redeploy $svc =="
  if ! make -C "$SHARED" "redeploy-$svc" >"$TMP/dep" 2>&1; then
    echo "FAIL: redeploy-$svc:"; tail -25 "$TMP/dep"; exit 1
  fi
  cname="animeenigma-$svc"
  st="$(docker ps --filter "name=^${cname}$" --format '{{.Status}}')"
  if [ -z "$st" ]; then
    echo "  WARN: container $cname not found (check compose service name)"
  else
    echo "  $cname: $st"
  fi
done
echo "ALL REDEPLOYS DONE"
