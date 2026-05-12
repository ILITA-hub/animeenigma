---
phase: 19-animekai-gated
verified: 2026-05-12T19:55:00Z
status: passed
score: 7/7 must-haves verified
overrides_applied: 0
deferred:
  - truth: "SCRAPER-KAI-01: AnimeKai client resolves Shikimori/MAL ID to AnimeKai slug via malsync.moe"
    addressed_in: "v3.1 milestone"
    evidence: "ROADMAP Phase 19 success criterion 5 (escape hatch); REQUIREMENTS.md row: 'SCRAPER-KAI-01 | Phase 19 → v3.1 | Carry — escape hatch'"
  - truth: "SCRAPER-KAI-02: ListEpisodes returns AnimeKai episode list (sub/dub split)"
    addressed_in: "v3.1 milestone"
    evidence: "ROADMAP Phase 19 success criterion 5 (escape hatch); REQUIREMENTS.md row: 'SCRAPER-KAI-02 | Phase 19 → v3.1 | Carry — escape hatch'"
  - truth: "SCRAPER-KAI-03: ListServers enumerates AnimeKai's embed hosts"
    addressed_in: "v3.1 milestone"
    evidence: "ROADMAP Phase 19 success criterion 5 (escape hatch); REQUIREMENTS.md row: 'SCRAPER-KAI-03 | Phase 19 → v3.1 | Carry — escape hatch'"
  - truth: "SCRAPER-KAI-04: In-house MegaUp token generation in megacloud-extractor sidecar"
    addressed_in: "v3.1 milestone"
    evidence: "ROADMAP Phase 19 success criterion 5 (escape hatch); REQUIREMENTS.md row: 'SCRAPER-KAI-04 | Phase 19 → v3.1 | Carry — escape hatch'; sidecar route exists as HTTP 501 stub"
  - truth: "SCRAPER-KAI-07: End-to-end AnimePahe → 9anime → AnimeKai failover verified"
    addressed_in: "v3.1 milestone"
    evidence: "ROADMAP Phase 19 success criterion 5 (escape hatch); REQUIREMENTS.md row: 'SCRAPER-KAI-07 | Phase 19 → v3.1 | Carry — blocked on KAI-01..04'"
---

# Phase 19: AnimeKai (gated) Verification Report

**Phase Goal:** Ship feature-flag-gated AnimeKai third-provider scaffold (default off) with sidecar `/animekai-token` returning 501; document SCRAPER-KAI-01..04 + KAI-07 as v3.1 carryover; ensure Phase 20 cutover is unblocked.
**Verified:** 2026-05-12T19:55:00Z
**Status:** passed
**Re-verification:** No — initial verification
**Strategy:** ESCAPE HATCH PATH (R&D non-convergence per ROADMAP Phase 19 success criterion 5; AnimeKai officially announced shutdown 2026-05-10)

## Goal Achievement

### Observable Truths

