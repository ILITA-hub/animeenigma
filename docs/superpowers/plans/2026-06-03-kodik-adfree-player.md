# Ad-free Kodik Player Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Play Kodik RU streams ad-free in a native HTML5 player by extracting the real HLS `.m3u8` from the Kodik embed (`/ftor` + ROT/base64 decode) and serving it through the existing HLS proxy, with a 5-second branded pre-roll replacing Kodik's ads.

**Architecture:** A new in-process Go module `libs/kodikextract` turns a Kodik embed URL into decoded `.m3u8` URLs. The catalog service exposes them at `GET /api/anime/{id}/kodik/stream`. A new `KodikAdFreePlayer.vue` (modeled on `RawPlayer.vue`) plays the proxied HLS via hls.js after a client-side branded intro. The legacy iframe `KodikPlayer.vue` is untouched and stays a parallel player choice.

**Tech Stack:** Go (stdlib `net/http` + `net/http/cookiejar`), Vue 3 + TypeScript, hls.js (`~1.5.20`), existing `libs/videoutils` HLS proxy.

**Reference:** Spec at `docs/superpowers/specs/2026-06-03-kodik-adfree-player-design.md`. PoC notes (recipe) in memory `reference_kodik_ftor_stream_extraction`. Mirror existing files: `services/catalog/internal/parser/kodik/client.go` (kodik client), `services/catalog/internal/service/catalog.go:1483` (`GetKodikVideoSource`), `services/catalog/internal/handler/catalog.go:526` (`GetKodikVideo`), `frontend/web/src/components/player/RawPlayer.vue` (direct-stream player), `frontend/web/src/components/player/KodikPlayer.vue` (translation/episode scaffolding).

---

## File Structure

**Create:**
- `libs/kodikextract/go.mod` — module manifest
- `libs/kodikextract/extract.go` — `Resolve(embedURL)` (scrape + `/ftor` POST + decode)
- `libs/kodikextract/decode.go` — ROT/base64 `src` decode
- `libs/kodikextract/extract_test.go` — unit tests against fixtures
- `libs/kodikextract/testdata/ftor.json` — captured `/ftor` response fixture
- `frontend/web/src/components/player/KodikAdFreePlayer.vue` — ad-free player
- `frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts` — Vitest
- `frontend/web/public/branding/intro.mp4` — 5s branded pre-roll asset

**Modify:**
- `go.work` — add `./libs/kodikextract`
- `services/catalog/go.mod` — require + replace `libs/kodikextract`
- `services/catalog/Dockerfile` — COPY the module in the deps stage
- `services/catalog/internal/domain/` — add `KodikStreamSource` type (same file as `KodikVideoSource`)
- `services/catalog/internal/service/catalog.go` — add `GetKodikStreamSource`
- `services/catalog/internal/handler/catalog.go` — add `GetKodikStream`
- `services/catalog/internal/transport/router.go:123` — add `/kodik/stream` route
- `libs/videoutils/proxy.go` — add `solodcdn.com` allowlist entries
- `frontend/web/src/api/client.ts:600` — add `kodikApi.getStream`
- `frontend/web/src/views/Anime.vue` — register chip + async component + conditional render
- `frontend/web/src/locales/en.json` + `ru.json` — `player.kodikAdfree.*` keys

---

## Task 1: `libs/kodikextract` — module skeleton + decode

**Files:**
- Create: `libs/kodikextract/go.mod`
- Create: `libs/kodikextract/decode.go`
- Create: `libs/kodikextract/decode_test.go`

- [ ] **Step 1: Create the module manifest**

Create `libs/kodikextract/go.mod`:

```
module github.com/ILITA-hub/animeenigma/libs/kodikextract

go 1.22
```

- [ ] **Step 2: Write the failing decode test**

Create `libs/kodikextract/decode_test.go`:

```go
package kodikextract

import "testing"

func TestDecodeSrc(t *testing.T) {
	// Real encoded src captured from /ftor (Quintessential Quintuplets ep1, 720p).
	// Decodes (ROT 18 + base64) to a //cloud.solodcdn.com manifest URL.
	const enc = "Tg9rjO91HK5hj2fdHOVsjq5rj20dlFVtkvDejO9pHPUdVhNrUhYgUEUbUERqVK00UBUfTBs0GrIbVLYeHBlqVBIhUrppThC3VLRuHrVuUBI0GBk5VLG2UBlsVrY2UrUhVBG3VhI2WrQeUrGeVrIhUrQdVhQeTu1eVLxwjPU6jENciEHtk3YcjBV1WI"

	got, ok := DecodeSrc(enc)
	if !ok {
		t.Fatal("DecodeSrc returned ok=false, want true")
	}
	if !contains(got, "cloud.solodcdn.com") || !contains(got, "mp4:hls:manifest.m3u8") {
		t.Fatalf("decoded URL unexpected: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd libs/kodikextract && go test ./... -run TestDecodeSrc -v`
Expected: FAIL — `DecodeSrc` undefined (build error).

- [ ] **Step 4: Implement the decoder**

Create `libs/kodikextract/decode.go`:

```go
// Package kodikextract turns a Kodik iframe embed URL into the real HLS
// .m3u8 stream URLs, so the stream can be played ad-free in our own player.
package kodikextract

import (
	"encoding/base64"
	"strings"
)

const upperAlpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const lowerAlpha = "abcdefghijklmnopqrstuvwxyz"

// DecodeSrc decodes one Kodik `links[...].src` value into a stream URL.
//
// Kodik Caesar-shifts the base64 string (letters only; digits/symbols left
// as-is) before serving it. The shift varies per response, so we brute-force
// all 26 rotations and accept the candidate that base64-decodes to a string
// containing the "mp4:hls:manifest" marker.
func DecodeSrc(src string) (string, bool) {
	for rot := 0; rot < 26; rot++ {
		shifted := rotateLetters(src, rot)
		if pad := (4 - len(shifted)%4) % 4; pad > 0 {
			shifted += strings.Repeat("=", pad)
		}
		decoded, err := base64.StdEncoding.DecodeString(shifted)
		if err != nil {
			continue
		}
		out := string(decoded)
		if strings.Contains(out, "mp4:hls:manifest") {
			return out, true
		}
	}
	return "", false
}

func rotateLetters(s string, n int) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(upperAlpha[(int(c-'A')+n)%26])
		case c >= 'a' && c <= 'z':
			b.WriteByte(lowerAlpha[(int(c-'a')+n)%26])
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd libs/kodikextract && go test ./... -run TestDecodeSrc -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add libs/kodikextract/go.mod libs/kodikextract/decode.go libs/kodikextract/decode_test.go
git commit -m "feat(kodikextract): add ROT/base64 src decoder with real-fixture test"
```

---

## Task 2: `libs/kodikextract` — `Resolve` (scrape + /ftor)

**Files:**
- Create: `libs/kodikextract/extract.go`
- Create: `libs/kodikextract/extract_test.go`

- [ ] **Step 1: Write the failing test (parsing + assembly, no network)**

Create `libs/kodikextract/extract_test.go`:

```go
package kodikextract

import "testing"

const sampleEmbed = `
<script>
  videoInfo.type = 'seria';
  videoInfo.hash = '71dcc2d2bb2459ae1ae89f58e17cabff';
  videoInfo.id = '782423';
  var domain = "kodikplayer.com";
  var d_sign = "c0167a7b33be40af";
  var pd_sign = "c0167a7b33be40af";
  var ref = "https://kodikplayer.com/";
  var ref_sign = "a525bb4353fafa27";
</script>`

func TestParseEmbedParams(t *testing.T) {
	p, err := parseEmbedParams(sampleEmbed)
	if err != nil {
		t.Fatalf("parseEmbedParams err: %v", err)
	}
	if p.Type != "seria" || p.ID != "782423" {
		t.Fatalf("type/id wrong: %+v", p)
	}
	if p.Ref != "https://kodikplayer.com/" {
		t.Fatalf("ref wrong: %q (must not match href= attributes)", p.Ref)
	}
	if p.Domain != "kodikplayer.com" || p.DSign == "" || p.RefSign == "" {
		t.Fatalf("signed params missing: %+v", p)
	}
}

func TestParseEmbedParamsMissing(t *testing.T) {
	if _, err := parseEmbedParams("<html>nope</html>"); err == nil {
		t.Fatal("expected error for embed with no params")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd libs/kodikextract && go test ./... -run TestParseEmbedParams -v`
Expected: FAIL — `parseEmbedParams` undefined.

- [ ] **Step 3: Implement `extract.go`**

Create `libs/kodikextract/extract.go`:

```go
package kodikextract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
	baseOrigin = "https://kodikplayer.com"
	ftorPath   = "/ftor" // == atob("L2Z0b3I=") in app.player_single*.js
)

// Stream is one decoded quality variant.
type Stream struct {
	Quality int    // 360, 480, 720, ...
	M3U8URL string // absolute https URL on the Kodik CDN (solodcdn.com)
}

// Result is the decoded set of streams for an embed.
type Result struct {
	Default int      // server's default quality
	Streams []Stream // sorted ascending by Quality
	Referer string   // Referer to send to the CDN ("https://kodikplayer.com/")
}

type embedParams struct {
	Type, Hash, ID            string
	Domain, DSign, PdSign     string
	Ref, RefSign              string
}

var (
	reJSVar  = regexp.MustCompile(`\.(type|hash|id)\s*=\s*'([^']*)'`)
	// ref_sign BEFORE ref so the longer name wins; \b stops href= matching ref=.
	reGoVar  = regexp.MustCompile(`(?m)\b(domain|d_sign|pd_sign|ref_sign|ref)\s*=\s*"([^"]*)"`)
)

