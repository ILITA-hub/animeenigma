# Scraper P2 — AllAnime sub/dub/raw categories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the AllAnime provider honestly serve and advertise sub/dub categories — `GetStream` honors the requested `category` (today it hardcodes `translationType:"sub"`, so AllAnime can *never* play a dub even when one exists), and `ListServers` returns dub-tagged servers when the title has a dub.

**Architecture:** Thread an AllAnime `translationType` ("sub"|"dub"|"raw") derived from `domain.Category` through `buildSourcesVariables` → `fetchSources` → `GetStream`, and key the server/stream caches by it so sub and dub don't collide. `ListEpisodes` already fetches the show's `availableEpisodesDetail.{Sub,Dub,Raw}` — cache which categories are non-empty there, so `ListServers` probes only the categories that actually exist (no extra call for the common sub-only title).

**Tech Stack:** Go, `gorm.io`-free pure provider code, `net/http/httptest` + JSON fixtures under `services/scraper/internal/providers/allanime/testdata/`.

**Scope note (grounded in exploration):** P2's spec ambition was "improve category detection for all providers," but only **AllAnime** has an untapped, reliable category signal. gogoanime (`-dub` slug), animepahe (`data-audio`), and miruro (episodes-map key) are **already honest**. nineanime (single MP4 embed), animefever (no audio field), and 18anime (raw JP-only) have **no upstream category signal** — their P1 trait `supports_dub=false` already encodes that. So this plan touches AllAnime only. Quality is likewise already handled where it can be (miruro/animepahe extract it; the rest lives in the HLS master read at play time) — no quality work here.

**Convention:** every `git commit` includes the standard co-author trailer:
```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
```
Path-scope every `git add` (shared working tree with concurrent committers); no `git add -A`; no push (the controller lands commits).

---

## File Structure

**Modify (all under `services/scraper/internal/providers/allanime/`):**
- `queries.go` — `buildSourcesVariables` takes a `translationType` arg.
- `client.go` — add `translationTypeFor`; `fetchSources` + `materializeServers` take the type/category; `GetStream` honors `category`; `ListServers` probes per available category; `ListEpisodes` caches available categories.
- `cache.go` — server + stream cache keys gain a `tt` (translationType) segment; new `getCategories`/`setCategories`.

**Tests:**
- `queries_test.go` (create if absent) — `buildSourcesVariables` emits the requested type.
- `client_test.go` — GetStream(dub) path, ListServers multi-category, sub-only fallback. Reuse the existing `newTestProvider`/`httptest` pattern and `testdata/` fixtures (add a dub fixture).

---

## Task 1: Thread translationType; GetStream honors category; cache keyed by type

**Files:**
- Modify: `services/scraper/internal/providers/allanime/queries.go`
- Modify: `services/scraper/internal/providers/allanime/cache.go`
- Modify: `services/scraper/internal/providers/allanime/client.go`
- Test: `services/scraper/internal/providers/allanime/queries_test.go` (create), `client_test.go` (extend)

- [ ] **Step 1: Write the failing query test** — create/append `services/scraper/internal/providers/allanime/queries_test.go`:

```go
package allanime

import (
	"strings"
	"testing"
)

func TestBuildSourcesVariables_TranslationType(t *testing.T) {
	for _, tt := range []string{"sub", "dub", "raw"} {
		got, err := buildSourcesVariables("SHOW123", "5", tt)
		if err != nil {
			t.Fatalf("buildSourcesVariables(%q): %v", tt, err)
		}
		if !strings.Contains(got, `"translationType":"`+tt+`"`) {
			t.Errorf("translationType %q not in vars: %s", tt, got)
		}
	}
	// Empty defaults to sub.
	got, _ := buildSourcesVariables("SHOW123", "5", "")
	if !strings.Contains(got, `"translationType":"sub"`) {
		t.Errorf("empty type should default to sub: %s", got)
	}
}
```