| #   | Truth                                                                                                                                                                                       | Status     | Evidence                                                                                                                                                                                                                                                            |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | With `SCRAPER_ANIMEKAI_ENABLED` unset/false, scraper boots and registers exactly TWO providers: animepahe + gogoanime (animekai NOT registered).                                            | VERIFIED   | Live `GET /scraper/health` returned `{"providers":{"animepahe":{...},"gogoanime":{...}}}` — no animekai key. `/metrics provider_health_up` series contain ONLY animepahe + gogoanime labels (no animekai labels emitted).                                            |
| 2   | With `SCRAPER_ANIMEKAI_ENABLED=true`, scraper boots and registers THREE providers; animekai's Provider methods wrap `domain.ErrProviderDown` so orchestrator fails over to gogoanime.       | VERIFIED   | Conditional `if cfg.AnimeKai.Enabled` block in `main.go:164-191`; all four methods in `client.go:207-233` wrap `errAnimeKaiStub` via `domain.WrapProviderDown`. Tests `TestProvider_{FindID,ListEpisodes,ListServers,GetStream}_StubReturnsErrProviderDown` all PASS. |
| 3   | `POST /animekai-token` on megacloud-extractor sidecar returns HTTP 501 with JSON body referencing v3.1 carryover.                                                                            | VERIFIED   | Live `curl -X POST http://localhost:3200/animekai-token` returned `HTTP 501` + body `{"error":"AnimeKai sidecar not yet converged — carry to v3.1"}`. Route guard `req.method === "POST"` at server.js:251.                                                          |
| 4   | `grep -r 'enc-dec.app' services/ docker/megacloud-extractor/` returns zero matches.                                                                                                          | VERIFIED   | Live grep run: zero matches. No external decryption dependency leaked into the new code. ROADMAP success criterion 2 trivially satisfied.                                                                                                                            |
| 5   | REQUIREMENTS.md marks KAI-01..04 + KAI-07 as "Phase 19 → v3.1 / Carry — escape hatch"; Implementation note block prepended above SCRAPER-KAI-01.                                            | VERIFIED   | Lines 173-179 of REQUIREMENTS.md show the 5 expected `Phase 19 → v3.1` rows (KAI-01..04 + KAI-07); line 81 contains the Implementation-note block dated 2026-05-12. KAI-05/06 marked `Done`.                                                                          |
| 6   | docker/.env.example documents `SCRAPER_ANIMEKAI_ENABLED` with default-off semantics + v3.1 caveat; docker-compose.yml plumbs the env var via `${SCRAPER_ANIMEKAI_ENABLED:-false}`.            | VERIFIED   | docker-compose.yml:168 `SCRAPER_ANIMEKAI_ENABLED: ${SCRAPER_ANIMEKAI_ENABLED:-false}`; docker/.env.example:100-105 contain the Phase 19 documentation block with toggle instructions, default-off line, mirror override.                                              |
| 7   | Wiring invariant in main.go fatals at boot if RegisteredProviders count != 2 (flag off) or != 3 (flag on).                                                                                    | VERIFIED   | main.go:276-292: `expectedProviders := 2; if cfg.AnimeKai.Enabled { expectedProviders = 3 }` then `log.Fatalw` if count diverges, including `registered` slice of names (WR-05 fix). Live deploy proves the flag-off branch fires correctly (2 providers).            |

**Score:** 7/7 truths verified

### Deferred Items

Items not yet met but explicitly addressed in later milestone phases (escape-hatch carryover to v3.1, per ROADMAP Phase 19 success criterion 5).

| # | Item                                                                              | Addressed In  | Evidence                                                                                                |
| - | --------------------------------------------------------------------------------- | ------------- | ------------------------------------------------------------------------------------------------------- |
| 1 | SCRAPER-KAI-01: malsync.moe Shikimori/MAL → AnimeKai slug resolution             | v3.1 milestone | REQUIREMENTS.md row: `SCRAPER-KAI-01 \| Phase 19 → v3.1 \| Carry — escape hatch`                       |
| 2 | SCRAPER-KAI-02: ListEpisodes against AnimeKai's `aitem-wrapper`/`alist-group` markup | v3.1 milestone | REQUIREMENTS.md row: `SCRAPER-KAI-02 \| Phase 19 → v3.1 \| Carry — escape hatch`                       |
| 3 | SCRAPER-KAI-03: ListServers enumerates AnimeKai's MegaUp embed hosts             | v3.1 milestone | REQUIREMENTS.md row: `SCRAPER-KAI-03 \| Phase 19 → v3.1 \| Carry — escape hatch`                       |
| 4 | SCRAPER-KAI-04: In-house MegaUp token generation in sidecar (`/animekai-token` body) | v3.1 milestone | REQUIREMENTS.md row: `SCRAPER-KAI-04 \| Phase 19 → v3.1 \| Carry — escape hatch`; route exists as 501 stub |
| 5 | SCRAPER-KAI-07: End-to-end failover verification with flag on                    | v3.1 milestone | REQUIREMENTS.md row: `SCRAPER-KAI-07 \| Phase 19 → v3.1 \| Carry — blocked on KAI-01..04`              |

### Required Artifacts

