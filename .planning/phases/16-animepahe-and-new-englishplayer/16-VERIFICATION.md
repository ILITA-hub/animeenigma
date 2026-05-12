---
phase: 16-animepahe-and-new-englishplayer
verified: 2026-05-12T05:52:18Z
status: human_needed
score: 11/11 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Redeploy scraper container so live runtime reflects Phase 16 code"
    expected: "After `make redeploy-scraper`, `curl http://localhost:8088/scraper/health | jq '.data.providers'` returns an object with an `animepahe` key (currently `{}` because the running container is still the Phase-15 build). Boot logs show: 'registered embed extractor name=kwik', 'redis connected host=redis port=6379', 'registered provider name=animepahe'."
    why_human: "Live deploy is the orchestrator's responsibility per Plan 16-05 SUMMARY; sequential worktree executors cannot redeploy. Running scraper container still serves `{\"error\":\"not-yet-implemented\",\"phase\":15}` on `/scraper/episodes?mal_id=21`."
  - test: "End-to-end stream playback through the English tab"
    expected: "After `make redeploy-web` + `make redeploy-scraper`, a logged-in EN-language user opens an anime detail page with malsync coverage (e.g. Frieren / Naruto), clicks the English tab, picks an episode, and the HLS stream loads via Video.js + HLS.js. Network tab shows `/api/anime/{id}/scraper/stream` returning 200 with `data.meta.tried: [\"animepahe\"]`."
    why_human: "Visual / interactive verification; requires live AnimePahe egress (Wave 1 16-01 connectivity note: direct `animepahe.ru` TCP-times-out from this server; Cloudflare-fronted `animepahe.com` was the working entry point during fixture capture). Set `ANIMEPAHE_BASE_URL=https://animepahe.com` via docker-compose env override if `.ru` egress remains blocked, then `make restart-scraper`."
  - test: "DDoS-Guard handshake against real upstream"
    expected: "After first `/scraper/episodes` call against a malsync-covered anime, `docker compose exec redis redis-cli KEYS 'malsync:*'` shows entries; the BaseHTTPClient cookie jar contains a `__ddg2_*` cookie for animepahe.ru. Provider HealthCheck reports per-stage success/failure under `/scraper/health.data.providers.animepahe.stages`."
    why_human: "Requires real AnimePahe upstream traffic; unit tests use synthetic fixtures + httptest mocks. The full handshake against the real DDoS-Guard challenge has not been exercised yet (Plan 16-03 SUMMARY 'Followups' line: 'DDoS-Guard end-to-end live test: the cookie helper hasn't been exercised against the real DDoS-Guard upstream yet')."
  - test: "Legacy `?legacy=1` flag reveals HiAnime + Consumet (debug) tabs"
    expected: "On a deployed anime detail page, `?legacy=1` query string adds HiAnime (debug) and Consumet (debug) tab buttons next to the English tab. Without the flag the legacy tabs are NOT visible. EnglishPlayer remains the default tab regardless."
    why_human: "Visual / interactive verification; Playwright spec compiles + lists but live run was deferred to post-merge deploy per Plan 16-06 deviations."
  - test: "Playwright e2e spec passes against live deployment"
    expected: "`cd frontend/web && BASE_URL=https://animeenigma.ru bunx playwright test english-player.spec.ts --reporter=list` — 4 tests pass (1 may skip if no malsync-uncovered seed anime exists)."
    why_human: "Playwright spec exists (190 lines, 4 tests, 12 instances discovered across 3 browser projects) but a live run requires a redeployed scraper + web stack with AnimePahe egress active."
  - test: "ReportButton modal shows Provider + Tried rows when used from the English tab"
    expected: "Open the ReportButton inside EnglishPlayer; modal renders two new rows: 'Provider: AnimePahe' and 'Tried: animepahe'. Submit a test report; Telegram admin chat receives it with these fields populated in the diagnostics payload."
    why_human: "Visual verification of UI rendering + Telegram integration; the snake_case `scraper_provider` and `tried_chain` payload fields are correctly defined in diagnostics.ts but end-to-end Telegram receipt has not been observed in the running stack."
