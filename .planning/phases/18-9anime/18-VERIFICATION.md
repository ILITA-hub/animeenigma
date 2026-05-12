---
phase: 18-9anime
verified: 2026-05-12T16:49:27Z
status: human_needed
score: 32/32 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Live failover smoke in production browser"
    expected: "With AnimePahe forced into a failing state (e.g., gauge=0 or upstream unreachable), the English player transparently serves a playable HLS stream from gogoanime/Anitaku, and Prometheus counter parser_fallback_total{from=\"animepahe\",to=\"gogoanime\"} increments by at least 1."
    why_human: "Requires live browser + real network playback + visual confirmation of HLS playback. Compensating control: TestOrchestrator_AnimePaheToGogoanimeFailover asserts the same contract at the unit level (PASS confirmed) — see services/scraper/internal/service/orchestrator_phase18_test.go."
---

# Phase 18: 9anime → Anitaku/Gogoanime (pivoted) Verification Report

**Phase Goal:** A second alive EN provider (gogoanime via anitaku.to) is in rotation so a single provider failure does not blank the English tab for users.
**Verified:** 2026-05-12T16:49:27Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal — a second alive EN provider in rotation behind AnimePahe — is achieved. The mirror chain pivot (9anime → gogoanime/anitaku.to, documented in REQUIREMENTS.md and 18-RESEARCH.md) was a deliberate, justified deviation: all 6 SCRAPER-9ANI requirement IDs are mapped to the gogoanime provider, the orchestrator failover contract is enforced in code and verified at the unit level, and live `/scraper/health` confirms both providers (animepahe + gogoanime) are registered and probed across all 5 stages. One contractually-required behavior — live end-to-end failover smoke in a real browser — is routed to human verification per autonomous-mode policy, with a compensating integration test (PASS).

