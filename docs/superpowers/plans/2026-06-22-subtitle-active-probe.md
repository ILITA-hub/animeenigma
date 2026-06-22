# Active Subtitle-Provider Probe + Degraded Note — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a periodic active probe of the Jimaku + OpenSubtitles APIs that records up/degraded/down + latency, exposes it as Prometheus gauges + a Grafana panel, and surfaces a "provider degraded/down" note in the subtitle picker — complementing the existing passive `catalog_subtitle_*` observability.

**Architecture:** Probe lives in `catalog` (it already owns the Jimaku/OpenSubtitles clients + API keys). Scheduler triggers `POST /internal/subtitle-probe/run` every 5 min; the probe pings each provider's cheapest non-quota endpoint, classifies the verdict, stores it in an in-memory `HealthStore`, and emits `probe_subtitle_*` gauges. The `SubsAggregator` overlays the live `HealthStore` snapshot onto the `/subtitles/all` response (fresh, never cached), merged with the existing passive `providers_down`; `OtherSubsPanel.vue` renders one merged note.

**Tech Stack:** Go (chi, prometheus/promauto, robfig/cron), Vue 3 + TypeScript (vitest), Grafana JSON dashboards.

## Global Constraints

- **Effort/impact metrics:** no time units. CHANGELOG/plan scoring uses UXΔ / CDI / MVQ per `.planning/CONVENTIONS.md`.
- **Worktree only:** all work in `.claude/worktrees/subtitle-active-probe` (already created off fresh origin/main). Never touch `/data/animeenigma` base tree.
- **Go conventions:** snake_case files, PascalCase exported types, `libs/errors` for domain errors, `libs/logger` structured logging. Mock external APIs in tests (`httptest`), never hit live APIs.
- **Active-probe metrics are `probe_`-prefixed** and MUST stay distinct from the existing passive `catalog_subtitle_*` family (do not modify the passive metrics).
- **`provider_health` is NEVER cached** — overlaid fresh from the in-memory store on every response.
- **Internal route** `/internal/*` is Docker-network-only (gateway does not proxy it) — no auth middleware.
- **DS lint (frontend):** semantic tokens only; `provider`-accent hues `cyan/pink/orange/rose` are exempt but new copy uses `text-warning`. Only `font-medium`/`font-semibold`.
- **Locale parity:** any new i18n key must be added to `en.json`, `ru.json`, AND `ja.json` (parity test enforces it).

---

## File Structure

**Create:**
- `services/catalog/internal/service/subprobe/store.go` — `HealthStore` (in-memory verdict cache).
- `services/catalog/internal/service/subprobe/store_test.go`
- `services/catalog/internal/service/subprobe/probe.go` — `Probe`, `Pinger`, classification, gauge emission.
- `services/catalog/internal/service/subprobe/probe_test.go`
- `services/catalog/internal/handler/internal_subtitle_probe.go` — `POST /internal/subtitle-probe/run` handler.
- `services/catalog/internal/handler/internal_subtitle_probe_test.go`
- `services/scheduler/internal/jobs/subtitle_probe_trigger.go`
- `services/scheduler/internal/jobs/subtitle_probe_trigger_test.go`

**Modify:**
- `libs/metrics/probe.go` — 3 new gauges.
- `services/catalog/internal/parser/jimaku/client.go` — `Ping`.
- `services/catalog/internal/parser/opensubtitles/client.go` — `Ping`.
- `services/catalog/internal/service/subs_aggregator.go` — `ProviderHealth` type/field + overlay + new ctor param.
- `services/catalog/internal/transport/router.go` — new internal route + ctor param.
- `services/catalog/cmd/catalog-api/main.go` — construct store/probe/handler; wire store into aggregator + router.
- `services/scheduler/internal/config/config.go` — `SubtitleProbeCron`.
- `services/scheduler/internal/service/job.go` — register `subtitle_probe` cron.
- `services/scheduler/cmd/scheduler-api/main.go` — construct + pass the job.
- `frontend/web/src/types/raw.ts` — `ProviderHealth` + `provider_health`.
- `frontend/web/src/components/player/OtherSubsPanel.vue` — merged note.
- `frontend/web/src/components/player/OtherSubsPanel.spec.ts` — note cases.
- `frontend/web/src/locales/{en,ru,ja}.json` — note keys.
- `docker/grafana/dashboards/subtitle-health.json` — active-probe panels.
- `docker/docker-compose.yml` — scheduler `SUBTITLE_PROBE_CRON` env (+ CLAUDE.md doc).

---

## Task 1: Active-probe metrics

**Files:**
- Modify: `libs/metrics/probe.go`
- Test: `libs/metrics/probe_subtitle_test.go` (create)

**Interfaces:**
- Produces: `metrics.ProbeSubtitleProviderUp *prometheus.GaugeVec` (`probe_subtitle_provider_up{provider}`), `metrics.ProbeSubtitleLatencySeconds *prometheus.GaugeVec` (`probe_subtitle_latency_seconds{provider}`), `metrics.ProbeSubtitleLastRun prometheus.Gauge` (`probe_subtitle_last_run_timestamp`).

- [ ] **Step 1: Write the failing test**

Create `libs/metrics/probe_subtitle_test.go`:

```go
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProbeSubtitleProviderUp_SetAndGather(t *testing.T) {
	ProbeSubtitleProviderUp.Reset()
	ProbeSubtitleProviderUp.WithLabelValues("jimaku").Set(0.5)
	if got := testutil.ToFloat64(ProbeSubtitleProviderUp.WithLabelValues("jimaku")); got != 0.5 {
		t.Fatalf("probe_subtitle_provider_up{jimaku} = %v; want 0.5", got)
	}
}

func TestProbeSubtitleLatency_SetAndGather(t *testing.T) {
	ProbeSubtitleLatencySeconds.WithLabelValues("opensubtitles").Set(1.25)
	if got := testutil.ToFloat64(ProbeSubtitleLatencySeconds.WithLabelValues("opensubtitles")); got != 1.25 {
		t.Fatalf("probe_subtitle_latency_seconds{opensubtitles} = %v; want 1.25", got)
	}
}

func TestProbeSubtitleLastRun_Set(t *testing.T) {
	ProbeSubtitleLastRun.Set(1700000000)
	if got := testutil.ToFloat64(ProbeSubtitleLastRun); got != 1700000000 {
		t.Fatalf("probe_subtitle_last_run_timestamp = %v; want 1700000000", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/subtitle-active-probe && go test ./libs/metrics/ -run ProbeSubtitle -v`
Expected: FAIL — `undefined: ProbeSubtitleProviderUp` (compile error).

- [ ] **Step 3: Add the gauges**

Append to the `var (...)` block in `libs/metrics/probe.go` (before the closing `)`):

