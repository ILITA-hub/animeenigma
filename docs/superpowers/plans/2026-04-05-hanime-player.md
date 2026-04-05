# Hanime Player Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Hanime.tv as a 5th video provider for hentai content, with a new "18+" language tab.

**Architecture:** New Go parser client authenticates with Hanime's undocumented API (universal-cdn.com), searches by anime name, returns MP4 streams at multiple qualities. Frontend adds a HanimePlayer.vue component under a new "18+" language tab in Anime.vue.

**Tech Stack:** Go (backend parser + service methods), Vue 3 + TypeScript (frontend player), HTML5 `<video>` for MP4 playback.

**Spec:** `docs/superpowers/specs/2026-04-05-hanime-player-design.md`

---

### File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `services/catalog/internal/parser/hanime/client.go` | Hanime API client (auth, search, episodes, streams) |
| Modify | `services/catalog/internal/config/config.go` | Add HanimeConfig struct + env vars |
| Modify | `services/catalog/internal/domain/anime.go` | Add Hanime domain types |
| Modify | `services/catalog/internal/service/catalog.go` | Add hanimeClient field, service methods |
| Modify | `services/catalog/internal/handler/catalog.go` | Add Hanime HTTP handlers |
| Modify | `services/catalog/internal/transport/router.go` | Register Hanime routes |
| Modify | `services/catalog/cmd/catalog-api/main.go` | Pass Hanime config to service |
| Modify | `libs/videoutils/proxy.go` | Add Hanime CDN to allowed domains |
| Modify | `frontend/web/src/api/client.ts` | Add hanimeApi methods |
| Create | `frontend/web/src/components/player/HanimePlayer.vue` | Hanime player component |
| Modify | `frontend/web/src/views/Anime.vue` | Add 18+ tab + HanimePlayer integration |
| Modify | `frontend/web/src/types/preference.ts` | Extend WatchCombo type with hanime |

---

### Task 1: Hanime Parser Client

**Files:**
- Create: `services/catalog/internal/parser/hanime/client.go`

- [ ] **Step 1: Create the parser client file**

```go
package hanime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	appBaseURL    = "https://www.universal-cdn.com/rapi/v4"
	searchBaseURL = "https://search.htv-services.com/"
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Client is an HTTP client for the Hanime.tv API.
type Client struct {
	httpClient   *http.Client
	email        string
	password     string
	sessionToken string
	tokenExpiry  time.Time
	mu           sync.Mutex
}

// SearchRequest is the body for the search endpoint.
type SearchRequest struct {
	SearchText string   `json:"search_text"`
	Tags       []string `json:"tags"`
	Blacklist  []string `json:"blacklist"`
	Brands     []string `json:"brands"`
	OrderBy    string   `json:"order_by"`
	Ordering   string   `json:"ordering"`
	Page       int      `json:"page"`
	TagsMode   string   `json:"tags_mode"`
}

// SearchHit represents a single search result.
type SearchHit struct {
	Name  string   `json:"name"`
	Slug  string   `json:"slug"`
	Brand string   `json:"brand"`
	Tags  []string `json:"tags"`
}

// SearchResponse is the search endpoint response.
type SearchResponse struct {
	Hits    json.RawMessage `json:"hits"`
	NbPages int             `json:"nbPages"`
}

// VideoResponse is the authenticated video endpoint response.
type VideoResponse struct {
	HentaiVideo              HentaiVideo        `json:"hentai_video"`
	VideosManifest           VideosManifest      `json:"videos_manifest"`
	HentaiFranchise          HentaiFranchise     `json:"hentai_franchise"`
	HentaiFranchiseVideos    []FranchiseEpisode  `json:"hentai_franchise_hentai_videos"`
}

// HentaiVideo is video metadata.
type HentaiVideo struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Brand       string `json:"brand"`
	Views       int    `json:"views"`
	PosterURL   string `json:"poster_url"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
}

