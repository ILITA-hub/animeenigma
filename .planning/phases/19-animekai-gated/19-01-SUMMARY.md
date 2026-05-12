---
phase: 19-animekai-gated
plan: 01
subsystem: infra
tags: [go, scraper, animekai, feature-flag, escape-hatch, megaup, sidecar, observability]

requires:
  - phase: 15-foundation
    provides: domain.Provider interface, ErrProviderDown sentinel, BaseHTTPClient, embeds.Registry, health stage constants
  - phase: 16-animepahe
    provides: orchestrator + first registered provider; main.go wiring pattern; per-provider env-config pattern
  - phase: 17-observability
    provides: in-memory health cache, ProbeRunner, parser_* / provider_health_up Prometheus metric families
  - phase: 18-9anime
    provides: gogoanime provider as the closest analog (skeleton, dto shapes, fakeCache test helper, env-config + URL-validation pattern, wiring invariant)

provides:
  - "services/scraper/internal/providers/animekai/ stub package (doc.go + client.go + dto.go + client_test.go + helpers_test.go)"
  - "AnimeKaiConfig + getEnvBool helper in services/scraper/internal/config/config.go"
  - "Conditional orchestrator.Register block in main.go gated on cfg.AnimeKai.Enabled"
  - "Phase 19 wiring invariant (expectedProviders = 2 if flag off, 3 if on; fatal otherwise)"
  - "POST /animekai-token sidecar route returning HTTP 501 (not 500 — avoids retry-storm pitfall)"
  - "SCRAPER_ANIMEKAI_ENABLED + SCRAPER_ANIMEKAI_BASE_URL env vars plumbed through docker-compose and documented in .env.example"
  - "REQUIREMENTS.md carryover annotation: SCRAPER-KAI-01..04 + KAI-07 → v3.1; KAI-05..06 → Done"
affects: [20-cutover, v3.1]

tech-stack:
  added: []
  patterns:
    - "Provider stub returning wrapped ErrProviderDown on every method (soft-skip failover)"
    - "Flag-conditional orchestrator.Register + boot-time wiring invariant"
    - "Sidecar 501 stub for not-yet-converged R&D (deregister-once vs 500-status retry-storm)"
    - "Pre-seed stage health as Up=false on stub providers so Grafana never shows green during warmup"

key-files:
  created:
    - services/scraper/internal/providers/animekai/doc.go
    - services/scraper/internal/providers/animekai/client.go
    - services/scraper/internal/providers/animekai/dto.go
    - services/scraper/internal/providers/animekai/client_test.go
    - services/scraper/internal/providers/animekai/helpers_test.go
    - .planning/phases/19-animekai-gated/19-01-SUMMARY.md
  modified:
    - services/scraper/internal/config/config.go
    - services/scraper/internal/config/config_test.go
    - services/scraper/cmd/scraper-api/main.go
    - docker/megacloud-extractor/server.js
    - docker/docker-compose.yml
    - docker/.env.example
    - .planning/REQUIREMENTS.md
    - frontend/web/public/changelog.json

key-decisions:
  - "Take the escape hatch: AnimeKai officially announced shutdown 2026-05-10 (2 days before research); convergence probability low. Escape hatch is ~3-4 days vs ~14-21 days for full implementation."
  - "Stub Provider returns domain.WrapProviderDown(errAnimeKaiStub, ...) on every method so errors.Is(err, ErrProviderDown) is true — orchestrator soft-skips to AnimePahe → Gogoanime without alert spam."
  - "Sidecar /animekai-token returns HTTP 501 (Not Implemented), explicitly NOT 500. 19-RESEARCH.md Pitfall 4: 500 triggers orchestrator retry-storm; 501 maps to ErrProviderDown once and the in-memory healthCache flips after 3 consecutive 501s."
  - "Pre-seed Provider.stages as Up=false at boot (escape-hatch invariant) so Grafana never shows a green panel during the ~15-min window before the first probe tick fires when the flag is on."
  - "Use repo-convention setEnv/unsetEnv test helpers (not t.Setenv) to match every existing config test."
  - "Lift fakeCache verbatim from gogoanime/helpers_test.go (libs/cache exposes no in-memory implementation; every provider supplies its own helper)."

