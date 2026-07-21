# Graduated Degradation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the coarse 0/1/2 degradation shedding in content-verify and Camoufox with a continuous PSI-derived pressure score (0.00–1.00) driving graduated worker/instance caps, plus a user-stream-sacred 3-stage Camoufox scale-down.

**Architecture:** Recording rules normalize existing PSI signals to per-signal scores anchored at (half-elevated→0, elevated→0.5, critical→1.0); the governor smooths the max (rise-fast/decay-slow) and publishes `ae:degradation:score` alongside the untouched integer level. content-verify maps `min(curve(score), demandCap)` to per-loop sit-outs; Camoufox maps the score to a pool target with a drain/migrate/force-kill ladder that never kills a browser streaming for a user.

**Tech Stack:** PromQL recording rules, Go (governor, libs/cache, content-verify), Python asyncio (stealth-scraper), Grafana provisioned JSON.

**Spec:** `docs/superpowers/specs/2026-07-21-graduated-degradation-design.md` — read it first.

## Global Constraints

- All work in a worktree off fresh `origin/main`; NEVER edit `/data/animeenigma` base tree. Commit by pathspec (never bare `git commit` without paths, never `git add -A`), co-authors on every commit: `Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`.
- New Prometheus metrics go in service-local packages (`govmetrics`, `cvmetrics`, stealth `metrics.py`) — NEVER `libs/metrics` (auto-registration trap: plain promauto in libs exports permanent-0 impostors from every importer).
- Do NOT run `go work sync` (bumps unrelated modules fleet-wide). Never `gofmt -w` / `make fmt` (smart-quote landmine).
- The integer level machine, its Redis keys, override semantics, and every binary consumer (library, scheduler, catalog backfill) stay untouched.
- Fail-open everywhere: missing/invalid score anywhere ⇒ 0.0 ⇒ full speed.
- Go tests: `go test ./...` from the service dir. Stealth tests: `cd services/stealth-scraper && python3 -m unittest discover -s tests -v`.
- content-verify k8s stays `replicas: 1` (in-process leases).
- Effort metrics: UXΔ/CDI/MVQ only — never days/hours.

---

### Task 1: Phase-0 budget raises (config only)

**Files:**
- Modify: `services/stealth-scraper/app/config.py:144-145` (RAM defaults), `:50` + `:174` (pool_size), add `pool_curve` field
- Modify: `services/content-verify/internal/config/config.go` (`clampWorkers` bound 4→6)
- Create: `services/content-verify/internal/config/config_test.go`
- Modify: `docker/docker-compose.yml` (stealth env defaults ~line 172-177; cv `mem_limit` + `CV_WORKERS` ~line 1147+)
- Modify (assert-updates only if they fail): `services/stealth-scraper/tests/test_config_capacity.py`

**Interfaces:**
- Produces: `Config.pool_curve: str` (stealth, default `"0.40:6,0.60:2,0.80:1"`, env `STEALTH_POOL_CURVE`) — consumed by Task 6. `clampWorkers` ceiling 6 — consumed by Task 5.

- [ ] **Step 1: Write the failing Go test**

```go
// services/content-verify/internal/config/config_test.go
package config

import "testing"

// Graduated-degradation Phase 0: the worker ceiling is 6 (was 4) so the
// score curve's top band is reachable. Floor stays 1.
func TestClampWorkersBounds(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1}, {-3, 1}, {1, 1}, {4, 4}, {6, 6}, {7, 6}, {100, 6},
	}
	for _, c := range cases {
		if got := clampWorkers(c.in); got != c.want {
			t.Errorf("clampWorkers(%d) = %d; want %d", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run it — must fail**

Run: `cd services/content-verify && go test ./internal/config/ -run TestClampWorkersBounds -v`
Expected: FAIL — `clampWorkers(6) = 4; want 6`

- [ ] **Step 3: Raise the clamp**

In `services/content-verify/internal/config/config.go` change the `clampWorkers` body and comment:

```go
// clampWorkers silently clamps CV_WORKERS to [1,6] — the in-process probe
// pool has no supervisor to report a misconfiguration to, so out-of-range
// values are corrected rather than rejected. Ceiling raised 4→6 for the
// graduated-degradation score curve (spec 2026-07-21); the curve + demand
// cap decide how many of the spawned loops actually claim.
func clampWorkers(n int) int {
	if n < 1 {
		return 1
	}
	if n > 6 {
		return 6
	}
	return n
}
```

Also update the `Workers` struct-field comment: `// concurrent in-process probe loops (clamped 1..6)`.

- [ ] **Step 4: Run test — passes; then stealth config defaults**

Run: `cd services/content-verify && go test ./internal/config/ -v`
Expected: PASS (all).

In `services/stealth-scraper/app/config.py` change (keeping surrounding comments, updating the numbers mentioned in them):

```python
    pool_size: int = 6
    ...
    # Host-fit budget raised 2026-07-21 (graduated-degradation Phase 0): the
    # score curve is the regulator now; these are generous ceilings, not the
    # primary brake. ~1 GiB per warm Firefox => 6 lightly-loaded instances
    # fit the hard budget with little slack.
    ram_soft_bytes: int = 4 * 1024 * 1024 * 1024   # 4 GiB
    ram_hard_bytes: int = 6 * 1024 * 1024 * 1024   # 6 GiB
    ...
    # Graduated pool curve (spec 2026-07-21): "score:cap,..." breakpoints,
    # linear between points, floor()-rounded, clamped to [1, pool_size].
    pool_curve: str = "0.40:6,0.60:2,0.80:1"
```

And in the `load()`/env section:

```python
            pool_size=_int(g("STEALTH_POOL_SIZE"), 6),
            ...
            ram_soft_bytes=_int(g("STEALTH_RAM_SOFT_BYTES"), 4 * 1024 * 1024 * 1024),
            ram_hard_bytes=_int(g("STEALTH_RAM_HARD_BYTES"), 6 * 1024 * 1024 * 1024),
            ...
            pool_curve=g("STEALTH_POOL_CURVE", "0.40:6,0.60:2,0.80:1").strip(),
```

- [ ] **Step 5: Compose defaults + mem_limit**

In `docker/docker-compose.yml` stealth-scraper env block:

```yaml
      STEALTH_POOL_SIZE: ${STEALTH_POOL_SIZE:-6}
      # RAM budgets raised 2026-07-21 (graduated degradation Phase 0): the
      # PSI score curve is the regulator; these are ceilings. 4 GiB soft / 6 GiB hard.
      STEALTH_RAM_SOFT_BYTES: ${STEALTH_RAM_SOFT_BYTES:-4294967296}
      STEALTH_RAM_HARD_BYTES: ${STEALTH_RAM_HARD_BYTES:-6442450944}
```

content-verify block:

```yaml
    mem_limit: 6g # up to 6 CV_WORKERS probe loops => up to 6 whisper runs at ~1g each (graduated curve caps actual concurrency)
```

and in its `environment:` add:

```yaml
      CV_WORKERS: 6
```

- [ ] **Step 6: Run stealth config tests, fix assertions on old defaults**

Run: `cd services/stealth-scraper && python3 -m unittest tests.test_config_capacity -v`
If any assertion pins the old `2 GiB`/`3 GiB`/`pool_size 4` defaults, update the expected values to `4 GiB`/`6 GiB`/`6` (the test intent — env parsing — is unchanged).
Expected: OK.

- [ ] **Step 7: Commit**

```bash
git add services/content-verify/internal/config/ services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_config_capacity.py docker/docker-compose.yml
git commit -m "feat(degradation): phase-0 budget raises for graduated curves" -- services/content-verify/internal/config services/stealth-scraper/app/config.py services/stealth-scraper/tests/test_config_capacity.py docker/docker-compose.yml
```

(Include the standard co-author trailer in this and every commit below.)

---

### Task 2: Pressure-score recording rules

**Files:**
- Modify: `docker/prometheus/rules/degradation.yml` (append to the `ae_degradation_levels` group)

**Interfaces:**
- Produces: series `ae:pressure_score:signal{signal=...}` (0..1 per signal) and `ae:pressure_score:preview` (max across signals). Task 3's governor parses `ae:pressure_score:preview` by exact name.

- [ ] **Step 1: Append the score rules**

Append inside the `ae_degradation_levels` group (after `ae:pressure_level:preview`):

