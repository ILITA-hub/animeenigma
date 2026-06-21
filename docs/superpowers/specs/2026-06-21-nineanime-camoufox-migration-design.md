# nineanime → Camoufox browser migration — design

**Date:** 2026-06-21
**Status:** design (awaiting review)
**Scope:** Migrate the **nineanime** EN scraper provider onto the Camoufox stealth-scraper
framework so it returns real streams again. allanime is a **separate follow-up** (needs
R&D — see "Out of scope"). animepahe stays on its existing `animepahe-resolver` + Kwik.

Owner directive: *"make Go logic for everyone, python only execution"* — all
parse/decrypt/discovery logic stays in Go; the Python sidecar (Camoufox) is a pure
execution layer (browser fetch + player interception). This is **Approach 2** (transport
swap), chosen over porting provider logic into Python recipes.

---

## 1. Problem (live-verified, 2026-06-21)

nineanime is registered + enabled (`stream_providers.engine='http'`) but returns **no real
streams in production**. Root cause, confirmed from inside the scraper container:

- **Discovery is dead.** `GET https://9anime.me.uk/series/<slug>/` → *context deadline
  exceeded* (the whole site sits behind a DDoS-Guard/JS challenge that hangs a plain Go
  `net/http` client). FindID then negative-caches the ID, so every later lookup short-circuits
  to not-found. `9anime.me.uk/wp-json/...` times out the same way.
- **The stream leg needs JS too.** Upstream migrated its popular catalog to
  `megaplay.buzz` (`Server: cloudflare`, stream computed by player JS). The in-process
  pure-Go megaplay "recording" extractor can't execute that JS → 403 / stale segments.

So unlike gogoanime (whose `gogoanimes.fi` discovery is reachable in plain Go and only the
megaplay *stream* needs the browser), **nineanime is gated at *both* discovery and stream.**

### Spike evidence (Camoufox, in the deployed container)

- Navigating `https://9anime.me.uk/` in Camoufox → **200, real title** ("9anime - Watch
  Anime online…") — the challenge is solved by the browser.
- In-page `fetch` of `…/wp-json/wp/v2/search?search=frieren` → **200 JSON**, returns the
  *Frieren S2* series + slug.
- `…/series/frieren-beyond-journeys-end-season-2/` → **200, 119 KB of real HTML**.
- megaplay stream interception is **already proven working** for gogoanime (same player).

→ nineanime is fully migratable to the browser with high confidence.

---

## 2. Architecture (Approach 2 — Go logic, Python execution)

Two capabilities in `services/stealth-scraper`:

1. **Resolve recipe** *(exists)* — navigate a JS player, intercept `getSources` +
   `master.m3u8`, retain a session, restream segments. Used for the **megaplay stream leg**.
2. **Browser-fetch primitive** *(new `POST /fetch`)* — fetch an allowlisted URL through a
   warm, challenge-solved session and return the **raw body**. Carries **gated discovery**
   (9anime WP-REST JSON + series/episode HTML + the `my.1anime.site` iframe page). This is
   the same pattern the existing `animepahe-resolver` already uses (warm the page past the
   challenge, then proxy fetches), generalized into Camoufox using the in-page-fetch
   machinery we already ship (`_in_page_fetch` — APIRequestContext/curl get 403'd; only
   in-page fetch clears the WAF).

The Go provider keeps **every** parse line; when `engine=browser` it swaps its HTTP
transport to the sidecar.

### 2.1 New sidecar endpoint `POST /fetch`

```
POST /fetch  { provider, url, method="GET", headers?{}, body? }
→ 200       { status, headers{}, body }            # raw passthrough (no m3u8 rewrite)
→ 403       { kind:"challenge" }                   # challenge survived rotation
→ 502/504   { kind:"error"|"timeout" }
```

- **Session model:** the engine keeps an internal warm session **keyed by
  `(provider, origin)`**, transparent to Go (Go does NOT manage `sid` for discovery). First
  call for an origin warms it (navigate origin → solve challenge); later calls reuse cookies
  via in-page fetch. Sliding TTL + the existing reaper evict idle sessions.
- **SSRF:** the `url` host must pass the recipe's `allowed_hosts` for `provider` (reuse
  `recipes.base.host_allowed`) **and** the hardened `host_allowed_for_session` family
  (https-only, no private/loopback). No new third-party CDN is added to the videoutils
  allowlist — discovery hosts are provider-owned and recipe-allowlisted.
- **Reuses:** `_in_page_fetch_js` (body cap), `FetchTimeout`→504, `PoolExhausted`→503,
  `looks_like_challenge`→rotate. GET first; POST is wired for future allanime.

### 2.2 Go sidecar client

Add to `services/scraper/internal/sidecar/client.go`:

```go
// Fetch routes one upstream HTTP request through the sidecar's warm browser
// session for `provider` and returns the raw body + status.
func (c *Client) Fetch(ctx, provider, rawURL, method string, headers map[string]string, body []byte) (status int, respBody []byte, err error)
```

Same `kind`→error mapping as `ResolveEmbed` (404→`ErrNotFound`, 5xx/transport→`ErrProviderDown`).

### 2.3 nineanime Go provider (surgical)

The provider already funnels **all** discovery through one helper, `httpGetBody(ctx, url,
max)` (used by `FindID` search, `ListEpisodes` series page, `GetStream` episode + iframe
pages), and already has a `Megaplay domain.EmbedExtractor` field + an iframe-host branch.

