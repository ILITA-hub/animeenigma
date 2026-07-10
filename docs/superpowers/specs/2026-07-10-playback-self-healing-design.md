# Playback Self-Healing: solodcdn edge rotation + stealth session resurrection

**Date:** 2026-07-10 · **Status:** approved by owner (chat), pending implementation
**Origin:** tNeymik's Yani Neko watch session 2026-07-10 03:30–03:36 UTC — two independent
playback deaths in one sitting (chronic Kodik/solodcdn segment 500s; gogoanime stream killed
by our own scraper/stealth-scraper redeploys).

**Metrics:** UXΔ = +3 (Better) · CDI = 0.04 * 13 · MVQ = Phoenix 92%/85%

## Problem

1. **Kodik/solodcdn partial edge failure (chronic, daily).** Specific `.ts` segments on
   `p12`/`p13.solodcdn.com` intermittently return HTTP 500 while sibling segments succeed
   (Prometheus `proxy_upstream_errors_total`: p12 500s = 132/77/445 on 07-08/09/10; p13 =
   36/226/170). The same path+signature on a sibling edge (`p14`) serves fine — verified
   live 2026-07-10 (p12 302→p14 200 for previously-500 segments). Our HLS proxy does zero
   retries: one upstream 5xx ⇒ 502 ⇒ hls.js stalls ⇒ user bails.

2. **Redeploys kill in-flight stealth streams.** stealth-scraper sessions
   (`sid → live Camoufox page`) are in-memory only; container recreate ⇒ every sid 410s.
   Worse, the scraper's per-provider Redis stream cache (gogoanime TTL ≤5 min) keeps
   returning the URL embedding the dead sid, so the FE's auto-retry loops on a corpse
   until cache expiry (observed 03:33:05–03:33:33: 4 retries, same dead
   `sid=db193501…`). Camoufox profiles (cookies, CF clearance) already persist on the
   `docker_stealth_profiles` volume — only the session registry and pages die. Warming
   also re-runs on every profile relaunch even when the on-disk profile is already warm.

## Design

### Layer A — solodcdn edge rotation (streaming service)

In `libs/videoutils/proxy.go`: when the upstream response is **≥500** and the request host
matches `^p\d+\.solodcdn\.com$`, retry the identical path on sibling edges from
`STREAMING_SOLODCDN_EDGES` (default `p12,p13,p14`), skipping the edge that failed,
**max 2 alternates**, then return the existing 502. Applies to every solodcdn GET
(manifest + segment). Never retry 4xx. New counter
`proxy_edge_rotations_total{from,to,outcome}`.

### Layer B — stealth-scraper session resurrection (lazy rehydrate, same sid)

**Persist.** On session create, write `/data/profiles/sessions/{sid}.json`:
`{sid, master_url, player_url, referer, profile_id, proxy_id, user_key, expires_at,
camoufox_build}`. Rewrite when the sliding TTL extends on activation; delete on
close/eviction/expiry sweep. `camoufox_build` = camoufox package version + image build
stamp (env, baked at image build).

**Rehydrate (lazy).** `/hls` with an unknown sid, under a per-sid lock:
persisted record exists ∧ `expires_at` not past ∧ `camoufox_build` matches the running
build → lease a healthy profile (prefer `profile_id`, else any free), relaunch
**without warming**, reopen `player_url`, in-page fetch `master_url` once to verify
(must be 200) → re-register the `Session` under the **same sid**, slide TTL, serve.
Any failure ⇒ delete the record, 410 (falls through to the safety net). The watcher
sees a buffer blip, not an error.

**Warming skip (general).** Per-profile persisted marker `{warmed_at, camoufox_build}`.
`_ensure_browser` skips `warm_profile` when the marker is fresher than
`WARM_MARKER_TTL` (default 24h) **and** the build matches. A Camoufox update invalidates
both the warm markers and rehydration eligibility — the owner's rule: reuse old sessions
only when Camoufox itself didn't change.

### Safety net — dead-sid liveness gate (scraper)

stealth-scraper gains `GET /session/{sid}/alive` → `alive | rehydratable | gone`
(in-memory registry + persisted-record lookup; ~1ms). The scraper, before serving a
**cached** stream whose `source_url` embeds a stealth sid, calls it; `gone` ⇒ treat as
cache miss and re-resolve fresh (also delete the stale entry). No cross-service events,
no cache-key index. FE unchanged — its existing auto-retry + playback-position snapshot
already resume correctly once the backend stops returning dead URLs.

### Deploy hygiene (ops note, no code)

The 03:33 recreate came from a prometheus-only commit — check during implementation why
compose recreated stealth-scraper for it, and avoid redeploying sidecars whose
code/config didn't change.

## Edge cases

- Recorded profile leased/unhealthy → rehydrate on any healthy free profile (gates are
  fingerprint+referer, not cookies; the master-fetch verify catches the rest).
- Corrupt/unreadable session record → delete, 410.
- Concurrent segment requests for one dead sid → per-sid lock; waiters reuse the
  rehydrated session.
- Restart rehydrate storm → naturally bounded by the profile pool + existing breaker.
- Upstream master URL expired while down → verify fetch fails → 410 → liveness gate +
  FE re-resolve take over.

## Testing

- Unit (stealth): rehydrate happy path from a fixture record; build-mismatch and
  expired-record refusals; warm-marker skip logic.
- Unit (streaming): httptest edge rotation — 500 on p12 ⇒ 200 from sibling; 4xx not
  retried; alternates capped at 2.
- Unit (scraper): cached stream + `gone` liveness ⇒ cache miss + re-resolve.
- Live (house rule — real anime): resolve a gogoanime stream, `docker restart
  animeenigma-stealth-scraper`, confirm the same sid resumes serving and the FE never
  surfaces an error; Kodik watch on Yani Neko (shikimori 63403) to confirm rotation
  metrics when an edge flaps.

## Out of scope

- Eager rehydrate on boot (rejected: RAM/pool pressure for mostly-abandoned sessions).
- Graceful-drain / blue-green sidecar deploys (rejected in favor of heal-fast).
- FE changes (existing retry + resume machinery suffices).
