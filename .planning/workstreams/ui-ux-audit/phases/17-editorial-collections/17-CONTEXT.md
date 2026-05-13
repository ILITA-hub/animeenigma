# Phase 17: Editorial collections (Dragon) - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, Dragon-level new feature — admin-curated collections)

<domain>
## Phase Boundary

Admin-curated `Подборки` (Collections) system, distinct from algorithmic recommendations. Closes UX-33 / Tier E #5.

**Backend:**
- New tables (`services/catalog/`):
  - `collections` — admin-curated playlists/sets. Fields: id, slug (URL-safe), title, title_ru, title_jp, description, description_ru, description_jp, cover_image_url, published bool, created_by user_id, created_at, updated_at, deleted_at.
  - `collection_items` — m2m anime ↔ collection ordering. Fields: id, collection_id, anime_id, sort_order int, created_at.
- New endpoints:
  - `GET /api/collections` — public list of PUBLISHED collections (slugs + titles + cover).
  - `GET /api/collections/:slug` — public detail with full anime list joined.
  - `GET /api/admin/collections` — admin list incl. drafts (JWT + admin role).
  - `POST /api/admin/collections` — create.
  - `PUT /api/admin/collections/:id` — update (title, description, items).
  - `DELETE /api/admin/collections/:id` — soft-delete.
  - `POST /api/admin/collections/:id/items` — add anime to collection.
  - `DELETE /api/admin/collections/:id/items/:animeId` — remove from collection.

**Frontend (admin):**
- New admin view: `/admin/collections` — list of collections with create/edit/delete.
- New admin view: `/admin/collections/:id` — edit form (title, description, cover, items via search-and-add picker).

**Frontend (public):**
- Home.vue: new row "Подборки" rendering top N published collections as cards (link to `/collections/:slug`).
- New view: `/collections/:slug` — public collection detail page showing all anime in the collection.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Pragmatic scope-reduction for v0.1:**
This is a Dragon-level feature. Full implementation = ~30 new files + 2 tables + 7 endpoints + 4 views. To ship cleanly within autonomous run, **constrain scope:**

- **Backend:** ship all 2 tables + 7 endpoints. Tables auto-migrate via GORM.
- **Admin frontend:** ship the minimum-viable list + create/edit form. No drag-and-drop reordering (use a "sort_order" number input field). No image upload to MinIO for cover_image_url — admin pastes a URL.
- **Public frontend:** ship the Home row + detail page. No filter/sort on collection items (use the admin-defined sort_order).

**Data model details:**
- `collections.slug` — URL-safe, unique. Auto-generated from title on create (admin can override).
- `collection_items.sort_order` — INT, lower values render first. Admin sets explicitly.
- `collections.published` — only `true` collections appear on the public surface. Drafts visible to admins only.

**API shapes:**
- `GET /api/collections` returns `{ collections: [{ id, slug, title, title_ru, title_jp, cover_image_url, item_count }] }`. Lightweight — no items inline.
- `GET /api/collections/:slug` returns `{ collection: { ...all_fields, items: [{ anime_id, anime: {...}, sort_order }] } }`. Full anime info joined.
- Admin endpoints return same shape including drafts.

**Auth & rate limiting:**
- Public endpoints: no JWT required.
- Admin endpoints: JWT required + admin role check at gateway (same pattern as `/api/admin/recs/*`).

**Components:**
- `frontend/web/src/views/Collections.vue` — public detail at `/collections/:slug`.
- `frontend/web/src/components/home/CollectionsRow.vue` — Home row.
- `frontend/web/src/views/admin/AdminCollections.vue` — admin list.
- `frontend/web/src/views/admin/AdminCollectionEdit.vue` — admin create/edit form.

**i18n keys:**
- `collections.title` (page header)
- `home.collections` (row label)
- `admin.collections.*` (10+ keys for CRUD UI)
- Total ~20 keys × 3 locales = 60 entries.

### Locked from ROADMAP

- Phase 17 depends on Phase 8 (Home row pattern) + Phase 12 (admin UX patterns) — both complete.
- Scope-pragmatic: ships minimum-viable admin (no drag-drop), full public surface.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/domain/anime.go` — `Anime` model. `AnimeInfo` projection.
- Player service has `anime_list` (user watchlist) which is structurally similar to `collection_items` — pattern reference.
- Phase 12's `AdminRecs.vue` is the canonical admin-view pattern (table + edit).
- Phase 9's `BrowseSidebar` pattern shows multi-select UI for admin item picker.

### Established Patterns

- GORM auto-migrate for new tables.
- Gateway routing: `/api/admin/*` → catalog with JWT + admin gate (defense-in-depth).
- Public routes: register before admin routes due to chi longest-prefix matching.

### Integration Points

- New tables: `collections`, `collection_items`. Auto-migrate on catalog startup.
- Gateway routing: `/api/collections/*` (public) + `/api/admin/collections/*` (admin).
- Admin router: extend admin role middleware to cover `/admin/collections`.

</code_context>

<specifics>
## Specific Ideas

- Slug auto-gen: simple kebab-case from title English. Admin can override.
- The Home "Подборки" row is the visual headline. Each card is a tall poster-style tile with title overlay (matching the Top-10 visual mode).
- Detail page (`/collections/:slug`) shows the curated description as a hero blurb + the anime grid below.
- Empty state: when no published collections exist, hide the Home row entirely.

</specifics>

<deferred>
## Deferred Ideas

- Drag-and-drop reordering of items in admin edit — Phase 20 polish.
- Image upload to MinIO for cover — URL paste is sufficient v0.1.
- Public sort/filter within a collection — out of scope; curator decides order.
- Featured/promoted collection slot — defer.

</deferred>
