# Shikimori Image Optimization Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-side watchlist pagination, a backend image proxy with MinIO cache + MAL fallback, and a frontend adaptive image fallback composable — with Grafana observability.

**Architecture:** The player service gets paginated list endpoints + a lightweight statuses endpoint. The streaming service gets a new image proxy handler with singleflight dedup, MinIO cache, and Jikan API fallback. The frontend gets a `useImageProxy` composable that tries Shikimori directly and falls back to the backend proxy after 3 failures (adaptive session switch).

**Tech Stack:** Go (chi, GORM, minio-go, singleflight), Vue 3 (composables, Pinia), Prometheus, Grafana

**Spec:** `docs/superpowers/specs/2026-03-16-shikimori-image-optimization-design.md`

---

## Chunk 1: Server-Side Watchlist Pagination (Player Service)

### File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Modify | `services/player/internal/domain/watch.go` | Add `PaginationParams` struct |
| Modify | `services/player/internal/repo/list.go` | Add paginated query methods + statuses query |
| Modify | `services/player/internal/service/list.go` | Add paginated + statuses service methods |
| Modify | `services/player/internal/handler/list.go` | Add pagination param parsing, `GetWatchlistStatuses` handler |
| Modify | `services/player/internal/transport/router.go` | Add `/watchlist/statuses` route |

---

### Task 1: Add PaginationParams Domain Struct

**Files:**
- Modify: `services/player/internal/domain/watch.go`

- [ ] **Step 1: Add PaginationParams struct**

Add after line 131 in `services/player/internal/domain/watch.go`:

```go
// PaginationParams holds pagination and sorting options
type PaginationParams struct {
	Page    int    `json:"page"`
	PerPage int    `json:"per_page"`
	Sort    string `json:"sort"`
	Order   string `json:"order"`
}

// Validate checks pagination params and applies defaults
func (p *PaginationParams) Validate() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PerPage < 1 || p.PerPage > 100 {
		p.PerPage = 24
	}

	allowedSorts := map[string]bool{
		"updated_at": true,
		"created_at": true,
		"score":      true,
		"status":     true,
	}
	if !allowedSorts[p.Sort] {
		p.Sort = "updated_at"
	}

	if p.Order != "asc" {
		p.Order = "desc"
	}
}

// Offset returns the SQL offset
func (p *PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

// AnimeStatusEntry is a lightweight entry for the status map
type AnimeStatusEntry struct {
	AnimeID string `json:"anime_id" gorm:"column:anime_id"`
	Status  string `json:"status" gorm:"column:status"`
}
```

- [ ] **Step 2: Verify build**

Run: `cd services/player && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/domain/watch.go
git commit -m "feat(player): add PaginationParams and AnimeStatusEntry domain types"
```

---

### Task 2: Add Paginated Repository Methods

**Files:**
- Modify: `services/player/internal/repo/list.go`

- [ ] **Step 1: Add GetByUserPaginated method**

Add after the `GetByUserAndStatus` method (after line 64) in `repo/list.go`:

```go
func (r *ListRepository) GetByUserPaginated(ctx context.Context, userID, status string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != "" {
		base = base.Where("status = ?", status)
	}

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderClause := params.Sort + " " + params.Order
	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(orderClause).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}

func (r *ListRepository) GetByUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	var entries []domain.AnimeStatusEntry
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeListEntry{}).
		Select("anime_id, status").
		Where("user_id = ?", userID).
		Scan(&entries).Error
	return entries, err
}

func (r *ListRepository) GetByUserAndStatusesPaginated(ctx context.Context, userID string, statuses []string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("user_id = ? AND status IN ?", userID, statuses)

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderClause := params.Sort + " " + params.Order
	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(orderClause).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}
```

- [ ] **Step 2: Verify build**

Run: `cd services/player && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/repo/list.go
git commit -m "feat(player): add paginated repository methods for watchlist"
```

---

### Task 3: Add Paginated Service Methods

**Files:**
- Modify: `services/player/internal/service/list.go`

- [ ] **Step 1: Add paginated service methods**

Add after the `GetUserList` method (after line 32) in `service/list.go`:

```go
// GetUserListPaginated returns user's anime list with pagination
func (s *ListService) GetUserListPaginated(ctx context.Context, userID, status string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	return s.listRepo.GetByUserPaginated(ctx, userID, status, params)
}

// GetUserStatuses returns lightweight anime_id+status pairs for the entire list
func (s *ListService) GetUserStatuses(ctx context.Context, userID string) ([]domain.AnimeStatusEntry, error) {
	return s.listRepo.GetByUserStatuses(ctx, userID)
}

// GetPublicWatchlistPaginated returns user's public watchlist with pagination
func (s *ListService) GetPublicWatchlistPaginated(ctx context.Context, userID string, statuses []string, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	if len(statuses) == 0 {
		return s.listRepo.GetByUserPaginated(ctx, userID, "", params)
	}
	return s.listRepo.GetByUserAndStatusesPaginated(ctx, userID, statuses, params)
}
```

- [ ] **Step 2: Verify build**

Run: `cd services/player && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/service/list.go
git commit -m "feat(player): add paginated service methods for watchlist"
```

