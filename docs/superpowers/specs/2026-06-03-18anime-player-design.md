# 18anime (18+) Player — Design Spec

**Date:** 2026-06-03
**Status:** Approved (brainstorming) → pending implementation plan
**Topic:** Add `18anime.me` as a new, separate 18+ video provider/player alongside Hanime.

---

## 1. Purpose & Scope

Add a new **user-facing 18+ player surface, "18anime"**, sourced from `18anime.me`, selectable
alongside the existing Hanime player (the way RU content offers Kodik and AniLib as distinct
players). This is independently valuable now because the existing Hanime provider periodically
breaks on upstream outages (e.g. `search.htv-services.com` returning Cloudflare 522), leaving the
18+ section with no working source.

**In scope (MVP):**
- A self-contained catalog parser for `18anime.me`.
- Stream extraction from **two** embed mirrors: **mp4upload** and **turbovidhls**, with failover
  between them per episode.
- A new `Anime18Player.vue` frontend surface + provider sub-tab in `Anime.vue`.
- HLS-proxy allowlist additions for the resolved stream hosts.
- Feature flag `VITE_ANIME18_ENABLED` (default **off** during break-in).

**Out of scope (deferred):**
- Other embed mirrors on 18anime (`abyssplayer`/Hydrax, `bysesayeveum`, `hentai-hub.upns.online`).
  `abyssplayer` in particular is obfuscated/DRM-like and fragile — explicitly deferred.
- Folding 18anime into a shared 18+ failover surface with Hanime (chosen model is **separate
  players**, not a unified failover surface).
- Promoting embed extractors into a shared `libs/embeds` module (possible follow-up; not MVP).

## 2. Site Recon (facts established 2026-06-03)

- `18anime.me` is an **open** DataLife Engine site — **no CAPTCHA / bot-wall** (unlike
  `hentai-hub.net`, which is fully gated behind a slider/canvas/pattern CAPTCHA and was therefore
  rejected as a source).
- Clean episode URLs: `https://18anime.me/hentai/<id>-<romaji-slug>-episode-N.html`.
- Player: **Plyr**. The site does **not** host video; each episode page embeds an inline JSON list
  of third-party mirror "servers", each `{ "link": "<embed-url>", "quality": "FullHD" }`:
  - `mp4upload.com/embed-<id>.html` (classic packed-eval-JS file host)
  - `turbovidhls.com/t/<id>` (HLS embed)
  - `abyssplayer.com/<id>` (Hydrax/Abyss — deferred)
  - `bysesayeveum.com/e/<id>`, `hentai-hub.upns.online/#<id>` (obscure — deferred)
- Posters/images served from `cdn.hentaigo.me`.

This matches the project's existing **OurEnglish scraper** model: a source site that yields embed
links, plus extractors that turn embed URLs into playable streams.

> **⚠️ SUPERSEDED (2026-06-03, post-ship): 18anime moved into the scraper microservice as a
> separate adult provider group.** The original ship placed the parser in the **catalog** service
> (Approach A below). Per a follow-up request, the extraction was ported to
> `services/scraper/internal/providers/eighteenanime/` (implements `domain.Provider`) and
> registered into a **second, dedicated `Orchestrator`** (the "adult group") served on the
> `/anime18/{episodes,servers,stream}` route family. The EN (OurEnglish) orchestrator NEVER
> registers it, so the "no hentai in the EN failover chain" guarantee is now *structural* (two
> separate orchestrators) rather than "EN-untouched". Provider group is intrinsic
> (`config.GroupOf`, validated — a YAML typo cannot move it into EN). 18anime appears in
> `scraper-providers.yaml` + the Grafana provider-management dashboard; enable/disable there only
> affects the 18+ player. The catalog keeps its `/api/anime/{id}/anime18/*` routes +
> `Anime18Player.vue` unchanged but now **forwards** to the scraper; the catalog parser package was
> deleted. mp4upload/turbovid are exposed as selectable **servers** (turbovid now directly
> reachable). No automated golden probe for 18anime (no 18+ golden pool) — health is seeded.

## 3. Architecture & Data Flow

Mirrors the Hanime provider — a self-contained path through the **catalog** service (Approach A;
the working EN scraper microservice is left untouched, and there is zero risk of hentai leaking
into the EN failover orchestrator).

