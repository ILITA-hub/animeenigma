# Phase 25: Audit Findings Resolution — Context

**Gathered:** 2026-05-19
**Status:** Ready for planning (`/gsd-plan-phase --phase 25`)
**Milestone:** v3.1 Scraper Self-Healing (REOPENED 2026-05-19)
**Spec:** `.planning/milestones/v3.1-REQUIREMENTS.md` (SCRAPER-HEAL-21..24)
**Source audit:** `.planning/milestones/v3.1-MILESTONE-AUDIT.md` (now annotated as superseded; original findings preserved verbatim)

<domain>
## Phase Boundary

Close every gap the 2026-05-13 milestone audit surfaced. The audit verdict was "gaps_found (with strong implementation foundation)" — the self-healing infrastructure ships and is structurally sound, but four observable issues were logged and deferred. Phase 25 makes them go away.

**Concretely, this phase delivers:**

1. **BLK-INT-01 closed (SCRAPER-HEAL-21).** The deferred Task 4 manual smoke of Plan 23-03 (`infra/grafana/alerts/scraper.yaml` end-to-end webhook dispatch) runs: operator manually triggers the canary, watches Grafana's `ScraperPlayabilityRegression` alert state transition, confirms the maintenance bot's Telegram diagnosis arrives with the right `known_pattern` (Pattern 7) + `tier` (`button_fix`) + `affected_files` (`libs/videoutils/proxy.go`). Once the loop is proven on synthetic-then-live, the live hls3 hosts captured 2026-05-19 (`cdn-centaurus.com`, `meadowlarkdesignstudio.cfd`) are added to `HLSProxyAllowedDomains` via the maintenance-bot proposal flow — not by direct human edit. The objective is to *exercise* the self-healing pipeline, not just code-update the allowlist.
2. **W-INT-01 closed (SCRAPER-HEAL-22).** `services/scraper/internal/providers/gogoanime/client_gated_test.go::TestGetStreamWithGate_AdDecoy_Skipped` rewritten so the parallel-probe race (parCancel fires on first success before the AdDecoy probe's counter Inc completes) no longer causes a 5/5 reproducible failure. Test-only fix; implementation at `client.go:881-887` is correct and stays unchanged. Suggested approach: swap priority order so the AdDecoy server is in position 2 (sequential remainder branch where there's no race) — OR add a sync barrier on the counter Inc completion before the assertion.
3. **W-INT-02 closed (SCRAPER-HEAL-23).** `.claude/maintenance-prompt.md` Pattern 7 troubleshooting block updated to reference the actual scraper-side function name. The audit observed the prompt references a `cacheStream` symbol that doesn't exist in the scraper codebase (symbol-stability test passes via slash-alternative semantics, so CI is green, but the human-facing guidance text is stale). One-line text fix.
4. **W-INT-03 closed (SCRAPER-HEAL-24).** `services/streaming/internal/handler` HLS-proxy handler returns HTTP 502 on the "domain not allowed for HLS proxy" code path instead of the current silent HTTP 200 / Content-Length 0. The current behavior makes Pattern 7 button_fix triage harder because users see a "playing nothing" rather than a clear error — and the canary doesn't detect it as a failure. Switching to 502 makes the failure observable both in user reports and in canary metrics, improving the self-healing loop's signal quality.

**Out of scope:**

- Adding new providers (Phase 26).
- Restoring EnglishPlayer (Phase 24).
- Switching the maintenance-bot from human-acknowledge-then-apply to fully autonomous allowlist updates (separate future spec; the trust model for self-modifying production config is not yet established).

**Requirements covered:** SCRAPER-HEAL-21, SCRAPER-HEAL-22, SCRAPER-HEAL-23, SCRAPER-HEAL-24.

</domain>

<decisions>
## Implementation Decisions

### D1 — BLK-INT-01 closure goes through the self-healing pipeline, NOT a direct edit

The simplest fix is `Edit libs/videoutils/proxy.go to add the rotated hosts`. We deliberately do NOT take that shortcut. The whole point of v3.1 is that hls3 host rotation is *expected and frequent* (Phase 22 hosts already rotated by Phase 23 ship date — the audit observed `strategicplanning.sbs` two weeks after `managementadvisory.sbs` was added). The self-healing loop must demonstrably handle this rotation, or v3.1's premise is unsupported.

So Phase 25's plan for SCRAPER-HEAL-21:
1. Verify the canary + alert + maintenance-bot pipeline works end-to-end (the deferred Task 4 from Plan 23-03).
2. Use that pipeline to add the rotated hosts.
3. Document the operator runbook for future rotations: "trigger canary, watch alert, accept maintenance-bot proposal."

Trade-off: takes longer than a 1-line code edit. Worth it — the operator learns the loop, and the loop's correctness is proven.

### D2 — W-INT-01 fix is test-only; production code stays untouched

The audit was explicit: "Implementation is sound; only the test that locks the contract is broken." The implementation at `services/scraper/internal/providers/gogoanime/client.go:881-887` correctly Incs both `parser_unplayable_total{reason=ad_decoy}` AND `parser_ad_decoy_total` when the AdDecoy probe completes. The race is in the test's assertion timing.

We do NOT add `runtime.Gosched()` or `time.Sleep(50ms)` hacks to production code to "fix" a race. The production code is correct. The fix lives in the test.

### D3 — W-INT-03 returns 502 specifically (not 503, not 4xx)