// VideosManifest contains server/stream info.
type VideosManifest struct {
	Servers []Server `json:"servers"`
}

// Server is a video CDN server.
type Server struct {
	Name    string   `json:"name"`
	Streams []Stream `json:"streams"`
}

// Stream is a single video stream with quality info.
type Stream struct {
	URL              string  `json:"url"`
	Height           string  `json:"height"`
	Width            int     `json:"width"`
	FilesizeMBs      float64 `json:"filesize_mbs"`
	Extension        string  `json:"extension"`
	MimeType         string  `json:"mime_type"`
	IsGuestAllowed   bool    `json:"is_guest_allowed"`
	IsMemberAllowed  bool    `json:"is_member_allowed"`
	IsPremiumAllowed bool    `json:"is_premium_allowed"`
}

// HentaiFranchise groups related episodes.
type HentaiFranchise struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// FranchiseEpisode is an episode entry within a franchise.
type FranchiseEpisode struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// SessionResponse is the login response.
type sessionResponse struct {
	SessionToken          string `json:"session_token"`
	SessionTokenExpireUnix int64  `json:"session_token_expire_time_unix"`
}

// NewClient creates a new Hanime API client.
func NewClient(email, password string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		email:      email,
		password:   password,
	}
}

// IsConfigured returns true if credentials are set.
func (c *Client) IsConfigured() bool {
	return c.email != "" && c.password != ""
}

// computeSignature generates the x-signature header value.
func computeSignature(t int64) string {
	str := fmt.Sprintf("9944822%d8%d113", t, t)
	h := sha256.Sum256([]byte(str))
	return fmt.Sprintf("%x", h)
}

// authenticate logs in and stores the session token.
func (c *Client) authenticate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Token still valid
	if c.sessionToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return nil
	}

	t := time.Now().Unix()
	body := fmt.Sprintf(`{"burger":%q,"fries":%q}`, c.email, c.password)

	req, err := http.NewRequest("POST", appBaseURL+"/sessions", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("x-claim", fmt.Sprintf("%d", t))
	req.Header.Set("x-signature-version", "app2")
	req.Header.Set("x-signature", computeSignature(t))
	req.Header.Set("x-session-token", "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(data))
	}

	var session sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}

	c.sessionToken = session.SessionToken
	c.tokenExpiry = time.Unix(session.SessionTokenExpireUnix, 0)
	return nil
}

// doAuthenticatedRequest makes a GET request with auth headers.
func (c *Client) doAuthenticatedRequest(url string) ([]byte, error) {
	if err := c.authenticate(); err != nil {
		return nil, err
	}

	t := time.Now().Unix()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-claim", fmt.Sprintf("%d", t))
	req.Header.Set("x-signature-version", "app2")
	req.Header.Set("x-signature", computeSignature(t))
	req.Header.Set("x-session-token", c.sessionToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed (status %d): %s", resp.StatusCode, string(data))
	}

	return io.ReadAll(resp.Body)
}

// Search searches for hentai by title. No auth required.
func (c *Client) Search(title string) ([]SearchHit, error) {
	sr := SearchRequest{
		SearchText: title,
		Tags:       []string{},
		Blacklist:  []string{},
		Brands:     []string{},
		OrderBy:    "title_sortable",
		Ordering:   "asc",
		Page:       0,
		TagsMode:   "AND",
	}
	body, err := json.Marshal(sr)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", searchBaseURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed (status %d)", resp.StatusCode)
	}

	var sr2 SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr2); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	// Hits can be a JSON string or a JSON array
	var hits []SearchHit
	raw := sr2.Hits

	// Try as string first (Hanime sometimes wraps hits as a JSON string)
	var hitsStr string
	if json.Unmarshal(raw, &hitsStr) == nil {
		if err := json.Unmarshal([]byte(hitsStr), &hits); err != nil {
			return nil, fmt.Errorf("decode hits string: %w", err)
		}
	} else if err := json.Unmarshal(raw, &hits); err != nil {
		return nil, fmt.Errorf("decode hits: %w", err)
	}

	return hits, nil
}