---

### Task 4: Update Handlers for Pagination

**Files:**
- Modify: `services/player/internal/handler/list.go`
- Modify: `services/player/internal/transport/router.go`

- [ ] **Step 1: Add helper to parse pagination params**

Add at the bottom of `handler/list.go` (before the `splitAndTrim` function, around line 239):

```go
func parsePaginationParams(r *http.Request) *domain.PaginationParams {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	params := &domain.PaginationParams{
		Page:    page,
		PerPage: perPage,
		Sort:    sort,
		Order:   order,
	}
	params.Validate()
	return params
}
```

Add `"strconv"` to the imports block.

- [ ] **Step 2: Replace GetUserList handler body**

Replace the `GetUserList` method (lines 28-49) with:

```go
func (h *ListHandler) GetUserList(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	status := r.URL.Query().Get("status")
	params := parsePaginationParams(r)

	entries, total, err := h.listService.GetUserListPaginated(r.Context(), claims.UserID, status, params)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if entries == nil {
		entries = []*domain.AnimeListEntry{}
	}

	totalPages := int(total) / params.PerPage
	if int(total)%params.PerPage > 0 {
		totalPages++
	}

	httputil.JSONWithMeta(w, http.StatusOK, entries, httputil.Meta{
		Page:       params.Page,
		PageSize:   params.PerPage,
		TotalCount: total,
		TotalPages: totalPages,
	})
}
```

- [ ] **Step 3: Add GetWatchlistStatuses handler**

Add after `GetUserList`:

```go
// GetWatchlistStatuses returns lightweight anime_id+status pairs for the entire user list
func (h *ListHandler) GetWatchlistStatuses(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	statuses, err := h.listService.GetUserStatuses(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if statuses == nil {
		statuses = []domain.AnimeStatusEntry{}
	}

	httputil.OK(w, statuses)
}
```

- [ ] **Step 4: Replace GetPublicWatchlist handler body**

Replace the `GetPublicWatchlist` method (lines 208-238) with:

```go
func (h *ListHandler) GetPublicWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user ID is required")
		return
	}

	// Support both "status" (single) and "statuses" (comma-separated) for backward compat
	var statuses []string
	if s := r.URL.Query().Get("status"); s != "" {
		statuses = []string{s}
	} else if statusesParam := r.URL.Query().Get("statuses"); statusesParam != "" {
		for _, s := range splitAndTrim(statusesParam, ",") {
			if s != "" {
				statuses = append(statuses, s)
			}
		}
	}

	params := parsePaginationParams(r)

	entries, total, err := h.listService.GetPublicWatchlistPaginated(r.Context(), userID, statuses, params)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if entries == nil {
		entries = []*domain.AnimeListEntry{}
	}

	totalPages := int(total) / params.PerPage
	if int(total)%params.PerPage > 0 {
		totalPages++
	}

	httputil.JSONWithMeta(w, http.StatusOK, entries, httputil.Meta{
		Page:       params.Page,
		PageSize:   params.PerPage,
		TotalCount: total,
		TotalPages: totalPages,
	})
}
```

- [ ] **Step 5: Add `/watchlist/statuses` route**

In `services/player/internal/transport/router.go`, add after line 57 (`r.Get("/watchlist", listHandler.GetUserList)`):

**IMPORTANT:** This route MUST appear before `r.Get("/watchlist/{animeId}", ...)` (line 61). Otherwise chi's `{animeId}` parameter will greedily match "statuses" as an anime ID.

```go
r.Get("/watchlist/statuses", listHandler.GetWatchlistStatuses)
```

- [ ] **Step 6: Verify build**

Run: `cd services/player && go build ./...`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add services/player/internal/handler/list.go services/player/internal/transport/router.go
git commit -m "feat(player): add paginated watchlist endpoints and statuses endpoint"
```

---

## Chunk 2: Backend Image Proxy (Streaming Service)

### File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `services/streaming/internal/handler/image_proxy.go` | HTTP handler: URL validation, headers, serve image |
| Create | `services/streaming/internal/service/image_proxy.go` | Fallback chain: MinIO cache → Shikimori → Jikan/MAL → placeholder |
| Modify | `services/streaming/internal/transport/router.go` | Add `/api/v1/image-proxy` route |
| Modify | `services/streaming/cmd/streaming-api/main.go` | Wire up image proxy handler |
| Modify | `services/gateway/internal/transport/router.go` | Add gateway route for image-proxy |

---

### Task 5: Create Image Proxy Service

**Files:**
- Create: `services/streaming/internal/service/image_proxy.go`

- [ ] **Step 1: Add Client() and BucketName() accessor methods to Storage**

In `libs/videoutils/storage.go`, add after the `Storage` struct (after line 29):

```go
// Client returns the underlying MinIO client
func (s *Storage) Client() *minio.Client {
	return s.client
}

// BucketName returns the configured bucket name
func (s *Storage) BucketName() string {
	return s.bucketName
}
```

Run: `cd libs/videoutils && go build ./...`
Expected: Build succeeds

- [ ] **Step 2: Create image proxy service**

Create `services/streaming/internal/service/image_proxy.go`:

```go
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/singleflight"
)