```yaml
      # --- Continuous pressure score (graduated degradation, spec 2026-07-21) ---
      # Piecewise-linear per-signal normalization anchored to the EXISTING
      # thresholds: 0 at half-elevated, 0.5 at elevated, 1.0 at critical —
      # so the score inherits the 07-14 baseline calibration. Governor smooths
      # max() of these and publishes ae:degradation:score; consumers map the
      # score to worker/instance caps via their own curves.
      - record: ae:pressure_score:signal
        expr: >-
          0.5 * clamp_max(clamp_min((ae:host_psi_cpu_some:ratio - 0.125) / 0.125, 0), 1)
          + 0.5 * clamp_max(clamp_min((ae:host_psi_cpu_some:ratio - 0.25) / 0.20, 0), 1)
        labels: {signal: psi_cpu_some}
      - record: ae:pressure_score:signal
        expr: >-
          0.5 * clamp_max(clamp_min((ae:host_psi_io_full:ratio - 0.125) / 0.125, 0), 1)
          + 0.5 * clamp_max(clamp_min((ae:host_psi_io_full:ratio - 0.25) / 0.05, 0), 1)
        labels: {signal: psi_io_full}
      - record: ae:pressure_score:signal
        expr: >-
          0.5 * clamp_max(clamp_min((ae:host_psi_mem_full:ratio - 0.06) / 0.06, 0), 1)
          + 0.5 * clamp_max(clamp_min((ae:host_psi_mem_full:ratio - 0.12) / 0.03, 0), 1)
        labels: {signal: psi_mem_full}
      # mem_available is inverted (lower = worse): 0 at 0.15, 0.5 at 0.10 (elevated), 1.0 at 0.05 (critical).
      - record: ae:pressure_score:signal
        expr: >-
          0.5 * clamp_max(clamp_min((0.15 - ae:host_mem_available:ratio) / 0.05, 0), 1)
          + 0.5 * clamp_max(clamp_min((0.10 - ae:host_mem_available:ratio) / 0.05, 0), 1)
        labels: {signal: mem_available}
      - record: ae:pressure_score:preview
        expr: clamp_max(max(ae:pressure_score:signal), 1)
```

- [ ] **Step 2: Validate with promtool (against the WORKTREE copy)**

```bash
docker run --rm -v "$(git rev-parse --show-toplevel)/docker/prometheus/rules:/r:ro" \
  --entrypoint promtool "$(docker inspect animeenigma-prometheus --format '{{.Config.Image}}')" \
  check rules /r/degradation.yml
```

Expected: `SUCCESS: <N> rules found` (N grows by 5), zero errors.

- [ ] **Step 3: Commit**

```bash
git add docker/prometheus/rules/degradation.yml
git commit -m "feat(degradation): per-signal pressure-score recording rules + preview" -- docker/prometheus/rules/degradation.yml
```

(Deploy note for Task 10: dir-mounted rules ⇒ base-tree ff-sync + `curl -X POST http://localhost:9090/prometheus/-/reload` — NO recreate.)

---

### Task 3: Governor publishes the smoothed score

**Files:**
- Modify: `libs/cache/degradation.go` (add `DegradationScoreKey` const only — reader comes in Task 4)
- Modify: `services/governor/internal/domain/degradation.go` (Verdict.Score, Snapshot.Score, RedisScoreKey)
- Modify: `services/governor/internal/promquery/client.go` (parse preview series)
- Create: `services/governor/internal/service/smoother.go` + `services/governor/internal/service/smoother_test.go`
- Modify: `services/governor/internal/service/governor.go` (smooth + override-pin + publish), `services/governor/internal/repo/redis_store.go` (PublishLevel gains score), `services/governor/internal/govmetrics/metrics.go` (score gauge), `services/governor/internal/config/config.go` (alpha envs), `services/governor/cmd/governor-api/main.go` (wire alphas)
- Modify: existing governor/service tests' fake `LevelStore` implementations (signature change)

**Interfaces:**
- Consumes: `ae:pressure_score:preview` (Task 2).
- Produces: Redis key `ae:degradation:score` (string `"0.42"`, 2 decimals, same TTL as level — consumed by Tasks 4/6); `domain.Snapshot.Score float64 \`json:"score"\`` (served on `/api/degradation/status` — consumed by Task 6); gauge `ae_degradation_score`; `NewSmoother(alphaUp, alphaDown float64) *Smoother` with `Tick(raw float64) float64` and `Reset()`.

- [ ] **Step 1: Write the failing smoother test**

```go
// services/governor/internal/service/smoother_test.go
package service

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSmootherRisesFastDecaysSlow(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	// Step input 0 -> 1: alphaUp=0.5 halves the gap each tick.
	if v := s.Tick(1.0); !almost(v, 0.5) {
		t.Fatalf("tick1 = %v; want 0.5", v)
	}
	if v := s.Tick(1.0); !almost(v, 0.75) {
		t.Fatalf("tick2 = %v; want 0.75", v)
	}
	// Input drops to 0: alphaDown=0.05 decays 5% of the gap per tick.
	if v := s.Tick(0); !almost(v, 0.75*0.95) {
		t.Fatalf("decay tick = %v; want %v", v, 0.75*0.95)
	}
}

func TestSmootherSnapsToZeroAndReset(t *testing.T) {
	s := NewSmoother(0.5, 0.05)
	s.Tick(0.004) // tiny raw
	// Residue below 0.005 with raw 0 snaps to exact 0 so a recovered box
	// publishes a clean 0.00.
	if v := s.Tick(0); v != 0 {
		t.Fatalf("snap = %v; want exact 0", v)
	}
	s.Tick(1.0)
	s.Reset()
	if v := s.Tick(0); v != 0 {
		t.Fatalf("post-reset = %v; want 0", v)
	}
}
```

- [ ] **Step 2: Run — must fail (NewSmoother undefined)**

Run: `cd services/governor && go test ./internal/service/ -run TestSmoother -v`
Expected: FAIL — `undefined: NewSmoother`

- [ ] **Step 3: Implement the smoother**

```go
// services/governor/internal/service/smoother.go
package service

// Smoother is the asymmetric EWMA over the raw pressure score: rise fast so a
// genuine ramp is tracked in ~4 ticks (~60s at the 15s tick, mirroring the
// level machine's enterTicks), decay slow (~5min, mirroring exitTicks) so the
// probes→pressure→fewer-probes feedback loop steps down and STAYS down
// instead of oscillating. Pure — no clock, no IO.
type Smoother struct {
	alphaUp, alphaDown float64
	value              float64
}

// NewSmoother builds a Smoother. Alphas outside (0,1] are clamped to sane
// defaults (0.5 up, 0.05 down).
func NewSmoother(alphaUp, alphaDown float64) *Smoother {
	if alphaUp <= 0 || alphaUp > 1 {
		alphaUp = 0.5
	}
	if alphaDown <= 0 || alphaDown > 1 {
		alphaDown = 0.05
	}
	return &Smoother{alphaUp: alphaUp, alphaDown: alphaDown}
}

// Tick feeds one raw score sample and returns the smoothed value.
func (s *Smoother) Tick(raw float64) float64 {
	a := s.alphaDown
	if raw > s.value {
		a = s.alphaUp
	}
	s.value += a * (raw - s.value)
	// Snap the asymptotic tail to a clean 0.00 once recovered.
	if raw == 0 && s.value < 0.005 {
		s.value = 0
	}
	return s.value
}

// Reset zeroes the state (fail-open after sustained Prometheus loss).
func (s *Smoother) Reset() { s.value = 0 }
```

- [ ] **Step 4: Run — smoother tests pass**

Run: `cd services/governor && go test ./internal/service/ -run TestSmoother -v`
Expected: PASS.

- [ ] **Step 5: Thread the score through domain, promquery, repo, metrics, config**

`libs/cache/degradation.go` — below `DegradationLevelKey`:

```go
// DegradationScoreKey is the Redis key the governor publishes the continuous
// pressure score to ("0.00".."1.00", TTL'd). Mirrors
// services/governor/internal/domain.RedisScoreKey — keep in sync.
const DegradationScoreKey = "ae:degradation:score"
```

`domain/degradation.go`:
- In the Redis-keys const block: `RedisScoreKey = cache.DegradationScoreKey` (with a comment: `// RedisScoreKey holds the smoothed pressure score as "0.00".."1.00".`)
- `Verdict` gains: `// Score is the raw (pre-smoothing) ae:pressure_score:preview sample.` `Score float64`
- `Snapshot` gains: `Score float64 \`json:"score"\`` (after `Level`).

