# Catalog Pagination Design

**Date:** 2026-03-09
**Status:** Approved
**Scope:** Frontend-only (backend already supports page-based pagination)

## Problem

Users browse the catalog, click into an anime, press back, and lose their position because "Load More" accumulates results in memory that get reset on navigation. Users want to browse by visual impression ("looks interesting") and maintain their place.

## Solution

Replace "Load More" with numbered page navigation. Store `page` in URL query params so browser back button preserves position.

## Design

### Pagination Bar
```
[ < ] [ 1 ] [ 2 ] [*3*] [ 4 ] [ 5 ] [ ... ] [ 12 ] [ > ]
```
- Max ~7 visible page buttons with ellipsis
- First/last page always visible
- Current page highlighted
- Prev/next disabled at boundaries
- 20 items per page

### URL Sync
- Page stored in URL: `/browse?page=3&genre=action&status=ongoing`
- On mount, read `page` from `route.query` (default 1)
- On page change, update `route.query.page` via `router.replace()`
- Filters already use URL query params — no change needed

### Navigation Flow
1. User browses page 3 → URL: `/browse?page=3`
2. Clicks anime → navigates to `/anime/123`
3. Presses back → returns to `/browse?page=3`
4. Page 3 loads with same filters

### Changes
- **Browse.vue**: Replace "Load More" button with pagination component, read/write `page` query param, replace accumulated results with per-page results, scroll to top on page change
- **No backend changes**: API already returns `{ meta: { page, page_size, total_count, total_pages } }`

### What Stays the Same
- All filters (genre, year, status, sort)
- Live search dropdown
- "Search on Shikimori" button
- 20 items per page default
- Backend API
