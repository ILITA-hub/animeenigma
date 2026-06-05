---
phase: 2
slug: be-egress-recorder
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-05
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Derived from `02-RESEARCH.md` § Validation Architecture.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `testcontainers-go/modules/clickhouse v0.40.0` (real CH, Phase-1 precedent) |
| **Config file** | none (Go convention; per-package `*_test.go`) |
| **Quick run command** | `go test ./<touched-package>/... -short -count=1` |
| **Full suite command** | `go test ./... -count=1` (+ `-run TestClickHouse`/`TestEffectsIngest` with Docker for CH-backed tests) |
| **Estimated runtime** | ~30s quick (per module); ~2–4 min full with CH testcontainer |

---

## Sampling Rate

- **After every task commit:** Run `go test ./<touched-package>/... -short -count=1`
- **After every plan wave:** Run full per-module `go test ./... -count=1` (CH tests with Docker)
- **Before `/gsd:verify-work`:** All five AR-EGRESS tests + the sink + non-block tests green; `make redeploy-{catalog,scraper,streaming,analytics}` + `make health` smoke.
- **Max feedback latency:** ~30 seconds (quick), ~4 min (full)

---

## Per-Requirement Verification Map

> Task IDs are assigned by the planner; this maps each requirement to its automated proof. The plan-checker reconciles task IDs into this table before `nyquist_compliant: true`.

| Requirement | Behavior | Test Type | Automated Command | File Exists |
|-------------|----------|-----------|-------------------|-------------|
| AR-EGRESS-01 | One outbound call → one effect row (host/status/bytes/duration) | integration | `go test ./libs/tracing/ -run TestRecordingTransport -count=1` | ❌ W0 |
| AR-EGRESS-02 | Baggage seeded inbound is read in the recorder (origin/operation/user_id) | unit | `go test ./libs/tracing/ -run TestBaggageSeedRead -count=1` | ❌ W0 |
| AR-EGRESS-02 | E2E: inbound middleware → outbound RoundTripper carries operation | integration | `go test ./libs/tracing/ -run TestBaggageE2E -count=1` | ❌ W0 |
| AR-EGRESS-03 | Each retrofit client's outbound is recorded | integration | `go test ./libs/idmapping/ ./libs/kodikextract/ ./services/catalog/internal/parser/opensubtitles/ ./services/scraper/... -run Transport -count=1` | ❌ W0 |
| AR-EGRESS-04 | N segment GETs under one `?sess=` → exactly ONE aggregated row after idle flush | integration | `go test ./services/streaming/internal/service/ -run TestHLSSessionAggregation -count=1` | ❌ W0 |
| AR-EGRESS-05 | bytes_in (upstream) and bytes_out (client) both non-zero and distinct | integration | `go test ./libs/videoutils/ -run TestDualByteCount -count=1` | ❌ W0 |
| (sink) | `/internal/effects` → batcher → CH row with effect dims populated | integration | `go test ./services/analytics/internal/... -run TestEffectsIngest -count=1` (CH testcontainer) | ❌ W0 |
| (non-block) | Producer drops on full buffer, never blocks, increments dropped metric | unit | `go test ./libs/tracing/ -run TestProducerDropOnFull -count=1` | ❌ W0 |
| (security) | `user_id` does NOT leave the process in propagated baggage on 3rd-party-bound requests | unit | `go test ./libs/tracing/ -run TestNoUserIDOnOutboundWire -count=1` | ❌ W0 |

*Status legend: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `libs/tracing/client_test.go` — recording transport: byte/duration/status capture + drop-on-full producer (AR-EGRESS-01, non-block, security)
- [ ] `libs/tracing/middleware_test.go` (extend) — baggage seed from claims+route; E2E inbound→outbound (AR-EGRESS-02)
- [ ] `services/analytics/internal/handler/effects_test.go` — `/internal/effects` → batcher → CH effect row (sink)
- [ ] `services/streaming/internal/service/hls_sessions_test.go` — session aggregation + idle flush + reaper eviction (AR-EGRESS-04)
- [ ] `libs/videoutils/proxy_test.go` (extend) — dual byte count + `?sess=` injection in `rewriteHLSURL` (AR-EGRESS-05)
- [ ] Retrofit transport-injection tests per leaf client (AR-EGRESS-03)
- Framework install: none — Go `testing` + testcontainers already present from Phase 1.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live egress rows appear in ClickHouse under load | AR-EGRESS-01/03 | Needs real 3rd-party traffic (scraper/streaming) post-deploy | After `make redeploy-*`, trigger a real EN-player resolve + a stream session; query `analytics.events WHERE effect_kind='egress'` for host/provider/bytes rows |
| One aggregated row per real watch session | AR-EGRESS-04 | Real HLS playback across many segments | Play an episode through the proxy, stop, wait ~idle window, assert a single `(session,host)` row with summed bytes + segment count |

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < ~30s quick
- [ ] `nyquist_compliant: true` set in frontmatter (after plan-checker reconciles task IDs)

**Approval:** pending
