# Roster-Centric Alerting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut garbage maintenance-bot alerts and idle Claude runs by deferring transient streaming/gateway error-rate alerts (no data loss) and replacing per-provider alert noise with one roster-derived per-group fleet-outage alert sourced from the catalog `provider_state` lifecycle gauge.

**Architecture:** The catalog already publishes `provider_state{provider}` — a numeric lifecycle gauge (`4=UP,3=Recovering,2=Degraded,1=Down,0=Disabled`) it is the *sole* emitter of. We add a `group` label to it, write two uniform per-group Grafana rules over it, route the resulting (already-aggregated) alert through the bot's normal Claude-analysis path, and defer the noisy streaming/gateway `High Error Rate` alerts at the bot's webhook dispatch via the existing `SUPPRESSED_ALERTS` mechanism. Phase 2 then demotes the now-redundant per-provider pagers to dashboard-only and deletes the bot's runtime state re-derivation.

**Tech Stack:** Go 1.22 microservices (catalog, maintenance), Prometheus client (`libs/metrics`), Grafana unified alerting (provisioned YAML), systemd-hosted maintenance bot (tracked `bin/maintenance` binary).

**Spec:** `docs/superpowers/specs/2026-06-30-roster-centric-alerting-design.md`

## Global Constraints

- **Work in the worktree only** — `/data/animeenigma/.claude/worktrees/roster-centric-alerting` (branch `roster-centric-alerting`). NEVER edit the base tree `/data/animeenigma` except host-only git-ignored env files (`docker/maintenance.env`). Editing the base tree pauses the ff-only autosync.
- **Tests:** handwritten fakes only — NO testify/mock. `go test ./...` per service. Catalog metric tests use `prometheus/client_golang/prometheus/testutil`.
- **`provider_state` is catalog-sole-emitted** over the FULL roster (scraper- and catalog-operated rows). Both emit sites must change together when the label set changes, or `WithLabelValues` will panic at runtime (arg count mismatch).
- **Commit co-authors (every commit):**
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Maintenance bot deploy:** `make build-maintenance` → tracked `bin/maintenance`; commit the rebuilt binary (established pattern, e.g. `e72608be`); ExecStart is `/data/animeenigma/bin/maintenance`, env is host-only `/data/animeenigma/docker/maintenance.env`, run by `animeenigma-maintenance.service`.
- **No `bool` PromQL / no `count(...)==0`** for the "nothing playable" rule — use `max by (group)` (never an empty result). See Task 3.
- **R5 baseline gate is mandatory** before the fleet rules go live (Task 5): every group must read `max(provider_state) >= 3` in steady state, else the uniform rule pages constantly.
- After all implementation: run `/animeenigma-after-update`.

---

## Phase 1 — Additive (deferral + roster fleet rules + bot routing)

### Task 1: Defer streaming/gateway `High Error Rate` at the bot webhook path

**Files:**
- Modify: `services/maintenance/cmd/maintenance/main.go` (webhook dispatch loop in `processBatch`, ~line 597, beside the existing dedup block)
- Test: `services/maintenance/cmd/maintenance/main_test.go`
- Host config (NOT git): `/data/animeenigma/docker/maintenance.env`

**Interfaces:**
- Consumes: `s.isSuppressed(alertKey string) bool` (exists, `main.go:1709`, reads `s.cfg.SuppressedAlerts`); `domain.MessageAlertFiring`; `msg.Alerts[0].Name`, `msg.Alerts[0].Service`.
- Produces: nothing new for later tasks (behavioral gate only).

- [ ] **Step 1: Write the failing test**

Add to `services/maintenance/cmd/maintenance/main_test.go`:

