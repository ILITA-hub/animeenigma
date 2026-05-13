---
id: 23-03
phase: 23
plan: "03"
type: execute
wave: 3
depends_on:
  - 23-01
  - 23-02
files_modified:
  - infra/grafana/alerts/scraper.yaml
  - infra/grafana/alerts/README.md
  - docker/grafana/provisioning/alerting/rules.yml
  - docker/grafana/provisioning/alerting/contactpoints.yml
  - docker/docker-compose.yml
  - services/maintenance/internal/transport/webhook_synthetic_test.go
  - services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go
  - .claude/maintenance-prompt.md
  - frontend/web/public/changelog.json
requirements:
  - SCRAPER-HEAL-15
  - SCRAPER-HEAL-16
autonomous: false
tags: [grafana, alerts, prometheus, maintenance, synthetic-test, dispatch, after-update]

must_haves:
  truths:
    - "Three Prometheus/Grafana alert rules exist in `infra/grafana/alerts/scraper.yaml`: ScraperPlayabilityRegression (warning, 25h window over canary fail), ScraperAdDecoySurge (warning, rate(parser_ad_decoy_total[5m]) > 0 sustained 5m), ScraperUnplayableSpike (critical, rate ratio > 5% sustained 5m)"
    - "All three alerts carry labels { severity, provider, server, reason } so the maintenance-bot reason-enum dispatch table can match"
    - "All three alerts route to the existing `services/maintenance` `/api/grafana-webhook` contact point — no new contact point definition needed; rule's `contactPoint` field references the existing one by name"
    - "Synthetic Pattern 6 alert payload (provider=gogoanime, server=vibeplayer, reason=ad_decoy) posted to /api/grafana-webhook unmarshals into the GrafanaWebhookPayload struct without error AND the request returns 202 Accepted in test mode"
    - "Synthetic Pattern 7 alert payload (provider=gogoanime, server=streamhg, reason=zero_match) similarly unmarshals + returns 202"
    - "`.claude/maintenance-prompt.md` still contains Pattern 6 + Pattern 7 + 'Scraper Playability Regression' sections + reason-enum dispatch table (SCRAPER-HEAL-16 verification — no edits, only assertion)"
    - "Symbol-stability test asserts: `cacheStream` AND `computeStreamTTL` Go symbols exist in `services/scraper/internal/providers/gogoanime/`; all 7 Reason enum values from libs/streamprobe.AllReasons() appear textually in .claude/maintenance-prompt.md (so dispatch-table coverage is current)"
    - "MAINTENANCE_TEST_MODE env var (or equivalent) added so synthetic tests can hit the webhook in dry-run mode without triggering a real Claude dispatch"
    - "Final task invokes /animeenigma-after-update which redeploys scheduler + reloads grafana + reloads prometheus + updates changelog.json + commits + pushes"
  artifacts:
    - path: infra/grafana/alerts/scraper.yaml
      provides: "Three Grafana unified-alerting rules in YAML, group 'Scraper Self-Healing', interval 1m, all routing to webhook contact point"
      contains: "ScraperPlayabilityRegression"
    - path: infra/grafana/alerts/README.md
      provides: "Brief doc explaining infra/grafana/alerts/ is the source-of-truth for v3.1 scraper alerts; copy-deployed into docker/grafana/provisioning/alerting/ via volume mount + provider config; production via deploy/kustomize"
      contains: "scraper.yaml"
    - path: docker/grafana/provisioning/alerting/rules.yml
      provides: "Extends the existing rules.yml by APPENDing the three new alert rules to the same group structure (or references the infra/ file via a second provisioning provider)"
      contains: "ScraperPlayabilityRegression"
    - path: services/maintenance/internal/transport/webhook_synthetic_test.go
      provides: "TestWebhook_SyntheticPattern6_Accepted + TestWebhook_SyntheticPattern7_Accepted + TestWebhook_DispatchedPayload_HasRequiredLabels — uses httptest + the real webhookHandler; asserts unmarshal + 202 + dispatcher callback receives labels"
      contains: "TestWebhook_Synthetic"
    - path: services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go
      provides: "TestMaintenancePrompt_ContainsPatterns6And7 + TestMaintenancePrompt_AllReasonsCovered + TestScraperGoSymbols_StillExist — reads .claude/maintenance-prompt.md + greps services/scraper for cacheStream + computeStreamTTL"
      contains: "TestMaintenancePrompt"
    - path: docker/docker-compose.yml
      provides: "maintenance service gets MAINTENANCE_TEST_MODE optional env var documented as set-by-test-only; mounts infra/grafana/alerts/ into grafana so the new rules are auto-loaded"
      contains: "infra/grafana/alerts"
    - path: frontend/web/public/changelog.json
      provides: "User-facing changelog entry announcing v3.1 self-healing canary is live"
      contains: "Self-Healing"
  key_links:
    - from: infra/grafana/alerts/scraper.yaml
      to: services/maintenance /api/grafana-webhook
      via: "contactPoint: webhook reference matching the name already declared in docker/grafana/provisioning/alerting/contactpoints.yml"
      pattern: "grafana-webhook"
    - from: services/maintenance/internal/transport/webhook_synthetic_test.go
      to: services/maintenance/internal/transport/webhook.go
      via: "httptest of webhookHandler with the synthetic payload constructed against domain.GrafanaWebhookPayload"
      pattern: "webhookHandler"
    - from: services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go
      to: .claude/maintenance-prompt.md + services/scraper/internal/providers/gogoanime/
      via: "os.ReadFile + grep for symbol stability"
      pattern: "cacheStream"
---

