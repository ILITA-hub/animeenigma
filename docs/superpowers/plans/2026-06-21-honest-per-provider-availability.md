# Honest Per-Provider Availability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the scraper able to answer "does provider X have THIS anime?" honestly (no-failover), so the playback probe can re-roll not-found anime and score by pass-percentage, and the AePlayer can show hacker-mode tooltips distinguishing "doesn't have this anime" from "CDN unreachable for this anime."

**Architecture:** A new `exclusive=true` query param disables scraper failover (returns `404 not_found` / `502 provider_down` for one provider). The analytics probe consumes it to classify not-found vs broken, re-rolls one random top-100 anime on not-found, and scores `>50%`→UP / `>0%`→DEGRADED / `0%`→DOWN. The AePlayer uses it (on `getEpisodes` only) for a hacker-mode-only, cached availability tooltip.

**Tech Stack:** Go (scraper/catalog/analytics services, `chi`, `net/http`, `httptest`), Vue 3 + TypeScript (`frontend/web`, Vitest, reka-ui), ClickHouse (probe_runs), Prometheus.

## Global Constraints

- **Base branch:** Execute in a CLEAN git worktree branched from the deployed probe commit `69822378` (the `/data/ae-probe-impl` branch — it has the probe engine + scheduler trigger + ClickHouse wiring + the `aePlayer/` FE rename that `main` lacks). Do NOT work in the dirty shared tree `/data/animeenigma`. Coordinate the eventual merge of this branch to `main` separately.
- **Do NOT touch** `services/scraper/internal/providers/` or `services/scraper/internal/embeds/` (provider matching/extraction is frozen). The only scraper change is orchestrator routing + handler query parsing.
- **Reason vocab is frozen:** reuse the EXISTING `streamprobe.ReasonZeroMatch` (`"zero_match"`) for the probe's not-found case and the EXISTING `probe.StageSearch` (`"search"`) stage — do NOT add new reason/stage constants (the reason enum is coupled to `.claude/maintenance-prompt.md` Pattern 6/7).
- **Effort metrics (NO time units):** any CHANGELOG/commit scoring uses UXΔ / CDI / MVQ per `.planning/CONVENTIONS.md`.
- **Commit co-authors** on every commit:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Frontend tooling:** `bun` / `bunx` (never npm/npx). Type-check with `bunx vue-tsc --noEmit` (catches `.vue` template errors `tsc` misses).

---

## Phase A — Backend `exclusive=true` (scraper + catalog)

### Task A1: Orchestrator no-failover mode

**Files:**
- Modify: `services/scraper/internal/service/orchestrator.go`
- Test: `services/scraper/internal/service/orchestrator_test.go`

**Interfaces:**
- Produces: `Orchestrator.orderedProviders(prefer string, exclusive bool) []domain.Provider`, `OrderedProviderNames(prefer string, exclusive bool) []string`, `FindIDNamed(ctx, ref, prefer string, exclusive bool)`, `ListEpisodesNamed(ctx, providerID, prefer string, exclusive bool)`, `ListServers(ctx, providerID, episodeID, prefer string, exclusive bool)`, `GetStreamGated(ctx, providerID, episodeID, serverID string, cat domain.Category, prefer string, exclusive bool)`.

- [ ] **Step 1: Write the failing test**

Add to `services/scraper/internal/service/orchestrator_test.go`:

```go
func TestOrchestrator_Exclusive_NoFailover_NotFound(t *testing.T) {
	t.Parallel()
	pa := &fakeProvider{
		nameVal: "gogo_" + t.Name(),
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return nil, domain.WrapNotFound(errors.New("no match"), "gogo")
		},
	}
	pb := &fakeProvider{
		nameVal: "other_" + t.Name(),
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: "ep1"}}, nil
		},
	}
	o := newTestOrchestrator(t, pa, pb)

	// exclusive=true pinned to gogo: must NOT fail over to pb; must return ErrNotFound.
	_, _, err := o.ListEpisodesNamed(context.Background(), "x", pa.Name(), true)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("exclusive pin must return ErrNotFound, got %v", err)
	}
	// And the failover-chain must be exactly [gogo].
	if names := o.OrderedProviderNames(pa.Name(), true); len(names) != 1 || names[0] != pa.Name() {
		t.Fatalf("exclusive OrderedProviderNames = %v, want [%s]", names, pa.Name())
	}
	// exclusive=false still fails over to pb.
	got, _, err := o.ListEpisodesNamed(context.Background(), "x", pa.Name(), false)
	if err != nil || len(got) != 1 {
		t.Fatalf("non-exclusive must fail over, got %v err %v", got, err)
	}
}
```

> Note: confirm the not-found wrapper name with `grep -n 'func WrapNotFound\|func WrapProviderDown' services/scraper/internal/domain/errors.go`. If it is `domain.WrapNotFound`, use it as above; the existing test uses `domain.WrapProviderDown`.

- [ ] **Step 2: Run it — expect FAIL (compile error: too few args)**

Run: `cd services/scraper && go test ./internal/service/ -run TestOrchestrator_Exclusive_NoFailover_NotFound -v`
Expected: FAIL — `not enough arguments in call to o.ListEpisodesNamed`.

- [ ] **Step 3: Add the `exclusive` param to `orderedProviders`**

Replace `func (o *Orchestrator) orderedProviders(prefer string) []domain.Provider {` body with:

```go
func (o *Orchestrator) orderedProviders(prefer string, exclusive bool) []domain.Provider {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if len(o.providers) == 0 {
		return nil
	}
	out := make([]domain.Provider, 0, len(o.providers))
	preferredIdx := -1
	if prefer != "" {
		for i, p := range o.providers {
			if p.Name() == prefer {
				preferredIdx = i
				out = append(out, p) // an explicit prefer reaches degraded providers too
				break
			}
		}
	}
	if exclusive {
		// No-failover: route ONLY to the preferred provider (empty when prefer
		// is unset/unknown, which the handler surfaces as "no providers").
		return out
	}
	for i, p := range o.providers {
		if i == preferredIdx {
			continue // already inserted at position 0
		}
		if o.degraded[p.Name()] {
			continue
		}
		out = append(out, p)
	}
	return out
}
```

- [ ] **Step 4: Thread `exclusive` through the public methods**

Update these signatures + their single internal `o.orderedProviders(prefer)` call to `o.orderedProviders(prefer, exclusive)`:

```go
func (o *Orchestrator) OrderedProviderNames(prefer string, exclusive bool) []string {
	ps := o.orderedProviders(prefer, exclusive)
	if len(ps) == 0 {
		return []string{}
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name()
	}
	return out
}

func (o *Orchestrator) FindIDNamed(ctx context.Context, ref domain.AnimeRef, prefer string, exclusive bool) (string, string, error) {
	return runFailoverNamed(ctx, o.log, o.orderedProviders(prefer, exclusive), o.cache, o.providerBudget(),
		func(c context.Context, p domain.Provider) (string, error) {
			return p.FindID(c, ref)
		})
}

func (o *Orchestrator) ListEpisodesNamed(ctx context.Context, providerID, prefer string, exclusive bool) ([]domain.Episode, string, error) {
	return runFailoverNamed(ctx, o.log, o.orderedProviders(prefer, exclusive), o.cache, o.providerBudget(),
		func(c context.Context, p domain.Provider) ([]domain.Episode, error) {
			return p.ListEpisodes(c, providerID)
		})
}

func (o *Orchestrator) ListServers(ctx context.Context, providerID, episodeID, prefer string, exclusive bool) ([]domain.Server, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer, exclusive), o.cache, o.providerBudget(),
		func(c context.Context, p domain.Provider) ([]domain.Server, error) {
			return p.ListServers(c, providerID, episodeID)
		})
}
```

