# shellcheck shell=bash
# infra/host/animeenigma-maint-gate.sh
# Sourceable maintenance-routine gate helper for host automations.
# Reads policy-service (host-published 127.0.0.1:8098). FAIL-OPEN: any error
# (unreachable / non-200 / parse-fail) is treated as "enabled" so a policy
# outage never silently pauses a host routine.
# Install to: /usr/local/lib/animeenigma/maint-gate.sh  (source it from scripts)

MAINT_POLICY_BASE="${MAINT_POLICY_BASE:-http://localhost:8098}"

# maint_gate_enabled <routine_id> -> return 0 (run) unless gate says enabled:false
maint_gate_enabled() {
  local id="$1" body enabled
  body=$(curl -fsS -m 4 "$MAINT_POLICY_BASE/internal/maintenance/routines/$id" 2>/dev/null) || return 0
  # jq's `//` treats JSON `false` like null (falsy default), which would swallow
  # an explicit enabled:false. Use strict equality so ONLY a real boolean false
  # pauses the routine; null/missing/true all fall through to "true" (run).
  enabled=$(printf '%s' "$body" | jq -r 'if (.data.enabled == false) then "false" else "true" end' 2>/dev/null) || return 0
  [ "$enabled" = "false" ] && return 1
  return 0
}

# maint_gate_setting <routine_id> <key> -> prints value (empty on any miss)
maint_gate_setting() {
  local id="$1" key="$2" body
  body=$(curl -fsS -m 4 "$MAINT_POLICY_BASE/internal/maintenance/routines/$id" 2>/dev/null) || return 0
  # Same `//` landmine: a boolean-false (or 0) setting would be swallowed as
  # "missing". Only a genuinely null/absent key yields empty; false/0/"x" print.
  printf '%s' "$body" | jq -r --arg k "$key" 'if (.data.settings[$k] == null) then empty else (.data.settings[$k]|tostring) end' 2>/dev/null || true
}

# maint_status <routine_id> <ok:0|1> <summary> -> fire-and-forget status POST
maint_status() {
  local id="$1" ok="$2" summary="$3" okjson=false
  [ "$ok" = "0" ] && okjson=true
  curl -fsS -m 4 -X POST -H 'Content-Type: application/json' \
    -d "$(jq -nc --argjson ok "$okjson" --arg s "$summary" '{ok:$ok,summary:$s}')" \
    "$MAINT_POLICY_BASE/internal/maintenance/routines/$id/status" >/dev/null 2>&1 || true
}
