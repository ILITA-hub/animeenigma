# Hanime Player — Design Spec

**Date:** 2026-04-05
**Issue:** AUTO-030
**Status:** Approved

## Overview

Add Hanime.tv as a 5th video provider for adult (hentai) anime content. Hanime appears as a new "18+" language tab alongside existing RU/EN tabs on the anime detail page.

## Problem

Hentai anime is already available in the catalog via Shikimori, but none of the existing 4 players (Kodik, AnimeLib, HiAnime, Consumet) serve hentai video content. Users see hentai titles but have no way to watch them.

## Solution

Integrate Hanime.tv's undocumented API as a new backend parser + frontend player.

## Architecture

### Backend Parser: `services/catalog/internal/parser/hanime/`

**client.go** — Go HTTP client for Hanime.tv API.

#### Authentication

Hanime requires authentication for stream URLs. Without auth, the API returns dummy `streamable.cloud` placeholder URLs.

**Login endpoint:** `POST https://www.universal-cdn.com/rapi/v4/sessions`

Request body uses obfuscated field names:
```json
{"burger": "<email>", "fries": "<password>"}
```

Required headers for all authenticated requests:
```
x-claim: <unix_timestamp_seconds>
x-signature-version: app2
x-signature: SHA256("994482" + "2" + t + "8" + t + "113")
x-session-token: <session_token>
```

Response returns `session_token` + `session_token_expire_time_unix`. Token stored in memory, auto-refreshed before expiry.

#### Search / Title Matching

**Search endpoint (no auth):** `POST https://search.htv-services.com/`

```json
{
  "search_text": "<anime name from Shikimori>",
  "tags": [],
  "blacklist": [],
  "brands": [],
  "order_by": "title_sortable",
  "ordering": "asc",
  "page": 0,
  "tags_mode": "AND"
}
```

Returns `hits` array with `name`, `slug`, `brand` fields. Match by comparing Shikimori anime name (English/Japanese) against hit names. Use the first matching franchise.

#### Episode Listing

From the video endpoint response, `hentai_franchise_hentai_videos` contains ordered episode list with `name` and `slug` for each episode.

#### Stream URLs

**Authenticated video endpoint:** `GET https://www.universal-cdn.com/rapi/v4/hentai-videos/<slug>`

Response `videos_manifest.servers[].streams[]` contains:
- `url` — real MP4/HLS URL
- `height` — resolution (360, 480, 720, 1080)
- `width` — pixel width
- `filesize_mbs` — file size
- `is_guest_allowed` / `is_member_allowed` / `is_premium_allowed` — access tier flags
- `extension` — "mp4" or "m3u8"

Filter out streams with empty URLs. 1080p requires premium account.

### Environment Variables

```
HANIME_EMAIL=<hanime.tv account email>
HANIME_PASSWORD=<hanime.tv account password>
```

Added to `services/catalog/internal/config/` and `docker/.env`.

### Service Integration

**CatalogService** gets a new `hanimeClient *hanime.Client` field, initialized from config. If HANIME_EMAIL is empty, client is nil (feature disabled).

### API Routes

New handler methods in catalog service:

| Route | Method | Description |
|-------|--------|-------------|
| `/api/anime/:id/hanime/episodes` | GET | List Hanime episodes for a Shikimori anime |
| `/api/anime/:id/hanime/watch/:slug` | GET | Get stream URLs for a specific episode |

Gateway routing: `/api/anime/*` already routes to catalog:8081.

### Backend Proxy

Stream MP4 URLs proxied through `libs/videoutils/proxy.go`. Add Hanime CDN domains to the allowed list (domains discovered at runtime from API responses).

### Caching

- Hanime search results: cache 1 hour (keyed by anime name)
- Episode lists: cache 1 hour (keyed by franchise slug)
- Stream URLs: cache 30 minutes (URLs may expire)
- Session token: cache until expiry time from API response

### Frontend

#### Language Tab

New "18+" tab in `Anime.vue` alongside RU/EN. Contains single "Hanime" provider button. Only shown when Hanime is configured (check via a new API endpoint or feature flag).

#### HanimePlayer.vue

New async component loaded when user selects Hanime provider.

Features:
- HTML5 `<video>` element for MP4 playback (like AnimeLibPlayer)
- Quality selector dropdown (available resolutions from API)
- Episode list from franchise
- Watch progress tracking (reuse existing player store)
- ReportButton integration

Does NOT need:
- HLS.js (streams are MP4)
- SubtitleOverlay (Hanime doesn't provide subtitle tracks)
- Translation selection (Hanime has one version per episode)

#### API Client

New methods in `frontend/web/src/api/client.ts`:

```typescript
export const hanimeApi = {
  getEpisodes: (animeId: string) => apiClient.get(`/anime/${animeId}/hanime/episodes`),
  getWatch: (animeId: string, slug: string) => apiClient.get(`/anime/${animeId}/hanime/watch/${slug}`),
}
```

## Error Handling

- If Hanime credentials not configured: 18+ tab hidden, no errors
- If Hanime login fails: log error, return 503 to frontend, show "Provider unavailable"
- If search finds no match: return empty episodes, frontend shows "No episodes found"
- If stream URLs expired: re-fetch from API (bypass cache)

## Security

- Hanime credentials stored only in server-side env vars, never exposed to frontend
- Session token managed server-side only
- Stream URLs proxied through backend, original CDN URLs not exposed to client