```
Anime.vue
  provider sub-tab "18anime"  →  videoProvider = 'anime18'
  → <Anime18Player>
      GET /api/anime/{uuid}/anime18/episodes
      GET /api/anime/{uuid}/anime18/stream?ep=<episode-slug>
        catalog → parser/eighteenanime:
          Search(title)        → 18anime slug   (romaji/EN title match, like hanime)
          ListEpisodes(slug)   → []Episode      (parse episode pages / listing)
          GetServers(ep)       → []embed        (inline-JSON mirror list)
          GetStream(ep)        → {url, quality} (try mp4upload, then turbovid; first success wins)
      → Plyr + hls.js (turbovid HLS) or native <video> (mp4upload MP4)
        stream fetched via /api/streaming/hls-proxy  (Referer injection + allowlist)
```

Entry path is identical to Hanime: the anime must already exist in our catalog (18+ entry); the
player resolves the external source by title at watch time. No catalog pre-population.

## 4. Backend (catalog, Approach A)

**Package & directory.** `services/catalog/internal/parser/eighteenanime/` — Go package
`eighteenanime`. (A Go package/identifier cannot begin with a digit, so `18anime` is not a legal
package name. The URL path segment, frontend provider key, and user-facing label remain
`anime18` / "18anime".)

**`client.go`** implements:
- `Search(title string) (*SearchHit, error)` — POST `index.php?do=search` (DataLife Engine; the
  live spike confirmed search is **POST** with form `do=search&subaction=search&story=<query>`,
  not GET). Each result anchor is an individual **episode** page
  (`/hentai/<id>-<slug>-episode-N.html`); fuzzy-match by romaji/English title against the slug
  (reuse the multi-title scoring lessons from existing scraper providers).
- `ListEpisodes(title string) ([]Episode, error)` — 18anime has **no series page**; every episode
  is its own page and the search-results page already lists all of a series' episodes as separate
  anchors. So enumeration = parse all hits from the search page, derive each hit's **base slug**
  (strip the trailing `-episode-N` or bare `-N` and the leading numeric id), keep the group whose
  base slug **exactly** equals the matched series' base (exact, not prefix — `jk-to-inkou-kyoushi-4`
  and `jk-to-inkou-kyoushi-4-feat-ero-giin-sensei` are distinct series), and parse the episode
  number from each. Returns episodes sorted ascending.
- `GetServers(episodeURL) ([]Mirror, error)` — fetch the episode page, extract the inline-JSON
  mirror array (`{"link":…,"quality":…}`); filter to supported hosts (mp4upload, turbovid).
- `GetStream(episodeRef) (*VideoSource, error)` — for each supported embed in priority order
  (mp4upload → turbovid), call its extractor; return the first success. If all fail, return a
  typed "source unavailable" error (NOT an empty success).

**Embed extractors** (co-located in the package for MVP):
- `embed_mp4upload.go` — fetch embed page, extract the direct `.mp4` URL from the jwplayer
  `player.src({type:"video/mp4",src:"…video.mp4"})` object literal (the live spike on 2026-06-03
  found the URL in **plaintext** inside the `src:` field — no `eval(p,a,c,k,e,d)` de-packing
  needed for the current bundle). Stream requires `Referer: https://www.mp4upload.com/` (403
  without). If a future bundle reverts to packed JS, add a de-pack fallback before the regex.
- `embed_turbovid.go` — fetch embed page, extract the `.m3u8` manifest URL (present literally in
  the jwplayer config; no deobfuscation needed). Master + nested variants need no Referer.

**Routes** (`internal/transport/router.go`, mirroring the Hanime block):
- `GET /{animeId}/anime18/episodes` → `catalogHandler.GetAnime18Episodes`
- `GET /{animeId}/anime18/stream`   → `catalogHandler.GetAnime18Stream`

**Handlers** (`internal/handler/catalog.go`): `GetAnime18Episodes`, `GetAnime18Stream`, mirroring
`GetHanimeEpisodes` / `GetHanimeStream`.

**Config.** No credentials required (18anime is open) — the client is always considered configured.

**Gateway.** `/api/anime/*` already proxies to catalog; no new gateway route needed. The stream
proxy continues via `/api/streaming/hls-proxy`.

