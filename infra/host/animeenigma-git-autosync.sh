#!/usr/bin/env bash
# AnimeEnigma — auto-sync the shared base tree (/data/animeenigma) to origin/main.
#
# SAFE BY DESIGN: fetch + fast-forward-only. This script NEVER merges (non-ff),
# rebases, stashes, resets, force-updates, or pushes. If the tree has diverged
# (local commits not on origin/main) or a fast-forward is blocked by uncommitted
# changes, it logs the reason and exits 0 having touched nothing. That makes it
# safe to run unattended on the shared tree and compatible with the git-guard hook.
#
# Source of truth: repo copy at infra/host/animeenigma-git-autosync.sh
# Installed to:    /usr/local/bin/animeenigma-git-autosync.sh
# Scheduled by:    /etc/cron.d/animeenigma-git-sync  (every 10 min, as root)
set -uo pipefail

REPO=/data/animeenigma
BRANCH=main
LOG=/var/log/animeenigma-git-sync.log
LOCK=/tmp/animeenigma-git-sync.lock

log() { echo "$(date -Is) $*" >>"$LOG"; }

# Single-flight: never let two syncs overlap.
exec 9>"$LOCK" 2>/dev/null || exit 0
flock -n 9 || { log "skip: another sync already running"; exit 0; }

cd "$REPO" 2>/dev/null || { log "skip: $REPO not present"; exit 0; }

# Only operate when the base tree is checked out on $BRANCH (its invariant).
cur=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || echo "DETACHED")
if [ "$cur" != "$BRANCH" ]; then
  log "skip: HEAD is on '$cur', not '$BRANCH' — leaving as-is"
  exit 0
fi

# Fetch non-interactively. BatchMode => never blocks on a passphrase/known-hosts.
if ! GIT_SSH_COMMAND='ssh -o BatchMode=yes -o ConnectTimeout=10' \
     git fetch --quiet origin "$BRANCH" 2>>"$LOG"; then
  log "skip: 'git fetch origin $BRANCH' failed"
  exit 0
fi

local_sha=$(git rev-parse --short HEAD 2>/dev/null)
remote_sha=$(git rev-parse --short "origin/$BRANCH" 2>/dev/null)

if [ "$local_sha" = "$remote_sha" ]; then
  log "ok: already current ($local_sha)"
else
  # Refuse to act if we carry local commits origin/$BRANCH does not (divergence).
  ahead=$(git rev-list --count "origin/$BRANCH..HEAD" 2>/dev/null || echo "?")
  if [ "$ahead" != "0" ]; then
    log "skip: DIVERGED — $ahead local commit(s) not on origin/$BRANCH (local=$local_sha remote=$remote_sha). Push or drop them; ff-only sync paused until then."
  # Fast-forward only. Harmlessly aborts if uncommitted changes touch incoming files.
  elif git merge --ff-only --quiet "origin/$BRANCH" 2>>"$LOG"; then
    log "ok: fast-forwarded $local_sha -> $(git rev-parse --short HEAD)"
  else
    log "skip: fast-forward blocked (uncommitted changes overlap incoming files); left at $local_sha"
  fi
fi

# Cheap hygiene: drop worktree refs whose dirs are gone (does not delete live worktrees).
git worktree prune 2>>"$LOG" || true

# Self-trim the log so it can never grow unbounded (we hold the lock here).
if [ -f "$LOG" ]; then
  tail -n 2000 "$LOG" >"$LOG.tmp" 2>/dev/null && mv "$LOG.tmp" "$LOG" 2>/dev/null || true
fi

exit 0
