# 2026 EN-Source Survival Sweep

**Date:** 2026-05-19
**Requirement:** SCRAPER-HEAL-26
**Phase context:** `.planning/phases/26-provider-expansion/26-CONTEXT.md` D2 (operator decision gate)
**Probe egress:** production server (animeenigma.ru) — same IP as the scraper container.

## Purpose

Following the v3.0 Phase 20 EN-tab removal and the v3.1 Phase 24 reconnect
of gogoanime/animepahe (currently both in `SCRAPER_DEGRADED_PROVIDERS`),
this sweep evaluates every 2026 EN-source candidate worth considering for
the scraper failover pool. Each candidate gets a verdict (`live | dead |
uncertain`), an anti-bot posture summary, a recommendation
(`worth-implementing | needs-deeper-PoC | not-worth`), and — if recommended
— an effort estimate. Per CONTEXT.md D2, the operator picks 0–2 survivors
at the Decision Gate at the bottom; we do NOT pre-commit to implementing
every live candidate.

## Methodology

Each candidate was probed twice with `curl --max-time 10` from the production
server. First a `HEAD /` probe with a Chrome desktop UA to surface
Cloudflare / Turnstile / IP-block evidence in headers and cookies; then a
`GET /` (or where applicable a documented JSON endpoint) with the same UA
to read 200–600 bytes of body content for keyword signals (seizure
notices, "goodbye" messages, JS-challenge HTML, real anime listings).

No headless browser, no JS execution, no IP rotation. This matches the
scraper's CI rejection list (`chromedp`, `go-rod`, `utls`, `tls-client`,
`cloudscraper_go`, `flaresolverr`) — anything that requires those tools
is automatically `not-worth` for the same reason gogoanime mirrors got
flagged earlier this milestone.

## Candidate Pool

- **Miruro** — https://www.miruro.tv — referenced widely as a 2026 ani-cli
  alternative; reportedly uses upstream embed extractors via a proxy.
- **AnimeOwl** — multiple legacy domains; reportedly seized 2025.
- **AnimeFever** — https://animefever.cc — listed in scattered 2026 forum
  threads as still operational.
- **AniWatchTV** — historic aniwatch.to / aniwatchtv.to; rumoured shut by
  USTR action in March 2026.
- **HiAnime** — v3.0 Phase 20 removed the parser when hianime.to died;
  multiple TLDs (.nz / .io / .so / .tv) re-evaluated here.
- **Crunchyroll-Free** — legal/free tier of crunchyroll.com; integration
  shape verdict only.

## Verdict Matrix

| Candidate | Status | Anti-Bot | Recommendation | Effort (days) | Notes |
|---|---|---|---|---|---|
| Miruro | live | Cloudflare (managed) | needs-deeper-PoC | 5-7 | Frontend is a SPA; documented API endpoints all return `{"error":"Gone"}`; real backend is proxied via `pro.ultracloud.cc`. PoC requires reverse-engineering the obfuscated proxy URL (VITE_PROXY_OBF_KEY) before any scraper work. |
| AnimeOwl | dead | n/a | not-worth | n/a | `animeowl.cc` returns an "explainer / safe alternatives" page (no API). `animeowl.live` returns ad-redirect script. Legacy `.tv`/`.net`/`.ru` do not resolve. The site as a media source is gone. |
| AnimeFever | live | Cloudflare (passive) | needs-deeper-PoC | 4-6 | Returns full HTML pages; no JSON API documented; would require HTML scraping. PHPSESSID cookie set on first request. Worth evaluating IF operator wants an HTML-scraping path. |
| AniWatchTV | dead | n/a | not-worth | n/a | `aniwatchtv.to` and `aniwatch.to` time out / return 404 from prod IP via Cloudflare. Consistent with the March 2026 USTR seizure claim (no positive confirmation of seizure notice body since DNS/CF tier returns 404 before any response). |
| HiAnime | dead | n/a | not-worth | n/a | `hianime.to` / `hianime.tv` / `hianime.pe` / `hianime.bz` do not resolve from prod. `hianime.nz` is alive but the body content is the literal goodbye message "It's time to say goodbye." `hianime.io` is an unrelated generic aggregator site. `hianime.so` is a near-empty redirect-style placeholder. None usable as a v3.1 EN provider. |
| Crunchyroll-Free | live (legal) | Cloudflare Challenge + login wall | not-worth | n/a | Public root returns HTTP 403 with `cf-mitigated: challenge` + a JS challenge page. The free tier additionally requires a logged-in cookie + ad-supported playback. Integration shape does not match the scraper's open-public-API pattern. Legal status is the disqualifier, not technical — using Crunchyroll content without an authorized API key would violate ToS. |