```go
	// ProbeSubtitleProviderUp is the ACTIVE subtitle probe verdict per provider:
	// 1 up, 0.5 degraded, 0 down. Distinct from the passive
	// catalog_subtitle_provider_up (which is driven by real resolve traffic).
	// Reset() each run so a provider that drops out of the probe set is not left
	// with a stale series.
	ProbeSubtitleProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_subtitle_provider_up",
		Help: "Active subtitle-probe verdict per provider: 1 up, 0.5 degraded, 0 down.",
	}, []string{"provider"})

	// ProbeSubtitleLatencySeconds is the last active-probe ping latency per provider.
	ProbeSubtitleLatencySeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_subtitle_latency_seconds",
		Help: "Last active subtitle-probe ping latency per provider, in seconds.",
	}, []string{"provider"})

	// ProbeSubtitleLastRun is the unix timestamp of the last completed subtitle probe run.
	ProbeSubtitleLastRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "probe_subtitle_last_run_timestamp",
		Help: "Unix timestamp of the last completed active subtitle probe run.",
	})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./libs/metrics/ -run ProbeSubtitle -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add libs/metrics/probe.go libs/metrics/probe_subtitle_test.go
git commit -m "feat(metrics): active subtitle-probe gauges (probe_subtitle_*)"
```

---

## Task 2: Provider Ping methods

**Files:**
- Modify: `services/catalog/internal/parser/jimaku/client.go`
- Modify: `services/catalog/internal/parser/opensubtitles/client.go`
- Test: `services/catalog/internal/parser/jimaku/ping_test.go` (create)
- Test: `services/catalog/internal/parser/opensubtitles/ping_test.go` (create)

**Interfaces:**
- Produces: `(*jimaku.Client).Ping(ctx context.Context) (time.Duration, error)` and `(*opensubtitles.Client).Ping(ctx context.Context) (time.Duration, error)`. Both return round-trip latency; non-2xx → error. OpenSubtitles returns `opensubtitles.ErrRateLimited` on 429 and `opensubtitles.ErrUnauthorized` on 401/403.

- [ ] **Step 1: Write the failing tests**

Create `services/catalog/internal/parser/jimaku/ping_test.go`:

```go
package jimaku

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/entries/search" || r.URL.Query().Get("anilist_id") != "1" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()
	c := NewClient("key")
	c.baseURL = srv.URL
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping OK: unexpected error %v", err)
	}
}

func TestPing_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := NewClient("key")
	c.baseURL = srv.URL
	if _, err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping 503: expected error, got nil")
	}
}
```

Create `services/catalog/internal/parser/opensubtitles/ping_test.go`:

```go
package opensubtitles

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/infos/formats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Api-Key") != "key" {
			t.Errorf("missing Api-Key header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping OK: unexpected error %v", err)
	}
}

func TestPing_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "key", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Ping 429: want ErrRateLimited, got %v", err)
	}
}

func TestPing_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "bad", BaseURL: srv.URL})
	if _, err := c.Ping(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Ping 401: want ErrUnauthorized, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/catalog/internal/parser/jimaku/ ./services/catalog/internal/parser/opensubtitles/ -run Ping -v`
Expected: FAIL — `c.Ping undefined`.

- [ ] **Step 3: Implement `jimaku.Client.Ping`**

In `services/catalog/internal/parser/jimaku/client.go`, add `"context"` to the import block (keep the others), then add:

```go
// Ping checks Jimaku reachability + API-key validity with the cheapest possible
// authenticated call: a search for a stable AniList ID (1). Any 200 (even an
// empty result array) proves the API answered. Returns the round-trip latency.
// The caller is expected to supply a ctx with a timeout.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/entries/search?anilist_id=1", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Since(start), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return time.Since(start), fmt.Errorf("jimaku: ping status %d", resp.StatusCode)
	}
	return time.Since(start), nil
}
```

(`io`, `fmt`, `net/http`, `time` are already imported in client.go.)

- [ ] **Step 4: Implement `opensubtitles.Client.Ping`**

In `services/catalog/internal/parser/opensubtitles/client.go`, add:

```go
// Ping checks OpenSubtitles reachability + API-key validity via /infos/formats,
// a static reference endpoint that needs only the Api-Key header and does NOT
// consume the daily download quota. Returns round-trip latency. 401/403 →
// ErrUnauthorized, 429 → ErrRateLimited, other non-2xx → a generic error.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/infos/formats", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Api-Key", c.cfg.APIKey)
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Since(start), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return time.Since(start), ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return time.Since(start), ErrRateLimited
	case resp.StatusCode >= 400:
		return time.Since(start), fmt.Errorf("opensubtitles: ping status %d", resp.StatusCode)
	}
	return time.Since(start), nil
}
```

