# Phase 2: BE Egress Recorder - Research

**Researched:** 2026-06-05
**Domain:** Go HTTP transport instrumentation, OTel baggage, ClickHouse wide-event ingestion, HLS streaming proxy
**Confidence:** HIGH (all seams verified by reading actual code; otel baggage API confirmed in module cache + official docs)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (research HOW to implement, do NOT relitigate)

- **D-01:** Do NOT derive a logical "provider" from the outbound host for general egress. Raw **host** is the primary egress dimension. No host→provider mapping table.
- **D-02:** Streaming is the exception. Scraper stream providers route through arbitrary 3rd-party CDN hosts, so additionally tag an explicit **stream-provider** value (`nineanime`/`allanime`/`animepahe`/`miruro`/`animefever`/`gogoanime`/`animekai`) on the streaming path. `target = provider + host`; `provider` populated only for streaming, host always.
- **D-03:** HLS flush trigger = idle-timeout "session ended." In-memory per-session running totals (`bytes_in`, `bytes_out`, segment count, `duration_ms`) in the streaming process; a reaper goroutine emits ONE aggregated row when no segment seen for the idle window (default ~45s, planner may tune 30–60s).
- **D-04:** Session key = a token injected into the rewritten `.m3u8` (`?sess=…`), riding the existing URL-rewrite seam. Returning segment GETs carry it. NOT an IP+stream heuristic.
- **D-05:** Both `bytes_out` (client egress, the `io.Copy` sink) and `bytes_in` (upstream ingress, `resp.Body` source) counted at `proxy.go` `ProxyStream` (~:164) + `ProxyWithReferer` (~:591).
- **D-06:** On graceful shutdown, flush all open sessions. Hard crash = open-session totals lost (acceptable — awareness register, not billing). In-memory state per-streaming-process; fine on single host.
- **D-07:** `operation` in Phase 2 comes from baggage seeded at inbound middleware (`libs/tracing/middleware.go`). Auto stack-frame attribution (`runtime.Callers`) stays Phase 3 — OUT of scope.
- **D-08:** Host-only transport-swap for Kodik extractor, OpenSubtitles, idmapping — route their `http.Client` through `tracing.WrapTransport`; no further per-client edits.
- **D-09:** Plus an explicit stream-provider context tag threaded ONLY on the scraper/streaming path (`services/scraper` `BaseHTTPClient` + streaming HLS path).
- **D-10:** Recorder hands one row per outbound request to the Phase-1 `EventStore`. Async + batched + drop-on-full with a dropped-event metric. MUST never add latency to or fail a hot-path outbound request. `bytes`/`duration`/`status` measured at the `RoundTripper` boundary.

### Claude's Discretion
- Exact idle-timeout value (30–60s; default ~45s), session-token format/param name, counting-wrapper implementation, where the in-memory session map lives in the streaming service, and how the stream-provider tag is threaded (context key vs baggage).

### Deferred Ideas (OUT OF SCOPE)
- Auto operation discovery via `runtime.Callers` → Phase 3.
- `AggregatingMergeTree`/`SummingMergeTree` rollups → v2 (AR-V2-01).
- Host→provider enrichment for non-streaming egress → intentionally NOT built (D-01).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AR-EGRESS-01 | Effect recorder at `WrapTransport` outbound seam — one egress row per 3rd-party request (provider, host, status, bytes, duration) | §Architecture Pattern 1 (recorder RoundTripper); the seam is `libs/tracing/client.go:WrapTransport` which today is bare `otelhttp.NewTransport` — recorder wraps inside/around it |
| AR-EGRESS-02 | `origin` + `operation` + `user_id` ride OTel baggage from inbound middleware to recorder | §Architecture Pattern 2; `propagation.Baggage{}` already registered in composite propagator (`libs/tracing/tracing.go:41-43`); `authz.ClaimsFromContext` supplies `user_id` |
| AR-EGRESS-03 | Migrate 4 uninstrumented clients onto wrapped transport | §Standard Stack + §Architecture Pattern 3 (per-client retrofit shapes — note idmapping/kodikextract are dependency-free leaf modules; prefer transport INJECTION over making them import `libs/tracing`) |
| AR-EGRESS-04 | HLS proxy egress aggregated to one row per (stream-session, host) — never per segment | §Architecture Pattern 4 (session-token rewrite + in-memory tally + reaper) |
| AR-EGRESS-05 | Capture both `bytes_out` and `bytes_in` where proxy reads upstream | §Architecture Pattern 5 (dual counting wrappers around `io.Copy` sink + `resp.Body` source) |
</phase_requirements>

## Summary

This phase wires the **backend egress half** of the v4.0 Activity Register. Every third-party HTTP request a BE service makes should post exactly one dimensioned effect row into the Phase-1 ClickHouse wide-event store, recorded at the shared `libs/tracing` `WrapTransport` seam, with `{origin, operation, user_id}` riding OTel baggage from inbound middleware. HLS proxy traffic is aggregated to one row per (stream-session, host) so the register never drowns in per-segment noise.

**The single biggest finding — and the crux the planner must resolve first:** *there is no backend effect-ingestion path today.* The only ingestion endpoint is `POST /api/analytics/collect`, which is **FE-shaped** (a `wireEnvelope` of clickstream events keyed by `anonymous_id`/`session_id`) and **public/gateway-proxied**. The `domain.Event` struct has **no effect dimension fields** (`origin`/`operation`/`effect_kind`/`target`/`requests`/`bytes_in`/`bytes_out`/`duration_ms`) — those columns exist in the ClickHouse schema but are hard-coded to zero/default by `ClickHouseStore.InsertBatch`. So this phase must add: (a) effect fields to the domain event (or a parallel `EffectEvent` type + a store method), and (b) a new **internal** ingestion path (`POST /internal/effects` on analytics, reachable Docker-network-only at `http://analytics:8092`) plus a thin shared producer client that BE services call async/batched/drop-on-full.

