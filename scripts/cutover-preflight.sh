#!/usr/bin/env bash
#
# scripts/cutover-preflight.sh
#
# Phase 20 Cutover — HARD Guardrail Pre-flight Script
# ----------------------------------------------------
#
# Purpose:
#   Enforces ROADMAP v3.0 success criterion 1: the deletion PR for Phase 20
#   (cutover — removes HiAnime + Consumet code paths) MUST NOT ship until
#   EnglishPlayer has served >= 7 days of clean production traffic.
#
#   This script is the gate. It is read-only. It never deletes anything.
#   Every Phase 20 plan (20-02..20-05) MUST re-run this script as its first
#   task and refuse to proceed unless the exit code is 0.
#
# The four gates:
#   1. Date check        — today must be on or after 2026-05-19
#                          (earliest ship: 2026-05-19 = 2026-05-12 + 7 days;
#                           EnglishPlayer first shipped 2026-05-12 via
#                           commit 9e9d9a2).
#   2. Per-provider error rate — sum(rate(parser_requests_total{status="error"}))
#                                / sum(rate(parser_requests_total)) over 7d
#                                must be <= 5% per provider (animepahe, gogoanime).
#                                No-traffic is treated as fail-closed.
#   3. Telegram alerts   — Prometheus ALERTS{alertname="ProviderHealthStreamSegmentDown",
#                          alertstate="firing"} must show no firing in last 7d.
#                          (Phase 17 wired this alert to Telegram, so "alert never
#                          fired" implies "no Telegram notification was sent".)
#   4. docs/issues/      — no markdown files modified in last 7d that mention
#                          EnglishPlayer / animepahe / gogoanime / anitaku /
#                          scraper (ui-audit-* reports are excluded).
#
# Why hard-coded 2026-05-19:
#   EnglishPlayer first shipped to production on 2026-05-12 via commit 9e9d9a2.
#   7 calendar days later = 2026-05-19. This is the earliest legitimate ship
#   date for the deletion PR. Do NOT loosen this threshold; it is the gate.
#
# Override for testing:
#   PROM_URL=http://127.0.0.1:9 bash scripts/cutover-preflight.sh
#   (use an invalid port to simulate Prometheus-unavailable; the script must
#   still exit 1 — fail-closed semantics.)
#
# Exit semantics:
#   exit 0 — all four gates satisfied, cutover may proceed
#   exit 1 — at least one gate failed, do NOT run Plans 20-02..20-05
#
# Trust model:
#   The operator has full root on the host. If they want to bypass the
#   guardrail they can edit this script directly. This script defends against
#   honest-mistake premature deletion, NOT against an adversarial operator.
#

set -euo pipefail

# ---- Constants ----
PROM_URL="${PROM_URL:-http://localhost:9090/prometheus}"
EARLIEST_SHIP="2026-05-19"           # 7 days after EnglishPlayer first ship 2026-05-12 (commit 9e9d9a2)
ERROR_RATE_THRESHOLD=0.05            # 5% per-provider error rate ceiling
GUARDRAIL_WINDOW="7d"                # range vector for PromQL
PROVIDERS=(animepahe gogoanime)      # current EN providers as of 2026-05-12 (see STATE.md Phase 18 complete)
CURL_TIMEOUT=10                      # seconds; fail-closed if Prometheus is slow

# ---- Helpers ----
failed=()

# prom_query <PromQL expression> -> echoes the first scalar value (or empty on failure)
prom_query() {
  local expr="$1"
  curl -fsS --max-time "$CURL_TIMEOUT" --get \
    --data-urlencode "query=${expr}" \
    "$PROM_URL/api/v1/query" 2>/dev/null \
    | jq -r '.data.result[0].value[1] // ""' 2>/dev/null \
    || echo ""
}

# prom_query_any_series <PromQL expression> -> "yes" if result vector has any series, else "no"
prom_query_any_series() {
  local expr="$1"
  local count
  count=$(curl -fsS --max-time "$CURL_TIMEOUT" --get \
    --data-urlencode "query=${expr}" \
    "$PROM_URL/api/v1/query" 2>/dev/null \
    | jq -r '.data.result | length' 2>/dev/null || echo "ERROR")
  if [[ "$count" == "ERROR" || -z "$count" ]]; then
    echo "ERROR"
  elif [[ "$count" -gt 0 ]]; then
    echo "yes"
  else
    echo "no"
  fi
}

# ---- Gate 1: Date ----
today=$(date -u +%Y-%m-%d)
if [[ "$today" < "$EARLIEST_SHIP" ]]; then
  echo "[FAIL] Gate 1 (date): today=$today; earliest ship: $EARLIEST_SHIP (>= 7 days after EnglishPlayer shipped 2026-05-12)" >&2
  failed+=("date")