Reason: 502 ("Bad Gateway") accurately describes the failure ("upstream gave us a URL on a domain we can't proxy"). 503 implies temporary unavailability (wrong — the domain is permanently blocked from our allowlist). 4xx implies caller error (wrong — the caller passed a legitimate request; the upstream is what's broken). 502 is what a reverse proxy returns when a backend hands it an invalid response, which is precisely the situation.

Side effect: any existing client that treats 200-with-empty-body as "no content" now starts seeing 502s. The FE error boundary in EnglishPlayer (restored in Phase 24) handles HTTP errors gracefully, so this is a positive observable: a real error becomes a real error message.

### D4 — Phase 25 ships independently of Phase 24

The four audit fixes are backend / infra / tooling — none of them require EnglishPlayer to exist. Phase 25 can ship before, during, or after Phase 24 without coupling. Recommendation: ship Phase 24 first (user-visible regression repair is the higher-priority win), then Phase 25 immediately after to close the audit.

### D5 — Phase 25 does NOT re-run the full milestone audit

Re-running `/gsd-audit-milestone` after the four fixes ship would produce a new audit document with potentially new gaps. That's good hygiene but not in Phase 25's scope. The supersedes-trace in `v3.1-MILESTONE-AUDIT.md` is the right place to point future audits at; running a fresh audit is a Phase 27+ concern (or a `/gsd-audit-milestone --milestone v3.1` re-invocation by the operator when the full milestone is ready to close).

</decisions>

<open_questions>
None — all four findings are well-scoped by the original audit's `mitigation` / `suggested_fix` fields. Implementation discoveries belong to `/gsd-plan-phase`.
</open_questions>

<risks>
## Risks specific to this phase

- **The maintenance bot's Pattern 7 dispatch doesn't actually self-heal as designed**: Plan 23-03 Task 4 was deferred for a reason — nobody has run the live end-to-end test. SCRAPER-HEAL-21 may discover that the bot proposes the right fix but the proposal doesn't auto-apply (or applies to the wrong file, or runs against stale repo state). Mitigation: SCRAPER-HEAL-21's plan must include a "what if the loop is broken" branch — debug + fix becomes part of the phase, not punted.
- **hls3 hosts rotate again between Phase 25 plan-time and ship-time**: by the day SCRAPER-HEAL-21 ships, `cdn-centaurus.com` may have rotated to a new host. Mitigation: the operator re-runs the canary at plan-time and updates the target host names. The pipeline being exercised matters; the specific host names will be stale within weeks regardless.
- **W-INT-01 test rewrite uncovers a real race in production**: if the "fix the test" attempt reveals that the production code has a latent race (the counter Inc IS dropped under some real-world scheduler condition), then the implementation has to change after all. Mitigation: re-run the race detector against the modified test to confirm — `go test -race` should be green for both the test fix AND a sanity-check synthetic that exercises the production path with both servers swapping winner positions.
- **Switching streaming-handler 200→502 breaks a downstream consumer**: any frontend or third-party consumer that polls the HLS proxy and treats 200/0 as "no segments yet, retry" would suddenly see 502 and may surface a hard error. Mitigation: grep frontend + scheduler for `status === 200 &&` checks against HLS-proxy responses; if any consumer treats empty 200 as transient, switch it to retry-on-502 explicitly.
</risks>

<dependencies>
## Phase Dependencies

- **Hard dependency on:** v3.1 Phase 23 (canary + alert + maintenance-bot dispatch infrastructure already shipped).
- **No dependency on:** Phase 24 (EN reconnect — independent), Phase 26 (provider expansion — independent).
- **Blocked by:** nothing.
- **Blocks:** v3.1 milestone-audit re-run (operator's decision when to invoke `/gsd-audit-milestone --milestone v3.1` again — typically after all three new phases ship).
</dependencies>

<plan_sketch>
## Plan Sketch (for `/gsd-plan-phase` to flesh out)

**Wave 1 — Parallel quick wins (W-INT-01, W-INT-02, W-INT-03)**

- 25-01-PLAN.md: Fix `TestGetStreamWithGate_AdDecoy_Skipped` race (test-only). `go test -race -count=10` on the rewritten test must be 10/10 green. SCRAPER-HEAL-22.
- 25-02-PLAN.md: One-line text fix in `.claude/maintenance-prompt.md` Pattern 7 — replace `cacheStream` reference with actual function name. Symbol-stability test stays green under new content. SCRAPER-HEAL-23.
- 25-03-PLAN.md: Streaming handler "domain not allowed" path returns 502 with descriptive JSON body. Update or add unit test asserting status code + body shape. `make redeploy-streaming` + curl smoke. SCRAPER-HEAL-24.

**Wave 2 — BLK-INT-01 end-to-end self-heal (operator-driven)**

- 25-04-PLAN.md: Manual operator smoke per Plan 23-03 Task 4 — trigger canary → observe Grafana → confirm maintenance-bot Telegram diagnosis → accept `button_fix` proposal that adds the rotated hls3 hosts. Phase is GREEN only after a `git log` shows a maintenance-bot-attributed commit landing `cdn-centaurus.com` + `meadowlarkdesignstudio.cfd` (or whatever the live rotation is at ship time) into `HLSProxyAllowedDomains`. SCRAPER-HEAL-21.
- Document the runbook for future hls3 rotations in `docs/issues/README.md` (ISS-011 entry update OR new ISS-012 entry — operator's choice based on whether the rotation closes ISS-011 or extends it).
</plan_sketch>
