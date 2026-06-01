# Any-language OpenSubtitles for the Raw player's "other subs" panel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Light up OpenSubtitles so the Raw player's "other subs" panel shows subtitles in any language, with quota-conserving lazy resolution + 24h caching, and turn that panel into a full-screen surface filterable by provider and language.

**Architecture:** Search stays cheap (zero quota) — the aggregator returns OpenSubtitles tracks whose `url` is a *catalog resolve endpoint* encoding the numeric `file_id`, not the file. Only when a user selects a track does the catalog server spend one OpenSubtitles `POST /download` (then caches the bytes in Redis for 24h). The browser never touches opensubtitles.com, so the HLS proxy allowlist is untouched. The frontend panel filters the existing `.all()` response client-side.

**Tech Stack:** Go (chi, go-redis via `libs/cache`, `libs/httputil`), Vue 3 + TypeScript + Tailwind, Vitest, OpenSubtitles v1 REST API.

**Spec:** `docs/superpowers/specs/2026-06-01-raw-player-anylang-subs-opensubtitles.md`

---

## File Structure

**Backend (`services/catalog/`):**
- `internal/parser/opensubtitles/client.go` — add `Download(ctx, fileID)` method (POST /download → temp link → bytes).
- `internal/parser/opensubtitles/client_test.go` — extend with Download tests.
- `internal/service/subs_aggregator.go` — (a) `fetchOpenSubtitles` emits resolve-path URLs + drops `FileID==0`; (b) new `ResolveOpenSubtitlesFile(ctx, fileID)` with Redis cache + `cachedSubFile` type.
- `internal/service/subs_aggregator_resolve_test.go` — new; tests resolve (cache miss/hit, quota) against a real Redis + httptest OpenSubtitles.
- `internal/handler/subtitles.go` — add `GetOpenSubtitlesFile` handler.
- `internal/handler/subtitles_resolve_test.go` — new; handler-level test (400 / 429 / 200).
- `internal/transport/router.go:144` — add the resolve route.

**Frontend (`frontend/web/`):**
- `src/components/player/SubtitleOverlay.vue:240-251` — same-origin URLs fetched directly (skip hls-proxy).
- `src/components/player/SubtitleOverlay.spec.ts` — new; same-origin vs proxy branch.
- `src/components/player/OtherSubsPanel.vue` — full-screen modal + provider/language filters.
- `src/components/player/OtherSubsPanel.spec.ts` — new; filter logic + counts + empty state.
- `src/locales/en.json` + `src/locales/ru.json` — add `player.otherSubs.filter.*` keys.

**Deploy:**
- `docker/docker-compose.yml:423` — forward `OPENSUBTITLES_API_KEY` (+ user agent) to catalog.
- `docker/.env` — add the key (gitignored, not committed).

---

## Task 1: `opensubtitles.Client.Download` method

**Files:**
- Modify: `services/catalog/internal/parser/opensubtitles/client.go`
- Test: `services/catalog/internal/parser/opensubtitles/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `client_test.go`:

```go
func TestClient_Download_Success(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/download":
			if r.Header.Get("Api-Key") != "k" {
				t.Errorf("missing api key header")
			}
			var body struct {
				FileID int `json:"file_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.FileID != 42 {
				t.Errorf("file_id = %d, want 42", body.FileID)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"link":%q,"file_name":"ep.srt","remaining":99}`, srv.URL+"/file")
		case r.URL.Path == "/file":
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "k", BaseURL: srv.URL})
	body, name, err := c.Download(context.Background(), 42)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if name != "ep.srt" {
		t.Errorf("name = %q, want ep.srt", name)
	}
	if !strings.Contains(string(body), "hi") {
		t.Errorf("body = %q, want subtitle text", string(body))
	}
}

func TestClient_Download_QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"You have reached download limit","remaining":0}`))
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "k", BaseURL: srv.URL})
	_, _, err := c.Download(context.Background(), 7)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestClient_Download_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "bad", BaseURL: srv.URL})
	_, _, err := c.Download(context.Background(), 7)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
```

The existing `client_test.go` already imports `context`, `errors`, `fmt`, `net/http`, `net/http/httptest`, `strings`, `testing` — but **NOT `encoding/json`**, which `TestClient_Download_Success` uses (`json.NewDecoder`). You MUST add `"encoding/json"` to the import block or Task 1 won't compile.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/catalog && go test ./internal/parser/opensubtitles/ -run TestClient_Download -count=1`
Expected: FAIL — `c.Download undefined`.