- [ ] **Step 2: Run it, confirm failure**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/providers/allanime/ -run TestBuildSourcesVariables_TranslationType -v`
Expected: FAIL — `too many arguments in call to buildSourcesVariables`

- [ ] **Step 3: Make `buildSourcesVariables` take the type** — in `queries.go`, replace the whole `buildSourcesVariables` func with:

```go
// buildSourcesVariables encodes the variables payload for the sources query.
// translationType is one of "sub" | "dub" | "raw" (empty defaults to "sub").
func buildSourcesVariables(showID, episodeString, translationType string) (string, error) {
	if translationType == "" {
		translationType = "sub"
	}
	v := map[string]any{
		"showId":          showID,
		"translationType": translationType,
		"episodeString":   episodeString,
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("buildSourcesVariables: %w", err)
	}
	return string(b), nil
}
```

- [ ] **Step 4: Run the query test, confirm pass**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/providers/allanime/ -run TestBuildSourcesVariables_TranslationType -v`
Expected: PASS (the rest of the package won't compile yet — that's fine for this focused run; the build is fixed in later steps).

- [ ] **Step 5: Add `translationTypeFor` + update `fetchSources` + `materializeServers`** — in `client.go`:

Add near the top-level helpers (e.g. just above `materializeServers`):

```go
// translationTypeFor maps a domain.Category to AllAnime's translationType enum.
func translationTypeFor(c domain.Category) string {
	switch c {
	case domain.CategoryDub:
		return "dub"
	case domain.CategoryRaw:
		return "raw"
	default:
		return "sub"
	}
}
```

Change `fetchSources` to take a `translationType` arg (find `func (p *Provider) fetchSources(ctx context.Context, showID, ep string)`):

```go
func (p *Provider) fetchSources(ctx context.Context, showID, ep, translationType string) ([]sourceURL, error) {
	vars, err := buildSourcesVariables(showID, ep, translationType)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "allanime: buildSourcesVariables")
	}
	// ... rest of the function body UNCHANGED ...
```

Change `materializeServers` to tag with a passed category (replace the whole func):

```go
func materializeServers(sources []sourceURL, cat domain.Category) []domain.Server {
	out := make([]domain.Server, 0, len(sources))
	for _, s := range sources {
		name := s.SourceName
		if name == "" {
			name = "Default"
		}
		out = append(out, domain.Server{ID: name, Name: name, Type: cat})
	}
	return out
}
```

- [ ] **Step 6: Key the server + stream caches by translationType** — in `cache.go`:

Replace `keyServers`, `getServers`, `setServers`:

```go
func keyServers(showID, ep, tt string) string {
	return fmt.Sprintf("scraper:allanime:servers:%s:%s:%s", showID, ep, tt)
}

func (l *cacheLayer) getServers(ctx context.Context, showID, ep, tt string) ([]sourceURL, bool) {
	var out []sourceURL
	if err := l.c.Get(ctx, keyServers(showID, ep, tt), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setServers(ctx context.Context, showID, ep, tt string, src []sourceURL) {
	if len(src) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyServers(showID, ep, tt), src, serversCacheTTL)
}
```

Replace `keyStream`, `getStream`, `setStream` (keep `cachedStream`/`cachedSubtitle` types unchanged):

```go
func keyStream(showID, ep, tt, server string) string {
	return fmt.Sprintf("scraper:allanime:stream:%s:%s:%s:%s", showID, ep, tt, server)
}

func (l *cacheLayer) getStream(ctx context.Context, showID, ep, tt, server string) (*cachedStream, bool) {
	var out cachedStream
	if err := l.c.Get(ctx, keyStream(showID, ep, tt, server), &out); err == nil && out.URL != "" {
		return &out, true
	}
	return nil, false
}

func (l *cacheLayer) setStream(ctx context.Context, showID, ep, tt, server string, s *cachedStream) {
	if s == nil || s.URL == "" {
		return
	}
	_ = l.c.Set(ctx, keyStream(showID, ep, tt, server), s, streamTTLCap)
}
```

- [ ] **Step 7: Update all call sites in `client.go`** — grep first: `grep -n "fetchSources(\|materializeServers(\|\.getServers(\|\.setServers(\|\.getStream(\|\.setStream(" services/scraper/internal/providers/allanime/client.go`.

In `ListServers` (keep it sub-only for now — Task 2 makes it multi-category):
```go
	if hit, ok := p.cache.getServers(ctx, showID, ep, "sub"); ok {
		p.markStage(health.StageServers, nil)
		return materializeServers(hit, domain.CategorySub), nil
	}

	sources, err := p.fetchSources(ctx, showID, ep, "sub")
	// ... unchanged error/empty handling ...

	p.cache.setServers(ctx, showID, ep, "sub", sources)
	p.markStage(health.StageServers, nil)
	return materializeServers(sources, domain.CategorySub), nil
```

