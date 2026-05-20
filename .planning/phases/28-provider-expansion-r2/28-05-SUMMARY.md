---
phase: 28-provider-expansion-r2
plan: 05
subsystem: scraper
tags: [nineanime, provider-lift, scraper-heal-39, mp4-extraction, wp-rest-api, brand-jack]

# Dependency graph
requires:
  - phase: 26-en-revival
    provides: domain.Provider interface + 6-method provider template (allanime/client.go)
  - phase: 28-provider-expansion-r2
    plan: 02
    provides: AnimeFever pattern for WP REST + HTML scrape (used as Wave 1 reference)
  - phase: 16-en-streaming-resilience
    provides: Frontend MP4-via-Kwik precedent that frees nineanime from HLS-only assumption
provides:
  - "nineanime.Provider — failover slot 6 (LAST per CONTEXT.md D5)"
  - "WP REST API title-fuzzy FindID with year + season-number tie-breakers (Discretion #2)"
  - "MP4-direct GetStream returning Stream.Sources[0].Type='mp4' (Pitfall 6)"
  - "Negative cache on WP search misses (mirrors animepahe/malsync pattern, 24h TTL)"
  - "1 HLS proxy allowlist entry (my.1anime.site) via the structured AllowedDomain format"
  - "14-test client_test.go suite using captured live-recon testdata fixtures"
  - "Episode.ID stores the canonical episode URL per Pitfall 5 (irregular `hd-` prefix slugs)"
affects:
  - "Scraper orchestrator failover chain — nineanime is slot 6 (last-resort)"
  - "v3.1 ship gate — SCRAPER-HEAL-39 resolved"
  - "Operator runbook — SCRAPER_DEGRADED_PROVIDERS=nineanime kill-switch documented"

# Tech tracking
tech-stack:
  added: []  # No new external Go modules — pure HTML/JSON scrape using existing stack
  patterns:
    - "Negative cache + isNegative return — getShowID(ctx, key) returns (slug, isNegative, ok)"
    - "Inline MP4 extraction — Provider does NOT route through embed registry (WR-04 dead-Embeds-field guard)"
    - "Pitfall 5 storage convention — Episode.ID = full canonical URL (never reconstruct from slug+number)"
    - "Compile-time `var _ domain.Provider = (*Provider)(nil)` interface assertion (test file + client file)"

key-files:
  created:
    - services/scraper/internal/providers/nineanime/doc.go
    - services/scraper/internal/providers/nineanime/dto.go
    - services/scraper/internal/providers/nineanime/cache.go
    - services/scraper/internal/providers/nineanime/client.go
    - services/scraper/internal/providers/nineanime/client_test.go
    - services/scraper/internal/providers/nineanime/testdata/wp_search_frieren.json
    - services/scraper/internal/providers/nineanime/testdata/series_frieren_s2.html
    - services/scraper/internal/providers/nineanime/testdata/episode_1.html
    - services/scraper/internal/providers/nineanime/testdata/embed_1anime_site.html
    - .planning/phases/28-provider-expansion-r2/28-05-SUMMARY.md
  modified:
    - services/scraper/internal/config/config.go
    - services/scraper/internal/config/config_test.go
    - services/scraper/cmd/scraper-api/main.go
    - libs/videoutils/proxy.go

key-decisions:
  - "Inline MP4 extraction (NO embed registry) — nineanime returns Stream directly from a 2-step regex walk (iframe → <source>). Per WR-04, the Embeds field from the allanime template is INTENTIONALLY OMITTED from Deps."
  - "Episode.ID stores the full canonical URL (Pitfall 5) — some episodes have `hd-` prefix and some do not. Reconstructing by string concat breaks ~1/28 episodes on Frieren S2."
  - "Negative cache for WP search misses (24h TTL) — 9anime's catalog has known gaps (e.g. Frieren S1 absent). Without negative caching, every miss re-hits the WP REST endpoint."
  - "Year + season-number tie-breakers (NOT episode-count) — AnimeRef lacks an EpisodeCount field. Use the year-substring-in-title heuristic + the season-N normalize-then-compare for tie-breaking inside the JW≥0.85 band."
  - "Failover slot 6 (LAST) — registered AFTER miruro per D5; reflects CONTEXT.md D2's explicit acceptance of 9anime as low-quality last-resort."
  - "Per-host RPS 1.0 burst 2 for both 9anime.me.uk and my.1anime.site — conservative pacing matches the upstream's Cloudflare/Engintron config."

