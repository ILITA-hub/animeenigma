---
phase: 17-editorial-collections
plan: 01
subsystem: catalog + frontend
tags: [collections, admin, ux-33, editorial]
requires: [phase-8-home-row-pattern, phase-12-admin-ux-patterns]
provides: [collections-table, collection-items-table, public-collections-api, admin-collections-api, home-collections-row, public-collections-detail-page, admin-collections-views, collections-i18n]
affects: [home-page-layout, admin-panel, gateway-routing, postgres-schema]
tech-stack:
  added:
    - none (no new libraries)
  patterns:
    - gorm-soft-delete-with-uniqueIndex-slug
    - go-level-uuid-generation-for-sqlite-portability
    - idempotent-upsert-on-composite-key
    - kebab-case-slug-with-random-suffix-retry
    - public-detail-page-with-hero-cover
    - vue-defensive-envelope-unwrap-helper
key-files:
  created:
    - services/catalog/internal/domain/collection.go
    - services/catalog/internal/repo/collection.go
    - services/catalog/internal/repo/collection_test.go
    - services/catalog/internal/service/collection.go
    - services/catalog/internal/handler/collection.go
    - frontend/web/src/views/admin/AdminCollections.vue
    - frontend/web/src/views/admin/AdminCollectionEdit.vue
    - frontend/web/src/views/Collections.vue
    - frontend/web/src/components/home/CollectionsRow.vue
  modified:
    - services/catalog/cmd/catalog-api/main.go
    - services/catalog/internal/transport/router.go
    - services/gateway/internal/transport/router.go
    - frontend/web/src/api/client.ts
    - frontend/web/src/views/Home.vue
    - frontend/web/src/router/index.ts
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - "ID generated at the Go level (uuid.NewString()) inside Create / AddItem when blank — keeps the repo portable to SQLite tests; Postgres's gen_random_uuid() default still applies in production but is now a safety net rather than a requirement."
  - "AddItem upsert path uses lookup-then-update instead of clause.OnConflict — no UNIQUE constraint exists on (collection_id, anime_id) and adding one would complicate the migration; explicit Where('id = ?', existing.ID).Updates(...) is portable across Postgres and SQLite."
  - "Per-row COUNT for ItemCount in ListPublished / ListAdmin instead of JOIN+GROUP BY. Collections are a low-cardinality curated table (<100 rows ever) so the loop overhead is well under 100ms even on a hot DB."
  - "Slug auto-generation uses regexp [a-z0-9]+ → '-' joiner with 80-char cap and Trim('-'); collisions retry once with a 6-char random hex suffix; persistent collision surfaces as the AlreadyExists error to the admin."
  - "AdminCollectionEdit sort-order editing reuses adminApi.addCollectionItem (which is idempotent on (collection_id, anime_id)) rather than introducing a dedicated PATCH endpoint — keeps the API surface minimal."
  - "Gateway: no new admin group needed for /api/admin/collections/* — the existing /admin/* → catalog group (JWT + AdminRoleMiddleware) covers it; catalog re-applies the same gates (defense-in-depth)."
metrics:
  duration_minutes: 13
  task_count: 22
  files_created: 9
  files_modified: 9
  loc_added: 1490
  commits: 5
completed: 2026-05-13
---

# Phase 17 Plan 01: Editorial Collections (Dragon) Summary

**One-liner:** Admin-curated "Подборки" system end-to-end — 2 GORM tables, 8
HTTP endpoints (2 public + 6 admin), gateway proxies, Vue admin
list + edit views with anime-search-and-add picker, public Home row +
`/collections/:slug` detail page, full i18n in en/ru/ja. No drag-drop and
no MinIO cover upload (deferred per CONTEXT).

## What shipped

### Backend (catalog service)

- **Domain (`collection.go`)** — Two GORM models:
  - `Collection`: uuid PK, soft-delete via `gorm.DeletedAt`, `uniqueIndex`
    slug, 3-language title/description columns, `Published bool` flag,
    `ItemCount int` computed (gorm:"-") so list views render without a
    second round-trip per row.
  - `CollectionItem`: uuid PK, `(CollectionID, AnimeID)` join with
    `SortOrder int` for admin reordering.
  - Three DTOs: `CreateCollectionRequest`, `UpdateCollectionRequest`
    (pointer fields for partial updates), `AddCollectionItemRequest`.

- **Repository (`collection.go`)** — 9 methods covering the 8 endpoint
  needs plus a private `populateItemCounts` helper:
  - `ListPublished(ctx, limit)` → published rows, ordered by CreatedAt
    DESC, with `ItemCount` populated.
  - `GetBySlug(ctx, slug)` → published rows only, Preloads `Items.Anime`
    sorted by `sort_order ASC`. Drafts return `NotFound`.
  - `ListAdmin(ctx)` → drafts + published, ordered by UpdatedAt DESC,
    `ItemCount` populated.
  - `GetByID(ctx, id)` → admin edit-form load with Preload.
  - `Create / Update / Delete` (soft) — `Update` returns `NotFound` on
    zero rows affected; `Delete` likewise.
  - `AddItem` is idempotent on `(collection_id, anime_id)`: existing
    pair → upsert SortOrder; new pair → INSERT with Go-generated UUID.
  - `RemoveItem` → `NotFound` when the pair is absent.