- [ ] **Step 3: Implement `Download`**

Append to `client.go` (after `Search`):

```go
// Download resolves a subtitle file_id to its actual content. It spends one
// unit of the OpenSubtitles daily download quota per call (per RAW-NF-01 the
// caller is expected to cache the result). Returns the raw bytes plus the
// server-provided file name (used for format detection).
//
// On quota exhaustion returns ErrRateLimited; on 401/403 ErrUnauthorized.
func (c *Client) Download(ctx context.Context, fileID int) ([]byte, string, error) {
	if !c.IsConfigured() {
		return nil, "", ErrUnauthorized
	}

	reqBody, _ := json.Marshal(map[string]int{"file_id": fileID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/download", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: build download request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.cfg.APIKey)
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: download request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, "", ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, "", ErrRateLimited
	case strings.Contains(string(body), "download limit"),
		strings.Contains(string(body), "Reached download limit"),
		strings.Contains(string(body), "maximum number"):
		return nil, "", ErrRateLimited
	case resp.StatusCode >= 400:
		return nil, "", fmt.Errorf("opensubtitles: download %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var doc struct {
		Link      string `json:"link"`
		FileName  string `json:"file_name"`
		Remaining int    `json:"remaining"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "", fmt.Errorf("opensubtitles: parse download: %w", err)
	}
	// Spec rule: exhausted quota presents as remaining<=0 and/or no usable link
	// (OpenSubtitles returns 200 with a message in that case, not a 4xx).
	if doc.Link == "" {
		return nil, "", ErrRateLimited
	}

	fileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, doc.Link, nil)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: build file request: %w", err)
	}
	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: fetch file: %w", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("opensubtitles: fetch file %d", fileResp.StatusCode)
	}
	content, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: read file: %w", err)
	}
	return content, doc.FileName, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/parser/opensubtitles/ -run TestClient_Download -count=1`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/opensubtitles/client.go services/catalog/internal/parser/opensubtitles/client_test.go
git commit -m "feat(catalog/opensubtitles): add Download(fileID) with quota mapping"
```

---

## Task 2: Aggregator — resolve-path URLs + `ResolveOpenSubtitlesFile`

**Files:**
- Modify: `services/catalog/internal/service/subs_aggregator.go` (`fetchOpenSubtitles` ~236-248; add new method + type)
- Test: `services/catalog/internal/service/subs_aggregator_resolve_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/subs_aggregator_resolve_test.go`:

```go
package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

func resolveTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 12})
	if err != nil {
		t.Skipf("redis unreachable (%v); skipping resolve test", err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() { _ = c.Client().FlushDB(context.Background()).Err(); _ = c.Close() })
	return c
}

func TestResolveOpenSubtitlesFile_CachesAfterFirstHit(t *testing.T) {
	calls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			calls++
			fmt.Fprintf(w, `{"link":%q,"file_name":"ep.srt","remaining":99}`, srv.URL+"/file")
			return
		}
		_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	}))
	defer srv.Close()

	osc := opensubtitles.NewClient(opensubtitles.Config{APIKey: "k", BaseURL: srv.URL})
	agg := NewSubsAggregator(nil, osc, nil, nil, resolveTestRedis(t), logger.Default())

	body, format, err := agg.ResolveOpenSubtitlesFile(context.Background(), 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Format is derived from file_name ("ep.srt") via formatFromName, NOT the
	// body content — so it must be "srt" here.
	if string(body) == "" || format != "srt" {
		t.Fatalf("body=%q format=%q", string(body), format)
	}
	// Second call must be served from cache — no new /download hit.
	if _, _, err := agg.ResolveOpenSubtitlesFile(context.Background(), 42); err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("download calls = %d, want 1 (second served from cache)", calls)
	}
}

func TestResolveOpenSubtitlesFile_QuotaPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"download limit","remaining":0}`))
	}))
	defer srv.Close()
	osc := opensubtitles.NewClient(opensubtitles.Config{APIKey: "k", BaseURL: srv.URL})
	agg := NewSubsAggregator(nil, osc, nil, nil, resolveTestRedis(t), logger.Default())
	_, _, err := agg.ResolveOpenSubtitlesFile(context.Background(), 7)
	if err == nil {
		t.Fatal("want quota error, got nil")
	}
}
```

> Note: `logger.Default()` is the no-arg constructor used across catalog tests (`logger.New` takes a `logger.Config` and returns `(*Logger, error)` — do NOT pass a string).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestResolveOpenSubtitlesFile -count=1`
Expected: FAIL — `agg.ResolveOpenSubtitlesFile undefined`.