In `GetStream`, derive `tt` from `category` and thread it through cache + fetch. After `showID, ep := splitEpisodeID(episodeID)` (and its validity check), add `tt := translationTypeFor(category)`, then:
```go
	if hit, ok := p.cache.getStream(ctx, showID, ep, tt, serverID); ok {
		p.markStage(health.StageStream, nil)
		return cachedToStream(hit), nil
	}

	sources, ok := p.cache.getServers(ctx, showID, ep, tt)
	if !ok {
		var ferr error
		sources, ferr = p.fetchSources(ctx, showID, ep, tt)
		if ferr != nil {
			p.markStage(health.StageStream, ferr)
			return nil, ferr
		}
		p.cache.setServers(ctx, showID, ep, tt, sources)
	}
```
and at the end where the stream is cached:
```go
	p.cache.setStream(ctx, showID, ep, tt, serverID, cached)
```

- [ ] **Step 8: Write the GetStream-honors-category test** — append to `client_test.go`. Mirror the existing GetStream test's fixture/server setup; the new assertion is that the upstream request carries `translationType":"dub"` when `category=dub`. Add:

```go
func TestGetStream_HonorsDubCategory(t *testing.T) {
	var gotDub bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "dub") || strings.Contains(r.URL.RawQuery, "%22dub%22") {
			gotDub = true
		}
		w.Header().Set("Content-Type", "application/json")
		// Minimal sources response with one direct mp4 (no embed probe needed).
		_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/dub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, _ = p.GetStream(context.Background(), "", "SHOW123:1", "Default", domain.CategoryDub)
	if !gotDub {
		t.Error("GetStream(dub) did not send translationType=dub upstream")
	}
}
```
> NOTE: adapt `newTestProvider(t, srv)` and the episodeID format (`splitEpisodeID`) to the EXACT shapes the existing `client_test.go` uses — read an existing GetStream test first and mirror its provider construction, episodeID, and how `classify`/probe is stubbed so the direct-mp4 source is accepted without a live probe. If the probe can't be stubbed in-test, assert on `gotDub` only (the upstream `translationType`), which is the point of this task.

- [ ] **Step 9: Build + test the package**

Run: `cd /data/animeenigma/services/scraper && gofmt -w ./internal/providers/allanime/ && go build ./... && go test ./internal/providers/allanime/ && go vet ./internal/providers/allanime/`
Expected: builds clean, all allanime tests pass (existing + new).

- [ ] **Step 10: Commit**

```bash
git add services/scraper/internal/providers/allanime/queries.go services/scraper/internal/providers/allanime/queries_test.go services/scraper/internal/providers/allanime/cache.go services/scraper/internal/providers/allanime/client.go services/scraper/internal/providers/allanime/client_test.go
git commit -m "feat(scraper): allanime GetStream honors sub/dub/raw category

Thread translationType (from domain.Category) through buildSourcesVariables/
fetchSources/GetStream and key server+stream caches by it, so AllAnime can
serve a dub/raw stream when requested (was hardcoded translationType=sub).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: ListServers discovers dub servers per-title

**Files:**
- Modify: `services/scraper/internal/providers/allanime/cache.go` (categories cache)
- Modify: `services/scraper/internal/providers/allanime/client.go` (`ListEpisodes` caches available categories; `ListServers` probes per available category)
- Test: `services/scraper/internal/providers/allanime/client_test.go`; add a dub fixture under `testdata/`.

- [ ] **Step 1: Add a categories cache** — in `cache.go`, after the episodes cache section, add:

```go
// --- available categories (which of sub/dub exist for a show) -------------
// Populated by ListEpisodes (free — it already fetches availableEpisodesDetail)
// and read by ListServers so it probes only categories that actually exist.

func keyCategories(showID string) string {
	return fmt.Sprintf("scraper:allanime:cats:%s", showID)
}

func (l *cacheLayer) getCategories(ctx context.Context, showID string) ([]string, bool) {
	var out []string
	if err := l.c.Get(ctx, keyCategories(showID), &out); err == nil && len(out) > 0 {
		return out, true
	}
	return nil, false
}