<objective>
Ship the three Prometheus alert rules that turn the canary's `playability_canary_runs_total` counter and the production `parser_ad_decoy_total` / `parser_unplayable_total` counters into actionable Telegram-routed alerts. Verify the maintenance bot's prompt + dispatcher chain still parses and accepts both synthetic Pattern 6 and Pattern 7 payloads end-to-end. Lock in symbol stability so the maintenance-prompt's reason-enum dispatch table cannot drift out of sync with the actual Go code without a test failure. Final task: full `/animeenigma-after-update` — scheduler redeploy + grafana reload + changelog + commit + push.

This is the plan that closes the loop: canary detects → alert fires → maintenance bot receives → known-pattern fix proposed. Covers SCRAPER-HEAL-15 + SCRAPER-HEAL-16.

Purpose: Without alerts, the dashboard from Plan 23-02 is purely passive. The three alert thresholds were tuned by the spec (§4.3.b): "any canary fail in 25h" catches one missed nightly + tolerates late starts; "ad_decoy rate > 0 sustained 5m" catches in-prod ad-decoy regressions inside a single TV-watching window; "unplayable rate > 5% of get_stream rate sustained 5m" catches structural breakage with a tight enough ratio to ignore single-user transient failures. The synthetic-alert test in this plan is the explicit verification step the spec calls out (§8 acceptance criterion 3): "Alert fires → maintenance bot receives → tiers correctly per Pattern 6/7."

Output:
- `infra/grafana/alerts/scraper.yaml` — three alert rules in Grafana unified-alerting YAML syntax (matching the existing `docker/grafana/provisioning/alerting/rules.yml` shape).
- Provisioning wiring so Grafana auto-loads the new rules.
- Two test files in services/maintenance asserting (a) synthetic payloads dispatch correctly, (b) maintenance-prompt + Go symbol pair stays in sync.
- D6 hard rule: `.claude/maintenance-prompt.md` is NOT edited. Only an assertion that the existing content still parses.
- Final after-update step: scheduler redeploy, grafana reload, prometheus reload, changelog entry, commit, push.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/23-self-maintenance-loop/23-CONTEXT.md
@docs/plans/2026-05-13-scraper-self-healing-spec.md
@.planning/phases/23-self-maintenance-loop/23-01-canary-cron-PLAN.md
@.planning/phases/23-self-maintenance-loop/23-02-grafana-dashboard-PLAN.md
@CLAUDE.md

<interfaces>
<!-- From Plan 23-01: counter that fuels ScraperPlayabilityRegression -->
```
playability_canary_runs_total{provider, server, result, reason, anime_slot}
```

<!-- From Phase 21: counters that fuel ScraperAdDecoySurge + ScraperUnplayableSpike -->
```
parser_ad_decoy_total{provider, server}
parser_unplayable_total{provider, server, reason}
parser_requests_total{provider, operation, status}   # denominator for spike ratio (operation="get_stream")
```

<!-- Maintenance webhook surface (services/maintenance/internal/transport/webhook.go): -->
```go
// webhookHandler validates BasicAuth + JSON-unmarshals into domain.GrafanaWebhookPayload
// + calls submitAlert(payload) + returns 202 Accepted with {"status":"accepted"}.

func webhookHandler(submitAlert AlertEventFunc, webhookUser, webhookPass string) http.HandlerFunc
```

<!-- Existing Grafana provisioning rules.yml structure (docker/grafana/provisioning/alerting/rules.yml): -->
```yaml
apiVersion: 1
groups:
  - orgId: 1
    name: AnimeEnigma Alerts
    folder: AnimeEnigma
    interval: 1m
    rules:
      - uid: service-unreachable
        title: Service Unreachable
        condition: C
        data:
          - refId: A
            relativeTimeRange: { from: 300, to: 0 }
            datasourceUid: PBFA97CFB590B2093
            model: { expr: up, instant: true, refId: A }
          # ... reduce + threshold refs ...
        for: 2m
        labels: { severity: critical }
        annotations: { summary: "Service {{ $labels.job }} is unreachable" }
```
The new file MUST use the same shape (refId: A + reduce/threshold expr refs, datasourceUid placeholder, `for:` duration, labels, annotations). The Prometheus datasourceUid placeholder in current rules.yml is `PBFA97CFB590B2093`.

<!-- Existing contact point (docker/grafana/provisioning/alerting/contactpoints.yml) — the new alert rules will reference its name; do not redefine: -->
```bash
grep -E "name:|type:" docker/grafana/provisioning/alerting/contactpoints.yml
```
The maintenance webhook contact point is named (verify by reading the file during execution; likely `grafana-webhook` or `maintenance-bot`).

<!-- Maintenance-prompt sections that MUST still parse: -->
- `### Pattern 6: Scraper Provider Ad-Decoy Poisoning (VibePlayer)`
- `### Pattern 7: Scraper Provider Schema Drift (anitaku / packed-JS rotation)`
- `### Scraper Playability Regression (WARNING / CRITICAL)`
- The reason-enum dispatch list (numbered 1-6) under that section

<!-- Go symbols the maintenance-prompt's "signed_url_expired → search cacheStream / computeStreamTTL" guidance references — these must still exist in the scraper service or the prompt is pointing at dead code: -->
- `cacheStream` (substring should appear in some .go file in services/scraper/internal/providers/gogoanime/)
- `computeStreamTTL` (same)
- Note: if either name was refactored in a later phase, this test FAILS — and the fix is either to rename back, or to update the prompt's hint string. Either is acceptable as a follow-up; the failing test is the alarm.