- [ ] **Step 3: Implement the method + cached type, and change the track URL**

In `subs_aggregator.go`, replace the track-append loop inside `fetchOpenSubtitles` (currently ~236-248, the `for _, e := range entries` block that uses `e.DownloadURL`) with:

```go
	tracks := make([]SubtitleTrack, 0, len(entries))
	for _, e := range entries {
		if e.FileID == 0 {
			continue // can't resolve without a numeric file id
		}
		tracks = append(tracks, SubtitleTrack{
			URL:      fmt.Sprintf("/api/anime/%s/subtitles/opensubtitles/file/%d", anime.ID, e.FileID),
			Lang:     e.Language,
			Label:    e.Release,
			Format:   e.Format,
			Provider: "opensubtitles",
			Release:  e.Release,
		})
	}
	return tracks, nil
```

Then add, at the end of `subs_aggregator.go`:

```go
// cachedSubFile is the Redis-stored resolved subtitle. []byte marshals to
// base64 in JSON, so non-UTF-8 subtitle bytes survive the round-trip.
type cachedSubFile struct {
	Body   []byte `json:"body"`
	Format string `json:"format"`
}

// ResolveOpenSubtitlesFile turns a numeric OpenSubtitles file_id into the
// actual subtitle bytes. It spends one download quota unit on a cache miss,
// then caches the result for 24h so re-watches cost nothing (RAW-NF-01).
func (s *SubsAggregator) ResolveOpenSubtitlesFile(ctx context.Context, fileID int) ([]byte, string, error) {
	if s.opensubs == nil || !s.opensubs.IsConfigured() {
		// Sentinel so the handler maps "no key" to a clean 503, not a 500.
		return nil, "", opensubtitles.ErrUnauthorized
	}
	cacheKey := fmt.Sprintf("subsfile:opensubtitles:%d", fileID)

	var hit cachedSubFile
	if err := s.cache.Get(ctx, cacheKey, &hit); err == nil && len(hit.Body) > 0 {
		return hit.Body, hit.Format, nil
	}

	body, filename, err := s.opensubs.Download(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	format := formatFromName(filename)
	_ = s.cache.Set(ctx, cacheKey, cachedSubFile{Body: body, Format: format}, 24*time.Hour)
	return body, format, nil
}
```

