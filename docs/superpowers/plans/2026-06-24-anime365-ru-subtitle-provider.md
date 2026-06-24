# anime365 Russian-Subtitle Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add anime365/smotret-anime as a third subtitle provider in the catalog SubsAggregator so Russian (`subRu`) tracks appear for anime that currently have no RU subtitles.

**Architecture:** A new internal parser package `services/catalog/internal/parser/anime365` resolves anime → series (by MAL id) → episode → Russian-subtitle translations, and downloads the subtitle file (ASS primary, VTT fallback). It is fanned out in parallel inside `SubsAggregator.FetchAll` alongside Jimaku and OpenSubtitles, contributing `SubtitleTrack`s whose URLs point at a new proxy endpoint that fetches+caches the file server-side (mirroring the OpenSubtitles file endpoint). Frontend needs no functional change — `ru` is already a first-class UI language.

**Tech Stack:** Go 1.x (catalog service), chi router, Redis cache (`libs/cache`), Vue 3 + TypeScript frontend. Module path `github.com/ILITA-hub/animeenigma`.

## Global Constraints

- Module path: `github.com/ILITA-hub/animeenigma` (all imports).
- Go file names snake_case; packages lowercase single word.
- Provider string emitted by the backend is lowercase `anime365` (matches `SubtitleTrack.Provider` of `jimaku`/`opensubtitles`).
- anime365 base URL is configurable via `ANIME365_BASE_URL` (default `https://smotret-anime.org`); enable flag `ANIME365_ENABLED` (default `true`). No API key.
- Per-provider failures must be fail-soft: never abort `FetchAll`; a failure lands in `ProvidersDown`. "Not found" / "no RU sub" is `nil, nil`, NOT an error.
- Outbound HTTP from catalog uses the egress-recording transport: pass `Transport: tracing.WrapTransport(nil)` (AR-EGRESS-03 parity with OpenSubtitles).
- Commits use Conventional Commits and MUST carry the co-author trailer:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- All work happens in the current worktree `/data/animeenigma/.claude/worktrees/aeplayer-subs-chooser` — never edit the base tree.
- Frontend `.vue`/`.ts` edits trip the DS-lint PostToolUse hook; the FE task must pass `/frontend-verify`.
- Do NOT add anime365 to the HLS proxy allowlist — subtitles are served through the catalog file endpoint, not the HLS proxy.

The catalog tests in `services/catalog/internal/service/` need a reachable Redis (they `t.Skip` if absent). Run Go commands from `services/catalog`.

---

### Task 1: anime365 client — resolution (series → episodes → translations)

**Files:**
- Create: `services/catalog/internal/parser/anime365/types.go`
- Create: `services/catalog/internal/parser/anime365/client.go`
- Test: `services/catalog/internal/parser/anime365/client_test.go`

