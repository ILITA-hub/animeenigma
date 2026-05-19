# hls3 Rotation Self-Heal Runbook

**First established:** 2026-05-19 (Phase 25 / SCRAPER-HEAL-21 / BLK-INT-01 closure).

## When to use

Trigger this runbook every time one of these conditions is observed:

- A Grafana alert with `alertname=ScraperPlayabilityRegression` + a
  `reason` label of `cdn_unreachable` is firing.
- User reports surface "EN tab plays nothing" alongside scraper logs
  showing `domain not allowed for HLS proxy` (now returned as HTTP 502
  per Phase 25 SCRAPER-HEAL-24, no longer a silent 200).
- The maintenance bot's Telegram diagnosis identifies
  `known_pattern=7` (Scraper Provider Schema Drift) + `tier=button_fix`
  + `affected_files` containing `libs/videoutils/proxy.go` or
  `HLSProxyAllowedDomains`.

## The pipeline at a glance

```
scheduler canary cron / manual POST
       │ POST /api/v1/jobs/scraper_playability_canary
       ▼
parser_unplayable_total{reason=cdn_unreachable}
playability_canary_failures_total{provider,server,anime}
       │
       ▼
Prometheus / Grafana managed-alert
ScraperPlayabilityRegression (warning)
       │ Grafana contact point "maintenance-webhook"
       ▼
services/maintenance webhook handler
POST http://host-gateway:8087/api/grafana-webhook (BasicAuth)
       │
       ▼
classifier + dispatcher (Claude CLI via .claude/maintenance-prompt.md)
       │ classifies to Pattern 6 / 7 + tier (button_fix / escalate)
       ▼
Telegram bot message to TELEGRAM_ADMIN_CHAT_ID
       │ operator reads diagnosis + clicks reaction or replies
       ▼
either:
  Path A — bot auto-commit on feature branch (operator merges)
  Path B — operator manually applies the bot's recommendation
```

## Operator step-by-step

### 0. Preflight (run once before the first execution per session)

```bash
curl -s http://localhost:8085/health        # scheduler
curl -s http://localhost:8087/health        # maintenance daemon
curl -s "http://localhost:3004/api/alertmanager/grafana/api/v2/alerts" \
  -u admin:admin | jq '.[] | {state: .status.state, name: .labels.alertname}'
grep -E "^TELEGRAM_ADMIN_CHAT_ID=" docker/.env
```

All four checks must show healthy / non-empty. If any fail, fix the
broken link first; do NOT continue.

### 1. Trigger the canary

```bash
curl -sX POST http://localhost:8085/api/v1/jobs/scraper_playability_canary \
  -w "\nHTTP_CODE=%{http_code}\n"
```

Expect HTTP 200 with `{"success":true,"data":{"status":"job triggered"}}`.

### 2. Wait ~90-120s for evaluation, then confirm alert fired

Open `http://localhost:3004/alerting/list` (Grafana → Alerting). Filter
for `ScraperPlayabilityRegression`. Expect state `Firing`.

Or via API:

```bash
curl -s "http://localhost:3004/api/alertmanager/grafana/api/v2/alerts" \
  -u admin:admin | jq '.[] | select(.labels.alertname=="ScraperPlayabilityRegression")'
```

### 3. Verify Telegram diagnosis

In the configured Telegram chat (`TELEGRAM_ADMIN_CHAT_ID`), the
maintenance bot posts the classified diagnosis. Confirm:

- `known_pattern` is `Pattern 7` / `7` / `Scraper Provider Schema
  Drift` (or a clearly equivalent name).
- `tier` is `button_fix` for cdn_unreachable.
- `affected_files` mentions `libs/videoutils/proxy.go` OR
  `HLSProxyAllowedDomains`.

### 4. Apply the bot's proposal — Path A (preferred)

If the bot's auto-commit lane is wired:

1. `git fetch --all` and look for a branch matching
   `maintenance-bot/heal-blk-int-01-*` or the bot's naming convention.
2. `git diff main..<bot-branch> -- libs/videoutils/proxy.go` — expect
   ONLY additions inside the `HLSProxyAllowedDomains` slice.
3. Fast-forward merge: `git checkout main && git merge <bot-branch> --ff-only`.
4. Push: `git push origin main`.
5. Redeploy: `make redeploy-streaming && make redeploy-scraper`.

### 4'. Apply manually — Path B (fallback)

If the bot's auto-commit lane is incomplete (current state as of
2026-05-19; tracked as a v3.2+ hardening), the operator edits
manually following the bot's recommendation literally:

1. Open `libs/videoutils/proxy.go`, locate
   `HLSProxyAllowedDomains = []string{...}`.