## Per-Candidate Detail

### Miruro

```
$ curl --max-time 10 -sI -A 'Mozilla/5.0 ... Chrome/126' https://www.miruro.tv/ | head -5
HTTP/2 200
date: Tue, 19 May 2026 06:19:12 GMT
content-type: text/html; charset=UTF-8
server: cloudflare
cf-ray: 9fe0fd1d09f1be58-HAM

$ curl --max-time 10 -sS 'https://www.miruro.tv/api/anime/list?page=1'
{"error":"Gone"}

$ curl --max-time 10 -sS 'https://www.miruro.tv/api/anime/search?q=frieren'
{"error":"Gone"}

$ curl --max-time 10 -sS 'https://www.miruro.tv/env2.js'
window.env=JSON.parse("{\"VITE_ANILIST_CLIENT_ID\":\"18233\",\"VITE_ANILIST_REDIRECT_URI\":...
\"VITE_PIPE_OBF_KEY\":\"71951034f8fbcf53d89db52ceb3dc22c\",
\"VITE_PROXY_A\":\"https://pro.ultracloud.cc/\",
\"VITE_PROXY_B\":\"https://pru.ultracloud.cc/\",
\"VITE_PROXY_OBF_KEY\":\"a54d389c18527d9fd3e7f0643e27edbe\"}");

$ curl --max-time 10 -sI 'https://pro.ultracloud.cc/'
HTTP/2 404
server: cloudflare
```

**Status:** live (frontend SPA loads), but the documented `/api/anime/*`
paths all return `{"error":"Gone"}` — they've been migrated to obfuscated
URLs through `pro.ultracloud.cc` (a Cloudflare-fronted proxy) with two
obfuscation keys (`VITE_PIPE_OBF_KEY` and `VITE_PROXY_OBF_KEY`). The
literal proxy host itself returns 404 on the root — endpoints exist but
are only addressable through the obfuscation transform.

**Anti-Bot:** Cloudflare managed challenges (`cf-mitigated` not set on
frontend, but `cf-ray` and `cf-cache-status` present). Frontend doesn't
trigger a challenge from prod IP; the proxy host's behaviour under load
is unknown.

**Recommendation:** `needs-deeper-PoC`. Implementing requires:

1. Reverse-engineering the JS bundle to extract the obfuscation function
   (likely an HMAC-SHA256 or AES-CTR transform keyed on the two `OBF_KEY`s).
2. Replicating the transform in Go to construct valid proxy URLs.
3. Verifying the proxy returns playable HLS / MP4 sources for those URLs
   from the prod IP (Cloudflare might TLS-fingerprint scrape clients).

This is borderline crossing into the anti-bot territory we explicitly
reject. The obfuscation isn't a security mechanism, but the proxy might
TLS-fingerprint at request time, which would force `utls`. **Operator
should treat this as the highest-risk pick — likely 1-2 days of spike
work before committing to a full provider lift.**

### AnimeOwl

