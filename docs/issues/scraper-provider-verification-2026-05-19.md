# Scraper Provider Verification — 2026-05-19

**Date:** 2026-05-19
**Target anime:** Frieren: Beyond Journey's End — MAL 52991 (Shikimori 52991)
**Internal anime UUID:** `f0b40660-6627-4a59-8dcf-7ec8596b3623`
**Requirement:** SCRAPER-HEAL-20 — retired Phase 24 Wave 0 context (available in Git history)
**Operator:** Claude Code (autonomous GSD execute-phase 24)
**Verdict (TL;DR):** **HARD GATE BLOCKED** — both P0 providers (gogoanime, animepahe) FAIL end-to-end; animekai is `escape-hatch-stub` (intentionally inert per SCRAPER-KAI-01..04). The default `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` baked into docker-compose.yml is the correct operating posture for the current upstream state. Phase 24 frontend work pauses per D2 until at least one EN provider is recovered or Phase 26 (AllAnime lift, SCRAPER-HEAL-25) lands.

---

## Test Environment

Docker compose stack (excerpt from `docker compose ps`):

| Container | Status |
|---|---|
| animeenigma-scraper | recreated 2026-05-19T05:03:53Z with verification override |
| animeenigma-gateway | up 22h |
| animeenigma-catalog | up 23m |
| animeenigma-megacloud-extractor | up 6d (healthy) |

**Production env (default `docker/.env`) — operating posture as of 2026-05-19:**
```
SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe   # default in docker-compose.yml
SCRAPER_ANIMEKAI_ENABLED=false                    # default in docker-compose.yml
```

**Verification override (temporary, restored after test):**
```
SCRAPER_DEGRADED_PROVIDERS=__none__   # sentinel — bypasses compose default lookup; parses to empty set
SCRAPER_ANIMEKAI_ENABLED=true         # enable animekai to confirm escape-hatch fall-through
```
This override forces all three providers to register with the orchestrator so the gate can probe each one end-to-end. Production defaults will be restored after the gate runs.

---

## Curl Pipeline

The verification uses the production-equivalent gateway path on `http://localhost:8000`, which proxies `/api/anime/{id}/scraper/*` to the catalog service, which in turn forwards to the scraper microservice on `http://scraper:8088/scraper/*`. The anime UUID `f0b40660-6627-4a59-8dcf-7ec8596b3623` (Frieren / MAL 52991) is the test subject.

```bash
BASE="http://localhost:8000"
ANIME_ID="f0b40660-6627-4a59-8dcf-7ec8596b3623"

# 1. Episodes
curl -sS "$BASE/api/anime/${ANIME_ID}/scraper/episodes?prefer=gogoanime"
curl -sS "$BASE/api/anime/${ANIME_ID}/scraper/episodes?prefer=animepahe"
curl -sS "$BASE/api/anime/${ANIME_ID}/scraper/episodes?prefer=animekai"
curl -sS "$BASE/api/anime/${ANIME_ID}/scraper/episodes"  # auto / orchestrator pick

# 2. Servers (would be issued per episode_id after Episodes succeeds — skipped, prerequisite failed)

# 3. Stream (would be issued per server after Servers succeeds — skipped, prerequisite failed)

# 4. Health (per-provider stage matrix — issued directly against scraper)
docker exec animeenigma-scraper wget -qO- http://localhost:8088/scraper/health
```

---

## Verdict Matrix

