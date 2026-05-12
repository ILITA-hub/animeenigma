---
phase: 18-9anime
plan: 02
subsystem: api
tags: [scraper, gogoanime, anitaku, provider, fuzzy-match, malsync, html-scraping, goquery, hls, ttl, redis]

# Dependency graph
requires:
  - phase: 18-9anime/01
    provides: "services/scraper/internal/fuzzy/ shared package + 8 anitaku.to goldens + RED-state test scaffolds + SCRAPER_GOGOANIME_BASE_URL config"
  - phase: 16-animepahe
    provides: "domain.Provider contract + animepahe analog (client.go / malsync.go / cache.go structural shape)"
  - phase: 17-observability
    provides: "internal/health/stage.go canonical 5-stage constants + libs/metrics.ParserZeroMatchTotal CounterVec"
  - phase: 15-orchestrator
    provides: "embeds.Registry + domain.WrapNotFound/WrapProviderDown/WrapExtractFailed three-family error taxonomy"
provides:
  - "gogoanime.Provider implementing domain.Provider (Name/FindID/ListEpisodes/ListServers/GetStream/HealthCheck)"
  - "Exported gogoanime.Deps struct + gogoanime.New(d Deps) (*Provider, error) constructor (field-for-field match with animepahe.Deps so Plan 18-04 main.go wires symmetrically)"
  - "Exported gogoanime.NewMalSyncClient(c cache.Cache, opts ...MalSyncOption) *MalSyncClient + gogoanime.MalSyncOption type — forward-compat probe with 24h positive + 24h negative cache"
  - "gogoanime.computeStreamTTL — &e=<delta_seconds> parser (paired with optional &s=<unix_signed_at> for absolute expiry) per RESEARCH.md Pitfall 3"
  - "Cloudflare-Turnstile skip-list (myvidplay.com / playmogo.com, suffix-matched) at ListServers"
  - "Selector constants selectorSearchResult / selectorEpisodeRow / selectorAnimeMutiLinkItem so parser_zero_match_total label cardinality stays bounded (SCRAPER-NF-04)"
  - "All Plan 18-01 RED-state gogoanime/ scaffolds turned GREEN (7 named tests + 9 supporting tests, 16 total PASS, 0 SKIP, 0 FAIL)"
affects:
  - "18-03 (3 embed extractors — vibeplayer / streamhg / earnvids — receive the embed URLs filtered + dedup'd by this provider's ListServers)"
  - "18-04 (orchestrator wiring — main.go constructs gogoanime.New(Deps{...}) and gogoanime.NewMalSyncClient(...) using the exported shapes finalized here)"

# Tech tracking
tech-stack:
  added:
    - "(none — all dependencies already in tree: goquery, libs/cache, libs/metrics, libs/logger, internal/fuzzy, internal/health, internal/domain)"
  patterns:
    - "WordPress/Madara-adjacent HTML scrape (goquery over .anime_muti_link + p.name + a[href*=-episode-])"
    - "Sub/dub merge by episode number with soft-404 (anitaku.to returns HTTP 200 with <title>Pages not found at Anitaku for missing dub categories)"
    - "Three-family error wrapping discipline (WrapProviderDown / WrapExtractFailed / WrapNotFound) for orchestrator failoverDecision"
    - "Named selector constants gating parser_zero_match_total label cardinality"