const (
	posterPrefix      = "posters/"
	placeholderPrefix = "posters/placeholder/"
	maxImageSize      = 5 * 1024 * 1024 // 5 MB
	fetchTimeout      = 10 * time.Second
	maxConcurrent     = 50
)

var (
	allowedDomains = []string{
		"shiki.one",
		"shikimori.io",
		"shikimori.one",
		"cdn.myanimelist.net",
	}

	// Matches /uploads/poster/animes/{id}/ in Shikimori poster URLs
	shikimoriAnimeIDRe = regexp.MustCompile(`/uploads/poster/animes/(\d+)/`)
)

// ImageSource describes where an image was resolved from
type ImageSource string

const (
	SourceCache       ImageSource = "cache_hit"
	SourceShikimori   ImageSource = "shikimori"
	SourceMAL         ImageSource = "mal"
	SourcePlaceholder ImageSource = "placeholder"
)

// ImageResult holds the fetched image data and metadata
type ImageResult struct {
	Data        []byte
	ContentType string
	Source      ImageSource
}

// ImageProxyService handles image proxying with MinIO cache and MAL fallback
type ImageProxyService struct {
	storage    *videoutils.Storage
	httpClient *http.Client
	sfGroup    singleflight.Group
	semaphore  chan struct{}
	log        *logger.Logger
}

func NewImageProxyService(storage *videoutils.Storage, log *logger.Logger) *ImageProxyService {
	return &ImageProxyService{
		storage: storage,
		httpClient: &http.Client{
			Timeout: fetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		semaphore: make(chan struct{}, maxConcurrent),
		log:       log,
	}
}

// GetImage resolves an image through the fallback chain: cache → upstream → MAL → placeholder
func (s *ImageProxyService) GetImage(ctx context.Context, rawURL string) (*ImageResult, error) {
	if rawURL == "" {
		return s.placeholderResult(), nil
	}

	if !s.isDomainAllowed(rawURL) {
		return nil, fmt.Errorf("domain not allowed")
	}

	cacheKey := posterPrefix + s.hashURL(rawURL)

	// Singleflight: dedup concurrent requests for the same URL
	result, err, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		return s.resolveImage(ctx, rawURL, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImageResult), nil
}

func (s *ImageProxyService) resolveImage(ctx context.Context, rawURL, cacheKey string) (*ImageResult, error) {
	// 1. Check MinIO cache
	if result, err := s.fromCache(ctx, cacheKey); err == nil {
		return result, nil
	}

	// Also check placeholder cache
	placeholderKey := placeholderPrefix + s.hashURL(rawURL)
	if result, err := s.fromCache(ctx, placeholderKey); err == nil {
		result.Source = SourcePlaceholder
		return result, nil
	}

	// Acquire semaphore for upstream fetch
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 2. Fetch from original URL (Shikimori)
	if data, contentType, err := s.fetchURL(ctx, rawURL); err == nil {
		result := &ImageResult{Data: data, ContentType: contentType, Source: SourceShikimori}
		s.storeInCache(ctx, cacheKey, data, contentType)
		return result, nil
	} else {
		s.log.Warnw("shikimori image fetch failed", "url", rawURL, "error", err)
	}

	// 3. Try MAL fallback via Jikan
	if malResult := s.tryMALFallback(ctx, rawURL, cacheKey); malResult != nil {
		return malResult, nil
	}

	// 4. All failed → store and serve placeholder
	placeholder := s.placeholderResult()
	s.storeInCache(ctx, placeholderKey, placeholder.Data, placeholder.ContentType)
	return placeholder, nil
}

func (s *ImageProxyService) tryMALFallback(ctx context.Context, originalURL, cacheKey string) *ImageResult {
	animeID := s.extractAnimeID(originalURL)
	if animeID == "" {
		return nil
	}

	// Call Jikan API to get MAL poster URL
	jikanURL := fmt.Sprintf("https://api.jikan.moe/v4/anime/%s", animeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jikanURL, nil)
	if err != nil {
		return nil
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.Warnw("jikan API call failed", "anime_id", animeID, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.log.Warnw("jikan API returned non-200", "anime_id", animeID, "status", resp.StatusCode)
		return nil
	}

	// Parse Jikan response to extract image URL
	// Read body with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 64KB max for JSON
	if err != nil {
		return nil
	}

	malPosterURL := extractJikanImageURL(body)
	if malPosterURL == "" {
		s.log.Warnw("no image URL in jikan response", "anime_id", animeID)
		return nil
	}

	// Fetch the MAL poster
	data, contentType, err := s.fetchURL(ctx, malPosterURL)
	if err != nil {
		s.log.Warnw("MAL image fetch failed", "url", malPosterURL, "error", err)
		return nil
	}

	result := &ImageResult{Data: data, ContentType: contentType, Source: SourceMAL}
	s.storeInCache(ctx, cacheKey, data, contentType)
	return result
}

func (s *ImageProxyService) fromCache(ctx context.Context, key string) (*ImageResult, error) {
	obj, err := s.storage.Client().GetObject(ctx, s.storage.BucketName(), key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(obj, maxImageSize))
	if err != nil {
		return nil, err
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return &ImageResult{Data: data, ContentType: contentType, Source: SourceCache}, nil
}

func (s *ImageProxyService) storeInCache(ctx context.Context, key string, data []byte, contentType string) {
	if contentType == "" {
		contentType = "image/jpeg"
	}
	reader := bytes.NewReader(data)
	_, err := s.storage.Client().PutObject(ctx, s.storage.BucketName(), key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		s.log.Errorw("failed to cache image in minio", "key", key, "error", err)
	}
}

func (s *ImageProxyService) fetchURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("unexpected content-type: %s", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return nil, "", err
	}

	return data, contentType, nil
}

func (s *ImageProxyService) isDomainAllowed(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, d := range allowedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (s *ImageProxyService) hashURL(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%x", h)
}

func (s *ImageProxyService) extractAnimeID(posterURL string) string {
	matches := shikimoriAnimeIDRe.FindStringSubmatch(posterURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func (s *ImageProxyService) placeholderResult() *ImageResult {
	// 1x1 transparent PNG as minimal placeholder
	// In production, replace with a proper branded placeholder
	return &ImageResult{
		Data:        placeholderPNG,
		ContentType: "image/png",
		Source:      SourcePlaceholder,
	}
}

// Minimal 1x1 grey PNG placeholder (67 bytes)
var placeholderPNG = func() []byte {
	// This is a valid 1x1 pixel grey PNG
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
		0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0x60, 0x60, 0x60, 0x00,
		0x00, 0x00, 0x04, 0x00, 0x01, 0x27, 0x34, 0x27, 0x0a, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}()

// extractJikanImageURL extracts the large image URL from a Jikan API response.
// Uses simple byte scanning to avoid importing encoding/json for a small extraction.
func extractJikanImageURL(body []byte) string {
	s := string(body)
	// Look for "large_image_url":"..."
	key := `"large_image_url":"`
	idx := strings.Index(s, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return ""
	}
	u := s[start : start+end]
	// Unescape JSON string (only \/ is common in URLs)
	u = strings.ReplaceAll(u, `\/`, `/`)
	return u
}

```

- [ ] **Step 4: Verify singleflight is available**

`golang.org/x/sync` is already a direct dependency in streaming's `go.mod`. No `go get` needed — just import `golang.org/x/sync/singleflight`.

- [ ] **Step 5: Verify build**

Run: `cd services/streaming && go build ./...`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add libs/videoutils/storage.go services/streaming/internal/service/image_proxy.go
git commit -m "feat(streaming): add image proxy service with MinIO cache and MAL fallback"
```

---

### Task 6: Create Image Proxy Handler + Prometheus Metrics

**Files:**
- Create: `services/streaming/internal/handler/image_proxy.go`

- [ ] **Step 1: Create image proxy handler**

Create `services/streaming/internal/handler/image_proxy.go`:

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	imageProxyRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "image_proxy_requests_total",
		Help: "Total image proxy requests by source",
	}, []string{"source"})

	imageProxyUpstreamDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "image_proxy_upstream_duration_seconds",
		Help:    "Upstream image fetch latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"upstream"})

	imageProxyUpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "image_proxy_upstream_errors_total",
		Help: "Upstream image fetch errors by reason",
	}, []string{"upstream", "reason"})

	imageProxyCacheWriteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "image_proxy_cache_write_errors_total",
		Help: "Cache write failures to MinIO",
	})
)