<!-- D6 — maintenance-prompt is NOT edited in this phase. Only asserted. If the assertion fails, the failure mode is "raise a P-23 follow-up to amend the prompt", NOT "patch it silently here". -->
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create infra/grafana/alerts/scraper.yaml + provisioning wiring + docker-compose mount</name>
  <files>infra/grafana/alerts/scraper.yaml, infra/grafana/alerts/README.md, docker/grafana/provisioning/alerting/rules.yml, docker/docker-compose.yml</files>
  <read_first>
    - docker/grafana/provisioning/alerting/rules.yml (full file — the existing `service-unreachable` rule is the canonical shape: condition: C, data: [refId A query, refId B reduce, refId C threshold], for:, labels:, annotations:. Match it exactly for the new rules.)
    - docker/grafana/provisioning/alerting/contactpoints.yml (full file — find the name of the webhook contact point that points at services/maintenance /api/grafana-webhook; the new alert rules reference it via name, not redefine it)
    - docker/grafana/provisioning/alerting/policies.yml (full file — verify which severity-label routes exist; if `warning` and `critical` are not already mapped to the webhook receiver, append routes that map them)
    - docs/plans/2026-05-13-scraper-self-healing-spec.md §4.3.b (alert thresholds verbatim)
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md §domain bullet #7 (label contract: provider, server, reason MUST be present)
    - libs/streamprobe/reason.go (Reason enum values — used as `reason` label values in alert annotations to format the human-readable message)
  </read_first>
  <behavior>
    - `yq eval '.groups[0].name' infra/grafana/alerts/scraper.yaml` returns `Scraper Self-Healing`.
    - `yq eval '[.groups[0].rules[].title] | sort' infra/grafana/alerts/scraper.yaml` returns the sorted list `[ScraperAdDecoySurge, ScraperPlayabilityRegression, ScraperUnplayableSpike]`.
    - `yq eval '.groups[0].rules[] | select(.title=="ScraperPlayabilityRegression") | .labels.severity' infra/grafana/alerts/scraper.yaml` returns `warning`.
    - `yq eval '.groups[0].rules[] | select(.title=="ScraperAdDecoySurge") | .labels.severity' infra/grafana/alerts/scraper.yaml` returns `warning`.
    - `yq eval '.groups[0].rules[] | select(.title=="ScraperUnplayableSpike") | .labels.severity' infra/grafana/alerts/scraper.yaml` returns `critical`.
    - All three rules include `provider` and `server` and `reason` keys inside their `annotations` block (rendered from `{{ $labels.provider }}` etc.) so the maintenance bot's dispatch table can match.
    - `for:` duration on ScraperPlayabilityRegression is `25h` (covers nightly + late starts per spec); on ScraperAdDecoySurge is `5m`; on ScraperUnplayableSpike is `5m`.
    - The `expr` (refId A `model.expr`) for ScraperPlayabilityRegression is `sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[25h]))`.
    - The `expr` for ScraperAdDecoySurge is `sum by (provider, server) (rate(parser_ad_decoy_total[5m]))`.
    - The `expr` for ScraperUnplayableSpike is `sum by (provider, server, reason) (rate(parser_unplayable_total[5m])) / scalar(sum(rate(parser_requests_total{operation="get_stream",status="success"}[5m])) > 0)` OR an equivalent ratio expression — the verifier checks the literal substring `rate(parser_unplayable_total` AND `rate(parser_requests_total` AND `0.05` are all present in the ScraperUnplayableSpike rule body.
    - `grep -c "infra/grafana/alerts" docker/docker-compose.yml` returns ≥ 1 (mount added).
    - `docker compose -f docker/docker-compose.yml config > /dev/null` exits 0.
    - infra/grafana/alerts/README.md exists and references scraper.yaml.
  </behavior>
  <action>
    1. **Read existing `docker/grafana/provisioning/alerting/contactpoints.yml`** and capture the name of the webhook contact point (e.g., `grafana-webhook` or `maintenance-webhook`). Use this exact name in the new alert rules' route configuration. If the contact point does NOT yet point at `services/maintenance /api/grafana-webhook`, append a new contact point (HTTP webhook) targeting `http://maintenance:8090/api/grafana-webhook` with BasicAuth set from the same env vars services/maintenance reads (search `services/maintenance/internal/config/config.go` for the env var names; commonly `MAINTENANCE_WEBHOOK_USER` / `MAINTENANCE_WEBHOOK_PASS`).
    2. **Create `infra/grafana/alerts/scraper.yaml`** in Grafana unified-alerting YAML syntax. The shape MUST match the existing `docker/grafana/provisioning/alerting/rules.yml` (so it can be loaded by the same provisioning provider). Top level:
       ```yaml
       apiVersion: 1
       groups:
         - orgId: 1
           name: Scraper Self-Healing
           folder: AnimeEnigma
           interval: 1m
           rules:
             - uid: scraper-playability-regression
               title: ScraperPlayabilityRegression
               condition: C
               data:
                 - refId: A
                   relativeTimeRange: { from: 90000, to: 0 }   # 25 hours in seconds
                   datasourceUid: PBFA97CFB590B2093
                   model:
                     expr: 'sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[25h]))'
                     instant: true
                     refId: A
                 - refId: B
                   relativeTimeRange: { from: 90000, to: 0 }
                   datasourceUid: __expr__
                   model:
                     conditions:
                       - evaluator: { params: [0], type: gt }
                         operator: { type: and }
                         reducer: { type: last }
                     refId: B
                     type: reduce
                     expression: A
                     reducer: last
                 - refId: C
                   relativeTimeRange: { from: 90000, to: 0 }
                   datasourceUid: __expr__
                   model:
                     conditions:
                       - evaluator: { params: [0], type: gt }
                         operator: { type: and }
                       refId: C
                       type: threshold
                       expression: B
               for: 0s   # condition is intrinsically "fail in 25h window" — no extra hold
               labels:
                 severity: warning
                 component: scraper
               annotations:
                 summary: 'Scraper canary fail: {{ $labels.provider }} / {{ $labels.server }} ({{ $labels.reason }})'
                 description: 'playability_canary_runs_total recorded a fail in the last 25h for provider={{ $labels.provider }}, server={{ $labels.server }}, reason={{ $labels.reason }}. Maintenance bot Pattern 6/7 dispatch applies.'
             - uid: scraper-ad-decoy-surge
               title: ScraperAdDecoySurge
               # ... rate(parser_ad_decoy_total[5m]) > 0, for: 5m, severity: warning ...
             - uid: scraper-unplayable-spike
               title: ScraperUnplayableSpike
               # ... rate(parser_unplayable_total) / rate(parser_requests_total{operation="get_stream"}) > 0.05, for: 5m, severity: critical ...
       ```
       Fill in the two remaining rules following the same shape. The ScraperUnplayableSpike `expr` SHOULD be the ratio form. Because Grafana's three-step refId pipeline (A query → B reduce → C threshold) does not naturally support ratio queries, prefer the single-query form with the ratio embedded in `model.expr` and the threshold step comparing against `0.05`:
       ```yaml
       model:
         expr: 'sum by (provider, server, reason) (rate(parser_unplayable_total[5m])) / ignoring(reason) group_left sum by (provider, server) (rate(parser_requests_total{operation="get_stream"}[5m]) > 0)'
         instant: true
       ```
       Then refId C threshold compares against `0.05` (`evaluator: { params: [0.05], type: gt }`).
       Verify the PromQL expr parses against the live Prometheus during the after-update smoke step.
    3. **Create `infra/grafana/alerts/README.md`** explaining the directory's purpose mirroring infra/grafana/dashboards/README.md from Plan 23-02; cite scraper.yaml as the first entry and note that production K8s alert provisioning uses `deploy/kustomize/base/monitoring/grafana/` (out-of-scope here).
    4. **Wire into Grafana provisioning** — TWO options; pick the lower-churn one:
       - Option A (preferred): Append the new alert rules INLINE into `docker/grafana/provisioning/alerting/rules.yml` so Grafana picks them up from the existing provider. Source-of-truth file remains `infra/grafana/alerts/scraper.yaml`; rules.yml gets the contents copied or symlinked. Choose this if alerting provisioning does NOT support multiple files in the same dir.
       - Option B: Add a second alerting provisioning entry pointing at a new path `/etc/grafana/provisioning/alerting/infra/`, mount `infra/grafana/alerts/` there. Choose this if the existing provisioning supports it (Grafana 10+ generally does).
       Verify which works by reading Grafana's alerting provisioning docs during execution; settle on Option A as the safe default and add a comment in rules.yml noting "// Block appended from infra/grafana/alerts/scraper.yaml — keep in sync".
    5. **Edit `docker/docker-compose.yml`** — append a volume mount to the grafana service block so the new alerts directory is readable by the container (regardless of which option chosen, this lets future plans pick the better path without re-mounting):
       ```yaml
         - ../infra/grafana/alerts:/var/lib/grafana/alerts/infra:ro
       ```
  </action>
  <verify>
    <automated>cd /data/animeenigma && [ -f infra/grafana/alerts/scraper.yaml ] && yq eval '.groups[0].name' infra/grafana/alerts/scraper.yaml | grep -F 'Scraper Self-Healing' && yq eval '[.groups[0].rules[].title] | sort | join(",")' infra/grafana/alerts/scraper.yaml | grep -F 'ScraperAdDecoySurge,ScraperPlayabilityRegression,ScraperUnplayableSpike' && grep -F 'rate(parser_unplayable_total' infra/grafana/alerts/scraper.yaml && grep -F 'rate(parser_requests_total' infra/grafana/alerts/scraper.yaml && grep -F '0.05' infra/grafana/alerts/scraper.yaml && grep -c 'infra/grafana/alerts' docker/docker-compose.yml && docker compose -f docker/docker-compose.yml config > /dev/null</automated>
  </verify>
  <done>infra/grafana/alerts/scraper.yaml exists with the 3 required rules, correct severity labels, correct PromQL expressions referencing the right counters, and required `provider`/`server`/`reason` labels in annotations. docker-compose mount added. docker-compose config validates.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Synthetic Pattern 6 + Pattern 7 webhook tests + MAINTENANCE_TEST_MODE plumbing</name>
  <files>services/maintenance/internal/transport/webhook_synthetic_test.go, services/maintenance/internal/config/config.go, services/maintenance/internal/transport/webhook.go, docker/docker-compose.yml</files>
  <read_first>
    - services/maintenance/internal/transport/webhook.go (full file — webhookHandler signature, BasicAuth check, JSON unmarshal into domain.GrafanaWebhookPayload, submitAlert callback shape, 202 response body)
    - services/maintenance/internal/domain/webhook.go (full file — GrafanaWebhookPayload + GrafanaWebhookAlert struct field names, JSON tags — the synthetic payload MUST match these exactly)
    - services/maintenance/internal/transport/router.go (full file — find where webhookHandler is registered + the BasicAuth env var names)
    - services/maintenance/internal/config/config.go (full file — env-var loading pattern; add MAINTENANCE_TEST_MODE here with default `false`)
    - services/maintenance/cmd/maintenance/main.go (lines around `From: domain.User{Username: "grafana-webhook"...}` — the alert injection path from webhook → classifier → dispatcher; the test stubs `submitAlert` and asserts the payload's labels arrive intact)
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md §risks bullet "Synthetic Pattern 6 test crashes the production maintenance bot" (MAINTENANCE_TEST_MODE is the documented mitigation)
  </read_first>
  <behavior>
    - Test `TestWebhook_SyntheticPattern6_Accepted`: build a `GrafanaWebhookPayload` with one alert having `Labels: { alertname: "ScraperAdDecoySurge", severity: "warning", provider: "gogoanime", server: "vibeplayer", reason: "ad_decoy" }` + `Status: "firing"`; POST it to httptest-wrapped webhookHandler (with valid BasicAuth); response is 202 Accepted; the test-captured `submitAlert` callback received exactly that payload (provider/server/reason labels present and equal to the expected values).
    - Test `TestWebhook_SyntheticPattern7_Accepted`: similar but `alertname: "ScraperPlayabilityRegression"` + `server: "streamhg"` + `reason: "zero_match"`; same assertions.
    - Test `TestWebhook_RequiredLabels_PresentInDispatched`: synthetic payload missing one of `provider` / `server` / `reason` labels — the handler still returns 202 (Grafana's contract), but the captured payload demonstrates the label is missing; the test documents the gap (this is information-only — the dispatcher should handle missing labels gracefully; if it doesn't, that's a downstream fix tracked separately).
    - Test `TestMaintenanceConfig_TestModeDefault`: `config.Load()` with no env returns `TestMode == false`; `t.Setenv("MAINTENANCE_TEST_MODE", "true")` returns `TestMode == true`.
    - Behavior assertion (documented, not enforced by the test): the production webhook IS NOT hit by these tests; httptest.NewServer wraps a local instance of the same handler, so the production maintenance container is never touched.
  </behavior>
  <action>
    1. **Edit services/maintenance/internal/config/config.go** — add `TestMode bool` to the Config struct + read `MAINTENANCE_TEST_MODE` env var (default false). This is documentation + future-hook: the production dispatcher can short-circuit on `cfg.TestMode == true` if a future task wires that gate; this plan only adds the field + the unit test confirming Load behavior.
    2. **Optional (and OFF unless needed by Plan 23-01's canary tests):** Add `TestMode` short-circuit in `webhookHandler` or in the `submitAlert` callback (in main.go) so when TestMode is true, the handler still returns 202 but skips invoking the Claude dispatcher. Implement IF AND ONLY IF the synthetic tests need real dispatcher short-circuit; otherwise leave as a config-only field. Decide at execution time. Document the decision inline.
    3. **Create services/maintenance/internal/transport/webhook_synthetic_test.go** with the three test functions above. Use:
       - `httptest.NewServer(webhookHandler(captureSubmit, "user", "pass"))` to spin up the handler in-process.
       - A test-local `captureSubmit AlertEventFunc` that pushes the payload into a `chan domain.GrafanaWebhookPayload`; the test reads from the chan with a 1s timeout via `select` + `time.After`.
       - Build the synthetic payload directly as a Go struct, then `json.Marshal` it; POST via `http.NewRequest` with `req.SetBasicAuth("user", "pass")`.
       - Assert response status == 202; assert chan receives a payload within timeout; assert `payload.Alerts[0].Labels["provider"] == "gogoanime"` etc.
       - For Pattern 7 use `server="streamhg"`, `reason="zero_match"`, `alertname="ScraperPlayabilityRegression"`.
    4. **Edit docker/docker-compose.yml** — find the maintenance service block; add `MAINTENANCE_TEST_MODE: "false"` to the `environment:` list as an explicit default (makes the env var visible in container introspection without changing runtime behavior).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/maintenance && go test -count=1 -race -run 'TestWebhook_Synthetic|TestWebhook_RequiredLabels|TestMaintenanceConfig_TestMode' ./internal/transport/... ./internal/config/... 2>&1 | tail -20</automated>
  </verify>
  <done>All three webhook tests pass; MAINTENANCE_TEST_MODE field exists in config + docker-compose; production maintenance container is not touched during the test run.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Lock maintenance-prompt + scraper Go symbols (SCRAPER-HEAL-16 + cacheStream/computeStreamTTL stability)</name>
  <files>services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go</files>
  <read_first>
    - .claude/maintenance-prompt.md (entire file — confirm Pattern 6 + Pattern 7 + 'Scraper Playability Regression' sections + the reason-enum numbered list 1-6 are present; the test asserts substring presence)
    - libs/streamprobe/reason.go (Reason enum string values — the test loops over AllReasons() and confirms each appears in the prompt)
    - services/scraper/internal/providers/gogoanime/client.go (grep for `cacheStream` + `computeStreamTTL` — the test confirms both substrings appear in some .go file under services/scraper/internal/providers/gogoanime/)
    - .planning/phases/23-self-maintenance-loop/23-CONTEXT.md D6 (maintenance prompt is NOT edited in this phase; only asserted)
  </read_first>
  <behavior>
    - Test `TestMaintenancePrompt_ContainsPatterns6And7`: reads `.claude/maintenance-prompt.md` from project root; asserts the strings `"### Pattern 6:"`, `"### Pattern 7:"`, and `"### Scraper Playability Regression"` are all present (substring match, line-anchored).
    - Test `TestMaintenancePrompt_AllReasonsCovered`: loops over `streamprobe.AllReasons()`; for each reason value (e.g. `ad_decoy`, `zero_match`, `cdn_unreachable`, `signed_url_expired`, `status_403`, `empty_response`, `playable`), asserts the string appears at least once in the prompt body. (Allows fuzzy match — `status_403` may appear as `status_403` OR `403_upstream` per the prompt's wording at line 170; the test accepts EITHER form.) The intent: a new reason added to the enum without updating the prompt fails the test.
    - Test `TestScraperGoSymbols_StillExist`: greps every `.go` file under `services/scraper/internal/providers/gogoanime/` for the substring `cacheStream`; asserts ≥ 1 file contains it. Same for `computeStreamTTL`. If either name was refactored away, the test fails with a clear message naming the missing symbol — the operator's fix is either to rename back, OR to edit `.claude/maintenance-prompt.md` to reference the new symbol name (a follow-up, NOT done here per D6).
    - Test `TestMaintenancePrompt_FilePresentInWorkingDir`: simple sanity that `.claude/maintenance-prompt.md` exists; fail-message names the expected absolute path so a relocation can be diagnosed quickly.
    - Path resolution: the test discovers the project root by walking up from its own file location until a `.claude/maintenance-prompt.md` is found, OR by reading `os.Getenv("ANIMEENIGMA_ROOT")` if set (fallback for non-standard CI layouts). Document both paths in test comments.
  </behavior>
  <action>
    1. **Create services/maintenance/internal/classifier/maintenance_prompt_symbols_test.go** with the four test functions above. Key implementation details:
       - Use `filepath.Walk` or `runtime.Caller(0)` to locate the project root (test file location → `../../../../..` → repo root) since `go test` runs with the package dir as cwd.
       - Use `os.ReadFile` to read the prompt; assert via `strings.Contains`.
       - For the Go-symbol test, walk `services/scraper/internal/providers/gogoanime/` and call `bytes.Contains(content, []byte("cacheStream"))` on each .go file; `t.Fatalf` with a helpful message if none match.
       - Add file-header comment explaining these tests are SCRAPER-HEAL-16 enforcement + Phase 23 CONTEXT.md D6 ("prompt is not edited; only asserted").
       - Import `libs/streamprobe` for `AllReasons()` — verify libs/streamprobe is already in services/maintenance/go.mod; add via the standard `require + replace` pattern if not, and run `go work sync`. (Project Memory rule "Adding New libs/ Module".)
    2. **Verify no edits to .claude/maintenance-prompt.md** — `git diff .claude/maintenance-prompt.md` MUST be empty at the end of this task. If the test fails because the prompt is out of date, raise the failure to the operator (do NOT silently fix the prompt; that violates D6).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/maintenance && go test -count=1 -run 'TestMaintenancePrompt|TestScraperGoSymbols' ./internal/classifier/... 2>&1 | tail -20 && cd /data/animeenigma && git diff --quiet .claude/maintenance-prompt.md && echo 'prompt unmodified (correct per D6)'</automated>
  </verify>
  <done>All four assertion tests pass; the maintenance-prompt is unmodified; the scraper Go symbols are still present so the prompt's dispatch-table hints point at live code.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 4: Pre-deploy review — confirm rule expressions parse against live Prometheus + maintenance webhook reaches /api/grafana-webhook with the right contact point</name>
  <files>infra/grafana/alerts/scraper.yaml, docker/grafana/provisioning/alerting/rules.yml, docker/grafana/provisioning/alerting/contactpoints.yml</files>
  <action>
    Operator-driven verification gate. No files modified by this task — only read. Runs the three verification steps in `<how-to-verify>` against the live development environment (localhost Prometheus + Grafana provisioning config + maintenance test suite). The agent pauses for the operator response; if `approved` then proceed to Task 5; if `revise: <issue>` then fix and re-run the checkpoint.
  </action>
  <what-built>
    - 3 alert rules in `infra/grafana/alerts/scraper.yaml` with PromQL expressions and a webhook contact-point reference.
    - Provisioning wiring in `docker/grafana/provisioning/alerting/rules.yml` (Option A) — rules appended inline.
    - Synthetic webhook tests covering Pattern 6 + Pattern 7 dispatch paths.
    - Symbol-stability tests locking maintenance-prompt + scraper Go symbols.
    Nothing has been deployed yet. The redeploy is in Task 5.
  </what-built>
  <how-to-verify>
    1. **Confirm PromQL parses against the running Prometheus** (catch a typo before the deploy):
       ```bash
       # ScraperPlayabilityRegression
       curl -s 'http://localhost:9090/api/v1/query' \
         --data-urlencode 'query=sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[25h]))' \
         | jq -r '.status'
       # → "success" expected (data: [] is fine — metric is not yet emitted; query parsing is what matters)

       # ScraperAdDecoySurge
       curl -s 'http://localhost:9090/api/v1/query' \
         --data-urlencode 'query=sum by (provider, server) (rate(parser_ad_decoy_total[5m]))' \
         | jq -r '.status'
       # → "success"

       # ScraperUnplayableSpike (ratio)
       curl -s 'http://localhost:9090/api/v1/query' \
         --data-urlencode 'query=sum by (provider, server, reason) (rate(parser_unplayable_total[5m])) / ignoring(reason) group_left sum by (provider, server) (rate(parser_requests_total{operation="get_stream"}[5m]) > 0)' \
         | jq -r '.status'
       # → "success" (data: [] acceptable since current rate may be zero)
       ```
       If any returns `"error"` or 4xx, FIX the expression in `infra/grafana/alerts/scraper.yaml` and rules.yml before approving.
    2. **Confirm the webhook contact point used by the new rules** is actually defined in contactpoints.yml and points at maintenance:
       ```bash
       grep -A 5 "name:" docker/grafana/provisioning/alerting/contactpoints.yml
       grep "$(yq eval '.groups[0].rules[0].title' infra/grafana/alerts/scraper.yaml || echo '__missing__')" infra/grafana/alerts/scraper.yaml
       ```
       The contact-point name referenced in scraper.yaml MUST equal one of the names in contactpoints.yml.
    3. **Confirm tests pass with `-race` against the current main branch state** (no surprise regressions):
       ```bash
       cd /data/animeenigma/services/maintenance && go test -race -count=1 ./...
       ```
       Should exit 0.
    4. Type `approved` to proceed to Task 5 (after-update). Type `revise: <issue>` to send back for fixes.
  </how-to-verify>
  <verify>
    <automated>curl -s 'http://localhost:9090/api/v1/query' --data-urlencode 'query=sum by (provider, server, reason) (increase(playability_canary_runs_total{result="fail"}[25h]))' | jq -r '.status' | grep -q success && curl -s 'http://localhost:9090/api/v1/query' --data-urlencode 'query=sum by (provider, server) (rate(parser_ad_decoy_total[5m]))' | jq -r '.status' | grep -q success && cd services/maintenance && go test -race -count=1 ./... 2>&1 | tail -5</automated>
  </verify>
  <done>Operator typed `approved` after confirming all three PromQL expressions parse against the live Prometheus, the contact-point name in scraper.yaml matches one defined in contactpoints.yml, and the maintenance test suite passes with -race.</done>
  <resume-signal>Type "approved" or describe issues to fix.</resume-signal>
</task>

<task type="auto">
  <name>Task 5: /animeenigma-after-update — lint + redeploy scheduler + reload grafana + reload prometheus + changelog + commit + push</name>
  <files>frontend/web/public/changelog.json</files>
  <read_first>
    - frontend/web/public/changelog.json (full file — copy the existing changelog entry shape: version, date, title, items array with emoji-prefixed strings; locale handling for ru/en/ja)
    - CLAUDE.md "After-Update Skill (MUST USE)" section (the canonical step sequence)
    - .planning/phases/22-provider-robustness/22-02-hls-proxy-allowlist-and-iss011-PLAN.md (final task — pattern reference for how the prior plan invoked /animeenigma-after-update; mirror it)
  </read_first>
  <behavior>
    - `make redeploy-scheduler` exits 0 (scheduler image rebuilds with the canary code from Plan 23-01; container restarts; `make health` passes for the scheduler).
    - `curl -s http://localhost:8085/metrics | grep -c "playability_canary_runs_total"` returns ≥ 1 (the counter is registered even before the first run, because libs/metrics uses promauto).
    - `curl -X POST -s -o /dev/null -w '%{http_code}' http://localhost:8085/api/v1/jobs/scraper_playability_canary` returns `202` (manual trigger works against the live scheduler).
    - After the manual trigger completes (~30-60s for 5 anime × ~3 servers via the live scraper), `curl -s http://localhost:8085/metrics | grep playability_canary_runs_total` shows non-zero counter values across at least 2 anime_slot labels (anchor_frieren + anchor_one_piece must both have ≥ 1 increment).
    - `ls /var/lib/docker/volumes/$(docker volume ls -q | grep player_reports)/_data/canary-runs/*.json | tail -1` shows the per-run log file created by Task 1's canary writeRunLog.
    - Grafana reload: `curl -X POST http://localhost:3000/api/admin/provisioning/dashboards/reload` and `curl -X POST http://localhost:3000/api/admin/provisioning/alerting/reload` both return 200 (with appropriate auth from `docker/.env`). The new dashboard appears at `http://localhost:3000/d/scraper-provider-health-canary/`. The new alert rules appear in `http://localhost:3000/alerting/list` under the "Scraper Self-Healing" group.
    - Prometheus reload: `curl -X POST http://localhost:9090/-/reload` returns 200.
    - frontend/web/public/changelog.json gains a new entry for v3.1 self-healing (date 2026-05-13 or later) with informative + enthusiastic tone — items mention canary, dashboard, alerts, auto-dispatch to maintenance bot. All 3 locale arrays (ru/en/ja) updated.
    - A git commit is produced with the required co-authors (Co-Authored-By Claude Opus 4.6 + 0neymik0 + NANDIorg per Project Memory) and pushed to remote.
  </behavior>
  <action>
    1. **Run `/animeenigma-after-update`** — the project skill that automates the full sequence. Per CLAUDE.md, this skill:
       - Lints + builds (Go: `go vet ./...` + `go build ./...` in changed services; frontend: `bunx tsc --noEmit` + `bunx eslint`; no frontend changes here, but the skill verifies anyway).
       - Redeploys affected services. For this phase: `make redeploy-scheduler` (canary code lives here) and `make restart-grafana` (rules + dashboards re-provisioned). Do NOT redeploy scraper or maintenance — neither has Go-source changes from Plan 23 (only test code additions, which the dev mounts pick up without container rebuild).
       - Runs `make health` — must exit 0.
       - Updates `frontend/web/public/changelog.json` with a new entry. Suggested copy:
         ```json
         {
           "version": "v3.1 Self-Healing",
           "date": "2026-05-13",
           "title_ru": "v3.1 Самовосстановление парсера",
           "title_en": "v3.1 Scraper Self-Healing",
           "title_ja": "v3.1 スクレイパー自己修復",
           "items_ru": [
             "🔄 Ночной канарейка-крон проверяет 5 аниме каждый день в 03:00 — мы узнаем о поломках раньше пользователей.",
             "📊 Новая Grafana-панель Scraper Provider Health показывает, какой провайдер падает чаще всего.",
             "🚨 Три новых алерта автоматически зовут бота-обслуживания в Telegram, когда что-то ломается."
           ],
           "items_en": [
             "🔄 Nightly canary cron probes 5 anime every day at 03:00 — we now find out about breakage before users do.",
             "📊 New Grafana dashboard Scraper Provider Health surfaces which provider is failing most often.",
             "🚨 Three new alerts auto-call the maintenance bot in Telegram when something breaks."
           ],
           "items_ja": [
             "🔄 毎晩03:00に5本のアニメを再生確認するクローンが導入されました — ユーザーが気づく前に故障を検出します。",
             "📊 新しいGrafanaダッシュボード「Scraper Provider Health」が、どのプロバイダが最も失敗しているかを表示します。",
             "🚨 3つの新しいアラートが、故障時にTelegramのメンテナンスボットを自動的に呼び出します。"
           ]
         }
         ```
         Insert this as the newest entry at the top of the array. Verify the schema by reading the existing entries first.
       - Commits all modified files with the required co-author trailer block (Project Memory).
       - Pushes to origin/main (the only branch in this repo per `git status`).
    2. **Post-redeploy smoke** — manually trigger the canary against the live scheduler and verify the round-trip:
       ```bash
       curl -X POST -i http://localhost:8085/api/v1/jobs/scraper_playability_canary
       # Wait ~60s for the run to finish
       sleep 75
       curl -s http://localhost:8085/metrics | grep playability_canary_runs_total | head -20
       # Should show ≥ 10 series (5 anime × ≥2 servers)
       ls -1 /var/lib/docker/volumes/$(docker volume ls -q | grep -E 'player_reports$' | head -1)/_data/canary-runs/ 2>/dev/null | tail -3
       # Should show at least one .json log file
       ```
       If the canary returns no metric increments OR no log file, dig in immediately (scheduler logs: `make logs-scheduler`) before declaring success.
    3. **Verify Grafana dashboard renders** — open `http://localhost:3000/d/scraper-provider-health-canary/` in a browser; the Last Canary Run panel should display a recent timestamp; the other 3 panels may be sparse but should render without errors.
    4. **Verify alerts are loaded** — open `http://localhost:3000/alerting/list`; the Scraper Self-Healing group should appear with 3 rules. None should be firing initially (the canary just succeeded). Confirm the alert state "Normal" for each.
    5. **Do NOT trigger a real Pattern 6/7 alert against the live maintenance bot** — that's out of scope. The synthetic test in Task 2 covers the dispatch path in unit-test isolation.
  </action>
  <verify>
    <automated>cd /data/animeenigma && curl -s http://localhost:8085/metrics 2>/dev/null | grep -c "playability_canary_runs_total" | grep -v '^0$' && curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/api/health 2>/dev/null | grep -E '^(200|302|401)$' && grep -c 'v3.1 Self-Healing\|v3.1 Scraper Self-Healing' /data/animeenigma/frontend/web/public/changelog.json</automated>
  </verify>
  <done>Scheduler container is redeployed and emitting playability_canary_runs_total; canary log files appear in the player_reports volume; new Grafana dashboard + 3 alert rules are loaded and visible; changelog.json has the new v3.1 entry in all 3 locales; commit with required co-authors is pushed to origin/main.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Grafana → maintenance webhook | BasicAuth-protected; existing posture unchanged. New alert rules reuse the existing contact point. |
| Synthetic-test → webhookHandler | In-process httptest; never touches the live maintenance container. MAINTENANCE_TEST_MODE field added as future-hook for stricter isolation if needed. |
| Maintenance bot → .claude/maintenance-prompt.md | File-system read at dispatch time; D6 enforces no edits in this phase. The symbol-stability test asserts the prompt's referenced symbols still exist in scraper code — a failed test is the alarm that the prompt is drifting. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-23-09 | E + T | Synthetic alert injection bypasses production auth on /api/grafana-webhook | mitigate | Synthetic test runs against `httptest.NewServer(webhookHandler(...))` — an in-process handler clone, NOT the live container. Live webhook BasicAuth still protected; even if someone replayed a synthetic payload at production, the BasicAuth would reject without the env-var-loaded credentials. |
| T-23-10 | I | Maintenance bot Edit action triggered by synthetic alert modifies real Go files | mitigate | Two layers: (a) synthetic tests do not invoke the dispatcher — they only assert the handler's submitAlert callback receives the payload; (b) MAINTENANCE_TEST_MODE field added to config so a future test can short-circuit the dispatcher before any Edit. The current test design does NOT depend on the dispatcher running at all. |
| T-23-11 | D | Three new alert rules + dashboard increase Prometheus scrape cost | accept | Counters are bounded-cardinality (per Plan 23-01 T-23-04). Rule evaluation is 1m interval. Negligible additional load. |
| T-23-12 | I | Alert annotation `description` leaks internal label values | accept | Labels are normalized identifiers (provider, server, reason) — no PII, no URLs, no user IDs. Severity is operational metadata. Same risk surface as the existing 7 Telegram alerts. |
| T-23-13 | T | Operator hand-edits .claude/maintenance-prompt.md → drift from scraper code | mitigate | Task 3's TestMaintenancePrompt_AllReasonsCovered + TestScraperGoSymbols_StillExist catch drift on every `go test ./...` run. Test failure surfaces the drift loudly. |

All ASVS L1: alert rules carry only normalized labels, contact-point auth unchanged, no new privileged access added.
</threat_model>

<verification>
- `[ -f infra/grafana/alerts/scraper.yaml ]` AND `yq eval '[.groups[0].rules[].title] | length' infra/grafana/alerts/scraper.yaml` == 3.
- `cd services/maintenance && go test -race -count=1 ./...` exits 0.
- `git diff --quiet .claude/maintenance-prompt.md` (file unmodified per D6).
- `curl -s http://localhost:8085/metrics | grep -c playability_canary_runs_total` ≥ 1 after redeploy.
- `curl -s http://localhost:3000/api/alertmanager/grafana/api/v1/alerts 2>/dev/null` (or the equivalent Grafana unified-alerting REST endpoint) lists the three new rules.
- `git log -1 --pretty='%an %s'` shows the commit with the required co-authors.
- `grep -c "v3.1.*Self-Healing\|Scraper Self-Healing" frontend/web/public/changelog.json` ≥ 1.
</verification>

<success_criteria>
- All 5 tasks complete; checkpoint passed.
- Phase 23 ROADMAP Success Criteria #4: three alert rules in infra/grafana/alerts/scraper.yaml routing to maintenance webhook with required labels — VERIFIED via yq + Task 4 checkpoint.
- Phase 23 ROADMAP Success Criteria #5: synthetic Pattern 6 + Pattern 7 payloads accepted by the webhook handler — VERIFIED via 2 unit tests; the dispatcher response shape (Pattern names, tier="button_fix") will be re-verified live the next time a real alert fires (out of this phase's scope per CONTEXT.md D6).
- Phase 23 ROADMAP Success Criteria #6: maintenance-prompt + cacheStream/computeStreamTTL symbols still present — VERIFIED via Task 3's symbol-stability tests.
- All 5 SCRAPER-HEAL requirements (12-16) addressed across plans 23-01/02/03 — every requirement ID appears in at least one plan's `requirements` field.
- Live production environment is running the new canary + dashboard + alerts after `/animeenigma-after-update`.
</success_criteria>

<output>
After completion, create `.planning/phases/23-self-maintenance-loop/23-03-SUMMARY.md`.
</output>