key-files:
  created:
    - "services/scraper/internal/providers/gogoanime/dto.go (56 LOC) — searchResult, episodeRow, serverRow + malSync JSON DTOs"
    - "services/scraper/internal/providers/gogoanime/cache.go (78 LOC) — computeStreamTTL with delta-or-absolute &e= semantics"
    - "services/scraper/internal/providers/gogoanime/malsync.go (221 LOC) — exported NewMalSyncClient + MalSyncOption + 24h positive/negative cache flow"
    - "services/scraper/internal/providers/gogoanime/client.go (~680 LOC) — Provider impl: FindID + ListEpisodes + ListServers + GetStream + HealthCheck"
    - "services/scraper/internal/providers/gogoanime/helpers_test.go (122 LOC) — shared in-memory fakeCache for tests"
  modified:
    - "services/scraper/internal/providers/gogoanime/doc.go — replaced placeholder with real package doc"
    - "services/scraper/internal/providers/gogoanime/dto_test.go — RED scaffolds turned GREEN (3 tests)"
    - "services/scraper/internal/providers/gogoanime/cache_test.go — RED scaffold turned GREEN (1 test, 7 subtests)"
    - "services/scraper/internal/providers/gogoanime/malsync_test.go — RED scaffold turned GREEN (1 test)"
    - "services/scraper/internal/providers/gogoanime/client_test.go — RED scaffolds turned GREEN + 5 additional supporting tests (11 tests, 1 with 4 subtests)"

key-decisions:
  - "ddosguard.go intentionally OMITTED — anitaku.to does not sit behind DDoS-Guard per RESEARCH.md §Mirror Viability. CONTEXT.md D2 lists ddosguard.go as an OPTIONAL file; the omission is documented in client.go's header comment."
  - "Soft-404 detection in ListEpisodes — anitaku.to serves HTTP 200 with the literal `<title>Pages not found at Anitaku` for missing dub categories (e.g. /category/one-piece-dub). The provider parses the title element and treats this as ErrNotFound internally, so a sub-only show still returns a clean sub episode list rather than failing the whole call."
  - "Sub/dub merge prefers the sub slug as canonical ID — when both sub and dub variants exist at the same episode number, the sub Episode.ID wins. Dub-only episodes use the -dub-* slug. The frontend's category-aware UI can still derive the audio category from the slug (containing -dub-) or by selecting a dub-tagged server at ListServers time."
  - "Cache keys are lowercase even though the malsync wire-key is TitleCase — `malsync:<mal_id>:gogoanime{,:miss}` per CONTEXT.md key-shape convention; the upstream slug `Gogoanime` is constant-encoded in malSyncProviderSlug."
  - "Selector zero-match emit uses literal `\"gogoanime\"` (not the providerName const) at the WithLabelValues call site per Plan 18-02 acceptance criterion. The trade-off: tiny duplication for greppability of the metric-emit sites."

patterns-established:
  - "EXPORTED Deps + New + NewMalSyncClient signatures match the animepahe analog field-for-field — main.go can wire both providers with identical literal patterns. Future EN providers should preserve this shape."
  - "Soft-404 detection — when an upstream returns HTTP 200 with a `not found` title element, treat as NotFound at the provider boundary so the orchestrator can fail over cleanly."
  - "Sub/dub fan-out via two GETs + map-merge by episode number — alternative to provider-side state (a single source returning both audio variants in one payload would be cleaner but the upstream forces this shape)."
  - "Selector identifier constants (selectorSearchResult / selectorEpisodeRow / selectorAnimeMutiLinkItem) bound the {selector=...} label cardinality on parser_zero_match_total — never inline raw CSS in WithLabelValues."

requirements-completed:
  - SCRAPER-9ANI-01  # FindID: fuzzy /search.html primary + malsync.moe "Gogoanime" forward-compat probe with 24h negative cache
  - SCRAPER-9ANI-02  # ListEpisodes: WordPress-adjacent /category/<slug> scrape + sub/dub merge + 6h cache at episodes:gogoanime:<base_slug>
  - SCRAPER-9ANI-06  # Failover receiver — gogoanime returns a real *domain.Stream so the orchestrator can route to it when AnimePahe is unhealthy (metric verification owned by Plan 18-04)

# Metrics
duration: ~10min
completed: 2026-05-12
---

# Phase 18 Plan 02: Gogoanime/Anitaku Provider Implementation Summary