| Provider | Episodes? | Servers? | Stream URL Returned? | Stream URL Fetchable? | Disposition |
|---|---|---|---|---|---|
| **gogoanime** | FAIL (`[]`) | N/A — no episode_id to query | N/A | N/A | FAIL — search stage down (`0 search results for "Frieren: Beyond Journey's End"`). Documented in `services/scraper/internal/config/config.go:34-37` as anitaku.to → anineko.to migration breakage. Episodes-probe stage shows `up:true` (the probe timer is up — does not assert real episodes). |
| **animepahe** | FAIL (HTTP 500 → upstream context-deadline-exceeded after 15s) | N/A | N/A | N/A | FAIL — search returned a `release_id` for Frieren, but the release fetch (`https://animepahe.ru/api?m=release&id=5319&sort=episode_asc&page=1`) times out (context canceled, 2 attempts). Documented as IP-block / FingerprintJS gate in `services/scraper/internal/config/config.go:34-37`. |
| **animekai** | FAIL (`[]`) | N/A | N/A | N/A | FAIL-AS-DOCUMENTED — animekai is an `escape-hatch-stub` per SCRAPER-KAI-01..04 (carried to v3.1). All five stages (`search`, `episodes`, `servers`, `stream`, `stream_segment`) report `up:false` with last_err = "not implemented". This is expected behavior — animekai's role is to exercise the orchestrator's `ErrProviderDown` fall-through, NOT to actually return episodes. With the two P0 providers down, fall-through has nowhere to fall to. |
| **auto (no `prefer`)** | FAIL (`[]`) | N/A | N/A | N/A | FAIL — orchestrator tries `[gogoanime, animepahe, animekai]` in sequence and returns the empty-episodes union (`{"episodes": [], "meta": {"tried": ["gogoanime", "animepahe", "animekai"]}}`). |

---

## Raw Responses (Excerpts)

### gogoanime — episodes

```
HTTP=200 bytes=94 time=1.108590s
{"success":true,"data":{"episodes":[],"meta":{"tried":["gogoanime","animepahe","animekai"]}}}
```

Note: HTTP 200 with an empty episodes array is the orchestrator's "I tried everyone and got nothing playable" signal. It is not a 503 because the orchestrator successfully ran — the providers themselves had nothing to return.

### animepahe — episodes

```
HTTP=500 bytes=273 time=15.007869s
{"success":false,"error":{"code":"INTERNAL","message":"scraper http: Get \"http://scraper:8088/scraper/episodes?mal_id=52991&prefer=animepahe&title=Frieren: Beyond Journey's End\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"}}
```

The 15s round-trip is the upstream `animepahe.ru` failing to respond within the catalog's HTTP timeout while the scraper itself was awaiting `https://animepahe.ru/api?m=release&id=5319` (visible in scraper health output below).

### animekai — episodes

```
HTTP=200 bytes=94 time=0.034764s
{"success":true,"data":{"episodes":[],"meta":{"tried":["animekai","gogoanime","animepahe"]}}}
```

Note the `tried` order — `animekai` first because of `?prefer=animekai`, then orchestrator fell through to gogoanime (empty) and animepahe (timeout) — still ended at `[]`. The 35ms total round-trip is the stub returning instantly.

### scraper health — direct probe (per-stage status)

```json
{
  "providers": {
    "gogoanime": {
      "stages": {
        "episodes": { "up": true,  "last_ok": "2026-05-19T05:04:51.491587452Z" },
        "search":   { "up": false, "last_err": "gogoanime: 0 search results for Frieren: Beyond Journey's End: scraper: not found" },
        "servers":  { "up": true,  "last_ok": "0001-01-01T00:00:00Z" },
        "stream":   { "up": true,  "last_ok": "0001-01-01T00:00:00Z" }
      }
    },
    "animepahe": {
      "stages": {
        "episodes": { "up": false, "last_err": "animepahe: release fetch: GET https://animepahe.ru/api?m=release&id=5319 giving up after 2 attempt(s): context canceled" },
        "search":   { "up": true,  "last_ok": "2026-05-19T05:04:51.491470219Z" },
        "servers":  { "up": true,  "last_ok": "0001-01-01T00:00:00Z" },
        "stream":   { "up": true,  "last_ok": "0001-01-01T00:00:00Z" }
      }
    },
    "animekai": {
      "stages": {
        "search":   { "up": false, "last_err": "animekai: FindID not implemented: scraper: provider down (cause: animekai: escape-hatch stub (SCRAPER-KAI-01..04 carried to v3.1))" },
        "episodes": { "up": false, "last_err": "animekai: ListEpisodes not implemented: scraper: provider down" },
        "servers":  { "up": false, "last_err": "escape-hatch stub: SCRAPER-KAI-01..04 carried to v3.1" },
        "stream":   { "up": false, "last_err": "escape-hatch stub: SCRAPER-KAI-01..04 carried to v3.1" }
      }
    }
  }
}
```

