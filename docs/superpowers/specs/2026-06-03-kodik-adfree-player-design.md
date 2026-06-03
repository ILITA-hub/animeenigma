# Design: Ad-free Kodik player + branded pre-roll

**Date:** 2026-06-03
**Status:** Approved (brainstorm) — pending spec review → implementation plan
**Author:** Claude + @0neymik0

## 1. Goal

Play Kodik RU streams in our own HTML5 player **without Kodik's ads**, by
extracting the real HLS `.m3u8` from the Kodik embed and serving it through our
existing HLS proxy — instead of embedding Kodik's ad-laden iframe. Replace the
ads with our own **5-second branded intro** (site logo) as a client-side
pre-roll.

This is a **new player alongside** the existing iframe `KodikPlayer.vue`, not a
replacement. The iframe player stays as a separate, independently-selectable
surface.

## 2. Feasibility — PROVEN (PoC 2026-06-03)

A throwaway PoC pulled a playable 720p stream end-to-end from a live Kodik
embed (Quintessential Quintuplets ep1). Verified pipeline:

1. **GET embed page** (UA + `Referer: https://kodikplayer.com/`); keep the
   `__ddg*` DDoS-Guard cookies (handed over with no JS challenge). Scrape:
   - var-form: `domain`, `d_sign`, `pd_sign`, `ref` (`https://kodikplayer.com/`), `ref_sign`
   - js-form:  `.type` (e.g. `seria`), `.hash`, `.id`
2. **POST** form-urlencoded to `https://kodikplayer.com/ftor`
   (endpoint literal = `atob("L2Z0b3I=")` in `app.player_single*.js`).
   Headers: `X-Requested-With: XMLHttpRequest`, `Origin` + `Referer` = embed.
   Body: `d, d_sign, pd(=domain), pd_sign, ref, ref_sign, bad_user=false,
   cdn_is_working=true, type, hash, id`.
   → `200 JSON { default, links:{"360":[{src,type:"application/x-mpegURL"}],"480":…,"720":…}, vast:[…ads…] }`
3. **Decode `src`**: brute-force ROT 0..25 over `A-Za-z` only, `+`-pad to a
   multiple of 4, base64-decode, accept the candidate containing
   `mp4:hls:manifest` (shift varies per response; PoC hit ROT 18).
   → `//cloud.solodcdn.com/useruploads/<uuid>/<sig>:<expiry>/720.mp4:hls:manifest.m3u8`
4. Manifest `302`-redirects to a node (`draco.cloud.solodcdn.com`); VOD, 6s
   segments, relative `./720.mp4:hls:seg-N-v1-a1.ts`. Segment = `video/MP2T`,
   H.264 1280×720 + AAC. **Playable.**

**The ads** live only in the JSON `vast`/`reserve_vast` arrays. Our player never
requests them — that is the entire ad-free win. No ad-blocking heuristics needed.

PoC notes archived at `/tmp/kodik-poc/POC-RESULT.md` and memory
`reference_kodik_ftor_stream_extraction`.

## 3. Architecture

```
KodikAdFreePlayer.vue
  └─ GET /api/anime/{id}/kodik/stream?episode=&translation=&quality=   (catalog)
        └─ catalog kodik parser: GetEpisodeLink(shikimoriID, ep, tr)  → embed URL  (reuses token/search)
        └─ libs/kodikextract.Resolve(embedURL)                         → []Stream{Quality, M3U8URL}
        └─ returns proxied URL: /api/streaming/hls-proxy?url=<m3u8>&referer=https://kodikplayer.com/&type=hls
  └─ pre-roll: play public/branding/intro.mp4 (5s) → on `ended` → attach HLS via hls.js
  └─ SubtitleOverlay / EpisodeSelector / watched-state / ReportButton  (shared)
```

### 3a. Backend extraction — `libs/kodikextract/` (in-process Go module)

Chosen over a sidecar because the PoC proved **no headless browser is needed** —
plain HTTP + cookies + decode suffices. A stealth-Chromium sidecar (à la
`animepahe-resolver`) stays a documented **future escape hatch** if Kodik later
puts a JS challenge in front of `/ftor`.

- **Input:** embed URL (catalog already produces it via `GetEpisodeLink`).
- **Output:** `[]Stream{ Quality int; M3U8URL string }` (qualities 360/480/720…),
  plus the chosen default.
- **Steps:** exactly the PoC pipeline (§2). Uses an IPv4-forced transport
  consistent with the rest of the platform; carries a cookie jar across the
  GET→POST so `__ddg*` cookies ride along.
- **Decode:** brute-force ROT + base64, accept on `mp4:hls:manifest`.
- New `libs/` module wiring (per project convention): add to `go.work`,
  catalog `go.mod` require+replace, catalog `Dockerfile` COPY, `go work sync`.
- The abandoned `GetVideoURL` / `decryptVideoURL` / `convertChar`
  (`//nolint:unused`) stub in `services/catalog/internal/parser/kodik/client.go`
  is **deleted** — its logic moves into the module with the real `/ftor` POST.

### 3b. Catalog route

`GET /api/anime/{animeId}/kodik/stream?episode=N&translation=ID&quality=720`
- Resolves embed via existing parser → `kodikextract.Resolve` → picks requested
  quality (or default) → returns `{ stream_url, quality, qualities:[…],
  episode, translation_id }` where `stream_url` is the `/api/streaming/hls-proxy`
  URL.
- Cache ≤ 1h (URLs carry an `:<expiry>` token — honor "don't cache >1h").
- Mirrors the existing `/kodik/translations` + `/kodik/video` handler/service
  pattern.

