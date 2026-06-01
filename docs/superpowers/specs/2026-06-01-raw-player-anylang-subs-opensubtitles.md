# Any-language subtitles for the Raw player via OpenSubtitles

**Date:** 2026-06-01
**Workstream:** raw-jp (subtitle aggregation, follow-on to Phase 02)
**Status:** Design approved, pending spec review

## Problem

The Raw player's "other subs" menu (`OtherSubsPanel.vue`) is architecturally
multi-language already — it calls `subtitlesApi.all()`, and the backend
`SubsAggregator` is designed to merge **Jimaku** (JP-only) with
**OpenSubtitles** (every other language). In practice the menu only ever shows
Japanese, for one reason:

- **OpenSubtitles is never queried.** `OPENSUBTITLES_API_KEY` is unset, so
  `opensubtitles.Client.IsConfigured()` returns false and the aggregator
  silently skips the provider. Jimaku (`lang: "ja"`) is all that remains.

Two further gaps mean a bare API key alone would not produce working subs:

1. **Download-link resolution.** The aggregator uses OpenSubtitles'
   `attributes.url` as the track URL (`subs_aggregator.go:241`). In the v1 API
   that field is the subtitle's HTML landing page, not the file. The real file
   requires `POST /download` with the numeric `file_id`, which returns a
   short-lived temp link **and spends one unit of a tight daily download quota**.
2. **Compose wiring.** `docker-compose.yml:423` passes only `JIMAKU_API_KEY` to
   the catalog service; `OPENSUBTITLES_API_KEY` is not forwarded.

## Goals

1. Light up OpenSubtitles so the "other subs" menu shows subtitles in **any
   language** OpenSubtitles has for the title.
2. Be **quota-mindful**: search spends nothing; only a user *selecting* a track
   spends one download; resolved files are cached so re-watches cost zero.
3. Turn the "other subs" modal into a **full-screen panel with filters by
   provider and by language**.

## Non-goals

- No change to the inline quick-dropdown (stays `['ja','en','ru']`); this work
  is scoped to the "other subs" panel.
- No change to the HLS proxy allowlist (the design avoids needing it — see below).
- No perfecting of episode mapping or subtitle text-encoding (see Limitations).

## Quota model (the spine of the design)

```
SEARCH  (cheap, 0 quota):
  OtherSubsPanel → subtitlesApi.all() → SubsAggregator → OpenSubtitles /subtitles
    → returns tracks. Each OpenSubtitles track.url is NOT the file —
      it is a catalog endpoint that encodes the numeric file_id.

SELECT  (spends 1 download, then cached 24h):
  user picks track
    → activeSubUrl = "/api/anime/{animeId}/subtitles/opensubtitles/file/{fileID}"
    → SubtitleOverlay fetches it directly (same-origin; no hls-proxy)
      → catalog handler:
          Redis hit?  → return cached bytes (0 quota)
          else        → client.Download(fileID): POST /download → temp link
                        → GET temp link → cache bytes 24h → return text
```

Because the **catalog server** fetches opensubtitles.com (never the browser),
the proxy allowlist is untouched, and temp-link expiry / per-result quota waste
are both avoided.

## Components

### Backend

1. **`opensubtitles.Client.Download(ctx, fileID int) ([]byte, string, error)`**
   — new method in `parser/opensubtitles/client.go`.
   - `POST {BaseURL}/download`, JSON body `{"file_id": fileID}`, headers
     `Api-Key`, `User-Agent`, `Content-Type: application/json`, `Accept`.
   - Parse `{ link string, file_name string, remaining int }`.
   - `remaining <= 0` or a "limit"/"quota" body → `ErrRateLimited`.
   - `401/403` → `ErrUnauthorized`.
   - `GET link` → return `(bytes, file_name, nil)`.

2. **Aggregator: stop using `attributes.url`** — `service/subs_aggregator.go`
   `fetchOpenSubtitles` (lines ~236-248). For each entry with `FileID != 0`,
   set `URL` to the relative resolve path
   `/api/anime/{animeID}/subtitles/opensubtitles/file/{FileID}`; skip entries
   with `FileID == 0`. Search continues to spend **zero** quota.

