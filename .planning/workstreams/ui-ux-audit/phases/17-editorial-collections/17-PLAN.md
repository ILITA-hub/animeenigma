---
phase: 17-editorial-collections
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - services/catalog/internal/domain/collection.go
  - services/catalog/internal/repo/collection.go
  - services/catalog/internal/repo/collection_test.go
  - services/catalog/internal/service/collection.go
  - services/catalog/internal/handler/collection.go
  - services/catalog/internal/transport/router.go
  - services/catalog/cmd/catalog-api/main.go
  - services/gateway/internal/transport/router.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/views/admin/AdminCollections.vue
  - frontend/web/src/views/admin/AdminCollectionEdit.vue
  - frontend/web/src/views/Collections.vue
  - frontend/web/src/components/home/CollectionsRow.vue
  - frontend/web/src/views/Home.vue
  - frontend/web/src/router/index.ts
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
autonomous: true
requirements:
  - UX-33
must_haves:
  truths:
    - "GET /api/collections returns a flat array of published collections (slug, title, title_ru, title_jp, cover_image_url, item_count); drafts are hidden."
    - "GET /api/collections/:slug returns the full collection record including its items[] joined to anime, sorted by sort_order ASC."
    - "POST /api/admin/collections (admin-only) creates a row; PUT updates; DELETE soft-deletes; POST/DELETE /:id/items adds/removes a CollectionItem."
    - "GET /api/admin/collections returns drafts + published; the gateway 403s when the caller is not an admin (defense-in-depth: catalog re-checks)."
    - "/admin/collections renders a list-of-collections table with a create button; /admin/collections/:id renders a form with title (3 langs), description (3 langs), cover URL, published toggle, slug field, and an anime-search-and-add item picker with per-item sort_order input + remove button."
    - "Home.vue renders a CollectionsRow between ThisWeekRow and ContinueWatchingRow that fetches GET /api/collections and links each card to /collections/:slug; the row hides itself when zero published collections exist."
    - "/collections/:slug renders the collection hero (title + description in current locale + cover image) and the curated anime grid sorted by sort_order ASC."
    - "All ~20 new i18n keys render in en/ru/ja with no fallback to key names."
  artifacts:
    - path: "services/catalog/internal/domain/collection.go"
      provides: "Collection + CollectionItem GORM models + DTO request types"
      contains: "type Collection struct"
      min_lines: 50
    - path: "services/catalog/internal/repo/collection.go"
      provides: "CollectionRepository with ListPublished / GetBySlug / ListAdmin / GetByID / Create / Update / Delete / AddItem / RemoveItem"
      contains: "func (r *CollectionRepository)"
      min_lines: 120
    - path: "services/catalog/internal/repo/collection_test.go"
      provides: "Unit tests for collection repo using libs/database test helpers (sqlite in-memory or testcontainers per existing repo test pattern)"
      contains: "func Test"
      min_lines: 60
    - path: "services/catalog/internal/service/collection.go"
      provides: "CollectionService wrapping repo + slug-auto-gen on Create"
      contains: "func (s *CollectionService)"
      min_lines: 60
    - path: "services/catalog/internal/handler/collection.go"
      provides: "CollectionHandler with 8 methods: ListPublic, GetBySlug, ListAdmin, Create, Update, Delete, AddItem, RemoveItem"
      contains: "func (h *CollectionHandler)"
      min_lines: 100
    - path: "frontend/web/src/views/admin/AdminCollections.vue"
      provides: "Admin list view with table + create button + delete action"
      min_lines: 80
    - path: "frontend/web/src/views/admin/AdminCollectionEdit.vue"
      provides: "Admin edit form (title × 3 / description × 3 / slug / cover URL / published / item picker)"
      min_lines: 150
    - path: "frontend/web/src/views/Collections.vue"
      provides: "Public detail page at /collections/:slug — hero + curated grid"
      min_lines: 80
    - path: "frontend/web/src/components/home/CollectionsRow.vue"
      provides: "Home row rendering top N published collections as tall poster tiles"
      min_lines: 60
  key_links:
    - from: "services/catalog/cmd/catalog-api/main.go"
      to: "db.AutoMigrate"
      via: "register &domain.Collection{} + &domain.CollectionItem{}"
      pattern: "Collection{}"
    - from: "services/catalog/internal/transport/router.go"
      to: "CollectionHandler"
      via: "public /api/collections + admin /api/admin/collections groups"
      pattern: "collections"
    - from: "services/gateway/internal/transport/router.go"
      to: "proxyHandler.ProxyToCatalog"
      via: "/api/collections/* (public) + /api/admin/collections/* (already covered by existing /api/admin/* admin group)"
      pattern: "/collections"
    - from: "frontend/web/src/views/Home.vue"
      to: "frontend/web/src/components/home/CollectionsRow.vue"
      via: "import + place between <ThisWeekRow /> and <ContinueWatchingRow />"
      pattern: "CollectionsRow"
    - from: "frontend/web/src/api/client.ts"
      to: "GET/POST/PUT/DELETE /api/(admin/)?collections/*"
      via: "animeApi.listCollections / animeApi.getCollection + adminApi.listCollections / createCollection / updateCollection / deleteCollection / addItem / removeItem"
      pattern: "listCollections"
---

# Phase 17 Plan: Editorial Collections (Dragon)

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Ship an admin-curated `Подборки` system end-to-end: 2 GORM tables (`collections`
+ `collection_items`), 8 endpoints (4 public + 4 admin), gateway proxies, admin
list + edit views, public Home row + detail page, i18n in en/ru/ja. No
drag-and-drop reordering (admin sets `sort_order` numerically), no MinIO upload
(admin pastes a cover URL) — those are deferred to a polish phase per CONTEXT.

## Tasks

### Wave 1 — Backend domain + repo + migrations