else
  echo "[PASS] Gate 1 (date): today=$today >= $EARLIEST_SHIP"
fi

# ---- Gate 2: Per-provider error rate <= 5% over 7d ----
for provider in "${PROVIDERS[@]}"; do
  expr="sum(rate(parser_requests_total{parser=\"${provider}\",status=\"error\"}[${GUARDRAIL_WINDOW}])) / sum(rate(parser_requests_total{parser=\"${provider}\"}[${GUARDRAIL_WINDOW}]))"
  value=$(prom_query "$expr")
  if [[ -z "$value" || "$value" == "NaN" ]]; then
    echo "[WARN] Gate 2 (error_rate): provider=$provider has no recorded traffic in last 7d (or Prometheus unreachable at $PROM_URL)" >&2
    failed+=("error_rate:${provider}:no-traffic")
    continue
  fi
  # awk comparison handles floats portably (no bc dependency required)
  exceeds=$(awk -v v="$value" -v t="$ERROR_RATE_THRESHOLD" 'BEGIN { print (v+0 > t+0) ? "yes" : "no" }')
  if [[ "$exceeds" == "yes" ]]; then
    echo "[FAIL] Gate 2 (error_rate): provider=$provider error_rate=$value > $ERROR_RATE_THRESHOLD" >&2
    failed+=("error_rate:${provider}")
  else
    echo "[PASS] Gate 2 (error_rate): provider=$provider error_rate=$value <= $ERROR_RATE_THRESHOLD"
  fi
done

# ---- Gate 3: Zero Telegram alerts (proxied via Prometheus ALERTS firing history) ----
# Phase 17 17-04 wired ProviderHealthStreamSegmentDown -> Alertmanager -> Telegram.
# If the alert never reached firing state in the 7d window, no Telegram message was sent.
alert_expr="max_over_time(ALERTS{alertname=\"ProviderHealthStreamSegmentDown\",alertstate=\"firing\"}[${GUARDRAIL_WINDOW}])"
alert_state=$(prom_query_any_series "$alert_expr")
case "$alert_state" in
  yes)
    echo "[FAIL] Gate 3 (telegram_alerts): ProviderHealthStreamSegmentDown fired in last 7d (= Telegram notification was sent)" >&2
    failed+=("telegram_alerts")
    ;;
  no)
    echo "[PASS] Gate 3 (telegram_alerts): no firing alerts in last 7d"
    ;;
  ERROR|*)
    echo "[WARN] Gate 3 (telegram_alerts): Prometheus unreachable at $PROM_URL — cannot confirm alert state, fail-closed" >&2
    failed+=("telegram_alerts:unreachable")
    ;;
esac

# ---- Gate 4: No new player-breakage entries in docs/issues/ in last 7d ----
# GNU date supports `-d 'N days ago'`; BSD date uses `-v -Nd`. Try both.
cutoff=$(date -u -d '7 days ago' +%Y-%m-%d 2>/dev/null || date -u -v -7d +%Y-%m-%d 2>/dev/null || echo "")
if [[ -z "$cutoff" ]]; then
  echo "[WARN] Gate 4 (docs_issues): neither GNU nor BSD date supports the 7-days-ago flag; fail-closed" >&2
  failed+=("docs_issues:date-tool-missing")
else
  # Find files newer than the cutoff, filter for player-breakage keywords,
  # then drop ui-audit-* reports (those are scheduled audits, not breakage).
  matches=$(find docs/issues/ -type f -name '*.md' -newermt "$cutoff" 2>/dev/null \
    | xargs -r grep -lEi 'EnglishPlayer|english player|english tab|animepahe|gogoanime|anitaku|scraper' 2>/dev/null \
    | grep -v 'ui-audit-' \
    || true)
  if [[ -n "$matches" ]]; then
    echo "[FAIL] Gate 4 (docs_issues): potential player breakage entries since $cutoff:" >&2
    while IFS= read -r f; do echo "          $f" >&2; done <<<"$matches"
    failed+=("docs_issues")
  else
    echo "[PASS] Gate 4 (docs_issues): no new player-breakage entries since $cutoff"
  fi
fi

# ---- Final report ----
echo ""
echo "=========================================="
echo "Phase 20 Cutover Pre-flight Guardrail"
echo "=========================================="
echo "Run timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "Prometheus URL: $PROM_URL"
echo "Window: $GUARDRAIL_WINDOW; earliest ship: $EARLIEST_SHIP"
echo ""

if [[ ${#failed[@]} -eq 0 ]]; then
  echo "[PASS] All 4 gates satisfied. Cutover may proceed."
  exit 0
else
  echo "[FAIL] guardrail not met; earliest ship: $EARLIEST_SHIP"
  echo "Failed gates: ${failed[*]}"
  echo "Do NOT run Plans 20-02..20-05 until this script exits 0."
  exit 1
fi