func parseEmbedParams(html string) (*embedParams, error) {
	p := &embedParams{}
	for _, m := range reJSVar.FindAllStringSubmatch(html, -1) {
		switch m[1] {
		case "type":
			p.Type = m[2]
		case "hash":
			p.Hash = m[2]
		case "id":
			p.ID = m[2]
		}
	}
	// \b before the name prevents href= matching ref= and pd_sign matching d_sign.
	for _, m := range reGoVar.FindAllStringSubmatch(html, -1) {
		switch m[1] {
		case "domain":
			p.Domain = m[2]
		case "d_sign":
			p.DSign = m[2]
		case "pd_sign":
			p.PdSign = m[2]
		case "ref":
			p.Ref = m[2]
		case "ref_sign":
			p.RefSign = m[2]
		}
	}
	if p.Type == "" || p.Hash == "" || p.ID == "" || p.DSign == "" || p.RefSign == "" {
		return nil, fmt.Errorf("kodikextract: embed page missing required params")
	}
	return p, nil
}

// newClient builds an HTTP client with a cookie jar (to carry __ddg* DDoS-Guard
// cookies from the GET into the /ftor POST) and an IPv4-forced dialer
// (containers have no IPv6 egress; matches libs/videoutils).
func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp4", addr)
			},
		},
	}
}

type ftorResponse struct {
	Default int `json:"default"`
	Links   map[string][]struct {
		Src  string `json:"src"`
		Type string `json:"type"`
	} `json:"links"`
}

// Resolve fetches a Kodik embed URL and returns the decoded HLS streams.
func Resolve(ctx context.Context, embedURL string) (*Result, error) {
	if !strings.HasPrefix(embedURL, "http") {
		embedURL = "https:" + embedURL
	}
	client := newClient()

	// 1. GET embed page (carry cookies).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", baseOrigin+"/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kodikextract: embed GET: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kodikextract: embed GET status %d", resp.StatusCode)
	}

	p, err := parseEmbedParams(string(body))
	if err != nil {
		return nil, err
	}

	// 2. POST /ftor with signed params.
	form := url.Values{
		"d":              {p.Domain},
		"d_sign":         {p.DSign},
		"pd":             {p.Domain},
		"pd_sign":        {p.PdSign},
		"ref":            {p.Ref},
		"ref_sign":       {p.RefSign},
		"bad_user":       {"false"},
		"cdn_is_working": {"true"},
		"type":           {p.Type},
		"hash":           {p.Hash},
		"id":             {p.ID},
	}
	preq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseOrigin+ftorPath, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	preq.Header.Set("User-Agent", userAgent)
	preq.Header.Set("Referer", embedURL)
	preq.Header.Set("Origin", baseOrigin)
	preq.Header.Set("X-Requested-With", "XMLHttpRequest")
	preq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	preq.Header.Set("Accept", "application/json")
	presp, err := client.Do(preq)
	if err != nil {
		return nil, fmt.Errorf("kodikextract: /ftor POST: %w", err)
	}
	pbody, _ := io.ReadAll(presp.Body)
	presp.Body.Close()
	if presp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kodikextract: /ftor status %d", presp.StatusCode)
	}

	var fr ftorResponse
	if err := json.Unmarshal(pbody, &fr); err != nil {
		return nil, fmt.Errorf("kodikextract: /ftor decode: %w", err)
	}

	// 3. Decode each quality's src.
	res := &Result{Default: fr.Default, Referer: baseOrigin + "/"}
	for qStr, links := range fr.Links {
		if len(links) == 0 {
			continue
		}
		dec, ok := DecodeSrc(links[0].Src)
		if !ok {
			continue
		}
		if strings.HasPrefix(dec, "//") {
			dec = "https:" + dec
		}
		q, _ := strconv.Atoi(qStr)
		res.Streams = append(res.Streams, Stream{Quality: q, M3U8URL: dec})
	}
	if len(res.Streams) == 0 {
		return nil, fmt.Errorf("kodikextract: no decodable streams")
	}
	sort.Slice(res.Streams, func(i, j int) bool { return res.Streams[i].Quality < res.Streams[j].Quality })
	return res, nil
}

