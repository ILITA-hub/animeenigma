# Playback-Health Roster Unification — Phase 3: Per-Provider Metric Parity

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the EN scraper chain per-provider parser latency (the literal fix for "Parser p95 shows 4 not 8") and give the first-party `ae` survivor full instrumentation (parser metrics + a real liveness probe), so the playback-health metrics cover the active path and the survivor end to end.

**Architecture:** Three focused, additive instrumentation changes. (1) Scraper: thread an `operation` label through the orchestrator's generic failover loop (`runFailoverNamed`) and emit `metrics.ObserveParser` per provider attempt — one central hook covers all 8 EN+adult providers across the 4 operations. (2) Catalog: `defer metrics.ObserveParser("ae", …)` in the two library-resolution methods. (3) Catalog: extend the existing `PlayerHealthChecker` with an `ae` liveness probe that calls the library client's `Ping`, emitting `provider_health_up{provider="ae",stage="liveness"}`. All changes are `defer`/wrapper-based and never alter resolution behavior.

**Tech Stack:** Go, Prometheus client_golang (+ `prometheus/testutil`), existing `libs/metrics.ObserveParser`, `library.Client.Ping`.

## Global Constraints

- **Effort/impact units:** UXΔ / CDI / MVQ only — never days/hours/sprints (`.planning/CONVENTIONS.md`).
- **Go conventions:** snake_case files, PascalCase exported types, `libs/errors` for domain errors, structured `libs/logger`.
- **Shared dirty tree:** path-scoped commits (`git commit <pathspec>`), never `git add -A`, never `--amend`, `git show --stat HEAD` after each commit. Implementers do NOT `git push` — the controller lands commits onto `origin/main`. Execute from an isolated worktree off `origin/main`.
- **Scope (locked in brainstorming — "survivors + active path only"):** instrument the EN scraper chain (parser latency) + `ae` (parser + health probe). Do NOT add new probes/telemetry for the retiring legacy players (kodik keeps its existing probe; animelib/hanime/raw get nothing new). The bespoke `/api/anime/:id/ae` panel retirement is Phase 4 (dashboard), NOT this plan.
- **ae probe mechanism (locked):** ae liveness = library-service liveness via `library.Client.Ping(ctx)` (HTTP GET, non-2xx → error). Does NOT verify any specific title is encoded (that is inherently per-request: 404 = not encoded yet).
- **Instrumentation must be behavior-preserving:** parser observation is `defer`-based and the failover/skip/error control flow is unchanged. The health-cache SKIP path (provider not called) must NOT emit a parser observation (no upstream request was made).
- **Operation labels (bounded set):** `find_id`, `list_episodes`, `list_servers`, `get_stream` (scraper); `get_episodes`, `get_stream` (ae). Stable identifiers, never raw input.

**Spec:** `docs/superpowers/specs/2026-06-17-playback-health-provider-roster-unification-design.md` (§2/§3 parity). **Phases 1–2 (shipped):** roster table + `scraper_operated` + shared `EmitProviderRoster`; `provider_info`/`provider_enabled` already cover all 13.