`promquery/client.go`:
- Add const: `scoreName = "ae:pressure_score:preview"` next to the breach names.
- In the result-fold switch add a case BEFORE the hostSignalPrefix case:

```go
		case name == scoreName:
			v.Score = val
```

`repo/redis_store.go` — PublishLevel signature + body:

```go
func (s *RedisStore) PublishLevel(ctx context.Context, level domain.Level, score float64, reasons []domain.Reason, ttl time.Duration) error {
	if err := s.client.Set(ctx, domain.RedisLevelKey, strconv.Itoa(int(level)), ttl).Err(); err != nil {
		return fmt.Errorf("set level: %w", err)
	}
	if err := s.client.Set(ctx, domain.RedisScoreKey, strconv.FormatFloat(score, 'f', 2, 64), ttl).Err(); err != nil {
		return fmt.Errorf("set score: %w", err)
	}
	blob, err := json.Marshal(reasons)
	...unchanged...
}
```

`govmetrics/metrics.go` — add:

```go
	// DegradationScore is the authoritative smoothed pressure score (0.00-1.00)
	// after asymmetric EWMA and override pinning.
	DegradationScore = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ae_degradation_score",
			Help: "Published continuous pressure score (0.00 normal .. 1.00 critical), smoothed.",
		},
	)
```

`config/config.go` — add fields `ScoreAlphaUp`, `ScoreAlphaDown float64` loaded via a `getEnvFloat` helper (add one mirroring `getEnvDuration` if absent): `GOVERNOR_SCORE_ALPHA_UP` default `0.5`, `GOVERNOR_SCORE_ALPHA_DOWN` default `0.05`.

- [ ] **Step 6: Integrate into the tick loop**

`service/governor.go`:
- `LevelStore` interface: `PublishLevel(ctx context.Context, level domain.Level, score float64, reasons []domain.Reason, ttl time.Duration) error`.
- `Governor` struct gains `smoother *Smoother`; `New(...)` gains trailing params `alphaUp, alphaDown float64` and sets `smoother: NewSmoother(alphaUp, alphaDown)`.
- In `RunTick`, after the machine tick:

```go
	score := 0.0
	if promHealthy {
		score = g.smoother.Tick(verdict.Score)
	} else if g.failCountAtLeast(g.promFailTicks) {
		g.smoother.Reset() // sustained loss: fail-open to a clean 0
	} else {
		score = g.smoother.Tick(0) // grace window: decay, don't freeze
	}
```

Implement `failCountAtLeast(n int) bool` as a small mu-guarded reader. NOTE: the existing early-return grace-window branch (`if fails < g.promFailTicks { g.publish(...); return }`) must now pass a score — change that call to `g.publish(ctx, g.currentPublished(), g.smoother.Tick(0), g.Snapshot().Reasons)` and delete the `else`-branch duplication above accordingly (one smoother tick per RunTick — do not tick twice on the same cycle).
- Override pinning, after the existing override block:

```go
	if override != nil {
		score = map[domain.Level]float64{
			domain.LevelNormal: 0, domain.LevelElevated: 0.5, domain.LevelCritical: 1.0,
		}[*override]
	}
```

- `publish` signature: `publish(ctx context.Context, level domain.Level, score float64, reasons []domain.Reason)`; body adds `govmetrics.DegradationScore.Set(score)` and passes score to `PublishLevel`.
- `snapshot` assignment adds `Score: score`.

`cmd/governor-api/main.go`: pass `cfg.ScoreAlphaUp, cfg.ScoreAlphaDown` to `service.New`.

- [ ] **Step 7: Fix fakes, run the full governor suite**

Any test fake implementing `LevelStore` gains the `score float64` param (record it on the fake for assertions). Add one integration-style assertion to the existing governor test file: after a `RunTick` with a verdict carrying `Score: 1.0`, the fake store's recorded score is `0.5` (first tick, alphaUp 0.5), and with an override of level 1 set the recorded score is exactly `0.5` regardless of raw.

Run: `cd services/governor && go test ./... && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 8: Commit**

```bash
git add libs/cache/degradation.go services/governor/
git commit -m "feat(governor): publish smoothed continuous pressure score" -- libs/cache/degradation.go services/governor
```

---

### Task 4: libs/cache DegradationWatcher.Score()

**Files:**
- Modify: `libs/cache/degradation.go`
- Modify: `libs/cache/degradation_test.go`

**Interfaces:**
- Produces: `func (w *DegradationWatcher) Score() float64` — nil-safe, fail-open 0.0, clamped [0,1]. Consumed by Task 5.

- [ ] **Step 1: Write the failing test**

Mirror the existing test setup in `libs/cache/degradation_test.go` (it already constructs a watcher against a Redis fixture — reuse the same helper/fixture pattern for these):

```go
func TestWatcherScoreReadsKeyAndFailsOpen(t *testing.T) {
	// setup identical to the existing Level tests in this file
	// 1) key "ae:degradation:score" = "0.42"  -> Score() == 0.42
	// 2) key absent                            -> Score() == 0.0
	// 3) key = "garbage"                       -> Score() == 0.0
	// 4) key = "7.5" (out of range)            -> Score() == 1.0 (clamped)
	// 5) (*DegradationWatcher)(nil).Score()    -> 0.0, no panic
}
```

Write the five cases as real code following the file's existing fixture (do not invent a new Redis harness — copy the arrange/act pattern of `TestWatcherLevel...` in the same file, calling `w.refresh(ctx)` between writes exactly as those tests do).

- [ ] **Step 2: Run — must fail**

Run: `cd libs/cache && go test -run TestWatcherScore -v`
Expected: FAIL — `w.Score undefined`

- [ ] **Step 3: Implement**

In `degradation.go`:

```go
import "math"           // add
// struct gains:
	score atomic.Uint64 // math.Float64bits-encoded

// Score returns the last-read continuous pressure score (0.00..1.00).
// Safe on a nil receiver (returns 0). Fail-open: missing key, parse error,
// or Redis error all read as 0.0.
func (w *DegradationWatcher) Score() float64 {
	if w == nil {
		return 0
	}
	return math.Float64frombits(w.score.Load())
}
```

And extend `refresh` (same context/timeout) to read the score key after the level key:

```go
	sraw, serr := w.cache.Client().Get(rctx, DegradationScoreKey).Result()
	if serr != nil {
		w.score.Store(0)
		return
	}
	f, serr := strconv.ParseFloat(sraw, 64)
	if serr != nil {
		w.score.Store(0)
		return
	}
	w.score.Store(math.Float64bits(math.Max(0, math.Min(1, f))))
```

(The existing level-key early-returns must ALSO zero the score before returning — a dead governor zeroes both.)

- [ ] **Step 4: Run — passes**

Run: `cd libs/cache && go test ./... -v`
Expected: PASS (all, including pre-existing).

- [ ] **Step 5: Commit**

```bash
git add libs/cache/
git commit -m "feat(cache): DegradationWatcher continuous score reader" -- libs/cache
```

---

### Task 5: content-verify graduated worker curve

**Files:**
- Create: `services/content-verify/internal/service/curve.go` + `curve_test.go`
- Modify: `services/content-verify/internal/service/worker.go`, `worker_test.go`
- Modify: `services/content-verify/internal/queue/engine.go` (PendingCount)
- Modify: `services/content-verify/internal/config/config.go` (CV_CURVE, CV_DEMAND_PER_WORKER)
- Modify: `services/content-verify/internal/cvmetrics/metrics.go` (WorkerCap gauge)
- Modify: `services/content-verify/cmd/content-verify-api/main.go` (wiring)

**Interfaces:**
- Consumes: `(*cache.DegradationWatcher).Score() float64` (Task 4).
- Produces: `type Curve []CurvePoint` with `ParseCurve(s string, def Curve) Curve` and `(Curve) Cap(score float64) int`; `(*queue.Engine) PendingCount() int`; worker interfaces `ScoreSource { Score() float64 }` and `DemandSource { PendingCount() int }`. Task 10 verifies `content_verify_worker_cap{kind=...}` live.

- [ ] **Step 1: Write the failing curve test**

```go
// services/content-verify/internal/service/curve_test.go
package service

import "testing"

