# PlatformStats Joke Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the `platform_stats` spotlight card into a bombastic "Trump-style" status card — a grounded-health hero (Работает/Аптайм from Prometheus + canned UXΔ/CDI/MVQ on a daily-random service) plus a 2×2 grid of 4 random non-zero Prometheus metrics over random day/week/all-time windows.

**Architecture:** The catalog `PlatformStatsResolver` queries Prometheus (new tiny client) for real health + tile values, and draws joke copy from two `go:embed`-ed JSON pools (composed ~80/10/10 RU/EN/JP). A date-seeded RNG makes every pick stable for the UTC day and reproducible in tests; the assembled payload is cached once per day under `spotlight:stats:<DateKeyUTC>`. Everyone sees the same daily pick verbatim — no i18n, no locale switching. The card stays the `platform_stats` discriminator with a fully replaced payload (`hero` + `tiles`).

**Tech Stack:** Go (GORM-free for this card), `net/http` Prometheus client, `go:embed`, Vue 3 `<script setup>` + Tailwind, Vitest, Go testing with handwritten fakes (no testify/mock — project rule).

**Spec:** `docs/superpowers/specs/2026-05-25-platform-stats-joke-card-design.md`

**Refinement over spec:** `StatsTile.Value` is `float64` (TS `number`), not `int64` — preserves sub-second values for future `seconds`-format tiles. Tile PromQL is `sum(...)`-wrapped so each tile is one aggregate number across all service instances.

---

## File Structure

**New:**
- `services/catalog/internal/parser/prometheus/client.go` — instant-query + health client
- `services/catalog/internal/parser/prometheus/client_test.go`
- `services/catalog/internal/service/spotlight/cards/platform_stats_jokes.json` — hero joke pool
- `services/catalog/internal/service/spotlight/cards/platform_stats_prom.json` — tile query allowlist
- `services/catalog/internal/service/spotlight/cards/platform_stats_pools.go` — embed + structs + loader
- `services/catalog/internal/service/spotlight/cards/platform_stats_pools_test.go`

**Rewritten:**
- `services/catalog/internal/service/spotlight/types.go` — `PlatformStatsData`/`StatsHero`/`StatsTile` (drop `StatsMetric`)
- `services/catalog/internal/service/spotlight/types_test.go` — relevant cases
- `services/catalog/internal/service/spotlight/cards/platform_stats.go` — resolver
- `services/catalog/internal/service/spotlight/cards/platform_stats_test.go`
- `frontend/web/src/types/spotlight.ts` — `PlatformStatsData` variant (drop `PlatformMetric`)
- `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue`
- `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts`

**Edited:**
- `services/catalog/internal/config/config.go` — `Prometheus` config + env
- `services/catalog/cmd/catalog-api/main.go` — DI
- `docker/docker-compose.yml` — catalog `PROMETHEUS_SERVICE_URL`
- `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` — remove `spotlight.platformStats`
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` — `cardTitle()` literal
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` — drop `platformStats` assertions

---

