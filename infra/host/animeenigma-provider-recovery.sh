#!/usr/bin/env bash
#
# AnimeEnigma — daily provider-recovery operator (headless Claude Code).
#
# Runs ONE autonomous Claude Code session that adopts a single unhealthy EN
# scraper provider, diagnoses the root cause, attempts a real recovery (an honest
# probe-result verdict and/or a small worktree fix + /animeenigma-after-update),
# verifies it end-to-end, appends to docs/issues/provider-recovery-log.md, and
# posts one report to the Telegram admin chat. Driven by
# animeenigma-provider-recovery.timer (daily).
#
# FULL AUTONOMY (authorized by the project owner): the session runs with
# --dangerously-skip-permissions. It can edit code in worktrees, run
# `make redeploy-*`, and `git push` to main with NO human in the loop. The
# committed git-guard + ds-lint hooks in .claude/ still apply. To stop it:
#   systemctl disable --now animeenigma-provider-recovery.timer
#
# Source of truth: infra/host/animeenigma-provider-recovery.sh
# Installed to:    /usr/local/bin/animeenigma-provider-recovery.sh  (mode 755)
#
# Usage:
#   animeenigma-provider-recovery.sh           # full run (what the timer calls)
#   animeenigma-provider-recovery.sh --check   # prerequisites only, no agent run
#
set -uo pipefail

REPO="${ANIMEENIGMA_REPO:-/data/animeenigma}"
PROMPT_FILE="${RECOVERY_PROMPT_FILE:-/usr/local/share/animeenigma/provider-recovery-prompt.md}"
CLAUDE_BIN="${CLAUDE_BIN:-/root/.local/bin/claude}"
MODEL="${RECOVERY_MODEL:-sonnet}"
LOG="${RECOVERY_LOG:-/var/log/animeenigma-provider-recovery.log}"

# Root workaround: Claude Code refuses --dangerously-skip-permissions when running
# as root unless it believes it is sandboxed. The systemd unit (a one-shot,
# resource-capped, timer-driven cgroup) is that boundary. Override per-env if the
# CLI ever accepts the flag as root directly.
export IS_SANDBOX="${IS_SANDBOX:-1}"

ts(){ date -u +%FT%TZ; }
log(){ local m; m="$(ts) $*"; printf '%s\n' "$m" >>"$LOG" 2>/dev/null; printf '%s\n' "$m" >&2; }

check(){
  local rc=0
  if [[ ! -x "$CLAUDE_BIN" ]]; then log "FAIL: claude binary not executable: $CLAUDE_BIN"; rc=1; fi
  if [[ ! -r "$PROMPT_FILE" ]]; then log "FAIL: prompt file unreadable: $PROMPT_FILE"; rc=1; fi
  if [[ ! -d "$REPO/.git" ]]; then log "FAIL: git repo not found: $REPO"; rc=1; fi
  if curl -fsS -m 5 http://localhost:8081/internal/scraper/providers >/dev/null 2>&1; then
    log "ok: catalog internal endpoint reachable (localhost:8081)"
  else
    log "WARN: catalog internal endpoint unreachable (localhost:8081) — roster read may fail"
  fi
  if [[ -r "$REPO/docker/.env" ]]; then
    log "ok: docker/.env readable (Telegram secrets present)"
  else
    log "WARN: docker/.env unreadable — Telegram report may fail"
  fi
  return "$rc"
}

case "${1:-run}" in
  --check|check) check; exit $? ;;
esac

if ! check; then
  log "prerequisite check failed — aborting run"
  exit 1
fi

# shellcheck source=/dev/null
if [ -r /usr/local/lib/animeenigma/maint-gate.sh ]; then
  . /usr/local/lib/animeenigma/maint-gate.sh
elif [ -r "$(dirname "$0")/animeenigma-maint-gate.sh" ]; then
  . "$(dirname "$0")/animeenigma-maint-gate.sh"
fi

if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled provider_recovery; then
  log "skip: provider_recovery paused via /admin/policy"
  exit 0
fi

log "=== provider-recovery run START (model=$MODEL, repo=$REPO) ==="
cd "$REPO" || { log "FAIL: cannot cd into $REPO"; exit 1; }

PROMPT="$(cat "$PROMPT_FILE")"
"$CLAUDE_BIN" -p "$PROMPT" \
  --model "$MODEL" \
  --dangerously-skip-permissions \
  >>"$LOG" 2>&1
rc=$?
log "=== provider-recovery run END (exit=$rc) ==="
if command -v maint_status >/dev/null 2>&1; then
  maint_status provider_recovery "$rc" "recovery run exit=$rc (model=$MODEL)"
fi
exit "$rc"
