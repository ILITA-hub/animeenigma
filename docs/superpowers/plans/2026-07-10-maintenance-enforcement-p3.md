# Maintenance Routines — P3 Enforcement Wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `/admin/policy` Maintenance tab's toggles/knobs actually take effect — each background routine reads its policy-service gate and self-skips when paused, tunes behavior from its knobs, and reports last-run status back.

**Architecture:** Pull-config enforcement. policy-service (:8098) already serves `GET /internal/maintenance/routines/{id}` → `{"success":true,"data":{"enabled":bool,"settings":{...}}}` and `POST .../{id}/status {ok,summary,next_run_at?}` (P1). Each routine reads the gate at the top of every run (fail-open on any error) and POSTs status after. **No shared Go lib** — the 2 Go consumers (scheduler, host-native maintenance daemon) each get a small `internal/maintenancegate` client (a shared lib would ripple to 19 Dockerfiles for 2 users). Host bash scripts share one sourceable helper.

**Tech Stack:** Go (scheduler + maintenance daemon), Bash + curl + jq (host scripts), Vue/TS + i18n (one roster-correction touch-up).

## Global Constraints

- **Fail-open (spec §6.1, verbatim):** a gate read that is unreachable / non-200 / parse-fail ⇒ treat as **`enabled=true`**. A policy outage must NEVER silently pause a routine.
- **Endpoints:** gate `GET {POLICY}/internal/maintenance/routines/{id}`; status `POST {POLICY}/internal/maintenance/routines/{id}/status`. `{POLICY}` = `http://policy:8098` (Docker services) or `http://localhost:8098` (host-native maintenance daemon + host bash scripts; policy is published at `127.0.0.1:8098`). Envelope: read **`.data.enabled`** (server wraps every payload in `{success,data}` — `libs/httputil/response.go`).
- **Status body:** `{"ok":<bool>,"summary":"<≤512-rune line>","next_run_at":null}`. Fire-and-forget: a failed status POST never fails the routine.
- **Routine ids (must match `services/policy/internal/domain/maintenance.go` SeedRoutines + `frontend/web/src/config/maintenanceRoutines.ts`):** `maintenance_bot, provider_recovery, git_autosync, disk_prune, build_cache_prune, subtitle_probe, shikimori_sync, playability_canary, provider_self_heal`. NOTE the scheduler's playback-probe job is internally labelled `playback_probe` but the routine id is **`playability_canary`**.
- **provider_self_heal = NO enforcement** (owner decision, 2026-07-10): auto-demote was retired 07-08; its actuation shares the `playability_canary` pipeline. It is a status/config row only — the sole change is a knob rename (Task 7). Its gate is never read; catalog's playback-critical health path is NOT touched. **Document that pausing `playability_canary` also freezes provider health-promotion** (shared pipeline).
- **Host installs are the OWNER's step.** Tasks 4–6 land committed diffs in `infra/host/`; the owner installs to `/usr/local/bin` + `/usr/local/lib/animeenigma/` and rebuilds the daemon (`make build-maintenance`). The auto-mode classifier blocks the AI from self-wiring host units. I can deploy+verify only the Docker `scheduler` (Task 1).
- **gofmt landmine:** never `gofmt -w` / `make fmt` (curly-quotes string literals). Fix import order manually.
- **Commit co-authors** (all three, verbatim) on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Worktree:** all work in `/data/ae-maint-p3` (branch `feat/maintenance-enforcement-p3`); never edit `/data/animeenigma`.

---

## Task 1: Scheduler enforcement (gate 3 crons + status-ping)

Gates `shikimori_sync`, `subtitle_probe`, `playability_canary` (the `playback_probe` job); pings status for each after it runs, and also stamps `provider_self_heal` status when the canary runs (shared pipeline — gives the status-only row real activity without touching catalog).

**Files:**
- Create: `services/scheduler/internal/maintenancegate/client.go`
- Create: `services/scheduler/internal/maintenancegate/client_test.go`
- Modify: `services/scheduler/internal/config/config.go` (add `PolicyServiceURL`)
- Modify: `services/scheduler/internal/service/job.go` (inject client, gate + ping the 3 closures)
- Modify: `services/scheduler/cmd/scheduler-api/main.go` (construct client, pass to JobService)
- Modify: `docker/docker-compose.yml` (scheduler `POLICY_SERVICE_URL` env)