Decode: even with all three providers registered, only `gogoanime.episodes` and `animepahe.search` probes report green. Both providers fail at the second stage (gogoanime's search stage = `0 results`, animepahe's episodes stage = upstream timeout). animekai is stub-mode across the board. Stages `servers` and `stream` reporting `up:true` for gogoanime/animepahe with `last_ok: "0001-01-01T00:00:00Z"` is the zero-value: the probe has not run those stages yet because the prior stage (search/episodes) is failing.

---

## Disposition

- **gogoanime**: **FAIL** — search stage broken (anitaku.to → anineko.to migration). Real-world impact: even when registered, returns 0 episodes for any anime query.
- **animepahe**: **FAIL** — episodes stage broken (animepahe.ru IP-blocked, animepahe.io FingerprintJS-gated per code comments).
- **animekai**: **FALL-THROUGH-EXPECTED, NO SURVIVING TARGET** — animekai is stub-only (SCRAPER-KAI-01..04 not implemented); orchestrator's `ErrProviderDown` fall-through routes to gogoanime/animepahe, both of which also fail. Net result: no working English provider exists today.

---

## Action Required

**P0 providers FAIL and there is no working fall-through provider.** Per D2 in `.planning/phases/24-en-reconnect/24-CONTEXT.md`, Phase 24 frontend tasks (plans 02 + 03 + 04 + 05) should pause until at least one EN provider is recovered OR until Phase 26 (SCRAPER-HEAL-25 — AllAnime lift) lands.

**Recovery options for the operator:**

1. **Recover gogoanime** — port the parser to anineko.to (the migration target) or pin to a working mirror. Estimated effort: medium (parser surgery in `services/scraper/internal/providers/gogoanime/`).
2. **Recover animepahe** — either tunnel through a residential proxy that doesn't trigger the IP-block, or migrate to animepahe.io with FingerprintJS bypass. Estimated effort: medium-high.
3. **Skip ahead to Phase 26** — lift the existing `services/catalog/internal/parser/allanime/` client into a scraper-side provider (SCRAPER-HEAL-25). The AllAnime path is already used for raw-JP and known-working. Estimated effort: similar to Phase 24 frontend work (the EN tab + i18n + Anime.vue still ship; only the provider underneath changes).
4. **Ship Phase 24 anyway** — per D5 of CONTEXT.md, "the EN tab is shown UNCONDITIONALLY and an empty-state inside EnglishPlayer covers the 'no providers responding' case." This is a documented design decision. **However**, the Wave 0 hard gate (D2) was added specifically to prevent shipping into a fully-broken provider surface. Choosing this option requires explicit operator override of D2.

**Recommendation (Claude / autonomous executor):** Restore production env defaults (`SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe`, `SCRAPER_ANIMEKAI_ENABLED=false`), HALT Phase 24 frontend work, and surface this verdict log + the choice between options 1-4 to the human operator. The operator decides.

---

## Backend allow-list pre-flight (Plan 01 dependency check, deferred)

Plan 01 widens `services/player/internal/domain/preference.go::ValidPlayers` and `services/player/internal/handler/report.go::allowedPlayerTypes` to accept `"english"`. This backend change is independent of provider availability and could ship even with all providers down (it just unblocks the 422 on save/report when the EN player IS eventually wired). However, per D2's hard-gate semantics, Plan 01 also pauses until the gate clears.

---

## Commit

This file is committed in a new commit referencing SCRAPER-HEAL-20. Co-authors per `CLAUDE.md` / `MEMORY.md`.

---

## Post-ship verification — Phase 27 (2026-05-19)

**Requirement:** SCRAPER-HEAL-32 (Plan 27-04)
**Trigger:** Phase 27 ship — stealth-Chromium sidecar (`services/animepahe-resolver/`) + parser rewrite to UUID-session contract via sidecar transport. Phase 27 plans 27-01..27-03 landed; this gate verifies end-to-end before Plan 27-05 flips the `SCRAPER_DEGRADED_PROVIDERS` compose default.