### Observable Truths

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | REQUIREMENTS.md annotates SCRAPER-9ANI-01..06 as implemented by gogoanime/Anitaku | VERIFIED | `.planning/REQUIREMENTS.md` lines 9-11 carry the 2026-05-12 pivot note; all 6 IDs marked Complete in the traceability table. |
| 2 | ROADMAP.md Phase 18 reflects the pivot and lists 4 plans | VERIFIED | gsd-sdk roadmap.get-phase 18 returns 4 plans (18-01..04), all checked, success_criteria intact. |
| 3 | services/scraper/internal/fuzzy/ package exports NormalizeTitle and JaroWinkler (verbatim relocation) | VERIFIED | `jarowinkler.go:9 func JaroWinkler`, `normalize.go:20 func NormalizeTitle`. |
| 4 | animepahe imports fuzzy; no longer defines local jaroWinkler/normalizeTitle | VERIFIED | `services/scraper/internal/providers/animepahe/client.go:51` imports the fuzzy package; `client.go:294,300` call `fuzzy.NormalizeTitle` / `fuzzy.JaroWinkler`. |
| 5 | 8 gogoanime golden fixtures exist + anonymized (no Set-Cookie, __ddg2_, cf_clearance, Bearer) | VERIFIED | 8 files present in `services/scraper/testdata/gogoanime/`; `grep -lE` for forbidden secrets returned empty. |
| 6 | config.go exposes GogoanimeConfig.BaseURL with env override default https://anitaku.to | VERIFIED | `services/scraper/internal/config/config.go` exposes `GogoanimeConfig`; docker/.env.example documents `SCRAPER_GOGOANIME_BASE_URL=https://anitaku.to` (line 86). |
| 7 | `make capture-goldens-gogoanime` target exists | VERIFIED | `Makefile:120-123` defines target invoking `services/scraper/scripts/capture-gogoanime-goldens.sh`. |
| 8 | gogoanime.Provider implements domain.Provider — Name + FindID + ListEpisodes + ListServers + GetStream + HealthCheck | VERIFIED | `gogoanime/client.go` lines 197, 232, 330, 538, 639, 217 respectively; compile-time assertion at line 705 (`var _ domain.Provider = (*Provider)(nil)`). |
| 9 | FindID calls malsync first, then fuzzy fallback with JaroWinkler ≥ 0.85 against fuzzy.NormalizeTitle | VERIFIED | `gogoanime/client.go:298,304` invoke fuzzy.NormalizeTitle + fuzzy.JaroWinkler; gogoanime/malsync.go uses provider key "Gogoanime" (line 34). |
| 10 | Malsync misses negative-cached for 24h at malsync:{mal_id}:gogoanime:miss | VERIFIED | `gogoanime/malsync.go:40-46,135-145,152,193-194`. Tests pass: TestMalSync_NegativeCacheForGogoanime, TestFindID_MalsyncNegativeCache. |
| 11 | ListEpisodes fetches /category/<slug> AND -dub, merges by episode number, tags CategorySub/Dub, caches 6h | VERIFIED | client.go:330 + tests TestListEpisodes_SubDubMerge, TestListEpisodes_CacheHit (PASS). |
| 12 | ListServers parses anime_muti_link a[data-video], normalizes proto-relative, filters myvidplay/playmogo | VERIFIED | client.go:586 `doc.Find(".anime_muti_link a[data-video]")`; `turnstileHosts = []string{"myvidplay.com", "playmogo.com"}` at line 111; test TestListServers_AnimeMutiLink + TestListServers_DoodstreamSkipped (PASS). |
| 13 | GetStream looks up embedURL via p.embeds.Find, dispatches with Referer: anitaku.to, TTL = min(parsedExpiry-30s, 5min) | VERIFIED | client.go:651 `p.embeds.Find(serverID)`; cache.go has computeStreamTTL; test TestGetStream_DispatchesToRegistry + TestGetStream_StreamTTL (PASS). |
| 14 | HealthCheck reports 4 canonical stages with health.StageSearch/Episodes/Servers/Stream | VERIFIED | client.go:87-90, 237, 244, 252... — all 4 stage constants used + LastOK/LastErr recording. |
| 15 | Error wrapping discipline: WrapProviderDown, WrapExtractFailed, WrapNotFound used per upstream class | VERIFIED | gogoanime/client.go uses `domain.WrapProviderDown` for transport, `WrapExtractFailed` for parse, `WrapNotFound` for 0 results / fuzzy < 0.85 — orchestrator failover relies on this discipline (confirmed by passing failover test). |
| 16 | parser_zero_match_total{provider="gogoanime",selector=<named_const>} increments on zero-match paths | VERIFIED | `selectorAnimeMutiLinkItem` etc. defined as named consts (client.go:100); metrics.ParserZeroMatchTotal use threaded through gogoanime + packed_common. |
| 17 | 3 EmbedExtractors implement domain.EmbedExtractor with compile-time assertions | VERIFIED | `vibeplayer.go:224`, `streamhg.go:64`, `earnvids.go:59`: `var _ domain.EmbedExtractor = (*X)(nil)`. |
| 18 | Subdomain-impostor rejection: evilvibeplayer.site does NOT match vibeplayer.site | VERIFIED | TestVibePlayer_Matches_RejectsSubdomainImposters + TestStreamHG_Matches_RejectsSubdomainImposters + TestEarnvids_Matches_RejectsSubdomainImposters (PASS). |
| 19 | All 3 extractors use io.LimitReader(2 MiB cap) | VERIFIED | `vibeplayer.go:166` (maxVibePlayerBody = 2<<20); `packed_common.go:189` (maxPackedBody = 2<<20). |
| 20 | Each Extract returns *domain.Stream{Sources: [{URL, Type: "hls"}], Headers: {Referer}} | VERIFIED | vibeplayer.go:180-182; packed_common.go:230-232 (shared base) — both set Headers["Referer"]. |
| 21 | StreamHG/Earnvids share packedExtractor base — Dean-Edwards unpacking implemented ONCE | VERIFIED | streamhg.go + earnvids.go both compose `*packedExtractor`; goja runner lives in packed_common.go. |
| 22 | main.go registers all 3 new extractors BEFORE gogoanime provider | VERIFIED | main.go:59-67 instantiate vibeplayer/streamhg/earnvids extractors; line 142 constructs gogoanime.New AFTER animepahe registration. |
| 23 | main.go adds per-host RPS limits (1.0 RPS, burst 2) for anitaku.to, vibeplayer.site, otakuhg.site, otakuvid.online | VERIFIED | main.go:135-138 add 4 `domain.WithPerHostRPS` entries. |
| 24 | libs/videoutils/proxy.go::HLSProxyAllowedDomains contains 5 new entries (anitaku.to, vibeplayer.site, premilkyway.com, dramiyos-cdn.com, cdn.cimovix.store) appended | VERIFIED | proxy.go:258-262 contain all 5; TestHLSProxyAllowedDomains_Phase18Additions PASS. |
| 25 | Phase 16 regression: kwik.cx + owocdn.top + uwucdn.top still present | VERIFIED | proxy.go:243-245; TestHLSProxyAllowedDomains_Phase16RegressionLocked PASS. |
| 26 | Rotating-subdomain match logic still works (e.g. OkqtSs1gBbNcA8e.premilkyway.com allowlisted) | VERIFIED | TestIsHLSDomainAllowed_RotatingSubdomains/Phase_18_subdomain + StreamHG_rotating_subdomain + Earnvids_rotating_subdomain PASS. |
| 27 | EnglishPlayer.vue::capitalizeProvider has `if (slug === 'gogoanime') return 'Anitaku'` | VERIFIED | `EnglishPlayer.vue:542` exact match. |
| 28 | EnglishPlayer.vue source dropdown v-else replaced with multi-option panel (accent on selected, hover on available, offline suffix, tried-chain) | VERIFIED | EnglishPlayer.vue:194-224 panel; `panelOpen` state; tried chain at line 224. |
| 29 | switchProvider(next) defined: saves currentTime, sets selectedProvider + preferredScraperProvider, re-fetches, restores on success, rolls back + toasts on failure | VERIFIED | EnglishPlayer.vue:962 `async function switchProvider(next: string)`. |
| 30 | Source dropdown trigger role=combobox + aria-expanded + aria-controls; items role=option + aria-selected + aria-disabled; aria-live polite announcement | VERIFIED | EnglishPlayer.vue:168-170, 196-198, 227 — all six aria attributes present. |
| 31 | changelog.json gains a feature entry announcing Anitaku as second EN source (informative + enthusiastic + emojis) | VERIFIED | changelog.json contains `🎌 Добавили резервный английский источник Anitaku/Gogoanime ... 🚀` — informative + enthusiastic + emojis per CLAUDE.md convention. |
| 32 | curl /scraper/health shows both animepahe + gogoanime providers with HealthCheck snapshots; /metrics shows provider_health_up{provider="gogoanime",...} across all stages | VERIFIED | Live `curl http://localhost:8088/scraper/health` returns both providers with 4 stage snapshots each; `curl /metrics` shows provider_health_up{provider="gogoanime",stage="..."} = 1 for episodes/search/servers/stream/stream_segment. |

