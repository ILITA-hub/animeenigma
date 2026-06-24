# anime365 Russian-Subtitle Provider — Design

**Date:** 2026-06-24
**Status:** Approved design, pending implementation plan
**Origin:** Feedback `2026-06-09T12-04-35_notebook_feedback` — *"Добавить субтитры для аниме из AniJoy"* — reported live against Witch Hat Atelier (*Tongari Boushi no Atelier*, MAL `51553`) ep 12, which has no RU subtitles today.

---

## 1. Problem

Our subtitle aggregator (`services/catalog/internal/service/subs_aggregator.go`) sources from exactly two providers:

- **Jimaku** — Japanese only, keyed by AniList ID.
- **OpenSubtitles** — multi-language, keyed by IMDb/TMDB.

Russian is already a first-class UI language (`FAST_LANGS`/`PRIMARY_LANGS` include `ru`; the handler defaults `langs` to `ja,en,ru`), but **no provider reliably yields Russian fansubs**, especially for current-season ongoings. OpenSubtitles carries little/no RU for new titles, and Jimaku is JP-only. Result: RU is a dead button for most anime.

## 2. Goal

Add **anime365 / smotret-anime** as a third subtitle provider that contributes Russian (`subRu`) tracks, slotting into the existing aggregator pattern with **no functional frontend changes**.

### Why anime365 (decided during brainstorming)

Verified end-to-end against the reported episode (Witch Hat Atelier ep 12):

- Public JSON API, **no API key, no auth, no rate-limit gate observed**.
- Indexed by **MAL ID** — which we already store as `domain.Anime.MALID` (`anime.go:44`, DB `mal_id`) — so resolution is exact, not fuzzy.
- For ep 12 it returned **2 RU subtitle tracks** ("Crunchyroll", "Sa4ko aka Kiyoso"); files download with no auth:
  - **ASS** (full styling): `GET https://smotret-anime.org/episodeTranslations/{transId}.ass` → `200`, `application/octet-stream`, ~152 KB.
  - **VTT** (pre-converted): `GET https://smotret-anime.org/translations/vtt/{transId}` → `200`, `text/vtt`, verified real Russian text.
