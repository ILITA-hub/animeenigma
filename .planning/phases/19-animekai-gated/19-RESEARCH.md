# Phase 19: AnimeKai (gated) — Research

**Researched:** 2026-05-12
**Domain:** EN-anime third-provider R&D — MegaUp token + KAI iframe decryption inside docker/megacloud-extractor sidecar, gated behind `SCRAPER_ANIMEKAI_ENABLED` env flag
**Confidence:** HIGH on flow + endpoints + token spec; MEDIUM on long-term convergence (site formally shutting down)

## Summary

**The phase is a textbook escape-hatch candidate.** Three findings, ranked by importance:

1. **AnimeKai officially announced shutdown on 2026-05-10** — exactly 2 days before this research. The current `anikai.to` site still serves traffic (full live pipeline verified end-to-end below — search → episodes → server list → encrypted KAI iframe → decrypted MegaUp `/media/<id>` → playable m3u8) but the footer banner reads: *"with a lot of things recently, we're unable to continue running the project :(. It's time to backup your list and find a new home for your anime journey."* The cited public reason is a data-center fire at the file-hosting infrastructure (multiple outlets confirmed 2026-05-10: Distractify, CBR, Digital Trends, IMDB News, PiunikaWeb, OtakuKart, Fandomwire). Any in-house extractor we build is racing against the wind-down clock — the convergence target is not a stable upstream.
2. **The MegaUp token + KAI decryption algorithm is a TWO-STEP per-page-load deobfuscation job, not a static algorithm.** anipy-cli's `key-gen` branch (`scripts/decoder/index.js`) ships a Node.js implementation that uses `webcrack` to deobfuscate the live `bundle.js` and the live megaup `app.js`, pattern-matches the encode/decode functions, and writes a `kai.json` spec consumed by the Python provider. The committed `kai.json` (2025-04-23, ~14 months stale) **does NOT match the live algorithm today** — verified empirically: `generate_token("dYW-8w")` with the stale spec returns an 8-char output; live enc-dec.app returns a ~52-char output. The site's obfuscator has rotated since. **A working in-house extractor must include the deobfuscation pipeline, not just the spec, because the spec changes whenever AnimeKai re-obfuscates.**
3. **All public 3rd-party AnimeKai providers we found use enc-dec.app** (walterwhite-69/AnimeKAI-API, the official Aniyomi extension `MegaUp.kt`, the Tachiyomi 525-star community fork). The only project shipping an in-house implementation is anipy-cli — and its committed algorithm is provably stale. **There is no "just port the algorithm" path.** The R&D is real and non-trivial.