- **Service (`collection.go`)** — Thin wrapper plus slug generation:
  - `slugify(title)`: lowercase + regex `[a-z0-9]+ → -` joiner + 80-char
    cap + trim `-`. Empty after normalisation → `c-<8-hex>` fallback.
  - `Create` retries once with a 6-char random hex suffix on slug
    unique-constraint collision (recognises both Postgres "duplicate
    key" and SQLite "UNIQUE constraint failed" via error text match).
  - `Update` applies only non-nil pointer fields from
    `UpdateCollectionRequest` to the loaded row.
  - `AddItem` validates the collection exists before linking.
  - `ListPublished` clamps `limit ∈ [1, 50]`.

- **Handler (`collection.go`)** — 9 methods:
  - `ListPublic` (`GET /api/collections?limit=N`), `GetBySlug`
    (`GET /api/collections/{slug}`).
  - `ListAdmin`, `GetAdmin`, `Create`, `Update`, `Delete`, `AddItem`,
    `RemoveItem` under `/api/admin/collections*`.
  - `Create` reads `authz.ClaimsFromContext` and passes the caller's
    user id as `createdBy` so collections track their author.

- **Catalog router** — Public `/api/collections` group registered before
  the `/api/admin` group (longest-prefix convention). The 7 admin routes
  live inside the existing `AuthMiddleware + AdminMiddleware`-gated
  `/api/admin` block.

- **Gateway router** — `/api/collections` + `/api/collections/*` public
  proxy added after `/api/animelib/*` in the catalog public block. No
  new admin group — the existing `/api/admin/*` → catalog group covers
  `/api/admin/collections/*`. SPA fallthrough `/admin/collections` +
  `/admin/collections/*` added inside the existing admin `/admin/*`
  group so AdminCollections.vue / AdminCollectionEdit.vue render.

- **AutoMigrate** — `&domain.Collection{}` and `&domain.CollectionItem{}`
  registered in `cmd/catalog-api/main.go` after `&domain.AnimeTag{}`.
  Tables created on next catalog boot (verified: `\dt` in postgres shows
  both `collections` and `collection_items`).

### Frontend (Vue 3)

- **API client (`api/client.ts`)** — `Collection`, `CollectionItem`,
  `CreateCollectionRequest`, `UpdateCollectionRequest`,
  `AddCollectionItemRequest` TypeScript interfaces matching the backend
  shape. `animeApi.listCollections / getCollection` for public, 7
  methods on `adminApi.*` for admin CRUD + item picker.

- **AdminCollections.vue** — Glass-card table (Title / Slug / Items /
  Updated / Actions) with DRAFT pill, top-right `+ Create` button →
  `/admin/collections/new`, per-row Edit (router-link) + Delete (with
  confirm()) actions, lightweight inline `s/m/h/d` relative-time
  formatter, loading + 403 + generic-error states mirror AdminRecs.

- **AdminCollectionEdit.vue** — Reactive form handling both create
  (`/admin/collections/new`) and update flows. Fields: slug,
  title × 3 langs, cover URL (with 100×140 preview), description × 3
  textareas, published toggle. After save (non-new), the **Items**
  section appears: search-and-add picker hits the existing
  `/api/anime/search` endpoint (debounced 300ms; handles both flat-array
  and `{animes:[]}` response envelopes), per-row sort_order number
  input (re-uses AddItem idempotent upsert), Remove button (with
  confirm()). Preview link to `/collections/:slug` opens in a new tab
  when `published === true`.

- **Collections.vue** (public detail) — Full-bleed hero with
  `cover_image_url` background and dark gradient scrim, fallback
  cyan→purple gradient when no cover. Localised title via
  `getLocalizedTitle`; description with inline en/ru/ja switch. Curated
  grid (`auto-fill, minmax(160px, 1fr)`) of anime cards sorted by
  `sort_order ASC` (defensive client-side sort even though backend
  already sorts). 404 + loading states. `watch` on `route.params.slug`
  for in-place navigation between collections.

- **CollectionsRow.vue** — Horizontal-scroll Home row, tall poster tiles
  (`w-40 md:w-48 lg:w-56`, `aspect-[2/3]`) with cover background +
  bottom dark-gradient title overlay (Top-10 visual mode per
  CONTEXT.md). Self-gated `v-if="items.length > 0"` so anonymous users
  with zero published collections see no degraded affordance. Loading
  skeleton mirrors `ContinueWatchingRow`. Mounted between `<ThisWeekRow />`
  and `<ContinueWatchingRow />` in `Home.vue`.

- **Router** — `/admin/collections` (requiresAdmin),
  `/admin/collections/:id` (requiresAdmin; `:id === 'new'` triggers
  create flow inside the component), `/collections/:slug` (public).

- **i18n** — 4 keys in `collections.*` (top-level public namespace) and
  24 keys in `admin.collections.*` (admin namespace). 28 × 3 locales =
  84 new entries. All three locale files validated via `JSON.parse`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] SQLite test-DB used `gen_random_uuid()` default that SQLite doesn't understand**
