# Shikimori Image Optimization: Pagination + Proxy Fallback

**Date:** 2026-03-16
**Status:** Draft
**Scope:** Server-side watchlist pagination, backend image proxy with regional fallback, Grafana observability

---

## Problem Statement

Two issues with Shikimori poster images:

1. **Over-requesting:** Image-rich pages (e.g., `/user/:username` profile) load all watchlist anime at once. The heaviest user currently has 957 entries — 530 KB of JSON and 957 simultaneous image requests. Power users with 3,000+ entries would produce 1.6 MB payloads (10x the main JS bundle).

2. **Regional blocking:** In some regions (notably Russia), Shikimori's CDN is blocked by ISPs. Images fail to load with no fallback, leaving broken placeholders across the UI.

### Current Data (2026-03-16)

| Metric | Value |
|--------|-------|
| Users with watchlist | 9 |
| Total watchlist entries | 2,469 |
| Avg entries/user | 274 |
| Max entries/user | 957 |
| P90 entries/user | 798 |
| Heaviest status tab | completed (avg 209, max 868 per user) |
| Estimated JSON per entry | ~567 bytes (list fields + anime + genres) |

### Poster URL Domains in Database

| Domain | Count |
|--------|-------|
| `shiki.one` | 1,668 |
| `shikimori.io` | 1,653 |

Both domains are used roughly 50/50 and must be whitelisted.

---

## Design Overview

Four components, deployable independently:

1. **Server-side watchlist pagination** — caps API payload at ~14 KB/page
2. **Backend image proxy with MinIO cache** — serves cached posters, fallback chain for blocked regions
3. **Smart frontend fallback** — per-image `onerror` with adaptive session-wide proxy switch
4. **Grafana observability** — metrics for cache performance, upstream errors, proxy adoption

---

## 1. Server-Side Watchlist Pagination

### Endpoints Affected

Two existing endpoints need pagination:

1. **Authenticated (own list):** `GET /api/users/watchlist` — user ID from JWT claims
2. **Public (other user):** `GET /api/users/{userId}/watchlist/public` — user ID from URL path

Both currently return unbounded lists.

### New Query Parameters

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `status` | string | (all) | Filter by status. Authenticated endpoint uses `status` (single value, existing). Public endpoint currently uses `statuses` (comma-separated) — migrate to `status` (single value) for consistency. |
| `page` | int | 1 | Page number (1-indexed) |
| `per_page` | int | 24 | Items per page (max 100) |
| `sort` | string | `updated_at` | Sort field (whitelist: `updated_at`, `score`, `status`, `created_at`) |
| `order` | string | `desc` | Sort direction (`asc`/`desc`) |

**Sort field validation:** Only whitelisted fields are accepted. Unknown sort values return 400.

### Response Format

Uses the existing `httputil.JSONWithMeta()` and `httputil.Meta` struct:

```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "anime": { "id": "uuid", "name": "...", "poster_url": "...", ... },
      "status": "completed",
      "score": 8,
      "episodes": 24
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 24,
    "total_count": 868,
    "total_pages": 37
  }
}
```

### Watchlist Status Map

**Problem:** The watchlist store builds a `watchlistMap` (Map of anime_id → status) used by AnimeCard components to show quick status indicators across the site. With pagination, only 24 entries would be in this map, breaking status badges on Browse/search pages.

**Solution:** Add a lightweight authenticated endpoint (JWT-required) that returns only status pairs. No gateway routing change needed — the existing `r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)` wildcard covers it.

`GET /api/users/watchlist/statuses`

```json
{
  "success": true,
  "data": [
    { "anime_id": "uuid-1", "status": "watching" },
    { "anime_id": "uuid-2", "status": "completed" }
  ]
}
```

This returns all entries but only two fields per entry (~50 bytes each). For a 3,000-entry user: ~150 KB uncompressed, ~20 KB gzipped. The watchlist store uses this endpoint for the status map, while Profile.vue uses the paginated endpoint for display.

### Backend Implementation

**Service:** `services/player/`