#### W1.1 Domain models (`services/catalog/internal/domain/collection.go`)

- [ ] Create the file with two GORM models + DTOs. `Collection` mirrors the
  `Anime` table's tag style (uuid PK, soft-delete via `gorm.DeletedAt`).
  `CollectionItem` is a join row with its own integer PK so admin reordering
  via `sort_order` is trivial:
  ```go
  // Collection is an admin-curated set of anime ("Подборки") rendered as a
  // Home row + a public /collections/:slug detail page. Phase 17 (UX-33).
  type Collection struct {
      ID             string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
      Slug           string         `gorm:"size:120;uniqueIndex" json:"slug"`
      Title          string         `gorm:"size:200" json:"title"`
      TitleRU        string         `gorm:"size:200" json:"title_ru,omitempty"`
      TitleJP        string         `gorm:"size:200" json:"title_jp,omitempty"`
      Description    string         `gorm:"type:text" json:"description,omitempty"`
      DescriptionRU  string         `gorm:"type:text" json:"description_ru,omitempty"`
      DescriptionJP  string         `gorm:"type:text" json:"description_jp,omitempty"`
      CoverImageURL  string         `gorm:"type:text" json:"cover_image_url,omitempty"`
      Published      bool           `gorm:"default:false;index" json:"published"`
      CreatedBy      string         `gorm:"type:uuid;index" json:"created_by,omitempty"`
      Items          []CollectionItem `gorm:"foreignKey:CollectionID" json:"items,omitempty"`
      ItemCount      int            `gorm:"-" json:"item_count"`
      CreatedAt      time.Time      `json:"created_at"`
      UpdatedAt      time.Time      `json:"updated_at"`
      DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
  }

  // CollectionItem is one anime in a collection with admin-defined order.
  type CollectionItem struct {
      ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
      CollectionID string    `gorm:"type:uuid;index" json:"collection_id"`
      AnimeID      string    `gorm:"type:uuid;index" json:"anime_id"`
      Anime        *Anime    `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
      SortOrder    int       `gorm:"default:0;index" json:"sort_order"`
      CreatedAt    time.Time `json:"created_at"`
  }
  ```
  Also add DTOs in the same file:
  ```go
  type CreateCollectionRequest struct {
      Slug          string `json:"slug"`
      Title         string `json:"title" validate:"required"`
      TitleRU       string `json:"title_ru"`
      TitleJP       string `json:"title_jp"`
      Description   string `json:"description"`
      DescriptionRU string `json:"description_ru"`
      DescriptionJP string `json:"description_jp"`
      CoverImageURL string `json:"cover_image_url"`
      Published     bool   `json:"published"`
  }

  type UpdateCollectionRequest struct {
      Slug          *string `json:"slug,omitempty"`
      Title         *string `json:"title,omitempty"`
      TitleRU       *string `json:"title_ru,omitempty"`
      TitleJP       *string `json:"title_jp,omitempty"`
      Description   *string `json:"description,omitempty"`
      DescriptionRU *string `json:"description_ru,omitempty"`
      DescriptionJP *string `json:"description_jp,omitempty"`
      CoverImageURL *string `json:"cover_image_url,omitempty"`
      Published     *bool   `json:"published,omitempty"`
  }

  type AddCollectionItemRequest struct {
      AnimeID   string `json:"anime_id" validate:"required"`
      SortOrder int    `json:"sort_order"`
  }
  ```

#### W1.2 Auto-migrate registration (`services/catalog/cmd/catalog-api/main.go`)

- [ ] In the existing `db.AutoMigrate(...)` call (around line 47), add the two
  new models to the slice immediately after `&domain.AnimeTag{}`:
  ```go
  &domain.Collection{},
  &domain.CollectionItem{},
  ```
  Per CLAUDE.md "Database migrations" — GORM auto-creates the new tables on
  catalog startup, no manual SQL.

#### W1.3 Repository (`services/catalog/internal/repo/collection.go`)

- [ ] Create the file with the `CollectionRepository` type. Mirror the
  `AnimeRepository` constructor pattern (single `*gorm.DB`, methods return
  `(value, error)`). Required methods:

  ```go
  type CollectionRepository struct {
      db *gorm.DB
  }

  func NewCollectionRepository(db *gorm.DB) *CollectionRepository { ... }

  // ListPublished — public Home row + public detail-page link list.
  // Returns published rows ordered by CreatedAt DESC. Sets ItemCount via a
  // single COUNT subquery per row (or a JOIN+GROUP BY, implementer's call —
  // ItemCount must be populated, not the gorm-ignored zero).
  func (r *CollectionRepository) ListPublished(ctx context.Context, limit int) ([]*domain.Collection, error)

  // GetBySlug — public detail page. Preloads Items.Anime (full anime
  // record so the public page renders titles + posters without a second
  // query). Returns liberrors.NotFound when missing or unpublished.
  func (r *CollectionRepository) GetBySlug(ctx context.Context, slug string) (*domain.Collection, error)

  // ListAdmin — admin list view. Returns drafts + published, ordered
  // by UpdatedAt DESC. Populates ItemCount the same way ListPublished does.
  func (r *CollectionRepository) ListAdmin(ctx context.Context) ([]*domain.Collection, error)

  // GetByID — admin edit-form load. Preloads Items.Anime so the edit form
  // can render the curated grid for reordering.
  func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*domain.Collection, error)

  func (r *CollectionRepository) Create(ctx context.Context, c *domain.Collection) error
  func (r *CollectionRepository) Update(ctx context.Context, c *domain.Collection) error
  func (r *CollectionRepository) Delete(ctx context.Context, id string) error // soft-delete via gorm

  func (r *CollectionRepository) AddItem(ctx context.Context, item *domain.CollectionItem) error
  func (r *CollectionRepository) RemoveItem(ctx context.Context, collectionID, animeID string) error
  ```

- [ ] All methods use `liberrors.NotFound` on `gorm.ErrRecordNotFound`, wrap
  other errors with `fmt.Errorf("%s: %w", op, err)` — matches the pattern in
  `services/catalog/internal/repo/anime.go`.
- [ ] `ListPublished` and `ListAdmin` MUST set `ItemCount` on each returned
  collection. Pick the implementation that reads cleanest in GORM (a
  separate COUNT query per row in a loop is acceptable at this scale —
  collections are a curated, low-cardinality table; expected size <100 rows
  ever).
- [ ] `GetBySlug` only returns rows where `published = true` AND
  `deleted_at IS NULL` — drafts are admin-only via `GetByID` /
  `ListAdmin`.
- [ ] `AddItem` is idempotent on `(collection_id, anime_id)` — duplicate
  call updates `sort_order` rather than inserting a second row. Use GORM
  upsert (`clause.OnConflict`) on the composite key, or a Where-then-Save
  flow.

#### W1.4 Repo unit tests (`services/catalog/internal/repo/collection_test.go`)

- [ ] Mirror the test pattern in any existing `*_test.go` under the catalog
  repo package (check `repo/anime_test.go` / `repo/video_test.go` for the
  in-memory or testcontainers DB setup — reuse exactly that helper).
- [ ] Cover the happy paths for each method:
  - `Create` then `GetByID` round-trip preserves all fields.
  - `Create` with `Published=true` shows up in `ListPublished`; `Published=false` does NOT.
  - `Create` with `Published=false` shows up in `ListAdmin`.
  - `GetBySlug` returns the row for a published slug; returns NotFound for
    a draft slug or an unknown slug.
  - `AddItem` × 2 with the same `(collection_id, anime_id)` does not
    duplicate the row; the second call updates `sort_order`.
  - `RemoveItem` deletes the join row; `RemoveItem` on a missing pair
    returns NotFound.
  - `Delete` soft-deletes — `GetByID` returns NotFound afterward;
    `ListAdmin` no longer returns the row.

#### W1.5 Wave 1 verification

- [ ] `cd services/catalog && go test ./internal/repo/... -run TestCollection -v` — all pass.
- [ ] `cd services/catalog && go test ./... && go vet ./...` — full package suite clean.

### Wave 2 — Backend service + handler + routes

#### W2.1 Service (`services/catalog/internal/service/collection.go`)

- [ ] Create `CollectionService` as a thin wrapper over `CollectionRepository`
  plus slug-generation on `Create`. Constructor takes `*repo.CollectionRepository`
  and `*logger.Logger`. Methods 1:1 with the handler:
  ```go
  func (s *CollectionService) ListPublished(ctx context.Context, limit int) ([]*domain.Collection, error)
  func (s *CollectionService) GetBySlug(ctx context.Context, slug string) (*domain.Collection, error)
  func (s *CollectionService) ListAdmin(ctx context.Context) ([]*domain.Collection, error)
  func (s *CollectionService) GetByID(ctx context.Context, id string) (*domain.Collection, error)
  func (s *CollectionService) Create(ctx context.Context, req *domain.CreateCollectionRequest, createdBy string) (*domain.Collection, error)
  func (s *CollectionService) Update(ctx context.Context, id string, req *domain.UpdateCollectionRequest) (*domain.Collection, error)
  func (s *CollectionService) Delete(ctx context.Context, id string) error
  func (s *CollectionService) AddItem(ctx context.Context, collectionID string, req *domain.AddCollectionItemRequest) (*domain.CollectionItem, error)
  func (s *CollectionService) RemoveItem(ctx context.Context, collectionID, animeID string) error
  ```
- [ ] Slug auto-gen on `Create` when `req.Slug` is empty:
  ```go
  // Simple kebab-case from req.Title — lowercase, strip non-alnum,
  // collapse whitespace to '-'. Append a 6-char random suffix on
  // unique-constraint collision (re-try once). Admin can override via
  // explicit req.Slug. Phase 17.
  func slugify(title string) string { ... }
  ```
  Implementation hint: `strings.ToLower` + a regex that keeps `[a-z0-9]+`,
  joined by `-`. Truncate at 80 chars. Re-roll the suffix on retry.
- [ ] `Update` applies only the non-nil pointer fields from the request to the
  loaded row (partial update — admin's "save title only" use case).

#### W2.2 Handler (`services/catalog/internal/handler/collection.go`)

- [ ] Create the file with `CollectionHandler` (mirrors `AdminHandler`
  pattern from `services/catalog/internal/handler/admin.go`). Constructor:
  ```go
  type CollectionHandler struct {
      svc *service.CollectionService
      log *logger.Logger
  }

  func NewCollectionHandler(svc *service.CollectionService, log *logger.Logger) *CollectionHandler { ... }
  ```
- [ ] Implement the 8 handler methods. Use `httputil.OK` / `Created` /
  `BadRequest` / `Error` from the existing libs:

  | Method | Verb + path | Auth | Body |
  |---|---|---|---|
  | `ListPublic`   | `GET    /api/collections`                            | none  | — |
  | `GetBySlug`    | `GET    /api/collections/{slug}`                     | none  | — |
  | `ListAdmin`    | `GET    /api/admin/collections`                      | admin | — |
  | `Create`       | `POST   /api/admin/collections`                      | admin | `CreateCollectionRequest` |
  | `Update`       | `PUT    /api/admin/collections/{id}`                 | admin | `UpdateCollectionRequest` |
  | `Delete`       | `DELETE /api/admin/collections/{id}`                 | admin | — |
  | `AddItem`      | `POST   /api/admin/collections/{id}/items`           | admin | `AddCollectionItemRequest` |
  | `RemoveItem`   | `DELETE /api/admin/collections/{id}/items/{animeId}` | admin | — |

- [ ] Each admin handler reads the caller's user id from
  `authz.ClaimsFromContext(r.Context())` (same pattern the existing admin
  handlers use). `Create` passes that id as the `createdBy` argument to
  `svc.Create`.
- [ ] `ListPublic` accepts an optional `?limit=N` query param (default 12,
  max 50) and forwards to `svc.ListPublished`.
- [ ] All responses use the existing `httputil` JSON helpers — no custom
  envelope.

#### W2.3 Catalog router wiring (`services/catalog/internal/transport/router.go`)

- [ ] Add `collectionHandler` to the `NewRouter` signature so `main.go` can
  pass it in. The new param goes immediately after `newsHandler`.
- [ ] Register the public routes inside the existing `r.Route("/api", ...)`
  block, BEFORE the `r.Route("/admin", ...)` group (chi matches in
  registration order — the admin group below uses a literal `/admin`
  prefix so it does not collide, but keep the public-before-admin
  convention used elsewhere):
  ```go
  r.Route("/collections", func(r chi.Router) {
      r.Get("/", collectionHandler.ListPublic)
      r.Get("/{slug}", collectionHandler.GetBySlug)
  })
  ```
- [ ] Inside the existing `r.Route("/admin", func(r chi.Router) { ... })`
  block (already wraps `AuthMiddleware` + `AdminMiddleware`), register the
  4 admin routes:
  ```go
  r.Get(   "/collections",                    collectionHandler.ListAdmin)
  r.Post(  "/collections",                    collectionHandler.Create)
  r.Put(   "/collections/{id}",               collectionHandler.Update)
  r.Delete("/collections/{id}",               collectionHandler.Delete)
  r.Post(  "/collections/{id}/items",         collectionHandler.AddItem)
  r.Delete("/collections/{id}/items/{animeId}", collectionHandler.RemoveItem)
  ```

#### W2.4 Wire service + handler in `main.go` (`services/catalog/cmd/catalog-api/main.go`)

- [ ] After the existing `animeRepo := repo.NewAnimeRepository(...)` block,
  add:
  ```go
  collectionRepo := repo.NewCollectionRepository(db.DB)
  ```
- [ ] After the existing `catalogService := service.NewCatalogService(...)`
  block, add:
  ```go
  collectionService := service.NewCollectionService(collectionRepo, log)
  ```
- [ ] After `newsHandler := handler.NewNewsHandler(...)`, add:
  ```go
  collectionHandler := handler.NewCollectionHandler(collectionService, log)
  ```
- [ ] Pass `collectionHandler` to `transport.NewRouter` (extra arg matching
  the W2.3 signature change).

#### W2.5 Gateway routing (`services/gateway/internal/transport/router.go`)

- [ ] Add the public proxy line in the catalog public block (right after the
  existing `r.HandleFunc("/animelib/*", proxyHandler.ProxyToCatalog)` line):
  ```go
  // Phase 17 (UX-33) — public editorial collections.
  r.HandleFunc("/collections", proxyHandler.ProxyToCatalog)
  r.HandleFunc("/collections/*", proxyHandler.ProxyToCatalog)
  ```
- [ ] **No new admin group needed** — the existing
  ```go
  r.Group(func(r chi.Router) {
      r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
      r.Use(AdminRoleMiddleware)
      r.HandleFunc("/admin/*", proxyHandler.ProxyToCatalog)
  })
  ```
  already covers `/api/admin/collections/*` (chi `/admin/*` matches any
  admin sub-path and proxies to catalog, which re-applies JWT +
  AdminMiddleware in `services/catalog/internal/transport/router.go` —
  defense-in-depth, no change needed).

#### W2.6 Wave 2 verification

- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `cd services/gateway && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog && make redeploy-gateway` — both healthy.
- [ ] Confirm the two new tables exist:
  ```bash
  docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
    -c "\dt" | grep -E "collections|collection_items"
  ```
  Expected: both tables present.
- [ ] Smoke the public list:
  ```bash
  curl -s http://localhost:8000/api/collections | jq
  ```
  Expected: `{"data": []}` (empty — no rows yet). Status 200.
- [ ] Smoke the admin guard (no token):
  ```bash
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/admin/collections
  ```
  Expected: `401`.
- [ ] Smoke admin create via the ui_audit_bot API key (per CLAUDE.md test
  user pattern — bot is admin-roled? if NOT admin, use a real admin token
  from the dev DB):
  ```bash
  # Replace with an admin token; ui_audit_bot is a USER, not admin.
  curl -s -X POST http://localhost:8000/api/admin/collections \
    -H "Authorization: Bearer $ADMIN_JWT" \
    -H "Content-Type: application/json" \
    -d '{"title":"Smoke","title_ru":"Дым","published":true}' | jq
  ```
  Expected: 201 + the created row with an auto-generated slug "smoke".
- [ ] Re-curl `GET /api/collections` — the smoke row is now in the result;
  `item_count: 0`.

### Wave 3 — Admin frontend

#### W3.1 API client methods (`frontend/web/src/api/client.ts`)

- [ ] Inside the existing `export const animeApi = { ... }` object (public
  methods), add:
  ```ts
  listCollections: (limit = 12) =>
    apiClient.get<Collection[]>('/collections', { params: { limit } }),
  getCollection: (slug: string) =>
    apiClient.get<Collection>(`/collections/${encodeURIComponent(slug)}`),
  ```
- [ ] Inside the existing `export const adminApi = { ... }` object, add:
  ```ts
  listCollections: () =>
    apiClient.get<Collection[]>('/admin/collections'),
  getCollection: (id: string) =>
    apiClient.get<Collection>(`/admin/collections/${id}`),
  createCollection: (body: CreateCollectionRequest) =>
    apiClient.post<Collection>('/admin/collections', body),
  updateCollection: (id: string, body: UpdateCollectionRequest) =>
    apiClient.put<Collection>(`/admin/collections/${id}`, body),
  deleteCollection: (id: string) =>
    apiClient.delete<void>(`/admin/collections/${id}`),
  addCollectionItem: (id: string, body: AddCollectionItemRequest) =>
    apiClient.post<CollectionItem>(`/admin/collections/${id}/items`, body),
  removeCollectionItem: (id: string, animeId: string) =>
    apiClient.delete<void>(`/admin/collections/${id}/items/${animeId}`),
  ```
- [ ] Add TypeScript types at the top of the file (alongside other shared
  types) — minimum surface to type the methods:
  ```ts
  export interface CollectionItem {
    id: string
    collection_id: string
    anime_id: string
    anime?: Anime
    sort_order: number
    created_at: string
  }
  export interface Collection {
    id: string
    slug: string
    title: string
    title_ru?: string
    title_jp?: string
    description?: string
    description_ru?: string
    description_jp?: string
    cover_image_url?: string
    published: boolean
    created_by?: string
    items?: CollectionItem[]
    item_count: number
    created_at: string
    updated_at: string
  }
  export interface CreateCollectionRequest {
    slug?: string
    title: string
    title_ru?: string
    title_jp?: string
    description?: string
    description_ru?: string
    description_jp?: string
    cover_image_url?: string
    published?: boolean
  }
  export type UpdateCollectionRequest = Partial<CreateCollectionRequest>
  export interface AddCollectionItemRequest {
    anime_id: string
    sort_order?: number
  }
  ```

#### W3.2 Admin list view (`frontend/web/src/views/admin/AdminCollections.vue`)

- [ ] Create the file following the `AdminRecs.vue` page chrome (cyan
  buttons, glass-card table, white text on dark bg). Renders a table:
  | col | content |
  |---|---|
  | Title | `collection.title` (with a small "DRAFT" pill when `!published`) |
  | Slug  | `<code>{{ collection.slug }}</code>` |
  | Items | `collection.item_count` |
  | Updated | `formatDistanceToNow(collection.updated_at)` |
  | Actions | "Edit" `router-link` to `/admin/collections/{id}` + "Delete" button (with `confirm()` guard) |
- [ ] Top-right "Create" button → router-pushes to
  `/admin/collections/new`. The edit view (W3.3) treats `:id === 'new'` as
  the create flow.
- [ ] On mount: `adminApi.listCollections()` → bind to a `ref` array.
  Loading + error states match `AdminRecs.vue` (`isLoading` spinner, red
  `glass-card` error block on 403 + generic error).
- [ ] Delete handler:
  ```ts
  async function onDelete(id: string) {
    if (!confirm(t('admin.collections.deleteConfirm'))) return
    await adminApi.deleteCollection(id)
    await load()
  }
  ```

#### W3.3 Admin edit view (`frontend/web/src/views/admin/AdminCollectionEdit.vue`)

- [ ] Create the file. The view handles BOTH create and update:
  - `route.params.id === 'new'` → empty form, POST on save.
  - otherwise → fetch via `adminApi.getCollection(id)`, PUT on save.
- [ ] Form fields (single column, labeled, glass-card wrapper):
  - **Slug** — `<input type="text">` (auto-generated server-side if blank
    on create; editable on update).
  - **Title (EN)** — required.
  - **Title (RU)** — optional.
  - **Title (JP)** — optional.
  - **Cover image URL** — `<input type="url">`. Below the input, render a
    100×140 preview when the URL is non-empty.
  - **Description (EN/RU/JP)** — 3 `<textarea>` blocks.
  - **Published** — toggle switch (when false, the collection is a draft).
- [ ] Below the form, an **Items** section:
  - Render the existing items as a list — each row shows poster (48×64),
    title, a `<input type="number" min="0">` for `sort_order`, and a
    "Remove" button. Changing `sort_order` triggers a `PUT
    /admin/collections/{id}` (no, simpler: change is local until "Save
    items" is pressed — pick whichever is cleaner; recommended:
    debounce-on-blur, call `removeItem` then `addItem` with the new
    `sort_order` since `AddItem` upserts).
  - Below the list, a search-and-add picker: `<input>` that hits
    `animeApi.searchAnime(query)` (debounced ~300ms), renders matches in a
    dropdown; clicking a match calls
    `adminApi.addCollectionItem(id, { anime_id })`, refreshes the list.
- [ ] **Item picker constraint:** the search call is the existing public
  search endpoint (`/api/anime/search?q=…`) — no new backend needed.
- [ ] Top-right "Save" button (sticky on scroll). On click:
  - For `new`: `adminApi.createCollection(body)` → on success
    `router.push('/admin/collections/' + created.id)` so the same view
    re-loads in edit mode and the items section becomes available.
  - For existing id: `adminApi.updateCollection(id, body)` → success toast.
- [ ] Show a "Preview" link next to Save that opens `/collections/:slug` in
  a new tab (only enabled when `published === true`).

#### W3.4 Router entries (`frontend/web/src/router/index.ts`)

- [ ] Add the two admin routes just after the existing `admin-recs-picker`
  block (line ~109), mirroring the `requiresAuth + requiresAdmin` meta:
  ```ts
  {
    path: '/admin/collections',
    name: 'admin-collections',
    component: () => import('@/views/admin/AdminCollections.vue'),
    meta: { titleKey: 'admin.collections.title', requiresAuth: true, requiresAdmin: true },
  },
  {
    path: '/admin/collections/:id',
    name: 'admin-collection-edit',
    component: () => import('@/views/admin/AdminCollectionEdit.vue'),
    meta: { titleKey: 'admin.collections.editTitle', requiresAuth: true, requiresAdmin: true },
  },
  ```
- [ ] Add the matching pass-through line in the gateway admin SPA block
  (`services/gateway/internal/transport/router.go` line ~134, the same
  pattern that exists for `/admin/recs`):
  ```go
  // Phase 17: /admin/collections/* falls through to the web SPA.
  r.HandleFunc("/collections", proxyHandler.ProxyToWeb)
  r.HandleFunc("/collections/*", proxyHandler.ProxyToWeb)
  ```
  These go inside the existing `r.Route("/admin", ...)` group so the admin
  JWT + role check still applies.

#### W3.5 Wave 3 verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — clean.
- [ ] `cd frontend/web && bunx eslint src/views/admin/AdminCollections.vue src/views/admin/AdminCollectionEdit.vue src/api/client.ts src/router/index.ts` — zero errors.
- [ ] `make redeploy-web && make redeploy-gateway` — both healthy.
- [ ] Manual: log in as admin (NOT the bot — bot is a regular user), visit
  `/admin/collections`. Create a row. Edit it. Add 2-3 anime via the
  search picker. Set published=true. Save.

### Wave 4 — Public frontend

#### W4.1 Public detail view (`frontend/web/src/views/Collections.vue`)

- [ ] Create the file. Mirrors the visual chrome of `Anime.vue` /
  `Schedule.vue` (gradient hero, `max-w-7xl` container, dark glass cards).
  On mount: `animeApi.getCollection(route.params.slug)` → bind.
- [ ] Hero block at the top:
  - Full-bleed background = `cover_image_url` (or a gradient fallback when
    empty), with a dark scrim for legibility.
  - `<h1>` = `getLocalizedTitle(c.title, c.title_ru, c.title_jp)` (existing
    `@/utils/title` helper).
  - Description = `getLocalizedDescription(c.description, c.description_ru, c.description_jp)`
    — if that helper doesn't exist yet, inline a small switch on `i18n.locale`.
- [ ] Below the hero, render the curated grid using the existing
  `AnimeCardNew.vue` (or whichever shared card the Browse view uses) — one
  card per `c.items[i].anime`, ordered by `sort_order ASC` (the backend
  already returns them sorted, but verify in the v-for key).
- [ ] Empty state: when `c.items.length === 0`, render a small "No anime
  yet" message under the hero — don't break the page.
- [ ] 404 handling: when `getCollection` returns a NotFound error, render
  the same `NotFound.vue`-style block ("Collection not found" + back link
  to `/`).
- [ ] Title key: `collections.title` is the page title meta (set via
  `useHead` if the project uses it, else just `document.title` in
  `onMounted`).

#### W4.2 Home row component (`frontend/web/src/components/home/CollectionsRow.vue`)

- [ ] Create the file mirroring `ContinueWatchingRow.vue` template chrome
  (`max-w-7xl mx-auto px-4 lg:px-8 mb-8`, label `<h2>`, horizontal-scroll
  flex). Differences:
  - Each card is a **tall poster tile** with the `cover_image_url` as the
    background (aspect-[2/3]) and the localised title overlaid at the
    bottom with a dark gradient — matches the "Top-10 visual mode" in
    CONTEXT.md "specifics".
  - Card width: `w-40 md:w-48 lg:w-56` — slightly wider than the
    Continue-Watching tile so the title can breathe.
  - `<router-link :to="`/collections/${c.slug}`">`.
- [ ] On mount: `animeApi.listCollections()` → bind. Hide the entire row
  when `items.length === 0` (same `v-if="items.length > 0"` pattern as
  `ContinueWatchingRow.vue` — per CONTEXT.md "specifics": empty state
  hides the row entirely).
- [ ] Loading skeleton: same flexible grey-pulse pattern as
  `ContinueWatchingRow.vue` lines 59-68 for visual consistency.

#### W4.3 Mount the row in Home.vue (`frontend/web/src/views/Home.vue`)

- [ ] Import: `import CollectionsRow from '@/components/home/CollectionsRow.vue'` near the existing `ContinueWatchingRow` import.
- [ ] In the template, place the row BETWEEN `<ThisWeekRow />` (line ~396)
  and `<ContinueWatchingRow />` (line ~404):
  ```vue
  <ThisWeekRow />

  <!-- Phase 17 (UX-33) — admin-curated editorial collections. -->
  <CollectionsRow />

  <ContinueWatchingRow />
  ```

#### W4.4 Public router entry (`frontend/web/src/router/index.ts`)

- [ ] Add immediately after the existing `/anime/:id` route block:
  ```ts
  {
    path: '/collections/:slug',
    name: 'collection-detail',
    component: () => import('@/views/Collections.vue'),
    meta: { titleKey: 'collections.title' },
  },
  ```

#### W4.5 Wave 4 verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — clean.
- [ ] `cd frontend/web && bunx eslint src/views/Collections.vue src/components/home/CollectionsRow.vue src/views/Home.vue src/router/index.ts` — zero errors.
- [ ] `make redeploy-web` — healthy.
- [ ] Manual: log out (anonymous). Visit `/`. The collections row shows the
  admin-created Smoke row. Click it. Land on `/collections/smoke`. Hero +
  curated grid render. Back to `/`.

### Wave 5 — i18n (en / ru / ja)

#### W5.1 Locale entries (`frontend/web/src/locales/{en,ru,ja}.json`)

- [ ] Add a new top-level `collections` namespace + an
  `admin.collections.*` namespace to each locale. ~20 keys × 3 = ~60 entries.
- [ ] **EN copy** (`frontend/web/src/locales/en.json`):
  ```json
  "collections": {
    "title": "Collections",
    "emptyItems": "No anime in this collection yet.",
    "notFound": "Collection not found.",
    "homeRowLabel": "Collections"
  },
  ```
  Inside the existing `"admin": { ... }` block:
  ```json
  "collections": {
    "title": "Collections (admin)",
    "editTitle": "Edit collection",
    "createNew": "Create collection",
    "tableTitle": "Title",
    "tableSlug": "Slug",
    "tableItems": "Items",
    "tableUpdated": "Updated",
    "tableActions": "Actions",
    "draftPill": "DRAFT",
    "edit": "Edit",
    "delete": "Delete",
    "deleteConfirm": "Delete this collection? Items are unlinked but anime records remain.",
    "fieldSlug": "Slug (URL)",
    "slugPlaceholder": "auto-generated from title if blank",
    "fieldTitleEn": "Title (EN)",
    "fieldTitleRu": "Title (RU)",
    "fieldTitleJp": "Title (JP)",
    "fieldDescriptionEn": "Description (EN)",
    "fieldDescriptionRu": "Description (RU)",
    "fieldDescriptionJp": "Description (JP)",
    "fieldCover": "Cover image URL",
    "fieldPublished": "Published",
    "itemsSection": "Anime in this collection",
    "itemsEmpty": "No anime yet — add one below.",
    "itemSearchPlaceholder": "Search anime by name…",
    "itemSortOrder": "Order",
    "itemRemove": "Remove",
    "save": "Save",
    "saved": "Saved",
    "preview": "Preview"
  }
  ```
- [ ] **RU copy** (`frontend/web/src/locales/ru.json`):
  ```json
  "collections": {
    "title": "Подборки",
    "emptyItems": "В этой подборке пока нет аниме.",
    "notFound": "Подборка не найдена.",
    "homeRowLabel": "Подборки"
  },
  ```
  Inside `"admin"`:
  ```json
  "collections": {
    "title": "Подборки (админ)",
    "editTitle": "Редактирование подборки",
    "createNew": "Создать подборку",
    "tableTitle": "Название",
    "tableSlug": "Слаг",
    "tableItems": "Записей",
    "tableUpdated": "Обновлено",
    "tableActions": "Действия",
    "draftPill": "ЧЕРНОВИК",
    "edit": "Редактировать",
    "delete": "Удалить",
    "deleteConfirm": "Удалить эту подборку? Связи разорвутся, аниме останутся.",
    "fieldSlug": "Слаг (URL)",
    "slugPlaceholder": "сгенерируется автоматически, если пусто",
    "fieldTitleEn": "Название (EN)",
    "fieldTitleRu": "Название (RU)",
    "fieldTitleJp": "Название (JP)",
    "fieldDescriptionEn": "Описание (EN)",
    "fieldDescriptionRu": "Описание (RU)",
    "fieldDescriptionJp": "Описание (JP)",
    "fieldCover": "URL обложки",
    "fieldPublished": "Опубликовано",
    "itemsSection": "Аниме в подборке",
    "itemsEmpty": "Пока пусто — добавьте аниме ниже.",
    "itemSearchPlaceholder": "Поиск аниме по названию…",
    "itemSortOrder": "Порядок",
    "itemRemove": "Удалить",
    "save": "Сохранить",
    "saved": "Сохранено",
    "preview": "Превью"
  }
  ```
- [ ] **JA copy** (`frontend/web/src/locales/ja.json`):
  ```json
  "collections": {
    "title": "コレクション",
    "emptyItems": "このコレクションにはまだアニメがありません。",
    "notFound": "コレクションが見つかりません。",
    "homeRowLabel": "コレクション"
  },
  ```
  Inside `"admin"`:
  ```json
  "collections": {
    "title": "コレクション (管理)",
    "editTitle": "コレクション編集",
    "createNew": "コレクション作成",
    "tableTitle": "タイトル",
    "tableSlug": "スラッグ",
    "tableItems": "件数",
    "tableUpdated": "更新日時",
    "tableActions": "操作",
    "draftPill": "下書き",
    "edit": "編集",
    "delete": "削除",
    "deleteConfirm": "このコレクションを削除しますか？ アニメ自体は残ります。",
    "fieldSlug": "スラッグ (URL)",
    "slugPlaceholder": "空欄ならタイトルから自動生成",
    "fieldTitleEn": "タイトル (EN)",
    "fieldTitleRu": "タイトル (RU)",
    "fieldTitleJp": "タイトル (JP)",
    "fieldDescriptionEn": "説明 (EN)",
    "fieldDescriptionRu": "説明 (RU)",
    "fieldDescriptionJp": "説明 (JP)",
    "fieldCover": "カバー画像 URL",
    "fieldPublished": "公開",
    "itemsSection": "コレクション内のアニメ",
    "itemsEmpty": "まだありません — 下から追加してください。",
    "itemSearchPlaceholder": "アニメを名前で検索…",
    "itemSortOrder": "順番",
    "itemRemove": "削除",
    "save": "保存",
    "saved": "保存しました",
    "preview": "プレビュー"
  }
  ```
- [ ] Adjust trailing commas so each locale stays valid JSON.

### Wave 6 — Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` — clean.
- [ ] `cd frontend/web && bunx eslint src/` — clean (or only pre-existing warnings).
- [ ] JSON validity:
  ```bash
  cd frontend/web && bun -e "['en','ru','ja'].forEach(l => JSON.parse(require('fs').readFileSync('src/locales/'+l+'.json','utf8'))); console.log('ok')"
  ```
- [ ] `cd services/catalog && go test ./... && go vet ./...` — clean.
- [ ] `cd services/gateway && go test ./... && go vet ./...` — clean.
- [ ] `make redeploy-catalog && make redeploy-gateway && make redeploy-web && make health` — all healthy.
- [ ] **Admin smoke (end-to-end):** log in as admin. `/admin/collections` →
  Create → title "Test Collection" / title_ru "Тестовая подборка" / a real
  cover URL / published=true → Save. Add 3 anime via the picker. Preview
  in new tab → `/collections/test-collection` renders correctly.
- [ ] **Public smoke (anonymous):** log out. Visit `/`. The
  `CollectionsRow` is visible between ThisWeek and ContinueWatching. Click
  the Test Collection tile → land on `/collections/test-collection`. Hero
  shows the cover, title in current locale, description below. Grid shows
  the 3 anime in `sort_order` order. Switch locale (en/ru/ja) — title +
  description swap correctly.
- [ ] **Empty-row hide:** admin un-publishes the Test Collection → reload
  `/` → CollectionsRow disappears (only collection was the test one).
- [ ] **Gateway gate:** `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/admin/collections` → 401; with a non-admin JWT → 403; with admin JWT → 200.
- [ ] axe-core re-run on `/`, `/collections/test-collection`, and
  `/admin/collections` — zero new violations vs. baseline.

## Files touched

```
services/catalog/internal/domain/collection.go                       (NEW — Collection + CollectionItem models + DTOs)
services/catalog/internal/repo/collection.go                         (NEW — repo with 10 methods)
services/catalog/internal/repo/collection_test.go                    (NEW — unit tests for repo)
services/catalog/internal/service/collection.go                      (NEW — service wrapper + slugify)
services/catalog/internal/handler/collection.go                      (NEW — 8 HTTP handlers)
services/catalog/internal/transport/router.go                        (+ public /api/collections group + 6 admin routes; NewRouter signature gains collectionHandler)
services/catalog/cmd/catalog-api/main.go                             (+ AutoMigrate Collection + CollectionItem; + collectionRepo + collectionService + collectionHandler wiring)
services/gateway/internal/transport/router.go                        (+ /api/collections public proxy; + /admin/collections SPA fallthrough inside existing admin group)
frontend/web/src/api/client.ts                                       (+ Collection / CollectionItem / *Request types; + animeApi.listCollections + getCollection; + 7 adminApi.* collection methods)
frontend/web/src/views/admin/AdminCollections.vue                    (NEW — admin list)
frontend/web/src/views/admin/AdminCollectionEdit.vue                 (NEW — admin form + item picker)
frontend/web/src/views/Collections.vue                               (NEW — public detail page)
frontend/web/src/components/home/CollectionsRow.vue                  (NEW — Home row)
frontend/web/src/views/Home.vue                                      (+ import + place CollectionsRow between ThisWeekRow and ContinueWatchingRow)
frontend/web/src/router/index.ts                                     (+ /collections/:slug + /admin/collections + /admin/collections/:id routes)
frontend/web/src/locales/en.json                                     (+ collections.* + admin.collections.* — ~20 keys EN)
frontend/web/src/locales/ru.json                                     (+ collections.* + admin.collections.* — ~20 keys RU)
frontend/web/src/locales/ja.json                                     (+ collections.* + admin.collections.* — ~20 keys JA)
.planning/workstreams/ui-ux-audit/phases/17-editorial-collections/
  17-CONTEXT.md                                                      (already exists)
  17-PLAN.md                                                         (this file)
  17-SUMMARY.md                                                      (written at execute-phase end)
  17-VERIFICATION.md                                                 (written at execute-phase end)
```

No new external libraries. Two new database tables created by GORM
AutoMigrate on catalog startup. No drag-and-drop (sort_order is a number
input). No MinIO upload (cover URL is pasted by the admin) — both deferred
per CONTEXT.md "Deferred Ideas".

## Closes

| Requirement | Surface | Mechanism |
|---|---|---|
| UX-33 | Home row + `/collections/:slug` + `/admin/collections*` | Backend: two new GORM tables (`collections`, `collection_items`) auto-migrated on catalog startup; 8 endpoints split public (`GET /api/collections`, `GET /api/collections/:slug`) and admin (`GET/POST/PUT/DELETE /api/admin/collections`, `POST/DELETE /api/admin/collections/:id/items`). Gateway proxies public path through and reuses the existing `/api/admin/*` admin-gated group for admin path. Frontend: `useFocusTrap`-free admin list + edit views (table + form + search-and-add item picker with numeric `sort_order`), public `CollectionsRow` (poster-style tiles, hides on empty) inserted between ThisWeekRow and ContinueWatchingRow on Home, public `Collections.vue` detail page with cover-image hero + curated grid sorted by `sort_order ASC`. i18n in en/ru/ja covers all new copy. Slug auto-generated kebab-case from title on create; admin can override. |

## Wave outline

| Wave | Tasks | Rationale |
|---|---|---|
| 1 (Domain + repo) | W1.1 models → W1.2 AutoMigrate → W1.3 repo with 10 methods → W1.4 unit tests → W1.5 go test | Pure backend foundation. Tables exist + repo round-trips work before any handler depends on them. |
| 2 (Service + handler + routes) | W2.1 service → W2.2 handler (8 methods) → W2.3 catalog router → W2.4 main.go wiring → W2.5 gateway proxies → W2.6 redeploy + curl smoke | Layer up to HTTP. Public endpoint returns `[]` and admin endpoint 401s before any frontend exists — proves the chain. |
| 3 (Admin frontend) | W3.1 api client → W3.2 list view → W3.3 edit view + item picker → W3.4 router + gateway SPA fallthrough → W3.5 vue-tsc + redeploy + manual create flow | Admin surface first so we have data to render publicly in Wave 4. |
| 4 (Public frontend) | W4.1 detail page → W4.2 home row → W4.3 mount in Home.vue → W4.4 router → W4.5 redeploy + anon smoke | Public surface depends on Wave 3 producing real data; verified end-to-end against the row created in W3. |
| 5 (i18n) | W5.1 ~20 keys × 3 locales (en / ru / jа) | Single sweep after the surfaces stabilise, before final verification. |
| 6 (Verification) | vue-tsc + eslint + JSON validity + go test + redeploys + make health + admin end-to-end + public anon smoke + locale switching + empty-row hide + gateway auth gates + axe-core re-run | One terminal gate covering all 4 endpoint families + both frontend surfaces + i18n + a11y before commit. |