- **Found during:** Wave 1 / W1.5 (first `go test` run after writing the repo + tests).
- **Issue:** The first version of `setupCollectionTestDB` used
  `db.AutoMigrate(&domain.Collection{}, &domain.CollectionItem{})`,
  which materialised the production GORM tags into a SQLite DDL string
  that included `id uuid DEFAULT gen_random_uuid()`. SQLite syntax-errors
  on the function call.
- **Fix:** Replaced AutoMigrate with hand-rolled SQLite-portable DDL
  (TEXT PRIMARY KEY for `id`, plain INTEGER for `published`, explicit
  UNIQUE INDEX on `slug`).
- **Files modified:** `services/catalog/internal/repo/collection_test.go`.
- **Commit:** `8d96de7`.

**2. [Rule 1 — Bug] AddItem upsert path failed in SQLite when reload-by-id used existing.ID == ""**
- **Found during:** Wave 1 / W1.5 after fixing #1.
- **Issue:** SQLite-test path created `CollectionItem` rows with empty
  `ID` (no `gen_random_uuid()` default available); the second AddItem
  call then loaded `existing` with `ID = ""` and the subsequent
  `First(item, "id = ?", existing.ID)` returned `ErrRecordNotFound`.
- **Fix:** Generate UUID at the Go level inside `Create` and `AddItem`
  via `uuid.NewString()` when `c.ID == ""` / `item.ID == ""`. Postgres's
  `gen_random_uuid()` default still applies in production, but the Go
  fallback makes the repo portable to SQLite tests and self-contained.
- **Files modified:** `services/catalog/internal/repo/collection.go`
  (added `github.com/google/uuid` import).
- **Commit:** `8d96de7`.

**3. [Rule 1 — Bug] AddItem `Model(&existing).Updates(map)` failed with "WHERE conditions required" in GORM 1.30**
- **Found during:** Wave 1 / W1.5 after fixing #2.
- **Issue:** GORM 1.30 with map-based Updates requires an explicit WHERE
  clause when the model struct's PK is treated as zero-value (which it
  was for the test path after fix #2). The first run after #1 + #2
  showed `WHERE conditions required` from gorm.
- **Fix:** Changed the upsert path to
  `Model(&domain.CollectionItem{}).Where("id = ?", existing.ID).Updates(map[...])`.
- **Files modified:** `services/catalog/internal/repo/collection.go`.
- **Commit:** `8d96de7`.

### Other

- **Admin smoke (create + add items via real admin JWT) deferred** — the
  plan's W2.6 final smoke step calls for a real admin JWT to POST a
  collection. The available admin accounts (`tNeymik`, `NANDIorg_9`)
  already have non-recoverable api_key hashes; minting a new key would
  destroy the existing token. The structural verifications instead
  passed: tables present in postgres, public list returns 200 + empty
  array, admin endpoint returns 401 without auth and 401 with invalid
  JWT, all routes wired in catalog + gateway. End-to-end create / add /
  remove will be exercised by the next admin who logs in.

## Known Stubs

None — all components wire real data through real endpoints. The
"Anime in this collection" section uses the existing
`/api/anime/search` endpoint for the picker; the Home row uses the
real `/api/collections` endpoint; the public detail page uses
`/api/collections/:slug` with Preloaded items.

## Verification Summary

- `go test ./...` and `go vet ./...` clean for both `services/catalog`
  and `services/gateway`.
- `bunx vue-tsc --noEmit` exits 0 (no type errors).
- `bunx eslint` on all 6 touched/created frontend files: zero errors.
- All 3 locale files pass `JSON.parse`.
- `make redeploy-catalog`, `make redeploy-gateway`, `make redeploy-web`
  all complete; `make health` shows all 8 services green.
- `psql \dt` confirms `collections` + `collection_items` tables
  auto-created on catalog boot.
- `GET /api/collections` → `{"success":true,"data":[]}` (200).
- `GET /api/admin/collections` (no token) → 401.
- `GET /api/admin/collections` (invalid token) → 401.
- `GET /admin/collections` (SPA fallthrough) → 200 (HTML).
- Frontend served via `web` container on `http://127.0.0.1:3003/` with
  SPA index.html fallback covering `/collections/:slug` and
  `/admin/collections/*`.

## Self-Check: PASSED