**Interfaces:**
- Produces: `maintenancegate.Client` with `New(baseURL string, timeout time.Duration) *Client`, `Enabled(ctx context.Context, id string) bool` (fail-open true), `PostStatus(ctx context.Context, id string, ok bool, summary string)`.

- [ ] **Step 1: Write the failing client test**

```go
// services/scheduler/internal/maintenancegate/client_test.go
package maintenancegate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnabled_failsOpen(t *testing.T) {
	// unreachable base URL → fail-open true
	c := New("http://127.0.0.1:1/", 200*time.Millisecond)
	if !c.Enabled(context.Background(), "git_autosync") {
		t.Fatal("unreachable gate must fail open (enabled=true)")
	}
}

func TestEnabled_readsDataEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/maintenance/routines/subtitle_probe" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"enabled":false,"settings":{}}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)
	if c.Enabled(context.Background(), "subtitle_probe") {
		t.Fatal("gate enabled=false must be read as false")
	}
}

func TestEnabled_non200_failsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)
	if !c.Enabled(context.Background(), "nope") {
		t.Fatal("404 must fail open (enabled=true)")
	}
}

func TestPostStatus_sendsBody(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		got <- string(buf)
		w.Write([]byte(`{"success":true,"data":{"id":"x"}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second)
	c.PostStatus(context.Background(), "shikimori_sync", true, "412 updated")
	select {
	case body := <-got:
		if body == "" {
			t.Fatal("empty status body")
		}
	case <-time.After(time.Second):
		t.Fatal("status POST never arrived")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/scheduler && go test ./internal/maintenancegate/ -v`
Expected: FAIL — package/`New` undefined.

- [ ] **Step 3: Implement the client**

```go
// services/scheduler/internal/maintenancegate/client.go
// Package maintenancegate is a tiny fail-open client for policy-service's
// /internal/maintenance/routines gate + status endpoints. Duplicated (not a
// shared lib) because only scheduler + the host-native maintenance daemon use
// it — a libs/ module would ripple to 19 Dockerfiles for 2 consumers.
package maintenancegate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: timeout}}
}

type gateEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Enabled bool `json:"enabled"`
	} `json:"data"`
}

// Enabled reports whether the routine may run. FAIL-OPEN: any error
// (unreachable, non-200, parse-fail) returns true so a policy outage never
// silently pauses a routine.
func (c *Client) Enabled(ctx context.Context, id string) bool {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s", c.base, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return true
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return true
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return true
	}
	var env gateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return true
	}
	return env.Data.Enabled
}

