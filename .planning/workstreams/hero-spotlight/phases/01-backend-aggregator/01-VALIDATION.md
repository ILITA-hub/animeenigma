---
phase: 1
slug: backend-aggregator
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-05-21
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution of
> the hero-spotlight workstream's backend aggregator.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — `go.mod` per-service |
| **Quick run command** | `cd services/catalog && go test ./internal/service/spotlight/... ./internal/handler/... -count=1 -short` |
| **Full suite command** | `cd services/catalog && go test ./... -count=1 -race` |
| **Estimated runtime** | ~30s quick, ~60s full |

---

## Sampling Rate

- **After every task commit:** Run quick command (`go test ./internal/service/spotlight/... ./internal/handler/... -count=1 -short`)
- **After every plan wave:** Run full suite (`go test ./... -count=1 -race`)
- **Before `/gsd-verify-work`:** Full suite must be green + `make redeploy-catalog && make redeploy-gateway` clean + `curl -s http://localhost:8000/api/home/spotlight | jq '.cards | length'` returns ≥ 3 (4 expected; 3+ is acceptable if `latest_news` web fetch flaked)
- **Max feedback latency:** ≤30s

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | HSB-BE-02 | — | Package compiles; aggregator struct + Resolver interface defined | unit | `go build ./services/catalog/internal/service/spotlight/...` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 1 | HSB-BE-02 | — | Card / SpotlightResponse types serialize to expected JSON shape | unit | `go test ./services/catalog/internal/service/spotlight -run TestTypes_JSONShape -count=1` | ❌ W0 | ⬜ pending |
| 1-02-01 | 02 | 1 | HSB-BE-10 | — | `anime_of_day` resolver picks deterministic date-seeded anime; cache HIT returns identical anime within same UTC date | unit | `go test ./services/catalog/internal/service/spotlight/cards -run TestAnimeOfDay -count=1` | ❌ W0 | ⬜ pending |
| 1-02-02 | 02 | 1 | HSB-BE-11 | — | `random_tail` resolver picks from ranks 101+ (skip top-100); deterministic per date | unit | `go test ./services/catalog/internal/service/spotlight/cards -run TestRandomTail -count=1` | ❌ W0 | ⬜ pending |
| 1-02-03 | 02 | 1 | HSB-BE-12 | — | `latest_news` resolver fetches changelog.json shape, returns ≤3 newest entries; HTTP-client mock fails-soft (card dropped, not error) | unit | `go test ./services/catalog/internal/service/spotlight/cards -run TestLatestNews -count=1` | ❌ W0 | ⬜ pending |
| 1-02-04 | 02 | 1 | HSB-BE-13 | — | `platform_stats` resolver: card eligible iff ≥1 metric non-null; `anime_added_7d` queries `animes` `created_at` > NOW()-7d; `episodes_added_7d` returns nil (no per-episode log); `active_rooms_7d` returns nil (rooms is Redis-only per RESEARCH.md) | unit | `go test ./services/catalog/internal/service/spotlight/cards -run TestPlatformStats -count=1` | ❌ W0 | ⬜ pending |
| 1-03-01 | 03 | 2 | HSB-BE-03 | T-1-01 (resolver timeout) | Per-card `ctx.WithTimeout(800ms)` — slow resolver drops its card, other 3 succeed | unit | `go test ./services/catalog/internal/service/spotlight -run TestAggregator_PerCardTimeout -count=1 -race` | ❌ W0 | ⬜ pending |
| 1-03-02 | 03 | 2 | HSB-BE-04 | T-1-02 (overall budget) | Aggregator overall `ctx.WithTimeout(2s)` — returns partial results on overall deadline | unit | `go test ./services/catalog/internal/service/spotlight -run TestAggregator_OverallTimeout -count=1 -race` | ❌ W0 | ⬜ pending |
| 1-03-03 | 03 | 2 | HSB-BE-05 | T-1-03 (eligibility leak) | Cards with `eligible=false` are stripped from response payload | unit | `go test ./services/catalog/internal/service/spotlight -run TestAggregator_EligibilityFilter -count=1` | ❌ W0 | ⬜ pending |
| 1-03-04 | 03 | 2 | HSB-BE-04 | — | When 0 cards resolve AND `spotlight:snapshot:anon:<date>` key exists, aggregator returns snapshot | unit | `go test ./services/catalog/internal/service/spotlight -run TestAggregator_SnapshotFallback -count=1` | ❌ W0 | ⬜ pending |
| 1-03-05 | 03 | 2 | HSB-NF-03 | — | All Redis keys written under `spotlight:` prefix | unit | `go test ./services/catalog/internal/service/spotlight -run TestKeyPrefix -count=1` | ❌ W0 | ⬜ pending |
| 1-04-01 | 04 | 3 | HSB-BE-01 | — | Handler `GET /api/home/spotlight` returns 200 with `{cards, generated_at}` shape | unit | `go test ./services/catalog/internal/handler -run TestSpotlightHandler -count=1` | ❌ W0 | ⬜ pending |
| 1-04-02 | 04 | 3 | HSB-BE-07 | T-1-04 (flag bypass) | When `SpotlightEnabled=false` → handler returns 404 | unit | `go test ./services/catalog/internal/handler -run TestSpotlightHandler_FlagOff -count=1` | ❌ W0 | ⬜ pending |
| 1-04-03 | 04 | 3 | HSB-BE-01 | — | Handler tolerates `Authorization` header present (does not 401; Phase 1 is public) | unit | `go test ./services/catalog/internal/handler -run TestSpotlightHandler_OptionalAuth -count=1` | ❌ W0 | ⬜ pending |
| 1-05-01 | 05 | 3 | HSB-BE-06 | T-1-05 (gateway route open) | Gateway proxies `/api/home/spotlight` → `catalog:8081`; route reachable through gateway | unit | `go test ./services/gateway/internal/transport -run TestRouter_Spotlight -count=1` | ❌ W0 | ⬜ pending |
| 1-06-01 | 06 | 3 | HSB-BE-01..07, HSB-NF-01 | — | Smoke: `curl -s http://localhost:8000/api/home/spotlight \| jq '.cards \| length'` returns ≥3; second curl <1s shows cache hit; `/metrics` `http_request_duration_seconds{path="/api/home/spotlight"}` p95 < 100ms cached | smoke | `bash scripts/smoke-spotlight.sh` (added in Plan 06) | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `services/catalog/internal/service/spotlight/aggregator_test.go` — table-driven tests for concurrent fan-out, per-card timeout, overall timeout, eligibility filter, snapshot fallback, key prefix.
- [ ] `services/catalog/internal/service/spotlight/cards/{anime_of_day,random_tail,latest_news,platform_stats}_test.go` — one per resolver.
- [ ] `services/catalog/internal/service/spotlight/types_test.go` — JSON marshal shape check on Card discriminated union.
- [ ] `services/catalog/internal/handler/spotlight_test.go` — handler-level tests including flag-off 404 and optional-auth tolerance.
- [ ] `services/gateway/internal/transport/router_spotlight_test.go` — gateway proxy-route registration test.
- [ ] `scripts/smoke-spotlight.sh` — curl + jq + redis-cli smoke runner used by execute-phase's success criteria check.

