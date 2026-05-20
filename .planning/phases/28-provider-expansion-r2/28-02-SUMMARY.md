---
phase: 28-provider-expansion-r2
plan: 02
subsystem: api
tags: [scraper, animefever, en-failover, html-scrape, goquery, jaro-winkler, hls, provider-lift]

# Dependency graph
requires:
  - phase: 26-allanime-en-lift
    provides: domain.Provider template + allanime client.go shape + cache.go pattern
  - phase: 25-scraper-hardening
    provides: HLSProxyAllowedDomains fail-closed gate (libs/videoutils/proxy.go)
  - phase: 15-scraper-foundation
    provides: BaseHTTPClient, domain.Provider/Registry/EmbedExtractor, health.AllStages
provides:
  - "services/scraper/internal/providers/animefever — failover slot 4 EN provider"
  - "AnimeFever upstream HTML/JSON scrape pipeline (search → info → watch → AJAX iframe)"
  - "Compile-time domain.Provider conformance assertion for animefever.Provider"
  - "HLS proxy allowlist entries for am.vidstream.vip + static-cdn-ca1.mofl.pro"
  - "Phase 19 wiring invariant updated to expect 4 (or 5 w/ animekai) registered providers"
affects: [28-03-vidstream-vip-extractor, 28-06-source-dropdown-polish, v3.1-milestone-audit]

# Tech tracking
tech-stack:
  added: []  # no new external Go modules — pure stdlib + goquery (pre-existing)
  patterns:
    - "Provider clone of allanime/client.go shape with HTML-scrape data path adaptation"
    - "stale-ctk evict-and-retry-once pattern via internal sentinel error"
    - "Cookie jar implicit via BaseHTTPClient (PHPSESSID propagation Pitfall 2)"
    - "fuzzy.JaroWinkler ≥0.85 threshold against fuzzy.NormalizeTitle output"

key-files:
  created:
    - services/scraper/internal/providers/animefever/doc.go
    - services/scraper/internal/providers/animefever/dto.go
    - services/scraper/internal/providers/animefever/cache.go
    - services/scraper/internal/providers/animefever/client.go
    - services/scraper/internal/providers/animefever/client_test.go
    - services/scraper/internal/providers/animefever/testdata/search_frieren.html
    - services/scraper/internal/providers/animefever/testdata/info_frieren.html
    - services/scraper/internal/providers/animefever/testdata/watch_ep28.html
    - services/scraper/internal/providers/animefever/testdata/ajax_load_ep28.json
    - .planning/phases/28-provider-expansion-r2/deferred-items.md
  modified:
    - services/scraper/internal/config/config.go
    - services/scraper/internal/config/config_test.go
    - services/scraper/cmd/scraper-api/main.go
    - libs/videoutils/proxy.go

key-decisions:
  - "Split ID format <slug>:<eid> via strings.LastIndex(:) so dots in slug are preserved (e.g. frieren-beyond-journeys-end.14401)."
  - "Cache servers as []string{tserver,hserver} instead of probing AJAX — server set is static; orchestrator handles fall-through."
  - "ctk cached separately from servers (15min TTL) so an expired token doesn't invalidate the server list."
  - "Stale-ctk handling uses internal errStaleCtk sentinel wrapped under ErrExtractFailed for the retry-once path."
  - "Frieren E2E live gate (Task 4) deferred to post-merge per orchestrator guidance — 28-03 vidstream_vip extractor is the missing piece."

patterns-established:
  - "Pattern: Per-host rate-limit triple at construction (animefever.cc + am.vidstream.vip + CDN) matches allanime's 2-host shape."
  - "Pattern: Stale-token retry-once via sentinel error wrapped under ErrExtractFailed."

requirements-completed: [SCRAPER-HEAL-36]

# Metrics
duration: 6 min
completed: 2026-05-20
---

# Phase 28 Plan 02: AnimeFever Provider Lift Summary

**AnimeFever EN provider registered as failover slot 4 — full domain.Provider implementation (HTML scrape + AJAX-POST data path, JaroWinkler title match, embed delegation to vidstream_vip)**

## Performance

- **Duration:** ~6 min (3 atomic task commits)
- **Started:** 2026-05-20 (worktree-agent-a161967d4e0368dec, base dc7f89f)
- **Completed:** 2026-05-20
- **Tasks:** 3 implemented + 1 deferred checkpoint (Task 4, see below)
- **Files modified:** 14 (10 created, 4 modified)