The good news: every enabling primitive already exists. otel v1.38.0 (with the `baggage` package) is a dependency of `libs/tracing` and transitively of catalog/scraper/streaming. The composite propagator **already includes `propagation.Baggage{}`**, so baggage propagates FE→gateway→service over the wire for free. `authz.ClaimsFromContext(ctx).UserID` gives the user. `BaseHTTPClient` already exposes a `WithTransport(http.RoundTripper)` option. The HLS proxy already rewrites `.m3u8` segment URLs in `rewriteHLSURL` — the exact seam to inject `?sess=`. The Phase-1 batcher (`ingest.Batcher`) is the proven async/drop-on-full template to mirror.

**Primary recommendation:** Extend `libs/tracing` with (1) baggage seed/read helpers + a recording RoundTripper that reads baggage + measures status/bytes/duration and hands an effect to (2) a new shared async producer (`libs/tracing` sub-package or `libs/effects`) that POSTs batched effect rows to a new analytics `POST /internal/effects` endpoint, which enqueues onto the existing batcher. Add effect fields to `domain.Event` (lowest-churn) and populate the already-existing ClickHouse columns. Retrofit the 4 clients via transport injection (NOT by adding `libs/tracing` to the dependency-free leaf modules). Build the HLS aggregator as a streaming-process-local component keyed on the injected `?sess=` token.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Seed baggage `{origin, user_id, operation}` | BE inbound middleware (`libs/tracing/middleware.go`) | — | One seam covers every service; user_id from JWT claims already in ctx |
| Record general egress effect (host, status, bytes, duration) | BE outbound transport (`libs/tracing` recording RoundTripper) | — | One place covers every client on the shared transport (AR-EGRESS-01) |
| Ship effect rows to the store | New shared async producer → analytics `/internal/effects` | analytics batcher → EventStore | Cross-service: BE process can't write ClickHouse directly; EventStore lives in analytics |
| Persist effect row | Analytics service (`ClickHouseStore.InsertBatch`) | — | Phase-1 sink; columns already exist, just unpopulated |
| HLS per-session aggregation + reaper | Streaming process (in-memory map) | streaming → shared producer | Segment GETs are fresh browser requests with no baggage; session token is the only correlation key (D-04) |
| Dual byte counting (in/out) | Streaming proxy (`libs/videoutils/proxy.go`) | — | Only the proxy reads upstream `resp.Body` and writes client sink (AR-EGRESS-05) |
| Stream-provider tag | Scraper (provider known at orchestrator/construction) | streaming | CDN host hides provider (D-02); scraper owns provider identity |

## Standard Stack

### Core (all already present — NO new external packages required)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go.opentelemetry.io/otel/baggage` | v1.38.0 (sub-pkg of `otel`) | Seed/read `{origin,user_id,operation}` on `context.Context` | `[VERIFIED: module cache]` Part of the already-required `go.opentelemetry.io/otel` module — no go.mod change for services that have `libs/tracing` |
| `go.opentelemetry.io/otel/propagation` | v1.38.0 | `Baggage{}` propagator (wire FE→BE) | `[VERIFIED: libs/tracing/tracing.go:41-43]` Already in the registered composite propagator |
| `go.opentelemetry.io/contrib/.../otelhttp` | v0.63.0 | The transport `WrapTransport` already wraps | `[VERIFIED: libs/tracing/client.go]` Existing |
| `github.com/ClickHouse/clickhouse-go/v2` | v2.42.0 | Native batch insert (in analytics only) | `[VERIFIED: 01-02-SUMMARY.md]` Phase-1 dep, pinned to avoid go 1.25 bump |

### Supporting (existing project libs to reuse)
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `services/analytics/internal/ingest.Batcher` | Async, drop-on-full, periodic-flush enqueue→InsertBatch | The producer-side template AND the analytics-side sink; reuse, don't rebuild |
| `libs/logger` | Structured logging | Recorder errors logged WARN (never fatal — D-10) |
| `libs/httputil` | `OK`/`BadRequest`/CORS/RequestLogger | New `/internal/effects` handler |
| `github.com/google/uuid` | event_id | Effect-row identity |
| `libs/authz` (`ClaimsFromContext`) | user_id from JWT | Baggage seeding at inbound middleware |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| New `POST /internal/effects` HTTP endpoint on analytics | Direct ClickHouse write from each BE service | REJECTED — would make catalog/scraper/streaming depend on `clickhouse-go` + duplicate batcher; couples every service to the store. HTTP keeps the EventStore swap-seam intact. |
| Add effect fields to `domain.Event` | New parallel `domain.EffectEvent` + new `EventStore` method | Either works. Adding fields to `Event` is lowest churn (the CH column order in `InsertBatch` already reserves the slots); a parallel type is cleaner but needs a 2nd batcher path. Planner picks; lean toward extending `Event`. |
| Transport INJECTION into leaf libs | Make `libs/idmapping`/`libs/kodikextract` import `libs/tracing` | REJECTED for the two dependency-free leaf modules — see Pitfall 1. Injection avoids a go-directive bump + the 14-Dockerfile checklist + dragging otel into clean modules. |

**Installation:** None. No new external packages. (`baggage` is a sub-package of the already-required `go.opentelemetry.io/otel`.)

**Version verification:**
- `go.opentelemetry.io/otel v1.38.0` — `[VERIFIED: libs/tracing/go.mod:8]` direct require; baggage package present at `/root/go/pkg/mod/go.opentelemetry.io/otel@v1.38.0/baggage/`.
- `propagation.Baggage{}` registered — `[VERIFIED: libs/tracing/tracing.go:41-43]`.

## Package Legitimacy Audit

> No external packages are installed in this phase. All dependencies are already vendored in `go.work` modules (otel v1.38.0, clickhouse-go v2.42.0) and were legitimacy-audited in Phase 1. slopcheck not applicable — zero new registry installs.

| Package | Registry | Disposition |
|---------|----------|-------------|
| (none) | — | No new installs; reuse existing go.work deps |

