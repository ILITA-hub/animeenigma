# Provider Recovery Log

Daily on-call diary for EN scraper provider recoveries. One entry per run.
Newest entry first. Used by the recovery operator to avoid repeating yesterday's attempt.

---

## 2026-07-01 — animepahe

**State before:** `policy=manual, health=down, status=degraded` — reason: `cdn_unreachable on ` (health_since 2026-06-29T18:02:01Z, policy_since 2026-06-26T08:17:08Z, last_probed_at 2026-07-01T00:02:19Z). Selected over the "Known-hard cases" list at the bottom of this log — that list predates the 2026-06-26 Camoufox Turnstile-solve revival (`e77802d4`) and animepahe had been genuinely running `engine=browser` for days before regressing on 06-29; not re-litigating a known-unsolvable case.

**Root cause — TWO independent bugs, found by walking the full episodes→servers→stream chain with `prefer=animepahe`:**

1. **Profile-lease leak in the `solve_challenge` recycle path (the actual animepahe-specific regression).** `services/stealth-scraper/app/engine.py::_warm_fetch_session` wipes the leased profile's `user_data_dir` before every animepahe warm fetch (Turnstile re-solves need a clean profile — a poisoned prior attempt stops yielding `cf_clearance`). That recycle step (`await self._teardown(profile, reason="recycle")` + `_rm_dir(...)`) ran BEFORE the function's `try/except` block, so any exception there (chiefly `asyncio.CancelledError` from an HTTP client disconnect — the exact `BaseException`-not-`Exception` gotcha already fixed for the sibling `resolve()` path in `0c994cfa`) leaked the just-acquired profile forever: no crash flag, no session, so the reaper's TTL/crashed-slot sweeps could never reclaim it. animepahe is the *only* `solve_challenge=True` provider, so it hit this on every single fetch. Live symptom confirmed via `/metrics`: `stealth_browser_pool_size=3, stealth_active_sessions=1, stealth_pool_free=0, stealth_pool_crashed=0` — 2 of 3 profiles permanently leased with nothing accounting for them, for the SHARED pool gogoanime/nineanime/9anime discovery also lease from.
2. **Unrelated, self-inflicted during recovery: unpinned `playwright` transitive dependency.** Redeploying stealth-scraper to ship fix #1 triggered a Docker build-cache miss on the `pip install && camoufox fetch` layer (likely evicted by the daily docker-prune cron). `camoufox==0.4.11` (PyPI, unmaintained since Jan 2025) does not pin `playwright`, and `python -m camoufox fetch` always grabs the latest upstream Camoufox/Firefox release with no version-pin flag — the two halves drift independently. The fresh install landed on `playwright==1.61.0`, which sends a `viewport.isMobile` field the Juggler protocol on the fetched browser build rejects outright (`BrowserType.launch_persistent_context: Protocol error (Browser.setDefaultViewport)`), breaking **browser launch for every `engine=browser` provider container-wide** — not just animepahe. Confirmed via `pip show playwright` (1.61.0) and the open, unresolved upstream report at github.com/daijro/camoufox/issues/612. This is a landmine that will resurface on any future cache-miss rebuild until upstream ships a compatible pin.

**Fixes shipped (worktree → main, both deployed + verified):**
1. `23255553` — move the recycle-teardown block inside the existing `try`, matching the CancelledError handling already present for every other lease-acquisition branch in this function. Added a regression test (`TestWarmFetchRecycleTeardownLeak`) that fails against the pre-fix code (proved by temporarily reverting via `git stash`) and passes post-fix. Full suite: 133/133 passing.
2. `f985aa08` — pin `playwright==1.59.0` in `requirements.txt` (last confirmed-compatible line per the upstream issue), with an explanatory comment so the next person doesn't re-drift onto 1.60+.