(`io`, `fmt`, `net/http`, `time`, `context` are already imported.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/catalog/internal/parser/jimaku/ ./services/catalog/internal/parser/opensubtitles/ -run Ping -v`
Expected: PASS (5 tests).

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/parser/jimaku/ services/catalog/internal/parser/opensubtitles/
git commit -m "feat(catalog): Ping reachability methods on jimaku + opensubtitles clients"
```

---

## Task 3: HealthStore

**Files:**
- Create: `services/catalog/internal/service/subprobe/store.go`
- Test: `services/catalog/internal/service/subprobe/store_test.go`

**Interfaces:**
- Produces:
  - `subprobe.Status` (string type) with consts `StatusUp="up"`, `StatusDegraded="degraded"`, `StatusDown="down"`, `StatusUnknown="unknown"`.
  - `subprobe.Health struct { Status Status; LatencyMS int64; CheckedAt time.Time }`.
  - `subprobe.NewStore(staleAfter time.Duration) *Store`.
  - `(*Store).Record(provider string, h Health)`.
  - `(*Store).Snapshot() map[string]Health` — entries older than `staleAfter` are returned with `Status = StatusUnknown`.
  - test-only seam: `(*Store).now func() time.Time` field set via `NewStore` (default `time.Now`).

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subprobe/store_test.go`:

```go
package subprobe

import (
	"sync"
	"testing"
	"time"
)

func TestStore_RecordAndSnapshot(t *testing.T) {
	s := NewStore(15 * time.Minute)
	s.Record("jimaku", Health{Status: StatusUp, LatencyMS: 120, CheckedAt: time.Unix(1000, 0)})
	snap := s.Snapshot()
	if snap["jimaku"].Status != StatusUp || snap["jimaku"].LatencyMS != 120 {
		t.Fatalf("snapshot = %+v; want up/120", snap["jimaku"])
	}
}

func TestStore_StaleDowngradesToUnknown(t *testing.T) {
	now := time.Unix(10_000, 0)
	s := NewStore(60 * time.Second)
	s.now = func() time.Time { return now }
	s.Record("jimaku", Health{Status: StatusUp, CheckedAt: now.Add(-120 * time.Second)})
	if got := s.Snapshot()["jimaku"].Status; got != StatusUnknown {
		t.Fatalf("stale status = %q; want unknown", got)
	}
}

func TestStore_SnapshotIsCopy(t *testing.T) {
	s := NewStore(time.Minute)
	s.Record("jimaku", Health{Status: StatusUp, CheckedAt: time.Now()})
	snap := s.Snapshot()
	snap["jimaku"] = Health{Status: StatusDown}
	if s.Snapshot()["jimaku"].Status != StatusUp {
		t.Fatal("Snapshot must return a copy; mutation leaked into the store")
	}
}

func TestStore_ConcurrentRecordSnapshot(t *testing.T) {
	s := NewStore(time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Record("jimaku", Health{Status: StatusUp, CheckedAt: time.Now()}) }()
		go func() { defer wg.Done(); _ = s.Snapshot() }()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/service/subprobe/ -v`
Expected: FAIL — package/symbols undefined.

- [ ] **Step 3: Implement the store**

Create `services/catalog/internal/service/subprobe/store.go`:

```go
// Package subprobe implements the active subtitle-provider health probe: it
// pings the Jimaku + OpenSubtitles APIs on a fixed interval, classifies each as
// up/degraded/down, stores the latest verdict in an in-memory HealthStore, and
// emits probe_subtitle_* Prometheus gauges. The SubsAggregator reads the store
// to overlay provider_health on the /subtitles/all response.
package subprobe

import (
	"sync"
	"time"
)

// Status is the active-probe verdict for one provider.
type Status string

const (
	StatusUp       Status = "up"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
	StatusUnknown  Status = "unknown"
)

// Health is the latest active-probe verdict for one provider.
type Health struct {
	Status    Status
	LatencyMS int64
	CheckedAt time.Time
}

// Store holds the latest per-provider Health, guarded for concurrent
// probe writes + request-path reads.
type Store struct {
	mu         sync.RWMutex
	health     map[string]Health
	staleAfter time.Duration
	now        func() time.Time
}

// NewStore returns a Store. Entries whose CheckedAt is older than staleAfter are
// reported as StatusUnknown by Snapshot (never a stale "up").
func NewStore(staleAfter time.Duration) *Store {
	return &Store{
		health:     map[string]Health{},
		staleAfter: staleAfter,
		now:        time.Now,
	}
}

// Record stores the latest verdict for a provider.
func (s *Store) Record(provider string, h Health) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[provider] = h
}

// Snapshot returns a copy of all known provider health, downgrading any entry
// older than staleAfter to StatusUnknown.
func (s *Store) Snapshot() map[string]Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Health, len(s.health))
	now := s.now()
	for p, h := range s.health {
		if now.Sub(h.CheckedAt) > s.staleAfter {
			h.Status = StatusUnknown
		}
		out[p] = h
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/catalog/internal/service/subprobe/ -race -v`
Expected: PASS (4 tests), no race.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subprobe/store.go services/catalog/internal/service/subprobe/store_test.go
git commit -m "feat(catalog): subprobe HealthStore with staleness downgrade"
```

---

## Task 4: Probe + classification + gauge emission

**Files:**
- Create: `services/catalog/internal/service/subprobe/probe.go`
- Test: `services/catalog/internal/service/subprobe/probe_test.go`

**Interfaces:**
- Consumes: `subprobe.Store` (Task 3), `metrics.ProbeSubtitle*` (Task 1), `opensubtitles.ErrRateLimited` (existing).
- Produces:
  - `subprobe.Pinger` interface: `Ping(ctx context.Context) (time.Duration, error)` — satisfied by `*jimaku.Client` and `*opensubtitles.Client` (Task 2).
  - `subprobe.New(store *Store, pingers map[string]Pinger, degradedLatency, timeout time.Duration, log *logger.Logger) *Probe`.
  - `(*Probe).RunOnce(ctx context.Context)` — pings each provider (per-provider timeout + panic-recover), records to the store, emits gauges, sets last-run.
  - test seam: `(*Probe).now func() time.Time` (default `time.Now`).

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subprobe/probe_test.go`:

```go
package subprobe

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

type fakePinger struct {
	lat time.Duration
	err error
}

func (f fakePinger) Ping(ctx context.Context) (time.Duration, error) { return f.lat, f.err }

func TestRunOnce_ClassifiesAndRecords(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"jimaku":        fakePinger{lat: 100 * time.Millisecond},                 // fast → up
		"opensubtitles": fakePinger{lat: 5 * time.Second},                        // slow → degraded
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())

	snap := store.Snapshot()
	if snap["jimaku"].Status != StatusUp {
		t.Errorf("jimaku = %q; want up", snap["jimaku"].Status)
	}
	if snap["opensubtitles"].Status != StatusDegraded {
		t.Errorf("opensubtitles = %q; want degraded", snap["opensubtitles"].Status)
	}
}

func TestRunOnce_TransportErrorIsDown(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"jimaku": fakePinger{err: context.DeadlineExceeded},
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())
	if store.Snapshot()["jimaku"].Status != StatusDown {
		t.Fatalf("transport error: want down, got %q", store.Snapshot()["jimaku"].Status)
	}
}