- **Discovery:** when `browserEnabled()`, `httpGetBody` calls `sidecar.Fetch("nineanime", url)`
  instead of `p.http.Do`. One change, all gated GETs covered; parsers untouched.
- **Stream (`GetStream`), branch on iframe host (unchanged shape):**
  - `1anime.site` / `megaplay.buzz` / `vidwish.live` → `BrowserResolve(ctx, iframeURL,
    category)` → sidecar `/resolve` with `provider="nineanime"` (resolve recipe). Replaces
    the broken pure-Go megaplay extractor when `browserEnabled()`.
  - `my.1anime.site` (legacy MP4) → `httpGetBody` (now browser-routed) fetches the iframe
    HTML, existing regex extracts the MP4, play via the **signed** streaming proxy.
- **Wiring:** add `UseBrowser func() bool` + `BrowserResolve BrowserResolveFunc` to
  `nineanime.Deps`; in `main.go` mirror gogoanime:
  `EngineOf("nineanime")==EngineBrowser` and `stealthClient.ResolveEmbed(…, "nineanime", …)`.
  Keep the existing pure-Go path intact as the `engine=http` fallback (a DB flip, not a deletion).

### 2.4 Recipe

Register `"nineanime"` in the engine `_recipes`. Because Go always supplies the megaplay
`embed_url`, the nineanime recipe needs only the **embed_url branch** of the existing
gogoanime `resolve()` plus its own `allowed_hosts`
(`9anime.me.uk, 1anime.site, megaplay.buzz, vidwish.live, mewstream.buzz, lostproject.club`).

- **Minimal:** `class NineAnimeRecipe(GogoanimeRecipe)` overriding `name` + `allowed_hosts`
  (the megaplay interception, `_embed_to_player`, `_await_*`, `parse_getsources`,
  `_probe_master` are reused as-is; the search path is never taken).
- **Optional cleanup (recommended):** factor a `MegaplayRecipe` base holding the shared
  megaplay logic; `GogoanimeRecipe` and `NineAnimeRecipe` subclass it (animefever can join
  later). Keeps one copy of the interception code.

### 2.5 Database

After live validation, set `stream_providers.engine='browser'` for nineanime (idempotent
UPDATE; the Go-embedded seed + guarded migration follows the existing pattern). Set
`base_url='https://9anime.me.uk'` if not already implied by config.

---

## 3. Rollout & safety (validate-then-flip)

1. Build + deploy **stealth-scraper** (`/fetch` + nineanime recipe) and **scraper** (Fetch
   client + nineanime seam) from a clean origin/main worktree.
2. **Validate live before flipping:** with `engine` still `http`, drive a real resolve
   through the sidecar for a known title (Frieren S2 ep 1) and confirm a playable
   `master.m3u8`.
3. Flip `engine='browser'` for nineanime; watch the probe dashboard +
   `parser_zero_match_total{provider="nineanime"}` and the sidecar metrics
   (`stealth_active_sessions`, `stealth_pool_exhausted_total`).
4. Rollback = flip `engine='http'` (instant, no redeploy).

---

## 4. Testing

- **Python:** `/fetch` unit tests — SSRF guard (off-allowlist host, private IP, http→reject),
  session-reuse per origin, GET passthrough, body cap, challenge→rotation, timeout→504,
  pool-exhausted→503. nineanime recipe `allowed_hosts` + embed_url resolve (FakePage).
- **Go:** `sidecar.Client.Fetch` (success / transport→down / 404→notfound / kind surfacing);
  nineanime `httpGetBody` browser-routing when `browserEnabled()`; `GetStream` host-branch
  routing (megaplay→BrowserResolve, my.1anime→fetch+extract). No live-API tests (mock).

---

## 5. Scoring (project convention — no time units)

- **UXΔ = +3 (Better)** — restores a dead EN provider (real streams instead of failover-to-nothing).
- **CDI = 0.03 × 13** — small surface spread (one new sidecar endpoint + one Go helper swap +
  a thin recipe), low coherence shift (mirrors the existing gogoanime seam), Effort_Fib 13.
- **MVQ = Griffin 85%/80%** — a clean, reusable primitive that also unblocks allanime/animefever.

---

## 6. Out of scope (explicit)

- **allanime** — separate follow-up. Spike proved GraphQL **discovery** migrates (in-page
  fetch from the `allmanga.to` origin beats the WAF), but **stream resolution is blocked**:
  the top sources are `/apivtwo/clock?id=…` paths and that endpoint is dead from the browser
  (CORS `NetworkError` on fetch, 403 on navigate), the Go client skips clock today, and the
  `allmanga.to` SPA didn't auto-resolve a stream in the spike. allanime needs a focused R&D
  step (drive the real SPA to click-play and capture the exact request it makes) **before**
  a recipe is committed.
- **animepahe** — stays on `animepahe-resolver` (DDoS-Guard) + in-process Kwik.
- No changes to the videoutils HLS-proxy allowlist (streams stay signed).

---

## 7. Risks

- **9anime HTML shape drift** — mitigated: the existing Go parsers are unchanged and the
  `parser_zero_match_total{selector="my_1anime_iframe"}` maintenance signal still fires.
- **Session memory** — each warm discovery session is a browser profile; the pool is capped
  (`STEALTH_POOL_SIZE`) and the reaper evicts idle ones. Discovery sessions are short-lived
  and reused across a title's calls.
- **CORS on cross-origin discovery** (`my.1anime.site` iframe fetched from a 9anime session)
  — avoided by keying `/fetch` sessions per-origin (a `my.1anime.site` fetch warms its own
  same-origin session). Confirm in the build-time spike.