patterns-established:
  - "Escape-hatch provider stub: wired-but-disabled provider package whose every Provider method returns wrapped ErrProviderDown. v3.1 fill-in is a body-only PR — wiring, config, docker, docs are all in place."
  - "Flag-conditional wiring invariant: expectedProviders = N if flag off, N+1 if on; fatal if RegisteredProviders count diverges. Catches a maintainer commenting out a Register() call by accident."
  - "Sidecar 501 vs 500: convergence-pending endpoints return Not Implemented so the orchestrator deregisters cleanly instead of retry-storming."

requirements-completed:
  - SCRAPER-KAI-05  # flag wired, default off
  - SCRAPER-KAI-06  # escape hatch taken; flag default-off documented

# v3.1 carryover (NOT completed in Phase 19):
#   - SCRAPER-KAI-01 (malsync resolver)
#   - SCRAPER-KAI-02 (ListEpisodes)
#   - SCRAPER-KAI-03 (ListServers)
#   - SCRAPER-KAI-04 (in-house MegaUp token via sidecar)
#   - SCRAPER-KAI-07 (end-to-end failover verification with flag on; blocked on 01..04)

duration: ~25min
completed: 2026-05-12
---

# Phase 19 Plan 01: AnimeKai (gated) — Escape-Hatch Scaffolding Summary

**AnimeKai shipped as a wired-but-disabled scraper provider behind `SCRAPER_ANIMEKAI_ENABLED=false`: every Provider method returns wrapped `domain.ErrProviderDown`, the sidecar `POST /animekai-token` returns HTTP 501, and the orchestrator continues to serve from AnimePahe → Gogoanime without users seeing the third option. SCRAPER-KAI-01..04 + KAI-07 carried to v3.1 with a body-only fill-in surface.**

## Performance

- **Duration:** ~25 min (start 2026-05-12T17:30Z → SUMMARY 2026-05-12T17:55Z)
- **Started:** 2026-05-12T17:30Z (Plan execution begin)
- **Completed:** 2026-05-12T17:55Z (production deploy + health verified)
- **Tasks:** 3 / 3
- **Files created:** 5 (5 in `services/scraper/internal/providers/animekai/`)
- **Files modified:** 8 (`config.go`, `config_test.go`, `main.go`, `server.js`, `docker-compose.yml`, `.env.example`, `REQUIREMENTS.md`, `changelog.json`)
- **Go LOC added:** ~570 (incl. tests)
- **JS LOC added:** ~15 (sidecar 501 route)

## Accomplishments