## Task 1: Backend payload types

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go` (replace `StatsMetric` + `PlatformStatsData`, lines 59-78)
- Test: `services/catalog/internal/service/spotlight/types_test.go`

- [ ] **Step 1: Write the failing test**

Add to `types_test.go`:

```go
func TestPlatformStatsData_RoundTrip(t *testing.T) {
	pct := 99.4
	in := PlatformStatsData{
		Hero: StatsHero{
			WorkingOK:     true,
			UptimePercent: &pct,
			UptimeQuip:    "ОЧЕНЬ МНОГО",
			Service:       "catalog",
			UXDelta:       "+5 (Tremendous)",
			CDI:           "0.00 * 99",
			MVQ:           "Dragon 99%/99%",
			Tagline:       "Лучшая платформа. Поверьте.",
		},
		Tiles: []StatsTile{
			{Label: "Запросов обработано", Value: 48201, Window: "day", Format: "int"},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out PlatformStatsData
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Hero.UptimeQuip != "ОЧЕНЬ МНОГО" || out.Hero.UptimePercent == nil || *out.Hero.UptimePercent != 99.4 {
		t.Fatalf("hero round-trip mismatch: %+v", out.Hero)
	}
	if len(out.Tiles) != 1 || out.Tiles[0].Value != 48201 || out.Tiles[0].Window != "day" {
		t.Fatalf("tiles round-trip mismatch: %+v", out.Tiles)
	}
}

func TestPlatformStatsData_EmptyTilesMarshalArray(t *testing.T) {
	b, err := json.Marshal(PlatformStatsData{Hero: StatsHero{}, Tiles: []StatsTile{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tiles":[]`) {
		t.Fatalf("expected tiles:[] in %s", b)
	}
}

func TestStatsHero_UptimePercentOmittedWhenNil(t *testing.T) {
	b, err := json.Marshal(StatsHero{WorkingOK: false, UptimeQuip: "x", Service: "s", Tagline: "t"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "uptime_percent") {
		t.Fatalf("uptime_percent should be omitted when nil: %s", b)
	}
}
```

Ensure the test file imports `"encoding/json"` and `"strings"` (add if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestPlatformStatsData -count=1`
Expected: FAIL — compile error (`StatsHero`/`StatsTile` undefined) or old `StatsMetric` mismatch.

- [ ] **Step 3: Replace the types**

In `types.go`, delete the existing `StatsMetric` struct and `PlatformStatsData` struct (the block currently at lines ~59-78) and replace with:

```go
// StatsHero is the bombastic top line of the platform_stats card.
// WorkingOK + UptimePercent are REAL (from Prometheus); the remaining
// fields are canned joke content (single-language strings) drawn from the
// embedded pool. UptimePercent is a pointer so it omits when Prometheus is
// unreachable.
type StatsHero struct {
	WorkingOK     bool     `json:"working_ok"`
	UptimePercent *float64 `json:"uptime_percent,omitempty"`
	UptimeQuip    string   `json:"uptime_quip"`
	Service       string   `json:"service"`
	UXDelta       string   `json:"ux_delta"`
	CDI           string   `json:"cdi"`
	MVQ           string   `json:"mvq"`
	Tagline       string   `json:"tagline"`
}

// StatsTile is one micro-grid cell — a single aggregated Prometheus metric
// over one window. Value is non-zero (the resolver filters out <= 0).
type StatsTile struct {
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Window string  `json:"window"` // "day" | "week" | "all"
	Format string  `json:"format"` // "int" | "bytes" | "seconds"
}

// PlatformStatsData is the payload for Card{Type: "platform_stats"}.
// Tiles MUST be initialized as []StatsTile{} (never nil) so it marshals as
// [] not null (the frontend treats null as a parse failure).
type PlatformStatsData struct {
	Hero  StatsHero   `json:"hero"`
	Tiles []StatsTile `json:"tiles"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/spotlight/ -run TestPlatformStatsData -count=1`
Expected: PASS (the resolver in `cards/` will not compile yet — that's Task 4; this package-level test still builds because it's in package `spotlight`).

Note: if the `spotlight` package test target pulls in nothing from `cards/`, this passes standalone. If `go test ./internal/service/spotlight/` errors on unrelated existing `StatsMetric` references in `types_test.go`, delete those old cases now.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/spotlight/types.go services/catalog/internal/service/spotlight/types_test.go
git commit -m "feat(spotlight): platform_stats payload -> hero+tiles joke shape

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Prometheus client

**Files:**
- Create: `services/catalog/internal/parser/prometheus/client.go`
- Test: `services/catalog/internal/parser/prometheus/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func vectorJSON(value string) string {
	return fmt.Sprintf(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1716600000,%q]}]}}`, value)
}

func TestQuery_ParsesVectorValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, vectorJSON("48201.5"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got, err := c.Query(context.Background(), "sum(http_requests_total)")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got != 48201.5 {
		t.Fatalf("want 48201.5, got %v", got)
	}
}

func TestQuery_EmptyResultIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Query(context.Background(), "x"); err == nil {
		t.Fatal("expected error on empty result")
	}
}

func TestQuery_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Query(context.Background(), "x"); err == nil {
		t.Fatal("expected error on non-200")
	}
}

func TestHealth_AllUpAndUptime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		switch {
		case q == "count(up == 0) or vector(0)":
			fmt.Fprint(w, vectorJSON("0"))
		case q == "avg(avg_over_time(up[7d]))":
			fmt.Fprint(w, vectorJSON("0.994"))
		default:
			t.Errorf("unexpected query %q", q)
		}
	}))
	defer srv.Close()

	allUp, pct, err := NewClient(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if !allUp {
		t.Fatal("want allUp=true")
	}
	if pct < 99.3 || pct > 99.5 {
		t.Fatalf("want ~99.4, got %v", pct)
	}
}

func TestHealth_SomeDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == "count(up == 0) or vector(0)" {
			fmt.Fprint(w, vectorJSON("2"))
			return
		}
		fmt.Fprint(w, vectorJSON("0.8"))
	}))
	defer srv.Close()
	allUp, _, err := NewClient(srv.URL).Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if allUp {
		t.Fatal("want allUp=false when 2 targets down")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/prometheus/ -count=1`
Expected: FAIL — `NewClient`/`Query`/`Health` undefined.

- [ ] **Step 3: Write the client**

`client.go`:

```go
// Package prometheus is a minimal read-only client for the Prometheus
// instant-query HTTP API, used by the catalog spotlight platform_stats
// card. It intentionally supports only the "vector" result shape we emit
// (sum/count/avg aggregations always return a vector).
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client targets a Prometheus base URL such as
// "http://prometheus:9090/prometheus" (note the route-prefix). The instant
// query endpoint is "{base}/api/v1/query".
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client with an 800ms timeout to respect the spotlight
// aggregator's per-card deadline.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 800 * time.Millisecond},
	}
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value []json.RawMessage `json:"value"` // [ <ts>, "<value>" ]
		} `json:"result"`
	} `json:"data"`
}

// Query runs an instant PromQL query and returns the first sample value.
// Returns an error on transport failure, non-200, decode failure, or empty
// result.
func (c *Client) Query(ctx context.Context, promql string) (float64, error) {
	endpoint := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promql))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus: status %d", resp.StatusCode)
	}
	var qr queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return 0, err
	}
	if qr.Status != "success" || len(qr.Data.Result) == 0 {
		return 0, fmt.Errorf("prometheus: empty result for %q", promql)
	}
	v := qr.Data.Result[0].Value
	if len(v) != 2 {
		return 0, fmt.Errorf("prometheus: malformed value")
	}
	var s string
	if err := json.Unmarshal(v[1], &s); err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// Health reports whether all scrape targets are up and the 7-day average
// uptime percentage (0..100). Any query failure returns a non-nil error;
// callers treat that as "Prometheus unreachable".
func (c *Client) Health(ctx context.Context) (allUp bool, uptimePct float64, err error) {
	down, err := c.Query(ctx, "count(up == 0) or vector(0)")
	if err != nil {
		return false, 0, err
	}
	allUp = down == 0
	avg, err := c.Query(ctx, "avg(avg_over_time(up[7d]))")
	if err != nil {
		return allUp, 0, err
	}
	uptimePct = math.Max(0, math.Min(100, avg*100))
	return allUp, uptimePct, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/prometheus/ -count=1`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/prometheus/
git commit -m "feat(catalog): minimal Prometheus instant-query client

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Embedded joke + tile pools

**Files:**
- Create: `services/catalog/internal/service/spotlight/cards/platform_stats_jokes.json`
- Create: `services/catalog/internal/service/spotlight/cards/platform_stats_prom.json`
- Create: `services/catalog/internal/service/spotlight/cards/platform_stats_pools.go`
- Test: `services/catalog/internal/service/spotlight/cards/platform_stats_pools_test.go`

- [ ] **Step 1: Create the joke pool JSON**

`platform_stats_jokes.json` (composed ~80% RU / 10% EN / 10% JP; expand later by editing this file only):

```json
{
  "taglines": [
    "Лучшая платформа для аниме. Никто не стримит лучше нас. Поверьте.",
    "У нас самое большое аниме. Огромное. Невероятное аниме.",
    "Этот сервер работает. Сильно работает. Очень сильно.",
    "Мы делаем стриминг великим снова. Каждый день.",
    "Тысячи серий. Может, миллионы. Никто не считал, но много.",
    "Скажу честно: качество — топ. Все так говорят.",
    "Best anime platform. Nobody streams like us. Believe me.",
    "最高のアニメサイト。誰にも負けない。本当だよ。"
  ],
  "uptime_quips": [
    "ОЧЕНЬ МНОГО",
    "ОГРОМНЫЙ",
    "ЛУЧШИЙ В МИРЕ",
    "КОЛОССАЛЬНЫЙ",
    "TREMENDOUS"
  ],
  "vibes": [
    { "ux_delta": "+5 (Tremendous)", "cdi": "0.00 * 99", "mvq": "Dragon 99%/99%" },
    { "ux_delta": "+5 (Bigly)", "cdi": "0.01 * 88", "mvq": "Phoenix 95%/95%" },
    { "ux_delta": "+4 (Huge)", "cdi": "0.02 * 77", "mvq": "Griffin 92%/90%" },
    { "ux_delta": "+5 (The Best)", "cdi": "0.00 * 100", "mvq": "Kraken 98%/96%" }
  ]
}
```

- [ ] **Step 2: Create the tile allowlist JSON**

`platform_stats_prom.json` (metric names match the project's `/metrics` exposition; `metric` is wrapped in `sum(...)` by the resolver):

```json
[
  { "id": "requests", "label": "Запросов обработано", "metric": "http_requests_total", "format": "int", "windows": ["day", "week", "all"] },
  { "id": "responses", "label": "Ответов отдано", "metric": "http_request_duration_seconds_count", "format": "int", "windows": ["day", "week", "all"] },
  { "id": "bytes", "label": "Данных отдано", "metric": "http_response_size_bytes_sum", "format": "bytes", "windows": ["day", "week", "all"] },
  { "id": "go_routines", "label": "Горутин крутится", "metric": "go_goroutines", "format": "int", "windows": ["all"] },
  { "id": "anime_requests", "label": "Запросов к каталогу", "metric": "http_requests_total", "format": "int", "windows": ["week", "all"] }
]
```

- [ ] **Step 3: Write the failing integrity test**

`platform_stats_pools_test.go`:

```go
package cards

import "testing"

func TestPlatformStatsPools_LoadOK(t *testing.T) {
	if poolErr != nil {
		t.Fatalf("embedded pools failed to parse: %v", poolErr)
	}
	if len(parsedJokes.Taglines) == 0 {
		t.Fatal("no taglines")
	}
	if len(parsedJokes.UptimeQuips) == 0 {
		t.Fatal("no uptime quips")
	}
	if len(parsedJokes.Vibes) == 0 {
		t.Fatal("no vibes")
	}
	if len(parsedTiles) == 0 {
		t.Fatal("no tile entries")
	}
	for i, tl := range parsedTiles {
		if tl.Metric == "" || tl.Label == "" || len(tl.Windows) == 0 {
			t.Fatalf("tile %d incomplete: %+v", i, tl)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run TestPlatformStatsPools -count=1`
Expected: FAIL — `poolErr`/`parsedJokes`/`parsedTiles`/`vibe`/`promTile` undefined.

- [ ] **Step 5: Write the loader**

`platform_stats_pools.go`:

```go
package cards

import (
	_ "embed"
	"encoding/json"
)

//go:embed platform_stats_jokes.json
var jokesJSON []byte

//go:embed platform_stats_prom.json
var promJSON []byte

// vibe is one canned UXΔ/CDI/MVQ value-set (language-neutral).
type vibe struct {
	UXDelta string `json:"ux_delta"`
	CDI     string `json:"cdi"`
	MVQ     string `json:"mvq"`
}

// jokePool is the hero joke content. Each slice is mixed-language (~80/10/10
// RU/EN/JP); a uniform random pick makes exposure track that ratio.
type jokePool struct {
	Taglines    []string `json:"taglines"`
	UptimeQuips []string `json:"uptime_quips"`
	Vibes       []vibe   `json:"vibes"`
}

// promTile is one allowlisted Prometheus tile query. Metric is wrapped in
// sum(...) by windowPromQL so each tile aggregates across instances.
type promTile struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Metric  string   `json:"metric"`
	Format  string   `json:"format"`
	Windows []string `json:"windows"`
}

var (
	parsedJokes jokePool
	parsedTiles []promTile
	poolErr     error
)

func init() {
	if poolErr = json.Unmarshal(jokesJSON, &parsedJokes); poolErr != nil {
		return
	}
	poolErr = json.Unmarshal(promJSON, &parsedTiles)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run TestPlatformStatsPools -count=1`
Expected: PASS. (The package will not fully build until Task 4 rewrites `platform_stats.go` — if the old resolver still references `StatsMetric`, this run may fail to compile. In that case proceed to Task 4 and run this test at the end of Task 4.)

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/service/spotlight/cards/platform_stats_jokes.json services/catalog/internal/service/spotlight/cards/platform_stats_prom.json services/catalog/internal/service/spotlight/cards/platform_stats_pools.go services/catalog/internal/service/spotlight/cards/platform_stats_pools_test.go
git commit -m "feat(spotlight): embedded joke + Prometheus tile pools for platform_stats

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Resolver rewrite

**Files:**
- Rewrite: `services/catalog/internal/service/spotlight/cards/platform_stats.go`
- Rewrite: `services/catalog/internal/service/spotlight/cards/platform_stats_test.go`

- [ ] **Step 1: Write the failing test**

Replace the entire contents of `platform_stats_test.go` with:

```go
package cards

import (
	"context"
	"errors"
	"testing"

	spotlight "github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// type alias so assertions read cleanly.
type spotlightPlatformStatsData = spotlight.PlatformStatsData

// fakePrometheus implements promQuerier with scripted return values.
// healthErr (if set) is returned by Health; queryErr (if set) is returned by
// every Query — used to simulate Prometheus being unreachable.
type fakePrometheus struct {
	healthAllUp bool
	healthPct   float64
	healthErr   error
	queryErr    error
}

func (f *fakePrometheus) Health(_ context.Context) (bool, float64, error) {
	return f.healthAllUp, f.healthPct, f.healthErr
}

func (f *fakePrometheus) Query(_ context.Context, _ string) (float64, error) {
	if f.queryErr != nil {
		return 0, f.queryErr
	}
	return 0, errors.New("no data")
}

// allNonZeroProm returns the same non-zero value for any Query and fixed
// health — used to guarantee tiles populate.
type allNonZeroProm struct {
	allUp bool
	pct   float64
	val   float64
}

func (p *allNonZeroProm) Health(_ context.Context) (bool, float64, error) {
	return p.allUp, p.pct, nil
}
func (p *allNonZeroProm) Query(_ context.Context, _ string) (float64, error) {
	return p.val, nil
}

func newPlatformStatsResolverForTest(prom promQuerier) *PlatformStatsResolver {
	return NewPlatformStatsResolver(prom, &fakeCache{store: map[string][]byte{}}, testLogger())
}

func TestPlatformStats_HappyPath(t *testing.T) {
	prom := &allNonZeroProm{allUp: true, pct: 99.4, val: 12345}
	card, err := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil || card.Type != "platform_stats" {
		t.Fatalf("bad card: %+v", card)
	}
	data := card.Data.(spotlightPlatformStatsData)
	if !data.Hero.WorkingOK {
		t.Fatal("want WorkingOK=true")
	}
	if data.Hero.UptimePercent == nil || *data.Hero.UptimePercent != 99.4 {
		t.Fatalf("want uptime 99.4, got %v", data.Hero.UptimePercent)
	}
	if data.Hero.Tagline == "" || data.Hero.Service == "" || data.Hero.MVQ == "" {
		t.Fatalf("hero joke fields unpopulated: %+v", data.Hero)
	}
	if len(data.Tiles) != 4 {
		t.Fatalf("want 4 tiles, got %d", len(data.Tiles))
	}
}

func TestPlatformStats_DailyStability(t *testing.T) {
	prom := &allNonZeroProm{allUp: true, pct: 50, val: 7}
	a, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	b, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	da := a.Data.(spotlightPlatformStatsData)
	db := b.Data.(spotlightPlatformStatsData)
	if da.Hero.Tagline != db.Hero.Tagline || da.Hero.Service != db.Hero.Service {
		t.Fatal("same day must yield identical hero picks")
	}
	if len(da.Tiles) != len(db.Tiles) {
		t.Fatal("same day must yield identical tile count")
	}
	for i := range da.Tiles {
		if da.Tiles[i].Window != db.Tiles[i].Window || da.Tiles[i].Label != db.Tiles[i].Label {
			t.Fatalf("tile %d differs across same-day calls", i)
		}
	}
}

func TestPlatformStats_PrometheusDownStillEligible(t *testing.T) {
	prom := &fakePrometheus{healthErr: errors.New("down"), queryErr: errors.New("down")}
	card, err := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve should not error: %v", err)
	}
	if card == nil {
		t.Fatal("card must remain eligible when Prometheus is down")
	}
	data := card.Data.(spotlightPlatformStatsData)
	if data.Hero.WorkingOK {
		t.Fatal("WorkingOK must be false when health errors")
	}
	if data.Hero.UptimePercent != nil {
		t.Fatal("UptimePercent must be nil when health errors")
	}
	if len(data.Tiles) != 0 {
		t.Fatalf("want 0 tiles when all queries fail, got %d", len(data.Tiles))
	}
	if data.Hero.Tagline == "" {
		t.Fatal("hero tagline must still render from the pool")
	}
}

func TestPlatformStats_FiltersZeroTiles(t *testing.T) {
	// Health ok, but every Query returns 0 -> all tiles filtered out.
	prom := &allNonZeroProm{allUp: true, pct: 100, val: 0}
	card, _ := newPlatformStatsResolverForTest(prom).Resolve(context.Background(), nil)
	data := card.Data.(spotlightPlatformStatsData)
	if len(data.Tiles) != 0 {
		t.Fatalf("zero-valued metrics must be filtered, got %d tiles", len(data.Tiles))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run TestPlatformStats -count=1`
Expected: FAIL — `NewPlatformStatsResolver` signature mismatch / `promQuerier` undefined.

- [ ] **Step 3: Rewrite the resolver**

Replace the entire contents of `platform_stats.go` with:

```go
package cards

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// promQuerier is the subset of the Prometheus client this resolver needs.
// Defined here so tests inject a handwritten fake (no testify/mock).
type promQuerier interface {
	Query(ctx context.Context, promql string) (float64, error)
	Health(ctx context.Context) (allUp bool, uptimePct float64, err error)
}

// statsServices is the fixed roster of real backend services the daily
// "vibe" line can name. Order is irrelevant — the pick is RNG-driven.
var statsServices = []string{
	"auth", "catalog", "streaming", "player", "rooms",
	"scheduler", "themes", "notifications", "gateway",
}

const (
	defaultTagline = "Лучшая платформа для аниме. Поверьте."
	defaultQuip    = "ОЧЕНЬ МНОГО"
)

var defaultVibe = vibe{UXDelta: "+5 (Tremendous)", CDI: "0.00 * 99", MVQ: "Dragon 99%/99%"}

// PlatformStatsResolver implements spotlight.Resolver for the bombastic
// platform_stats card. It draws real health + tile values from Prometheus
// and joke copy from the embedded pools. Always eligible: the pool-backed
// hero renders even when Prometheus is fully down.
type PlatformStatsResolver struct {
	prom  promQuerier
	cache cache.Cache
	log   *logger.Logger
}

// NewPlatformStatsResolver constructs the resolver.
func NewPlatformStatsResolver(prom promQuerier, c cache.Cache, log *logger.Logger) *PlatformStatsResolver {
	return &PlatformStatsResolver{prom: prom, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *PlatformStatsResolver) Type() string { return "platform_stats" }

// Resolve assembles the daily card. userID is ignored — everyone sees the
// same global daily pick. The payload is cached once per UTC day.
func (r *PlatformStatsResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	dateKey := spotlight.DateKeyUTC(time.Now())
	key := "spotlight:stats:" + dateKey

	var cached spotlight.PlatformStatsData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	rng := dateSeededRng(dateKey)

	// --- Hero joke content (RNG order is fixed for determinism) ---------
	quip := pickString(rng, parsedJokes.UptimeQuips, defaultQuip)
	service := statsServices[rng.Intn(len(statsServices))]
	tagline := pickString(rng, parsedJokes.Taglines, defaultTagline)
	v := pickVibe(rng, parsedJokes.Vibes)

	hero := spotlight.StatsHero{
		UptimeQuip: quip,
		Service:    service,
		Tagline:    tagline,
		UXDelta:    v.UXDelta,
		CDI:        v.CDI,
		MVQ:        v.MVQ,
	}

	// --- Real health ----------------------------------------------------
	if allUp, pct, err := r.prom.Health(ctx); err != nil {
		r.log.Warnw("spotlight.stats_health_failed", "error", err)
		hero.WorkingOK = false
		hero.UptimePercent = nil
	} else {
		hero.WorkingOK = allUp
		p := math.Round(pct*10) / 10
		hero.UptimePercent = &p
	}

	// --- Tiles: shuffle allowlist, pick a random window each, keep > 0 --
	tiles := make([]spotlight.StatsTile, 0, 4)
	order := make([]promTile, len(parsedTiles))
	copy(order, parsedTiles)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	for _, t := range order {
		if len(tiles) >= 4 {
			break
		}
		if len(t.Windows) == 0 {
			continue
		}
		window := t.Windows[rng.Intn(len(t.Windows))]
		val, err := r.prom.Query(ctx, windowPromQL(t.Metric, window))
		if err != nil {
			r.log.Warnw("spotlight.stats_tile_failed", "metric", t.Metric, "window", window, "error", err)
			continue
		}
		if val <= 0 {
			continue
		}
		tiles = append(tiles, spotlight.StatsTile{
			Label:  t.Label,
			Value:  val,
			Window: window,
			Format: t.Format,
		})
	}

	data := spotlight.PlatformStatsData{Hero: hero, Tiles: tiles}

	if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}

// dateSeededRng returns an RNG seeded from the UTC date key, so the daily
// pick is stable within the day and reproducible in tests.
func dateSeededRng(dateKey string) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write([]byte(dateKey))
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// windowPromQL builds the sum()-aggregated PromQL for a metric + window.
func windowPromQL(metric, window string) string {
	switch window {
	case "day":
		return fmt.Sprintf("sum(increase(%s[1d]))", metric)
	case "week":
		return fmt.Sprintf("sum(increase(%s[7d]))", metric)
	default: // "all"
		return fmt.Sprintf("sum(%s)", metric)
	}
}

func pickString(rng *rand.Rand, pool []string, fallback string) string {
	if len(pool) == 0 {
		return fallback
	}
	return pool[rng.Intn(len(pool))]
}

func pickVibe(rng *rand.Rand, pool []vibe) vibe {
	if len(pool) == 0 {
		return defaultVibe
	}
	return pool[rng.Intn(len(pool))]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/spotlight/cards/ -run 'TestPlatformStats|TestPlatformStatsPools' -count=1 -race`
Expected: PASS (happy path, daily stability, Prometheus-down, zero-filter, pools).

If `TestPlatformStats_HappyPath` reports fewer than 4 tiles: the allowlist has ≥5 entries each with a non-zero value via `allNonZeroProm`, so 4 should always populate. Confirm `platform_stats_prom.json` has ≥4 entries.

- [ ] **Step 5: Run the whole spotlight package**

Run: `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race`
Expected: PASS. (`main.go` won't compile yet — that's Task 5 — but package tests don't build `cmd/`.)

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/service/spotlight/cards/platform_stats.go services/catalog/internal/service/spotlight/cards/platform_stats_test.go
git commit -m "feat(spotlight): rewrite platform_stats resolver as joke card

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Config, DI, and compose wiring

**Files:**
- Modify: `services/catalog/internal/config/config.go` (Config struct + Load)
- Modify: `services/catalog/cmd/catalog-api/main.go` (line ~243)
- Modify: `docker/docker-compose.yml` (catalog env block)

- [ ] **Step 1: Add Prometheus config**

In `config.go`, add a field to the `Config` struct (next to `SpotlightEnabled`):

```go
	// Prometheus — base URL for the spotlight platform_stats card's
	// instant queries (workstream hero-spotlight). Default mirrors the
	// gateway's PROMETHEUS_SERVICE_URL incl. the /prometheus route-prefix.
	Prometheus PrometheusConfig
```

Add the config type (near the other `*Config` structs):

```go
type PrometheusConfig struct {
	URL string
}
```

In `Load()`'s returned struct literal (next to `SpotlightEnabled: ...`):

```go
		Prometheus: PrometheusConfig{
			URL: getEnv("PROMETHEUS_SERVICE_URL", "http://prometheus:9090/prometheus"),
		},
```

- [ ] **Step 2: Verify config compiles**

Run: `cd services/catalog && go build ./internal/config/`
Expected: no output (success).

- [ ] **Step 3: Wire the client in main.go**

In `services/catalog/cmd/catalog-api/main.go`, add the import (with the other internal parser imports):

```go
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/prometheus"
```

Just before `spotlightResolvers := []spotlight.Resolver{` (after the `spotlightRng :=` line ~237), add:

```go
	prometheusClient := prometheus.NewClient(cfg.Prometheus.URL)
```

Replace the existing resolver line:

```go
		cards.NewPlatformStatsResolver(db.DB, redisCache, log),
```

with:

```go
		cards.NewPlatformStatsResolver(prometheusClient, redisCache, log),
```

- [ ] **Step 4: Build the catalog binary**

Run: `cd services/catalog && go build ./...`
Expected: no output (success). Fix any unused-import issues (e.g. if `db.DB` was only used here — it is used widely elsewhere, so no change needed).

- [ ] **Step 5: Add the compose env var**

In `docker/docker-compose.yml`, inside the **catalog** service `environment:` block (the block starting at line ~405), add:

```yaml
      PROMETHEUS_SERVICE_URL: http://prometheus:9090/prometheus
```

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/config/config.go services/catalog/cmd/catalog-api/main.go docker/docker-compose.yml
git commit -m "feat(catalog): wire Prometheus client into platform_stats resolver

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Frontend types

**Files:**
- Modify: `frontend/web/src/types/spotlight.ts` (replace `PlatformMetric` + `PlatformStatsData`, lines ~119-138)

- [ ] **Step 1: Replace the types**

Delete the `PlatformMetric` interface and the `PlatformStatsData` interface and replace with:

```ts
export interface StatsHero {
  working_ok: boolean
  // Real 7-day uptime %, from Prometheus. Omitted/null when Prometheus is
  // unreachable — the card then shows the quip without a number.
  uptime_percent?: number | null
  uptime_quip: string
  service: string
  ux_delta: string
  cdi: string
  mvq: string
  tagline: string
}

export interface StatsTile {
  label: string
  value: number
  window: 'day' | 'week' | 'all'
  format: 'int' | 'bytes' | 'seconds'
}

export interface PlatformStatsData {
  hero: StatsHero
  tiles: StatsTile[]
}
```

Leave the union member at line ~242 (`| { type: 'platform_stats'; data: PlatformStatsData }`) unchanged.

- [ ] **Step 2: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: FAIL — `PlatformStatsCard.vue` still references the old `metrics` shape (fixed in Task 7). Confirm the ONLY errors are in `PlatformStatsCard.vue` / its spec. If errors appear elsewhere, something else consumed `PlatformMetric` — grep and fix.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/types/spotlight.ts
git commit -m "feat(spotlight): frontend PlatformStatsData -> hero+tiles shape

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: Frontend card + spec

**Files:**
- Rewrite: `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue`
- Rewrite: `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts`

- [ ] **Step 1: Write the failing spec**

Replace the entire contents of `PlatformStatsCard.spec.ts` with:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PlatformStatsCard from './PlatformStatsCard.vue'
import type { PlatformStatsData } from '@/types/spotlight'

const base: PlatformStatsData = {
  hero: {
    working_ok: true,
    uptime_percent: 99.4,
    uptime_quip: 'ОЧЕНЬ МНОГО',
    service: 'catalog',
    ux_delta: '+5 (Tremendous)',
    cdi: '0.00 * 99',
    mvq: 'Dragon 99%/99%',
    tagline: 'Лучшая платформа. Поверьте.',
  },
  tiles: [
    { label: 'Запросов обработано', value: 48201, window: 'day', format: 'int' },
    { label: 'Данных отдано', value: 1610612736, window: 'week', format: 'bytes' },
  ],
}

const clone = (over: Partial<PlatformStatsData['hero']> = {}): PlatformStatsData => ({
  hero: { ...base.hero, ...over },
  tiles: base.tiles,
})

describe('PlatformStatsCard (joke)', () => {
  it('renders ДА when working_ok is true', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.text()).toContain('ДА')
    expect(w.text()).not.toContain('ТЕХНИЧЕСКИ ДА')
  })

  it('renders ТЕХНИЧЕСКИ ДА when working_ok is false', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone({ working_ok: false }) } })
    expect(w.text()).toContain('ТЕХНИЧЕСКИ ДА')
  })

  it('renders the uptime quip + percent, and omits percent when null', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.text()).toContain('ОЧЕНЬ МНОГО')
    expect(w.text()).toContain('99.4%')
    const w2 = mount(PlatformStatsCard, { props: { data: clone({ uptime_percent: null }) } })
    expect(w2.text()).toContain('ОЧЕНЬ МНОГО')
    expect(w2.text()).not.toContain('%')
  })

  it('renders the vibe row and tagline', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    const txt = w.text()
    expect(txt).toContain('catalog')
    expect(txt).toContain('+5 (Tremendous)')
    expect(txt).toContain('Dragon 99%/99%')
    expect(txt).toContain('Лучшая платформа. Поверьте.')
  })

  it('renders one tile per entry with Russian window badges', () => {
    const w = mount(PlatformStatsCard, { props: { data: clone() } })
    expect(w.findAll('li')).toHaveLength(2)
    expect(w.text()).toContain('ЗА ДЕНЬ')
    expect(w.text()).toContain('ЗА НЕДЕЛЮ')
  })

  it('renders hero only when there are no tiles', () => {
    const w = mount(PlatformStatsCard, { props: { data: { hero: base.hero, tiles: [] } } })
    expect(w.findAll('li')).toHaveLength(0)
    expect(w.text()).toContain('ДА')
  })
})
```

- [ ] **Step 2: Run spec to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts`
Expected: FAIL — the old SFC reads `data.metrics`.

