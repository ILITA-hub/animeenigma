# Pre-probe Camoufox warmup + "Last tick metrics" — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Warm the Camoufox session immediately before the measured health probe for `engine=browser` providers (so the probe reflects the warm path real viewers hit, not the cold Turnstile solve), and capture per-tick warmup/latency/throughput/metadata into a new `last_tick_metrics` column surfaced on a dedicated Grafana "Last Tick Metrics" panel.

**Architecture:** Catalog's probe-plan gains an `engine` field (DB-derived). The analytics probe engine, for browser entries, runs one unscored warmup `Resolve()` before the measured probe, times resolve/validate, reads throughput from the segment the validator already downloads, and ships a `metrics` object on the existing `probe-result` POST. Catalog persists it to `stream_providers.last_tick_metrics` (text/JSON). A new Postgres-datasource Grafana panel renders it.

**Tech Stack:** Go (services/analytics, services/catalog), GORM + in-memory SQLite tests, `httptest` stubs, Grafana dashboard JSON (Postgres datasource).

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-06-probe-warmup-last-tick-metrics-design.md`.
- **Worktree only:** all work in `/data/ae-probe-warmup-metrics` (branch `feat/probe-warmup-last-tick-metrics`). NEVER edit the base tree `/data/animeenigma`.
- **Tests:** handwritten fakes/stubs only — NO testify/mock. Catalog handler tests use in-memory SQLite (`gorm.Open(sqlite.Open(":memory:"))`, see `internal_provider_policy_test.go:newHandlerTestDB`). Analytics tests use `httptest` (see `catalog_plan_test.go`).
- **Backward-compatible:** every wire change is additive. A legacy caller posting the 3-field `probe-result` body must still work; a legacy plan without `engine` must still probe (no warmup).
- **Browser set is DB-derived** — gate warmup on `entry.Engine == "browser"`, never a hardcoded provider list.
- **Column storage:** `last_tick_metrics` is a **text** column holding JSON (`gorm.io/datatypes` is NOT a dep). Grafana casts `::jsonb` in SQL. Mirrors the `ai_probe_notes` precedent.
- **Clock:** `Engine.now()` returns **unix seconds** (`time.Now().Unix()`). Durations use `time.Since(...).Milliseconds()`.
- **Commits:** each task commits with the three co-authors:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Verify a Go package:** `cd services/<svc> && go test ./internal/... -count=1`.

---

### Task 1: Catalog — emit `engine` in the probe-plan

**Files:**
- Modify: `services/catalog/internal/handler/internal_provider_policy.go` (`probePlanEntry` struct ~L105, `ProbePlan` append ~L145)
- Test: `services/catalog/internal/handler/internal_provider_policy_test.go`

**Interfaces:**
- Produces: probe-plan JSON entries now include `"engine":"browser"|"http"` (read from `ScraperProvider.Engine`).

- [ ] **Step 1: Write the failing test**

Add to `internal_provider_policy_test.go` (reuse the file's existing `newHandlerTestDB` + seeding style):

```go
func TestProbePlanIncludesEngine(t *testing.T) {
	db := newHandlerTestDB(t)
	// Two scraper-operated rows, due to probe (LastProbedAt far in the past),
	// one browser one http.
	old := time.Now().Add(-48 * time.Hour).UTC()
	for _, p := range []domain.ScraperProvider{
		{Name: "miruro", Policy: domain.PolicyManual, Health: domain.HealthDown, Engine: "browser", ScraperOperated: true, Group: "en", LastProbedAt: old},
		{Name: "allanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, Engine: "http", ScraperOperated: true, Group: "en", LastProbedAt: old},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("seed %s: %v", p.Name, err)
		}
	}
	h := NewInternalProviderPolicyHandler(db, testPolicyCfg(), testLogger())
	rr := httptest.NewRecorder()
	h.ProbePlan(rr, httptest.NewRequest(http.MethodGet, "/internal/providers/probe-plan", nil))

	var body struct {
		Data struct {
			Plan []struct {
				Provider string `json:"provider"`
				Engine   string `json:"engine"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, e := range body.Data.Plan {
		got[e.Provider] = e.Engine
	}
	if got["miruro"] != "browser" {
		t.Fatalf("miruro engine = %q, want browser (plan=%+v)", got["miruro"], body.Data.Plan)
	}
	if got["allanime"] != "http" {
		t.Fatalf("allanime engine = %q, want http", got["allanime"])
	}
}
```

> If `testPolicyCfg()` / `testLogger()` helpers don't exist under those names, use whatever the neighboring tests in this file already call to build a `config.ProviderPolicyConfig` (with non-zero `Cadence.Manual`/`Up`) and a `*logger.Logger`. The rest of the test is self-contained.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbePlanIncludesEngine -count=1 -v`
Expected: FAIL — `Engine` is absent (empty string) from the plan JSON.

- [ ] **Step 3: Add the field and populate it**

In `internal_provider_policy.go`, extend `probePlanEntry`:

```go
type probePlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
	Engine     string `json:"engine"`
}
```

And in `ProbePlan`, set it on append:

```go
		plan = append(plan, probePlanEntry{Provider: p.Name, SampleSize: size, FailFast: ff, Engine: p.Engine})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbePlanIncludesEngine -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/catalog/internal/handler/internal_provider_policy.go services/catalog/internal/handler/internal_provider_policy_test.go
git commit -m "feat(catalog): expose provider engine in the probe-plan

Browser-engine providers need a longer, warmed probe path; the analytics
engine gates that on the plan's engine field (DB-derived, no hardcoded list)." -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Analytics — decode `engine` in `PlanEntry`

**Files:**
- Modify: `services/analytics/internal/probe/catalog_plan.go` (`PlanEntry` struct ~L41)
- Test: `services/analytics/internal/probe/catalog_plan_test.go`

**Interfaces:**
- Produces: `PlanEntry.Engine string` — consumed by Task 4's warmup gate.

- [ ] **Step 1: Write the failing test**

Add to `catalog_plan_test.go`:

```go
func TestFetchPlanDecodesEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"plan":[
			{"provider":"miruro","sample_size":1,"fail_fast":true,"engine":"browser"},
			{"provider":"allanime","sample_size":3,"fail_fast":false,"engine":"http"}]}}`))
	}))
	defer srv.Close()

	entries, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("FetchPlan: %v", err)
	}
	if len(entries) != 2 || entries[0].Engine != "browser" || entries[1].Engine != "http" {
		t.Fatalf("engine not decoded: %+v", entries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestFetchPlanDecodesEngine -count=1 -v`
Expected: FAIL — `entries[0].Engine` is empty.

- [ ] **Step 3: Add the field**

```go
type PlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
	Engine     string `json:"engine"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/probe/ -run TestFetchPlanDecodesEngine -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/analytics/internal/probe/catalog_plan.go services/analytics/internal/probe/catalog_plan_test.go
git commit -m "feat(analytics): decode engine field from the probe-plan" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Analytics — instrument `HTTPValidator` (latency, bytes, CDN host, quality)

**Files:**
- Modify: `services/analytics/internal/probe/types.go` (`Verdict` struct ~L68)
- Modify: `services/analytics/internal/probe/validator.go` (`Validate` ~L108, add a helper)
- Test: `services/analytics/internal/probe/validator_test.go` (create if absent)

**Interfaces:**
- Produces: `Verdict` gains measurement fields, populated by `HTTPValidator` on the reached-playback path (zero otherwise):
  ```go
  ManifestMs   int64  // master manifest fetch latency (ms)
  SegmentMs    int64  // first-segment fetch latency (ms)
  SegmentBytes int64  // first-segment bytes downloaded
  CDNHost      string // host of rs.MasterURL
  Quality      string // e.g. "1080p" from master #EXT-X-STREAM-INF, best-effort
  ```

- [ ] **Step 1: Write the failing test**

Create/extend `validator_test.go`. The validator fetches via `v.streaming + /api/v1/hls-proxy?url=<raw>...`, so the stub reads the `url` query param and serves a master (with a variant STREAM-INF), then the segment:

```go
package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okProber is a VideoProber that always accepts (decode gate off for the test).
type okProber struct{}

func (okProber) Probe(_ context.Context, _ []byte) error { return nil }

func TestValidatePopulatesMetrics(t *testing.T) {
	const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\nvariant.m3u8\n"
	const variant = "#EXTM3U\n#EXTINF:5.0,\nseg-1.ts\n"
	segment := strings.Repeat("A", 200000) // 200 KB "segment"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("url")
		switch {
		case strings.HasSuffix(raw, "master.m3u8"):
			_, _ = w.Write([]byte(master))
		case strings.HasSuffix(raw, "variant.m3u8"):
			_, _ = w.Write([]byte(variant))
		default: // seg-1.ts
			_, _ = w.Write([]byte(segment))
		}
	}))
	defer srv.Close()

	v := NewHTTPValidator(srv.URL, srv.Client(), okProber{})
	rs := ResolvedStream{Provider: "miruro", MasterURL: "https://cdn.example.test/master.m3u8"}
	got := v.Validate(context.Background(), rs)

	if !got.Playable() {
		t.Fatalf("expected playable, got reason=%q", got.Reason)
	}
	if got.ManifestMs < 0 || got.SegmentBytes != int64(len(segment)) {
		t.Fatalf("bad measures: ManifestMs=%d SegmentBytes=%d", got.ManifestMs, got.SegmentBytes)
	}
	if got.CDNHost != "cdn.example.test" {
		t.Fatalf("CDNHost = %q, want cdn.example.test", got.CDNHost)
	}
	if got.Quality != "1080p" {
		t.Fatalf("Quality = %q, want 1080p", got.Quality)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestValidatePopulatesMetrics -count=1 -v`
Expected: FAIL — `Verdict` has no `ManifestMs`/`SegmentBytes`/`CDNHost`/`Quality` fields (compile error).

- [ ] **Step 3: Add Verdict fields + instrument Validate**

In `types.go`, extend `Verdict` (append after `Reason`):

```go
type Verdict struct {
	Provider  string
	AnimeUUID string
	AnimeName string
	Slot      AnimeSlot
	Server    string
	Stage     Stage
	Reason    streamprobe.Reason
	// Measurement fields, populated by HTTPValidator on the reached-playback
	// path (zero otherwise). Consumed by the engine to assemble TickMetrics.
	ManifestMs   int64
	SegmentMs    int64
	SegmentBytes int64
	CDNHost      string
	Quality      string
}
```

In `validator.go`, add imports `"regexp"` and `"time"` (time is already imported), and a package-level helper + host/quality extraction. Add near the other helpers:

```go
var resolutionRe = regexp.MustCompile(`RESOLUTION=\d+x(\d+)`)

// qualityFromMaster returns e.g. "1080p" from the first #EXT-X-STREAM-INF
// RESOLUTION tag, or "" when absent.
func qualityFromMaster(master []byte) string {
	if m := resolutionRe.FindSubmatch(master); m != nil {
		return string(m[1]) + "p"
	}
	return ""
}

// hostOf returns the hostname of a raw URL, or "" if unparseable.
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}
```

Then instrument `Validate`. Set `CDNHost` up-front, time the master fetch, record quality, and time the segment fetch. Replace the master-fetch block and the in-loop segment handling:

```go
func (v *HTTPValidator) Validate(ctx context.Context, rs ResolvedStream) Verdict {
	ctx, cancel := context.WithTimeout(ctx, validatorBudget)
	defer cancel()
	verdict := Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, AnimeName: rs.AnimeName, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback}
	verdict.CDNHost = hostOf(rs.MasterURL)

	mstart := time.Now()
	master, status, err := v.fetch(ctx, v.proxyURL(rs, rs.MasterURL))
	verdict.ManifestMs = time.Since(mstart).Milliseconds()
	if err != nil {
		verdict.Reason = streamprobe.ReasonCDNUnreachable
		return verdict
	}
	if status == http.StatusForbidden {
		verdict.Reason = streamprobe.ReasonStatus403
		return verdict
	}
	if status != http.StatusOK || len(master) == 0 {
		verdict.Reason = streamprobe.ReasonEmptyResponse
		return verdict
	}
	verdict.Quality = qualityFromMaster(master)
```

Keep the existing progressive-media (`!looksLikeManifest(master)`) block unchanged. Then in the `for hops` loop, time each hop's fetch and, when a media segment is reached, record `SegmentMs`/`SegmentBytes` before returning playable. Change the fetch line and the segment branch:

```go
		sstart := time.Now()
		body, st, err := v.fetch(ctx, v.proxyURL(rs, line))
		hopMs := time.Since(sstart).Milliseconds()
		if err != nil {
			verdict.Reason = streamprobe.ReasonCDNUnreachable
			return verdict
		}
		if st == http.StatusForbidden {
			verdict.Reason = streamprobe.ReasonStatus403
			return verdict
		}
		if st != http.StatusOK || len(body) == 0 {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		if !looksLikeManifest(body) {
			// reached a media segment — record throughput sample
			verdict.SegmentMs = hopMs
			verdict.SegmentBytes = int64(len(body))
			if encrypted {
				verdict.Reason = streamprobe.ReasonPlayable
				return verdict
			}
			if perr := v.prober.Probe(ctx, body); perr != nil {
				verdict.Reason = streamprobe.ReasonDecodeFailed
				return verdict
			}
			verdict.Reason = streamprobe.ReasonPlayable
			return verdict
		}
```

(Leave the `hasAES128(body)` propagation + `cur = body` lines that follow unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/probe/ -run TestValidatePopulatesMetrics -count=1 -v`
Expected: PASS

Also run the whole package to catch any construction sites broken by the struct change:
Run: `cd services/analytics && go test ./internal/probe/ -count=1`
Expected: PASS (measurement fields are zero-valued everywhere they aren't set).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/analytics/internal/probe/types.go services/analytics/internal/probe/validator.go services/analytics/internal/probe/validator_test.go
git commit -m "feat(analytics): measure manifest/segment latency, bytes, CDN host, quality in the validator" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Analytics — pre-probe warmup for browser providers

**Files:**
- Modify: `services/analytics/internal/probe/engine.go` (add `EngineBrowser` const + `warmup` method; call it in `RunOnce` ~L231)
- Test: `services/analytics/internal/probe/engine_test.go` (create if absent)

**Interfaces:**
- Consumes: `PlanEntry.Engine` (Task 2), `ProbeTarget.Resolver`.
- Produces: `func (e *Engine) warmup(ctx, t ProbeTarget, refs []AnimeRef) int64` — best-effort resolve of `refs[0]`, returns elapsed ms (0 if no refs). `const EngineBrowser = "browser"`.

- [ ] **Step 1: Write the failing test**

Add to `engine_test.go`. Use a counting fake `Resolver` (the `Resolver` interface is `Resolve(ctx, uuid, name string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error)` — match its exact signature from `validator.go`/`engine.go` usage):

```go
func TestWarmupResolvesTopRefBestEffort(t *testing.T) {
	var calls int
	res := resolverFunc(func(_ context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		calls++
		return nil, StageStream, errors.New("cold solve failed") // best-effort: error must be swallowed
	})
	e := &Engine{now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	ms := e.warmup(context.Background(), tgt, refs)
	if calls != 1 {
		t.Fatalf("warmup resolve calls = %d, want 1", calls)
	}
	if ms < 0 {
		t.Fatalf("warmup ms = %d, want >= 0", ms)
	}
	// No refs → no resolve, zero ms.
	calls = 0
	if got := e.warmup(context.Background(), tgt, nil); got != 0 || calls != 0 {
		t.Fatalf("empty warmup: ms=%d calls=%d, want 0/0", got, calls)
	}
}
```

Add the `resolverFunc` adapter (if the package doesn't already have one) in the test file:

```go
type resolverFunc func(context.Context, string, string, int, AnimeSlot, string) ([]ResolvedStream, Stage, error)

func (f resolverFunc) Resolve(ctx context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
	return f(ctx, uuid, name, ep, slot, prov)
}
```

> If `AnimeRef`'s episode field is not named `Episode`/typed `int`, match the real struct (see `engine.go` line ~81 `ref.Episode` usage). Adjust `resolverFunc`'s `int` param to the real episode type if it differs.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestWarmupResolvesTopRefBestEffort -count=1 -v`
Expected: FAIL — `e.warmup` undefined (compile error).

- [ ] **Step 3: Implement warmup + gate it in RunOnce**

In `engine.go`, add near the top (after imports; ensure `"time"` is imported):

```go
// EngineBrowser is the ScraperProvider.Engine value for Camoufox-resolved
// providers. Browser providers get a pre-probe warmup so the measured probe
// runs against a warm cf_clearance session instead of a cold Turnstile solve.
const EngineBrowser = "browser"

// warmup resolves the top ref once, best-effort, to prime the browser session
// (cold Cloudflare-Turnstile solve → cf_clearance cached on the pool profile).
// Errors are swallowed — a failed warmup never marks the provider down; the
// measured probe still runs and reports honestly. Returns elapsed ms.
func (e *Engine) warmup(ctx context.Context, t ProbeTarget, refs []AnimeRef) int64 {
	if len(refs) == 0 {
		return 0
	}
	r := refs[0]
	start := time.Now()
	_, _, _ = t.Resolver.Resolve(ctx, r.UUID, r.Name, r.Episode, r.Slot, t.Provider)
	return time.Since(start).Milliseconds()
}
```

In `RunOnce`, in the plan-driven loop, warm before probing (this task threads `warmupMs` no further yet — Task 5 consumes it):

```go
		refs, _ := t.AnimeSet.Resolve(ctx)
		var warmupMs int64
		if entry.Engine == EngineBrowser {
			warmupMs = e.warmup(ctx, t, refs)
			if e.log != nil {
				e.log.Infow("probe warmup complete", "provider", t.Provider, "warmup_ms", warmupMs)
			}
		}
		_ = warmupMs // consumed in Task 5
		verdicts, pass := e.probeProvider(ctx, t, refs, entry.SampleSize, entry.FailFast)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/probe/ -run TestWarmupResolvesTopRefBestEffort -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/analytics/internal/probe/engine.go services/analytics/internal/probe/engine_test.go
git commit -m "feat(analytics): pre-probe warmup for browser providers

One best-effort, unscored Resolve() eats the cold Turnstile solve so the
measured probe runs against a warm cf_clearance session. Fixes the cold-vs-warm
trap pinning animepahe/miruro down despite playing fine warm." -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Analytics — assemble `TickMetrics`

**Files:**
- Modify: `services/analytics/internal/probe/types.go` (add `TickMetrics` + internal `tickMeasure`)
- Modify: `services/analytics/internal/probe/engine.go` (`probeProvider` returns a measure; `RunOnce` builds `TickMetrics`)
- Test: `services/analytics/internal/probe/engine_test.go`

**Interfaces:**
- Produces:
  ```go
  type TickMetrics struct {
      At, Reason, ProviderUsed, Anime, Slot, CDNHost, Quality string
      Pass                                                    bool
      SampleSize                                              int
      WarmupMs, ResolveMs, ValidateMs, ThroughputKbps         int64
  }
  ```
  with JSON tags below. `probeProvider` signature becomes `(...) (verdicts []Verdict, pass bool, meas tickMeasure)`.

- [ ] **Step 1: Write the failing test**

Add to `engine_test.go` a test that drives `probeProvider` with a fake resolver returning one stream and a fake validator returning a playable verdict carrying measures, then asserts the assembled `tickMeasure`:

```go
func TestProbeProviderAssemblesMeasure(t *testing.T) {
	res := resolverFunc(func(_ context.Context, uuid, name string, ep int, slot AnimeSlot, prov string) ([]ResolvedStream, Stage, error) {
		return []ResolvedStream{{Provider: prov, AnimeUUID: uuid, AnimeName: name, Slot: slot, MasterURL: "https://cdn.test/m.m3u8"}}, StageStream, nil
	})
	val := validatorFunc(func(_ context.Context, rs ResolvedStream) Verdict {
		return Verdict{Provider: rs.Provider, AnimeName: rs.AnimeName, Slot: rs.Slot, Stage: StagePlayback,
			Reason: streamprobe.ReasonPlayable, ManifestMs: 40, SegmentMs: 20, SegmentBytes: 250000, CDNHost: "cdn.test", Quality: "720p"}
	})
	e := &Engine{val: val, now: func() int64 { return 0 }}
	tgt := ProbeTarget{Provider: "miruro", Resolver: res}
	refs := []AnimeRef{{UUID: "u1", Name: "Frieren", Slot: SlotAnchor}}

	_, pass, meas := e.probeProvider(context.Background(), tgt, refs, 1, true)
	if !pass {
		t.Fatal("expected pass")
	}
	if meas.CDNHost != "cdn.test" || meas.Quality != "720p" || meas.Anime != "Frieren" {
		t.Fatalf("bad meta: %+v", meas)
	}
	if meas.ValidateMs != 60 { // ManifestMs + SegmentMs
		t.Fatalf("ValidateMs = %d, want 60", meas.ValidateMs)
	}
	// throughput = bytes*8/segmentMs = 250000*8/20 = 100000 kbps
	if meas.ThroughputKbps != 100000 {
		t.Fatalf("ThroughputKbps = %d, want 100000", meas.ThroughputKbps)
	}
	if meas.SampleSize != 1 {
		t.Fatalf("SampleSize = %d, want 1", meas.SampleSize)
	}
}
```

Add the `validatorFunc` adapter to the test file if absent:

```go
type validatorFunc func(context.Context, ResolvedStream) Verdict

func (f validatorFunc) Validate(ctx context.Context, rs ResolvedStream) Verdict { return f(ctx, rs) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestProbeProviderAssemblesMeasure -count=1 -v`
Expected: FAIL — `probeProvider` returns 2 values, not 3 (compile error).

- [ ] **Step 3: Implement TickMetrics + measure assembly**

In `types.go`, add:

```go
// TickMetrics is the JSON summary of one probe tick for a provider. Persisted to
// stream_providers.last_tick_metrics and rendered on the Grafana "Last Tick
// Metrics" panel. *Ms are milliseconds; ThroughputKbps is kilobits/sec.
type TickMetrics struct {
	At             string `json:"at"`
	Pass           bool   `json:"pass"`
	Reason         string `json:"reason"`
	ProviderUsed   string `json:"provider_used"`
	Anime          string `json:"anime"`
	Slot           string `json:"slot"`
	SampleSize     int    `json:"sample_size"`
	WarmupMs       int64  `json:"warmup_ms,omitempty"`
	ResolveMs      int64  `json:"resolve_ms"`
	ValidateMs     int64  `json:"validate_ms"`
	ThroughputKbps int64  `json:"throughput_kbps,omitempty"`
	CDNHost        string `json:"cdn_host,omitempty"`
	Quality        string `json:"quality,omitempty"`
}

// tickMeasure is the per-tick measurement probeProvider gathers from the top
// ref; RunOnce finalizes it into a TickMetrics (adding At/Pass/Reason/Warmup).
type tickMeasure struct {
	ResolveMs, ValidateMs, ThroughputKbps int64
	CDNHost, Quality, Anime, Slot         string
	SampleSize                            int
}
```

In `engine.go`, change `probeProvider` to return `tickMeasure`. Update the signature and the `defer`/return sites (the panic-recovery `defer` must set named return `meas` implicitly — it stays zero, which is fine). Capture the top-ref resolve timing and the representative playable verdict's measures:

```go
func (e *Engine) probeProvider(ctx context.Context, t ProbeTarget, refs []AnimeRef, sampleSize int, failFast bool) (verdicts []Verdict, pass bool, meas tickMeasure) {
	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorw("probe provider panicked", "provider", t.Provider, "panic", r)
			}
			verdicts = append(verdicts, Verdict{Provider: t.Provider, Stage: StageStream, Reason: streamprobe.ReasonCDNUnreachable})
		}
	}()

	n := len(refs)
	if sampleSize > 0 && sampleSize < n {
		n = sampleSize
	}
	meas.SampleSize = n
	if n > 0 {
		meas.Anime = refs[0].Name
		meas.Slot = string(refs[0].Slot)
	}

	allPlayed := true
	topPlayed := false

	for i := 0; i < n; i++ {
		ref := refs[i]
		rstart := time.Now()
		streams, stage, rerr := t.Resolver.Resolve(ctx, ref.UUID, ref.Name, ref.Episode, ref.Slot, t.Provider)
		if i == 0 {
			meas.ResolveMs = time.Since(rstart).Milliseconds()
		}
		if rerr != nil {
			// ... existing error handling UNCHANGED ...
```

Leave the entire existing error-handling and validate loop bodies unchanged, EXCEPT: after collecting `refVerdicts` for ref 0, capture the representative measures. Right after the `verdicts = append(verdicts, refVerdicts...)` line, add:

```go
		if i == 0 {
			rep := refVerdicts[0]
			for _, rv := range refVerdicts {
				if rv.Reason == streamprobe.ReasonPlayable {
					rep = rv
					break
				}
			}
			meas.ValidateMs = rep.ManifestMs + rep.SegmentMs
			if rep.SegmentMs > 0 {
				meas.ThroughputKbps = rep.SegmentBytes * 8 / rep.SegmentMs
			}
			if rep.CDNHost != "" {
				meas.CDNHost = rep.CDNHost
			}
			if rep.Quality != "" {
				meas.Quality = rep.Quality
			}
		}
```

> Guard: `refVerdicts` is non-empty here (the loop only reaches this point after `refVerdicts` was appended from a successful resolve with ≥1 stream). If a resolve can succeed with zero streams in this codebase, wrap the `rep :=` block in `if len(refVerdicts) > 0 {`.

Keep the `pass` computation and the final `if len(verdicts)==0` / `if n==0 { pass=false }` unchanged, then `return` (named returns carry `meas`).

Now update BOTH `probeProvider` call sites in `RunOnce`:

Legacy fallback path:
```go
			verdicts, _, _ := e.probeProvider(ctx, t, refs, 0, false)
```

Plan-driven path — replace the Task-4 `_ = warmupMs` line and the probe call with the finalize + PostVerdict wiring (PostVerdict still takes 3 args until Task 6):
```go
		verdicts, pass, meas := e.probeProvider(ctx, t, refs, entry.SampleSize, entry.FailFast)

		pv := Rollup(t.Provider, filterProbed(verdicts))
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, pv)

		reason := ""
		if !pass {
			reason = pv.Reason
		}
		tm := &TickMetrics{
			At:             time.Unix(e.now(), 0).UTC().Format(time.RFC3339),
			Pass:           pass,
			Reason:         reason,
			ProviderUsed:   t.Provider,
			Anime:          meas.Anime,
			Slot:           meas.Slot,
			SampleSize:     meas.SampleSize,
			WarmupMs:       warmupMs,
			ResolveMs:      meas.ResolveMs,
			ValidateMs:     meas.ValidateMs,
			ThroughputKbps: meas.ThroughputKbps,
			CDNHost:        meas.CDNHost,
			Quality:        meas.Quality,
		}
		_ = tm // wired into PostVerdict in Task 6
		if postErr := e.plan.PostVerdict(ctx, t.Provider, pass, reason); postErr != nil {
```

(Delete the now-duplicated `pv := Rollup...`/`allVerdicts`/`provVerdicts`/`reason` lines that previously followed the probe call — they are folded into the block above.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/analytics && go test ./internal/probe/ -count=1`
Expected: PASS (all — including the new measure test and existing engine/reporter tests).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/analytics/internal/probe/types.go services/analytics/internal/probe/engine.go services/analytics/internal/probe/engine_test.go
git commit -m "feat(analytics): assemble per-tick TickMetrics (warmup/resolve/validate/throughput/CDN/quality)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: Analytics — ship `TickMetrics` on the probe-result POST

**Files:**
- Modify: `services/analytics/internal/probe/catalog_plan.go` (`PlanClient` interface, `httpPlanClient`, `PostVerdict` func)
- Modify: `services/analytics/internal/probe/engine.go` (`RunOnce` PostVerdict call — pass `tm`)
- Test: `services/analytics/internal/probe/catalog_plan_test.go`; update any fake `PlanClient` in `engine_test.go`

**Interfaces:**
- Changed: `PostVerdict(ctx, provider string, pass bool, reason string, metrics *TickMetrics) error` (interface + free function + `httpPlanClient` method).

- [ ] **Step 1: Write the failing test**

Add to `catalog_plan_test.go` — assert the POST body carries the metrics object:

```go
func TestPostVerdictIncludesMetrics(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tm := &TickMetrics{At: "2026-07-06T00:00:00Z", Pass: true, ProviderUsed: "miruro", WarmupMs: 9800, ResolveMs: 1900, ThroughputKbps: 5400, CDNHost: "kwik.cx"}
	if err := PostVerdict(context.Background(), srv.URL, srv.Client(), "miruro", true, "", tm); err != nil {
		t.Fatalf("PostVerdict: %v", err)
	}
	m, ok := gotBody["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics missing from body: %+v", gotBody)
	}
	if m["cdn_host"] != "kwik.cx" || m["warmup_ms"].(float64) != 9800 {
		t.Fatalf("bad metrics payload: %+v", m)
	}
}
```

(Requires `encoding/json` in the test imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestPostVerdictIncludesMetrics -count=1 -v`
Expected: FAIL — `PostVerdict` takes 5 args, not 6 (compile error).

- [ ] **Step 3: Extend PostVerdict + the interface + the call site**

In `catalog_plan.go`:

```go
type PlanClient interface {
	FetchPlan(ctx context.Context) ([]PlanEntry, error)
	PostVerdict(ctx context.Context, provider string, pass bool, reason string, metrics *TickMetrics) error
}

func (c *httpPlanClient) PostVerdict(ctx context.Context, p string, pass bool, reason string, metrics *TickMetrics) error {
	return PostVerdict(ctx, c.catalogURL, c.hc, p, pass, reason, metrics)
}

// PostVerdict reports a provider's probe pass/fail (+ optional last-tick metrics)
// to catalog's state machine. metrics may be nil (omitted from the body).
func PostVerdict(ctx context.Context, catalogURL string, c *http.Client, provider string, pass bool, reason string, metrics *TickMetrics) error {
	body := map[string]any{"provider": provider, "pass": pass, "reason": reason}
	if metrics != nil {
		body["metrics"] = metrics
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, catalogURL+"/internal/providers/probe-result", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe-result status %d", resp.StatusCode)
	}
	return nil
}
```

In `engine.go` `RunOnce`, pass the metrics and drop the `_ = tm`:

```go
		if postErr := e.plan.PostVerdict(ctx, t.Provider, pass, reason, tm); postErr != nil {
```

Update any fake `PlanClient` in `engine_test.go` to the new `PostVerdict` signature (add the `metrics *TickMetrics` param; a counting/recording fake can store the last metrics for assertions).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/analytics && go test ./internal/probe/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/analytics/internal/probe/catalog_plan.go services/analytics/internal/probe/engine.go services/analytics/internal/probe/catalog_plan_test.go services/analytics/internal/probe/engine_test.go
git commit -m "feat(analytics): POST last-tick metrics with the probe verdict" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: Catalog — persist + expose `last_tick_metrics`

**Files:**
- Modify: `services/catalog/internal/domain/scraper_provider.go` (add `LastTickMetrics` field)
- Modify: `services/catalog/internal/handler/internal_provider_policy.go` (`probeResultReq` + `ProbeResult` Updates)
- Modify: `services/catalog/internal/handler/internal_scraper_providers.go` (`providerWire` + `toWire`)
- Test: `services/catalog/internal/handler/internal_provider_policy_test.go`

**Interfaces:**
- Consumes: the `metrics` object on `POST /internal/providers/probe-result` (Task 6).
- Produces: `stream_providers.last_tick_metrics` (text/JSON) + `last_tick_metrics` in the admin health feed.

- [ ] **Step 1: Write the failing test**

Add to `internal_provider_policy_test.go`:

```go
func TestProbeResultPersistsMetrics(t *testing.T) {
	db := newHandlerTestDB(t)
	if err := db.Create(&domain.ScraperProvider{
		Name: "miruro", Policy: domain.PolicyManual, Health: domain.HealthDown,
		Engine: "browser", ScraperOperated: true, Group: "en",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := NewInternalProviderPolicyHandler(db, testPolicyCfg(), testLogger())

	body := `{"provider":"miruro","pass":true,"reason":"","metrics":{"warmup_ms":9800,"resolve_ms":1900,"cdn_host":"kwik.cx"}}`
	rr := httptest.NewRecorder()
	h.ProbeResult(rr, httptest.NewRequest(http.MethodPost, "/internal/providers/probe-result", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var p domain.ScraperProvider
	if err := db.First(&p, "name = ?", "miruro").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !strings.Contains(p.LastTickMetrics, `"cdn_host":"kwik.cx"`) {
		t.Fatalf("last_tick_metrics not persisted: %q", p.LastTickMetrics)
	}

	// A verdict WITHOUT metrics must not wipe the stored summary.
	rr2 := httptest.NewRecorder()
	h.ProbeResult(rr2, httptest.NewRequest(http.MethodPost, "/internal/providers/probe-result",
		strings.NewReader(`{"provider":"miruro","pass":false,"reason":"cdn_unreachable"}`)))
	_ = db.First(&p, "name = ?", "miruro")
	if !strings.Contains(p.LastTickMetrics, "kwik.cx") {
		t.Fatalf("metrics wiped by metrics-less verdict: %q", p.LastTickMetrics)
	}
}
```

(Requires `strings` in the test imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbeResultPersistsMetrics -count=1 -v`
Expected: FAIL — `p.LastTickMetrics` field doesn't exist (compile error).

- [ ] **Step 3: Add the column, decode + persist, expose in the feed**

In `domain/scraper_provider.go`, add after `AIProbeNotes`:

```go
	// LastTickMetrics is the JSON summary of the most recent probe tick (warmup/
	// resolve/validate timings, throughput, CDN, quality), written by the
	// probe-result handler and rendered on the Grafana "Last Tick Metrics" panel.
	// Stored as text (the JSON blob); the panel casts ::jsonb. Empty until first
	// probed under the warmup pipeline. Routine health/policy writes never touch
	// it, so it persists across auto-demote/promote cycles (like ai_probe_notes).
	LastTickMetrics string `gorm:"column:last_tick_metrics;type:text" json:"last_tick_metrics"`
```

In `internal_provider_policy.go`, extend `probeResultReq` and persist when present:

```go
type probeResultReq struct {
	Provider string          `json:"provider"`
	Pass     bool            `json:"pass"`
	Reason   string          `json:"reason"`
	Metrics  json.RawMessage `json:"metrics"`
}
```

In `ProbeResult`, build the updates map with the metrics conditionally (replace the `Updates(map[string]any{...})` literal with a variable so the optional key can be added):

```go
	updates := map[string]any{
		"policy":         p.Policy,
		"health":         p.Health,
		"health_since":   p.HealthSince,
		"policy_since":   p.PolicySince,
		"last_probed_at": p.LastProbedAt,
		"reason":         p.Reason,
	}
	if len(req.Metrics) > 0 {
		updates["last_tick_metrics"] = string(req.Metrics)
	}

	if err := h.db.Model(&domain.ScraperProvider{}).
		Where("name = ?", p.Name).
		Updates(updates).Error; err != nil {
```

In `internal_scraper_providers.go`, add to `providerWire` (after `Engine`/`BaseURL`) and set it in `toWire`:

```go
	LastTickMetrics  string    `json:"last_tick_metrics"`
```
```go
		LastTickMetrics:  p.LastTickMetrics,
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/handler/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add services/catalog/internal/domain/scraper_provider.go services/catalog/internal/handler/internal_provider_policy.go services/catalog/internal/handler/internal_scraper_providers.go services/catalog/internal/handler/internal_provider_policy_test.go
git commit -m "feat(catalog): persist + expose last_tick_metrics from the probe verdict" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 8: Grafana — "Last Tick Metrics" panel after the roster

**Files:**
- Modify: `docker/grafana/dashboards/playback-health.json`

**Interfaces:**
- Consumes: `stream_providers.last_tick_metrics` (Task 7).

- [ ] **Step 1: Add the panel + shift the panels below it**

The roster panel `"Provider Roster & Playability"` is `id:102`, `gridPos {h:12,w:24,x:0,y:6}` (ends at y=18). The next panel row `"Provider Health"` starts at y=18. Insert the new panel at `y:18 h:9` and shift EVERY panel currently at `y >= 18` down by 9.

Use `jq` to do the shift deterministically, then splice the panel. Run from the worktree root:

```bash
cd /data/ae-probe-warmup-metrics
F=docker/grafana/dashboards/playback-health.json

# 1. Shift every panel at y>=18 down by 9 to make room.
jq '(.panels[] | select(.gridPos.y >= 18) | .gridPos.y) += 9' "$F" > "$F.tmp" && mv "$F.tmp" "$F"

# 2. Splice the new panel right after the roster (id 102). Written as a jq arg.
jq --argjson p '{
  "id": 140,
  "type": "table",
  "title": "Last Tick Metrics",
  "description": "Summary of each provider'\''s most recent probe tick: warmup (browser cold-solve), resolve + validate latency, first-segment throughput, CDN host and quality.",
  "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
  "gridPos": { "h": 9, "w": 24, "x": 0, "y": 18 },
  "options": { "showHeader": true },
  "fieldConfig": { "defaults": { "custom": { "align": "auto" } }, "overrides": [] },
  "targets": [
    {
      "refId": "A",
      "format": "table",
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "rawSql": "SELECT name AS \"Provider\", round((last_tick_metrics::jsonb->>'\''warmup_ms'\'')::numeric/1000,1) AS \"Warmup s\", round((last_tick_metrics::jsonb->>'\''resolve_ms'\'')::numeric/1000,2) AS \"Resolve s\", round((last_tick_metrics::jsonb->>'\''validate_ms'\'')::numeric/1000,2) AS \"Validate s\", round((last_tick_metrics::jsonb->>'\''throughput_kbps'\'')::numeric/1000,1) AS \"Speed Mbps\", last_tick_metrics::jsonb->>'\''cdn_host'\'' AS \"CDN\", last_tick_metrics::jsonb->>'\''quality'\'' AS \"Quality\", (last_tick_metrics::jsonb->>'\''at'\'')::timestamptz AS \"Last tick\" FROM stream_providers WHERE last_tick_metrics <> '\'''\'' AND last_tick_metrics IS NOT NULL ORDER BY \"group\", name;"
    }
  ]
}' '.panels |= (reduce range(0; length) as $i ([]; . + [.[$i]] + (if $env.SPLICE_AFTER == ($i|tostring) then [] else [] end)))' "$F" >/dev/null 2>&1 || true
```

> The reduce trick above is fiddly. Simpler + reliable: splice by finding the roster panel's array index and inserting after it. Use this instead:

```bash
cd /data/ae-probe-warmup-metrics
F=docker/grafana/dashboards/playback-health.json
PANEL='{"id":140,"type":"table","title":"Last Tick Metrics","description":"Summary of each provider'"'"'s most recent probe tick: warmup (browser cold-solve), resolve + validate latency, first-segment throughput, CDN host and quality.","datasource":{"type":"postgres","uid":"aenigma-postgres"},"gridPos":{"h":9,"w":24,"x":0,"y":18},"options":{"showHeader":true},"fieldConfig":{"defaults":{"custom":{"align":"auto"}},"overrides":[]},"targets":[{"refId":"A","format":"table","datasource":{"type":"postgres","uid":"aenigma-postgres"},"rawSql":"SELECT name AS \"Provider\", round((last_tick_metrics::jsonb->>'"'"'warmup_ms'"'"')::numeric/1000,1) AS \"Warmup s\", round((last_tick_metrics::jsonb->>'"'"'resolve_ms'"'"')::numeric/1000,2) AS \"Resolve s\", round((last_tick_metrics::jsonb->>'"'"'validate_ms'"'"')::numeric/1000,2) AS \"Validate s\", round((last_tick_metrics::jsonb->>'"'"'throughput_kbps'"'"')::numeric/1000,1) AS \"Speed Mbps\", last_tick_metrics::jsonb->>'"'"'cdn_host'"'"' AS \"CDN\", last_tick_metrics::jsonb->>'"'"'quality'"'"' AS \"Quality\", (last_tick_metrics::jsonb->>'"'"'at'"'"')::timestamptz AS \"Last tick\" FROM stream_providers WHERE last_tick_metrics <> '"'"''"'"' AND last_tick_metrics IS NOT NULL ORDER BY \"group\", name;"}]}'
jq --argjson panel "$PANEL" '
  .panels as $ps
  | (.panels | map(.id == 102) | index(true)) as $idx
  | .panels = ($ps[0:$idx+1] + [$panel] + $ps[$idx+1:])
' "$F" > "$F.tmp" && mv "$F.tmp" "$F"
```

- [ ] **Step 2: Verify the JSON is valid and the panel is present**

```bash
cd /data/ae-probe-warmup-metrics
python3 -m json.tool docker/grafana/dashboards/playback-health.json > /dev/null && echo "JSON OK"
jq '.panels[] | select(.title=="Last Tick Metrics") | {title, y:.gridPos.y, ds:.targets[0].datasource.type}' docker/grafana/dashboards/playback-health.json
jq '[.panels[] | {t:.title, y:.gridPos.y}] | sort_by(.y)' docker/grafana/dashboards/playback-health.json
```
Expected: `JSON OK`; the panel prints with `y:18 ds:"postgres"`; the sorted list shows `Provider Roster & Playability` (y:6) → `Last Tick Metrics` (y:18) → `Provider Health` (y:27) with no y-overlap.

- [ ] **Step 3: Commit**

```bash
cd /data/ae-probe-warmup-metrics
git add docker/grafana/dashboards/playback-health.json
git commit -m "feat(grafana): Last Tick Metrics panel after the provider roster

Postgres-datasource table over stream_providers.last_tick_metrics (::jsonb):
warmup/resolve/validate latency, first-segment speed, CDN host, quality." -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 9: Integrate, deploy, and verify end-to-end

**Files:** none (deploy + verification). This runs from the **base tree** `/data/animeenigma` AFTER the branch is merged to `main` (see `/animeenigma-after-update`), because `make redeploy-*` builds from the shared tree.

- [ ] **Step 1: Full package tests + build**

```bash
cd /data/ae-probe-warmup-metrics
( cd services/analytics && go build ./... && go test ./internal/probe/ -count=1 )
( cd services/catalog && go build ./... && go test ./internal/handler/ -count=1 )
```
Expected: all PASS, both build.

- [ ] **Step 2: Land on main + redeploy (via after-update)**

Redeploy `analytics` and `catalog`; restart `grafana` (dashboard JSON is provisioned, no code build):
```bash
cd /data/animeenigma
make redeploy-analytics
make redeploy-catalog
make restart-grafana
make health
```
Expected: `make health` green for analytics, catalog, grafana.

- [ ] **Step 3: Force an animepahe probe and assert the column populates**

```bash
# Backdate so animepahe enters the plan (24h manual cadence), then run the probe.
docker exec animeenigma-postgres psql -U postgres -d animeenigma -c \
  "UPDATE stream_providers SET last_probed_at = now() - interval '25 hours' WHERE name='animepahe';"
curl -s -X POST http://127.0.0.1:8092/internal/probe/run
# Give it a few ticks, then read the metrics back.
docker exec animeenigma-postgres psql -U postgres -d animeenigma -c \
  "SELECT name, last_tick_metrics FROM stream_providers WHERE name IN ('animepahe','miruro');"
```
Expected: `last_tick_metrics` is a populated JSON blob for the probed provider(s), containing `warmup_ms`, `resolve_ms`, `cdn_host`. (Backdating writes shared prod state — it needs owner authorization; if denied, wait for the next automated probe instead.)

- [ ] **Step 4: Confirm the Grafana panel renders**

Load `https://animeenigma.org/admin/grafana` → Playback Health dashboard → the "Last Tick Metrics" table appears directly under "Provider Roster & Playability" with populated rows. (Opt-in Chrome smoke per DS-NF-06 — ask the owner before running a browser check.)

- [ ] **Step 5: Follow the after-update flow**

Run `/animeenigma-after-update` to lint, changelog (Trump-mode), and push. The changelog entry should credit the warmup fix + the new metrics panel.

---

## Self-Review

**Spec coverage:**
- Warmup (browser-only, back-to-back, unscored, `warmup_ms`) → Tasks 4, 5. ✓
- `engine` on the plan (drift-proof) → Tasks 1, 2. ✓
- Metrics (warmup/resolve/validate/throughput/cdn/quality/metadata) → Tasks 3, 5. ✓
- Transport (metrics on probe-result) → Task 6. ✓
- Storage (`last_tick_metrics` column, write-when-present) → Task 7. ✓
- Surface: admin feed → Task 7; Grafana dedicated panel after roster → Task 8. ✓
- Throughput = first-segment fetch (reused validator bytes) → Task 3. ✓
- `pool_size=2` reuse verify → Task 9 Step 3 (E2E on animepahe). ✓

**Placeholder scan:** No TBD/TODO; each code step shows the code. The two hedges ("if `testPolicyCfg` differs…", "if `AnimeRef.Episode` differs…") are typed-fact confirmations against real files, not missing content.

**Type consistency:** `TickMetrics`/`tickMeasure` fields match across types.go → engine.go → catalog_plan.go. `PostVerdict` 6-arg signature consistent across interface + method + free func + call site + test. `Verdict` measurement fields (`ManifestMs/SegmentMs/SegmentBytes/CDNHost/Quality`) defined in Task 3, consumed in Task 5. `probeProvider` 3-return consistent across both call sites (legacy fallback + plan path). `last_tick_metrics` column name consistent: domain tag, Updates key, Grafana SQL.
