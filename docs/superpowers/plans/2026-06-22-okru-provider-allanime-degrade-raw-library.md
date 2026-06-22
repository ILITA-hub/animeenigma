# okru provider + allanime degrade + raw library-only — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a distinct `okru` scraper provider that resolves AllAnime's clock-free `Ok` (ok.ru) sources, degrade the CF-walled `allanime` provider, and make the JP-audio `raw` provider library-only.

**Architecture:** `okru` reuses `allanime`'s working `api.allanime.day` GraphQL discovery via a thin new exported accessor (`allanime.Provider.EpisodeSourceURLs`), filters to `Ok` sources, and resolves them with a new static `ok.ru` `data-options` extractor (no clock, no JS, no browser). allanime is flipped to `degraded` (seed + guarded migration). `raw` drops its AllAnime backend and serves JP audio from the library only.

**Tech Stack:** Go (scraper + catalog microservices), Vue 3 / TypeScript (frontend), GORM, Redis cache.

**Spec:** `docs/superpowers/specs/2026-06-22-okru-provider-allanime-degrade-raw-library-design.md`

**Working dir:** clean worktree `/tmp/ae-okru-impl` on branch `feat/okru-provider`. Run all commands from there. Go module root is the repo root; run `go test` with package paths.

---

### Task 1: allanime — exported per-episode source accessor

Lets `okru` reuse AllAnime's GraphQL discovery + cache without copying the persisted-query/decrypt code.

**Files:**
- Modify: `services/scraper/internal/providers/allanime/client.go`
- Test: `services/scraper/internal/providers/allanime/client_test.go`

- [ ] **Step 1: Write the failing test.** Append to `client_test.go`:

```go
func TestEpisodeSourceURLs_FiltersAndDecodes(t *testing.T) {
	// Upstream returns one plain "Ok" source + one "--"-encoded clock source.
	// EpisodeSourceURLs must return BOTH, decoded, with names intact.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// minimal SourceUrls response: an Ok ok.ru embed + a Default clock source
		fmt.Fprint(w, `{"data":{"episode":{"episodeString":"1","sourceUrls":[`+
			`{"sourceUrl":"https://ok.ru/videoembed/123","sourceName":"Ok","type":"iframe"},`+
			`{"sourceUrl":"https://cdn.example/x.m3u8","sourceName":"Default","type":"iframe"}`+
			`]}}}`)
	}))
	defer srv.Close()

	p, err := New(Deps{BaseURL: srv.URL, HTTP: testHTTP(t), Cache: newTestCache(), Log: logger.Default()})
	if err != nil { t.Fatal(err) }

	got, err := p.EpisodeSourceURLs(context.Background(), "SHOW:1", domain.CategorySub)
	if err != nil { t.Fatalf("EpisodeSourceURLs: %v", err) }
	if len(got) != 2 { t.Fatalf("want 2 sources, got %d: %+v", len(got), got) }
	if got[0].Name != "Ok" || got[0].URL != "https://ok.ru/videoembed/123" {
		t.Errorf("source[0] = %+v", got[0])
	}
}

func TestEpisodeSourceURLs_ForeignID_NotFound(t *testing.T) {
	p, err := New(Deps{HTTP: testHTTP(t), Cache: newTestCache(), Log: logger.Default()})
	if err != nil { t.Fatal(err) }
	_, err = p.EpisodeSourceURLs(context.Background(), "gogoanime-slug-no-colon", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

> Reuse the existing test helpers in `client_test.go` (`testHTTP`, `newTestCache`). If their names differ, match the file's existing fixtures — read the top of `client_test.go` first and mirror how other tests construct `New(Deps{...})`.

- [ ] **Step 2: Run it — expect FAIL** (`EpisodeSourceURLs` undefined):

```bash
cd /tmp/ae-okru-impl && go test ./services/scraper/internal/providers/allanime/ -run EpisodeSourceURLs -v
```

- [ ] **Step 3: Implement.** Add to `client.go` (near `materializeServers`, after `translationTypeFor`):

```go
// NamedSource is a public view of one decoded AllAnime source: its upstream
// sourceName ("Ok", "Default", "S-mp4", …) and the decoded, fully-qualified
// embed/stream URL. Exposed so sibling providers (e.g. okru) can reuse
// AllAnime's GraphQL discovery + cache and resolve a specific source family
// without duplicating the persisted-query / decrypt code.
type NamedSource struct {
	Name string
	URL  string
}