**Gogoanime/Anitaku scraper provider implementing domain.Provider end-to-end against the Plan 18-01 anitaku.to goldens — fuzzy /search.html ID resolution (primary path), sub/dub category merge with 6h cache, Cloudflare-Turnstile skip-list at ListServers, and registry-dispatched GetStream with &e=<delta> + &s=<unix> stream TTL.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-05-12T15:55:00Z (approximate — first commit timestamp)
- **Completed:** 2026-05-12T16:07:34Z
- **Tasks:** 2 (both autonomous, both TDD-flagged)
- **Files created:** 5 source + 1 shared test helper
- **Files modified:** 4 test scaffolds (RED → GREEN)
- **Tests:** 16 PASS, 0 SKIP, 0 FAIL across the gogoanime package
- **Regression:** services/scraper/internal/providers/animepahe/... still PASS; full `go test -race ./services/scraper/...` clean

## Accomplishments

- **gogoanime.Provider** satisfies domain.Provider — verified by `var _ domain.Provider = (*Provider)(nil)` compile-time assertion.
- **EXPORTED** Deps / New / MalSyncClient / NewMalSyncClient / MalSyncOption with field-for-field parity to the animepahe analog, so Plan 18-04 main.go wires both providers with identical literal patterns.
- **Fuzzy `/search.html` as the primary FindID path** — malsync.moe is queried first (forward-compat probe with 24h negative cache) but is expected to miss for every MAL ID until malsync ships a Gogoanime key. The 0.85 Jaro-Winkler threshold matches animepahe's discipline for cross-provider comparability.
- **Sub/dub episode merge** — fetches `/category/<base>` plus `/category/<base>-dub`, soft-404-detects the dub page when missing, merges by episode number with sub winning canonical ID. 6h cache at `episodes:gogoanime:<base_slug>`.
- **Cloudflare-Turnstile skip-list** — `myvidplay.com` and `playmogo.com` (both suffix-matched) are filtered at ListServers time so they never reach GetStream. Dedup by URL.
- **&e=<delta_seconds> + optional &s=<unix_signed_at>** stream TTL semantics — paired absolute expiry interpretation when `s=` is present, fallback delta-from-now otherwise. Distinct from animepahe's absolute `expires=<unix>` shape.
- **Strict three-family error wrapping** — WrapProviderDown for transport failures, WrapExtractFailed for parse / decode / regex no-match failures, WrapNotFound for 404 + zero results + fuzzy < 0.85. Orchestrator failover relies on this discipline.
- **parser_zero_match_total** emits on every zero-match path (search, episode rows, anime_muti_link items) with NAMED selector constants — label cardinality bounded.

## Task Commits

1. **Task 1: dto.go + cache.go + malsync.go + RED-to-GREEN supporting-layer tests** — `9f7711a` (feat)
2. **Task 2: client.go end-to-end + client_test.go RED-to-GREEN** — `d781e93` (feat)

_TDD note: Plan 18-01 emitted the RED-state scaffolds. Both Task 1 and Task 2 in this plan are the GREEN step (RED already shipped in 18-01 commit `336abb1` `test(scraper/gogoanime): add RED-state test scaffolds...`)._

## Files Created/Modified

### Source files
- `services/scraper/internal/providers/gogoanime/dto.go` (56 LOC) — searchResult / episodeRow / serverRow HTML DTOs + malSync JSON DTOs.
- `services/scraper/internal/providers/gogoanime/cache.go` (78 LOC) — `computeStreamTTL` parsing `&e=<delta_seconds>` (paired with optional `&s=<unix_signed_at>`) per RESEARCH.md Pitfall 3.
- `services/scraper/internal/providers/gogoanime/malsync.go` (221 LOC) — exported `NewMalSyncClient(c cache.Cache, opts ...MalSyncOption) *MalSyncClient` + `MalSyncOption` type. Wire-key `"Gogoanime"` (TitleCase); Redis cache keys `malsync:<mal_id>:gogoanime{,:miss}`. 24h positive + 24h negative.
- `services/scraper/internal/providers/gogoanime/client.go` (~680 LOC) — Provider impl. Slug literal `"gogoanime"`. Default BaseURL `https://anitaku.to`. Stream cache key `stream:gogoanime:<slug>:<epID>:<sha256(serverID)[:8]>`.
- `services/scraper/internal/providers/gogoanime/doc.go` (34 LOC) — package documentation (replaces Plan 18-01 placeholder).

