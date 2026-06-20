# Stealth Browser-Scraper Framework — migrate curl scrapers → real browser engine

**Status:** PLAN (awaiting engine decision)
**Author:** AI + @0neymik0
**Date:** 2026-06-20
**First target:** gogoanime → megaplay (`cdn.mewstream.buzz`). Generalize after.

---

## 1. Problem (correct framing)

Our scrapers are **naïve HTTP clients** (`p.http.Get(...)` + a `Referer` header). They
present:

- a Go `net/http` **TLS fingerprint** (JA3/JA4) and **HTTP/2 settings** fingerprint that
  scream "bot" — nothing like a real Chrome/Firefox handshake;
- **no JavaScript engine** → cannot run a Cloudflare / DDoS-Guard / Turnstile interstitial,
  cannot produce a `cf_clearance` cookie;
- **no browser fingerprint** (canvas/WebGL/audio/fonts/`navigator.*`), no cookies, no
  history, no behavioural signal.

`cdn.mewstream.buzz` (megaplay's HLS origin, shared by gogoanime **and** nineanime) sits
behind Cloudflare's managed challenge. A `curl`/Go request gets a 403 "Attention Required"
**regardless of exit IP** — residential, datacenter, or WARP. **The IP is not the binding
constraint; the client identity is.** Swapping IPs alone does not solve a challenge that
expects a real browser to execute JS and carry a clearance cookie.

We already proved the browser approach works once: `services/animepahe-resolver/` is a
warm puppeteer-extra-stealth Chromium sidecar that beats DDoS-Guard on `animepahe.pw`.
This plan **generalises that one-off into a reusable framework** and migrates providers
onto it, gogoanime first.

> Memory note: do **not** record "Cloudflare blocks our IP" as the root cause — that is the
> misanalysis. Root cause = curl-class client cannot pass a browser challenge.

---

## 2. Engine choice — Playwright vs Camoufox vs (current) puppeteer-extra-stealth

| Dimension | **Playwright (Chromium) + playwright-extra/stealth** | **Camoufox (Firefox, anti-detect fork)** | puppeteer-extra-stealth (current sidecar) |
|---|---|---|---|
| Stealth technique | JS-patch evasions (detectable by advanced WAFs) | **C++/Gecko-level** fingerprint spoofing — no JS patches to detect; BrowserForge fingerprint injection built in | JS-patch evasions (oldest, most-fingerprinted) |
| Cloudflare managed-challenge pass rate (2026) | Medium — Chromium is the most-profiled engine | **High** — Firefox + native spoofing is currently least-detected | Medium-low |
| Network interception (capture getSources XHR + m3u8) | **Excellent** (`route`/`fulfill`, response bodies, CDP) | Good (Firefox devtools proto, less ergonomic) | Good (CDP) |
| Multi-browser / future flex | Chromium/Firefox/WebKit | Firefox only | Chromium only |
| Proxy + per-context geo/locale/timezone | First-class per-`BrowserContext` | **First-class, purpose-built** (proxy+geoIP+locale auto-aligned) | Per-launch only |
| Ecosystem / maintenance | Large, MS-backed; `playwright-extra` lags upstream a bit | Smaller, fast-moving, single-maintainer risk; Python-first (Node via server/CDP) | Mature but stealth plugin maintenance is slowing |
| Resource footprint | Chromium ~heavy | Firefox ~heavy, slightly more RAM | Chromium ~heavy |
| Reuse of our existing sidecar | Rewrite (puppeteer→playwright is mechanical) | Rewrite + new runtime | Already here |

### Recommendation — **engine-abstracted, Playwright primary, Camoufox as "hard mode"**

Build the framework around a small **`Engine` interface** (`newContext`, `resolve`,
`teardown`) and ship **two implementations**:

1. **Playwright + Chromium + playwright-extra-stealth** as the default — best network
   interception (we need to *capture* the getSources JSON + `master.m3u8` the page itself
   fetches, not re-request them), biggest ecosystem, easy proxy/context-per-session.
2. **Camoufox** as a per-provider opt-in for upstreams that defeat Chromium stealth (likely
   mewstream/Cloudflare). Selected by a provider config flag, not a rewrite.

This hedges single-engine risk and lets us A/B block-rates per upstream. **Do not** keep
extending puppeteer — the user's instinct to move off it is correct (it's the most-profiled
stack and its stealth plugin is decaying). We migrate `animepahe-resolver` onto the same
framework later (it becomes one more provider profile).