- [ ] **Step 3: Rewrite the SFC**

Replace the entire contents of `PlatformStatsCard.vue` with:

```vue
<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop variant="gradient-mesh" accent="teal" />
    <div
      aria-hidden="true"
      class="absolute inset-0 opacity-5"
      style="background-image: repeating-linear-gradient(0deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px), repeating-linear-gradient(90deg, transparent, transparent 39px, rgba(255,255,255,.5) 40px);"
    />
    <div
      class="relative z-10 w-full h-full grid md:grid-cols-[2fr_3fr] gap-6 p-4 md:p-6 lg:p-8"
    >
      <!-- Hero (left) -->
      <div class="flex flex-col justify-center min-w-0">
        <div class="flex items-center gap-2 mb-3">
          <SpotlightIcon name="chart" class="w-5 h-5 text-teal-300" />
          <h3 class="text-base font-semibold text-white">Как дела у платформы</h3>
        </div>

        <p class="text-2xl md:text-3xl font-semibold text-white leading-tight">
          Работает:
          <span :class="hero.working_ok ? 'text-teal-300' : 'text-amber-300'">
            {{ hero.working_ok ? 'ДА' : 'ТЕХНИЧЕСКИ ДА' }}
          </span>
        </p>

        <p class="mt-2 text-lg font-medium text-teal-200">
          Аптайм: {{ hero.uptime_quip
          }}<template v-if="hero.uptime_percent != null"> — {{ hero.uptime_percent }}%</template>
        </p>

        <p class="mt-3 text-sm font-medium text-gray-200 break-words">
          {{ hero.service }} — UXΔ {{ hero.ux_delta }} · CDI {{ hero.cdi }} · MVQ {{ hero.mvq }}
        </p>

        <p class="mt-4 text-base md:text-lg font-medium text-white/90 italic">
          «{{ hero.tagline }}»
        </p>
      </div>

      <!-- Tiles (right, 2×2) -->
      <ul class="grid grid-cols-2 gap-3 content-center min-w-0">
        <li
          v-for="(tile, i) in tiles"
          :key="i"
          class="flex flex-col p-3 rounded-lg bg-white/5 backdrop-blur-sm"
        >
          <span class="text-[10px] font-medium text-teal-300 uppercase tracking-wider">
            {{ windowLabel(tile.window) }}
          </span>
          <p class="mt-1 text-2xl font-semibold text-white tabular-nums">
            {{ formatValue(tile) }}
          </p>
          <p class="text-[11px] font-medium text-gray-400 truncate">{{ tile.label }}</p>
        </li>
      </ul>
    </div>
  </article>
</template>

<script setup lang="ts">
// Workstream hero-spotlight — Trump-style joke rewrite of platform_stats.
// SINGLE-ROOT <article>, NO top-level v-if (Transition mode="out-in" safety).
// Deliberately i18n-free: chrome is fixed Russian and the joke content is
// rendered verbatim from the backend payload (everyone sees the same).
import { computed } from 'vue'
import type { PlatformStatsData, StatsTile } from '@/types/spotlight'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'

const props = defineProps<{ data: PlatformStatsData }>()

const hero = computed(() => props.data.hero)
const tiles = computed(() => props.data.tiles ?? [])

function windowLabel(w: StatsTile['window']): string {
  switch (w) {
    case 'day':
      return 'ЗА ДЕНЬ'
    case 'week':
      return 'ЗА НЕДЕЛЮ'
    default:
      return 'ЗА ВСЁ ВРЕМЯ'
  }
}

function formatValue(tile: StatsTile): string {
  if (tile.format === 'bytes') return formatBytes(tile.value)
  if (tile.format === 'seconds') return `${tile.value.toFixed(2)} с`
  return Math.round(tile.value).toLocaleString('ru')
}

function formatBytes(n: number): string {
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`
}
</script>
```

- [ ] **Step 4: Run spec to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts`
Expected: PASS (6 tests). The bytes tile (`1610612736` = 1.5 ГБ) and int tile (`48 201`) render via `formatValue`.

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors related to PlatformStats. (Locale parity test may still fail until Task 8 — that's a runtime test, not tsc.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.spec.ts
git commit -m "feat(spotlight): Trump-style PlatformStatsCard.vue (hero + tile grid)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 8: i18n cleanup

**Files:**
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (remove `spotlight.platformStats`)
- Modify: `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (`cardTitle()` ~line 336)
- Modify: `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts`

- [ ] **Step 1: Remove the `platformStats` block from all three locales**

In each of `en.json`, `ru.json`, `ja.json`, delete the entire `"platformStats": { ... }` object inside the top-level `"spotlight"` object (it starts at line ~1022 in en/ru; find the matching block in ja). Ensure the JSON stays valid — remove the trailing comma issue (the key before/after must still be comma-correct).

Verify valid JSON:

Run: `cd frontend/web && node -e "require('./src/locales/en.json');require('./src/locales/ru.json');require('./src/locales/ja.json');console.log('ok')"`
Expected: `ok`

- [ ] **Step 2: Update `cardTitle()` to a fixed literal**

In `HeroSpotlightBlock.vue`, find the `cardTitle()` switch case:

```ts
    case 'platform_stats':
      return t('spotlight.platformStats.title')
```

Replace with:

```ts
    case 'platform_stats':
      return 'Как дела у платформы'
```

- [ ] **Step 3: Update the parity spec**

In `spotlight-keys.spec.ts`:

1. Remove `'platformStats',` from the `expectedSubNamespaces` array.
2. Delete the entire `platformStatsKeys` block and its `it.each(...)` (the lines defining `const platformStatsKeys = [...]` through the closing `})` of its `it.each`).

- [ ] **Step 4: Run the parity spec**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts`
Expected: PASS — no `platformStats` references remain, en/ru key sets identical.