**Interfaces:**
- Produces:
  - `type Config struct { BaseURL string; Enabled bool; UserAgent string; Timeout time.Duration; Transport http.RoundTripper }`
  - `func NewClient(cfg Config) *Client`
  - `func (c *Client) IsConfigured() bool`
  - `func (c *Client) SearchSeriesByMAL(ctx context.Context, malID, title string) (int, error)` — returns anime365 series id, or `0` (not found) with nil error.
  - `func (c *Client) ListEpisodes(ctx context.Context, seriesID int) ([]Episode, error)`
  - `func (c *Client) ListTranslations(ctx context.Context, episodeID int) ([]Translation, error)`
  - `type Episode struct { ID int; EpisodeInt string; EpisodeType string; IsActive bool }`
  - `type Translation struct { ID int; TypeKind string; TypeLang string; AuthorsSummary string }`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/parser/anime365/client_test.go`:

```go
package anime365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchSeriesByMAL_MatchesMyAnimeListID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/series" {
			_, _ = w.Write([]byte(`{"data":[{"id":111,"myAnimeListId":999},{"id":28440,"myAnimeListId":51553}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	id, err := c.SearchSeriesByMAL(context.Background(), "51553", "Tongari Boushi no Atelier")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if id != 28440 {
		t.Fatalf("series id = %d, want 28440", id)
	}
}

func TestSearchSeriesByMAL_NoMatchReturnsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":111,"myAnimeListId":999}]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	id, err := c.SearchSeriesByMAL(context.Background(), "51553", "x")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if id != 0 {
		t.Fatalf("series id = %d, want 0 (no match)", id)
	}
}

func TestListEpisodes_DecodesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":349360,"episodeInt":"1","episodeType":"tv","isActive":true},
			{"id":371073,"episodeInt":"1","episodeType":"preview","isActive":true},
			{"id":380283,"episodeInt":"12","episodeType":"tv","isActive":true}
		]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	eps, err := c.ListEpisodes(context.Background(), 28440)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(eps) != 3 || eps[2].ID != 380283 || eps[2].EpisodeInt != "12" {
		t.Fatalf("episodes = %+v", eps)
	}
	if eps[1].EpisodeType != "preview" {
		t.Fatalf("expected preview type, got %q", eps[1].EpisodeType)
	}
}

func TestListTranslations_DecodesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":380283,"translations":[
			{"id":5825652,"typeKind":"sub","typeLang":"ru","authorsSummary":"Crunchyroll"},
			{"id":5819457,"typeKind":"sub","typeLang":"ru","authorsSummary":"Sa4ko aka Kiyoso"},
			{"id":111,"typeKind":"voice","typeLang":"ru","authorsSummary":"AniJoy"}
		]}}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	trs, err := c.ListTranslations(context.Background(), 380283)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(trs) != 3 || trs[0].ID != 5825652 || trs[0].TypeKind != "sub" || trs[0].TypeLang != "ru" {
		t.Fatalf("translations = %+v", trs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/anime365/ -run TestSearchSeriesByMAL -v`
Expected: FAIL — `undefined: NewClient` / package has no buildable files.

- [ ] **Step 3: Write the types**

Create `services/catalog/internal/parser/anime365/types.go`:

```go
// Package anime365 is a read-only client for the smotret-anime / anime365
// public API. It resolves anime → series → episode → Russian-subtitle
// translations and downloads the subtitle files. No API key is required.
package anime365

// Episode is one episode row from GET /api/episodes?seriesId=.
type Episode struct {
	ID          int    `json:"id"`
	EpisodeInt  string `json:"episodeInt"`
	EpisodeType string `json:"episodeType"`
	IsActive    bool   `json:"isActive"`
}

// Translation is one translation (sub/voice, per language) from
// GET /api/episodes/{id}.
type Translation struct {
	ID             int    `json:"id"`
	TypeKind       string `json:"typeKind"` // "sub" | "voice" | "raw"
	TypeLang       string `json:"typeLang"` // "ru" | "en" | "ja"
	AuthorsSummary string `json:"authorsSummary"`
}

// series is the minimal shape we need from GET /api/series search results.
type series struct {
	ID            int `json:"id"`
	MyAnimeListID int `json:"myAnimeListId"`
}

// episodeDetail is the GET /api/episodes/{id} payload (data object).
type episodeDetail struct {
	ID           int           `json:"id"`
	Translations []Translation `json:"translations"`
}
```

- [ ] **Step 4: Write the client**

Create `services/catalog/internal/parser/anime365/client.go`:

```go
package anime365

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://smotret-anime.org"

// Config configures the anime365 client. Empty fields fall back to defaults.
type Config struct {
	BaseURL   string
	Enabled   bool
	UserAgent string
	Timeout   time.Duration
	Transport http.RoundTripper // egress-recording transport (AR-EGRESS-03)
}

// Client is the anime365 read-only HTTP client.
type Client struct {
	baseURL    string
	enabled    bool
	userAgent  string
	httpClient *http.Client
}

// NewClient builds a Client, applying safe defaults.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0 (+https://animeenigma.org)"
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		enabled:    cfg.Enabled,
		userAgent:  cfg.UserAgent,
		httpClient: &http.Client{Timeout: cfg.Timeout, Transport: cfg.Transport},
	}
}

// IsConfigured reports whether the provider is enabled. anime365 needs no key,
// so this is just the enable flag; the aggregator skips it when false.
func (c *Client) IsConfigured() bool { return c != nil && c.enabled }

// getJSON GETs path and decodes the JSON body into out.
func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("anime365: GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchSeriesByMAL finds the anime365 series id whose myAnimeListId matches
// malID. title is used as the search query. Returns (0, nil) when no result
// matches — the caller treats that as "not on anime365", not an error.
func (c *Client) SearchSeriesByMAL(ctx context.Context, malID, title string) (int, error) {
	mal, err := strconv.Atoi(strings.TrimSpace(malID))
	if err != nil || mal <= 0 {
		return 0, fmt.Errorf("anime365: invalid mal id %q", malID)
	}
	q := url.Values{}
	q.Set("query", title)
	q.Set("limit", "20")
	var env struct {
		Data []series `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/series?"+q.Encode(), &env); err != nil {
		return 0, err
	}
	for _, s := range env.Data {
		if s.MyAnimeListID == mal {
			return s.ID, nil
		}
	}
	return 0, nil
}

// ListEpisodes returns all episodes for a series.
func (c *Client) ListEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	q := url.Values{}
	q.Set("seriesId", strconv.Itoa(seriesID))
	q.Set("limit", "1000")
	var env struct {
		Data []Episode `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/episodes?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ListTranslations returns the translations for one episode.
func (c *Client) ListTranslations(ctx context.Context, episodeID int) ([]Translation, error) {
	var env struct {
		Data episodeDetail `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/episodes/"+strconv.Itoa(episodeID), &env); err != nil {
		return nil, err
	}
	return env.Data.Translations, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/parser/anime365/ -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/parser/anime365/types.go \
        services/catalog/internal/parser/anime365/client.go \
        services/catalog/internal/parser/anime365/client_test.go