// PostStatus stamps last-run status. Fire-and-forget: errors are ignored so a
// status write never affects the routine.
func (c *Client) PostStatus(ctx context.Context, id string, ok bool, summary string) {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s/status", c.base, id)
	body, err := json.Marshal(map[string]any{"ok": ok, "summary": summary})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd services/scheduler && go test ./internal/maintenancegate/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Add `PolicyServiceURL` to scheduler config**

In `services/scheduler/internal/config/config.go`, add a field to `JobsConfig` next to `CatalogServiceURL` and load it in `config.Load()` next to the analogous line:

```go
// in JobsConfig struct (next to CatalogServiceURL):
	PolicyServiceURL string
```
```go
// in config.Load(), next to CatalogServiceURL: getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"):
		PolicyServiceURL: getEnv("POLICY_SERVICE_URL", "http://policy:8098"),
```

- [ ] **Step 6: Inject the client into JobService + gate the 3 closures**

In `services/scheduler/internal/service/job.go`: add a `maint *maintenancegate.Client` field to `JobService`, a setter `SetMaintenanceGate(*maintenancegate.Client)` (mirrors `SetShedChecker`), and a nil-safe local helper. Then gate each closure. Example for `subtitle_probe` (job.go ~290) — the pattern is identical for `shikimori_sync` (~104) and `playback_probe` (~199):

```go
// add near skipIfDegraded:
func (s *JobService) maintPaused(ctx context.Context, id string) bool {
	if s.maint == nil {
		return false // gate not wired → run (fail-open)
	}
	if s.maint.Enabled(ctx, id) {
		return false
	}
	s.log.Infow("skipping job: routine paused via /admin/policy", "routine", id)
	return true
}
```

`subtitle_probe` closure becomes:
```go
	_, err = s.cron.AddFunc(subtitleProbeCron, func() {
		ctx := context.Background()
		if s.maintPaused(ctx, "subtitle_probe") {
			return
		}
		s.log.Info("starting scheduled subtitle-health probe")
		start := time.Now()
		if err := s.subtitleProbeJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
			s.log.Errorw("subtitle-health probe failed", "error", err)
			s.maint.PostStatus(ctx, "subtitle_probe", false, "probe error: "+err.Error())
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("subtitle_probe").SetToCurrentTime()
			s.lastSubtitleProbeRun = time.Now()
			s.log.Info("subtitle-health probe completed successfully")
			s.maint.PostStatus(ctx, "subtitle_probe", true, "probe ok")
		}
	})
```
`PostStatus` calls must be nil-guarded the same way (`if s.maint != nil { s.maint.PostStatus(...) }`) — since `maintPaused` returned false either because `s.maint==nil` OR enabled=true, guard the ping. Cleanest: add a `postStatus(ctx,id,ok,summary)` method on JobService that nil-checks `s.maint`, and call THAT.

Add the nil-safe wrapper and use it everywhere:
```go
func (s *JobService) postStatus(ctx context.Context, id string, ok bool, summary string) {
	if s.maint != nil {
		s.maint.PostStatus(ctx, id, ok, summary)
	}
}
```
Apply the same `maintPaused` guard + `postStatus` pair to `shikimori_sync` (summary `fmt.Sprintf("synced ok")` / `"sync error: "+err`) and `playback_probe` (routine id **`playability_canary`**). In the `playback_probe` success/error branches, ALSO `s.postStatus(ctx, "provider_self_heal", ok, summary)` — the canary run IS the self-heal actuation, giving the status-only self_heal row real activity with no catalog change. Keep the existing `skipIfDegraded("playback_probe")` guard; the `maintPaused` check goes AFTER it (either short-circuits a run).

- [ ] **Step 7: Construct the client in main.go**

In `services/scheduler/cmd/scheduler-api/main.go`, near the `shedWatcher` wiring (~line 162):
```go
	jobService.SetMaintenanceGate(maintenancegate.New(cfg.Jobs.PolicyServiceURL, 3*time.Second))
```
Add the import `"github.com/ILITA-hub/animeenigma/services/scheduler/internal/maintenancegate"`. (`cfg.Jobs` is the `JobsConfig`; match the actual field path used for `CatalogServiceURL`.)

- [ ] **Step 8: Add compose env**

In `docker/docker-compose.yml`, scheduler service `environment:` block (~883-899), add:
```yaml
      POLICY_SERVICE_URL: http://policy:8098
```

- [ ] **Step 9: Build + test + commit**

Run: `cd services/scheduler && go build ./... && go test ./... -count=1`
Expected: PASS.
```bash
git -C /data/ae-maint-p3 add services/scheduler docker/docker-compose.yml
git -C /data/ae-maint-p3 commit -m "feat(scheduler): gate probe/sync crons on maintenance routines + status-ping

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Maintenance-bot daemon enforcement (gate + auto_apply_max_risk)

`maintenance_bot`: gate `decideAutoApply` — when the routine is disabled, never auto-apply (revert to button-only, daemon stays up + passive); and cap auto-apply by the `auto_apply_max_risk` knob (none/low/medium).

**Files:**
- Create: `services/maintenance/internal/maintenancegate/client.go` (identical to Task 1's client, in this module's path)
- Create: `services/maintenance/internal/maintenancegate/client_test.go`
- Modify: `services/maintenance/internal/config/config.go` (add `PolicyURL`)
- Modify: `services/maintenance/cmd/maintenance/main.go` (`decideAutoApply` gate + max-risk cap; construct client; status-ping)
- Modify: `docker/maintenance.env` (add `POLICY_URL=http://localhost:8098`) — **owner-installed file; land the line in the repo's tracked sample if one exists, else document in README**

**Interfaces:**
- Consumes: nothing from Task 1 (separate module — client code is repeated here verbatim).
- Produces: gate applied inside `decideAutoApply`.

- [ ] **Step 1: Create the client + test** — copy Task 1's `client.go` (Step 3 body) + `client_test.go` (Step 1 body) verbatim into `services/maintenance/internal/maintenancegate/` (`package maintenancegate`), then ADD the `MaxRisk` method + its test from Step 5 below (this module's client has one extra method vs the scheduler's). Run `cd services/maintenance && go test ./internal/maintenancegate/ -v` → PASS.

- [ ] **Step 2: Add `PolicyURL` to maintenance config**

In `services/maintenance/internal/config/config.go`, add to `Config` (next to `CatalogURL`) and load it (next to `CatalogURL`'s `getEnv`):
```go
	PolicyURL string
```
```go
	// in Load(): mirror CatalogURL's getEnv default (Docker name; overridden to
	// localhost in docker/maintenance.env since the bot is host-native).
	PolicyURL: getEnv("POLICY_URL", "http://policy:8098"),
```

- [ ] **Step 3: Write the failing decideAutoApply gate test**

Add to `services/maintenance/cmd/maintenance/autofix_test.go` (the existing decideAutoApply test file). Model the test on the existing ones there (they build a `service` with a fake gate). Add a `maint *maintenancegate.Client` field to `service` and inject a test server. Assertions:
- routine disabled (gate server returns `enabled:false`) ⇒ `decideAutoApply` returns `apply=false` even for a low-risk real bug.
- `auto_apply_max_risk:"low"` ⇒ a medium-risk admin bug that would normally auto-apply returns `apply=false`.
- `auto_apply_max_risk:"none"` ⇒ even low-risk returns `apply=false`.
- gate unreachable ⇒ fail-open, behaves as today (auto-apply allowed).

(Write concrete cases mirroring `autofix_test.go`'s existing table; read that file first for the exact `service`/`ClassifiedMessage`/`AnalysisResult` fixtures.)

- [ ] **Step 4: Run to verify fail** — `cd services/maintenance && go test ./cmd/maintenance/ -run AutoApply -v` → FAIL.

- [ ] **Step 5: Implement the gate in decideAutoApply**

Add a `maint *maintenancegate.Client` field to the `service` struct and a `maintRiskCeiling(ctx) (allowed bool, ceiling string)` helper that reads the gate (enabled + `auto_apply_max_risk` from settings). At the TOP of `decideAutoApply` (main.go ~1520):
```go
	// Maintenance gate: when the bot routine is paused, never auto-apply
	// (fall back to the button path; the poller/analysis keep running).
	if s.maint != nil && !s.maint.Enabled(context.Background(), "maintenance_bot") {
		return false, "", "maintenance_bot paused via /admin/policy — needs admin button"
	}
```
First add the `MaxRisk` reader to this module's `maintenancegate/client.go` (the risk types are `domain.FixRisk` = `"low"/"medium"/"high"`, `models.go:40-45`; `AnalysisResult.Risk` is `FixRisk`, `models.go:146`):
```go
// add to services/maintenance/internal/maintenancegate/client.go
type settingsEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Settings map[string]any `json:"settings"`
	} `json:"data"`
}

// MaxRisk returns the auto_apply_max_risk knob ("none"/"low"/"medium"), or ""
// on any error/miss (fail-open = no cap).
func (c *Client) MaxRisk(ctx context.Context, id string) string {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s", c.base, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var env settingsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return ""
	}
	if v, ok := env.Data.Settings["auto_apply_max_risk"].(string); ok {
		return v
	}
	return ""
}
```
Then in `main.go`, after the per-level decision computes an auto-apply (i.e. right before the loop-guard block near the end of `decideAutoApply`), enforce the ceiling, with two rank helpers at package scope:
```go
// riskRank ranks a fix's self-assessed risk; unset/unknown ⇒ high.
func riskRank(r domain.FixRisk) int {
	switch r {
	case domain.RiskLow:
		return 1
	case domain.RiskMedium:
		return 2
	default: // RiskHigh or unset
		return 3
	}
}

// ceilingRank ranks the auto_apply_max_risk knob; "" ⇒ no cap (rank above any real risk).
func ceilingRank(c string) int {
	switch c {
	case "none":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	default: // "" (gate error/unset) or unknown ⇒ no cap
		return 3
	}
}
```
```go
	// Cap by the admin-configured max auto-apply risk (none<low<medium). A gate
	// error/unset knob ⇒ "" ⇒ no cap (fail-open).
	if s.maint != nil {
		if ceiling := s.maint.MaxRisk(context.Background(), "maintenance_bot"); ceiling != "" &&
			riskRank(result.Risk) > ceilingRank(ceiling) {
			return false, "", "auto_apply_max_risk ceiling — needs admin button"
		}
	}
```
Add a test to `maintenancegate/client_test.go` for `MaxRisk` (server returns `{"success":true,"data":{"settings":{"auto_apply_max_risk":"low"}}}` → `"low"`; non-200 → `""`).

- [ ] **Step 6: Construct the client + status-ping**

In `main.go` where the `service` is built, set `maint: maintenancegate.New(cfg.PolicyURL, 3*time.Second)`. After a completed auto-fix (near the existing `TrySetStatus`/apply-success point, main.go ~1400), `s.maint.PostStatus(context.Background(), "maintenance_bot", true, "auto-fixed "+result.FixPlan.Target)` (guard nil).

- [ ] **Step 7: Build + test + commit**

Run: `cd services/maintenance && go build ./... && go test ./... -count=1` → PASS. Then:
```bash
git -C /data/ae-maint-p3 add services/maintenance
git -C /data/ae-maint-p3 commit -m "feat(maintenance): gate auto-fix on maintenance_bot routine + risk ceiling

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
> **Owner install:** `make build-maintenance` + `systemctl restart animeenigma-maintenance`, and add `POLICY_URL=http://localhost:8098` to `/data/animeenigma/docker/maintenance.env`.

---

## Task 3: Shared host bash gate helper

A sourceable helper so the host scripts (Tasks 4–6) don't each re-implement the curl+jq gate read. Fail-open, jq-based (`jq` is a house tool — used in `animeenigma-walpurgis-watch.sh`).

**Files:**
- Create: `infra/host/animeenigma-maint-gate.sh`
- Modify: `infra/host/README.md` (document the helper + its install path)

**Interfaces:**
- Produces (sourceable functions): `maint_gate_enabled <id>` → exit 0 if enabled OR on any error (fail-open), exit 1 only when the gate explicitly says `enabled:false`; `maint_gate_setting <id> <key>` → prints the settings value (empty on miss); `maint_status <id> <ok:0|1> <summary>` → fire-and-forget POST.

- [ ] **Step 1: Write the helper**

```bash
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
  enabled=$(printf '%s' "$body" | jq -r '.data.enabled // empty' 2>/dev/null) || return 0
  [ "$enabled" = "false" ] && return 1
  return 0
}

# maint_gate_setting <routine_id> <key> -> prints value (empty on any miss)
maint_gate_setting() {
  local id="$1" key="$2" body
  body=$(curl -fsS -m 4 "$MAINT_POLICY_BASE/internal/maintenance/routines/$id" 2>/dev/null) || return 0
  printf '%s' "$body" | jq -r --arg k "$key" '.data.settings[$k] // empty' 2>/dev/null || true
}

# maint_status <routine_id> <ok:0|1> <summary> -> fire-and-forget status POST
maint_status() {
  local id="$1" ok="$2" summary="$3" okjson=false
  [ "$ok" = "0" ] && okjson=true
  curl -fsS -m 4 -X POST -H 'Content-Type: application/json' \
    -d "$(jq -nc --argjson ok "$okjson" --arg s "$summary" '{ok:$ok,summary:$s}')" \
    "$MAINT_POLICY_BASE/internal/maintenance/routines/$id/status" >/dev/null 2>&1 || true
}
```
(`ok` arg uses shell convention: `0`=success→`ok:true`.)

- [ ] **Step 2: Shellcheck + a smoke test against the live gate**

Run: `shellcheck infra/host/animeenigma-maint-gate.sh` (fix any warning). Then a manual smoke (documented, run by whoever has host access):
```bash
source infra/host/animeenigma-maint-gate.sh
maint_gate_enabled git_autosync && echo "RUN" || echo "PAUSED"   # RUN (seeded enabled)
maint_gate_setting disk_prune high_water_pct                      # 80
```

- [ ] **Step 3: Document in README + commit**

Add a section to `infra/host/README.md`: helper source-of-truth `infra/host/animeenigma-maint-gate.sh`, install to `/usr/local/lib/animeenigma/maint-gate.sh`, sourced by provider-recovery / git-autosync / docker-prune. Commit:
```bash
git -C /data/ae-maint-p3 add infra/host/animeenigma-maint-gate.sh infra/host/README.md
git -C /data/ae-maint-p3 commit -m "feat(host): sourceable maintenance-gate helper for host automations

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Wire `provider-recovery.sh` (gate-check + status-ping)

**Files:**
- Modify: `infra/host/animeenigma-provider-recovery.sh`

- [ ] **Step 1: Source the helper + gate-check before the run**

At the top (after the existing config vars), source the helper (installed path, with the repo path as a dev fallback):
```bash
# shellcheck source=/dev/null
if [ -r /usr/local/lib/animeenigma/maint-gate.sh ]; then
  . /usr/local/lib/animeenigma/maint-gate.sh
elif [ -r "$(dirname "$0")/animeenigma-maint-gate.sh" ]; then
  . "$(dirname "$0")/animeenigma-maint-gate.sh"
fi
```
Between the `if ! check; then ... fi` block and the `log "=== ... START ..."` line (script ~line 68), add:
```bash
if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled provider_recovery; then
  log "skip: provider_recovery paused via /admin/policy"
  exit 0
fi
```

- [ ] **Step 2: Status-ping after the run**

Replace the tail (`rc=$?` … `exit "$rc"`) with:
```bash
rc=$?
log "=== provider-recovery run END (exit=$rc) ==="
if command -v maint_status >/dev/null 2>&1; then
  maint_status provider_recovery "$rc" "recovery run exit=$rc (model=$MODEL)"
fi
exit "$rc"
```

- [ ] **Step 3: Shellcheck + `--check` dry-run + commit**

Run: `shellcheck infra/host/animeenigma-provider-recovery.sh` and `bash infra/host/animeenigma-provider-recovery.sh --check` (must still exit per prereqs; gate absence is harmless). Commit:
```bash
git -C /data/ae-maint-p3 add infra/host/animeenigma-provider-recovery.sh
git -C /data/ae-maint-p3 commit -m "feat(host): provider-recovery reads maintenance gate + reports status

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
> **Owner install:** copy the updated script to `/usr/local/bin/animeenigma-provider-recovery.sh`.

---

## Task 5: Wire `git-autosync.sh` (gate-check + status-ping)

**Files:**
- Modify: `infra/host/animeenigma-git-autosync.sh`

- [ ] **Step 1: Source helper + gate-check**

After the `flock -n 9` lock acquisition (~line 24) and before `cd "$REPO"`:
```bash
# shellcheck source=/dev/null
[ -r /usr/local/lib/animeenigma/maint-gate.sh ] && . /usr/local/lib/animeenigma/maint-gate.sh
if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled git_autosync; then
  log "skip: git_autosync paused via /admin/policy"
  exit 0
fi
```

- [ ] **Step 2: Status-ping at the end**

The sync decision tree already sets a descriptive `log "..."` per branch. Capture a `summary`/`ok` in each branch (add a `RESULT="..."` and `OKFLAG=0|1` assignment beside each existing `log` line), then after the tree (before the `git worktree prune` housekeeping ~line 66):
```bash
if command -v maint_status >/dev/null 2>&1; then
  maint_status git_autosync "${OKFLAG:-0}" "${RESULT:-unknown} · HEAD $(git rev-parse --short HEAD 2>/dev/null)"
fi
```
Set `OKFLAG=0` (success) for the `already current` / `fast-forwarded` branches and `OKFLAG=1` (problem) + `RESULT="DIVERGED ($ahead ahead)"` etc. for the skip branches, so the tab's badge reflects the DIVERGED state (spec §5 status line "in-sync / DIVERGED").

- [ ] **Step 3: Shellcheck + dry-run + commit**

Run: `shellcheck infra/host/animeenigma-git-autosync.sh`. Commit (message `feat(host): git-autosync reads maintenance gate + reports in-sync/DIVERGED status`, co-authors). Owner install → `/usr/local/bin/animeenigma-git-autosync.sh`.

---

## Task 6: Adopt disk/build-cache prune into the repo (gated + high_water_pct)

The prune crons are host-only today; adopt them into `infra/host/` (repo source-of-truth), gate them, and wire the `high_water_pct` knob (daily disk prune runs only when disk% exceeds it).

**Files:**
- Create: `infra/host/animeenigma-docker-prune.sh` (one script, `daily`|`weekly` arg)
- Create: `infra/host/animeenigma-docker-prune.cron` (daily + weekly entries)
- Modify: `infra/host/README.md` (install steps)

- [ ] **Step 1: Write the prune script**

```bash
# infra/host/animeenigma-docker-prune.sh
# Container disk hygiene (adopted into the repo 2026-07-10 so /admin/policy can
# pause/tune it). Two modes:
#   daily  -> routine disk_prune:        docker system prune (only when disk% > high_water_pct)
#   weekly -> routine build_cache_prune: docker builder prune + image prune
# Install to /usr/local/bin/animeenigma-docker-prune.sh (mode 755).
set -uo pipefail
LOG=/var/log/animeenigma-docker-prune.log
log(){ echo "$(date -Is) $*" >>"$LOG"; }
# shellcheck source=/dev/null
[ -r /usr/local/lib/animeenigma/maint-gate.sh ] && . /usr/local/lib/animeenigma/maint-gate.sh

mode="${1:-daily}"
case "$mode" in
  daily)
    if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled disk_prune; then
      log "skip: disk_prune paused via /admin/policy"; exit 0
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
    ;;
  weekly)
    if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled build_cache_prune; then
      log "skip: build_cache_prune paused via /admin/policy"; exit 0
    fi
    docker builder prune -f --reserved-space 30GB >>"$LOG" 2>&1
    docker image prune -f >>"$LOG" 2>&1
    rc=$?
    log "build_cache_prune done (rc=$rc)"
    command -v maint_status >/dev/null 2>&1 && maint_status build_cache_prune "$rc" "build-cache + image prune"
    ;;
  *) echo "usage: $0 daily|weekly" >&2; exit 2 ;;
esac
```
> **Behavior change (intended):** the daily prune becomes threshold-gated (`high_water_pct`, default 80) instead of unconditional. The enabled-gate is the hard control; the threshold avoids needless pruning below it. Keeps the "containerd fills /" guard (prunes above threshold).

- [ ] **Step 2: Cron file**

```bash
# infra/host/animeenigma-docker-prune.cron  → install to /etc/cron.d/animeenigma-docker-prune
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
17 4 * * *   root /usr/local/bin/animeenigma-docker-prune.sh daily  >> /var/log/animeenigma-docker-prune.log 2>&1
23 4 * * 0   root /usr/local/bin/animeenigma-docker-prune.sh weekly >> /var/log/animeenigma-docker-prune.log 2>&1
```
(Replaces the un-tracked `/etc/cron.d/docker-prune` + `/etc/cron.weekly/docker-prune` — the README install step notes removing the old ones.)

- [ ] **Step 3: Shellcheck + README + commit**

`shellcheck infra/host/animeenigma-docker-prune.sh`. Add README install steps (install script 755 to `/usr/local/bin/`, cron to `/etc/cron.d/animeenigma-docker-prune`, remove the old host-only `/etc/cron.d/docker-prune` + `/etc/cron.weekly/docker-prune`). Commit (`feat(host): adopt docker-prune into repo — gated + high_water_pct threshold`, co-authors).
> **Owner install:** install script + cron; remove the two old host-only prune crons; ensure `maint-gate.sh` (Task 3) is installed first.

---

## Task 7: Roster correction — provider_self_heal knob `demote_after` → `promote_after`

`provider_self_heal` gets NO enforcement (owner decision). Its dead `demote_after` knob (auto-demote retired 07-08) is renamed to `promote_after` so the UI names a live concept. Seed + FE registry + i18n only.

**Files:**
- Modify: `services/policy/internal/domain/maintenance.go` (seed settings)
- Modify: `frontend/web/src/config/maintenanceRoutines.ts` (knob descriptor)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`admin.policy.maintenance.knobs.demoteAfter` → `.promoteAfter`)
- Test: `services/policy/internal/domain/maintenance_test.go` (update the seed assertion if it names the key)

- [ ] **Step 1: Seed** — in `SeedRoutines()`, change the `provider_self_heal` settings from `{"demote_after":"24h","probe_every":"6h"}` to `{"promote_after":"24h","probe_every":"6h"}`. (Insert-if-absent means the deployed prod row keeps `demote_after`; add a deploy note: one-off `PUT /api/admin/policy/maintenance/routines/provider_self_heal {enabled:true, settings:{promote_after:"24h",probe_every:"6h"}}` OR SQL update to migrate the live row, else the FE knob shows its default.)

- [ ] **Step 2: FE registry** — in `config/maintenanceRoutines.ts`, rename the `provider_self_heal` knob `{ key:'demote_after', ..., labelKey:'admin.policy.maintenance.knobs.demoteAfter' }` to `{ key:'promote_after', ..., labelKey:'admin.policy.maintenance.knobs.promoteAfter' }` (keep options `['12h','24h','48h']`). Update its spec if it asserts the key.

- [ ] **Step 3: i18n** — in en/ru/ja, rename `admin.policy.maintenance.knobs.demoteAfter` → `promoteAfter`; en `"Promote after"`, ru `"Повысить через"`, ja `"昇格までの時間"`. Keep identical structure across all three (parity gate).

- [ ] **Step 4: Verify + commit** — `cd services/policy && go test ./internal/domain/` ; `cd frontend/web && bunx vitest run src/config src/locales && bunx tsc --noEmit && bash scripts/design-system-lint.sh`. All green. Commit (`fix(policy,web): provider_self_heal knob demote_after→promote_after (auto-demote retired)`, co-authors).

---

## Verification (before ship)

- [ ] `cd services/scheduler && go test ./... -count=1` ; `cd services/maintenance && go test ./... -count=1` ; `cd services/policy && go test ./internal/domain/` → PASS.
- [ ] `cd frontend/web && bunx vitest run src/config src/locales && bunx tsc --noEmit && bash scripts/design-system-lint.sh` → PASS.
- [ ] `shellcheck infra/host/animeenigma-*.sh` → clean.
- [ ] Deploy the ONE Docker service I can: `make redeploy-scheduler` (+ `redeploy-web` for Task 7 i18n). **Prod-verify the scheduler gate:** pause `subtitle_probe` in the UI (or `PUT .../subtitle_probe {enabled:false}`), confirm the next `*/5` tick logs `skipping job: routine paused` (`make logs-scheduler`), re-enable, confirm it runs + `GET .../subtitle_probe` shows a fresh `lastRunAt`.
- [ ] `/animeenigma-after-update` (simplify → changelog SKIP unless owner wants an admin-facing note → commit → push).

## Owner install checklist (host-side — not deployable by AI)
1. Install `infra/host/animeenigma-maint-gate.sh` → `/usr/local/lib/animeenigma/maint-gate.sh`.
2. `POLICY_URL=http://localhost:8098` → `/data/animeenigma/docker/maintenance.env`; `make build-maintenance` + `systemctl restart animeenigma-maintenance`.
3. Copy updated `animeenigma-provider-recovery.sh` + `animeenigma-git-autosync.sh` → `/usr/local/bin/`.
4. Install `animeenigma-docker-prune.sh` → `/usr/local/bin/` + `animeenigma-docker-prune.cron` → `/etc/cron.d/animeenigma-docker-prune`; remove old `/etc/cron.d/docker-prune` + `/etc/cron.weekly/docker-prune`.
5. Migrate the live `provider_self_heal` settings row (Task 7 Step 1 note).

## Effort scoring
- **UXΔ = +2 (Better)** — the tab's toggles/knobs stop being cosmetic; pausing a routine actually pauses it, status badges go live.
- **CDI = 0.05 × 34** — spread across scheduler + maintenance daemon + 3 host scripts + policy seed + FE (wide); additive + fail-open (low per-site shift); Effort_Fib 34.
- **MVQ = Griffin 87% / 84%** — methodical control-plane wiring fronting many watchful routines; disciplined, fail-open-guarded.