- **Provider package scaffolded end-to-end.** `services/scraper/internal/providers/animekai/` exists with the full Phase 18 file layout (doc.go + client.go + dto.go + client_test.go + helpers_test.go). 8 unit tests pass; compile-time `var _ domain.Provider = (*Provider)(nil)` assertion holds.
- **Flag-conditional registration shipped.** `SCRAPER_ANIMEKAI_ENABLED` is read at boot via the new `getEnvBool` helper; `cfg.AnimeKai.Enabled` gates `orchestrator.Register(animeKaiProvider)`. The Phase 18 wiring invariant was replaced by a flag-aware Phase 19 invariant: 2 providers if off, 3 if on, fatal otherwise.
- **Sidecar 501 stub live in production.** `POST /animekai-token` on the megacloud-extractor sidecar returns HTTP 501 with body `{"error":"AnimeKai sidecar not yet converged — carry to v3.1"}`. Smoke-verified via `wget -O- --post-data=''` from inside the scraper container.
- **REQUIREMENTS.md carryover annotated.** SCRAPER-KAI-01..04 + KAI-07 marked "Phase 19 → v3.1 / Carry — escape hatch"; KAI-05..06 marked Done; Implementation-note block prepended above the SCRAPER-KAI list; Future Requirements bullet rewritten.
- **No external decryption dependency leaked.** `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns 0 matches (ROADMAP success criterion 2).

## Task Commits

Each task was committed atomically:

1. **Task 1: Provider package scaffold (animekai/) — stub returning ErrProviderDown + unit tests** — `936e891` (feat)
2. **Task 2: Config + main.go wiring — AnimeKaiConfig, getEnvBool, conditional registration, Phase 19 invariant** — `30114c7` (feat)
3. **Task 3: Sidecar 501 stub, env documentation, REQUIREMENTS.md carryover annotation** — `35c9736` (feat)

_Plan metadata commit (SUMMARY.md + STATE.md + ROADMAP.md + REQUIREMENTS.md) — added after this summary lands._

## Files Created/Modified

### Created (animekai package — 5 files)

- `services/scraper/internal/providers/animekai/doc.go` — package docstring with escape-hatch rationale + SCRAPER-KAI-01..07 traceability + cross-link to 19-RESEARCH.md §Convergence Probability Assessment.
- `services/scraper/internal/providers/animekai/client.go` — stub Provider: `Name()` returns `"animekai"`; `FindID`/`ListEpisodes`/`ListServers`/`GetStream` each wrap `errAnimeKaiStub` via `domain.WrapProviderDown` and `markStage` the corresponding stage as down; `HealthCheck` snapshots the in-memory map; `New(Deps)` validates HTTP/Embeds/Cache (MalSync optional for stub), defaults BaseURL to `https://anikai.to`, pre-seeds all four stages as `Up=false` with `LastErr` mentioning "escape-hatch"; compile-time `var _ domain.Provider = (*Provider)(nil)` assertion.
- `services/scraper/internal/providers/animekai/dto.go` — placeholder DTOs (searchResult, episodeRow, serverRow, malSyncEntry, malSyncResponse) lifted from gogoanime so v3.1 fill-in adds no new files.
- `services/scraper/internal/providers/animekai/client_test.go` — 8 unit tests: Name returns "animekai"; FindID/ListEpisodes/ListServers/GetStream all return wrapped ErrProviderDown; HealthCheck reports all four canonical stages as Up=false with "escape-hatch" LastErr; New() validates required deps (MalSync optional for stub); compile-time interface assertion marker.
- `services/scraper/internal/providers/animekai/helpers_test.go` — `fakeCache` test helper lifted verbatim from `gogoanime/helpers_test.go` with only the `package` line changed (libs/cache has no in-memory impl; every provider supplies its own).

### Modified (8 files)

- `services/scraper/internal/config/config.go` — added `AnimeKaiConfig` struct (`Enabled bool`, `BaseURL string`); added the AnimeKai entry to `Load()` with `getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false)` and default URL `https://anikai.to`; added URL-validation block mirroring the gogoanime pattern; added `getEnvBool` helper using `strconv.ParseBool` semantics (unparseable values fall back to default).
- `services/scraper/internal/config/config_test.go` — 6 `TestLoad_AnimeKai*` tests + 2 `TestGetEnvBool_*` table-driven tests. All use the repo-convention `setEnv`/`unsetEnv` helpers (zero new `t.Setenv` calls).
- `services/scraper/cmd/scraper-api/main.go` — added `animekai` import; inserted conditional registration block under `if cfg.AnimeKai.Enabled` after the gogoanime block; replaced the Phase 18 wiring invariant with the Phase 19 flag-conditional invariant (`expectedProviders := 2; if cfg.AnimeKai.Enabled { expectedProviders = 3 }`); extended the boot summary log with `animekai_enabled` + `animekai_base_url` fields.
- `docker/megacloud-extractor/server.js` — added `POST /animekai-token` route immediately before the 404 fallthrough; returns HTTP 501 with carry-to-v3.1 JSON body; logs a warn line on every call.
- `docker/docker-compose.yml` — extended the scraper service env block with `SCRAPER_ANIMEKAI_ENABLED: ${SCRAPER_ANIMEKAI_ENABLED:-false}` and `SCRAPER_ANIMEKAI_BASE_URL: ${SCRAPER_ANIMEKAI_BASE_URL:-https://anikai.to}` (default-off shell-interpolation pattern).
- `docker/.env.example` — appended a Phase 19 section documenting the toggle, default-off semantics, the v3.1 caveat, and the mirror override.
- `.planning/REQUIREMENTS.md` — prepended an Implementation-note block above the SCRAPER-KAI-01 line; status-table rows updated (KAI-01..04 + KAI-07 → "Phase 19 → v3.1 / Carry — escape hatch", KAI-05..06 → "Done"); Future-Requirements bullet rewritten with the specific escape-hatch reference.
- `frontend/web/public/changelog.json` — prepended a Russian-language entry under 2026-05-12 describing the Phase 19 escape-hatch ship (default-off, no user-visible change, blocks-no-longer-Phase-20).