For `GetStreamGated`, change the signature to add `exclusive bool` after `prefer string`, and its first line `providers := o.orderedProviders(prefer)` to `providers := o.orderedProviders(prefer, exclusive)`.

- [ ] **Step 5: Fix every remaining `orderedProviders(` caller**

Run: `grep -rn 'o.orderedProviders(' services/scraper/internal/service/`
For each remaining call that still passes ONE arg (e.g. the non-`Named` convenience wrappers `ListEpisodes`/`FindID`/`GetStream` used by existing tests), add `, false`:

```bash
# Each remaining hit becomes orderedProviders(prefer, false). Example:
#   return runFailover(ctx, o.log, o.orderedProviders(prefer), ...)
# →  return runFailover(ctx, o.log, o.orderedProviders(prefer, false), ...)
```

- [ ] **Step 6: Run the new test + the whole package — expect PASS**

Run: `cd services/scraper && go build ./... && go test ./internal/service/ -v`
Expected: PASS (new test passes; existing failover tests still green).

- [ ] **Step 7: Commit**

```bash
git add services/scraper/internal/service/orchestrator.go services/scraper/internal/service/orchestrator_test.go
git commit -m "feat(scraper): exclusive no-failover routing in orchestrator

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task A2: Scraper handler `exclusive` query param

**Files:**
- Modify: `services/scraper/internal/handler/scraper.go`
- Test: `services/scraper/internal/handler/scraper_test.go` (or the existing handler test file — confirm with `ls services/scraper/internal/handler/*_test.go`)

**Interfaces:**
- Consumes: A1's orchestrator methods (now `exclusive bool`-aware).
- Produces: handler honors `?exclusive=true` end-to-end.

- [ ] **Step 1: Write the failing test**

Add a handler test that wires a fake service recording the `exclusive` it received. Mirror the existing handler-test harness (find it: `grep -n 'func Test' services/scraper/internal/handler/scraper_test.go | head`). Minimal shape:

```go
func TestGetEpisodes_ForwardsExclusive(t *testing.T) {
	var gotExclusive bool
	svc := &fakeScraperSvc{ // mirror the existing handler test's fake service type
		orderedNames: func(prefer string, exclusive bool) []string { gotExclusive = exclusive; return []string{"gogoanime"} },
		// ... stub resolveProviderID/listEpisodes deps as the existing fake does ...
	}
	h := NewScraperHandler(svc, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/scraper/episodes?mal_id=1&prefer=gogoanime&exclusive=true", nil)
	h.GetEpisodes(httptest.NewRecorder(), req)
	if !gotExclusive {
		t.Fatal("exclusive=true query param was not forwarded to the service")
	}
}
```

> Adapt the fake to the existing test's service interface. The point: assert `exclusive` reaches `OrderedProviderNames`.

- [ ] **Step 2: Run it — expect FAIL** (`gotExclusive` false / compile error).

Run: `cd services/scraper && go test ./internal/handler/ -run TestGetEpisodes_ForwardsExclusive -v`

- [ ] **Step 3: Add `exclusive` to `queryParams` + `parseQuery`**

In `services/scraper/internal/handler/scraper.go`, add field to the struct:

```go
type queryParams struct {
	malID     string
	title     string
	altTitles []string
	episode   string
	server    string
	category  string
	prefer    string
	exclusive bool
}
```

In `parseQuery`, before the `return`, add:

```go
exclusive := q.Get("exclusive") == "true"
```

and add `exclusive: exclusive,` to the returned struct literal.

- [ ] **Step 4: Thread `qp.exclusive` through the three handlers + `resolveProviderID`**

In `GetEpisodes`, `GetServers`, `GetStream`: change `tried := h.svc.OrderedProviderNames(qp.prefer)` → `tried := h.svc.OrderedProviderNames(qp.prefer, qp.exclusive)`, and add `qp.exclusive` to every `h.resolveProviderID(...)`, `h.svc.ListEpisodesNamed(...)`, `h.svc.ListServers(...)`, `h.svc.GetStreamGated(...)` call (append as the new trailing arg, matching A1's signatures).

Update `resolveProviderID`'s signature to accept `exclusive bool` and pass it to `FindIDNamed`:

```go
func (h *ScraperHandler) resolveProviderID(ctx context.Context, malID, title string, altTitles []string, prefer string, exclusive bool) (string, string, error) {
	// ... existing body, but the FindIDNamed call becomes: ...
	// return h.svc.FindIDNamed(ctx, ref, prefer, exclusive)  (or o.FindIDNamed — match existing)
}
```

> Run `grep -n 'FindIDNamed\|OrderedProviderNames\|ListEpisodesNamed\|ListServers\|GetStreamGated' services/scraper/internal/handler/scraper.go` and update the service-interface declaration (the `h.svc` type) so each method's signature gains the `exclusive bool` arg too.

- [ ] **Step 5: Run handler tests — expect PASS**

Run: `cd services/scraper && go build ./... && go test ./internal/handler/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/scraper/internal/handler/scraper.go services/scraper/internal/handler/scraper_test.go
git commit -m "feat(scraper): honor exclusive=true query param in handlers

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task A3: Catalog passthrough forwards `exclusive`

**Files:**
- Modify: `services/catalog/internal/handler/scraper.go`, `services/catalog/internal/service/scraper.go`, `services/catalog/internal/parser/scraper/client.go`
- Test: `services/catalog/internal/parser/scraper/client_test.go` (confirm exists; else add)

**Interfaces:**
- Produces: catalog `/api/anime/{uuid}/scraper/*?exclusive=true` adds `exclusive=true` to the outbound scraper URL.

- [ ] **Step 1: Write the failing test** (client URL building)

Add to `services/catalog/internal/parser/scraper/client_test.go`:

```go
func TestClient_GetEpisodes_ExclusiveParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, srv.Client()) // match the real NewClient signature
	_, _, err := c.GetEpisodes(context.Background(), 1, "Frieren", nil, "gogoanime", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "exclusive=true") {
		t.Fatalf("outbound query %q missing exclusive=true", gotQuery)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (too few args).

Run: `cd services/catalog && go test ./internal/parser/scraper/ -run TestClient_GetEpisodes_ExclusiveParam -v`

- [ ] **Step 3: Add `exclusive bool` to the client methods**

In `services/catalog/internal/parser/scraper/client.go`, append `exclusive bool` to `GetEpisodes`, `GetServers`, `GetStream` and, before `return c.doGET(...)`, add:

```go
if exclusive {
	q.Set("exclusive", "true")
}
```

- [ ] **Step 4: Thread through service + handler**

`services/catalog/internal/service/scraper.go`:
- Add `exclusive bool` to `scraperForwarder` interface's `GetEpisodes`/`GetServers`/`GetStream`.
- Add `exclusive bool` to `scraperOps.GetScraperEpisodes`/`GetScraperServers`/`GetScraperStream` and pass it through to `o.scraperClient.GetEpisodes(... , exclusive)` etc.
- Update the matching interface the handler depends on (`scraperSvc`).

`services/catalog/internal/handler/scraper.go` — in each of `GetScraperEpisodes`/`GetScraperServers`/`GetScraperStream`, after reading `prefer`, add:

```go
exclusive := r.URL.Query().Get("exclusive") == "true"
```

and pass `exclusive` as the new trailing arg to `h.scraperSvc.GetScraper*(...)`.

- [ ] **Step 5: Run catalog build + tests — expect PASS**

Run: `cd services/catalog && go build ./... && go test ./internal/parser/scraper/ ./internal/service/ ./internal/handler/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/handler/scraper.go services/catalog/internal/service/scraper.go services/catalog/internal/parser/scraper/client.go services/catalog/internal/parser/scraper/client_test.go
git commit -m "feat(catalog): forward exclusive=true through scraper passthrough

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase B — Probe not-found re-roll + percentage scorer (analytics)

### Task B1: Resolver — exclusive + classify not-found

**Files:**
- Modify: `services/analytics/internal/probe/resolver.go`
- Test: `services/analytics/internal/probe/resolver_test.go`

**Interfaces:**
- Produces: package var `ErrProbeNotFound error`; `HTTPResolver.Resolve` sends `exclusive=true` and returns `(nil, StageSearch, ErrProbeNotFound)` on a `404` from the episodes call.

- [ ] **Step 1: Write the failing test**

Add to `resolver_test.go`:

```go
func TestHTTPResolver_NotFound404_ReturnsSearchSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "exclusive=true") {
			t.Errorf("resolver must send exclusive=true; got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"success":false,"error":{"code":"NOT_FOUND"}}`))
	}))
	defer srv.Close()
	r := NewHTTPResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "uuid-x", SlotRandom, "gogoanime")
	if !errors.Is(err, ErrProbeNotFound) {
		t.Fatalf("want ErrProbeNotFound, got %v", err)
	}
	if stage != StageSearch {
		t.Fatalf("want StageSearch, got %v", stage)
	}
}
```

Add `"errors"` to the test imports if missing.

- [ ] **Step 2: Run it — expect FAIL** (`ErrProbeNotFound` undefined).

Run: `cd services/analytics && go test ./internal/probe/ -run TestHTTPResolver_NotFound404 -v`

- [ ] **Step 3: Implement**

In `resolver.go`, add the sentinel + status-aware `get`:

```go
// ErrProbeNotFound signals the preferred provider has no match for the anime
// (scraper returned 404 not_found under exclusive routing). The engine treats
// this as "skip + re-roll", never a provider failure.
var ErrProbeNotFound = errors.New("probe: provider has no match for anime")
```

Add `"errors"` to imports. Change `get` so a `404` returns the sentinel:

```go
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProbeNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> %d", path, resp.StatusCode)
	}
```

In `Resolve`, add `"exclusive": {"true"}` to ALL THREE `url.Values` (episodes, servers, stream). On the episodes error, map the sentinel to `StageSearch`:

```go
	eps, err := r.get(ctx, base+"/episodes", url.Values{"prefer": {provider}, "exclusive": {"true"}})
	if err != nil {
		if errors.Is(err, ErrProbeNotFound) {
			return nil, StageSearch, ErrProbeNotFound
		}
		return nil, StageEpisodes, err
	}
```

The `servers` and `stream` `get` calls also add `"exclusive": {"true"}` to their `url.Values`.

- [ ] **Step 4: Run resolver tests — expect PASS**

Run: `cd services/analytics && go test ./internal/probe/ -run TestHTTPResolver -v`
Expected: PASS (happy-path + no-episodes + new not-found test).

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/probe/resolver.go services/analytics/internal/probe/resolver_test.go
git commit -m "feat(probe): resolver sends exclusive=true + classifies 404 not-found

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B2: PopularPool (top-100 re-roll source)

**Files:**
- Create: `services/analytics/internal/probe/popularpool.go`
- Test: `services/analytics/internal/probe/popularpool_test.go`

**Interfaces:**
- Produces: `type PopularPool interface { UUIDs(ctx context.Context) ([]string, error) }` and `func NewHTTPPopularPool(catalogBaseURL string, hc *http.Client) *HTTPPopularPool`.

- [ ] **Step 1: Write the failing test**

Create `popularpool_test.go`:

```go
package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPPopularPool_UUIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/anime/popular" || r.URL.Query().Get("page_size") != "100" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"success":true,"data":[{"id":"u1","name":"A"},{"id":"u2","name":"B"}],"meta":{}}`))
	}))
	defer srv.Close()
	p := NewHTTPPopularPool(srv.URL, srv.Client())
	ids, err := p.UUIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "u1" || ids[1] != "u2" {
		t.Fatalf("ids = %v", ids)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (undefined).

Run: `cd services/analytics && go test ./internal/probe/ -run TestHTTPPopularPool -v`

- [ ] **Step 3: Implement**

Create `popularpool.go`:

```go
package probe

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// PopularPool returns a pool of well-known anime UUIDs the probe can re-roll
// into when a provider has no match for a slot's anime.
type PopularPool interface {
	UUIDs(ctx context.Context) ([]string, error)
}

type HTTPPopularPool struct {
	base string
	hc   *http.Client
}

func NewHTTPPopularPool(catalogBaseURL string, hc *http.Client) *HTTPPopularPool {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPPopularPool{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

func (p *HTTPPopularPool) UUIDs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/anime/popular?page_size=100", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(env.Data))
	for _, a := range env.Data {
		if a.ID != "" {
			out = append(out, a.ID)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run it — expect PASS.** `cd services/analytics && go test ./internal/probe/ -run TestHTTPPopularPool -v`

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/probe/popularpool.go services/analytics/internal/probe/popularpool_test.go
git commit -m "feat(probe): HTTPPopularPool — top-100 re-roll source from /api/anime/popular

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B3: Engine — one re-roll on not-found

**Files:**
- Modify: `services/analytics/internal/probe/engine.go`
- Test: `services/analytics/internal/probe/engine_test.go`

**Interfaces:**
- Consumes: `ErrProbeNotFound` (B1), `PopularPool` (B2).
- Produces: `func NewEngine(providers []string, as AnimeSetResolver, res Resolver, val Validator, rep Reporter, pool PopularPool, rng *rand.Rand, now func() int64, log *logger.Logger) *Engine`.

- [ ] **Step 1: Write the failing test**

Add to `engine_test.go` (and add `"math/rand"` to imports):

```go
// fakePool returns a fixed re-roll candidate.
type fakePool struct{ ids []string }

func (f fakePool) UUIDs(_ context.Context) ([]string, error) { return f.ids, nil }

// notFoundOnce: anchor anime → ErrProbeNotFound; the re-roll uuid "POOL1" → playable.
type notFoundThenOK struct{}

func (notFoundThenOK) Resolve(_ context.Context, u string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	if u == "POOL1" {
		return []ResolvedStream{{Provider: p, AnimeUUID: u, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
	}
	return nil, StageSearch, ErrProbeNotFound
}

func TestEngine_NotFound_RerollsOnce_Pass(t *testing.T) {
	rep := &capRep{}
	e := NewEngine([]string{"gogoanime"}, fakeAS{}, notFoundThenOK{}, fakeVal{}, rep,
		fakePool{ids: []string{"POOL1"}}, rand.New(rand.NewSource(1)), func() int64 { return 9 }, nil)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The not-found original is logged AND the re-roll playable verdict exists.
	var sawNotFound, sawPlayable bool
	for _, v := range rep.run.Verdicts {
		if v.Reason == streamprobe.ReasonZeroMatch && v.Stage == StageSearch {
			sawNotFound = true
		}
		if v.AnimeUUID == "POOL1" && v.Playable() {
			sawPlayable = true
		}
	}
	if !sawNotFound || !sawPlayable {
		t.Fatalf("want logged not-found + playable re-roll; verdicts=%+v", rep.run.Verdicts)
	}
	// Provider rolls up UP (the single slot passed via re-roll).
	if rep.run.ProviderVerdicts[0].Status != StatusUp {
		t.Fatalf("status=%v want Up", rep.run.ProviderVerdicts[0].Status)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (NewEngine arity).

Run: `cd services/analytics && go test ./internal/probe/ -run TestEngine_NotFound_RerollsOnce -v`

- [ ] **Step 3: Implement the new engine**

Replace `engine.go` with:

```go
package probe

import (
	"context"
	"math/rand"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type Engine struct {
	providers []string
	as        AnimeSetResolver
	res       Resolver
	val       Validator
	rep       Reporter
	pool      PopularPool
	rng       *rand.Rand
	now       func() int64
	log       *logger.Logger
}

func NewEngine(providers []string, as AnimeSetResolver, res Resolver, val Validator, rep Reporter, pool PopularPool, rng *rand.Rand, now func() int64, log *logger.Logger) *Engine {
	return &Engine{providers: providers, as: as, res: res, val: val, rep: rep, pool: pool, rng: rng, now: now, log: log}
}

// resolveAndValidate resolves one anime for a provider and validates every
// server it yields, tagging each verdict with the given slot.
func (e *Engine) resolveAndValidate(ctx context.Context, uuid string, slot AnimeSlot, p string) ([]Verdict, error) {
	streams, stage, err := e.res.Resolve(ctx, uuid, slot, p)
	if err != nil {
		if err == ErrProbeNotFound { //nolint:errorlint // sentinel
			return []Verdict{{Provider: p, AnimeUUID: uuid, Slot: slot, Stage: StageSearch, Reason: streamprobe.ReasonZeroMatch}}, ErrProbeNotFound
		}
		return []Verdict{{Provider: p, AnimeUUID: uuid, Slot: slot, Stage: stage, Reason: streamprobe.ReasonCDNUnreachable}}, nil
	}
	out := make([]Verdict, 0, len(streams))
	for _, s := range streams {
		out = append(out, e.val.Validate(ctx, s))
	}
	return out, nil
}

// reroll picks one random pool UUID not equal to `exclude`, resolves+validates
// it under the same slot. Returns nil verdicts if no candidate is available.
func (e *Engine) reroll(ctx context.Context, exclude string, slot AnimeSlot, p string) []Verdict {
	ids, err := e.pool.UUIDs(ctx)
	if err != nil || len(ids) == 0 {
		if e.log != nil {
			e.log.Warnw("probe re-roll pool unavailable", "provider", p, "error", err)
		}
		return nil
	}
	// One random pick (skip the not-found uuid).
	start := e.rng.Intn(len(ids))
	pick := ""
	for i := 0; i < len(ids); i++ {
		c := ids[(start+i)%len(ids)]
		if c != exclude {
			pick = c
			break
		}
	}
	if pick == "" {
		return nil
	}
	v, _ := e.resolveAndValidate(ctx, pick, slot, p) // re-roll failure of any kind => its verdicts are FAIL/not-found
	return v
}

func (e *Engine) probeProvider(ctx context.Context, p string, refs []AnimeRef) (verdicts []Verdict) {
	defer func() {
		if r := recover(); r != nil {
			if e.log != nil {
				e.log.Errorw("probe provider panicked", "provider", p, "panic", r)
			}
			verdicts = append(verdicts, Verdict{Provider: p, Stage: StageStream, Reason: streamprobe.ReasonCDNUnreachable})
		}
	}()
	for _, ref := range refs {
		vs, err := e.resolveAndValidate(ctx, ref.UUID, ref.Slot, p)
		verdicts = append(verdicts, vs...)
		if err == ErrProbeNotFound { //nolint:errorlint
			// Skip this anime; give the provider exactly one re-roll for the slot.
			verdicts = append(verdicts, e.reroll(ctx, ref.UUID, ref.Slot, p)...)
		}
	}
	return verdicts
}

func (e *Engine) RunOnce(ctx context.Context) error {
	refs, err := e.as.Resolve(ctx)
	if err != nil && len(refs) == 0 {
		return err
	}
	var allVerdicts []Verdict
	var provVerdicts []ProviderVerdict
	for _, p := range e.providers {
		verdicts := e.probeProvider(ctx, p, refs)
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, Rollup(p, verdicts))
	}
	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
```

- [ ] **Step 4: Update the OTHER engine tests' `NewEngine` calls**

The existing `TestEngine_RunOnce`, `TestEngine_ResolveError_SynthesizesCDNUnreachable`, `TestEngine_ProviderPanic_Isolated` call `NewEngine` with the OLD arity. Add `fakePool{}, rand.New(rand.NewSource(1)),` before the `now` func in each:

```go
e := NewEngine([]string{"gogoanime"}, fakeAS{}, fakeRes{}, fakeVal{}, rep, fakePool{}, rand.New(rand.NewSource(1)), func() int64 { return 42 }, nil)
```

(Apply the same insertion to all three existing constructions.)

- [ ] **Step 5: Run the probe package — expect PASS**

Run: `cd services/analytics && go test ./internal/probe/ -run TestEngine -v`
Expected: PASS (new re-roll test + the 3 updated existing tests). Note `TestEngine_ResolveError_SynthesizesCDNUnreachable` still passes because `context.DeadlineExceeded != ErrProbeNotFound` → CDNUnreachable path.

- [ ] **Step 6: Commit**

```bash
git add services/analytics/internal/probe/engine.go services/analytics/internal/probe/engine_test.go
git commit -m "feat(probe): one re-roll from top-100 pool on provider not-found

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B4: Scorer — per-slot pass-percentage thresholds

**Files:**
- Modify: `services/analytics/internal/probe/scorer.go`
- Test: `services/analytics/internal/probe/scorer_test.go`

**Interfaces:**
- Produces: `Rollup(provider string, verdicts []Verdict) ProviderVerdict` with `>50%`→Up, `>0%`→Degraded, `0%`→Down over distinct slots (slot passes iff any verdict in it is playable).

- [ ] **Step 1: Rewrite the scorer test with slot-based boundary cases**

Replace `scorer_test.go` contents with:

```go
package probe

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// v builds a verdict in a slot with a reason.
func v(slot AnimeSlot, reason streamprobe.Reason) Verdict {
	return Verdict{Slot: slot, Stage: StagePlayback, Reason: reason, Server: "https://h/x?type=hd-1&y"}
}

func TestRollup_Up_Over50(t *testing.T) { // 3/4 slots pass
	pv := Rollup("p", []Verdict{
		v(SlotAnchor, streamprobe.ReasonPlayable),
		v(SlotFeatured, streamprobe.ReasonPlayable),
		v(SlotSpotlightRandom, streamprobe.ReasonPlayable),
		v(SlotRandom, streamprobe.ReasonStatus403),
	})
	if pv.Status != StatusUp {
		t.Fatalf("3/4 must be Up, got %v", pv.Status)
	}
}

func TestRollup_Degraded_Exactly50(t *testing.T) { // 2/4 = 50% is NOT >50 → Degraded
	pv := Rollup("p", []Verdict{
		v(SlotAnchor, streamprobe.ReasonPlayable),
		v(SlotFeatured, streamprobe.ReasonPlayable),
		v(SlotSpotlightRandom, streamprobe.ReasonStatus403),
		v(SlotRandom, streamprobe.ReasonCDNUnreachable),
	})
	if pv.Status != StatusDegraded {
		t.Fatalf("2/4 must be Degraded, got %v", pv.Status)
	}
}

func TestRollup_Degraded_OneOfFour(t *testing.T) { // 1/4 → Degraded
	pv := Rollup("p", []Verdict{
		v(SlotAnchor, streamprobe.ReasonPlayable),
		v(SlotFeatured, streamprobe.ReasonStatus403),
		v(SlotSpotlightRandom, streamprobe.ReasonStatus403),
		v(SlotRandom, streamprobe.ReasonCDNUnreachable),
	})
	if pv.Status != StatusDegraded {
		t.Fatalf("1/4 must be Degraded, got %v", pv.Status)
	}
	if !strings.Contains(pv.Reason, "status_403") {
		t.Fatalf("dominant reason want status_403, got %q", pv.Reason)
	}
}

func TestRollup_Down_Zero(t *testing.T) { // 0/2 → Down
	pv := Rollup("p", []Verdict{
		v(SlotAnchor, streamprobe.ReasonStatus403),
		v(SlotFeatured, streamprobe.ReasonCDNUnreachable),
	})
	if pv.Status != StatusDown {
		t.Fatalf("0%% must be Down, got %v", pv.Status)
	}
}

func TestRollup_SlotPassIfAnyServerPlays(t *testing.T) { // anchor hd-1 plays, hd-2 fails → slot passes → 1/1 Up
	pv := Rollup("p", []Verdict{
		{Slot: SlotAnchor, Stage: StagePlayback, Reason: streamprobe.ReasonPlayable, Server: "x?type=hd-1"},
		{Slot: SlotAnchor, Stage: StagePlayback, Reason: streamprobe.ReasonEmptyResponse, Server: "x?type=hd-2"},
	})
	if pv.Status != StatusUp {
		t.Fatalf("any-server-plays slot must pass, got %v", pv.Status)
	}
}

func TestRollup_Empty(t *testing.T) {
	if Rollup("p", nil).Status != StatusDown {
		t.Fatal("empty must be Down")
	}
}

func TestRollup_TieBreakDeterministic(t *testing.T) { // 0 pass → Down; reason tie-break lexicographic
	pv := Rollup("p", []Verdict{
		{Slot: SlotAnchor, Stage: StagePlayback, Reason: streamprobe.ReasonStatus403, Server: "https://example.com/x"},
		{Slot: SlotFeatured, Stage: StagePlayback, Reason: streamprobe.ReasonCDNUnreachable, Server: "https://cdn.example.com/y"},
	})
	if pv.Status != StatusDown {
		t.Fatalf("want Down, got %v", pv.Status)
	}
	if !strings.HasPrefix(pv.Reason, "cdn_unreachable on ") { // "cdn_unreachable" < "status_403"
		t.Fatalf("tie-break reason = %q", pv.Reason)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (old Rollup uses any-pass→Up; several new cases fail).

Run: `cd services/analytics && go test ./internal/probe/ -run TestRollup -v`

- [ ] **Step 3: Rewrite `Rollup`**

Replace the `Rollup` function in `scorer.go` (keep `serverShortLabel` unchanged) with:

```go
// Rollup scores a provider by the fraction of distinct anime slots that played.
// A slot passes iff ANY of its verdicts is playable. >50% Up, >0% Degraded,
// 0% Down. The dominant non-playable reason (deterministic lexicographic tie-
// break) labels Degraded/Down.
func Rollup(provider string, verdicts []Verdict) ProviderVerdict {
	pv := ProviderVerdict{Provider: provider, Status: StatusDown}
	if len(verdicts) == 0 {
		return pv
	}
	slotPass := map[AnimeSlot]bool{}
	slotSeen := map[AnimeSlot]bool{}
	counts := map[streamprobe.Reason]int{}
	firstServer := map[streamprobe.Reason]string{}
	for _, vd := range verdicts {
		slotSeen[vd.Slot] = true
		if vd.Playable() {
			slotPass[vd.Slot] = true
			continue
		}
		counts[vd.Reason]++
		if _, ok := firstServer[vd.Reason]; !ok {
			firstServer[vd.Reason] = vd.Server
		}
	}
	pass := 0
	for s := range slotSeen {
		if slotPass[s] {
			pass++
		}
	}
	ratio := float64(pass) / float64(len(slotSeen))
	switch {
	case ratio > 0.5:
		pv.Status = StatusUp
		return pv
	case ratio > 0:
		pv.Status = StatusDegraded
	default:
		pv.Status = StatusDown
	}
	// Dominant non-playable reason (deterministic tie-break: higher count wins,
	// ties broken by lexicographically-smaller reason). Preserves commit 22d3c095.
	var domR streamprobe.Reason
	best := -1
	for r, c := range counts {
		if c > best || (c == best && string(r) < string(domR)) {
			best, domR = c, r
		}
	}
	if best >= 0 {
		pv.Reason = string(domR) + " on " + serverShortLabel(firstServer[domR])
	}
	return pv
}
```

- [ ] **Step 4: Run scorer tests — expect PASS.** `cd services/analytics && go test ./internal/probe/ -run TestRollup -v`

- [ ] **Step 5: Run the WHOLE probe package** (engine tests depend on Rollup semantics):

Run: `cd services/analytics && go test ./internal/probe/ -v`
Expected: PASS. If `TestEngine_RunOnce` (single anchor slot, playable) expects `StatusUp` — 1/1 = 100% > 50% → Up, still passes.

- [ ] **Step 6: Commit**

```bash
git add services/analytics/internal/probe/scorer.go services/analytics/internal/probe/scorer_test.go
git commit -m "feat(probe): per-slot pass-percentage scorer (>50 up / >0 degraded / 0 down)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task B5: Wire PopularPool + rng into the engine at startup

**Files:**
- Modify: `services/analytics/cmd/analytics-api/main.go`

**Interfaces:**
- Consumes: `NewEngine` (B3 arity), `NewHTTPPopularPool` (B2).

- [ ] **Step 1: Update the wiring block**

In `services/analytics/cmd/analytics-api/main.go`, the probe wiring block currently reads:

```go
animeSet := probe.NewHTTPAnimeSet(cfg.CatalogURL, cfg.ProbeAnchorUUID, nil, rand.New(rand.NewSource(time.Now().UnixNano()))) //nolint:gosec
engine := probe.NewEngine(
	strings.Split(cfg.ProbeProviders, ","),
	animeSet, resolver, validator,
	probe.NewPromReporter(chStore),
	func() int64 { return time.Now().Unix() },
	log,
)
```

Replace with:

```go
animeSet := probe.NewHTTPAnimeSet(cfg.CatalogURL, cfg.ProbeAnchorUUID, nil, rand.New(rand.NewSource(time.Now().UnixNano()))) //nolint:gosec
pool := probe.NewHTTPPopularPool(cfg.CatalogURL, nil)
engine := probe.NewEngine(
	strings.Split(cfg.ProbeProviders, ","),
	animeSet, resolver, validator,
	probe.NewPromReporter(chStore),
	pool, rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	func() int64 { return time.Now().Unix() },
	log,
)
```

- [ ] **Step 2: Build the whole service — expect PASS**

Run: `cd services/analytics && go build ./... && go vet ./internal/probe/...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add services/analytics/cmd/analytics-api/main.go
git commit -m "feat(probe): wire HTTPPopularPool + rng into the probe engine

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase C — Player hacker-mode availability tooltip (frontend)

> All paths under `frontend/web/src/`. Run all FE commands from `frontend/web/`.

### Task C1: `scraperApi` gains an `exclusive` param

**Files:**
- Modify: `frontend/web/src/api/client.ts`

**Interfaces:**
- Produces: `scraperApi.getEpisodes(animeId, prefer?, exclusive?)`, `getServers(..., exclusive?)`, `getStream(..., exclusive?)` adding `exclusive: 'true'` to params when truthy.

- [ ] **Step 1: Edit `scraperApi`**

Replace the `scraperApi` object (lines ~745-769) with:

```ts
export const scraperApi = {
  getEpisodes: (animeId: string, prefer?: string, exclusive?: boolean) =>
    apiClient.get(`/anime/${animeId}/scraper/episodes`, {
      params: { ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  getServers: (animeId: string, episodeId: string, prefer?: string, exclusive?: boolean) =>
    apiClient.get(`/anime/${animeId}/scraper/servers`, {
      params: { episode: episodeId, ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  getStream: (
    animeId: string,
    episodeId: string,
    serverId: string,
    category: 'sub' | 'dub',
    prefer?: string,
    exclusive?: boolean,
  ) =>
    apiClient.get(`/anime/${animeId}/scraper/stream`, {
      params: { episode: episodeId, server: serverId, category, ...(prefer && { prefer }), ...(exclusive && { exclusive: 'true' }) },
    }),
  getHealth: () => apiClient.get(`/anime/_/scraper/health`),
}
```

- [ ] **Step 2: Type-check — expect PASS**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no new errors (existing `getEpisodes(id, prefer)` calls still type-check — `exclusive` is optional).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/api/client.ts
git commit -m "feat(web): scraperApi optional exclusive param

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task C2: `useProviderAvailability` composable

**Files:**
- Create: `frontend/web/src/composables/aePlayer/useProviderAvailability.ts`
- Test: `frontend/web/src/composables/aePlayer/useProviderAvailability.spec.ts`

**Interfaces:**
- Produces:
  ```ts
  type AvailReason = 'not_found' | 'cdn_unreachable'
  interface ProviderAvailability { available: boolean; reason?: AvailReason }
  function useProviderAvailability(animeId: Ref<string>): {
    get(providerId: string): ProviderAvailability | undefined
    checkExists(providerId: string): Promise<void>     // exclusive getEpisodes; 404 → not_found
    markCdnUnreachable(providerId: string): void        // called on resolve/playback failure
    reset(): void
  }
  ```

- [ ] **Step 1: Write the failing test**

Create `useProviderAvailability.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref, nextTick } from 'vue'
import { useProviderAvailability } from './useProviderAvailability'

const getEpisodes = vi.fn()
vi.mock('@/api/client', () => ({
  scraperApi: { getEpisodes: (...a: unknown[]) => getEpisodes(...a) },
}))

describe('useProviderAvailability', () => {
  beforeEach(() => getEpisodes.mockReset())

  it('checkExists records not_found on a 404', async () => {
    getEpisodes.mockRejectedValueOnce({ response: { status: 404 } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: false, reason: 'not_found' })
    // exclusive=true was requested
    expect(getEpisodes).toHaveBeenCalledWith('anime1', 'gogoanime', true)
  })

  it('checkExists records available on 200 with episodes', async () => {
    getEpisodes.mockResolvedValueOnce({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: true })
  })

  it('markCdnUnreachable overrides to cdn_unreachable', () => {
    const a = useProviderAvailability(ref('anime1'))
    a.markCdnUnreachable('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: false, reason: 'cdn_unreachable' })
  })

  it('caches checkExists (one request per provider)', async () => {
    getEpisodes.mockResolvedValue({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    await a.checkExists('gogoanime')
    expect(getEpisodes).toHaveBeenCalledTimes(1)
  })

  it('resets cache when anime changes', async () => {
    getEpisodes.mockResolvedValue({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const id = ref('anime1')
    const a = useProviderAvailability(id)
    await a.checkExists('gogoanime')
    id.value = 'anime2'
    await nextTick()
    expect(a.get('gogoanime')).toBeUndefined()
  })
})

describe('overlayAvailability', () => {
  const t = (k: string) => k
  const row = { def: { id: 'gogoanime', name: 'GogoAnime', scraper: true }, state: 'active' } as unknown as import('@/types/aePlayer').ProviderRow

  it('passes through when available or unknown', () => {
    expect(overlayAvailability(row, undefined, t)).toBe(row)
    expect(overlayAvailability(row, { available: true }, t)).toBe(row)
  })
  it('not_found → irrelevant + lacks-anime reason', () => {
    const r = overlayAvailability(row, { available: false, reason: 'not_found' }, t)
    expect(r.state).toBe('irrelevant')
    expect(r.reason).toBe('player.sources.providerLacksAnime')
  })
  it('cdn_unreachable → down + cdn reason', () => {
    const r = overlayAvailability(row, { available: false, reason: 'cdn_unreachable' }, t)
    expect(r.state).toBe('down')
    expect(r.reason).toBe('player.sources.providerCdnUnreachable')
  })
})
```

Update the import line at the top of the spec to also import the helper:

```ts
import { useProviderAvailability, overlayAvailability } from './useProviderAvailability'
```

- [ ] **Step 2: Run it — expect FAIL** (module not found).

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderAvailability.spec.ts`

- [ ] **Step 3: Implement**

Create `useProviderAvailability.ts`:

```ts
import { ref, watch, type Ref } from 'vue'
import { scraperApi } from '@/api/client'

export type AvailReason = 'not_found' | 'cdn_unreachable'
export interface ProviderAvailability {
  available: boolean
  reason?: AvailReason
}

/**
 * Hacker-mode-only, lazy + cached per-provider availability for the current
 * anime. `checkExists` does a single no-failover `getEpisodes(exclusive=true)`
 * "search" — a 404 means the provider genuinely lacks this anime. A resolve or
 * playback failure on a provider that DOES have it is recorded via
 * `markCdnUnreachable`. Never invoked for casual users (gated at the call site).
 */