- [ ] **Step 5: Type-check + the card spec together**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts`
Expected: PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue frontend/web/src/locales/__tests__/spotlight-keys.spec.ts
git commit -m "chore(spotlight): drop platformStats i18n (joke card is i18n-free)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 9: Full verification + deploy

**Files:** none (verification + deploy).

- [ ] **Step 1: Full backend test sweep**

Run: `cd services/catalog && go build ./... && go test ./internal/service/spotlight/... ./internal/parser/prometheus/... ./internal/config/ -count=1 -race`
Expected: PASS, build clean.

- [ ] **Step 2: Full frontend sweep**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts`
Expected: PASS, no type errors.

- [ ] **Step 3: Deploy + changelog + push via the after-update skill**

Per `CLAUDE.md`, invoke `/animeenigma-after-update`. It lints/builds, redeploys `catalog` and `web` (`make redeploy-catalog`, `make redeploy-web`), runs health checks, appends a user-facing entry to `frontend/web/public/changelog.json`, and commits + pushes. Do NOT hand-roll the deploy.

- [ ] **Step 4: Manual smoke (memory rule: i18n/string changes need a browser smoke-test)**

After redeploy, load the home page and rotate to the platform_stats card. Confirm: `Работает: ДА`, an `Аптайм` line, the `<service> — UXΔ … · CDI … · MVQ …` vibe row, a tagline, and up to 4 tiles with `ЗА ДЕНЬ`/`ЗА НЕДЕЛЮ`/`ЗА ВСЁ ВРЕМЯ` badges. Confirm no raw `spotlight.platformStats.*` key strings appear anywhere.