**Score:** 32/32 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `services/scraper/internal/fuzzy/jarowinkler.go` | Shared JaroWinkler | VERIFIED | exports `JaroWinkler` |
| `services/scraper/internal/fuzzy/normalize.go` | Shared title normalization | VERIFIED | exports `NormalizeTitle` |
| `services/scraper/internal/fuzzy/fuzzy_test.go` | Migrated tests | VERIFIED | TestJaroWinkler_KnownPairs + TestNormalizeTitle_Cases PASS |
| `services/scraper/internal/providers/gogoanime/client.go` | Provider impl | VERIFIED | 705+ lines, all interface methods, compile-time assertion |
| `services/scraper/internal/providers/gogoanime/dto.go` | DTOs | VERIFIED | searchResult, episodeRow, serverRow + malsync shape |
| `services/scraper/internal/providers/gogoanime/malsync.go` | 24h pos/neg cache | VERIFIED | uses "Gogoanime" provider key |
| `services/scraper/internal/providers/gogoanime/cache.go` | computeStreamTTL | VERIFIED | TestComputeStreamTTL_StreamHGSignedURL PASS |
| `services/scraper/internal/providers/gogoanime/client_test.go` | GREEN tests | VERIFIED | 16 tests across gogoanime package PASS |
| `services/scraper/internal/embeds/vibeplayer.go` | regex-only extractor | VERIFIED | TestVibePlayer_* PASS |
| `services/scraper/internal/embeds/streamhg.go` | packed extractor | VERIFIED | TestStreamHG_* PASS |
| `services/scraper/internal/embeds/earnvids.go` | packed extractor | VERIFIED | TestEarnvids_* PASS |
| `services/scraper/internal/embeds/packed_common.go` | shared Dean-Edwards base | VERIFIED | TestPackedExtractor_* PASS |
| `services/scraper/cmd/scraper-api/main.go` | wiring: extractors + gogoanime + RPS | VERIFIED | all 3 extractors registered before gogoanime; 4 new RPS limits added |
| `libs/videoutils/proxy.go` | 5 new HLS proxy hosts | VERIFIED | append-only edit; Phase 16 hosts preserved |
| `libs/videoutils/proxy_test.go` | Phase 18 + Phase 16 regression tests | VERIFIED | all 3 added tests PASS |
| `frontend/web/src/components/player/EnglishPlayer.vue` | multi-option dropdown + switchProvider + aria + capitalizeProvider | VERIFIED | all 4 contracts present |
| `frontend/web/public/changelog.json` | feature entry for Anitaku | VERIFIED | RU localized message with emojis |
| `services/scraper/testdata/gogoanime/*` (8 fixtures + README) | Anonymized goldens | VERIFIED | 8 files; no Set-Cookie/__ddg2_/cf_clearance/Bearer |
| `services/scraper/scripts/capture-gogoanime-goldens.sh` | Capture script | VERIFIED | Referenced from Makefile target |
| `services/scraper/internal/service/orchestrator_phase18_test.go` | Failover contract test | VERIFIED | TestOrchestrator_AnimePaheToGogoanimeFailover PASS |

