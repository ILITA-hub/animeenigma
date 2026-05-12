---
phase: 20-cutover
plan: 01
subsystem: infra
tags: [bash, prometheus, promql, guardrail, preflight, cutover]

# Dependency graph
requires:
  - phase: 17-observability
    provides: parser_requests_total + ProviderHealthStreamSegmentDown alert wired to Telegram (used by gates 2 and 3)
  - phase: 18-9anime
    provides: gogoanime provider registered with full 5-stage health metrics (subject of gate-2 traffic checks)
provides:
  - "scripts/cutover-preflight.sh: 4-gate hard guardrail enforcing >= 7 days clean EnglishPlayer production traffic before any Phase 20 deletion runs"
  - "Read-only / fail-closed gate semantics: any Prometheus timeout or zero-traffic case marks the gate failed rather than passed"
  - "Re-runnable contract that downstream Plans 20-02..20-05 invoke as their first task; if it exits non-zero, those plans must abort"
affects: [20-02-PLAN, 20-03-PLAN, 20-04-PLAN, 20-05-PLAN, cutover-PR, release-engineering]

# Tech tracking
tech-stack:
  added: []  # pure bash + curl + jq, all already present on host
  patterns:
    - "Fail-closed pre-flight script — every gate's failure mode is 'add to failed[] and continue', exit 1 at end; never passes by default"
    - "PromQL ratio queries via /prometheus/api/v1/query with --data-urlencode + jq scalar extraction"
    - "Telegram-notification proxy: query ALERTS{alertstate=\"firing\"} range-vector since Alertmanager already maps the alert to TG (Phase 17)"

key-files:
  created:
    - "scripts/cutover-preflight.sh (181 lines, executable)"
  modified: []

key-decisions:
  - "EARLIEST_SHIP hard-coded as 2026-05-19 (= 2026-05-12 + 7d, EnglishPlayer first ship commit 9e9d9a2). Documented in script header; do NOT loosen this constant — it IS the guardrail."
  - "Zero-traffic provider treated as gate-2 fail rather than gate-2 pass: 'no recorded traffic in last 7d' fails the criterion 'EnglishPlayer has served clean production traffic'."
  - "Telegram silence verified via Prometheus ALERTS{alertname=\"ProviderHealthStreamSegmentDown\"} firing history rather than the Telegram API. Phase 17 wired the alert to TG, so 'alert never fired' == 'no TG notification was sent'."
  - "Gate 4 grep keyword list (EnglishPlayer, animepahe, gogoanime, anitaku, scraper) intentionally over-broad — false positives are cheap (operator can re-classify) but false negatives would let a real breakage slip past."
  - "Excludes docs/issues/ui-audit-*.md from gate 4 — those are scheduled audits, not user-reported player breakage."
  - "Curl --max-time 10 on every Prometheus call (T-20-02 mitigation): script never blocks indefinitely; timeouts are a fail-closed condition."

patterns-established:
  - "Pre-flight guardrail script — read-only, idempotent, every Phase 20 plan re-runs it as task 1 and aborts on non-zero exit"
  - "Fail-closed default: PromQL/scan returning empty/NaN/timeout always counts as failed gate, never as pass"

requirements-completed: []  # SCRAPER-CUT-01 deliverable is code deletion (Plans 20-02+); this plan only ships the guardrail that gates CUT-01. SDK's auto-mark was reverted — see "Deviations from Plan" below.

# Metrics
duration: 2min
completed: 2026-05-12
---

# Phase 20 Plan 01: Cutover Pre-flight Guardrail Summary

**4-gate hard guardrail Bash script that blocks Phase 20 deletion until EnglishPlayer has served >= 7 days of clean production traffic (earliest legitimate ship: 2026-05-19, since EnglishPlayer first shipped 2026-05-12 via commit 9e9d9a2).**

## Performance

- **Duration:** 2 min
- **Started:** 2026-05-12T18:14:00Z
- **Completed:** 2026-05-12T18:15:48Z
- **Tasks:** 2
- **Files created:** 1

## Accomplishments