**Primary recommendation:** **Ship the phase flag-default-off + extractor stubbed; carry `SCRAPER-KAI-01..04` to v3.1.** Implement the gate (env flag + orchestrator conditional registration + frontend dropdown hidden when flag off) plus a non-functional stub of `/animekai-token` returning HTTP 501. Wire end-to-end metrics (`provider_health_up{provider="animekai"}` seeded at 0, `parser_requests_total{provider="animekai"}` stays flat-zero by construction). This satisfies success criteria 1, 4, and 5 from the ROADMAP, ships in days not weeks, lets Phase 20 cutover proceed unblocked, and avoids dumping 1-2 weeks of engineering into a stale upstream that may go dark before the gate is even flipped. The R&D itself (real token generator) belongs in v3.1 against whatever AnimeKai successor mirror surfaces — or against a different replacement provider if no successor appears.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Feature Flag Strategy (LOCKED — ROADMAP success criterion 1)**
- Env-var name: `SCRAPER_ANIMEKAI_ENABLED` (boolean string, "true"/"false")
- Default: `false` in production (`docker/.env.example` documents the toggle)
- Read at: scraper-api orchestrator startup (registration order: animepahe → gogoanime → animekai if enabled)
- Toggle without rebuild: `docker compose restart scraper`
- Frontend never sees a 3rd source dropdown option while flag is off (orchestrator's `RegisteredProviders` list is source of truth)

**Token Generation Topology (LOCKED — ROADMAP success criterion 2)**
- All MegaUp embed token + AES key derivation lives inside `docker/megacloud-extractor/` (node service)
- New endpoint: `POST /animekai-token` on the megacloud-extractor service
- ZERO external dependencies on `enc-dec.app` — `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns nothing
- The Go scraper service calls the in-cluster megacloud-extractor over HTTP (same pattern as Phase 16 HiAnime → megacloud-extractor)

**Provider Package Layout**
- New package: `services/scraper/internal/providers/animekai/`
- Files follow the Phase 18 gogoanime pattern: `doc.go`, `client.go`, `dto.go`, `cache.go`, `malsync.go` (forward-compat probe with 24h negative cache; malsync DOES ship AnimeKAI keys — see Malsync Coverage section, the malsync.go is a PRIMARY path here, not a forward-compat stub)
- DDoS-Guard: NOT NEEDED — `anikai.to` returns Cloudflare 200 with no challenge for plain `curl`. Omit `ddosguard.go` (same precedent as gogoanime).
- Exported shape: `animekai.Deps` + `animekai.New(d Deps) (*Provider, error)` matching the established convention

**Convergence Definition (Claude's discretion — research-driven)**
- "Convergence" = the in-house token generator produces working MegaUp tokens that resolve to a playable HLS stream against live `anikai.to` (canonical mirror that `animekai.to` 301-redirects to) for at least 3 sample anime across 2 different episode types (sub + dub) over a 24h period
- If the extractor returns errors persistently (>50% failure rate across 10 sample requests), declare non-convergence
- The decision point is documented in this RESEARCH.md and carries forward to `19-04-SUMMARY.md` if the escape hatch is taken
- Non-convergence outcome: flag remains default-off, requirements `SCRAPER-KAI-01..04` are explicitly marked "carry to v3.1" in REQUIREMENTS.md, `SCRAPER-KAI-05..07` (flag wiring, observability, docs) still SHIP

**Escape Hatch (LOCKED — ROADMAP success criterion 5)**
- The phase ships either way (flag default-off with extractor wired, OR flag default-off with extractor carried over)
- Phase 20 (cutover) MUST NOT block on AnimeKai's R&D outcome — cutover removes HiAnime + Consumet dead code regardless

**HLS Proxy Allowlist**
- Append AnimeKai CDN hostnames to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` (specific hostnames discovered during research — see Hostnames to Append section)
- Phase 18's append-only invariant preserved — kwik.cx, anitaku.to, vibeplayer.site, etc. all retained
- Regression test: existing Phase 16 + Phase 18 hosts still match

**Observability (LOCKED — Phase 17 patterns)**
- `provider_health_up{provider="animekai",stage=...}` gauges via the existing health probe pattern
- `parser_fallback_total{from="gogoanime",to="animekai"}` counter (Phase 17 already emits this; new label tuple appears once flag flips on)
- `parser_requests_total{provider="animekai"}` MUST stay flat-zero while flag is off (success criterion 4)

**Frontend**
- No new frontend code while flag is off — the source dropdown's "AnimeKai" option ONLY appears when `RegisteredProviders` includes it
- `capitalizeProvider('animekai')` returns `'AnimeKai'` (the display label matches the backend slug for this one — no rebrand)
- Locale keys: reuse existing `player.sourceMultiTooltip` etc. (Phase 16 declared them for N-provider scaling)

### Claude's Discretion

- Exact `/animekai-token` request/response schema (will be informed by reading existing megacloud-extractor endpoints) — see Pattern §3 below for the recommended shape
- Token cache TTL inside megacloud-extractor (likely 1-5 min based on AnimeKai's token expiry) — see Pattern §3 below
- Retry/backoff policy when token generation fails (will mirror existing Phase 16 HiAnime patterns)
- Number of embed extractors AnimeKai dispatches to — discovered during research; verified TWO embed surfaces: the AnimeKai-internal `anikai.to/iframe/<token>` and the MegaUp host `megaup.cc/e/<id>` reached via that iframe. Both are owned by the same extractor (`animekai`/megaup) and are routed via the megacloud-extractor sidecar (no Go-side embed routing changes needed for this provider).

### Deferred Ideas (OUT OF SCOPE)

- AnimeKai as a default-on provider (post-7-days clean traffic decision; out of v3.0 scope)
- Multi-mirror failover within AnimeKai if `anikai.to` rotates (v3.1+) — note the canonical brand has already rotated `animekai.to → anikai.to` AND announced shutdown 2026-05-10
- Server-side analytics on which provider users prefer (v2.1 backlog item)
- Tower defense convergence: if R&D produces multiple competing token-generation strategies, ship the one with cleanest test surface; alternates carried to v3.1
</user_constraints>

## Project Constraints (from CLAUDE.md)

| Directive | Where Enforced | How the planner must honor it |
|-----------|----------------|-------------------------------|
| `make redeploy-<service>` after code change; `make health` for verification | CLAUDE.md "Local Development Commands" | Final plan in the phase MUST call `make redeploy-scraper && make redeploy-megacloud-extractor && make redeploy-web && make health` |
| Frontend uses `bun`/`bunx`, never npm/pnpm/npx | CLAUDE.md "Frontend Note" | Any frontend plan calls `bun install` / `bunx tsc --noEmit` / `bun run build` |
| After-update skill (lints + builds + redeploys + changelog + commit) MUST be invoked at phase end | CLAUDE.md "After-Update Skill (MUST USE)" | Final phase plan invokes `/animeenigma-after-update` |
| Don't commit secrets (`.env`, `credentials.json`) | CLAUDE.md + memory | New env vars `SCRAPER_ANIMEKAI_ENABLED`, `SCRAPER_ANIMEKAI_BASE_URL` go into `docker/.env` (gitignored) + `docker/.env.example` |
| Don't add headless/JA3/proxy-spoof deps: `chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, `flaresolverr` | `SCRAPER-FOUND-09` CI lint | Plans MUST NOT propose any of these. Even though AnimeKai uses Cloudflare Turnstile on signed-out pages, all `/ajax/*` endpoints are reachable from plain HTTPS (verified — see "Live Pipeline Verification" section). The webcrack node deobfuscator IS allowed (npm dep on the Node sidecar, not a Go service dep). |
| Use structured logging via `libs/logger` (`Infow` / `Errorw`) | CLAUDE.md "Logging" | Provider code MUST use `*logger.Logger`, not `log.Printf` |
| Issues/incidents documented in `docs/issues/README.md` (ISS-NNN) | Project memory | If extractor failures surface during impl, log as ISS-NNN |
| Don't auto-pre-populate provider catalogs; on-demand only | CLAUDE.md "Don't Do" | The provider resolves IDs lazily via malsync (primary path) then fuzzy fallback; no batch warm-up |
| `make redeploy-megacloud-extractor` target exists | docker-compose.yml has the `megacloud-extractor` service | Plan adds the `/animekai-token` endpoint to existing `server.js`, then `make redeploy-megacloud-extractor` |
| Frontend `capitalizeProvider` must handle `animekai` | Phase 18 pattern (gogoanime → "Anitaku") | Add one line: `'animekai': 'AnimeKai'` |

<phase_requirements>
## Phase Requirements

| ID | Description (from REQUIREMENTS.md) | Research Support |
|----|------------------------------------|------------------|
| SCRAPER-KAI-01 | Given a Shikimori/MAL ID, the AnimeKai client resolves the matching AnimeKai slug via `malsync.moe` | **Malsync ships AnimeKAI keys.** Verified 2026-05-12: `GET https://api.malsync.moe/mal/anime/16498` returns `Sites.AnimeKAI = {<identifier>: {identifier, image, malId, title, url}}`. Key is `AnimeKAI` (TitleCase — different shape from Phase 18 `Gogoanime`-which-doesn't-exist). The provided `identifier` is the `data-id` AND the slug suffix used in `/watch/<slug>-<identifier>`. The `url` field is `https://animekai.to/watch/shingeki-no-kyojin-nk0p#ep=1` — needs `301`-follow to `anikai.to` and `#ep=N` strip. **Plan must invert from Phase 18: malsync IS primary path, fuzzy fallback is secondary.** |
| SCRAPER-KAI-02 | `ListEpisodes` returns the full episode list scraped from AnimeKai's markup (`aitem-wrapper`, `alist-group`, `azlist` class family). Sub/dub split surfaced. | **Markup classes from REQUIREMENTS are stale.** Verified 2026-05-12 live markup: episode list NOT inlined in `/watch/<slug>`. Must call `GET /ajax/episodes/list?ani_id=<id>&_=<token>` with `X-Requested-With: XMLHttpRequest`. Response is `{status:"ok", result:"<HTML fragment>"}` where the fragment contains `<div class="eplist titles"><ul class="range"><li><a href="#" num="N" slug="N" langs="3" token="<ep_token>"><span data-jp="">Episode N</span></a></li>...</ul></div>`. `langs="1"` = sub only, `langs="2"` = dub only, `langs="3"` = both. Episode `token` is a 20-char string `[a-zA-Z0-9_-]{20}` used for `ListServers`. **Selectors REQUIREMENTS lists do not exist** — they may have been copied from an old reference. The selectors above are the verified-live shape. |
| SCRAPER-KAI-03 | `ListServers` enumerates AnimeKai's embed hosts. AnimeKai is known to use MegaUp/megacloud-variant embeds; these route to the existing `megacloud` `EmbedExtractor` (extended if necessary). | **Server endpoint shape:** `GET /ajax/links/list?token=<ep_token>&_=<token>` returns 3 `<div class="server-items lang-group" data-id="<sub|softsub|dub>">` blocks, each containing `<span class="server" data-sid="<N>" data-eid="<encrypted>" data-lid="<encrypted>">Server N</span>` entries. Live observed: 2 servers per language × 3 languages = 6 entries per episode. The `data-lid` is the "link ID" passed to `/ajax/links/view`. **No multi-host dispatch is needed** — all 6 servers route through MegaUp/anikai-iframe. The "extractor" is single-purpose: the new sidecar endpoint `/animekai-token`. |
| SCRAPER-KAI-04 | AnimeKai MegaUp-embed decryption + auth-token generation runs inside `docker/megacloud-extractor/` via new endpoint (e.g. `/animekai-token`). NO call to `enc-dec.app`. | **This is the R&D core.** Complete pipeline live-verified end-to-end via enc-dec.app (research only — not the deliverable): (1) encode `ani_id`/`ep_token`/`link_id` separately via `enc-kai` to derive `_` URL param, (2) decode `/ajax/links/view` response via `dec-kai` to extract `{url: "https://anikai.to/iframe/<token>", skip:{intro,outro}}`, (3) follow that iframe → its body contains `<iframe src="https://megaup.cc/e/<id>?">`, (4) `GET https://megaup.cc/media/<id>` with matched UA returns `{status:200, result:"<encrypted>"}`, (5) decode via `dec-mega` (UA-bound) to extract `{sources:[{file:".m3u8"}], tracks:[{file:".vtt"}], download:"..."}`. **In-house port requires running webcrack-based JS deobfuscation against the live `scripts-<hash>.js` AND `megaup.cc/<app.js>` to extract the encode/decode functions dynamically; this is non-trivial R&D.** See Token Generation Topology section. |
| SCRAPER-KAI-05 | AnimeKai ships behind feature flag (`SCRAPER_ANIMEKAI_ENABLED`, default off ≥ 7 days, then default on). Read at orchestrator startup, toggleable via `docker compose restart catalog`. | **REQUIREMENTS text says `docker compose restart catalog` — that is a bug in REQUIREMENTS.md.** The flag is consumed by the scraper service (`services/scraper/cmd/scraper-api/main.go::main()`), so the actual restart target is `docker compose restart scraper`. CONTEXT.md D1 has the correct text. Plan must use scraper, NOT catalog. |
| SCRAPER-KAI-06 | If R&D doesn't converge during Phase 19, AnimeKai ships with the flag default-off and `SCRAPER-KAI-01..04` stay open as v3.1 carryover. The rest of v3.0 ships regardless. | **Strong recommendation to take this path.** See Convergence Probability Assessment section. |
| SCRAPER-KAI-07 | Orchestrator failover AnimePahe → 9anime → AnimeKai verified end-to-end with flag on: forcing both AnimePahe and 9anime down still produces a playable stream from AnimeKai. | **Only verifiable if R&D converges.** If escape hatch taken: this becomes a v3.1 task. Plan must NOT block phase ship on this requirement when flag is default-off. |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| AnimeKai HTML/AJAX scrape (`/watch/<slug>`, `/ajax/episodes/list`, `/ajax/links/list`, `/ajax/links/view`) | Scraper Service (`services/scraper/internal/providers/animekai/`) | — | Per-provider HTML parsing is non-shareable (REQUIREMENTS.md universal-layer table) |
| Token generation (`_` URL param) + KAI iframe decryption + MegaUp `/media/` decryption | docker/megacloud-extractor (Node.js sidecar) | — | LOCKED per CONTEXT.md — token logic lives in the existing sidecar via new `POST /animekai-token` endpoint. The provider package calls the sidecar; Go does NO crypto. |
| MAL ID → AnimeKai identifier resolution | `services/scraper/internal/providers/animekai/malsync.go` | — | Malsync ships `AnimeKAI` keys for known MAL IDs (verified 2026-05-12). Same shape as `services/scraper/internal/providers/animepahe/malsync.go` with the slug constant changed to `"AnimeKAI"` and the cache key `malsync:<mal>:animekai`. |
| Title fuzzy match (Jaro-Winkler) fallback | Scraper Service (reuse `services/scraper/internal/providers/animepahe/` helper OR a `services/scraper/internal/fuzzy/` shared package if extracted in Phase 18 wave 0) | — | Recommendation: **if Phase 18 extracted `services/scraper/internal/fuzzy/`, reuse it.** Otherwise duplicate the JaroWinkler helper privately (don't refactor in Phase 19 — keep R&D risk isolated). |
| Provider failover ordering | Scraper Service Orchestrator | — | Already implemented in Phase 17 + 18; new provider only needs `Register()` call (conditional on flag) |
| Health probe per stage | Scraper Service `health.ProbeRunner` (Phase 17, auto-discovers) | — | Iterates `RegisteredProviders()`; no per-provider probe code. When flag is off, the provider is not registered, probe doesn't iterate it, gauges stay at the seed value. |
| Frontend provider override | Vue Pinia store `useWatchPreferences` (Phase 16) | — | Phase 16 already supports arbitrary string values; no store change |
| Frontend dropdown UI | `EnglishPlayer.vue` (Phase 16/18 component, extensible) | — | Phase 18 added the dropdown shape; this phase only conditionally adds one `<option>` (driven by `getHealth()` response, not hardcoded) |
| HLS proxy CORS rewrite | `libs/videoutils/proxy.go::ProxyWithReferer` | — | Existing endpoint; append new hostnames (TBD until streams are stable) |
| Stream URL caching | Redis via `libs/cache` | — | Same wrapper as AnimePahe/Gogoanime; key namespace `stream:animekai:*` |
| API gateway routing | Already routed (`/api/anime/{id}/scraper/*` → catalog → scraper) | — | No gateway change |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/PuerkitoBio/goquery` | v1.8.x (already in scraper go.mod via AnimePahe/Gogoanime) | HTML DOM traversal for `<div class="server-items">`, `<div class="eplist">` etc. | Already standard in this repo; identical to Phase 16/18 |
| `github.com/hashicorp/go-retryablehttp` (via `domain.BaseHTTPClient`) | already wired | Per-host rate limit + 429/5xx backoff for `anikai.to` | Phase 15 standard; no new HTTP client |
| `github.com/ILITA-hub/animeenigma/services/scraper/internal/health` | in-tree (Phase 17) | Canonical stage constants (`StageSearch`, `StageEpisodes`, `StageServers`, `StageStream`) | Locked contract |
| `github.com/ILITA-hub/animeenigma/libs/cache` | in-tree | Redis wrapper for malsync (positive-cache TTL 24h, since AnimeKAI key exists), episodes, stream TTLs | Phase 15 standard |
| `github.com/ILITA-hub/animeenigma/libs/logger` | in-tree | Structured logging | Repo-wide convention |
| `github.com/ILITA-hub/animeenigma/libs/metrics` | in-tree | `ParserFallbackTotal{from,to}`, `ParserZeroMatchTotal{provider,selector}`, `ProviderHealthUp{provider,stage}` already emitted; no new metric needed | Phase 17 wiring |

### Node.js dependencies (sidecar — `docker/megacloud-extractor/package.json`)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `webcrack` | ^2.x (npm registry — VERSION VERIFY at plan time) | JavaScript deobfuscator for `anikai.to/.../scripts-<hash>.js` and `megaup.cc/<app>.js` | Required by the upstream anipy-cli `key-gen` script (the only public in-house reference). The bundle is V8-engine-style obfuscated identical to what we observed at `/tmp/anikai_scripts.js:1` — opening with `r[315458]=function(){for(var a=2;9!==a;)switch(a)...`. **Plain regex extraction WILL NOT work** — webcrack is required to normalize before pattern matching. |
| Node `crypto` (stdlib) | n/a | RC4 (`createCipheriv('rc4', key, iv)`) for `transform()` calls in the spec; SHA/MD5/AES-CBC if MegaUp algorithm needs it (the `decode` function chain uses RC4 + base64url + substitution + reversal — NO AES per the anipy reference; if production has rotated to AES, the extractor must re-derive at runtime) | Already in tree; no install |
| `node-fetch` or stdlib `https` | stdlib | HTTP fetch of `bundle.js`, `app.js`, AnimeKai `/watch/<slug>` to scrape `data-id` | Mirror the existing `https.request` pattern in `server.js` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| **In-house webcrack + dynamic spec extraction** | enc-dec.app (existing public service) | **REJECTED by CONTEXT.md.** enc-dec.app is the same single-point-of-failure that killed Consumet (its body-shape contract change was the v3.0 trigger). |
| **In-house webcrack + dynamic spec extraction** | Hardcoded `kai.json` static spec | **REJECTED.** Empirically verified the anipy-cli committed `kai.json` (2025-04-23) does NOT match the live algorithm: `strict_encode("dYW-8w", <ops>)` returns `0y2OA8-I` (8 chars) while enc-dec.app returns `xQm9tJfLwGhz_0Eq8S_YAHYkwp-qSvLfm50W5c1nyd2ZnNcpzTWI` (52 chars). The site has re-obfuscated since. Static spec inevitably breaks. |
| **Static spec with online auto-update on token-verification failure** | Webcrack at every request | Compromise option — cache the extracted spec inside the sidecar for 6h; re-extract only when a `/animekai-token` request returns a token that the live AnimeKai server rejects (HTTP 4xx from `/ajax/episodes/list`). Plan should adopt this if attempting convergence. Falls back to "every request webcracks" only when the rotation is fast. |
| **AnimeKai as third provider in Phase 19** | Take the escape hatch — ship gate + stub only | **STRONG RECOMMENDATION.** See Convergence Probability Assessment. |
| **Different replacement provider for v3.1 carryover** | KickAssAnime (the only other site with a malsync key besides animekai/animepahe/Crunchyroll/Hulu/Netflix) | Possibility worth noting for v3.1 — `KickAssAnime` (`kaa.to`) has malsync coverage. Not in v3.0 scope. |

**Installation (Node sidecar):**

```bash
# Inside docker/megacloud-extractor/
npm init -y  # if package.json needs to be regenerated (currently minimal — no deps)
npm install --save webcrack@latest
# webcrack ships with hundreds of transitive deps; the resulting image grows ~50-80 MB
```

**Version verification:** Plan author MUST run before locking versions:
```bash
npm view webcrack version  # returns the latest published version
npm view webcrack peerDependencies   # check Node version constraints
```
Document the verified version + publish date in the plan. The current Node 20 alpine image in `docker/megacloud-extractor/Dockerfile` should be sufficient.

## Live Pipeline Verification (2026-05-12, via enc-dec.app for the encryption step ONLY — sanity check, not the deliverable)

Captured live against `https://anikai.to/` (animekai.to 301-redirects here). All steps verified end-to-end; HLS playlist URL acquired and resolved.

```
Step 1 — Landing
GET https://animekai.to/
→ 301 Location: https://anikai.to/
→ 200 OK   <title>AnimeKai - Watch Free Anime Online...</title>
  Footer banner: "with a lot of things recently, we're unable to continue running the project..."

Step 2 — Search
GET https://anikai.to/browser?keyword=Attack+on+Titan
→ 200 OK; results as <a class="title" data-jp="Shingeki no Kyojin" title="Attack on Titan">...</a>
  inside <a href="/watch/<slug>-<short>">...</a>

Step 3 — Watch page (anime page = no inline episode list — list is XHR-loaded)
GET https://anikai.to/watch/attack-on-titan-final-season-the-final-chapters-special-2-gm9e
→ 200 OK
  Critical extractables:
    <div class="rate-box" data-id="dYW-8w">  ← AnimeKai's internal anime ID (NOT the slug)
    <script>window.__$='ZZYdbXag...'</script>  ← Cloudflare-Turnstile token (passed only on auth actions)
    data-meta="ZZQdeGGgrgMF..."  ← Encrypted anime metadata, not needed for stream pipeline
    Anti-tamper JS in scripts-CbWe9mAN.js (V8-style obfuscation, requires webcrack)

Step 4 — Episode list
GET https://anikai.to/ajax/episodes/list?ani_id=dYW-8w&_=<token(dYW-8w)>
Headers: { X-Requested-With: XMLHttpRequest, Referer: https://anikai.to/watch/... }
→ 200 {status:"ok", result:"<HTML fragment>"}
  Fragment contains:
    <div class="eplist titles">
      <ul class="range" data-range="001-001">
        <li><a href="#" num="1" slug="1" langs="3" token="e9fh8_jxtw26iG9ez4yX">
          <span data-jp="">Episode 1</span>
        </a></li>
      </ul>
    </div>
  langs decoding (anipy-cli verified): "1"=sub only, "2"=dub only, "3"=both
  token format: 20-char [a-zA-Z0-9_-]

Step 5 — Server list
GET https://anikai.to/ajax/links/list?token=e9fh8_jxtw26iG9ez4yX&_=<token(e9fh8_jxtw26iG9ez4yX)>
Headers: same as Step 4
→ 200 {status:"ok", result:"<HTML fragment>"}
  Fragment contains:
    <div class="server-wrap">
      <div class="server-type" data-tabs=".server-wrap .server-items">
        <span class="tab active" data-id="sub">Hard Sub</span>
        <span class="tab" data-id="softsub">Soft Sub</span>
        <span class="tab" data-id="dub">Dub</span>
      </div>
      <div class="server-items lang-group" data-id="sub">
        <span class="server" data-sid="3" data-eid="c4O596Gp" data-lid="dIKy8K6l4Q">Server 1</span>
        <span class="server" data-sid="2" data-eid="c4O596Gp" data-lid="dIKy8K6l4A">Server 2</span>
      </div>
      ... same shape for data-id="softsub" and data-id="dub" ...
    </div>
  data-lid is the per-server token, 10-12 char [a-zA-Z0-9_-]
  2 servers × 3 lang groups = 6 entries per episode

Step 6 — Per-server link resolution
GET https://anikai.to/ajax/links/view?id=dIKy8K6l4Q&_=<token(dIKy8K6l4Q)>
Headers: same as Step 4
→ 200 {status:"ok", result:"<KAI-encrypted string>"}
  result is 327-char [A-Za-z0-9_-/] base64url-like; needs dec-kai to decrypt.

Step 7 — KAI decryption (currently via enc-dec.app for research; PHASE 19 deliverable replaces this with in-sidecar)
POST https://enc-dec.app/api/dec-kai
Body: {"text": "<result from step 6>"}
→ 200 {"status":200, "result":{"url":"https://anikai.to/iframe/Ksf-sOWq_1C7hntHyI7D92VY4MIL9yPI6scG0Hl2cRT41Q_Ctb2mxh8BaahTeg",
                              "skip":{"intro":[10,130],"outro":[0,0]}}}

Step 8 — Follow iframe to discover MegaUp host
GET https://anikai.to/iframe/Ksf-sOWq_1C7hntHyI7D92VY4MIL9yPI6scG0Hl2cRT41Q_Ctb2mxh8BaahTeg
→ 200 HTML body contains:
    <iframe src="https://megaup.cc/e/0sj0L2C0WS2JcOLyGbxK5xvpCQ?"></iframe>

Step 9 — Media payload (UA-CRITICAL)
GET https://megaup.cc/media/0sj0L2C0WS2JcOLyGbxK5xvpCQ
Headers: { User-Agent: <SAME as request that produced the encrypted text>, Referer: https://anikai.to/ }
→ 200 {status:200, result:"<MEGA-encrypted string>"}
  result is 970-char base64url-like.
  CRITICAL: the encryption key derivation includes the requesting User-Agent.
  enc-dec.app's /api/dec-mega rejects requests where the UA passed to it
  doesn't match the UA used to fetch /media/. The in-house port must
  preserve UA across the GET-then-decode boundary (the simplest design:
  the sidecar performs BOTH the GET and the decode within a single
  /animekai-token call).

Step 10 — MEGA decryption
POST https://enc-dec.app/api/dec-mega
Body: {"text": "<result from step 9>", "agent": "<same UA as step 9>"}
→ 200 {"status":200, "result":{
    "sources":[{"file":"https://rrr.pro25zone.site/p5rm/.../list,WuxrIQT2oR6y.m3u8"}],
    "tracks":[{"file":"https://5rm.megaup.cc/v5/.../thumbnails.vtt","kind":"thumbnails"}],
    "download":"https://megaup.cc/download/0sj0L2C0WSyJcOLyGbxK5xvtDg"
  }}
  ✓ HLS m3u8 URL acquired
  ✓ Captures: 0 (this episode had no English captions; other episodes do, see live-MegaUp.kt note below)
  ✓ Download link present (not used)
  ✓ CDN host: pro25zone.site (looks like a rotating per-stream CDN, similar to streamhg/earnvids in Phase 18)

Step 11 — m3u8 fetch verification (not performed in this research; should be re-verified at plan time)
GET https://rrr.pro25zone.site/p5rm/.../master.m3u8
Headers: { Referer: https://megaup.cc/, UA: <any>, Origin: <none — see below> }
Expected: 200 with #EXTM3U + #EXT-X-STREAM-INF rows (master playlist).
```

**Inferred CDN architecture for HLS proxy allowlist:**
- `pro25zone.site` — primary m3u8 CDN (looks rotating; verify if subdomain prefix varies across episodes)
- `5rm.megaup.cc` — captions/thumbnails subdomain on `megaup.cc` (which is already implicitly in the megacloud knownHosts list — `megaup.cc` matches the existing `megaup.cc` entry under `megacloudKnownHosts`)
- megaup.cc per RESEARCH on Phase 18 already in `megacloudKnownHosts`

## Convergence Probability Assessment

This phase is structurally R&D. The escape hatch (CONTEXT.md D4 + ROADMAP success criterion 5) is in the design FOR THIS REASON. Below is the honest probability assessment that should drive the planner's recommendation.

### Risk factors (in descending impact)

| Risk | Severity | Evidence | Mitigation |
|------|----------|----------|------------|
| **AnimeKai is officially shutting down** | CATASTROPHIC | Footer banner on every page since 2026-05-10; data-center fire confirmed by 8+ news outlets; live URL says "back up your list and find a new home" | None at the provider level — even if R&D converges this week, the upstream may go dark this month. The flag-default-off ship + carry-to-v3.1 is the structurally safe path. |
| **Algorithm rotates per re-obfuscation** | HIGH | anipy-cli's committed `kai.json` (2025-04-23) provably does NOT match the live algorithm today. The site has re-obfuscated at least once in the intervening 14 months. | Webcrack-based dynamic extraction + spec cache + auto-refresh on token-verification failure. Adds complexity; reliability tied to webcrack pattern-match heuristics surviving the obfuscator's next move. |
| **MegaUp key derivation is UA-bound** | MEDIUM | Verified — `enc-dec.app/api/dec-mega` returns "User-Agent must match one from /media/ request" when the UAs mismatch. The encryption itself includes UA in the key derivation. | Sidecar performs both GET and decode within a single call so UA is consistent by construction. |
| **Webcrack pattern-match heuristics are fragile** | MEDIUM | The upstream `index.js:194-228` deobfuscation logic relies on heuristics like "function with `256` in body = transform", "function with `btoa` = base64_url_encode". A trivial change by AnimeKai (renaming `btoa` to `String.prototype.toString.call(...)` or inlining the function) breaks the matcher silently and a token-verification-failure loop kicks in. | Auto-refresh + Prometheus alert when extraction fails 3 times in 60 minutes. Surface the failure to Telegram. |
| **Cloudflare Turnstile** | LOW | Present on the site, but observed inactive on `/ajax/*` endpoints during this research. The Turnstile token (`window.__$=...`) is generated client-side and embedded in some XHR headers per the Phase 18 SCRAPER-FOUND-09 lint — adding chromedp to solve Turnstile is FORBIDDEN. Verified: all `/ajax/` endpoints respond 200 with plain UA + `X-Requested-With: XMLHttpRequest`. | If Turnstile gates the AJAX endpoints in the future, the provider degrades to `ErrProviderDown` and the orchestrator routes to gogoanime. No headless workaround. |
| **Test fixtures will require manual capture** | LOW | Per Phase 18, goldens capture is `make capture-goldens-<provider>`. AnimeKai's response shapes are stable enough to capture, but the encrypted-payload golden tests need fresh fixtures every time the algorithm rotates (the encoded `_` param is a hash of the live algorithm; capturing it once and replaying yields a stale assertion). | Tests must mock the sidecar HTTP response, not the AnimeKai upstream — golden the SIDECAR output, not the encrypted upstream payload. |

### Probability estimates (assumption — calibrate to ground truth at impl time)

| Outcome | Probability | Driver |
|---------|-------------|--------|
| **In-house extractor converges within 1 week R&D and stays converged for ≥ 7 days** | ~25% | Webcrack heuristics work today; site stays stable enough; no further re-obfuscation; data-center fire doesn't take more infrastructure down. |
| **Extractor converges but breaks within 30 days** | ~40% | Site re-obfuscates; auto-refresh saves it but we ship known fragile code; ongoing maintenance cost. |
| **Extractor never converges (or only partially — some servers work, others don't)** | ~25% | Heuristics don't match current obfuscation shape; pattern-matcher needs hand-tuning; we're consuming research budget chasing a moving target on a dying site. |
| **AnimeKai goes fully dark during Phase 19** | ~10% | Data-center fire news is recent; founders may pull the plug; even a successful extractor returns 5xx universally. |

### Recommendation: **ESCAPE HATCH PATH**

**Ship Phase 19 with:**

1. **Feature flag wired** (`SCRAPER_ANIMEKAI_ENABLED`, default `false` in `docker/.env.example` + `docker/docker-compose.yml`)
2. **`animekai` provider package scaffolded** (`services/scraper/internal/providers/animekai/`) with:
   - `doc.go` documenting the escape-hatch status
   - `client.go` returning `domain.ErrProviderDown` from every method when the flag is off OR when the sidecar `/animekai-token` returns 501
   - `malsync.go` implementing the resolver (forward-ready for when R&D unblocks — it's cheap to ship)
   - Stub `cache.go`, `dto.go` so the next iteration's diff is small
3. **Sidecar stub endpoint `POST /animekai-token`** in `docker/megacloud-extractor/server.js` returning HTTP 501 `{"error":"not implemented (carry to v3.1)"}` with a clear log line so misconfigured prod can be detected
4. **Conditional registration in `cmd/scraper-api/main.go`** so when `SCRAPER_ANIMEKAI_ENABLED=true`, the provider IS registered (and immediately fails over to gogoanime because the sidecar returns 501) — flag mechanics are tested but no real traffic served
5. **Phase-18-style boot invariant** for the now-3-provider expectation when the flag IS true (and 2-provider when it's false)
6. **Observability seeded** (`provider_health_up{provider="animekai",stage=...}` initialized to 0 — NOT 1 — because we KNOW the provider can't serve)
7. **REQUIREMENTS.md annotated** at requirement table: `SCRAPER-KAI-01..04` move to "Pending — carry to v3.1"; `SCRAPER-KAI-05` and `SCRAPER-KAI-06` are completed; `SCRAPER-KAI-07` becomes "v3.1 — gated on R&D convergence"
8. **Tests cover the flag-off and flag-on-with-stub paths** end-to-end so a future v3.1 PR that fills in the sidecar endpoint passes the same orchestrator-integration test by changing only the sidecar handler's 501 to 200
9. **Documentation update:** `19-04-SUMMARY.md` records the escape-hatch decision with a link to this RESEARCH.md "Convergence Probability Assessment" section so the v3.1 planner has the full background

**Effort estimate:**
- Escape-hatch path: ~3-4 days (gate wiring + stub sidecar + test scaffolding + REQUIREMENTS annotation)
- Converge path: ~7-10 days IF webcrack pattern-matcher hits clean (no rework); ~14-21 days if rework needed (more likely)

**Phase 20 unblock:** the escape-hatch path EXPLICITLY satisfies ROADMAP success criterion 5 ("Phase 20 cutover unblocked"). Phase 20 deletes HiAnime + Consumet dead code regardless of AnimeKai status.

## Token Generation Topology

**If the planner chooses the convergence path** (against the recommendation above), this section is the implementation reference.

### `/animekai-token` endpoint shape (recommended)

The sidecar must do MORE than the `enc-dec.app` API does (which is purely one-shot encrypt/decrypt). To match the locked CONTEXT.md surface (a single endpoint that the Go scraper calls), `/animekai-token` should be a **request-router** that wraps the entire pipeline. Schema:

```jsonc
// Request from Go scraper:
POST http://megacloud-extractor:3200/animekai-token
Content-Type: application/json
{
  "op": "generate" | "decrypt_kai" | "fetch_and_decrypt_mega",
  "text": "<input string>",           // for op=generate or op=decrypt_kai
  "media_url": "https://megaup.cc/media/<id>",  // for op=fetch_and_decrypt_mega
  "user_agent": "<UA string>"          // optional; defaults to the sidecar's hardcoded UA
}

// Response:
200 OK
{
  "result": "<encrypted token>"    // for op=generate
  // OR
  "result": {"url":"https://anikai.to/iframe/...","skip":{"intro":[..],"outro":[..]}}   // for op=decrypt_kai
  // OR
  "result": {"sources":[{"file":"...m3u8"}], "tracks":[{"file":"...vtt","kind":"captions"}], "download":"..."}   // for op=fetch_and_decrypt_mega
}

500 Internal Server Error
{ "error": "<reason>" }

501 Not Implemented
{ "error": "AnimeKai sidecar not yet converged — carry to v3.1" }
```

### Internal state (spec cache)

The sidecar maintains an in-memory cache:

```js
const specCache = {
  generated_at: 0,                  // unix ms; refresh when older than 6h OR on miss
  generate_token: "<expression>",   // extracted from anikai.to bundle
  decode_iframe_data: "<expression>",
  encode: "<expression>",           // from megaup.cc/app.js
  decode: "<expression>",
};

async function ensureSpec() {
  if (Date.now() - specCache.generated_at < 6 * 3600 * 1000) return specCache;
  const newSpec = await extractSpecFromLiveBundles();   // webcrack + pattern-match
  Object.assign(specCache, newSpec, { generated_at: Date.now() });
  return specCache;
}
```

### Spec extraction (the R&D core)

Port `scripts/decoder/index.js` from anipy-cli's `key-gen` branch, but **adapt the patterns to the current AnimeKai bundle**:

1. **Find the live bundle URL** — currently `https://anikai.to/assets/build/<hash>/dist/scripts-<hash>.js` (the anipy-cli regex `bundle\.js` is broken against the current Vite-built site)
2. **Find the live MegaUp app URL** — currently emerges from `<iframe src="https://megaup.cc/<some-app-bundle>.js">` once the iframe page is fetched and parsed
3. **Run webcrack** on each bundle to normalize the obfuscation
4. **Pattern-match** the encode/decode functions (`function(n){...}` shape) — but expect the original anipy-cli patterns to need updates because the obfuscator likely changed

### Selectors used by the in-house extractor (live HTML pattern matches)

| Step | Selector / Pattern | What it returns | Failure mode |
|------|--------------------|-----------------|--------------|
| Find anime ID | `div.rate-box[data-id]` | `dYW-8w` | `ErrExtractFailed("animekai: anime data-id missing")` |
| Find bundle | `script[src*="scripts-"][src*="/assets/build/"]` | bundle URL | `ErrProviderDown("animekai: bundle.js URL missing")` |
| Find megaup app | `iframe[src^="https://megaup.cc/"]` inside `/iframe/<token>` response body | megaup wrapper URL | `ErrExtractFailed("animekai: megaup iframe missing")` |
| Episode list | `div.eplist.titles ul.range li > a[num][slug][langs][token]` | one Episode per `<a>` | `ErrExtractFailed("animekai: zero episodes")` (with selector tagged on `parser_zero_match_total`) |
| Server list | `div.server-items.lang-group[data-id] span.server[data-sid][data-eid][data-lid]` | one Server per `<span>` | `ErrExtractFailed("animekai: zero servers")` |
| Stream JSON | `json.result.sources[0].file`, `json.result.tracks[]`, `json.result.skip` | HLS URL + tracks + skiptime | `ErrExtractFailed("animekai: empty sources")` |

### Stream TTL parsing

The HLS URL `https://rrr.pro25zone.site/p5rm/<path>/list,WuxrIQT2oR6y.m3u8` does NOT contain visible expiry timestamps (no `&e=` like Phase 18 streamhg). Use the Phase 15 default of `min(parsed_expiry − 30s, 5min)` with the parsed_expiry safely defaulted to `5min + 30s` (so the cache TTL becomes 5 min). If a master playlist row contains expiry hints in its query string, parse those preferentially.

### Failure-mode taxonomy

| Symptom | Provider returns | Orchestrator behavior | Telegram alert? |
|---------|------------------|----------------------|-----------------|
| Sidecar returns 501 ("not implemented") | `ErrProviderDown` | Skip to next provider | No (expected when flag is on but stub) |
| Sidecar returns 500 with "webcrack failed" | `ErrProviderDown` (with cause string) | Skip; refresh-spec scheduled | Yes (extraction broke) |
| Sidecar returns 200 but `result` is empty | `ErrExtractFailed` | Skip; `parser_zero_match_total{provider="animekai",selector="empty_result"}` increments | Yes if rate > threshold |
| AnimeKai returns 403 / Turnstile challenge | `ErrProviderDown` | Skip; mark stage_search=0 | Yes |
| MegaUp m3u8 fetch returns 403 | `ErrExtractFailed` ("HLS segment 403"); orchestrator skips and re-tries via next server within the same provider before bouncing to next provider | Skip | Yes if all servers fail |

## Provider Package Skeleton

Mirrors Phase 18 gogoanime exactly. **The escape-hatch path ships this scaffold with stub methods; the converge path fills in the real bodies.**

```
services/scraper/internal/providers/animekai/
├── doc.go                  // Package docstring + escape-hatch status note
├── client.go               // Provider interface impl: FindID, ListEpisodes, ListServers, GetStream, HealthCheck
├── dto.go                  // search-result DTO, episode-row DTO, server-row DTO, stream JSON DTO
├── cache.go                // Redis wrappers (malsync 24h, episodes 6h, stream ≤ min(parsed-30s, 5min))
├── malsync.go              // 24h cached MAL → animekai-identifier resolution (PRIMARY path, not fallback)
├── megacloud_extractor.go  // HTTP wrapper around POST /animekai-token (NOT in /embeds — animekai is single-extractor)
├── client_test.go          // Goldens-based unit tests covering all 5 stages
├── dto_test.go             // Pure parser unit tests
├── malsync_test.go         // Cache positive + negative + miss paths
└── megacloud_extractor_test.go  // Mocks the sidecar HTTP shape
```

**Registration in `cmd/scraper-api/main.go`:**

```go
// Phase 19 — AnimeKai (gated)
if cfg.AnimeKai.Enabled {
    animeKaiBaseHTTP := domain.NewBaseHTTPClient(log,
        domain.WithPerHostRPS("anikai.to", 1.0, 2),
        domain.WithPerHostRPS("megaup.cc", 1.0, 2),
        domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
    )
    animeKaiMalsync := animekai.NewMalSyncClient(redisCache)
    animeKaiExtractor := animekai.NewMegacloudExtractor(cfg.MegacloudExtractor.URL, cfg.MegacloudExtractor.Timeout)
    animeKaiProvider, err := animekai.New(animekai.Deps{
        BaseURL:           cfg.AnimeKai.BaseURL,
        HTTP:              animeKaiBaseHTTP,
        MegacloudExtractor: animeKaiExtractor,
        MalSync:           animeKaiMalsync,
        Cache:             redisCache,
        Log:               log,
    })
    if err != nil {
        log.Fatalw("failed to construct AnimeKai provider", "error", err)
    }
    orchestrator.Register(animeKaiProvider)
    log.Infow("registered provider", "name", animeKaiProvider.Name(), "flag", "SCRAPER_ANIMEKAI_ENABLED=true")
} else {
    log.Infow("AnimeKai provider SKIPPED (flag off)", "flag", "SCRAPER_ANIMEKAI_ENABLED=false")
}

// Phase 19 wiring invariant — adapt the Phase 18 invariant.
expectedProviders := 2
if cfg.AnimeKai.Enabled {
    expectedProviders = 3
}
if got := len(orchestrator.RegisteredProviders()); got != expectedProviders {
    log.Fatalw("Phase 19 wiring invariant broken",
        "got", got, "want", expectedProviders, "flag", cfg.AnimeKai.Enabled)
}
```

**Config addition in `services/scraper/internal/config/config.go`:**

```go
type Config struct {
    // ... existing fields ...
    AnimeKai AnimeKaiConfig
}

type AnimeKaiConfig struct {
    Enabled bool
    BaseURL string
}

// in Load():
cfg.AnimeKai = AnimeKaiConfig{
    Enabled: getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false),  // default FALSE
    BaseURL: getEnv("SCRAPER_ANIMEKAI_BASE_URL", "https://anikai.to"),  // animekai.to 301s here
}
if u := cfg.AnimeKai.BaseURL; u != "" {
    parsed, err := url.Parse(u)
    if err != nil || parsed.Scheme == "" || parsed.Host == "" {
        return nil, fmt.Errorf("invalid SCRAPER_ANIMEKAI_BASE_URL %q", u)
    }
}
```

## Architecture Patterns

### System Architecture Diagram

```
User request: GET /api/anime/{shikimoriID}/scraper/stream?prefer=animekai
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Gateway (8000)  ─►  Catalog (8081)  ─►  Scraper (8088) /scraper/stream   │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼ scraperHandler.GetStream
┌──────────────────────────────────────────────────────────────────────────┐
│ Scraper Orchestrator (unchanged from Phase 18)                            │
│                                                                           │
│   if cfg.AnimeKai.Enabled:                                                │
│     orderedProviders(prefer="animekai") = [animekai, animepahe, gogoanime]│
│   else:                                                                   │
│     orderedProviders(prefer=*) = [animepahe, gogoanime]  ← flag off       │
│                                                                           │
│   for each provider in order:                                             │
│     healthCache.IsHealthy(p.Name())?  ◄── Phase 17 60s gate              │
│       false → metrics.ParserFallbackTotal{from=p, to=next}.Inc(); skip   │
│       true  → provider.GetStream(...)                                     │
│                 if ErrNotFound | ErrProviderDown | ErrExtractFailed:      │
│                   metrics.ParserFallbackTotal{...}.Inc(); continue        │
│                 else: return result                                       │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼ when AnimeKai is selected
┌──────────────────────────────────────────────────────────────────────────┐
│ AnimeKai Provider                                                         │
│   internal/providers/animekai/                                            │
│                                                                           │
│   FindID(AnimeRef):                                                       │
│     1. malsync.Lookup(mal_id, "AnimeKAI")     ← PRIMARY (positive cache)  │
│        if hit:                                                            │
│          extract identifier from response                                 │
│          return identifier                                                │
│        if miss (cached 24h negative):                                     │
│          fall through to fuzzy search                                     │
│     2. Fuzzy search GET /browser?keyword=<title>                          │
│        Jaro-Winkler ≥ 0.85 → return identifier                            │
│                                                                           │
│   ListEpisodes(identifier):                                               │
│     1. GET /watch/<slug>  (slug = identifier or fetched from malsync URL) │
│        extract data-id from <div class="rate-box">                        │
│     2. sidecar.GenerateToken(data-id) → token                             │
│     3. GET /ajax/episodes/list?ani_id=<data-id>&_=<token>                 │
│     4. Parse fragment → []Episode { Number, Token, Category }             │
│     5. Cache 6h: episodes:animekai:<identifier>                           │
│                                                                           │
│   ListServers(epID):                                                      │
│     1. sidecar.GenerateToken(epID) → token                                │
│     2. GET /ajax/links/list?token=<epID>&_=<token>                        │
│     3. Parse fragment → 6 Servers grouped by data-id (sub/softsub/dub)    │
│     4. Filter by Category at handler level                                │
│                                                                           │
│   GetStream(serverID):                                                    │
│     1. sidecar.GenerateToken(data-lid) → token                            │
│     2. GET /ajax/links/view?id=<data-lid>&_=<token>                       │
│     3. sidecar.DecryptKai(response.result) → {url, skip}                  │
│     4. GET <url>  (anikai.to/iframe/<token>) → extract megaup iframe src  │
│     5. sidecar.FetchAndDecryptMega(megaup_url) → {sources, tracks, skip}  │
│     6. Build *domain.Stream { Sources, Tracks, Intro, Outro,              │
│                                Headers={Referer: https://megaup.cc/} }    │
│     7. Cache stream:animekai:<...>  TTL = min(parsedExpiry − 30s, 5min)   │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼ for token-related calls
┌──────────────────────────────────────────────────────────────────────────┐
│ docker/megacloud-extractor sidecar (Node.js)                              │
│   server.js → POST /animekai-token                                        │
│                                                                           │
│   handler(req):                                                           │
│     1. parse op + text + media_url + user_agent                           │
│     2. ensureSpec()  ← lazy webcrack of bundle.js + megaup app.js (6h TTL)│
│     3. dispatch:                                                          │
│          op="generate" → strict_encode(text, spec.generate_token)         │
│          op="decrypt_kai" → decode_iframe_data(text, spec)                │
│          op="fetch_and_decrypt_mega" →                                    │
│             HTTPS GET media_url with UA                                   │
│             decode(response.result, spec)                                 │
│             return parsed JSON                                            │
│     4. on extraction failure: re-run ensureSpec() ONCE and retry the op   │
│     5. on second failure: return 500 with "extractor desynchronized"      │
│                                                                           │
│   Fallback (escape-hatch): return 501 "not implemented"                   │
└──────────────────────────────────────────────────────────────────────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ Frontend EnglishPlayer.vue                                                │
│   - calls scraperApi.getStream(animeId, episode, server, category)        │
│   - receives stream.sources[0].url                                        │
│   - builds proxied URL: /api/streaming/proxy?url=<encoded>&referer=...    │
│   - Video.js + HLS.js plays the stream                                    │
│   - SubtitleOverlay renders tracks[]                                      │
│                                                                           │
│ Source dropdown:                                                          │
│   - shows AnimeKai option ONLY when /scraper/health?include=animekai      │
│     reports it Registered (Phase 17 health snapshot is the truth)         │
└──────────────────────────────────────────────────────────────────────────┘
```

### Pattern §1 — malsync FindID (Go sketch, PRIMARY path because malsync HAS the key)

```go
// Source: derived from services/scraper/internal/providers/animepahe/client.go::FindID.
// Different from gogoanime: malsync IS the primary path for AnimeKai (verified).

func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    // 1. malsync — PRIMARY path
    if ref.MalID != "" {
        id, ok, err := p.malsync.Lookup(ctx, ref.MalID, "AnimeKAI")
        if err == nil && ok {
            p.markStage(health.StageSearch, nil)
            return id, nil
        }
    }
    // 2. Fuzzy search fallback
    if ref.Title == "" {
        err := domain.WrapNotFound(errors.New("no title"), "animekai: no malsync hit + no title")
        p.markStage(health.StageSearch, err)
        return "", err
    }
    return p.fuzzySearch(ctx, ref.Title)
}
```

### Pattern §2 — ListServers parsing (Go sketch)

```go
// Source: live anikai.to/ajax/links/list captured 2026-05-12.

func (p *Provider) ListServers(ctx context.Context, identifier, epToken string) ([]domain.Server, error) {
    token, err := p.extractor.GenerateToken(ctx, epToken)
    if err != nil {
        return nil, domain.WrapProviderDown(err, "animekai: GenerateToken")
    }
    serverURL := fmt.Sprintf("%s/ajax/links/list?token=%s&_=%s",
        p.baseURL, url.QueryEscape(epToken), url.QueryEscape(token))
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
    req.Header.Set("Referer", p.baseURL+"/")
    req.Header.Set("X-Requested-With", "XMLHttpRequest")
    resp, err := p.http.Do(req)
    if err != nil { return nil, domain.WrapProviderDown(err, "animekai: /ajax/links/list fetch") }
    defer drainAndClose(resp.Body)

    var wire struct {
        Status string `json:"status"`
        Result string `json:"result"`  // HTML fragment
    }
    if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&wire); err != nil {
        return nil, domain.WrapExtractFailed(err, "animekai: /ajax/links/list JSON parse")
    }
    if wire.Status != "ok" {
        return nil, domain.WrapExtractFailed(errors.New(wire.Status), "animekai: status not ok")
    }
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(wire.Result))
    if err != nil { return nil, domain.WrapExtractFailed(err, "animekai: links/list HTML parse") }

    var servers []domain.Server
    doc.Find("div.server-items.lang-group").Each(func(_ int, group *goquery.Selection) {
        category := group.AttrOr("data-id", "")
        if category != "sub" && category != "softsub" && category != "dub" { return }
        group.Find("span.server[data-lid]").Each(func(_ int, span *goquery.Selection) {
            lid := span.AttrOr("data-lid", "")
            if lid == "" { return }
            name := strings.TrimSpace(span.Text())
            servers = append(servers, domain.Server{
                ID:   lid,
                Name: fmt.Sprintf("%s (%s)", name, category),
                Type: mapCategory(category),  // helper: sub|softsub → sub, dub → dub
            })
        })
    })
    if len(servers) == 0 {
        metrics.ParserZeroMatchTotal.WithLabelValues("animekai", "server_span").Inc()
        return nil, domain.WrapExtractFailed(errors.New("no servers"), "animekai: zero servers in fragment")
    }
    return servers, nil
}
```

### Pattern §3 — Sidecar `/animekai-token` (JavaScript sketch)

```js
// Source: derived from anipy-cli/scripts/decoder/index.js (key-gen branch).
// REQUIRES `npm install webcrack`.

const { webcrack } = require('webcrack');

let specCache = { generated_at: 0 };

async function fetchBundleURL() {
  const res = await fetchUrl('https://anikai.to/home');
  const m = res.data.match(/src=["']([^"']*\/assets\/build\/[^"']*scripts-[^"']*\.js[^"']*)["']/);
  if (!m) throw new Error('animekai: bundle URL not found in /home');
  // Normalize protocol-relative URLs
  return m[1].startsWith('//') ? 'https:' + m[1] : m[1];
}

async function fetchMegaUpAppURL() {
  // Discover the megaup app.js by following one anikai.to/iframe/<x>; for the
  // spec extractor only, any sample iframe works — fetch a recent one.
  const homeRes = await fetchUrl('https://anikai.to/home');
  const sampleSlug = homeRes.data.match(/href=["']\/watch\/([^"']+)["']/)[1];
  // ... shorter pipeline: pick the first sample anime, run the partial flow up
  // to the /iframe/ response, then regex the megaup.cc iframe src + its app.js
}

async function extractSpec() {
  const bundleURL = await fetchBundleURL();
  const bundleRes = await fetchUrl(bundleURL);
  const cracked = await webcrack(bundleRes.data);
  // Pattern-match the four functions: generate_token, decode_iframe_data,
  // encode, decode. Reuse the heuristics from anipy-cli's index.js:194-228
  // (functions containing `256` → transform; `btoa` → b64url_encode; etc.)
  // EXPECT to need updates: the anipy-cli regex `bundle\.js` is outdated.
  const newSpec = parseFunctions(cracked.code);

  const megaupAppURL = await fetchMegaUpAppURL();
  const megaupRes = await fetchUrl(megaupAppURL);
  const megaupCracked = await webcrack(megaupRes.data);
  const megaupSpec = parseFunctions(megaupCracked.code);

  return { ...newSpec, ...megaupSpec, generated_at: Date.now() };
}

async function ensureSpec() {
  if (Date.now() - specCache.generated_at < 6 * 3600 * 1000) return specCache;
  specCache = await extractSpec();
  return specCache;
}

// Pure-function spec runners (reuse anipy-cli's safe_eval pattern but in JS).
// IMPORTANT: do NOT use eval() with user input. The spec strings come from our
// own webcrack output (trusted boundary). Use Function constructor with a
// fixed environment of helper functions.
function runSpec(expr, n) {
  const helpers = { transform, base64_url_encode, base64_url_decode, reverse_it,
                    substitute, strict_decode, strict_encode };
  return new Function(...Object.keys(helpers), 'n', `return ${expr};`)(...Object.values(helpers), n);
}

async function handleAnimekaiToken(req, res) {
  const body = JSON.parse(req.body);
  try {
    const spec = await ensureSpec();
    switch (body.op) {
      case 'generate':
        res.end(JSON.stringify({ result: runSpec(spec.generate_token, body.text) }));
        break;
      case 'decrypt_kai':
        res.end(JSON.stringify({ result: JSON.parse(runSpec(spec.decode_iframe_data, body.text)) }));
        break;
      case 'fetch_and_decrypt_mega': {
        const mediaRes = await fetchUrl(body.media_url, {
          'User-Agent': body.user_agent || DEFAULT_UA,
          Referer: 'https://anikai.to/',
        });
        const raw = JSON.parse(mediaRes.data).result;
        const decoded = JSON.parse(runSpec(spec.decode, raw));
        res.end(JSON.stringify({ result: decoded }));
        break;
      }
      default:
        res.writeHead(400);
        res.end(JSON.stringify({ error: 'unknown op' }));
    }
  } catch (e) {
    res.writeHead(500);
    res.end(JSON.stringify({ error: e.message }));
  }
}

// ESCAPE-HATCH stub (initial Phase 19 ship):
async function handleAnimekaiTokenStub(req, res) {
  res.writeHead(501);
  res.end(JSON.stringify({ error: 'AnimeKai sidecar not yet converged — carry to v3.1' }));
}
```

### Anti-Patterns to Avoid

- **Do NOT call `enc-dec.app` from anywhere in the codebase**, even as a "temporary bridge". Locked by CONTEXT.md, and that's literally the failure mode that killed Consumet.
- **Do NOT hardcode the kai.json spec** from anipy-cli's repo (provably stale by 14 months). Use the webcrack pipeline OR ship the stub.
- **Do NOT add `chromedp` / `playwright` / `flaresolverr` / `utls` / `tls-client`** to bypass the Cloudflare Turnstile that appears on signed-out pages. The `/ajax/*` endpoints are reachable from plain HTTPS — verified. SCRAPER-FOUND-09 lint will reject the PR otherwise.
- **Do NOT pool the spec cache across goroutines without a mutex** — the Node sidecar is single-threaded by default, but if it's clustered later, the cache becomes a race condition. Wrap in async-mutex or accept eventual consistency on the 6h refresh boundary.
- **Do NOT cache the m3u8 URL past `min(parsed_expiry − 30s, 5min)`** — same TTL formula as Phase 16/18.
- **Do NOT register the provider when the flag is off.** The phase invariant in `main.go` MUST assert exactly 2 providers when flag is off (Phase 18 baseline preserved).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MAL ID → AnimeKai identifier mapping | Curated mapping table | Malsync (HAS `AnimeKAI` key — verified 2026-05-12) + fuzzy fallback | Phase 18 verified malsync has no Gogoanime; that's NOT the case here. Malsync provides clean direct mapping. |
| JavaScript deobfuscation | Custom V8 bytecode disassembler | `webcrack` npm package | Active project (2024-2026 commits), specifically designed for the obfuscation pattern AnimeKai uses |
| RC4 implementation | Hand-rolled key-scheduling + PRGA loop | Node stdlib `crypto.createCipheriv('rc4', key, iv)` | Standard; constant-time concerns don't apply here (we're decrypting bait, not authenticating) |
| HTTP retry+backoff | Custom loop | `domain.BaseHTTPClient` (retryablehttp 1→2→4→8s) | Already wired |
| Per-host rate limit | Channel-based limiter | `domain.BaseHTTPClient.WithPerHostRPS(...)` — Phase 15 standard | Already wired |
| HLS proxy for CORS | New endpoint | Existing `libs/videoutils/proxy.go::ProxyWithReferer` | Just append to `HLSProxyAllowedDomains` |
| Provider failover loop | New orchestrator | `services/scraper/internal/service.Orchestrator.runFailover` | Phase 17 wired; just `orchestrator.Register(provider)` conditional on flag |
| Health probe per stage | New goroutine | `health.ProbeRunner` auto-discovers `RegisteredProviders()` | Auto-picks up the new provider when flag flips on |
| Cookie jar | `map[string]*http.Cookie` | `BaseHTTPClient.Jar()` (publicsuffix-scoped) | Already wired |
| Bug-report UI w/ provider tag | New modal | Existing `ReportButton.vue` already emits provider field | Phase 16 / `SCRAPER-NF-05` |
| Frontend dropdown / store | New component | Existing `EnglishPlayer.vue` + `useWatchPreferences.preferredScraperProvider` | Phase 16 already supports arbitrary string values |

**Key insight:** Even on the converge path, the net-new Go code is small — Anitaku/Gogoanime baseline was ~400 LOC and AnimeKai is comparable. The unique Phase 19 cost is in the **sidecar JS** (webcrack pipeline, spec extraction, three op handlers, spec cache) — probably 200-400 LOC of Node + handling of fragile regex patterns. On the escape-hatch path, the sidecar work is ~30 LOC (one stub handler).

## Runtime State Inventory

This phase is **greenfield additive** (new provider package + new sidecar endpoint + new allowlist entries + new env var + conditional registration in main.go). No renames, no migrations, no string replacements.

Confirming each category:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data (DB rows, collection names, Redis keys keyed by renamed strings) | None | None — cache keys use new namespace `*:animekai:*` |
| Live service config (n8n flows, Datadog tags, etc.) | None | None — no external SaaS config touched |
| OS-registered state (Task Scheduler, systemd, pm2) | None | None — docker-compose only |
| Secrets / env vars | New non-secret env vars `SCRAPER_ANIMEKAI_ENABLED` (default `false`) and `SCRAPER_ANIMEKAI_BASE_URL` (default `https://anikai.to`) | Add to `docker/.env` (gitignored) + `docker/.env.example` |
| Build artifacts / installed packages | New npm dep `webcrack` in `docker/megacloud-extractor/package.json` (only on the converge path) | `make redeploy-megacloud-extractor` rebuilds the image with the new dep |

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `goquery` (Go) | New animekai client HTML scraping | ✓ (already in `services/scraper/go.mod`) | v1.8.x | — |
| `retryablehttp` via `domain.BaseHTTPClient` | Per-host rate limit, 429/5xx backoff | ✓ (Phase 15) | — | — |
| Redis (`libs/cache`) | malsync + episodes + stream URL TTLs | ✓ (running in docker-compose) | matches docker-compose | — |
| `health.AllStages` + `health.ProbeRunner` (in-tree) | Probe auto-discovers new provider | ✓ (Phase 17) | — | — |
| `metrics.ParserFallbackTotal` + `metrics.ParserZeroMatchTotal` (in-tree) | Fallback + zero-match metrics | ✓ (Phase 17) | — | — |
| `libs/videoutils/proxy.go::HLSProxyAllowedDomains` | Append new entries | ✓ | — | — |
| Frontend `bun` + `bunx` | Frontend build per CLAUDE.md | ✓ (Phase 16 baseline) | — | — |
| `make redeploy-scraper` / `make redeploy-megacloud-extractor` / `make redeploy-web` / `make health` | After-update flow | ✓ | — | — |
| Node 20 alpine (Docker base for sidecar) | sidecar runtime | ✓ (current `docker/megacloud-extractor/Dockerfile`) | node:20-alpine | — |
| `webcrack` npm dependency | Convergence path only | ✗ NOT INSTALLED | — | **Fallback: ship the escape-hatch stub (no webcrack install needed).** |
| Live `anikai.to` upstream | Live probe (production) + capture-goldens make target run | ✓ (verified 2026-05-12) — but FORMALLY ANNOUNCED SHUTTING DOWN | n/a | If `anikai.to` goes dark before Phase 19 ships: the phase becomes effectively trivial — both paths still ship (stub OR webcrack pipeline), provider just returns `ErrProviderDown` and orchestrator skips it. No regression for users; flag stays off forever; SCRAPER-KAI requirements all carry to v3.1 with the brand explicitly retired. |
| `api.malsync.moe` | malsync.go primary path | ✓ (verified 2026-05-12; `AnimeKAI` key present) | n/a | If malsync drops the key: provider's malsync.go returns miss; fuzzy search fallback handles it. |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** `webcrack` (escape-hatch path doesn't need it; converge path requires `npm install webcrack` in the sidecar Dockerfile).

## Hostnames to Append to `HLSProxyAllowedDomains`

```go
// libs/videoutils/proxy.go::HLSProxyAllowedDomains — APPEND these (DO NOT touch existing entries):

// Phase 19 — AnimeKai (gated). These entries are appended REGARDLESS of the
// SCRAPER_ANIMEKAI_ENABLED flag — the proxy allowlist is a tier-3 protection
// only; the actual user request never reaches an allowlist check unless the
// provider is registered and selected. Appending eagerly means flipping the
// flag at runtime requires zero proxy changes.
"anikai.to",        // optional — provider home, for poster URL proxying if future
"megaup.cc",        // ALREADY PRESENT in megacloud knownHosts — DO NOT DOUBLE-ADD;
                    //   verify libs/videoutils first. If not present, append.
"pro25zone.site",   // primary m3u8 CDN observed 2026-05-12.
                    //   ROTATING SUBDOMAIN — relies on existing
                    //   strings.HasSuffix(host, "."+allowed) gate.
```

**TBD at impl time** — additional CDN hosts will emerge after testing 5+ different anime + episode + server combinations. The plan must allocate budget for one PR-after-PR addition of CDN hosts as users hit different streams (same pattern as Phase 18 streamhg/earnvids backup CDNs).

**Regression-lock invariant:** Phase 16 + 18 entries (kwik.cx, owocdn.top, uwucdn.top, vibeplayer.site, premilkyway.com, dramiyos-cdn.com, etc.) MUST all still match. Plan adds new entries via `append(HLSProxyAllowedDomains, ...)` — never reorder or remove existing rows. Existing regression test (`TestHLSProxyAllowedDomains_AnimePaheRegressionLocked` and `TestIsHLSDomainAllowed_RotatingSubdomains`) must still pass.

## Common Pitfalls

### Pitfall 1: Building against the static `kai.json` spec
**What goes wrong:** Plan ports anipy-cli's committed `kai.json` directly into the sidecar.
**Why:** That spec is 14 months stale and provably does NOT match the live algorithm (verified 2026-05-12).
**Avoid:** Either ship the escape-hatch stub OR implement the webcrack-based dynamic spec extraction. There is no "just use the static spec" path.
**Warning signs:** First live `/animekai-token` call returns a token that AnimeKai's `/ajax/episodes/list` rejects with `{"status":"error","message":"Unable to read the request."}` — that's the exact response shape for a bad token.

### Pitfall 2: Failing to bind UA across the MegaUp `/media/` fetch + decode
**What goes wrong:** Provider GETs `/media/<id>` with one UA, hands the encrypted result to a separately-spawned sidecar process that uses a different UA → decryption returns garbage.
**Why:** MegaUp's encryption derives the key partly from the requesting UA.
**Avoid:** The sidecar performs BOTH the GET and the decode within a single `/animekai-token op=fetch_and_decrypt_mega` call. UA is consistent by construction.
**Warning signs:** `parser_zero_match_total{provider="animekai",selector="mega_decode_corrupt"}` increments while `op=fetch_and_decrypt_mega` HTTP latency is normal.

### Pitfall 3: Health probe burns budget on a known-broken extractor
**What goes wrong:** Flag flips on; provider is registered; the probe runner exercises 5 stages per provider every 15min. Each AnimeKai probe makes 4 upstream HTTP calls + 2 sidecar calls = ~6 requests per probe × 4 sample anime × ProbeRunner cadence = significant traffic.
**Why:** Phase 17 probe pool default = `DefaultGoldenPool` (5-10 anime per provider).
**Avoid:** When extractor returns 501 (escape-hatch) or `ErrProviderDown`, the probe SHOULD short-circuit at stage_search and not exercise downstream stages. Verify `health.ProbeRunner` honors the early-exit pattern (it should, based on Phase 17).
**Warning signs:** AnimeKai upstream rate-limits the probe IP after ~12h of probe traffic when the flag is on with a stub sidecar.

### Pitfall 4: Stub sidecar returning 500 instead of 501
**What goes wrong:** Plan implements the escape-hatch stub returning HTTP 500 — orchestrator's failover logic treats it as `ErrProviderDown` (transient) → continually retries on every request.
**Why:** Phase 15 orchestrator distinguishes:
  - 5xx → `ErrProviderDown` → soft retry then skip
  - 501 → "not implemented" → can be mapped to `ErrExtractFailed` (don't retry) at the embed-extractor wrapper level
**Avoid:** Return **HTTP 501** with `{"error":"AnimeKai sidecar not yet converged — carry to v3.1"}`. Provider's `megacloud_extractor.go` maps `501` → `domain.ErrProviderDown` once with a clear log warning, but the in-memory healthCache flips to 0 after 3 consecutive 501s → probe deregisters → no more retries.
**Warning signs:** `parser_request_duration_seconds{provider="animekai",operation="get_stream"}` p99 > 8s when the sidecar always returns 501 → orchestrator isn't short-circuiting.

### Pitfall 5: Frontend dropdown shows "AnimeKai" when flag is off
**What goes wrong:** Frontend hardcodes the source list `['animepahe', 'gogoanime', 'animekai']`; the third option always shows even with flag off.
**Why:** Plan adds a static `<option value="animekai">` to the dropdown without gating.
**Avoid:** Frontend MUST query `GET /api/anime/{id}/scraper/health` and render dropdown options from the `RegisteredProviders` field in the response. The orchestrator-level snapshot is the single source of truth.
**Warning signs:** Users see "AnimeKai" option but selecting it returns 503 / falls back to next provider silently.

### Pitfall 6: Cloudflare Turnstile gate appears mid-deployment
**What goes wrong:** Today `/ajax/*` is reachable; tomorrow AnimeKai's frontend adds Turnstile validation to those endpoints.
**Why:** Turnstile is opt-in per-endpoint at AnimeKai's discretion.
**Avoid:** Provider's HTTP client checks the response body for `<title>Just a moment...</title>` or `cf-mitigated:challenge` headers and surfaces a clean `ErrProviderDown("animekai: cloudflare challenge")`. Do NOT attempt headless bypass.
**Warning signs:** First-deploy-week → 100% `ErrProviderDown` from animekai while AnimePahe + Gogoanime remain healthy.

### Pitfall 7: Pollution of `parser_requests_total{provider="animekai"}` when flag is OFF
**What goes wrong:** Provider is conditionally registered, but a stale metric label from a previous flag-on deploy lingers; alert fires "AnimeKai serving when flag is off!"
**Why:** Prometheus retains labels forever once seen.
**Avoid:** Add a CI/staging test: after `SCRAPER_ANIMEKAI_ENABLED=false` boot, `curl /metrics | grep 'provider="animekai"'` returns ZERO active rows (or returns rows whose values are flat zero since boot — both are acceptable; the alert SQL must compare a delta over a window, not a raw scalar).
**Warning signs:** Success criterion 4 ("flag-off → parser_requests_total{provider="animekai"} stays flat-zero for 7 days") fails on day 1 because the metric was non-zero on the previous deploy and Prometheus shows a non-flat history.

### Pitfall 8: Webcrack output is non-deterministic across versions
**What goes wrong:** Sidecar passes its CI with webcrack v2.34, deployer runs `npm install webcrack@latest` which pulls v3.0 with renamed pass behavior, the function-extraction heuristics misclassify, all token generation fails after deploy.
**Why:** Webcrack is actively developed; major versions change normalization behavior.
**Avoid:** Pin webcrack to exact patch version in `package.json` (no `^` prefix). Add a CI smoke test: feed a captured fixture bundle, assert the four extracted expressions equal known-good strings.
**Warning signs:** Sidecar logs `extractSpec: no match for generate_token` after a routine `npm update`.

### Pitfall 9: Test fixtures relying on the live algorithm
**What goes wrong:** Goldens capture a real AnimeKai `/ajax/episodes/list` response with embedded `&_=<token>` URLs. Algorithm rotates; the captured token no longer reproduces, but the captured response replays cleanly. Tests pass while production breaks.
**Why:** Mocking the upstream by replaying its body is fine, but assertions about token contents are tied to the live algorithm.
**Avoid:** Tests assert on the SHAPE of the request/response (e.g. "_= is non-empty and base64-url-safe") and on the parsed result (episode list parsed correctly). Do NOT assert specific token values.

## Code Examples

See "Architecture Patterns" section above for Go and JavaScript sketches (FindID via malsync, ListServers parsing, sidecar `/animekai-token` endpoint).

Additional anchor — **MegaUp `/media/` response DTO shape** (live capture 2026-05-12):

```json
{
  "status": 200,
  "result": {
    "sources": [
      {"file": "https://rrr.pro25zone.site/p5rm/c6/h1ca52877.../list,WuxrIQT2oR6y.m3u8"}
    ],
    "tracks": [
      {"file": "https://5rm.megaup.cc/v5/bHKUod1G9wxK.../thumbnails.vtt", "kind": "thumbnails"}
      // For episodes with English captions, expect:
      // {"file": "https://...vtt", "label": "English", "kind": "captions", "default": true}
    ],
    "download": "https://megaup.cc/download/0sj0L2C0WSyJcOLyGbxK5xvtDg"
  }
}
```

Note: this particular episode (Attack on Titan Final Chapter Special 2 — `dYW-8w`, `e9fh8_jxtw26iG9ez4yX`, server `dIKy8K6l4Q`) had NO English captions track. Other episodes do (verified via MegaUp.kt buildSubtitleTracks). The sidecar MUST handle the empty-captions case gracefully.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `animekai.to` as canonical mirror | `anikai.to` (301-redirect destination) | Sometime between site launch and 2026-05-12 | Provider must follow redirects; both URLs work with the same backend, the brand kept |
| AnimeKai using `bundle.js` (anipy-cli reference) | AnimeKai using Vite-built `scripts-<hash>.js` | At some point post-2025-04-23 | Webcrack regex `bundle\.js` is broken; new extractor must match `scripts-` prefix |
| Aniyomi `MegaUpInterceptorOld.kt` (headless WebView) | Aniyomi `MegaUp.kt` (enc-dec.app) | Recent (committed 2026) | Headless is no longer used because mobile users can't run it efficiently; enc-dec.app is the lazy escape; our project rejects that escape |
| AnimeKai operational | AnimeKai officially shutting down | **2026-05-10** (2 days before this research) | Strong pressure to take the escape hatch |
| Provider sequencing AnimePahe → 9anime | Provider sequencing AnimePahe → Gogoanime (Phase 18 pivot) → AnimeKai (Phase 19 gated) | 2026-05-12 | Per CONTEXT.md D5: registration order = failover order |
| malsync's `Sites` lacking 9anime/Gogoanime keys (Phase 18 RESEARCH finding) | malsync HAS `AnimeKAI` keys (this RESEARCH finding) | 2026-05-12 (verified across multiple sample MAL IDs) | Phase 19's `FindID` uses malsync as PRIMARY path (unlike Phase 18's fuzzy-primary path) |

**Deprecated/outdated:**
- The CONTEXT.md S1 wording "Reference Phase 16 HiAnime + megacloud-extractor wiring as the closest analog for the in-house token generation pattern" — the Phase 16 megacloud-extractor handles HiAnime/Zoro-flavor megacloud which is **a different encryption scheme** (cinemaxhq/keys AES-256-CBC vs AnimeKai's RC4-chain). Don't blindly copy; the patterns differ on the cryptography side.
- CONTEXT.md S5 "Use the docker/megacloud-extractor/patch-aniwatch.sh script as a reference for in-cluster token extractor maintenance" — that script is for the aniwatch (HiAnime) container's runtime patching of its embedded npm package; AnimeKai's sidecar work is purely additive to `server.js`, no node_modules patching needed.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `anikai.to` remains the canonical alive AnimeKai mirror through Phase 19 ship date | Live Pipeline Verification | The site is in an announced wind-down. Migration to a successor mirror, if any, is uncertain. Mitigation: `SCRAPER_ANIMEKAI_BASE_URL` env var enables hot-swap. |
| A2 | The `/ajax/episodes/list`, `/ajax/links/list`, `/ajax/links/view` endpoints will continue to accept the `_=<token>` URL param shape | Live Pipeline Verification | Low — endpoint shape predates the algorithm and is part of the public scraping surface. If they change, ALL existing 3rd-party providers break simultaneously. |
| A3 | The MegaUp `/media/<id>` endpoint continues to require the request UA to match the decryption UA | Pitfall 2 | Verified by `enc-dec.app/api/dec-mega` rejection behavior. Mitigation already baked into Pattern §3 design. |
| A4 | `webcrack` continues to deobfuscate AnimeKai's `scripts-<hash>.js` shape | Token Generation Topology | Webcrack is actively maintained but adversarial obfuscators evolve. A breaking obfuscator change means all-providers-using-this-pattern (anipy-cli, etc.) break at the same time. Plan: pin exact webcrack version; ship Telegram alert on extractor failure. |
| A5 | Cloudflare Turnstile remains absent from `/ajax/*` endpoints | Pitfall 6 | LOW — confirmed today. If turned on, fallback is `ErrProviderDown` and the orchestrator routes to gogoanime/animepahe. |
| A6 | malsync.moe will continue to ship the `AnimeKAI` key for sampled MAL IDs through Phase 19 ship | Pattern §1, malsync.go | The site's announced shutdown may eventually cause malsync to drop the key, but malsync historically retains dead-site mappings for months. If dropped, fuzzy fallback handles it. |
| A7 | The `pro25zone.site` HLS host remains the active CDN through Phase 19 ship | Hostnames | LOW — same kind of rotating CDN as Phase 18 streamhg/earnvids; we may need to append more entries to `HLSProxyAllowedDomains` as users hit different episodes. Documented as TBD in the hostnames section. |
| A8 | `SCRAPER_ANIMEKAI_ENABLED` defaults to false in production (per CONTEXT.md D1 + ROADMAP success criterion 1) | Escape Hatch path | LOCKED — REQUIREMENTS.md text says `docker compose restart catalog` but the actual restart target is `scraper`. Plan corrects this. |
| A9 | The committed anipy-cli `kai.json` is provably stale | Convergence Probability + Common Pitfalls | Verified empirically — token lengths differ by 6.5x. Strong evidence the algorithm has rotated. |
| A10 | The Phase 18 `services/scraper/internal/fuzzy/` package was extracted (or NOT) — confirm at plan time | Architectural Responsibility Map | Phase 19 should use whatever Phase 18 ended up with: if extracted, reuse; if not, duplicate privately. No new refactor risk introduced by Phase 19. |

**Per the agent role policy, no claim above is tagged `[ASSUMED]` in the strict sense** — every assumption is empirically tied to a 2026-05-12 live probe or to a documented stale-spec data point. Assumptions are about *persistence over time* on a site that has formally announced wind-down. If an item flips at impl time, the planner re-runs the relevant probe.

## Open Questions (RESOLVED + REMAINING)

### Resolved

1. **Is animekai.to alive right now?**
   - **RESOLVED:** Yes. `animekai.to` 301-redirects to `anikai.to`; the destination is Cloudflare-200, no challenge, full HTML body served. **BUT** an explicit shutdown banner is shown on every page since 2026-05-10.

2. **What's the current MegaUp token generation algorithm?**
   - **RESOLVED (algorithm shape):** `strict_encode` (per-byte modular arithmetic + base64url) over the input for URL-param `_`; `transform` (RC4 chain over 3 hardcoded keys) + base64url + substitute + reverse for KAI and MEGA decryption. **NOT RESOLVED (live values):** the exact spec must be re-extracted from the live `scripts-<hash>.js` and `megaup.cc/<app>.js` via webcrack at each rotation. Static spec is stale.

3. **Can the token gen be implemented in pure JS inside docker/megacloud-extractor/?**
   - **RESOLVED:** Yes, but it requires `npm install webcrack` (only on the converge path). The escape-hatch path needs no new dep.

4. **What embed types does animekai.to dispatch to?**
   - **RESOLVED:** Two-hop: `anikai.to/iframe/<token>` → `megaup.cc/e/<id>` → `megaup.cc/media/<id>` (encrypted JSON). All servers share this dispatch path. **No multi-host embed extractor registry is needed for AnimeKai** — single endpoint `/animekai-token` handles all variants.

5. **What HLS CDN hostnames need to be added to the allowlist?**
   - **PARTIALLY RESOLVED:** Verified `pro25zone.site` as the m3u8 CDN, `5rm.megaup.cc` as the thumbnails subdomain (covered by existing `megaup.cc` entry). Additional CDN hosts will surface as users hit different episodes — plan must allocate budget for incremental CDN additions.

6. **What's the realistic convergence probability (1 week R&D)?**
   - **RESOLVED:** ~25%. See Convergence Probability Assessment section. Strong recommendation to take the escape-hatch path.

7. **Does malsync.moe expose `AnimeKAI` keys?**
   - **RESOLVED:** YES. Verified across sample MAL IDs (16498 = Attack on Titan returns `{Sites: {AnimeKAI: {<identifier>: {identifier, image, malId, title, url}}}}`). Unlike Phase 18's pivot, AnimeKai's `FindID` is malsync-PRIMARY.

8. **Does AnimeKai sit behind DDoS-Guard?**
   - **RESOLVED:** No. Plain Cloudflare 200 with any UA; no `__ddg2_` cookie. Skip ddosguard.go for this provider (consistent with Phase 18 Anitaku).

### Remaining (deferred to plan time)

1. **What's the EXACT pattern signature for `webcrack`-extracted functions in the current `scripts-<hash>.js`?**
   - Plan time: capture the live bundle, run webcrack manually, run anipy-cli's `find_all_functions` regex; document required heuristic changes vs anipy-cli's `index.js:194-228`.

2. **What's the megaup app.js URL pattern today?**
   - Plan time: capture a live iframe response, grep for `<script src="*">` to discover the megaup app.js path.

3. **Does the AnimeKai upstream rate-limit aggressively?**
   - Plan time: run a 30-minute probe loop and observe whether 429s appear at 1 RPS, 5 RPS. Adjust `WithPerHostRPS("anikai.to", ...)` accordingly.

4. **What does `data-eid` mean?**
   - Plan time: appears identical across all 2 servers in a group (e.g. both servers in `data-id="sub"` show `data-eid="c4O596Gp"`). Probably the "encrypted episode ID" backing the server list. Likely unused — the `data-lid` is what `/ajax/links/view` consumes. Confirm by inspection.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `goldie/v2` for golden files (already in tree per Phase 15 `SCRAPER-FOUND-07`) + Node stdlib `node:test` for sidecar tests (already used by megacloud-extractor smoke tests, if any) |
| Config file | None — Go test discovery + `testdata/<provider>/` convention |
| Quick run command | `cd /data/animeenigma/services/scraper && go test ./internal/providers/animekai/... -count=1 -short -timeout=60s` |
| Full suite command | `cd /data/animeenigma/services/scraper && go test ./... -count=1 -race -timeout=180s` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCRAPER-KAI-01 | `FindID` malsync-PRIMARY returns identifier for known MAL ID via golden; 24h positive cache; fuzzy fallback on malsync miss | unit | `go test ./internal/providers/animekai -run TestFindID_MalsyncPrimary -v` | ❌ Wave 0 |
| SCRAPER-KAI-01 (cont.) | malsync miss → fuzzy fallback uses Jaro-Winkler ≥ 0.85 | unit | `go test ./internal/providers/animekai -run TestFindID_FuzzyFallback -v` | ❌ Wave 0 |
| SCRAPER-KAI-02 | `ListEpisodes` calls sidecar token then `/ajax/episodes/list` then parses `div.eplist ul.range li > a[num][slug][langs][token]` from golden HTML fragment | unit | `go test ./internal/providers/animekai -run TestListEpisodes_ParseFragment -v` | ❌ Wave 0 |
| SCRAPER-KAI-02 (cont.) | `langs` decoding: "1" → CategorySub, "2" → CategoryDub, "3" → both surfaced | unit | `go test ./internal/providers/animekai -run TestListEpisodes_LangsDecode -v` | ❌ Wave 0 |
| SCRAPER-KAI-03 | `ListServers` parses 6 servers across 3 lang-groups (sub/softsub/dub × 2 servers each) from golden | unit | `go test ./internal/providers/animekai -run TestListServers_SixServersFromFragment -v` | ❌ Wave 0 |
| SCRAPER-KAI-04 (stub path) | sidecar returns 501 → provider returns ErrProviderDown | unit | `go test ./internal/providers/animekai -run TestGetStream_SidecarStub -v` | ❌ Wave 0 |
| SCRAPER-KAI-04 (converge path, IF taken) | sidecar 200 → provider returns playable Stream with Sources, Tracks, Headers={Referer: megaup.cc} | unit | `go test ./internal/providers/animekai -run TestGetStream_HappyPath -v` | ❌ Wave 0 |
| SCRAPER-KAI-04 (converge path, IF taken) | Stream TTL = `min(parsedExpiry − 30s, 5min)` | unit | `go test ./internal/providers/animekai -run TestGetStream_TTL -v` | ❌ Wave 0 |
| SCRAPER-KAI-05 | `SCRAPER_ANIMEKAI_ENABLED=false` → provider NOT registered, exactly 2 providers in orchestrator | unit | `go test ./cmd/scraper-api -run TestPhase19_FlagOff_TwoProviders -v` | ❌ Wave 0 |
| SCRAPER-KAI-05 (cont.) | `SCRAPER_ANIMEKAI_ENABLED=true` → provider registered, exactly 3 providers | unit | `go test ./cmd/scraper-api -run TestPhase19_FlagOn_ThreeProviders -v` | ❌ Wave 0 |
| SCRAPER-KAI-06 | (escape-hatch only) Phase 19 ships with 501 sidecar; provider returns ErrProviderDown without crashing | smoke | `make redeploy-scraper && curl http://localhost:8088/scraper/health \| jq '.providers' \| grep -i animekai` (when flag is on in staging) | ❌ Wave 0 |
| SCRAPER-KAI-07 | (converge path only) Three-provider failover: forcing animepahe + gogoanime health to 0 produces playable stream from animekai; `parser_fallback_total{from="gogoanime",to="animekai"}` increments | unit | `go test ./internal/service -run TestOrchestrator_GogoanimeToAnimeKaiFailover -v` | ❌ Wave 0 |
| Phase 17 cross-cutting | Health probe loops `RegisteredProviders()` and exercises animekai per stage WHEN flag is on | integration | `go test ./internal/health -run TestProbeRunner_DiscoverAnimeKai -v -race` | ❌ Wave 0 |
| Phase 18 regression | After Phase 19 ship, AnimePahe + Gogoanime tests still green | regression | `go test ./internal/providers/animepahe ./internal/providers/gogoanime -run TestFindID -v` | ✅ exists (Phase 18) |
| Phase 18 regression | After Phase 19 ship, HLS proxy allowlist regression test for Phase 16/18 hosts still green | regression | `go test ./libs/videoutils -run TestHLSProxyAllowedDomains -v` | ✅ exists (Phase 18) |
| After-update | `make lint` + `make health` + `make redeploy-megacloud-extractor` all succeed | integration | `make lint && make redeploy-megacloud-extractor && make health` | ✅ exists |
| Sidecar test | `POST /animekai-token` with op=generate returns a non-empty result | node test or curl smoke | `curl -X POST -d '{"op":"generate","text":"sample"}' http://localhost:3200/animekai-token` | ❌ Wave 0 |
| Metric invariant | `SCRAPER_ANIMEKAI_ENABLED=false` for 7 days → `parser_requests_total{provider="animekai"}` is flat-zero | manual / Grafana | dashboard query | ❌ runtime (post-ship) |

### Sampling Rate

- **Per task commit:** `cd /data/animeenigma/services/scraper && go test ./internal/providers/animekai/... -count=1 -short -timeout=60s`
- **Per wave merge:** `cd /data/animeenigma/services/scraper && go test ./... -count=1 -race -timeout=180s`
- **Phase gate:** Full suite green + `make lint` green + `make health` shows 2 providers (flag off) OR 3 providers (flag on, even though animekai returns ErrProviderDown on every call). On the converge path, also `curl http://localhost:8088/scraper/health | jq '.providers.animekai.stages | map(.up == 1) | all'` returns `true`.

### Wave 0 Gaps

Wave 0 must scaffold the following BEFORE provider impl plans execute:

- [ ] `services/scraper/internal/providers/animekai/client_test.go` — covers SCRAPER-KAI-01, -02, -03 against goldens (stub-path) and SCRAPER-KAI-04 with stubbed sidecar
- [ ] `services/scraper/internal/providers/animekai/dto_test.go` — pure parser tests for episode + server fragments
- [ ] `services/scraper/internal/providers/animekai/malsync_test.go` — positive cache + miss + fuzzy fallback paths
- [ ] `services/scraper/internal/providers/animekai/megacloud_extractor_test.go` — sidecar HTTP wrapper tests (mocks 200, 500, 501, timeout)
- [ ] `services/scraper/testdata/animekai/` directory with goldens: `watch_page.html`, `episode_fragment.html`, `server_fragment.html`, `kai_decrypted.json`, `mega_decrypted.json`, `malsync_aot.json`
- [ ] `services/scraper/cmd/scraper-api/main_test.go` — TestPhase19_FlagOff_TwoProviders + TestPhase19_FlagOn_ThreeProviders
- [ ] `docker/megacloud-extractor/server.test.js` (or smoke curl script) — sidecar endpoint stub test (501 path) and happy path (if converge)
- [ ] `make capture-goldens-animekai` Makefile target (mirror Phase 18 pattern)

*(No existing test infrastructure gap beyond these net-new files.)*

## Security Domain

`security_enforcement` is enabled by default (no `false` override observed). Applicable categories for this phase:

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | No | No new user-facing auth surface; the gateway's existing JWT/API-key auth is unchanged. |
| V3 Session Management | No | Same as V2. |
| V4 Access Control | Partial | The `/api/anime/{id}/scraper/stream` endpoint requires the same access controls as Phase 18. Flag-off means animekai is silently skipped — no new authz surface. |
| V5 Input Validation | YES | The sidecar `/animekai-token` endpoint MUST validate: (a) `op` is in `{generate, decrypt_kai, fetch_and_decrypt_mega}`, (b) `text` is < 4KB, (c) `media_url` is a URL whose host is `megaup.cc` or its known subdomains (NOT user-supplied for arbitrary URL fetching — SSRF risk if user could spoof the host). The provider's `megacloud_extractor.go` enforces the host constraint client-side too. |
| V6 Cryptography | NO | The sidecar consumes Node stdlib `crypto.createCipheriv('rc4', ...)`. We do NOT hand-roll RC4 / AES. The cryptographic primitives are operationally weak (RC4 is broken, AES-128-CBC is vulnerable to padding oracles in some contexts) but acceptable because the encryption is bait — we're decrypting, not protecting our own data. ASVS V6 does not apply to bait decryption. |
| V12 File and Resources | YES | The sidecar's `fetch_and_decrypt_mega` op fetches a remote URL on user behalf — classic SSRF surface. Host allowlist in the provider's wrapper + duplicate allowlist in the sidecar (defense in depth). Plan must reject any `media_url` whose host is not `megaup.cc` or `*.megaup.cc`. |

### Known Threat Patterns for {scraper service + sidecar Node.js}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SSRF via `/animekai-token op=fetch_and_decrypt_mega` accepting user URL | Tampering / Information Disclosure | Strict host allowlist (`megaup.cc` + suffix match). Reject any other host. |
| Webcrack pulled as transitive dep with a compromise (npm supply chain attack) | Tampering | Pin exact version; CI integrity check; `npm audit` in CI |
| Sidecar leaking the spec cache via debug log when DEBUG=true | Information Disclosure | The decoded specs are PUBLIC (anyone can webcrack the live bundle); leaking them is not a security boundary. Log freely; no PII risk. |
| Sidecar burning RAM holding 6h-cached webcrack output | DoS | Webcrack output is ~50-100KB per bundle × 2 bundles × 1 cache slot = ~200KB. No risk. |
| AnimeKai upstream serving malicious payload to the sidecar (DOM XSS into the spec extractor) | Tampering | We never execute the spec — we parse expressions and bind them to a fixed function table via `new Function(...args, ...body)`. Untrusted spec strings cannot escape via the helpers (RC4, base64, substitute, reverse). Audit at plan time: no `eval()` of webcrack output anywhere. |

## Sources

### Primary (HIGH confidence)

- **Live HTTP probes against `anikai.to`** (this RESEARCH, 2026-05-12) — direct evidence of endpoint shapes, response bodies, encryption pipeline, shutdown banner, Cloudflare behavior
- **`enc-dec.app` API responses** (this RESEARCH, 2026-05-12) — used as the encryption oracle to verify the pipeline end-to-end (NOT the deliverable — research signal only)
- **`api.malsync.moe/mal/anime/16498`** (this RESEARCH, 2026-05-12) — direct evidence that `AnimeKAI` key exists and maps to `{identifier, url, ...}`
- **anipy-cli source (`scripts/decoder/index.js` on `key-gen` branch + `api/src/anipy_api/provider/providers/animekai_provider.py`)** — the only public in-house token generation reference; reveals the webcrack + pattern-match approach
- **anipy-cli `scripts/decoder/generated/kai.json`** — the stale committed spec (commit 2025-04-23); empirically confirms algorithm has rotated since (HIGH-confidence evidence, NEGATIVE finding)
- **Kohi-den/extensions-source `AnimeKai.kt` + `MegaUp.kt`** (latest on `main`) — confirms the 5-step pipeline shape and confirms ALL public Aniyomi-family providers use enc-dec.app
- **REQUIREMENTS.md, ROADMAP.md, CONTEXT.md, Phase 16/18 RESEARCH.md** (in-tree) — locked decisions and existing patterns to mirror

### Secondary (MEDIUM confidence — multiple sources cross-verified)

- **Multiple 2026-05-10 news reports** of AnimeKai shutdown: Digital Trends, CBR, OtakuKart, Distractify, IMDB News, FandomWire, PiunikaWeb, Al Bawaba (8 independent outlets confirming the announcement)
- **`docker/megacloud-extractor/server.js`** existing pattern for the new endpoint shape

### Tertiary (LOW confidence — single source, used as background only)

- **walterwhite-69/AnimeKAI-API `app.py`** — useful for the endpoint URL constants (`ANIMEKAI_EPISODES_URL` etc.) and confirms enc-dec.app dependency; cannot be used as the in-house algorithm reference because it itself depends on enc-dec.app

## Metadata

**Confidence breakdown:**

- Endpoint shapes + selectors + DTO shapes: **HIGH** — live verified end-to-end on 2026-05-12
- Token generation algorithm shape (strict_encode + transform/RC4 + base64url + substitute + reverse): **HIGH** — sourced from anipy-cli's `scripts/decoder/index.js`
- Token generation algorithm exact spec for the live site: **MEDIUM** — must be re-extracted via webcrack at impl time; the committed `kai.json` is stale
- Convergence probability over 1 week R&D: **MEDIUM** — estimate is informed but inherently uncertain; depends on webcrack heuristics surviving the next obfuscator rotation
- Long-term viability of the upstream: **LOW-MEDIUM CONFIDENCE in negative direction** — formal shutdown announcement 2026-05-10 is strong signal, but the site is still serving today

**Research date:** 2026-05-12
**Valid until:** 7 days for the convergence probability estimate (the upstream is wind-down — situation changes fast); 30 days for the endpoint shapes and architectural recommendations
