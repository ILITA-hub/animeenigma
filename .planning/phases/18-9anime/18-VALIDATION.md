---
phase: 18
slug: 9anime
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-12
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (services/catalog), bun test (frontend/web) |
| **Config file** | services/catalog/go.mod, frontend/web/package.json |
| **Quick run command** | `make redeploy-catalog && curl -s localhost:8000/api/streaming/health` |
| **Full suite command** | `cd services/catalog && go test ./... && cd frontend/web && bun run test:e2e -- english-player` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./services/catalog/internal/parser/gogoanime/...`
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
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

- [ ] `services/catalog/internal/parser/gogoanime/parser_test.go` — stubs for SCRAPER-9ANI-01..06
- [ ] `services/catalog/internal/parser/gogoanime/extractor_test.go` — embed extractor stubs (vibeplayer/streamhg/earnvids)
- [ ] `frontend/web/tests/e2e/english-player-multi-provider.spec.ts` — playwright e2e for source dropdown

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Visual: source dropdown opens/closes, item state visible | SCRAPER-9ANI-01 (UI) | Visual styling per UI-SPEC | Open english player; click source chip; verify 2 options visible; verify accent on selected |
| End-to-end stream playback via gogoanime | SCRAPER-9ANI-02 | Requires live provider — e2e test uses real network | Force animepahe health to 0; switch to gogoanime; verify HLS playback for ≥30s |
| Telegram notification on provider failover | SCRAPER-9ANI-04 | External Telegram API | Force animepahe error; verify Telegram message arrives with both provider chains |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