// PickQuality returns the stream matching want, or the highest ≤ want, or the
// highest available. want==0 means "use Default / highest".
func (r *Result) PickQuality(want int) Stream {
	best := r.Streams[len(r.Streams)-1] // highest
	if want <= 0 {
		for _, s := range r.Streams {
			if s.Quality == r.Default {
				return s
			}
		}
		return best
	}
	var chosen Stream
	found := false
	for _, s := range r.Streams {
		if s.Quality <= want && (!found || s.Quality > chosen.Quality) {
			chosen, found = s, true
		}
	}
	if found {
		return chosen
	}
	return best
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd libs/kodikextract && go test ./... -run TestParseEmbedParams -v`
Expected: PASS (both parse tests).

- [ ] **Step 5: Run go vet + full module tests**

Run: `cd libs/kodikextract && go vet ./... && go test ./... -v`
Expected: vet clean; all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add libs/kodikextract/extract.go libs/kodikextract/extract_test.go
git commit -m "feat(kodikextract): Resolve embed -> decoded HLS streams via /ftor"
```

---

## Task 3: Wire `kodikextract` into the catalog build

**Files:**
- Modify: `go.work`
- Modify: `services/catalog/go.mod`
- Modify: `services/catalog/Dockerfile`

- [ ] **Step 1: Add module to the workspace**

In `go.work`, add `./libs/kodikextract` to the `use (...)` block (keep alphabetical near `./libs/idmapping`):

```
	./libs/idmapping
	./libs/kodikextract
	./libs/logger
```

- [ ] **Step 2: Add require + replace to catalog**

In `services/catalog/go.mod`, add to the `require (...)` block:

```
	github.com/ILITA-hub/animeenigma/libs/kodikextract v0.0.0
```

and to the `replace (...)` block (mirror the existing `libs/idmapping` replace line):

```
	github.com/ILITA-hub/animeenigma/libs/kodikextract => ../../libs/kodikextract
```

- [ ] **Step 3: Add Dockerfile COPY**

In `services/catalog/Dockerfile`, find the deps stage block that COPYs `libs/*/go.mod` (next to the `libs/idmapping` COPY) and add a matching line for `kodikextract`. Search first:

Run: `grep -n "libs/idmapping" services/catalog/Dockerfile`
Then add, mirroring the idmapping lines exactly (both the `go.mod` deps-stage COPY and any full-source `COPY libs/ ...` if present):

```dockerfile
COPY libs/kodikextract/go.mod libs/kodikextract/go.mod
```

- [ ] **Step 4: Sync the workspace**

Run: `cd /data/animeenigma && go work sync && cd services/catalog && go build ./...`
Expected: builds clean (module resolves).

- [ ] **Step 5: Commit**

```bash
git add go.work services/catalog/go.mod services/catalog/go.sum services/catalog/Dockerfile
git commit -m "build(catalog): wire in libs/kodikextract module"
```

---

## Task 4: Domain type + catalog service method

**Files:**
- Modify: `services/catalog/internal/domain/` (the file defining `KodikVideoSource`)
- Modify: `services/catalog/internal/service/catalog.go`

- [ ] **Step 1: Locate the KodikVideoSource definition**

Run: `grep -rn "type KodikVideoSource" services/catalog/internal/domain/`
Open that file; add the new type directly below it:

```go
// KodikStreamSource is the decoded, ad-free HLS stream for a Kodik episode.
// Unlike KodikVideoSource (an iframe embed link), this carries a direct .m3u8
// URL that the frontend proxies through /api/streaming/hls-proxy.
type KodikStreamSource struct {
	StreamURL     string `json:"stream_url"`     // raw .m3u8 on the Kodik CDN
	Referer       string `json:"referer"`        // Referer to send to the CDN
	Quality       int    `json:"quality"`        // chosen quality
	Qualities     []int  `json:"qualities"`      // all available qualities
	Episode       int    `json:"episode"`
	TranslationID int    `json:"translation_id"`
	Translation   string `json:"translation"`
}
```

- [ ] **Step 2: Add the import**

At the top of `services/catalog/internal/service/catalog.go`, add to the import block:

```go
	"github.com/ILITA-hub/animeenigma/libs/kodikextract"
```

- [ ] **Step 3: Add the service method**

In `services/catalog/internal/service/catalog.go`, directly after `GetKodikVideoSource` (ends near line 1540), add:

```go
// GetKodikStreamSource resolves the ad-free HLS stream for a Kodik episode.
// quality<=0 means "use the provider default / highest".
func (s *CatalogService) GetKodikStreamSource(ctx context.Context, animeID string, episode, translationID, quality int) (_ *domain.KodikStreamSource, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("kodik", "get_adfree_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("kodik-adfree").Inc()
	if s.kodikClient == nil {
		return nil, errors.NotFound("kodik not available")
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime.ShikimoriID == "" {
		return nil, errors.NotFound("anime does not have shikimori_id")
	}

	cacheKey := fmt.Sprintf("kodik:stream:%s:%d:%d:%d", animeID, episode, translationID, quality)
	var cached domain.KodikStreamSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	embedLink, err := s.kodikClient.GetEpisodeLink(anime.ShikimoriID, episode, translationID)
	if err != nil {
		s.log.Warnw("kodik adfree: embed link failed", "anime_id", animeID, "episode", episode, "translation_id", translationID, "error", err)
		return nil, errors.NotFound("video not found on kodik")
	}

	resolved, err := kodikextract.Resolve(ctx, embedLink)
	if err != nil {
		s.log.Warnw("kodik adfree: extraction failed", "anime_id", animeID, "embed", embedLink, "error", err)
		return nil, errors.NotFound("could not extract kodik stream")
	}
	chosen := resolved.PickQuality(quality)

	qualities := make([]int, 0, len(resolved.Streams))
	for _, st := range resolved.Streams {
		qualities = append(qualities, st.Quality)
	}

	translationName := ""
	if translations, terr := s.kodikClient.GetTranslations(anime.ShikimoriID); terr == nil {
		for _, t := range translations {
			if t.ID == translationID {
				translationName = t.Title
				break
			}
		}
	}

	source := &domain.KodikStreamSource{
		StreamURL:     chosen.M3U8URL,
		Referer:       resolved.Referer,
		Quality:       chosen.Quality,
		Qualities:     qualities,
		Episode:       episode,
		TranslationID: translationID,
		Translation:   translationName,
	}

	// Cache <1h — the CDN URL carries an expiry token.
	_ = s.cache.Set(ctx, cacheKey, source, 30*time.Minute)
	return source, nil
}
```

- [ ] **Step 4: Build**

Run: `cd services/catalog && go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/domain services/catalog/internal/service/catalog.go
git commit -m "feat(catalog): GetKodikStreamSource via kodikextract"
```

---

## Task 5: Handler + route

**Files:**
- Modify: `services/catalog/internal/handler/catalog.go`
- Modify: `services/catalog/internal/transport/router.go`

- [ ] **Step 1: Add the handler**

In `services/catalog/internal/handler/catalog.go`, directly after `GetKodikVideo` (ends ~line 565), add:

```go
// GetKodikStream returns the decoded ad-free HLS stream for a Kodik episode.
func (h *CatalogHandler) GetKodikStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodeStr := r.URL.Query().Get("episode")
	translationIDStr := r.URL.Query().Get("translation")
	if episodeStr == "" {
		httputil.BadRequest(w, "episode number is required")
		return
	}
	if translationIDStr == "" {
		httputil.BadRequest(w, "translation ID is required")
		return
	}
	episode, err := strconv.Atoi(episodeStr)
	if err != nil {
		httputil.BadRequest(w, "invalid episode number")
		return
	}
	translationID, err := strconv.Atoi(translationIDStr)
	if err != nil {
		httputil.BadRequest(w, "invalid translation ID")
		return
	}
	quality := 0
	if q := r.URL.Query().Get("quality"); q != "" {
		quality, _ = strconv.Atoi(q) // optional; 0 = default
	}

	source, err := h.catalogService.GetKodikStreamSource(r.Context(), animeID, episode, translationID, quality)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, source)
}
```

- [ ] **Step 2: Add the route**

In `services/catalog/internal/transport/router.go`, right after line 123 (`r.Get("/{animeId}/kodik/video", ...)`), add:

```go
			r.Get("/{animeId}/kodik/stream", catalogHandler.GetKodikStream)
```

- [ ] **Step 3: Build**

Run: `cd services/catalog && go build ./...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add services/catalog/internal/handler/catalog.go services/catalog/internal/transport/router.go
git commit -m "feat(catalog): GET /api/anime/{id}/kodik/stream route"
```

---

## Task 6: HLS proxy allowlist — solodcdn.com

**Files:**
- Modify: `libs/videoutils/proxy.go`
- Test: `libs/videoutils/proxy_test.go` (or wherever `isHLSDomainAllowed` is tested)

- [ ] **Step 1: Write the failing allowlist test**

Find the existing allowlist test:

Run: `grep -rn "isHLSDomainAllowed\|HLSProxyAllowedDomains" libs/videoutils/*_test.go`

Add a test (adapt to the existing test's helper/signature — mirror a sibling case):

```go
func TestSolodcdnAllowed(t *testing.T) {
	cases := []string{
		"https://cloud.solodcdn.com/useruploads/x/y:1/720.mp4:hls:manifest.m3u8",
		"https://draco.cloud.solodcdn.com/useruploads/x/y:1/720.mp4:hls:seg-1-v1-a1.ts",
	}
	for _, u := range cases {
		if !isHLSDomainAllowed(u) {
			t.Errorf("expected %s to be allowed", u)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd libs/videoutils && go test ./... -run TestSolodcdnAllowed -v`
Expected: FAIL — domains not yet allowed.

- [ ] **Step 3: Add the allowlist entries**

In `libs/videoutils/proxy.go`, inside `HLSProxyAllowedDomainsWithProvenance`, add near the AnimeLib/CDN cluster:

```go
	// Kodik ad-free HLS CDN (kodikextract). Manifest on cloud.solodcdn.com
	// 302-redirects to node hosts (draco.cloud.solodcdn.com, ...); the eTLD+1
	// entry covers those via the HasSuffix(host, "."+allowed) match.
	{Domain: "solodcdn.com", Reason: "Kodik ad-free HLS manifest + segments (node subdomains)", Owner: "@0neymik0", Added: "2026-06-03"},
	{Domain: "cloud.solodcdn.com", Reason: "Kodik ad-free HLS manifest + relative segment base", Owner: "@0neymik0", Added: "2026-06-03"},
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd libs/videoutils && go test ./... -run TestSolodcdnAllowed -v`
Expected: PASS. If the node subdomain case fails, confirm `isHLSDomainAllowed` uses `strings.HasSuffix(host, "."+allowed)` for eTLD+1 entries (it does for the Phase 18 rotating-subdomain entries) — the `solodcdn.com` entry covers `draco.cloud.solodcdn.com`.

- [ ] **Step 5: Run the full videoutils suite**

Run: `cd libs/videoutils && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add libs/videoutils/proxy.go libs/videoutils/proxy_test.go
git commit -m "feat(proxy): allowlist solodcdn.com for Kodik ad-free HLS"
```

---

## Task 7: Frontend API + i18n + flag + branding asset

**Files:**
- Modify: `frontend/web/src/api/client.ts`
- Modify: `frontend/web/src/locales/en.json`, `frontend/web/src/locales/ru.json`
- Create: `frontend/web/public/branding/intro.mp4`

- [ ] **Step 1: Add the API method**

In `frontend/web/src/api/client.ts`, inside the `kodikApi` object (starts line 600), add a `getStream` method mirroring `getVideo`:

```ts
  getStream: (animeId: string, episode: number, translation: number, quality?: number) =>
    apiClient.get(`/anime/${animeId}/kodik/stream`, {
      params: { episode, translation, ...(quality ? { quality } : {}) },
    }),
```

- [ ] **Step 2: Add i18n keys (both locales)**

In `frontend/web/src/locales/en.json`, add under the `player` namespace:

```json
    "kodikAdfree": {
      "tab": "Kodik (ad-free)",
      "label": "Kodik · ad-free",
      "skipIntro": "Skip",
      "extractError": "Couldn't load the ad-free stream. Try the standard Kodik player or report it."
    }
```

In `frontend/web/src/locales/ru.json`, add the matching keys (parity test enforces this):

```json
    "kodikAdfree": {
      "tab": "Кодик (без рекламы)",
      "label": "Кодик · без рекламы",
      "skipIntro": "Пропустить",
      "extractError": "Не удалось загрузить поток без рекламы. Попробуйте обычный плеер Кодик или сообщите об ошибке."
    }
```

- [ ] **Step 3: Add the branding asset placeholder**

The real 5s logo MP4 is provided by the project owner. For now create the directory and a placeholder so the path resolves; the player degrades gracefully if it is missing/short.

Run:
```bash
mkdir -p frontend/web/public/branding
# Drop the real asset here as intro.mp4 (<=5s, H.264/AAC).
# Until then the player's onerror handler skips straight to the stream.
```

> NOTE: If no asset is supplied, leave `intro.mp4` absent — Task 9's `onerror`/`onstalled` guard skips the intro. Do NOT commit a fake/empty .mp4.

- [ ] **Step 4: Verify locale parity + types**

Run: `cd frontend/web && bunx vitest run src/locales/__tests__ && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/api/client.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "feat(web): kodikApi.getStream + kodikAdfree i18n keys"
```

---

## Task 8: `KodikAdFreePlayer.vue` — scaffold + stream playback

**Files:**
- Create: `frontend/web/src/components/player/KodikAdFreePlayer.vue`
- Create: `frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts`

This player needs Kodik translation + episode selection (like `KodikPlayer.vue`) but HLS playback (like `RawPlayer.vue`). Build the script setup by mirroring those two files; the genuinely new pieces are given in full below.

- [ ] **Step 1: Create the component**

Create `frontend/web/src/components/player/KodikAdFreePlayer.vue`. Mirror `KodikPlayer.vue` for the props (`animeId`, `shikimoriId`, etc.), translation fetching (`kodikApi.getTranslations`), the `EpisodeSelector` block, watched-state (`useWatchedEpisodes`), `SubtitleOverlay`, and `ReportButton`. Replace the iframe with a `<video ref="videoRef">` and use these new functions for playback:

```ts
import Hls from 'hls.js'
import { ref } from 'vue'
import { kodikApi } from '@/api/client'

const videoRef = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null
// Remember the active selection so we can re-extract once if the signed CDN
// URL expires mid-session (spec §5).
let current: { episode: number; translationID: number } | null = null
let reloadedOnce = false

function buildProxyUrl(url: string, referer: string): string {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

function disposePlayer() {
  if (hls) { hls.destroy(); hls = null }
  const v = videoRef.value
  if (v) { v.removeAttribute('src'); try { v.load() } catch { /* ignore */ } }
}

function attachStream(streamUrl: string, referer: string) {
  const v = videoRef.value
  if (!v) return
  disposePlayer()
  const proxyUrl = buildProxyUrl(streamUrl, referer)
  if (Hls.isSupported()) {
    hls = new Hls({ enableWorker: true, lowLatencyMode: false, backBufferLength: 90 })
    hls.loadSource(proxyUrl)
    hls.attachMedia(v)
    hls.on(Hls.Events.MANIFEST_PARSED, () => { v.play().catch(() => {}) })
    hls.on(Hls.Events.ERROR, (_e, data) => {
      if (!data.fatal) return
      // Expired signed CDN URL -> re-extract a fresh stream once, then give up.
      if (!reloadedOnce && current) {
        reloadedOnce = true
        loadStream(current.episode, current.translationID)
      } else {
        streamError.value = true
      }
    })
  } else if (v.canPlayType('application/vnd.apple.mpegurl')) {
    v.src = proxyUrl
    v.addEventListener('loadedmetadata', () => { v.play().catch(() => {}) }, { once: true })
  }
}

const streamError = ref(false)

async function loadStream(episode: number, translationID: number) {
  streamError.value = false
  // Reset the one-shot retry budget only on a NEW selection, never on the
  // retry itself (which re-calls loadStream with the same ep/translation).
  const changed = !current || current.episode !== episode || current.translationID !== translationID
  if (changed) reloadedOnce = false
  current = { episode, translationID }
  try {
    const resp = await kodikApi.getStream(props.animeId, episode, translationID)
    const data = resp.data?.data ?? resp.data
    // Task 9 wraps this in playWithIntro(); for now attach directly.
    attachStream(data.stream_url, data.referer)
  } catch {
    streamError.value = true
  }
}
```

Wire `loadStream(episode, translationID)` to the same selection events `KodikPlayer.vue` uses to load a video (episode pick + translation pick). Render `streamError` as an inline error block that shows `t('player.kodikAdfree.extractError')` plus the existing `ReportButton`. Use `hls.js` import like `RawPlayer.vue` (already pinned `~1.5.20` in package.json).

- [ ] **Step 2: Write a Vitest spec**

Create `frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts` with at least 5 assertions. Mirror the mocking style of `OurEnglishPlayer.spec.ts` (mock `@/api/client`, stub `hls.js`). Cover:

```ts
// 1. renders a <video> element (not an iframe)
// 2. calls kodikApi.getStream on episode+translation selection
// 3. builds a /api/streaming/hls-proxy URL with the returned stream_url + referer
// 4. shows the extractError block when getStream rejects
// 5. renders ReportButton in the error state
```

- [ ] **Step 3: Run the spec + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/__tests__/KodikAdFreePlayer.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/components/player/KodikAdFreePlayer.vue frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts
git commit -m "feat(web): KodikAdFreePlayer with HLS playback via hls-proxy"
```

---

## Task 9: Branded pre-roll intro

**Files:**
- Modify: `frontend/web/src/components/player/KodikAdFreePlayer.vue`
- Modify: `frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts`

- [ ] **Step 1: Add the pre-roll logic**

In `KodikAdFreePlayer.vue` add:

```ts
const INTRO_SRC = '/branding/intro.mp4'
const showSkip = ref(false)
const introPlaying = ref(false)
const introShownFor = new Set<string>()
let skipTimer: ReturnType<typeof setTimeout> | null = null
let proceedFn: (() => void) | null = null

function playWithIntro(streamUrl: string, referer: string, episodeKey: string) {
  const v = videoRef.value
  if (!v) return
  if (introShownFor.has(episodeKey)) { attachStream(streamUrl, referer); return }
  introShownFor.add(episodeKey)

  disposePlayer()
  introPlaying.value = true
  showSkip.value = false

  const proceed = () => {
    if (!introPlaying.value) return
    introPlaying.value = false
    showSkip.value = false
    if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
    v.onended = null; v.onerror = null
    attachStream(streamUrl, referer)
  }
  proceedFn = proceed

  v.src = INTRO_SRC
  v.onended = proceed
  v.onerror = proceed   // missing/unplayable asset -> straight to stream
  skipTimer = setTimeout(() => { showSkip.value = true }, 3000)
  v.play().catch(() => { proceed() }) // autoplay blocked -> don't trap the user on the intro
}

function skipIntro() { proceedFn?.() }
```

Change `loadStream` to call `playWithIntro(data.stream_url, data.referer, `${translationID}:${episode}`)` instead of `attachStream(...)` directly.

- [ ] **Step 2: Add the Skip button to the template**

Inside the player surface, over the `<video>`:

```vue
<button
  v-if="introPlaying && showSkip"
  class="absolute bottom-6 right-6 z-20 rounded-md bg-background/80 px-4 py-2 text-sm font-medium text-foreground hover:bg-accent"
  @click="skipIntro"
>
  {{ $t('player.kodikAdfree.skipIntro') }}
</button>
```

(Adjust container to be `relative`; keep brand-cyan accent consistent with other Kodik UI. Tokens only — no hardcoded hex.)

- [ ] **Step 3: Extend the spec**

Add to `KodikAdFreePlayer.spec.ts`:

```ts
// 6. intro: <video>.src is set to /branding/intro.mp4 before the stream
// 7. intro `ended` event -> attachStream builds the hls-proxy URL
// 8. Skip button hidden initially, shown after the 3s timer (use fake timers)
// 9. second load of the SAME episode skips the intro (introShownFor guard)
```

- [ ] **Step 4: Run the spec + type-check**

Run: `cd frontend/web && bunx vitest run src/components/player/__tests__/KodikAdFreePlayer.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/player/KodikAdFreePlayer.vue frontend/web/src/components/player/__tests__/KodikAdFreePlayer.spec.ts
git commit -m "feat(web): branded 5s pre-roll intro with skip in KodikAdFreePlayer"
```

---

## Task 10: Register the player in `Anime.vue`

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Add the lazy import + flag**

Near line 1000 (the `defineAsyncComponent` block), add:

```ts
// Ad-free Kodik player (libs/kodikextract HLS extraction). Behind a flag so it
// can dark-ship; defaults ON.
const KodikAdFreePlayer = defineAsyncComponent(() => import('@/components/player/KodikAdFreePlayer.vue'))
const kodikAdfreeEnabled = import.meta.env.VITE_KODIK_ADFREE_ENABLED !== 'false'
```

- [ ] **Step 2: Add the provider chip**

Next to the existing Kodik chip (around line 416, the `onUserPickedProvider('kodik')` button), add a sibling chip gated by `kodikAdfreeEnabled`, mirroring that button's markup and selected-state classes:

```vue
<button
  v-if="kodikAdfreeEnabled"
  @click="onUserPickedProvider('kodik-adfree')"
  :aria-pressed="videoProvider === 'kodik-adfree'"
  :class="videoProvider === 'kodik-adfree' ? /* selected classes, copy from the kodik chip */ : /* idle classes */"
>
  {{ $t('player.kodikAdfree.tab') }}
</button>
```

- [ ] **Step 3: Render the player**

Next to the `<KodikPlayer v-if="videoProvider === 'kodik'" ...>` block (around line 530), add:

```vue
<!-- Ad-free Kodik Player (HLS via kodikextract) -->
<KodikAdFreePlayer
  v-if="videoProvider === 'kodik-adfree'"
  :anime-id="anime.id"
  :shikimori-id="anime.shikimoriId"
  ...mirror the prop set passed to <KodikPlayer> above...
/>
```

- [ ] **Step 4: Allow the provider value**

Find the `PlayerKind` / `videoProvider` type union and `onUserPickedProvider` signature (search `type PlayerKind` and `videoProvider`); add `'kodik-adfree'` to the union so `tsc` accepts it. Confirm `onUserPickedProvider` accepts the new value without breaking the resolve cascade (it should — it just sets `videoProvider`).

- [ ] **Step 5: Type-check + unit run**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "feat(web): register ad-free Kodik player chip + render in Anime.vue"
```

---

## Task 11: Build, deploy, and live verification

- [ ] **Step 1: Backend lint + tests**

Run: `cd /data/animeenigma && go work sync && go build ./services/catalog/... && go test ./libs/kodikextract/... ./libs/videoutils/...`
Expected: all PASS.

- [ ] **Step 2: Frontend lint gate + tests**

Run: `cd frontend/web && bash scripts/design-system-lint.sh && bunx vitest run src/components/player/ src/locales/__tests__ && bunx tsc --noEmit`
Expected: lint ERRORS=0; tests PASS.

- [ ] **Step 3: Redeploy**

Run: `cd /data/animeenigma && make redeploy-catalog && make redeploy-web && make health`
Expected: catalog + web healthy.

- [ ] **Step 4: Live API smoke**

```bash
AID=d0444da1-a3af-4ec9-b80d-6493d2706951   # Quintessential Quintuplets
TR=1215
curl -s "http://localhost:8000/api/anime/$AID/kodik/stream?episode=1&translation=$TR" | head -c 400
```
Expected: JSON `{ "success": true, "data": { "stream_url": "https://cloud.solodcdn.com/...m3u8", "referer": "https://kodikplayer.com/", "quality": ..., "qualities": [...] } }`.

- [ ] **Step 5: Proxy smoke (manifest loads through the proxy)**

```bash
M3U8=$(curl -s "http://localhost:8000/api/anime/$AID/kodik/stream?episode=1&translation=$TR" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['stream_url'])")
curl -s "http://localhost:8000/api/streaming/hls-proxy?url=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))" "$M3U8")&referer=https://kodikplayer.com/" -o /tmp/k.m3u8 -w "%{http_code} %{content_type}\n"
head -5 /tmp/k.m3u8
```
Expected: `200 application/vnd.apple.mpegurl`; body starts with `#EXTM3U` and rewritten segment URLs pointing back at `/api/streaming/hls-proxy`.

- [ ] **Step 6: In-browser smoke (DS-NF-06 standing rule)**

Open a real browser on the watch page for the test anime, pick the "Kodik (ad-free)" chip, select a translation + episode. Verify: the 5s intro plays (or skips if no asset), Skip appears ~3s in, then the episode plays with no Kodik ads. Check desktop + mobile widths. Capture any console errors via the player's diagnostics.

- [ ] **Step 7: Run `/animeenigma-after-update`**

This redeploys, writes the Russian Trump-mode changelog entry, commits with co-authors, and pushes. (Do NOT hand-write the changelog — the skill owns it.)

---

## Notes for the implementer

- **DO NOT push** until the `/animeenigma-after-update` step (project push policy: commit anytime, push only via after-update or when explicitly asked).
- The legacy iframe `KodikPlayer.vue` and its watch-together RPC path are **out of scope** — do not modify them.
- If `kodikextract.Resolve` starts returning `/ftor status 500`, re-capture a fresh embed and diff the param names — Kodik rotates these (see memory `reference_kodik_ftor_stream_extraction`). The iframe player remains the always-works fallback.
- Keep `hls.js` at `~1.5.20` (1.6.x has a fatal `bufferAddCodecError` on CODECS-less HLS — see memory `project_hlsjs_16x_codec_regression`).
- Watch-together support for this player is deliberately deferred (Raw was too).
