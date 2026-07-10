#!/usr/bin/env bash
# degradation-override.sh — owner CLI for the graceful-degradation governor
# (docs/superpowers/specs/2026-07-10-graceful-degradation-design.md).
#
# Pins or clears the published degradation level via the Redis override key.
# The governor keeps evaluating in the background, so `clear` snaps back to
# the computed level on the next tick (~15s).
#
# Usage:
#   bin/degradation-override.sh status      # current level/reasons/override
#   bin/degradation-override.sh set 0|1|2   # pin level (0 normal, 1 elevated, 2 critical)
#   bin/degradation-override.sh clear       # remove the pin
set -euo pipefail

REDIS=(docker exec animeenigma-redis redis-cli)
OVERRIDE_KEY="ae:degradation:override"
LEVEL_KEY="ae:degradation:level"
REASONS_KEY="ae:degradation:reasons"

cmd="${1:-status}"
case "$cmd" in
  status)
    level="$("${REDIS[@]}" GET "$LEVEL_KEY")"
    reasons="$("${REDIS[@]}" GET "$REASONS_KEY")"
    override="$("${REDIS[@]}" GET "$OVERRIDE_KEY")"
    echo "published level : ${level:-<no key — governor down or not deployed; consumers fail open to 0>}"
    echo "reasons         : ${reasons:-[]}"
    if [ -n "$override" ]; then
      echo "override        : PINNED to $override  (bin/degradation-override.sh clear to release)"
    else
      echo "override        : none"
    fi
    # Live governor view (richer: raw signals, hysteresis target, prom health).
    curl -sf --max-time 3 http://127.0.0.1:8099/api/degradation/status 2>/dev/null \
      | python3 -m json.tool 2>/dev/null || true
    ;;
  set)
    lvl="${2:?usage: degradation-override.sh set 0|1|2}"
    case "$lvl" in
      0|1|2) ;;
      *) echo "level must be 0, 1 or 2" >&2; exit 1 ;;
    esac
    "${REDIS[@]}" SET "$OVERRIDE_KEY" "$lvl" > /dev/null
    echo "override pinned to $lvl — takes effect on the next governor tick (~15s)."
    echo "REMEMBER to 'bin/degradation-override.sh clear' when done: the pin has no expiry."
    ;;
  clear)
    "${REDIS[@]}" DEL "$OVERRIDE_KEY" > /dev/null
    echo "override cleared — governor returns to the computed level on the next tick."
    ;;
  *)
    echo "usage: $(basename "$0") status | set 0|1|2 | clear" >&2
    exit 1
    ;;
esac
