#!/usr/bin/env bash
#
# AnimeEnigma — Docker disk hygiene (adopted into the repo 2026-07-10 so
# /admin/policy can pause/tune it). Two modes:
#   daily  -> routine disk_prune:        docker system prune (only when disk% > high_water_pct)
#   weekly -> routine build_cache_prune: docker builder prune + image prune
#
# Replaces the old host-only /etc/cron.d/docker-prune + /etc/cron.weekly/docker-prune
# (see infra/host/README.md for the install/removal steps).
#
# Source of truth: infra/host/animeenigma-docker-prune.sh
# Installed to:    /usr/local/bin/animeenigma-docker-prune.sh  (mode 755)
#
# Usage:
#   animeenigma-docker-prune.sh daily    # disk_prune  (what the 04:17 cron calls)
#   animeenigma-docker-prune.sh weekly   # build_cache_prune (what the Sunday 04:23 cron calls)
#
set -uo pipefail

LOG=/var/log/animeenigma-docker-prune.log
log(){ echo "$(date -Is) $*" >>"$LOG"; }

# shellcheck source=/dev/null
[ -r /usr/local/lib/animeenigma/maint-gate.sh ] && . /usr/local/lib/animeenigma/maint-gate.sh

mode="${1:-daily}"
case "$mode" in
  daily)
    if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled disk_prune; then
      log "skip: disk_prune paused via /admin/policy"
      exit 0
    fi

    hw="$(command -v maint_gate_setting >/dev/null 2>&1 && maint_gate_setting disk_prune high_water_pct)"
    hw="${hw:-80}"

    used=$(df --output=pcent / | tail -1 | tr -dc '0-9')
    if [ "${used:-0}" -le "$hw" ]; then
      log "skip: disk ${used}% <= high_water ${hw}% — nothing to prune"
      command -v maint_status >/dev/null 2>&1 && maint_status disk_prune 0 "disk ${used}% <= ${hw}% (no prune)"
      exit 0
    fi

    before=$(df -h / | tail -1)
    docker system prune -af --filter "until=72h" >>"$LOG" 2>&1
    rc=$?
    after=$(df --output=pcent / | tail -1 | tr -dc '0-9')
    log "disk_prune done (was ${used}%, now ${after}%, rc=$rc); ${before}"
    command -v maint_status >/dev/null 2>&1 && maint_status disk_prune "$rc" "pruned: ${used}% -> ${after}%"
    exit "$rc"
    ;;

  weekly)
    if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled build_cache_prune; then
      log "skip: build_cache_prune paused via /admin/policy"
      exit 0
    fi

    docker builder prune -f --reserved-space 30GB >>"$LOG" 2>&1
    docker image prune -f >>"$LOG" 2>&1
    rc=$?
    log "build_cache_prune done (rc=$rc)"
    command -v maint_status >/dev/null 2>&1 && maint_status build_cache_prune "$rc" "build-cache + image prune"
    exit "$rc"
    ;;

  *)
    echo "usage: $0 daily|weekly" >&2
    exit 2
    ;;
esac