func TestCurveCapBands(t *testing.T) {
	c := ParseCurve("0.40:6,0.60:2,0.80:0", nil)
	cases := []struct {
		score float64
		want  int
	}{
		{0.0, 6}, {0.39, 6}, {0.40, 6},
		{0.41, 5},          // floor(6 - 4*(0.01/0.20*... )) = floor(5.8)
		{0.50, 4},          // midpoint of 6->2
		{0.60, 2}, {0.70, 1}, {0.80, 0}, {0.95, 0}, {1.0, 0},
	}
	for _, tc := range cases {
		if got := c.Cap(tc.score); got != tc.want {
			t.Errorf("Cap(%v) = %d; want %d", tc.score, got, tc.want)
		}
	}
}

func TestParseCurveFallsBackOnGarbage(t *testing.T) {
	def := Curve{{0.5, 3}}
	for _, bad := range []string{"", "nonsense", "0.6:2,0.4:6" /* unsorted */, "0.4:-1"} {
		got := ParseCurve(bad, def)
		if len(got) != 1 || got[0].Cap != 3 {
			t.Errorf("ParseCurve(%q) did not fall back to default", bad)
		}
	}
}
```

- [ ] **Step 2: Run — must fail**

Run: `cd services/content-verify && go test ./internal/service/ -run TestCurve -v` → FAIL (undefined).

- [ ] **Step 3: Implement the curve**

```go
// services/content-verify/internal/service/curve.go
// Package-level score→cap curve for graduated degradation (spec 2026-07-21):
// piecewise-linear between breakpoints, floor()-rounded, endpoint-clamped.
package service

import (
	"math"
	"strconv"
	"strings"
)

type CurvePoint struct {
	Score float64
	Cap   int
}

// Curve is an ascending-score list of breakpoints. Below the first point the
// first cap applies; at/after the last point the last cap applies.
type Curve []CurvePoint

// ParseCurve parses "0.40:6,0.60:2,0.80:0". Any malformed piece, a negative
// cap, or non-ascending scores falls back to def (operator env, not user input).
func ParseCurve(s string, def Curve) Curve {
	parts := strings.Split(s, ",")
	out := make(Curve, 0, len(parts))
	prev := -1.0
	for _, p := range parts {
		scoreStr, capStr, ok := strings.Cut(strings.TrimSpace(p), ":")
		if !ok {
			return def
		}
		sc, err1 := strconv.ParseFloat(scoreStr, 64)
		cp, err2 := strconv.Atoi(capStr)
		if err1 != nil || err2 != nil || cp < 0 || sc <= prev {
			return def
		}
		prev = sc
		out = append(out, CurvePoint{Score: sc, Cap: cp})
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// Cap maps a score to the allowed worker count.
func (c Curve) Cap(score float64) int {
	if len(c) == 0 {
		return 0
	}
	if score <= c[0].Score {
		return c[0].Cap
	}
	last := c[len(c)-1]
	if score >= last.Score {
		return last.Cap
	}
	for i := 1; i < len(c); i++ {
		if score <= c[i].Score {
			a, b := c[i-1], c[i]
			frac := (score - a.Score) / (b.Score - a.Score)
			return int(math.Floor(float64(a.Cap) + frac*float64(b.Cap-a.Cap)))
		}
	}
	return last.Cap
}
```

- [ ] **Step 4: Run — curve tests pass**

Run: `cd services/content-verify && go test ./internal/service/ -run 'TestCurve|TestParseCurve' -v` → PASS.

- [ ] **Step 5: Engine PendingCount + config + metric**

`queue/engine.go`: add to the struct `pendingCount atomic.Int64` (import `sync/atomic`); in `bandedCandidates`, right next to the existing `cvmetrics.QueueDepth.Set(float64(len(all)))` line, add `e.pendingCount.Store(int64(len(all)))`; add:

```go
// PendingCount returns the candidate count from the last queue build — the
// worker's demand signal. 0 before the first build (the demand cap floors at
// one loop, so a cold start still claims and triggers the first build).
func (e *Engine) PendingCount() int { return int(e.pendingCount.Load()) }
```

`config/config.go`: add fields + loads:

```go
	Curve           string // CV_CURVE breakpoints "score:cap,..."
	DemandPerWorker int    // pending units that justify one active loop
	...
	Curve:           getEnv("CV_CURVE", "0.40:6,0.60:2,0.80:0"),
	DemandPerWorker: getEnvInt("CV_DEMAND_PER_WORKER", 5),
```

`cvmetrics/metrics.go`: add:

```go
	WorkerCap = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "content_verify_worker_cap",
		Help: "Graduated worker cap by kind (pressure = curve(score), demand = ceil(pending/per), effective = min).",
	}, []string{"kind"}) // kind: pressure|demand|effective
```

- [ ] **Step 6: Write failing worker tests for the graduated tick**

In `worker_test.go`, replace `fakeShed` and add fakes/tests (the existing `Claimer` fakes gain a `PendingCount() int`):

```go
type fakeScore struct{ v float64 }

func (f fakeScore) Score() float64 { return f.v }

type fakeDemand struct{ pending int }

func (f fakeDemand) PendingCount() int { return f.pending }

// score 0.50 with the default curve => pressure cap 4: loops 0..3 claim,
// loops 4 and 5 sit out with the "degraded" skip reason.
func TestTickPressureCapSitsOutHigherLoops(t *testing.T) {
	claimer := &recordingClaimer{} // reuse/extend the file's existing claimer fake
	w := NewWorker(time.Minute, 6, 10*time.Second, fakeScore{v: 0.50}, claimer, prober, store,
		nil, nil, 0, nil, ParseCurve("0.40:6,0.60:2,0.80:0", nil), 5, fakeDemand{pending: 100})
	w.tick(context.Background(), 3) // i=3 < cap 4 -> claims
	if claimer.calls != 1 {
		t.Fatalf("loop 3 should claim under cap 4; calls=%d", claimer.calls)
	}
	w.tick(context.Background(), 4) // i=4 >= cap 4 -> sits out
	if claimer.calls != 1 {
		t.Fatalf("loop 4 must sit out at score 0.50; calls=%d", claimer.calls)
	}
}

// Shallow queue: pending=3, per=5 => demand cap max(1, ceil(3/5)) = 1: only loop 0 runs.
func TestTickDemandCapShallowQueue(t *testing.T) { /* same shape: i=0 claims, i=1 sits out, score 0 */ }

// pending=0 must still allow loop 0 (bootstrap: the claim builds the queue).
func TestTickDemandCapFloorsAtOne(t *testing.T) { /* i=0 claims with pending=0 */ }

// score 1.0 => cap 0: even loop 0 sits out.
func TestTickFullPressureStopsAllLoops(t *testing.T) { /* i=0 sits out at score 1.0 */ }
```

Adjust every existing `NewWorker(...)` call in the file to the new signature with `fakeScore{v: 0}`, `ParseCurve("0.40:6,0.60:2,0.80:0", nil)`, `5`, `fakeDemand{pending: 100}` — semantics of those tests are unchanged at score 0.

- [ ] **Step 7: Run — fail; then implement the worker change**

Run: `cd services/content-verify && go test ./internal/service/ -v` → FAIL (signatures).

`worker.go` changes:

```go
// ScoreSource is satisfied by *cache.DegradationWatcher.
type ScoreSource interface {
	Score() float64
}