**Artifact result:** all 27 declared artifacts pass existence + substance checks. The single gsd-sdk-flagged "missing pattern: TestStreamHG_ComputeTTL" is a benign symbol rename (the RED-state scaffold name `TestStreamHG_ComputeTTL` was renamed to `TestStreamHG_ExtractURL_HasExpiryQuery` during the 18-03 GREEN phase, which is the actual TTL contract test — verified to PASS). No artifact is a stub, missing, or unsubstantiated.

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| animepahe/client.go | scraper/internal/fuzzy | import + fuzzy.JaroWinkler / NormalizeTitle | WIRED (line 51 + 294 + 300) |
| config/config.go | docker/.env | `SCRAPER_GOGOANIME_BASE_URL` | WIRED (line 86 of .env.example) |
| Makefile | scripts/capture-gogoanime-goldens.sh | `capture-goldens-gogoanime` target | WIRED (Makefile:120-123) |
| gogoanime/client.go | internal/domain | implements domain.Provider | WIRED (lines 232, 330, 538, 639 use domain.* types; compile-time assert at 705) |
| gogoanime/client.go | internal/fuzzy | fuzzy.JaroWinkler + NormalizeTitle | WIRED (lines 298, 304) |
| gogoanime/client.go | internal/embeds (registry) | p.embeds.Find | WIRED (line 651) |
| gogoanime/client.go | internal/health | health.StageSearch/Episodes/Servers/Stream | WIRED (lines 87-90, 237, 244, …) |
| gogoanime/client.go | libs/metrics | ParserZeroMatchTotal | WIRED |
| gogoanime/malsync.go | libs/cache | Redis 24h pos+neg cache | WIRED |
| embeds/vibeplayer.go | internal/domain | implements EmbedExtractor | WIRED (line 224 compile-time assert) |
| embeds/streamhg.go | embeds/packed_common.go | composes *packedExtractor | WIRED |
| embeds/earnvids.go | embeds/packed_common.go | composes *packedExtractor | WIRED |
| embeds/packed_common.go | embeds/kwik.go | reuses goja + LimitReader patterns | WIRED |
| {3 extractors} | libs/metrics | ParserZeroMatchTotal | WIRED (via packed_common.go:191,202,216,222 + vibeplayer.go) |
| cmd/scraper-api/main.go | providers/gogoanime | gogoanime.New + register | WIRED (line 142) |
| cmd/scraper-api/main.go | internal/embeds (3 new) | New{VibePlayer,StreamHG,Earnvids}Extractor | WIRED (lines 59, 63, 67) |
| EnglishPlayer.vue | composables/useWatchPreferences | setPreferredScraperProvider | WIRED |
| EnglishPlayer.vue | api/client.ts scraperApi | scraperApi.getServers / getStream | WIRED (EnglishPlayer.vue:866, 1070) |