## Accomplishments

- New `animefever.Provider` satisfies `domain.Provider` end-to-end (6 methods + HealthCheck + Name) with compile-time assertion in both `client.go` and `client_test.go`.
- HTML-scrape pipeline implemented end-to-end against captured testdata fixtures: `/search/<term>` → `/info/<slug>` → `/watch/<slug>?ep=<eid>` → POST `/ajax/anime/load_episodes_v2`.
- Embed delegation wired to `domain.Registry.Find`; routed iframe URL goes to whichever extractor matches (Plan 28-03's `vidstream_vip.go` is the production target).
- main.go registers AnimeFever as failover slot 4 after AllAnime, before AnimeKai gated block. Phase 19 wiring invariant expanded to 4 candidate providers (`gogoanime`, `animepahe`, `allanime`, `animefever`).
- HLS proxy allowlist (`libs/videoutils/proxy.go::HLSProxyAllowedDomains`) gains `am.vidstream.vip` + `static-cdn-ca1.mofl.pro` in the SAME commit per D7.
- 12 unit tests cover FindID (success/empty/no-match/transport-fail), ListEpisodes (≥28 episodes sorted ascending), ListServers (both tserver+hserver in order), GetStream (success via fake extractor / no-extractor → ErrExtractFailed / AJAX status:false → ErrExtractFailed), New() Dep validation (HTTP/Embeds/Cache required), Name(), markStage on success/failure. All pass with `-race -count=2`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Scaffold animefever package + config block** — `d30d39c` (feat)
2. **Task 2: Implement Provider client.go + 12 table-driven tests** — `7da5fa6` (feat)
3. **Task 3: Register in main.go + HLS proxy allowlist + Phase 19 invariant** — `4668ee6` (feat)
4. **Task 4: Frieren E2E gate** — DEFERRED to post-merge (see Deviations)

## Files Created/Modified

**Created:**
- `services/scraper/internal/providers/animefever/doc.go` (69 lines) — package doc + lift decision log + upstream data path notes
- `services/scraper/internal/providers/animefever/dto.go` (37 lines) — `ajaxLoadEpisodeResponse` + `episodeRef` internal DTOs
- `services/scraper/internal/providers/animefever/cache.go` (172 lines) — 5 key families (show, episodes, servers, stream, ctk) with `scraper:animefever:` prefix
- `services/scraper/internal/providers/animefever/client.go` (674 lines) — `Provider`, `Deps`, `New`, `Name`, `markStage`, `HealthCheck`, `FindID`, `ListEpisodes`, `ListServers`, `GetStream`, helpers, compile-time assertion
- `services/scraper/internal/providers/animefever/client_test.go` (484 lines) — 12 test cases with in-memory cache + fake embed extractor + httptest fixtures
- `services/scraper/internal/providers/animefever/testdata/{search_frieren.html, info_frieren.html, watch_ep28.html, ajax_load_ep28.json}` — golden files for offline tests
- `.planning/phases/28-provider-expansion-r2/deferred-items.md` — out-of-scope discovery log

**Modified:**
- `services/scraper/internal/config/config.go` — new `AnimeFeverConfig{BaseURL}` block, `SCRAPER_ANIMEFEVER_BASE_URL` env binding, default `https://animefever.cc`, URL parse validation
- `services/scraper/internal/config/config_test.go` — 3 new tests (defaults / override / invalid URL)
- `services/scraper/cmd/scraper-api/main.go` — import animefever, register provider, update Phase 19 invariant from 3 → 4 candidate providers, add `animefever_base_url` to startup banner
- `libs/videoutils/proxy.go::HLSProxyAllowedDomains` — `am.vidstream.vip` + `static-cdn-ca1.mofl.pro`

## Test Fixtures and Behaviors Asserted

| Fixture | Asserted Behavior |
|---------|-------------------|
| `search_frieren.html` | FindID parses `div.card-block`, picks slug `frieren-beyond-journeys-end.14401` with JaroWinkler ≥0.85; cards below threshold yield ErrNotFound. |
| `info_frieren.html` | ListEpisodes returns exactly 28 episodes (1..28) sorted ascending; Episode.ID format `<slug>:<eid>`. |
| `watch_ep28.html` | ListServers returns `[tserver, hserver]` (tserver primary per Pitfall 3); CTK extraction regex matches `var ctk = '1f13010abb82454ebdc982c366dcaf17';`. |
| `ajax_load_ep28.json` | GetStream parses `{status:true, value:"<iframe ...>", embed:true}`; iframe URL routed to embed Registry; fake extractor returns the `static-cdn-ca1.mofl.pro/master.m3u8` HLS source unchanged. |

## main.go Registration Block

Registered AFTER AllAnime, BEFORE AnimeKai (failover slot 4 per CONTEXT.md D5):

```go
animeFeverBaseHTTP := domain.NewBaseHTTPClient(log,
    domain.WithPerHostRPS("animefever.cc", 1.0, 2),
    domain.WithPerHostRPS("am.vidstream.vip", 1.0, 2),
    domain.WithPerHostRPS("static-cdn-ca1.mofl.pro", 2.0, 4),
)
animeFeverProvider, err := animefever.New(animefever.Deps{
    BaseURL: cfg.AnimeFever.BaseURL,
    HTTP:    animeFeverBaseHTTP,
    Embeds:  registry,
    Cache:   redisCache,
    Log:     log,
})
// kill-switch + orchestrator.Register(...) follow the allanime pattern
```

Phase 19 wiring invariant updated:
```go
candidateProviders := []string{"gogoanime", "animepahe", "allanime", "animefever"}
```

## HLS Proxy Allowlist Entries Added

```go
// Phase 28 (SCRAPER-HEAL-36) — AnimeFever embed + HLS CDN hosts.
// am.vidstream.vip serves the JWPlayer embed page; static-cdn-ca1.mofl.pro
// hosts the master.m3u8 + segment payloads.
"am.vidstream.vip",
"static-cdn-ca1.mofl.pro",
```

## Frieren E2E Gate Evidence

**DEFERRED to post-merge build gate.**

Per the parallel-worktree objective: Plan 28-02 depends on Plan 28-03's `vidstream_vip` embed extractor for the Frieren E2E to pass. 28-03 is running in a sibling worktree and HEAD does not yet have its commits. The live Task 4 checkpoint (`curl ... | jq '. | length' ≥ 28`) requires:
1. `make redeploy-scraper` after merge — only the orchestrator will have all post-merge commits;
2. Production network access to `animefever.cc`, `am.vidstream.vip`, and `static-cdn-ca1.mofl.pro` — not available in this worktree sandbox.

**Recommended re-run path** (post-merge):
```bash
# After 28-02 + 28-03 are merged
cd /data/animeenigma && make redeploy-scraper
sleep 30
curl -s http://localhost:8088/scraper/health | jq '.providers.animefever.stages'
curl -s 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=animefever' | jq '. | length'
# Expected: ≥ 28
```

If 28-03 lands FIRST (vidstream_vip extractor registered in main.go), this plan's `p.embeds.Find(iframeURL)` will return that extractor and GetStream resolves end-to-end. If 28-03 ships an `am.vidstream.vip` host matcher, no further change to 28-02 is required.

## Decisions Made

- **`<slug>:<eid>` split via `strings.LastIndex(":")`**: AnimeFever slugs may contain `.` (e.g. `frieren-beyond-journeys-end.14401`) but never `:`. `strings.LastIndex` is robust to colons accidentally appearing in slugs in future upstream changes.
- **Static server list cached as `[]string{"tserver","hserver"}`**: AnimeFever's server set is fixed (per Pitfall 3 + spike notes). Probing the AJAX endpoint just to enumerate them would waste a request. The orchestrator's "try first server, fall through on failure" pattern handles unavailable servers per-episode.
- **ctk cached separately from servers (15min TTL)**: the ctk token is per-watch-page and rotates faster than the server set. Storing them in the same cache entry would coupled their TTLs unnecessarily. Token eviction on `status:false` AJAX response handles the live-rotation case.
- **No new external Go modules added**: `goquery` is already in `services/scraper/go.mod` (used by animepahe). The plan's "no new deps" gate is preserved.

## Deviations from Plan

### Auto-fixed Issues — none

### Deferred (with rationale)

**1. Task 4 (Frieren E2E human-verify checkpoint) — explicitly deferred to post-merge**

- **Found during:** Plan setup (worktree sandbox does not have access to 28-03's vidstream_vip extractor or production network).
- **Plan-mandated approach:** Per the objective passed to this executor, "Wait/document and skip the live E2E (it'll be re-run in Wave 2 post-merge)" was the recommended path.
- **Disposition:** Documented in this SUMMARY's "Frieren E2E Gate Evidence" section. The post-merge build gate is expected to re-run the live curl-pipeline.

**2. Pre-existing test failure logged out-of-scope**

- **Found during:** Task 3 verification (`go test ./services/scraper/... -race -count=1`).
- **Test:** `TestOrchestrator_AnimePaheToGogoanimeFailover` in `services/scraper/internal/service/orchestrator_phase18_test.go:307` — "orch.ListEpisodes returned 0 episodes".
- **Verification of pre-existing status:** `git stash && go test ./services/scraper/internal/service/...` on base commit `dc7f89f` reproduces the failure identically. Plan 28-02 does NOT touch `services/scraper/internal/service/`.
- **Disposition:** Logged to `.planning/phases/28-provider-expansion-r2/deferred-items.md` per executor SCOPE BOUNDARY rules. Out-of-scope for this plan.

---

**Total deviations:** 0 auto-fixed; 2 documented deferrals (1 plan-mandated, 1 pre-existing).
**Impact on plan:** None — both deferrals align with executor guidance and SCOPE BOUNDARY rules. Provider implementation is functionally complete and unit-tested.

## Issues Encountered

- `services/scraper/internal/service` has a pre-existing test failure (`TestOrchestrator_AnimePaheToGogoanimeFailover`) that reproduces on the worktree base commit. Logged to `deferred-items.md` — not caused by Plan 28-02, not in this plan's scope to fix.

## User Setup Required

None — no new external service configuration required. The provider is always-on by default; operator may set `SCRAPER_DEGRADED_PROVIDERS=animefever` as a kill-switch if upstream breaks.

`SCRAPER_ANIMEFEVER_BASE_URL` env var is optional; defaults to `https://animefever.cc` (the canonical upstream confirmed via 2026-05-20 live recon).

## Next Phase Readiness

- Plan 28-03 (vidstream_vip extractor) is the immediate downstream dependency. Once 28-03 ships, `p.embeds.Find("https://am.vidstream.vip/...")` returns a working extractor and GetStream resolves end-to-end.
- After both 28-02 and 28-03 merge, the orchestrator's daily canary (Phase 23) and `/scraper/health` will exercise the AnimeFever pipeline live. Plan 28-06 (source dropdown polish) can then surface AnimeFever as a user-selectable source.
- Failover chain length grows from 3 working providers (allanime + degraded gogoanime/animepahe) to 4. Phase 19 wiring invariant now expects 4 (or 5 w/ animekai) registered providers — encoded in main.go.

## Self-Check: PASSED

**Files created:**
- `services/scraper/internal/providers/animefever/doc.go` — FOUND
- `services/scraper/internal/providers/animefever/dto.go` — FOUND
- `services/scraper/internal/providers/animefever/cache.go` — FOUND
- `services/scraper/internal/providers/animefever/client.go` — FOUND
- `services/scraper/internal/providers/animefever/client_test.go` — FOUND
- `services/scraper/internal/providers/animefever/testdata/search_frieren.html` — FOUND
- `services/scraper/internal/providers/animefever/testdata/info_frieren.html` — FOUND
- `services/scraper/internal/providers/animefever/testdata/watch_ep28.html` — FOUND
- `services/scraper/internal/providers/animefever/testdata/ajax_load_ep28.json` — FOUND

**Commits:**
- `d30d39c` (Task 1: scaffold) — FOUND
- `7da5fa6` (Task 2: client + tests) — FOUND
- `4668ee6` (Task 3: main.go + allowlist) — FOUND

**Build + test gates:**
- `go build ./services/scraper/cmd/scraper-api/...` — PASS
- `go test ./services/scraper/internal/providers/animefever/... -race -count=2` — PASS (12/12 tests, ×2 = 24/24 runs)
- `go test ./services/scraper/internal/config/... -race -count=2 -run AnimeFever` — PASS
- `go vet ./services/scraper/... ./libs/videoutils/...` — PASS
- `grep -E '"animefever"' main.go` — 6 matches (≥3 required: import + Deps + candidateProviders + 3 occurrences in registration block)
- `grep -E '"am\.vidstream\.vip"' proxy.go` — 1 match
- `grep -E "chromedp|utls|tls-client|flaresolverr" providers/animefever/` — 0 matches (no forbidden imports)

---
*Phase: 28-provider-expansion-r2*
*Plan: 02*
*Completed: 2026-05-20*