export function useProviderAvailability(animeId: Ref<string>) {
  const cache = ref(new Map<string, ProviderAvailability>())
  const inflight = new Map<string, Promise<void>>()

  function reset() {
    cache.value = new Map()
    inflight.clear()
  }
  watch(animeId, reset)

  function get(providerId: string): ProviderAvailability | undefined {
    return cache.value.get(providerId)
  }

  function set(providerId: string, v: ProviderAvailability) {
    const next = new Map(cache.value)
    next.set(providerId, v)
    cache.value = next
  }

  function markCdnUnreachable(providerId: string) {
    // Do not downgrade a known not_found (provider truly lacks the title).
    if (cache.value.get(providerId)?.reason === 'not_found') return
    set(providerId, { available: false, reason: 'cdn_unreachable' })
  }

  function checkExists(providerId: string): Promise<void> {
    if (cache.value.has(providerId)) return Promise.resolve()
    const existing = inflight.get(providerId)
    if (existing) return existing
    const animeForReq = animeId.value
    const p = scraperApi
      .getEpisodes(animeForReq, providerId, true)
      .then((resp: { data?: { data?: { episodes?: unknown[] } } }) => {
        if (animeForReq !== animeId.value) return // anime changed mid-flight
        const eps = resp.data?.data?.episodes ?? []
        set(providerId, eps.length > 0 ? { available: true } : { available: false, reason: 'not_found' })
      })
      .catch((err: { response?: { status?: number } }) => {
        if (animeForReq !== animeId.value) return
        if (err?.response?.status === 404) set(providerId, { available: false, reason: 'not_found' })
        // 502/other: leave unknown — a real "cdn unreachable for this anime"
        // is recorded at playback time via markCdnUnreachable, not here.
      })
      .finally(() => inflight.delete(providerId))
    inflight.set(providerId, p)
    return p
  }

  return { get, checkExists, markCdnUnreachable, reset }
}