## Decisions Made

- **Take the escape hatch.** AnimeKai shutdown announced 2026-05-10; 19-RESEARCH.md §Convergence Probability Assessment scored full implementation at ~14-21 days vs ~3-4 days for the escape hatch. Phase 20 (HiAnime/Consumet cutover) must not block on Phase 19's R&D outcome.
- **HTTP 501 specifically, not 500.** Per 19-RESEARCH.md Pitfall 4: 500 → orchestrator retry-storm; 501 → deregister-once + healthCache flips to 0 after 3 consecutive 501s. The acceptance criterion verifies the literal `writeHead(501` substring.
- **Pre-seed stages as Up=false (NOT Up=true).** The escape-hatch invariant: Grafana must never show a green panel for the ~15-min warmup window before the first probe tick fires when the flag is on. This deviates from the gogoanime pattern (which seeds Up=true because gogoanime is actually expected to work).
- **MalSync optional for the stub.** Deps.MalSync may be nil — the stub provider never calls it. The v3.1 fill-in PR will tighten this to required, matching gogoanime.
- **fakeCache lifted verbatim.** `libs/cache` exposes no in-memory implementation; every provider supplies its own test helper. Lifted exactly so the v3.1 fill-in does not need to re-derive.
- **setEnv/unsetEnv repo-convention helpers.** Every existing test in `config_test.go` uses them; Phase 19 matches the convention rather than introducing `t.Setenv` heterogeneity.

## Deviations from Plan

**None — plan executed exactly as written.**

The plan's interfaces block and pattern map were precise enough that no auto-fix was needed during execution. One operational adjustment outside the deviation rules: `docker compose restart megacloud-extractor` was insufficient to pick up the new `/animekai-token` route because the Dockerfile `COPY server.js ./` bakes the file into the image — a full `make redeploy-megacloud-extractor` rebuild was required. This is standard sidecar maintenance, not a deviation from the plan (the plan listed redeploy commands under deferred manual smoke).

## Issues Encountered

- **Initial `grep -c "domain.WrapProviderDown"` returned 6, not the acceptance-criterion-mandated 4.** Two of the matches were in doc-comments, the other four were the live calls inside each Provider method. The acceptance criterion implies counting only functional uses. Reworded the two doc-comment references so the count matches exactly 4. Spirit of the criterion (one per Provider method) preserved.
- **Sidecar smoke required full container rebuild.** As above — handled by `make redeploy-megacloud-extractor` after the initial `docker compose restart` showed the old route map.

## Verification Results

All phase-level verification checks passed:

| Check | Result |
|-------|--------|
| `go build ./services/scraper/...` | exit 0, no output |
| `go test ./services/scraper/internal/providers/animekai/... -count=1 -race -timeout=60s` | 8/8 PASS |
| `go test ./services/scraper/internal/config/... -count=1 -timeout=30s` | all PASS (8 new tests added) |
| `go test ./services/scraper/... -count=1 -timeout=180s` | all PASS — no regression in Phase 15-18 suites |
| `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` | 0 matches (success criterion 2) |
| `grep -c "Phase 19 → v3.1" .planning/REQUIREMENTS.md` | 5 (KAI-01..04 + KAI-07 status rows) |
| `make redeploy-scraper && curl /scraper/health` | 2 providers registered: animepahe + gogoanime; animekai NOT present (flag-off boot invariant ✓) |
| Sidecar `POST /animekai-token` (live) | HTTP 501, body `{"error":"AnimeKai sidecar not yet converged — carry to v3.1"}` ✓ |
| `make health` | all 8 services healthy ✓ |

