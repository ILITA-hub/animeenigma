# Лудка (Gacha) — Phase 2: Cards + Groups + Banners (domain & admin API) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the live `services/gacha/` (Phase 1: wallet+ledger) with the content domain — admin-curated **Cards** (image stored in a new MinIO bucket `gacha-cards`), admin-created **Groups** (M:N organizational tool), and **Banners** (gameplay pools with scheduling) — exposed via an admin-gated CRUD API plus a public image-serving route. No pull engine yet (Phase 3), no UI (Phase 5).

**Architecture:** Same service, new domain slice. Images: admin sends a file (multipart) OR a URL; either way the backend stores a copy in MinIO bucket `gacha-cards` (reusing `libs/videoutils.Storage`, the generic MinIO wrapper streaming already uses) and the card stores OUR serving path. Cards are served publicly via `GET /images/*` (browser `<img>` tags can't send JWT headers). Admin endpoints are double-gated: gateway (JWT + AdminRole, ALWAYS — independent of `GachaAdminOnly`) and service-side re-check via `authz.IsAdmin` (defense in depth, same as every other service re-validating the gateway's JWT).

**Tech Stack:** Go 1.24, chi/v5, GORM (Postgres), `libs/videoutils` (MinIO), `libs/authz`, sqlite for tests, httptest for the URL-download path.

---

## Context for the implementer

- Spec: `docs/superpowers/specs/2026-06-09-gacha-ludka-design.md` (§4.1–4.3, §4.8, §7, §12).
- Phase 1 (already LIVE on prod): `services/gacha/` — read `cmd/gacha-api/main.go`, `internal/transport/router.go`, `internal/repo/wallet.go`, `internal/repo/wallet_test.go` (the sqlite raw-DDL test-helper pattern), `internal/handler/*.go` before writing anything; mirror those patterns exactly.
- MinIO wrapper to reuse: `libs/videoutils/storage.go` — `NewStorage(StorageConfig)`, `EnsureBucket(ctx)`, `Upload(ctx, key, reader, size, contentType)`, `Download(ctx, key)`. Streaming's usage: `services/streaming/cmd/streaming-api/main.go:56`, multipart example `services/streaming/internal/handler/upload.go`.
- Admin-role helper: `authz.IsAdmin(ctx)` (`libs/authz/jwt.go:223`) over claims placed by the existing `AuthMiddleware`.
- **Rollout reality:** the gateway is NOT being redeployed until the bundled release (uncommitted parallel WIP in its working tree). Gateway route changes are committed but dormant. Live smoke therefore tests the gacha service directly on `127.0.0.1:8093`.
- **Dirty tree rules (unchanged from Phase 1):** never `git add -A`/`.`/`-a`; path-scoped commits; do not stage `go.work.sum`; do not touch other services' files. Co-author trailers on every commit (see Phase 1 plan).

## File Structure (Phase 2)

| File | Responsibility |
|------|----------------|
| `internal/domain/card.go` | `Card`, `Group`, `CardGroup`, `Banner`, `BannerCard` models + `Rarity` type/consts |
| `internal/repo/card.go` | Cards CRUD + group-filter list; Groups CRUD; membership ops |
| `internal/repo/card_test.go` | sqlite tests (raw DDL, mirror wallet_test.go) |
| `internal/repo/banner.go` | Banners CRUD, set/add cards, `AddGroupCards`, `ActiveNow` |
| `internal/repo/banner_test.go` | sqlite tests incl. active-window logic |
| `internal/service/content.go` | `ContentService` — validation + orchestration for cards/groups/banners |
| `internal/service/content_test.go` | validation tests |
| `internal/service/images.go` | `ImageService` — multipart/URL → MinIO; serving reader |
| `internal/service/images_test.go` | URL-download via httptest; type/size rejection |
| `internal/handler/admin.go` | `/api/gacha/admin/*` handlers (cards, groups, banners, upload) |
| `internal/handler/images.go` | `GET /images/*` public streaming handler |
| `internal/transport/router.go` (modify) | mount admin group (+service-side admin middleware) + images route |
| `cmd/gacha-api/main.go` (modify) | migrate new tables, MinIO init + EnsureBucket, DI |
| `internal/config/config.go` (modify) | `Storage videoutils.StorageConfig` from `MINIO_*` env (bucket default `gacha-cards` via `GACHA_MINIO_BUCKET`) |
| `go.mod` (modify) | + `libs/videoutils` require/replace |
| `docker/docker-compose.yml` (modify) | gacha env: `MINIO_ENDPOINT/ACCESS_KEY/SECRET_KEY/USE_SSL`, `GACHA_MINIO_BUCKET` |
| gateway (3 files, controller-handled) | admin + images routes, `/admin/gacha` SPA fall-through |