// EpisodeSourceURLs returns the decoded sources for one episode+category via the
// same fetchSources path (and Redis cache) used by ListServers/GetStream.
// episodeID is "<showID>:<episodeString>". A foreign/invalid ID → ErrNotFound,
// so a caller (okru) is skipped by the orchestrator instead of marked DOWN.
func (p *Provider) EpisodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]NamedSource, error) {
	showID, ep := splitEpisodeID(episodeID)
	if showID == "" || ep == "" {
		return nil, domain.WrapNotFound(
			fmt.Errorf("invalid episode ID %q", episodeID),
			"allanime: EpisodeSourceURLs")
	}
	tt := translationTypeFor(category)
	srcs, hit := p.cache.getServers(ctx, showID, ep, tt)
	if !hit {
		fetched, ferr := p.fetchSources(ctx, showID, ep, tt)
		if ferr != nil {
			return nil, ferr
		}
		p.cache.setServers(ctx, showID, ep, tt, fetched)
		srcs = fetched
	}
	out := make([]NamedSource, 0, len(srcs))
	for _, s := range srcs {
		name := s.SourceName
		if name == "" {
			name = "Default"
		}
		out = append(out, NamedSource{Name: name, URL: decodeSourceURL(s.SourceURL)})
	}
	return out, nil
}
```

- [ ] **Step 4: Run it — expect PASS:**

```bash
cd /tmp/ae-okru-impl && go test ./services/scraper/internal/providers/allanime/ -run EpisodeSourceURLs -v
```

- [ ] **Step 5: Commit:**

```bash
cd /tmp/ae-okru-impl && git add services/scraper/internal/providers/allanime/ && \
git commit -m "feat(allanime): export EpisodeSourceURLs accessor for source reuse"
```

---

### Task 2: ok.ru embed extractor

Static `data-options` parse (proven live 2026-06-22) → `okcdn.ru` HLS + MP4 fallbacks. No JS, no browser.

**Files:**
- Create: `services/scraper/internal/embeds/okru.go`
- Test: `services/scraper/internal/embeds/okru_test.go`

- [ ] **Step 1: Write the failing test.** Create `okru_test.go`:

```go
package embeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realistic ok.ru /videoembed page: a <div data-options="{...escaped json...}">
// whose flashvars.metadata is a JSON-encoded STRING carrying hlsManifestUrl +
// videos[]. This mirrors the live shape captured 2026-06-22.
const okruEmbedHTML = `<!DOCTYPE html><html><body>` +
	`<div data-module="OKVideo" data-options="{&quot;flashvars&quot;:{&quot;metadata&quot;:&quot;{\&quot;hlsManifestUrl\&quot;:\&quot;https://vd1.okcdn.ru/video.m3u8?x=1\&quot;,\&quot;videos\&quot;:[{\&quot;name\&quot;:\&quot;hd\&quot;,\&quot;url\&quot;:\&quot;https://vd1.okcdn.ru/hd.mp4\&quot;}]}&quot;}}"></div>` +
	`</body></html>`

func TestOkru_Matches(t *testing.T) {
	e := NewOkruExtractor()
	for _, u := range []string{"https://ok.ru/videoembed/123", "https://m.ok.ru/videoembed/9"} {
		if !e.Matches(u) { t.Errorf("Matches(%q) = false, want true", u) }
	}
	for _, u := range []string{"https://evil.com/ok.ru", "https://notok.ru/x", "ftp://ok.ru/x"} {
		if e.Matches(u) { t.Errorf("Matches(%q) = true, want false", u) }
	}
}