func TestRunOnce_RateLimitedIsDegraded(t *testing.T) {
	store := NewStore(time.Minute)
	p := New(store, map[string]Pinger{
		"opensubtitles": fakePinger{lat: 50 * time.Millisecond, err: opensubtitles.ErrRateLimited},
	}, 2*time.Second, 8*time.Second, nil)
	p.RunOnce(context.Background())
	if store.Snapshot()["opensubtitles"].Status != StatusDegraded {
		t.Fatalf("rate-limited: want degraded, got %q", store.Snapshot()["opensubtitles"].Status)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		lat  time.Duration
		err  error
		want Status
	}{
		{"fast ok", 100 * time.Millisecond, nil, StatusUp},
		{"slow ok", 3 * time.Second, nil, StatusDegraded},
		{"rate limited", 0, opensubtitles.ErrRateLimited, StatusDegraded},
		{"unauthorized", 0, opensubtitles.ErrUnauthorized, StatusDown},
		{"deadline", 0, context.DeadlineExceeded, StatusDown},
	}
	for _, c := range cases {
		if got := classify(c.lat, c.err, 2*time.Second); got != c.want {
			t.Errorf("%s: classify = %q; want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/service/subprobe/ -run RunOnce -v`
Expected: FAIL — `New`/`Pinger`/`classify` undefined.

- [ ] **Step 3: Implement the probe**

Create `services/catalog/internal/service/subprobe/probe.go`:

```go
package subprobe

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

// Pinger is one provider's reachability check. Satisfied by *jimaku.Client and
// *opensubtitles.Client (both expose Ping(ctx) (time.Duration, error)).
type Pinger interface {
	Ping(ctx context.Context) (time.Duration, error)
}

// Probe pings each configured provider on demand (driven by the scheduler-fired
// /internal/subtitle-probe/run endpoint), records verdicts in the store, and
// emits the probe_subtitle_* gauges.
type Probe struct {
	pingers         map[string]Pinger
	store           *Store
	degradedLatency time.Duration
	timeout         time.Duration
	log             *logger.Logger
	now             func() time.Time
}

// New builds a Probe. pingers holds only the CONFIGURED providers (unconfigured
// ones are omitted by the caller so they never show a permanent "down").
func New(store *Store, pingers map[string]Pinger, degradedLatency, timeout time.Duration, log *logger.Logger) *Probe {
	return &Probe{
		pingers:         pingers,
		store:           store,
		degradedLatency: degradedLatency,
		timeout:         timeout,
		log:             log,
		now:             time.Now,
	}
}

// RunOnce probes every configured provider once. Each provider is isolated by a
// per-provider timeout + panic-recover so one slow/broken provider can neither
// hang nor abort the others. Verdicts are recorded in the store and emitted as
// gauges; ProbeSubtitleProviderUp is Reset() first so a dropped provider does
// not leave a stale series.
func (p *Probe) RunOnce(ctx context.Context) {
	metrics.ProbeSubtitleProviderUp.Reset()
	for provider, pinger := range p.pingers {
		h := p.probeOne(ctx, provider, pinger)
		p.store.Record(provider, h)
		metrics.ProbeSubtitleProviderUp.WithLabelValues(provider).Set(gaugeValue(h.Status))
		metrics.ProbeSubtitleLatencySeconds.WithLabelValues(provider).Set(float64(h.LatencyMS) / 1000)
	}
	metrics.ProbeSubtitleLastRun.Set(float64(p.now().Unix()))
}

func (p *Probe) probeOne(ctx context.Context, provider string, pinger Pinger) (h Health) {
	defer func() {
		if r := recover(); r != nil {
			if p.log != nil {
				p.log.Errorw("subtitle probe panicked", "provider", provider, "panic", r)
			}
			h = Health{Status: StatusDown, CheckedAt: p.now()}
		}
	}()
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	lat, err := pinger.Ping(cctx)
	if err != nil && p.log != nil {
		p.log.Warnw("subtitle probe ping failed", "provider", provider, "error", err, "latency_ms", lat.Milliseconds())
	}
	return Health{Status: classify(lat, err, p.degradedLatency), LatencyMS: lat.Milliseconds(), CheckedAt: p.now()}
}

// classify maps a ping result to a verdict: a transient rate-limit is degraded,
// any other error is down, a slow success is degraded, a fast success is up.
func classify(lat time.Duration, err error, degradedLatency time.Duration) Status {
	if err != nil {
		if errors.Is(err, opensubtitles.ErrRateLimited) {
			return StatusDegraded
		}
		return StatusDown
	}
	if lat > degradedLatency {
		return StatusDegraded
	}
	return StatusUp
}

func gaugeValue(s Status) float64 {
	switch s {
	case StatusUp:
		return 1
	case StatusDegraded:
		return 0.5
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/catalog/internal/service/subprobe/ -race -v`
Expected: PASS (all store + probe tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subprobe/probe.go services/catalog/internal/service/subprobe/probe_test.go
git commit -m "feat(catalog): subprobe Probe — classify + gauge emission + panic-isolated RunOnce"
```

---

## Task 5: ProviderHealth overlay on the subs response

**Files:**
- Modify: `services/catalog/internal/service/subs_aggregator.go`
- Test: `services/catalog/internal/service/subs_aggregator_health_test.go` (create)

**Interfaces:**
- Consumes: `subprobe.Store.Snapshot()` (Task 3) via a `HealthSnapshotter` interface.
- Produces:
  - `service.ProviderHealth struct { Provider string `json:"provider"`; Status string `json:"status"`; LatencyMS int64 `json:"latency_ms,omitempty"` }`.
  - `AggregateResponse.ProviderHealth []ProviderHealth `json:"provider_health,omitempty"``.
  - `service.HealthSnapshotter` interface: `Snapshot() map[string]subprobe.Health`.
  - `NewSubsAggregator(..., health HealthSnapshotter, log)` — NEW param inserted before `log`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subs_aggregator_health_test.go`:

```go
package service

import (
	"sort"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/subprobe"
)

type fakeSnapshotter struct{ snap map[string]subprobe.Health }

func (f fakeSnapshotter) Snapshot() map[string]subprobe.Health { return f.snap }

func TestOverlayHealth_MergesProbeAndPassive(t *testing.T) {
	agg := &SubsAggregator{health: fakeSnapshotter{snap: map[string]subprobe.Health{
		"jimaku":        {Status: subprobe.StatusDegraded, LatencyMS: 4200, CheckedAt: time.Now()},
		"opensubtitles": {Status: subprobe.StatusUp, CheckedAt: time.Now()},
	}}}
	resp := &AggregateResponse{ProvidersDown: []string{"opensubtitles"}} // passive failure this request
	agg.overlayHealth(resp)

	got := map[string]string{}
	for _, h := range resp.ProviderHealth {
		got[h.Provider] = h.Status
	}
	// jimaku: probe degraded → surfaced as degraded.
	if got["jimaku"] != "degraded" {
		t.Errorf("jimaku = %q; want degraded", got["jimaku"])
	}
	// opensubtitles: probe up BUT passive down → worse wins → down.
	if got["opensubtitles"] != "down" {
		t.Errorf("opensubtitles = %q; want down (worse-wins)", got["opensubtitles"])
	}
}

func TestOverlayHealth_OmitsUpAndUnknown(t *testing.T) {
	agg := &SubsAggregator{health: fakeSnapshotter{snap: map[string]subprobe.Health{
		"jimaku":        {Status: subprobe.StatusUp, CheckedAt: time.Now()},
		"opensubtitles": {Status: subprobe.StatusUnknown, CheckedAt: time.Now()},
	}}}
	resp := &AggregateResponse{}
	agg.overlayHealth(resp)
	if len(resp.ProviderHealth) != 0 {
		t.Fatalf("up/unknown must not surface; got %+v", resp.ProviderHealth)
	}
}

func TestOverlayHealth_SortedAndNilSafe(t *testing.T) {
	agg := &SubsAggregator{} // nil health
	resp := &AggregateResponse{}
	agg.overlayHealth(resp) // must not panic
	if resp.ProviderHealth != nil {
		t.Fatal("nil health must leave ProviderHealth nil")
	}

	agg.health = fakeSnapshotter{snap: map[string]subprobe.Health{
		"opensubtitles": {Status: subprobe.StatusDown, CheckedAt: time.Now()},
		"jimaku":        {Status: subprobe.StatusDown, CheckedAt: time.Now()},
	}}
	resp = &AggregateResponse{}
	agg.overlayHealth(resp)
	names := []string{resp.ProviderHealth[0].Provider, resp.ProviderHealth[1].Provider}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("ProviderHealth not sorted: %v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/service/ -run OverlayHealth -v`
Expected: FAIL — `health` field / `overlayHealth` / `ProviderHealth` undefined.

- [ ] **Step 3: Add the type, field, interface, and overlay**

In `services/catalog/internal/service/subs_aggregator.go`:

(a) Add the subprobe import to the import block:

```go
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/subprobe"
```

(b) Add the `health` field to the `SubsAggregator` struct (after `cache`):

```go
	health    HealthSnapshotter
```

(c) Add the interface + type near `AggregateResponse`:

```go
// HealthSnapshotter exposes the active subtitle probe's latest per-provider
// verdict. Satisfied by *subprobe.Store. Nil-safe: a nil snapshotter overlays
// nothing.
type HealthSnapshotter interface {
	Snapshot() map[string]subprobe.Health
}

// ProviderHealth is one provider's surfaced health on the response. Only
// degraded/down providers are included (the FE shows a note for those only).
type ProviderHealth struct {
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
}
```

(d) Add `ProviderHealth` to `AggregateResponse`:

```go
	ProviderHealth []ProviderHealth `json:"provider_health,omitempty"`
```

(e) Update `NewSubsAggregator` to accept + store the snapshotter. Change the signature to add `health HealthSnapshotter,` before `log *logger.Logger,` and set `health: health,` in the returned struct.

(f) Add the overlay method (worse-wins merge; only degraded/down surface):

```go
// overlayHealth injects the active probe's verdict (merged with this request's
// passive ProvidersDown) onto the response. It is called AFTER the Redis cache
// get/set so health is always fresh and never frozen into a cached body. Only
// degraded/down providers are surfaced (the FE shows a note for those only);
// "down" (probe or passive) always wins over "degraded"/"up".
func (s *SubsAggregator) overlayHealth(resp *AggregateResponse) {
	if s.health == nil {
		return
	}
	snap := s.health.Snapshot()
	passiveDown := map[string]bool{}
	for _, p := range resp.ProvidersDown {
		passiveDown[p] = true
	}
	providers := map[string]bool{}
	for p := range snap {
		providers[p] = true
	}
	for p := range passiveDown {
		providers[p] = true
	}
	out := make([]ProviderHealth, 0, len(providers))
	for p := range providers {
		st := subprobe.StatusUnknown
		var lat int64
		if h, ok := snap[p]; ok {
			st = h.Status
			lat = h.LatencyMS
		}
		if passiveDown[p] {
			st = subprobe.StatusDown // worse-wins: a live failure this request beats any milder probe verdict
		}
		if st == subprobe.StatusDegraded || st == subprobe.StatusDown {
			out = append(out, ProviderHealth{Provider: p, Status: string(st), LatencyMS: lat})
		}
	}
	if len(out) == 0 {
		return
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	resp.ProviderHealth = out
}
```

(`sort` is already imported in subs_aggregator.go.)

(g) Call the overlay on BOTH return paths in `FetchAll`. Cache-hit path (after `if err := s.cache.Get(...); err == nil {`):

```go
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		s.overlayHealth(&cached)
		return &cached, nil
	}
```

Fresh path — AFTER the `s.cache.Set(...)` line (so the cached bytes never include health), before `return resp, nil`:

```go
	dedupe(resp.Languages)
	_ = s.cache.Set(ctx, cacheKey, resp, subsCacheTTL(resp))
	s.overlayHealth(resp)
	return resp, nil
```

- [ ] **Step 4: Update the constructor call site (so the package compiles)**

In `services/catalog/cmd/catalog-api/main.go`, the `NewSubsAggregator(...)` call gets its new arg in Task 6. For now, run the targeted test which only constructs `&SubsAggregator{...}` directly — it does not need main.go to compile. Run:

Run: `go test ./services/catalog/internal/service/ -run OverlayHealth -v`
Expected: PASS (3 tests). (The full `catalog-api` build is fixed in Task 6.)

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subs_aggregator.go services/catalog/internal/service/subs_aggregator_health_test.go
git commit -m "feat(catalog): overlay fresh provider_health on subs response (worse-wins merge)"
```

---

## Task 6: Internal handler + route + main.go wiring

**Files:**
- Create: `services/catalog/internal/handler/internal_subtitle_probe.go`
- Test: `services/catalog/internal/handler/internal_subtitle_probe_test.go`
- Modify: `services/catalog/internal/transport/router.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`

**Interfaces:**
- Consumes: `subprobe.New`, `subprobe.NewStore` (Tasks 3–4), `NewSubsAggregator(...health...)` (Task 5), `*jimaku.Client`/`*opensubtitles.Client` Ping (Task 2).
- Produces:
  - `handler.NewInternalSubtitleProbeHandler(runner SubtitleProbeRunner, log *logger.Logger) *InternalSubtitleProbeHandler`.
  - `handler.SubtitleProbeRunner` interface: `RunOnce(ctx context.Context)` — satisfied by `*subprobe.Probe`.
  - `(*InternalSubtitleProbeHandler).Run(w, r)` — runs the probe (on `context.Background()`, not the request ctx, so a client disconnect can't abort mid-run), writes `204`.
  - `NewRouter(..., internalSubtitleProbeHandler *handler.InternalSubtitleProbeHandler, ...)` — new param.

- [ ] **Step 1: Write the failing handler test**

Create `services/catalog/internal/handler/internal_subtitle_probe_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fakeRunner struct{ runs int32 }

func (f *fakeRunner) RunOnce(ctx context.Context) { atomic.AddInt32(&f.runs, 1) }

func TestInternalSubtitleProbe_Run204(t *testing.T) {
	r := &fakeRunner{}
	h := NewInternalSubtitleProbeHandler(r, nil)
	rec := httptest.NewRecorder()
	h.Run(rec, httptest.NewRequest(http.MethodPost, "/internal/subtitle-probe/run", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", rec.Code)
	}
	if atomic.LoadInt32(&r.runs) != 1 {
		t.Fatalf("RunOnce called %d times; want 1", r.runs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/handler/ -run InternalSubtitleProbe -v`
Expected: FAIL — `NewInternalSubtitleProbeHandler` undefined.

- [ ] **Step 3: Implement the handler**

Create `services/catalog/internal/handler/internal_subtitle_probe.go`:

```go
package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// SubtitleProbeRunner runs one active subtitle probe sweep. Satisfied by
// *subprobe.Probe.
type SubtitleProbeRunner interface {
	RunOnce(ctx context.Context)
}

// InternalSubtitleProbeHandler exposes POST /internal/subtitle-probe/run. The
// scheduler fires it on a 5-min cron. Docker-network-only (the gateway does not
// proxy /internal/*), so no auth middleware.
type InternalSubtitleProbeHandler struct {
	runner SubtitleProbeRunner
	log    *logger.Logger
}

func NewInternalSubtitleProbeHandler(runner SubtitleProbeRunner, log *logger.Logger) *InternalSubtitleProbeHandler {
	return &InternalSubtitleProbeHandler{runner: runner, log: log}
}

// Run executes one probe sweep synchronously and returns 204. It uses a fresh
// background context (NOT the request ctx) so a client disconnect can't abort
// the sweep mid-write — the same lesson as the playback probe handler.
func (h *InternalSubtitleProbeHandler) Run(w http.ResponseWriter, r *http.Request) {
	h.runner.RunOnce(context.Background())
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/catalog/internal/handler/ -run InternalSubtitleProbe -v`
Expected: PASS.

- [ ] **Step 5: Add the route**

In `services/catalog/internal/transport/router.go`:

(a) Add the param to `NewRouter` (place it right after `internalProbeHandler *handler.InternalProbeHandler,`):

```go
	internalSubtitleProbeHandler *handler.InternalSubtitleProbeHandler,
```

(b) Register the route in the internal-endpoints block (after the `internalProbeHandler` block, ~line 99):

```go
	if internalSubtitleProbeHandler != nil {
		r.Post("/internal/subtitle-probe/run", internalSubtitleProbeHandler.Run)
	}
```

- [ ] **Step 6: Wire main.go**

In `services/catalog/cmd/catalog-api/main.go`, between the `openSubsClient := opensubtitles.NewClient(...)` block (~line 423) and `subsAggregator := service.NewSubsAggregator(...)` (~line 434), insert:

```go
	// Active subtitle-provider probe (subprobe): pings the configured providers
	// on a scheduler-fired cron and records up/degraded/down verdicts the
	// aggregator overlays as provider_health. staleAfter = 15m (3× the 5-min
	// cron) so a missed run downgrades to "unknown" rather than a stale "up".
	subHealthStore := subprobe.NewStore(15 * time.Minute)
	subPingers := map[string]subprobe.Pinger{}
	if jimakuClient.IsConfigured() {
		subPingers["jimaku"] = jimakuClient
	}
	if openSubsClient.IsConfigured() {
		subPingers["opensubtitles"] = openSubsClient
	}
	subtitleProbe := subprobe.New(subHealthStore, subPingers, 2*time.Second, 8*time.Second, log)
	internalSubtitleProbeHandler := handler.NewInternalSubtitleProbeHandler(subtitleProbe, log)
```

Change the `subsAggregator` construction to pass the store:

```go
	subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, idMapClient, animeRepo, redisCache, subHealthStore, log)
```

Add `internalSubtitleProbeHandler` to the `transport.NewRouter(...)` call, in the same position as the router param (right after `internalProbeHandler,`).

Add the imports if not present (check the import block):

```go
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/subprobe"
	// "time" is already imported
```

- [ ] **Step 7: Build + test the whole catalog service**

Run: `go build ./services/catalog/... && go test ./services/catalog/... -count=1`
Expected: build OK; tests PASS.

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/handler/internal_subtitle_probe.go services/catalog/internal/handler/internal_subtitle_probe_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): /internal/subtitle-probe/run + wire probe/store into aggregator"
```

---

## Task 7: Scheduler trigger job

**Files:**
- Create: `services/scheduler/internal/jobs/subtitle_probe_trigger.go`
- Test: `services/scheduler/internal/jobs/subtitle_probe_trigger_test.go`
- Modify: `services/scheduler/internal/config/config.go`
- Modify: `services/scheduler/internal/service/job.go`
- Modify: `services/scheduler/cmd/scheduler-api/main.go`

**Interfaces:**
- Consumes: catalog `POST /internal/subtitle-probe/run` (Task 6), `JobsConfig.CatalogServiceURL` (existing).
- Produces: `jobs.NewSubtitleProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *SubtitleProbeTriggerJob` with `Run(ctx) error`; `JobsConfig.SubtitleProbeCron` (env `SUBTITLE_PROBE_CRON`, default `*/5 * * * *`).

- [ ] **Step 1: Write the failing test**

Create `services/scheduler/internal/jobs/subtitle_probe_trigger_test.go`:

```go
package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

func TestSubtitleProbeTrigger_PostsCatalog(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	j := NewSubtitleProbeTriggerJob(&config.JobsConfig{CatalogServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/internal/subtitle-probe/run" {
		t.Fatalf("path = %q; want /internal/subtitle-probe/run", gotPath)
	}
}

func TestSubtitleProbeTrigger_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j := NewSubtitleProbeTriggerJob(&config.JobsConfig{CatalogServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/scheduler/internal/jobs/ -run SubtitleProbeTrigger -v`
Expected: FAIL — `NewSubtitleProbeTriggerJob` undefined.

- [ ] **Step 3: Implement the job**

Create `services/scheduler/internal/jobs/subtitle_probe_trigger.go`:

```go
// Package jobs — subtitle_probe_trigger.go fires the active subtitle-provider
// health probe. The probe lives in catalog (it owns the Jimaku/OpenSubtitles
// clients + API keys), so this job POSTs catalog's /internal/subtitle-probe/run
// on a 5-min cron. Mirrors probe_trigger.go (same HTTP-trigger pattern), but
// targets catalog instead of analytics.
package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

// subtitleProbeReqTimeout caps the POST. The probe is two cheap HTTP pings
// (8s budget each), so 30s is comfortable.
const subtitleProbeReqTimeout = 30 * time.Second

// SubtitleProbeTriggerJob triggers catalog's active subtitle health probe.
type SubtitleProbeTriggerJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewSubtitleProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *SubtitleProbeTriggerJob {
	return &SubtitleProbeTriggerJob{
		client: &http.Client{Timeout: subtitleProbeReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs catalog's /internal/subtitle-probe/run. A non-2xx or transport error
// is returned so the JobService metrics wrapper records a failure; a single
// missed run is tolerated (health carries CheckedAt → stale downgrades to unknown).
func (j *SubtitleProbeTriggerJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting subtitle-health probe trigger")
	}
	url := j.config.CatalogServiceURL + "/internal/subtitle-probe/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build subtitle probe request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post subtitle probe: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("subtitle probe returned status %d", resp.StatusCode)
	}
	if j.log != nil {
		j.log.Info("subtitle-health probe trigger completed")
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/scheduler/internal/jobs/ -run SubtitleProbeTrigger -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Add the cron config**

In `services/scheduler/internal/config/config.go`, add a field to `JobsConfig` (after `ProviderRankingCron string`):

```go
	// Active subtitle-provider health probe trigger. SubtitleProbeCron: cron for
	// the probe (default `*/5 * * * *`, every 5 min — Jimaku/OpenSubtitles go down
	// intermittently and the passive metrics go blind with no traffic, so a tight
	// active cadence makes a 4am outage visible within minutes). The scheduler
	// POSTs catalog's /internal/subtitle-probe/run (reuses CatalogServiceURL).
	SubtitleProbeCron string
```

And in `Load()` (after the `ProviderRankingCron:` line):

```go
			SubtitleProbeCron: getEnv("SUBTITLE_PROBE_CRON", "*/5 * * * *"),
```

- [ ] **Step 6: Register the job in JobService**

In `services/scheduler/internal/service/job.go`:

(a) Struct field (after `providerRankingJob`):

```go
	subtitleProbeJob           *jobs.SubtitleProbeTriggerJob
```

(b) `lastSubtitleProbeRun time.Time` (after `lastProviderRankingRun`).

(c) `NewJobService` param (after `providerRankingJob *jobs.ProviderRankingJob,`):

```go
	subtitleProbeJob *jobs.SubtitleProbeTriggerJob,
```

and assignment in the struct literal:

```go
		subtitleProbeJob:       subtitleProbeJob,
```

(d) `Start` param: add `subtitleProbeCron string` to the signature (after `providerRankingCron`).

(e) Register block (after the provider-ranking block, before the autocache Logic A block):

```go
	// Schedule the active subtitle-provider health probe (every 5 min). The probe
	// lives in catalog (owns the Jimaku/OpenSubtitles clients + keys), so this job
	// just POSTs catalog's /internal/subtitle-probe/run. Nil-guarded for symmetry.
	if s.subtitleProbeJob != nil {
		_, err = s.cron.AddFunc(subtitleProbeCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled subtitle-health probe")
			start := time.Now()
			if err := s.subtitleProbeJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
				s.log.Errorw("subtitle-health probe failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("subtitle_probe").SetToCurrentTime()
				s.lastSubtitleProbeRun = time.Now()
				s.log.Info("subtitle-health probe completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: subtitle_probe")
	}
```

(f) `GetStatus` entry (after `provider_ranking_recompute`):

```go
		"subtitle_probe": map[string]interface{}{
			"last_run": s.lastSubtitleProbeRun,
		},
```

- [ ] **Step 7: Wire scheduler main.go**

In `services/scheduler/cmd/scheduler-api/main.go`:

(a) Construct the job (after `providerRankingJob := jobs.NewProviderRankingJob(...)`):

```go
	// Active subtitle-provider health probe trigger (every 5 min).
	subtitleProbeJob := jobs.NewSubtitleProbeTriggerJob(&cfg.Jobs, log)
```

(b) Add `subtitleProbeJob` to the `service.NewJobService(...)` call (after `providerRankingJob,`).

(c) Add `cfg.Jobs.SubtitleProbeCron,` to the `jobService.Start(...)` call (after `cfg.Jobs.ProviderRankingCron,`).

- [ ] **Step 8: Build + test scheduler**

Run: `go build ./services/scheduler/... && go test ./services/scheduler/... -count=1`
Expected: build OK; tests PASS.

- [ ] **Step 9: Commit**

```bash
git add services/scheduler/
git commit -m "feat(scheduler): 5-min subtitle-health probe trigger (SUBTITLE_PROBE_CRON)"
```

---

## Task 8: Frontend merged degraded note

**Files:**
- Modify: `frontend/web/src/types/raw.ts`
- Modify: `frontend/web/src/components/player/OtherSubsPanel.vue`
- Modify: `frontend/web/src/components/player/OtherSubsPanel.spec.ts`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Consumes: `GroupedSubs.provider_health` (new) + existing `providers_down`.
- Produces: `ProviderHealth` TS interface; a single merged note keyed by `player.otherSubs.providerIssues` + `player.otherSubs.status.{degraded,down}`.

- [ ] **Step 1: Add the type**

In `frontend/web/src/types/raw.ts`, add after the `GroupedSubs` interface:

```ts
export interface ProviderHealth {
  provider: string
  status: 'degraded' | 'down'
  latency_ms?: number
}
```

and add the field inside `GroupedSubs`:

```ts
  provider_health?: ProviderHealth[]
```

- [ ] **Step 2: Write the failing FE test**

Add to `frontend/web/src/components/player/OtherSubsPanel.spec.ts` (follow the file's existing mount/fetch helpers; this assumes the existing pattern of mounting with stubbed `subtitlesApi.all`). Add these cases:

```ts
it('shows a merged degraded note from provider_health', async () => {
  const wrapper = await mountWithData({
    languages: {}, episode: 1,
    provider_health: [{ provider: 'jimaku', status: 'degraded' }],
  })
  expect(wrapper.text()).toContain('Jimaku')
  // i18n: "Some subtitle sources may be unavailable: ..."
  expect(wrapper.text().toLowerCase()).toContain('jimaku')
})

it('merges provider_health (down) with providers_down without duplication', async () => {
  const wrapper = await mountWithData({
    languages: {}, episode: 1,
    provider_health: [{ provider: 'opensubtitles', status: 'down' }],
    providers_down: ['opensubtitles'],
  })
  const matches = wrapper.text().match(/OpenSubtitles/g) ?? []
  expect(matches.length).toBe(1)
})

it('shows no provider-issue note when all healthy', async () => {
  const wrapper = await mountWithData({ languages: {}, episode: 1 })
  expect(wrapper.find('[data-testid="provider-issues"]').exists()).toBe(false)
})
```

> NOTE for the implementer: open `OtherSubsPanel.spec.ts` first and reuse its existing
> mount/stub helper. If the helper is not named `mountWithData`, adapt these three cases
> to the file's actual helper (the assertions on text + the `data-testid` are what matter).

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/OtherSubsPanel.spec.ts`
Expected: FAIL (note/testid not rendered yet).

- [ ] **Step 4: Implement the merged note in the SFC**

In `frontend/web/src/components/player/OtherSubsPanel.vue`:

(a) Update the import of types to include `ProviderHealth`:

```ts
import type { GroupedSubs, SubtitleTrack, ProviderHealth } from '@/types/raw'
```

(b) Replace the `providersDown` computed (line ~218) with a merged `providerIssues` computed + a localized text helper:

```ts
// Merged provider-issue list: the active probe's verdict (provider_health,
// degraded/down) unioned with this request's passive providers_down. Backend
// already applies worse-wins; the union here is belt-and-suspenders so a
// providers_down entry the probe didn't cover still surfaces exactly once.
const providerIssues = computed<ProviderHealth[]>(() => {
  const out: ProviderHealth[] = []
  const seen = new Set<string>()
  for (const h of data.value?.provider_health ?? []) {
    out.push(h)
    seen.add(h.provider)
  }
  for (const p of data.value?.providers_down ?? []) {
    if (!seen.has(p)) {
      out.push({ provider: p, status: 'down' })
      seen.add(p)
    }
  }
  return out
})

const providerIssuesText = computed(() =>
  providerIssues.value
    .map((i) => `${providerLabel(i.provider)} (${t(`player.otherSubs.status.${i.status}`)})`)
    .join(', '),
)
```

(c) Replace the note `<p>` (lines ~115-117) with:

```html
      <p
        v-if="providerIssues.length > 0"
        data-testid="provider-issues"
        class="text-warning/80 text-xs text-center pt-2 border-t border-white/10"
      >
        {{ $t('player.otherSubs.providerIssues', { issues: providerIssuesText }) }}
      </p>
```

(`providerLabel` already exists at ~line 228; `t` is already destructured from `useI18n()` at line 144.)

- [ ] **Step 5: Add i18n keys (all three locales)**

In each of `en.json`, `ru.json`, `ja.json`, inside `player.otherSubs`, add the `providerIssues` message and a `status` sub-object. Keep the existing `providersDown` key (harmless; no longer referenced).

`en.json`:
```json
    "providerIssues": "Some subtitle sources may be unavailable: {issues}",
    "status": { "degraded": "degraded", "down": "down" },
```

`ru.json`:
```json
    "providerIssues": "Некоторые источники субтитров могут быть недоступны: {issues}",
    "status": { "degraded": "сбои", "down": "недоступен" },
```

`ja.json`:
```json
    "providerIssues": "一部の字幕ソースが利用できない可能性があります: {issues}",
    "status": { "degraded": "不安定", "down": "停止中" },
```

- [ ] **Step 6: Run tests + typecheck + lint**

Run:
```bash
cd frontend/web
bunx vitest run src/components/player/OtherSubsPanel.spec.ts src/locales/__tests__/
bunx tsc --noEmit
bash scripts/design-system-lint.sh
```
Expected: vitest PASS (incl. locale parity), tsc clean, DS lint `ERRORS=0`.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/types/raw.ts frontend/web/src/components/player/OtherSubsPanel.vue frontend/web/src/components/player/OtherSubsPanel.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(web): merged subtitle provider degraded/down note from provider_health"
```

---

## Task 9: Extend the Grafana dashboard

**Files:**
- Modify: `docker/grafana/dashboards/subtitle-health.json`

**Interfaces:**
- Consumes: `probe_subtitle_provider_up`, `probe_subtitle_latency_seconds`, `probe_subtitle_last_run_timestamp` (Task 1).

- [ ] **Step 1: Add the active-probe panels**

Open `docker/grafana/dashboards/subtitle-health.json`. Find the `"panels": [ ... ]` array and the highest existing panel `id` + bottom `y`. Append three panels below the existing ones (use the next free `id`s and a `y` below the last row — the existing passive panels occupy roughly `y: 0`, `7`, `14`; place these at `y: 21` and `28`). Each panel mirrors the existing `timeseries`/`stat` panel shape (datasource `${DS_PROMETHEUS}`):

```json
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": { "defaults": { "color": { "mode": "thresholds" }, "mappings": [], "max": 1, "min": 0, "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "yellow", "value": 0.5 }, { "color": "green", "value": 1 } ] } }, "overrides": [] },
      "gridPos": { "h": 7, "w": 12, "x": 0, "y": 21 },
      "id": 7,
      "options": { "legend": { "displayMode": "list", "placement": "bottom" } },
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "probe_subtitle_provider_up", "legendFormat": "{{provider}}", "refId": "A" } ],
      "title": "Active Probe — Per-Provider Status (1 up / 0.5 degraded / 0 down)",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": { "defaults": { "color": { "mode": "palette-classic" }, "custom": {}, "mappings": [], "unit": "s" }, "overrides": [] },
      "gridPos": { "h": 7, "w": 12, "x": 12, "y": 21 },
      "id": 8,
      "options": { "legend": { "displayMode": "list", "placement": "bottom" } },
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "probe_subtitle_latency_seconds", "legendFormat": "{{provider}}", "refId": "A" } ],
      "title": "Active Probe — Ping Latency",
      "type": "timeseries"
    },
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "fieldConfig": { "defaults": { "color": { "mode": "thresholds" }, "mappings": [], "unit": "dateTimeFromNow", "thresholds": { "mode": "absolute", "steps": [ { "color": "green", "value": null } ] } }, "overrides": [] },
      "gridPos": { "h": 7, "w": 12, "x": 0, "y": 28 },
      "id": 9,
      "options": { "colorMode": "value", "graphMode": "none", "justifyMode": "auto", "orientation": "auto", "reduceOptions": { "calcs": [ "lastNotNull" ], "fields": "", "values": false }, "textMode": "auto" },
      "targets": [ { "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "probe_subtitle_last_run_timestamp * 1000", "refId": "A" } ],
      "title": "Active Probe — Last Run",
      "type": "stat"
    }
```

> If any of those `id` values (7/8/9) collide with an existing panel, bump all three to the next free ids. Keep commas valid (comma before each new object since they follow existing panels).

- [ ] **Step 2: Validate JSON**

Run: `cd /data/animeenigma/.claude/worktrees/subtitle-active-probe && python3 -m json.tool docker/grafana/dashboards/subtitle-health.json > /dev/null && echo "valid JSON"`
Expected: `valid JSON`.

- [ ] **Step 3: Commit**

```bash
git add docker/grafana/dashboards/subtitle-health.json
git commit -m "feat(grafana): active subtitle-probe panels on subtitle-health dashboard"
```

---

## Task 10: Compose env + docs

**Files:**
- Modify: `docker/docker-compose.yml` (scheduler service env)
- Modify: `CLAUDE.md` (scheduler env docs)

**Interfaces:**
- Consumes: `SUBTITLE_PROBE_CRON` (Task 7).

- [ ] **Step 1: Add the scheduler env var**

In `docker/docker-compose.yml`, find the `scheduler:` service `environment:` block (it already lists `PLAYBACK_PROBE_CRON` / `PROVIDER_RANKING_CRON` if set there; otherwise the defaults come from config). Add (if the block sets cron vars explicitly; if it relies on config defaults, add it for visibility):

```yaml
      SUBTITLE_PROBE_CRON: ${SUBTITLE_PROBE_CRON:-*/5 * * * *}
```

> If the scheduler service env block does NOT explicitly set the other `*_CRON` vars (they default in config.go), still add this line so the cadence is discoverable/overridable from compose. Place it next to the other scheduler probe/job vars.

- [ ] **Step 2: Document the env var in CLAUDE.md**

In `CLAUDE.md`, in the scheduler-related env section (near the existing job/probe descriptions), add a short note:

```
SUBTITLE_PROBE_CRON   # scheduler: active subtitle-provider health probe cadence
                      # (default */5 * * * *). Scheduler POSTs catalog's
                      # /internal/subtitle-probe/run; catalog pings Jimaku +
                      # OpenSubtitles (cheap non-quota endpoints), records
                      # up/degraded/down + latency → probe_subtitle_* gauges +
                      # provider_health overlay on /subtitles/all.
```

- [ ] **Step 3: Commit**

```bash
git add docker/docker-compose.yml CLAUDE.md
git commit -m "chore: document + wire SUBTITLE_PROBE_CRON scheduler env"
```

---

## Task 11: Full-stack verification gate (forced failure)

**Files:** none (verification only — this is the owner's "cheapest next step" proof).

- [ ] **Step 1: Build everything changed**

Run: `cd /data/animeenigma/.claude/worktrees/subtitle-active-probe && go build ./... && go vet ./libs/metrics/... ./services/catalog/... ./services/scheduler/...`
Expected: clean.

- [ ] **Step 2: Full Go test sweep for touched packages**

Run: `go test ./libs/metrics/... ./services/catalog/... ./services/scheduler/... -count=1`
Expected: PASS.

- [ ] **Step 3: Frontend gate**

Run: `cd frontend/web && bunx vitest run src/components/player/OtherSubsPanel.spec.ts src/locales/__tests__/ && bunx tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: PASS / clean / `ERRORS=0`.

- [ ] **Step 4: Deploy + forced-failure proof (after merge to main, via after-update)**

This is the owner's explicit gate and is executed during `/animeenigma-after-update`
(redeploy catalog + scheduler), THEN:

```bash
# Healthy run: trigger the probe and read the gauge.
docker exec animeenigma-catalog wget -qO- --post-data='' http://localhost:8081/internal/subtitle-probe/run ; echo "(204 = ok)"
curl -s http://localhost:8081/metrics | grep probe_subtitle_provider_up
# Expect: probe_subtitle_provider_up{provider="jimaku"} 1  (and opensubtitles 1)

# Forced failure: temporarily set a bad Jimaku key (host .env override only — git-ignored),
# restart catalog, trigger again, and confirm the gauge flips to 0:
#   (edit .env: JIMAKU_API_KEY=deliberately-bad)  ->  make restart-catalog
docker exec animeenigma-catalog wget -qO- --post-data='' http://localhost:8081/internal/subtitle-probe/run
curl -s http://localhost:8081/metrics | grep 'probe_subtitle_provider_up{provider="jimaku"}'
# Expect: probe_subtitle_provider_up{provider="jimaku"} 0
#   then restore the real key + make restart-catalog and confirm it flips back to 1.
```

Expected: gauge reads `1` healthy, `0` on forced bad key, `1` again after restore.
Record the observed values in the feedback report / changelog as proof.

- [ ] **Step 5: No commit (verification only).** Proceed to push + `/animeenigma-after-update`.

---

## Self-Review

**Spec coverage:**
- Active probe in catalog → Tasks 3,4,6. ✓
- Cheapest non-quota endpoints (Jimaku search anilist_id=1, OpenSubtitles /infos/formats) → Task 2. ✓
- up/degraded/down classification + staleness→unknown → Tasks 3,4. ✓
- 5-min scheduler trigger reusing CatalogServiceURL → Task 7. ✓
- `probe_subtitle_*` gauges distinct from passive → Task 1. ✓
- provider_health overlaid fresh (never cached), worse-wins merge → Task 5. ✓
- Unconfigured provider skipped (not "down") → Task 6 Step 6 (only configured pingers added). ✓
- FE one merged note + i18n en/ru/ja → Task 8. ✓
- Extend existing dashboard → Task 9. ✓
- Env + docs → Task 10. ✓
- Forced-failure verification gate → Task 11. ✓

**Placeholder scan:** No TBD/TODO; every code step has full code. The two NOTE callouts (Task 8 spec helper name, Task 9 panel-id collision) are conditional guidance, not missing content.

**Type consistency:** `subprobe.Status`/`Health`/`Store`/`Pinger`/`New`/`NewStore`/`RunOnce` consistent across Tasks 3–6. `HealthSnapshotter.Snapshot() map[string]subprobe.Health` matches `*Store.Snapshot()`. `SubtitleProbeRunner.RunOnce(ctx)` matches `*Probe.RunOnce(ctx)`. `ProviderHealth` JSON (`provider`/`status`/`latency_ms`) matches the TS `ProviderHealth` interface in Task 8. Scheduler `CatalogServiceURL` reused (confirmed in config.go). ✓
