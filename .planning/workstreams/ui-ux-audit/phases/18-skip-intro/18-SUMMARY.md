---
phase: 18
plan: 1
subsystem: player
workstream: ui-ux-audit
tags: [skip-intro, aniskip, player, hianime, consumet, ux-34]
dependency_graph:
  requires: []
  provides: [skip-intro-cta, aniskip-proxy]
  affects: [HiAnimePlayer.vue, ConsumetPlayer.vue, Anime.vue]
tech_stack:
  added: [api.aniskip.com v2 (external)]
  patterns: [backend cache.GetOrSet proxy, Vue composable, reactive watch + abort-token]
key_files:
  created:
    - services/catalog/internal/handler/skip_times.go
    - frontend/web/src/composables/useSkipTimes.ts
    - .planning/workstreams/ui-ux-audit/phases/18-skip-intro/18-SUMMARY.md
    - .planning/workstreams/ui-ux-audit/phases/18-skip-intro/18-VERIFICATION.md
  modified:
    - services/catalog/internal/transport/router.go
    - services/catalog/cmd/catalog-api/main.go
    - services/gateway/internal/transport/router.go
    - frontend/web/src/api/client.ts
    - frontend/web/src/components/player/HiAnimePlayer.vue
    - frontend/web/src/components/player/ConsumetPlayer.vue
    - frontend/web/src/views/Anime.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - Cache TTL 7d (skip timestamps are crowdsourced and effectively immutable).
  - episodeLength=0 wildcard required by aniskip v2 (returns HTTP 400 otherwise).
  - camelCase JSON passthrough (startTime/endTime/skipType) — mirrors upstream.
  - -1s tail on visibility window (currentTime < end - 1) prevents post-seek flicker.
  - Graceful degradation everywhere — null malId, 404, network error → no CTA, no error surface.
metrics:
  duration_minutes: ~45
  completed: 2026-05-13
  tasks: 6
  commits: 6
  files_touched: 11
requirements: [UX-34]
---

# Phase 18 Plan 1: Skip-Intro detection (Griffin) Summary

Surfaced a "Skip Intro" / "Skip Ending" CTA in the HiAnime and Consumet players using crowdsourced aniskip.com timestamps. Closes UX-34 / Tier E #13.

## One-liner

Backend proxy of api.aniskip.com/v2 with 7-day Redis cache + a per-player reactive composable that renders a cyan bottom-right CTA inside the OP / ED window and seeks past it on click.

## What shipped

**Backend (Waves 1–2):**
- `SkipTimesHandler` in catalog (5s upstream timeout, GetOrSet w/ 7-day TTL, coerces all upstream failure modes to a uniform `{ found: false, results: [] }` shape).
- Public route `GET /api/skip-times/{malId}/{episode}` on catalog, no auth.
- Gateway proxy `/api/skip-times/*` → catalog. Public passthrough, registered alongside the other public catalog routes.

**Frontend (Waves 3–5):**
- `useSkipTimes(malId, episode)` composable — reactive `{ opening, ending, loading, error }`. In-flight token guards against fast-scrub races. Maps aniskip's `op` / `ed` / `mixed-op` / `mixed-ed` skip types to a uniform `SkipSegment` shape.
- `animeApi.getSkipTimes(malId, ep)` on `client.ts`.
- `HiAnimePlayer.vue` + `ConsumetPlayer.vue`: new `malId` prop, useSkipTimes wiring, `showSkipIntro` / `showSkipOutro` computed (window: `currentTime ∈ [start, end - 1)`), `seekTo(time)` player-agnostic seek (videojs `currentTime(t)` / native `el.currentTime = t`), and two cyan overlay buttons (`absolute bottom-20 right-4 z-10`) via `v-if`/`v-else-if` so OP and ED never stack.
- `views/Anime.vue`: passes `:mal-id="anime.malId"` to both players.
- i18n: `player.skipIntro` / `player.skipOutro` × 3 locales (EN / RU / JA).

## Commits

| Wave | Hash | Message |
|---|---|---|
| 1 | `1a29c58` | feat(ui-ux-audit/18): SkipTimesHandler with aniskip proxy + 7d cache |
| 2 | `1e5ad21` | feat(ui-ux-audit/18): gateway /api/skip-times proxy → catalog |
| 3 | `5122e03` | feat(ui-ux-audit/18): useSkipTimes composable + animeApi.getSkipTimes |
| 4a | `99ce2c3` | feat(ui-ux-audit/18): HiAnimePlayer skip-intro/outro overlay |
| 4b | `72a800f` | feat(ui-ux-audit/18): ConsumetPlayer skip-intro/outro overlay |
| 5 | `f7e9efd` | feat(ui-ux-audit/18): player.skipIntro / player.skipOutro i18n (en/ru/ja) |