**Note on tool false-negatives:** `gsd-sdk verify.key-links` flagged 6 links as "Pattern not found" due to its regex engine not handling Go-style `\b` word boundary the same way Go's regexp does. All 6 were manually confirmed WIRED via direct `grep` against the source files (evidence cited in the table). The phase has no actual broken wiring.

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|---|---|---|---|---|
| EnglishPlayer.vue source dropdown | `availableProviders` / `unhealthyProviders` | `scraperApi.getHealth()` (line 1732) | Yes — live `/scraper/health` returns both animepahe + gogoanime snapshots | FLOWING |
| EnglishPlayer.vue stream | `streamUrl` via `scraperApi.getStream(...)` | live scraper service `/stream` endpoint, dispatched to gogoanime.Provider.GetStream when preferred | Yes — provider chain exists and orchestrator failover test confirms backend behavior | FLOWING |
| gogoanime.Provider.ListEpisodes output | `[]domain.Episode` | `/category/<slug>` + `/category/<slug>-dub` HTTP fetches merged | Yes — golden-driven tests assert real merge logic | FLOWING |
| HLSProxyAllowedDomains | static slice with 5 new entries | source-controlled at proxy.go:243-262 | Yes — proxy gate consumes via `IsHLSDomainAllowed`, tested rotating + exact | FLOWING |

No HOLLOW / DISCONNECTED artifacts found.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Fuzzy package tests pass | `go test ./services/scraper/internal/fuzzy/ -count=1` | `ok ... 0.004s` | PASS |
| Gogoanime provider tests pass | `go test ./services/scraper/internal/providers/gogoanime/ -count=1` | `ok ... 0.078s` (16 tests) | PASS |
| 3 embed extractors tests pass | `go test ./services/scraper/internal/embeds/ -count=1` | `ok ... 0.137s` (14 phase-18 tests + Phase 16 kwik) | PASS |
| Orchestrator failover contract | `go test ./services/scraper/internal/service/ -run TestOrchestrator_AnimePaheToGogoanimeFailover` | `--- PASS: ... (0.03s)` | PASS |
| HLS proxy Phase 18 additions + Phase 16 regression + rotating subdomains | `go test ./libs/videoutils/ -run TestHLSProxy...|TestIsHLSDomainAllowed_RotatingSubdomains` | all 13 sub-tests PASS | PASS |
| All services up | `make health` | 8/8 services healthy (incl. scraper:8088) | PASS |
| Live gogoanime registered | `curl /scraper/health` | both `animepahe` and `gogoanime` provider snapshots present, all 4 stages each | PASS |
| Prometheus exposes gogoanime gauges | `curl /metrics \| grep provider_health_up.*gogoanime` | 5 gauges (episodes/search/servers/stream/stream_segment) all = 1 | PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description (verbatim, post-pivot mapping) | Status | Evidence |
|---|---|---|---|---|
| SCRAPER-9ANI-01 | 18-01, 18-02 | Shikimori/MAL → gogoanime slug via malsync.moe with same caching + fuzzy fallback as AnimePahe | SATISFIED | gogoanime/malsync.go + client.go FindID (malsync-first then fuzzy ≥ 0.85); TestFindID_FuzzyPath + TestFindID_MalsyncNegativeCache PASS |
| SCRAPER-9ANI-02 | 18-01, 18-02 | ListEpisodes with sub/dub split, cached 6h | SATISFIED | client.go ListEpisodes merges /category/<slug> + -dub by episode number; 6h cache key `episodes:gogoanime:<slug>`; TestListEpisodes_SubDubMerge PASS |
| SCRAPER-9ANI-03 | 18-01, 18-03, 18-04 | ListServers enumerates embed hosts; each registered as EmbedExtractor reusable for future providers | SATISFIED | ListServers parses anime_muti_link; 3 new EmbedExtractors (vibeplayer, streamhg, earnvids) registered in main.go:59-67 alongside kwik/megacloud |
| SCRAPER-9ANI-04 | 18-01, 18-03, 18-04 | GetStream resolves via ListServers then dispatches to matching EmbedExtractor; no extraction logic in provider | SATISFIED | client.go:651 `p.embeds.Find(serverID)` dispatches; no goja/regex extraction in gogoanime/client.go; TestGetStream_DispatchesToRegistry PASS |
| SCRAPER-9ANI-05 | 18-01, 18-04 | CDN hostnames appended to HLSProxyAllowedDomains | SATISFIED | 5 new entries appended (anitaku.to, vibeplayer.site, premilkyway.com, dramiyos-cdn.com, cdn.cimovix.store) at proxy.go:258-262; Phase 16 hosts preserved |
| SCRAPER-9ANI-06 | 18-01, 18-04 | Failover AnimePahe → gogoanime verified end-to-end; parser_fallback_total increments | SATISFIED (unit-level) + HUMAN-NEEDED (live browser) | TestOrchestrator_AnimePaheToGogoanimeFailover PASS asserts parser_fallback_total{from=animepahe,to=gogoanime} increment + fakePahe.GetStream never called; live browser smoke routed to HUMAN-UAT per autonomous-mode policy |

