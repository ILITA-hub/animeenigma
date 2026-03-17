# JSON List Export

**Date:** 2026-03-17
**Status:** Approved

## Overview

Add a "Export to JSON" option in Profile > Settings that downloads the user's full watchlist as a JSON file. Backend endpoint returns the file directly; frontend triggers the download.

## Backend

### New endpoint

`GET /api/users/export/json` (protected, JWT required)

- Lives in `services/player/internal/handler/export.go`
- Uses existing `ListRepository.GetByUser()` to fetch all entries with anime+genres preloaded
- Returns JSON with `Content-Disposition: attachment` header
- Per-user rate limit: 1 request per minute (in-memory `sync.Map` tracking last export timestamp per userID)

### Response format

```json
{
  "exported_at": "2026-03-17T12:00:00Z",
  "user": "username",
  "total_entries": 42,
  "entries": [
    {
      "animeenigma_id": "uuid-string",
      "mal_id": 123,
      "shikimori_id": 123,
      "title": "Attack on Titan",
      "title_ru": "Атака титанов",
      "title_jp": "進撃の巨人",
      "poster_url": "/api/streaming/image-proxy?url=...",
      "episodes_total": 25,
      "episodes_aired": 25,
      "genres": ["Action", "Drama"],
      "status": "completed",
      "score": 9,
      "episodes_watched": 25,
      "notes": "Great show",
      "tags": "favorite,rewatch",
      "is_rewatching": false,
      "priority": "",
      "started_at": "2025-01-15T00:00:00Z",
      "completed_at": "2025-03-20T00:00:00Z",
      "created_at": "2025-01-15T10:30:00Z",
      "updated_at": "2025-03-20T18:45:00Z"
    }
  ]
}
```

Notes:
- `mal_id` and `shikimori_id` are the same value (Shikimori IDs = MAL IDs). Both included for clarity.
- `mal_id`/`shikimori_id` will be `null` if not mapped.
- Genres are flattened to string array of English names.

### Rate limiting

Per-user in-memory rate limiter in the handler:
- `sync.Map` of `userID -> lastExportTime`
- If less than 60 seconds since last export, return 429 Too Many Requests
- Cleanup: entries older than 5 minutes removed periodically (goroutine)

### Route registration

In `services/player/internal/transport/router.go`, add under protected routes:
```go
r.Get("/api/users/export/json", exportHandler.ExportJSON)
```

Gateway: already covered by `r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)`.

## Frontend

### Settings tab changes

Add an "Export" card in `Profile.vue` Settings tab, placed between the Import card and Public Profile card.

Contents:
- Title: "Export" (i18n)
- Description text: "Download your anime list as a JSON file" (i18n)
- Button: "Export to JSON" with download icon
- Loading spinner while request is in flight
- Error message display (e.g., rate limit hit)

### API client

Add to `userApi` in `frontend/web/src/api/client.ts`:
```typescript
exportJSON: () => api.get('/users/export/json', { responseType: 'blob' })
```

### Download trigger

On button click:
1. Call `exportJSON()`
2. Create a Blob URL from the response
3. Create a temporary `<a>` element, set `href` and `download` attribute
4. Click it programmatically, then revoke the URL

### i18n

Add keys to both `en.json` and `ru.json`:
- `profile.export.title`
- `profile.export.description`
- `profile.export.button`
- `profile.export.exporting`
- `profile.export.error`
- `profile.export.rateLimited`

## Files changed

| File | Change |
|------|--------|
| `services/player/internal/handler/export.go` | New file: ExportHandler with ExportJSON method + per-user rate limiter |
| `services/player/internal/transport/router.go` | Register export route |
| `services/player/cmd/player-api/main.go` | Wire ExportHandler |
| `frontend/web/src/views/Profile.vue` | Add Export card in Settings tab |
| `frontend/web/src/api/client.ts` | Add `exportJSON()` method |
| `frontend/web/src/locales/en.json` | Add export i18n keys |
| `frontend/web/src/locales/ru.json` | Add export i18n keys |