func TestOkru_Extract_HLSAndMP4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, okruEmbedHTML)
	}))
	defer srv.Close()
	e := NewOkruExtractor()
	// point Matches-bypass: Extract calls Matches first, so use a host it accepts
	// by overriding the fetch URL via the test server while keeping ok.ru host.
	// Simplest: temporarily allow the test server host.
	e.allowTestHost(strings.TrimPrefix(srv.URL, "http://"))
	st, err := e.Extract(context.Background(), srv.URL+"/videoembed/1", nil)
	if err != nil { t.Fatalf("Extract: %v", err) }
	if len(st.Sources) == 0 || st.Sources[0].Type != "hls" {
		t.Fatalf("want HLS first source, got %+v", st.Sources)
	}
	if st.Sources[0].URL != "https://vd1.okcdn.ru/video.m3u8?x=1" {
		t.Errorf("hls url = %q", st.Sources[0].URL)
	}
	if st.Headers["Referer"] != "https://ok.ru/" {
		t.Errorf("referer = %q", st.Headers["Referer"])
	}
}
```

> The test needs the extractor to accept the httptest host. Add a tiny unexported test seam `allowTestHost(host string)` that appends to the host allowlist (guard it so it is test-only by keeping it unexported in `okru.go`). This is the same approach other embed tests use when they can't hit the real host.

- [ ] **Step 2: Run it — expect FAIL** (package/type undefined):

```bash
cd /tmp/ae-okru-impl && go test ./services/scraper/internal/embeds/ -run Okru -v
```

- [ ] **Step 3: Implement.** Create `okru.go` (mirror `vibeplayer.go`'s structure — host allowlist, `Matches`, `Extract`, body cap, compile-time assertion):

```go
// okru.go — OkruExtractor for ok.ru /videoembed pages.
//
// ok.ru (Odnoklassniki) embeds carry a static `data-options="{…}"` attribute
// whose flashvars.metadata (a JSON-encoded string) holds hlsManifestUrl /
// ondemandHls (HLS master) + a videos[] array of progressive MP4s. No JS
// execution and no Cloudflare — a plain GET from our egress returns it
// (verified live 2026-06-22). okcdn.ru manifests are IP-locked to the
// requesting egress; catalog signs the resolved URL so the HLS proxy (same
// host) trusts it. Falls back to POST /dk?cmd=videoPlayerMetadata when the
// inline metadata is absent (yt-dlp Odnoklassniki algorithm).
package embeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const (
	defaultOkruHTTPTimeout = 15 * time.Second
	maxOkruBody            = 4 << 20 // 4 MiB DoS guard (ok.ru pages are larger)
	okruReferer            = "https://ok.ru/"
	okruUA                 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

var okruHosts = []string{"ok.ru", "okru.ru"} // host equality OR strict subdomain (m.ok.ru, etc.)

// dataOptionsRe captures the data-options attribute payload (HTML-escaped JSON).
var dataOptionsRe = regexp.MustCompile(`data-options="([^"]*)"`)

// okMetadata is the inner flashvars.metadata object.
type okMetadata struct {
	HLSManifestURL string `json:"hlsManifestUrl"`
	OndemandHls    string `json:"ondemandHls"`
	Videos         []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"videos"`
}

// OkruExtractor resolves ok.ru /videoembed pages to a Stream. Pure parse, no JS.
type OkruExtractor struct {
	http      *http.Client
	extraHost string // test-only seam for httptest
}

// NewOkruExtractor returns an OkruExtractor with the default HTTP timeout.
func NewOkruExtractor() *OkruExtractor {
	return &OkruExtractor{http: &http.Client{Timeout: defaultOkruHTTPTimeout}}
}

// allowTestHost lets unit tests point Extract at an httptest server.
func (e *OkruExtractor) allowTestHost(host string) { e.extraHost = strings.ToLower(host) }

// Name implements domain.EmbedExtractor.
func (e *OkruExtractor) Name() string { return "okru" }

// Matches reports whether embedURL is an ok.ru (or strict subdomain) URL.
func (e *OkruExtractor) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	if e.extraHost != "" && (host == e.extraHost || host+":"+u.Port() == e.extraHost) {
		return true
	}
	for _, known := range okruHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the embed page, parses data-options → flashvars.metadata, and
// returns a Stream (HLS first, MP4 fallbacks). Referer is set so the HLS proxy
// carries it to okcdn.ru segment fetches.
func (e *OkruExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(errors.New("host not in allowlist"), "okru: Matches gate")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: build request")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", okruUA)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", okruReferer)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: fetch")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapProviderDown(fmt.Errorf("upstream %d", resp.StatusCode), "okru: HTTP status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOkruBody))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: read")
	}
	md, err := parseOkMetadata(body)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "okru: parse data-options")
	}
	stream := &domain.Stream{Headers: map[string]string{"Referer": okruReferer}}
	if hls := md.HLSManifestURL; hls != "" {
		stream.Sources = append(stream.Sources, domain.Source{URL: hls, Type: "hls", Quality: "auto"})
	} else if hls := md.OndemandHls; hls != "" {
		stream.Sources = append(stream.Sources, domain.Source{URL: hls, Type: "hls", Quality: "auto"})
	}
	for _, v := range md.Videos {
		if v.URL == "" {
			continue
		}
		stream.Sources = append(stream.Sources, domain.Source{URL: v.URL, Type: "mp4", Quality: v.Name})
	}
	if len(stream.Sources) == 0 {
		return nil, domain.WrapExtractFailed(errors.New("no hls/mp4 in metadata"), "okru: empty metadata")
	}
	return stream, nil
}

// parseOkMetadata pulls flashvars.metadata out of the data-options attribute.
// metadata is itself a JSON-encoded string, so it is unmarshalled twice.
func parseOkMetadata(body []byte) (okMetadata, error) {
	var md okMetadata
	m := dataOptionsRe.FindSubmatch(body)
	if m == nil {
		return md, errors.New("no data-options attribute")
	}
	raw := html.UnescapeString(string(m[1]))
	var opts struct {
		Flashvars struct {
			Metadata json.RawMessage `json:"metadata"`
		} `json:"flashvars"`
	}
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return md, fmt.Errorf("data-options json: %w", err)
	}
	if len(opts.Flashvars.Metadata) == 0 {
		return md, errors.New("no flashvars.metadata")
	}
	// metadata is usually a JSON-encoded string; try string-decode first.
	var mdStr string
	if err := json.Unmarshal(opts.Flashvars.Metadata, &mdStr); err == nil {
		if err := json.Unmarshal([]byte(mdStr), &md); err != nil {
			return md, fmt.Errorf("metadata string json: %w", err)
		}
		return md, nil
	}
	if err := json.Unmarshal(opts.Flashvars.Metadata, &md); err != nil {
		return md, fmt.Errorf("metadata object json: %w", err)
	}
	return md, nil
}

// Compile-time assertion: OkruExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*OkruExtractor)(nil)
```

- [ ] **Step 4: Run it — expect PASS:**

```bash
cd /tmp/ae-okru-impl && go test ./services/scraper/internal/embeds/ -run Okru -v
```

- [ ] **Step 5: Commit:**

```bash
cd /tmp/ae-okru-impl && git add services/scraper/internal/embeds/okru.go services/scraper/internal/embeds/okru_test.go && \
git commit -m "feat(embeds): ok.ru data-options extractor (HLS + MP4 fallbacks)"
```

---

### Task 3: okru provider

Reuses allanime discovery (Task 1) + ok.ru extractor (Task 2); serves only `Ok` sources.

**Files:**
- Create: `services/scraper/internal/providers/okru/doc.go`
- Create: `services/scraper/internal/providers/okru/client.go`
- Test: `services/scraper/internal/providers/okru/client_test.go`

- [ ] **Step 1: Write `doc.go`:**

```go
// Package okru is a scraper provider that serves AllAnime's "Ok" (ok.ru)
// sources WITHOUT touching AllAnime's Cloudflare-Turnstile-walled
// /apivtwo/clock endpoint.
//
// Discovery (FindID / ListEpisodes) is delegated to an internal allanime
// provider — the api.allanime.day GraphQL works fine from our egress; only the
// clock leg is walled. For ListServers / GetStream, okru reads the episode's
// source list via allanime.Provider.EpisodeSourceURLs, keeps ONLY the "Ok"
// sources (ok.ru/videoembed/<id>), and resolves them with the ok.ru extractor
// (static data-options → okcdn.ru HLS). EN sub/dub only — NEVER raw (raw =
// JP-audio-no-burned-subs, served library-only by the catalog Raw resolver).
package okru
```

- [ ] **Step 2: Write the failing test.** Create `client_test.go`:

```go
package okru

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// fakeDisc stands in for the internal allanime discovery.
type fakeDisc struct {
	sources map[string][]allNamed // keyed by episodeID
	foreign bool
}
type allNamed = struct{ Name, URL string } // shape mirror; real type is allanime.NamedSource

func TestGetStream_OnlyOkSources(t *testing.T) {
	p := newTestProvider(t, map[string][]struct{ Name, URL string }{
		"SHOW:1": {
			{"Default", "https://cdn.example/clock.m3u8"}, // must be IGNORED
			{"Ok", "https://ok.ru/videoembed/123"},        // the only one resolved
		},
	}, /*extractorOK=*/ true)

	st, err := p.GetStream(context.Background(), "SHOW", "SHOW:1", "Ok", domain.CategorySub)
	if err != nil { t.Fatalf("GetStream: %v", err) }
	if len(st.Sources) == 0 { t.Fatal("no sources resolved") }
}

func TestGetStream_NoOkSource_NotFound(t *testing.T) {
	p := newTestProvider(t, map[string][]struct{ Name, URL string }{
		"SHOW:1": {{"Default", "https://cdn.example/clock.m3u8"}},
	}, true)
	_, err := p.GetStream(context.Background(), "SHOW", "SHOW:1", "Ok", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetStream_ForeignID_NotFound(t *testing.T) {
	p := newTestProvider(t, nil, true)
	_, err := p.GetStream(context.Background(), "x", "foreign-no-colon", "Ok", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

> Implement `newTestProvider` in the test with small fakes for the two collaborators the provider depends on (a *source lister* and an *extractor*). Define those collaborators as **interfaces** in `client.go` (see Step 3) so the test injects fakes without hitting the network. `newTestProvider` builds a `*Provider` with the fakes. The "foreign id" fake lister returns `domain.WrapNotFound(...)`.

- [ ] **Step 3: Run it — expect FAIL.** Then implement `client.go`:

```go
package okru

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime"
)

const providerName = "okru"

var stageNames = health.AllStages

// sourceLister is the discovery surface okru needs from allanime (test seam).
type sourceLister interface {
	FindID(ctx context.Context, ref domain.AnimeRef) (string, error)
	ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error)
	EpisodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]allanime.NamedSource, error)
}