// GetVideo fetches video details and stream URLs. Requires auth.
func (c *Client) GetVideo(slug string) (*VideoResponse, error) {
	data, err := c.doAuthenticatedRequest(appBaseURL + "/hentai-videos/" + slug)
	if err != nil {
		return nil, err
	}

	var vr VideoResponse
	if err := json.Unmarshal(data, &vr); err != nil {
		return nil, fmt.Errorf("decode video: %w", err)
	}

	return &vr, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./internal/parser/hanime/`
Expected: clean compile, no errors.

- [ ] **Step 3: Commit**

```bash
git add services/catalog/internal/parser/hanime/client.go
git commit -m "feat(hanime): add parser client with auth, search, and video endpoints"
```

---

### Task 2: Config + Domain Types

**Files:**
- Modify: `services/catalog/internal/config/config.go`
- Modify: `services/catalog/internal/domain/anime.go`

- [ ] **Step 1: Add HanimeConfig to config.go**

In the `Config` struct (after `AnimeLib AnimeLibConfig`), add:
```go
Hanime      HanimeConfig
```

Add the config type (after `AnimeLibConfig`):
```go
type HanimeConfig struct {
	Email    string
	Password string
}
```

In the `Load()` function (after `AnimeLib` block), add:
```go
Hanime: HanimeConfig{
	Email:    getEnv("HANIME_EMAIL", ""),
	Password: getEnv("HANIME_PASSWORD", ""),
},
```

- [ ] **Step 2: Add domain types to anime.go**

Append after the `ConsumetSearchResult` type:

```go
// HanimeEpisode represents an episode from Hanime.
type HanimeEpisode struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// HanimeSource represents a single video quality source from Hanime.
type HanimeSource struct {
	URL      string  `json:"url"`
	Height   string  `json:"height"`
	Width    int     `json:"width"`
	SizeMB   float64 `json:"size_mb"`
}

// HanimeStream represents stream data from Hanime.
type HanimeStream struct {
	Sources []HanimeSource `json:"sources"`
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./...`
Expected: clean compile.

- [ ] **Step 4: Commit**

```bash
git add services/catalog/internal/config/config.go services/catalog/internal/domain/anime.go
git commit -m "feat(hanime): add config and domain types"
```

---

### Task 3: Service Layer Integration

**Files:**
- Modify: `services/catalog/internal/service/catalog.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`

- [ ] **Step 1: Add hanimeClient to CatalogService struct**

In `CatalogService` struct (after `animelibClient *animelib.Client`), add:
```go
hanimeClient  *hanime.Client
```

Add import for the hanime parser package:
```go
"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/hanime"
```

- [ ] **Step 2: Add Hanime fields to CatalogServiceOptions**

In `CatalogServiceOptions` (after `AnimeLibToken string`), add:
```go
HanimeEmail    string
HanimePassword string
```

- [ ] **Step 3: Initialize hanimeClient in NewCatalogService**

After the `animelibClient` initialization block (around line 103), add:
```go
hanimeClient := hanime.NewClient(hanimeEmail, hanimePassword)
if hanimeClient.IsConfigured() {
	log.Infow("hanime client initialized")
} else {
	log.Infow("hanime client not configured, hanime features will be unavailable")
}
```

Extract the variables from opts (in the opts block around line 82-88), add:
```go
var hanimeEmail, hanimePassword string
```
And inside `if len(opts) > 0`:
```go
hanimeEmail = opts[0].HanimeEmail
hanimePassword = opts[0].HanimePassword
```

In the `return &CatalogService{...}` block, add:
```go
hanimeClient:  hanimeClient,
```

- [ ] **Step 4: Add accessor method**

After the existing accessor methods:
```go
func (s *CatalogService) HanimeClient() *hanime.Client {
	return s.hanimeClient
}
```

- [ ] **Step 5: Add service methods**

Append to catalog.go:

```go
// GetHanimeEpisodes searches Hanime for an anime and returns its franchise episodes.
func (s *CatalogService) GetHanimeEpisodes(ctx context.Context, animeID string) (_ []domain.HanimeEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hanime", "get_episodes", start, &retErr)

	if !s.hanimeClient.IsConfigured() {
		return nil, errors.NotFound("hanime provider is not configured")
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("hanime:episodes:%s", animeID)
	var cached []domain.HanimeEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Search by English name, then Japanese name
	var hits []hanime.SearchHit
	searchNames := []string{anime.Name}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	for _, name := range searchNames {
		hits, err = s.hanimeClient.Search(name)
		if err == nil && len(hits) > 0 {
			break
		}
	}

	if len(hits) == 0 {
		_ = s.cache.Set(ctx, cacheKey, []domain.HanimeEpisode{}, 30*time.Minute)
		return []domain.HanimeEpisode{}, nil
	}

	// Get the first hit's video to access franchise info
	video, err := s.hanimeClient.GetVideo(hits[0].Slug)
	if err != nil {
		s.log.Warnw("failed to get hanime video", "slug", hits[0].Slug, "error", err)
		return []domain.HanimeEpisode{}, nil
	}

	episodes := make([]domain.HanimeEpisode, len(video.HentaiFranchiseVideos))
	for i, ep := range video.HentaiFranchiseVideos {
		episodes[i] = domain.HanimeEpisode{
			Name: ep.Name,
			Slug: ep.Slug,
		}
	}

	// If no franchise episodes, treat the single video as one episode
	if len(episodes) == 0 {
		episodes = []domain.HanimeEpisode{{
			Name: video.HentaiVideo.Name,
			Slug: video.HentaiVideo.Slug,
		}}
	}

	_ = s.cache.Set(ctx, cacheKey, episodes, time.Hour)
	return episodes, nil
}

// GetHanimeStream fetches stream URLs for a specific Hanime episode slug.
func (s *CatalogService) GetHanimeStream(ctx context.Context, animeID string, slug string) (_ *domain.HanimeStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hanime", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("hanime").Inc()

	if !s.hanimeClient.IsConfigured() {
		return nil, errors.NotFound("hanime provider is not configured")
	}

	cacheKey := fmt.Sprintf("hanime:stream:%s:%s", animeID, slug)
	var cached domain.HanimeStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	video, err := s.hanimeClient.GetVideo(slug)
	if err != nil {
		return nil, errors.NotFound(fmt.Sprintf("Stream unavailable: %s", err.Error()))
	}

	var sources []domain.HanimeSource
	for _, srv := range video.VideosManifest.Servers {
		for _, stream := range srv.Streams {
			if stream.URL == "" {
				continue
			}
			sources = append(sources, domain.HanimeSource{
				URL:    stream.URL,
				Height: stream.Height,
				Width:  stream.Width,
				SizeMB: stream.FilesizeMBs,
			})
		}
	}

	if len(sources) == 0 {
		return nil, errors.NotFound("no stream sources available")
	}

	result := &domain.HanimeStream{
		Sources: sources,
	}

	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)
	return result, nil
}
```

- [ ] **Step 6: Pass Hanime config in main.go**

In `main.go`, inside the `service.CatalogServiceOptions{...}` block (after `AnimeLibToken`), add:
```go
HanimeEmail:    cfg.Hanime.Email,
HanimePassword: cfg.Hanime.Password,
```

- [ ] **Step 7: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./...`
Expected: clean compile.

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/service/catalog.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(hanime): add service layer with episodes and stream methods"
```

---

### Task 4: HTTP Handlers + Routes

**Files:**
- Modify: `services/catalog/internal/handler/catalog.go`
- Modify: `services/catalog/internal/transport/router.go`

- [ ] **Step 1: Add handler methods to catalog.go**

Append to the handler file:

```go
// GetHanimeEpisodes gets episodes from Hanime for an anime.
func (h *CatalogHandler) GetHanimeEpisodes(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	episodes, err := h.catalogService.GetHanimeEpisodes(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, episodes)
}

// GetHanimeStream gets stream URLs for a Hanime episode.
func (h *CatalogHandler) GetHanimeStream(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}

	slug := r.URL.Query().Get("slug")
	if slug == "" {
		httputil.BadRequest(w, "slug is required")
		return
	}

	stream, err := h.catalogService.GetHanimeStream(r.Context(), animeID, slug)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, stream)
}
```

- [ ] **Step 2: Register routes in router.go**

In `router.go`, after the AnimeLib routes block (line 89), add:

```go
// Hanime video sources
r.Get("/{animeId}/hanime/episodes", catalogHandler.GetHanimeEpisodes)
r.Get("/{animeId}/hanime/stream", catalogHandler.GetHanimeStream)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./...`
Expected: clean compile.

- [ ] **Step 4: Commit**

```bash
git add services/catalog/internal/handler/catalog.go services/catalog/internal/transport/router.go
git commit -m "feat(hanime): add HTTP handlers and routes"
```

---

### Task 5: Proxy Allowed Domains

**Files:**
- Modify: `libs/videoutils/proxy.go`

- [ ] **Step 1: Add Hanime CDN domains to allowed list**

In `HLSProxyAllowedDomains` (after `"hentaicdn.org"`), add:

```go
// Hanime video CDN
"highwinds-cdn.com",      // Hanime HLS CDN
"m3u8s.highwinds-cdn.com", // Hanime M3U8 segments
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma/libs/videoutils && go build ./...`
Expected: clean compile.

- [ ] **Step 3: Commit**

```bash
git add libs/videoutils/proxy.go
git commit -m "feat(hanime): add Hanime CDN domains to proxy allowlist"
```

---

### Task 6: Frontend API Client

**Files:**
- Modify: `frontend/web/src/api/client.ts`

- [ ] **Step 1: Add hanimeApi to client.ts**

After the `animeLibApi` export, add:

```typescript
export const hanimeApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/hanime/episodes`),
  getStream: (animeId: string, slug: string) =>
    apiClient.get(`/anime/${animeId}/hanime/stream`, {
      params: { slug }
    }),
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/web/src/api/client.ts
git commit -m "feat(hanime): add frontend API client methods"
```

