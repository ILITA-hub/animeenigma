---
phase: 17
slug: observability
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-12
---

# Phase 17 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
>
> See `17-RESEARCH.md` `## Validation Architecture` for the full 22-test design rationale.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (Go 1.22) |
| **Config file** | services/scraper/go.mod (no extra test config; relies on `testdata/`) |
| **Quick run command** | `go test ./services/scraper/internal/health/... ./services/scraper/internal/service/... -count=1 -short` |
| **Full suite command** | `go test ./services/scraper/... ./services/gateway/... ./libs/metrics/... -count=1` |
| **Estimated runtime** | ~25 seconds full / ~6 seconds quick |

---

## Sampling Rate

- **After every task commit:** Run quick command (`go test ./services/scraper/internal/health/... ./services/scraper/internal/service/... -count=1 -short`)
- **After every plan wave:** Run full suite (`go test ./services/scraper/... ./services/gateway/... ./libs/metrics/... -count=1`)
- **Before `/gsd-verify-work`:** Full suite green + `make redeploy-scraper` succeeds + `curl :8088/metrics | grep provider_health_up` shows a series
- **Max feedback latency:** 25 seconds

---

## Per-Task Verification Map

(Filled in during planning — planner maps each task to a test or Wave 0 stub. See `17-RESEARCH.md` `## Validation Architecture` for the full table by requirement.)

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | Status |
|---------|------|------|-------------|-----------|-------------------|--------|
| (planner fills) | (planner fills) | (planner fills) | SCRAPER-OBS-01..05, SCRAPER-NF-04 | unit/integration | (from RESEARCH 22-test plan) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `services/scraper/internal/health/golden.go` — static golden anime pool (5–10 entries) with per-provider MAL→native ID resolution stubs
- [ ] `services/scraper/internal/health/probe_test.go` — clock injection scaffolding (`clock.Clock` interface or fake `Now()`)
- [ ] `services/scraper/internal/health/cache_test.go` — health cache RWMutex test fixtures + 60s TTL clock helpers
- [ ] `libs/metrics/provider_health.go` (or extension of `libs/metrics/parser.go`) — gauge + counter definitions for `provider_health_up{provider,stage}` and `parser_zero_match_total{provider,selector}` (the latter is currently missing — required by SCRAPER-NF-04)
- [ ] `docker/prometheus/prometheus.yml` — add scrape job for `animeenigma-scraper:8088/metrics` (BLOCKER: without this, all metrics this phase emits are invisible to Grafana)
- [ ] Fake provider for tests (`services/scraper/internal/health/testutil_provider.go`) — programmable Search/ListEpisodes/ListServers/GetStream/StreamSegment that can return errors after N calls to drive the 3-consecutive-failures-in-15-min threshold

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Telegram alert delivery end-to-end | SCRAPER-OBS-04 | Requires real Grafana → alertmanager → Telegram bot pipeline | Break AnimePahe stream stage for 15 min in prod (rename the Kwik referer header to garbage); confirm Telegram message lands; revert |
| Grafana dashboard panel renders | SCRAPER-OBS-04 | UI rendering check, not a code test | Import dashboard JSON via Grafana UI, visually confirm per-provider/per-stage tiles update on probe tick |
| Admin endpoint auth (real JWT) | SCRAPER-OBS-05 | JWT signing key is env-injected, not unit-testable end-to-end | `curl -H "Authorization: Bearer <admin JWT>" https://animeenigma.ru/api/admin/scraper/health` returns 200 + JSON; same call without/with non-admin JWT returns 401/403 |
| Probe survives 24h soak | SCRAPER-OBS-01 | Long-running goroutine stability under real traffic | After deploy, leave for 24h; `kubectl logs animeenigma-scraper` (or docker logs) shows no panics; `/metrics` continuously emits |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 25s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