// streamExtractor is the ok.ru resolver (test seam).
type streamExtractor interface {
	Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error)
}

// Deps is the constructor input for New().
type Deps struct {
	HTTP  *domain.BaseHTTPClient
	Cache cache.Cache
	Log   *logger.Logger
}

// Provider implements domain.Provider, serving AllAnime's ok.ru "Ok" sources.
type Provider struct {
	disc      sourceLister
	extractor streamExtractor
	log       *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs the provider: an internal allanime discovery client + the
// ok.ru extractor. Dependencies validated eagerly (mirrors allanime.New).
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("okru: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("okru: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	disc, err := allanime.New(allanime.Deps{HTTP: d.HTTP, Cache: d.Cache, Log: d.Log})
	if err != nil {
		return nil, fmt.Errorf("okru: internal allanime: %w", err)
	}
	p := &Provider{
		disc:      disc,
		extractor: embeds.NewOkruExtractor(),
		log:       d.Log,
		stages:    make(map[string]domain.StageHealth, len(stageNames)),
	}
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) markStage(stage string, err error) {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	sh := p.stages[stage]
	if err == nil {
		sh.Up, sh.LastOK, sh.LastErr = true, time.Now(), ""
	} else {
		sh.Up, sh.LastErr = false, err.Error()
	}
	p.stages[stage] = sh
}

func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	snap := make(map[string]domain.StageHealth, len(p.stages))
	for k, v := range p.stages {
		snap[k] = v
	}
	return domain.Health{Provider: providerName, Stages: snap}
}

// FindID / ListEpisodes delegate to the shared allanime discovery.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	id, err := p.disc.FindID(ctx, ref)
	p.markStage(health.StageSearch, err)
	return id, err
}

func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	eps, err := p.disc.ListEpisodes(ctx, providerID)
	p.markStage(health.StageEpisodes, err)
	return eps, err
}

