---
phase: 02-be-egress-recorder
plan: 01
subsystem: infra
tags: [opentelemetry, baggage, clickhouse, prometheus, go, egress, tracing, analytics]

# Dependency graph
requires:
  - phase: 01-clickhouse-foundation-eventstore-swap
    provides: "wide-event ClickHouse store + InsertBatch with reserved (zeroed) effect columns; ingest.Batcher async sink; analytics router /internal/erase pattern"
provides:
  - "domain.Event effect dimensions (Origin/Operation/EffectKind/TargetKind/Target/Source/Accuracy/AnimeID) + measures (Requests/BytesIn/BytesOut/DurationMS/RowCount)"
  - "InsertBatch now POPULATES the CH effect columns from Event fields (no longer hard-coded zero); clickstream rows keep historical defaults via defaultStr fallback"
  - "POST /internal/effects ingestion endpoint (Docker-network-only, shares the existing batcher Sink)"
  - "libs/tracing.SeedBaggage/ReadBaggage (origin+operation on W3C wire baggage)"
  - "libs/tracing.WithUserID/UserIDFromContext + WithProvider/ProviderFromContext (PRIVATE non-propagated ctx values)"
  - "libs/tracing.SeedMiddleware (chi) — seeds origin + coarse operation + user_id from claims+route"
  - "libs/tracing.WrapRecording / recordingTransport — one egress Effect per outbound request (host/status/bytes/duration), body wrapped not buffered, emits on Close"
  - "libs/tracing.Producer — non-blocking drop-on-full async batcher POSTing to /internal/effects (tracing_effects_dropped_total counter)"
  - "libs/tracing.SetGlobalSink — WrapTransport composes recording when a process-global sink is set (nil-safe)"
affects: [02-02, 02-03, 02-04, be-egress-wiring, http-client-retrofits, hls-aggregation, grafana-pivot-reports]

# Tech tracking
tech-stack:
  added:
    - "go.opentelemetry.io/otel/baggage (sub-pkg of existing otel require — no external registry install)"
    - "github.com/go-chi/chi/v5 v5.2.5 (libs/tracing — for SeedMiddleware RoutePattern)"
    - "github.com/ILITA-hub/animeenigma/libs/authz (libs/tracing — replace ../authz, for ClaimsFromContext/UserIDFromContext)"
    - "github.com/prometheus/client_golang v1.23.2 (libs/tracing — dropped counter)"
  patterns:
    - "Wide-event one-row-per-effect: BE Effect → producer → /internal/effects → existing ingest.Batcher → InsertBatch (single write path shared with clickstream)"
    - "PII split: origin/operation ride W3C wire baggage; user_id + provider ride PRIVATE unexported-key ctx values that never propagate outbound (T-02-PII)"
    - "Lazy operation resolution: chi Use-middleware runs before route match, so operation is resolved at read time from a stashed RouteContext pointer"
    - "Recording RoundTripper wraps resp.Body in a counting ReadCloser and emits exactly one Effect on Close — never buffers the whole body (D-10)"
    - "Producer mirrors ingest.Batcher (buffered channel + non-blocking drop-on-full select default + size/interval flush + graceful Stop drain)"

key-files:
  created:
    - "services/analytics/internal/handler/effects.go"
    - "services/analytics/internal/handler/effects_test.go"
    - "libs/tracing/baggage.go"
    - "libs/tracing/baggage_test.go"
    - "libs/tracing/seed_middleware_test.go"
    - "libs/tracing/effect.go"
    - "libs/tracing/producer.go"
    - "libs/tracing/client_recording_test.go"
  modified:
    - "services/analytics/internal/domain/event.go"
    - "services/analytics/internal/repo/clickhouse_store.go"
    - "services/analytics/internal/transport/router.go"
    - "services/analytics/cmd/analytics-api/main.go"
    - "libs/tracing/client.go"
    - "libs/tracing/middleware.go"
    - "libs/tracing/go.mod"

key-decisions:
  - "operation resolved LAZILY via a stashed chi RouteContext pointer (read at endpoint/outbound time) because a chi Use-middleware runs before the route pattern is populated — verified empirically"
  - "user_id and provider ride PRIVATE unexported-key ctx values, never W3C baggage, so they cannot leak to 3rd-party hosts on outbound requests (T-02-PII)"
  - "Producer is process-wide (global Prometheus counter shared across producers) and best-effort: POST failures are swallowed so a producer/analytics outage never breaks the outbound caller (T-02-DOS / D-10)"
  - "InsertBatch effect columns now driven by Event fields with defaultStr/(\"api\",\"be\",\"exact\") fallback so clickstream rows are byte-identical to before"
  - "libs/tracing gained chi+authz+prometheus deps (existing workspace module libs/authz via relative replace ../authz, mirroring libs/cache→libs/metrics) — no NEW workspace module, so no Dockerfile COPY changes needed"

