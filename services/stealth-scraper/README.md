# stealth-scraper

Camoufox-based **browser** scraping sidecar. Resolves provider stream sources by
driving a real anti-detect Firefox (Camoufox) through the provider's player
chain — so we pass the JS / fingerprint / TLS / clearance challenges that a
curl-class Go `net/http` client cannot.

**Phase 1 target:** `gogoanime → megaplay` (`cdn.mewstream.buzz`, Cloudflare-fronted).
Internal-only service; the Go `scraper` is the only caller. Engine = **Camoufox**
(chosen 2026-06-20). Plan: `docs/superpowers/plans/2026-06-20-stealth-browser-scraper-framework.md`.

> Why not "just use a residential proxy with curl": the binding constraint is the
> **client identity** (TLS/JA3 + HTTP/2 fingerprint, no JS engine, no
> `cf_clearance`), not the exit IP. Swapping IPs fixes one signal; the challenge
> fails on the rest. A real browser fixes all of them at once.

## HTTP contract

```
GET    /healthz        → {status, pool_size, live_browsers, active_sessions, proxies[...]}
GET    /metrics        → Prometheus
POST   /resolve        → resolve a stream session (retains a browser session)
GET    /hls?sid=&url=  → MANDATORY stream proxy: fetch playlist/segment via the
                         session's clearance-bearing browser context
DELETE /session/{sid}  → release a session's browser/profile
```

### `POST /resolve`

Request:
```jsonc
{
  "provider": "gogoanime",
  "title": "One Piece",        // or "keyword"
  "episode": 1100,
  "category": "sub",           // sub | dub
  // OR skip discovery with a known episode page:
  "episode_url": "https://gogoanimes.fi/one-piece-episode-1100",
  "proxy_type": "residential"  // optional tunnel preference
}
```

Success (`200`):
```jsonc
{
  "success": true,
  "data": {
    "session_id": "ab12…",                              // ← use for /hls + DELETE
    "master_url": "https://cdn.mewstream.buzz/anime/.../master.m3u8",
    "playlist_proxy_path": "/hls?sid=ab12…&url=<master>", // ← play THIS (host-prefixed)
    "referer": "https://megaplay.buzz/",
    "subtitles": [{ "url": "...vtt", "label": "English", "default": true }],
    "intro": { "start": 0, "end": 130 },
    "outro": { "start": 0, "end": 0 },
    "cdn_probe_status": 200,
    "cookies": [{ "name": "cf_clearance", "value": "...", "domain": "...", "path": "/" }],
    "user_agent": "Mozilla/5.0 (...) Firefox/...",
    "proxy_id": "res-de",
    "cdn_host": "cdn.mewstream.buzz",
    "resolved_via": "camoufox",
    "expires_at": 1750000000
  }
}
```

Failures: `404` (`not_found`), `502` (`challenge` — all exits blocked / `error`),
`500` (`internal`).

> **⚠️ Clearance binding — now handled IN this sidecar (mandatory).** A
> `cf_clearance` cookie is bound to **(exit IP, User-Agent)**, so the playlist +
> every `.ts` segment must be fetched through the **same** browser context that
> resolved the session. `GET /hls` does exactly that (via the session's
> Playwright `APIRequestContext`) and rewrites playlists so child URIs route back
> through it. A consumer plays `playlist_proxy_path`, NOT the raw `master_url`.
> This keeps the whole flow inside "scrapers" — the player/streaming HLS proxy is
> untouched for now.

## Architecture

| Module | Role |
|---|---|
| `app/engine.py` | Warm browser pool, recipe execution, **challenge → rotate exit IP** retry loop, cookie harvest. |
| `app/tunnels.py` | `ProxyPool`: typed exits, sticky-per-session, rotate-on-block, health scoring. |
| `app/profiles.py` | Persistent aged identities (own `user_data_dir`, pinned fingerprint, sticky proxy). |
| `app/fingerprint.py` | Camoufox launch options; per-profile OS/fingerprint seed; proxy→Playwright dict. |
| `app/warming.py` | Session aging — pre-browse Google/YouTube/etc. so profiles look human. |
| `app/recipes/gogoanime.py` | The gogoanime→megaplay chain + pure parsers (`search_keywords`, `parse_getsources`). |
| `app/main.py` | FastAPI server. |