3. **Resolve handler + route**
   - `handler/subtitles.go`: `GetOpenSubtitlesFile(w, r)`.
     - Parse `fileID` from path; 400 on non-numeric.
     - Redis key `subsfile:opensubtitles:<fileID>`, TTL 24h. Cache hit → return
       bytes.
     - Else `aggregator`/client `Download`; cache bytes; return
       `Content-Type: text/plain; charset=utf-8`.
     - `ErrRateLimited` → `429` + clear JSON message; not configured /
       `ErrUnauthorized` → `503`.
   - `transport/router.go` (after line 144):
     `r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", subtitlesHandler.GetOpenSubtitlesFile)`.
   - The handler needs access to the OpenSubtitles client + Redis. Expose a
     `ResolveOpenSubtitlesFile(ctx, fileID)` method on `SubsAggregator`
     (it already holds `opensubs` + `cache`) and call it from the handler, to
     keep the handler thin and the quota/cache logic in one place.

4. **Config / deploy**
   - Add `OPENSUBTITLES_API_KEY: ${OPENSUBTITLES_API_KEY:-}` (and optionally
     `OPENSUBTITLES_USER_AGENT`) to the catalog service env in
     `docker/docker-compose.yml` (next to `JIMAKU_API_KEY`, line ~423).
   - Add `OPENSUBTITLES_API_KEY=<key>` to the gitignored `docker/.env`
     (never committed). The key was supplied by the user.

### Frontend

5. **`SubtitleOverlay.loadSubtitles`** (`SubtitleOverlay.vue:240-251`) — if `url`
   starts with `/` (same-origin), `fetch(url, …)` directly; otherwise keep
   wrapping in `/api/streaming/hls-proxy?url=…` (today's behavior, used for
   Jimaku/external tracks). One small branch; makes backend-served subs
   first-class.

6. **Full-screen "other subs" panel with filters** — `OtherSubsPanel.vue`.
   - `Modal size="full"` (the size already exists in `Modal.vue:95`).
   - **Provider filter:** segmented chips `All | Jimaku | OpenSubtitles`,
     default `All`. Derived from which providers appear in the result set
     (hide a chip when its provider returned nothing).
   - **Language filter:** chips (or dropdown) of the languages present in the
     result set, each with a count, plus `All`, default `All`. Reuses the
     existing `languageHeader(lang)` i18n labels.
   - Filtering is **purely client-side** over the existing `.all()` response —
     no extra requests, no extra quota.
   - Keep current per-track rendering: provider badge, label/release, format,
     select button, `providers_down` note. Show a per-filter empty state.
   - i18n: add `player.otherSubs.filter.*` keys (provider, language, all,
     count) to BOTH `en.json` and `ru.json`.

### Selection path (already works, no change)

`RawPlayer.onOtherSubSelected` already injects a synthetic choice for exotic
languages and sets `activeSubUrl = track.url`. With the new resolve-path URL,
`SubtitleOverlay` fetches it directly (step 5). No RawPlayer change needed.

## Testing

- **Backend**
  - `opensubtitles.Client.Download`: httptest mocks for success, quota
    (`remaining: 0`) → `ErrRateLimited`, 401 → `ErrUnauthorized`.
  - Resolve handler / `ResolveOpenSubtitlesFile`: cache-hit (0 calls), cache-miss
    (resolves + caches), quota → 429.
  - `fetchOpenSubtitles`: asserts track URLs are the resolve path and
    `FileID == 0` entries are dropped.
- **Frontend**
  - `SubtitleOverlay`: same-origin URL fetched directly vs external URL wrapped
    in hls-proxy.
  - `OtherSubsPanel.spec.ts` (new): provider filter, language filter, combined
    filter, counts, empty state.

## Limitations (acknowledged, out of scope)

- **Episode mapping** keeps the existing `season=1, episode=N` heuristic —
  imperfect for multi-cour / absolute-numbered series.
- **Encoding** — some OpenSubtitles files are non-UTF-8 (e.g. Windows-1251 for
  Russian) and may render garbled; transcoding can be added later if it bites.
- **Quota** is shared across the whole instance; the 24h cache softens it but a
  busy day can still exhaust the free tier, surfaced as a clear "limit reached"
  message (429), never a silent failure.

## Metrics

- **UXΔ = +3 (Better)** — turns a JP-only menu into genuine any-language
  subtitle access with discoverable provider/language filters.
- **CDI = 0.03 * 13** — Spread: 2 services (catalog, frontend) + compose/env;
  Shift: small (one new method, one endpoint, one client branch, one panel
  rework); Effort_Fib: 13.
- **MVQ = Griffin 85%/80%** — solid, composite feature riding existing
  aggregator bones; slop-resistant because the quota/cache discipline is
  explicit and the proxy allowlist is deliberately untouched.