### 3c. HLS proxy allowlist

Add the `solodcdn.com` family to `HLSProxyAllowedDomainsWithProvenance` in
`libs/videoutils/proxy.go` (provenance: Owner `@0neymik0`, Added `2026-06-03`,
Reason "Kodik ad-free HLS manifest + segments"):
- `cloud.solodcdn.com` — covers the manifest and the relative segments (whose
  rewrite base is the manifest's `cloud.solodcdn.com` dir; each then `302`s to a
  node).
- `solodcdn.com` as the eTLD+1 (subdomain-inclusive) to also cover node hosts
  (`draco.cloud.solodcdn.com`, etc.) should a manifest ever embed absolute node
  URLs.

The proxy already (verified): follows redirects with headers preserved
(`CheckRedirect`, ≤10), and `rewriteM3U8URLs` resolves relative segments against
the passed source URL. No proxy-core change expected beyond the allowlist; a
plan-phase task confirms node redirects resolve cleanly under the proxy.

### 3d. Frontend — `KodikAdFreePlayer.vue`

- Direct-stream player modeled on `RawPlayer.vue` / `OurEnglishPlayer.vue`:
  `hls.js` (pinned `~1.5.20` per the known 1.6.x codec regression) over the
  proxied manifest; native HLS on Safari.
- Reuses shared `EpisodeSelector`, watched-state highlighting, `SubtitleOverlay`,
  and `ReportButton`.
- **No in-player iframe fallback.** On extraction/playback failure → inline error
  state + `ReportButton`. The legacy iframe Kodik remains a separate player
  choice on the watch page.
- Provider-accent: Kodik cyan (lint-exempt brand hue).

### 3e. Branded pre-roll (5s intro)

Pure client-side pre-roll — **no server-side manifest stitching** (overkill for a
self-hosted setup; avoids transcoding our bumper to matching TS/codecs).

- Asset: static `frontend/web/public/branding/intro.mp4` (≤5s, ships with the
  frontend; admin-uploadable is a future iteration).
- Flow: on episode load, the same `<video>` plays `intro.mp4` first; on `ended`
  (or Skip), we attach the episode HLS and start playback.
- **Skip:** a "Skip" control appears ~3s in.
- **Frequency:** once per episode start — NOT on seek/replay/quality-change
  within the same episode (guarded by an "intro already shown for this episode"
  flag).
- If the asset is missing/unplayable, skip straight to the stream (intro is
  best-effort, never blocks playback).

## 4. Data flow (happy path)

1. User opens watch page, selects the ad-free Kodik player + translation.
2. Player calls `/kodik/stream` → catalog resolves embed → `kodikextract` returns
   proxied `.m3u8`.
3. Player plays `intro.mp4` (5s, skippable @3s).
4. On intro end → `hls.js` loads the proxied manifest → segments stream through
   `/api/streaming/hls-proxy` (Referer injected, redirects followed).
5. Watched-state + subtitles behave as in Raw/OurEnglish.

## 5. Error handling

- Embed scrape misses a field / `/ftor` ≠ 200 / no decodable manifest →
  `kodikextract` returns a typed error → route returns a clean failure → player
  shows error + ReportButton.
- Expired URL mid-session (`:<expiry>` passed) → hls.js fatal → player re-requests
  `/kodik/stream` once (fresh extraction) before surfacing an error.
- Redis/cache outage → extraction runs live (slower, still works).

## 6. Out of scope (this iteration)

- Admin-uploadable / per-site-config intro asset (static file for v1).
- Server-side ad/intro stitching into one HLS timeline.
- Stealth-Chromium Kodik sidecar (future escape hatch only).
- Touching the existing iframe `KodikPlayer.vue` or its watch-together RPC path.
- Watch-together support for the new player (can follow later; Raw was similarly
  deferred).

## 7. Risks

- **Fragility.** Kodik changes `/ftor` params / cipher / CDN periodically (the
  original author abandoned extraction for this reason). Mitigation: brute-force
  decode (shift-agnostic), typed errors + ReportButton, and the iframe player
  remains as a parallel always-works option. Re-verify the pipeline if it breaks.
- **DDoS-Guard hardening.** Today cookies come with no JS challenge. If that
  changes, escalate to the stealth-Chromium sidecar.
- **Allowlist drift.** CDN host could rotate off `solodcdn.com`; the resolver
  should log the decoded host so a new family is easy to spot/add.

## 8. Project metrics (UXΔ / CDI / MVQ)

- **UXΔ = +3 (Better)** — ad-free RU playback in a native, instrumented player
  (subs, progress, watched-state) instead of an ad-laden iframe black box.
- **CDI = 0.03 * 21** — moderate spread (new libs module + catalog route +
  proxy allowlist + new SFC + branding asset), low shift (follows the
  Raw/OurEnglish direct-stream pattern), Effort_Fib 21.
- **MVQ = Griffin 85% / 80%** — solid, conventional build on a proven pattern;
  slop-resistance tempered by upstream-extraction fragility.

## 9. Verification (plan-phase)

- `libs/kodikextract` unit test against a captured `/ftor` fixture (decode →
  expected manifest URL).
- Live smoke: `/kodik/stream` for a real title returns a proxied URL that loads
  through `/api/streaming/hls-proxy` and plays in-browser (desktop + mobile).
- Pre-roll: intro plays once, Skip works, missing-asset path degrades to stream.
- Confirm solodcdn node redirects resolve under the proxy.