patterns-established:
  - "Pattern: BE effect ingestion reuses the analytics ingest.Batcher Sink — one InsertBatch write path for clickstream + effect rows"
  - "Pattern: PII-safe attribution — origin/operation on wire baggage, user_id/provider on private ctx values"
  - "Pattern: non-blocking drop-on-full producer on the outbound hot path with a Prometheus dropped counter"

requirements-completed: [AR-EGRESS-01, AR-EGRESS-02]

# Metrics
duration: 10min
completed: 2026-06-05
---

# Phase 02 Plan 01: BE Egress Recorder + Effect Sink Summary

**The BE effect-ingestion path that did not exist before: domain.Event now carries effect dimensions+measures, InsertBatch populates the (previously zeroed) ClickHouse effect columns, POST /internal/effects ingests effect batches into the existing batcher, and libs/tracing gained a recording RoundTripper + non-blocking producer + PII-safe baggage/ctx attribution helpers.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-06-05T07:49:19Z
- **Completed:** 2026-06-05T07:59:34Z
- **Tasks:** 3 (all TDD)
- **Files modified/created:** 16

## Accomplishments
- `domain.Event` extended with 8 effect dimensions + 5 measures; `InsertBatch` populates the CH effect columns from Event fields (clickstream rows unchanged via defaults).
- `POST /internal/effects` ingestion endpoint registered Docker-network-only beside `/internal/erase`, sharing the existing `ingest.Batcher` Sink (256 KB LimitReader + 1000-row cap).
- `libs/tracing`: `SeedBaggage`/`ReadBaggage` (origin+operation on wire baggage) + `WithUserID`/`UserIDFromContext` + `WithProvider`/`ProviderFromContext` (private ctx values, never propagated).
- `SeedMiddleware` (chi) seeds origin + coarse `service METHOD routePattern` operation + user_id from claims; operation resolved lazily to work around chi's pre-route-match middleware timing.
- `recordingTransport` / `WrapRecording`: one egress `Effect` per outbound request (host/status/bytes/duration), body wrapped in a counting ReadCloser and emitted on Close — never buffered.
- `Producer`: non-blocking drop-on-full async batcher POSTing JSON effect batches to `/internal/effects`, with a `tracing_effects_dropped_total` Prometheus counter and graceful Stop() drain.

## Task Commits

Each task was committed atomically (TDD: test → feat):

1. **Task 1: Effect fields + InsertBatch + /internal/effects** - `e6da0256` (test), `f166977c` (feat)
2. **Task 2: Baggage seed/read + private user_id ctx value + SeedMiddleware** - `f01a8516` (feat; test co-committed — see Issues)
3. **Task 3: Recording RoundTripper + async producer** - `97df31cc` (test), `82315228` (feat)

_Plan metadata commit (this SUMMARY) follows separately._

## Files Created/Modified
- `services/analytics/internal/domain/event.go` - effect dimension + measure fields on Event
- `services/analytics/internal/repo/clickhouse_store.go` - InsertBatch populates effect columns from Event fields + `defaultStr` helper
- `services/analytics/internal/handler/effects.go` + `effects_test.go` - `/internal/effects` ingestion handler (LimitReader + array cap)
- `services/analytics/internal/transport/router.go` - registers `POST /internal/effects`
- `services/analytics/cmd/analytics-api/main.go` - constructs EffectsHandler sharing the batcher Sink
- `libs/tracing/baggage.go` + `baggage_test.go` - wire-baggage origin/operation + private user_id/provider ctx values + lazy operation resolver
- `libs/tracing/middleware.go` + `seed_middleware_test.go` - `SeedMiddleware`
- `libs/tracing/effect.go` - `Effect` struct + `EffectSink` interface
- `libs/tracing/producer.go` - async drop-on-full `Producer` + dropped counter
- `libs/tracing/client.go` + `client_recording_test.go` - `recordingTransport`, `WrapRecording`, `SetGlobalSink`, global-sink composition in `WrapTransport`
- `libs/tracing/go.mod` / `go.sum` - chi v5, libs/authz (replace ../authz), prometheus/client_golang

## Decisions Made
- **Lazy operation resolution.** A chi `Use`-middleware runs BEFORE the route tree match completes (empirically verified: `RoutePattern()` is `""` at middleware time, populated only in the handler). `SeedMiddleware` therefore stashes the chi RouteContext pointer + service/method on a private ctx value and `ReadBaggage` resolves `service METHOD routePattern` at read time (handler / outbound-call time), keeping callers timing-agnostic.
- **PII split.** `user_id` and `provider` never enter W3C baggage (which is injected on outbound requests and would leak to 3rd-party hosts — RESEARCH §Security "sharpest finding"). They ride unexported-key ctx values only. `TestUserIDCtxValue` / `TestSeedMiddleware` assert `baggage.FromContext(ctx).Member("user_id")` is empty.
- **Best-effort producer.** POST failures are swallowed and a full buffer drops (counter-incremented) rather than blocking the outbound hot path (D-10, T-02-DOS).
- **InsertBatch backward-compat.** Clickstream rows leave Event's effect fields empty/zero; `defaultStr` restores the historical `origin="api"`, `source="be"`, `accuracy="exact"` column defaults, so existing rows are byte-identical.

