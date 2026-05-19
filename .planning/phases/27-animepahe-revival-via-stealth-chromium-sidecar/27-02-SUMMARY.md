---
phase: 27-animepahe-revival-via-stealth-chromium-sidecar
plan: 02
subsystem: scraper
tags: [animepahe, kwik, malsync, ddosguard-removed, resolver-sidecar, go, httpclient]

# Dependency graph
requires:
  - phase: 27-animepahe-revival-via-stealth-chromium-sidecar/01
    provides: stealth-Chromium sidecar Dockerfile + /search /release /play HTTP contract (soft dep — goldens captured against documented shapes; live re-capture deferred to 27-03/27-04)
provides:
  - "resolverClient HTTP transport replacing direct upstream animepahe.* calls"
  - "kwikReferer = 'https://animepahe.pw/' package-level constant for the Kwik extractor Referer chain (D2 alignment)"
  - "Persistent MalSync reverse-mapping cache key (malsync_reverse:animepahe:<animeSession>) enabling A9 single-strike /release-404 invalidation across process restarts"
  - "Three Frieren goldens shaped per the documented animepahe.pw API contract (search.json, release.json, play.html)"
  - "SCRAPER_ANIMEPAHE_RESOLVER_URL env binding (default http://animepahe-resolver:3000); ANIMEPAHE_BASE_URL removed"
