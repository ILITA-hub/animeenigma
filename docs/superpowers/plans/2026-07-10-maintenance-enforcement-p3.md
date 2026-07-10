# Maintenance Routines — P3 Enforcement Wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `/admin/policy` Maintenance tab's toggles/knobs actually take effect — each background routine reads its policy-service gate and self-skips when paused, tunes behavior from its knobs, and reports last-run status back.

**Architecture:** Pull-config enforcement. policy-service (:8098) already serves `GET /internal/maintenance/routines/{id}` → `{"success":true,"data":{"enabled":bool,"settings":{...}}}` and `POST .../{id}/status {ok,summary,next_run_at?}` (P1). Each routine reads the gate at the top of every run (fail-open on any error) and POSTs status after. A **shared `libs/maintenancegate` module** (owner decision, 2026-07-10) provides the Go client; scheduler + the host-native maintenance daemon import it. Host bash scripts share one sourceable helper.

**Tech Stack:** Go (`libs/maintenancegate` + scheduler + maintenance daemon), Bash + curl + jq (host scripts), Vue/TS + i18n (one roster-correction touch-up).

## Global Constraints

- **Fail-open (spec §6.1, verbatim):** a gate read that is unreachable / non-200 / parse-fail ⇒ treat as **`enabled=true`**. A policy outage must NEVER silently pause a routine.
- **Endpoints:** gate `GET {POLICY}/internal/maintenance/routines/{id}`; status `POST {POLICY}/internal/maintenance/routines/{id}/status`. `{POLICY}` = `http://policy:8098` (Docker services) or `http://localhost:8098` (host-native maintenance daemon + host bash scripts; policy is published at `127.0.0.1:8098`). Envelope: read **`.data.enabled`** (server wraps every payload in `{success,data}` — `libs/httputil/response.go`).
- **Status body:** `{"ok":<bool>,"summary":"<≤512-rune line>","next_run_at":null}`. Fire-and-forget: a failed status POST never fails the routine.
- **Routine ids (must match `services/policy/internal/domain/maintenance.go` SeedRoutines + `frontend/web/src/config/maintenanceRoutines.ts`):** `maintenance_bot, provider_recovery, git_autosync, disk_prune, build_cache_prune, subtitle_probe, shikimori_sync, playability_canary, provider_self_heal`. NOTE the scheduler's playback-probe job is internally labelled `playback_probe` but the routine id is **`playability_canary`**.
- **provider_self_heal = NO enforcement** (owner decision): auto-demote was retired 07-08; its actuation shares the `playability_canary` pipeline. It is a status/config row only — the sole change is a knob rename (Task 8). Its gate is never read; catalog's playback-critical health path is NOT touched. **Document that pausing `playability_canary` also freezes provider health-promotion** (shared pipeline).
- **Shared gate client** = `libs/maintenancegate` (owner decision): one Go module both consumers import (NOT per-service copies). This ripples a `COPY libs/maintenancegate/go.mod ...` line into every Go-service Dockerfile + `go.work` + `go work sync` (Task 1).
- **Host installs are the OWNER's step.** Tasks 4–7 land committed diffs in `infra/host/`; the owner installs to `/usr/local/bin` + `/usr/local/lib/animeenigma/` and rebuilds the daemon (`make build-maintenance`). The auto-mode classifier blocks the AI from self-wiring host units. I can deploy+verify only the Docker `scheduler` (Task 2) + `web` (Task 8).
- **gofmt landmine:** never `gofmt -w` / `make fmt` (curly-quotes string literals). Fix import order manually.
- **Commit co-authors** (all three, verbatim) on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Worktree:** all work in `/data/ae-maint-p3` (branch `feat/maintenance-enforcement-p3`); never edit `/data/animeenigma`.

---

## Task 1: `libs/maintenancegate` shared Go module

The fail-open gate client both scheduler and the maintenance daemon import. Creating a new `libs/` module requires the workspace ripple (go.work + every Go-service Dockerfile + `go work sync`).

**Files:**
- Create: `libs/maintenancegate/go.mod`
- Create: `libs/maintenancegate/client.go`
- Create: `libs/maintenancegate/client_test.go`
- Modify: `go.work` (add `./libs/maintenancegate` to `use (...)`)
- Modify: EVERY Go-service Dockerfile (add one `COPY libs/maintenancegate/go.mod ...` line)
- Modify: `services/scheduler/go.mod` + `services/maintenance/go.mod` (add `require` + `replace`)