**Packages removed due to slopcheck [SLOP]:** none
**Packages flagged [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                         INBOUND (browser/gateway → BE service)
  HTTP request ──► tracing.HTTPMiddleware (otelhttp: Extracts wire baggage)
                          │
                          ▼
            [NEW] baggage-seed middleware (libs/tracing/middleware.go)
              reads authz.ClaimsFromContext(ctx).UserID + route + origin
              writes baggage{origin, user_id, operation} onto ctx
                          │  ctx flows down handler→service→client
                          ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │  GENERAL EGRESS PATH                  STREAMING / HLS PATH         │
   │                                                                    │
   │  client.Do(req) with ctx              browser segment GET ?sess=T  │
   │       │                                      │ (NO baggage; fresh) │
   │       ▼                                      ▼                     │
   │ [NEW] recording RoundTripper        ProxyWithReferer (proxy.go)    │
   │  - base = otelhttp.NewTransport      - dual byte counters          │
   │  - read baggage.FromContext         - rewriteHLSURL injects ?sess= │
   │  - measure status/bytes/duration       into rewritten .m3u8        │
   │  - build effect row                  - tally into in-mem session   │
   │       │                                 map[sess]→{in,out,segs}     │
   │       │                                      │ reaper (idle ~45s)  │
   │       ▼                                      ▼ end-of-session       │
   │   one effect row                        ONE aggregated effect row  │
   │       │                                      │                      │
   │       └──────────────┬───────────────────────┘                     │
   │                      ▼                                              │
   │     [NEW] shared async producer (libs/effects or libs/tracing)     │
   │       - in-proc ring buffer, drop-on-full + dropped metric         │
   │       - periodic POST batch ──► http://analytics:8092/internal/effects
   └──────────────────────────────────────────────────────────────────┘
                          │
                          ▼
        [NEW] analytics POST /internal/effects handler
              parse JSON batch → batcher.Enqueue(domain.Event{effect…})
                          │
                          ▼
        ingest.Batcher (EXISTING, async/drop-on-full) → EventStore.InsertBatch
                          │
                          ▼
        ClickHouse `events` (effect columns already exist, finally populated)
```

### Recommended Project Structure
```
libs/tracing/
├── client.go            # extend: recording RoundTripper around otelhttp transport
├── middleware.go        # extend: seed baggage from claims+route
├── baggage.go           # NEW: typed Seed()/Read() helpers (origin/operation/user_id keys)
└── effect.go            # NEW: Effect struct + the recorder's row-builder
libs/effects/            # NEW shared module (OR a libs/tracing sub-pkg) — async producer
├── producer.go          # ring buffer + drop-on-full + periodic HTTP POST
└── go.mod               # if standalone → triggers the 14-Dockerfile checklist
services/analytics/internal/
├── handler/effects.go   # NEW: POST /internal/effects → batcher.Enqueue
├── domain/event.go      # extend Event with effect fields (or add EffectEvent)
└── transport/router.go  # add r.Post("/internal/effects", …)
services/streaming/internal/
└── service/hls_sessions.go  # NEW: in-mem session map + reaper + flush
libs/videoutils/proxy.go     # extend: ?sess= injection + dual byte counters
```

### Pattern 1: Recording RoundTripper (AR-EGRESS-01)
**What:** Wrap (or compose with) the existing `otelhttp.NewTransport` so each outbound request is measured and an effect row is produced.
**When to use:** The single seam in `libs/tracing/client.go` — every client on the shared transport inherits it.
```go
// Source: pattern derived from libs/tracing/client.go + otel baggage API (VERIFIED in module cache)
type recordingTransport struct {
    base http.RoundTripper // otelhttp.NewTransport(...)
    rec  EffectSink        // async producer; nil-safe
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    start := time.Now()
    resp, err := t.base.RoundTrip(req) // span + traceparent + baggage-inject happen here
    if t.rec == nil { return resp, err }

    // Read baggage seeded inbound (VERIFIED: baggage.FromContext / Member().Value()).
    bg := baggage.FromContext(req.Context())
    eff := Effect{
        Origin:    orDefault(bg.Member("origin").Value(), "api"),
        Operation: bg.Member("operation").Value(),
        UserID:    bg.Member("user_id").Value(),
        EffectKind: "egress",
        Host:      req.URL.Host,
        // Provider populated only on streaming path (D-02) via a ctx tag, not baggage.
        Provider:  providerTagFromContext(req.Context()),
        DurationMS: time.Since(start).Milliseconds(),
    }
    if resp != nil {
        eff.Status = resp.StatusCode
        // bytes_in: wrap resp.Body so the caller's reads increment the counter,
        // flushed when Body closes. (See note below — do NOT ReadAll here.)
        resp.Body = countingReadCloser(resp.Body, &eff.BytesIn, func() { t.rec.Record(eff) })
    } else {
        t.rec.Record(eff) // error path: no body, record immediately
    }
    return resp, err
}
```
**Critical:** Do NOT buffer/ReadAll the response body in the RoundTripper — it would break streaming and add latency (violates D-10). Wrap `resp.Body` in a counting `io.ReadCloser` and emit the effect on `Close()`. `target = provider + host` per Phase-1 shape (D-02).

### Pattern 2: Baggage seed + read (AR-EGRESS-02)
**What:** Seed `{origin, user_id, operation}` at inbound middleware; read in the RoundTripper.
**When to use:** Seed once per inbound request; the otelhttp composite propagator carries it cross-service automatically.
```go
// Source: go.opentelemetry.io/otel/baggage v1.38.0 (VERIFIED in module cache)
// NewMember validates the value (W3C baggage). Use NewMemberRaw for arbitrary
// values (route strings contain '/', '{', '}' which NewMember percent-handling
// tolerates, but raw is safest for operation strings). PITFALL: empty value
// makes NewMember error — guard non-empty before adding.
func SeedBaggage(ctx context.Context, origin, userID, operation string) context.Context {
    var members []baggage.Member
    add := func(k, v string) {
        if v == "" { return }
        if m, err := baggage.NewMemberRaw(k, v); err == nil { members = append(members, m) }
    }
    add("origin", origin); add("user_id", userID); add("operation", operation)
    bg, err := baggage.New(members...)
    if err != nil { return ctx }
    return baggage.ContextWithBaggage(ctx, bg)
}
```
`operation` in P2 is coarse: `service + " " + METHOD + " " + routePattern` (e.g. `catalog GET /anime/{id}/scraper/stream`) — D-07. Get the route pattern from chi (`chi.RouteContext(r.Context()).RoutePattern()`) inside the seed middleware, placed AFTER chi routing. `user_id` from `authz.ClaimsFromContext(ctx)` (`[VERIFIED: libs/authz/jwt.go:199,205]`).

### Pattern 3: Per-client retrofit (AR-EGRESS-03)
**What:** Route the 4 uninstrumented clients through the recording transport.
| Client | Owning service(s) | Current construction | Retrofit shape |
|--------|-------------------|---------------------|----------------|
| `services/scraper` `BaseHTTPClient` | scraper (7 providers) | `NewBaseHTTPClient(log, opts…)` per provider in `scraper main` | `WithTransport(tracing.WrapRecording(...))` already exists as an Option (`httpclient.go:98`) — but its doc says "tests only / production MUST NOT set"; **update the doc + wire it in production** with the recording transport. Per-provider construction means the stream-provider tag is known here (D-02/D-09). |
| OpenSubtitles | catalog | `opensubtitles.NewClient(Config{Timeout})` builds `&http.Client{Timeout}` internally (`client.go:65`) | Add a `Transport http.RoundTripper` (or `HTTPClient *http.Client`) field to `Config`; catalog passes a wrapped transport. Host-only (D-08). |
| idmapping (ARM/AniList) | catalog + scraper (miruro cache) | `idmapping.NewClient()` (no args) builds its own IPv4 transport (`client.go:79`) | Add a functional option `WithTransport(rt)` that **wraps** (not replaces) the IPv4 dialer transport. Callers inject `tracing.WrapRecording(ipv4Transport)`. Host-only (D-08). |
| Kodik extractor | catalog | `kodikextract.Resolve(ctx, url)` — **package-level func**, builds a per-call client (`extract.go:89`) | Hardest: no client injection point. Add `ResolveWithClient(ctx, url, *http.Client)` (or an options variant) and have catalog pass a recording-wrapped client; keep `Resolve` as a thin wrapper for back-compat. Host-only (D-08). |

**Why injection, not import:** `libs/idmapping` and `libs/kodikextract` are `go 1.22` modules with **zero dependencies** (`[VERIFIED: their go.mod]`). Making them `import "libs/tracing"` would (a) bump their go directive to 1.24, (b) pull the entire otel tree into two clean modules, and (c) trigger the 14-Dockerfile COPY checklist. Inject a `http.RoundTripper`/`*http.Client` from the owning service (catalog already has `libs/tracing`) instead. See Pitfall 1.

### Pattern 4: HLS session-token aggregation (AR-EGRESS-04)
**What:** One row per (session, host), keyed on a token injected into the rewritten manifest.
**When to use:** Streaming HLS path only.
```go
// Injection seam: libs/videoutils/proxy.go rewriteHLSURL (~:684) already builds
//   "/api/streaming/hls-proxy?url=…&referer=…&exp=…&sig=…"
// Add &sess=<token> here. The token is minted once per manifest rewrite
// (one watch = one master/variant fetch → one token → all its segments share it).
// On the segment GET, the HLSProxy handler reads r.URL.Query().Get("sess") and
// tallies into an in-process map keyed by (sess, upstreamHost).

type sessionTally struct {
    bytesIn, bytesOut uint64
    segments          uint32
    firstSeen, lastSeen time.Time
    host, provider    string
    operation, userID string // captured from the manifest fetch (which DID carry baggage)
}
// A reaper goroutine scans every ~10s; any tally idle > idleWindow (~45s) is
// flushed as ONE effect row (requests=segments, bytes_in/out summed,
// duration_ms = lastSeen-firstSeen) then deleted. Graceful shutdown flushes all (D-06).
```
**Key correlation insight:** the **manifest fetch** (master/variant `.m3u8`) IS made by the streaming process via `ProxyWithReferer` and *does* carry the inbound request's baggage (operation/user_id) — capture those into the `sessionTally` at token-mint time. The **segment GETs** are fresh browser requests with no baggage; they only carry `?sess=`, which is exactly why the token (not IP) is the grouping key (D-04). Concurrent same-NAT viewers get distinct tokens → exact grouping.

**Memory-leak guard:** the reaper MUST evict idle sessions (the whole point), but also cap map size and evict oldest on overflow so a flood of distinct tokens can't OOM the streaming process. Use a single `sync.Mutex` (or `sync.Map` + atomic counters) — never hold the lock across the `io.Copy` (would serialize all streams).

### Pattern 5: Dual byte counting (AR-EGRESS-05)
**What:** Count `bytes_out` (client sink) and `bytes_in` (upstream source) at the proxy.
```go
// bytes_out: the streaming handler ALREADY wraps w in metrics.CountingResponseWriter
//   (stream.go:197) — extend a parallel counter, or read crw's count post-copy.
// bytes_in: wrap resp.Body in a counting reader BEFORE io.Copy / rateLimitedCopy.
type countReader struct { r io.Reader; n *uint64 }
func (c *countReader) Read(p []byte) (int, error) {
    n, err := c.r.Read(p); atomic.AddUint64(c.n, uint64(n)); return n, err
}
// proxy.go ProxyStream:164  →  io.Copy(w, &countReader{resp.Body, &in})
// proxy.go ProxyWithReferer:591/593  →  same wrap before io.Copy / rateLimitedCopy
```
For HLS these per-segment counts feed the `sessionTally` (Pattern 4), not a per-segment row. For non-HLS direct video (`ProxyStream`), emit one effect row per proxied stream (it's already one request).

### Anti-Patterns to Avoid
- **ReadAll-ing response bodies in the RoundTripper** to count bytes — breaks streaming, adds latency, violates D-10. Wrap the body, count on read, emit on close.
- **Blocking the outbound request on the producer** — `Record()` must be a non-blocking channel send with drop-on-full (mirror `Batcher.Enqueue`).
- **Holding the session-map mutex across `io.Copy`** — serializes every concurrent stream. Lock only for the map update.
- **Making leaf libs import `libs/tracing`** — see Pitfall 1.
- **Deriving provider from host for general egress** — explicitly forbidden (D-01).
- **Per-segment effect rows** — explicitly forbidden (AR-EGRESS-04 / Out-of-Scope).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Async batched drop-on-full ingestion | A new buffering loop from scratch | Mirror `ingest.Batcher` (`services/analytics/internal/ingest/batcher.go`) | Proven, tested, exact semantics D-10 wants (non-blocking enqueue, periodic flush, drain-on-stop) |
| Baggage wire propagation FE→BE | Custom headers | `propagation.Baggage{}` (already registered) + otelhttp | W3C-standard, already wired; otelhttp injects/extracts automatically |
| ClickHouse insert | New CH writer in each BE service | `EventStore.InsertBatch` via analytics `/internal/effects` | Keeps the swap-seam; CH dep stays in analytics only |
| Response-byte counting | Manual Content-Length parsing | Counting `io.Reader`/`io.ReadCloser` wrapper | Content-Length is often absent/chunked; counting on read is exact |
| Stream-provider identity | host→provider lookup table | Provider `Name()` known at scraper construction, threaded via ctx tag | D-01 forbids the table; provider is in scope at the orchestrator |
| Client-byte counting (out) | New writer wrapper | Existing `metrics.CountingResponseWriter` (`libs/metrics/bandwidth.go`) | Already wraps the streaming response writer |

**Key insight:** ~80% of this phase is *composition of existing seams*. The genuinely new code is: the recording RoundTripper, the baggage seed helper, the cross-service producer + `/internal/effects` endpoint, the effect fields on `domain.Event`, and the HLS session aggregator. Everything else is wiring.

## Runtime State Inventory

> This is a feature/instrumentation phase, not a rename/refactor. Most categories are N/A, but the cross-service ingestion path and in-memory session state warrant explicit notes.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | ClickHouse `events` table effect columns (`origin`/`operation`/`effect_kind`/`target`/`requests`/`bytes_in`/`bytes_out`/`duration_ms`/`row_count`) already exist from Phase 1 but are written as zero/default for every row. | No migration — columns exist; this phase finally populates them. Verify column order in `InsertBatch` if `domain.Event` gains fields. |
| Live service config | New `POST /internal/effects` must NOT be gateway-proxied (Docker-network-only, mirrors analytics `/internal/erase`). New env var for the producer's analytics URL (e.g. `ANALYTICS_INTERNAL_URL=http://analytics:8092`) added to catalog/scraper/streaming compose entries. | Add router route + env vars to 3 services' compose blocks + `CLAUDE.md` env docs. |
| OS-registered state | None. | None — verified: no Task Scheduler / systemd / pm2 involvement. |
| Secrets/env vars | None new secrets. The producer URL is non-secret service discovery. | None. |
| Build artifacts | IF a new standalone `libs/effects` module is created → the **14-Dockerfile COPY checklist** (`MEMORY.md: Adding New libs/ Module`) applies (`go.work`, importing services' go.mod require+replace, all 14 service Dockerfiles, `go work sync`). Preferring a `libs/tracing` sub-package avoids this entirely. | Planner decides; recommend sub-package of `libs/tracing` to skip the checklist. |

## Common Pitfalls

### Pitfall 1: Dragging otel into dependency-free leaf modules
**What goes wrong:** Adding `import "libs/tracing"` to `libs/idmapping` or `libs/kodikextract` (currently `go 1.22`, zero deps) bumps their go directive to 1.24 and pulls the full otel + grpc + protobuf tree into clean modules, and triggers the 14-Dockerfile checklist.
**Why it happens:** The naive reading of D-08 ("route their http.Client through `tracing.WrapTransport`") implies the lib imports tracing.
**How to avoid:** Inject the wrapped transport from the owning service (catalog already depends on `libs/tracing`). Add a `WithTransport(http.RoundTripper)`/`HTTPClient` option to each leaf constructor; the service wraps and passes it in. `[VERIFIED: libs/idmapping/go.mod, libs/kodikextract/go.mod both have zero requires]`
**Warning signs:** `go work sync` rewriting `libs/idmapping/go.mod`'s go directive; otel appearing in its go.sum.

### Pitfall 2: The FE collect endpoint is the wrong ingestion path
**What goes wrong:** Trying to POST effect rows to `/api/analytics/collect` — they get parsed as `wireEnvelope` clickstream events (require `anonymous_id`/`session_id`, drop on `Validate()` failure) and the route is public/gateway-exposed.
**Why it happens:** It's the only existing ingestion endpoint; easy to assume it's general-purpose.
**How to avoid:** Add a separate `POST /internal/effects` (internal-only, never gateway-proxied — mirror `/internal/erase` at `router.go:56`). `[VERIFIED: collect.go wireEnvelope shape; router.go:55-56]`
**Warning signs:** Effect rows silently disappearing (validation drop) or 400s.

### Pitfall 3: `domain.Event` has no effect fields
**What goes wrong:** Assuming you can just build an effect row — the struct has no `Origin`/`Operation`/`EffectKind`/`Target`/`BytesIn`/`BytesOut`/`DurationMS`/`Requests` fields; the CH store hard-codes them to defaults (`clickhouse_store.go:103-134`).
**Why it happens:** Phase-1 reserved the columns but only wired clickstream rows.
**How to avoid:** Extend `domain.Event` with the effect fields (and update `InsertBatch`'s Append order — the column slots already exist in the DDL) OR add a parallel `EffectEvent` + a 2nd store path. Either way the schema is unchanged; only the Go-side mapping is new. `[VERIFIED: event.go has no effect fields; clickhouse_store.go:101-134 hardcodes them]`
**Warning signs:** Effect rows landing with empty `effect_kind`/zero measures despite being "recorded."

### Pitfall 4: Baggage member validation rejects empty/invalid values
**What goes wrong:** `baggage.NewMember(key, "")` errors; the whole `baggage.New(...)` then fails and the seed is silently dropped.
**Why it happens:** W3C baggage forbids empty values; route strings have special chars.
**How to avoid:** Guard non-empty before adding; use `NewMemberRaw` for operation/route strings; tolerate per-member errors without failing the whole set. `[VERIFIED: baggage.go:267 NewMember, :291 NewMemberRaw]`
**Warning signs:** `operation`/`user_id` empty in effect rows even though the inbound request had them.

### Pitfall 5: HLS session map memory leak / lock contention
**What goes wrong:** Unbounded session map (no reaper or no overflow cap) OOMs the streaming process; or holding the map mutex across `io.Copy` serializes all concurrent streams.
**Why it happens:** The reaper is the flush trigger (D-03) AND the GC; forgetting the overflow cap, or scoping the lock too widely.
**How to avoid:** Reaper evicts idle (>idleWindow) sessions; add a hard map-size cap with oldest-eviction; lock only around the map mutation, never the copy. `[VERIFIED: ProxyWithReferer streams under a semaphore, max 50 concurrent — stream.go:20,25]`
**Warning signs:** Streaming RSS climbing under load; p99 stream latency rising with concurrent viewers.

### Pitfall 6: Recorder adds latency or fails the request
**What goes wrong:** Synchronous POST-to-analytics or ReadAll inside RoundTrip — adds RTT/latency to every scrape/stream and can fail a hot-path request.
**Why it happens:** Treating "record" as a blocking call.
**How to avoid:** `Record()` = non-blocking channel send, drop-on-full + a dropped-counter metric (mirror `Batcher.Enqueue`/`observ.EventsDropped`). The HTTP ship-out happens on the producer's own goroutine. `[VERIFIED: batcher.go:56-66 Enqueue pattern]` (D-10)
**Warning signs:** Outbound request p99 tracking analytics availability; scrape failover starvation regressions.

## Code Examples

### Internal effects endpoint (analytics side)
```go
// Source: mirror of router.go:56 (/internal/erase) + collect.go handler shape (VERIFIED)
// transport/router.go:
r.Post("/internal/effects", effectsHandler.ServeHTTP) // Docker-network-only; gateway never proxies /internal/*

// handler/effects.go — parse a batch of effect rows → batcher.Enqueue(domain.Event{...})
// Reuse the same Sink interface (Enqueue(domain.Event) bool) the CollectHandler uses.
```

### Producer (BE side, non-blocking)
```go
// Source: mirror of ingest/batcher.go Enqueue (VERIFIED async/drop-on-full pattern)
func (p *Producer) Record(e Effect) {
    select {
    case p.ch <- e:
    default:
        p.dropped.Inc() // Prometheus counter on the BE service's /metrics
    }
}
// p.run(): accumulate up to MaxBatch or FlushInterval, then one HTTP POST of the batch.
```

### Stream-provider tag threading (D-02/D-09)
```go
// Provider Name() is known at scraper construction (scraper main builds one
// BaseHTTPClient per provider — main.go:131,176,254,…). Thread the tag via a
// ctx key set when the orchestrator dispatches to a provider, OR bake it into
// that provider's recording transport at construction (simplest: construction-time,
// since the client is per-provider). The recorder reads it for `target=provider+host`.
// Use a private ctx key type (NOT a string) to avoid collisions.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Coarse Prometheus egress counters (`libs/metrics/{external,parser,bandwidth}.go`) | Dimensioned per-request effect rows in ClickHouse | This milestone (v4.0) | Pivotable by any dimension; exact counts (Prometheus stays for RED/alerting) |
| Bytes are client-egress only, not per-host | Dual `bytes_in`/`bytes_out` per (session,host) | This phase | Upstream-cost visibility |
| `WrapTransport` = bare `otelhttp.NewTransport` (traceparent only) | + recording RoundTripper | This phase | One seam records every shared-transport client |

**Deprecated/outdated:**
- Relying on Tempo for counts — it tail-samples ~80% (design spec gap #4); the register is the counting source. (Tempo retirement is Phase 6, not here.)

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Extending `domain.Event` with effect fields is lower-churn than a parallel `EffectEvent` type | Standard Stack / Pitfall 3 | If the team prefers a clean separation, a parallel type + 2nd batcher path is the alternative — both are viable, planner's call |
| A2 | A new `POST /internal/effects` HTTP endpoint is the right cross-service ingestion shape (vs. a future Redis-stream/queue) | Summary / Pattern (crux) | If a queue is later wanted, the producer abstraction makes the transport swappable; HTTP is the minimal proven path matching the existing `/internal/erase` precedent |
| A3 | The stream-provider tag is best baked at scraper-construction time (per-provider client) rather than via baggage | Pattern 3 / Code Examples | If providers ever share one BaseHTTPClient, a ctx tag set at orchestrator dispatch is the fallback — both noted |
| A4 | Idle window default ~45s is acceptable; reaper scan ~10s | Pattern 4 | User explicitly granted discretion (30–60s); only risk is a too-long window delaying the row or a too-short one splitting a buffering pause into 2 rows |
| A5 | Recommend a `libs/tracing` sub-package for the producer to avoid the 14-Dockerfile checklist | Project Structure | If a standalone `libs/effects` is chosen, the checklist applies — flagged in Runtime State Inventory |

## Open Questions

1. **Effect-row shape: extend `Event` vs new `EffectEvent`?**
   - What we know: CH columns exist; `InsertBatch` hard-codes them. Both approaches keep the schema unchanged.
   - What's unclear: team preference for coupling clickstream + effect in one struct.
   - Recommendation: extend `Event` (lowest churn); revisit if the struct gets unwieldy.

2. **Does the analytics batcher need a separate buffer/metric for effects vs clickstream?**
   - What we know: one `Batcher` feeds one `EventStore`. Effects could share it or get a 2nd batcher.
   - Recommendation: share the existing batcher (one `InsertBatch` path); add an `effects_dropped` counter distinct from `analytics_events_dropped_total` only if drop-attribution matters.

3. **Operation string format for HLS (segment GETs have no inbound operation).**
   - What we know: the manifest fetch carries baggage; segments don't.
   - Recommendation: capture operation/user_id at token-mint (manifest fetch) into the `sessionTally`; the aggregated row inherits them.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `go.opentelemetry.io/otel/baggage` | Baggage seed/read | ✓ | v1.38.0 | — (in module cache + go.work) |
| `propagation.Baggage{}` propagator | Wire propagation | ✓ (registered) | v1.38.0 | — |
| ClickHouse `events` table effect columns | Effect persistence | ✓ | Phase-1 DDL | — |
| `ingest.Batcher` | Async ingestion template | ✓ | existing | — |
| `analytics` service @ `:8092` | Internal effects sink | ✓ | running | — |
| `authz.ClaimsFromContext` | user_id seeding | ✓ | existing | — |
| `BaseHTTPClient.WithTransport` | Scraper retrofit | ✓ | existing (`httpclient.go:98`) | — |

**Missing dependencies with no fallback:** none — all primitives present.
**Missing dependencies with fallback:** none.

## Validation Architecture

> nyquist_validation is enabled (config.json: `workflow.nyquist_validation: true`). This section is REQUIRED.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `testcontainers-go/modules/clickhouse v0.40.0` (real CH, Phase-1 precedent) |
| Config file | none (Go convention; per-package `*_test.go`) |
| Quick run command | `go test ./internal/... -short -count=1` (per touched module) |
| Full suite command | `go test ./... -count=1` (+ `-run TestClickHouse` with Docker for CH-backed tests) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AR-EGRESS-01 | One outbound call → one effect row (host/status/bytes/duration) | integration | `go test ./libs/tracing/ -run TestRecordingTransport -count=1` | ❌ Wave 0 |
| AR-EGRESS-02 | Baggage seeded inbound is read in the recorder (origin/operation/user_id) | unit | `go test ./libs/tracing/ -run TestBaggageSeedRead -count=1` | ❌ Wave 0 |
| AR-EGRESS-02 | End-to-end: inbound middleware → outbound RoundTripper carries operation | integration | `go test ./libs/tracing/ -run TestBaggageE2E -count=1` (httptest inbound + outbound) | ❌ Wave 0 |
| AR-EGRESS-03 | Each retrofit client's outbound is recorded | integration | `go test ./libs/idmapping/ ./libs/kodikextract/ ./services/catalog/internal/parser/opensubtitles/ ./services/scraper/internal/domain/ -run Transport -count=1` | ❌ Wave 0 |
| AR-EGRESS-04 | N segment GETs under one `?sess=` → exactly ONE aggregated row after idle flush | integration | `go test ./services/streaming/internal/service/ -run TestHLSSessionAggregation -count=1` | ❌ Wave 0 |
| AR-EGRESS-05 | bytes_in (upstream) and bytes_out (client) both non-zero and distinct | integration | `go test ./libs/videoutils/ -run TestDualByteCount -count=1` | ❌ Wave 0 |
| (sink) | `/internal/effects` → batcher → CH row with effect dims populated | integration | `go test ./services/analytics/internal/... -run TestEffectsIngest -count=1` (CH testcontainer) | ❌ Wave 0 |
| (non-block) | Producer drops on full buffer, never blocks, increments dropped metric | unit | `go test ./libs/tracing/ -run TestProducerDropOnFull -count=1` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./<touched-package>/... -short -count=1` (< 30s)
- **Per wave merge:** full per-module `go test ./... -count=1` (CH tests with Docker)
- **Phase gate:** all five AR-EGRESS tests + the sink + non-block tests green before `/gsd:verify-work`; `make redeploy-{catalog,scraper,streaming,analytics}` + `make health` smoke.

### Wave 0 Gaps
- [ ] `libs/tracing/client_test.go` — recording transport, byte/duration/status capture, drop-on-full producer (AR-EGRESS-01, non-block)
- [ ] `libs/tracing/middleware_test.go` (extend) — baggage seed from claims+route; E2E inbound→outbound (AR-EGRESS-02)
- [ ] `services/analytics/internal/handler/effects_test.go` — `/internal/effects` → batcher (sink)
- [ ] `services/streaming/internal/service/hls_sessions_test.go` — session aggregation + idle flush + reaper eviction (AR-EGRESS-04)
- [ ] `libs/videoutils/proxy_test.go` (extend) — dual byte count + `?sess=` injection in `rewriteHLSURL` (AR-EGRESS-05)
- [ ] Retrofit transport-injection tests per leaf client (AR-EGRESS-03)
- Framework install: none — Go testing + testcontainers already present from Phase 1.

## Security Domain

> security_enforcement not set to false in config → enabled. Scoped to this phase's surface.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No new auth; reuses existing JWT claims (read-only) |
| V3 Session Management | partial | The HLS `?sess=` token is an internal correlation id, NOT an auth/session credential — must be unguessable-enough to avoid cross-viewer tally collisions but carries NO authority. Use a random token; do NOT derive from user_id/PII. |
| V4 Access Control | yes | `POST /internal/effects` MUST be Docker-network-only (never gateway-proxied) — mirror `/internal/erase`. Verify the gateway router does NOT add a route for it. |
| V5 Input Validation | yes | The effects endpoint parses a JSON batch from internal services — still validate shape, cap body size (mirror collect.go's 256KB `LimitReader`), bound array length. |
| V6 Cryptography | no | No new crypto. The existing provenance HMAC (`signProvenance`) is unchanged; `?sess=` is non-cryptographic correlation. |

### Known Threat Patterns for {Go HTTP instrumentation + ClickHouse ingestion}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Baggage leaking PII over the wire to 3rd parties | Information disclosure | otelhttp injects baggage on OUTBOUND requests — `user_id` would be sent to external CDNs/APIs in a `baggage:` header. **MITIGATION REQUIRED:** strip/clear baggage on requests leaving to 3rd parties, OR only seed `user_id` for internal hops, OR use a separate non-propagated ctx value for user_id and only put non-PII (origin/operation) in wire baggage. Flag for the planner — this is the sharpest security finding. |
| `/internal/effects` exposed via gateway | Elevation/Tampering | Docker-network-only; assert no gateway route (V4) |
| Unbounded effect/session memory → DoS | Denial of service | drop-on-full producer + capped+reaped session map (Pitfall 5/6) |
| SQL/CH injection via effect dims | Tampering | Parameterized `PrepareBatch.Append` (already used, `clickhouse_store.go`) — no string interpolation |
| `?sess=` token guessing to corrupt another viewer's tally | Tampering | Random per-manifest token; worst case = mis-attributed awareness data (not security-critical), but use crypto/rand to avoid collisions |

**Sharpest finding for the planner:** baggage propagates on OUTBOUND wire requests by design. Putting raw `user_id` in W3C baggage means it is transmitted to every external host the recorder instruments (Kodik, OpenSubtitles, ARM, AniList, CDNs). The recorder reads baggage from context **in-process** — it does NOT need user_id on the wire. Recommend: carry `user_id` via a **private in-process ctx value** (not baggage), and keep only non-PII `origin`/`operation` in propagated baggage; OR clear baggage on 3rd-party-bound requests. Plan a task + test for this.

## Sources

### Primary (HIGH confidence)
- Codebase (read directly): `libs/tracing/{client,middleware,tracing,setup}.go`, `libs/videoutils/proxy.go`, `services/analytics/internal/{domain,ingest,handler,transport,repo,observ}/*`, `services/scraper/internal/domain/httpclient.go`, `services/scraper/cmd/scraper-api/main.go`, `services/streaming/internal/handler/stream.go`, `libs/{idmapping,kodikextract}/{client,extract}.go` + go.mod, `services/catalog/internal/parser/opensubtitles/client.go`, `libs/authz/jwt.go`, `libs/metrics/bandwidth.go`.
- `go.opentelemetry.io/otel@v1.38.0/baggage/{baggage,context}.go` (module cache) — `NewMember`/`NewMemberRaw`/`New`/`ContextWithBaggage`/`FromContext`/`Member().Value()` signatures.
- `.planning/phases/01-clickhouse-foundation-eventstore-swap/01-02-SUMMARY.md` — EventStore impl, `row_count` column note, dep pins.
- `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md` — wide-event model, HLS discipline, P2/P3 boundary.

### Secondary (MEDIUM confidence)
- [otelhttp transport docs](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) — Transport propagates baggage via headers before the base RoundTripper.
- [opentelemetry-go-contrib transport.go](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/transport.go) — confirms inject-on-outbound behavior.
- [OpenTelemetry Baggage spec](https://opentelemetry.io/docs/concepts/signals/baggage/) — baggage is added to outgoing requests; PII caution.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all deps verified present in go.work/module cache; zero new external packages.
- Architecture: HIGH — every seam read directly; the crux (no BE ingestion path) confirmed by reading collect.go + router.go + every BE service's analytics access.
- Pitfalls: HIGH — leaf-module dep risk, missing effect fields, FE-shaped collect endpoint, and baggage-PII-on-wire all verified against actual code/specs.

**Research date:** 2026-06-05
**Valid until:** 2026-07-05 (stable internal codebase; otel API stable; re-verify if otel or clickhouse-go is bumped)

## RESEARCH COMPLETE

**Phase:** 02 - BE Egress Recorder
**Confidence:** HIGH

### Key Findings
- **The crux: no BE effect-ingestion path exists.** `/api/analytics/collect` is FE-shaped + public; `domain.Event` has no effect fields (CH columns exist but are hard-coded to defaults). This phase must add a new internal `POST /internal/effects` endpoint + a shared async producer + effect fields on the event. This is the dependency root for every other task.
- **All enabling primitives already exist:** otel `baggage` (v1.38.0) is in `go.work`; `propagation.Baggage{}` is already registered (wire propagation is free); `authz.ClaimsFromContext` gives user_id; `BaseHTTPClient.WithTransport` exists; `rewriteHLSURL` is the exact `?sess=` injection seam; `ingest.Batcher` is the async/drop-on-full template to mirror.
- **Leaf-module trap:** `libs/idmapping` and `libs/kodikextract` are `go 1.22` zero-dependency modules — retrofit them via transport INJECTION from the owning service (catalog), NOT by importing `libs/tracing` (would bump go directive + trigger the 14-Dockerfile checklist).
- **Security: baggage rides OUTBOUND wire requests** — raw `user_id` in baggage would be sent to every external host. Carry user_id via a private in-process ctx value (not propagated baggage), or strip baggage on 3rd-party-bound requests. Plan a task + test.
- **Client ownership map:** catalog owns kodik/opensubtitles/idmapping; scraper owns BaseHTTPClient (7 providers) + idmapping(miruro); streaming owns the HLS proxy. Redeploy targets: catalog, scraper, streaming, analytics.

### File Created
`.planning/phases/02-be-egress-recorder/02-RESEARCH.md`

### Confidence Assessment
| Area | Level | Reason |
|------|-------|--------|
| Standard Stack | HIGH | Zero new packages; all verified in go.work/module cache |
| Architecture | HIGH | Every seam read directly; ingestion gap confirmed |
| Pitfalls | HIGH | Leaf-dep, missing effect fields, FE-collect mismatch, baggage-PII all code-verified |

### Suggested Wave/Dependency Structure (for the planner)
- **Wave 1 (foundation, sequential root):** effect fields on `domain.Event` + `/internal/effects` endpoint + shared async producer + baggage seed/read helpers. Everything else depends on this sink existing.
- **Wave 2 (parallel — no shared files):**
  - (a) recording RoundTripper in `libs/tracing` + general-egress wiring.
  - (b) HLS aggregator in `services/streaming` (`?sess=` injection in `libs/videoutils/proxy.go` + session map + reaper + dual byte counters).
  - (c) the 4 client retrofits (idmapping, kodikextract, opensubtitles via catalog; BaseHTTPClient via scraper) — independent files per client; the scraper one also threads the stream-provider tag (D-02/D-09).
- **Wave 3 (integration + hardening):** end-to-end baggage test, baggage-PII strip task + test, redeploy + smoke (`make redeploy-{catalog,scraper,streaming,analytics}` + `make health`).
- **Caution:** (a) and (c) both touch `libs/tracing` if the recording transport and the inject helper share a file — split into `client.go` (transport) vs `baggage.go` (helpers) so Wave-2 work doesn't collide.

### Project Convention Note (UXΔ/CDI/MVQ — no days/hours)
Per `CLAUDE.md` + `.planning/CONVENTIONS.md`, the planner MUST score plans on UXΔ / CDI / MVQ and never in days/hours/sprints. Suggested phase-level framing for the planner to refine: **UXΔ = +1 (Better)** (internal observability, indirect user benefit); **CDI = (Spread × Shift) * Effort_Fib** with Spread moderate (touches catalog, scraper, streaming, analytics, `libs/tracing`, `libs/videoutils`) and a non-trivial Effort_Fib (new ingestion path + 4 retrofits + HLS aggregator); **MVQ = Kraken** (a many-tentacled recorder reaching into every service's egress path), match/slop-resistance high (built on existing seams). Do NOT pre-multiply CDI.