**Decision needed from owner:** confirm "Playwright primary + Camoufox hard-mode", or pick a
single engine. Phase 1 code differs only in the engine impl behind the interface.

---

## 3. Framework architecture — `services/stealth-scraper/` (Node + Playwright)

A standalone sidecar on the internal Docker network (mirrors animepahe-resolver's shape:
Fastify + warm browser pool + Prometheus). The Go `scraper` service calls it over HTTP.

```
services/stealth-scraper/
├── server.js            # Fastify: POST /resolve, GET /healthz, /metrics
├── engine/
│   ├── index.js         # Engine interface + registry
│   ├── playwright.js    # Chromium + playwright-extra-stealth impl
│   └── camoufox.js      # Camoufox impl (hard mode)
├── tunnels/
│   ├── pool.js          # ProxyPool: select/stick/rotate/health-score
│   └── providers.js     # residential / mobile / datacenter / warp / direct entries
├── fingerprint/
│   ├── generate.js      # BrowserForge/fingerprint-suite → per-profile fp
│   └── extensions.js    # curated extension-set randomiser (see 3.3)
├── profiles/            # persistent per-profile user-data-dirs (cookies/history/storage)
│   └── manager.js       # aged-profile pool: lease, warm, retire
├── warming/
│   └── warm.js          # session-aging routine (fake history / GA cookies)
├── recipes/
│   └── gogoanime.js     # provider-specific in-browser resolution script (Phase 1)
└── Dockerfile
```

### 3.1 Tunnels (pluggable IP proxy) — `tunnels/`

- **`ProxyPool`** holds typed entries: `{ id, type: residential|mobile|datacenter|warp|direct,
  url: 'http://user:pass@host:port', geo: 'DE', healthScore, lastBlockedAt }`.
- **Selection policy** per resolve request: prefer least-recently-blocked of the requested
  type; **sticky per profile/session** (a Cloudflare clearance is IP-bound — see §3.6).
- **Rotation**: on a detected challenge-fail, mark the entry blocked, rotate to the next,
  retry up to N. Health-score decays block rate so dead exits drop out.
- **Geo alignment**: proxy `geo` drives the browser `timezone`, `locale`, `Accept-Language`,
  and `navigator.language` so the fingerprint is internally consistent (a German IP with a
  `America/New_York` clock is an instant tell).
- Config-driven: proxy endpoints live in `docker/.env` (secrets) → injected; **never** in git.
  Start with `direct` + one residential gateway; add mobile later.

### 3.2 Fingerprint generation — `fingerprint/generate.js`