// isOk reports whether a source name is AllAnime's ok.ru family.
func isOk(name string) bool { return strings.EqualFold(strings.TrimSpace(name), "ok") }

// ListServers returns only the Ok servers across sub+dub (dub best-effort).
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	var all []domain.Server
	var firstErr error
	for _, cat := range []domain.Category{domain.CategorySub, domain.CategoryDub} {
		srcs, err := p.disc.EpisodeSourceURLs(ctx, episodeID, cat)
		if err != nil {
			if cat == domain.CategorySub {
				firstErr = err
			}
			continue
		}
		for _, s := range srcs {
			if !isOk(s.Name) {
				continue
			}
			id := "Ok"
			if cat != domain.CategorySub {
				id = "Ok-" + string(cat)
			}
			all = append(all, domain.Server{ID: id, Name: "OK.ru", Type: cat})
		}
	}
	if len(all) == 0 {
		err := firstErr
		if err == nil {
			err = domain.WrapNotFound(fmt.Errorf("no Ok source for %s", episodeID), "okru: ListServers")
		}
		p.markStage(health.StageServers, err)
		return nil, err
	}
	p.markStage(health.StageServers, nil)
	return all, nil
}

// GetStream resolves the first playable Ok source for the episode+category.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	srcs, err := p.disc.EpisodeSourceURLs(ctx, episodeID, category)
	if err != nil {
		// Foreign-ID / not-found bubbles up as NotFound so the orchestrator skips us.
		p.markStage(health.StageStream, err)
		return nil, err
	}
	var lastErr error
	for _, s := range srcs {
		if !isOk(s.Name) {
			continue
		}
		stream, exErr := p.extractor.Extract(ctx, s.URL, nil)
		if exErr != nil {
			lastErr = exErr
			continue
		}
		if stream != nil && len(stream.Sources) > 0 {
			p.markStage(health.StageStream, nil)
			return stream, nil
		}
	}
	if lastErr != nil {
		err = domain.WrapExtractFailed(lastErr, "okru: GetStream")
	} else {
		err = domain.WrapNotFound(fmt.Errorf("no Ok source for %s", episodeID), "okru: GetStream")
	}
	p.markStage(health.StageStream, err)
	return nil, err
}

// Compile-time assertion.
var _ domain.Provider = (*Provider)(nil)
```

> The test's `newTestProvider` builds `&Provider{disc: fakeLister, extractor: fakeExtractor, log: logger.Default(), stages: …}` directly (same package, so unexported fields are reachable). The fake lister's `EpisodeSourceURLs` returns `[]allanime.NamedSource{...}` (import the allanime package in the test). For the foreign-id case the fake returns `domain.WrapNotFound(...)`.

- [ ] **Step 4: Run it — expect PASS:**

```bash
cd /tmp/ae-okru-impl && go test ./services/scraper/internal/providers/okru/ -v
```

- [ ] **Step 5: Commit:**

```bash
cd /tmp/ae-okru-impl && git add services/scraper/internal/providers/okru/ && \
git commit -m "feat(okru): provider serving AllAnime Ok (ok.ru) sources clock-free"
```

---

### Task 4: register okru in the scraper

**Files:**
- Modify: `services/scraper/cmd/scraper-api/main.go`
- Modify: `services/scraper/internal/config/providers.go`

- [ ] **Step 1: Add okru to `KnownProviders`.** In `providers.go`, find the `KnownProviders` slice (~line 15-18) and append `"okru"`. Read the surrounding lines first to match formatting.

- [ ] **Step 2: Construct + register okru in `main.go`.** Read the existing `allanime` registration block (`services/scraper/cmd/scraper-api/main.go:339-360`) and the orchestrator provider slice (`~562-568`). Mirror the allanime block. After the allanime construction, add:

```go
	okruBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("api.allanime.day", 1.0, 2),
		domain.WithPerHostRPS("ok.ru", 1.0, 2),
		domain.WithProvider("okru"),
		domain.WithTransport(egressTransport),
	)
	okruProvider, err := okru.New(okru.Deps{HTTP: okruBaseHTTP, Cache: redisCache, Log: log})
	if err != nil {
		log.Fatalw("construct okru provider", "error", err)
	}
```

> Use the exact variable names already in scope (`log`, `egressTransport`, `redisCache` — match whatever the allanime block uses; if the cache var is named differently, use that). Add the import `"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/okru"`.

- [ ] **Step 3: Add okru to the orchestrator registration** right AFTER allanime in the provider slice (so failover order is `…allanime, okru, …`). Match the existing registration call shape (e.g. `orch.Register(okruProvider)` or appending to a `[]domain.Provider` — mirror allanime exactly).

- [ ] **Step 4: Build + vet:**

```bash
cd /tmp/ae-okru-impl && go build ./services/scraper/... && go vet ./services/scraper/...
```

Expected: clean build.

- [ ] **Step 5: Commit:**

```bash
cd /tmp/ae-okru-impl && git add services/scraper/cmd/scraper-api/main.go services/scraper/internal/config/providers.go && \
git commit -m "feat(scraper): register okru provider after allanime"
```

---

### Task 5: roster — seed okru, degrade allanime

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go`
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`
- Test: `services/catalog/internal/service/scraperprovider/migrate_test.go`

- [ ] **Step 1: Seed okru + add to scraperOperatedNames + degrade allanime seed.** In `seed.go`:

(a) Change the `allanime` row (lines 21-24) to:

```go
	{
		Name: "allanime", Status: domain.StatusDegraded,
		Reason: "Stream broken — AllAnime sources behind Cloudflare Turnstile clock (2026-06-22)",
		Description: "AllAnime discovery still works, but its primary sources decode to " +
			"/apivtwo/clock.json behind a Cloudflare managed/Turnstile challenge (api.allanime.day) " +
			"or a down bare host — unsolvable from our egress. Degraded: out of auto-failover, " +
			"manually selectable (hacker mode). Its ok.ru ('Ok') sources are served clock-free by " +
			"the 'okru' provider. Existing DBs flipped via AllAnimeDegrade.",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 90,
	},
