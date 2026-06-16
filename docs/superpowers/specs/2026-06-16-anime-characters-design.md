# Anime Characters Page — Design Spec

**Date:** 2026-06-16
**Status:** Approved (design) → pending implementation plan
**Scope metrics:** UXΔ = +3 (Better) · CDI = 0.04 × 21 · MVQ = Griffin 85%/80%

## Summary

Add an anime-characters feature to AnimeEnigma:

1. A **Characters section** on the anime detail page (`Anime.vue`) showing a grid of
   character cards (poster + name + role).
2. Clicking a card opens a **dedicated character page** (`/characters/:id`) with the
   character's poster, names (RU/EN/JP), synonyms, role, and description.

Data comes from **Shikimori GraphQL**, durably stored in **Postgres** and hot-cached in
**Redis**. Voice actors (seiyu) and character image galleries are explicitly **out of scope**.

## Data Source: Shikimori (verified 2026-06-16)

Verified via live GraphQL introspection + queries against `https://shikimori.io/api/graphql`
(shikimori.one 301-redirects to shikimori.io; DDoS-Guard present but GraphQL POST works).

### List of an anime's characters

`Anime.characterRoles` returns **characters only** (not staff). Each role:

```graphql
query($id: String!) {
  animes(ids: $id) {
    id
    characterRoles {
      rolesRu        # e.g. ["Main"] / ["Supporting"]
      rolesEn
      character {
        id           # Shikimori character id (canonical)
        malId
        name         # English/romaji
        russian      # RU name (site is RU-first — primary display)
        japanese     # JP name
        poster { originalUrl mainUrl }
      }
    }
  }
}
```

- `CharacterRole` has **only** `id`, `rolesRu`, `rolesEn`, `character`. **No `person`/seiyu field** —
  confirmed by introspection. Seiyu cannot be paired with a character from Shikimori (GraphQL
  `personRoles` and REST `/roles` both leave them unlinked). Hence **seiyu is out of scope**.
- Frieren (mal 52991) returns a clean set of character roles.

### Single character detail

```graphql
query($id: String!) {
  characters(ids: $id) {
    id name russian japanese synonyms url
    poster { originalUrl mainUrl }
    description descriptionHtml
  }
}
```

- `Character` exposes `description`, `descriptionHtml`, `synonyms`, `poster`, `malId`, names.
- **Description sanitization (required):** `description` contains bbcode links like
  `[character=196826]Хайтера[/character]`; `descriptionHtml` contains `<a href="shikimori.io/...">`
  anchors. The backend strips these to **plain text** (keep inner display name, drop the tag/link).
  No external links, no raw HTML to the client → no XSS surface.
- A character's poster is a single image in multiple sizes — **no image gallery** is available.

## Backend (catalog service)

### Postgres (GORM AutoMigrate)

`characters` table:

| field | type | notes |
|-------|------|-------|
| `id` | uuid pk | `gen_random_uuid()` |
| `shikimori_id` | string, unique index | canonical id used in the route |
| `mal_id` | string | nullable |
| `name` | string | English/romaji |
| `name_ru` | string | RU (primary display) |
| `name_jp` | string | JP |
| `synonyms` | string[]/jsonb | |
| `poster_url` | string | Shikimori `originalUrl` |
| `description` | text | **sanitized plain text** |
| `url` | string | Shikimori profile url (reference) |
| `created_at`/`updated_at` | timestamps | |

`anime_characters` join table (mirrors the existing `AnimeTag` pattern):

| field | type | notes |
|-------|------|-------|
| `anime_id` | uuid | FK → animes |
| `character_id` | uuid | FK → characters |
| `role` | string | `main` / `supporting` |
| `position` | int | display order within role |

PK = (`anime_id`, `character_id`).

### Endpoints (gateway → catalog:8081)

- `GET /api/anime/{uuid}/characters` — ordered list (Main first, then Supporting, by `position`).
  Reuses the existing anime-id resolution (UUID / `mal_*` / `shiki_*`).
- `GET /api/characters/{shikimoriId}` — single character detail.
- **Gateway:** add `/api/characters/*` → catalog:8081 route (new). `/api/anime/*` already routes to catalog.

### Caching & resilience (Postgres durable + Redis hot)

Flow for each endpoint:

1. Check Redis (`characters:anime:{id}` / `character:{id}`, TTL 6h, `cache.TTLAnimeDetails`).
2. On miss → fetch from Shikimori → **upsert into Postgres** → set Redis.
3. If Shikimori is down/errors → serve from Postgres (last-known-good).

Populate-if-absent: first request for an anime triggers the `characterRoles` fetch + upsert.
Redis TTL drives refresh (miss → re-fetch + re-upsert). No new sync scheduler in this phase.

## Frontend

- **Characters section in `Anime.vue`** (`#section-characters`, alongside episodes/comments/similar):
  responsive grid; Main characters first; cap ~12 with an inline **"Show all"** expand; skeletons
  while loading; empty-state when an anime has no characters.
- **`CharacterCard.vue`** (`components/characters/`): poster + name (RU, fallback EN) + role badge.
  Links to `/characters/{shikimoriId}`. Reuses `ui` primitives (Card/Badge/Avatar). Co-located `.spec.ts`.
- **`views/Character.vue`** — route `/characters/:id`: large poster, names (RU/EN/JP), synonyms, role,
  sanitized description, back link. Router prefetch guard mirroring `/anime/:id`.
- **Images** routed through streaming `/image-proxy` (same as anime posters — Shikimori CORS/referer).
- **Composables:** `useCharacters(animeId)` (list) + `useCharacter(id)` (detail).
- **i18n:** new keys added to **all three** locales (`en.json` / `ru.json` / `ja.json`) — i18n-lint
  is a hard prerequisite of `make redeploy-web`.
- **Design system:** semantic tokens only; `font-medium`/`font-semibold` only; reuse `ui` primitives;
  passes `design-system-lint.sh`.

## Testing

- **Go:** service + handler tests with handwritten fakes (no testify/mock): list ordering,
  detail fetch, bbcode→plain-text sanitization, populate-if-absent, Shikimori-down → Postgres fallback.
- **Frontend:** Vitest for `CharacterCard.vue` (name/role/link) and the composables; `tsc --noEmit`.

## Out of Scope (YAGNI — future phases)

- Voice actors / seiyu (Shikimori can't pair them to characters).
- Character image gallery (Shikimori gives one poster per character).
- "All anime featuring this character" cross-listing.
- Character search / a standalone all-characters index page.
- Background re-sync scheduler (TTL-driven refresh is sufficient for v1).

## Key Risks / Notes

- **bbcode/HTML sanitization** is mandatory before storage/return — don't ship raw `descriptionHtml`.
- **New libs/migrations:** no new `libs/` module needed (lives inside catalog). Postgres columns are
  additive via AutoMigrate.
- **Shikimori rate limiting:** reuse the existing rate-limited Shikimori client in catalog.
- **Image proxy:** character posters must join the proxied-image path; do not hotlink shikimori.io.