- Use **BrowserForge / fingerprint-suite** to generate a coherent fingerprint per profile:
  UA + `navigator` props + screen/viewport + canvas/WebGL/audio noise + font list + hardware
  concurrency + device memory — all **mutually consistent** and matching the engine's real
  build (UA must match the actual binary or it's a tell).
- One fingerprint is **pinned per persistent profile** (not regenerated per request) — humans
  don't change their canvas hash between page loads.

### 3.3 Random extensions — `fingerprint/extensions.js`

The user asked for "random browser extensions for fingerprint". Two sub-options, with a
recommendation:

- **(a) Real loaded extensions** (uBlock Origin / Dark Reader / consent-auto-dismiss / a
  randomised benign subset). *Pro:* realistic — most humans run extensions; adblock also
  strips ad-decoy/interstitial noise (helps the AnimeFever-class ad-substitution problem too).
  *Con:* extensions **add** detectable surface (web-accessible resources, injected DOM) and
  Chromium headless+extensions is fiddly; the current sidecar deliberately uses
  `--disable-extensions`.
- **(b) Fingerprint injection only** (no real extensions; spoof the fingerprint surface via
  BrowserForge). *Pro:* lower detection surface, deterministic. *Con:* less "human texture".

**Recommendation:** default to **(b) injection** for the fingerprint, and load a **small
curated real-extension set (adblock + consent-dismiss)** for *functional* reasons (cleaner
pages, fewer decoys), randomising only within that safe set. Treat "random arbitrary
extensions" as experimental behind a flag — variety there tends to *raise* detectability,
not lower it.

### 3.4 Session aging / fake history — `warming/warm.js` + `profiles/`

The user asked for "fake browser history to build up Google Analytics and other spying stuff"
so sessions look like aged humans:

- **Persistent profiles** (`launchPersistentContext(userDataDir)`) accumulate cookies,
  `localStorage`, IndexedDB, and history across runs.
- A **warming routine** drives each fresh profile through a realistic browsing pass *before*
  it ever touches a target: a Google search, a couple of YouTube/news/Wikipedia visits, dwell
  + scroll + mouse movement, organic navigation to the anime site. This seeds `_ga`/`_gid`/
  `NID`/consent cookies and a plausible history so anti-bot heuristics that weight
  "cookie age / GA presence / referrer chain" see a human, not a cold bot.
- **Aged-profile pool**: maintain K warmed profiles; lease one per resolve, return it, retire
  after M uses or on block. Amortises both warming cost and challenge-solving cost.
- Each profile is **sticky to one proxy IP** (clearance binding, §3.6).

### 3.5 In-browser resolution + interception (the actual scrape)

Instead of re-fetching URLs with curl, **drive the real page and capture what it loads**:

1. `page.goto(gogoanimes.fi/search...)` → click through to `/category/<slug>` → episode page.
2. Let the page load `newplayer.php` → the nested `megaplay.buzz` iframe → its JS fires
   `getSources`.
3. **Intercept** the `getSources` XHR response (`page.on('response')`) → read the
   `master.m3u8` URL directly from the JSON the browser received. No second request, no
   data-id reverse-engineering, no broken HEAD probe.
4. Optionally fetch the `master.m3u8` (and one segment) **in-page** to confirm playability and
   to mint the clearance cookie against the CDN.

### 3.6 ⚠️ Clearance binding — the make-or-break integration detail

A Cloudflare `cf_clearance` cookie is bound to **(exit IP, User-Agent)**. So:

- It is **not enough** to solve the challenge in the sidecar and hand a bare `master.m3u8`
  back to the Go HLS proxy — the proxy egresses from our datacenter IP with a Go UA and gets
  re-challenged on the playlist + segments.
- The sidecar must return a **stream session**:
  `{ masterURL, variantURLs, cookies: {cf_clearance,...}, userAgent, proxyId, expiresAt }`.
- `libs/videoutils/proxy.go` (streaming) must, for these sessions, **egress through the same
  `proxyId` and replay the same cookie + UA** when fetching the playlist and the rotating
  `.ts` segments. This means teaching the HLS proxy to route a flagged session via a named
  upstream proxy + cookie jar.

This is the largest cross-service change and the highest-risk part. Phase 1 validates it
end-to-end on gogoanime before we generalise.

### 3.7 Other anti-detection measures worth adding

- **TLS (JA3/JA4) + HTTP/2 fingerprint**: free win — a real browser handshake is the whole
  point of moving off Go `net/http`.
- **Human behaviour emulation**: bezier mouse paths, scroll, randomized dwell/typing cadence
  (e.g. `ghost-cursor`).
- **Turnstile/CAPTCHA fallback**: integrate a solver (CapSolver/2captcha) only when a profile
  hits an interactive challenge the stealth context can't auto-clear; last resort, costed.
- **Clearance/cookie cache**: persist solved `cf_clearance` per (host, proxyId) in Redis with
  TTL; reuse across requests to avoid re-solving every time.
- **Block-rate telemetry**: per-upstream/per-proxy/per-engine challenge + block counters to
  Prometheus → a Grafana panel; drives auto-rotation and tells us when an engine decays.
- **Concurrency/RAM budget**: warm-context pool with a hard RSS cap (animepahe-resolver caps
  500 MB); queue resolves, recycle contexts (Pitfall 4/6 from Phase 27 RESEARCH still apply).
- **Defence-in-depth host allowlist** per recipe (the animepahe sidecar's T-27-01-01 invariant):
  a recipe may only `goto` its declared host set — no user-controlled navigation (SSRF).

---

## 4. Phase 1 — gogoanime only (build target)

**Goal:** real users can play gogoanime EN streams again, resolved through a browser engine,
with clearance-bound segment delivery.

1. **Scaffold `services/stealth-scraper/`** — Fastify + chosen engine, warm-context pool,
   `/healthz` `/metrics`, Dockerfile, docker-compose service (internal-only), RSS cap.
2. **`tunnels/pool.js`** — ProxyPool with `direct` + one residential entry from `docker/.env`;
   sticky-per-profile + rotate-on-block.
3. **`fingerprint/` + `profiles/` + `warming/`** — generated fingerprint per persistent
   profile; a small warmed-profile pool; basic warming pass (Google + 2 sites). Curated
   adblock+consent extension (flagged).
4. **`recipes/gogoanime.js`** — full in-browser chain (search → category → episode →
   newplayer.php → megaplay) with `getSources` interception → returns the stream session
   (§3.6 shape).
5. **`POST /resolve`** contract: `{ provider:'gogoanime', mal/shikimori id, episode, category }`
   → stream session JSON. JSON-schema validated; host-allowlisted.
6. **Go scraper integration** — new gogoanime stream path that calls the sidecar (mirror
   `providers/animepahe/resolver.go`); behind `SCRAPER_GOGOANIME_ENGINE=browser|http` so we
   can dark-ship and fall back.
7. **HLS proxy clearance binding** (`libs/videoutils/proxy.go` + streaming) — route flagged
   sessions through the bound `proxyId` + replay cookie/UA for playlist & segments.
8. **Verify end-to-end**: `/scraper/stream` for a known title returns non-empty url; the
   playlist + a real segment return 200 through the proxy; canary `stream_segment` flips
   green for gogoanime; play in-browser.

**Interim mitigation (ship immediately, independent of this build):** mark gogoanime (and
nineanime — same megaplay CDN) `status=degraded` via the AUTO-484 pattern so users stop being
routed to a dead provider while the framework is built. (Owner to confirm — see open question.)

## 5. Phase 2+ — unify

Migrate providers onto the framework as recipes: nineanime (same megaplay recipe, near-free),
animefever (adblock extension may also defeat the ad-decoy), then fold `animepahe-resolver`
in as a Camoufox/Playwright recipe and retire the bespoke sidecar. Each recipe = one file +
one config row.

---

## 6. Risks & cons (honest)

- **Heavyweight**: real browsers cost RAM/CPU vs curl. Mitigate with warm-pool + caching +
  resolve-once-then-cache-the-session.
- **Clearance binding (§3.6)** is genuinely hard and touches the streaming hot path. Highest
  risk; gate Phase 1 on it.
- **Arms race**: Cloudflare adapts. Engine abstraction + telemetry + Camoufox hard-mode are
  the hedge; expect periodic recipe maintenance (like STEALTH-PINS.md).
- **Residential proxy cost/ToS** + the legality posture of aging fake profiles — consistent
  with existing project practice (we already run a stealth sidecar) but worth the owner's
  explicit sign-off.
- **Single-maintainer risk** on Camoufox; mitigated by it being the *secondary* engine.

---

## 7. Effort metrics (per `.planning/CONVENTIONS.md`)

- **Phase 1 (gogoanime):** UXΔ = +3 (Better) — restores a primary EN provider to actually-plays.
  CDI = 0.06 * 34 (new sidecar service + cross-service streaming-proxy change — broad spread,
  big shift). MVQ = Kraken 80%/75% (many tentacles: browser pool, proxy, clearance binding).
- **Framework generalisation (Phase 2+):** UXΔ = +2 (Better). CDI = 0.03 * 21. MVQ = Griffin
  85%/80%.

---

## 8. Open decisions for owner

1. **Engine:** Playwright-primary + Camoufox-hard-mode (recommended) — or single engine?
2. **Interim:** degrade gogoanime + nineanime now while the framework is built?
3. **Residential proxy:** which provider/budget? (need a real endpoint to test the recipe).
4. **Extensions:** injection-only (recommended) vs real random extension set?
