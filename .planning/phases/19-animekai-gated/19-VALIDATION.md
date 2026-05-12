---
phase: 19
slug: animekai-gated
status: draft
nyquist_compliant: false
wave_0_complete: false
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
| _to be filled by planner_ | | | | | | | | | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `services/scraper/internal/providers/animekai/` — package skeleton + ErrProviderDown stubs + test scaffold
- [ ] `services/scraper/internal/config/config.go` — `AnimeKaiConfig` struct + `SCRAPER_ANIMEKAI_ENABLED` env binding (default false)
- [ ] `docker/megacloud-extractor/server.js` — `/animekai-token` route returning 501 with carry-to-v3.1 body

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