git commit -m "feat(catalog): anime365 client — series/episodes/translations resolution

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: anime365 client — subtitle download (ASS→VTT) + Ping

**Files:**
- Modify: `services/catalog/internal/parser/anime365/client.go` (append methods)
- Test: `services/catalog/internal/parser/anime365/download_test.go`

**Interfaces:**
- Consumes: `*Client` from Task 1.
- Produces:
  - `func (c *Client) DownloadSubtitle(ctx context.Context, transID int) (body []byte, format string, err error)` — fetches `/episodeTranslations/{id}.ass`; on non-200 or invalid ASS, falls back to `/translations/vtt/{id}`. `format` is `"ass"` or `"vtt"`.
  - `func (c *Client) Ping(ctx context.Context) (time.Duration, error)` — satisfies `subprobe.Pinger`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/parser/anime365/download_test.go`:

```go
package anime365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleASS = "[Script Info]\nTitle: x\nScriptType: v4.00+\n\n[Events]\n" +
	"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Привет\n"

func TestDownloadSubtitle_PrefersASS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/episodeTranslations/") {
			_, _ = w.Write([]byte(sampleASS))
			return
		}
		t.Fatalf("unexpected path %s (should not fall back)", r.URL.Path)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	body, format, err := c.DownloadSubtitle(context.Background(), 5819457)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "ass" {
		t.Fatalf("format = %q, want ass", format)
	}
	if !strings.Contains(string(body), "Dialogue:") {
		t.Fatalf("body missing Dialogue: %q", string(body))
	}
}

func TestDownloadSubtitle_FallsBackToVTTWhenASSMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/episodeTranslations/"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/translations/vtt/"):
			_, _ = w.Write([]byte("WEBVTT\n\n00:01.000 --> 00:02.000\nПривет\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	body, format, err := c.DownloadSubtitle(context.Background(), 1)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "vtt" {
		t.Fatalf("format = %q, want vtt", format)
	}
	if !strings.HasPrefix(string(body), "WEBVTT") {
		t.Fatalf("body not vtt: %q", string(body))
	}
}

func TestDownloadSubtitle_FallsBackWhenASSMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/episodeTranslations/"):
			_, _ = w.Write([]byte("<html>paywall</html>")) // 200 but not ASS
		case strings.HasPrefix(r.URL.Path, "/translations/vtt/"):
			_, _ = w.Write([]byte("WEBVTT\n\n00:01.000 --> 00:02.000\nПривет\n"))
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	_, format, err := c.DownloadSubtitle(context.Background(), 1)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if format != "vtt" {
		t.Fatalf("format = %q, want vtt (malformed ASS should fall back)", format)
	}
}

func TestPing_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, Enabled: true})
	if _, err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/anime365/ -run 'TestDownloadSubtitle|TestPing' -v`
Expected: FAIL — `c.DownloadSubtitle undefined` / `c.Ping undefined`.

- [ ] **Step 3: Append the download + ping methods**

Append to `services/catalog/internal/parser/anime365/client.go`:

```go
// DownloadSubtitle fetches the subtitle file for a translation. It prefers the
// ASS form (preserves styling; rendered by the frontend SubtitleOverlay via
// ass-compiler) and falls back to the pre-converted VTT when the ASS request
// fails or returns a body that is not valid ASS.
func (c *Client) DownloadSubtitle(ctx context.Context, transID int) ([]byte, string, error) {
	assBody, assErr := c.fetchRaw(ctx, fmt.Sprintf("/episodeTranslations/%d.ass?willcache", transID))
	if assErr == nil && isValidASS(assBody) {
		return assBody, "ass", nil
	}
	vttBody, vttErr := c.fetchRaw(ctx, fmt.Sprintf("/translations/vtt/%d", transID))
	if vttErr != nil {
		if assErr != nil {
			return nil, "", fmt.Errorf("anime365: ass failed (%v) and vtt failed: %w", assErr, vttErr)
		}
		return nil, "", fmt.Errorf("anime365: ass invalid and vtt failed: %w", vttErr)
	}
	return vttBody, "vtt", nil
}

// Ping checks anime365 reachability via a cheap search query. Returns latency.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	q := url.Values{}
	q.Set("query", "naruto")
	q.Set("limit", "1")
	var env struct {
		Data []series `json:"data"`
	}
	err := c.getJSON(ctx, "/api/series?"+q.Encode(), &env)
	return time.Since(start), err
}

