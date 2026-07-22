#!/usr/bin/env bash
# degradation-override.sh — owner CLI for the graceful-degradation governor
# (docs/superpowers/specs/2026-07-10-graceful-degradation-design.md).
#
# Pins or clears the published degradation level via the Redis override key.
# The governor keeps evaluating in the background, so `clear` snaps back to
# the computed level on the next tick (~15s).
#
# Usage:
#   bin/degradation-override.sh status                # current level/reasons/override(+TTL)
#   bin/degradation-override.sh set 0|1|2             # pin level, auto-expires (DEGRADATION_OVERRIDE_TTL, default 2h)
#   bin/degradation-override.sh set 0|1|2 --permanent # pin with NO expiry (remember to clear)
#   bin/degradation-override.sh clear                 # remove the pin
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
      ottl="$("${REDIS[@]}" TTL "$OVERRIDE_KEY")"
      case "$ottl" in
        -1) echo "override        : PINNED to $override  (NO expiry — 'clear' when done)" ;;
        -2) echo "override        : none" ;;
        *)  echo "override        : PINNED to $override  (auto-expires in ${ottl}s)" ;;
      esac
    else
      echo "override        : none"
    fi
    # Live governor view (richer: raw signals, hysteresis target, prom health).
    curl -sf --max-time 3 http://127.0.0.1:8100/api/degradation/status 2>/dev/null \
      | python3 -m json.tool 2>/dev/null || true
    ;;
  set)
    lvl="${2:?usage: degradation-override.sh set 0|1|2 [--permanent]}"
    case "$lvl" in
      0|1|2) ;;
      *) echo "level must be 0, 1 or 2" >&2; exit 1 ;;
    esac
    # The override is the ONE fail-CLOSED key in this otherwise fail-open system:
    # forgotten at a shed level it starves background work indefinitely. So it
    # AUTO-EXPIRES by default (DEGRADATION_OVERRIDE_TTL seconds, default 2h); the
    # governor keeps computing, so on expiry it snaps back to the real level.
    # Use --permanent for the rare "pin until I clear it" case.
    ttl="${DEGRADATION_OVERRIDE_TTL:-7200}"
    if [ "${3:-}" = "--permanent" ]; then
      "${REDIS[@]}" SET "$OVERRIDE_KEY" "$lvl" > /dev/null
      echo "override pinned to $lvl PERMANENTLY (no expiry) — takes effect on the next tick (~15s)."
      echo "REMEMBER to 'bin/degradation-override.sh clear' when done: this pin will NOT auto-expire."
    else
      "${REDIS[@]}" SET "$OVERRIDE_KEY" "$lvl" EX "$ttl" > /dev/null
      echo "override pinned to $lvl for ${ttl}s (auto-expires) — takes effect on the next tick (~15s)."
      echo "'bin/degradation-override.sh clear' to release early; 'set $lvl --permanent' to pin with no expiry."
    fi
    ;;
  clear)
    "${REDIS[@]}" DEL "$OVERRIDE_KEY" > /dev/null
    echo "override cleared — governor returns to the computed level on the next tick."
    ;;
  *)
    echo "usage: $(basename "$0") status | set 0|1|2 [--permanent] | clear" >&2
    exit 1
    ;;
esac