### Test files
- `services/scraper/internal/providers/gogoanime/helpers_test.go` (122 LOC) — shared in-memory fakeCache. Mirror of animepahe's pattern.
- `services/scraper/internal/providers/gogoanime/dto_test.go` (174 LOC) — `TestSearchResult_GoldenParse` / `TestEpisodeRow_GoldenParse` / `TestServerRow_GoldenParse` against the captured goldens.
- `services/scraper/internal/providers/gogoanime/cache_test.go` (94 LOC) — `TestComputeStreamTTL_StreamHGSignedURL` with 7 subtests (absolute / delta / fallback / expired / large / non-integer / zero).
- `services/scraper/internal/providers/gogoanime/malsync_test.go` (74 LOC) — `TestMalSync_NegativeCacheForGogoanime` asserting both the first-call HTTP fire + second-call cache short-circuit.
- `services/scraper/internal/providers/gogoanime/client_test.go` (537 LOC) — 11 tests (1 with 4 subtests): Name, FindID_FuzzyPath, FindID_MalsyncNegativeCache, FindID_NoMatch, ListEpisodes_SubDubMerge, ListEpisodes_CacheHit, ListServers_AnimeMutiLink, ListServers_DoodstreamSkipped, GetStream_DispatchesToRegistry, GetStream_StreamTTL, New_RequiresDependencies.

## Cache Key Shapes Used

| Key shape | TTL | Purpose |
|---|---|---|
| `malsync:<mal_id>:gogoanime` | 24h | Positive MAL→slug mapping (forward-compat, expected to remain empty as of 2026-05-12) |
| `malsync:<mal_id>:gogoanime:miss` | 24h | Negative cache for missing-key responses (the steady state today) |
| `episodes:gogoanime:<base_slug>` | 6h | Merged sub+dub episode list |
| `stream:gogoanime:<slug>:<epID>:<sha256(serverID)[:8]>` | min(parsed expiry − 30s, 5min) | Extracted stream URL — never cached when TTL ≤ 0 |

## Selector Constants

| Constant | Value | Emission site (`metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", …)`) |
|---|---|---|
| `selectorSearchResult` | `"search_result"` | FindID — zero `p.name a[href^='/category/']` matches |
| `selectorEpisodeRow` | `"episode_row"` | ListEpisodes — zero `a[href*="-episode-"]` matches in `fetchEpisodes` |
| `selectorAnimeMutiLinkItem` | `"anime_muti_link_item"` | ListServers — zero `.anime_muti_link a[data-video]` matches |

## Doodstream / Cloudflare-Turnstile Skip-List

| Host | Reason | Match shape |
|---|---|---|
| `myvidplay.com` (and `*.myvidplay.com`) | Cloudflare Turnstile interactive challenge required to obtain the m3u8 — un-automatable per RESEARCH.md Pitfall 9 | suffix match (case-insensitive) |
| `playmogo.com` (and `*.playmogo.com`) | Same Turnstile gate (sister property of Doodstream) | suffix match (case-insensitive) |