func (l *cacheLayer) setCategories(ctx context.Context, showID string, cats []string) {
	if len(cats) == 0 {
		return
	}
	_ = l.c.Set(ctx, keyCategories(showID), cats, episodesCacheTTL)
}
```

- [ ] **Step 2: Cache available categories in `ListEpisodes`** — read the current `ListEpisodes` (it unmarshals `resp.Data.Show.AvailableEpisodesDetail` and uses `.Sub`). Right after that detail is available, add (using whatever the local variable for the detail is — confirm by reading; it's `resp.Data.Show.AvailableEpisodesDetail`):

```go
	// Cache which EN categories the show actually has, so ListServers probes
	// only those (raw is excluded — that's the Raw player's domain, not OurEnglish).
	detail := resp.Data.Show.AvailableEpisodesDetail
	cats := make([]string, 0, 2)
	if len(detail.Sub) > 0 {
		cats = append(cats, "sub")
	}
	if len(detail.Dub) > 0 {
		cats = append(cats, "dub")
	}
	p.cache.setCategories(ctx, showID, cats)
```
> Place this so it does not change the existing `raw := resp.Data.Show.AvailableEpisodesDetail.Sub` logic — just add the caching alongside. If the function already binds the detail to a local, reuse it instead of re-reading.

- [ ] **Step 3: Write the failing multi-category ListServers test** — add a dub fixture `services/scraper/internal/providers/allanime/testdata/sources_dub_ep1.json`:

```json
{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/dub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}
```

Add to `client_test.go` a test that drives ListServers when the categories cache says both sub+dub exist, with an httptest server that returns sub sources for `translationType=sub` and the dub fixture for `translationType=dub`, and asserts the returned servers include one `domain.CategorySub` AND one `domain.CategoryDub`:

```go
func TestListServers_SubAndDub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "dub") || strings.Contains(r.URL.RawQuery, "%22dub%22") {
			_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/dub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"episode":{"episodeString":"1","sourceUrls":[{"sourceUrl":"https://cdn.allanime.day/sub-ep1.mp4","sourceName":"Default","type":"player","priority":9,"fileExtenstion":"mp4","subtitles":[]}]}}}`))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	// Seed the categories cache (ListEpisodes would normally do this).
	p.cache.setCategories(context.Background(), "SHOW123", []string{"sub", "dub"})

	servers, err := p.ListServers(context.Background(), "", "SHOW123:1")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	var sawSub, sawDub bool
	for _, s := range servers {
		if s.Type == domain.CategorySub {
			sawSub = true
		}
		if s.Type == domain.CategoryDub {
			sawDub = true
		}
	}
	if !sawSub || !sawDub {
		t.Errorf("want both sub and dub servers; got sub=%v dub=%v (%+v)", sawSub, sawDub, servers)
	}
}
```
> Adapt `newTestProvider`/`p.cache` access to the real test helpers. If `p.cache` is unexported and unreachable from the external test package, either (a) make the test `package allanime` (white-box) like `queries_test.go`, or (b) drive category caching through a real `ListEpisodes` call against a fixture. Prefer white-box for directness.

- [ ] **Step 4: Run it, confirm failure**

Run: `cd /data/animeenigma/services/scraper && go test ./internal/providers/allanime/ -run TestListServers_SubAndDub -v`
Expected: FAIL — only sub servers returned (ListServers still sub-only).

- [ ] **Step 5: Make `ListServers` probe per available category** — replace the body of `ListServers` (keep the `splitEpisodeID` + invalid-ID guard) with:

```go
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	showID, ep := splitEpisodeID(episodeID)
	if showID == "" || ep == "" {
		err := domain.WrapExtractFailed(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"allanime: ListServers")
		p.markStage(health.StageServers, err)
		return nil, err
	}

	// Probe only the categories the show actually has (cached by ListEpisodes).
	// Cold miss → sub only (the conservative default; ListEpisodes precedes
	// ListServers in the normal flow).
	cats, ok := p.cache.getCategories(ctx, showID)
	if !ok || len(cats) == 0 {
		cats = []string{"sub"}
	}

	var all []domain.Server
	var subErr error
	for _, tt := range cats {
		cat := domain.CategorySub
		if tt == "dub" {
			cat = domain.CategoryDub
		}
		srcs, hit := p.cache.getServers(ctx, showID, ep, tt)
		if !hit {
			fetched, ferr := p.fetchSources(ctx, showID, ep, tt)
			if ferr != nil || len(fetched) == 0 {
				if tt == "sub" && ferr != nil {
					subErr = ferr // sub failure is the meaningful error
				}
				continue // a dub probe that errors/empties is non-fatal
			}
			p.cache.setServers(ctx, showID, ep, tt, fetched)
			srcs = fetched
		}
		all = append(all, materializeServers(srcs, cat)...)
	}

	if len(all) == 0 {
		err := subErr
		if err == nil {
			err = domain.WrapExtractFailed(
				fmt.Errorf("empty sourceUrls for %s ep %s", showID, ep),
				"allanime: ListServers")
		}
		p.markStage(health.StageServers, err)
		return nil, err
	}

	p.markStage(health.StageServers, nil)
	return all, nil
}
```

- [ ] **Step 6: Run the new test + the full package**

Run: `cd /data/animeenigma/services/scraper && gofmt -w ./internal/providers/allanime/ && go test ./internal/providers/allanime/ -run TestListServers && go test ./internal/providers/allanime/ && go vet ./internal/providers/allanime/`
Expected: `TestListServers_SubAndDub` passes; ALL existing allanime tests still pass (the sub-only path is preserved when cats=["sub"]). Confirm an existing sub-only ListServers test still passes (cats cache miss → ["sub"]).

- [ ] **Step 7: Build the whole service**

Run: `cd /data/animeenigma/services/scraper && go build ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add services/scraper/internal/providers/allanime/cache.go services/scraper/internal/providers/allanime/client.go services/scraper/internal/providers/allanime/client_test.go services/scraper/internal/providers/allanime/testdata/sources_dub_ep1.json
git commit -m "feat(scraper): allanime ListServers advertises dub servers per-title

ListEpisodes caches which categories the show has (free from
availableEpisodesDetail); ListServers probes only those and returns
dub-tagged servers when a dub exists. Sub-only titles are unchanged (one call).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Integration verification (after both tasks)

- [ ] **Redeploy + smoke** (from the shared tree if clean for scraper, else a HEAD worktree):

```bash
make redeploy-scraper && make health
```

- [ ] **Confirm a dub-bearing title returns dub servers** — pick an anime known to have an English dub on AllAnime, resolve its scraper episodes/servers via the gated path, and confirm the `servers` response now includes `"type":"dub"` entries (and a `category=dub` stream resolves). If you don't have a known-dub title handy, at minimum confirm sub-only titles still return sub servers and play (no regression) — the unit tests already prove the dub path.

---

## Self-Review (completed during authoring)

- **Spec coverage (P2, revised scope):** AllAnime category honesty — `GetStream` honors category (T1) ✔, `ListServers` advertises dub (T2) ✔. Other providers: documented as already-honest or upstream-limited (no code — correct, the signal doesn't exist). Quality: no actionable work (already extracted where possible / lives in HLS master) — documented.
- **Type consistency:** `buildSourcesVariables(showID, ep, tt)` ↔ `fetchSources(ctx, showID, ep, tt)` ↔ cache `getServers/setServers/getStream/setStream(... tt ...)` ↔ `materializeServers(sources, cat)` ↔ `translationTypeFor(category)` — signatures agree across both tasks. Category strings ("sub"/"dub") used consistently in cache vs `domain.Category` enums in server tags, bridged by `translationTypeFor` and the `tt=="dub"` check.
- **Placeholder scan:** none; every code step is complete. The two test steps that depend on the existing `newTestProvider`/probe-stub shape say so explicitly and tell the implementer to read an existing test and mirror it (the unavoidable fixture-dependency the plan flagged from the start).
- **Risk:** cache key shape changed (added `tt` segment) → old `scraper:allanime:servers:*`/`stream:*` keys go stale and expire (15min/5min TTL) — no flush needed. Cold-`ListServers` (no prior `ListEpisodes`) defaults to sub-only — conservative, matches today's behavior; dub still serves via `GetStream(category=dub)` once the capability layer (P3/P4) offers it.