```

(b) Add a new okru row (place it right after the allanime row):

```go
	{
		Name: "okru", Status: domain.StatusEnabled,
		Reason: "AllAnime 'Ok' sources via ok.ru CDN (clock-free)",
		Description: "Reuses AllAnime's GraphQL discovery (api.allanime.day) and resolves ONLY its " +
			"ok.ru ('Ok') sources via ok.ru data-options metadata → okcdn.ru HLS, bypassing the " +
			"Cloudflare-Turnstile-walled /apivtwo/clock endpoint that broke allanime. EN sub/dub, " +
			"hardsubbed (ok.ru has no soft-sub track).",
		SupportsSub: true, SupportsDub: true, SubDelivery: "hard",
		QualityCeiling: "1080p", PreferenceWeight: 35,
	},
```

(c) Add `"okru": true` to the `scraperOperatedNames` map (line 178-181).

- [ ] **Step 2: Write the failing migration test.** Append to `migrate_test.go` (mirror the existing `AnimefeverDeclaim`/`MiruroDubOnly` tests — read them first for the in-memory sqlite/gorm fixture):

```go
func TestAllAnimeDegrade_FlipsOnceIdempotent(t *testing.T) {
	db := newMigrateTestDB(t) // same helper the other migrate tests use
	// seed an enabled allanime row
	if err := db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := AllAnimeDegrade(db); err != nil { t.Fatalf("first: %v", err) }
	var row domain.ScraperProvider
	db.Where("name = ?", "allanime").First(&row)
	if row.Status != domain.StatusDegraded {
		t.Fatalf("status = %q, want degraded", row.Status)
	}
	// operator re-enables; second run must NOT clobber (guard already written)
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "allanime").Update("status", domain.StatusEnabled)
	if err := AllAnimeDegrade(db); err != nil { t.Fatalf("second: %v", err) }
	db.Where("name = ?", "allanime").First(&row)
	if row.Status != domain.StatusEnabled {
		t.Fatalf("status = %q after re-enable+rerun, want enabled (not clobbered)", row.Status)
	}
}
```

- [ ] **Step 3: Run it — expect FAIL** (`AllAnimeDegrade` undefined):

```bash
cd /tmp/ae-okru-impl && go test ./services/catalog/internal/service/scraperprovider/ -run AllAnimeDegrade -v
```

- [ ] **Step 4: Implement `AllAnimeDegrade`** in `migrate.go` (mirror `AnimefeverDeclaim`, lines 225-263). Add the guard-key const near line 184 and the function after `AnimefeverDeclaim`:

```go
// allanimeDegradeGuardKey marks AllAnimeDegrade as applied.
const allanimeDegradeGuardKey = "allanime_degrade"

