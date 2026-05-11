# v3.0 Universal Anime Scraper — Requirements

**Milestone:** v3.0
**Status:** Draft — pending user confirmation
**Last updated:** 2026-05-11

## Scope summary

Build a new self-hosted Go scraping subsystem at `services/catalog/internal/parser/scraper/` that **replaces** the dead `aniwatch` (HiAnime) and broken `consumet-api` (Consumet) provider paths. The old per-provider frontend identities (`HiAnimePlayer.vue`, `ConsumetPlayer.vue`, `/api/anime/{id}/hianime/*`, `/api/anime/{id}/consumet/*`) **die in this milestone** — they are not preserved or repointed. The replacement is a single unified English-source player surface backed by an orchestrator that fans out to three alive providers.

**Providers in v3.0:** AnimePahe (independent lineage), 9anime (WordPress/Madara lineage), AnimeKai (behind feature flag — R&D risk on in-house token generator). Kodik (RU iframe) and AnimeLib (RU MP4) parsers are **untouched**. Anitaku/Gogoanime and AniZone are **out of v3.0 scope**.

**Universal layer empirically validated 2026-05-11:**

| Layer | Sharable? | Reason |
|---|---|---|
| HTML scraping selectors | ❌ Per-site | Site probes showed AnimePahe / 9anime / AnimeKai use unrelated markup (custom / WordPress-Madara / custom) — they are **not** forks of a common codebase. |
| HTTP infrastructure (retry, rate limit, cookie jar, headers, timeouts) | ✅ 100% shared | One `BaseHTTPClient` in Go used by all providers. |
| Stream-URL **embed extraction** (megacloud / kwik / vidstreaming / streamsb / mp4upload) | ✅ Shared per embed type | Each `EmbedExtractor` handles one embed family; providers delegate by URL match. Where two providers embed the same player, the extractor is shared automatically. |
| Provider interface contract | ✅ 100% shared | One Go interface, one orchestrator, sentinel errors. |

