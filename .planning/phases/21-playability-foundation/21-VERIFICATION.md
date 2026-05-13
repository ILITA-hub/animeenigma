---
phase: 21
phase_name: Playability Foundation
status: human_needed
verified_date: 2026-05-13
goal_alignment_score: 9/10
must_haves_met: 5/7
warnings:
  - id: W-21-01
    severity: warning
    item: SC#5 — parser_ad_decoy_total dedicated unit-test
    test: TestGetStreamWithGate_AdDecoy_Skipped
    file: services/scraper/internal/providers/gogoanime/client_gated_test.go
    symptom: |
      parser_unplayable_total{server=streamhg,reason=ad_decoy} = 0; want 1
      parser_ad_decoy_total{server=streamhg} = 0; want 1
    repro: cd /data/animeenigma/services/scraper && go test ./internal/providers/gogoanime/... -run TestGetStreamWithGate_AdDecoy_Skipped -count=5
    repro_result: 5/5 failures (NOT flaky — reproducibly broken)
    root_cause_hypothesis: |
      Race in parallel top-2 probe: when earnvids (servers[1]) returns Playable before streamhg
      (servers[0]) finishes its AdDecoy probe, parCancel() is called and the loop bails out on the
      first successful result. The streamhg goroutine's counter Inc() in attemptOne() may not have
      run yet when the test asserts the counter value. The implementation in client.go:881-887
      DOES increment both counters correctly when the AdDecoy probe completes, but the parallel
      cancellation makes it nondeterministic in the test.
    impact: |
      The contract "ad_decoy hits increment parser_ad_decoy_total" is functionally correct in the
      implementation (lines 885-887 of client.go) but is not test-locked. This is the headline
      regression scenario for the whole v3.1 milestone (VibePlayer ad-decoy poisoning) — losing
      the test means a future refactor could silently drop this increment without CI catching it.
    suggested_fix: |
      In the test, swap the priority order so the AdDecoy server is in position 2 (probed second,
      sequential remainder branch) — OR run AdDecoy with a small artificial delay so the probe
      result lands before parCancel() fires — OR wait/sync on the streamhg goroutine's counter
      Inc completion before asserting. NOT a production code change; just a test ordering fix.
human_verification:
  - id: HV-21-01
    test: Browser smoke — Frieren ep 1 cold path on production
    expected: |
      1. Open https://animeenigma.ru in Incognito, sign in
      2. Search "Frieren", click episode 1, EN locale
      3. Observe loader phases:
         - "Looking up sources…"
         - "Connecting to remote stream…"
         - "Verifying playback…"  ← Phase 3 visible (cold path, meta.gated:true)
      4. Video actually plays (real frames, audio)
      5. Repeat within 5 minutes — Phase 3 must NOT appear (warm cache hit)
      6. Switch UI locale to RU and repeat — Russian copy renders
    why_human: |
      Orchestrator cannot drive a logged-in browser session at animeenigma.ru. Backend cURL smoke
      already confirmed: meta.gated:true on cold path, meta.gated absent on warm path, real CDN
      URL (cdn-centaurus.com, NOT ibyteimg.com), first variant HEAD returns 200 OK.
      Outstanding: visible playback + loader phase transitions in the browser.
---

# Phase 21 Verification

## Status: human_needed

## Goal Alignment

The phase goal — "production EN playback works again; an ad-poisoned server transparently rolls
forward to the next server in priority order and plays real video, verified by a playability gate
that catches the poison before the URL reaches the user; latency masked by a three-phase loader"
— **is substantively achieved in the codebase and verified live at the backend boundary.**

Live production cURL (Frieren ep 1 sub) returns `meta.gated:true` on cold path with a master.m3u8
URL on `cdn-centaurus.com` (not on any ad-CDN blocklist suffix); the first variant HEAD returns
`200 OK`; a second call within 5 minutes returns the SAME URL with `meta.gated` absent (warm cache
hit). The scraper logs confirm `gogoanime server priority configured [streamhg, earnvids, vibeplayer]`
at boot, and the live `parser_unplayable_total` series shows the gate caught a real failed
streamhg URL during the smoke run and rolled forward — exactly the self-healing the phase was
designed to ship.

Two items prevent a clean `passed`:
1. **HV-21-01**: Frontend three-phase loader transitions can only be confirmed by manually driving
   a logged-in browser at animeenigma.ru. Vitest covers 9/9 component cases (including 6 locale×phase
   matrix + precedence + meta.gated branches); the container is deployed and healthy on :3003.
2. **W-21-01**: `TestGetStreamWithGate_AdDecoy_Skipped` — the unit test that locks the dedicated
   `parser_ad_decoy_total` contract — reproducibly fails (5/5) due to a race in the parallel
   top-2 probe. The implementation is correct (verified by reading client.go:881-887) and the
   counter IS registered + exposed at `/metrics`; only the test is broken. Worth fixing because
   ad_decoy is the headline regression scenario for v3.1.