/**
 * Pure row overlay: maps an availability verdict onto a ProviderRow so the
 * SourcePanel/ProviderChip render the right state + tooltip. Returns the row
 * unchanged when available/unknown. Extracted so it is unit-testable without a
 * full player mount.
 */
export function overlayAvailability(
  row: ProviderRow,
  av: ProviderAvailability | undefined,
  t: (k: string) => string,
): ProviderRow {
  if (!av || av.available) return row
  return av.reason === 'not_found'
    ? { def: row.def, state: 'irrelevant', reason: t('player.sources.providerLacksAnime') }
    : { def: row.def, state: 'down', reason: t('player.sources.providerCdnUnreachable') }
}
```

Add the type import at the top of the file: `import type { ProviderRow } from '@/types/aePlayer'`.

- [ ] **Step 4: Run it — expect PASS.** `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderAvailability.spec.ts`

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/aePlayer/useProviderAvailability.ts frontend/web/src/composables/aePlayer/useProviderAvailability.spec.ts
git commit -m "feat(web): useProviderAvailability — hacker-mode per-provider availability cache

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task C3: Wire availability into AePlayer + tooltip + i18n

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/locales/__tests__/locale-parity.spec.ts` (must stay green)

**Interfaces:**
- Consumes: `useProviderAvailability` (C2), existing `displayRows`, `onSelectProvider`, `advanceToNextSource`, `state.hackerMode`, the anime-change watcher, `PROVIDER_REGISTRY`.