| Artifact                                                              | Expected                                                                                  | Status     | Details                                                                                                                                                              |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `services/scraper/internal/providers/animekai/doc.go`                 | Package docstring with escape-hatch rationale + SCRAPER-KAI-01..07 traceability         | VERIFIED   | 1615 bytes; `package animekai` present                                                                                                                              |
| `services/scraper/internal/providers/animekai/client.go`              | Stub Provider; every method wraps `errAnimeKaiStub` via `domain.WrapProviderDown`        | VERIFIED   | 9484 bytes; 4× `domain.WrapProviderDown` calls (one per method); 7× `errAnimeKaiStub` usages; `var _ domain.Provider = (*Provider)(nil)` present at line 238           |
| `services/scraper/internal/providers/animekai/dto.go`                 | DTO scaffold (placeholder shapes)                                                          | VERIFIED   | 1918 bytes; `package animekai` present                                                                                                                              |
| `services/scraper/internal/providers/animekai/client_test.go`         | Unit tests proving all Provider methods return wrapped `domain.ErrProviderDown`          | VERIFIED   | 8569 bytes; 9 tests PASS (Name, FindID/ListEpisodes/ListServers/GetStream stub return, HealthCheck stages-down, StageNames match AllStages, New RequiresAllDeps, interface conformance) |
| `services/scraper/internal/providers/animekai/helpers_test.go`        | fakeCache test helper lifted verbatim from gogoanime                                       | VERIFIED   | 2924 bytes; `type fakeCache struct` + `var _ cache.Cache = (*fakeCache)(nil)` present                                                                               |
| `services/scraper/internal/config/config.go`                          | AnimeKaiConfig struct + getEnvBool helper + env binding + URL validation                  | VERIFIED   | `AnimeKaiConfig` declared (line 82); `getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false)` at line 118; URL validation block at lines 150-156; `getEnvBool` helper at 194 |
| `services/scraper/internal/config/config_test.go`                     | Tests covering default-off, override, invalid URL, getEnvBool edge cases                  | VERIFIED   | 8 new tests PASS: TestLoad_AnimeKai{Defaults,EnabledTrue,EnabledFalseExplicit,EnabledInvalid,BaseURLOverride,InvalidBaseURL}, TestGetEnvBool_{Truthy,Falsy,LogsOnUnparseable} |
| `services/scraper/cmd/scraper-api/main.go`                            | Conditional registration block + Phase 19 wiring invariant + animekai import              | VERIFIED   | `import .../animekai` at line 19; conditional `if cfg.AnimeKai.Enabled { animekai.New(...) }` block at lines 164-191; `expectedProviders` invariant at 276-292; CR-01 `bootHealthSeedValue` helper at 346 |
| `docker/megacloud-extractor/server.js`                                | POST `/animekai-token` route returning HTTP 501 with carry-to-v3.1 body                  | VERIFIED   | Route at line 251; `writeHead(501, ...)` at line 260; WR-02 drain-body fix applied                                                                                  |
| `docker/docker-compose.yml`                                           | scraper env block extended with `SCRAPER_ANIMEKAI_ENABLED` + `SCRAPER_ANIMEKAI_BASE_URL` | VERIFIED   | Lines 168-169: `${SCRAPER_ANIMEKAI_ENABLED:-false}` + `${SCRAPER_ANIMEKAI_BASE_URL:-https://anikai.to}`                                                              |
| `docker/.env.example`                                                 | Phase 19 documentation block (defaults, toggle instructions, v3.1 caveat)                 | VERIFIED   | Lines 100-105: documentation block + commented `SCRAPER_ANIMEKAI_ENABLED=false` default + `SCRAPER_ANIMEKAI_BASE_URL=https://anikai.to` override comment            |
| `.planning/REQUIREMENTS.md`                                           | Carryover annotation + status table updates                                              | VERIFIED   | Line 81 Implementation-note block; lines 173-179 status table with KAI-01..04+07 → "Phase 19 → v3.1 / Carry — escape hatch"; KAI-05/06 → "Done"; line 113 Future-Requirements bullet |

### Key Link Verification

