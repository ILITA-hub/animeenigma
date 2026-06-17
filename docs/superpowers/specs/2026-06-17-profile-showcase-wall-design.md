# Profile Showcase Wall — Design Spec

**Date:** 2026-06-17
**Status:** v1 SHIPPED + deployed (dark-shipped admin-only). **v2 design approved** (this revision) — adds layout variants, 4 new block types, and a curation model; pending v2 implementation plan.
**Feature:** Steam-style customizable profile showcase ("стена"), dark-shipped admin-only to release bundled with Gacha («Лудка»).
**Visual reference (v2):** interactive mockup at `docs/superpowers/specs/assets/` is not committed; the live design exploration lived at the per-block mockup pages (hub + about/anime/stats/characters/cards/new) built in the Neon Tokyo design system. v2 layout/motion below is derived from it.

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

## v2 — Layout variants, new block types & curation model

v1 (above) is shipped and deployed. v2 is an additive expansion **behind the same
dark-ship gate** (no new flag): more block types, a per-block `variant` (layout)
field, and an explicit content-curation model. Nothing in v2 changes the v1
storage contract except adding the optional `variant` field to `Block`.

### `variant` field (added to the Block model)

```go
type Block struct {
    Type    string         `json:"type"`
    Variant string         `json:"variant,omitempty"` // layout; defaults to the type's default when empty
    Order   int            `json:"order"`
    Config  map[string]any `json:"config"`
}
```

- `Variant` is validated **per type** against an allowlist (below). Unknown
  `(type, variant)` pairs are rejected at save; empty `Variant` ⇒ the type's
  default variant. The frontend renders the variant via a sub-dispatch inside
  each block component (one SFC per block type, a `v-if` chain on `variant`).
- Backend stays a **structural** validator — it checks the variant string is in
  the type's allowlist; it does NOT care what the variant looks like.

### Block types (v1 + v2) — variants, curation, data source