// AllAnimeDegrade flips allanime to status=degraded exactly once. AllAnime's
// stream leg is dead (its sources decode to /apivtwo/clock.json behind a
// Cloudflare Turnstile, unsolvable from our egress); the ok.ru sources are
// served clock-free by the new 'okru' provider. The seed is insert-if-absent
// and never updates an existing prod row, so this RUN-ONCE guarded migration
// carries the flip to live DBs. Guarded via catalog_migration_guards so it is
// a no-op on later boots and never clobbers an operator re-enable. Idempotent.
func AllAnimeDegrade(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return fmt.Errorf("migrate catalog_migration_guards: %w", err)
	}
	var guards int64
	if err := db.Model(&migrationGuard{}).
		Where("key = ?", allanimeDegradeGuardKey).Count(&guards).Error; err != nil {
		return fmt.Errorf("check allanime-degrade guard: %w", err)
	}
	if guards > 0 {
		return nil // already applied — never clobber a later operator re-enable
	}
	result := db.Model(&domain.ScraperProvider{}).
		Where("name = ?", "allanime").
		Updates(map[string]interface{}{
			"status":      domain.StatusDegraded,
			"reason":      "Stream broken — AllAnime sources behind Cloudflare Turnstile clock (2026-06-22)",
			"description": "AllAnime discovery still works, but its primary sources decode to /apivtwo/clock.json behind a Cloudflare managed/Turnstile challenge (api.allanime.day) or a down bare host — unsolvable from our egress. Degraded: out of auto-failover, manually selectable (hacker mode). Its ok.ru ('Ok') sources are served clock-free by the 'okru' provider. Existing DBs flipped via AllAnimeDegrade.",
		})
	if result.Error != nil {
		return fmt.Errorf("allanime degrade: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("allanime degrade: no row found for name=allanime")
	}
	if err := db.Create(&migrationGuard{Key: allanimeDegradeGuardKey}).Error; err != nil {
		return fmt.Errorf("write allanime-degrade guard: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Wire the migration in `catalog-api/main.go`.** Find the `AnimefeverDeclaim(db.DB)` call and add right after it:

```go
	if err := scraperprovider.AllAnimeDegrade(db.DB); err != nil {
		log.Errorw("allanime degrade migration", "error", err)
	}
```

> Match the exact `db` handle + log call style used by the adjacent `AnimefeverDeclaim`/`NineanimeBrowser` calls.

- [ ] **Step 6: Run tests + build:**

```bash
cd /tmp/ae-okru-impl && go test ./services/catalog/internal/service/scraperprovider/ -v && go build ./services/catalog/...
```

Expected: PASS + clean build.

- [ ] **Step 7: Commit:**

```bash
cd /tmp/ae-okru-impl && git add services/catalog/internal/service/scraperprovider/ services/catalog/cmd/catalog-api/main.go && \
git commit -m "feat(roster): seed okru + degrade allanime (guarded migration)"
```

---

### Task 6: frontend provider registry

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/providerRegistry.ts`

- [ ] **Step 1: Add the okru entry + update allanime blurb.** In the EN scraper block, after the `nineanime` entry (line 27), add:

```ts
  { id: 'okru',       name: 'OK.ru',      hue: '#00d4ff', group: 'en', audios: ['sub', 'dub'], langs: ['en'], content: ['common'], scraper: true,
    blurb: 'EN scraper — AllAnime index, OK.ru CDN (clock-free).' },
```

> Uses the EN-cluster cyan `#00d4ff` (already allowlisted; no new DS-lint hex entry). Update the `allanime` entry's blurb (line 17) to: `'EN scraper — degraded: stream behind Cloudflare Turnstile (use OK.ru).'`

- [ ] **Step 2: Add okru to `CURATED_TIER`.** Insert `'okru',` after `'nineanime',` (line 64). Keep the trailing comment style.

- [ ] **Step 3: Type-check:**

```bash
cd /tmp/ae-okru-impl/frontend/web && bunx vue-tsc --noEmit 2>&1 | tail -20
```

Expected: no new errors referencing providerRegistry.ts. (If `bun`/node_modules aren't present in the worktree, symlink them: `ln -s /data/animeenigma/frontend/web/node_modules /tmp/ae-okru-impl/frontend/web/node_modules` first.)

- [ ] **Step 4: Run the registry/provider specs:**

```bash
cd /tmp/ae-okru-impl/frontend/web && bunx vitest run src/components/player/aePlayer/ src/composables/aePlayer/ 2>&1 | tail -25
```

Expected: PASS (fix any spec that hardcodes the provider count/list to include `okru`).

- [ ] **Step 5: Commit:**

```bash
cd /tmp/ae-okru-impl && git add frontend/web/src/components/player/aePlayer/providerRegistry.ts && \
git commit -m "feat(frontend): add okru provider chip + degrade allanime blurb"
```

---

### Task 7: raw → library-only

Drop the AllAnime backend from the JP-audio Raw resolver. **Read `services/catalog/internal/service/raw_resolver.go` in full first** — apply the transformation below to the live code (do not transcribe blindly).

**Files:**
- Modify: `services/catalog/internal/service/raw_resolver.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`
- Modify: `services/catalog/internal/service/scraperprovider/seed.go` (raw row description)
- Delete (if no other importer): `services/catalog/internal/parser/allanime/`
- Test: `services/catalog/internal/service/raw_resolver_test.go`

- [ ] **Step 1: Confirm the parser/allanime importers** (decides whether the package is deletable):

```bash
cd /tmp/ae-okru-impl && grep -rn "internal/parser/allanime" services/ --include=*.go | grep -v "_test.go"
```

Expected: only `raw_resolver.go` + `catalog-api/main.go` reference it → safe to delete the package. If anything else imports it, KEEP the package and only remove raw's usage.

- [ ] **Step 2: Rewrite the raw tests to library-only.** In `raw_resolver_test.go`, replace the AllAnime-fallback fixtures so:
  - `NewRawResolver(libraryClient, animeRepo, redisCache, log)` is called WITHOUT an allanime client.
  - `GetStream` returns the library MinIO stream on a library 200.
  - `GetStream` returns `NotFound` on a library 404 (no AllAnime fallback).
  - `GetEpisodes` returns `{Episodes:[], Available:false}` when the library is unconfigured / anime has no `ShikimoriID`.
  - Assert no AllAnime HTTP call occurs (drop the AllAnime test server entirely).

Run — expect FAIL (constructor signature mismatch):

```bash
cd /tmp/ae-okru-impl && go test ./services/catalog/internal/service/ -run RawResolver -v
```

- [ ] **Step 3: Apply the resolver transformation** to `raw_resolver.go`:
  - Remove the `import ".../internal/parser/allanime"` and the `client *allanime.Client` field.
  - `NewRawResolver(libraryClient *library.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver` (drop the allanime arg).
  - `GetEpisodes` → call `r.library.ListEpisodes(ctx, anime.ShikimoriID)` directly; empty/no-ShikimoriID → `EpisodesResponse{Episodes: []…{}, Available: false, Source: "library"}`. Delete the AllAnime lookup + `resolveShowID` call.
  - `GetStream` → `(1)` empty `ShikimoriID` → `errors.NotFound`; `(2)` `r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber)`; `(3)` resp → `newLibraryStream(resp.MinIOURL, quality)` + `has_raw`; `(4)` nil resp → `errors.NotFound`; `(5)` error → wrap unavailable. Delete the source-decision cache + library-first/AllAnime-fallback branching.
  - Delete `resolveShowID`, `doSearch`, `isUpstreamFailure`, and any AllAnime-only cache keys / `rawLookup` singleflight.
  - KEEP `newLibraryStream`, `GetLibraryEpisodes`, `GetLibraryStream`, `fireSignal` (still used by `ae` + library record hooks).

- [ ] **Step 4: Update `catalog-api/main.go`:** delete the `allanimeClient := allanime.NewClient(...)` construction and the `parser/allanime` import; change the call to `service.NewRawResolver(libraryClient, animeRepo, redisCache, log)` (drop the allanime arg). Match the real variable names in scope.

- [ ] **Step 5: Delete the dead package** (only if Step 1 confirmed no other importer):

```bash
cd /tmp/ae-okru-impl && rm -rf services/catalog/internal/parser/allanime
```

Also remove the now-dead `AllAnimeConfig` wiring in `services/catalog/internal/config/config.go` (the struct field + its `Load()` block + the type def) — grep `AllAnime` in `config.go` and remove only those lines; leave everything else.

- [ ] **Step 6: Update the raw seed description** in `seed.go` (lines 146-151) to reflect library-only:

```go
	{
		Name: "raw", Status: domain.StatusEnabled,
		Reason:      "JP original-audio player (library-only, self-hosted HLS)",
		Description: "Raw JP player (MinIO library HLS, no AllAnime backend). JP audio with no burned-in subs; subs overlay softly (Jimaku).",
		SupportsSub: true, SupportsRaw: true, SubDelivery: "soft",
		QualityCeiling: "1080p", PreferenceWeight: 0,
	},
```

> Seed is insert-if-absent (won't touch live rows) — this only affects fresh DBs. The roster `raw` row stays `group=jp`, `sub_delivery=soft`. No guarded migration needed (the row's status/columns don't change, only the cosmetic description on fresh installs).

- [ ] **Step 7: Run tests + build:**

```bash
cd /tmp/ae-okru-impl && go test ./services/catalog/... 2>&1 | tail -30 && go build ./services/catalog/...
```

Expected: PASS + clean build.

- [ ] **Step 8: Commit:**

```bash
cd /tmp/ae-okru-impl && git add -A services/catalog/ && \
git commit -m "refactor(raw): library-only JP source, drop AllAnime backend"
```

---

### Task 8: full verification

- [ ] **Step 1: Whole-tree Go tests + build:**

```bash
cd /tmp/ae-okru-impl && go build ./... && go test ./services/scraper/... ./services/catalog/... 2>&1 | tail -40
```

Expected: all PASS, clean build.

- [ ] **Step 2: Frontend gate:**

```bash
cd /tmp/ae-okru-impl/frontend/web && bunx vue-tsc --noEmit 2>&1 | tail -10 && bunx vitest run src/components/player/aePlayer/ 2>&1 | tail -15
```

Expected: no new type errors; specs pass.

- [ ] **Step 3: DS lint** (frontend touched a `.ts`, not `.vue`, but run the gate):

```bash
cd /tmp/ae-okru-impl && bash frontend/web/scripts/design-system-lint.sh 2>&1 | tail -5
```

Expected: 0 errors.

- [ ] **Step 4:** The live deploy + e2e (`prefer=okru` → HLS proxy 200), roster verification (`allanime=degraded`, `okru=enabled scraper_operated=true`), changelog, push, and `/animeenigma-after-update` are handled by the **after-update** flow once all tasks are green — NOT part of task-by-task execution.

---

## Self-Review

**Spec coverage:** Component 1 (okru) → Tasks 1-4, 6. Component 2 (allanime degrade) → Task 5 + Task 6 (blurb). Component 3 (raw library-only) → Task 7. Roster row → Task 5. Testing → per-task TDD + Task 8. Taxonomy (okru EN/hard, raw JP/soft, no crossover) → enforced in Task 3 (`isOk` filter, sub/dub only) + Task 7 (raw untouched). ✓ No gaps.

**Placeholder scan:** No TBD/TODO; every code step has real code or a precise read-then-transform instruction (Task 7, justified — the resolver is large and live-read is safer than transcription).

**Type consistency:** `allanime.NamedSource{Name,URL}` defined Task 1, consumed Tasks 1/3. `okru.Deps{HTTP,Cache,Log}` defined Task 3, used Task 4. `OkruExtractor`/`NewOkruExtractor` defined Task 2, used Task 3. `AllAnimeDegrade`/`allanimeDegradeGuardKey` defined Task 5. Provider methods match `domain.Provider` (Task 3 compile-time assertion). ✓