(`fmt`, `time`, and the `opensubtitles` package are already imported in this file; `errors` too, still used by `fetchJimaku`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run TestResolveOpenSubtitlesFile -count=1`
Expected: PASS (skips only if Redis is unreachable).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/subs_aggregator.go services/catalog/internal/service/subs_aggregator_resolve_test.go
git commit -m "feat(catalog/subs): emit resolve-path URLs + cached ResolveOpenSubtitlesFile"
```

---

## Task 3: Resolve handler + route

**Files:**
- Modify: `services/catalog/internal/handler/subtitles.go`
- Modify: `services/catalog/internal/transport/router.go:144`
- Test: `services/catalog/internal/handler/subtitles_resolve_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/handler/subtitles_resolve_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGetOpenSubtitlesFile_BadID(t *testing.T) {
	h := &SubtitlesHandler{} // aggregator unused on the bad-id path
	r := chi.NewRouter()
	r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", h.GetOpenSubtitlesFile)

	req := httptest.NewRequest(http.MethodGet, "/x/subtitles/opensubtitles/file/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestGetOpenSubtitlesFile -count=1`
Expected: FAIL — `h.GetOpenSubtitlesFile undefined`.

- [ ] **Step 3: Implement the handler**

Add to `services/catalog/internal/handler/subtitles.go`. First extend the imports to include `errors` and the `opensubtitles` package:

```go
import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/go-chi/chi/v5"
)
```

Then append the method:

```go
// GetOpenSubtitlesFile — GET /api/anime/{animeId}/subtitles/opensubtitles/file/{fileID}.
//
// Resolves a numeric OpenSubtitles file_id to the actual subtitle text,
// spending one download quota unit on a cache miss. Returns text/plain so the
// frontend SubtitleOverlay can parse SRT/ASS/VTT directly. Quota exhaustion is
// surfaced as 429 (clear message, not a silent failure); a missing key as 503.
func (h *SubtitlesHandler) GetOpenSubtitlesFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.Atoi(chi.URLParam(r, "fileID"))
	if err != nil || fileID <= 0 {
		httputil.BadRequest(w, "fileID must be a positive integer")
		return
	}

	body, _, err := h.aggregator.ResolveOpenSubtitlesFile(r.Context(), fileID)
	if err != nil {
		switch {
		case errors.Is(err, opensubtitles.ErrRateLimited):
			httputil.JSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "OpenSubtitles daily download limit reached — try again later or pick a different subtitle.",
			})
		case errors.Is(err, opensubtitles.ErrUnauthorized):
			httputil.JSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "OpenSubtitles is not configured.",
			})
		default:
			h.log.Warnw("opensubtitles file resolve failed", "file_id", fileID, "error", err)
			httputil.Error(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
```

> The aggregator now returns `opensubtitles.ErrUnauthorized` on the not-configured path (Task 2), so both "no key" and a rejected key land in the `ErrUnauthorized` → 503 branch with the "not configured" message — matching the spec. `strings` is already imported by this file and stays used by `splitTrimLower`.

- [ ] **Step 4: Add the route**

In `services/catalog/internal/transport/router.go`, directly after line 144 (`r.Get("/{animeId}/subtitles/all", subtitlesHandler.GetAll)`), add:

```go
			// Lazy OpenSubtitles file resolve — spends 1 download quota unit
			// per cache miss, then cached 24h (workstream raw-jp follow-on).
			r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", subtitlesHandler.GetOpenSubtitlesFile)
```

- [ ] **Step 5: Run tests + build to verify**

Run: `cd services/catalog && go test ./internal/handler/ -run TestGetOpenSubtitlesFile -count=1 && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/handler/subtitles.go services/catalog/internal/handler/subtitles_resolve_test.go services/catalog/internal/transport/router.go
git commit -m "feat(catalog/subs): resolve endpoint for OpenSubtitles files (429 on quota)"
```

---

## Task 4: Frontend — `SubtitleOverlay` fetches same-origin URLs directly

**Files:**
- Modify: `frontend/web/src/components/player/SubtitleOverlay.vue:240-252`
- Test: `frontend/web/src/components/player/SubtitleOverlay.spec.ts` (new)

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/player/SubtitleOverlay.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SubtitleOverlay from './SubtitleOverlay.vue'

function mockFetchText(text: string) {
  const spy = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: () => Promise.resolve(text),
  })
  vi.stubGlobal('fetch', spy)
  return spy
}