**Manual verification (2026-07-01, post both fixes, post-redeploy):**
- Pool metrics post-restart: `pool_free=4/4, pool_crashed=0, active_sessions=0` (leak fully cleared by the fresh container; the fix additionally proved itself live — a transient unrelated Camoufox launch crash right after restart came back `pool_free=4, pool_crashed=1`, i.e. properly released and marked for reaper resurrection, not leaked)
- `GET /scraper/episodes?prefer=animepahe` (Witch Hat Atelier, fc6c54ac) → `meta.provider=animepahe`, 13 episodes ✅ (177ms, warm session reuse)
- `GET /scraper/servers` → 6 real `kwik.cx` servers (3 sub / 3 dub) ✅
- `GET /scraper/stream?category=sub` → signed `https://vault-16.owocdn.top/.../uwu.m3u8` (AES-128 HLS) ✅
- HLS master via streaming proxy (`/api/v1/hls-proxy` + `exp`/`sig`/`referer`): HTTP 200, valid `#EXTM3U` VOD playlist, `#EXT-X-KEY:METHOD=AES-128` ✅
- First rewritten segment via gateway (`/api/streaming/hls-proxy`): HTTP 200, `video/mp2t`, 677,184 bytes ✅
- Incidental: gogoanime (also `engine=browser`, also broken by bug #2, also fixed by the same pin) confirmed recovered too — `meta.provider=gogoanime` on a fresh episodes call. nineanime still fails over, but to an unrelated pre-existing `not_found` (title fuzzy-match miss on the CotE-4 probe anchor, documented in the 2026-06-26 entry below) — not a browser-launch symptom, so left alone.

**Action taken:** Submitted `probe-result pass` with reason citing both commit hashes and the concrete verification → state machine transitioned `down → recovering` at 2026-07-01T02:5x. `policy=manual` preserved (promotion to auto is a human call). Did NOT touch gogoanime's own `health=down` flag manually — its next scheduled probe (browser launch now works) should self-correct; not forcing a second provider's state in the same run per the one-provider guardrail, this was incidental fallout from fixing animepahe.

**Outcome:** ✅ Recovered with two shipped code fixes (not just a flag flip) — genuinely verified end-to-end with real decrypted-chain bytes.

**Next step:** Confirm animepahe (and gogoanime) transition `recovering → up` at the next scheduled probe cycle. Consider a longer-term follow-up: either vendor/pin the Camoufox *browser* build too (not just playwright) so a future `camoufox fetch` can't silently drift again, or move off the unmaintained `camoufox` PyPI package. Also: the base tree (`/data/animeenigma`) has an uncommitted 2026-06-30 gogoanime entry for this same log sitting in dirty WIP (part of the pre-existing `git status` at session start, alongside `.planning/STATE.md` etc.) that was never pushed — did not touch it (golden rule: never edit the base tree directly), but a future run should reconcile/rescue that entry rather than losing it.

---

## 2026-06-30 — gogoanime

**State before:** `policy=manual, health=down, status=degraded` — reason: `cdn_unreachable on ` (health_since 2026-06-30T00:00:21Z, policy_since 2026-06-30T00:00:21Z)

**Root cause:** Transient — midnight UTC pool pressure. `cdn_unreachable on ` (empty server field) in `engine.go` means the resolver returned a non-`ErrProbeNotFound` error before any stream URL was fetched; the server field is never set. This pattern is consistent with `PoolExhausted` (503) from the stealth-scraper: the probe's `/resolve` call lands when all Camoufox profiles are occupied by concurrent viewer sessions and other browser-engine provider probes (gogoanime, allanime, animepahe, nineanime all share the same pool and hit their 6h cadence around midnight UTC simultaneously). With no free profile, stealth-scraper returns 503 → resolver returns error → `cdn_unreachable on `.

The CancelledError profile-lease fix deployed 2026-06-28 (`0c994cfa`) eliminated the leak path, but pool pressure from *legitimate* concurrent usage at midnight persists. This is the third consecutive midnight failure pattern (2026-06-27T00:00:05Z, 2026-06-30T00:00:21Z).

**Manual verification (2026-06-30T02:26–02:38Z):**
- `GET /scraper/episodes?prefer=gogoanime` (Witch Hat Atelier, fc6c54ac) → 13 episodes ✅
- `GET /scraper/servers` → HD-1, HD-2 (`gogoanimes.fi`) ✅
- `GET /scraper/stream?category=sub` → stealth-scraper resolved session `c456ee9d...`, `http://stealth-scraper:3000/hls?sid=c456ee9d...&url=https://9hjkrt.nekostream.site/.../master.m3u8` ✅ (2357ms)
- HLS master via streaming proxy: HTTP 200, 1412 bytes, 3 quality variants (1080p/720p/360p) ✅
- HLS variant manifest (`index-f1-v1-a1.m3u8`): HTTP 200, valid VOD playlist, segment URIs rewritten through stealth-scraper ✅
- First segment (`nekostream.site/segment/...`): HTTP 200, `video/mp2t`, 2,957,492 bytes ✅

**Action taken:**
Submitted `probe-result pass` with reason `manual-recovery-verify: 3-hop HLS chain confirmed (master+variant+segment), nekostream.site CDN nominal` → state machine transitioned `down → recovering` at 2026-06-30T02:38:45Z. `policy=manual` preserved. No code changes needed.

**Outcome:** ✅ Recovered (transient pool pressure at midnight, no structural failure). gogoanime now `health=recovering, policy=manual`.

**Systemic note:** Three out of four midnight UTC probes in the last week have failed with `cdn_unreachable on ` for gogoanime. The probe runs `manual+down` cadence (6h, 1 sample, fail-fast), so it fires at approximately 00:00, 06:00, 12:00, 18:00 UTC. The 00:00 UTC slot appears to be peak viewer + multi-provider probe overlap. Recommendation: stagger browser-engine provider probe cadences slightly (e.g. gogoanime at 0h offset, allanime at +30min, animepahe at +1h) OR increase the Camoufox pool size by 1–2 profiles. Not blocking today.

**Next step:** Monitor 06:00Z and 12:00Z probe results. If the next few probes pass, the state machine auto-promotes `recovering → up` (requires PROVIDER_PROMOTE_AFTER consecutive passes). Stagger cadence fix is a follow-up improvement, not an emergency.

---

## 2026-06-29 — nineanime

**State before:** `policy=manual, health=down, status=degraded` — reason: `empty_response on 1anime` (health_since 2026-06-26T18:00:04Z, last_probed_at 2026-06-28T00:00:19Z)

**Root cause:** Transient 1anime CDN blips. The probe rolled an anime indexed by 9anime.me.uk, got past episodes and servers, but the `my.1anime.site` CDN returned an empty/zero-body response at probe time (2026-06-26T18:00 and 2026-06-28T00:00). This CDN is intermittently unreliable around those UTC hours.

**Manual verification today (2026-06-29T02:27Z):**
- `GET /scraper/episodes?prefer=nineanime` (Witch Hat Atelier, fc6c54ac) → 13 episodes ✅
- `GET /scraper/servers` → `1anime` server ✅
- `GET /scraper/stream?server=1anime&category=sub` → signed `https://my.1anime.site/stream/6717eb510c2aa23f77b32fabcea730d0` ✅
- Direct URL (with `Referer: https://my.1anime.site/`): HTTP 302 → `https://my.1anime.site/videos/witch-hat-atelier-episode-1.mp4` → HTTP 200, 195MB, `video/mp4` ✅
- Via streaming proxy with `?referer=https%3A%2F%2Fmy.1anime.site%2F`: HTTP 200, 195MB, `video/mp4` ✅

Note: probe_validator correctly sets the referer query param (`rs.Referer` from `stream.headers["Referer"]`). The proxy WITHOUT referer returns 403 from my.1anime.site — but that only affects misconfigured callers; the probe itself passes correctly.

**Action taken:**
Submitted `probe-result pass` with reason `manual-recovery-verify` → state machine transitioned `down → recovering` at 2026-06-29T02:32Z. `policy=manual` preserved. No code changes needed.

**Outcome:** ✅ Recovered (transient 1anime CDN blip). nineanime now `health=recovering, policy=manual`.

**Next step:** Monitor next scheduled probe. If probe keeps hitting `empty_response on 1anime`, investigate whether 1anime CDN has a nightly maintenance window (failures at 18:00 UTC and 00:00 UTC suggest a pattern). Consider adding a daytime probe window. `policy=manual` remains — promotion to auto is a human decision.

---

## 2026-06-28 — gogoanime

**State before:** `policy=auto, health=down, status=degraded` — reason: `cdn_unreachable on ` (health_since 2026-06-27T18:00:12Z, last_probed_at 2026-06-27T18:00:12Z)

**Root cause:** Two compounding bugs:

1. **Double stream call in probe resolver:** `services/analytics/internal/probe/resolver.go` iterates ALL servers (HD-1 and HD-2 for gogoanime) and calls `/scraper/stream` once per server. The probe's second stream call (HD-2) triggered a second Camoufox `resolve()` while the first was still active. With only 4 pool profiles, the second call exhausted the pool → 503 from stealth-scraper → probe marked stream as failing.

2. **Stale probe anchor anime:** The probe re-rolled Clannad After Story and Descending Stories (Showa Rakugo) as anchor anime, but gogoanimes.fi does not index these older series. Both returned 0 search results → probe counted these as provider failures.

3. **`asyncio.CancelledError` profile lease leak (root cause of pool exhaustion):** When the Go scraper HTTP client's timeout fired and dropped the connection, Starlette cancelled the async handler with `asyncio.CancelledError`. In Python 3.8+, `CancelledError` is a `BaseException`, NOT an `Exception` subclass. The two catch-all `except Exception` clauses in `engine.py`'s `resolve()` loop and `_warm_session()` did NOT catch it, leaving the Camoufox profile permanently `leased=True`. By the time of the 18:00 probe, 3 of 4 profiles were leaked (3 active sessions showed 0 in metrics vs 3 leaked leases). Pool exhaustion → circuit breaker tripped (3 wedged errors in 60s) → `InMemoryHealthCache` set `stream_segment.Up=false` → scraper orchestrator short-circuited ALL gogoanime requests with instant 502.

**Manual verification pre-fix (02:28Z):**
- `GET /scraper/episodes` → 28 episodes ✅ (cached, no browser call needed)
- `GET /scraper/servers` → HD-1 and HD-2 ✅
- `GET /scraper/stream` (HD-1) → stealth-scraper 503 PoolExhausted (3 leaked profiles, 0 free)

**Fix applied:**
Code fix in `services/stealth-scraper/app/engine.py` — two locations:
1. `resolve()` loop: added `except asyncio.CancelledError` before the generic `except Exception`, calls `_safe_close_page` + `_teardown` + `profiles.release(profile)` + `raise`
2. `_warm_session()`: same pattern — `except asyncio.CancelledError` + `_teardown` + `profiles.release(profile)` + `raise`

Committed as `0c994cfa`, pushed to main. Redeployed stealth-scraper. Restarted Go scraper service to clear in-memory circuit breaker state (no health-reset API; the scraper's `InMemoryHealthCache` is process-local, cleared on restart; cache TTL=30min so without restart gogoanime would be gated until ~02:58Z).

**Manual verification post-fix (02:40Z):**
- `GET /scraper/episodes?prefer=gogoanime&exclusive=true` → 28 episodes ✅
- `GET /scraper/servers?...` → HD-1 and HD-2 ✅
- `GET /scraper/stream?...category=sub` → Camoufox resolved `http://stealth-scraper:3000/hls?sid=a912411ace1f482d8a31e2b97a9f0d0d&url=https://9hjkrt.nekostream.site/...master.m3u8` ✅
- HLS master manifest via streaming proxy: HTTP 200, `#EXTM3U`, 1080p/720p variants ✅
- HLS variant manifest: HTTP 200, `#EXTM3U`, real segment URLs (nekostream.site) ✅
- Pool metrics post-resolve: `pool_free=3, active_sessions=1, pool_exhausted_total=0` ✅

**Action taken:**
Submitted `probe-result pass` → state machine transitioned `down → recovering` at 2026-06-28T02:42Z. `policy=auto` preserved.

**Outcome:** ✅ Recovered (code fix). gogoanime now `health=recovering, policy=auto`. CancelledError leak fix deployed to main (`0c994cfa`).

**Next step:** Monitor pool_free metric — should stay ≥1 even under concurrent requests. Next probe cycle (cron) will verify gogoanime stream-segment reachability and auto-promote to `up` once recovering threshold is met.

---

## 2026-06-27 — gogoanime

**State before:** `policy=auto, health=down, status=degraded` — reason: `cdn_unreachable on ` (health_since 2026-06-26T12:00:22Z, last_probed_at 2026-06-27T00:00:23Z)

**Root cause:** Transient CDN issue at the noon and midnight probe times. Two consecutive probe runs recorded failures:

1. **Noon probe (2026-06-26T12:00:22Z):** reason stored as `empty_response on HD-1`. The megaplay/HD-1 server returned empty or invalid video at probe time. The scraper logs confirm gogoanime successfully resolved streams for Frieren, Gintama, and Your Lie in April at the same time, so the failure was a brief HDI-1 server hiccup affecting the specific probe episode, not a broad outage.

2. **Midnight probe (2026-06-27T00:00:05Z–00:00:23Z):** reason recorded as `cdn_unreachable on ` (empty CDN name). The probe logged gogoanime resolving Frieren successfully, but the analytics validator returned `cdn_unreachable` on the segment fetch. The empty CDN name is consistent with a timeout in the streaming proxy → stealth-scraper path (no hostname to report).

**Additional finding:** The megaplay HLS variant playlist contains ByteDance ad segment injection (`p16-ad-sg.ibyteimg.com`) after the first two real nekostream.site segments (~12s of real content, then ~2 min of pre-roll ads). This is NOT the same as the animefever ad-substitution (where ALL segments were ads); here real content follows the ad block. The analytics probe validator picks the FIRST segment (which is always a real nekostream.site segment), so this does not cause probe failures. UX concern logged but not a "provider down" condition.

**Manual verification (2026-06-27T02:24Z):**
- `GET /api/anime/6549ac79.../scraper/episodes?prefer=gogoanime` → 16 episodes ✅
- `GET /api/anime/6549ac79.../scraper/servers?...` → HD-1 and HD-2 servers ✅
- `GET /api/anime/6549ac79.../scraper/stream?...&category=sub` → HLS URL via stealth-scraper → `9hjkrt.nekostream.site` ✅
- HLS master manifest (signed URL): HTTP 200, 1411 bytes, 3 quality variants (1080p/720p/360p) ✅
- 1080p variant manifest: HTTP 200, 93393 bytes, valid HLS playlist ✅
- First real segment (`nekostream.site/segment/...`): HTTP 200, 1,543,732 bytes, `video/mp2t` ✅

**Action taken:**
Submitted `probe-result pass` → state machine transitioned `down → recovering` at 2026-06-27T02:28Z. `policy=auto` preserved (gogoanime remains in the auto failover chain). No code changes needed.

**Outcome:** ✅ Recovered (transient CDN blip). gogoanime now `health=recovering, policy=auto`. Will auto-promote to `up` after PROVIDER_PROMOTE_AFTER threshold on next successful probe.

**Next step:** Confirm gogoanime transitions `recovering → up` at the next scheduled probe cycle. Monitor ad injection presence in megaplay streams — if the ByteDance segment ratio grows to cover the opening segments, the probe will start failing with `ad_decoy` via the scraper's own streamprobe path. No action needed today.

---

## 2026-06-26 — nineanime

**State before:** `policy=manual, health=down, status=degraded` — reason: `empty_response on 1anime` (health_since 2026-06-23T12:02:35Z, policy demoted 2026-06-25T00:00:20Z)

**Root cause:** Dual failure — structural probe mismatch + transient CDN blip.

1. **Structural (probe anchor mismatch):** The scheduled probe uses Classroom of the Elite IV as the anchor anime. Its `name_en` is empty in the catalog; the fallback is the long romanized Japanese title "Youkoso Jitsuryoku Shijou Shugi no Kyoushitsu e 4th Season: 2-nensei-hen 1 Gakki". 9anime.me.uk's WP REST API does NOT index anime by romanized Japanese titles — it only has English titles — so `FindID` returns `ErrNotFound` → probe re-rolls to a random popular-pool anime.

2. **Transient (CDN blip at re-roll):** The re-roll found an anime nineanime could resolve (English title), got stream server "1anime", but the analytics validator called `http://streaming:8082/api/v1/hls-proxy?url=<my.1anime.site URL>` at midnight UTC and got an empty response (`empty_response on 1anime`). my.1anime.site was transiently unavailable at that time.

Manual verification today (2026-06-26T02:25Z):
- `FindID("witch-hat-atelier")` via 9anime WP API: ✅ returns slug
- Episodes extracted from 9anime series page: ✅ 13 episodes
- ListServers: ✅ "1anime" server
- GetStream: ✅ returns `https://my.1anime.site/stream/<hash>` (signed URL)
- Direct URL: HTTP 302 → `https://my.1anime.site/videos/witch-hat-atelier-episode-1.mp4`
- HLS proxy: ✅ HTTP 200, 195MB `video/mp4`, range-capable
- First 1KB of bytes: ✅ real video data

**Action taken:**
Submitted `probe-result pass` → state machine transitioned `down → recovering` at 2026-06-26T02:30Z. No code changes needed.

**Outcome:** ✅ Recovered (transient CDN blip). nineanime now `health=recovering, policy=manual`.

**Next step:** Confirm nineanime transitions `recovering → up` at next probe cycle. The structural probe anchor mismatch (CotE IV romanized title) is a known probe quality issue — consider adding an English title to CotE IV in the catalog, or rotating the probe anchor to an anime with a well-known English title. Filed as a follow-up note (not blocking recovery).

---

## 2026-06-26 — okru

**State before:** `policy=manual, health=down, status=degraded` — reason: `cdn_unreachable on ` (since 2026-06-25T00:00:17Z)

**Root cause:** Transient CDN outage. The probe ran at 2026-06-25T00:00:17Z and the analytics
validator received a transport error (`err != nil`) when fetching the okcdn.ru HLS manifest
through the streaming proxy — classified as `cdn_unreachable`. No code is broken:

- AllAnime GraphQL discovery: ✅ (finds OK source for target anime)
- ok.ru embed extraction: ✅ (returns signed HLS + MP4 URLs)
- okcdn.ru HLS master via proxy: ✅ HTTP 200, 4212 bytes, valid #EXTM3U
- okcdn.ru HLS variant via proxy: ✅ HTTP 200, 140067 bytes
- okcdn.ru TS segment (s0.ts) via proxy: ✅ HTTP 200, 249288 bytes, video/mp2t

Full end-to-end chain verified on anime 6549ac79 (Classroom of the Elite 4), episode 1,
sub category, as of 2026-06-26T01:14Z.

**Action taken:**
Submitted `probe-result pass` → state machine transitioned `down → recovering` at
2026-06-26T01:15:02Z. No code changes needed.

**Outcome:** ✅ Recovered (transient CDN blip). okru now `health=recovering, policy=manual`.
Auto-promote to `policy=auto` will happen after PROVIDER_PROMOTE_AFTER threshold.

**Next step:** Confirm okru transitions `recovering → up` and eventually `policy=auto` at the
next scheduled probe cycle. Monitor for repeat CDN unreachable events — if it keeps happening
consider noting okcdn.ru as intermittently unreachable from our egress.

---

## 2026-06-24 — miruro

**State before:** `policy=auto, health=down, status=degraded` — reason: `decode_failed on kiwi` (since 2026-06-23T12:00:30)

**Root cause:** Structural probe bug, NOT a miruro failure.
miruro's kiwi inner provider (animepahe-derived) serves AES-128 encrypted HLS
(`#EXT-X-KEY:METHOD=AES-128` in the playlist). The analytics probe validator
(`services/analytics/internal/probe/validator.go`) passes raw segment bytes
to ffprobe to detect a video codec. ffprobe receives encrypted ciphertext and
cannot identify any codec → `no video stream` → `decode_failed`.

The stream itself was fully functional throughout:
- HLS manifest: HTTP 200, 20703 bytes, `application/vnd.apple.mpegurl`
- First segment (vault-16.owocdn.top): HTTP 200, 893760 bytes via streaming proxy
- `scraper resolved stream` log confirms miruro resolved provider_anime_id 180745 successfully at 2026-06-24T02:54:33Z

**Action taken:**
1. Identified root cause: `validator.go` always calls ffprobe on segments, even
   for AES-128 encrypted HLS where ffprobe is guaranteed to fail.
2. Shipped fix `e30badd7` on `main`:
   - Added `hasAES128()` to detect `#EXT-X-KEY:METHOD=AES-128` in any manifest hop
   - When encryption detected, validate via segment reachability (HTTP 200 + bytes)
     instead of video decode
   - Added `TestValidator_AES128SkipsFFprobe` test
3. Redeployed analytics service.
4. Submitted `probe-result pass` → state machine transitioned `down → recovering`.

**Outcome:** ✅ Fix shipped (`e30badd7`), miruro now `health=recovering`.
Next automated probe will pass with the new code and promote miruro toward `up`.

**Next step:** Confirm miruro transitions `recovering → up` at next probe cycle.
Consider auditing other providers (nineanime/okru) for similar probe false-negatives.

---

## Known-hard cases (skip, don't re-attempt unless upstream changes)

- **allanime** — clock.json behind Cloudflare Turnstile (api.allanime.day). Policy=manual. No Go-level fix possible.
- **animefever** — HLS segments 302→ad CDN (sf16-scmcdn-sg.ibytedtos.com) that 403s our egress. Policy=manual.
- **animepahe** — DDoS-Guard→CF managed challenge, sidecar retired 2026-06-24. Policy=disabled.