- [ ] **Step 1: Add the i18n keys (all three locales)**

Add to the `player.sources` object in `en.json`:

```json
"providerLacksAnime": "This provider doesn't have this anime",
"providerCdnUnreachable": "Provider CDN is unreachable for this anime"
```

`ru.json` (same keys):

```json
"providerLacksAnime": "У этого источника нет этого аниме",
"providerCdnUnreachable": "CDN источника недоступен для этого аниме"
```

`ja.json` (same keys):

```json
"providerLacksAnime": "この配信元にこのアニメはありません",
"providerCdnUnreachable": "この配信元のCDNにこのアニメで接続できません"
```

> Place each pair inside the existing `player.sources` block (after `"subSelectable"`), preserving JSON validity. Run `bunx vitest run src/locales/__tests__/locale-parity.spec.ts` to confirm key/placeholder parity.

- [ ] **Step 2: Instantiate the composable + helper in `AePlayer.vue` `<script setup>`**

Near the existing `const resolver = useProviderResolver()` (line ~420), add:

```ts
import { useProviderAvailability } from '@/composables/aePlayer/useProviderAvailability'
import { PROVIDER_REGISTRY } from '@/composables/aePlayer/useProviderHealth' // or wherever PROVIDER_REGISTRY is exported

const animeIdRef = computed(() => props.animeId)
const availability = useProviderAvailability(animeIdRef)
const scraperProviderIds = new Set(PROVIDER_REGISTRY.filter((d) => d.scraper).map((d) => d.id))
```

