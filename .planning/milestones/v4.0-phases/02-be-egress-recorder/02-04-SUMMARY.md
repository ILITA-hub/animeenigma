---
phase: 02-be-egress-recorder
plan: 04
subsystem: infra
tags: [egress, tracing, baggage, pii, opentelemetry, clickhouse, go, catalog, scraper, streaming, analytics, deploy]

# Dependency graph
requires:
  - phase: 02-be-egress-recorder
    plan: 01
    provides: "libs/tracing SeedMiddleware, recording RoundTripper, Producer, SetGlobalSink, PII-safe baggage + private user_id/provider ctx helpers; analytics POST /internal/effects ingestion"
  - phase: 02-be-egress-recorder
    plan: 02
    provides: "four-client egress retrofit (Kodik, OpenSubtitles, idmapping, scraper BaseHTTPClient ×7) routed through tracing.WrapTransport + scraper stream-provider ctx tag"
  - phase: 02-be-egress-recorder
    plan: 03
    provides: "HLS session aggregation (ONE effect row per (sess,host)) + dual byte counting; streaming producer + SetGlobalSink already wired"
provides:
  - "Defense-in-depth baggage-PII strip on the outbound recording RoundTripper (no user_id member ever rides the wire) + TestNoUserIDOnOutboundWire proof (T-02-PII)"
  - "TestBaggageE2E — inbound SeedMiddleware → outbound recording transport carries the same origin/operation end-to-end (AR-EGRESS-02)"
  - "catalog + scraper main.go: tracing.Producer (ANALYTICS_INTERNAL_URL) + SetGlobalSink + SeedMiddleware wired into the chi router (streaming + analytics already wired by 02-03/02-01)"
  - "ANALYTICS_INTERNAL_URL=http://analytics:8092 on catalog/scraper/streaming compose blocks + CLAUDE.md env docs"
  - "Live-verified BE egress recorder: real per-client scraper egress rows + ONE aggregated HLS row per (session,host) in ClickHouse; zero PII on the wire"
affects: [grafana-pivot-reports, be-egress-observability]

# Tech tracking
tech-stack:
  added: []  # no new external packages (RESEARCH §Package Legitimacy Audit; T-02-SC)
  patterns:
    - "Defense-in-depth PII strip: even though user_id is designed onto a private non-propagated ctx value (02-01), the recording RoundTrip asserts/strips any user_id baggage member so a future caller cannot accidentally add it to the wire"
    - "SeedMiddleware mounted on the chi router (NOT main.go) so chi RouteContext is populated for lazy operation resolution; egress Producer + SetGlobalSink constructed at BE service boot"
    - "Provider folded into the egress row's target host (no separate provider column) — the recorder pivots target = provider host"

key-files:
  created:
    - ".planning/phases/02-be-egress-recorder/02-04-SUMMARY.md"
  modified:
    - "libs/tracing/client.go"
    - "libs/tracing/client_test.go"
    - "services/catalog/cmd/catalog-api/main.go"
    - "services/scraper/cmd/scraper-api/main.go"
    - "docker/docker-compose.yml"
    - "CLAUDE.md"

key-decisions:
  - "Defense-in-depth strip of any user_id baggage member on the outbound RoundTrip — the design (02-01) already keeps user_id off the wire via a private ctx value, but the strip makes the PII boundary tamper-proof against a future accidental baggage add (T-02-PII)"
  - "SeedMiddleware is wired into the chi router (after RouteContext), NOT in main.go — the lazy operation resolver needs the populated chi RouteContext (verified in 02-01)"
  - "streaming + analytics producer/sink were already wired by 02-03 (streaming) and 02-01 (analytics serves /internal/effects); this plan only adds catalog + scraper producer wiring + the env var, avoiding double-wiring"
  - "provider is folded into the egress row's target host rather than a separate column — sufficient for the AR-EGRESS pivots; a dedicated provider column is deferred"

patterns-established:
  - "Pattern: outbound recording RoundTrip strips PII baggage members defensively, never trusting upstream baggage to be clean"
  - "Pattern: end-to-end baggage proof test (inbound middleware → outbound transport) asserts origin/operation survive a real hop while user_id does not"

requirements-completed: [AR-EGRESS-01, AR-EGRESS-02, AR-EGRESS-03, AR-EGRESS-04, AR-EGRESS-05]

# Metrics
duration: ~25min
completed: 2026-06-05
---

# Phase 02 Plan 04: BE Egress Recorder Phase Closeout Summary

**The PII boundary is now hardened and proven (user_id never rides outbound wire baggage — only origin/operation do, end-to-end), the egress Producer + SeedMiddleware are wired into catalog/scraper (streaming/analytics were already wired), and the full BE egress recorder is verified LIVE in ClickHouse: real per-client scraper egress rows, ONE aggregated HLS row per (session,host), and ClickHouse `count(user_id != '') = 0`.**

## Performance

- **Duration:** ~25 min (across implementation waves + blocking human-verify approval)
- **Tasks:** 3 (Task 1 TDD; Task 3 blocking human-verify, APPROVED)
- **Files modified:** 6 (+ this SUMMARY)