---

## Self-Review

**Spec coverage:**
- D1 hand-authored pool → Task 3 (embedded JSON). ✓
- D2 Prometheus tiles → Task 2 (client) + Task 4 (resolver tiles). ✓
- D3 grounded hero + canned vibe + daily service → Task 4 (`Health()`, `pickVibe`, `statsServices`). ✓
- D4 replace in place → Tasks 1/4/6/7 keep `platform_stats` discriminator. ✓
- D5 80/10/10, everyone sees same, no i18n → Task 3 pool composition, Task 4 global daily cache + date-seeded RNG, Task 7 i18n-free SFC, Task 8 i18n removal. ✓
- Determinism (DateKeyUTC + date-seeded RNG) → Task 4 + `TestPlatformStats_DailyStability`. ✓
- Always-eligible / Prometheus-down failure mode → Task 4 + `TestPlatformStats_PrometheusDownStillEligible`. ✓
- Non-zero tile filter → Task 4 + `TestPlatformStats_FiltersZeroTiles`. ✓
- Compose env + DI → Task 5. ✓
- Tests (backend fake, frontend ≥5 assertions, parity) → Tasks 2/4/7/8. ✓

**Placeholder scan:** Task 4 Step 1 intentionally shows a wrong placeholder line, then immediately replaces it with the real assertions in the same step (flagged in prose) — the engineer ends with concrete, compiling assertions. No other TBD/TODO.

**Type consistency:** `StatsHero`/`StatsTile`/`PlatformStatsData` field names + JSON tags match between Go (`types.go`, Task 1) and TS (`spotlight.ts`, Task 6). `promQuerier` (`Query`, `Health`) matches the `prometheus.Client` methods (Task 2). `Value float64` ↔ TS `number`. Resolver constructor `NewPlatformStatsResolver(prom, cache, log)` matches the test helper and the `main.go` call site (Task 5). `windowPromQL` outputs match the `fakePrometheus`/`allNonZeroProm` (which ignore the query string) and the `client_test.go` health query strings.