**Interfaces:**
- Produces: `maintenancegate.New(baseURL string, timeout time.Duration) *Client`; `(*Client).Enabled(ctx, id string) bool` (fail-open true); `(*Client).PostStatus(ctx, id string, ok bool, summary string)`; `(*Client).MaxRisk(ctx, id string) string` (`""` on error/miss).

- [ ] **Step 1: go.mod (stdlib-only)**

```
// libs/maintenancegate/go.mod
module github.com/ILITA-hub/animeenigma/libs/maintenancegate

go 1.25.0
```
(Match the exact `go` version line used by `libs/logger/go.mod` if it differs.)

- [ ] **Step 2: Write the failing client test**

```go
// libs/maintenancegate/client_test.go
package maintenancegate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnabled_failsOpen(t *testing.T) {
	c := New("http://127.0.0.1:1/", 200*time.Millisecond) // unreachable
	if !c.Enabled(context.Background(), "git_autosync") {
		t.Fatal("unreachable gate must fail open (enabled=true)")
	}
}

func TestEnabled_readsDataEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/maintenance/routines/subtitle_probe" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"enabled":false,"settings":{}}}`))
	}))
	defer srv.Close()
	if New(srv.URL, time.Second).Enabled(context.Background(), "subtitle_probe") {
		t.Fatal("gate enabled=false must be read as false")
	}
}

func TestEnabled_non200_failsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if !New(srv.URL, time.Second).Enabled(context.Background(), "nope") {
		t.Fatal("404 must fail open (enabled=true)")
	}
}

func TestMaxRisk_readsSetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"enabled":true,"settings":{"auto_apply_max_risk":"low"}}}`))
	}))
	defer srv.Close()
	if got := New(srv.URL, time.Second).MaxRisk(context.Background(), "maintenance_bot"); got != "low" {
		t.Fatalf("MaxRisk = %q; want low", got)
	}
}

func TestMaxRisk_non200_empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if got := New(srv.URL, time.Second).MaxRisk(context.Background(), "x"); got != "" {
		t.Fatalf("MaxRisk on 500 = %q; want empty (no cap)", got)
	}
}

func TestPostStatus_sendsBody(t *testing.T) {
	got := make(chan int64, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.ContentLength
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"x"}}`))
	}))
	defer srv.Close()
	New(srv.URL, time.Second).PostStatus(context.Background(), "shikimori_sync", true, "412 updated")
	select {
	case n := <-got:
		if n <= 0 {
			t.Fatal("empty status body")
		}
	case <-time.After(time.Second):
		t.Fatal("status POST never arrived")
	}
}
```

- [ ] **Step 3: Run to verify fail** — `cd libs/maintenancegate && go test ./... -v` → FAIL (package/`New` undefined).

- [ ] **Step 4: Implement the client**

```go
// libs/maintenancegate/client.go
// Package maintenancegate is a fail-open client for policy-service's
// /internal/maintenance/routines gate + status endpoints, shared by the
// scheduler and the host-native maintenance daemon.
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
	Data struct {
		Enabled  bool           `json:"enabled"`
		Settings map[string]any `json:"settings"`
	} `json:"data"`
}

func (c *Client) fetch(ctx context.Context, id string) (*gateEnvelope, bool) {
	url := fmt.Sprintf("%s/internal/maintenance/routines/%s", c.base, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var env gateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, false
	}
	return &env, true
}

// Enabled reports whether the routine may run. FAIL-OPEN: any error
// (unreachable, non-200, parse-fail) returns true so a policy outage never
// silently pauses a routine.
func (c *Client) Enabled(ctx context.Context, id string) bool {
	env, ok := c.fetch(ctx, id)
	if !ok {
		return true
	}
	return env.Data.Enabled
}