func (c *Client) fetchRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anime365: GET %s: status %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// isValidASS does a cheap structural check so an HTML error/paywall page never
// reaches the player as a "subtitle".
func isValidASS(b []byte) bool {
	s := string(b)
	return strings.Contains(s, "[Script Info]") && strings.Contains(s, "Dialogue:")
}
```

Add `"io"` to the import block of `client.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/parser/anime365/ -v`
Expected: PASS (all tests, Tasks 1+2).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/anime365/client.go \
        services/catalog/internal/parser/anime365/download_test.go
git commit -m "feat(catalog): anime365 client — subtitle download (ASS→VTT) + Ping

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Aggregator — fan-out, fetchAnime365, series-id cache

**Files:**
- Modify: `services/catalog/internal/service/subs_aggregator.go`
- Modify (call-site fixups): `services/catalog/internal/service/subs_aggregator_resolve_test.go`, `subs_aggregator_cache_test.go`, `subs_aggregator_health_test.go`, `subs_aggregator_metrics_test.go`
- Test: `services/catalog/internal/service/subs_aggregator_anime365_test.go`

**Interfaces:**
- Consumes: `anime365.NewClient`, `(*anime365.Client).SearchSeriesByMAL/ListEpisodes/ListTranslations` from Tasks 1-2.
- Produces:
  - `NewSubsAggregator(jimaku *jimaku.Client, opensubs *opensubtitles.Client, anime365Client *anime365.Client, idmap *idmapping.Client, animeRepo *repo.AnimeRepository, cache *cache.RedisCache, health HealthSnapshotter, log *logger.Logger) *SubsAggregator` (NEW 3rd param).
  - `func (s *SubsAggregator) fetchAnime365(ctx, anime *domain.Anime, episode int) ([]SubtitleTrack, error)` (unexported; tested in-package).

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subs_aggregator_anime365_test.go`:

