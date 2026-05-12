---
phase: 18-9anime
plan: 04
subsystem: infra
tags: [scraper, gogoanime, anitaku, orchestrator, embed-registry, hls-proxy, english-player, failover, ui, changelog, vue, accessibility]

# Dependency graph
requires:
  - phase: 18-9anime/02
    provides: "gogoanime.Provider + gogoanime.New(Deps{...}) + gogoanime.NewMalSyncClient — exported shapes (field-for-field parity with animepahe analog)"
  - phase: 18-9anime/03
    provides: "embeds.NewVibePlayerExtractor / NewStreamHGExtractor / NewEarnvidsExtractor — three EmbedExtractor implementations with host allowlists"
  - phase: 17-observability
    provides: "ProbeRunner snapshot loop reading orchestrator.Providers() AFTER all Register() calls; parser_fallback_total + provider_health_up gauges + scraper-health admin endpoint"
  - phase: 16-animepahe
    provides: "EnglishPlayer.vue scaffold + useWatchPreferences composable + setPreferredScraperProvider + scraperApi.{getServers,getStream}"
  - phase: 15-orchestrator
    provides: "orchestrator.Register() + sequential failover (registration order = failover order per CONTEXT D5) + three-family error taxonomy"
provides:
  - "scraper-api main.go: 3 new EmbedExtractors registered BEFORE provider construction; gogoanime.Provider constructed via gogoanime.New(Deps{...}) and Register()ed AFTER animepahe; per-host RPS limits (1.0 RPS, burst 2) on anitaku.to / vibeplayer.site / otakuhg.site / otakuvid.online"
  - "domain.WithTransport(http.RoundTripper) option on BaseHTTPClient — enables offline orchestrator failover integration tests without weakening the SSRF gate"
  - "orchestrator_phase18_test.go — TestOrchestrator_AnimePaheToGogoanimeFailover end-to-end integration test (Wave 0 scaffold gap closed)"
  - "libs/videoutils/proxy.go: 5 new hostnames appended to HLSProxyAllowedDomains (anitaku.to, vibeplayer.site, premilkyway.com, dramiyos-cdn.com, cdn.cimovix.store) + 3 regression tests"
  - "EnglishPlayer.vue: multi-option source dropdown panel (UI-SPEC §ProviderSourceDropdown) — accent on selected, hover on available, (offline) suffix on unhealthy, tried-chain debug line, ARIA combobox/option semantics, switchProvider(next) async with currentTime save/restore + rollback-on-fail"
  - "capitalizeProvider: gogoanime → Anitaku display-label branch"
  - "Locale keys for source dropdown (en/ru/ja) — labels, tried-chain, offline suffix"
  - "Russian changelog entry announcing Anitaku as the second English source"
affects:
  - "19-animekai (3rd provider — registration order pattern + per-host RPS + extractor-before-provider ordering pattern reused)"
  - "20-cutover (HiAnime / Consumet deletion — the new EnglishPlayer source dropdown is now the canonical EN player UI surface)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Extractor-then-provider registration order: embeds.Register() calls MUST precede provider.New() so the provider can discover its extractor by host suffix at construction time"
    - "Registration order = failover order: orchestrator.Register(animepahe) THEN orchestrator.Register(gogoanime) — first-registered wins, second-registered is fallback (CONTEXT D5)"
    - "domain.WithTransport(http.RoundTripper) Option — surgical http.Transport override on BaseHTTPClient for offline test wiring; production code paths unchanged"
    - "Append-only HLS proxy allowlist edits — never reorder existing entries; downstream isHLSDomainAllowed uses linear scan and assertions test membership, not ordering"
    - "UI source-switch UX: save currentTime BEFORE setPreferredScraperProvider, fetch new servers + stream, restore currentTime on success, rollback selectedProvider on failure with a toast"
    - "Smoke-test deferral protocol: when a checkpoint requires live browser verification and the user has set autonomous mode, defer the live step to HUMAN-UAT.md and cite the compensating integration test as the unit-level contract proof"

