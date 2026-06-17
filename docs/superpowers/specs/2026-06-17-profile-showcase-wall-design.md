# Profile Showcase Wall — Design Spec

**Date:** 2026-06-17
**Status:** Approved (design), pending implementation plan
**Feature:** Steam-style customizable profile showcase ("стена"), dark-shipped admin-only to release bundled with Gacha («Лудка»).

## Summary

Add an owner-editable, drag-and-drop "showcase" to user profiles — modeled on Steam's
profile showcases/About section. The **owner** customizes a stack of blocks (text,
favorite anime, stats, favorite characters, gacha card collection); **visitors view it
read-only**. The feature ships to main + prod immediately but is hidden behind an
admin-only gate (mirroring the Gacha «Лудка» dark-ship), so it can be revealed
simultaneously with Gacha by flipping env flags.

This is NOT a comment wall (no visitor-posted messages, no per-role posting privacy).
It is a self-expression showcase: only the profile owner edits it.

## Decisions (locked)

- **Type:** Steam Showcase / "About" — owner-edited content blocks, visitors read-only.
- **Blocks v1 (5 types):** `about` (free text), `favorite_anime` (poster grid),
  `stats` (auto-computed), `favorite_character` (Shikimori characters feature), and
  `card_collection` (gacha cards the owner chooses to display).
- **Editor UX:** drag-and-drop reordering (`vuedraggable`/SortableJS — touch support
  for mobile), add/remove blocks, per-block settings.
- **Architecture:** Extend existing `services/player` (NOT a new microservice).
  Single table with a JSONB `blocks` config column; content resolved at read time
  from existing sources (no duplication).
- **Dark-ship:** mirror Gacha gate. Env `PROFILE_WALL_ADMIN_ONLY` (gateway, default
  `true`) + `VITE_PROFILE_WALL_ADMIN_ONLY` (frontend, default `true`).

## Architecture

The showcase is a thin CRUD slice on `services/player` (port 8083), mirroring the
existing `comment.go` slice (domain → repo → service → handler). The DB stores only
the **layout config** (which blocks, in what order, with which references); the actual
display content (anime posters, character cards, gacha cards, stats) is resolved at
read time from existing data sources. This is the Steam model: showcase config is
stored separately, content is pulled in on render.

### Data model

One row per user. GORM AutoMigrate.

```go
// services/player/internal/domain/showcase.go
type ProfileShowcase struct {
    UserID    string         `gorm:"type:uuid;primaryKey" json:"user_id"`
    Blocks    datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"-"` // []Block
    UpdatedAt time.Time      `gorm:"not null;default:now()" json:"updated_at"`
}