---

## Task 1: Domain models

**Files:** Create `services/gacha/internal/domain/card.go`

- [ ] **Step 1: Write the models**

```go
package domain

import (
	"time"

	"gorm.io/gorm"
)

// Rarity is the card tier. Pull odds per tier live in Phase 3 config.
type Rarity string

const (
	RarityN   Rarity = "N"
	RarityR   Rarity = "R"
	RaritySR  Rarity = "SR"
	RaritySSR Rarity = "SSR"
)

// ValidRarity reports whether r is one of the four known tiers.
func ValidRarity(r Rarity) bool {
	switch r {
	case RarityN, RarityR, RaritySR, RaritySSR:
		return true
	}
	return false
}

// Card is an admin-curated collectible character card (spec §4.1). ImagePath
// is the object key inside the gacha-cards MinIO bucket (e.g.
// "cards/<uuid>.webp") — the public URL is derived as /api/gacha/images/<path>.
type Card struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	SourceTitle string         `gorm:"size:256" json:"source_title"`
	ImagePath   string         `gorm:"size:512;not null" json:"image_path"`
	Rarity      Rarity         `gorm:"size:8;not null;index" json:"rarity"`
	Enabled     bool           `gorm:"not null;default:false;index" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Card) TableName() string { return "gacha_cards" }

// Group is an admin-created named collection of cards (spec §4.8) — an
// organizational tool only; it never affects pulls (banners do).
type Group struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Group) TableName() string { return "gacha_groups" }

// CardGroup is the M:N card↔group join row.
type CardGroup struct {
	GroupID string `gorm:"type:uuid;not null;uniqueIndex:idx_card_group,priority:1" json:"group_id"`
	CardID  string `gorm:"type:uuid;not null;uniqueIndex:idx_card_group,priority:2" json:"card_id"`
}

func (CardGroup) TableName() string { return "gacha_card_groups" }

// Banner is a gameplay pull pool (spec §4.2): a scheduled, admin-curated
// selection of cards. Exactly one banner should have IsStandard=true (the
// always-on pool); the rest are timed events layered on top.
type Banner struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Description string         `gorm:"size:1024" json:"description"`
	ArtPath     string         `gorm:"size:512" json:"art_path"`
	IsStandard  bool           `gorm:"not null;default:false" json:"is_standard"`
	Enabled     bool           `gorm:"not null;default:false;index" json:"enabled"`
	ActiveFrom  *time.Time     `json:"active_from,omitempty"`
	ActiveTo    *time.Time     `json:"active_to,omitempty"`
	SortOrder   int            `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Banner) TableName() string { return "gacha_banners" }

// BannerCard is the M:N banner↔card join row (the pull pool contents).
type BannerCard struct {
	BannerID string `gorm:"type:uuid;not null;uniqueIndex:idx_banner_card,priority:1" json:"banner_id"`
	CardID   string `gorm:"type:uuid;not null;uniqueIndex:idx_banner_card,priority:2" json:"card_id"`
}

func (BannerCard) TableName() string { return "gacha_banner_cards" }
```

- [ ] **Step 2:** `cd /data/animeenigma && go build ./services/gacha/internal/domain/` → success.
- [ ] **Step 3:** Commit `feat(gacha): card/group/banner domain models` (path: `services/gacha/internal/domain/`).

---

## Task 2: Cards + Groups repository (TDD)

**Files:** Create `services/gacha/internal/repo/card.go`, test `services/gacha/internal/repo/card_test.go`

- [ ] **Step 1: Failing tests.** Extend the sqlite raw-DDL helper pattern from `wallet_test.go` — add a `newContentTestDB(t)` creating `gacha_cards`, `gacha_groups`, `gacha_card_groups` with sqlite-compatible DDL (uuid via `lower(hex(randomblob(16)))`, `deleted_at DATETIME` nullable, the two unique indexes). Tests:

```go
func TestCardCRUD_CreateGetUpdateSoftDelete(t *testing.T)
// Create card (name, rarity SR, image path, enabled=false) → Get returns it;
// Update rarity→SSR + enabled=true → Get reflects; Delete → Get returns
// apperrors NotFound (soft-deleted rows must be invisible to List/Get).

func TestCardList_FiltersByRarityEnabledAndGroup(t *testing.T)
// Seed 4 cards (N/R/SR/SSR; two enabled), one group containing 2 of them.
// List(filter{Rarity:"SR"}) → 1; List(filter{Enabled:true}) → 2;
// List(filter{GroupID:g}) → 2; empty filter → 4.

func TestGroupCRUD_AndMembership(t *testing.T)
// Create group; rename; AddCards([c1,c2]); adding c1 again is a no-op (ON
// CONFLICT DO NOTHING, count stays 2); RemoveCard(c1) → 1 left; DeleteGroup
// removes the group AND its join rows (no orphans).
```

- [ ] **Step 2:** Run `cd /data/animeenigma && go test ./services/gacha/internal/repo/ -run 'Card|Group' -v` → FAIL (undefined).
- [ ] **Step 3: Implement `card.go`.** `ContentRepository` struct (one repo for cards+groups; banners get their own file). Methods:

```go
func NewContentRepository(db *gorm.DB) *ContentRepository

type CardFilter struct {
	Rarity  domain.Rarity // "" = any
	Enabled *bool         // nil = any
	GroupID string        // "" = any; joins gacha_card_groups
}

func (r *ContentRepository) CreateCard(ctx context.Context, c *domain.Card) error
func (r *ContentRepository) GetCard(ctx context.Context, id string) (*domain.Card, error)            // apperrors.NotFound on miss
func (r *ContentRepository) UpdateCard(ctx context.Context, c *domain.Card) error                    // Save by ID, NotFound if missing
func (r *ContentRepository) DeleteCard(ctx context.Context, id string) error                         // soft delete
func (r *ContentRepository) ListCards(ctx context.Context, f CardFilter) ([]domain.Card, error)      // ordered created_at DESC
func (r *ContentRepository) CreateGroup(ctx context.Context, g *domain.Group) error
func (r *ContentRepository) ListGroups(ctx context.Context) ([]domain.Group, error)
func (r *ContentRepository) RenameGroup(ctx context.Context, id, name string) error
func (r *ContentRepository) DeleteGroup(ctx context.Context, id string) error                        // tx: delete joins then group
func (r *ContentRepository) AddCardsToGroup(ctx context.Context, groupID string, cardIDs []string) error // ON CONFLICT DO NOTHING
func (r *ContentRepository) RemoveCardFromGroup(ctx context.Context, groupID, cardID string) error
func (r *ContentRepository) GroupCardIDs(ctx context.Context, groupID string) ([]string, error)
```

GroupID filter joins: `JOIN gacha_card_groups cg ON cg.card_id = gacha_cards.id AND cg.group_id = ?`.

- [ ] **Step 4:** Tests pass. **Step 5:** Commit `feat(gacha): cards + groups repository (CRUD, membership, filtered list)`.

---

## Task 3: Banners repository (TDD)

**Files:** Create `services/gacha/internal/repo/banner.go`, test `services/gacha/internal/repo/banner_test.go`

- [ ] **Step 1: Failing tests** (extend the same DDL helper with `gacha_banners`, `gacha_banner_cards`):

```go
func TestBannerCRUD_AndCardSet(t *testing.T)
// Create banner; SetCards([c1,c2,c3]) replaces the whole set atomically;
// SetCards([c2]) → only c2 remains; AddCards([c1]) appends; duplicate add no-ops.

func TestBannerAddGroupCards(t *testing.T)
// Group with 2 cards; AddGroupCards(banner, group) pulls both in; calling it
// again no-ops (still 2). This is the "добавить всю группу X" admin action.

func TestBannerActiveNow_WindowAndFlags(t *testing.T)
// 5 banners: enabled+no-window (active); enabled+window-around-now (active);
// enabled+window-in-past (NOT); disabled+no-window (NOT); standard+enabled
// (active, returned FIRST regardless of sort_order). ActiveNow(now) returns
// exactly the 3 active ones ordered: is_standard DESC, sort_order ASC.
// Pass `now time.Time` explicitly — no time.Now() inside the repo query
// (deterministic tests).
```

- [ ] **Step 2:** FAIL. **Step 3: Implement** `BannerRepository` (`NewBannerRepository(db)`): `CreateBanner/GetBanner/UpdateBanner/DeleteBanner/ListBanners` (admin list = all, created_at DESC), `SetCards` (tx: delete all joins for banner, bulk insert), `AddCards` (ON CONFLICT DO NOTHING), `AddGroupCards(ctx, bannerID, groupID)` (INSERT...SELECT from gacha_card_groups with ON CONFLICT DO NOTHING), `BannerCardIDs`, and:

```go
// ActiveNow returns banners visible to players at `now`: enabled AND
// (active_from IS NULL OR active_from <= now) AND (active_to IS NULL OR
// active_to >= now), ordered is_standard DESC, sort_order ASC, created_at ASC.
func (r *BannerRepository) ActiveNow(ctx context.Context, now time.Time) ([]domain.Banner, error)
```

- [ ] **Step 4:** Tests pass. **Step 5:** Commit `feat(gacha): banners repository (CRUD, card set ops, group import, ActiveNow)`.

---

## Task 4: Content service (validation layer, TDD)

**Files:** Create `services/gacha/internal/service/content.go`, test `content_test.go`

- [ ] **Step 1: Failing tests:**

```go
func TestCreateCard_ValidatesNameRarityImage(t *testing.T)
// empty name → InvalidInput; bad rarity "XX" → InvalidInput; empty image
// path → InvalidInput; valid → persisted via repo.

func TestCreateBanner_DefaultsAndValidation(t *testing.T)
// empty name → InvalidInput; ActiveTo before ActiveFrom → InvalidInput;
// valid → persisted.
```

- [ ] **Step 2:** FAIL. **Step 3: Implement** `ContentService` wrapping both repos. Thin: validate (`ValidRarity`, non-empty name/image, window sanity `ActiveTo.After(ActiveFrom)` when both set), then delegate. Request structs with json tags (`CreateCardRequest{Name, SourceTitle, ImagePath, Rarity, Enabled, GroupIDs []string}` — GroupIDs assigned via `AddCardsToGroup` after create; `UpdateCardRequest` same + ID; banner equivalents; group create/rename take a bare name, non-empty). Reuse `apperrors.InvalidInput`.
- [ ] **Step 4:** Pass. **Step 5:** Commit `feat(gacha): content service — validation + orchestration for cards/groups/banners`.

---

## Task 5: Image service — MinIO upload from file or URL (TDD)

**Files:** Create `services/gacha/internal/service/images.go`, test `images_test.go`; modify `internal/config/config.go`, `go.mod`

