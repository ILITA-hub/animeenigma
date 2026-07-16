# Anime Production Details — design

**Origin:** feedback report `2026-07-13T16-14-17_tNeymik_telegram` (TODO):
> «На странице аниме не хватает инфа типа кто когда сделал аниме (режиссёр студия и тп)»
> *(The anime page is missing info on who made the anime and when — director, studio, etc.)*

**Goal:** surface production metadata (studio, dates, source, rating, duration), the key
staff/crew (director & co.), and the voice cast on the anime page.

---

## Reachability findings (verified live against `shikimori.io`, 2026-07-16)

The `.one` domain now 301-redirects to `.io`; both the GraphQL (`/api/graphql`) and REST
(`/api/…`) endpoints answer there.

1. **Metadata is already on our wire.** `airedOn`, `duration`, `season`, `status`, `origin`
   (→ `MaterialSource`), `rating`, and **`studios`** are already fetched by the Shikimori
   client, stored on `domain.Anime`, and serialized by `GET /api/anime/{id}`. The frontend
   simply never renders them. Only the **end date** (`releasedOn`) is missing from both the
   query and the `Anime` struct.

2. **Staff/crew is reachable via GraphQL `personRoles`.** The old note in
   `2026-06-16-anime-characters-design.md` ("Shikimori can't link people to roles") is about
   `characterRoles` — a *different* field. `personRoles` carries a full `Person`
   (`{id, name, russian, japanese, isSeyu, isProducer, isMangaka, poster}`) plus `rolesEn` /
   `rolesRu`. Example (FMA:Brotherhood, id 5114): **488 person-roles, 35 distinct role
   labels** — but 334 are "Key Animation". The viewer-relevant crew is ~12 roles. RU labels
   ("Режиссёр", "Автор оригинала") come free from `rolesRu`; Shikimori provides no JP role
   label.

3. **Voice cast is reachable — per character, via REST only.**
   - GraphQL `Character` type has **no** seiyu field.
   - The anime-level REST `/api/animes/{id}/roles` returns 604 entries but pairs a character
     with a person **zero** times (characters and staff are separate entries).
   - The per-character REST `/api/characters/{id}` returns a **`seyu[]`** array
     (`{id, name, russian, image{original,preview,x96,x48}, url}`). JP seiyu and dub actors
     are mixed in one list with **no language flag** (Shikimori generally lists JP first).
   - ⇒ Getting the cast requires **one REST call per character**.

---

## Scope (locked with owner)

- **Staff roles:** headline whitelist (~12 roles), not the full crew.
- **Voice cast:** surfaced on the **character detail page only** (1 REST call per
  character-open) — no per-character enrichment of the anime-page list in this cut.
- **Presentation:** new info lives behind a **"Details" disclosure** on the anime page
  (collapsed by default, mirroring the existing synopsis Show more/less toggle).

---

## Three data pieces

### Piece A — Metadata block (frontend-only + one small backend field)

Already-serialized fields rendered into a key/value block: **studio(s)**, **aired → released
dates**, **source** (`material_source`), **age rating**, **episode duration**, plus existing
year/type/episodes/status.

Backend delta (minimal):
- Add `ReleasedOn *time.Time` (`json:"released_on,omitempty"`, gorm indexed) to
  `domain.Anime`. GORM `AutoMigrate` adds the column on restart (additive, safe).
- Add `releasedOn { year month day }` to the Shikimori queries that back
  `GetAnimeByID` (typed struct + the raw-query siblings), and map it in the parser like
  `airedOn`.

Frontend: extend `ApiAnime` / `Anime` (`useAnime.ts`) + `transformAnime` with
`studios`, `releasedOn`, `materialSource`, `rating`, `episodeDuration` (several already flow
through the API — just untyped/undisplayed).

### Piece B — Staff/crew: one flat denormalized table (owner directive: "one table, role as a col, don't normalize")

New table **`anime_person_roles`** — no separate `Person` entity, no m2m join:

| column | source | notes |
|---|---|---|
| `id` | ours (uuid) | PK |
| `anime_id` | ours | FK → `animes.id`, indexed |
| `shikimori_person_id` | `person.id` | for dedup / future links |
| `name`, `name_ru`, `name_jp` | `person.{name, russian, japanese}` | inline (denormalized) |
| `poster_url` | `person.poster.originalUrl` | inline |
| **`role`** | one row per (person, role) | **scalar column** (EN canonical) |
| `role_ru` | `rolesRu` | RU label, free from Shikimori |
| `is_producer`, `is_mangaka` | `person` flags | cheap; lets FE badge |
| `position` | ordering | whitelist rank, then name |
| `created_at`, `updated_at` | ours | |

- **One row per (person, role)** so the UI can group by role; a person credited for two
  whitelisted roles appears twice — that is the intended flat shape.
- **Whitelist** (canonical EN → stored `role`; matched against `rolesEn`):
  `Director`, `Original Creator`, `Series Composition`, `Script`, `Music`,
  `Character Design`, `Art Director`, `Sound Director`, `Chief Animation Director`,
  `Producer`, `Executive Producer`, `Director of Photography`.
  Roles outside the whitelist are dropped at parse time (keeps ~15-25 rows/anime).