| From                                       | To                                                  | Via                                                          | Status | Details                                                                                                                                                                         |
| ------------------------------------------ | --------------------------------------------------- | ------------------------------------------------------------ | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/scraper-api/main.go`                  | `internal/providers/animekai`                       | import + conditional `orchestrator.Register(animeKaiProvider)` under `cfg.AnimeKai.Enabled` | WIRED  | Import at main.go:19; conditional block calls `animekai.New(Deps{...})` then `orchestrator.Register(animeKaiProvider)` at line 187 with proper boot-summary log                  |
| `internal/config/config.go`                | `SCRAPER_ANIMEKAI_ENABLED` env var                  | `getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false)`              | WIRED  | Read at config.go:118; default false; main.go reads `cfg.AnimeKai.Enabled` at boot. Tests prove unset/false/invalid all resolve to false; true/1/t/True/TRUE all resolve to true |
| `internal/providers/animekai/client.go`    | `domain.ErrProviderDown`                            | `domain.WrapProviderDown(errAnimeKaiStub, ...)` in every method | WIRED  | 4 invocations (one per Provider method, lines 208, 215, 222, 230). Tests assert `errors.Is(err, domain.ErrProviderDown)` is true on every return path.                          |
| `docker/docker-compose.yml` scraper svc    | `docker/.env.example`                               | `${SCRAPER_ANIMEKAI_ENABLED:-false}` shell-style sourcing    | WIRED  | docker-compose.yml line 168 references the env var; .env.example documents the same key with default value comment.                                                              |
| `.planning/REQUIREMENTS.md` status table   | v3.1 carryover                                      | "Phase 19 → v3.1" label on KAI-01..04 + KAI-07 rows          | WIRED  | `grep -c "Phase 19 → v3.1"` returns 5 (matches expected). KAI-05/06 explicitly NOT carried (marked "Done").                                                                      |

### Data-Flow Trace (Level 4)

Not applicable — Phase 19 is an escape-hatch scaffold. The provider's data path is intentionally short-circuited: every Provider method returns wrapped `ErrProviderDown` before any upstream HTTP call. There is no dynamic data to trace because the stub deliberately produces no data. This is the contract, verified by the test suite.

### Behavioral Spot-Checks

| Behavior                                                              | Command                                                                  | Result                                                                                                                              | Status |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- | ------ |
| Scraper module builds clean                                            | `go build ./services/scraper/...`                                        | Exit 0, no output                                                                                                                   | PASS   |
| Animekai package tests pass                                            | `go test ./services/scraper/internal/providers/animekai/... -count=1`    | 9 tests PASS (TestProvider_{Name,FindID,ListEpisodes,ListServers,GetStream}_StubReturnsErrProviderDown, HealthCheck, StageNames, New, ConformsToInterface) | PASS   |
| Config tests pass (AnimeKaiConfig + getEnvBool)                        | `go test ./services/scraper/internal/config/... -count=1`                | All PASS (incl. TestLoad_AnimeKai* × 6 + TestGetEnvBool_* × 3)                                                                       | PASS   |
| No regression in full scraper test suite                               | `go test ./services/scraper/... -count=1 -timeout=180s`                  | 14 packages: all PASS (cmd/scraper-api, config, domain, embeds, fuzzy, golint, handler, health, animekai, animepahe, gogoanime, service, testharness, transport) | PASS   |
| Live flag-off boot: registry = 2 providers (animepahe + gogoanime)     | `curl -s http://localhost:8088/scraper/health`                           | Returns exactly 2 providers; animekai NOT present                                                                                  | PASS   |
| Live `/metrics` shows no animekai labels                               | `curl -s http://localhost:8088/metrics \| grep provider_health_up`       | Only animepahe + gogoanime label tuples (5 stages each = 10 rows); zero animekai rows                                              | PASS   |
| Live sidecar `POST /animekai-token` returns HTTP 501                   | `curl -s -X POST http://localhost:3200/animekai-token -w "\nHTTP %{http_code}\n"` | `HTTP 501` + `{"error":"AnimeKai sidecar not yet converged — carry to v3.1"}`                                              | PASS   |
| All 8 services healthy                                                 | `make health`                                                            | gateway, auth, catalog, streaming, player, rooms, scheduler, scraper — all UP                                                       | PASS   |
| No `enc-dec.app` leakage                                               | `grep -r "enc-dec.app" services/ docker/megacloud-extractor/`            | Zero matches                                                                                                                       | PASS   |

### Requirements Coverage

| Requirement      | Source Plan        | Description                                                                                              | Status                  | Evidence                                                                                                                          |
| ---------------- | ------------------ | -------------------------------------------------------------------------------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| SCRAPER-KAI-01   | 19-01-PLAN.md      | malsync.moe Shikimori/MAL → AnimeKai slug resolution                                                    | DEFERRED to v3.1        | REQUIREMENTS.md row 173: "Phase 19 → v3.1 \| Carry — escape hatch". Phase 19 ships stub `FindID` that returns wrapped ErrProviderDown. |
| SCRAPER-KAI-02   | 19-01-PLAN.md      | ListEpisodes returns episode list from AnimeKai markup                                                  | DEFERRED to v3.1        | REQUIREMENTS.md row 174: "Phase 19 → v3.1 \| Carry — escape hatch". Phase 19 ships stub `ListEpisodes` returning ErrProviderDown.   |
| SCRAPER-KAI-03   | 19-01-PLAN.md      | ListServers enumerates AnimeKai's embed hosts                                                            | DEFERRED to v3.1        | REQUIREMENTS.md row 175: "Phase 19 → v3.1 \| Carry — escape hatch". Phase 19 ships stub `ListServers` returning ErrProviderDown.    |
| SCRAPER-KAI-04   | 19-01-PLAN.md      | In-house MegaUp token via sidecar — NO `enc-dec.app` dependency                                          | DEFERRED to v3.1 (partial: route stubbed) | REQUIREMENTS.md row 176: "Phase 19 → v3.1 \| Carry — escape hatch". Phase 19 ships HTTP 501 stub route; zero `enc-dec.app` leakage verified. |
| SCRAPER-KAI-05   | 19-01-PLAN.md      | Feature flag `SCRAPER_ANIMEKAI_ENABLED`, default off, toggleable via docker restart                       | SATISFIED               | config.go:118 reads flag; docker-compose.yml:168 plumbs through; .env.example:100-105 documents; main.go:164 gates registration. REQUIREMENTS.md row 177 marks Done. |
| SCRAPER-KAI-06   | 19-01-PLAN.md      | If R&D doesn't converge, flag default-off + KAI-01..04 carry to v3.1; rest of v3.0 ships                | SATISFIED               | Escape hatch taken explicitly. REQUIREMENTS.md row 178 marks Done; row 81 Implementation-note documents the decision; v3.0 milestone unblocked. |
| SCRAPER-KAI-07   | 19-01-PLAN.md      | Failover chain AnimePahe → 9anime → AnimeKai verified end-to-end with flag on                            | DEFERRED to v3.1        | REQUIREMENTS.md row 179: "Phase 19 → v3.1 \| Carry — blocked on KAI-01..04". Blocked by carryover deps.                          |