**Changes:**
- `handler/list.go` — parse pagination query params in both `GetUserList` and `GetPublicWatchlist`, return via `httputil.JSONWithMeta()`. Add new `GetWatchlistStatuses` handler.
- `service/list.go` — accept pagination params, pass to repo. Add `GetStatuses()` method.
- `repo/list.go` — add `.Offset((page-1)*perPage).Limit(perPage)` and a `COUNT(*)` query for total. Add lightweight statuses query.
- `transport/router.go` — add `GET /watchlist/statuses` route

**Page size 24** — divisible by 2, 3, 4, 6 grid columns. Matches existing Browse page pattern.

### Frontend Implementation

**Changes:**
- `stores/watchlist.ts` — split into two fetch paths:
  - `fetchStatuses()` — calls `/watchlist/statuses`, builds the `watchlistMap` for status badges
  - `fetchWatchlistPage(status, page, sort, order)` — calls the paginated endpoint for Profile display
- `views/Profile.vue` — use pagination component (same as Browse page), trigger API call on page change instead of client-side slicing
- Remove the 2-minute full-list cache, replace with per-page cache keyed by `(status, page, sort, order)` and a separate statuses cache

**Filter/sort tabs:** Status filter tabs (`watching`, `completed`, etc.) each reset to page 1 and fetch from API. Sort changes also reset to page 1.

---

## 2. Backend Image Proxy with MinIO Cache

### New Endpoint

**Route:** `GET /api/streaming/image-proxy?url=<encoded_url>`

**Internal streaming route:** `GET /api/v1/image-proxy` (inside the existing `/api/v1` route block in `streaming/internal/transport/router.go`)

**Gateway routing:** Add `/api/streaming/image-proxy` → `streaming:8082` (gateway rewrites to `/api/v1/image-proxy`)

### Fallback Chain

```
Request arrives
    │
    ├─ url param empty?
    │   └─ Return static placeholder image (200, cached)
    │
    ├─ URL domain not in whitelist?
    │   └─ Return 400 (SSRF prevention)
    │
    ├─ Check MinIO key: "posters/{sha256_of_url}"
    │   └─ Cache hit → Serve directly (200, Cache-Control: 7d)
    │
    ├─ Fetch from original URL (Shikimori CDN)
    │   └─ Success → Store in MinIO with Content-Type metadata, serve (200)
    │
    ├─ Shikimori failed → Extract anime ID from URL path
    │   ├─ Shikimori IDs = MAL IDs (no mapping needed)
    │   ├─ Call Jikan API: GET https://api.jikan.moe/v4/anime/{mal_id}
    │   ├─ Extract images.jpg.large_image_url from response
    │   └─ Fetch from MAL CDN URL
    │       └─ Success → Store in MinIO (keyed by original URL), serve (200)
    │
    └─ All failed → Store placeholder in MinIO under "posters/placeholder/{sha256}"
        └─ Separate prefix allows independent flush of failed entries
```

### Implementation Details

**Service:** `services/streaming/`

**New files:**
- `internal/handler/image_proxy.go` — HTTP handler, URL validation, response headers
- `internal/service/image_proxy.go` — fallback chain logic, MinIO read/write, Jikan resolution

**Domain whitelist:** `shiki.one`, `shikimori.io`, `shikimori.one`, `cdn.myanimelist.net`

All three Shikimori domains are needed (DB has `shiki.one` and `shikimori.io`; `shikimori.one` appears in some older entries and proxy config).

**Reused patterns from `libs/videoutils/proxy.go`:**
- Domain validation via `isDomainAllowed()` pattern
- User-Agent spoofing (Mozilla UA)
- SSRF prevention via strict domain validation

**Concurrency:**
- `golang.org/x/sync/singleflight` — dedup concurrent requests for the same uncached URL (new dependency, add to `go.mod`)
- Concurrency semaphore (50 max concurrent upstream fetches, matching HLS proxy pattern) — prevents a burst of cache misses from overwhelming upstream

