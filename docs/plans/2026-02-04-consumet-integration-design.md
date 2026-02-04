# Consumet Integration & HiAnime Quick Fix

## Overview

Add Consumet API as a new English anime streaming provider alongside existing Kodik and HiAnime. Also fix HiAnime reliability issues.

## Architecture

```
Frontend tabs: [Kodik] [HiAnime] [Consumet]
                  ↓         ↓          ↓
Backend:      kodik/    hianime/   consumet/
              parser     parser      parser
                           ↓            ↓
                    aniwatch:4000  consumet:3101
```

## Changes

### 1. Docker - Add Consumet Container
- Image: `riimuru/consumet-api`
- Port: 3101:3000
- Health check: `/health`

### 2. Backend - Consumet Parser
New file: `services/catalog/internal/parser/consumet/client.go`
- Search anime by title
- Get episodes for anime
- Get available servers
- Get stream URL (HLS)

### 3. Backend - Handler Endpoints
Add to `services/catalog/internal/handler/catalog.go`:
- `GET /api/anime/{id}/consumet/episodes`
- `GET /api/anime/{id}/consumet/servers?episode={epId}`
- `GET /api/anime/{id}/consumet/stream?episode={epId}&server={srv}`

### 4. Frontend - ConsumetPlayer Component
New file: `frontend/web/src/components/player/ConsumetPlayer.vue`
- Similar structure to HiAnimePlayer.vue
- Episode selection, server selection, HLS playback

### 5. Frontend - Add Tab
Update `frontend/web/src/views/Anime.vue`:
- Add "Consumet" tab button
- Import and render ConsumetPlayer

### 6. HiAnime Quick Fix
Update `services/catalog/internal/parser/hianime/client.go`:
- Default server order: hd-2 first (hd-1 is blocked)
- Add retry logic with exponential backoff
- Add 1-2 second delay between requests to avoid rate limiting

### 7. E2E Tests
Update `frontend/web/e2e/`:
- Add consumet integration tests
- Update hianime tests with server preference
- Add comprehensive statistics gathering

## Consumet API Endpoints

Base: `http://consumet:3000`

```
GET /anime/zoro/{query}           - Search
GET /anime/zoro/info?id={id}      - Anime info + episodes
GET /anime/zoro/watch?episodeId={id}&server={srv} - Stream URL
```

Note: Consumet uses "zoro" as the provider name for HiAnime source.