**Deferred (NOT in this plan, documented so coverage gaps are not silent):**
- The 18anime egress *telemetry* provider-tag (`domain.WithProvider`) — non-trivial (the `eighteenanime` provider builds its own `*http.Client`, not a tagged `BaseHTTPClient`); low value; 18anime already gets parser-latency parity via Task 1. Follow-up.
- `GetStreamGated` (gogoanime's playability-gated stream path) has its own loop and is NOT instrumented here; gogoanime still appears in parser metrics via `find_id`/`list_episodes`/`list_servers`. Documented known-partial.

---

## Phase 3 File Structure

| File | Responsibility | Change |
|---|---|---|
| `services/scraper/internal/service/orchestrator.go` | Failover loop | Add `operation string` param to `runFailover`+`runFailoverNamed`; `ObserveParser` per attempt; update the 4 call sites |
| `services/scraper/internal/service/orchestrator_parser_metrics_test.go` | Test the per-attempt parser emission | **Create** |
| `services/catalog/internal/service/raw_resolver.go` | ae/raw resolution | `defer metrics.ObserveParser("ae", …)` in `GetLibraryEpisodes`/`GetLibraryStream` |
| `services/catalog/internal/service/health_checker.go` | Player liveness probe | Add `aePinger` field + `checkAe()`; call from `checkAll()` |
| `services/catalog/internal/service/health_checker_test.go` | Test checkAe up/down mapping | **Create** |
| `services/catalog/cmd/catalog-api/main.go` | Boot wiring | Pass the library client to `NewPlayerHealthChecker` (construct it after `libraryClient`) |

---

### Task 1: Scraper EN-chain parser latency (the headline)

**Files:**
- Modify: `services/scraper/internal/service/orchestrator.go` (`runFailover` ~228, `runFailoverNamed` ~281, the 4 call sites ~404/423/431/439)
- Create: `services/scraper/internal/service/orchestrator_parser_metrics_test.go`

**Interfaces:**
- Produces: `runFailover`/`runFailoverNamed` gain a leading-after-providerTimeout `operation string` param; each provider attempt emits `parser_requests_total{provider,operation,status}` + `parser_request_duration_seconds{provider,operation}`.

- [ ] **Step 1: Write the failing test**

Create `services/scraper/internal/service/orchestrator_parser_metrics_test.go`. Reuse the existing fake-provider pattern from `orchestrator_test.go` (find the fake `domain.Provider` it uses; if it's named e.g. `fakeProvider`/`stubProvider`, use that — it must implement `Name()` and the `domain.Provider` interface). This test calls `runFailoverNamed` directly with a one-element provider list and asserts the parser counter incremented:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRunFailoverNamed_EmitsParserMetrics(t *testing.T) {
	// success path → status="success"
	before := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_ok", "find_id", "success"))
	_, name, err := runFailoverNamed(
		context.Background(), nil,
		[]domain.Provider{newTestProvider("p3_ok")}, nil, 0, "find_id",
		func(c context.Context, p domain.Provider) (string, error) { return "X", nil },
	)
	if err != nil || name != "p3_ok" {
		t.Fatalf("runFailoverNamed = (%q, %v), want (p3_ok, nil)", name, err)
	}
	after := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_ok", "find_id", "success"))
	if after != before+1 {
		t.Errorf("parser_requests_total{p3_ok,find_id,success} = %v, want %v", after, before+1)
	}

	// error path → status="error", and failover still advances
	eBefore := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_err", "get_stream", "error"))
	_, _, _ = runFailoverNamed(
		context.Background(), nil,
		[]domain.Provider{newTestProvider("p3_err")}, nil, 0, "get_stream",
		func(c context.Context, p domain.Provider) (string, error) { return "", errors.New("boom") },
	)
	eAfter := testutil.ToFloat64(metrics.ParserRequestsTotal.WithLabelValues("p3_err", "get_stream", "error"))
	if eAfter != eBefore+1 {
		t.Errorf("parser_requests_total{p3_err,get_stream,error} = %v, want %v", eAfter, eBefore+1)
	}
}
```

> `newTestProvider(name)` must construct whatever the package's existing fake provider is. If `orchestrator_test.go` already defines a helper that builds a `domain.Provider` with a settable name, call it. If the existing fake has a different constructor name/shape, adapt this line to it (the test only needs a provider whose `Name()` returns the given string — the `call` closure, not the provider's own methods, is what runs). Do NOT add a second fake type if one already exists.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/scraper && go test ./internal/service/ -run TestRunFailoverNamed_EmitsParserMetrics -v`
Expected: COMPILE FAIL — `runFailoverNamed` takes the wrong number of args (no `operation` param yet). That compile failure IS the red state.

- [ ] **Step 3: Add the `operation` param + instrumentation**

In `services/scraper/internal/service/orchestrator.go`:

(a) `runFailover` — add `operation string` after `providerTimeout`, pass it through:

```go
func runFailover[T any](
	ctx context.Context,
	log *logger.Logger,
	providers []domain.Provider,
	cache *health.InMemoryHealthCache,
	providerTimeout time.Duration,
	operation string,
	call func(ctx context.Context, p domain.Provider) (T, error),
) (T, error) {
	v, _, err := runFailoverNamed(ctx, log, providers, cache, providerTimeout, operation, call)
	return v, err
}
```

(b) `runFailoverNamed` — add `operation string` after `providerTimeout`, and wrap the provider call (the existing block at ~323) with `ObserveParser`:

```go
func runFailoverNamed[T any](
	ctx context.Context,
	log *logger.Logger,
	providers []domain.Provider,
	cache *health.InMemoryHealthCache,
	providerTimeout time.Duration,
	operation string,
	call func(ctx context.Context, p domain.Provider) (T, error),
) (T, string, error) {
```

Then, inside the loop, replace the existing call block:

```go
		result, err := providerCall(ctx, providerTimeout, func(c context.Context) (T, error) {
			return call(c, p)
		})
		if err == nil {
			return result, p.Name(), nil
		}
```

with a parser-observed version (the `defer` inside a closure scopes the observation to exactly this attempt; the health-cache SKIP path above is untouched, so skipped providers emit no observation):

```go
		result, err := func() (res T, e error) {
			defer metrics.ObserveParser(p.Name(), operation, time.Now(), &e)
			return providerCall(ctx, providerTimeout, func(c context.Context) (T, error) {
				return call(c, p)
			})
		}()
		if err == nil {
			return result, p.Name(), nil
		}
```

(`metrics` and `time` are already imported in orchestrator.go — `metrics.ParserFallbackTotal` and `time.Duration` are already used.)

(c) Update the 4 call sites to pass the operation label:
- `FindIDNamed` (~404): `…o.providerBudget(), "find_id",` before the `func(c, p)` arg.
- `ListEpisodesNamed` (~423): `…o.providerBudget(), "list_episodes",`
- `ListServers` (~431): `…o.providerBudget(), "list_servers",`
- `GetStream` (~439): `…o.providerBudget(), "get_stream",`

- [ ] **Step 4: Run it to verify it passes**

Run: `cd services/scraper && go build ./... && go test ./internal/service/ -run TestRunFailoverNamed_EmitsParserMetrics -v`
Expected: PASS. Then run the whole service package to confirm no other caller broke: `go test ./internal/service/ -count=1` → PASS (the 4 call sites are the only callers; the build proves they compile).

- [ ] **Step 5: Commit**

```bash
git commit services/scraper/internal/service/orchestrator.go \
  services/scraper/internal/service/orchestrator_parser_metrics_test.go \
  -m "feat(scraper): per-provider parser latency via orchestrator failover hook

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: ae parser metrics

**Files:**
- Modify: `services/catalog/internal/service/raw_resolver.go` (`GetLibraryEpisodes` ~370, `GetLibraryStream` ~409)

**Interfaces:**
- Produces: `parser_requests_total{provider="ae",operation}` + `parser_request_duration_seconds{provider="ae",operation}` for `get_episodes`/`get_stream`.

- [ ] **Step 1: Add `defer ObserveParser` via named error returns**

In `services/catalog/internal/service/raw_resolver.go`, give both methods a named `err` return and a `defer` at the top. `GetLibraryEpisodes`:

```go
func (r *RawResolver) GetLibraryEpisodes(ctx context.Context, animeID string) (_ *EpisodesResponse, err error) {
	defer metrics.ObserveParser("ae", "get_episodes", time.Now(), &err)
```

`GetLibraryStream`:

```go
func (r *RawResolver) GetLibraryStream(ctx context.Context, animeID string, episodeNumber int, quality string) (_ *RawStream, err error) {
	defer metrics.ObserveParser("ae", "get_stream", time.Now(), &err)
```

Leave the method bodies unchanged — every existing `return nil, someErr` now assigns the named `err` (so the defer records `status="error"`), and `return resp, nil` records `status="success"`. Add the imports if missing: `"time"` and `"github.com/ILITA-hub/animeenigma/libs/metrics"` (check the existing import block — `time` is likely already present; `libs/metrics` may not be).

> Note: "not in library" is signaled as a 404 → the method returns a sentinel error (e.g. `domain.ErrNotFound`/`ErrNoLibraryCopy`). That counts as `status="error"` in parser metrics, which is correct: from the ae provider's view, a 404 is a non-success resolution. The health probe (Task 3) measures library *liveness* separately, so a title simply not being encoded does not drag ae's health gauge down.

- [ ] **Step 2: Build + verify no behavior change**

Run: `cd services/catalog && go build ./... && go test ./internal/service/ -count=1`
Expected: clean build, existing tests still PASS (named returns + defer don't change return values). There is no focused unit test here: `RawResolver` holds a concrete `*library.Client` (not an interface), so the emission can't be faked at unit level without a wider refactor that is out of scope — coverage is the build + the deploy-time check (Task 4) that `parser_request_duration_seconds{provider="ae"}` appears after an ae request.

- [ ] **Step 3: Commit**

```bash
git commit services/catalog/internal/service/raw_resolver.go \
  -m "feat(catalog): emit ae parser latency metrics (get_episodes/get_stream)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: ae health probe (library liveness)

**Files:**
- Modify: `services/catalog/internal/service/health_checker.go`
- Create: `services/catalog/internal/service/health_checker_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (pass the library client; construct the checker after `libraryClient`)

**Interfaces:**
- Consumes: `library.Client.Ping(ctx context.Context) error` (existing).
- Produces: `provider_health_up{provider="ae",stage="liveness"}` (1=up,0=down) + `provider_probe_last_tick_timestamp{provider="ae"}`; `NewPlayerHealthChecker` gains a trailing `aeProbe aePinger` param.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/health_checker_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeAePinger struct{ err error }

func (f fakeAePinger) Ping(ctx context.Context) error { return f.err }

func TestCheckAe_UpWhenPingOK(t *testing.T) {
	h := NewPlayerHealthChecker(nil, 0, logger.NewNop(), fakeAePinger{err: nil})
	h.checkAe()
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues("ae", "liveness")); got != 1 {
		t.Errorf("provider_health_up{ae,liveness} = %v, want 1", got)
	}
}

func TestCheckAe_DownWhenPingErrors(t *testing.T) {
	h := NewPlayerHealthChecker(nil, 0, logger.NewNop(), fakeAePinger{err: errors.New("library down")})
	h.checkAe()
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues("ae", "liveness")); got != 0 {
		t.Errorf("provider_health_up{ae,liveness} = %v, want 0", got)
	}
}
```

> If `logger.NewNop()` is not the exact no-op logger constructor in this repo, use whatever the existing service tests use to build a `*logger.Logger` (grep `logger.` in `services/catalog/internal/service/*_test.go`). `nil` is passed for the kodik client because `checkAe` does not touch it.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestCheckAe -v`
Expected: COMPILE FAIL — `NewPlayerHealthChecker` takes 3 args, not 4; `checkAe` undefined.

- [ ] **Step 3: Implement the ae probe**

In `services/catalog/internal/service/health_checker.go`:

(a) Add the const + interface + struct field near the top (after the existing `providerKodik`/`kodikStage` consts):

```go
const (
	// providerAe is the provider label the self-hosted library reports under.
	providerAe = "ae"
	// aeStage is ae's single synthetic liveness stage (library-service reachability).
	aeStage = "liveness"
)

// aePinger is the minimal library-client surface the ae liveness probe needs.
// library.Client satisfies it via Ping(ctx) (HTTP GET, non-2xx → error).
type aePinger interface {
	Ping(ctx context.Context) error
}
```

Add the field to the struct:

```go
type PlayerHealthChecker struct {
	kodikClient *kodik.Client
	aeProbe     aePinger
	interval    time.Duration
	log         *logger.Logger

	prevStatus map[string]bool
}
```

(b) Update the constructor signature + body:

```go
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	interval time.Duration,
	log *logger.Logger,
	aeProbe aePinger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PlayerHealthChecker{
		kodikClient: kodikClient,
		aeProbe:     aeProbe,
		interval:    interval,
		log:         log,
		prevStatus:  make(map[string]bool),
	}
}
```

(c) Add `checkAe` and call it from `checkAll`:

```go
func (h *PlayerHealthChecker) checkAll() {
	h.checkProvider(providerKodik, kodikStage, h.checkKodik)
	h.checkProvider(providerAe, aeStage, h.checkAe)
}

// checkAe probes the self-hosted library service for liveness (ae availability IS
// library availability). A 5s-bounded Ping; non-2xx/unreachable → DOWN. Per-title
// "not encoded yet" (404 on a real resolve) is NOT measured here.
func (h *PlayerHealthChecker) checkAe() error {
	if h.aeProbe == nil {
		return fmt.Errorf("ae library client not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.aeProbe.Ping(ctx)
}
```

(`context` and `fmt` are already imported in health_checker.go.)

- [ ] **Step 4: Run it to verify it passes**

Run: `cd services/catalog && go test ./internal/service/ -run TestCheckAe -v`
Expected: PASS (both up and down cases).

- [ ] **Step 5: Wire the library client into the checker at boot**

In `services/catalog/cmd/catalog-api/main.go`, the `PlayerHealthChecker` is currently constructed (~line 231) BEFORE `libraryClient` (~line 271). Move the checker construction + goroutine start to AFTER `libraryClient` is created (after the `rawResolver := service.NewRawResolver(...)` line), and pass `libraryClient` as the new arg:

```go
	healthChecker := service.NewPlayerHealthChecker(
		catalogService.KodikClient(),
		cfg.HealthCheck.Interval,
		log,
		libraryClient,
	)
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go healthChecker.Start(healthCtx)
```

Delete the old construction+start block at ~231. (`libraryClient` is a `*library.Client`, which satisfies `aePinger` via its `Ping` method.)

- [ ] **Step 6: Build the whole catalog service**

Run: `cd services/catalog && go build ./... && go test ./internal/service/ -count=1`
Expected: clean build (the moved construction compiles; `libraryClient` is in scope at the new location), tests PASS.

- [ ] **Step 7: Commit**

```bash
git commit services/catalog/internal/service/health_checker.go \
  services/catalog/internal/service/health_checker_test.go \
  services/catalog/cmd/catalog-api/main.go \
  -m "feat(catalog): ae liveness probe via library Ping (provider_health_up{ae})

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 4: Deploy + verify parity

**Files:** none (deploy/verify only).

- [ ] **Step 1: Redeploy scraper then catalog**

Build from a fresh clean `origin/main` worktree (copy `docker/.env`; compose project stays `docker`), not the shared dirty tree.

Run: `make redeploy-scraper && make redeploy-catalog`

- [ ] **Step 2: Drive a little EN + ae traffic so the histograms have samples**

```bash
# EN: trigger a scraper resolve (any catalogued anime UUID works; use one you know).
curl -s "http://localhost:8000/api/anime/_/scraper/health" >/dev/null
# Give the catalog ae probe one tick (it runs on boot) + let scraper probes warm.
sleep 5
```

- [ ] **Step 3: Verify the EN chain now has per-provider parser latency (the "4→8" fix)**

```bash
curl -s http://localhost:8088/metrics | grep -oP 'parser_request_duration_seconds_count\{[^}]*provider="\K[^"]+' | sort -u
```
Expected: scraper-side providers appear (`gogoanime`, `allanime`, `miruro`, `animefever`, `nineanime`, … — whichever were exercised). Before Phase 3 this was empty on the scraper target. (Counts only accrue for operations actually run; a provider that was never dispatched won't appear until traffic hits it — note that in the result rather than treating absence as failure.)

- [ ] **Step 4: Verify ae parser metrics exist on the catalog target**

```bash
curl -s http://localhost:8081/metrics | grep -E 'parser_request(s_total|_duration_seconds_count)\{[^}]*provider="ae"'
```
Expected: `parser_requests_total{provider="ae",operation="get_episodes"|"get_stream",status=…}` present after an ae request (drive one via the ae endpoint for a title that exists locally, or accept that the series register on first ae use).

- [ ] **Step 5: Verify the ae liveness probe is live**

```bash
curl -s http://localhost:8081/metrics | grep -E 'provider_health_up\{provider="ae"|provider_probe_last_tick_timestamp\{provider="ae"'
```
Expected: `provider_health_up{provider="ae",stage="liveness"} 1` (library reachable) and a non-zero `provider_probe_last_tick_timestamp{provider="ae"}`. Confirm in catalog logs: `docker logs animeenigma-catalog | grep -i '"provider": "ae"'` shows an UP/DOWN line.

- [ ] **Step 6: Health check**

Run: `make health`
Expected: all services healthy; EN + ae playback behavior unchanged (instrumentation is `defer`/probe-only).

---

## Self-Review (Phase 3)

**Scope coverage (locked decisions):**
- EN scraper-chain parser latency → Task 1 (central `runFailoverNamed` hook; all 8 EN+adult providers × 4 ops). ✓ — fixes "Parser p95 shows 4 not 8".
- ae parser metrics → Task 2. ✓
- ae health probe via library `Ping` → Task 3. ✓
- No new instrumentation for retiring legacy players → respected (only `ae` + EN chain touched). ✓
- ae bespoke panel retirement → correctly deferred to Phase 4. ✓
- 18anime egress tag + `GetStreamGated` → explicitly deferred + documented (not silent). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every test step has assertion + run command + expected output. The one adaptation point (the existing fake-provider helper name in Task 1, and the no-op logger constructor in Task 3) is flagged explicitly with how to resolve it from the existing tests, not left vague. ✓

**Type consistency:** `runFailoverNamed`/`runFailover` gain `operation string` in the same position (after `providerTimeout`), and all 4 call sites + the internal `runFailover→runFailoverNamed` delegation pass it. `metrics.ObserveParser(provider, operation string, start time.Time, errp *error)` used identically in Tasks 1 & 2 (matches `libs/metrics/parser.go:87`). `aePinger.Ping(ctx) error` matches `library.Client.Ping` and is the param type added to `NewPlayerHealthChecker`; the same 4-arg constructor signature is used in the test (Task 3 Step 1) and the boot wiring (Task 3 Step 5). `provider_health_up`/`provider_probe_last_tick_timestamp` label usage matches the existing kodik probe in `checkProvider`. ✓
