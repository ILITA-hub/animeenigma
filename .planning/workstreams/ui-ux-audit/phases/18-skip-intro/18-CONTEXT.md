# Phase 18: Skip-Intro detection (Griffin) - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, Griffin-level new feature — aniskip integration)

<domain>
## Phase Boundary

Surface a "Пропустить опенинг" CTA on HiAnime + Consumet players using aniskip.com timestamps. Closes UX-34 / Tier E #13.

**Scope:**
- New backend endpoint: `GET /api/skip-times/:malId/:episode` proxying https://api.aniskip.com/v2/skip-times — proxy required for CORS + rate-limit hygiene.
- Frontend composable: `useSkipTimes(malId, episode)` returning `{ opening, ending }` with `{ start, end }` per segment.
- HiAnimePlayer.vue: render "Пропустить опенинг" button overlay when `currentTime` is within the OP window. Click → seek to OP end.
- ConsumetPlayer.vue: same.
- Backend caching: 7-day TTL for skip-times responses (timestamps don't change).

**Coordinate (don't duplicate) with root milestone Phase 16 (single-player abstraction):**
- Per ROADMAP.md, root milestone has a Phase 16 that consolidates the 4 players. We don't have visibility into that work in this workstream. Pragmatic approach: implement the feature directly in HiAnimePlayer.vue + ConsumetPlayer.vue. If the single-player abstraction lands later, the skip-intro logic moves with the consolidated player code. Acceptable duplication for v0.1.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**Backend:**
- New endpoint in catalog service: `GET /skip-times/:malId/:episode` (catalog has access to anime metadata; player service has user/playback state — skip-times are anime metadata).
- Implementation:
  ```go
  func (h *SkipTimesHandler) Get(w, r) {
    malID := chi.URLParam(r, "malId")
    ep := chi.URLParam(r, "episode")
    cached := cache.Get(...)
    if cached != nil { return cached }
    resp := httpGet("https://api.aniskip.com/v2/skip-times/"+malID+"/"+ep+"?types=op,ed")
    cache.Set(..., 7*24*time.Hour)
    json.Encode(w, resp)
  }
  ```
- Gateway proxy: `/api/skip-times/*` → catalog.
- Response shape (passthrough from aniskip): `{ found: bool, results: [{ skip_type: "op"|"ed", start_time: float, end_time: float, ... }] }`.

**Frontend:**
- Composable `useSkipTimes(malId, episode)`: fetches on mount/change, returns reactive `{ opening: { start, end } | null, ending: { start, end } | null, loading, error }`.
- Player overlay: when `currentTime >= opening.start && currentTime < opening.end - 1`, render a small floating button bottom-right of the player:
  ```vue
  <button
    v-if="showSkipIntro"
    @click="seekTo(opening.end)"
    class="absolute bottom-20 right-4 px-4 py-2 bg-cyan-500 text-white rounded-lg shadow-lg z-10"
  >
    {{ $t('player.skipIntro') }}
  </button>
  ```
- Same for ending CTA (lower priority — users often want to see the credits).
- Hide button after 1 second past start to give users a chance to dismiss it consciously (optional polish).

**MAL ID source:**
- Anime model has `MALID` field. The player receives `anime` prop in HiAnime/Consumet players. Pass `mal_id` through.
- If MAL ID is missing for an anime, the skip-intro feature gracefully degrades (no CTA shown).

**i18n:**
- `player.skipIntro` (button label):
  - EN "Skip Intro" / RU "Пропустить опенинг" / JA "オープニングをスキップ"
- `player.skipOutro`:
  - EN "Skip Ending" / RU "Пропустить эндинг" / JA "エンディングをスキップ"
- (2 keys × 3 locales = 6 entries)

### Locked from ROADMAP

- Coordinate with root P16 — don't try to consolidate players here. Ship duplicated logic in 2 player files; the consolidation phase merges it.
- Aniskip is a 3rd-party free API — no auth needed, but rate-limit-friendly via caching.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `services/catalog/internal/handler/anime.go` — handler pattern.
- Existing cache layer (`libs/cache`) — used for anime details + search results. Add new key prefix `skip-times:`.
- `services/gateway/internal/transport/router.go` — proxy registration pattern.
- HiAnimePlayer.vue has `currentTime` ref + `seekTo` capability via `vjsPlayer` (Video.js) or `nativeVideoRef.value.currentTime = ...`.
- ConsumetPlayer.vue uses same architecture — copy approach.

### Established Patterns

- Backend proxy of 3rd-party API with cache: see existing kodik/animelib/hianime/consumet parsers — same shape.
- Frontend composable pattern from Phase 8/9.

### Integration Points

- New gateway route: `/api/skip-times/*` → catalog (public, no auth required).
- No DB migration — pure pass-through with cache.

</code_context>

<specifics>
## Specific Ideas

- Aniskip API is community-maintained; some anime don't have crowdsourced timestamps. The composable returns null fields gracefully; CTA simply doesn't render.
- Cache key: `skip-times:{malId}:{episode}`. 7-day TTL.
- The button is positioned bottom-right of the player to mirror standard Crunchyroll/Funimation conventions.

</specifics>

<deferred>
## Deferred Ideas

- Auto-skip (vs. button) toggle — defer; CTA is the v0.1 pattern.
- Skip Ending button — implement in same PR but lower visual priority.
- AnimeLib + Kodik integration — iframe-based, no `currentTime` access. Not addressable in v0.1.
- Settings to enable/disable skip-intro feature — defer to Profile settings.

</deferred>