---

### Task 7: HanimePlayer.vue Component

**Files:**
- Create: `frontend/web/src/components/player/HanimePlayer.vue`

- [ ] **Step 1: Create the player component**

Create `frontend/web/src/components/player/HanimePlayer.vue`:

```vue
<template>
  <div class="space-y-4">
    <!-- Error -->
    <div v-if="error" class="bg-red-500/10 border border-red-500/30 rounded-lg p-4 text-red-400 text-sm">
      {{ error }}
    </div>

    <!-- Episode selector -->
    <div v-if="episodes.length > 0" class="flex flex-wrap gap-2">
      <button
        v-for="ep in episodes"
        :key="ep.slug"
        @click="selectEpisode(ep)"
        class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
        :class="selectedEpisode?.slug === ep.slug
          ? 'bg-pink-500/20 text-pink-400 border border-pink-500/50'
          : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
      >
        {{ episodeLabel(ep) }}
      </button>
    </div>

    <!-- Loading -->
    <div v-if="loadingEpisodes || loadingStream" class="flex items-center justify-center py-12">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-pink-400"></div>
    </div>

    <!-- No episodes found -->
    <div v-else-if="!loadingEpisodes && episodes.length === 0 && !error" class="text-center py-12 text-white/40">
      No episodes found on Hanime
    </div>

    <!-- Video player -->
    <div v-if="streamUrl" class="relative">
      <!-- Quality selector -->
      <div v-if="sources.length > 1" class="flex gap-2 mb-2">
        <button
          v-for="src in sources"
          :key="src.height"
          @click="selectQuality(src)"
          class="px-3 py-1 rounded text-xs font-medium transition-all"
          :class="selectedSource?.height === src.height
            ? 'bg-pink-500/20 text-pink-400 border border-pink-500/50'
            : 'bg-white/5 text-white/60 hover:bg-white/10'"
        >
          {{ src.height }}p
          <span v-if="src.size_mb" class="text-white/30 ml-1">({{ Math.round(src.size_mb) }}MB)</span>
        </button>
      </div>

      <video
        ref="videoEl"
        :src="proxyUrl"
        class="w-full rounded-lg bg-black"
        controls
        autoplay
        @timeupdate="handleTimeUpdate"
        @pause="saveProgress"
        @ended="handleEnded"
      />
    </div>

    <!-- Report button -->
    <ReportButton
      v-if="selectedEpisode"
      :anime-id="animeId"
      :anime-name="animeName"
      :episode-number="currentEpisodeNumber"
      player-type="hanime"
      :video-url="streamUrl || ''"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { hanimeApi, userApi } from '@/api/client'
import ReportButton from './ReportButton.vue'

interface HanimeEpisode {
  name: string
  slug: string
}

interface HanimeSource {
  url: string
  height: string
  width: number
  size_mb: number
}

interface Props {
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
}

const props = defineProps<Props>()

const episodes = ref<HanimeEpisode[]>([])
const selectedEpisode = ref<HanimeEpisode | null>(null)
const sources = ref<HanimeSource[]>([])
const selectedSource = ref<HanimeSource | null>(null)
const streamUrl = ref<string | null>(null)
const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)
const videoEl = ref<HTMLVideoElement | null>(null)

const currentEpisodeNumber = computed(() => {
  if (!selectedEpisode.value) return 0
  const idx = episodes.value.findIndex(e => e.slug === selectedEpisode.value!.slug)
  return idx + 1
})

const proxyUrl = computed(() => {
  if (!streamUrl.value) return ''
  const params = new URLSearchParams()
  params.set('url', streamUrl.value)
  params.set('referer', 'https://hanime.tv/')
  return `/api/streaming/hls-proxy?${params.toString()}`
})

function episodeLabel(ep: HanimeEpisode): string {
  const idx = episodes.value.indexOf(ep) + 1
  return `${idx}. ${ep.name}`
}

async function fetchEpisodes() {
  loadingEpisodes.value = true
  error.value = null
  try {
    const { data } = await hanimeApi.getEpisodes(props.animeId)
    episodes.value = data || []
    if (episodes.value.length > 0) {
      const startIdx = props.initialEpisode ? Math.min(props.initialEpisode - 1, episodes.value.length - 1) : 0
      selectEpisode(episodes.value[Math.max(0, startIdx)])
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load episodes'
  } finally {
    loadingEpisodes.value = false
  }
}

async function selectEpisode(ep: HanimeEpisode) {
  selectedEpisode.value = ep
  await fetchStream(ep.slug)
}

async function fetchStream(slug: string) {
  loadingStream.value = true
  error.value = null
  streamUrl.value = null
  sources.value = []
  selectedSource.value = null
  try {
    const { data } = await hanimeApi.getStream(props.animeId, slug)
    sources.value = (data.sources || []).sort((a: HanimeSource, b: HanimeSource) =>
      parseInt(b.height) - parseInt(a.height)
    )
    if (sources.value.length > 0) {
      // Prefer 720p, then highest available
      const preferred = sources.value.find(s => s.height === '720') || sources.value[0]
      selectQuality(preferred)
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load stream'
  } finally {
    loadingStream.value = false
  }
}

function selectQuality(src: HanimeSource) {
  const currentTime = videoEl.value?.currentTime || 0
  selectedSource.value = src
  streamUrl.value = src.url
  nextTick(() => {
    if (videoEl.value && currentTime > 0) {
      videoEl.value.currentTime = currentTime
    }
  })
}

// Progress tracking
function saveProgress() {
  if (!videoEl.value || !selectedEpisode.value) return
  const key = `watch_progress:${props.animeId}`
  const existing = JSON.parse(localStorage.getItem(key) || '{}')
  existing[currentEpisodeNumber.value] = {
    time: videoEl.value.currentTime,
    duration: videoEl.value.duration,
    updatedAt: Date.now(),
  }
  localStorage.setItem(key, JSON.stringify(existing))
}

function handleTimeUpdate() {
  if (!videoEl.value) return
  // Save progress every 15 seconds
  if (Math.floor(videoEl.value.currentTime) % 15 === 0) {
    saveProgress()
  }
}

function handleEnded() {
  saveProgress()
  if (!selectedEpisode.value) return
  // Mark as watched
  userApi.markEpisodeWatched(props.animeId, currentEpisodeNumber.value, {
    player: 'hanime',
    language: '18+',
    watch_type: 'sub',
    translation_id: 'hanime',
    translation_title: 'Hanime',
  }).catch(() => {})

  // Auto-advance to next episode
  const idx = episodes.value.findIndex(e => e.slug === selectedEpisode.value!.slug)
  if (idx < episodes.value.length - 1) {
    selectEpisode(episodes.value[idx + 1])
  }
}

onMounted(() => {
  fetchEpisodes()
})
</script>
```