## Accomplishments
- **T-02-PII hardened + proven.** Added a defense-in-depth strip in `recordingTransport.RoundTrip` ensuring no `user_id` baggage member ever rides an outbound 3rd-party-bound request; the recorder reads `UserID` from the private `UserIDFromContext` ctx value, not from baggage. `TestNoUserIDOnOutboundWire` inspects the actual `baggage:` header on an httptest server and asserts origin/operation are present but `user_id` is not — while the in-process Effect still carries `UserID`.
- **AR-EGRESS-02 end-to-end proven.** `TestBaggageE2E`: an httptest inbound handler wrapped in `SeedMiddleware` makes an outbound call on the recording transport; the emitted Effect carries the SAME origin/operation seeded inbound — baggage rides end-to-end across the hop.
- **catalog + scraper wired.** `tracing.Producer` constructed from `ANALYTICS_INTERNAL_URL` (default `http://analytics:8092`), set as the process-global EffectSink (`SetGlobalSink`), `Start()`ed with `Stop()` on graceful shutdown; `tracing.SeedMiddleware(service)` mounted on the chi router. (streaming wired by 02-03, analytics serves `/internal/effects` per 02-01.)
- **Env + docs.** `ANALYTICS_INTERNAL_URL=http://analytics:8092` added to the catalog/scraper/streaming compose blocks (3×) and documented in CLAUDE.md's Environment Variables section. No gateway route for `/internal/effects` (Docker-network-only — T-02-INT).
- **Live verified (Task 3, APPROVED).** Redeployed analytics + catalog + scraper + streaming; `make health` green; analytics serving `/internal/effects` (204). Live ClickHouse evidence below.

## Task Commits

1. **Task 1: Baggage-PII strip + end-to-end baggage proof (libs/tracing)** — `2cafceec` (test, RED) → `9ca28026` (feat, GREEN)
2. **Task 2: Wire SeedMiddleware + egress producer into catalog/scraper/streaming + ANALYTICS_INTERNAL_URL** — `54176523` (feat)
3. **Task 3: Redeploy + live ClickHouse egress verification** — checkpoint (deploy only, no code commit); APPROVED by human

**Plan metadata:** this SUMMARY (`docs(02-04)`) follows separately.

_Note: TDD Task 1 split test → feat per the RED/GREEN gate._

## Files Created/Modified
- `libs/tracing/client.go` — defense-in-depth `user_id` baggage strip in `recordingTransport.RoundTrip`; recorder reads UserID from the private ctx value
- `libs/tracing/client_test.go` — `TestNoUserIDOnOutboundWire` (wire `baggage:` header inspection) + `TestBaggageE2E` (inbound SeedMiddleware → outbound carries origin/operation)
- `services/catalog/cmd/catalog-api/main.go` — Producer + SetGlobalSink + SeedMiddleware on the chi router; Stop() on shutdown
- `services/scraper/cmd/scraper-api/main.go` — Producer + SetGlobalSink + SeedMiddleware on the chi router; Stop() on shutdown
- `docker/docker-compose.yml` — `ANALYTICS_INTERNAL_URL=http://analytics:8092` on catalog/scraper/streaming (3×)
- `CLAUDE.md` — documents the new `ANALYTICS_INTERNAL_URL` env var

## Live Verification Evidence (Task 3 — APPROVED)

Redeploy: `make redeploy-{analytics,catalog,scraper,streaming}` + `make health` green for catalog/scraper/streaming (analytics Up(healthy) but absent from the `make health` target list — see Known Limitations). Analytics serving `POST /internal/effects` → 204 from the scraper container.

- **AR-EGRESS-04 (HLS single-row aggregation):** ONE aggregated HLS row per (session,host) — `Q8jL.flarestorm.buzz` requests=5, ~6.42 MB, `bytes_in == bytes_out == 6423960`; manifest fetch `cdn.mewstream.buzz` requests=1. NOT one row per segment.
- **AR-EGRESS-03 (per-client scraper egress):** per-client scraper egress rows (`GET /scraper/{episodes,servers,stream}`) to real provider hosts — `www.gogoanime.is`, `api.allanime.day`, `animefever.cc`, `www.miruro.tv` — with non-zero bytes.
- **AR-EGRESS-01/02 (recorder + baggage e2e):** inbound→outbound origin/operation baggage rides end-to-end; effect ingestion path live (POST `/internal/effects` → 204 from scraper container).
- **PII (T-02-PII):** ClickHouse `count(user_id IS NOT NULL AND != '') = 0` — user_id never on the wire.
- **Producer health:** `tracing_effects_dropped_total = 0` on catalog/scraper/streaming. Egress rows grew 17 → 23 over the verification window.
- **Requirements covered live:** AR-EGRESS-01, -02, -03, -04, -05.