This is the real universal abstraction — not "Zoro-family HTML parser" (which doesn't exist), but an `EmbedExtractor` registry + `BaseHTTPClient` + `Provider` interface. Adding a new provider in the future is "implement HTML scraping for one site + register which embeds it uses."

---

## Requirements (numbered SCRAPER-*)

### Foundation (Phase A)

- [ ] **SCRAPER-FOUND-01**: A `Provider` Go interface in `services/catalog/internal/parser/scraper/provider.go` exposes `Name`, `FindID`, `ListEpisodes`, `ListServers`, `GetStream`, `HealthCheck`. New providers plug in without modifying the orchestrator or HTTP handlers.
- [ ] **SCRAPER-FOUND-02**: Three sentinel errors (`ErrNotFound`, `ErrProviderDown`, `ErrExtractFailed`) drive orchestrator failover semantics. Returning `[]Episode{}, nil` from a provider on selector drift is forbidden — providers must distinguish "real empty" from "scrape broke" and emit the appropriate sentinel.
- [ ] **SCRAPER-FOUND-03**: The Stream DTO returned by the scraper has **no `iframe_url` field at the type level**. EN providers serve HLS m3u8 + optional tracks or HTTP 404 with `{reason, tried[]}`. Silent cross-tier fallback to a Kodik iframe is structurally impossible.
- [ ] **SCRAPER-FOUND-04**: A service-layer `Orchestrator` (`services/catalog/internal/parser/scraper/orchestrator.go`) does sequential per-anime provider failover. Per-provider clients are unaware of each other (no in-client `FallbackProviders` like the dead Consumet parser).
- [ ] **SCRAPER-FOUND-05**: An `EmbedExtractor` interface + registry. Each registered extractor declares which URL hosts it handles (e.g. `megacloud.*`, `kwik.cx`, `vidstreaming.*`, `streamsb.*`, `mp4upload.*`). The orchestrator (or provider client, post-`ListServers`) routes each embed URL to the matching extractor. Adding a new embed family is one registry entry, not changes to provider clients.
- [ ] **SCRAPER-FOUND-06**: A `BaseHTTPClient` Go type encapsulates `hashicorp/go-retryablehttp` + per-host `golang.org/x/time/rate.Limiter` + scoped `net/http/cookiejar` + standard browser headers (UA, Accept-Language, Accept-Encoding). Every provider client uses this base — no ad-hoc `http.Client` per provider. Default budget: 1 RPS per provider, weight=2 per host, 10 s hard timeout per request.
- [ ] **SCRAPER-FOUND-07**: A golden-file test harness (`testdata/<provider>/<page>.html` + `sebdah/goldie/v2`) snapshots upstream HTML so unit tests stay deterministic when upstreams are down or rate-limited. `make capture-goldens` recipe documented for refreshing fixtures.
- [ ] **SCRAPER-FOUND-08**: A `MegacloudClient` Go HTTP wrapper calls the existing `docker/megacloud-extractor/` Node sidecar over HTTP. The sidecar is registered as the `megacloud` `EmbedExtractor` in the registry. No embed decryption is reimplemented in Go.
- [ ] **SCRAPER-FOUND-09**: CI lint rejects `go.mod` additions of `chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, and `flaresolverr` packages. Anti-bot scope creep is gated at the build.
- [ ] **SCRAPER-FOUND-10**: New gateway-routed HTTP endpoints under `/api/anime/{animeId}/scraper/*` are registered (returning HTTP 503 `not-yet-implemented` until Phase B):
  - `GET /api/anime/{animeId}/scraper/episodes?prefer={provider}`
  - `GET /api/anime/{animeId}/scraper/servers?episode={epId}&prefer={provider}`
  - `GET /api/anime/{animeId}/scraper/stream?episode={epId}&server={srvId}&category={sub|dub}&prefer={provider}`
  - `GET /api/anime/{animeId}/scraper/health`

### First Provider — AnimePahe (Phase B)

- [ ] **SCRAPER-PAHE-01**: Given a Shikimori/MAL ID, the AnimePahe client returns the matching AnimePahe session UUID via `malsync.moe` lookup (24 h cache) with a fuzzy-title fallback.
- [ ] **SCRAPER-PAHE-02**: `ListEpisodes` returns the complete episode list with `episode_number`, `id`, `title`, and `is_filler` where the upstream exposes it. Cached for 6 hours.
- [ ] **SCRAPER-PAHE-03**: `GetStream` returns HLS m3u8 URLs at the available qualities (480p / 720p / 1080p) via the `kwik.cx` packer using `dop251/goja` for the embedded JS unpacking. The Kwik unpacker registers as the `kwik` `EmbedExtractor`. Stream URLs cached for ≤ min(parsed expiry − 30 s, 5 min).
- [ ] **SCRAPER-PAHE-04**: DDoS-Guard cookies are handled via the per-provider `cookiejar` scoped by `golang.org/x/net/publicsuffix`. No headless browser.
- [ ] **SCRAPER-PAHE-05**: AnimePahe CDN hostnames (`kwik.cx`, `owocdn.top`, `uwucdn.top`, and any others discovered during implementation) are appended to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`.

### New unified English player frontend (Phase B)

- [ ] **SCRAPER-UI-01**: A new `frontend/web/src/components/player/EnglishPlayer.vue` component replaces both `HiAnimePlayer.vue` and `ConsumetPlayer.vue`. Same Video.js / HLS.js engine + the existing `SubtitleOverlay.vue` for Jimaku JP subs.
- [ ] **SCRAPER-UI-02**: The anime detail page surfaces **one** "English" player tab (replacing the previous two tabs labelled "HiAnime" and "Consumet"). Provider selection lives **inside** the player UI — a small "Source: AnimePahe / 9anime / AnimeKai" dropdown so users can override the orchestrator's default. User selection persists per anime via the existing watch-preference store.
- [ ] **SCRAPER-UI-03**: A new `frontend/web/src/api/client.ts::scraperApi` exposes `getEpisodes`, `getServers`, `getStream`, `getHealth` against the `/api/anime/{id}/scraper/*` endpoints. `hianimeApi` and `consumetApi` are **not** repointed — they will be deleted in Phase F.
- [ ] **SCRAPER-UI-04**: The two old player components (`HiAnimePlayer.vue`, `ConsumetPlayer.vue`) are **kept temporarily** during the soak (Phase B-E) so users have a working fallback if the new player misbehaves. Both are removed in Phase F. New users see only the English tab; the old tabs stay reachable via a dev flag (`?legacy=1`) for debug.

### Observability (Phase C)

- [ ] **SCRAPER-OBS-01**: A background liveness probe runs every 15 min ± 20 % jitter, exercising the full pipeline (search → episodes → servers → stream → first segment) against a rotating 5-10 anime golden pool **per provider**.
- [ ] **SCRAPER-OBS-02**: A Prometheus gauge family `provider_health_up{provider, stage}` reports per-stage health for 5 stages: `search`, `episodes`, `servers`, `stream`, `stream_segment`. A stage flips to 0 after 3 consecutive failures within 15 min.
- [ ] **SCRAPER-OBS-03**: The orchestrator skips any provider whose health gauge reads 0 in the last 60 s (in-memory health cache). Skipped providers re-enter rotation when the probe flips them back to 1.
- [ ] **SCRAPER-OBS-04**: A Grafana dashboard panel + alert fires when any `provider_health_up{stage="stream_segment"}` reads 0 for 15 min. Alerts target the existing Telegram admin chat (`TELEGRAM_ADMIN_CHAT_ID`).
- [ ] **SCRAPER-OBS-05**: `GET /api/admin/scraper/health` exposes the current per-provider / per-stage health snapshot + last successful timestamps for admin debugging.

### Second Provider — 9anime (Phase D)

- [ ] **SCRAPER-9ANI-01**: Given a Shikimori/MAL ID, the 9anime client resolves the matching 9anime slug via `malsync.moe` lookup with the same caching + fuzzy fallback as AnimePahe.
- [ ] **SCRAPER-9ANI-02**: `ListEpisodes` returns the full episode list scraped from 9anime's WordPress/Madara-themed markup (`bsx`, `bixbox`, `bs`, `bt` class family). Sub/dub split surfaced where present. Cached 6 hours.
- [ ] **SCRAPER-9ANI-03**: `ListServers` enumerates 9anime's embed hosts per episode. The set of embed hosts 9anime uses (`mp4upload`, `streamsb`, `streamtape`, megacloud variants, etc.) is discovered during implementation and **each is registered as an `EmbedExtractor`** so future providers using the same hosts reuse the extractor.
- [ ] **SCRAPER-9ANI-04**: `GetStream` resolves an embed URL via `ListServers`, then dispatches to the matching `EmbedExtractor`. No embed extraction logic lives inside the 9anime client itself — only HTML scraping + URL extraction.
- [ ] **SCRAPER-9ANI-05**: 9anime CDN hostnames (whatever `mp4upload` / `streamsb` / `streamtape` resolve to today, plus 9anime's own static asset hosts) are appended to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`.
- [ ] **SCRAPER-9ANI-06**: The orchestrator's sequential failover ordering AnimePahe → 9anime is verified end-to-end: forcing AnimePahe's health gauge to 0 produces a playable stream from 9anime; `parser_fallback_total{from="animepahe",to="9anime"}` increments.

### Third Provider — AnimeKai, gated (Phase E)

- [ ] **SCRAPER-KAI-01**: Given a Shikimori/MAL ID, the AnimeKai client resolves the matching AnimeKai slug via `malsync.moe`.
- [ ] **SCRAPER-KAI-02**: `ListEpisodes` returns the full episode list scraped from AnimeKai's custom markup (`aitem-wrapper`, `alist-group`, `azlist` class family). Sub/dub split surfaced.
- [ ] **SCRAPER-KAI-03**: `ListServers` enumerates AnimeKai's embed hosts. AnimeKai is known to use MegaUp/megacloud-variant embeds; these route to the existing `megacloud` `EmbedExtractor` (extended if necessary).
- [ ] **SCRAPER-KAI-04**: The AnimeKai MegaUp-embed decryption + auth-token generation runs **inside our own `docker/megacloud-extractor/` sidecar** via a new endpoint (e.g. `/animekai-token`). **No call to `enc-dec.app` or any other external decryption service is performed at any point in the AnimeKai pipeline** — the contract change of `enc-dec.app` is what killed Consumet; v3.0 will not reintroduce that single point of failure.
- [ ] **SCRAPER-KAI-05**: AnimeKai ships behind a feature flag (`SCRAPER_ANIMEKAI_ENABLED`, default off in production for ≥ 7 days after Phase E ships, then default on). The flag is read at orchestrator startup and toggleable without rebuild via `docker compose restart catalog`.
- [ ] **SCRAPER-KAI-06**: If the in-house token-generator R&D doesn't converge during Phase E (extractor returns errors against the live `animekai.to` embed), AnimeKai ships with the flag default-off and `SCRAPER-KAI-01..04` stay open as v3.1 carryover. The rest of v3.0 ships regardless.
- [ ] **SCRAPER-KAI-07**: The orchestrator's sequential failover ordering AnimePahe → 9anime → AnimeKai is verified end-to-end with the flag on: forcing both AnimePahe and 9anime down still produces a playable stream from AnimeKai.

### Cutover — delete dead code (Phase F)

- [ ] **SCRAPER-CUT-01**: After ≥ 7 days of clean production traffic on the new EnglishPlayer (per-provider error rate ≤ 5 %, no Telegram alerts, no user-reported player breakage), the following Go code is deleted in a single PR: `services/catalog/internal/parser/hianime/`, `services/catalog/internal/parser/consumet/`, the seven HiAnime + Consumet handler funcs in `services/catalog/internal/handler/catalog.go`, the six old routes in `services/catalog/internal/transport/router.go` (`/api/anime/{id}/hianime/*`, `/api/anime/{id}/consumet/*`).
- [ ] **SCRAPER-CUT-02**: `services/catalog/internal/config/` removes `AniwatchAPIURL` and `ConsumetAPIURL`. The catalog service no longer accepts or requires those env vars.
- [ ] **SCRAPER-CUT-03**: The `aniwatch` and `consumet` service blocks are removed from `docker/docker-compose.yml`. `docker compose ps` after redeploy shows neither container.
- [ ] **SCRAPER-CUT-04**: `docker/megacloud-extractor/patch-aniwatch.sh` is deleted (the Node string-substitution patch into `node_modules/aniwatch/dist/index.js` no longer has a target). The `megacloud-extractor` container entrypoint reverts to a plain `node server.js`.
- [ ] **SCRAPER-CUT-05**: Frontend deletes: `frontend/web/src/components/player/HiAnimePlayer.vue`, `frontend/web/src/components/player/ConsumetPlayer.vue`, the `hianimeApi` + `consumetApi` exports in `frontend/web/src/api/client.ts`, and the legacy `?legacy=1` flag from `SCRAPER-UI-04`. All references in `Anime.vue` / `views/Anime.vue` switch to the single `EnglishPlayer.vue` tab.
- [ ] **SCRAPER-CUT-06**: Redis cache keys from the dead namespaces are busted (`search:hianime:*`, `search:consumet:*`, `stream:hianime:*`, `stream:consumet:*`, `episodes:hianime:*`, `episodes:consumet:*`) via a one-shot script committed alongside the cutover PR.
- [ ] **SCRAPER-CUT-07**: Translation keys for "HiAnime" / "Consumet" labels are removed from `frontend/web/src/locales/{ru,en,ja}.json`. The "English" tab label is the only EN-source string in the locales.

### Cross-cutting non-functional

- [ ] **SCRAPER-NF-01**: Every upstream HTTP call has a hard 10 s timeout. No call hangs indefinitely.
- [ ] **SCRAPER-NF-02**: Cache TTLs match the data freshness contract: 24 h for malsync ID lookups, 6 h for episode lists, 15 min for search results, **≤ min(parsed expiry − 30 s, 5 min)** for stream URLs.
- [ ] **SCRAPER-NF-03**: `hashicorp/go-retryablehttp` handles 429 / 5xx with exponential backoff (1 s → 2 s → 4 s → 8 s) and a 5-minute circuit-break per host after repeated failures. Hand-rolled retry loops from the old parsers are not ported.
- [ ] **SCRAPER-NF-04**: `parser_requests_total`, `parser_request_duration_seconds`, `parser_fallback_total{from,to}`, and `parser_zero_match_total{provider,selector}` Prometheus metrics emit for the scraper using the existing `libs/metrics/parser.go` patterns. Per-provider breakdown labelled `{provider}`.
- [ ] **SCRAPER-NF-05**: `ReportButton` from existing players emits a `provider:<name>` field plus the active orchestrator provider chain (`tried: [animepahe, 9anime]`) so user-reported bugs are sourceable to a specific provider in the report payload.

---

## Future Requirements (deferred to v3.1+)

- **AnimeKai full enablement** if Phase E's R&D doesn't converge — token-generator implementation work + flag flip.
- **DIFF-04** fuzzy title fallback against AniList when malsync.moe returns no match (only if v3.0 ships and the empirical miss rate ≥ 5 %).
- **DIFF-06** `/api/admin/scraper/diag/:shikimoriId` admin debug endpoint that walks the full pipeline for one ID and dumps every intermediate response.
- **Anitaku/Gogoanime as fourth provider** — domain volatility (5+ rotations in 18 months) means maintenance cost is high; coverage overlap with the v3.0 trio is already high. Pull in only if a documented user-coverage gap appears.
- **In-house port of `megacloud-extractor` to pure Go** — only if maintaining the Node sidecar becomes burdensome.

---

## Out of Scope

| Item | Reason |
|---|---|
| Preserving HiAnime + Consumet as visible frontend identities | The upstreams died; the player tabs die with them. Clean break per `feedback_replace_dont_preserve.md`. |
| AniZone (`anizone.to`) provider | No public reference implementation in 2026; Cloudflare 403'd both research probes. |
| Russian providers (Kodik, AnimeLib) | Separate parsers, untouched. Kodik = iframe-only by design; AnimeLib = direct MP4 (Kodik fallback was disabled in commit `9347143`). |
| Headless browsers / Playwright / chromedp | Live triage shows plain HTTPS reaches all target providers. Adding headless tooling for a 10-user self-hosted deployment is an anti-pattern (PITFALLS §11). |
| TLS fingerprint spoofing (utls, tls-client) | No JA3 blocks observed. |
| Proxy rotation / residential proxies | 10-user load doesn't hit per-IP bans when sequential failover + per-host rate limiter are in place. |
| Parallel multi-provider fan-out | Multiplies upstream load by N providers, trips per-host bans, doubles latency tails. Sequential with health-aware skipping is sufficient. |
| Pre-populating provider catalogs | Same on-demand pattern as today (CLAUDE.md). |
| Auto-pick "first with results" ignoring per-anime preference | Violates `feedback_watch_preferences.md`. The orchestrator respects user preference order; provider fallback only kicks in when the preferred provider returns `ErrNotFound` or `ErrProviderDown`. |
| Custom AES key from training data | The exact failure mode that killed aniwatch. All decryption operates on keys fetched live from the upstream embed page. |
| Universal "Zoro-family" HTML parser | Empirically validated 2026-05-11: the three target providers do not share HTML markup. Sharing happens at the `EmbedExtractor` + `BaseHTTPClient` + `Provider` interface level, not at the HTML layer. |

---

## Traceability

| REQ-ID | Phase | Status |
|---|---|---|
| SCRAPER-FOUND-01..10 | A — Foundation | Pending |
| SCRAPER-PAHE-01..05, SCRAPER-UI-01..04 | B — AnimePahe + new EnglishPlayer | Pending |
| SCRAPER-OBS-01..05 | C — Observability | Pending |
| SCRAPER-9ANI-01..06 | D — 9anime | Pending |
| SCRAPER-KAI-01..07 | E — AnimeKai (gated) | Pending |
| SCRAPER-CUT-01..07 | F — Cutover | Pending |
| SCRAPER-NF-01..05 | Cross-cutting (woven through A-F) | Pending |

Traceability is finalized when the roadmapper writes ROADMAP.md.