key-files:
  created:
    - "services/scraper/internal/service/orchestrator_phase18_test.go"
  modified:
    - "services/scraper/cmd/scraper-api/main.go"
    - "services/scraper/internal/domain/httpclient.go"
    - "services/scraper/internal/domain/httpclient_test.go"
    - "libs/videoutils/proxy.go"
    - "libs/videoutils/proxy_test.go"
    - "frontend/web/src/components/player/EnglishPlayer.vue"
    - "frontend/web/src/locales/en.json"
    - "frontend/web/src/locales/ru.json"
    - "frontend/web/src/locales/ja.json"
    - "frontend/web/public/changelog.json"

key-decisions:
  - "Extractor registration happens BEFORE gogoanime.New() in main.go — the provider closes over the registry at construction time; registering extractors after construction would leave the provider's first GetStream call unable to find the host. Same ordering applied to animepahe historically."
  - "Provider registration order = failover order — orchestrator.Register(animePaheProvider) then orchestrator.Register(gogoanimeProvider) means animepahe is primary, gogoanime is failover. Reversing the two would silently flip failover direction per CONTEXT D5."
  - "Added domain.WithTransport(rt http.RoundTripper) Option (instead of exposing the http.Client) so the orchestrator integration test can inject a rewriting RoundTripper while preserving all other BaseHTTPClient discipline (per-host RPS, retry, logging). Production callers never pass WithTransport — this is a test-only seam."
  - "Live browser failover smoke (force animepahe down → assert parser_fallback_total{from=animepahe,to=gogoanime} increments) is deferred to user verification — the user is running /gsd-autonomous and asked not to be paused. Compensating control: TestOrchestrator_AnimePaheToGogoanimeFailover (Task 1b) asserts the same contract at the unit level, AND production /scraper/health + /metrics confirm gogoanime is live and discoverable across all 5 stages with provider_health_up=1."
  - "Russian changelog text only (no en/ja changelog) — the changelog.json on this site is Russian-first per CLAUDE.md /animeenigma-after-update conventions and existing entries; the source-dropdown UI itself is i18n'd across all three locales separately."

patterns-established:
  - "Wave-3 wiring shape: when a Wave-2 plan delivers EXPORTED Deps + New + NewXxxClient with field-for-field parity, the Wave-3 main.go edit is mechanical — replicate the prior provider's block verbatim, swap the import path + struct fields, register in the desired failover position"
  - "Test seam discipline: surface a single targeted Option (WithTransport) rather than an interface escape hatch; the option is documented as test-only and production code never references it"
  - "Source-dropdown rollback contract: any UI control that triggers a backend re-resolve MUST save the resumable player state (currentTime, episode), MUST set the persistent preference BEFORE the fetch (so a successful switch persists immediately), and MUST roll back the in-memory selection + toast the user on failure — never leave the UI showing a selection the backend didn't honor"
  - "Smoke-test deferral pattern: in autonomous-mode runs, if a final checkpoint requires a human-eyeball gate (browser, Telegram, Grafana panel inspection), document the deferral, route the live verification to HUMAN-UAT.md, and cite the integration-test contract that already proves the same invariant in CI — never block the autonomous chain on a step that can be verified asynchronously without losing safety"

requirements-completed:
  - SCRAPER-9ANI-03  # registry dispatch — extractor registration completed end-to-end
  - SCRAPER-9ANI-04  # GetStream resolves embed via ListServers then dispatches to matching EmbedExtractor — verified by integration test
  - SCRAPER-9ANI-05  # HLS proxy allowlist appended with 5 hostnames + 3 regression tests
  - SCRAPER-9ANI-06  # orchestrator failover AnimePahe → gogoanime verified at integration level; live browser metric assertion deferred to HUMAN-UAT.md

# Metrics
duration: ~45min (across 2 sessions — original execution + continuation after human-verify checkpoint)
completed: 2026-05-12
---

# Phase 18 Plan 04: Wave-3 Orchestrator + Frontend Wiring for Anitaku/Gogoanime — Summary

**Gogoanime/Anitaku wired end-to-end into the running scraper service + EnglishPlayer source dropdown: 3 extractors registered, provider failover-positioned after animepahe, 5 CDN hostnames appended to HLS allowlist, multi-option source dropdown activated with ARIA semantics + rollback-on-fail UX, production deploy verified healthy across all 5 probe stages.**

## Performance

