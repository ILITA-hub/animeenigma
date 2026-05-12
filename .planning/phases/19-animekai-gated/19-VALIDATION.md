---
phase: 19
slug: animekai-gated
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-12
---

# Phase 19 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> R&D ESCAPE-HATCH PATH: ship flag-default-off; SCRAPER-KAI-01..04 carry to v3.1.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (services/scraper), node + bunx (frontend/docker megacloud-extractor) |
| **Config file** | services/scraper/go.mod, docker/megacloud-extractor/package.json |
| **Quick run command** | `cd /data/animeenigma && go test ./services/scraper/... -count=1 -timeout=60s` |
| **Full suite command** | `cd /data/animeenigma && go test ./services/scraper/... -count=1 -race -timeout=180s && curl -fsS http://localhost:7860/animekai-token -X POST -d '{}' -o /dev/null -w '%{http_code}'` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./services/scraper/internal/providers/animekai/... ./services/scraper/internal/config/...`
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green + flag-off boot invariant verified
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

> Populated by gsd-planner — each PLAN.md task contributes a row here.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| Task 1 — Provider package scaffold (animekai/) | 19-01 | 1 | SCRAPER-KAI-01..04, SCRAPER-KAI-06 | T-19-04 (E — stub must return ErrProviderDown, not silent empty success) | Every Provider method returns `domain.WrapProviderDown(errAnimeKaiStub, ...)`; `HealthCheck` pre-seeds all four stages as `Up: false` at boot (no green-panel-for-15min anti-pattern); compile-time assertion `var _ domain.Provider = (*Provider)(nil)` guarantees interface conformance | unit (TDD, 8 tests) | `cd /data/animeenigma && go test ./services/scraper/internal/providers/animekai/... -count=1 -timeout=60s 2>&1 \| tail -20` | yes (files created by this task) | ⬜ pending |
| Task 2 — Config + main.go wiring (AnimeKaiConfig, getEnvBool, conditional registration, Phase 19 invariant) | 19-01 | 1 | SCRAPER-KAI-05, SCRAPER-KAI-06 | T-19-01 (T — wiring invariant fatals if RegisteredProviders count mismatches flag state); T-19-08 (T — getEnvBool falls back to default on garbage input, can't be flipped by adversarial env strings) | `SCRAPER_ANIMEKAI_ENABLED` defaults FALSE; `cfg.AnimeKai.Enabled` gates the `orchestrator.Register(animeKaiProvider)` call; Phase 19 invariant fatals at boot if `len(RegisteredProviders()) != 2` (flag off) or `!= 3` (flag on); `strconv.ParseBool` rejects unparseable values (returns default false) | unit (TDD, 8 tests) + build | `cd /data/animeenigma && go test ./services/scraper/internal/config/... -count=1 -timeout=30s 2>&1 \| tail -10 && go build ./services/scraper/... 2>&1 \| tail -5` | yes (files modified by this task) | ⬜ pending |
| Task 3 — Sidecar 501 stub, env documentation, REQUIREMENTS.md carryover annotation | 19-01 | 1 | SCRAPER-KAI-02, SCRAPER-KAI-05, SCRAPER-KAI-06, SCRAPER-KAI-01..04+07 carryover annotation | T-19-03 (D — sidecar returns HTTP **501** not 500 to terminate orchestrator retry-storm cleanly per RESEARCH.md Pitfall 4); T-19-07 (R — carryover decision documented in REQUIREMENTS.md + RESEARCH.md so escape hatch is non-repudiable) | `POST /animekai-token` returns HTTP 501 with `"carry to v3.1"` body; `${SCRAPER_ANIMEKAI_ENABLED:-false}` env interpolation defaults off; `.env.example` documents the flag + caveat; REQUIREMENTS.md status table marks KAI-01..04+07 as "Phase 19 → v3.1 / Carry"; `grep -r "enc-dec.app"` returns zero (no external decryption dependency leaked) | integration (grep gate + compose parse) | `grep -F "writeHead(501" /data/animeenigma/docker/megacloud-extractor/server.js \| grep -c "animekai\\\|/animekai-token" && grep -F 'SCRAPER_ANIMEKAI_ENABLED:' /data/animeenigma/docker/docker-compose.yml && grep -F 'SCRAPER_ANIMEKAI_ENABLED' /data/animeenigma/docker/.env.example && grep -c "Phase 19 → v3.1" /data/animeenigma/.planning/REQUIREMENTS.md && ! grep -rq "enc-dec.app" /data/animeenigma/services/ /data/animeenigma/docker/megacloud-extractor/ && echo "PASS"` | yes (files modified by this task) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `services/scraper/internal/providers/animekai/` — package skeleton + ErrProviderDown stubs + test scaffold (Task 1 of 19-01 wave 1 creates all four files including `helpers_test.go` with the `fakeCache` lifted verbatim from gogoanime)
- [x] `services/scraper/internal/config/config.go` — `AnimeKaiConfig` struct + `SCRAPER_ANIMEKAI_ENABLED` env binding (default false) (Task 2 of 19-01 wave 1 adds the struct, `getEnvBool` helper, and tests using the existing `setEnv`/`unsetEnv` convention from config_test.go lines 9-41)
- [x] `docker/megacloud-extractor/server.js` — `/animekai-token` route returning 501 with carry-to-v3.1 body (Task 3 of 19-01 wave 1)

All three Wave 0 requirements are folded into the single wave-1 plan (19-01); no separate scaffolding plan is needed because the package is small (~150 Go LOC + ~10 JS LOC) and the scaffold-and-fill-in surfaces are the same files.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Flag-off boot invariant: only animepahe + gogoanime registered | SCRAPER-KAI-05 | Requires running scraper container | `curl /scraper/health | jq '.providers \| keys'` returns exactly `["animepahe", "gogoanime"]` |
| 7-day flat-zero traffic check | SCRAPER-KAI-06 | Requires 7-day prod observation window | After deploy, verify `parser_requests_total{provider="animekai"} == 0` after 7 days |
| `grep enc-dec.app` zero-match | SCRAPER-KAI-02 | Verify no external dependency leakage | `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns empty |
| REQUIREMENTS.md carryover annotation | SCRAPER-KAI-01..04 | Document escape hatch in living doc | Lines exist marking SCRAPER-KAI-01..04 as "Pending — carry to v3.1" |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** signed off by planner (2026-05-12, revision iteration 2/3)