// Block — stored inside Blocks JSON, validated in code (not enforced by DB).
type Block struct {
    Type   string         `json:"type"`   // about|favorite_anime|stats|favorite_character|card_collection
    Order  int            `json:"order"`  // ascending render order
    Config map[string]any `json:"config"` // type-specific, see below
}
```

Per-type `config` shape (validated by the service):

| Type                 | `config` fields                          | Notes |
|----------------------|------------------------------------------|-------|
| `about`              | `{ "title": string, "text": string }`    | title ≤ 64 runes, text ≤ 2000 runes. Plain text (sanitized; no HTML). |
| `favorite_anime`     | `{ "anime_ids": [uuid...] }`             | ≤ 12 ids; each must exist & be visible per the owner's public watchlist visibility. |
| `stats`              | `{}`                                     | No config; resolved live from watchlist aggregates. |
| `favorite_character` | `{ "character_ids": [shikimoriId...] }`  | ≤ 12 ids; resolved via catalog characters endpoint. |
| `card_collection`    | `{ "card_ids": [uuid...] }`              | ≤ 12 ids; each must be **owned** by the user (verified against gacha collection). |

Global limits: ≤ 12 blocks total. Duplicate block types are allowed (e.g. two `about`
blocks). Unknown block types and over-limit arrays are rejected at save time.

### Backend slice (`services/player`)

Files (mirror `comment.go` set):

- `internal/domain/showcase.go` — `ProfileShowcase`, `Block`, block-type constants,
  and a `ValidateBlocks([]Block) error` helper.
- `internal/repo/showcase.go` — `Get(ctx, userID) (*ProfileShowcase, error)` (returns
  empty showcase, not error, when none exists) and `Upsert(ctx, userID, blocks []Block) error`.
- `internal/service/showcase.go`:
  - `GetShowcase(ctx, userID, viewerClaims)` — loads blocks, returns each block's
    stored config (the id references) **plus** resolves `stats` inline (the player
    service already owns watchlist aggregates). It respects the profile's existing
    privacy settings: a private watchlist hides `stats` and the `favorite_anime`
    block for non-owner viewers.
  - `SaveShowcase(ctx, ownerID, blocks)` — validates (types, limits, ownership of
    cards, visibility of anime), then `Upsert`.

  **Content resolution split (locked):** the backend resolves only `stats` (data it
  owns). For `favorite_anime`, `favorite_character`, and `card_collection`, the
  endpoint returns the id references and the **frontend** resolves display data
  (posters/characters/cards) via the public APIs it already calls on the profile,
  anime, and gacha pages. This avoids new internal player→catalog/gacha coupling.
  Deleted/unavailable refs are simply not rendered (graceful skip, not an error).
- `internal/handler/showcase.go`:
  - `GET /api/users/{userId}/showcase` — public read; returns resolved blocks.
  - `PUT /api/users/me/showcase` — owner saves the full block array (JWT; `me`
    resolves to claims user id). Replaces the whole array (idempotent upsert).
- `cmd/player-api/main.go` — add `&domain.ProfileShowcase{}` to `AutoMigrate(...)`,
  wire repo→service→handler, register routes.

Cross-service reads for content resolution reuse existing internal calls/clients
the player service already has (catalog for anime/characters, gacha for owned cards).
Where the player service lacks a client, the resolver returns the raw ids and the
**frontend** fetches display data via its existing public APIs (decided per-block in
the plan; prefer frontend resolution to avoid new internal coupling where cheap).

### Dark-ship gate (mirror «Лудка»)

**Gateway** (`services/gateway`):

- `internal/config/config.go` — add `ProfileWallAdminOnly bool` (env
  `PROFILE_WALL_ADMIN_ONLY`, default `true`), documented like `GachaAdminOnly`.
- `internal/transport/router.go` — register the showcase routes. Because the public
  read route must sit **before** the protected `/users/*` wildcard group (chi
  longest-prefix), branch on the flag:
  - When `ProfileWallAdminOnly == true` (dark ship): a dedicated group with
    `JWTValidationMiddleware` + `userRateLimit` + `AdminRoleMiddleware` covering both
    `GET /users/{userId}/showcase` and `PUT /users/{userId}/showcase` (admins only,
    read & write).
  - When `false` (released): `GET /users/{userId}/showcase` registered with
    `OptionalJWTValidationMiddleware` **before** the `/users/*` group (public read);
    `PUT /users/me/showcase` falls through to the existing protected `/users/*` group.
  - Player-side handler still enforces owner-only writes (`me` == claims id) as
    defense-in-depth regardless of gateway gating.

**Frontend:**

- `src/utils/profileWallGate.ts` — exact mirror of `gachaGate.ts`:
  ```ts
  export const PROFILE_WALL_ADMIN_ONLY =
    (import.meta.env.VITE_PROFILE_WALL_ADMIN_ONLY as string | undefined) !== 'false'
  export function useProfileWallVisible() {
    const authStore = useAuthStore()
    return computed(() =>
      PROFILE_WALL_ADMIN_ONLY ? authStore.isAdmin : authStore.isAuthenticated)
  }
  ```
  (Note: visibility means "can see/use the feature at all". Viewing another user's
  showcase when released is open to all; during dark-ship only admins fetch it.)

### Frontend components

- `src/components/profile/showcase/ProfileShowcase.vue` — container; fetches
  `GET /users/{userId}/showcase`, renders blocks ordered by `order`, read-only for
  non-owners. Shows nothing when `!profileWallVisible`.
- Block renderers (read view):
  - `AboutBlock.vue` — title + sanitized text.
  - `FavoriteAnimeBlock.vue` — reuses `components/anime/PosterCard.vue`.
  - `StatsBlock.vue` — reuses existing profile stats data.
  - `FavoriteCharacterBlock.vue` — reuses the characters feature card UI.
  - `CardCollectionBlock.vue` — reuses gacha card display.
- `ShowcaseEditor.vue` — owner edit mode: `vuedraggable` list for reorder,
  add-block menu (block-type picker), remove-block, per-block config editors
  (anime picker from own watchlist, character picker, owned-card picker, about
  text fields). "Save" → `PUT /users/me/showcase` with the full block array.
- `views/Profile.vue` — embed `ProfileShowcase` in the profile (own + others'),
  show "Edit showcase" entry only when `isOwnProfile && profileWallVisible`. Hide
  the entire showcase area when `!profileWallVisible`.

### Dependencies

- Add `vuedraggable` (Vue 3 compatible, wraps SortableJS — provides touch/mobile
  drag support). Pin it in `vite.config` `manualChunks` per the project's bundle
  conventions (lazy/route-level so it doesn't bloat the main chunk; editor is
  owner-only).

### i18n

- New `showcase.*` namespace added to **all three** locales: `en.json`, `ru.json`,
  `ja.json`. `i18n-lint.sh` is a hard prereq of `make redeploy-web` and fails the
  build on any missing key — Japanese must not lag.

## Testing

- **Go** (`services/player`):
  - `service/showcase_test.go` — block validator (unknown type rejected, >12 blocks
    rejected, oversized arrays rejected, card ownership enforced, non-visible anime
    rejected), content resolution drops deleted refs, privacy hides
    `stats`/`favorite_anime` for a private watchlist.
  - `repo/showcase_test.go` — upsert creates then replaces; get-empty returns empty.
- **Vitest** (`frontend/web`):
  - One `.spec.ts` per block renderer (≥ basic render assertions).
  - `ShowcaseEditor.spec.ts` — drag-reorder updates `order`; add/remove mutates the
    block array; Save emits the correct `PUT` payload.
  - i18n parity for the `showcase.*` namespace if a parity spec is added (otherwise
    rely on `i18n-lint`).

## Release plan

Code lands on `main` + prod immediately but stays admin-only via the default-`true`
gates. To reveal bundled with Gacha:

```
GACHA_ADMIN_ONLY=false
VITE_GACHA_ADMIN_ONLY=false
PROFILE_WALL_ADMIN_ONLY=false
VITE_PROFILE_WALL_ADMIN_ONLY=false
```
then `make restart-gateway` + `make redeploy-web`.

## Out of scope (v1 / YAGNI)

- Visitor-posted comments / guestbook (this is a showcase, not a comment wall).
- Rich text / markdown / custom HTML in `about` (plain sanitized text only).
- Uploaded screenshot/artwork blocks (no new image storage in v1).
- Per-block privacy controls beyond the existing profile/watchlist privacy.
- Showcase themes / colors / backgrounds.
```