type ImageProxyHandler struct {
	imageProxyService *service.ImageProxyService
	log               *logger.Logger
}

func NewImageProxyHandler(imageProxyService *service.ImageProxyService, log *logger.Logger) *ImageProxyHandler {
	return &ImageProxyHandler{
		imageProxyService: imageProxyService,
		log:               log,
	}
}

// ProxyImage handles GET /api/v1/image-proxy?url=<encoded_url>
func (h *ImageProxyHandler) ProxyImage(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		httputil.BadRequest(w, "url parameter is required")
		return
	}

	result, err := h.imageProxyService.GetImage(r.Context(), rawURL)
	if err != nil {
		if err.Error() == "domain not allowed" {
			httputil.BadRequest(w, "domain not allowed")
			return
		}
		h.log.Errorw("image proxy error", "url", rawURL, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Record metrics
	imageProxyRequestsTotal.WithLabelValues(string(result.Source)).Inc()

	// Set response headers
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=604800") // 7 days
	w.Header().Set("X-Image-Source", string(result.Source))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(result.Data)
}
```

- [ ] **Step 2: Verify build**

Run: `cd services/streaming && go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add services/streaming/internal/handler/image_proxy.go
git commit -m "feat(streaming): add image proxy handler with Prometheus metrics"
```

---

### Task 7: Wire Up Routes and DI

**Files:**
- Modify: `services/streaming/internal/transport/router.go`
- Modify: `services/streaming/cmd/streaming-api/main.go`
- Modify: `services/gateway/internal/transport/router.go`

- [ ] **Step 1: Add image proxy route to streaming router**

In `services/streaming/internal/transport/router.go`, update the `NewRouter` function signature to accept `imageProxyHandler`:

Change the signature (line 16-22) to:

```go
func NewRouter(
	streamHandler *handler.StreamHandler,
	uploadHandler *handler.UploadHandler,
	imageProxyHandler *handler.ImageProxyHandler,
	cfg *config.Config,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
```

Add the image proxy route inside the `/api/v1` block (after line 47, the `proxy-status` route):

```go
		// Image proxy (public, no auth)
		r.Get("/image-proxy", imageProxyHandler.ProxyImage)
```

- [ ] **Step 2: Wire up image proxy in main.go**

In `services/streaming/cmd/streaming-api/main.go`, add after the upload handler initialization (line 58):

```go
	// Initialize image proxy
	imageProxyService := service.NewImageProxyService(storage, log)
	imageProxyHandler := handler.NewImageProxyHandler(imageProxyService, log)
```

Update the `NewRouter` call (line 64) to include the new handler:

```go
	router := transport.NewRouter(streamHandler, uploadHandler, imageProxyHandler, cfg, log, metricsCollector)
```

- [ ] **Step 3: Add gateway route for image-proxy**

In `services/gateway/internal/transport/router.go`, inside the `/streaming` route group (after line 176, the `proxy-status` route), add:

```go
			r.Get("/image-proxy", proxyHandler.ProxyToStreaming)
```

- [ ] **Step 4: Verify builds**

Run:
```bash
cd services/streaming && go build ./... && cd ../gateway && go build ./...
```
Expected: Both build successfully

- [ ] **Step 5: Commit**

```bash
git add services/streaming/internal/transport/router.go services/streaming/cmd/streaming-api/main.go services/gateway/internal/transport/router.go
git commit -m "feat: wire up image proxy route in streaming service and gateway"
```

---

## Chunk 3: Frontend Changes

### File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `frontend/web/src/composables/useImageProxy.ts` | Per-image fallback logic, adaptive session switch |
| Modify | `frontend/web/src/stores/watchlist.ts` | Split into `fetchStatuses()` and paginated fetch |
| Modify | `frontend/web/src/api/client.ts` | Add paginated watchlist + statuses API methods |
| Modify | `frontend/web/src/views/Profile.vue` | Server-side pagination, use image proxy |
| Modify | `frontend/web/src/components/anime/AnimeCardNew.vue` | Use image proxy composable |
| Modify | `frontend/web/src/components/anime/AnimeCard.vue` | Use image proxy composable |
| Modify | `frontend/web/src/views/Anime.vue` | Use image proxy composable |
| Modify | `frontend/web/src/composables/useAnime.ts` | Use image proxy for coverImage mapping |

---

### Task 8: Create useImageProxy Composable

**Files:**
- Create: `frontend/web/src/composables/useImageProxy.ts`

- [ ] **Step 1: Create the composable**

Create `frontend/web/src/composables/useImageProxy.ts`:

```typescript
import { ref } from 'vue'

const STORAGE_KEY_BLOCKED = 'shikimori_blocked'
const STORAGE_KEY_FAILURES = 'shikimori_failures'
const FAILURE_THRESHOLD = 3

// Shikimori domains that may be blocked regionally
const SHIKIMORI_DOMAINS = ['shiki.one', 'shikimori.io', 'shikimori.one']

function isShikimoriUrl(url: string): boolean {
  try {
    const hostname = new URL(url).hostname.toLowerCase()
    return SHIKIMORI_DOMAINS.some(d => hostname === d || hostname.endsWith('.' + d))
  } catch {
    return false
  }
}

function isBlocked(): boolean {
  return sessionStorage.getItem(STORAGE_KEY_BLOCKED) === 'true'
}

function incrementFailures(): void {
  const count = parseInt(sessionStorage.getItem(STORAGE_KEY_FAILURES) || '0', 10) + 1
  sessionStorage.setItem(STORAGE_KEY_FAILURES, String(count))
  if (count >= FAILURE_THRESHOLD) {
    sessionStorage.setItem(STORAGE_KEY_BLOCKED, 'true')
  }
}

function proxyUrl(originalUrl: string): string {
  return `/api/streaming/image-proxy?url=${encodeURIComponent(originalUrl)}`
}

/**
 * Returns the appropriate image URL for a poster.
 * If Shikimori is detected as blocked in this session, returns proxy URL directly.
 * Otherwise returns the original URL (direct CDN).
 */
export function getImageUrl(originalUrl: string | undefined | null): string {
  if (!originalUrl) return ''
  if (!isShikimoriUrl(originalUrl)) return originalUrl
  if (isBlocked()) return proxyUrl(originalUrl)
  return originalUrl
}

/**
 * Call this from <img @error="onImageError(url)">
 * Swaps to proxy URL and tracks failures for adaptive switch.
 * Returns the fallback URL to set as img.src.
 */
export function getImageFallbackUrl(originalUrl: string): string {
  if (isShikimoriUrl(originalUrl)) {
    incrementFailures()
  }
  return proxyUrl(originalUrl)
}

/**
 * Vue composable for image proxy with reactive per-element fallback.
 * Usage:
 *   const { imageSrc, onError } = useImageProxy(props.posterUrl)
 *   <img :src="imageSrc" @error="onError" />
 */
export function useImageProxy(originalUrl: string | undefined | null) {
  const src = ref(getImageUrl(originalUrl))
  const hasFallback = ref(false)

  function onError() {
    if (hasFallback.value || !originalUrl) return
    hasFallback.value = true
    src.value = getImageFallbackUrl(originalUrl)
  }

  return { imageSrc: src, onError }
}
```

- [ ] **Step 2: Verify no TypeScript errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No new errors related to the composable

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/composables/useImageProxy.ts
git commit -m "feat(frontend): add useImageProxy composable with adaptive fallback"
```

---

### Task 9: Update API Client and Watchlist Store

**Files:**
- Modify: `frontend/web/src/api/client.ts`
- Modify: `frontend/web/src/stores/watchlist.ts`

- [ ] **Step 1: Audit all getWatchlist callers**

Before changing the API signature, grep for all callers:

Run: `cd frontend/web && grep -rn 'getWatchlist\|userApi.getWatchlist' src/`

Verify that all callers either pass no arguments or pass an object. If any caller passes a bare string like `getWatchlist('watching')`, update it to `getWatchlist({ status: 'watching' })`.

- [ ] **Step 2: Add API methods for paginated watchlist and statuses**

In `frontend/web/src/api/client.ts`, replace the existing `getWatchlist` method (line 177) and add new methods:

```typescript
  getWatchlist: (params?: { status?: string; page?: number; per_page?: number; sort?: string; order?: string }) =>
    apiClient.get('/users/watchlist', { params }),
  getWatchlistStatuses: () => apiClient.get('/users/watchlist/statuses'),
```

- [ ] **Step 2: Rewrite watchlist store**

Replace the entire `frontend/web/src/stores/watchlist.ts`:

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { userApi } from '@/api/client'

export const useWatchlistStore = defineStore('watchlist', () => {
  // Lightweight status map (for badges across the site)
  const statusEntries = ref<Array<{ anime_id: string; status: string }>>([])
  const statusLastFetched = ref<number>(0)
  const statusLoading = ref(false)
  const STATUS_CACHE_TTL = 2 * 60 * 1000 // 2 minutes

  const watchlistMap = computed(() => {
    const map = new Map<string, string>()
    for (const entry of statusEntries.value) {
      if (entry.anime_id && entry.status) {
        map.set(entry.anime_id, entry.status)
      }
    }
    return map
  })

  const isStatusFresh = () => Date.now() - statusLastFetched.value < STATUS_CACHE_TTL

  // Fetch lightweight statuses for the badge map
  const fetchStatuses = async (force = false) => {
    if (!force && isStatusFresh() && statusEntries.value.length > 0) return
    if (statusLoading.value) return

    statusLoading.value = true
    try {
      const response = await userApi.getWatchlistStatuses()
      statusEntries.value = response.data?.data || response.data || []
      statusLastFetched.value = Date.now()
    } catch {
      // Silently fail — status map is non-critical
    } finally {
      statusLoading.value = false
    }
  }

  // Backward compat: fetchWatchlist now fetches statuses
  const fetchWatchlist = async (force = false) => {
    return fetchStatuses(force)
  }

  const getStatus = (animeId: string): string | null => {
    return watchlistMap.value.get(animeId) || null
  }

  const getEntry = (animeId: string) => {
    return statusEntries.value.find(e => e.anime_id === animeId) || null
  }

  const invalidate = () => {
    statusLastFetched.value = 0
  }

  const clear = () => {
    statusEntries.value = []
    statusLastFetched.value = 0
  }

  // Expose entries as alias for backward compat
  const entries = statusEntries

  return {
    entries,
    loading: statusLoading,
    watchlistMap,
    fetchWatchlist,
    fetchStatuses,
    getStatus,
    getEntry,
    invalidate,
    clear,
  }
})
```

- [ ] **Step 3: Verify no TypeScript errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No new errors

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/api/client.ts frontend/web/src/stores/watchlist.ts
git commit -m "feat(frontend): add paginated watchlist API and statuses-based store"
```

---

### Task 10: Integrate Image Proxy into Components

**Files:**
- Modify: `frontend/web/src/composables/useAnime.ts`
- Modify: `frontend/web/src/components/anime/AnimeCard.vue`
- Modify: `frontend/web/src/components/anime/AnimeCardNew.vue`
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Update useAnime.ts coverImage mapping**

In `frontend/web/src/composables/useAnime.ts`, add import at the top:

```typescript
import { getImageUrl } from '@/composables/useImageProxy'
```

Change line 68 from:

```typescript
    coverImage: apiAnime.poster_url || '',
```

to:

```typescript
    coverImage: getImageUrl(apiAnime.poster_url),
```

- [ ] **Step 2: Add @error handler to AnimeCardNew.vue**

In `frontend/web/src/components/anime/AnimeCardNew.vue`, add import in the `<script setup>`:

```typescript
import { getImageFallbackUrl } from '@/composables/useImageProxy'
```

Update the `<img>` tag (around line 12-15) to add the error handler:

```html
        <img
          :src="anime.coverImage"
          :alt="localizedTitle"
          class="absolute inset-0 w-full h-full object-cover transition-[opacity,transform] duration-300 group-hover:scale-110"
          loading="lazy"
          @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.coverImage) } }"
        />
```

- [ ] **Step 3: Add @error handler to AnimeCard.vue**

In `frontend/web/src/components/anime/AnimeCard.vue`, add import:

```typescript
import { getImageFallbackUrl } from '@/composables/useImageProxy'
```

Update the `<img>` tag (around line 4):

```html
      <img :src="anime.coverImage" :alt="anime.title" loading="lazy" decoding="async"
        @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.coverImage) } }"
      />
```

- [ ] **Step 4: Add @error handler to Anime.vue poster images**

In `frontend/web/src/views/Anime.vue`, add import:

```typescript
import { getImageFallbackUrl } from '@/composables/useImageProxy'
```

Update poster `<img>` tags (around lines 21-23 and line 8 background-image) to add `@error` handlers where `<img>` is used.

For the `<img>` at line 21-23:
```html
            <img
              :src="anime.coverImage"
              :alt="anime.title"
              class="w-full h-full object-cover"
              @error="(e: Event) => { const img = e.target as HTMLImageElement; if (!img.dataset.fallback) { img.dataset.fallback = '1'; img.src = getImageFallbackUrl(anime.coverImage) } }"
            />
```

For the background-image `<div>` at line 8, use `getImageUrl`:

```typescript
import { getImageUrl, getImageFallbackUrl } from '@/composables/useImageProxy'
```

(Background images don't fire `onerror`, so use `getImageUrl` which applies the session-level proxy switch for the background-image URL.)

- [ ] **Step 5: Verify no TypeScript errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No new errors

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/composables/useAnime.ts frontend/web/src/components/anime/AnimeCard.vue frontend/web/src/components/anime/AnimeCardNew.vue frontend/web/src/views/Anime.vue
git commit -m "feat(frontend): integrate image proxy fallback into all poster components"
```

---

### Task 11: Add Server-Side Pagination to Profile.vue

**Files:**
- Modify: `frontend/web/src/views/Profile.vue`

- [ ] **Step 1: Add PaginationBar import**

In Profile.vue's imports section (around line 800), add:

```typescript
import { PaginationBar } from '@/components/ui'
import { getImageUrl, getImageFallbackUrl } from '@/composables/useImageProxy'
```

- [ ] **Step 2: Add pagination state variables**

After the `watchlistFilter` ref (around line 916), add:

```typescript
const watchlistPage = ref(1)
const watchlistTotalPages = ref(0)
const watchlistTotalCount = ref(0)
const watchlistPerPage = 24
const watchlistSort = ref('updated_at')
const watchlistOrder = ref('desc')
const watchlistPageLoading = ref(false)
```

- [ ] **Step 3: Add paginated fetch function**

Add a new function after the existing watchlist fetch logic (around line 1205):

```typescript
const fetchWatchlistPage = async () => {
  watchlistPageLoading.value = true
  try {
    if (isOwnProfile.value) {
      const status = watchlistFilter.value === 'all' ? undefined : watchlistFilter.value
      const response = await userApi.getWatchlist({
        status,
        page: watchlistPage.value,
        per_page: watchlistPerPage,
        sort: watchlistSort.value,
        order: watchlistOrder.value,
      })
      const data = response.data?.data || response.data || []
      const meta = response.data?.meta
      watchlist.value = data.map((entry: any) => ({
        ...entry,
        anime: entry.anime || {},
      }))
      if (meta) {
        watchlistTotalPages.value = meta.total_pages || 0
        watchlistTotalCount.value = meta.total_count || 0
      }
    } else {
      const statusParam = watchlistFilter.value === 'all' ? undefined : watchlistFilter.value
      const response = await publicApi.getPublicWatchlist(userId.value, {
        status: statusParam,
        page: watchlistPage.value,
        per_page: watchlistPerPage,
        sort: watchlistSort.value,
        order: watchlistOrder.value,
      })
      const data = response.data?.data || response.data || []
      const meta = response.data?.meta
      watchlist.value = data
      if (meta) {
        watchlistTotalPages.value = meta.total_pages || 0
        watchlistTotalCount.value = meta.total_count || 0
      }
    }
  } catch (err) {
    console.error('Failed to fetch watchlist page:', err)
  } finally {
    watchlistPageLoading.value = false
  }
}
```

- [ ] **Step 4: Add watchers for filter/sort/page changes**

Add watchers that trigger `fetchWatchlistPage`:

```typescript
watch(watchlistFilter, () => {
  watchlistPage.value = 1
  fetchWatchlistPage()
})

watch(watchlistPage, () => {
  fetchWatchlistPage()
})
```

- [ ] **Step 5: Update initial watchlist load to use pagination**

Replace the existing watchlist loading logic inside the profile data fetch (around lines 1177-1204) to call `fetchWatchlistPage()` instead of loading the full list. Keep `watchlistStore.fetchStatuses(true)` for the badge map.

The own-profile path becomes:
```typescript
      // Fetch own watchlist — lightweight statuses for badges
      await watchlistStore.fetchStatuses(true)
      // Fetch paginated display list
      await fetchWatchlistPage()
```

The public-profile path becomes:
```typescript
      // Fetch paginated public watchlist
      await fetchWatchlistPage()
```

- [ ] **Step 6: Add PaginationBar to template**

In the template, after the watchlist grid/table and before the closing div of the watchlist section (around line 447), add:

```html
            <PaginationBar
              :current-page="watchlistPage"
              :total-pages="watchlistTotalPages"
              @update:current-page="(p: number) => watchlistPage = p"
            />
```

- [ ] **Step 7: Update poster image references to use proxy**

In Profile.vue, update `animeCover` function (around line 865):

```typescript
const animeCover = (entry: WatchlistEntry): string =>
  getImageUrl(entry.anime?.poster_url) || ''
```

Add `@error` handlers to any `<img>` tags rendering watchlist posters in the template.

- [ ] **Step 8: Add public API method with pagination support**

In `frontend/web/src/api/client.ts`, find or add the `publicApi` object and add/update the `getPublicWatchlist` method to accept pagination params:

```typescript
  getPublicWatchlist: (userId: string, params?: { status?: string; statuses?: string; page?: number; per_page?: number; sort?: string; order?: string }) =>
    apiClient.get(`/users/${userId}/watchlist/public`, { params }),
```

- [ ] **Step 9: Verify frontend builds**

Run: `cd frontend/web && bun run build`
Expected: Build succeeds

- [ ] **Step 10: Commit**

```bash
git add frontend/web/src/views/Profile.vue frontend/web/src/api/client.ts
git commit -m "feat(frontend): add server-side pagination to Profile watchlist"
```

---

## Chunk 4: Grafana Dashboard and Deployment

### Task 12: Add Grafana Dashboard JSON

**Files:**
- Create: `deploy/kustomize/grafana/dashboards/image-proxy.json`

- [ ] **Step 1: Create Grafana dashboard**

Create `deploy/kustomize/grafana/dashboards/image-proxy.json` with a standard Grafana dashboard containing 7 panels:

1. **Cache Hit Rate (%)** — `rate(image_proxy_requests_total{source="cache_hit"}[5m]) / rate(image_proxy_requests_total[5m]) * 100`
2. **Fallback Chain Breakdown** — stacked time series by `source` label
3. **Upstream Error Rate** — `rate(image_proxy_upstream_errors_total[5m])` by `upstream` and `reason`
4. **Upstream Fetch Latency** — p50/p95 from `image_proxy_upstream_duration_seconds`
5. **Cache Growth** — `image_proxy_cache_size_bytes`
6. **Proxied Image %** — ratio of proxy requests to estimated total
7. **Proxy Session %** — unique IPs on image proxy vs total gateway IPs

(The actual dashboard JSON is large — generate it using the Grafana provisioning format. Key: set `datasource` to `Prometheus`, `refresh` to `30s`, and `uid` to `image-proxy`.)

- [ ] **Step 2: Commit**

```bash
git add deploy/kustomize/grafana/dashboards/image-proxy.json
git commit -m "feat(grafana): add Image Proxy monitoring dashboard"
```

---

### Task 13: Deploy and Verify

- [ ] **Step 1: Redeploy player service**

Run: `make redeploy-player`
Wait for health check to pass.

- [ ] **Step 2: Redeploy streaming service**

Run: `make redeploy-streaming`
Wait for health check to pass.

- [ ] **Step 3: Redeploy gateway**

Run: `make redeploy-gateway`
Wait for health check to pass.

- [ ] **Step 4: Redeploy frontend**

Run: `make redeploy-web`
Wait for health check to pass.

- [ ] **Step 5: Verify paginated watchlist API**

```bash
# Test authenticated endpoint (replace TOKEN with valid JWT)
curl -s http://localhost:8000/api/users/watchlist?page=1&per_page=5 \
  -H "Authorization: Bearer TOKEN" | jq '.meta'
# Expected: {"page":1,"page_size":5,"total_count":N,"total_pages":M}

# Test statuses endpoint
curl -s http://localhost:8000/api/users/watchlist/statuses \
  -H "Authorization: Bearer TOKEN" | jq '.data | length'
# Expected: total count of user's watchlist
```

- [ ] **Step 6: Verify image proxy**

```bash
# Test with a known Shikimori poster URL from the database
POSTER_URL=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -t -c "SELECT poster_url FROM animes WHERE poster_url IS NOT NULL LIMIT 1;" | tr -d ' ')
curl -sI "http://localhost:8000/api/streaming/image-proxy?url=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$POSTER_URL', safe=''))")"
# Expected: HTTP 200 with X-Image-Source: shikimori (first time) or cache_hit (subsequent)

# Test SSRF prevention
curl -sI "http://localhost:8000/api/streaming/image-proxy?url=https://evil.com/image.jpg"
# Expected: HTTP 400 "domain not allowed"
```

- [ ] **Step 7: Verify health of all services**

Run: `make health`
Expected: All services healthy

- [ ] **Step 8: Commit any remaining changes**

```bash
git add -A
git commit -m "chore: final deployment verification"
```