## Decisions Made
- **Defense-in-depth strip (T-02-PII).** Even though 02-01 keeps `user_id` off the wire by design (private ctx value), the outbound RoundTrip now actively strips any `user_id` baggage member so a future accidental `baggage.NewMember("user_id", …)` cannot leak it. The recorder still attributes UserID from `UserIDFromContext`.
- **SeedMiddleware on the chi router, not main.go.** The lazy operation resolver (02-01) needs the populated chi RouteContext, which a `Use`-middleware sees only when mounted on the router — so the wiring goes into the router chain, not a bare `main.go` `net/http` middleware.
- **No double-wiring of streaming/analytics.** 02-03 already wired the streaming Producer + SetGlobalSink (the HLS aggregator needed a live sink) and 02-01 already made analytics serve `/internal/effects`; this plan only adds the catalog + scraper producer wiring + the shared env var.

## Deviations from Plan

### Auto-fixed / clarifications

**1. [Design clarification] SeedMiddleware wired into the chi routers, not main.go.**
- **Found during:** Task 2 (wiring).
- **Detail:** The plan's `<action>` said "add `tracing.SeedMiddleware(service)` AFTER chi routing"; concretely this means mounting it on the chi router chain (where RouteContext is populated for the lazy operation resolver), not in `main.go` as a bare middleware. Streaming + analytics producer/sink were already wired by 02-03 / 02-01, so this plan touched only catalog + scraper main.go for producer construction.
- **Impact:** None — matches the 02-01 SeedMiddleware contract.

**2. [Design clarification] Provider folded into the egress `target` host.**
- **Found during:** Task 3 (live verification).
- **Detail:** The egress rows carry the provider via the `target` host (e.g. `www.gogoanime.is`, `animefever.cc`) rather than a dedicated provider column. The AR-EGRESS pivots are satisfied by host-level attribution; a separate provider column is deferred.
- **Impact:** None on AR-EGRESS-01/02/03/04/05 — documented as a Known Limitation.

No functional deviation from the plan's design — the PII strip, both proof tests, the producer/middleware wiring, env var, and CLAUDE.md docs all match.

## Known Limitations (documented, not blockers)
- **Sparse `operation` on failover/segment GETs.** Browser segment GETs and some failover-path GETs are fresh requests without inbound baggage, so `operation` is often empty on those rows (carried over from 02-03's known limitation). The load-bearing fields — host, bytes, requests, duration — are ALWAYS populated.
- **Provider folded into `target` host.** No separate provider column (see Deviation 2).
- **analytics absent from the `make health` target list.** The analytics container is Up(healthy) and serving `/internal/effects` (verified 204), but it is not in the `make health` service-list output — a cosmetic gap in the health target, not a service failure.

## Known Stubs
None. All paths are live and verified in production: the recording RoundTrip strips PII and emits Effects, catalog/scraper/streaming producers ship to analytics `/internal/effects`, and ClickHouse holds real egress + single-HLS rows.

## Threat Flags
None. No new network endpoints, auth paths, or trust boundaries beyond the plan's threat register. `/internal/effects` stays Docker-network-only (T-02-INT — no gateway route added); `user_id` never rides the wire (T-02-PII — strip + `TestNoUserIDOnOutboundWire`); no new external packages (T-02-SC).

## TDD Gate Compliance
- Task 1: `test(02-04)` (`2cafceec`, RED — `TestNoUserIDOnOutboundWire`/`TestBaggageE2E` failing pre-strip) → `feat(02-04)` (`9ca28026`, GREEN). ✓
- Task 2: `feat(02-04)` (`54176523`) — wiring + env + docs, no behavior-test gate (pure DI/config wiring).
- Task 3: blocking human-verify checkpoint, APPROVED — no code commit (deploy-only).

## User Setup Required
None — `ANALYTICS_INTERNAL_URL` defaults to `http://analytics:8092` and is already set in the compose blocks; no external service configuration.

## Next Phase Readiness
- **Phase 02 (BE egress recorder) work is complete:** the recorder, four-client retrofit, HLS aggregation, PII hardening, and BE-service wiring are all delivered and live-verified.
- **Ready for grafana-pivot-reports:** ClickHouse now holds real `effect_kind='egress'` rows (host/bytes/requests/duration, provider folded into host) for the BE egress dashboards.
- No blockers. The orchestrator runs phase verification + phase.complete after this plan-level closeout.

---
*Phase: 02-be-egress-recorder*
*Completed: 2026-06-05*

## Self-Check: PASSED

- **Created file present:** `.planning/phases/02-be-egress-recorder/02-04-SUMMARY.md` (this file).
- **Task commits present (verified via `git log --oneline`):** `2cafceec` (test), `9ca28026` (feat, PII strip), `54176523` (feat, wiring). Task 3 is a deploy-only checkpoint (no code commit).
- **docker-compose env:** `grep -c ANALYTICS_INTERNAL_URL docker/docker-compose.yml` = 3 (catalog/scraper/streaming). ✓
- **must_haves verified LIVE (Task 3, APPROVED):** user_id off the wire (ClickHouse count = 0 + `TestNoUserIDOnOutboundWire`); origin/operation end-to-end (`TestBaggageE2E`); producer + SeedMiddleware wired with `ANALYTICS_INTERNAL_URL`; analytics serves `/internal/effects` (204); real egress rows + single aggregated HLS row in ClickHouse; `make health` green for catalog/scraper/streaming.
- **Requirements completed:** AR-EGRESS-01, -02, -03, -04, -05.