The captured `one_piece_episode_1.html` golden contains 3 `myvidplay.com` entries; `TestListServers_DoodstreamSkipped` asserts none leak through to the server list (and fails loudly if a future golden refresh removes the Doodstream lines, so the test isn't silently rubber-stamping).

## Test Inventory

| Test | Requirement | Notes |
|---|---|---|
| `TestProvider_Name` | identity | Pins `"gogoanime"` literal |
| `TestSearchResult_GoldenParse` | SCRAPER-9ANI-01 | ≥ 5 search rows from `search_attack_on_titan.html` |
| `TestEpisodeRow_GoldenParse` | SCRAPER-9ANI-02 | ≥ 100 episode rows from `category_one_piece.html` |
| `TestServerRow_GoldenParse` | SCRAPER-9ANI-03 | ≥ 4 server rows from `one_piece_episode_1.html`, all with non-empty hosts |
| `TestComputeStreamTTL_StreamHGSignedURL` (7 subtests) | SCRAPER-9ANI-04 | Absolute / delta / fallback / expired / large / non-integer / zero |
| `TestMalSync_NegativeCacheForGogoanime` | SCRAPER-9ANI-01 | 2nd Lookup short-circuits HTTP via negative cache |
| `TestFindID_FuzzyPath` | SCRAPER-9ANI-01 | "Attack on Titan" → `attack-on-titan` slug |
| `TestFindID_MalsyncNegativeCache` | SCRAPER-9ANI-01 | Forward-compat probe still fires once per FindID |
| `TestFindID_NoMatch` | SCRAPER-9ANI-01 | Zero-row response → `ErrNotFound` |
| `TestListEpisodes_SubDubMerge` | SCRAPER-9ANI-02 | Sub list + dub soft-404 → ≥ 100 merged episodes, ep 1 uses sub slug |
| `TestListEpisodes_CacheHit` | SCRAPER-9ANI-02 | 2nd call short-circuits HTTP |
| `TestListServers_AnimeMutiLink` | SCRAPER-9ANI-03 | vibeplayer + otakuhg + otakuvid all present, all sub-tagged |
| `TestListServers_DoodstreamSkipped` | SCRAPER-9ANI-03 | No `myvidplay.com` or `playmogo.com` leaks |
| `TestGetStream_DispatchesToRegistry` | SCRAPER-9ANI-04 / SCRAPER-9ANI-06 | Provider has zero extraction logic; defers entirely to registry |
| `TestGetStream_StreamTTL` | SCRAPER-9ANI-04 | Stream cached under `stream:gogoanime:*` namespace |
| `TestNew_RequiresDependencies` (4 subtests) | WR-11 | Each missing required dep returns a tagged error |

## Decisions Made

See `key-decisions` in frontmatter. Notable additions during implementation:

- **TestListEpisodes_SubDubMerge** does NOT assert a CategoryDub episode is present because the captured `category_one_piece_dub.html` golden is the soft-404 page, not a real dub listing. The merge code paths are still exercised end-to-end; the soft-404 detector is the test's primary anchor. A future golden refresh that captures a real dub listing should add an explicit CategoryDub assertion.
- **fakeStreamExtractor.Matches always returns true** in tests because the registry's first-match semantics are simple and the provider doesn't depend on host-discrimination (it just hands off whatever URL ListServers gave it). The real Plan 18-03 extractors will Matches-discriminate by host.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Adjusted acceptance criterion compliance for two literal-grep checks**

- **Found during:** Task 2 verification
- **Issue:** Initial implementation used `return providerName` for `Name()` (DRY against the existing `providerName` const) and `metrics.ParserZeroMatchTotal.WithLabelValues(providerName, …)` for zero-match emits, which compile and behave identically to the literal-string forms but fail the acceptance criteria's literal greps (`grep -q 'return "gogoanime"'` and `grep -cE 'metrics\.ParserZeroMatchTotal\.WithLabelValues\("gogoanime", selector'`).
- **Fix:** Inlined the `"gogoanime"` literal at both call sites (`Name()` and 3 `ParserZeroMatchTotal` emits) per the acceptance criterion's explicit greppability requirement. Added a comment on `Name()` documenting the duplication-with-const rationale.
- **Files modified:** `services/scraper/internal/providers/gogoanime/client.go`
- **Verification:** All 16 tests still PASS; acceptance grep now exits 0.
- **Committed in:** `d781e93` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 — blocking acceptance criterion mismatch)
**Impact on plan:** Cosmetic deviation only — behavior identical, just satisfies the literal-grep acceptance checks. No scope creep.

## Issues Encountered

- **Soft-404 dub page** — anitaku.to serves HTTP 200 with `<title>Pages not found at Anitaku` for missing dub categories rather than a 404 status, which initially made `category_one_piece_dub.html` look like a real dub listing. Added a title-element check in `fetchEpisodes` to detect this and return ErrNotFound internally so `ListEpisodes` proceeds with sub-only. Documented in `key-decisions`.

## Self-Check: PASSED

**Source files exist:**
- `services/scraper/internal/providers/gogoanime/doc.go` — FOUND (34 LOC, real doc)
- `services/scraper/internal/providers/gogoanime/dto.go` — FOUND (56 LOC)
- `services/scraper/internal/providers/gogoanime/cache.go` — FOUND (78 LOC)
- `services/scraper/internal/providers/gogoanime/malsync.go` — FOUND (221 LOC)
- `services/scraper/internal/providers/gogoanime/client.go` — FOUND (680 LOC, > 250 LOC min)
- `services/scraper/internal/providers/gogoanime/helpers_test.go` — FOUND (122 LOC)

**Commits exist:**
- `9f7711a` (Task 1) — FOUND in `git log`
- `d781e93` (Task 2) — FOUND in `git log`

**Acceptance criteria:**
- `var _ domain.Provider = (*Provider)(nil)` — present (compile-time assertion)
- `func New(d Deps) (*Provider, error)` — present
- `type Deps struct` with fields BaseURL/HTTP/Embeds/MalSync/Cache/Log — all 6 fields present
- `func NewMalSyncClient(c cache.Cache, opts ...MalSyncOption) *MalSyncClient` — EXPORTED
- `type MalSyncOption func(*MalSyncClient)` — EXPORTED
- `"Gogoanime"` literal in malsync.go — present (TitleCase wire-key constant)
- `return "gogoanime"` in client.go Name() — present
- `fuzzy.JaroWinkler` / `fuzzy.NormalizeTitle` consumed — present (2 references)
- `health.Stage(Search|Episodes|Servers|Stream)` references — 28 (well above minimum 4)
- `myvidplay.com` + `playmogo.com` filter wired — present
- `io.LimitReader` — present (3 sites: search 4 MiB, /category 2 MiB, episode page 2 MiB)
- `metrics.ParserZeroMatchTotal.WithLabelValues("gogoanime", selector*)` — 3 named-constant emit sites
- 4 `Deps.<field> is required` guards — present
- 0 `t.Skip` lines remaining in test files — verified
- 16 PASS, 0 SKIP, 0 FAIL — verified

**Regression invariants:**
- `go build ./services/scraper/...` — exits 0
- `go vet ./services/scraper/...` — clean
- `go test ./services/scraper/... -count=1 -race -timeout=180s` — all packages PASS, including animepahe (no Wave 1 fuzzy refactor regression)

## Next Phase Readiness

- **Plan 18-03** can begin: the 3 EmbedExtractor scaffolds (`vibeplayer_test.go`, `streamhg_test.go`, `earnvids_test.go`) from Plan 18-01 are RED and will be turned GREEN against `vibeplayer_embed.html`, `streamhg_packed.html`, `earnvids_packed.html` goldens. The gogoanime provider's `GetStream` already routes through `p.embeds.Find(serverID)` and the test rig uses a `fakeStreamExtractor` — drop-in replacement once Plan 18-03 ships real extractors.
- **Plan 18-04** can wire `main.go`: the exported `gogoanime.Deps`, `gogoanime.New`, `gogoanime.NewMalSyncClient`, and `gogoanime.MalSyncOption` shapes are stable and identical to `animepahe.*`. The orchestrator can register the provider alongside animepahe; failover discipline (three-family error taxonomy + per-stage health) is already in place.
- **No blockers** for downstream waves. `ddosguard.go` omission is documented and intentional.

---

*Phase: 18-9anime*
*Plan: 02*
*Completed: 2026-05-12*