Backend slice (reuses the *characters flow*, not its normalized shape):
- **Parser:** `GetAnimeStaff(shikimoriID)` in `parser/shikimori/` — GraphQL
  `animes(ids){ personRoles { rolesEn rolesRu person{ id name russian japanese isProducer
  isMangaka poster{originalUrl} } } }`; flatten + whitelist-filter in Go.
- **Service:** `GetAnimeStaff(animeID)` — Redis → miss → Shikimori → bulk-replace in one tx →
  Postgres; on Shikimori error serve last-known-good from Postgres (same resilience as
  `GetAnimeCharacters`). Cache key `anime:staff:{animeID}`, TTL 6h (`TTLAnimeDetails`).
- **Handler + route:** `GET /api/anime/{animeId}/staff` (gateway already forwards
  `/api/anime/*` → catalog). Response: `[]AnimePersonRoleView`.

Frontend: `useStaff(animeId)` composable (mirrors `useCharacters`) → renders a role-grouped
table inside the Details disclosure. Lazy-loaded (IntersectionObserver) like the characters
rail — the disclosure fetches on first expand.

### Piece C — Voice cast: wired onto the existing character (owner directive: "just wire it to our existing characters")

- **Storage:** add `seyu` **inline on the `Character` record** — a denormalized JSONB column
  `Seyu datatypes.JSON` (or `[]CharacterSeyu` serialized) holding
  `{shikimori_id, name, name_ru, image_url, url}` per voice actor. **No separate seiyu
  table.**
- **Fetch:** the existing per-character path (`GetCharacterByID`, `service/character.go`) is
  extended to hit REST `/api/characters/{id}` (already the pattern used by
  `GetAnimeFranchise`) and capture `seyu[]`. This is the natural single-call site.
- **Surface:** `Character.vue` (route `/characters/:id`) renders a "Seiyū / Voice cast"
  section. The anime-page character *list* is unchanged in this cut (no N-call enrichment).
- Cache: rides the existing `character:{shikimoriID}` key, TTL 6h.

---

## Presentation — the "Details" disclosure

A new collapsible on `Anime.vue`, placed after the synopsis (`section-overview`) and before
the player (`section-episodes`), styled `glass-card p-4` like the synopsis body. Header row is
a button with `aria-expanded`; collapsed by default. Contents:

1. **Metadata** key/value grid (Piece A).
2. **Staff** table grouped by role (Piece B), lazy-fetched on first expand.

Voice cast (Piece C) is **not** in this disclosure — it lives with the characters, on the
character detail page.

i18n: add `anime.details.*` and `anime.roles.*` keys to **en/ja/ru** (parity gate). Staff
`role` display uses `role_ru` for RU, canonical EN for EN, and **EN fallback for JP** (no JP
labels from Shikimori). Character page adds `characters.seyu` label to all three locales.

---

## Non-goals (explicit)

- No character↔seiyu enrichment of the anime-page character rail (deferred; would be N REST
  calls/first-load).
- No JP-vs-dub separation of the voice cast (Shikimori doesn't flag language).
- No full-crew storage (Key Animation etc. dropped by the whitelist).
- No separate `Person` page/route (people are inline data, not linkable entities in this cut).
- Producers remain folded into `Studios` per Phase-12 Decision §A2 for the metadata block; the
  staff table's own `Producer`/`Executive Producer` rows come from `personRoles` and are
  independent of that.

---

## Metrics

- **UXΔ = +2 (Better)** — fills a real content gap (production credits + cast) users asked for;
  gated behind a disclosure so it doesn't crowd the hero.
- **CDI = 0.03 * 13** — Spread: catalog (parser/service/handler/domain/migration) + frontend
  (Anime.vue, Character.vue, composables, 3 locales). Shift: additive, one new table + one new
  route + one new column; no behavior change to existing paths. Effort_Fib 13.
- **MVQ = Griffin 85%/80%** — a composite of well-trodden parts (characters-flow clone, REST
  enrichment, disclosure UI); low slop risk, each piece has a direct in-repo analog.

---

## Files touched (anticipated)

**Backend (catalog):**
- `internal/domain/anime.go` — `+ReleasedOn`
- `internal/domain/person_role.go` *(new)* — `AnimePersonRole` + `AnimePersonRoleView`
- `internal/domain/character.go` — `+Seyu`
- `internal/parser/shikimori/client.go` — `+releasedOn`, staff query
- `internal/parser/shikimori/staff.go` *(new)* — `GetAnimeStaff` + whitelist
- `internal/parser/shikimori/characters.go` — seyu via REST on `GetCharacterByID`
- `internal/service/catalog.go` — staff service + AutoMigrate registration
- `internal/service/character.go` — persist/return seyu
- `internal/handler/catalog.go` + `internal/transport/router.go` — `GET …/{id}/staff`

**Frontend:**
- `composables/useAnime.ts` — types + `transformAnime` (metadata fields)
- `composables/useStaff.ts` *(new)* — staff fetch
- `api/client.ts` — `getStaff`, extend character fetch
- `types/character.ts` — `+seyu`
- `views/Anime.vue` — Details disclosure (metadata + staff)
- `views/Character.vue` — voice cast section
- `locales/{en,ru,ja}.json` — new keys (parity)