**MinIO storage:**
- **Existing bucket** `animeenigma` with key prefix `posters/` (e.g., `posters/a1b2c3d4...`)
- Placeholder entries use prefix `posters/placeholder/` for independent flushing
- Content-Type stored as MinIO object metadata during upload, restored on cache hits
- No TTL — posters are permanent. Admin can flush via `mc rm --recursive minio/animeenigma/posters/`

**Response headers:**
```
Cache-Control: public, max-age=604800    (7-day browser cache)
Content-Type: image/jpeg                  (from MinIO object metadata)
X-Image-Source: cache|shikimori|mal|placeholder   (for debugging/metrics)
```

**Security:**
- URL parameter must match whitelisted domains — rejects arbitrary URLs (SSRF prevention)
- Max image size: 5 MB (reject abnormally large responses)
- Timeout: 10s per upstream fetch

### MAL Poster URL Resolution via Jikan API

When Shikimori fails:
1. Parse the Shikimori anime ID from the poster URL path (e.g., `/uploads/poster/animes/33352/...` → ID `33352`)
2. Shikimori IDs = MAL IDs (per project conventions), so no ID mapping needed
3. Call Jikan API: `GET https://api.jikan.moe/v4/anime/{mal_id}`
4. Extract `data.images.jpg.large_image_url` from response (e.g., `https://cdn.myanimelist.net/images/anime/1234/567890l.jpg`)
5. Fetch that URL and cache it

**Jikan rate limit:** Jikan has a 3 req/s rate limit. Since this path only fires on Shikimori cache misses (which decrease over time as the cache fills), this is acceptable. Add `jikan.moe` to a request-level rate limiter as a safety net.

---

## 3. Smart Frontend Fallback

### Per-Image `onerror` with Adaptive Switch

**Composable:** `src/composables/useImageProxy.ts`

The composable exposes a reactive `getImageUrl(originalUrl): string` function and an `onImageError(originalUrl)` handler.

```
For each <img>:
  1. Check sessionStorage flag "shikimori_blocked"
     ├─ true  → return proxy URL immediately
     └─ false → return direct Shikimori URL
  2. On <img> onerror:
     ├─ Swap src to proxy URL
     ├─ Increment failure counter in sessionStorage
     └─ If failures >= 3 → set "shikimori_blocked" = true
        (all subsequent images skip Shikimori, go straight to proxy)
```

**Behavior by scenario:**

| Scenario | UX Impact |
|----------|-----------|
| Shikimori works fine | Zero proxy traffic. Direct CDN speed. |
| Full regional block | First 1-3 images fail fast (ISP reset, <1s), adaptive switch kicks in, rest load via proxy |
| Individual image 404 | Only that image falls back to proxy. No global switch. |
| Intermittent issues | Each image independently falls back as needed |
| Returning user (same session) | `sessionStorage` flag persists, proxy used immediately |

**Component integration:** Replace all direct `poster_url` `<img>` bindings with the composable. Affected components:
- `AnimeCard.vue` / `AnimeCardNew.vue`
- `Profile.vue` (watchlist grid/table)
- `Anime.vue` (detail page poster)
- Any other component displaying anime posters

**Failure counter reset:** `sessionStorage` clears on tab close. Each new session re-probes Shikimori. This handles users who travel between blocked/unblocked regions.

---

## 4. Grafana Observability

### New Prometheus Metrics (Streaming Service)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `image_proxy_requests_total` | Counter | `source`: `cache_hit`, `shikimori`, `mal`, `placeholder` | Where each image was served from |
| `image_proxy_upstream_duration_seconds` | Histogram | `upstream`: `shikimori`, `mal` | Fetch latency for cache misses |
| `image_proxy_upstream_errors_total` | Counter | `upstream`: `shikimori`, `mal`; `reason`: `timeout`, `4xx`, `5xx`, `connection_refused` | Upstream failure breakdown |
| `image_proxy_cache_size_bytes` | Gauge | — | Total MinIO posters prefix size |

### Grafana Dashboard Panels

**Dashboard:** "Image Proxy" (new)

1. **Cache Hit Rate (%)** — `rate(image_proxy_requests_total{source="cache_hit"}) / rate(image_proxy_requests_total)`. Expected to climb toward 95%+ as cache fills organically.