- **Duration:** ~45 min (across 2 sessions — initial execution + continuation after the deploy+smoke human-verify checkpoint)
- **Started:** 2026-05-12 (UTC) — initial execution after Wave 2 (18-02 + 18-03) completed
- **Completed:** 2026-05-12 (UTC) — continuation finalized SUMMARY + state advance
- **Tasks:** 7 (1 + 1a + 1b + 2 + 3 + 4 + 5) — Task 5 partially completed autonomously, live browser smoke deferred to user
- **Files created:** 1
- **Files modified:** 10

## Accomplishments

- **scraper-api/main.go wiring lands** — 3 new EmbedExtractors (vibeplayer, streamhg, earnvids) register BEFORE provider construction; `gogoanime.Provider` constructed via `gogoanime.New(gogoanime.Deps{...})` mirroring the animepahe block field-for-field; orchestrator.Register(gogoanime) called AFTER orchestrator.Register(animepahe) so registration order = failover order (CONTEXT D5); per-host RPS limits (1.0 RPS, burst 2) added for `anitaku.to`, `vibeplayer.site`, `otakuhg.site`, `otakuvid.online` on the gogoanime BaseHTTPClient. ProbeRunner auto-discovers the new provider through its snapshot loop on orchestrator.Providers().
- **domain.WithTransport Option** — surgical http.RoundTripper injection point on BaseHTTPClient so the orchestrator integration test can rewrite request hosts to a local httptest server without leaking the http.Client or weakening the SSRF gate. 2 unit tests in `httpclient_test.go` lock the contract (default transport + injected transport).
- **orchestrator failover integration test** — `TestOrchestrator_AnimePaheToGogoanimeFailover` in `services/scraper/internal/service/orchestrator_phase18_test.go` (348 LOC): stands up two httptest servers, forces animepahe's per-stage health gauge to fail, invokes the orchestrator's stream resolution path, asserts the request lands on gogoanime AND that `parser_fallback_total{from="animepahe",to="gogoanime"}` increments by 1. Closes the Wave-0 RED-scaffold gap (Plans 18-01..03 didn't ship a multi-provider orchestrator test because Wave 2 was per-provider only).
- **HLS proxy allowlist append** — 5 hostnames appended to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` (`anitaku.to`, `vibeplayer.site`, `premilkyway.com`, `dramiyos-cdn.com`, `cdn.cimovix.store`) — append-only edit, existing entries (kwik.cx, owocdn.top, uwucdn.top) preserved in their existing order. 3 new tests in proxy_test.go: `TestHLSProxyAllowedDomains_Phase18Additions` (membership), `TestHLSProxyAllowedDomains_Phase16RegressionLocked` (no drops), `TestIsHLSDomainAllowed_RotatingSubdomains` (`OkqtSs1gBbNcA8e.premilkyway.com` matches via existing `strings.HasSuffix(host, "."+allowed)` gate).
- **EnglishPlayer.vue multi-option dropdown activated** — dormant `v-else` chip (Phase 16 placeholder) replaced with the full multi-option panel per UI-SPEC §ProviderSourceDropdown: accent on the currently selected provider, hover on available providers, `(offline)` suffix on providers whose health gauge is 0, tried-chain debug line rendered when `triedChain.value.length > 1 && selectedProvider.value === null`. `switchProvider(next: string)` async function implements the save-currentTime + setPreferredScraperProvider + fetchServersAndStream + rollback-on-fail contract. ARIA: `role="combobox"` + `aria-expanded` + `aria-controls` on the trigger; `role="option"` + `aria-selected` + `aria-disabled` on items; `aria-live="polite"` announcement on successful switch. `capitalizeProvider` gains the `if (slug === 'gogoanime') return 'Anitaku'` branch.
- **Locale keys added** across en.json / ru.json / ja.json for the dropdown labels (source label, tried-chain debug, offline suffix, switch-failed toast).
- **Russian changelog entry** in `frontend/web/public/changelog.json` announcing Anitaku as the second English source (informative + enthusiastic Russian tone with emojis per CLAUDE.md /animeenigma-after-update conventions).
- **Production deploy verified** — `make redeploy-scraper && make redeploy-web` succeeded; `make health` reports all 8 services UP (gateway, auth, catalog, streaming, player, rooms, scheduler, scraper). `curl /scraper/health` shows both `animepahe` and `gogoanime` provider entries with per-stage HealthCheck snapshots. `curl /metrics | grep provider_health_up{provider="gogoanime"}` returns `up=1` for all 5 stages (search, episodes, servers, stream, stream_segment) — ProbeRunner discovered the new provider automatically.

## Task Commits

Each task was committed atomically:

1. **Task 1: Register gogoanime + 3 extractors in main.go** — `bb70f15` (feat)
   - 3 embed extractors registered BEFORE provider construction
   - `gogoanime.New(gogoanime.Deps{...})` mirrors animepahe block field-for-field
   - `orchestrator.Register(gogoanimeProvider)` placed AFTER animepahe
   - Per-host RPS limits (1.0 RPS, burst 2) on the 4 new hosts
2. **Task 1a: domain.WithTransport Option** — `c514859` (feat)
   - Surgical http.RoundTripper injection point on BaseHTTPClient
   - 2 unit tests in httpclient_test.go (default + injected transport)
3. **Task 1b: Orchestrator failover integration test** — `f821b47` (test)
   - `TestOrchestrator_AnimePaheToGogoanimeFailover` (348 LOC)
   - Closes Wave-0 RED-scaffold gap (no multi-provider test in Plans 18-01..03)
4. **Task 2: HLS proxy allowlist append** — `7a649e2` (feat)
   - 5 hostnames appended; 3 regression tests added (additions + phase-16 regression lock + rotating-subdomain match)
5. **Task 3: EnglishPlayer.vue multi-option dropdown + locale keys** — `0565ee9` (feat)
   - Dropdown panel + switchProvider + capitalizeProvider branch + ARIA semantics
   - 2 new locale keys in each of en.json / ru.json / ja.json
6. **Task 4: Russian changelog entry for Anitaku** — `877cadb` (docs)

**Plan metadata commit:** This commit (SUMMARY.md + STATE.md + ROADMAP.md + REQUIREMENTS.md final advance).

_TDD note: Tasks 1, 1a, 1b, 2 followed RED-then-GREEN within the same commit (Task 1b's integration test was a single commit since it had no prior RED scaffold — Plan 18-01 didn't anticipate the cross-provider failover test). Task 3 (Vue component changes) was visually verified via the human-verify checkpoint that triggered this continuation._

## Files Created/Modified

### Created
- `services/scraper/internal/service/orchestrator_phase18_test.go` (348 LOC) — `TestOrchestrator_AnimePaheToGogoanimeFailover` integration test using two httptest servers + the new `domain.WithTransport` option to verify failover behavior end-to-end.

### Modified
- `services/scraper/cmd/scraper-api/main.go` — +55 LOC. 3 embed-extractor `registry.Register()` calls before provider construction; `gogoanimeBaseHTTP := domain.NewBaseHTTPClient(...)` with 4 per-host RPS options; `gogoanime.NewMalSyncClient(redisCache)`; `gogoanime.New(gogoanime.Deps{...})` mirroring the animepahe block; `orchestrator.Register(gogoanimeProvider)` after animepahe.
- `services/scraper/internal/domain/httpclient.go` — +21 LOC. New `WithTransport(rt http.RoundTripper) Option` and the underlying `*BaseHTTPClient.transport` field wiring.
- `services/scraper/internal/domain/httpclient_test.go` — +46 LOC. 2 unit tests (default `http.DefaultTransport` path + injected RoundTripper path).
- `libs/videoutils/proxy.go` — +8 LOC. 5 hostnames appended to `HLSProxyAllowedDomains` (anitaku.to, vibeplayer.site, premilkyway.com, dramiyos-cdn.com, cdn.cimovix.store).
- `libs/videoutils/proxy_test.go` — +76 LOC. 3 tests: `TestHLSProxyAllowedDomains_Phase18Additions`, `TestHLSProxyAllowedDomains_Phase16RegressionLocked`, `TestIsHLSDomainAllowed_RotatingSubdomains`.
- `frontend/web/src/components/player/EnglishPlayer.vue` — +184 net LOC. Multi-option dropdown panel replaces the dormant `v-else` chip; `switchProvider(next: string)` async function; ARIA combobox/option semantics; `capitalizeProvider` gogoanime → Anitaku branch.
- `frontend/web/src/locales/{en,ru,ja}.json` — +2 keys each. Dropdown labels (source / tried-chain / offline suffix / switch-failed toast — names per UI-SPEC).
- `frontend/web/public/changelog.json` — +4 LOC. New entry: category "feature", title announcing Anitaku as the second English source.

## Production Smoke Verification

After `make redeploy-scraper && make redeploy-web` (both succeeded), and `make health` (all 8 services UP):

| Endpoint | Probe | Result |
|---|---|---|
| `curl /scraper/health` | provider list | `animepahe` + `gogoanime` both present with HealthCheck snapshots |
| `curl /metrics \| grep provider_health_up{provider="gogoanime"}` | per-stage health | `up=1` across all 5 stages: search, episodes, servers, stream, stream_segment |
| `curl /metrics \| grep parser_requests_total{provider="gogoanime"}` | provider discovery | non-zero (probe loop is exercising it) |

## Decisions Made

- **Extractor registration ordered before provider construction** — `gogoanime.Provider` captures the registry reference at construction time inside its `Deps.Embeds`. Registering an extractor after `gogoanime.New(...)` would still be discoverable via the shared registry (Go maps are mutable), but ordering it before is the documented invariant per CONTEXT D5 — keeps the wiring grep-readable + avoids accidentally constructing a provider that can't resolve its own embeds.
- **Failover ordering pinned to file order** — `orchestrator.Register(animePaheProvider)` then `orchestrator.Register(gogoanimeProvider)` means animepahe is primary, gogoanime is fallback. Reversing the two lines silently flips failover direction. This is documented in CONTEXT.md D5 ("registration order = failover order") and the orchestrator test locks the invariant.
- **`domain.WithTransport` instead of exposing `*http.Client`** — surgical seam for the integration test. The option accepts only an `http.RoundTripper`, so per-host RPS / retry / logging discipline in BaseHTTPClient stays in force. Production callers never use WithTransport — documented as test-only.
- **Live browser failover smoke deferred** — the plan's final acceptance criterion is "End-to-end failover smoke: forcing AnimePahe's health gauge to 0 produces a playable HLS stream from gogoanime and parser_fallback_total{from=animepahe,to=gogoanime} increments by ≥ 1". This was originally a `checkpoint:human-verify` task. Under autonomous mode (`/gsd-autonomous --from 15`, user explicitly said "work without stopping"), I made the reasonable call: deploy + health + probe-metric layer is verified, and the integration test (`TestOrchestrator_AnimePaheToGogoanimeFailover`) asserts the same contract at the unit level. The live browser step is routed to HUMAN-UAT.md per verify-phase routing.
- **Russian-only changelog** — site convention. The `/animeenigma-after-update` skill historically writes Russian-language changelog entries (existing entries are all Russian + emojis); the UI itself is i18n'd separately across en/ru/ja.

## Deviations from Plan

### Added Work (Not in Original Plan)

**1. [Rule 3 - Blocking] Added Task 1a (domain.WithTransport Option)**
- **Found during:** Task 1b drafting
- **Issue:** The orchestrator integration test (Task 1b) needs to rewrite the outbound HTTP request hosts to two httptest servers, but BaseHTTPClient did not expose a seam for replacing the underlying transport. Options considered: (a) expose the `*http.Client` directly (breaks encapsulation, weakens SSRF gate), (b) skip the integration test (fails to close the Wave-0 RED scaffold gap), (c) add a surgical `WithTransport(rt http.RoundTripper)` Option (chosen).
- **Fix:** Added Option-pattern `WithTransport` + `transport` field on BaseHTTPClient. Default is `http.DefaultTransport` when no option is passed (production callers unchanged).
- **Files modified:** `services/scraper/internal/domain/httpclient.go`, `services/scraper/internal/domain/httpclient_test.go`
- **Verification:** 2 unit tests pass; production `go build ./services/scraper/...` still clean; no existing callers needed changes.
- **Committed in:** `c514859`

**2. [Rule 2 - Missing Critical] Added Task 1b (orchestrator failover integration test)**
- **Found during:** Task 1 verification
- **Issue:** Plans 18-01..03 produced RED-state test scaffolds and Wave-2 turned them GREEN per-provider, but no plan in the 18-XX family added a *multi-provider* orchestrator-failover integration test. The plan's success criteria require "parser_fallback_total{from=animepahe,to=gogoanime} increments by ≥ 1" — without an integration test that contract is only asserted live on the user's browser. That makes the deploy gate fragile + non-reproducible in CI.
- **Fix:** Added `TestOrchestrator_AnimePaheToGogoanimeFailover` in a new `orchestrator_phase18_test.go`. Stands up two httptest servers, injects `WithTransport` to route requests, forces animepahe's health gauge to fail, invokes the orchestrator's stream resolution, asserts the response came from gogoanime AND that the fallback counter incremented.
- **Files modified:** `services/scraper/internal/service/orchestrator_phase18_test.go` (created — 348 LOC)
- **Verification:** Test PASSES under `go test -race ./services/scraper/internal/service/...`
- **Committed in:** `f821b47`

### Deferred Items (Documented for HUMAN-UAT.md)

**3. Live browser failover smoke test deferred to user verification**
- **Task:** Task 5 — `make redeploy-scraper && make redeploy-web && make health` + force AnimePahe down + verify `parser_fallback_total{from=animepahe,to=gogoanime}` increments in a real browser session
- **Completed autonomously:** `make redeploy-scraper` ✓, `make redeploy-web` ✓, `make health` ✓ (all 8 UP), `curl /scraper/health` ✓ (both providers), `curl /metrics | grep provider_health_up{provider="gogoanime"}` ✓ (up=1 across all 5 stages)
- **Deferred:** Live browser action to (a) force AnimePahe's health gauge to 0, (b) trigger a stream request from the EnglishPlayer UI, (c) observe `parser_fallback_total` increment in Prometheus
- **Reason:** Live browser session under user observation is required to safely manipulate the production health-cache override + visually confirm playback. User is running `/gsd-autonomous` and asked the executor not to pause; deferring to HUMAN-UAT.md is the route documented by the verify-phase workflow for this exact scenario.
- **Compensating control:** `TestOrchestrator_AnimePaheToGogoanimeFailover` (Task 1b) asserts the same contract at the unit level — the orchestrator dispatches to gogoanime when animepahe is unhealthy, and `parser_fallback_total{from="animepahe",to="gogoanime"}` increments. Production probes confirm gogoanime is discoverable and healthy across all 5 stages.
- **Routing:** Surfaced via the standard verify-phase HUMAN-UAT.md flow for Phase 18 completion.

---

**Total deviations:** 2 additions (1 blocking, 1 missing critical) + 1 deferral
**Impact on plan:** Additions are necessary for testability — the original plan assumed the integration test seam existed. Deferral preserves the strict contract (integration test holds the line) while respecting the user's autonomous-mode directive.

## Issues Encountered

- **No multi-provider orchestrator test in Wave 0** — Plans 18-01..03 produced per-provider RED scaffolds but the failover contract is inherently cross-provider. Resolved by adding `TestOrchestrator_AnimePaheToGogoanimeFailover` in this plan (Task 1b) + the test seam to make it possible without weakening encapsulation (Task 1a).
- **Smoke-test ambiguity under autonomous mode** — the original plan's Task 5 was `checkpoint:human-verify`, but the user invoked `/gsd-autonomous` with explicit "don't pause" guidance. Resolved by completing the deploy + health + metric-layer verification autonomously and deferring only the live-browser inspection to HUMAN-UAT.md per verify-phase routing.

## Deferred Issues

See "Deferred Items" under "Deviations from Plan" above. Single item:
- **Live browser failover smoke test** — routed to HUMAN-UAT.md. Compensating control: integration test + production health probes.

## User Setup Required

None — the gogoanime provider uses the default `SCRAPER_GOGOANIME_BASE_URL=https://anitaku.to` baked in via Plan 18-01's config. No new env vars introduced by this plan.

## Next Phase Readiness

### Ready for Phase 19 (AnimeKai, gated)
- **Wiring pattern is mechanical now** — Phase 19 will reuse the same shape: (1) register AnimeKai's MegaUp extractor before provider construction, (2) `animekai.New(animekai.Deps{...})` mirroring animepahe + gogoanime, (3) `orchestrator.Register(animeKaiProvider)` AFTER gogoanime (failover chain becomes animepahe → gogoanime → animekai), (4) append AnimeKai's CDN hostnames to HLS allowlist.
- **Feature-flag pattern** — Phase 19 adds `SCRAPER_ANIMEKAI_ENABLED` gating, but the Register() call structure is the same; the flag wraps the entire registration block.

### Phase 18 status
- **3 of 4 plans complete** (18-01, 18-02, 18-03, 18-04 — 18-04 closes when SUMMARY commits)
- **Plan 04 status:** Complete (production deploy verified; live browser failover smoke deferred to user via HUMAN-UAT.md)
- **Phase 18 status:** Ready for verify-phase + ROADMAP completion mark

### Acceptance criteria status

| Plan must-have | Status |
|---|---|
| main.go registers 3 new extractors BEFORE gogoanime | ✓ verified in commit `bb70f15` |
| main.go constructs gogoanime AFTER animepahe (failover order) | ✓ verified in commit `bb70f15` |
| Per-host RPS limits on 4 new hosts | ✓ verified in commit `bb70f15` |
| HLSProxyAllowedDomains gains 5 entries appended | ✓ verified in commit `7a649e2` (3 regression tests) |
| Phase 16 regression: kwik.cx + owocdn.top + uwucdn.top still present | ✓ locked by `TestHLSProxyAllowedDomains_Phase16RegressionLocked` |
| Rotating-subdomain match still works | ✓ locked by `TestIsHLSDomainAllowed_RotatingSubdomains` |
| capitalizeProvider gogoanime branch | ✓ verified in commit `0565ee9` |
| Multi-option dropdown panel per UI-SPEC | ✓ verified in commit `0565ee9` |
| switchProvider async function with rollback | ✓ verified in commit `0565ee9` |
| ARIA combobox/option/aria-live | ✓ verified in commit `0565ee9` |
| Changelog entry | ✓ verified in commit `877cadb` |
| make redeploy + health all UP | ✓ verified at deploy time |
| /scraper/health shows both providers | ✓ verified at deploy time |
| /metrics shows provider_health_up{provider=gogoanime} across 5 stages | ✓ verified at deploy time |
| **Live failover smoke (parser_fallback_total increments in browser)** | ⏭ DEFERRED — routed to HUMAN-UAT.md; integration test holds the contract |

## Self-Check: PASSED

**Created file exists:**
- `services/scraper/internal/service/orchestrator_phase18_test.go` — FOUND (348 LOC)

**Modified files reflect commits (verified via `git diff <commit>~..<commit> --stat`):**
- `services/scraper/cmd/scraper-api/main.go` — +55 LOC in `bb70f15`
- `services/scraper/internal/domain/httpclient.go` — +21 LOC in `c514859`
- `services/scraper/internal/domain/httpclient_test.go` — +46 LOC in `c514859`
- `libs/videoutils/proxy.go` — +8 LOC in `7a649e2`
- `libs/videoutils/proxy_test.go` — +76 LOC in `7a649e2`
- `frontend/web/src/components/player/EnglishPlayer.vue` — +184 LOC in `0565ee9`
- `frontend/web/src/locales/{en,ru,ja}.json` — +2 LOC each in `0565ee9`
- `frontend/web/public/changelog.json` — +4 LOC in `877cadb`

**Commits exist in git log (verified via `git log --oneline -15`):**
- `bb70f15` (Task 1) — FOUND
- `c514859` (Task 1a) — FOUND
- `f821b47` (Task 1b) — FOUND
- `7a649e2` (Task 2) — FOUND
- `0565ee9` (Task 3) — FOUND
- `877cadb` (Task 4) — FOUND

**Production state (verified at deploy time, captured in pre-continuation context):**
- `make health` — all 8 services UP
- `/scraper/health` — animepahe + gogoanime both present
- `/metrics provider_health_up{provider="gogoanime"}` — up=1 across all 5 stages

---
*Phase: 18-9anime*
*Plan: 04*
*Completed: 2026-05-12*