```go
func TestIsSuppressed_StreamingGatewayKeys(t *testing.T) {
	s := newTestServiceWithHTTP(t, "http://127.0.0.1:0", &http.Client{})
	s.cfg.SuppressedAlerts = []string{"High Error Rate:streaming", "High Error Rate:gateway"}

	cases := []struct {
		key  string
		want bool
	}{
		{"High Error Rate:streaming", true},
		{"High Error Rate:gateway", true},
		{"high error rate:STREAMING", true}, // EqualFold is case-insensitive
		{"High Error Rate:catalog", false},  // catalog still pages
		{"Parser Failure Rate:gogoanime", false},
	}
	for _, c := range cases {
		if got := s.isSuppressed(c.key); got != c.want {
			t.Errorf("isSuppressed(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes already (isSuppressed exists) — then prove the GATE is missing**

Run: `cd services/maintenance && go test ./cmd/maintenance/ -run TestIsSuppressed_StreamingGatewayKeys -v`
Expected: PASS (the matcher already works). This test locks the key semantics. The behavioral gap is that `processBatch` never calls `isSuppressed` — verify by inspection: `grep -n "isSuppressed" cmd/maintenance/main.go` shows it called only in the poller goroutine (~line 334), not in the webhook dispatch loop (~line 582+).

- [ ] **Step 3: Add the suppress gate to the webhook dispatch loop**

In `services/maintenance/cmd/maintenance/main.go`, inside the `for _, msg := range batch.Relevant {` loop in `processBatch`, immediately BEFORE the existing dedup block (`// Dedup: check if this alert is already being tracked`), insert:

```go
		// Defer suppressed alerts (e.g. transient streaming/gateway HLS-proxy
		// 5xx bursts): drop silently here so the webhook path honors
		// SUPPRESSED_ALERTS the same way the reconcile poller does. Data is
		// preserved in Prometheus + ClickHouse events; this only stops the
		// Telegram page + Claude run. Keyed alertName:service.
		if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
			if s.isSuppressed(msg.Alerts[0].Name + ":" + msg.Alerts[0].Service) {
				log.Infow("deferred alert (suppressed)", "alert", msg.Alerts[0].Name, "service", msg.Alerts[0].Service)
				continue
			}
		}
```

- [ ] **Step 4: Build + run the full maintenance test suite**

Run: `cd services/maintenance && go build ./... && go test ./...`
Expected: PASS (build clean, all tests green).

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/cmd/maintenance/main.go services/maintenance/cmd/maintenance/main_test.go
git commit -F - <<'EOF'
fix(maintenance): honor SUPPRESSED_ALERTS on the webhook path (defer streaming/gateway)

The webhook dispatch loop never consulted isSuppressed — only the reconcile
poller did — so suppression had no effect on the primary delivery path. Add the
gate beside dedup so deferred alerts (streaming/gateway High Error Rate) drop
silently: no Telegram, no Claude run, no issue. Data stays in Prometheus +
ClickHouse events.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

> Deploy of this binary + the `SUPPRESSED_ALERTS` env value happens together in Task 5 (after the roster rules exist), to keep one restart.

---

### Task 2: Add a `group` label to the `provider_state` gauge

**Files:**
- Modify: `libs/metrics/provider.go` (the `ProviderState` `GaugeVec` declaration, ~line 128)
- Modify: `services/catalog/internal/service/scraperprovider/roster_metrics.go:50` (boot seed emit)
- Modify: `services/catalog/internal/handler/internal_provider_policy.go:96` (live-transition emit)
- Test: `services/catalog/internal/service/scraperprovider/roster_metrics_test.go`

**Interfaces:**
- Consumes: `domain.ScraperProvider.Group string` (exists, `scraper_provider.go`, `gorm:"column:group"`, default `"en"`); `domain.ScraperProvider.StateCode() float64`.
- Produces: `metrics.ProviderState` now keyed `[]string{"provider", "group"}` — Grafana rules in Task 3 depend on the `group` label existing on the series.

- [ ] **Step 1: Write the failing test**

Add to `services/catalog/internal/service/scraperprovider/roster_metrics_test.go` (mirror the existing test's DB setup; if the file lacks a DB helper, copy the one used by the existing `EmitProviderStates`/`EmitCatalogSideRoster` test):

```go
func TestEmitProviderStates_CarriesGroupLabel(t *testing.T) {
	db := newTestDB(t) // existing in-memory/sqlite helper used by this package's tests
	if err := db.Create(&domain.ScraperProvider{
		Name: "gogoanime", Group: "en", Policy: domain.PolicyAuto, Health: domain.HealthUp,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := EmitProviderStates(db); err != nil {
		t.Fatalf("emit: %v", err)
	}
	// UP (auto+up) => StateCode 4, carried on the (provider, group) series.
	if got := testutil.ToFloat64(metrics.ProviderState.WithLabelValues("gogoanime", "en")); got != 4 {
		t.Errorf("provider_state{gogoanime,en} = %v, want 4", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestEmitProviderStates_CarriesGroupLabel -v`
Expected: FAIL to compile — `WithLabelValues("gogoanime", "en")` passes 2 args to a 1-label vector (`too many arguments`), or a runtime panic on label cardinality.

- [ ] **Step 3: Add the `group` label to the gauge declaration**

In `libs/metrics/provider.go`, change the `ProviderState` label slice:

```go
	ProviderState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_state",
			Help: "Derived provider lifecycle state code (4=UP, 3=Recovering, 2=Degraded, 1=Down, 0=Disabled) per provider, for the Grafana state-history timeline",
		},
		[]string{"provider", "group"},
	)
```

- [ ] **Step 4: Update both emit sites to pass `Group`**

`services/catalog/internal/service/scraperprovider/roster_metrics.go:50`:

```go
		metrics.ProviderState.WithLabelValues(r.Name, r.Group).Set(r.StateCode())
```

`services/catalog/internal/handler/internal_provider_policy.go:96`:

```go
	metrics.ProviderState.WithLabelValues(p.Name, p.Group).Set(p.StateCode())
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ ./internal/handler/ -run "ProviderState|ProviderStates|Policy" -v`
Then: `cd /data/animeenigma/.claude/worktrees/roster-centric-alerting && go build ./... 2>&1 | head` (catches any other `ProviderState.WithLabelValues` caller — grep confirms only the two above, plus `libs/metrics` self-test).
Expected: PASS, build clean.

- [ ] **Step 6: Run libs/metrics tests (the gauge's own package)**

Run: `cd libs/metrics && go test ./...`
Expected: PASS (the existing `provider_test.go` does not assert on `ProviderState`, so no change needed there; confirm).

- [ ] **Step 7: Commit**

```bash
git add libs/metrics/provider.go services/catalog/internal/service/scraperprovider/ services/catalog/internal/handler/internal_provider_policy.go
git commit -F - <<'EOF'
feat(catalog): add group label to provider_state gauge

provider_state now carries (provider, group) so the roster-centric fleet alert
rules can aggregate by (group) uniformly. Both emit sites (boot seed +
probe-result transition) updated together.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 3: Add the two per-group fleet Grafana rules

**Files:**
- Modify: `docker/grafana/provisioning/alerting/rules.yml` (append two rules to the `rules:` list under group `AnimeEnigma Alerts`)

**Interfaces:**
- Consumes: `provider_state{group}` series (Task 2).
- Produces: two firing alerts `ProviderFleetNoAutoPlayable` / `ProviderFleetCorrelatedDown`, each carrying a `group` label — Task 4 relies on the `group` label being present and on `service` being absent.

- [ ] **Step 1: Append the two rules**

Insert into `docker/grafana/provisioning/alerting/rules.yml`, as new entries in the `rules:` sequence (same indentation as `provider-health-stream-segment-down`). Datasource UID `PBFA97CFB590B2093` is the Prometheus datasource (copied from sibling rules):

```yaml
      - uid: provider-fleet-no-auto-playable
        title: ProviderFleetNoAutoPlayable
        condition: C
        noDataState: OK
        execErrState: Error
        for: 30m
        data:
          - refId: A
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: PBFA97CFB590B2093
            model:
              # Per group, fires when the HEALTHIEST provider is registered
              # (>=1, so all-Disabled steady-state groups are excluded) but below
              # auto-playable (<3 => no UP/Recovering). `max by (group)` never
              # yields an empty result, so a fully-Down group still produces a
              # value to threshold (the naive count(...>=3)==0 would vanish).
              expr: (max by (group) (provider_state) < 3) and (max by (group) (provider_state) >= 1)
              instant: true
              refId: A
          - refId: B
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: __expr__
            model:
              refId: B
              type: reduce
              expression: A
              reducer: last
          - refId: C
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: __expr__
            model:
              conditions:
                - evaluator:
                    params: [0]
                    type: gt
                  operator:
                    type: and
              refId: C
              type: threshold
              expression: B
        labels:
          severity: critical
        annotations:
          summary: "Provider group {{ $labels.group }}: no auto-playable source"
          description: "Every provider in the {{ $labels.group }} group is Down or only manually-selectable (Degraded) for 30m — auto-failover cannot serve this group. Check /api/admin/scraper/health and the playback-health roster."

      - uid: provider-fleet-correlated-down
        title: ProviderFleetCorrelatedDown
        condition: C
        noDataState: OK
        execErrState: Error
        for: 30m
        data:
          - refId: A
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: PBFA97CFB590B2093
            model:
              # Count of providers per group actively failing IN the auto chain
              # (state==1 Down). Gradual attrition is auto-demoted to manual
              # (Degraded=2) and never counts here, so >=2 means a correlated
              # simultaneous failure (shared cause), not slow one-at-a-time aging.
              expr: count by (group) (provider_state == 1)
              instant: true
              refId: A
          - refId: B
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: __expr__
            model:
              refId: B
              type: reduce
              expression: A
              reducer: last
          - refId: C
            relativeTimeRange:
              from: 600
              to: 0
            datasourceUid: __expr__
            model:
              conditions:
                - evaluator:
                    params: [1.5]
                    type: gt
                  operator:
                    type: and
              refId: C
              type: threshold
              expression: B
        labels:
          severity: critical
        annotations:
          summary: "Provider group {{ $labels.group }}: multiple sources failed together"
          description: "2+ providers in the {{ $labels.group }} group are actively Down in the auto chain for 30m — a correlated outage (shared cause), not gradual degrade. Check the Camoufox pool / shared dependency and /api/admin/scraper/health."
```

- [ ] **Step 2: Validate the YAML parses**

Run: `cd /data/animeenigma/.claude/worktrees/roster-centric-alerting && python3 -c "import yaml,sys; yaml.safe_load(open('docker/grafana/provisioning/alerting/rules.yml')); print('YAML OK')"`
Expected: `YAML OK`. (Grafana provisioning rejects the whole file on a parse error, so this guards a deploy break.)

- [ ] **Step 3: Confirm uniqueness of the new UIDs/titles**

Run: `grep -nE "uid: provider-fleet|title: ProviderFleet" docker/grafana/provisioning/alerting/rules.yml`
Expected: exactly the 2 new uids and 2 new titles, no duplicates.

- [ ] **Step 4: Commit**

```bash
git add docker/grafana/provisioning/alerting/rules.yml
git commit -F - <<'EOF'
feat(grafana): roster-centric per-group fleet alerts (NoAutoPlayable + CorrelatedDown)

Two uniform by-(group) rules over the catalog provider_state lifecycle gauge:
- NoAutoPlayable: max by(group) registered(>=1) but below auto-playable(<3).
- CorrelatedDown: >=2 providers Down(==1) in the auto chain at once.
30m hold. Same mechanism for every group; gradual degrade stays silent because
it auto-demotes to Degraded(2) and never counts as Down(1).

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 4: Make the bot dedup fleet alerts per group

**Files:**
- Modify: `services/maintenance/internal/grafana/extract.go` (`ExtractService` key list)
- Test: `services/maintenance/internal/grafana/extract_test.go` (create if absent)

**Interfaces:**
- Consumes: alert `labels` map carrying `group` but no `service`/`provider` (Task 3 rules).
- Produces: `ExtractService` returns the group name for fleet alerts, so the bot's dedup key `Name + ":" + Service` becomes `ProviderFleetNoAutoPlayable:en`, `…:ru`, etc. — distinct per group instead of colliding on `"unknown"`.

> Why this is the only bot change for the fleet alert: it flows through the normal Claude-analysis + Telegram + escalate path (per spec §4.3 — a Claude run IS wanted; no special handler). The Grafana rule already aggregates `by (group)`, so the bot receives one alert per affected group. Without this fix, two groups firing the same rule both extract `service="unknown"` and the second is dropped by dedup.

- [ ] **Step 1: Write the failing test**

Create/extend `services/maintenance/internal/grafana/extract_test.go`:

```go
package grafana

import "testing"

func TestExtractService_GroupFallback(t *testing.T) {
	// Fleet alert: only a `group` label, no service/provider → return the group.
	if got := ExtractService(map[string]string{"group": "en"}, map[string]string{}); got != "en" {
		t.Errorf("group-only labels: got %q, want %q", got, "en")
	}
	if got := ExtractService(map[string]string{"group": "ru"}, map[string]string{}); got != "ru" {
		t.Errorf("group-only labels: got %q, want %q", got, "ru")
	}
	// `service` still wins over `group` when both present.
	if got := ExtractService(map[string]string{"service": "streaming", "group": "en"}, map[string]string{}); got != "streaming" {
		t.Errorf("service must win over group: got %q, want %q", got, "streaming")
	}
	// No recognizable labels at all → unknown (unchanged behavior).
	if got := ExtractService(map[string]string{}, map[string]string{}); got != "unknown" {
		t.Errorf("empty: got %q, want %q", got, "unknown")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./internal/grafana/ -run TestExtractService_GroupFallback -v`
Expected: FAIL — `group-only labels: got "unknown", want "en"`.

- [ ] **Step 3: Add `group` as the last-priority extraction key**

In `services/maintenance/internal/grafana/extract.go`, change the key slice:

```go
	for _, key := range []string{"service", "job", "provider", "player", "group"} {
```

(Appending `group` LAST means it only applies when none of service/job/provider/player exist — i.e. the fleet alerts — so no other alert's `service` is affected.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./internal/grafana/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/maintenance/internal/grafana/extract.go services/maintenance/internal/grafana/extract_test.go
git commit -F - <<'EOF'
fix(maintenance): extract group label so per-group fleet alerts dedup distinctly

ProviderFleet* alerts carry a `group` label but no `service`, so both groups
mapped to "unknown" and the dedup key collided. Append `group` as the
last-priority ExtractService key so each group gets a distinct active-alert key.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

> Optional (deferred, not required): generalize `formatProviderFaultLine(providers, group)` + make `isScraperAlert` match `ProviderFleet*` so the 🔴 Firing message carries the per-provider fault line. Skipped for now — Claude's analysis already gathers provider detail, so this is cosmetic.

---

### Task 5: Baseline gate (R5) + deploy Phase 1 + runtime verification

**Files:** none (deploy + verification). Host config edit: `/data/animeenigma/docker/maintenance.env`.

This task is the **hard R5 gate**: the uniform rule evaluates every group, so any group whose steady state is below `UP` (e.g. if Kodik/`ru` or ae/`firstparty` is seeded `manual`/`down`) would page constantly. Verify baselines BEFORE the rules notify.

- [ ] **Step 1: Build the maintenance binary + deploy catalog (emits the new label)**

```bash
cd /data/animeenigma/.claude/worktrees/roster-centric-alerting
make build-maintenance            # produces bin/maintenance with the suppress gate
git add bin/maintenance
git commit -F - <<'EOF'
build(maintenance): rebuild bin/maintenance with webhook suppress gate + group extract

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

Push Phase 1 to main and let the base tree fast-forward (autosync ~10 min, or expedite per the git-workflow doc), then:

```bash
make redeploy-catalog             # catalog now emits provider_state{provider,group}
```

- [ ] **Step 2: BASELINE GATE — confirm every group reads `max >= 3` in steady state**

After catalog is up and scraped (~1 scrape interval), query Prometheus:

```bash
curl -s 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=max by (group) (provider_state)' | \
  python3 -m json.tool
```

Expected: every `group` in the result has `value >= 3`.
- If a group reads `< 3` in steady state (e.g. `firstparty`=2 because ae is seeded `manual`, or `ru`=2 because Kodik is `manual`): **STOP**. Either (a) correct that provider's seed `policy`/`health` so the lifecycle reflects reality, or (b) add a label-matcher exclusion to the two rules (e.g. add `{group!="firstparty"}` to the PromQL and document why inline). Do NOT proceed to Step 3 until `max by (group)(provider_state) >= 3` holds for every group that will be evaluated.

- [ ] **Step 3: Enable the rules + the deferral env, restart**

```bash
# Host-only env (git-ignored, allowed exception to the base-tree rule):
# append to /data/animeenigma/docker/maintenance.env:
#   SUPPRESSED_ALERTS=High Error Rate:streaming,High Error Rate:gateway
make restart-grafana                       # loads the two fleet rules
sudo systemctl restart animeenigma-maintenance   # picks up new binary + SUPPRESSED_ALERTS
```

- [ ] **Step 4: Verify deferral works (no data lost)**

- Confirm Grafana still shows the `High Error Rate` rule and its state-history (it still evaluates): Grafana → Alerting → rules.
- Confirm streaming egress still lands in ClickHouse (read-only):
  ```bash
  curl -s 'http://localhost:8123/' --data "SELECT count() FROM events WHERE effect_kind='egress' AND status IN (403,404,410) AND event_time > now() - INTERVAL 1 DAY"
  ```
  Expected: a non-zero count (the upstream-failure data the alert was computed from is still recorded).
- Tail the maintenance log during the next streaming `High Error Rate` firing and confirm a `deferred alert (suppressed)` line with NO Telegram/Claude:
  ```bash
  journalctl -u animeenigma-maintenance.service -f | grep -iE "deferred alert|streaming"
  ```

- [ ] **Step 5: Verify the fleet alert fires correctly (one alert, Claude runs)**

Either wait for a real transition or synthetically drive a group down. Synthetic (test datasource not available → drive a real provider via the probe-result endpoint on a throwaway provider, OR temporarily force-set the gauge is not possible — prefer waiting for a natural correlated event, OR set one EN provider Down via SQL on a non-prod-impacting provider and watch). Confirm:
- Exactly ONE `ProviderFleetNoAutoPlayable` (or CorrelatedDown) Telegram message for the affected group.
- A Claude analysis run occurred (`journalctl … | grep "claude started"`), producing an `escalate`-tier issue.
- A repeated Grafana delivery within the firing window is deduped (no second analysis).

- [ ] **Step 6: Soak**

Let Phase 1 run a few days. Confirm: gradual single-provider degrade (one provider auto-demoted to manual) does NOT fire either fleet rule, and streaming/gateway noise no longer reaches Telegram. Record the before/after alert volume.

---

## Phase 2 — Simplification cleanup (AFTER Phase 1 soaks clean)

> Do not start Phase 2 until Phase 1 has run a few days without regressions (Task 5 Step 6). Phase 2 is fully reversible.

### Task 6: Demote redundant per-provider pagers to dashboard-only

**Files:**
- Modify: `docker/grafana/provisioning/alerting/rules.yml` (add `severity: diagnostic` to 5 rules)
- Create: `docker/grafana/provisioning/alerting/mute_timings.yml`
- Modify: `docker/grafana/provisioning/alerting/policies.yml` (nested route muting `severity=diagnostic`)

The 5 rules whose raw signals already feed the lifecycle (so they need not page independently): `Parser Failure Rate`, `Scraper Provider Stream-Segment Down`, `ScraperPlayabilityRegression`, `ScraperAdDecoySurge`, `ScraperUnplayableSpike`. They KEEP evaluating (panels + state-history intact) but stop notifying — the Grafana-native "defer but keep data".

**Interfaces:**
- Consumes: nothing.
- Produces: the maintenance bot stops receiving these 5 alert classes → makes `shouldSuppressForProvider` dead (Task 7).

- [ ] **Step 1: Add `severity: diagnostic` to the 5 rules**

For each of the 5 rules above, change its `labels:` block's `severity:` value to `diagnostic` (e.g. `Parser Failure Rate` `severity: warning` → `severity: diagnostic`; `Scraper Provider Stream-Segment Down` `severity: critical` → `severity: diagnostic`). Leave all other rules (incl. the two new fleet rules) untouched.

- [ ] **Step 2: Create the always-on mute timing**

Create `docker/grafana/provisioning/alerting/mute_timings.yml`:

```yaml
apiVersion: 1

muteTimes:
  - orgId: 1
    name: always
    time_intervals:
      - times:
          - start_time: "00:00"
            end_time: "24:00"
```

- [ ] **Step 3: Route `severity=diagnostic` through the mute timing**

In `docker/grafana/provisioning/alerting/policies.yml`, add a nested route under the root policy so diagnostic alerts are matched, muted, and NOT bubbled up:

```yaml
apiVersion: 1

policies:
  - orgId: 1
    receiver: maintenance-webhook
    group_by:
      - alertname
      - service
    group_wait: 30s
    group_interval: 5m
    repeat_interval: 4h
    routes:
      - receiver: maintenance-webhook
        object_matchers:
          - ["severity", "=", "diagnostic"]
        mute_time_intervals:
          - always
        continue: false
```

- [ ] **Step 4: Validate YAML + deploy**

```bash
cd /data/animeenigma/.claude/worktrees/roster-centric-alerting
python3 -c "import yaml; [yaml.safe_load(open(f)) for f in ['docker/grafana/provisioning/alerting/rules.yml','docker/grafana/provisioning/alerting/policies.yml','docker/grafana/provisioning/alerting/mute_timings.yml']]; print('YAML OK')"
```
Expected: `YAML OK`. Commit, push, then `make restart-grafana`.

- [ ] **Step 5: Verify demotion + commit**

- Grafana → Alerting: the 5 rules still evaluate and show state, but firing instances no longer reach Telegram (the mute route swallows them). Trigger/await one and confirm no Telegram.
- Confirm the two fleet rules + all non-provider rules STILL page.

```bash
git add docker/grafana/provisioning/alerting/
git commit -F - <<'EOF'
chore(grafana): demote per-provider pagers to dashboard-only (mute severity=diagnostic)

Parser Failure Rate, Stream-Segment Down, PlayabilityRegression, AdDecoySurge,
UnplayableSpike now carry severity=diagnostic and route through an always-on
mute timing — they keep evaluating (panels + state history) but stop paging.
The roster-centric fleet rules cover the actionable aggregate.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 7: Delete the bot's runtime provider-state reconciliation

**Files:**
- Modify: `services/maintenance/cmd/maintenance/main.go` (delete `shouldSuppressForProvider` + its call site in the dispatch loop, ~lines 615-625 and ~1083-1122)
- Modify: `services/maintenance/cmd/maintenance/main_test.go` (delete the 3 `TestShouldSuppressForProvider*` tests)

**Interfaces:**
- Consumes: nothing.
- Produces: removes dead code — managed/disabled providers are simply absent from the fleet rules' `>=3`/`==1` counts (lifecycle-aware by construction), so runtime re-derivation is no longer needed. The per-provider alerts it suppressed no longer arrive (Task 6).

- [ ] **Step 1: Confirm the function is now dead**

Run: `cd services/maintenance && grep -n "shouldSuppressForProvider" cmd/maintenance/*.go`
Expected: only the definition, the dispatch-loop call (~line 618), and the 3 tests. No other consumers.

- [ ] **Step 2: Delete the dispatch-loop gate**

In `processBatch`, remove the block:

```go
		// Provider-policy suppress gate: skip escalation if provider is already
		// under manual management (policy != "auto"). Fail-open on catalog errors.
		if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
			if s.shouldSuppressForProvider(msg.Alerts[0].Service) {
				log.Infow("suppressing escalation: provider already managed (policy!=auto)",
					"provider", msg.Alerts[0].Service,
					"alert", msg.Alerts[0].Name,
				)
				continue
			}
		}
```

- [ ] **Step 3: Delete the function**

Remove the entire `func (s *service) shouldSuppressForProvider(provider string) bool { … }` (~lines 1083-1122).

- [ ] **Step 4: Delete its tests**

In `main_test.go`, remove `TestShouldSuppressForProvider`, `TestShouldSuppressForProvider_FailOpen`, `TestShouldSuppressForProvider_Non200`.

- [ ] **Step 5: Build + test + rebuild binary**

Run:
```bash
cd services/maintenance && go build ./... && go test ./...
cd /data/animeenigma/.claude/worktrees/roster-centric-alerting && make build-maintenance
```
Expected: build clean, tests PASS (no remaining reference to the deleted symbol).

- [ ] **Step 6: Commit + deploy**

```bash
git add services/maintenance/cmd/maintenance/main.go services/maintenance/cmd/maintenance/main_test.go bin/maintenance
git commit -F - <<'EOF'
refactor(maintenance): delete shouldSuppressForProvider runtime reconciliation

Dead once per-provider pagers are demoted (Task 6): managed/disabled providers
are absent from the fleet rules' counts, so the lifecycle-aware fleet alert needs
no runtime re-derivation. Removes the function, its dispatch gate, and tests.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

Push, base ff-syncs, `sudo systemctl restart animeenigma-maintenance`.

---

## Self-Review notes

- **Spec coverage:** §4.1 → Task 1+5. §4.2 (group label) → Task 2; (rules) → Task 3. §4.3 (Claude run, no 24h re-alert, per-group dedup) → Task 4 + flows through existing path. §4.4 → Tasks 6-7. §5 (data preserved) → Task 5 Step 4. §7 testing → embedded per task. §8 rollout → Task 5. §9 R5 → Task 5 Step 2 (hard gate). §10 params → Tasks 1,3,5.
- **No new storage** (YAGNI) — Task 5 Step 4 only *verifies* ClickHouse, builds nothing.
- **Type consistency:** `provider_state` labels `(provider, group)` used identically in Tasks 2 & 3; `ExtractService` key list extended once (Task 4); `isSuppressed`/`SuppressedAlerts` reused from existing code (Task 1).
- **Phasing:** Phase 2 (Tasks 6-7) gated on Phase 1 soak; both reversible.