All test files are new — no existing files are mocked. Mocks for `cache.Cache`, `AnimeRepository`, and the new `web_client.Client` use the project's handwritten-struct fake pattern (precedent: `services/catalog/internal/service/scraper_test.go:20-37`).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `web:80/changelog.json` is reachable from inside the catalog container | HSB-BE-12 | Docker network reachability is host-OS-dependent; CI-only assertion is brittle | After `make redeploy-catalog`, run `docker compose -f docker/docker-compose.yml exec catalog wget -qO- http://web:80/changelog.json \| head -c 200` and confirm JSON. |
| Snapshot fallback returns a sensible last-known-good payload when Redis is hard-down mid-test | HSB-BE-04 | Forced Redis outage on a live host disrupts other workstreams | Manually `docker compose stop redis`, hit the endpoint once, restart Redis. Confirm: while Redis is down the endpoint returns 200 with whatever cards still resolve in-memory (typically `random_tail` from DB without cache); no 500. |
| Latency targets (HSB-NF-01) under realistic load | HSB-NF-01 | Local single-shot curl ≠ p95 under concurrency | After deploy, run `for i in {1..50}; do curl -s -o /dev/null -w "%{time_total}\n" http://localhost:8000/api/home/spotlight; done \| sort -n \| awk 'NR==48 {print "p95="$1}'`. Cached p95 should be < 0.1s; cold (after `redis-cli DEL spotlight:*`) should be < 1.5s. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s (quick) / 60s (full)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