```
$ curl --max-time 10 -sI 'https://animeowl.cc/' | head -3
HTTP/2 200
content-type: text/html
server: cloudflare

$ curl --max-time 10 -sS 'https://animeowl.cc/' | head -3
<!doctype html>
<html lang="en">
<head>
  <title>AnimeOwl – Watch Anime with online for FREE, Updates & Safe Alternatives (2025)</title>
  <meta name="description" content="AnimeOwl.cc is an informational website explaining what happened to AnimeOwl, sharing updates, FAQs, and safe legal alternatives for anime fans.">

$ for d in animeowl.tv animeowl.com animeowl.net animeowl.ru animeowl.live; do
   echo "$d -> $(curl --max-time 5 -sI -o /dev/null -w "%{http_code}" -A 'Mozilla/5.0' "https://$d/")"
done
animeowl.tv -> 000
animeowl.com -> 404
animeowl.net -> 000
animeowl.ru -> 000
animeowl.live -> 200  (returns an ad-overlay redirect script — not the original site)
```

**Status:** dead. The `.cc` mirror is an explainer / "safe alternatives"
page, not an anime catalog. The `.live` mirror is a parking domain
running ad-redirect scripts. Legacy `.tv`/`.net`/`.ru` do not resolve.

**Anti-Bot:** n/a — the original AnimeOwl service no longer exists.

**Recommendation:** `not-worth`. There is no provider here to integrate.

### AnimeFever

```
$ curl --max-time 10 -sI 'https://animefever.cc/' | head -8
HTTP/2 200
date: Tue, 19 May 2026 06:19:21 GMT
content-type: text/html; charset=UTF-8
server: cloudflare
set-cookie: PHPSESSID=k2japn454cpvr9h326fdqhufq5; path=/
set-cookie: lcache=deleted; ...
expires: Thu, 19 Nov 1981 08:52:00 GMT
cache-control: no-store, no-cache, must-revalidate, post-check=0, pre-check=0

$ curl --max-time 10 -sS 'https://animefever.cc/' | head -3
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
   <head>
      <title>Animefever - Watch Anime English Subbed & Dubbed Online Free</title>

$ curl --max-time 10 -sS 'https://animefever.cc/search?keyword=frieren' | head -5
(returns the same homepage HTML — search may require POST or session cookie)
```

**Status:** live. The `.cc` domain serves a working HTML site with a
PHP backend (PHPSESSID cookie). The standard `/search?keyword=` GET path
returns the homepage rather than search results — search likely requires
a session cookie and either POST or AJAX.

**Anti-Bot:** Cloudflare passive (no `cf-mitigated`, no JS challenge in
response body). Site responded immediately to prod IP.

**Recommendation:** `needs-deeper-PoC`. Implementing requires:

1. HTML scraping (no documented JSON API) — full DOM parse for every
   anime listing, episode list, server list. Higher maintenance burden
   than a JSON API.
2. Session cookie management — PHPSESSID is set on every request; the
   scraper would need cookie-jar handling, which `domain.BaseHTTPClient`
   already supports.
3. Embed extractor research — unknown which video hosts AnimeFever
   uses for actual streams; could be StreamWish / Streamtape / FileLions
   / DoodStream (common in this tier).

Operator should weigh this against AllAnime (already live, GraphQL,
clean). AnimeFever's only edge over AllAnime is being a fully separate
upstream — meaningful for failover diversity, but the HTML-scraping
maintenance burden is real.

### AniWatchTV

```
$ curl --max-time 10 -sI 'https://aniwatchtv.to/'
(no response — times out at 10s)

$ curl --max-time 10 -sI 'https://aniwatch.to/'
HTTP/2 404
server: cloudflare
x-turbo-charged-by: LiteSpeed
```

**Status:** dead. `aniwatchtv.to` does not respond within 10s (DNS
resolves but the upstream times out). `aniwatch.to` returns Cloudflare's
404 — the domain still has DNS + a CF edge config but no backend behind
it. This is consistent with the March 2026 USTR-affiliated takedown
discussion in industry forums (no public-record seizure notice body
because Cloudflare 404s before any backend response).

**Anti-Bot:** n/a — service unreachable.

**Recommendation:** `not-worth`. Service unreachable; rumours of USTR
seizure are consistent with the observed evidence; even if the service
came back tomorrow, the legal cloud is a structural disqualifier.

### HiAnime

