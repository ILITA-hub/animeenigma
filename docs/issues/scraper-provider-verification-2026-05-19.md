# Scraper Provider Verification — 2026-05-19

**Date:** 2026-05-19
**Target anime:** Frieren: Beyond Journey's End — MAL 52991 (Shikimori 52991)
**Internal anime UUID:** `f0b40660-6627-4a59-8dcf-7ec8596b3623`
**Requirement:** [SCRAPER-HEAL-20](../../.planning/phases/24-en-reconnect/24-CONTEXT.md) — Phase 24 Wave 0 hard gate
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