2. **Fallback Chain Breakdown** — Stacked time series of `source` label values. Shows how many images came from cache vs. Shikimori vs. MAL vs. placeholder. If `mal` spikes, Shikimori is having issues.

3. **Upstream Error Rate** — `rate(image_proxy_upstream_errors_total)` by `upstream` and `reason`. Key alert: if `connection_refused` or `timeout` for Shikimori sustains >50% for 5 minutes, Shikimori may be experiencing an outage or block.

4. **Upstream Fetch Latency (p50/p95)** — Histogram quantiles from `image_proxy_upstream_duration_seconds`. Spot slow fetches early.

5. **Cache Growth** — `image_proxy_cache_size_bytes` over time. Sanity check. At ~100 KB/poster, 10,000 anime = ~1 GB.

6. **Proxied Image %** — Approximated: `rate(image_proxy_requests_total) / (rate(image_proxy_requests_total) + estimated_direct_loads)`. Direct loads estimated from page view rate x 24 items/page minus proxy requests. Answers: "what fraction of image loads needed our proxy?"

7. **Proxy Session %** — Approximated from unique client IPs hitting the image proxy vs. total unique IPs hitting the gateway. Derived from existing `http_requests_total` labels, no extra beacon endpoint needed. Answers: "what percentage of users are affected by regional blocking?"

---

## Affected Services and Files

### Backend

| Service | File | Change |
|---------|------|--------|
| `streaming` | `internal/handler/image_proxy.go` | New: proxy endpoint handler |
| `streaming` | `internal/service/image_proxy.go` | New: fallback chain, MinIO caching, singleflight |
| `streaming` | `internal/transport/router.go` | Add `GET /api/v1/image-proxy` inside existing route block |
| `player` | `internal/handler/list.go` | Add pagination params to `GetUserList` and `GetPublicWatchlist`. New `GetWatchlistStatuses` handler. |
| `player` | `internal/service/list.go` | Pass pagination to repo. New `GetStatuses()` method. |
| `player` | `internal/repo/list.go` | Add `.Offset().Limit()` + `COUNT(*)`. New lightweight statuses query. |
| `player` | `internal/transport/router.go` | Add `GET /watchlist/statuses` route |
| `gateway` | `internal/transport/router.go` | Add `/api/streaming/image-proxy` route |

### Frontend

| File | Change |
|------|--------|
| `src/composables/useImageProxy.ts` | New: per-image fallback logic with `getImageUrl()` and `onImageError()` |
| `src/stores/watchlist.ts` | Split into `fetchStatuses()` (lightweight) and `fetchWatchlistPage()` (paginated) |
| `src/views/Profile.vue` | Server-side pagination, use image proxy composable |
| `src/components/AnimeCard.vue` | Use image proxy composable for poster |
| `src/components/AnimeCardNew.vue` | Use image proxy composable for poster |
| `src/views/Anime.vue` | Use image proxy composable for poster |

### Infrastructure

| File | Change |
|------|--------|
| `deploy/kustomize/` | Add Grafana dashboard JSON for Image Proxy |

### New Dependencies

| Service | Package | Purpose |
|---------|---------|---------|
| `streaming` | `golang.org/x/sync/singleflight` | Dedup concurrent upstream fetches |

---

## Non-Goals

- **CDN integration** — self-hosted target, not needed
- **Image resizing/thumbnails** — store original resolution, let browser handle display size
- **Pre-warming the cache** — fills organically as users browse
- **Automatic cache invalidation** — posters don't change. Admin can manually flush MinIO prefix.
- **Separate MinIO bucket** — use existing `animeenigma` bucket with `posters/` prefix
- **Dedicated beacon endpoint** — proxy session % derivable from existing gateway metrics

---

## Open Questions

1. **Placeholder image** — Need to create or choose a static placeholder SVG/PNG for the project.
2. **Other image-heavy pages** — Browse/search results may also benefit from the frontend fallback composable, but those pages already have pagination via the Shikimori API.
3. **Jikan API availability** — If Jikan is also blocked in certain regions, the MAL fallback step would fail. This is acceptable since the placeholder covers this case.