- [ ] **Step 2: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build 2>&1 | tail -5`
Expected: build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/components/player/HanimePlayer.vue
git commit -m "feat(hanime): add HanimePlayer.vue component"
```

---

### Task 8: Integrate into Anime.vue

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`
- Modify: `frontend/web/src/types/preference.ts`

- [ ] **Step 1: Add HanimePlayer async import**

After the AnimeLibPlayer import (line 578), add:

```typescript
const HanimePlayer = defineAsyncComponent(() => import('@/components/player/HanimePlayer.vue'))
```

- [ ] **Step 2: Extend videoLanguage type**

Change `videoLanguage` ref (line 639) from:
```typescript
const videoLanguage = ref<'ru' | 'en'>(
  (localStorage.getItem('preferred_video_language') as 'ru' | 'en') || 'ru'
)
```
to:
```typescript
const videoLanguage = ref<'ru' | 'en' | '18+'>(
  (localStorage.getItem('preferred_video_language') as 'ru' | 'en' | '18+') || 'ru'
)
```

- [ ] **Step 3: Extend videoProvider type**

Change `videoProvider` ref (line 642) from:
```typescript
const videoProvider = ref<'kodik' | 'animelib' | 'hianime' | 'consumet'>(
  (localStorage.getItem('preferred_video_provider') as 'kodik' | 'animelib' | 'hianime' | 'consumet') || 'kodik'
)
```
to:
```typescript
const videoProvider = ref<'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime'>(
  (localStorage.getItem('preferred_video_provider') as 'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime') || 'kodik'
)
```

- [ ] **Step 4: Update switchLanguage function**

Change `switchLanguage` (line 978) from:
```typescript
const switchLanguage = (lang: 'ru' | 'en') => {
```
to:
```typescript
const switchLanguage = (lang: 'ru' | 'en' | '18+') => {
```

Add a new case in the function body (after the `else` block):
```typescript
} else if (lang === '18+') {
  videoProvider.value = 'hanime'
}
```

So the full function becomes:
```typescript
const switchLanguage = (lang: 'ru' | 'en' | '18+') => {
  videoLanguage.value = lang
  if (lang === 'ru') {
    const savedRu = localStorage.getItem('preferred_ru_provider') as 'kodik' | 'animelib' | null
    videoProvider.value = savedRu || 'kodik'
  } else if (lang === 'en') {
    const savedEn = localStorage.getItem('preferred_en_provider') as 'hianime' | 'consumet' | null
    videoProvider.value = savedEn || 'hianime'
  } else if (lang === '18+') {
    videoProvider.value = 'hanime'
  }
}
```

- [ ] **Step 5: Add 18+ language tab button**

In the language tabs section (after the EN button, around line 298, before the closing `</div>`), add:

```vue
<button
  @click="switchLanguage('18+')"
  class="px-3 py-1.5 rounded-md text-sm font-medium transition-all"
  :class="videoLanguage === '18+'
    ? 'bg-white/15 text-white'
    : 'text-white/50 hover:text-white/70'"
>
  18+
</button>
```

- [ ] **Step 6: Add Hanime provider button and player**

After the `<template v-else>` block for EN providers (around line 341, before the closing `</div>` of flex-wrap), add:

```vue
<template v-else-if="videoLanguage === '18+'">
  <button
    @click="videoProvider = 'hanime'"
    class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
    :class="videoProvider === 'hanime'
      ? 'bg-pink-500/20 text-pink-400 border border-pink-500/50'
      : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
  >
    Hanime
  </button>
</template>
```

**Important:** The existing `<template v-else>` for EN must become `<template v-else-if="videoLanguage === 'en'">`  to accommodate the new third branch.

In the player section (after ConsumetPlayer, around line 385), add:

```vue
<!-- Hanime Player -->
<HanimePlayer
  v-else-if="videoProvider === 'hanime'"
  :anime-id="anime.id"
  :anime-name="anime.title"
  :total-episodes="anime.totalEpisodes"
  :initial-episode="lastEpisode"
/>
```

- [ ] **Step 7: Update WatchCombo type**

In `frontend/web/src/types/preference.ts`, update the `player` field:
```typescript
player: 'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime'
```

And the `language` field:
```typescript
language: 'ru' | 'en' | '18+'
```

- [ ] **Step 8: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build 2>&1 | tail -5`
Expected: build succeeds.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/views/Anime.vue frontend/web/src/types/preference.ts
git commit -m "feat(hanime): add 18+ tab and HanimePlayer to Anime.vue"
```

---

### Task 9: Environment Config + Deploy

**Files:**
- Modify: `docker/.env` (or `docker/docker-compose.yml`)

- [ ] **Step 1: Add env vars to docker/.env**

Append:
```
HANIME_EMAIL=
HANIME_PASSWORD=
```

- [ ] **Step 2: Pass env vars to catalog service in docker-compose.yml**

In the catalog service environment section, add:
```yaml
HANIME_EMAIL: ${HANIME_EMAIL:-}
HANIME_PASSWORD: ${HANIME_PASSWORD:-}
```

- [ ] **Step 3: Commit**

```bash
git add docker/.env docker/docker-compose.yml
git commit -m "feat(hanime): add environment variables for Hanime credentials"
```

- [ ] **Step 4: Deploy and test**

Run: `make redeploy-catalog && make redeploy-web`
Check health: `make health`

---

### Task 10: Manual Verification

- [ ] **Step 1: Verify API endpoints**

Test episodes endpoint:
```bash
# Get a hentai anime ID from the DB first
ANIME_ID=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -t -c "SELECT id FROM animes WHERE name ILIKE '%overflow%' LIMIT 1;" | tr -d ' \n')
curl -s "http://localhost:8081/api/anime/${ANIME_ID}/hanime/episodes" | python3 -m json.tool | head -20
```

- [ ] **Step 2: Test stream endpoint**

```bash
# Use a slug from the episodes response
curl -s "http://localhost:8081/api/anime/${ANIME_ID}/hanime/stream?slug=<slug-from-step-1>" | python3 -m json.tool
```

- [ ] **Step 3: Verify frontend**

Open browser, navigate to a hentai anime, verify:
- 18+ tab appears alongside RU/EN
- Hanime button inside 18+ tab
- Episodes load
- Video plays with quality selection