> Confirm `PROVIDER_REGISTRY`'s export path with `grep -rn "export const PROVIDER_REGISTRY" frontend/web/src`.

- [ ] **Step 3: Fire `checkExists` on hacker-mode manual pick**

In `onSelectProvider(id)` (line ~1443), after `state.setProvider(id, '')`, add:

```ts
  if (state.hackerMode.value && scraperProviderIds.has(id)) {
    void availability.checkExists(id)
  }
```

- [ ] **Step 4: Record `cdn_unreachable` at the hacker-mode failover funnel**

In `advanceToNextSource`, inside the `if (state.hackerMode.value) {` block (line ~707), before `return false`, add:

```ts
    if (scraperProviderIds.has(provider)) {
      availability.markCdnUnreachable(provider)
    }
```

> `provider` is the failing provider in scope at that point (the function's subject). Confirm the in-scope variable name (`provider`) with the surrounding code.

- [ ] **Step 5: Reset availability on anime change**

In the `watch(() => props.animeId, ...)` block (line ~645), add `availability.reset()` alongside the other resets (the composable also self-resets via its internal watch — this is belt-and-suspenders and harmless).

- [ ] **Step 6: Merge availability into `displayRows` (hacker mode only)**

Replace the `displayRows` computed (line ~607) with one that also overlays availability:

```ts
const displayRows = computed<ProviderRow[]>(() =>
  rows.value.map((r) => {
    // Existing `ae` library-gating (unchanged).
    if (r.def.id === 'ae' && aeAvailable.value !== true) {
      return {
        def: r.def,
        state: 'irrelevant' as const,
        reason: aeAvailable.value === false
          ? 'Not in the AnimeEnigma library yet'
          : 'Checking the AnimeEnigma library…',
      }
    }
    // Hacker-mode per-anime availability overlay for scraper providers.
    if (state.hackerMode.value && scraperProviderIds.has(r.def.id)) {
      return overlayAvailability(r, availability.get(r.def.id), t)
    }
    return r
  }),
)
```

Update the C2 import line to also pull the helper:

```ts
import { useProviderAvailability, overlayAvailability } from '@/composables/aePlayer/useProviderAvailability'
```

> This needs the i18n `t`. Per memory AePlayer is `$t`-template-only, so add `import { useI18n } from 'vue-i18n'` + `const { t } = useI18n()` in `<script setup>`. Existing AePlayer specs that mount the component must then provide a vue-i18n mock — see the memory note on `AePlayer.room.spec` ([[project_wt_inplayer_button_wired]]).

- [ ] **Step 7: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: clean (resolve any missing-import errors surfaced).

- [ ] **Step 8: Confirm overlay coverage (already unit-tested)**

The row-overlay mapping is covered by the `overlayAvailability` tests added in C2 Step 1 (pure helper — no heavy mount needed). No new AePlayer-mount test is required; the `displayRows` wiring is guarded by the `vue-tsc` type-check in Step 7. If any EXISTING AePlayer-mount spec now fails because `useI18n()` was added (Step 6), add a `vue-i18n` mock to that spec's `global.plugins`/`mocks` (mirror `AePlayer.room.spec`).

- [ ] **Step 9: Run FE tests — expect PASS**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/ src/locales/__tests__/locale-parity.spec.ts src/components/player/aePlayer/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/src/composables/aePlayer/useProviderAvailability.ts frontend/web/src/composables/aePlayer/useProviderAvailability.spec.ts
git commit -m "feat(web): hacker-mode per-provider availability tooltip (lacks-anime vs cdn-unreachable)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase D — Scraper framework doc + memory

### Task D1: `docs/scraper-framework.md` + memory pointer

**Files:**
- Create: `docs/scraper-framework.md`
- Modify: `/root/.claude/projects/-data-animeenigma/memory/MEMORY.md` + a new memory file

- [ ] **Step 1: Write `docs/scraper-framework.md`**

Cover, with code + DB links, each section verified against the codebase as you write it:
1. **Overview** — the OurEnglish failover orchestrator; default chain `gogoanime → animepahe → allanime → animefever → miruro → nineanime` (+optional animekai); degraded providers excluded from auto-failover (`services/scraper/internal/service/orchestrator.go:orderedProviders`).
2. **Route family** — `GET /api/anime/{uuid}/scraper/episodes|servers|stream|health`, the `prefer` (soft) vs new `exclusive=true` (no-failover) semantics, with the catalog passthrough (`services/catalog/internal/handler/scraper.go`) → scraper handler (`services/scraper/internal/handler/scraper.go`).
3. **Typed errors → HTTP** — `ErrNotFound`→404 `NOT_FOUND`, `ErrProviderDown`→502 `PROVIDER_DOWN`, `ErrExtractFailed`→502 `EXTRACT_FAILED` (`services/scraper/internal/domain/errors.go`, `writeOrchestratorError`).
4. **Providers + embeds** — `services/scraper/internal/providers/{name}/`, `services/scraper/internal/embeds/`; FindID → ListEpisodes → ListServers → GetStreamGated pipeline.
5. **Stealth sidecar** — `services/stealth-scraper/` (Camoufox) for gogoanime CF-gated CDNs (engine=browser); link [[project_stealth_scraper_camoufox]].
6. **DB roster** — Postgres `stream_providers` (catalog) holding ALL providers + `status` enum (`enabled|degraded|disabled`) + `scraper_operated`; the catalog seeds + serves `/internal/scraper/providers`; the `GET /api/anime/{uuid}/capabilities` ranked per-anime view.
7. **Playability probe** — analytics `POST /internal/probe/run`; `exclusive=true` resolve + ffprobe through the HLS proxy; not-found re-roll from `/api/anime/popular`; per-slot `>50/>0/0` scorer; sinks `probe_provider_up`, `probe_runs_total`, ClickHouse `analytics.probe_runs`. Link [[project_unified_probe_live_first_run]].

- [ ] **Step 2: Add a memory file + index pointer**

Create `/root/.claude/projects/-data-animeenigma/memory/reference_scraper_framework_doc.md`:

```markdown
---
name: reference_scraper_framework_doc
description: docs/scraper-framework.md — canonical how-scraping-works doc; keep it updated
metadata:
  type: reference
---

`docs/scraper-framework.md` is the canonical map of how scraping works:
failover orchestrator + provider chain, the `/api/anime/{uuid}/scraper/*`
route family with `prefer` (soft) vs `exclusive=true` (no-failover) semantics,
typed errors → HTTP, providers+embeds, the Camoufox stealth sidecar, the
Postgres `stream_providers` roster (+status enum), and the analytics playback
probe. UPDATE IT whenever provider chain order, the route family, the error
taxonomy, or the probe scoring changes. Related: [[project_unified_probe_live_first_run]],
[[project_stealth_scraper_camoufox]], [[project_scraper_provider_config_db]].
```

Add to `MEMORY.md` (top of the list):

```markdown
## Scraper framework doc — docs/scraper-framework.md (canonical, keep updated)
- [reference_scraper_framework_doc.md](reference_scraper_framework_doc.md) — how scraping works end-to-end: failover chain, route family + prefer/exclusive, typed errors→HTTP, providers+embeds, stealth sidecar, stream_providers roster, playability probe.
```

- [ ] **Step 3: Commit**

```bash
git add docs/scraper-framework.md
git commit -m "docs(scraper): canonical scraper framework reference

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Final Verification & Deploy

- [ ] **All Go packages build + test:**
  `cd services/scraper && go build ./... && go test ./...`
  `cd services/catalog && go build ./... && go test ./...`
  `cd services/analytics && go build ./... && go test ./...`
- [ ] **Frontend gates:**
  `cd frontend/web && bunx vue-tsc --noEmit && bunx vitest run src/composables/aePlayer/ src/api/ src/locales/__tests__/ src/components/player/aePlayer/ && bash scripts/design-system-lint.sh`
- [ ] **Live smoke (after deploy):** `exclusive=true` honesty + a fresh probe run:
  ```bash
  # not-found is honest (404) under exclusive; failover still works without it:
  docker exec animeenigma-catalog sh -c 'wget -qS -O- "http://localhost:8081/api/anime/<euphonium2-uuid>/scraper/episodes?prefer=gogoanime&exclusive=true" 2>&1 | head'
  # rerun the probe and confirm gogoanime is now scored by pass-% (not any-pass):
  docker exec animeenigma-analytics sh -c "wget -qO- -T300 --post-data='' http://127.0.0.1:8092/internal/probe/run"
  docker exec animeenigma-analytics sh -c 'wget -qO- http://127.0.0.1:8092/metrics' | grep -E '^probe_provider_(up|status)\{provider="gogoanime"'
  ```
- [ ] **Deploy via `/animeenigma-after-update`** (redeploy scraper + catalog + analytics + web; Russian Trump-mode changelog entry; commit + push). Because work is on the deployed-probe branch, coordinate the merge to `main` per the Global Constraints base note.

---

## Self-Review

**Spec coverage:** D1 backend `exclusive` → A1/A2/A3 ✓. D3 probe resolves exclusive → B1 ✓. D4 not_found classification → B1 (reuses `StageSearch`+`ReasonZeroMatch`) ✓. D5 re-roll → B2/B3 ✓. D6 thresholds → B4 ✓. D7 hacker-mode gating → C3 (gated at call sites) ✓. D8 cached tooltip → C2/C3 ✓. D9 docs+memory → D1 ✓. §8 base-branch logistics → Global Constraints ✓.

**Deviations from spec (intentional, documented above):** (a) reuse `ReasonZeroMatch`/`StageSearch` instead of a new `not_found` reason (avoids the maintenance-prompt coupling); (b) `exclusive` + empty `prefer` yields the natural `503 no-providers` rather than `400` (FE/probe always pass a valid `prefer`); (c) FE needs `exclusive` only on `getEpisodes` — `cdn_unreachable` is recorded at the `advanceToNextSource` hacker funnel, not via resolver surgery.