## Deviations from Plan

All plan tasks were executed exactly as written (effect dimensions, measures, ingestion endpoint, baggage/ctx helpers, recording transport, producer). The only non-task action was reverting `go work sync`'s cross-workspace go.mod/go.sum churn (see Issues Encountered) — a Rule 3 blocking-cleanup to keep the change set scoped to the plan's files. No functional deviation from the plan's design.

## Issues Encountered
- **chi middleware route-pattern timing (Task 2).** Initial `SeedMiddleware` read `chi.RouteContext(r).RoutePattern()` directly and got `""` (the test `TestSeedMiddleware` failed expecting the coarse route pattern). Root cause: chi populates `RoutePattern()` only after the route match, which happens *after* `Use`-middlewares run. Resolved with the lazy-resolver pattern described above; verified with a throwaway probe test, then `TestSeedMiddleware` passes. Not a deviation — it's the planned middleware, just implemented to honor chi's documented timing.
- **TDD commit shape (Task 2).** The baggage helper tests cannot compile without the `baggage.go` API they exercise, so Task 2's test + feat were co-committed in `f01a8516` rather than split RED/GREEN. Tasks 1 and 3 have the standard separate `test(...)` → `feat(...)` gate commits.
- **libs/tracing dependency expansion.** Adding `SeedMiddleware` required chi + authz, and the producer counter required prometheus, in the previously otel-only `libs/tracing` module. `libs/authz` is an existing workspace module (relative `replace ../authz`, mirroring `libs/cache`→`libs/metrics`); standalone build verified with `GOWORK=off go build ./`. Because no NEW workspace module was added, the 13-Dockerfile COPY rule (MEMORY.md) does not apply.
- **Reverted `go work sync` cross-workspace churn.** `go work sync` rewrote all 45 workspace `go.mod`/`go.sum` files + `go.work.sum`, opportunistically bumping unrelated transitive deps (e.g. `golang.org/x/crypto` 0.44→0.46, `go.opentelemetry.io/otel` 1.38→1.39) across every service. This unrelated churn was reverted (`git checkout`) to avoid sweeping version bumps into services this plan does not touch (and avoid conflicts with parallel agents). `libs/tracing`'s own committed `go.mod`/`go.sum` are self-complete — verified by a clean `GOWORK=off go test ./` — so its new deps resolve without the workspace-wide changes.

## TDD Gate Compliance
- Task 1: `test(02-01)` (`e6da0256`, RED verified failing) → `feat(02-01)` (`f166977c`, GREEN). ✓
- Task 3: `test(02-01)` (`97df31cc`, RED verified failing) → `feat(02-01)` (`82315228`, GREEN). ✓
- Task 2: feat with co-committed tests (`f01a8516`) — library-API tests cannot compile pre-implementation; RED was observed locally before commit. Acceptable for pure helper APIs.

## Known Stubs
None. All code paths are wired: the handler shares the live batcher Sink, InsertBatch reads real Event fields, and the recording transport + producer are fully functional. `WrapTransport` global-sink composition is nil-safe (no sink installed yet — Wave 2 wires `SetGlobalSink` at BE service boot, by design).

## User Setup Required
None - no external service configuration required. (`ANALYTICS_INTERNAL_URL`-style wiring of the producer into BE services lands in Wave 2 plans.)

## Next Phase Readiness
- The dependency root for Phase 2 Waves 2-3 is in place: a working sink (`/internal/effects`), a recorder (`WrapRecording`/recordingTransport), a producer, and PII-safe attribution helpers.
- Wave 2 (general-egress wiring + retrofits) can now: call `SetGlobalSink(producer)` at BE boot, mount `SeedMiddleware` on each service router, and route non-shared HTTP clients through `WrapTransport`.
- T-02-PII strip + `TestNoUserIDOnOutboundWire` end-to-end assertion is scheduled for Plan 02-04 (per the threat register); the private-ctx-value foundation it relies on is delivered here.

---
*Phase: 02-be-egress-recorder*
*Completed: 2026-06-05*

## Self-Check: PASSED

- Created files all present: effects.go, baggage.go, effect.go, producer.go, SUMMARY.md, plus tests.
- All task commits present: e6da0256, f166977c, f01a8516, 97df31cc, 82315228 (+ docs f6f92a23).
- `libs/tracing` + `services/analytics` build and pass tests; `libs/tracing` verified standalone (`GOWORK=off`).
- Working tree clean (no unrelated workspace churn).