describe('SubtitleOverlay URL fetching', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetches same-origin (/api/...) URLs directly without the hls-proxy', async () => {
    const fetchSpy = mockFetchText('WEBVTT\n')
    mount(SubtitleOverlay, {
      props: {
        videoElement: null,
        subtitleUrl: '/api/anime/abc/subtitles/opensubtitles/file/42',
        format: 'vtt',
        visible: true,
        fullscreenContainer: null,
      },
    })
    await flushPromises()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    expect(fetchSpy.mock.calls[0][0]).toBe('/api/anime/abc/subtitles/opensubtitles/file/42')
  })

  it('wraps external URLs in the hls-proxy', async () => {
    const fetchSpy = mockFetchText('WEBVTT\n')
    mount(SubtitleOverlay, {
      props: {
        videoElement: null,
        subtitleUrl: 'https://jimaku.cc/file.srt',
        format: 'srt',
        visible: true,
        fullscreenContainer: null,
      },
    })
    await Promise.resolve()
    expect(fetchSpy.mock.calls[0][0]).toContain('/api/streaming/hls-proxy?url=')
    expect(fetchSpy.mock.calls[0][0]).toContain(encodeURIComponent('https://jimaku.cc/file.srt'))
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/SubtitleOverlay.spec.ts`
Expected: FAIL — same-origin case still hits `/api/streaming/hls-proxy`.

- [ ] **Step 3: Implement the branch**

In `SubtitleOverlay.vue`, replace lines 249-251:

```ts
    // Proxy through streaming service for CORS
    const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(url)}`
    const resp = await fetch(proxyUrl, { signal: subtitleAbortController.signal })
```

with:

```ts
    // Same-origin backend URLs (e.g. our OpenSubtitles resolve endpoint) are
    // fetched directly; external provider URLs go through the CORS proxy.
    const fetchUrl = url.startsWith('/')
      ? url
      : `/api/streaming/hls-proxy?url=${encodeURIComponent(url)}`
    const resp = await fetch(fetchUrl, { signal: subtitleAbortController.signal })
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/SubtitleOverlay.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/player/SubtitleOverlay.vue frontend/web/src/components/player/SubtitleOverlay.spec.ts
git commit -m "feat(player): SubtitleOverlay fetches same-origin sub URLs directly"
```

---

## Task 5: Frontend — full-screen `OtherSubsPanel` with provider + language filters

**Files:**
- Modify: `frontend/web/src/components/player/OtherSubsPanel.vue`
- Modify: `frontend/web/src/locales/en.json`, `frontend/web/src/locales/ru.json`
- Test: `frontend/web/src/components/player/OtherSubsPanel.spec.ts` (new)

- [ ] **Step 1: Add i18n keys**

In `en.json`, locate the `"otherSubs"` object under `"player"` and add a `"filter"` sub-object (keep existing keys):

```json
        "filter": {
          "provider": "Source",
          "language": "Language",
          "all": "All"
        }
```

In `ru.json`, the matching object:

```json
        "filter": {
          "provider": "Источник",
          "language": "Язык",
          "all": "Все"
        }
```

> If `player.otherSubs.providerChip.jimaku` / `.opensubtitles` keys do not already exist in both files, add them too: en `"Jimaku"`/`"OpenSubtitles"`, ru `"Jimaku"`/`"OpenSubtitles"`. (Verify with: `grep -n "providerChip" frontend/web/src/locales/en.json frontend/web/src/locales/ru.json`.)

- [ ] **Step 2: Write the failing test**

Create `frontend/web/src/components/player/OtherSubsPanel.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OtherSubsPanel from './OtherSubsPanel.vue'

// The component reads `useI18n().t`/`locale` in <script> (providerLabel,
// languageHeader, orderLangs) AND `$t` in <template> (filter labels), so BOTH
// mocks below are intentional — do not "dedupe" them. With t:(k)=>k, label
// lookups fall back to provider key / uppercased lang code, which is why the
// chip selectors below target data-* attributes, not rendered text.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }),
}))

vi.mock('@/api/client', () => ({
  subtitlesApi: {
    all: vi.fn().mockResolvedValue({
      data: {
        data: {
          episode: 1,
          languages: {
            ja: [{ url: 'https://jimaku.cc/a.srt', lang: 'ja', label: 'JP', provider: 'jimaku', format: 'srt' }],
            en: [{ url: '/api/anime/x/subtitles/opensubtitles/file/1', lang: 'en', label: 'EN rip', provider: 'opensubtitles', format: 'srt' }],
            ru: [{ url: '/api/anime/x/subtitles/opensubtitles/file/2', lang: 'ru', label: 'RU rip', provider: 'opensubtitles', format: 'srt' }],
          },
        },
      },
    }),
  },
}))

const mountPanel = () =>
  mount(OtherSubsPanel, {
    props: { modelValue: true, animeId: 'x', episode: 1, currentTrackUrl: null },
    global: {
      mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) },
      stubs: {
        Modal: { template: '<div><slot /></div>' },
        Badge: { template: '<span><slot /></span>' },
      },
    },
  })

describe('OtherSubsPanel filters', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows all three languages by default', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.html()).toContain('JP')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).toContain('RU rip')
  })

  it('provider filter = opensubtitles hides the Jimaku (ja) track', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const osBtn = wrapper.find('button[data-provider="opensubtitles"]')
    expect(osBtn.exists()).toBe(true)
    await osBtn.trigger('click')
    expect(wrapper.html()).not.toContain('JP')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).toContain('RU rip')
  })

  it('language filter = en narrows to the English track only', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    const enChip = wrapper.find('button[data-lang="en"]')
    expect(enChip.exists()).toBe(true)
    await enChip.trigger('click')
    expect(wrapper.html()).toContain('EN rip')
    expect(wrapper.html()).not.toContain('RU rip')
  })
})
```

> The chips carry `data-provider` / `data-lang` attributes (Step 4), so these selectors are decoupled from i18n rendering. If you restructure the markup, keep those attributes — don't weaken the assertions.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/OtherSubsPanel.spec.ts`
Expected: FAIL — provider/language filter buttons don't exist yet.

- [ ] **Step 4: Implement full-screen + filters**

Edit `OtherSubsPanel.vue`. **(a)** Change the Modal to full size — line 5, `size="lg"` → `size="full"`.

**(b)** Add the filter bar in the template, immediately after the opening `<div v-else class="space-y-6">` (line 21):

```html
    <div v-else class="space-y-4">
      <!-- Filter bar: provider + language. Filters the already-fetched
           .all() response client-side; no extra requests, no extra quota. -->
      <div class="space-y-3 sticky top-0 z-10 bg-black/60 backdrop-blur-sm py-2 -mx-1 px-1 rounded-lg">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-white/50 text-xs uppercase tracking-wide mr-1">{{ $t('player.otherSubs.filter.provider') }}</span>
          <button
            v-for="p in providerOptions"
            :key="p"
            type="button"
            :data-provider="p"
            class="px-3 py-1 rounded-full text-sm transition-colors"
            :class="providerFilter === p ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="providerFilter = p"
          >
            {{ p === 'all' ? $t('player.otherSubs.filter.all') : providerLabel(p) }}
          </button>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-white/50 text-xs uppercase tracking-wide mr-1">{{ $t('player.otherSubs.filter.language') }}</span>
          <button
            type="button"
            class="px-3 py-1 rounded-full text-sm transition-colors"
            :class="langFilter === 'all' ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="langFilter = 'all'"
          >
            {{ $t('player.otherSubs.filter.all') }}
          </button>
          <button
            v-for="l in languageOptions"
            :key="l.lang"
            type="button"
            :data-lang="l.lang"
            class="px-3 py-1 rounded-full text-sm transition-colors"
            :class="langFilter === l.lang ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="langFilter = l.lang"
          >
            {{ languageHeader(l.lang) }} ({{ l.count }})
          </button>
        </div>
      </div>
```

Then change the empty-results condition so it reacts to filtering. Replace the existing `v-else-if="languageGroups.length === 0"` block (lines 17-19) — move that empty check to be based on `filteredGroups` and render it *inside* the `v-else` after the filter bar. Concretely, the list section's `v-for` must iterate `filteredGroups` instead of `languageGroups`, and add an empty state:

```html
      <p v-if="filteredGroups.length === 0" class="py-12 text-center text-white/60 text-sm">
        {{ $t('player.otherSubs.empty') }}
      </p>

      <section
        v-for="group in filteredGroups"
        :key="group.lang"
        class="space-y-2"
      >
```

(Leave the original top-level `v-if="loading" / v-else-if="error"` checks as-is. Remove the now-redundant standalone `v-else-if="languageGroups.length === 0"` block at lines 17-19, since the empty state now lives inside the filtered view.)

**(c)** Script changes. Add filter state + computeds. After `const data = ref<GroupedSubs | null>(null)` (line 102) add:

```ts
const providerFilter = ref<'all' | string>('all')
const langFilter = ref<'all' | string>('all')

// Provider chips: 'all' + whichever providers actually returned tracks.
const providerOptions = computed<string[]>(() => {
  if (!data.value) return ['all']
  const set = new Set<string>()
  for (const tracks of Object.values(data.value.languages)) {
    for (const t of tracks ?? []) set.add(t.provider)
  }
  return ['all', ...[...set].sort()]
})

// Language chips with counts, honoring the active provider filter.
const languageOptions = computed<{ lang: string; count: number }[]>(() => {
  if (!data.value) return []
  const counts: Record<string, number> = {}
  for (const [lang, tracks] of Object.entries(data.value.languages)) {
    for (const t of tracks ?? []) {
      if (providerFilter.value !== 'all' && t.provider !== providerFilter.value) continue
      counts[lang] = (counts[lang] ?? 0) + 1
    }
  }
  return orderLangs(Object.keys(counts)).map((lang) => ({ lang, count: counts[lang] }))
})

// Groups after BOTH filters are applied.
const filteredGroups = computed(() => {
  return languageGroups.value
    .map((g) => ({
      lang: g.lang,
      tracks: g.tracks.filter(
        (t) => providerFilter.value === 'all' || t.provider === providerFilter.value,
      ),
    }))
    .filter((g) => g.tracks.length > 0)
    .filter((g) => langFilter.value === 'all' || g.lang === langFilter.value)
})

// When the provider filter changes, a previously-picked language may vanish —
// reset to 'all' so the user never lands on a dead empty view.
watch(providerFilter, () => {
  langFilter.value = 'all'
})
```

Refactor the existing `languageGroups` sort so its ordering helper is reusable by `languageOptions`. Replace the body of `languageGroups` (lines 104-120) to delegate to a shared `orderLangs`:

```ts
const orderLangs = (langs: string[]): string[] => {
  const preferred = [locale.value, 'ja', 'en', 'ru']
  return [...langs].sort((a, b) => {
    const ai = preferred.indexOf(a)
    const bi = preferred.indexOf(b)
    if (ai !== -1 && bi === -1) return -1
    if (bi !== -1 && ai === -1) return 1
    if (ai !== -1 && bi !== -1) return ai - bi
    return a.localeCompare(b)
  })
}

const languageGroups = computed(() => {
  if (!data.value) return []
  return orderLangs(Object.keys(data.value.languages)).map((lang) => ({
    lang,
    tracks: data.value!.languages[lang] ?? [],
  }))
})
```

Reset filters whenever fresh data loads — at the end of `fetchAll`'s `try` (after `data.value = ...`, line 161) add:

```ts
    providerFilter.value = 'all'
    langFilter.value = 'all'
```

Ensure `watch` is imported (it already is: `import { computed, ref, watch } from 'vue'`, line 78).

- [ ] **Step 5: Run test + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/OtherSubsPanel.spec.ts && bunx tsc --noEmit`
Expected: PASS + no type errors. (If a chip selector in the test doesn't match your final markup, adjust the *selector* only, keeping the assertions.)

- [ ] **Step 6: Locale parity + commit**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__`
Expected: PASS (en/ru parity holds).

```bash
git add frontend/web/src/components/player/OtherSubsPanel.vue frontend/web/src/components/player/OtherSubsPanel.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "feat(player): full-screen other-subs panel with provider + language filters"
```

---

## Task 6: Wire the API key + deploy

**Files:**
- Modify: `docker/docker-compose.yml:423`
- Modify: `docker/.env` (gitignored — NOT committed)

- [ ] **Step 1: Forward the env var to catalog**

In `docker/docker-compose.yml`, in the **catalog** service's `environment:` block, directly after line 423 (`JIMAKU_API_KEY: ${JIMAKU_API_KEY:-}`) add:

```yaml
      OPENSUBTITLES_API_KEY: ${OPENSUBTITLES_API_KEY:-}
      OPENSUBTITLES_USER_AGENT: ${OPENSUBTITLES_USER_AGENT:-AnimeEnigma/1.0}
```

- [ ] **Step 2: Add the key to `docker/.env`** (gitignored)

Append to `docker/.env` (replace with the real key supplied by the user):

```
# OpenSubtitles (any-language subs for the Raw player)
OPENSUBTITLES_API_KEY=<paste-key-here>
```

Verify it is NOT staged: `git status --short docker/.env` must show nothing (confirm `docker/.env` is in `.gitignore`).

- [ ] **Step 3: Redeploy catalog + web, then health-check**

```bash
cd /data/animeenigma
make redeploy-catalog
make redeploy-web
make health
```

Expected: catalog + web rebuild and report healthy. (i18n-lint gating `redeploy-web` is known flaky — if it reports phantom missing keys, re-run `make redeploy-web`; trust the vitest locale-parity test from Task 5.)

- [ ] **Step 4: Commit the compose change**

```bash
git add docker/docker-compose.yml
git commit -m "chore(catalog): forward OPENSUBTITLES_API_KEY/USER_AGENT to catalog service"
```

---

## Task 7: End-to-end verification (DRIVE the real pipeline)

> Per project rule: verify against a real anime, not just a mock. A rising counter or "menu opened" is not proof — confirm non-JP tracks appear AND one actually renders. Memory: `feedback_verify_streams`, `project_vidstream_vip_regex_verification` (drive the full pipeline).

- [ ] **Step 1: Confirm the provider is now configured (search returns OpenSubtitles tracks)**

Pick a popular title's catalog UUID (find one via the DB or an existing Raw-player URL). Then:

```bash
# Replace UUID + episode with a real, popular title (e.g. a mainline shounen).
curl -s "http://localhost:8000/api/anime/<UUID>/subtitles/all?episode=1" | jq '{down: .providers_down, langs: (.languages | keys)}'
```

Expected: `langs` includes non-JP codes (e.g. `en`, plus others), and `providers_down` does NOT contain `opensubtitles`. If `opensubtitles` is listed in `providers_down`, inspect catalog logs: `make logs-catalog | grep -i opensub` (likely a bad key, a missing IMDb/TMDB mapping for that title, or a 429).

- [ ] **Step 2: Confirm a resolve actually returns subtitle text (spends 1 quota, then caches)**

```bash
# Grab one OpenSubtitles track's resolve path from the search response:
URL=$(curl -s "http://localhost:8000/api/anime/<UUID>/subtitles/all?episode=1" \
  | jq -r '[.languages[][] | select(.provider=="opensubtitles")][0].url')
echo "resolve path: $URL"
curl -s "http://localhost:8000${URL}" | head -5         # first call: spends quota
curl -s -o /dev/null -w "cache hit status: %{http_code}\n" "http://localhost:8000${URL}"  # second: from cache
```

Expected: the first call prints recognizable subtitle text (SRT index/timestamps, `WEBVTT`, or `[Script Info]`); the second returns `200` fast. If you get `429`, the free-tier daily quota is exhausted — the UX message is correct; note it and retry later.

- [ ] **Step 3: Browser smoke (the actual user flow)**

Open the Raw player for that title, open the **Other subs** panel. Confirm:
- The panel is full-screen.
- Multiple languages appear (not just Japanese), with provider badges.
- The **Source** filter (All / Jimaku / OpenSubtitles) and **Language** filter chips (with counts) work — selecting OpenSubtitles hides Jimaku-only languages; selecting a language narrows the list.
- Selecting an OpenSubtitles (e.g. English) track makes subtitles render over the video (text appears in sync). This proves the same-origin fetch + resolve + parse path end-to-end.

> Memory `feedback_smoke_verify_i18n`: also glance that the new filter labels render as words ("Source"/"Language"/"All"), not raw `player.otherSubs.filter.*` keys.

- [ ] **Step 4: Full regression of touched suites**

```bash
cd /data/animeenigma/services/catalog && go test ./internal/parser/opensubtitles/... ./internal/service/... ./internal/handler/... -count=1
cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/ src/locales/__tests__ && bunx tsc --noEmit
```

Expected: all green (Redis-dependent service tests skip only if Redis is down).

---

## Post-implementation

Once Task 7 passes, invoke `/animeenigma-after-update` (per CLAUDE.md) to lint/build, redeploy, write the Russian Trump-mode changelog entry, commit, and push. Do not push before then.

---

## Self-Review (completed)

- **Spec coverage:** OpenSubtitles configured (T6), download-link resolution (T1+T2), lazy+24h-cache quota model (T2 `ResolveOpenSubtitlesFile`), resolve endpoint (T3), same-origin frontend fetch / no-allowlist-change (T4), full-screen panel + provider/language filters (T5), compose wiring (T6), verification driving the real pipeline (T7). All spec sections map to a task.
- **Placeholders:** none — every code step has complete code; the only `<paste-key-here>`/`<UUID>` tokens are deliberate user-supplied runtime values, called out as such.
- **Type consistency:** `Download(ctx, fileID int) ([]byte, string, error)` (T1) is consumed identically in `ResolveOpenSubtitlesFile` (T2); `ResolveOpenSubtitlesFile(ctx, fileID) ([]byte, string, error)` is consumed identically in the handler (T3); `cachedSubFile{Body []byte; Format string}` round-trips via JSON base64; track `url` resolve-path format string in T2 matches the route pattern in T3 and the same-origin (`/`-prefixed) branch in T4; `providerFilter`/`langFilter`/`filteredGroups`/`orderLangs`/`languageOptions`/`providerOptions` are defined once in T5 and referenced consistently in its template.