```go
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
)

// anime365TestServer serves the three resolution endpoints for MAL 51553 /
// episode 12 (anime365 ep id 380283) with two RU subtitle translations.
func anime365TestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/series":
			_, _ = w.Write([]byte(`{"data":[{"id":28440,"myAnimeListId":51553}]}`))
		case r.URL.Path == "/api/episodes":
			_, _ = w.Write([]byte(`{"data":[
				{"id":371073,"episodeInt":"1","episodeType":"preview","isActive":true},
				{"id":380283,"episodeInt":"12","episodeType":"tv","isActive":true}
			]}`))
		case r.URL.Path == "/api/episodes/380283":
			_, _ = w.Write([]byte(`{"data":{"id":380283,"translations":[
				{"id":5825652,"typeKind":"sub","typeLang":"ru","authorsSummary":"Crunchyroll"},
				{"id":5819457,"typeKind":"sub","typeLang":"ru","authorsSummary":"Sa4ko"},
				{"id":222,"typeKind":"voice","typeLang":"ru","authorsSummary":"AniJoy"},
				{"id":333,"typeKind":"sub","typeLang":"en","authorsSummary":"SubsPlease"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFetchAnime365_ReturnsRussianSubTracks(t *testing.T) {
	srv := anime365TestServer(t)
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-1", MALID: "51553", Name: "Tongari Boushi no Atelier", Kind: "tv"}
	tracks, err := agg.fetchAnime365(context.Background(), anime, 12)
	if err != nil {
		t.Fatalf("fetchAnime365: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2 (RU subs only): %+v", len(tracks), tracks)
	}
	for _, tr := range tracks {
		if tr.Lang != "ru" || tr.Provider != "anime365" || tr.Format != "ass" {
			t.Fatalf("bad track: %+v", tr)
		}
	}
	if tracks[0].URL != "/api/anime/uuid-1/subtitles/anime365/file/5825652" {
		t.Fatalf("url = %q", tracks[0].URL)
	}
}

func TestFetchAnime365_UnknownAnimeReturnsEmptyNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`)) // no series matches
	}))
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	anime := &domain.Anime{ID: "uuid-2", MALID: "999999", Name: "Nope", Kind: "tv"}
	tracks, err := agg.fetchAnime365(context.Background(), anime, 1)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(tracks) != 0 {
		t.Fatalf("tracks = %+v, want empty", tracks)
	}
}

func TestFetchAnime365_DisabledIsUnconfigured(t *testing.T) {
	a365 := anime365.NewClient(anime365.Config{Enabled: false})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())
	_, err := agg.fetchAnime365(context.Background(), &domain.Anime{ID: "x", MALID: "1"}, 1)
	if err != errProviderUnconfigured {
		t.Fatalf("err = %v, want errProviderUnconfigured", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestFetchAnime365 -v`
Expected: FAIL — `too many arguments in call to NewSubsAggregator` and `agg.fetchAnime365 undefined`.

- [ ] **Step 3: Add the struct field, constructor param, and import**

In `services/catalog/internal/service/subs_aggregator.go`:

Add import (in the existing import block):
```go
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
```

Add the struct field (after `opensubs`):
```go
	opensubs  *opensubtitles.Client
	anime365  *anime365.Client
```

Change the constructor signature and body:
```go
func NewSubsAggregator(
	jimakuClient *jimaku.Client,
	openSubsClient *opensubtitles.Client,
	anime365Client *anime365.Client,
	idMapClient *idmapping.Client,
	animeRepo *repo.AnimeRepository,
	redisCache *cache.RedisCache,
	health HealthSnapshotter,
	log *logger.Logger,
) *SubsAggregator {
	return &SubsAggregator{
		jimaku:    jimakuClient,
		opensubs:  openSubsClient,
		anime365:  anime365Client,
		idmap:     idMapClient,
		animeRepo: animeRepo,
		cache:     redisCache,
		health:    health,
		log:       log,
	}
}
```

Update the `SubtitleTrack.Provider` comment:
```go
	Provider string `json:"provider"` // "jimaku", "opensubtitles", or "anime365"
```

- [ ] **Step 4: Wire the fan-out**

In `FetchAll`, change the channel buffer from 2 to 3:
```go
	resultsCh := make(chan providerResult, 3)
```

Add a third provider goroutine right after the OpenSubtitles goroutine (before the `go func() { wg.Wait(); close(resultsCh) }()` block):
```go
	// anime365 — Russian fansubs, keyed by MAL id.
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracks, err := s.fetchAnime365(ctx, anime, episode)
		resultsCh <- providerResult{name: "anime365", tracks: tracks, err: err}
	}()
```

- [ ] **Step 5: Add fetchAnime365 + resolveAnime365Series**

Add to `subs_aggregator.go` (near `fetchOpenSubtitles`):
```go
func (s *SubsAggregator) fetchAnime365(ctx context.Context, anime *domain.Anime, episode int) ([]SubtitleTrack, error) {
	if s.anime365 == nil || !s.anime365.IsConfigured() {
		return nil, errProviderUnconfigured
	}
	mal := anime.MALID
	if mal == "" {
		mal = anime.ShikimoriID // Shikimori IDs are MAL-aligned for most TV titles
	}
	if mal == "" {
		return nil, nil
	}
	title := anime.NameEN
	if title == "" {
		title = anime.Name
	}

	seriesID, err := s.resolveAnime365Series(ctx, mal, title)
	if err != nil {
		return nil, err
	}
	if seriesID == 0 {
		return nil, nil // not on anime365
	}

	episodes, err := s.anime365.ListEpisodes(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	target := strconv.Itoa(episode)
	if strings.EqualFold(anime.Kind, "movie") {
		target = "1"
	}
	epID := 0
	for _, e := range episodes {
		if e.IsActive && !strings.EqualFold(e.EpisodeType, "preview") && e.EpisodeInt == target {
			epID = e.ID
			break
		}
	}
	if epID == 0 {
		return nil, nil
	}

	translations, err := s.anime365.ListTranslations(ctx, epID)
	if err != nil {
		return nil, err
	}
	tracks := []SubtitleTrack{}
	for _, t := range translations {
		if !strings.EqualFold(t.TypeKind, "sub") || !strings.EqualFold(t.TypeLang, "ru") {
			continue
		}
		tracks = append(tracks, SubtitleTrack{
			URL:      fmt.Sprintf("/api/anime/%s/subtitles/anime365/file/%d", anime.ID, t.ID),
			Lang:     "ru",
			Label:    t.AuthorsSummary,
			Format:   "ass",
			Provider: "anime365",
			Release:  t.AuthorsSummary,
		})
	}
	return tracks, nil
}

// resolveAnime365Series maps a MAL id to an anime365 series id, caching the
// result (hits long, misses shorter so a newly-added title self-heals).
func (s *SubsAggregator) resolveAnime365Series(ctx context.Context, malID, title string) (int, error) {
	cacheKey := fmt.Sprintf("subs:anime365:series:%s", malID)
	var cached int
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}
	seriesID, err := s.anime365.SearchSeriesByMAL(ctx, malID, title)
	if err != nil {
		return 0, err
	}
	ttl := 7 * 24 * time.Hour
	if seriesID == 0 {
		ttl = 6 * time.Hour
	}
	_ = s.cache.Set(ctx, cacheKey, seriesID, ttl)
	return seriesID, nil
}
```

- [ ] **Step 6: Fix existing NewSubsAggregator call sites**

Every existing caller passes 7 args; insert `nil` as the new 3rd arg (anime365). Find them:

Run: `cd services/catalog && grep -rn 'NewSubsAggregator(' internal/service/`

For each call in the test files (e.g. `subs_aggregator_resolve_test.go`, `subs_aggregator_cache_test.go`, `subs_aggregator_health_test.go`, `subs_aggregator_metrics_test.go`), change:
```go
agg := NewSubsAggregator(nil, osc, nil, nil, resolveTestRedis(t), nil, logger.Default())
```
to (insert `nil` after the opensubtitles arg — the 3rd position):
```go
agg := NewSubsAggregator(nil, osc, nil, nil, nil, resolveTestRedis(t), nil, logger.Default())
```
Apply the same insertion to every other `NewSubsAggregator(...)` call in those files (match by adding one `nil` in the 3rd slot). (main.go is updated in Task 6.)

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd services/catalog && go build ./... && go test ./internal/service/ -run 'TestFetchAnime365|TestResolveOpenSubtitles|Subtitle' -v`
Expected: PASS — new anime365 tests green, existing aggregator tests still green (Redis must be reachable; otherwise they Skip).

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/service/subs_aggregator.go \
        services/catalog/internal/service/subs_aggregator_anime365_test.go \
        services/catalog/internal/service/subs_aggregator_resolve_test.go \
        services/catalog/internal/service/subs_aggregator_cache_test.go \
        services/catalog/internal/service/subs_aggregator_health_test.go \
        services/catalog/internal/service/subs_aggregator_metrics_test.go
git commit -m "feat(catalog): fan out anime365 RU subs in SubsAggregator

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Aggregator — ResolveAnime365File (proxy + 24h cache)

**Files:**
- Modify: `services/catalog/internal/service/subs_aggregator.go` (append method)
- Test: `services/catalog/internal/service/subs_aggregator_anime365_resolve_test.go`

**Interfaces:**
- Consumes: `(*anime365.Client).DownloadSubtitle`, the existing `cachedSubFile` type, `errProviderUnconfigured`.
- Produces: `func (s *SubsAggregator) ResolveAnime365File(ctx context.Context, transID int) ([]byte, string, error)`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subs_aggregator_anime365_resolve_test.go`:

```go
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
)

func TestResolveAnime365File_CachesAfterFirstHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/episodeTranslations/") {
			calls++
			_, _ = w.Write([]byte("[Script Info]\n\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hi\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	body, format, err := agg.ResolveAnime365File(context.Background(), 5819457)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if format != "ass" || !strings.Contains(string(body), "Dialogue:") {
		t.Fatalf("body=%q format=%q", string(body), format)
	}
	if _, _, err := agg.ResolveAnime365File(context.Background(), 5819457); err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1 (second served from cache)", calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestResolveAnime365File -v`
Expected: FAIL — `agg.ResolveAnime365File undefined`.

- [ ] **Step 3: Add ResolveAnime365File**

Append to `subs_aggregator.go` (next to `ResolveOpenSubtitlesFile`):
```go
// ResolveAnime365File turns an anime365 translation id into the subtitle bytes,
// caching the result for 24h so re-watches cost no upstream fetch.
func (s *SubsAggregator) ResolveAnime365File(ctx context.Context, transID int) ([]byte, string, error) {
	if s.anime365 == nil || !s.anime365.IsConfigured() {
		return nil, "", errProviderUnconfigured
	}
	cacheKey := fmt.Sprintf("subsfile:anime365:%d", transID)

	var hit cachedSubFile
	if err := s.cache.Get(ctx, cacheKey, &hit); err == nil && len(hit.Body) > 0 {
		return hit.Body, hit.Format, nil
	}

	body, format, err := s.anime365.DownloadSubtitle(ctx, transID)
	if err != nil {
		return nil, "", err
	}
	_ = s.cache.Set(ctx, cacheKey, cachedSubFile{Body: body, Format: format}, 24*time.Hour)
	return body, format, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/ -run TestResolveAnime365File -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subs_aggregator.go \
        services/catalog/internal/service/subs_aggregator_anime365_resolve_test.go
git commit -m "feat(catalog): ResolveAnime365File proxy resolver with 24h cache

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Handler + route — GET .../subtitles/anime365/file/{transId}

**Files:**
- Modify: `services/catalog/internal/handler/subtitles.go`
- Modify: `services/catalog/internal/transport/router.go:208` (add route after the opensubtitles file route)
- Test: `services/catalog/internal/handler/subtitles_anime365_test.go`

**Interfaces:**
- Consumes: `(*service.SubsAggregator).ResolveAnime365File` (Task 4).
- Produces: `func (h *SubtitlesHandler) GetAnime365File(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/handler/subtitles_anime365_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAnime365File_BadTransIDIs400(t *testing.T) {
	h := &SubtitlesHandler{} // aggregator not needed: bad id rejected before use
	req := httptest.NewRequest(http.MethodGet, "/api/anime/x/subtitles/anime365/file/abc", nil)
	// chi URL param not set → chi.URLParam returns "" → Atoi fails → 400.
	rec := httptest.NewRecorder()
	h.GetAnime365File(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

(The happy-path is covered end-to-end by the aggregator resolve test in Task 4 and the manual smoke in Task 8; this guards the input-validation branch, which needs no live aggregator.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestGetAnime365File -v`
Expected: FAIL — `h.GetAnime365File undefined`.

- [ ] **Step 3: Add the handler**

Append to `services/catalog/internal/handler/subtitles.go`:
```go
// GetAnime365File — GET /api/anime/{animeId}/subtitles/anime365/file/{transId}.
//
// Resolves an anime365 translation id to the subtitle text (ASS preferred, VTT
// fallback), caching 24h. Returns text/plain so the frontend SubtitleOverlay
// parses ASS/VTT directly.
func (h *SubtitlesHandler) GetAnime365File(w http.ResponseWriter, r *http.Request) {
	transID, err := strconv.Atoi(chi.URLParam(r, "transId"))
	if err != nil || transID <= 0 {
		httputil.BadRequest(w, "transId must be a positive integer")
		return
	}

	body, _, err := h.aggregator.ResolveAnime365File(r.Context(), transID)
	if err != nil {
		h.log.Errorw("anime365 file resolve failed", "trans_id", transID, "error", err)
		httputil.Error(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

- [ ] **Step 4: Register the route**

In `services/catalog/internal/transport/router.go`, directly after the existing line 208:
```go
			r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", subtitlesHandler.GetOpenSubtitlesFile)
```
add:
```go
			// Lazy anime365 file resolve — fetches RU subtitle bytes (ASS→VTT),
			// proxied + cached 24h (spec 2026-06-24).
			r.Get("/{animeId}/subtitles/anime365/file/{transId}", subtitlesHandler.GetAnime365File)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go build ./... && go test ./internal/handler/ -run TestGetAnime365File -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/handler/subtitles.go \
        services/catalog/internal/handler/subtitles_anime365_test.go \
        services/catalog/internal/transport/router.go
git commit -m "feat(catalog): anime365 subtitle file endpoint + route

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: Config + DI + subprobe registration

**Files:**
- Modify: `services/catalog/internal/config/config.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`

**Interfaces:**
- Consumes: `config.Config.Anime365`, `anime365.NewClient`, the updated `NewSubsAggregator` signature (Task 3), `subprobe.Pinger`.
- Produces: `config.Anime365Config{ BaseURL string; Enabled bool }`.

- [ ] **Step 1: Add the config struct + loader**

In `services/catalog/internal/config/config.go`:

Add to the `Config` struct (near `OpenSubtitles`):
```go
	// Anime365 — RU fansub aggregator (smotret-anime). Spec 2026-06-24.
	Anime365 Anime365Config
```

Add the type (near `OpenSubtitlesConfig`):
```go
// Anime365Config — Russian subtitle source (smotret-anime / anime365).
// No API key; only a base URL + enable flag.
type Anime365Config struct {
	BaseURL string
	Enabled bool
}
```

Add to the returned config literal in `Load()` (near the `OpenSubtitles:` block):
```go
		Anime365: Anime365Config{
			BaseURL: getEnv("ANIME365_BASE_URL", "https://smotret-anime.org"),
			Enabled: getEnvBool("ANIME365_ENABLED", true),
		},
```

- [ ] **Step 2: Verify config compiles**

Run: `cd services/catalog && go build ./internal/config/`
Expected: no output (success).

- [ ] **Step 3: Wire the client in main.go**

In `services/catalog/cmd/catalog-api/main.go`:

Add the import (with the other parser imports):
```go
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
```

After the `openSubsClient := opensubtitles.NewClient(...)` block, construct the client:
```go
	// anime365 (smotret-anime) — Russian fansubs, no key. Spec 2026-06-24.
	anime365Client := anime365.NewClient(anime365.Config{
		BaseURL:   cfg.Anime365.BaseURL,
		Enabled:   cfg.Anime365.Enabled,
		Timeout:   8 * time.Second,
		Transport: tracing.WrapTransport(nil),
	})
```

Register it as a subprobe pinger (after the opensubtitles pinger registration):
```go
	if anime365Client.IsConfigured() {
		subPingers["anime365"] = anime365Client
	}
```

Pass it into the aggregator (insert as the new 3rd arg):
```go
	subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, anime365Client, idMapClient, animeRepo, redisCache, subHealthStore, log)
```

- [ ] **Step 4: Build the whole service**

Run: `cd services/catalog && go build ./... && go vet ./internal/parser/anime365/ ./internal/service/`
Expected: no output (success).

- [ ] **Step 5: Run the catalog test suite**

Run: `cd services/catalog && go test ./internal/parser/anime365/ ./internal/service/ ./internal/handler/ -count=1`
Expected: PASS (service tests Skip if Redis is unreachable — that's acceptable; run with a Redis available to fully exercise them).

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/config/config.go \
        services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): wire anime365 provider (config, DI, health probe)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: Frontend — provider badge hue (cosmetic)

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue:325-328`

**Interfaces:** none (pure presentation). `track.provider` arrives as lowercase `anime365` from the backend.

> Note: the existing `PROVIDER_HUES` keys are title-cased (`Jimaku`, `OpenSubtitles`) but `providerBadgeStyle` is called with the lowercase `track.provider`, so today they never match and every badge uses the fallback hue. We lowercase the keys (so they actually apply) and add `anime365`.

- [ ] **Step 1: Update PROVIDER_HUES**

In `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue`, replace:
```ts
const PROVIDER_HUES: Record<string, string> = {
  Jimaku: 'background: var(--accent-line); color: var(--brand-cyan)',
  OpenSubtitles: 'background: var(--line-strong); color: var(--ink-2)',
}
```
with:
```ts
const PROVIDER_HUES: Record<string, string> = {
  jimaku: 'background: var(--accent-line); color: var(--brand-cyan)',
  opensubtitles: 'background: var(--line-strong); color: var(--ink-2)',
  anime365: 'background: var(--accent-line); color: var(--brand-violet)',
}
```

- [ ] **Step 2: Type-check + DS gate via frontend-verify**

Run `/frontend-verify` (or, minimally: `cd frontend/web && bash scripts/design-system-lint.sh && bunx tsc --noEmit`).
Expected: DS-lint `ERRORS=0` (uses semantic tokens only), tsc clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue
git commit -m "feat(aePlayer): anime365 provider badge hue; fix provider-hue casing

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 8: End-to-end smoke verification

**Files:** none (verification only).

- [ ] **Step 1: Redeploy catalog**

Run: `make redeploy-catalog`
Expected: catalog rebuilds and restarts healthy (`make health` shows catalog up).

- [ ] **Step 2: Hit the aggregated endpoint for the reported episode**

Run:
```bash
curl -s "http://localhost:8000/api/anime/fc6c54ac-2b65-4729-9560-3f9d2c2bc48e/subtitles/all?episode=12" | jq '.languages.ru'
```
Expected: a non-empty `ru` array containing at least one track with `"provider":"anime365"` and a `url` of the form `/api/anime/fc6c54ac.../subtitles/anime365/file/<id>`.

- [ ] **Step 3: Fetch the proxied subtitle file**

Take a `url` from Step 2 and request it (prefix with the gateway host):
```bash
curl -s "http://localhost:8000/api/anime/fc6c54ac-2b65-4729-9560-3f9d2c2bc48e/subtitles/anime365/file/<id>" | head -20
```
Expected: ASS (`[Script Info]` … `Dialogue:`) or VTT (`WEBVTT`) with Russian text.

- [ ] **Step 4: Browser check (opt-in)**

Per project policy, a Chrome smoke is opt-in. If desired: open the Witch Hat Atelier ep 12 player, open the subtitle chooser, confirm the **RU** button is enabled and selecting it renders Russian subtitles, and that the Browse modal shows an `anime365` badge.

- [ ] **Step 5: Run /animeenigma-after-update**

This is the project's required closing step — it lints/builds affected services, redeploys, health-checks, writes the Russian-Trump-mode changelog entry, and commits+pushes. Invoke `/animeenigma-after-update`. Then flip the feedback report to `ai_done`:
```bash
bin/feedback-status 2026-06-09T12-04-35_notebook_feedback ai_done "claude-opus-4.8"
```

---

## Notes for the implementer

- **Why no frontend data changes:** `ru` is already in `FAST_LANGS`/`PRIMARY_LANGS`, the handler already defaults `langs` to `ja,en,ru`, and `useSubtitleTracks` already flattens `languages.ru[]`. New `anime365` RU tracks surface automatically on the RU fast button and in Browse.
- **Why no provider-rank change:** `pickDefaultSubtitle.ts` `providerRank` already buckets unknown providers (incl. `anime365`) at `1`, above `opensubtitles` (`2`) and below `jimaku` (`0`) — correct for RU, where anime365 is the preferred source. No edit needed (YAGNI).
- **MAL-id fallback caveat:** `mal == "" → use shikimori_id` is a heuristic (Shikimori IDs are MAL-aligned for the large majority of TV titles, but not universally). If a title resolves to the wrong series, the symptom is wrong-anime RU subs; the fix is to require a real `mal_id`. Acceptable for v1.
- **No i18n changes:** the badge shows the literal `anime365` string (a brand name), so no locale keys are added — the en/ru/ja parity gate is unaffected.
- **No HLS-proxy allowlist change:** subtitle bytes flow through the catalog file endpoint, not the streaming HLS proxy.
```