Camoufox sets its fingerprint at **launch** (patched Firefox), so identity is
per-profile, not per-context. We pin one fingerprint per persistent profile and
rotate by leasing a different profile. Cookies persist in the profile's context
jar across resolves → clearance is reused, not re-solved every time.

## Configuration (env)

| Var | Default | Notes |
|---|---|---|
| `PORT` | `3000` | |
| `STEALTH_POOL_SIZE` | `2` | concurrent browser profiles |
| `STEALTH_PROFILE_DIR` | `/data/profiles` | persistent profiles (mount a volume) |
| `STEALTH_HEADLESS` | `true` | |
| `STEALTH_GEOIP` | `true` | align locale/timezone/WebRTC to proxy exit |
| `STEALTH_HUMANIZE` | `true` | human-like cursor |
| `STEALTH_OS_ROTATE` | `windows,macos,linux` | per-profile OS fingerprint |
| `STEALTH_BLOCK_RESOURCES` | `image,font,media` | bandwidth + fingerprint surface |
| `STEALTH_PROXIES` | `""` | JSON array `[{"id","type","url","geo"}]` — **secret, via docker/.env** |
| `STEALTH_WARP_PROXY_URL` | `""` | e.g. `socks5://warp-proxy:1080` |
| `STEALTH_MAX_PROXY_RETRIES` | `2` | exit rotations per resolve on challenge |
| `STEALTH_WARMING_ENABLED` | `false` | session aging on cold profiles |

**No provider envs/consts.** Provider config (`base_url`, `engine`, status, …)
lives in the DB roster table `scraper_providers` (catalog domain; new `engine` +
`base_url` columns) and is passed per-request by the Go scraper as `base_url`.
The former `SCRAPER_GOGOANIME_BASE_URL` env is retired.

Phase 1 ships with a **placeholder proxy slot**: `direct` (+ optional WARP). Add a
residential endpoint to `STEALTH_PROXIES` — **required** to actually clear the
challenge (see below).

## Run on the server (no Docker yet)

The Camoufox Firefox binary is installed on the server in a venv:

```bash
cd services/stealth-scraper
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt && python -m camoufox fetch   # already done on this host
uvicorn app.main:app --host 0.0.0.0 --port 3000
```

## Deploy (compose snippet — wired in the integration workstream)

```yaml
  stealth-scraper:
    build: ./services/stealth-scraper
    environment:
      STEALTH_POOL_SIZE: "2"
      STEALTH_PROXIES: ${STEALTH_PROXIES:-}
      STEALTH_WARP_PROXY_URL: ${STEALTH_WARP_PROXY_URL:-}
    volumes:
      - stealth_profiles:/data/profiles
    # internal network only — NOT gateway-exposed
    expose: ["3000"]
    mem_limit: 1500m
```

## Tests

Pure-logic + the async recipe chain (fake page) run with **zero third-party deps**:

```bash
cd services/stealth-scraper && python3 -m unittest discover -s tests -v
```

**Verified locally:** 28 unit tests (proxy pool selection/rotation/sticky/cooldown,
keyword + getSources parsing, host-allowlist guard, challenge detection, the full
recipe happy-path + search/challenge/not-found paths via a fake page, and the HLS
playlist rewriter). Camoufox launches headless on the server and spoofs Firefox
135/Windows (live smoke OK).

**⚠️ Key finding (2026-06-20):** even a real Camoufox browser (JS executing, 7s
solve wait) gets a **403 Cloudflare "Attention Required" from our datacenter IP**
on `cdn.mewstream.buzz`. That is a WAF/IP-reputation block on the datacenter ASN,
not a JS challenge a browser can auto-solve. So:
- curl + any IP → fails (no browser identity)
- **browser + datacenter IP → fails** (WAF blocks the ASN) ← proven
- browser + residential/mobile IP → the combination that should work

End-to-end gogoanime is therefore **gated on a residential exit** in
`STEALTH_PROXIES`. The browser is necessary but not sufficient alone.

## Not yet done (next workstreams)

1. **Go scraper integration** — call this sidecar for gogoanime streams behind a
   `SCRAPER_GOGOANIME_ENGINE=browser|http` flag (dark-ship + fallback).
2. **HLS-proxy clearance binding** (`libs/videoutils/proxy.go`) — route flagged
   sessions through the bound `proxy_id` + replay cookie/UA for playlist+segments.
3. **Generalize** — nineanime (same megaplay recipe), animefever, then fold in
   animepahe as another recipe and retire the bespoke puppeteer sidecar.