patterns-established:
  - "Pattern: brand-jack provider lift — pure HTML/JSON scrape, no JS challenges, no anti-bot. Same template as AnimeFever."
  - "Pattern: WP REST API title-fuzzy — `/wp-json/wp/v2/search?search=<term>&per_page=20` filtered to `subtype:\"series\"` (Pitfall 4 — default ?s= is broken)"
  - "Pattern: MP4-direct provider — frontend handles MP4 via Phase 16 AnimePahe-via-Kwik precedent; no frontend changes required."

requirements-completed: [SCRAPER-HEAL-39]

# Metrics
duration: ~13min wall (Wave 3 parallel worktree)
completed: 2026-05-20
---

# Phase 28 Plan 05: 9anime.me.uk Provider Lift Summary

**One-liner:** 9anime.me.uk (failover slot 6, last-resort) shipped as a stdlib-only provider that scrapes the brand-jack WP install via the WP REST API + WP-post HTML walks, returns MP4-direct streams (frontend supports via AnimePahe-via-Kwik precedent), with a Discretion-#2 fuzzy match (year + season tie-breakers) and 24h negative cache on WP search misses.

## Performance

- **Duration:** ~13 min wall (Wave 3 parallel worktree)
- **Started:** 2026-05-20T02:38:11Z
- **Completed:** 2026-05-20T02:50:37Z
- **Tasks:** 4 of 5 (Task 5 is the post-merge human-verify checkpoint — deferred per Wave 3 / 28-04 precedent)
- **Files modified/created:** 13 (5 new package files + 4 testdata fixtures + 4 modified)

## Accomplishments