---

# Phase 16: AnimePahe + New EnglishPlayer — Verification Report

**Phase Goal:** "A user opens an anime in the new 'English' tab and watches it end-to-end via AnimePahe. The old HiAnime and Consumet player tabs continue to exist (in a debug-only path) so users have a soak-period fallback."

**Verified:** 2026-05-12T05:52:18Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A single "English" tab is rendered (replacing visible HiAnime + Consumet) for EN-language users; old tabs reachable only via `?legacy=1` | VERIFIED | `frontend/web/src/views/Anime.vue:360` (English tab button gated to `videoProvider === 'english'`); `:367` v-if on `$route.query.legacy === '1'` enables debug tabs; `:1320` default flips to `'english'` for EN-language; legacy tabs marked `(debug)` via `player.tabDebugSuffix` (`:375`, `:384`) |
| 2 | EnglishPlayer.vue exists atop scraperApi with Video.js + HLS.js + SubtitleOverlay + cyan accent | VERIFIED | `frontend/web/src/components/player/EnglishPlayer.vue:1` (1685 lines); class `english-player` (:2); imports `scraperApi` (:433); calls `scraperApi.getEpisodes/getServers/getStream/getHealth` (5 callsites); `--player-accent: #00d4ff` in scoped styles (:1595); SubtitleOverlay imported and rendered |
| 3 | Source dropdown renders in toolbar; Phase 16 single-option read-only chip labelled "AnimePahe" | VERIFIED | `EnglishPlayer.vue:156` `v-if="availableProviders.length === 1"` chip; `:541` `availableProviders = ref(['animepahe'])`; `:557` `reportProvider` computed defaults to first; `data-testid="source-chip"` for e2e |
| 4 | Backend orchestrator wired: KwikExtractor registered with embed registry; animepahe.Provider registered with orchestrator before HTTP serves | VERIFIED | `services/scraper/cmd/scraper-api/main.go:45` `embeds.NewKwikExtractor()` + `registry.Register`; `:58` `cache.New(...)`; `:89` `animepahe.New(animepahe.Deps{...})` returns `(*Provider, error)`; `:103` `orchestrator.Register(animePaheProvider)` — all before router/server start |
| 5 | Handler endpoints return real orchestrator-backed responses (NOT Phase 15's 503 stub); every response carries `meta.tried` | VERIFIED | `services/scraper/internal/handler/scraper.go` has zero `notYetImplemented` (verified by grep); `GetEpisodes/GetServers/GetStream` call `h.svc.ListEpisodes/ListServers/GetStream` (:117/:152/:196); `writeSuccess` always sets `data["meta"] = map[string]any{"tried": tried}` (:229); `writeError` always emits `meta:{tried:[...]}` (:246) |
| 6 | KwikExtractor implements domain.EmbedExtractor for kwik.cx/kwik.si with goja unpacking, SSRF guard, body cap, interrupt watchdog | VERIFIED | `services/scraper/internal/embeds/kwik.go:266/275/308` Name/Matches/Extract; `:435` fresh `goja.New()` per call; `:448-450` `vm.Interrupt` from watchdog goroutine; `:344` `io.LimitReader` 2 MiB cap; `kwikHosts = []string{"kwik.cx", "kwik.si"}` (:41); 9 Kwik tests green |
| 7 | AnimePahe Provider implements domain.Provider with malsync 24h cache + fuzzy fallback, episodes 6h cache, stream cache TTL ≤ min(expires-30s, 5min), DDoS-Guard handshake | VERIFIED | `services/scraper/internal/providers/animepahe/client.go:158/216/292/353/447/178` Name/FindID/ListEpisodes/ListServers/GetStream/HealthCheck; `:489` `var _ domain.Provider = (*Provider)(nil)`; `:219` `p.malsync.Lookup`; `:271` `jaroWinkler` fallback at 0.85; `:480` `computeStreamTTL`; `:191/207` DDoS-Guard handshake via `getWithDDoSGuard` + `ensureDDoSCookie`; 36+ unit tests green |
| 8 | scraperApi client exposed; ReportButton + diagnostics + useWatchPreferences extended; all 12 locale keys present in en/ru/ja | VERIFIED | `frontend/web/src/api/client.ts:416` `export const scraperApi`; `:392` `hiAnimeApi` + `:449` `consumetApi` untouched; ReportButton.vue:123-130 new props + defaults; diagnostics.ts:32-33/51-52 PlayerContext + DiagnosticReport extended (snake_case JSON); useWatchPreferences.ts:67/94/158-159 preferredScraperProvider + setter; locale keys verified in all 3 files (en.json/ru.json/ja.json lines 146-159) |
| 9 | meta.tried propagates from scraper response → triedChain → ReportButton → diagnostics payload (SCRAPER-NF-05 frontend half) | VERIFIED | `EnglishPlayer.vue:563-576` `extractTried` + `updateTriedChain` probes both `data.data.meta.tried` (success envelope) AND `data.meta.tried` (error envelope); `:421` `:scraper-provider="reportProvider"` + `:tried-chain="triedChain"`; ReportButton.vue:42-48 conditional rows render `player.reportProvider` + `player.reportTried`; diagnostics.ts:233-234 maps to `scraper_provider` + `tried_chain` snake_case |
| 10 | HLS proxy allowlist locked with regression test on kwik.cx + owocdn.top + uwucdn.top (SCRAPER-PAHE-05) | VERIFIED | `libs/videoutils/proxy.go:243-245` contains all three hosts; `libs/videoutils/proxy_test.go` contains `TestHLSProxyAllowedDomains_HasAnimePaheHosts` (locked by Plan 16-01); HLSProxyAllowedDomains drives the proxy ServeHTTP path at `:579` |
| 11 | docker-compose scraper service has REDIS_* envs + ANIMEPAHE_BASE_URL + depends_on redis: service_healthy | VERIFIED | `docker/docker-compose.yml:147-178` scraper block contains `REDIS_HOST: redis`, `REDIS_PORT: 6379`, `REDIS_PASSWORD: ""`, `REDIS_DB: 0`, `ANIMEPAHE_BASE_URL: https://animepahe.ru`, `depends_on: { redis: { condition: service_healthy } }`; `docker compose config --quiet` validates clean |

**Score:** 11/11 truths verified.

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `services/scraper/cmd/scraper-api/main.go` | Boot wiring: Kwik + Redis + AnimePahe registered before serve | VERIFIED | 147 lines; `embeds.NewKwikExtractor` (:45), `cache.New` (:58), `animepahe.New` (:89), `orchestrator.Register(animePaheProvider)` (:103) |
| `services/scraper/internal/handler/scraper.go` | Calls orchestrator (not notYetImplemented); response wraps with meta.tried | VERIFIED | 278 lines; zero `notYetImplemented` references; `meta:{tried:[...]}` on every response path |
| `services/scraper/internal/providers/animepahe/client.go` | Provider implements domain.Provider; FindID/ListEpisodes/ListServers/GetStream/HealthCheck | VERIFIED | 489 lines; compile-time assertion at :489; all 6 methods implemented; New(Deps) returns `(*Provider, error)` |
| `services/scraper/internal/embeds/kwik.go` | KwikExtractor implements domain.EmbedExtractor; goja unpack with SSRF + body + timeout guards | VERIFIED | 467 lines; KwikExtractor with Name/Matches/Extract; fresh goja runtime per call; vm.Interrupt from goroutine; 2 MiB body cap |
| `frontend/web/src/components/player/EnglishPlayer.vue` | Unified English-source player atop scraperApi with cyan accent | VERIFIED | 1685 lines; class `english-player`; `--player-accent: #00d4ff`; 5 scraperApi callsites; Source chip; ReportButton bindings |
| `frontend/web/src/views/Anime.vue` | English tab + legacy gating + videoProvider type extension | VERIFIED | 1501 lines; `EnglishPlayer` async import (:786); v-else-if mount (:450-451); legacy gating (:367); savedEn defaults to 'english' (:1320) |
| `frontend/web/e2e/english-player.spec.ts` | E2E covering EnglishPlayer happy path + ReportButton modal + ?legacy=1 | VERIFIED | 236 lines; 4 tests covering tab visibility, legacy flag, ReportButton modal, empty-state (skipped); 12 instances across 3 browser projects |
| `frontend/web/src/api/client.ts` | scraperApi export; hiAnimeApi + consumetApi untouched | VERIFIED | `scraperApi` (:416), `hiAnimeApi` (:392), `consumetApi` (:449) all present |
| `frontend/web/src/locales/{en,ru,ja}.json` | 12 new player.* keys present in all 3 locales | VERIFIED | Lines 146-159 in each file contain all 12 keys (tabEnglish, tabDebugSuffix, source, sourceSingleTooltip, sourceMultiTooltip, sourceUnhealthy, englishNotAvailable.{heading,body}, sourceSwitchFailed, sourceUnavailable, reportProvider, reportTried) |
| `docker/docker-compose.yml` | Scraper service has REDIS_* envs + ANIMEPAHE_BASE_URL + depends_on redis | VERIFIED | Lines 147-178; all required envs + dependency present; `docker compose config --quiet` clean |
| `libs/videoutils/proxy.go` + `proxy_test.go` | HLSProxyAllowedDomains contains kwik.cx + owocdn.top + uwucdn.top with regression test | VERIFIED | Hosts at proxy.go:243-245; regression test `TestHLSProxyAllowedDomains_HasAnimePaheHosts` locks them |

### Key Link Verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `main.go` | `providers/animepahe` | `animepahe.New(Deps{...}) → orchestrator.Register` | WIRED | grep `animepahe.New` returns 2 matches; orchestrator.Register on line 103 |
| `main.go` | `embeds/kwik.go` | `embeds.NewKwikExtractor() → registry.Register` | WIRED | grep `embeds.NewKwikExtractor` returns 1 match at line 45 |
| `main.go` | `libs/cache` | `cache.New(cache.Config{...})` | WIRED | grep `cache.New` returns 1 match at line 58; redisCache passed to provider Deps |
| `handler/scraper.go` | `service.Orchestrator` | `h.svc.ListEpisodes/ListServers/GetStream/FindID` | WIRED | All four called via the handler chain; resolveProviderID hops through orchestrator.FindID |
| `provider/client.go` | `embeds (KwikExtractor via registry)` | `p.embeds.Find(serverID)` inside GetStream | WIRED | client.go:459 calls `p.embeds.Find` then extractor.Extract with Referer header |
| `provider/client.go` | `BaseHTTPClient.Jar()` (DDoS cookie inspection) | `p.http.Jar()` via ensureDDoSCookie | WIRED | ddosguard.go uses Jar() to check `__ddg2_*` prefix cookies; 2 short-circuit paths |
| `provider/malsync.go` | `libs/cache` | Imports `cache.Cache` for Redis client | WIRED | malsync.go uses cache.Cache interface; 24h positive + 24h negative cache keys |
| `EnglishPlayer.vue` | `client.ts scraperApi` | Imports scraperApi; calls getEpisodes/getServers/getStream/getHealth | WIRED | 5 scraperApi callsites + 1 getHealth on mount |
| `EnglishPlayer.vue` | `SubtitleOverlay.vue` | Reused as-is, no edits | WIRED | Imported; rendered with teleport into fullscreen element |
| `EnglishPlayer.vue` | `ReportButton.vue` | Binds `:scraper-provider` + `:tried-chain` | WIRED | EnglishPlayer.vue:421 binds reportProvider computed + triedChain ref |
| `Anime.vue` | `EnglishPlayer.vue` | `defineAsyncComponent(() => import(...))` + v-else-if mount | WIRED | Anime.vue:786 + :450-451 |
| `ReportButton.vue` | `diagnostics.ts` | `collectDiagnostics` receives scraperProvider + triedChain | WIRED | ReportButton.vue:180-181 threads props into collectDiagnostics |
| `scraperApi (frontend)` | `/api/anime/{id}/scraper/*` (catalog → scraper) | axios.get against four routes | WIRED | client.ts:416-440 four methods point at correct routes |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|---|---|---|---|---|
| EnglishPlayer.vue `episodes.value` | episodes (ref<Episode[]>) | `scraperApi.getEpisodes(animeId, prefer)` → `response.data.data.episodes` | Yes (live orchestrator → AnimePahe Provider.ListEpisodes — 50-page pagination, 6h cache) | FLOWING (code path is intact; live runtime requires redeploy — see human verification #1) |
| EnglishPlayer.vue `servers.value` | servers (ref<Server[]>) | `scraperApi.getServers(animeId, episodeId, prefer)` | Yes (Provider.ListServers scrapes /play HTML with goquery for `data-src` kwik URLs; Server.Type populated from `data-audio`) | FLOWING (sub/dub filtering wired via CR-02 fix) |
| EnglishPlayer.vue `streamUrl.value` | streamUrl + sources | `scraperApi.getStream(...)` → KwikExtractor via embed registry | Yes (Provider.GetStream → registry.Find(kwikURL) → KwikExtractor.Extract → goja unpack of packed JS → m3u8 URLs with optional quality labels; TTL capped at min(expires-30s, 5min)) | FLOWING (code path is intact; live runtime depends on real DDoS-Guard handshake + AnimePahe egress) |
| EnglishPlayer.vue `triedChain.value` | triedChain (ref<string[]>) | `response.data.data.meta.tried` (success) OR `response.data.meta.tried` (error) | Yes (handler.go always emits `meta.tried` from orchestrator.OrderedProviderNames) | FLOWING |
| ReportButton.vue rows | scraperProvider + triedChain props | EnglishPlayer.vue's reportProvider computed + triedChain ref | Yes | FLOWING |
| Anime.vue tab visibility | `videoProvider` + `$route.query.legacy` | localStorage `preferred_en_provider` + URL query | Yes (default 'english'; legacy gate strict `=== '1'`) | FLOWING |

**Note on FLOWING with caveat:** All code paths exist and unit/integration tests pass. The running production container is still the Phase-15 build (verified by `curl http://localhost:8088/scraper/episodes?mal_id=21` returning the 503 stub). Live data flow from the deployed scraper to the deployed web tier requires `make redeploy-scraper && make redeploy-web` — listed as human verification #1.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Scraper Go tests all green | `cd services/scraper && go test ./... -count=1 -timeout 120s` | All 9 packages return `ok`; zero failures | PASS |
| Scraper Go build clean | `cd services/scraper && go build ./...` | No output (clean exit) | PASS |
| Catalog passthrough tests still green (regression check) | `cd services/catalog && go test ./internal/handler ./internal/parser/scraper -count=1` | Both packages return `ok` | PASS |
| docker-compose syntactically valid | `docker compose -f docker/docker-compose.yml config --quiet` | No errors | PASS |
| Locale keys present in all 3 locale files | `grep -c "tabEnglish" frontend/web/src/locales/{en,ru,ja}.json` | All 3 files contain the key (lines 146 of each) | PASS |
| Scraper container is running and healthy | `docker ps --format '{{.Names}} {{.Status}}'` | `animeenigma-scraper Up 23 hours (healthy)` | PASS (but running PRE-Phase-16 build) |
| Scraper `/health` endpoint responds 200 | `curl -sS http://localhost:8088/health` | `{"success":true,"data":{"status":"ok"}}` | PASS |
| Live scraper provider registered | `curl -sS http://localhost:8088/scraper/health` | `{"success":true,"data":{"providers":{}}}` — empty! | FAIL: running container is Phase-15 build; Phase-16 code on disk but not redeployed. Routed to human verification #1. |
| Live scraper episodes returns 200 (not 503) | `curl -sS http://localhost:8088/scraper/episodes?mal_id=21` | `{"error":"not-yet-implemented","phase":15}` | FAIL: same root cause — container not yet redeployed. Routed to human verification #1. |
| EnglishPlayer.vue chunk emits cleanly in production build | Confirmed by Plan 16-06 SUMMARY: `dist/assets/EnglishPlayer-DYqPGhIT.js + .css` | PASS (per SUMMARY); verified the source compiles + builds | PASS |

The two FAIL spot-checks are not gaps in Phase-16 deliverables — the code is correct and unit-tested. They reflect a pending redeploy gate, which is explicitly the orchestrator's responsibility per Plans 16-05 and 16-06 SUMMARY deviations.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|---|---|---|---|---|
| SCRAPER-PAHE-01 | 16-03, 16-05 | malsync.moe lookup with 24h cache + fuzzy-title fallback | SATISFIED | malsync.go Lookup + 24h positive/negative cache; client.go FindID jaroWinkler ≥ 0.85 (lines 219-291); 7 malsync tests green |
| SCRAPER-PAHE-02 | 16-03, 16-05 | ListEpisodes returns full episode list cached 6h | SATISFIED | client.go ListEpisodes paginates `/api?m=release` with 50-page cap; cache key `episodes:animepahe:{providerID}` TTL 6h; 5 ListEpisodes tests green |
| SCRAPER-PAHE-03 | 16-02, 16-03, 16-05 | HLS m3u8 via kwik.cx using dop251/goja; KwikExtractor registered | SATISFIED | embeds/kwik.go full impl; 9 Kwik tests green incl. SSRF, timeout, body-cap; registry.Register on main.go:45 |
| SCRAPER-PAHE-04 | 16-01, 16-03 | DDoS-Guard cookies via cookiejar + publicsuffix; no headless browser | SATISFIED | BaseHTTPClient.Jar() accessor (Plan 16-01); ensureDDoSCookie helper (Plan 16-03); prefix-match on `__ddg2_*` (CR-05 fix); idempotency test green |
| SCRAPER-PAHE-05 | 16-01 | AnimePahe CDN hostnames in HLSProxyAllowedDomains | SATISFIED | proxy.go:243-245 contains kwik.cx + owocdn.top + uwucdn.top; regression test locks them |
| SCRAPER-UI-01 | 16-06 | EnglishPlayer.vue replaces both HiAnime + Consumet (Video.js + HLS.js + SubtitleOverlay) | SATISFIED | EnglishPlayer.vue exists (1685 lines); Video.js/HLS.js engine + SubtitleOverlay reused; cyan accent |
| SCRAPER-UI-02 | 16-06 | One "English" tab; provider selection inside player via dropdown | SATISFIED | Anime.vue:360 single English tab; Source chip in EnglishPlayer.vue:156-170; preference persists via useWatchPreferences |
| SCRAPER-UI-03 | 16-04 | scraperApi exposes getEpisodes/getServers/getStream/getHealth; hianimeApi + consumetApi NOT repointed | SATISFIED | client.ts:416 scraperApi 4 methods; hiAnimeApi (:392) + consumetApi (:449) untouched |
| SCRAPER-UI-04 | 16-06 | Old HiAnime/Consumet kept reachable via ?legacy=1 dev flag | SATISFIED | Anime.vue:367 `v-if="$route.query.legacy === '1'"` reveals (debug)-tagged HiAnime + Consumet tab buttons; HiAnimePlayer.vue + ConsumetPlayer.vue unchanged |
| SCRAPER-NF-02 | 16-03, 16-05 | Cache TTLs: 24h malsync, 6h episodes, ≤ min(expires-30s, 5min) stream | SATISFIED | malsync.go 24h; client.go episodes 6h; cache.go computeStreamTTL; 8 sub-cases in TestComputeStreamTTL green |
| SCRAPER-NF-05 | 16-04, 16-05, 16-06 | ReportButton emits provider:<name> + tried[] chain | SATISFIED | Backend: handler.go always emits meta.tried; Frontend: triedChain captured from response, scraperProvider + triedChain threaded into ReportButton → diagnostics → Telegram payload (snake_case `scraper_provider` + `tried_chain`) |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| `services/scraper/internal/providers/animepahe/client.go` | 427-434 | Dead code: `hostnameOf` helper no longer referenced (WR-05 fix inlined `url.Parse(src).Hostname()`) | Info | None functional; linter `unused` rule could flag |
| `frontend/web/e2e/english-player.spec.ts` | 30, 64 | `FALLBACK_TEST_ANIME_ID` referenced only in an error message string template | Info | None functional; reader-clarity issue |
| `services/catalog/internal/handler/scraper_test.go` | 325-328 | Test stub builds URL via raw string concatenation; production code uses `url.Values{}.Encode()` | Info | Test-only; would miss encoding mismatches on special-char prefer values |
| `frontend/web/src/utils/diagnostics.ts` | 76-85 | `safeStringify` WeakSet replacer marks shared (non-cyclic) references as `[Circular]` | Warning | Diagnostics blob in user-submitted reports may show `[Circular]` for legitimate shared references (cosmetic; no correctness impact) — documented in iter-2 review as WR-12 |
| `frontend/web/src/components/player/EnglishPlayer.vue` | 824, 837 | Unsafe `as 'sub' \| 'dub'` cast silently narrows a `raw` Category value | Warning | Currently zero practical impact (AnimePahe never returns `data-audio="raw"`); future provider or schema change would resurrect CR-02 empty-list symptom — documented in iter-2 review as WR-13 |

None of the warnings are blockers. The two iter-2 WARNING items (WR-12, WR-13) are minor regressions of iter-1 fix intent, not Phase 16 contract violations.

### Human Verification Required

See frontmatter `human_verification` section. Six items require manual verification:

1. **Redeploy scraper container** — the running container is still serving Phase-15 503 stubs. `make redeploy-scraper` is needed to land the Phase-16 boot wiring. Verified by checking `/scraper/health.data.providers.animepahe` exists post-redeploy.

2. **End-to-end stream playback through the English tab** — visual + interactive verification against the deployed stack. Requires AnimePahe egress; if blocked, flip `ANIMEPAHE_BASE_URL=https://animepahe.com` via docker-compose env override.

3. **DDoS-Guard handshake against real upstream** — the cookie helper has unit-test coverage but has never run against the real DDoS-Guard challenge. First live `/scraper/episodes` call should populate Redis `malsync:*` keys + the BaseHTTPClient cookie jar with a `__ddg2_*` cookie.

4. **Legacy `?legacy=1` flag** — visual confirmation that the URL gate surfaces HiAnime (debug) + Consumet (debug) tabs.

5. **Playwright e2e spec live run** — `bunx playwright test english-player.spec.ts --reporter=list` against the live deployment. 4 tests, 12 instances; 1 may skip on empty-state.

6. **ReportButton modal Telegram receipt** — submit a test report from EnglishPlayer; verify Telegram admin chat receives it with `Provider: AnimePahe` + `Tried: animepahe` lines.

### Gaps Summary

**No code-level gaps found.** All 11 must-have truths verified; all artifacts exist and pass three-level inspection (exists, substantive, wired); all 11 phase requirements satisfied at the source-code level; data flows correctly through the wired pipeline.

The phase is **structurally complete**. What remains is the post-merge deploy gate — the running scraper container is the pre-Phase-16 build (verified by `/scraper/health` returning empty providers and `/scraper/episodes` returning the 503 stub). Plans 16-05 and 16-06 both explicitly deferred the live deploy to the orchestrator (the deploy operator) post-merge, with detailed recipes documented in their SUMMARYs.

All six human-verification items are pre-deploy expectations that gate the phase's **live runtime success**, not its **code-level achievement**. They are properly classified as `human_needed`, not `gaps_found`.

### Review Status Recap

- **Code Review iter-1:** 5 BLOCKER + 11 WARNING — all 16 fixed atomically (16-REVIEW-FIX.md, status: all_fixed)
- **Code Review iter-2:** Original 16 verified clean; 2 minor new WARNINGs (WR-12 safeStringify shared-ref edge case; WR-13 type-cast in EnglishPlayer affecting future providers only) and 3 INFOs (hostnameOf dead code; FALLBACK_TEST_ANIME_ID semi-dead; test stub URL concatenation). No BLOCKERs remain.

---

_Verified: 2026-05-12T05:52:18Z_
_Verifier: Claude (gsd-verifier)_
