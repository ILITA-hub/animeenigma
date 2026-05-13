# Phase 9: Per-card progress + Sub/Dub indicators + Episode-granular row - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, three Tier-E items sharing the card-render surface)

<domain>
## Phase Boundary

Three audit items batched because they all touch the same card-render surface (`AnimeCardNew.vue`, RecItem in `Home.vue`, episode rows in Ongoing column):

- **UX-16** — Per-card progress badge: render "Серия N / Y" (or `1-N из Y+`) on cards for anime the logged-in user has in-progress. Surfaces across RecItem (Home trending row), AnimeCardNew (Browse + Search), and EpisodeCard (if used). Tier E #2.
- **UX-17** — "Latest episodes" / Ongoing row links to the specific new episode, not just anime detail. Currently the Ongoing column on Home links to `/anime/{id}`; should link to `/anime/{id}?episode={episodes_aired+1}` when `next_episode_at` is set. Phase 8 already wired the `?episode=N` reader in `Anime.vue`, so this is a one-line URL change. Tier E #16.
- **UX-18** — Sub/Dub indicator badge on cards: small badge ("SUB" or "DUB") rendered on cards where the underlying anime has dubbed translation data available. Sourced from existing Kodik translation list — a new boolean `has_dub` on the anime record (or a derived field on the bulk-progress endpoint).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**UX-16 — Per-card progress:**
- Backend: new endpoint `GET /api/users/anime-progress?ids=a,b,c` returning `{ [animeId]: { latest_episode: int, episodes_count: int, episodes_aired: int, completed: bool, dropped: bool } }`. Limit query to 50 IDs (max card grid per page). Single SQL query with `WHERE user_id = ? AND anime_id IN (?)` + GROUP BY anime_id, MAX(episode_number) FILTER (WHERE completed=true). Join `animes` for `episodes_count` + `episodes_aired`. JWT-protected.
- Frontend composable: `useAnimeProgress(ids: Ref<string[]>)` — debounced fetch on `ids` change. Returns `{ progressMap: Ref<Map<string, ProgressEntry>>, loading, error }`. Skip fetch when not authenticated.
- Card prop: `AnimeCardNew.vue` and `RecItem` template (in Home.vue) accept optional `progressEntry?: ProgressEntry`. When set, render a small badge in the bottom-left corner showing `"Серия {latest_episode+1}"` (or i18n equivalent) for in-progress, `"{latest_episode} / {episodes_count}"` for completed-but-more-aired, hidden otherwise.
- Badge styling: `bg-purple-500/80 text-white text-[10px] font-medium px-1.5 py-0.5 rounded`. Distinct from the watchlist-status badge (cyan/emerald/amber/red).

**UX-17 — Episode-granular link in Ongoing row:**
- One-line change in `Home.vue:155` (the Ongoing column `<router-link>`): when `anime.next_episode_at && anime.episodes_aired`, replace `:to="`/anime/${anime.id}`"` with `:to="`/anime/${anime.id}?episode=${anime.episodes_aired + 1}`"`. Otherwise keep the bare `/anime/{id}` route.

**UX-18 — Sub/Dub indicator (MVP scope):**
- Backend: add `has_dub bool` field to the `Anime` model (GORM auto-migrate adds the column). Default false. Populate via the existing Kodik parser at write-time: when ingesting search results, if ANY translation in the Kodik response has `type=="voice"` (Kodik's dub indicator), set `has_dub=true`. Backfill is NOT included in this phase — the field starts false for existing rows and populates lazily when search re-touches the row.
- Frontend: `AnimeCardNew.vue` renders a small "DUB" badge in the top-right (paired with quality badge) when `anime.hasDub === true`. No badge when undefined/false. Tailwind: `bg-amber-500/90 text-white text-[10px] font-bold px-1.5 py-0.5 rounded`.
- i18n: keys `card.subBadge` (unused for MVP — no SUB indicator since most anime have subs by default) and `card.dubBadge` (rendered text "DUB" / "DUB" / "DUB" — single 3-char string per locale; English-style abbreviation appropriate cross-locale).

**Locked from ROADMAP:**
- Three items batched because they all touch card-render surfaces. No card-rendering pattern duplicated.
- "SUB" indicator deferred because every anime has subs (we proxy HLS with embedded subs); a SUB badge is meaningless. Only DUB is informative.
- Backfill of `has_dub` for existing rows deferred — search-driven re-ingest naturally backfills over time. If urgent, a one-off Goroutine script can backfill in a follow-up.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/domain/anime.go` — `Anime` model; new `HasDub bool` field with `gorm:"default:false;index"`.
- `services/catalog/internal/parser/kodik/client.go` — Kodik parser; translation entries already include `type` ("voice" or "subtitle"). Set `has_dub=true` when at least one "voice" translation exists.
- `services/player/internal/repo/progress.go` — pattern for the new bulk-progress query.
- `frontend/web/src/components/anime/AnimeCardNew.vue` — card with poster + badges. Add progress badge in bottom-left, DUB badge near top-right.
- `frontend/web/src/views/Home.vue` line 50 (RecItem in trending row), line 155 (Ongoing column), line 240 (Top row), line 320 (Announcements) — all use similar `<router-link>` patterns. Only UX-17 modifies line 155.

### Established Patterns

- Bulk endpoint pattern: comma-separated `ids` query param, server-side validation + parsing. See `services/catalog/internal/handler/anime.go` (if a similar endpoint exists — to verify in plan-phase).
- Phase 8 `?episode={N}` reader in Anime.vue is already live, so UX-17 just needs to emit the right URL.

### Integration Points

- Catalog service: `has_dub` lives in the catalog DB; the bulk-progress endpoint is in the player service which JOINs `animes`. Both services share the same postgres DB and `animes` table. Verified during plan.
- No new tables — adds one column via GORM auto-migrate.

</code_context>

<specifics>
## Specific Ideas

- The progress badge should be VISUALLY DISTINCT from the watchlist-status badge so users can read both at a glance. Watchlist = cyan/emerald/amber/red; progress = purple. Stack them vertically if both render (`gap-1 flex flex-col` in the bottom-left container).
- The bulk-progress endpoint must NOT 401 on anonymous users — it's only called from authenticated paths, but for defense return empty map for anonymous instead of 401 (smoother frontend gating). Actually NO — JWT-required is simpler; the composable already skips fetch when not authenticated.
- DUB badge text: locked to "DUB" (English abbreviation) across locales. Anime fans recognize this universally; localizing it would create ambiguity ("ДУБ" / "吹替" — three different identifiers for the same concept).

</specifics>

<deferred>
## Deferred Ideas

- Backfill existing `animes` rows with `has_dub` data — follow-up script via scheduler service.
- SUB indicator badge — deferred (every anime has subs, badge meaningless).
- Per-episode dub variant detection (e.g. only S1 has dub, S2 doesn't) — beyond v0.1 scope.
- AnimeLib / HiAnime / Consumet dub detection — Kodik is the canonical RU source and most reliable dub signal. EN players use their own translation conventions that don't map cleanly to a dub bool.

</deferred>