// MaxRisk returns the auto_apply_max_risk knob ("none"/"low"/"medium"), or ""
// on any error/miss (fail-open = no cap).
func (c *Client) MaxRisk(ctx context.Context, id string) string {
	env, ok := c.fetch(ctx, id)
	if !ok {
		return ""
	}
	if v, ok := env.Data.Settings["auto_apply_max_risk"].(string); ok {
		return v
	}
	return ""
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

- [ ] **Step 5: Run to verify pass** — `cd libs/maintenancegate && go test ./... -v` → PASS (6 tests).

- [ ] **Step 6: Register the module in the workspace**

Add to `go.work`'s `use (...)` block: `./libs/maintenancegate`. Add `require` + `replace` to BOTH `services/scheduler/go.mod` and `services/maintenance/go.mod` — mirror EXACTLY how `libs/logger` is required there (read those go.mod files; the pattern is `require github.com/ILITA-hub/animeenigma/libs/logger v0.0.0-...` + `replace github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger`). Add the analogous `maintenancegate` lines. Then from the repo root:
```bash
cd /data/ae-maint-p3 && go work sync
```

- [ ] **Step 7: Ripple the Dockerfiles**

Every Go-service Dockerfile does a per-lib `COPY libs/<name>/go.mod libs/<name>/go.sum* ./libs/<name>/` block so `go mod download` resolves the whole workspace. Add one line for `maintenancegate` to each. Find them:
```bash
cd /data/ae-maint-p3 && grep -l 'COPY libs/logger/go.mod' services/*/Dockerfile
```
For each listed Dockerfile, add (right after the last existing `COPY libs/.../go.mod` line):
```dockerfile
COPY libs/maintenancegate/go.mod libs/maintenancegate/go.sum* ./libs/maintenancegate/
```
(The `go.sum*` glob tolerates the absent go.sum — a stdlib-only module has none.) Do NOT touch `services/stealth-scraper/Dockerfile` (Python, no Go COPYs).

- [ ] **Step 8: Verify the whole workspace still builds + commit**

```bash
cd /data/ae-maint-p3 && go build ./... && go test ./libs/maintenancegate/...
```
Expected: PASS (no unresolved-module errors anywhere).
```bash
git -C /data/ae-maint-p3 add libs/maintenancegate go.work go.work.sum services/scheduler/go.mod services/maintenance/go.mod services/*/Dockerfile
git -C /data/ae-maint-p3 commit -m "feat(libs): maintenancegate — shared fail-open gate/status client

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Scheduler enforcement (gate 3 crons + status-ping)

Gates `shikimori_sync`, `subtitle_probe`, `playability_canary` (the `playback_probe` job); pings status after each, and also stamps `provider_self_heal` status when the canary runs (shared pipeline — gives the status-only row real activity without touching catalog).

**Files:**
- Modify: `services/scheduler/internal/config/config.go` (add `PolicyServiceURL`)
- Modify: `services/scheduler/internal/service/job.go` (inject client, gate + ping the 3 closures)
- Modify: `services/scheduler/cmd/scheduler-api/main.go` (construct client, pass to JobService)
- Modify: `docker/docker-compose.yml` (scheduler `POLICY_SERVICE_URL` env)

**Interfaces:**
- Consumes: `maintenancegate.New/Enabled/PostStatus` (Task 1).

- [ ] **Step 1: Add `PolicyServiceURL` to scheduler config**

In `services/scheduler/internal/config/config.go`, add to `JobsConfig` (next to `CatalogServiceURL`) and load in `config.Load()` next to the analogous line:
```go
	PolicyServiceURL string
```
```go
		PolicyServiceURL: getEnv("POLICY_SERVICE_URL", "http://policy:8098"),
```

- [ ] **Step 2: Inject the client + gate helpers into JobService**

In `services/scheduler/internal/service/job.go`: add `maint *maintenancegate.Client` field to `JobService` + `func (s *JobService) SetMaintenanceGate(m *maintenancegate.Client) { s.maint = m }` (mirrors `SetShedChecker`). Add two nil-safe helpers near `skipIfDegraded`:
```go
// maintPaused reports whether an admin has paused this routine via /admin/policy.
// Nil gate (not wired) ⇒ run (fail-open).
func (s *JobService) maintPaused(ctx context.Context, id string) bool {
	if s.maint == nil || s.maint.Enabled(ctx, id) {
		return false
	}
	s.log.Infow("skipping job: routine paused via /admin/policy", "routine", id)
	return true
}

func (s *JobService) postStatus(ctx context.Context, id string, ok bool, summary string) {
	if s.maint != nil {
		s.maint.PostStatus(ctx, id, ok, summary)
	}
}
```
Add the import `"github.com/ILITA-hub/animeenigma/libs/maintenancegate"`.

- [ ] **Step 3: Gate + ping the three closures**

For each of `shikimori_sync` (job.go ~104), `subtitle_probe` (~290), and `playback_probe` (~199, routine id **`playability_canary`**): add `if s.maintPaused(ctx, "<routine_id>") { return }` as the first line inside the closure (for `playback_probe`, AFTER the existing `skipIfDegraded` guard), and add `s.postStatus(ctx, "<routine_id>", ok, summary)` in both the error and success branches. Example (`subtitle_probe`):
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
			s.postStatus(ctx, "subtitle_probe", false, "probe error: "+err.Error())
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("subtitle_probe").SetToCurrentTime()
			s.lastSubtitleProbeRun = time.Now()
			s.log.Info("subtitle-health probe completed successfully")
			s.postStatus(ctx, "subtitle_probe", true, "probe ok")
		}
	})
```
`shikimori_sync` summary: `"sync ok"` / `"sync error: "+err.Error()`. `playback_probe` uses routine id `playability_canary`, and in BOTH branches ALSO `s.postStatus(ctx, "provider_self_heal", <ok>, <summary>)` (the canary run is the self-heal actuation — gives the status-only row activity, no catalog change).

- [ ] **Step 4: Construct the client in main.go**

In `services/scheduler/cmd/scheduler-api/main.go`, near the `shedWatcher` wiring:
```go
	jobService.SetMaintenanceGate(maintenancegate.New(cfg.Jobs.PolicyServiceURL, 3*time.Second))
```
Add the import. (Match the actual `cfg` field path used for `CatalogServiceURL`.)

- [ ] **Step 5: Compose env** — in `docker/docker-compose.yml` scheduler `environment:` block, add `POLICY_SERVICE_URL: http://policy:8098`.

- [ ] **Step 6: Build + test + commit**

Run: `cd services/scheduler && go build ./... && go test ./... -count=1` → PASS.
```bash
git -C /data/ae-maint-p3 add services/scheduler docker/docker-compose.yml
git -C /data/ae-maint-p3 commit -m "feat(scheduler): gate probe/sync crons on maintenance routines + status-ping

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Maintenance-bot daemon enforcement (gate + auto_apply_max_risk)

`maintenance_bot`: gate `decideAutoApply` — when the routine is disabled, never auto-apply (revert to button-only; the daemon stays up + passive); cap auto-apply by the `auto_apply_max_risk` knob (none/low/medium).

**Files:**
- Modify: `services/maintenance/internal/config/config.go` (add `PolicyURL`)
- Modify: `services/maintenance/cmd/maintenance/main.go` (`service` field + `decideAutoApply` gate + risk cap; construct client; status-ping)
- Modify: `services/maintenance/cmd/maintenance/autofix_test.go` (gate + cap tests)
- Doc: `docker/maintenance.env` line (owner-installed) — document in the task report

**Interfaces:**
- Consumes: `maintenancegate.New/Enabled/MaxRisk/PostStatus` (Task 1). Risk types: `domain.FixRisk` = `"low"/"medium"/"high"` (`models.go:40-45`); `AnalysisResult.Risk` is `FixRisk` (`models.go:146`). `service` struct fields at `main.go:193-201` (add `maint *maintenancegate.Client`).

- [ ] **Step 1: Add `PolicyURL` to config**

In `services/maintenance/internal/config/config.go`, add to `Config` (next to `CatalogURL`) + load it (mirror `CatalogURL`'s `getEnv`, Docker default; overridden to localhost in `maintenance.env` since the bot is host-native):
```go
	PolicyURL string
```
```go
	PolicyURL: getEnv("POLICY_URL", "http://policy:8098"),
```

- [ ] **Step 2: Write the failing gate/cap tests**

In `services/maintenance/cmd/maintenance/autofix_test.go` (read it first for the existing `service`/`ClassifiedMessage`/`AnalysisResult` fixtures + how a test `service` is built). Add a `maint` field to the test `service` pointed at an `httptest.Server`, and cases:
- gate `enabled:false` ⇒ `decideAutoApply` returns `apply=false` even for a low-risk real bug.
- `auto_apply_max_risk:"low"` + a medium-risk admin bug (normally auto) ⇒ `apply=false`.
- `auto_apply_max_risk:"none"` + a low-risk fix ⇒ `apply=false`.
- gate server unreachable (nil or bad URL client) ⇒ fail-open, behaves as today.
Write concrete cases mirroring the existing table in that file.

- [ ] **Step 3: Run to verify fail** — `cd services/maintenance && go test ./cmd/maintenance/ -run AutoApply -v` → FAIL.

- [ ] **Step 4: Implement**

Add `maint *maintenancegate.Client` to the `service` struct (main.go:193). At the TOP of `decideAutoApply` (main.go ~1520):
```go
	// Maintenance gate: when the bot routine is paused, never auto-apply (fall
	// back to the button path; the poller/analysis keep running).
	if s.maint != nil && !s.maint.Enabled(context.Background(), "maintenance_bot") {
		return false, "", "maintenance_bot paused via /admin/policy — needs admin button"
	}
```
Two package-scope rank helpers:
```go
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
Enforce the ceiling right before the loop-guard block (after the per-level branch computes an auto-apply):
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
Add the import `"github.com/ILITA-hub/animeenigma/libs/maintenancegate"`.

- [ ] **Step 5: Construct the client + status-ping**

Where the `service` is built (main.go, near the other field inits), set `maint: maintenancegate.New(cfg.PolicyURL, 3*time.Second)`. After a completed auto-fix (near the existing apply-success / `TrySetStatus` point, main.go ~1400): `if s.maint != nil { s.maint.PostStatus(context.Background(), "maintenance_bot", true, "auto-fixed "+result.FixPlan.Target) }`.

- [ ] **Step 6: Build + test + commit**

Run: `cd services/maintenance && go build ./... && go test ./... -count=1` → PASS.
```bash
git -C /data/ae-maint-p3 add services/maintenance
git -C /data/ae-maint-p3 commit -m "feat(maintenance): gate auto-fix on maintenance_bot routine + risk ceiling

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```
> **Owner install:** `make build-maintenance` + `systemctl restart animeenigma-maintenance`; add `POLICY_URL=http://localhost:8098` to `/data/animeenigma/docker/maintenance.env`.

---

## Task 4: Shared host bash gate helper

A sourceable helper so the host scripts (Tasks 5–7) don't re-implement the curl+jq gate read. Fail-open, jq-based (`jq` is a house tool — used in `animeenigma-walpurgis-watch.sh`).

**Files:**
- Create: `infra/host/animeenigma-maint-gate.sh`
- Modify: `infra/host/README.md` (document the helper + install path)

**Interfaces:**
- Produces (sourceable): `maint_gate_enabled <id>` → exit 0 if enabled OR on any error (fail-open), exit 1 only when the gate explicitly says `enabled:false`; `maint_gate_setting <id> <key>` → prints value (empty on miss); `maint_status <id> <ok:0|1> <summary>` → fire-and-forget POST.

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

- [ ] **Step 2: Shellcheck + smoke** — `shellcheck infra/host/animeenigma-maint-gate.sh` (fix warnings). Documented manual smoke (host-access only): `source ...; maint_gate_enabled git_autosync && echo RUN`, `maint_gate_setting disk_prune high_water_pct` → `80`.

- [ ] **Step 3: README + commit** — document source-of-truth + install path `/usr/local/lib/animeenigma/maint-gate.sh`. Commit (`feat(host): sourceable maintenance-gate helper`, co-authors).

---

## Task 5: Wire `provider-recovery.sh` (gate-check + status-ping)

**Files:** Modify `infra/host/animeenigma-provider-recovery.sh`.

- [ ] **Step 1: Source helper + gate-check** — after the config vars, source the helper (installed path, repo fallback); between the `if ! check; then ... fi` block and the START log (~line 68):
```bash
# shellcheck source=/dev/null
if [ -r /usr/local/lib/animeenigma/maint-gate.sh ]; then
  . /usr/local/lib/animeenigma/maint-gate.sh
elif [ -r "$(dirname "$0")/animeenigma-maint-gate.sh" ]; then
  . "$(dirname "$0")/animeenigma-maint-gate.sh"
fi
```
```bash
if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled provider_recovery; then
  log "skip: provider_recovery paused via /admin/policy"
  exit 0
fi
```

- [ ] **Step 2: Status-ping** — replace the tail (`rc=$?` … `exit "$rc"`):
```bash
rc=$?
log "=== provider-recovery run END (exit=$rc) ==="
if command -v maint_status >/dev/null 2>&1; then
  maint_status provider_recovery "$rc" "recovery run exit=$rc (model=$MODEL)"
fi
exit "$rc"
```

- [ ] **Step 3: Shellcheck + `--check` dry-run + commit** — `shellcheck ...`; `bash infra/host/animeenigma-provider-recovery.sh --check`. Commit (`feat(host): provider-recovery reads maintenance gate + reports status`, co-authors). Owner install → `/usr/local/bin/`.

---

## Task 6: Wire `git-autosync.sh` (gate-check + status-ping)

**Files:** Modify `infra/host/animeenigma-git-autosync.sh`.

- [ ] **Step 1: Source helper + gate-check** — after `flock -n 9` (~line 24), before `cd "$REPO"`:
```bash
# shellcheck source=/dev/null
[ -r /usr/local/lib/animeenigma/maint-gate.sh ] && . /usr/local/lib/animeenigma/maint-gate.sh
if command -v maint_gate_enabled >/dev/null 2>&1 && ! maint_gate_enabled git_autosync; then
  log "skip: git_autosync paused via /admin/policy"
  exit 0
fi
```

- [ ] **Step 2: Status-ping** — beside each existing branch `log "..."` in the sync decision tree, set `RESULT="..."` + `OKFLAG=0|1` (0=success for `already current`/`fast-forwarded`; 1 + `RESULT="DIVERGED ($ahead ahead)"` etc. for skip branches). After the tree (before the `git worktree prune` housekeeping ~line 66):
```bash
if command -v maint_status >/dev/null 2>&1; then
  maint_status git_autosync "${OKFLAG:-0}" "${RESULT:-unknown} · HEAD $(git rev-parse --short HEAD 2>/dev/null)"
fi
```

- [ ] **Step 3: Shellcheck + commit** — `shellcheck ...`. Commit (`feat(host): git-autosync reads maintenance gate + reports in-sync/DIVERGED status`, co-authors). Owner install → `/usr/local/bin/`.

---

## Task 7: Adopt disk/build-cache prune into the repo (gated + high_water_pct)

The prune crons are host-only today; adopt into `infra/host/` (repo source-of-truth), gate them, wire `high_water_pct` (daily disk prune runs only when disk% exceeds it).

**Files:**
- Create: `infra/host/animeenigma-docker-prune.sh` (one script, `daily`|`weekly` arg)
- Create: `infra/host/animeenigma-docker-prune.cron`
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
> **Behavior change (intended):** the daily prune becomes threshold-gated (`high_water_pct`, default 80) instead of unconditional. The enabled-gate is the hard control; the threshold avoids needless pruning below it (still guards the "containerd fills /" case above threshold).

- [ ] **Step 2: Cron file**

```bash
# infra/host/animeenigma-docker-prune.cron  → install to /etc/cron.d/animeenigma-docker-prune
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
17 4 * * *   root /usr/local/bin/animeenigma-docker-prune.sh daily  >> /var/log/animeenigma-docker-prune.log 2>&1
23 4 * * 0   root /usr/local/bin/animeenigma-docker-prune.sh weekly >> /var/log/animeenigma-docker-prune.log 2>&1
```

- [ ] **Step 3: Shellcheck + README + commit** — `shellcheck infra/host/animeenigma-docker-prune.sh`. README: install script 755 → `/usr/local/bin/`, cron → `/etc/cron.d/animeenigma-docker-prune`, and REMOVE the old host-only `/etc/cron.d/docker-prune` + `/etc/cron.weekly/docker-prune`. Commit (`feat(host): adopt docker-prune into repo — gated + high_water_pct threshold`, co-authors).

---

## Task 8: Roster correction — provider_self_heal knob `demote_after` → `promote_after`

`provider_self_heal` gets NO enforcement (owner decision). Its dead `demote_after` knob (auto-demote retired 07-08) is renamed to `promote_after`. Seed + FE registry + i18n only.

**Files:**
- Modify: `services/policy/internal/domain/maintenance.go` (seed settings)
- Modify: `frontend/web/src/config/maintenanceRoutines.ts` (knob descriptor)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`
- Test: `services/policy/internal/domain/maintenance_test.go` (update if it names the key)

- [ ] **Step 1: Seed** — in `SeedRoutines()`, change `provider_self_heal` settings from `{"demote_after":"24h","probe_every":"6h"}` to `{"promote_after":"24h","probe_every":"6h"}`. (Insert-if-absent ⇒ the deployed prod row keeps `demote_after`; deploy note: one-off `PUT /api/admin/policy/maintenance/routines/provider_self_heal {enabled:true,settings:{promote_after:"24h",probe_every:"6h"}}` OR SQL update to migrate the live row.)

- [ ] **Step 2: FE registry** — in `config/maintenanceRoutines.ts`, rename the `provider_self_heal` knob key `demote_after`→`promote_after` and labelKey `admin.policy.maintenance.knobs.demoteAfter`→`promoteAfter` (keep options `['12h','24h','48h']`). Update its spec if it asserts the key.

- [ ] **Step 3: i18n** — in en/ru/ja rename `admin.policy.maintenance.knobs.demoteAfter`→`promoteAfter`; en `"Promote after"`, ru `"Повысить через"`, ja `"昇格までの時間"`. Identical structure across all three (parity gate).

- [ ] **Step 4: Verify + commit** — `cd services/policy && go test ./internal/domain/` ; `cd frontend/web && bunx vitest run src/config src/locales && bunx tsc --noEmit && bash scripts/design-system-lint.sh` → all green. Commit (`fix(policy,web): provider_self_heal knob demote_after→promote_after (auto-demote retired)`, co-authors).

---

## Verification (before ship)

- [ ] `cd /data/ae-maint-p3 && go build ./...` (whole workspace resolves the new lib) ; `go test ./libs/maintenancegate/... ./services/scheduler/... ./services/maintenance/... ./services/policy/internal/domain/...` → PASS.
- [ ] `cd frontend/web && bunx vitest run src/config src/locales && bunx tsc --noEmit && bash scripts/design-system-lint.sh` → PASS.
- [ ] `shellcheck infra/host/animeenigma-*.sh` → clean.
- [ ] Deploy the Docker pieces I can: `make redeploy-scheduler` + `make redeploy-web`. **Prod-verify the scheduler gate:** pause `subtitle_probe` (UI or `PUT .../subtitle_probe {enabled:false}`), confirm the next `*/5` tick logs `skipping job: routine paused` (`make logs-scheduler`), re-enable, confirm it runs + `GET .../subtitle_probe` shows a fresh `lastRunAt`.
- [ ] `/animeenigma-after-update` (simplify → changelog SKIP unless owner wants an admin-facing note → commit → push).

## Owner install checklist (host-side — not deployable by AI)
1. Install `infra/host/animeenigma-maint-gate.sh` → `/usr/local/lib/animeenigma/maint-gate.sh`.
2. `POLICY_URL=http://localhost:8098` → `/data/animeenigma/docker/maintenance.env`; `make build-maintenance` + `systemctl restart animeenigma-maintenance`.
3. Copy updated `animeenigma-provider-recovery.sh` + `animeenigma-git-autosync.sh` → `/usr/local/bin/`.
4. Install `animeenigma-docker-prune.sh` → `/usr/local/bin/` + `animeenigma-docker-prune.cron` → `/etc/cron.d/animeenigma-docker-prune`; remove old `/etc/cron.d/docker-prune` + `/etc/cron.weekly/docker-prune`.
5. Migrate the live `provider_self_heal` settings row (Task 8 Step 1 note).

## Effort scoring
- **UXΔ = +2 (Better)** — the tab's toggles/knobs stop being cosmetic; pausing a routine actually pauses it, status badges go live.
- **CDI = 0.05 × 34** — spread across a new lib + 20 Dockerfiles + scheduler + daemon + 3 host scripts + policy seed + FE (wide); additive + fail-open (low per-site shift); Effort_Fib 34.
- **MVQ = Griffin 87% / 84%** — methodical control-plane wiring fronting many watchful routines; disciplined, fail-open-guarded.