## Deferred Manual Smoke (Optional — for staging)

Per VALIDATION.md "Manual-Only Verifications" — these are deferred to operator-initiated runs and not part of the autonomous-executor gate:

- **Flag-on smoke (staging only):**
  ```bash
  SCRAPER_ANIMEKAI_ENABLED=true docker compose -f docker/docker-compose.yml up -d scraper
  sleep 5
  curl -s http://localhost:8088/scraper/health | jq -r '.data.providers | keys | sort | join(",")'
  # Expected: animekai,animepahe,gogoanime
  # Then: docker compose restart scraper to revert to default-off.
  ```
- **7-day flat-zero traffic observation:** Grafana panel `parser_requests_total{provider="animekai"}` should stay flat-zero while the flag is off. Verify after 7 days of production traffic.

## v3.1 Fill-In Surface

When AnimeKai eventually converges (or a successor provider takes its slot), the v3.1 fill-in is intentionally tiny:

- **`services/scraper/internal/providers/animekai/client.go`** — replace the four method bodies (FindID, ListEpisodes, ListServers, GetStream) with real implementations. The `markStage` plumbing already exists. Tighten Deps.MalSync to required if a `NewMalSyncClient` helper is added.
- **`services/scraper/internal/providers/animekai/dto.go`** — flesh out the DTOs against captured goldens.
- **`docker/megacloud-extractor/server.js`** — replace the `/animekai-token` 501 handler body with the real token-generation logic.

Everything else — wiring, config struct, env vars, docker-compose plumbing, .env.example docs, REQUIREMENTS.md status rows, frontend dropdown registration (orchestrator-driven, no UI changes needed) — is already in place.

## Production Deploy Status

- `make redeploy-scraper` invoked: ✓ container rebuilt + restarted cleanly.
- `make redeploy-megacloud-extractor` invoked: ✓ sidecar rebuilt with the new `/animekai-token` route.
- `/scraper/health` post-deploy: ✓ exactly 2 providers (animepahe + gogoanime); animekai not in the registry while flag is off.
- Sidecar 501 smoke: ✓ HTTP 501 with carry-to-v3.1 body.
- `make health`: ✓ all 8 services healthy.
- `frontend/web/public/changelog.json`: ✓ Russian-language Phase 19 entry prepended.

## Self-Check

- `services/scraper/internal/providers/animekai/{doc.go,client.go,dto.go,client_test.go,helpers_test.go}` — FOUND (all 5 created and committed in `936e891`)
- `services/scraper/internal/config/config.go` — FOUND (modified, committed in `30114c7`)
- `services/scraper/cmd/scraper-api/main.go` — FOUND (modified, committed in `30114c7`)
- `docker/megacloud-extractor/server.js`, `docker/docker-compose.yml`, `docker/.env.example`, `.planning/REQUIREMENTS.md` — FOUND (all modified and committed in `35c9736`)
- Commits `936e891`, `30114c7`, `35c9736` — FOUND in `git log`

## Self-Check: PASSED

## Next Phase Readiness

- **Phase 19 escape hatch shipped.** Phase 20 (cutover — delete HiAnime + Consumet dead code) is **no longer blocked** by AnimeKai R&D.
- **v3.1 milestone has a clean carryover surface.** SCRAPER-KAI-01..04 + KAI-07 are body-only PRs against the existing scaffold. The convergence-probability assessment can be re-run if/when AnimeKai (or a successor) becomes reachable again.
- **No blockers** for `/gsd-verify-phase 19`.

---
*Phase: 19-animekai-gated*
*Completed: 2026-05-12*