## 5. Frontend

- **`frontend/web/src/components/player/Anime18Player.vue`** — cloned from `HanimePlayer.vue`.
  Uses hls.js for turbovid HLS sources and native `<video>` for mp4upload MP4, both routed through
  `/api/streaming/hls-proxy`. Quality/source dropdown reused from the Hanime pattern.
- **`frontend/web/src/api/client.ts`** — add `anime18Api` with `getEpisodes(animeId)` and
  `getStream(animeId, episodeSlug)`.
- **`frontend/web/src/views/Anime.vue`** — add a provider sub-tab/chip
  (`@click="videoProvider = 'anime18'"`, label "18anime") next to the Hanime chip, gated by
  `VITE_ANIME18_ENABLED`; add `<Anime18Player v-else-if="videoProvider === 'anime18'">` to the
  player dispatch chain. **Place the new branch correctly within the `v-if`/`v-else-if` chain**
  (no non-conditional element interleaved; independent components after the chain) per the Vue
  template rule.
- **i18n** — add a `player.anime18.*` sub-namespace to **both** `locales/en.json` and
  `locales/ru.json`; the locale-parity test must pass.

## 6. Proxy / Infrastructure

- **`libs/videoutils/proxy.go`** — add the **resolved stream hosts** (mp4upload's video CDN family
  and turbovid's HLS CDN family) to `HLSProxyAllowedDomainsWithProvenance`, each with
  Reason/Owner/Added provenance fields. These are the hosts the embeds point at, **not**
  `18anime.me` itself. The exact host families are confirmed by a spike during planning (signed/
  short-lived embed URLs mean they must be observed by driving the real extraction pipeline, not
  guessed).
- **Feature flag** `VITE_ANIME18_ENABLED` (frontend), default **off**, so the surface can be
  dark-shipped and enabled once verified — same pattern as Raw / OurEnglish.

## 7. Error Handling & Resilience

Carries forward the lesson from the current Hanime outage (a dead upstream produced a 30-second
hang and a silent empty `200`, surfacing to users as a generic "no episodes"):

- **Fail-fast.** The HTTP client carries an 8s per-request timeout, AND `resolveStream` wraps the
  whole multi-mirror loop in a single `context.WithTimeout` (~9s) so that two dead mirrors can't
  serialize into ~16s — the overall resolve stays under the catalog/frontend 10s budget. No
  30-second hangs on a dead mirror.
- **Explicit unavailability.** When every supported embed fails, the handler returns a clear
  "source temporarily unavailable" signal (typed error / explicit status), and the player shows a
  distinct message — never a silent empty success masquerading as "no content".
- **Embed failover.** Within a single episode, mp4upload ↔ turbovid failover so one dead mirror
  doesn't kill playback.

## 8. Testing

- **Go (no network).** Golden-HTML fixtures for: a 18anime episode page (mirror-list JSON), an
  mp4upload embed page (packed JS), and a turbovid embed page. Unit tests for `Search`,
  `ListEpisodes`, `GetServers`, and each extractor, plus the `GetStream` failover ordering.
  Follow the existing scraper-provider golden-fixture style; mock external APIs.
- **Frontend.** `Anime18Player.spec.ts` (≥5 Vitest assertions) + locale-parity test for the new
  `player.anime18.*` keys. In-browser smoke at desktop + mobile per the standing visual-change
  rule (DS-NF-06), since jsdom can't catch Tailwind cascade bugs.

## 9. Project Metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — adds a working 18+ source, especially valuable while Hanime is down.
- **CDI = 0.03 × 13** — low Spread×Shift (isolated new path, copy of the Hanime pattern);
  Effort_Fib = 13 (parser + 2 extractors + frontend surface + proxy allowlist).
- **MVQ = Griffin 80%/75%** — familiar pattern (Hanime/scraper provider) keeps match high, but
  embed-extractor fragility holds slop-resistance moderate.

## 10. Primary Risk

The **mp4upload / turbovid extractors** may be harder than expected (obfuscation changes,
signed-URL windows). This is confirmed only by a spike during planning. If both prove intractable
within reasonable effort, the MVP narrows to a single working embed; if neither works, the feature
is reassessed before further build. The extractors are the fragile core and should be spiked
**first** in the implementation plan.