// DemandSource reports the pending probe backlog (satisfied by *queue.Engine).
type DemandSource interface {
	PendingCount() int
}
```

Struct: replace `shed ShedChecker` with `score ScoreSource`, add `curve Curve`, `demandPer int`, `demand DemandSource`. `NewWorker` signature (keep param order stable with the tests above):

```go
func NewWorker(interval time.Duration, workers int, budget time.Duration, score ScoreSource, claimer Claimer, prober UnitProber, store VerdictStore,
	skipProber SkipUnitProber, skipStore SkipStore, skipBudget time.Duration, log *logger.Logger,
	curve Curve, demandPer int, demand DemandSource) *Worker {
```

(`workers` floor stays; `demandPer < 1` corrected to 1.)

`runLoop`: drop `shedMin`; pass `i` to tick: `w.tick(ctx, i)`.

`tick` head (replacing the ShouldShed gate; the rest of tick is UNCHANGED):

```go
// tick runs one claim+probe cycle for loop i. The loop participates only
// while i < min(curve(score), demandCap) — pressure removes loops from the
// top one at a time; a shallow queue keeps most loops parked (demand cap
// floors at 1 so a cold start can still build the first queue snapshot).
func (w *Worker) tick(ctx context.Context, i int) {
	pressureCap := w.curve.Cap(w.scoreValue())
	demandCap := 1
	if w.demand != nil {
		if pc := w.demand.PendingCount(); pc > 0 {
			demandCap = (pc + w.demandPer - 1) / w.demandPer
		}
	} else {
		demandCap = w.workers
	}
	effective := pressureCap
	if demandCap < effective {
		effective = demandCap
	}
	if i == 0 { // one writer for the gauges — loop 0 ticks most often
		cvmetrics.WorkerCap.WithLabelValues("pressure").Set(float64(pressureCap))
		cvmetrics.WorkerCap.WithLabelValues("demand").Set(float64(demandCap))
		cvmetrics.WorkerCap.WithLabelValues("effective").Set(float64(effective))
	}
	if i >= effective {
		cvmetrics.TicksSkippedTotal.WithLabelValues("degraded").Inc()
		return
	}
	...existing claim/probe body unchanged...
}

func (w *Worker) scoreValue() float64 {
	if w.score == nil {
		return 0
	}
	return w.score.Score()
}
```

`cmd/content-verify-api/main.go`: build the curve once and pass the engine as demand:

```go
		curve := service.ParseCurve(cfg.Curve, service.Curve{{0.40, 6}, {0.60, 2}, {0.80, 0}})
		worker := service.NewWorker(cfg.Interval, cfg.Workers, cfg.UnitBudget, shedWatcher, engine, pb, store, skipPb, store, cfg.SkipBudget, log,
			curve, cfg.DemandPerWorker, engine)
```

(`shedWatcher` already satisfies `ScoreSource` after Task 4.)

- [ ] **Step 8: Run full suite + build**

Run: `cd services/content-verify && go test ./... && go build ./...`
Expected: PASS, clean build (including `queue` package tests).

- [ ] **Step 9: Commit**

```bash
git add services/content-verify/
git commit -m "feat(content-verify): graduated score+demand worker curve" -- services/content-verify
```

---

### Task 6: Stealth — score consumption, pool target, warming gate

**Files:**
- Create: `services/stealth-scraper/app/scaling.py` + `services/stealth-scraper/tests/test_scaling.py`
- Modify: `services/stealth-scraper/app/engine.py` (`_degradation_loop`, `_warming_allowed`, new `_pool_target`), `services/stealth-scraper/app/metrics.py`
- Modify: `services/stealth-scraper/tests/test_engine_degradation.py`

**Interfaces:**
- Consumes: governor status JSON now containing `data.score` (Task 3); `cfg.pool_curve` (Task 1).
- Produces: `scaling.parse_curve(s: str) -> list[tuple[float, int]]`, `scaling.pool_target_for(score: float, curve, max_pool: int) -> int` (clamped `[1, max_pool]`); engine attrs `_degradation_score: float`, `_pool_target() -> int`; gauges `stealth_pool_target`, `stealth_pool_over_target`. Tasks 7/8 consume all of these.

- [ ] **Step 1: Write the failing scaling test**

```python
# services/stealth-scraper/tests/test_scaling.py
import unittest

from app import scaling


class PoolTargetTest(unittest.TestCase):
    CURVE = scaling.parse_curve("0.40:6,0.60:2,0.80:1")

    def test_bands(self):
        cases = [
            (0.0, 6), (0.40, 6), (0.41, 5), (0.50, 4),
            (0.60, 2), (0.70, 1), (0.80, 1), (1.0, 1),
        ]
        for score, want in cases:
            self.assertEqual(
                scaling.pool_target_for(score, self.CURVE, 6), want, f"score={score}"
            )

    def test_floor_is_one_even_for_zero_cap_curve(self):
        curve = scaling.parse_curve("0.40:6,0.80:0")
        self.assertEqual(scaling.pool_target_for(1.0, curve, 6), 1)

    def test_max_pool_clamps(self):
        self.assertEqual(scaling.pool_target_for(0.0, self.CURVE, 3), 3)

    def test_garbage_curve_falls_back(self):
        for bad in ("", "junk", "0.6:2,0.4:6", "0.4:-1"):
            curve = scaling.parse_curve(bad)
            self.assertEqual(scaling.pool_target_for(0.0, curve, 6), 6)
            self.assertEqual(scaling.pool_target_for(1.0, curve, 6), 1)
```

- [ ] **Step 2: Run — must fail (no module)**

Run: `cd services/stealth-scraper && python3 -m unittest tests.test_scaling -v`
Expected: `ModuleNotFoundError: No module named 'app.scaling'`

- [ ] **Step 3: Implement scaling.py**

```python
# services/stealth-scraper/app/scaling.py
"""Score -> pool-target curve for graduated degradation (spec 2026-07-21).

Pure functions: piecewise-linear between "score:cap" breakpoints,
floor()-rounded, clamped to [1, max_pool] — Camoufox always keeps one
instance (user-stream floor); the sustained-L2 DegradedShed backstop lives
in engine._shed_new_work, not here.
"""
from __future__ import annotations

import math

DEFAULT_CURVE = [(0.40, 6), (0.60, 2), (0.80, 1)]


def parse_curve(s: str) -> list[tuple[float, int]]:
    """Parse "0.40:6,0.60:2,0.80:1". Malformed input, negative caps, or
    non-ascending scores fall back to DEFAULT_CURVE (operator env)."""
    out: list[tuple[float, int]] = []
    prev = -1.0
    for part in (s or "").split(","):
        part = part.strip()
        if not part:
            return list(DEFAULT_CURVE)
        score_s, _, cap_s = part.partition(":")
        try:
            score, cap = float(score_s), int(cap_s)
        except ValueError:
            return list(DEFAULT_CURVE)
        if cap < 0 or score <= prev:
            return list(DEFAULT_CURVE)
        prev = score
        out.append((score, cap))
    return out or list(DEFAULT_CURVE)


def pool_target_for(score: float, curve: list[tuple[float, int]], max_pool: int) -> int:
    """Map a pressure score to the warm-browser target, clamped [1, max_pool]."""
    if not curve:
        return max(1, max_pool)
    if score <= curve[0][0]:
        raw = curve[0][1]
    elif score >= curve[-1][0]:
        raw = curve[-1][1]
    else:
        raw = curve[-1][1]
        for (s0, c0), (s1, c1) in zip(curve, curve[1:]):
            if score <= s1:
                frac = (score - s0) / (s1 - s0)
                raw = math.floor(c0 + frac * (c1 - c0))
                break
    return max(1, min(int(raw), max_pool))
```

- [ ] **Step 4: Run — passes**

Run: `cd services/stealth-scraper && python3 -m unittest tests.test_scaling -v` → OK.

- [ ] **Step 5: Wire score + target into the engine**

`metrics.py` — add next to `DEGRADATION_LEVEL_SEEN`:

```python
POOL_TARGET = Gauge("stealth_pool_target", "Graduated warm-browser target from the pressure score curve")
POOL_OVER_TARGET = Gauge(
    "stealth_pool_over_target",
    "Sessions above pool_target that survive on user-stream sanctity (0 = converged)",
)
```

`engine.py` `__init__` — next to `self._degradation_level`:

```python
        self._degradation_score: float = 0.0
        self._pool_curve = scaling.parse_curve(cfg.pool_curve)
```

(import `from . import scaling` alongside the existing relative imports).

`_degradation_loop` — extend `_fetch` to return both, and store both:

```python
        def _fetch() -> tuple[int, float]:
            with _rq.urlopen(url, timeout=3) as resp:  # noqa: S310 (fixed internal URL)
                body = _json.loads(resp.read(65536))
            data = body.get("data", {})
            level = int(data.get("level", 0))
            try:
                score = float(data.get("score", 0.0))
            except (TypeError, ValueError):
                score = 0.0
            return (
                level if 0 <= level <= 2 else 0,
                min(1.0, max(0.0, score)),
            )
```

In the loop body: `level, score = await loop.run_in_executor(None, _fetch)`; set `self._degradation_score = score` next to the level store; in the `except` fail-open branch also `self._degradation_score = 0.0`. After the metrics lines add `metrics.POOL_TARGET.set(self._pool_target())`.

Add the target helper next to `_warming_allowed`:

```python
    def _pool_target(self) -> int:
        """Warm-browser target from the graduated score curve (spec 2026-07-21)."""
        return scaling.pool_target_for(
            self._degradation_score, self._pool_curve, self.cfg.pool_size
        )
```

Replace `_warming_allowed`:

```python
    def _warming_allowed(self) -> bool:
        """Warming admits a new profile only while the warm count is under the
        graduated pool target (which shrinks with the pressure score — this
        replaces the old binary stop-at-L1) and combined RSS is under the
        soft budget (existing back-pressure, untouched)."""
        if len(self._sessions) >= self._pool_target():
            return False
        return self._read_ram() < self.cfg.ram_soft_bytes
```

- [ ] **Step 6: Extend the degradation engine test, run everything**

In `tests/test_engine_degradation.py`, following the file's existing fake-governor pattern, add: (a) a poll returning `{"data": {"level": 1, "score": 0.55}}` sets `engine._degradation_score == 0.55` and `engine._pool_target() == 3` (curve 6→2 over 0.40–0.60: floor(6-4*0.75)=3); (b) a poll error zeroes the score; (c) `_warming_allowed()` returns False when `len(_sessions) >= _pool_target()` (populate `engine._sessions` with that many dummy objects the way the file's other tests do).

Run: `cd services/stealth-scraper && python3 -m unittest discover -s tests -v`
Expected: OK (all — including untouched suites).

- [ ] **Step 7: Commit**

```bash
git add services/stealth-scraper/app/scaling.py services/stealth-scraper/app/engine.py services/stealth-scraper/app/metrics.py services/stealth-scraper/tests/
git commit -m "feat(stealth): pressure-score pool target + curve warming gate" -- services/stealth-scraper
```

---

### Task 7: Stealth — classification, drain, stream migration

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (Session fields, classification, victim pick, `_migrate_session`, `_scale_down_step`, owns_profile in close paths)
- Modify: `services/stealth-scraper/app/metrics.py`
- Create: `services/stealth-scraper/tests/test_engine_scaledown.py`

**Interfaces:**
- Consumes: `_pool_target()` (Task 6), `Session.in_use` / `last_persist` / `user_key` (existing).
- Produces: `Session.owns_profile: bool = True`, `Session.draining: bool = False`; `_session_is_user(s) -> bool`, `_pick_victim() -> tuple[str, Session] | None`, `_migrate_session(sid) -> bool`, `_scale_down_step()` (async, called from `_degradation_loop`); counter `stealth_stream_migrations_total{result}`. Task 8 consumes `_pick_victim` ordering and the draining flag.

**Design notes for the implementer (from the spec, owner-approved):**
- user-class session = truthy `user_key` AND a live stream: `in_use > 0` or `time.time() - last_persist < 180` (segments refresh `last_persist` ~1/min; 3× margin). An idle user session is service-class (killable). Probe traffic reaches the sidecar without `user_key` today, so no probe-identity special-case is needed in the engine — note this in a comment.
- Kill threshold: act only when `len(self._sessions) > math.ceil(target / 2) + 1` — relative to the CURRENT target.
- One drain step per degradation-loop tick (~5s cadence) — deliberate gradualism, no burst kills.
- Migration = _rehydrate's verify flow on a SURVIVOR's context: new page → `goto(player_url)` → in-page master fetch 200 → swap `self._sessions[sid]` → close the old page once its `in_use` drains. The new session sets `owns_profile=False` (the survivor's profile lease is NOT doubled); every close path releases the profile ONLY when `owns_profile` is true.

- [ ] **Step 1: Session fields + owns_profile in every close path**

`Session` dataclass gains (after `user_key`):

```python
    # Graduated scale-down (spec 2026-07-21): a migrated session rides its
    # survivor's browser — it does NOT own the profile lease, so close paths
    # must not release it. `draining` blocks new-work admission on a victim.
    owns_profile: bool = True
    draining: bool = False
```

Audit and patch the three close paths — `_evict_one_lru`, `aclose_session`, and the reaper's expiry close in `_reap` — so `self.profiles.release(session.profile, ok=True)` runs only under `if session.owns_profile:` (page close is unconditional). `_evict_one_lru` candidates additionally exclude draining sessions (`if s.in_use <= 0 and not s.draining`).

- [ ] **Step 2: Write failing scale-down tests (classification + victim order + drain trigger)**

```python
# services/stealth-scraper/tests/test_engine_scaledown.py
import time
import unittest

# Mirror the engine-construction/fixture helpers used by
# tests/test_engine_capacity.py (same fake profile/session builders).


class ClassificationTest(unittest.TestCase):
    def test_user_session_requires_key_and_live_stream(self):
        # (user_key, in_use, age_s) -> expected is_user
        cases = [
            ("u1", 1, 999, True),   # in-flight fetch
            ("u1", 0, 60, True),    # recent segment activity
            ("u1", 0, 600, False),  # idle user session = service-class
            (None, 5, 10, False),   # probe/warm: no key, never user
        ]
        ...build session, assert engine._session_is_user(s) == expected...


class VictimOrderTest(unittest.TestCase):
    def test_service_class_lru_first_then_user(self):
        # sessions: user-live (expires soonest), service (expires later)
        # -> victim must be the SERVICE one despite worse LRU position.
        ...


class DrainTriggerTest(unittest.TestCase):
    def test_no_action_at_or_under_threshold(self):
        # target=4 -> threshold ceil(4/2)+1 = 3; current=3 -> no victim picked.
        ...

    def test_over_threshold_picks_victim_and_sets_draining(self):
        # target=2 -> threshold 2; current=3 (all service) -> one victim drains.
        ...
```

Flesh the `...` out with the SAME fixture style as `test_engine_capacity.py` (real `Session` objects on a stub engine — no browser). Run: `python3 -m unittest tests.test_engine_scaledown -v` → FAIL (missing methods).

- [ ] **Step 3: Implement classification + victim selection**

```python
    def _session_is_user(self, s: Session) -> bool:
        """User-class = held by a real user AND actively streaming: an in-flight
        /hls fetch, or segment activity within 3 minutes (proxy_fetch refreshes
        last_persist ~1/min). Idle user sessions are service-class (killable).
        Probe/warm traffic carries no user_key, so it can never classify user."""
        if not s.user_key:
            return False
        return s.in_use > 0 or (time.time() - s.last_persist) < 180.0

    def _pick_victim(self) -> tuple[str, Session] | None:
        """Scale-down victim: service-class browsers first (LRU by expires_at),
        else a user-class browser — which may only ever be DRAINED (migrated),
        never force-killed. Draining sessions are already being handled."""
        service, user = [], []
        for sid, s in self._sessions.items():
            if s.draining:
                continue
            (user if self._session_is_user(s) else service).append((s.expires_at, sid, s))
        for bucket in (service, user):
            if bucket:
                bucket.sort(key=lambda t: t[0])
                _, sid, s = bucket[0]
                return sid, s
        return None
```

- [ ] **Step 4: Implement migration + the per-tick step**

`metrics.py` add:

```python
STREAM_MIGRATIONS = Counter(
    "stealth_stream_migrations_total",
    "User-stream migrations onto a surviving browser during scale-down",
    ["result"],  # ok|failed
)
```

`engine.py`:

```python
    async def _migrate_session(self, sid: str) -> bool:
        """Move a live user stream onto a surviving browser (graceful drain,
        spec 2026-07-21). A stream cannot be handed over literally — /hls
        fetches ride the session page's cookie/TLS-fingerprint context — so
        "migrate" = open the victim's player_url on the survivor's context,
        verify the master playlist fetches 200 there, then atomically swap
        self._sessions[sid]. The player never sees a URL change. On ANY
        failure the victim survives untouched (user-stream sanctity)."""
        victim = self._sessions.get(sid)
        if victim is None or not victim.player_url:
            return False
        survivors = [
            s for other_sid, s in self._sessions.items()
            if other_sid != sid and not s.draining and s.owns_profile
        ]
        if not survivors:
            metrics.STREAM_MIGRATIONS.labels(result="failed").inc()
            return False
        # Consolidation preference: survivors already serving users first,
        # freshest (largest expires_at) within each class.
        survivors.sort(key=lambda s: (not self._session_is_user(s), -s.expires_at))
        survivor = survivors[0]
        page = None
        try:
            context = await self._ensure_browser(survivor.profile, survivor.proxy_id)
            page = await context.new_page()
            await page.goto(
                victim.player_url,
                wait_until="domcontentloaded",
                timeout=self.cfg.nav_timeout_ms,
            )
            moved = Session(
                id=sid,
                profile=survivor.profile,
                proxy_id=survivor.proxy_id,
                referer=victim.referer,
                user_agent=survivor.user_agent,
                cdn_host=victim.cdn_host,
                master_url=victim.master_url,
                expires_at=time.time() + self.cfg.session_ttl_seconds,
                page=page,
                player_url=victim.player_url,
                user_key=victim.user_key,
                owns_profile=False,
            )
            status, _ctype, _final, _hdrs, _body = await self._in_page_fetch(
                moved, victim.master_url
            )
            if status != 200:
                raise RecipeError(f"migration verify: master fetch {status}")
        except Exception:  # noqa: BLE001 — any failure: victim survives
            if page is not None:
                await _safe_close_page(page)
            metrics.STREAM_MIGRATIONS.labels(result="failed").inc()
            return False
        self._sessions[sid] = moved
        moved.last_persist = time.time()
        self.store.save(self._session_record(moved))
        self._spawn(self._retire_after_drain(victim))
        metrics.STREAM_MIGRATIONS.labels(result="ok").inc()
        if self._log:
            self._log.info("stream migrated", extra={"sid": sid[:8]})
        return True

    async def _retire_after_drain(self, old: Session) -> None:
        """Close a migrated-away session object once its in-flight fetches
        drain (bounded wait — a wedged fetch must not pin the browser open
        forever), then release its profile (it owned one)."""
        deadline = time.time() + 60.0
        while old.in_use > 0 and time.time() < deadline:
            await asyncio.sleep(1.0)
        await _safe_close_page(old.page)
        if old.owns_profile:
            self.profiles.release(old.profile, ok=True)
```

And the per-tick driver + hookup at the end of `_degradation_loop`'s try block (after the metrics sets):

```python
                await self._scale_down_step()
```

```python
    async def _scale_down_step(self) -> None:
        """One graduated scale-down step per degradation tick (~5s): act only
        above the kill threshold ceil(target/2)+1 (relative to the CURRENT
        target), one victim at a time — gradualism over burst kills."""
        target = self._pool_target()
        current = len(self._sessions)
        metrics.POOL_OVER_TARGET.set(max(0, current - target))
        if current <= math.ceil(target / 2) + 1:
            return
        picked = self._pick_victim()
        if picked is None:
            return
        sid, victim = picked
        victim.draining = True
        if self._session_is_user(victim):
            if not await self._migrate_session(sid):
                # Migration impossible: the user-streaming browser SURVIVES
                # above target (spec: user-stream sanctity). Clear draining so
                # the stream keeps its session; we retry on a later tick.
                victim.draining = False
            return
        await self._force_kill(sid, mode="graceful" if victim.in_use <= 0 else "forced")
```

(`_force_kill` arrives in Task 8; for THIS task's commit, stub it as closing the session exactly like `_evict_one_lru` does for the `in_use <= 0` case — pop, store.delete, close page, conditional release — with mode ignored. Task 8 replaces the stub. `import math` at the top of engine.py.)

- [ ] **Step 5: Run the new suite + full stealth tests**

Run: `cd services/stealth-scraper && python3 -m unittest discover -s tests -v`
Expected: OK. Add one more test in `test_engine_scaledown.py` before moving on: migration-failure leaves the victim in `self._sessions` with `draining == False` (stub `_migrate_session` dependencies per the file's fixture style, or monkeypatch `_ensure_browser` to raise).

- [ ] **Step 6: Commit**

```bash
git add services/stealth-scraper/
git commit -m "feat(stealth): graduated scale-down with user-stream migration" -- services/stealth-scraper
```

---

### Task 8: Stealth — forced kill (service-class only) + honest 503 + RAM emergency

**Files:**
- Modify: `services/stealth-scraper/app/engine.py` (`_force_kill`, degraded-kill marker in `proxy_fetch`, `_admit_launch` emergency path)
- Modify: `services/stealth-scraper/app/main.py` (`/hls` handler catches DegradedShed → 503)
- Modify: `services/stealth-scraper/app/metrics.py`
- Modify: `services/stealth-scraper/tests/test_engine_scaledown.py`, `services/stealth-scraper/tests/test_engine_capacity.py`

**Interfaces:**
- Consumes: `_pick_victim`, `_session_is_user`, `draining` (Task 7).
- Produces: `_force_kill(sid: str, mode: str)`; counter `stealth_pool_kills_total{class="service", mode}`; `proxy_fetch` raises `DegradedShed` for recently-killed sids; `/hls` returns 503 `kind="degraded"` for them.

- [ ] **Step 1: Write failing tests**

Add to `test_engine_scaledown.py` (same fixtures):

```python
class ForceKillTest(unittest.TestCase):
    def test_user_streaming_browser_is_never_force_killed(self):
        # sessions: ONLY user-live ones, current over threshold
        # -> _scale_down_step attempts migration; with no survivor it fails;
        # session count unchanged, no kill metric increment.
        ...

    def test_service_kill_marks_sid_degraded_for_hls(self):
        # force-kill a service session with in_use=1 (mode=forced), then
        # proxy_fetch(sid, url) must raise DegradedShed (not SessionGone).
        ...

    def test_ram_emergency_kills_service_class_only(self):
        # _read_ram() stubbed >= hard budget; sessions: one user-live, one
        # service in_use=1. _admit_launch must kill the SERVICE one (even
        # in-use) and leave the user session untouched.
        ...
```

Run: `python3 -m unittest tests.test_engine_scaledown -v` → FAIL.

- [ ] **Step 2: Implement**

`metrics.py`:

```python
POOL_KILLS = Counter(
    "stealth_pool_kills_total",
    "Scale-down browser kills. class is hard-wired 'service' — any other value appearing is a bug.",
    ["class", "mode"],  # class: service; mode: graceful|forced
)
# NOTE: "class" is a Python keyword, so increments MUST use positional labels:
# metrics.POOL_KILLS.labels("service", mode).inc() — labels(class=...) is a SyntaxError.
```

`engine.py` — `__init__` gains `self._degraded_kills: dict[str, float] = {}` (sid → marker expiry). Replace the Task-7 stub:

```python
    def _force_kill(self, sid: str, mode: str) -> None:
        """Kill a SERVICE-class browser outright (graduated Stage 2). Its
        in-flight and follow-up /hls fetches see DegradedShed -> 503
        kind=degraded ("high load") for 120s — an honest shed signal the
        scraper breaker parks on, not a mystery 410. User-class browsers must
        never reach this method (guarded at every call site; classification
        is re-checked here as a last line)."""
        s = self._sessions.get(sid)
        if s is None:
            return
        if self._session_is_user(s):  # defense in depth — never kill user streams
            return
        self._sessions.pop(sid, None)
        self._degraded_kills[sid] = time.time() + 120.0
        self.store.delete(sid)
        self._spawn(_safe_close_page(s.page))
        if s.owns_profile:
            self.profiles.release(s.profile, ok=True)
        metrics.POOL_KILLS.labels("service", mode).inc()  # positional: "class" is a keyword
        metrics.ACTIVE_SESSIONS.set(len(self._sessions))
        if self._log:
            self._log.info("service session force-killed (%s)", mode, extra={"sid": sid[:8]})
```

Convert `_scale_down_step`'s call to `self._force_kill(sid, mode=...)` (drop the async stub — `_force_kill` is sync).

`proxy_fetch` — at the very top, before the session lookup:

```python
        until = self._degraded_kills.get(sid)
        if until is not None:
            if time.time() < until:
                raise DegradedShed("stream shed: high load (browser scaled down)")
            self._degraded_kills.pop(sid, None)
```

`_admit_launch` — in the `ram >= hard` branch, try an emergency service-class kill BEFORE the LRU-evict fallback:

```python
        if ram >= self.cfg.ram_hard_bytes:
            if self._emergency_kill_service():
                metrics.ADMISSION_TOTAL.labels(action="hard_kill").inc()
            else:
                metrics.ADMISSION_TOTAL.labels(
                    action="hard_evict" if self._evict_one_lru() else "hard_refuse"
                ).inc()
            raise CapacityExceeded(
                f"combined RSS {ram} >= hard budget {self.cfg.ram_hard_bytes}"
            )
```

```python
    def _emergency_kill_service(self) -> bool:
        """RAM hard-budget emergency: force-kill ONE service-class session,
        in-use or not (memory emergencies don't wait for drains). User-class
        sessions are exempt even here — worst case the pool rides above
        budget on user streams alone until they end (spec, owner decision 7)."""
        for sid, s in sorted(self._sessions.items(), key=lambda kv: kv[1].expires_at):
            if not self._session_is_user(s):
                self._force_kill(sid, mode="forced")
                return True
        return False
```

`main.py` — in the `/hls` route handler, add ABOVE the existing generic error handling (imports already include `DegradedShed`):

```python
    except DegradedShed as exc:
        # Graduated scale-down Stage 2: this stream's browser was shed under
        # host pressure. 503 kind=degraded = honest "high load" to the
        # consumer (breaker parks; player self-heal/badge surfaces it).
        return JSONResponse(
            {"success": False, "error": str(exc), "kind": "degraded"}, status_code=503
        )
```

(Match the handler's actual response style — if `/hls` streams raw bytes rather than JSON on success, mirror how it renders its existing `SessionGone`→410 error and use that shape with status 503 and `kind: degraded`.)

- [ ] **Step 3: Run everything**

Run: `cd services/stealth-scraper && python3 -m unittest discover -s tests -v`
Expected: OK — including `test_engine_capacity.py` (its `_admit_launch` hard-branch tests may need the new `hard_kill` action accounted for: with no sessions present `_emergency_kill_service()` returns False and the old behavior is preserved, so most should pass unchanged).

- [ ] **Step 4: Commit**

```bash
git add services/stealth-scraper/
git commit -m "feat(stealth): service-class force-kill + degraded 503 + RAM emergency path" -- services/stealth-scraper
```

---

### Task 9: Dashboard + docs

**Files:**
- Modify: `docker/grafana/provisioning/dashboards/degradation-overview.json`
- Modify: `docs/environment-variables.md`

- [ ] **Step 1: Add score + cap panels**

In `degradation-overview.json` (copy an existing timeseries panel object as the structural template — keep datasource refs/gridPos conventions of the file, place in the state row after the level timeline):

1. **"Pressure score (governor vs preview)"** — timeseries, two targets:
   - `ae_degradation_score{job="governor"}` (legend `smoothed (authoritative)`)
   - `ae:pressure_score:preview` (legend `raw preview`)
   - min 0 / max 1; thresholds 0.40 (yellow) and 0.80 (red) as area lines.
2. **"Per-signal score"** — timeseries, target `ae:pressure_score:signal` legend `{{signal}}`, min 0 / max 1.

In the heavy-actors row:

3. **"content-verify worker cap"** — timeseries, targets `content_verify_worker_cap{kind="pressure"}`, `{kind="demand"}`, `{kind="effective"}` (legend `{{kind}}`), plus `content_verify_inflight_leases` (legend `active`).
4. **"Camoufox pool"** — timeseries, targets `stealth_pool_target` (legend `target`), `stealth_active_sessions` (legend `sessions` — verify the exact exported name with `curl -s localhost:3000/metrics | grep stealth_` once deployed; it is `metrics.ACTIVE_SESSIONS`'s name in `app/metrics.py`), `stealth_pool_over_target` (legend `over target`), `stealth_pool_kills_total` as `increase(stealth_pool_kills_total[10m])` (legend `kills {{mode}}`).

Validate JSON: `python3 -m json.tool docker/grafana/provisioning/dashboards/degradation-overview.json > /dev/null && echo OK` → `OK`.

Also locate the content-verify service dashboard (`grep -rl "content_verify" docker/grafana/ infra/grafana/ 2>/dev/null` — it exists per the 2026-07 content-verify dashboard work) and add the same "worker cap" panel (target set of item 3) to it; if the grep finds nothing under provisioned paths, note that in the commit message and skip — degradation-overview is the primary surface.

- [ ] **Step 2: Document the new envs**

Add rows to `docs/environment-variables.md` in the governor / content-verify / stealth sections:

| Var | Default | Meaning |
|---|---|---|
| `GOVERNOR_SCORE_ALPHA_UP` | `0.5` | Score EWMA rise factor per 15s tick (fast ramp-track) |
| `GOVERNOR_SCORE_ALPHA_DOWN` | `0.05` | Score EWMA decay factor per tick (~5min recovery) |
| `CV_CURVE` | `0.40:6,0.60:2,0.80:0` | score:cap breakpoints for probe-loop cap |
| `CV_DEMAND_PER_WORKER` | `5` | pending units justifying one active probe loop (demand cap = ceil(pending/this), floor 1) |
| `CV_WORKERS` | `6` (compose) | spawned probe loops; curve+demand decide how many claim |
| `STEALTH_POOL_CURVE` | `0.40:6,0.60:2,0.80:1` | score:cap breakpoints for warm-browser target (floor 1) |
| `STEALTH_RAM_SOFT_BYTES` / `STEALTH_RAM_HARD_BYTES` | 4 GiB / 6 GiB | raised Phase-0 budgets (2026-07-21) |

- [ ] **Step 3: Commit**

```bash
git add docker/grafana/provisioning/dashboards/degradation-overview.json docs/environment-variables.md
git commit -m "feat(observability): pressure-score + graduated-cap dashboard panels, env docs" -- docker/grafana/provisioning/dashboards/degradation-overview.json docs/environment-variables.md
```

---

### Task 10: Deploy + live E2E drill

**Files:** none (operational task).

- [ ] **Step 1: Land + deploy**

```bash
git pull --rebase origin main && git push origin HEAD:main
# base tree ff-syncs within 10min, or trigger: sudo /usr/local/bin/animeenigma-git-autosync.sh
curl -s -X POST http://localhost:9090/prometheus/-/reload   # dir-mounted rules, NO recreate
cd /data/animeenigma && make redeploy-governor && make redeploy-content-verify && make redeploy-stealth-scraper
make restart-grafana   # provisioning-tpl entrypoint copy (dashboards)
make health
```

NOTE: the worktree needs `docker/.env` linked/copied before any `make redeploy-*` from inside it — deploy from the base tree AFTER push+ff-sync instead (as written above). content-verify's `mem_limit` change requires the recreate that `redeploy` already does; the stealth env-default changes are baked into the image/compose — recreate applies them.

- [ ] **Step 2: Verify score pipeline live**

```bash
curl -s 'http://localhost:9090/prometheus/api/v1/query?query=ae:pressure_score:preview' | python3 -m json.tool | grep -A2 value
docker exec animeenigma-redis redis-cli GET ae:degradation:score      # e.g. "0.00"
curl -s http://localhost:8100/api/degradation/status | python3 -m json.tool | grep -E 'score|level'
curl -s http://localhost:8101/metrics | grep content_verify_worker_cap
curl -s http://localhost:3000/metrics | grep -E 'stealth_pool_target|stealth_pool_over_target'
```

Expected: preview present; Redis score `"0.00"`-ish at idle; status JSON has `"score"`; cv caps `pressure=6`, `effective` ≤ 6; `stealth_pool_target 6`.

- [ ] **Step 3: Override drill**

```bash
bin/degradation-override.sh set 1
sleep 20
docker exec animeenigma-redis redis-cli GET ae:degradation:score   # "0.50"
curl -s http://localhost:8101/metrics | grep 'worker_cap{kind="pressure"}'   # 4
curl -s http://localhost:3000/metrics | grep stealth_pool_target             # 4
bin/degradation-override.sh set 2
sleep 20
docker exec animeenigma-redis redis-cli GET ae:degradation:score   # "1.00"
curl -s http://localhost:8101/metrics | grep 'worker_cap{kind="pressure"}'   # 0
curl -s http://localhost:3000/metrics | grep stealth_pool_target             # 1
bin/degradation-override.sh clear
sleep 20
docker exec animeenigma-redis redis-cli GET ae:degradation:score   # back to computed (~0.00)
```

Also confirm during `set 2`: a scraper resolve via a browser provider returns 503 `kind="degraded"` (the unchanged backstop), and after `clear` it recovers.

- [ ] **Step 4: Check the dashboard + finish**

Open `https://animeenigma.org/admin/grafana/d/degradation-overview/` — score timeline, per-signal score, cv cap, and Camoufox pool panels render with data.

Then run `/animeenigma-after-update` (per CLAUDE.md — simplify pass, changelog, commit/push) and set the feedback/memory records per project convention.

---

## Scores

- **UXΔ = +1 (Better)** · **CDI = 0.04 * 21** · **MVQ = Griffin 80%/85%** (per spec).