- It **aggregates dozens of RU fansub groups** (including AniJoy's work where it exists), so it satisfies the feedback's intent far better than scraping the RKN-blocked `animejoy.ru` directly.

**Rejected alternatives:** AnimeJoy-direct (RKN-blocked, bespoke DLE scraping, single group, fragile); OpenSubtitles-only (already wired, won't fix new ongoings); Kage/fansubs.ru (no clean API).

## 3. Architecture

One new parser package + ~4 small edits to existing files. Mirrors the OpenSubtitles integration exactly.

```
catalog request → SubsAggregator.FetchAll  (parallel fan-out, now 3 providers)
                     ├── fetchJimaku        (existing, JP)
                     ├── fetchOpenSubtitles (existing, multi-lang)
                     └── fetchAnime365      (NEW, RU)  ── parser/anime365.Client
                                                            ├── SearchSeriesByMAL
                                                            ├── ListEpisodes
                                                            └── ListTranslations
SubtitleTrack.URL → /api/anime/{id}/subtitles/anime365/file/{transId}  (NEW proxy endpoint)
                     → SubsAggregator.ResolveAnime365File → Client.DownloadSubtitle (ASS→VTT) → cache 24h
```

### 3.1 New package: `services/catalog/internal/parser/anime365/client.go`

A thin HTTP client against `ANIME365_BASE_URL` (default `https://smotret-anime.org`).

```go
type Client struct { baseURL string; http *http.Client; enabled bool }

func New(baseURL string, enabled bool) *Client
func (c *Client) IsConfigured() bool                 // enabled (no key needed)

// Resolution
func (c *Client) SearchSeriesByMAL(ctx, malID string) (seriesID int, err error)
func (c *Client) ListEpisodes(ctx, seriesID int) ([]Episode, error)      // GET /api/episodes?seriesId=&limit=...
func (c *Client) ListTranslations(ctx, episodeID int) ([]Translation, error) // GET /api/episodes/{id}

// Delivery
func (c *Client) DownloadSubtitle(ctx, transID int) (body []byte, format string, err error) // ASS primary, VTT fallback

// Health (subprobe Pinger interface)
func (c *Client) Ping(ctx) (time.Duration, error)    // cheap GET /api/series?query=naruto&limit=1
```

- `Episode`: `{ ID int; EpisodeInt string; EpisodeType string; IsActive bool }`.
- `Translation`: `{ ID int; TypeKind string; TypeLang string; AuthorsSummary string }`.
- `SearchSeriesByMAL`: `GET /api/series?query=<title>&limit=20`, then select the result whose `myAnimeListId == malID`. (A direct `?myAnimeListId=` filter param was inconclusive in testing — implementer should confirm; the query+match path is the robust fallback and the spec's baseline.) The title used for the query is `anime.NameEN || anime.Name`.
- `DownloadSubtitle`: fetch `/episodeTranslations/{transId}.ass`; if non-200 **or** the body fails a minimal ASS sanity check (must contain `[Script Info]` and at least one `Dialogue:` line), fall back to `/translations/vtt/{transId}` and return `format="vtt"`. On ASS success return `format="ass"`.

### 3.2 Aggregator wiring: `subs_aggregator.go`

1. Struct field `anime365 *anime365.Client` + constructor param in `NewSubsAggregator`.
2. `resultsCh := make(chan providerResult, 2)` → `3`; `wg.Add(1)` + a third goroutine calling `fetchAnime365`.
3. New method `fetchAnime365(ctx, anime, episode, langs) ([]SubtitleTrack, error)`:
   - Guard: `if s.anime365 == nil || !s.anime365.IsConfigured() { return nil, errProviderUnconfigured }`.
   - MAL key: `mal := anime.MALID; if mal == "" { mal = anime.ShikimoriID }` (Shikimori IDs are MAL-aligned for the large majority of TV titles; documented heuristic fallback).
   - Resolve series → episodes → pick the episode where `EpisodeInt == strconv.Itoa(episode)` **and** `IsActive` **and** `EpisodeType != "preview"`; for `anime.Kind == "movie"` treat as episode 1.
   - Filter translations to `TypeKind == "sub" && TypeLang == "ru"`; emit one `SubtitleTrack` per match:
     ```go
     SubtitleTrack{
       URL:      fmt.Sprintf("/api/anime/%s/subtitles/anime365/file/%d", anime.ID, t.ID),
       Lang:     "ru",
       Label:    t.AuthorsSummary,        // e.g. "Crunchyroll", "Sa4ko aka Kiyoso"
       Format:   "ass",
       Provider: "anime365",
       Release:  t.AuthorsSummary,
     }
     ```
   - Empty (anime not on anime365 / no RU sub for this episode) → `return nil, nil` (not an error, not "down").
4. New method `ResolveAnime365File(ctx, transID int) ([]byte, string, error)` mirroring `ResolveOpenSubtitlesFile`: cache key `subsfile:anime365:<transID>`, 24h TTL, delegates to `Client.DownloadSubtitle`.
5. Update the `SubtitleTrack.Provider` comment to include `anime365`.

### 3.3 Handler + route

- `subtitles.go`: new `GetAnime365File` mirroring `GetOpenSubtitlesFile` — resolves `{transId}`, writes `text/plain; charset=utf-8`, `Cache-Control: public, max-age=86400`. Unconfigured/disabled → `503`; upstream failure → `500` via `httputil.Error`.
- `transport/router.go`: register `GET /subtitles/anime365/file/{transId}` alongside the existing `/subtitles/opensubtitles/file/{fileID}` route (same auth posture).

### 3.4 Health probe: `subprobe`

`*anime365.Client` satisfies the `Pinger` interface (`Ping(ctx) (time.Duration, error)`). Add it to the pingers map where Jimaku/OpenSubtitles are registered (in `cmd/catalog-api/main.go`), so `anime365` appears in the `provider_health` overlay and the `/subtitles/all` panel.

### 3.5 Config + DI

- `config.go`: new `Anime365Config{ BaseURL string; Enabled bool }`; `BaseURL = getEnv("ANIME365_BASE_URL", "https://smotret-anime.org")`, `Enabled = getEnvBool("ANIME365_ENABLED", true)`.
- `cmd/catalog-api/main.go`: construct `anime365.New(cfg.Anime365.BaseURL, cfg.Anime365.Enabled)`, pass into `NewSubsAggregator` and the subprobe pingers map.

### 3.6 Frontend (cosmetic only — functionally zero)

The RU tracks render automatically. Two small polish edits:

1. `BrowseSubsModal.vue` `PROVIDER_HUES`: add an `anime365` brand hue for its provider badge (use an exempt brand hue per DS rules — e.g. `teal`/`indigo`, not an off-palette shade).
2. `pickDefaultSubtitle.ts` provider rank map: add `anime365` (RU has no competing provider today, so ordering is only relevant when anime365 returns multiple RU tracks — rank them stably, e.g. prefer the official/Crunchyroll release).

## 4. Data flow (concrete, the reported episode)

1. Player opens Witch Hat Atelier (`mal_id=51553`) ep 12 → `GET /api/anime/{uuid}/subtitles/all?episode=12`.
2. `fetchAnime365`: `SearchSeriesByMAL("51553")` → series `28440`; `ListEpisodes(28440)` → ep 12 is anime365 episode `380283`; `ListTranslations(380283)` → 2 `subRu` → 2 tracks pointing at `/api/anime/{uuid}/subtitles/anime365/file/5825652` and `/.../5819457`.
3. Player picks RU → fetches the proxied file URL → `ResolveAnime365File(5819457)` → `DownloadSubtitle` → ASS bytes (cached 24h) → `SubtitleOverlay` renders via `ass-compiler`.

## 5. Error handling & resilience

- **Fail-soft:** any anime365 error → logged WARN, added to `ProvidersDown`, request still returns Jimaku/OpenSubtitles results (existing aggregator behavior — no new code path).
- **Not-found ≠ error:** anime absent or no RU sub for the episode → `nil, nil` (empty RU, no "down" flag).
- **Disabled:** `ANIME365_ENABLED=false` → `errProviderUnconfigured` → classified `unconfigured`, never `down`.
- **Caching:** assembled tracks ride the aggregator's existing 6h/60s cache; series-id mapping cached separately (`subs:anime365:series:<malid>`, 7d, stable); file bytes 24h.
- **Upstream RKN/block risk:** `smotret-anime.org` is reachable today; `ANIME365_BASE_URL` lets us swap to a mirror without a redeploy of logic.

## 6. Testing

- `parser/anime365/client_test.go` — handwritten `httptest.Server` fakes for series/episodes/translations/ASS-download; assert MAL match, episode/preview filtering, `subRu` filtering, ASS→VTT fallback (serve malformed ASS → expect VTT). No testify/mock.
- `subs_aggregator` test — inject a fake anime365 client; assert RU tracks merge, dedupe holds, and an anime365 error lands in `ProvidersDown` without dropping the other providers.
- `subprobe` — anime365 appears in the snapshot.
- Manual smoke: `curl /api/anime/{uuid}/subtitles/all?episode=12` returns an `ru` group with `provider:"anime365"`; the proxied file URL returns valid ASS.

## 7. Out of scope (YAGNI)

- AnimeJoy-direct scraping.
- Kage/fansubs.ru integration (old-title gap — revisit only if requested).
- RU subtitle search UI changes (the existing Browse modal already covers it).
- Anime365 subscription/login features (we only read public subtitle files).

## 8. Effort / impact (project conventions)

- **UXΔ = +3 (Better)** — turns the RU subtitle button from dead-on-arrival to populated for the large catalog anime365 covers; zero functional UI work.
- **CDI = 0.02 * 8** — one new isolated parser package + ~4 small edits to established files (aggregator fan-out, handler, router, main DI), all following the OpenSubtitles template; low spread, low shift.
- **MVQ = Griffin 88%/85%** — clean fit to an existing seam, low slop risk; the only soft spot is dependence on a third-party site's URL shape (mitigated by configurable base URL + fail-soft).