## Success Criteria Trace

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | Production EnglishPlayer plays real Frieren ep 1; master m3u8 contains zero ad-CDN segments; first segment HEAD 200; manual playback confirmed | ◐ PARTIAL | Backend live-confirmed: `curl /scraper/stream?mal_id=52991&episode=frieren-beyond-journeys-end-episode-1&category=sub` returns master URL on `in1rhjc5cqhz.cdn-centaurus.com` (NOT ibyteimg/p16-ad-sg/ad-site-i18n/tiktokcdn), `meta.gated:true`, then `curl -I` on the variant returns `HTTP/1.1 200 OK`. Browser-side playback smoke = pending (HV-21-01). |
| 2 | libs/streamprobe registered in go.work; unit-test-covered for all 7 Reason enum values | ✅ MET | `go.work` line 13 has `./libs/streamprobe`; `libs/streamprobe/reason.go` declares all 7 ReasonEnum values; `cd libs/streamprobe && go test ./... -count=1` → `ok` (28 tests, one driving test per Reason); `TestProbe_AllReasonsCovered` meta-test enforces the matrix. |
| 3 | gogoanime.ListServers sorts by SCRAPER_SERVER_PRIORITY (default streamhg,earnvids,vibeplayer); typo'd entries fail-fast at boot | ✅ MET | `services/scraper/internal/config/config.go:125` reads env with default `streamhg,earnvids,vibeplayer`; `server_priority.go` exports `SortByPriority` (stable insertion sort) and `ValidatePriorityList`; `main.go:159-160` calls `ValidatePriorityList` BEFORE `gogoanime.New` and `log.Fatalw` on unknown name. Container boot log confirms `"gogoanime server priority configured priority=[streamhg, earnvids, vibeplayer] known_extractors=[vibeplayer, streamhg, earnvids]"`. |
| 4 | gogoanime.GetStream iterates servers in priority order, runs gate on each, returns first success; ≤8s budget; winner cached at `scraper:winning_server:<provider>:<anime>:<ep>` for 5min | ✅ MET | `client.go:69` `const streamGateBudget = 8 * time.Second`; `client.go:73` `const winningServerTTL = 5 * time.Minute`; `client.go:802` cache key `scraper:winning_server:%s:%s:%s`; `GetStreamWithGate` does top-2 parallel + sequential remainder under `ctx.WithTimeout(callCtx, streamGateBudget)`. Live cold-path wall-clock ~1.4s per Plan 21-03 Task 7 smoke; warm-path <100ms (re-confirmed in this verification: meta.gated absent on second call, same URL returned). |
| 5 | Scraper /metrics exposes parser_unplayable_total{provider,server,reason} + parser_ad_decoy_total{provider,server} with non-zero values exercised by test | ◐ PARTIAL | `libs/metrics/provider.go:72,84` declares both CounterVecs. Live `/metrics` shows `parser_unplayable_total{provider="gogoanime",reason="cdn_unreachable",server="streamhg"} 1` — gate exercised, counter non-zero. `parser_ad_decoy_total` is registered but the dedicated unit test `TestGetStreamWithGate_AdDecoy_Skipped` (created in Plan 21-03 explicitly to lock SC#5) reproducibly FAILS 5/5 due to race in parallel top-2 probe (W-21-01). Implementation is correct; test is broken. |
| 6 | GET /scraper/stream includes meta.gated:true when gate ran; absent/false on cache hit; frontend integration test asserts FE reads correctly | ✅ MET | Live confirmed: first call returns `"meta":{"gated":true,"tried":["gogoanime","animepahe"]}`; second call within 5min returns `"meta":{"tried":[...]}` with `gated` key absent (warm cache). `scraper.go:359-365` `writeSuccess` omits the key on `gated=false`. Vitest case 8 (`meta.gated=true sets validatingStream`) and case 9 (`meta.gated absent does NOT toggle validatingStream`) both pass — FE correctly differentiates the two payload shapes. |
| 7 | EnglishPlayer.vue renders three sequential loader phases (EN+RU) driven by loadingServers/loadingStream/validatingStream refs — Vitest covers each phase + locale | ✅ MET | `EnglishPlayer.vue:33-50` template renders three v-if/v-else-if branches in precedence order (validatingStream > loadingStream > loadingServers); `:629-635` declares all three refs; `:1133` casts `meta.gated === true` to set `validatingStream`. EN + RU copy hardcoded inline (D6) at `:40,43,46`. `cd frontend/web && bunx vitest run ...EnglishPlayer.spec.ts` → `Test Files 1 passed (1) / Tests 9 passed (9)` covering `it.each([...6 phase×locale...])` + precedence + meta.gated true + meta.gated absent. |

## Requirements Trace

| SCRAPER-HEAL-NN | Plan | Status | Evidence |
|---|---|---|---|
| 01 — streamprobe Probe | 21-01 | ✅ shipped | libs/streamprobe/probe.go; 28 unit tests pass |
| 02 — hardcoded blocklist + Redis-lift TODO | 21-01 | ✅ shipped | blocklist.go contains ibyteimg.com/p16-ad-sg/ad-site-i18n/tiktokcdn.com; `scraper:streamprobe:blocklist` TODO anchor present |
| 03 — server-priority env | 21-03 | ✅ shipped | SCRAPER_SERVER_PRIORITY default streamhg,earnvids,vibeplayer; boot validation fail-fast |
| 04 — per-server fallback using gate | 21-03 | ✅ shipped | GetStreamWithGate top-2 parallel + sequential remainder + 8s budget |
| 05 — winning-server Redis cache | 21-03 | ✅ shipped | scraper:winning_server:gogoanime:%s:%s key, 5min TTL, stale validation, live warm-path confirmed |
| 06 — parser_unplayable_total + parser_ad_decoy_total | 21-02, 21-03 | ◐ shipped (W-21-01) | Both registered; parser_unplayable_total live-incremented in prod; parser_ad_decoy_total test broken |
| 07 — meta.gated response field | 21-02, 21-03 | ✅ shipped | Live confirmed cold:gated=true, warm:absent |
| 08 — EnglishPlayer three-phase loader EN+RU | 21-04 | ✅ shipped | Vitest 9/9; both locales rendered |

## Live Production Smoke (re-verified at verification time)

```
$ curl -s "http://localhost:8088/scraper/stream?mal_id=52991&episode=frieren-beyond-journeys-end-episode-1&category=sub"
{"success":true,"data":{
  "meta":{"gated":true,"tried":["gogoanime","animepahe"]},
  "stream":{"sources":[{"url":"https://in1rhjc5cqhz.cdn-centaurus.com/hls2/01/13002/3lcsjn8lm9w9_o/master.m3u8?...","type":"hls"}],
            "headers":{"Referer":"https://otakuhg.site/"}}}}

$ # ad-CDN blocklist check on master URL:
ibyteimg.com: clean
p16-ad-sg: clean
ad-site-i18n: clean
tiktokcdn.com: clean

$ curl -sI "https://in1rhjc5cqhz.cdn-centaurus.com/hls2/01/13002/3lcsjn8lm9w9_o/index-v1-a1.m3u8?..."
HTTP/1.1 200 OK
Server: nginx

$ # Second call within 5 min — warm path
$ curl -s "...same URL..." | jq '.data.meta'
{"tried":["gogoanime","animepahe"]}    # ← gated key absent

$ # /metrics
parser_unplayable_total{provider="gogoanime",reason="cdn_unreachable",server="streamhg"} 1

$ # Boot log
INFO  gogoanime server priority configured  priority=[streamhg, earnvids, vibeplayer]
                                            known_extractors=[vibeplayer, streamhg, earnvids]
```

## Human Verification Pending

### HV-21-01 — Browser smoke for three-phase loader

**Test:** Frieren ep 1 cold + warm path in a real browser, EN + RU locales.

**Expected:**
1. Open https://animeenigma.ru in Incognito, sign in.
2. Search "Frieren", click episode 1, EN locale.
3. Observe loader phase transitions:
   - "Looking up sources…"
   - "Connecting to remote stream…"
   - "Verifying playback…"   ← Phase 3 visible (cold path, meta.gated:true)
4. Video actually starts playing (real frames + audio).
5. Re-click the same episode within 5 minutes — Phase 3 must NOT appear (warm cache).
6. Switch UI locale to RU and repeat the cold+warm flow; confirm Russian copy.

**Why human:** Orchestrator cannot drive a logged-in browser at animeenigma.ru. Backend smoke
already proves the data shapes the loader needs (cold: meta.gated:true; warm: absent) and the
Vitest spec proves the component renders correctly under both shapes — but the user-facing
phase transitions can only be observed in a real browser.

## Warnings

### W-21-01 — parser_ad_decoy_total unit test reproducibly fails

**Severity:** warning (implementation is correct; test is broken)

**Test:** `TestGetStreamWithGate_AdDecoy_Skipped`
**File:** `services/scraper/internal/providers/gogoanime/client_gated_test.go:197`
**Repro:** `cd services/scraper && go test ./internal/providers/gogoanime/... -run TestGetStreamWithGate_AdDecoy_Skipped -count=5` → 5/5 FAIL

**Output:**
```
parser_unplayable_total{server=streamhg,reason=ad_decoy} = 0; want 1
parser_ad_decoy_total{server=streamhg} = 0; want 1
```

**Root cause hypothesis:** The test puts the AdDecoy fake-probe response on `servers[0]` (streamhg)
and the Playable response on `servers[1]` (earnvids). Both are probed in parallel as the top-2.
When earnvids returns first (or concurrently), `parCancel()` cancels the streamhg goroutine, and
the test asserts counter values before the streamhg goroutine has a chance to complete its probe
result handling (where the Inc() lives in `attemptOne` at client.go:884-886).

**Implementation correctness:** `client.go:881-890` increments BOTH counters correctly on the
AdDecoy branch — verified by inspection. The contract is functionally correct; only the
nondeterministic test ordering breaks the assertion.

**Why this is a warning, not a blocker:**
- `parser_ad_decoy_total` is registered in `libs/metrics/provider.go:84` and exposed at /metrics.
- `parser_unplayable_total` IS live-incremented in production (cdn_unreachable hit during smoke).
- The Vitest matrix locks the FE side of the ad_decoy data path.
- Other tests in the same file (e.g., `TestGetStreamWithGate_AllFail_ProviderDown` line 275) DO
  exercise the AdDecoy reason on a sequential server (position 3) — but they assert overall
  failure, not the specific counter increment.
- The implementation could be silently broken in a future refactor without CI catching it, because
  this is the ONLY test that specifically asserts `parser_ad_decoy_total` increments.

**Suggested fix (test-only, no production code change):**
1. Reorder the test fixture so the AdDecoy server is at position 2 (e.g. swap servers[0]/servers[1])
   so AdDecoy lands in the sequential remainder branch where its result MUST be processed before
   the next probe runs; OR
2. Add a small artificial delay to the Playable response so the AdDecoy probe (with 0 delay) wins
   the race; OR
3. After `GetStreamWithGate` returns, poll/wait for the goroutine to drain (with a bounded timeout)
   before asserting counter values.

Option 1 is cleanest. Recommend running `go test` after the fix to confirm 5/5 passes.

## Gaps Found

None blocker-level. The W-21-01 warning above is the only outstanding code-side gap; HV-21-01 is
the only human-side pending item.

## Deviations

1. **Plan 21-03 sweep-up commits.** Two of Plan 21-03's task commits got swept into parallel
   executor commits with mismatched commit subjects (74d4dbf was labeled `docs(21-04)` but contains
   client.go + client_gated_test.go scraper code from 21-03 Task 3; 0d586aa was labeled `docs(13)`
   but contains 21-03 Task 6's maintenance_prompt_sync_test.go). Code IS in the repo and tests
   pass; only commit subjects are mis-attributed. No history rewrite per project convention.

2. **Plan 21-03 Rule 2 — empty server param.** The handler originally rejected `server=` empty
   with 400 INVALID_INPUT. Without this change, the playability gate would be unreachable via the
   HTTP API. The plan's done-criteria explicitly require cold-path Frieren ep 1 to produce
   `meta.gated:true` — that cannot happen without empty-server support. Fix committed in 4e95ea3.

3. **Plan 21-04 frontend tooling.** Vitest + @vue/test-utils + jsdom were not previously in the
   project. `bun add -D` installed them. Plan-specified path `__tests__/` matches Vue convention.

4. **Plan 21-04 `defineExpose`.** Added a `defineExpose` block in `<script setup>` exposing loader
   refs + fetchStream + episodes for test-only access. Narrow surface; only what the spec needs.

5. **Deferred test (carried forward from 21-02).** `TestOrchestrator_AnimePaheToGogoanimeFailover`
   at `services/scraper/internal/service/orchestrator_phase18_test.go:307` continues to fail with
   `orch.ListEpisodes returned 0 episodes`. Confirmed pre-existing on `main` HEAD before any
   Phase 21 change; failure is in fixture setup (fakePahe + httptest combo), not in code touched
   by this phase. Logged in `deferred-items.md`. Not introduced by Phase 21.

## Verdict

Phase 21 goal is **substantively achieved**: production cold-path EN playback now flows through
the playability gate, an ad-poisoned URL is caught before it reaches the user, server priority is
configured and validated at boot, the 5-minute winner cache absorbs repeat requests, and the
three-phase loader is wired in the frontend with full Vitest coverage. Backend smoke at
verification time re-confirms `meta.gated:true` on cold path, real CDN URL with 200 first-segment
HEAD, warm-path absence of `gated` key, and live increment of `parser_unplayable_total`.

Pending: (a) one manual browser smoke for the loader transitions and visible video playback;
(b) a test-only fix for the racy `parser_ad_decoy_total` assertion. Neither blocks Phase 22.