affects:
  - phase: 27-animepahe-revival-via-stealth-chromium-sidecar/03
    needs: docker-compose wiring for the new SCRAPER_ANIMEPAHE_RESOLVER_URL env var; will re-capture Frieren goldens against the live sidecar
  - phase: 27-animepahe-revival-via-stealth-chromium-sidecar/04
    needs: live end-to-end Frieren curl pipeline through the sidecar; kwikReferer constant value (https://animepahe.pw/) is the canonical Referer the stream-fetchability gate uses
  - phase: 27-animepahe-revival-via-stealth-chromium-sidecar/05
    needs: removal of `animepahe` from SCRAPER_DEGRADED_PROVIDERS once the gate clears

# Tech tracking
tech-stack:
  added:
    - "resolverClient (in-process Go HTTP client to the animepahe-resolver sidecar)"
    - "Persistent reverse-mapping pattern (malsync_reverse:<provider>:<providerID> key) backing single-strike cache invalidation"
  patterns:
    - "Sidecar-as-sole-transport: provider code no longer makes upstream calls; one stealth-Chromium service owns the entire challenge surface"
    - "Variadic cache.Delete for atomic dual-key eviction (forward + reverse MalSync entries)"
    - "Compile-time interface extension to drive a new behavior across both production and test paths (malSyncClient gained LookupMalID + Invalidate)"

key-files:
  created:
    - "services/scraper/internal/providers/animepahe/resolver.go (resolverClient + kwikReferer constant)"
    - "services/scraper/internal/providers/animepahe/resolver_test.go (TestResolverClient_ErrorMapping + helpers)"
    - "services/scraper/internal/providers/animepahe/malsync_invalidation_test.go (TestProvider_MalSyncInvalidationOn404 incl. cross-process-restart case)"
    - "services/scraper/testdata/animepahe/frieren-search.json"
    - "services/scraper/testdata/animepahe/frieren-release.json"
    - "services/scraper/testdata/animepahe/frieren-play.html"
    - ".planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/deferred-items.md"
  modified:
    - "services/scraper/internal/providers/animepahe/client.go (Provider.baseURL field deleted; Deps.BaseURL → Deps.ResolverURL; FindID/ListEpisodes/ListServers rewired through p.resolver; GetStream Referer source switched to kwikReferer; A9 invalidation call added)"
    - "services/scraper/internal/providers/animepahe/malsync.go (LookupMalID + Invalidate methods; reverse-key write on positive-cache hit)"
    - "services/scraper/internal/providers/animepahe/dto.go (doc comment noting A1/A2 shape verification)"
    - "services/scraper/internal/providers/animepahe/cache.go (doc comment noting A5 animeSession-keyed cache)"
    - "services/scraper/internal/providers/animepahe/client_test.go (handler paths rewired to /search /release /play; fakeMalSync extended; fakeKwikExtractor captures Referer)"
    - "services/scraper/internal/providers/animepahe/dto_test.go (TestDTO_Frieren added)"
    - "services/scraper/internal/config/config.go (AnimePaheConfig.BaseURL → ResolverURL; env binding ANIMEPAHE_BASE_URL → SCRAPER_ANIMEPAHE_RESOLVER_URL)"
    - "services/scraper/internal/config/config_test.go (existing tests updated; TestConfig_AnimepaheResolverURL added)"
    - "services/scraper/cmd/scraper-api/main.go (Deps.ResolverURL field; rate-limit list updated; boot log key renamed)"
  deleted:
    - "services/scraper/internal/providers/animepahe/ddosguard.go (load-bearing-zero now that the sidecar owns the challenge stack)"
    - "services/scraper/internal/providers/animepahe/ddosguard_test.go"

key-decisions:
  - "Persistent reverse-mapping cache key (not in-memory map): chosen so /release 404 invalidation survives process restarts AND works across scraper processes. The plan considered both options; the persistent-cache approach is strictly more robust and the per-request cost is one extra cache.Set (paired with the existing positive-cache write)."
  - "Interface extension of malSyncClient with LookupMalID + Invalidate (not type-assertion at call site): keeps the parser's call path linear and exercised by test fakes uniformly; avoids the inline-assertion code smell."
  - "Frieren goldens captured against documented animepahe.pw API shapes (CONTEXT.md §Specific Ideas), NOT against the live sidecar: Plan 27-01's sidecar Docker image is being built in a sibling worktree and not yet buildable on this branch. Per the orchestrator's cross-plan note, these are placeholder goldens shaped per the 2026-05-19 stealth-puppeteer probe payloads — they MUST be re-captured against the deployed sidecar during Plan 27-03/27-04 once the image lands."

patterns-established:
  - "Sidecar-as-sole-transport: the parser package no longer references upstream domain names. Every outbound call goes through resolverClient, which talks to ONE configurable host (SCRAPER_ANIMEPAHE_RESOLVER_URL). This is the template Plan 27-03/04 will replicate for any other stealth-required upstream."
  - "Reverse-mapping persistent cache for single-strike invalidation: write both directions on positive-cache hit (forward + reverse, same TTL), delete both directions on negative event. The pattern survives process restarts and parallel-scraper scenarios."
  - "Domain error mapping by HTTP status (200/404/502/other → ErrNotFound/ErrProviderDown/wrap): centralizes the contract in one mapStatus helper so adding a new contract row is one line in resolver.go AND one line in TestResolverClient_ErrorMapping."

requirements-completed:
  - SCRAPER-HEAL-30

# Metrics
duration: ~25min
completed: 2026-05-19
---

# Phase 27 Plan 02: Parser Rewrite + UUID-Session Contract + Sidecar Transport Summary

**The Go parser at `services/scraper/internal/providers/animepahe/` now routes its entire upstream-fetch transport through a new `resolverClient` HTTP client to the `animepahe-resolver` stealth-Chromium sidecar, completing the API-contract migration to UUID `session` tokens; `ddosguard.go` + `ddosguard_test.go` are deleted (sidecar owns the challenge stack), MalSync gained single-strike `/release`-404 invalidation backed by a persistent reverse-mapping cache key, and `ANIMEPAHE_BASE_URL` is replaced by `SCRAPER_ANIMEPAHE_RESOLVER_URL`.**

## Performance

- **Duration:** ~25 min
- **Tasks:** 3 / 3
- **Files created:** 7 (incl. 3 goldens, 2 new tests, 1 resolver.go, 1 deferred-items.md)
- **Files modified:** 9
- **Files deleted:** 2 (ddosguard.go + ddosguard_test.go)

## Accomplishments

- Sidecar-only transport in place: the only string references to animepahe in the parser are the `kwikReferer` constant value (`https://animepahe.pw/`) used as the Referer header to Kwik, and the `providerName` literal `"animepahe"` used as the orchestrator registry key. Every outbound HTTP call goes through `p.resolver.Search` / `Release` / `Play`.
- `GetStream` semantics-preserved: cache hit/miss path, `embeds.Registry.Find().Extract()` call, and `min(expires-30s, 5min)` TTL decision all byte-for-byte from the pre-Phase-27 implementation. Only the Referer source changed (deleted `p.baseURL` → `kwikReferer` constant). A new test assertion in `TestProvider_GetStream_HappyPath` pins this contract.
- A9 single-strike MalSync invalidation lives entirely in the persistent cache: ANY scraper process can invalidate a stale `(malID → animeSession)` mapping after a `/release` 404, including a freshly-restarted process with no in-memory state. The load-bearing test `TestProvider_MalSyncInvalidationOn404/WithoutPriorFindID_PersistedReverseKey` proves it by seeding both cache directions via direct `cache.Set` (no prior FindID call) and asserting both keys evict.
- Three Frieren goldens land at `services/scraper/testdata/animepahe/frieren-{search,release,play}.{json,html}` — shaped per the documented animepahe.pw API contract from 27-CONTEXT.md §Specific Ideas. `TestDTO_Frieren` exercises all three goldens against the existing DTO definitions to prove zero struct-field changes were needed when the transport migrated.

## Task Commits

1. **Task 1: Add resolverClient + kwikReferer + delete ddosguard + rewire transport** — `b7b634c` (feat)
2. **Task 2: Rewire tests + add resolver_test.go + MalSync /release-404 invalidation test** — `fcf5525` (test)
3. **Task 3: Config + main.go switch + Frieren goldens** — `5578c5f` (feat)

## Files Created/Modified

See key-files section in frontmatter.

## Decisions Made

See key-decisions section in frontmatter. Most consequential: the choice of persistent reverse-mapping cache key (vs in-memory map) was driven by the "works across process restarts AND across scraper processes" requirement in the plan's must-haves — the in-memory variant would have failed that test silently.

## Deviations from Plan

### Task ordering shifted minimally to maintain build invariant

The plan separates `client.go` rewrite (Task 1) from `malsync.go` extension (Task 2), but the Task 1 client.go references `p.malsync.LookupMalID` and `p.malsync.Invalidate` (the new methods). To keep the `go build ./...` gate green in Task 1's commit, the `malSyncClient` interface extension + the production `*MalSyncClient` method implementations were folded into Task 1's commit. The actual test files (`malsync_invalidation_test.go`, `resolver_test.go`) and the `client_test.go` handler-path rewires landed in Task 2 as planned. No semantic deviation — only the commit boundary moved.

Per Rule 3 (Auto-fix blocking issues): the alternative was a non-building intermediate commit, which the worktree's git pre-commit hook would have rejected and which violates the plan's own verify gate (`go build ./...`).

### Frieren goldens are placeholders, not live captures

The plan's Task 3 instructs running `docker build -t animepahe-resolver:dev -f services/animepahe-resolver/Dockerfile .` to start the sidecar and capture goldens via `curl http://localhost:3000/search?q=Frieren` etc. Plan 27-01's Dockerfile is being built in a sibling worktree and not present on this branch (confirmed: `services/animepahe-resolver/` directory does not exist on `worktree-agent-ade290a28e9a979cc`).

Per the orchestrator's cross-plan note in the task prompt, this case was anticipated. The placeholder goldens are shaped per the 2026-05-19 stealth-puppeteer probe payloads documented in 27-CONTEXT.md §Specific Ideas (search response: `data[].session` as UUID; release response: `data[].session` as 64-char hex; play HTML: ≥ 1 `button[data-src=...kwik...]`). They satisfy the existing DTO assertions and unblock unit-test development; they will be re-captured against the live sidecar during Plan 27-03 / 27-04 once the image is buildable.

### Pre-existing failure: `TestOrchestrator_AnimePaheToGogoanimeFailover`

Logged to `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/deferred-items.md` as DEF-001. The test fails identically on the worktree base commit (`e33be35`) before any Plan 27-02 changes. It exercises the gogoanime provider's ListEpisodes path (the fake animepahe is intentionally cache-skipped). Out of scope per the deviation rules' "Only auto-fix issues DIRECTLY caused by the current task's changes" boundary.

## Threat Flags

No new security-relevant surface introduced. The plan's threat model (T-27-02-01..03) is mitigated as documented:

- **T-27-02-01** (non-http(s) Kwik URL scheme): scheme check preserved in `client.go::ListServers` (`pu.Scheme != "http" && pu.Scheme != "https"`). The existing `TestProvider_ListServers_SchemeReject` test would have caught a regression — the test handler-rewiring kept the assertion intact.
- **T-27-02-02** (attacker-controlled `SCRAPER_ANIMEPAHE_RESOLVER_URL`): boot-time URL validation in `config.go::Load` (mirrors `MEGACLOUD_EXTRACTOR_URL` pattern); operator-set in compose, not user-influenceable.
- **T-27-02-03** (unbounded resolver response body): `maxBodyAPI` (4 MiB) and `maxBodyHTML` (2 MiB) `io.LimitReader` caps preserved in `resolver.go` (mirrors pre-Phase-27 `client.go` caps).

## Known Stubs

- **`services/scraper/testdata/animepahe/frieren-{search,release,play}.{json,html}`** — placeholder content shaped per documented API contract (see "Frieren goldens are placeholders" deviation above). Plan 27-03/27-04 will re-capture against the live sidecar.

## Self-Check

- [x] `services/scraper/internal/providers/animepahe/resolver.go` exists
- [x] `services/scraper/internal/providers/animepahe/resolver_test.go` exists
- [x] `services/scraper/internal/providers/animepahe/malsync_invalidation_test.go` exists
- [x] `services/scraper/testdata/animepahe/frieren-search.json` exists
- [x] `services/scraper/testdata/animepahe/frieren-release.json` exists
- [x] `services/scraper/testdata/animepahe/frieren-play.html` exists
- [x] `services/scraper/internal/providers/animepahe/ddosguard.go` removed
- [x] `services/scraper/internal/providers/animepahe/ddosguard_test.go` removed
- [x] Commit `b7b634c` present (Task 1)
- [x] Commit `fcf5525` present (Task 2)
- [x] Commit `5578c5f` present (Task 3)
- [x] `go build ./...` passes from `services/scraper/`
- [x] Plan exit-criteria block passes (all 11 invariants checked at end of execution)
- [x] `go test -count=1 ./internal/providers/animepahe/... ./internal/config/...` passes

## Self-Check: PASSED