| Type | Curation | Default variant | All variants | Data source |
|------|----------|-----------------|--------------|-------------|
| `about` | manual | `quote` | `quote`, `bio`, `terminal`, `minimal`, `vn` | config text |
| `favorite_anime` | manual **+ Auto button** | `row` | `row`, `podium`, `grid`, `list`, `banner` | FE: public anime API |
| `favorite_character` | manual | `circles` | `circles`, `portraits`, `hero`, `hex` | FE: characters API |
| `card_collection` | manual **+ Auto button** | `row` | `row`, `fan`, `grid`, `hero`, `tilt3d` | FE: gacha collection API |
| `stats` | auto | `tiles` | `tiles`, `rings`, `bars`, `strip`, `heatmap` | BE inline (watchlist aggregates) |
| `continue_watching` *(new)* | auto | `cards` | `cards` | FE: existing continue-watching endpoint |
| `op_ed` *(new)* | manual | `grid` | `grid` | FE: themes service (user's voted/owned themes) |
| `anime_dna` *(new)* | auto | `bars` | `bars` | FE: existing watchlist genre facets |
| `compatibility` *(new)* | auto (pairwise) | `ring` | `ring` | **BE: NEW endpoint** (see below) |

**Curation model (locked):**
- **Manual** (owner curates ids in `config`): `about`, `favorite_anime`,
  `favorite_character`, `op_ed`, `card_collection`.
- **Manual + Auto button:** `favorite_anime` (auto = top-N by user score) and
  `card_collection` (auto = rarest/newest owned). The "Auto" button in the editor
  fills `config` ids; the owner can then tweak. Stored result is still plain ids.
- **Auto** (no config, computed at render): `stats`, `anime_dna`,
  `continue_watching`, `compatibility`.

### New block: `compatibility` — the one new backend endpoint

This is the only v2 block that cannot be resolved purely on the FE (it needs both
users' full lists incl. private data + a server-side computation). It is **pairwise**
and only renders for a **logged-in viewer on ANOTHER user's profile** (hidden on own
profile and for anonymous viewers).

- New endpoint: `GET /api/users/{userId}/compatibility` (player service), JWT
  required; computes the viewer (from claims) vs `{userId}` (the profile owner).
- **Blend formula (locked):** `score = 0.5·overlap + 0.4·scoreAgreement + 0.1·genreSim`
  - `overlap` = Jaccard of the two users' list entries (titles in common / union).
  - `scoreAgreement` = 1 − normalized mean abs. difference of scores on commonly-rated titles.
  - `genreSim` = cosine similarity of the two users' genre-facet vectors.
  - Returns `{ percent, shared_count, shared_sample: [animeId…] }`. Respects both
    users' watchlist privacy (private lists ⇒ block returns "unavailable", not an error).
- This is the only v2 item that adds a player→(watchlist) cross-read; it stays
  within the player service (it already owns watchlists), so no new inter-service coupling.

### Other new blocks (FE-resolved, no backend change)

- `continue_watching` — reuses the existing continue-watching endpoint the profile
  already has; renders landscape cards with an episode progress bar. Owner-only
  meaningfully (a viewer sees the owner's in-progress titles subject to activity privacy).
- `op_ed` — reuses the **themes** service; owner manually picks favorite OP/ED from
  the themes of anime they've watched/voted; renders cover + play + song title.
- `anime_dna` — reuses watchlist **genre facets** (already computed); renders neon
  percentage bars per top genre.

### Design & motion (v2 visual contract)

- Neon Tokyo tokens only (per `DESIGN-SYSTEM.md`); glass blocks, neon accents,
  rarity-colored gacha frames (SSR gold / SR violet / R cyan).
- Motion follows the `12-principles-of-animation` craft rules adopted during design:
  user-initiated transitions < 300 ms, entrances `ease-out` / exits `ease-in`, no
  linear motion, `:active` press scale 0.95–1.05, dimmed dialog backdrop, honor
  `prefers-reduced-motion`.
- Card-detail **dialog**: clicking a gacha card opens a scale+fade dialog (ease-out
  in, ease-in out, dimmed blurred backdrop, close on backdrop/✕/Esc).

### Editor additions (v2)

- Per-block **variant picker** (segmented control / dropdown) in `ShowcaseEditor`
  showing the type's allowed variants.
- **"Auto" button** on `favorite_anime` and `card_collection` block editors.
- Pickers for the new manual block (`op_ed`) reuse existing search/list surfaces
  (themes list). `continue_watching`/`anime_dna`/`compatibility` have no config
  (auto) — they appear in the add-block menu but show no picker.

### v2 file impact

- Backend: extend `domain.ValidateBlocks` (variant allowlist + new types);
  `stats` resolver gains the `heatmap`-needed aggregate (daily watch counts) if not
  already available; new `compatibility` handler/service/route in `services/player`;
  gateway routes the new `GET /users/{userId}/compatibility` under the same
  `PROFILE_WALL_ADMIN_ONLY` gate.
- Frontend: each block SFC gains a `variant` sub-dispatch; new SFCs
  `ContinueWatchingBlock.vue`, `OpEdBlock.vue`, `AnimeDnaBlock.vue`,
  `CompatibilityBlock.vue`; editor variant picker + Auto buttons; `showcase.*` i18n
  keys for all new types/variants in en/ru/ja.

### v2 testing

- Go: variant allowlist validation (valid pair accepted, unknown pair rejected,
  empty ⇒ default); `compatibility` blend math (overlap/score/genre weights,
  privacy ⇒ unavailable, no shared titles ⇒ 0%).
- Vitest: each new block SFC renders; variant sub-dispatch picks the right layout;
  Auto button fills config ids; editor variant picker updates `block.variant`.

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

## Out of scope (v1 + v2 / YAGNI)

- Visitor-posted comments / guestbook (this is a showcase, not a comment wall).
- Rich text / markdown / custom HTML in `about` (plain sanitized text only).
- Uploaded screenshot/artwork blocks (no new image storage).
- Per-block privacy controls beyond the existing profile/watchlist privacy.
- **Showcase-level customization (the "section C" set), explicitly deferred:**
  per-profile accent color/theme, banner cover image, 2-column layout, and
  "share showcase as PNG". Revisit after v2 ships.
- **`achievements`/badges block — deferred** until real achievements exist.
- **`watch_together` status block — deferred.**
- (Both were mocked during design but cut from v2 scope.)
```