- Created `scripts/cutover-preflight.sh` (181 lines, executable, syntax-clean)
- Implemented all 4 ROADMAP-mandated guardrail gates (date, error_rate, telegram_alerts, docs_issues)
- Verified the script fails today (2026-05-12) with the documented message — gate is enforceable, not aspirational
- Verified fail-closed behavior when Prometheus is unreachable (PROM_URL=http://127.0.0.1:9 still exits 1, no crash)
- Established the read-only / idempotent contract every downstream Phase 20 plan must respect

## Task Commits

1. **Task 1: Create executable cutover-preflight.sh** — `3964c0b` (feat)
2. **Task 2: Smoke-run the script on today's date (must fail)** — verification only, no code change required (script worked correctly on first run; per task instructions, "Do NOT commit any change that makes the script exit 0 today")

**Plan metadata commit:** (final docs commit follows this summary)

## The 4 Guardrail Gates

### Gate 1 — Date
- **Check:** `today=$(date -u +%Y-%m-%d); [[ "$today" < "$EARLIEST_SHIP" ]]`
- **Constant:** `EARLIEST_SHIP="2026-05-19"` (= EnglishPlayer first ship 2026-05-12 + 7 days)
- **Today (2026-05-12) result:** FAIL → correct (the gate is intended to fail until 2026-05-19)

### Gate 2 — Per-provider error rate <= 5% over 7d
- **PromQL:** `sum(rate(parser_requests_total{parser="$provider",status="error"}[7d])) / sum(rate(parser_requests_total{parser="$provider"}[7d]))`
- **Providers iterated:** `animepahe`, `gogoanime`
- **Threshold:** 0.05 (5%)
- **Empty/NaN result:** counted as fail (no traffic == criterion not satisfied)
- **Today's result:** WARN (no recorded traffic — expected, EnglishPlayer just shipped)

### Gate 3 — Zero Telegram alerts fired in 7d
- **PromQL:** `max_over_time(ALERTS{alertname="ProviderHealthStreamSegmentDown",alertstate="firing"}[7d])`
- **Mechanism:** Phase 17 17-04 wired this alert to Telegram via Alertmanager. If the alert never reached firing state, no Telegram message was sent.
- **Today's result:** PASS (no firing in last 7d)

### Gate 4 — No new player-breakage entries in docs/issues/ in last 7d
- **Check:** `find docs/issues/ -type f -name '*.md' -newermt "$cutoff"` filtered by `grep -lEi 'EnglishPlayer|english player|english tab|animepahe|gogoanime|anitaku|scraper'` then `grep -v 'ui-audit-'`
- **Cutoff:** 7 days ago (GNU `date -d` with BSD `date -v` fallback)
- **Today's result:** FAIL (docs/issues/README.md matches — expected during active Phase 18/19/20 work)

## Files Created/Modified

- `scripts/cutover-preflight.sh` — Wave 0 hard guardrail; the script every subsequent Phase 20 plan must invoke first

## Decisions Made

- Hard-coded `EARLIEST_SHIP="2026-05-19"` rather than computing it from a stored "first ship date" — single source of truth, easy for an operator to audit, and the gate is unambiguous.
- Wave 0 plan's verify clause is intentionally adversarial (`! bash scripts/cutover-preflight.sh ... && grep -q "guardrail not met"`) — it confirms the gate works by ensuring the script fails today.
- Gate 2 uses `parser_requests_total` (labelled `parser=...`, `status=...`) rather than `provider_health_up` because the former counts real production traffic (the criterion is "served clean traffic", not just "health probes passed").
- Gate 3 uses ALERTS firing history instead of the Telegram Bot API — no upstream Telegram API call is needed, and Prometheus is the source of truth Alertmanager already drives.
- Did not add a `--force-pass` override flag. Operator can edit the script if they truly need to bypass; introducing a flag would normalize bypassing.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Reverted SDK auto-completion of SCRAPER-CUT-01 requirement**
- **Found during:** State updates after SUMMARY creation
- **Issue:** Plan 20-01 frontmatter lists `requirements: [SCRAPER-CUT-01]`, so `gsd-sdk query requirements.mark-complete SCRAPER-CUT-01` checked the requirement box and flipped the traceability row to "Complete". But SCRAPER-CUT-01's actual deliverable is the deletion of the HiAnime + Consumet Go parsers — which happens in Plans 20-02 and 20-04, NOT in this plan. Plan 20-01 only ships the gate that controls CUT-01.
- **Fix:** Reverted the checkbox to `[ ]` in REQUIREMENTS.md, added a note that the guardrail is in place but deletion is still pending. Updated the traceability table cell from "Complete" to "Guardrail in place (Plan 20-01); deletion pending Plans 20-02+ on/after 2026-05-19".
- **Files modified:** `.planning/REQUIREMENTS.md`
- **Verification:** `grep "SCRAPER-CUT-01"` confirms `[ ]` checkbox and accurate status text
- **Committed in:** plan-metadata commit (this commit)

**2. [Note - SDK schema mismatch] STATE.md does not contain Performance Metrics or Decisions sections**
- **Found during:** State updates
- **Issue:** `gsd-sdk query state.record-metric` and `state.add-decision` returned "section not found in STATE.md". The project uses a hand-crafted STATE.md structure rather than the SDK-default sections.
- **Fix:** None required — the metrics + decisions are captured in this SUMMARY.md frontmatter and body, which IS the canonical record. `state.advance-plan`, `state.update-progress`, and `state.record-session` ran successfully.
- **Impact:** Zero — Performance Metrics aren't part of any downstream contract; SUMMARY frontmatter is the source of truth.

---

**Total deviations:** 1 auto-fix (Rule 1 — correctness/accuracy of requirement-tracking) plus 1 SDK-schema observation.
**Impact on plan:** No scope change. The auto-fix preserves the integrity of the requirement traceability table — marking CUT-01 complete now would have falsely signaled that the deletion already shipped.

## Issues Encountered

None.

## User Setup Required

None — script uses only tools already installed on the host (`curl`, `jq`, `find`, `date`, `git`, `bash`). No env vars to set; `PROM_URL` defaults to the on-host Prometheus at `http://localhost:9090/prometheus`.

## Contract for Downstream Plans

Every Plan 20-02..20-05 MUST begin with:

```bash
bash scripts/cutover-preflight.sh || { echo "Guardrail not met — aborting plan execution"; exit 1; }
```

If the script exits non-zero, the executor of that plan MUST refuse to proceed — no exceptions, no overrides. This is how ROADMAP v3.0 success criterion 1 is mechanically enforced.

## Idempotency

The script is read-only:
- No `rm`, `git rm`, or `git push`
- No DB writes
- No Redis writes
- No filesystem writes (output is stdout/stderr only)

It can be run an arbitrary number of times in any order without side effects, which is exactly what every downstream plan's pre-flight needs.

## Self-Check: PASSED

- File exists: `/data/animeenigma/scripts/cutover-preflight.sh` ✓
- File executable: `test -x` ✓
- Syntax clean: `bash -n` ✓
- Line count: 181 (>= 80 required) ✓
- Contains `earliest ship: 2026-05-19`: ✓
- Contains `parser_requests_total`: ✓
- Contains `ProviderHealthStreamSegmentDown`: ✓
- Contains `docs/issues`: ✓
- Smoke run today exits 1 with documented message: ✓
- Fail-closed on unreachable Prometheus (PROM_URL=http://127.0.0.1:9): ✓
- Task 1 commit exists: `3964c0b` ✓

## Next Phase Readiness

- **Plan 20-02 onward:** Cannot proceed until 2026-05-19 at earliest. Each subsequent plan must invoke this script and abort on non-zero exit.
- **On or after 2026-05-19:** Operator re-runs the script. If all 4 gates pass (real production traffic landed, no alerts fired, no breakage reports filed), exit code becomes 0 and cutover may proceed.
- **No blockers** for waiting — Phase 19 (AnimeKai-gated) is still pending its own evaluation per ROADMAP, and the cutover deliberately waits on production telemetry.

---
*Phase: 20-cutover*
*Plan: 01*
*Completed: 2026-05-12*