**Orphaned-requirement check:** REQUIREMENTS.md maps exactly SCRAPER-9ANI-01..06 to Phase 18, and all 6 IDs appear in at least one plan's `requirements:` frontmatter. No orphans.

### Anti-Patterns Found

None. TODO/FIXME/XXX/HACK/PLACEHOLDER scan across all Phase 18 changed files returned zero hits. No `return null`/`return {}`/`return []` stubs in render or API-return paths. No console.log-only handlers. No hardcoded empty props in EnglishPlayer.vue source-dropdown wiring.

### Human Verification Required

#### 1. Live failover smoke (production browser)

**Test:** In production (animeenigma.ru) as a regular user, visit a watchable anime that resolves on both AnimePahe and gogoanime. Force AnimePahe into a failing state — either temporarily mark `provider_health_up{provider="animepahe"}` to 0 via admin tooling, or pick an anime/episode where AnimePahe is genuinely down — then attempt to play the English source.

**Expected:** The English player serves a playable HLS stream sourced from gogoanime/Anitaku without user intervention; the source dropdown shows the chain reflected the failover; `parser_fallback_total{from="animepahe",to="gogoanime"}` in `/metrics` increments by at least 1 from the playback.

**Why human:** Requires live browser playback, real HLS segment fetches through the proxy, and visual confirmation that the player surface (controls, subtitle overlay, time-position restore) survives a mid-load source switch. Programmatic checks cannot observe a rendered video element or assert that segments are actually decoded.

**Compensating control:** `services/scraper/internal/service/orchestrator_phase18_test.go::TestOrchestrator_AnimePaheToGogoanimeFailover` asserts the exact backend contract (parser_fallback_total increment, fakePahe.GetStream never called once health is DOWN) at the unit level — currently PASS.

### Gaps Summary

No functional gaps. All 32 must-have truths verified, all 27 artifacts substantive and wired, all 18 key links connected, all 6 SCRAPER-9ANI requirement IDs satisfied. The single deferred item is a live-browser failover smoke for SCRAPER-9ANI-06, with a passing unit-level compensating test. Status `human_needed` reflects that one item awaits human eyes on a rendered video element — not a missing implementation.

---

_Verified: 2026-05-12T16:49:27Z_
_Verifier: Claude (gsd-verifier)_