- [ ] **Step 1: go.mod.** Add to `services/gacha/go.mod`: require `github.com/ILITA-hub/animeenigma/libs/videoutils v0.0.0` + replace `=> ../../libs/videoutils`. Run `cd /data/animeenigma/services/gacha && go mod tidy` (workspace resolves it). Do NOT stage `go.work.sum` if it changes.
- [ ] **Step 2: Config.** Add to gacha `Config`: `Storage videoutils.StorageConfig`. In `Load()` (mirror streaming's env names, but bucket has its own var):

```go
		Storage: videoutils.StorageConfig{
			Endpoint:        getEnv("MINIO_ENDPOINT", "minio:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("MINIO_USE_SSL", false),
			BucketName:      getEnv("GACHA_MINIO_BUCKET", "gacha-cards"),
		},
```

- [ ] **Step 3: Failing tests** for the URL path + validation (no real MinIO — define a tiny `objectStore` interface and fake it):

```go
// images.go defines:
// type objectStore interface {
//     Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
// }
// (a thin adapter over *videoutils.Storage satisfies it in main.go)

func TestIngestFromURL_DownloadsAndStores(t *testing.T)
// httptest server returns 200 image/png bytes; IngestFromURL stores via fake
// objectStore; returned key matches `^cards/[0-9a-f-]{36}\.png$`; fake
// captured the right contentType + byte size.

func TestIngestFromURL_RejectsBadTypeAndTooLarge(t *testing.T)
// content-type text/html → InvalidInput; Content-Length 11MB → InvalidInput
// (also enforce via io.LimitReader so a lying server can't exceed the cap).

func TestIngestUpload_RejectsUnknownExtension(t *testing.T)
// multipart filename "x.exe" / content-type application/octet-stream → InvalidInput.
```

- [ ] **Step 4:** FAIL. **Step 5: Implement `images.go`.** `ImageService{store objectStore, http *http.Client, maxBytes int64}` (`NewImageService(store)` with 10s client timeout, 10 MiB cap):
  - `IngestFromURL(ctx, rawURL, kind string) (key string, err error)` — GET the URL (timeout, max size), allow content types `image/jpeg|png|webp|gif` (map → ext), key `fmt.Sprintf("%s/%s%s", kind, uuid.NewString(), ext)` where `kind ∈ {"cards","banners"}` (validate). Read into memory via `io.LimitReader(resp.Body, maxBytes+1)`; if read exceeds maxBytes → InvalidInput. Upload with the real size.
  - `IngestUpload(ctx, file io.Reader, filename, contentType, kind string) (string, error)` — same allowlist (trust contentType header, fall back to extension), same cap, same key scheme.
  - `uuid` — use `github.com/google/uuid` (already an indirect dep via GORM; add as direct require).
- [ ] **Step 6:** Pass (`go test ./services/gacha/internal/service/ -v`). **Step 7:** Commit `feat(gacha): image ingest service — file/URL → MinIO with type+size caps` (paths: `services/gacha/internal/service/`, `services/gacha/internal/config/`, `services/gacha/go.mod`, `services/gacha/go.sum`).

---

## Task 6: Admin + image handlers, router wiring

**Files:** Create `internal/handler/admin.go`, `internal/handler/images.go`; modify `internal/transport/router.go`

- [ ] **Step 1: `admin.go`.** `AdminHandler{content *service.ContentService, images *service.ImageService, log}`. Endpoints (all return via httputil; parse JSON via `httputil.Bind`):
  - Cards: `POST /cards` (CreateCardRequest), `GET /cards` (query params `rarity`, `enabled`, `group_id` → CardFilter), `GET /cards/{id}`, `PATCH /cards/{id}`, `DELETE /cards/{id}`.
  - Groups: `POST /groups {name}`, `GET /groups`, `PATCH /groups/{id} {name}`, `DELETE /groups/{id}`, `POST /groups/{id}/cards {card_ids: []}`, `DELETE /groups/{id}/cards/{cardId}`.
  - Banners: `POST /banners`, `GET /banners`, `GET /banners/{id}` (include `card_ids`), `PATCH /banners/{id}`, `DELETE /banners/{id}`, `PUT /banners/{id}/cards {card_ids: []}` (SetCards), `POST /banners/{id}/cards {card_ids: []}` (AddCards), `POST /banners/{id}/groups/{groupId}` (AddGroupCards — the "добавить всю группу" action).
  - Upload: `POST /upload` — if `Content-Type: multipart/form-data`: `r.ParseMultipartForm(12<<20)`, `FormFile("file")`, optional `kind` form value (default "cards") → `IngestUpload`. Else JSON `{image_url, kind}` → `IngestFromURL`. Respond `{"image_path": key, "image_url": "/api/gacha/images/" + key}`.
- [ ] **Step 2: `images.go`.** `ImagesHandler{store imageReader, log}` where `imageReader` is `Download(ctx, key) (io.ReadCloser, *videoutils.VideoFile, error)` — adapter over `*videoutils.Storage`. `GET /images/*`: take `chi.URLParam(r, "*")`, REJECT keys containing `..` or not matching `^(cards|banners)/[A-Za-z0-9._-]+$` (no traversal), stream with the stored content type + `Cache-Control: public, max-age=86400`; 404 on store miss.
- [ ] **Step 3: Router.** In `transport/router.go` add:

```go
	// Public card/banner art. Browsers load these via <img> (no JWT header
	// possible), so the route is unauthenticated by design: keys are
	// unguessable UUIDs and the content is anime character art.
	r.Get("/images/*", imagesHandler.Serve)

	// Admin content API. The gateway already requires JWT+AdminRole for
	// /api/gacha/admin/*; this service re-validates both (defense in depth,
	// same pattern as every service re-checking the gateway's JWT).
	r.Route("/api/gacha/admin", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))
		r.Use(RequireAdmin)
		// cards/groups/banners/upload routes per handler list above
	})
```

  `RequireAdmin` middleware (same file): claims already on ctx → `if !authz.IsAdmin(r.Context()) { httputil.Forbidden(w); return }` (verify the exact `httputil.Forbidden` signature via grep; if absent use the project's 403 helper).
- [ ] **Step 4:** `go build ./services/gacha/...` → OK. **Step 5:** Commit `feat(gacha): admin CRUD + upload handlers, public image route`.

---

## Task 7: main.go wiring + compose env

**Files:** Modify `cmd/gacha-api/main.go`, `docker/docker-compose.yml`

- [ ] **Step 1: main.go.** After the Phase 1 AutoMigrate add the five new models: `&domain.Card{}, &domain.Group{}, &domain.CardGroup{}, &domain.Banner{}, &domain.BannerCard{}`. Init storage after DB:

```go
	storage, err := videoutils.NewStorage(cfg.Storage)
	if err != nil {
		log.Fatalw("failed to init minio storage", "error", err)
	}
	if err := storage.EnsureBucket(context.Background()); err != nil {
		log.Fatalw("failed to ensure gacha-cards bucket", "error", err)
	}
```

  Wire `ContentRepository`, `BannerRepository`, `ContentService`, `ImageService` (storage adapter), `AdminHandler`, `ImagesHandler`; pass to `NewRouter` (extend its signature).
- [ ] **Step 2: compose.** In the `gacha` service env block add: `MINIO_ENDPOINT: minio:9000`, `MINIO_ACCESS_KEY: minioadmin`, `MINIO_SECRET_KEY: minioadmin`, `MINIO_USE_SSL: "false"`, `GACHA_MINIO_BUCKET: gacha-cards`; add `minio: { condition: service_healthy }` to its `depends_on`. Validate: `docker compose -f docker/docker-compose.yml config >/dev/null`.
- [ ] **Step 3:** Full check `cd /data/animeenigma && go build ./services/gacha/... && go test ./services/gacha/... -count=1` → green. **Step 4:** Commit `feat(gacha): wire content domain + MinIO gacha-cards bucket (boot, compose)` (paths: `services/gacha/`, `docker/docker-compose.yml`).

---

## Task 8 (CONTROLLER ONLY — dirty router.go): gateway routes

**Files:** Modify `services/gateway/internal/transport/router.go` (selective staging!)

- [ ] Add inside the existing `/gacha` route group (`router.go` ~473), as siblings of the wallet group:

```go
			// Public art route — must work from <img> tags (no JWT header).
			r.Get("/images/*", proxyHandler.ProxyToGacha)

			// Admin content API — ALWAYS admin-gated (independent of the
			// GachaAdminOnly dark-ship flag: these are admin tools, full stop).
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(AdminRoleMiddleware)
				r.HandleFunc("/admin/*", proxyHandler.ProxyToGacha)
			})
```

- [ ] Add the `/admin/gacha` SPA fall-through in the `/admin` group (next to `/raw-library`): `r.HandleFunc("/gacha", proxyHandler.ProxyToWeb)` + `r.HandleFunc("/gacha/*", proxyHandler.ProxyToWeb)`. **This lands near the parallel-WIP hunk — stage via the `git apply --cached` hunk-extraction used in Phase 1.**
- [ ] `cd services/gateway && go build ./...` → OK. Commit gateway files (router.go gacha hunks only), message `feat(gacha): gateway — admin content API + public images + /admin/gacha SPA fall-through`. Remains dormant until the bundled gateway redeploy.

---

## Task 9 (CONTROLLER ONLY): deploy + live smoke

- [ ] `make redeploy-gacha`; container healthy; boot log shows bucket ensured.
- [ ] Verify MinIO bucket exists: `docker compose -f docker/docker-compose.yml exec -T minio mc ls local/ 2>/dev/null || docker run --rm --network docker_default ...` — simplest reliable check: gacha boot log line + `SELECT` of `gacha_cards` table existence in psql.
- [ ] Direct-service auth smoke on `127.0.0.1:8093`: `GET /api/gacha/admin/cards` without token → 401; with non-admin `UI_AUDIT_API_KEY`-minted token → (cannot mint without gateway; acceptable: 401-only check) — full admin CRUD smoke deferred to Phase 5 UI / gateway activation.
- [ ] `make health` all green; push.

---

## Self-Review

- **Spec coverage:** §4.1 cards ✅ (T1/T2), §4.8 groups ✅ (T1/T2), §4.2-4.3 banners+join ✅ (T1/T3), §7.3 upload file/URL→MinIO `gacha-cards` ✅ (T5/T6/T7), §7.1/7.1a/7.2 admin API ✅ (T6, UI itself = Phase 5), images serving (needed by §6 UI) ✅ (T6), gateway admin-gating + SPA fall-through §3.2 ✅ (T8). Pull engine/collection — Phase 3 by design.
- **Placeholders:** route lists in T6 enumerate concrete endpoints; handler bodies follow Phase 1's established handler pattern (Bind→service→OK/Error) — the repo itself is the template, per "Context for the implementer".
- **Type consistency:** `domain.Rarity` used in CardFilter + validation; `ImagePath`/`image_path` consistent; `objectStore`/`imageReader` interfaces defined where used.
- **Risks:** videoutils dep adds minio-go to gacha (Dockerfile already copies videoutils go.mod — all Dockerfiles do). Banner `ActiveNow` takes `now` param → deterministic tests. Image route traversal guarded by regex.