**Coverage summary:** 2 SATISFIED (KAI-05, KAI-06), 5 DEFERRED to v3.1 per ROADMAP Phase 19 success criterion 5 (escape hatch). No orphans. The 5 deferrals are explicit per the escape-hatch contract, NOT gaps.

### Anti-Patterns Found

None blocking. The 4 INFO findings from 19-REVIEW.md (IN-01 SSRF in pre-existing `/extract` endpoint, IN-02 unused DTOs, IN-03 fragile test-count comment, IN-04 outdated comment about 501-from-sidecar mechanism) were intentionally out of scope for the review-fix pass and remain as known minor items. All 1 CRITICAL + 5 WARNING findings from 19-REVIEW.md were fixed in 19-REVIEW-FIX.md (commits 840181b, 1c66249, d0ffd46, 0da36be, ab29688, 07e2e30).

The CR-01 fix is independently verified in production: `curl /metrics | grep 'provider_health_up.*animekai'` returns ZERO matches — Grafana cannot show a green panel for animekai because no metric series exists with that label while the flag is off, and when the flag is on `bootHealthSeedValue("animekai") == 0` enforces the escape-hatch invariant.

### Human Verification Required

None. The phase goal is a flag-gated scaffold; all observable truths are programmatically verifiable. Operational smoke that requires waiting (e.g., the ROADMAP success criterion 4 "≥ 7 days flat-zero `parser_requests_total{provider=\"animekai\"}`") is deferred to operator observation but does not block Phase 19 verification — the precondition (no animekai labels emitted) is verified at deploy time.

### Gaps Summary

No gaps. The phase delivered exactly what the escape-hatch contract requires:

- Wired-but-disabled provider package with all 5 files (doc.go, client.go, dto.go, client_test.go, helpers_test.go) — 9 unit tests pass.
- Flag-conditional registration in main.go gated on `cfg.AnimeKai.Enabled`; Phase 19 wiring invariant fatals if registered-count diverges.
- Sidecar `/animekai-token` returns HTTP 501 (not 500 — avoids retry-storm pitfall per 19-RESEARCH.md Pitfall 4); request body drained per WR-02.
- `SCRAPER_ANIMEKAI_ENABLED` + `SCRAPER_ANIMEKAI_BASE_URL` plumbed through docker-compose and documented in `.env.example`.
- REQUIREMENTS.md formally annotates the carryover: 5 rows show "Phase 19 → v3.1 / Carry — escape hatch" (KAI-01..04 + KAI-07); 2 rows show "Done" (KAI-05, KAI-06).
- `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns zero matches — ROADMAP success criterion 2 trivially satisfied.
- All 14 scraper packages pass `go test`; no regression in Phase 15-18 suites.
- Live production state: `/scraper/health` returns exactly 2 providers; `/metrics` has no animekai labels; `make health` reports all 8 services UP; sidecar POST returns 501.

**Phase 20 cutover is unblocked.** SCRAPER-KAI-01..04 + KAI-07 are formally carried to v3.1 with a body-only fill-in surface: only `client.go` method bodies + `server.js` `/animekai-token` handler body need to change in the v3.1 PR — wiring, config, docker, docs are all in place.

---

_Verified: 2026-05-12T19:55:00Z_
_Verifier: Claude (gsd-verifier)_