2. Add the rotated hosts from the bot's diagnosis (NOT from this
   runbook — the rotation is live and the values in any document
   become stale within weeks).
3. Commit with a message that names the diagnosis as the proximate
   cause:

```
fix(allowlist): add hls3 rotation hosts surfaced by self-heal pipeline

Live canary trigger on YYYY-MM-DD HH:MM produced
ScraperPlayabilityRegression alert; maintenance-bot Telegram diagnosis
identified known_pattern=7, tier=button_fix,
affected_files=[libs/videoutils/proxy.go]. Applying the recommended
fix manually because the bot's auto-commit lane is not yet wired.

Hosts added: <list>

Refs: SCRAPER-HEAL-21, BLK-INT-01, ISS-011

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
```

4. `make redeploy-streaming && make redeploy-scraper`.

### 5. Verify the alert resolves

Re-trigger the canary (Step 1). Wait ~90-120s. The alert state should
flip to `Resolved` (or no longer appear in `alertmanager/grafana/api/v2/alerts`).

If the alert does NOT resolve, either the host names were wrong, the
rotation advanced again, or the failure mode is something other than
allowlist-rejection. Re-run the diagnosis loop.

## Abort criteria — STOP and re-plan if any of these are true

- **No Telegram message arrived within 5 minutes** after the alert
  fired → the maintenance daemon's webhook handler or Telegram client
  is broken. Run `docker logs animeenigma-maintenance` (or the
  maintenance daemon's host log). File as a follow-up issue; the
  rotation cannot self-heal via pipeline until this is fixed.
- **Telegram classification is Pattern 6 (ad_decoy)** instead of
  Pattern 7 → reason-label routing is broken. Inspect
  `services/maintenance/internal/classifier/` and the alert payload's
  `reason` label. Fix and re-run.
- **Telegram classification is `tier: escalate`** instead of
  `button_fix` → the upstream is DEAD (platform shutdown / permanent
  block), not rotated. The allowlist update is no longer the right
  fix; consider `SCRAPER_DEGRADED_PROVIDERS` env update + a new
  follow-up issue.
- **`affected_files` does NOT mention `libs/videoutils/proxy.go`** →
  the classifier's hint is wrong but you can still proceed manually.
  Note the gap as a maintenance-prompt or classifier follow-up.

## Provenance check for future audits

Every allowlist entry added via this loop should have one of these
attestations:

1. **Path A — bot commit:** commit author or co-author identifies the
   maintenance bot (`Anthropic Maintenance Bot <bot@animeenigma.local>`
   or equivalent).
2. **Path B — manual following bot:** commit message body explicitly
   references the maintenance-bot Telegram diagnosis as the proximate
   cause (canary trigger time, classifier output excerpt, or
   diagnosis fields).

A bare commit that adds an entry without either attestation is a
policy violation after this runbook is published — `git log -p
libs/videoutils/proxy.go` lets future audits flag them.

## Known limitations

- The maintenance bot's auto-commit lane (Path A) is **not yet wired**
  as of 2026-05-19. Path B (manual-following-bot) is the active path.
  Wiring Path A is tracked as a v3.2+ follow-up; see Phase 23-03
  SUMMARY and the Phase 25 dispatch state notes.
- The classifier currently selects `escalate` for some
  ScraperPlayabilityRegression alerts where the canary metric carries
  ambiguous reason labels — in those cases Path A/B should not run
  until the operator confirms the rotation is the actual failure mode
  (vs. upstream-dead).
- Subdomain matches via `strings.HasSuffix(host, "."+allowed)` in
  `isHLSDomainAllowed` cover rotating subdomains under the same
  eTLD+1, so a single allowlist entry covers many rotation steps. A
  truly new eTLD+1 is what triggers this runbook; sub-subdomain
  shifts do not.

## First execution log

| Date | Operator | Outcome |
|------|----------|---------|
| 2026-05-19 | Phase 25 execute-phase (autonomous portion) + operator-pending Telegram gate | Pipeline preflight confirmed healthy; canary triggered; ScraperPlayabilityRegression observed in Grafana (already active from prior daemon run); active_alerts AUTO-094 already exists with status `escalate` (per Abort criteria: not BLK-INT-01 in current rotation cycle). Allowlist already contains the rotated hosts (`cdn-centaurus.com`, `meadowlarkdesignstudio.cfd`, `goldenridgeproduction.shop`) added 2026-05-13 as a v3.1 audit hotfix. **Runbook published; Telegram-driven button_fix path remains gated on operator confirmation per Phase 25 Plan 25-04 Task 3.** |