```
$ curl --max-time 10 -sI 'https://hianime.to/'      → no response (timeout, DNS dead)
$ curl --max-time 10 -sI 'https://hianime.tv/'      → no response (timeout, DNS dead)
$ curl --max-time 10 -sI 'https://hianime.pe/'      → 000 (curl couldn't connect)
$ curl --max-time 10 -sI 'https://hianime.bz/'      → 000 (curl couldn't connect)

$ curl --max-time 10 -sI 'https://hianime.nz/'
HTTP/2 200
content-type: text/plain
server: cloudflare

$ curl --max-time 10 -sS 'https://hianime.nz/' | head -1
It's time to say goodbye. And thank you for a wonderful journey with great moments.

$ curl --max-time 10 -sI 'https://hianime.io/'
HTTP/2 200

$ curl --max-time 10 -sS 'https://hianime.io/' | head -3
<!DOCTYPE html>
<html lang="en">
<head>
    <title>HiAnime - Your Anime Streaming Platform    </title>

$ curl --max-time 10 -sI 'https://hianime.so/'
HTTP/2 200

$ curl --max-time 10 -sS 'https://hianime.so/' | head -3
<!DOCTYPE html>
<html>
<head>
    <meta name="robots" content="noindex,nofollow">
```

**Status:** dead (as a media provider). The original `hianime.to/.tv/.pe/.bz`
mirrors do not resolve. `hianime.nz` returns the literal "It's time to say
goodbye" body — the operators publicly retired the brand. `hianime.io` is
an unrelated generic-aggregator site that opportunistically registered the
old name; the HTML structure does not match any of HiAnime's historic
parser-known shapes. `hianime.so` is a placeholder.

**Anti-Bot:** n/a — no functional service.

**Recommendation:** `not-worth`. The brand is retired. The lookalike
domains are squatters / aggregators with no integration value.

### Crunchyroll-Free

```
$ curl --max-time 10 -sI 'https://www.crunchyroll.com/'
HTTP/2 403
cf-mitigated: challenge
content-security-policy: default-src 'none'; script-src 'nonce-...'
server: cloudflare

$ curl --max-time 10 -sS 'https://www.crunchyroll.com/' | head -1
<!DOCTYPE html><html lang="en-US"><head><title>Just a moment...</title>...
```

**Status:** live (technically). Returns Cloudflare's challenge page
unconditionally on the public root; the response body is a JS challenge.
Free-tier viewing requires a Crunchyroll account login + ad-supported
playback — not the open public-API pattern any scraper provider follows.

**Anti-Bot:** Cloudflare managed challenge (Turnstile or equivalent) on
the public root. Even the catalog browsing endpoints require a session
cookie.

**Recommendation:** `not-worth`. Two disqualifiers:

1. **Technical:** integration would require credentialed authentication
   (Crunchyroll's free tier is gated by user login + ad SDK), which is
   incompatible with the scraper's stateless `domain.Provider` interface.
2. **Legal:** using Crunchyroll content programmatically without their
   authorized API key violates their ToS; we would not ship this even
   if technically feasible.

## PoC Sketches for Survivors

No candidate received a `worth-implementing` verdict in this sweep. Two
candidates received `needs-deeper-PoC` (Miruro, AnimeFever) — sketches
below cover the spike work the operator would need to commit to before
either becomes a real Wave 3 plan.

### Miruro — `needs-deeper-PoC`

**FindID strategy:** likely `mal-id-direct` — Miruro's URL pattern
includes `/anime/<id>` where `<id>` may match an AniList ID (env shows
`VITE_ANILIST_CLIENT_ID`); MAL ID may be derivable via ARM. Confirm
during the spike.

**ListEpisodes endpoint shape:** UNKNOWN. The documented `/api/anime/*`
paths return `{"error":"Gone"}`. The real endpoints route through
`pro.ultracloud.cc` with an obfuscation transform keyed on two
constants embedded in the JS bundle. Spike work: reverse-engineer the
transform from the minified frontend JS (Vite-built; source maps
might be exposed).

**ListServers endpoint shape:** UNKNOWN. Same obfuscation gate.

**GetStream extraction approach:** UNKNOWN. Stream sources likely
route through `pru.ultracloud.cc` (the secondary proxy host in the env
constants). May land on existing embed extractors (StreamHG, Earnvids)
or may require a new ultracloud-specific extractor — too early to say
without the spike.

**Effort estimate:** 5–7 days IF the obfuscation reverse-engineering
converges in 1–2 days of spike. If the spike doesn't converge, scope
balloons to 10+ days and the operator should kill the plan instead of
committing.

**One-paragraph summary:** Miruro would be the second always-on EN
provider after AllAnime (slot after `allanime` in candidateProviders).
No new env flag because — if it ships — the obfuscation keys are
embedded in the package, not configured. The risk profile is high
because the obfuscation transform might rotate, and because Cloudflare
may add TLS fingerprinting at the proxy host. **Recommended workflow:
operator commits to a 2-day spike branch first; on convergence, the
full Wave 3 plan executes.**

### AnimeFever — `needs-deeper-PoC`

**FindID strategy:** `title-search-with-malsync-fallback`. AnimeFever
doesn't expose MAL/AniList IDs in URLs; integration would search by
title and pick the best fuzzy match — same pattern as gogoanime.

**ListEpisodes endpoint shape:** HTML scrape of
`https://animefever.cc/anime/<slug>` — episode list rendered server-side.

**ListServers endpoint shape:** HTML scrape of
`https://animefever.cc/anime/<slug>/episode-N` — server list rendered
server-side (likely as `<li>` tags or `<select>` options inside the
player widget).

**GetStream extraction approach:** UNKNOWN — depends on which embed
hosts AnimeFever proxies. Common candidates: StreamWish, Streamtape,
DoodStream, FileLions. Each of these would need a new extractor under
`services/scraper/internal/embeds/` (none currently registered).

**Effort estimate:** 4–6 days IF the existing embed registry covers
the video hosts (rare). 7–10 days if new extractors are needed.

**One-paragraph summary:** AnimeFever is the cleanest fallback to
AllAnime — no obfuscation, no JS challenge, just classic HTML scraping
with a PHP backend. The downside is the maintenance burden of HTML
selectors (which break when AnimeFever rebrands its templates) and
the embed-extractor research overhead. Slots after `allanime` in
candidateProviders. **Recommended workflow: operator confirms whether
HTML-scraping maintenance is acceptable for v3.1 before committing
the full plan.**

## Decision Gate

**Operator: pick 0–2 survivors below to implement.** Each pick spawns
its corresponding plan in Wave 3:

- [ ] Survivor #1 → executes 26-04-PLAN.md
- [ ] Survivor #2 → executes 26-05-PLAN.md
- [ ] No picks → 26-04 and 26-05 do not execute; SCRAPER-HEAL-26 ships
       as research-only.

**Operator selection:** _<fill in after reading the sweep>_

**Rationale:** _<one paragraph from the operator on why these picks (or
no picks)>_

**Sweep summary for decision-making:**

- Total candidates evaluated: 6
- Live (functional service): 3 (Miruro, AnimeFever, Crunchyroll-Free)
- Dead (no functional service): 3 (AnimeOwl, AniWatchTV, HiAnime)
- `worth-implementing`: 0
- `needs-deeper-PoC`: 2 (Miruro — obfuscation-spike-gated; AnimeFever —
  HTML-scraping-acceptable-gated)
- `not-worth`: 4

**Recommended default if operator does not select within 7 days:**
`research-only` — Phase 26 ships with AllAnime + (if 26-06 converges)
AnimeKai, and SCRAPER-HEAL-26 is closed as research-only per CONTEXT.md
D2 ("If the survey produces zero live candidates worth implementing,
SCRAPER-HEAL-26 ships as research-only and the phase is done with two
new providers — acceptable outcome").

The autonomous execution of this Phase 26 batch STOPS at this gate.
26-04 and 26-05 plans remain on disk; `gsd-execute-phase` does NOT
run them until the operator's selection lands.