- Full `domain.Provider` implementation for 9anime.me.uk (6 methods + HealthCheck) following the allanime template but with the dead `Embeds` field deliberately dropped (WR-04 guard).
- 4 captured live-recon testdata fixtures (Frieren S2 — the only Frieren-family entry present on the brand-jack upstream).
- WP REST API title-fuzzy with year + season-number tie-breakers (CONTEXT.md Discretion #2 — implemented without an `EpisodeCount` field on `domain.AnimeRef`).
- Negative cache on WP search misses (24h TTL) — mirrors `animepahe/malsync.go` pattern; saves an HTTP fetch on every repeated-miss FindID call.
- 14 table-driven unit tests in `client_test.go`, all passing with `-race -count=2`.
- Registered as failover slot 6 in `main.go` (AFTER miruro per CONTEXT.md D5).
- `my.1anime.site` added to the HLS proxy allowlist in the new structured `AllowedDomain{Domain,Reason,Owner,Added}` format.

## Task Commits

Each task was committed atomically (per-task TDD where applicable):

1. **Task 1: Capture 4 testdata fixtures** — `6e41c7b` (test) — live-recon JSON+HTML for Frieren S2 (wp_search, series page, episode 1, embed page)
2. **Task 2: Scaffold package + config** — `1aeb3d1` (feat) — doc.go, dto.go, cache.go (with negative-cache support), NineAnimeConfig + 3 config tests. TDD RED→GREEN on config tests verified.
3. **Task 3: Implement client.go + tests** — `92fe83f` (feat) — Provider struct + 6 methods (Name/FindID/ListEpisodes/ListServers/GetStream/HealthCheck) + 14 table-driven tests. TDD RED→GREEN verified.
4. **Task 4: Register in main.go + allowlist** — `f8b9c3a` (feat) — main.go failover slot 6 registration; `candidateProviders` slice updated; `my.1anime.site` added to `HLSProxyAllowedDomainsWithProvenance` in struct format.

_Plan metadata commit follows this SUMMARY._

## Test Fixture Set + Behaviors Asserted

### Fixtures (captured live 2026-05-20 from production-server probe)

| Fixture | Size | Provenance |
|---|---|---|
| `wp_search_frieren.json` | 341 B | `GET https://9anime.me.uk/wp-json/wp/v2/search?search=frieren&per_page=20` |
| `series_frieren_s2.html` | 140 KiB | `GET https://9anime.me.uk/series/frieren-beyond-journeys-end-season-2/` |
| `episode_1.html` | 143 KiB | `GET https://9anime.me.uk/hd-frieren-beyond-journeys-end-season-2-episode-1-english-subbed/` |
| `embed_1anime_site.html` | 4 KiB | `GET https://my.1anime.site/index.php?action=play&file=frieren-beyond-journeys-end-season-2-episode-1.mp4` (with `Referer: https://9anime.me.uk/`) |

### Behaviors asserted (14 tests)

| # | Test | Behavior |
|---|---|---|
| 1 | `TestNew_RequiresHTTP` | Missing Deps.HTTP → error mentions "HTTP" |
| 2 | `TestNew_RequiresCache` | Missing Deps.Cache → error mentions "Cache" |
| 3 | `TestNew_Name` | Name() returns "nineanime" |
| 4 | `TestFindID_Frieren` | WP REST search → slug "frieren-beyond-journeys-end-season-2" |
| 5 | `TestFindID_YearTiebreaker` | "Season 2" entry wins vs untagged entry on the +0.05 season bonus |
| 6 | `TestFindID_NoSeries` | subtype:post/page only → `ErrNotFound` |
| 7 | `TestFindID_NegativeCacheHit` | Second miss is served from negative cache (exactly 1 HTTP hit) |
| 8 | `TestFindID_BelowThreshold` | JW score < 0.85 → `ErrNotFound` |
| 9 | `TestListEpisodes_Frieren` | series HTML → ≥1 sorted episodes; Episode.ID is full canonical URL (Pitfall 5) |
| 10 | `TestListServers_SingleServer` | Single "1anime" server, Type=CategorySub |
| 11 | `TestGetStream_Frieren` | iframe walk → embed walk → absolute MP4 URL + Referer "https://my.1anime.site/" |
| 12 | `TestGetStream_NoIframe` | episode HTML lacking iframe → `ErrExtractFailed` |
| 13 | `TestGetStream_NoVideoSource` | embed HTML lacking `<source>` → `ErrExtractFailed` |
| 14 | `TestMarkStage_Success` | HealthCheck snapshot has all 5 stage keys |

Compile-time: `var _ domain.Provider = (*Provider)(nil)` in both `client.go` and `client_test.go`.

## Package Shape + Line Counts

| File | Lines | Plan minimum | Role |
|---|---|---|---|
| `doc.go` | 121 | — | Package doc, brand-jack trade-off, MP4-only contract, WR-04 anti-pattern |
| `dto.go` | 41 | 30 | WP search JSON shape + episodeRef cache shape |
| `cache.go` | 181 | 80 | 5 key families + negative-cache layer |
| `client.go` | 594 | 350 | Provider impl + 6 methods + helpers + compile-time assertion |
| `client_test.go` | 533 | 200 | 14 tests + in-memory cache double + fixture reader |

## Registration Position in main.go

Registered AFTER Miruro (slot 5), BEFORE the `cfg.AnimeKai.Enabled` gated block. The failover chain (per CONTEXT.md D5):

```
gogoanime → animepahe → allanime → animefever → miruro → nineanime → [animekai gated]
   slot 1      slot 2     slot 3     slot 4      slot 5    slot 6
```

`candidateProviders` slice now: `["gogoanime", "animepahe", "allanime", "animefever", "miruro", "nineanime"]`.

The Phase 19 wiring invariant (`expectedProviders` boot check) automatically accounts for the new entry — fatals at boot if the registered count doesn't match.

## Allowlist Entry Added

```go
// Phase 28 (SCRAPER-HEAL-39) — 9anime.me.uk MP4 embed + CDN host.
{Domain: "my.1anime.site", Reason: "9anime.me.uk MP4 embed + CDN host (Phase 28 SCRAPER-HEAL-39)", Owner: "@legacy", Added: "2026-05-20"},
```

Single new entry in the structured `HLSProxyAllowedDomainsWithProvenance` slice. The `HLSProxyAllowedDomains` flat-string view is regenerated automatically by `HLSProxyAllowedDomainsList()`. Per CONTEXT.md D7, the entry lands in the same commit as the provider it serves.

## Marriagetoxin E2E Evidence — DEFERRED to post-merge

Task 5 (`type="checkpoint:human-verify"`) is the post-merge human-verification gate. Per Wave 3 / 28-04 precedent, the parallel-worktree executor cannot run `make redeploy-scraper`/`make redeploy-streaming`/`curl http://localhost:8088/scraper/health` from inside the worktree — those gates require the merged main branch with all of Wave 1 + Wave 2 + Wave 3's plan-05 + plan-06 simultaneously deployed.

Recommendation: after the orchestrator merges 28-05 + 28-06, operator runs the Task-5 verification block:

```bash
# 1. Tests pass
cd /data/animeenigma
go test ./services/scraper/internal/providers/nineanime/... -race -count=2

# 2. Health snapshot
curl -s http://localhost:8088/scraper/health | jq '.providers.nineanime.stages'

# 3. Marriagetoxin episodes
curl -s 'http://localhost:8000/api/anime/<marriagetoxin-uuid>/scraper/episodes?provider=nineanime' | jq '. | length'

# 4. Stream type === "mp4"
curl -s 'http://localhost:8000/api/anime/<marriagetoxin-uuid>/scraper/stream?provider=nineanime&episode=<eid>' | jq '.data.sources[0].type'

# 5. HLS proxy gate
curl -sI "http://localhost:8000/api/streaming/proxy?url=<mp4-url>&referer=https%3A%2F%2Fmy.1anime.site%2F"

# 6. Failover smoke
SCRAPER_DEGRADED_PROVIDERS=allanime,animefever,miruro make redeploy-scraper
```

The unit-test path (Task 3) already covers the byte-level pipeline against the captured live-recon fixtures, so a regression in the brand-jack upstream surfaces as a stages.search→down on the daily canary within 24h.

## Observed Brand-Jack Quirks

Documented for future maintainers (in addition to the Pitfalls already captured in `doc.go`):

1. **Irregular slugs (Pitfall 5):** Episode 1 of Frieren S2 is at `/hd-frieren-beyond-journeys-end-season-2-episode-1-english-subbed/`. Episodes 2..28 lack the `hd-` prefix. The captured `series_frieren_s2.html` fixture confirms both forms appear in the anchor list. Our implementation stores the full `href` and never reconstructs it.
2. **Compound class on episode anchors:** The episode anchor has `class="item ep-item "` (two classes, with trailing space). goquery's `a.ep-item` selector matches correctly because goquery treats class lists.
3. **Per-D2 wrong-episode embed:** CONTEXT.md D2 documents that the episode-7 page was found embedding episode-6's MP4 file. This was NOT directly observed during this plan's recon (we captured episode 1, which embedded episode 1 correctly). It remains an accepted trade-off per D2 — `ReportButton` catches user-reported errors.
4. **WP REST API is live & well-shaped:** Returns clean JSON with `subtype:"series"` for real anime entries. Confirms the Pitfall 4 workaround (use `/wp-json/wp/v2/search`) is the right call.

## Decisions Made

See `key-decisions` in frontmatter for the full list; the most important:

- **Drop the dead Embeds field (WR-04):** `nineanime.Deps` has 4 fields (`BaseURL`, `HTTP`, `Cache`, `Log`) — NO `Embeds`. MP4 extraction is inline regex. Adding a dead field would mislead a future maintainer into thinking the embed registry plays a role here.
- **Episode.ID = full canonical URL (Pitfall 5):** Slug irregularity makes any string-concat URL reconstruction wrong for ~1/28 episodes.
- **Negative cache (24h TTL):** Pattern lifted from `animepahe/malsync.go`. CONTEXT.md `<risks>` flags 9anime catalog gaps.

## Deviations from Plan

### [Rule 2 — missing critical functionality] Test isolation pattern adjustment

**Found during:** Task 3 (writing `TestGetStream_Frieren`)

**Issue:** The plan's `<behavior>` specified an httptest pair, but the captured `episode_1.html` fixture has a hard-coded reference to `my.1anime.site` in the iframe `src`. Naive use would either (a) require fixture rewriting per test (brittle) or (b) require an actual DNS lookup of `my.1anime.site` (defeats the point of an httptest). The plan also pre-mentioned `srv2URL` as a function — but Go's httptest API doesn't expose the URL until after `NewServer` returns.

**Fix:** Used a single httptest server with a closure that captures the server URL after construction. The handler rewrites the fixture's iframe `src` to point at the same server's `/index.php?action=play&file=...` route. This gives test isolation (no DNS) AND tests the rewrite-and-walk pipeline end-to-end. The public-facing `Referer` header is hard-coded to `https://my.1anime.site/` in the provider so the test asserts both the host-walk (via server URL match) AND the Referer contract.

**Files modified:** `services/scraper/internal/providers/nineanime/client_test.go`

**Verification:** All 14 tests pass with `-race -count=2`; specifically `TestGetStream_Frieren` asserts `stream.Headers["Referer"] == "https://my.1anime.site/"`.

**Committed in:** `92fe83f` (Task 3 commit — written within the TDD GREEN phase as part of the implementation iteration).

---

**Total deviations:** 1 auto-fixed (Rule 2 — test-isolation completeness; spec-vs-API mismatch resolved cleanly)
**Impact on plan:** Zero scope creep. Test asserts the same contract the plan's `<behavior>` required, just with a cleaner closure pattern than the plan's pseudo-code.

## Issues Encountered

- None beyond the deviation above.

## Known Stubs

None — every method has live functionality, every test has a real assertion. The Task-5 E2E gate is intentionally deferred to post-merge, not stubbed.

## Deferred Items

1. **Task 5 (Marriagetoxin E2E gate):** Defer to the orchestrator's post-merge redeploy. See "Marriagetoxin E2E Evidence" above for the verification commands.
2. **None other.** The 9anime catalog gaps (Frieren S1 absent, etc.) are operator-accepted trade-offs per D2, not bugs to fix.

## Threat Flags

None new beyond the plan's `<threat_model>` (T-28-05-01..08). All threats are mitigated as documented:

- T-28-05-01 (Tampering — WP search injection): `url.QueryEscape` on `ref.Title` ✓
- T-28-05-02 (Repudiation — wrong-anime fuzzy match): JW≥0.85 + year/season tiebreakers + doc.go uncertainty note ✓
- T-28-05-03 (Information Disclosure — brand-jack wrong-episode embed): accepted per D2; no code defense ✓
- T-28-05-04 (DoS — large body): `io.LimitReader` 4 MiB series / 1 MiB embed ✓
- T-28-05-05 (SSRF — iframe URL): iframe URL parsed as `url.URL`; production layer protected by HLS proxy allowlist (`my.1anime.site` only) ✓
- T-28-05-06 (DoS — rate-limit abuse): `WithPerHostRPS("9anime.me.uk", 1.0, 2)` + `WithPerHostRPS("my.1anime.site", 1.0, 2)` ✓
- T-28-05-07 (Tampering — subtype rebrand): FindID returns ErrNotFound + negative cache; operator kill via DEGRADED env ✓
- T-28-05-08 (Repudiation — broken default search): we use WP REST API not `?s=` ✓

## User Setup Required

None — operator only needs to redeploy scraper + streaming after merge. No env vars to add (`SCRAPER_NINEANIME_BASE_URL` defaults to `https://9anime.me.uk`).

## Next Phase Readiness

- 9anime provider operational in failover slot 6.
- 28-06 (frontend source dropdown polish) can now reference `nineanime` as a known provider name; the `capitalizeProvider` mapping `'nineanime' → '9anime'` is documented in CONTEXT.md and 28-RESEARCH.md.
- v3.1 milestone audit gate: SCRAPER-HEAL-39 resolved.

## Commits

| Hash      | Subject                                                                              |
|-----------|--------------------------------------------------------------------------------------|
| `6e41c7b` | test(28-05): capture 9anime.me.uk live recon testdata fixtures                        |
| `1aeb3d1` | feat(28-05): scaffold nineanime package + NineAnimeConfig                             |
| `92fe83f` | feat(28-05): implement nineanime.Provider with WP REST + MP4 extraction               |
| `f8b9c3a` | feat(28-05): register nineanime as failover slot 6 + allowlist my.1anime.site         |

## Self-Check: PASSED

All 10 created/modified files present on disk. All 4 task commits resolve in `git log --all`. Tests green:

```
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/nineanime  1.072s
ok  github.com/ILITA-hub/animeenigma/services/scraper/internal/config               1.014s
ok  github.com/ILITA-hub/animeenigma/libs/videoutils                                0.005s
```

`go build ./services/scraper/cmd/scraper-api/...` succeeds. No forbidden imports (chromedp/utls/goja) introduced. WR-04 dead-Embeds-field rule honored.

---
*Phase: 28-provider-expansion-r2*
*Plan: 05*
*Completed: 2026-05-20*