**Override applied (mirrors Phase 24 verification posture):**
`docker/.env`: `SCRAPER_DEGRADED_PROVIDERS=__none__` (sentinel — parses to empty set, forces all providers to register).

### Updated verdict matrix

| Provider | Episodes? | Servers? | Stream URL Returned? | Stream URL Fetchable? | Disposition |
|---|---|---|---|---|---|
| **gogoanime** | <unchanged from Phase 24 — FAIL> | N/A | N/A | N/A | UNCHANGED (Phase 24's row stands; Phase 27 does not touch gogoanime) |
| **animepahe** | **PASS** (28 episodes) | **PASS** (>= 1 kwik server) | **PASS** (.m3u8 returned) | **PASS** (HTTP/2 200 with Referer=https://kwik.cx/ — see Referer note below) | **PASS** — sidecar transport + UUID-session contract working end-to-end |
| **animekai** | <unchanged — escape-hatch-stub> | N/A | N/A | N/A | UNCHANGED |
| **auto** | depends on order — animepahe alone now satisfies | — | — | — | Orchestrator can now return episodes when `prefer=animepahe` or when it falls through past gogoanime |

### Captured response excerpts

**animepahe episodes (truncated to first 2 + last 1, MAL 52991 = Frieren):**
```json
[
  {
    "id": "7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7",
    "number": 1,
    "title": "",
    "is_filler": false
  },
  {
    "id": "146ebd3e0f4370ccfcc22e5cddcb819d4b655dedaa8d1234d5c682ea6bb0a7c0",
    "number": 2,
    "title": "",
    "is_filler": false
  },
  {
    "id": "755447bbd153337e6549e60296e61c82428b2b12842a3fd8ca6c44878c191830",
    "number": 28,
    "title": "",
    "is_filler": false
  }
]
```

Episode IDs are now UUID-shaped session strings (matching the resolver's `^[A-Za-z0-9-]{16,128}$` schema), confirming the Phase 27 sidecar transport is delivering the new contract. (Phase 24's animepahe row failed with the legacy `id=5319` numeric — see SCRAPER-HEAL-32 deviation note in 27-04 SUMMARY.md.)

**animepahe servers (first 3, episode 1):**
```json
[
  {
    "id": "https://kwik.cx/e/aeNSh4eblrse",
    "name": "kwik",
    "type": "sub"
  },
  {
    "id": "https://kwik.cx/e/d3ccaeXzK7o4",
    "name": "kwik",
    "type": "sub"
  },
  {
    "id": "https://kwik.cx/e/iKAcIjd2ce0f",
    "name": "kwik",
    "type": "sub"
  }
]
```

Six total servers were returned (3 sub + 3 dub).

**animepahe stream (first source, episode 1, server 1):**
```json
{
  "url": "https://vault-08.uwucdn.top/stream/08/13/63abd0640a098853df01676699553c949b1b3038117d9f59232d56ca53be3fef/uwu.m3u8",
  "type": "hls",
  "headers": {
    "Referer": "https://kwik.cx/"
  }
}
```

**Referer note (correction to Plan 27-04 narration):** The parser's `kwikReferer` constant value `https://animepahe.pw/` is the Referer for fetching the Kwik *embed page* (kwik.cx/e/...), not for fetching the m3u8 itself. The DTO's `headers.Referer = "https://kwik.cx/"` is the correct Referer to use when loading the m3u8 — uwucdn.top expects `kwik.cx` as the referrer chain (kwik.cx → uwucdn.top, not animepahe → uwucdn). The plan body's gate-clear curl used `Referer: https://animepahe.pw/` which returned HTTP/2 403 (predictable — wrong referrer chain); re-running with the DTO-supplied `Referer: https://kwik.cx/` returned HTTP/2 200. The DTO surfacing of the correct Referer is the existing parser behavior — no DTO contract change.

**Stream HEAD response (Referer applied — DTO value):**
```
HTTP/2 200
date: Tue, 19 May 2026 11:11:59 GMT
content-type: application/vnd.apple.mpegurl
content-length: 22707
server: cloudflare
last-modified: Sun, 19 Nov 2000 08:52:00 GMT
expires: Sat, 08 May 2027 01:26:36 GMT
cache-control: max-age=31536000
etag: "3a1794b0-58b3"
access-control-allow-origin: *
```
The curl was `curl -sI -H "Referer: https://kwik.cx/" https://vault-08.uwucdn.top/stream/.../uwu.m3u8`.

**`/scraper/health` for animepahe (after fresh full pipeline run):**
```json
{
  "episodes": {
    "up": true,
    "last_ok": "2026-05-19T11:12:25.371479953Z"
  },
  "search": {
    "up": true,
    "last_ok": "2026-05-19T11:12:25.55037888Z"
  },
  "servers": {
    "up": true,
    "last_ok": "2026-05-19T11:12:25.482689651Z"
  },
  "stream": {
    "up": true,
    "last_ok": "2026-05-19T11:12:25.616941941Z"
  }
}
```

All four animepahe stages report `up:true`. `stream_segment` is owned by the probe runner (Plan 17) and excluded from this gate per the plan.

### Sidecar observability snapshot

`/metrics` from the resolver at gate-clear time:
```
stealth_challenge_failures_total{service="animepahe-resolver"} 0
stealth_challenge_solves_total{service="animepahe-resolver"} 0
page_recycle_total{service="animepahe-resolver"} 0
# upstream_403_total: no rows emitted (counter not incremented during this run)
```

Counter sanity: `stealth_challenge_failures_total` is 0 (no failed challenges); `stealth_challenge_solves_total` is 0 because the resolver did NOT encounter a DDoS-Guard challenge during this verification window — animepahe.pw served clean responses throughout the warm-page lifetime. This is healthy. If `stealth_challenge_failures_total >= 5` ever observed in production, surface as a follow-up — the pin in `STEALTH-PINS.md` may need refresh per Pattern 7.

### Phase 27 deviations surfaced by this gate-clear

Two Rule 1 bugs in 27-01/27-02 territory were discovered and auto-fixed during Task 2 (see Plan 27-04 SUMMARY.md for full deviation log):

1. **`services/scraper/internal/providers/animepahe/client.go`** — `FindID` accepted any string from malsync without validating shape, so legacy numeric IDs (e.g. `5319`) flowed through and failed the resolver's session-pattern schema with HTTP 400. Fix: validate against the session pattern and fall through to `/search` on mismatch. Added regression test `TestProvider_FindID_MalSyncLegacyNumeric`.
2. **`services/scraper/internal/embeds/kwik.go`** — modern Kwik embed pages contain TWO Dean-Edwards packer blocks (cookie helper + Plyr/HLS init); `extractPacker` only returned the first one, whose unpacked output has no m3u8 URL. Fix: added `extractAllPackers` + iterate in `Extract` until one yields an m3u8 match. Backward-compatible with single-packer pages.

Both fixes committed in `fix(27-04): parser+kwik gate-clear blockers (Rule 1 deviations, SCRAPER-HEAL-32)`.

### Disposition (post-ship)

- **animepahe column: FAIL → PASS.** The Phase 24 hard gate is now green for animepahe. Plan 27-05 is unblocked.
- gogoanime still FAILS; that's out of scope for Phase 27. Phase 28 (TBD) targets gogoanime recovery.
- `SCRAPER_DEGRADED_PROVIDERS` compose default still reads `gogoanime,animepahe` at the end of this plan — Plan 27-05 owns the flip. The `__none__` override in `docker/.env` is removed below.

### Production override restored

The `docker/.env` mutation from this verification is reverted to the pre-Phase-27 state — `SCRAPER_DEGRADED_PROVIDERS=__none__` line removed from `docker/.env` (relying on the compose default again). `make redeploy-scraper` re-runs and the scraper boots with `animepahe` in the degraded set again, as it was before this gate ran. The next plan (27-05) is what permanently removes animepahe from the compose default; this verification is a snapshot in time.

### Co-authors

(Standard per `MEMORY.md` / CLAUDE.md.)