## Deviations from Plan

### Plan corrections discovered during execution

**1. [Rule 1 — Bug] Aniskip v2 requires `episodeLength` query param**
- **Found during:** Wave 1 smoke test against `/api/skip-times/1535/1` returned `found:false` for every anime ID.
- **Issue:** Plan + CONTEXT.md spec the aniskip URL as `https://api.aniskip.com/v2/skip-times/{malId}/{ep}?types=op,ed`. Calling that directly returns HTTP 400 with `"episodeLength must be a number conforming to the specified constraints"`. The handler was silently coercing the 400 to an empty result, masking the bug.
- **Fix:** Added `episodeLength=0` to the query string (verified upstream — `0` works as a wildcard returning all crowdsourced submissions regardless of declared episode length). Documented inline in `fetchFromUpstream`.
- **Files modified:** `services/catalog/internal/handler/skip_times.go`
- **Commit:** Included in `1a29c58` (Wave 1).

**2. [Rule 1 — Bug] Aniskip v2 JSON is camelCase, not snake_case**
- **Found during:** Same smoke test as #1.
- **Issue:** Plan + CONTEXT.md docs the response as `{ skip_type, start_time, end_time }`. Real upstream emits `{ skipType, startTime, endTime }`. JSON tags on `SkipTimesResultItem` were originally snake_case → decode lost the `Interval` block and re-emitted zero-valued segments.
- **Fix:** Updated struct tags to camelCase to passthrough the upstream shape verbatim. Composable consumes the same camelCase keys.
- **Files modified:** `services/catalog/internal/handler/skip_times.go`
- **Commit:** Included in `1a29c58` (Wave 1).

**3. [Rule 2 — Missing critical functionality] Path-param validation**
- **Found during:** Initial handler review.
- **Issue:** Plan describes parsing `:malId` and `:episode` but doesn't specify validation. Without it, malformed traffic pollutes the cache with arbitrary string keys (`skip-times:<script>:foo`) and the upstream URL builder runs with unsanitized input.
- **Fix:** Validate `malId` is a positive integer (aniskip uses MAL numeric IDs) and `episode` is `>= 1` BEFORE touching the cache or the upstream. 400 with a descriptive message on either failure. Verified: `curl /api/skip-times/abc/1` → 400 INVALID_INPUT, `curl /api/skip-times/52991/0` → 400 INVALID_INPUT.
- **Files modified:** `services/catalog/internal/handler/skip_times.go`
- **Commit:** Included in `1a29c58` (Wave 1).

### Verification-gate adjustments

**4. [Rule 3 — Blocking] `scripts/i18n-lint.sh` does not exist**
- **Found during:** Wave 6 verification.
- **Issue:** Plan's verification checklist includes `bash scripts/i18n-lint.sh clean`. That script is not present in `scripts/`. The other workstreams' SUMMARY files don't reference it either — it appears to be a planning-time aspiration rather than an existing tool.
- **Fix:** Replaced with direct JSON validation (`python3 -c "import json; json.load(...)"` on all three locale files) and a check that the new keys exist in all three locales. Documented in VERIFICATION.md.
- **Files modified:** none — verification methodology adjusted only.

## Known Stubs

None. Both the backend handler and the composable have graceful degradation paths (404, network error, missing malId) that the player respects via `v-if`. There are no placeholder strings, mock data sources, or unwired components.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: new-outbound-call | services/catalog/internal/handler/skip_times.go | New outbound HTTP egress to api.aniskip.com. 5s timeout caps blast radius. No PII leaves our infra (only MAL ID + episode number, both already public). |

## Self-Check: PASSED

- `services/catalog/internal/handler/skip_times.go` — FOUND
- `services/catalog/internal/transport/router.go` — MODIFIED (skip-times route registered)
- `services/catalog/cmd/catalog-api/main.go` — MODIFIED (SkipTimesHandler wired)
- `services/gateway/internal/transport/router.go` — MODIFIED (proxy route)
- `frontend/web/src/composables/useSkipTimes.ts` — FOUND
- `frontend/web/src/api/client.ts` — MODIFIED (getSkipTimes)
- `frontend/web/src/components/player/HiAnimePlayer.vue` — MODIFIED (overlay + composable)
- `frontend/web/src/components/player/ConsumetPlayer.vue` — MODIFIED (overlay + composable)
- `frontend/web/src/views/Anime.vue` — MODIFIED (:mal-id passthrough)
- `frontend/web/src/locales/{en,ru,ja}.json` — MODIFIED (2 keys each)
- All 6 commits visible in `git log --oneline`.
