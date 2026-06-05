# Phase 2: BE Egress Recorder - Context

**Gathered:** 2026-06-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Every third-party request the **backend** makes posts exactly **one dimensioned egress effect row** into the Phase-1 ClickHouse wide-event store, recorded at the shared `libs/tracing` `WrapTransport` outbound seam. Inbound `{origin, user_id, operation}` ride OTel **baggage** to the recorder. The four currently-uninstrumented outbound clients are migrated onto the wrapped transport. HLS proxy egress is aggregated to **one row per (stream-session, host)** (never per ~6s segment), capturing both `bytes_out` (to client) and `bytes_in` (from upstream).

**Explicitly NOT in this phase** (later): DB/cache effects + **auto operation discovery via `runtime.Callers` stack-frame attribution** (Phase 3), FE causation + RUM (Phase 4), Grafana reports (Phase 5), Tempo/Loki→ClickHouse consolidation (Phase 6). This phase only wires the **BE egress half** of the register and proves it end-to-end.
</domain>

<decisions>
## Implementation Decisions

### Provider / target attribution (USER-LOCKED)
- **D-01:** Do **NOT** derive a logical "provider" from the outbound host for general egress. The raw **host** is the primary/most-meaningful egress dimension for most requests (idmapping ARM/AniList, OpenSubtitles, Jimaku, Kodik). No host→provider mapping table.
- **D-02:** **Streaming is the exception.** Scraper stream providers (nineanime / allanime / animepahe / miruro / animefever / gogoanime / animekai) route through arbitrary **3rd-party CDN hosts**, so the host alone hides which logical provider served the stream. On the streaming path, additionally tag an explicit **stream-provider** value so the register can pivot streaming egress **by stream-provider AND by host**. (Carried on the `target`/dimension shape from Phase 1: `target = provider + host`; here `provider` is populated only for streaming, host always.)

### HLS session aggregation (AR-EGRESS-04 / -05)
- **D-03:** **Flush trigger = idle-timeout "session ended."** Keep per-session running totals (`bytes_in`, `bytes_out`, segment `requests`/count, `duration_ms`) in an in-memory map in the streaming process; a reaper goroutine emits **one** aggregated effect row when no segment has been seen for the idle window (default ~45s, planner may tune 30–60s). Accuracy-first / one-row-per-watch, matches the milestone's awareness goal. Totals land at session end.
- **D-04:** **Session key = token injected into the rewritten `.m3u8`.** The proxy already rewrites HLS segment URLs when serving a manifest — inject a per-manifest **session token** (e.g. `?sess=…`) so returning segment GETs carry it. Grouping is exact even for concurrent same-IP / shared-NAT viewers. Tie to the existing URL-rewrite seam; do NOT use an IP+stream heuristic.
- **D-05:** Both `bytes_out` (client egress) and `bytes_in` (upstream ingress) are counted where the proxy reads upstream — via counting wrappers around the `io.Copy` sink and the upstream `resp.Body` source (proxy.go:164 `ProxyStream`, :591 `ProxyWithReferer`).
- **D-06 (default, not asked):** On **graceful shutdown**, flush all open sessions. On a **hard crash**, open-session totals are lost — acceptable for an awareness register (this is not billing). In-memory session state is per-streaming-process; fine on the single-host deployment.

### `operation` dimension — Phase 2 scope boundary
- **D-07:** In Phase 2, `operation` is populated from **baggage seeded at the inbound route/middleware** (`libs/tracing/middleware.go`) — coarse but non-empty (e.g. `catalog GET /anime/{id}/scraper/stream`). **Auto stack-frame attribution (`runtime.Callers` → nearest `*/internal/service/*` frame) stays Phase 3** per the design doc. No scope creep into P3.

### Retrofit approach for the 4 uninstrumented clients (AR-EGRESS-03)
- **D-08:** **Host-only transport-swap** for clients where the host suffices — Kodik extractor (`libs/kodikextract`), OpenSubtitles (`services/catalog/internal/parser/opensubtitles`), idmapping (`libs/idmapping`): route their `http.Client` through `tracing.WrapTransport` so the recorder records host + baggage; no further per-client edits.
- **D-09:** **Plus an explicit stream-provider context tag** threaded ONLY on the scraper / streaming path (`services/scraper` `BaseHTTPClient` + the streaming HLS path), since the 3rd-party CDN host can't reveal the provider (see D-02). Minimal edits elsewhere.

### Recorder mechanics (carried forward from Phase 1 — not re-asked)
- **D-10:** The egress recorder is the place where one egress row per outbound request is built and handed to the Phase-1 `EventStore`. Ingestion stays **async + batched + drop-on-full with a dropped-event metric** (Phase-1 philosophy) — the recorder must **never add latency to or fail** a hot-path outbound request (scraping/streaming). `bytes`/`duration`/`status` are measured at the `RoundTripper` boundary.

### Claude's Discretion
- Exact idle-timeout value (30–60s; default ~45s), session-token format/param name, counting-wrapper implementation, where the in-memory session map lives in the streaming service, and how the stream-provider tag is threaded (context key vs baggage) — planner/researcher decide, consistent with the locked decisions above.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Milestone design + requirements
- `docs/superpowers/specs/2026-06-04-analytics-activity-register-design.md` — the v4.0 design: wide-event model, baggage `{origin, user_id, operation}` seeded at inbound middleware, recorder at `WrapTransport`, HLS one-row-per-(session,host) discipline, and the explicit statement that auto operation discovery (stack-frame) is the **Phase 3** deliverable.
- `.planning/REQUIREMENTS.md` §AR-EGRESS-01..05 — the five locked requirements this phase satisfies.
- `.planning/ROADMAP.md` → "Phase 2: BE Egress Recorder" — goal, success criteria, dependency on Phase 1.

### Phase 1 carry-forward (the sink this phase writes to)
- `.planning/phases/01-clickhouse-foundation-eventstore-swap/01-CONTEXT.md` — locked wide-event schema: dimensions (`origin`, `operation`, `effect_kind`, `target_kind`, `target`, `trace_id`, `session_id`, nullable `user_id`/`anime_id`, `source`, `accuracy`) + measures (`requests`, `bytes_in`, `bytes_out`, `duration_ms`, `rows`).
- `.planning/phases/01-clickhouse-foundation-eventstore-swap/01-02-SUMMARY.md` — the `EventStore` impl: `OpenClickHouse`, `NewClickHouseStore`, `EnsureSchema`, `InsertBatch`, the unchanged `domain.EventStore` interface (the seam the recorder feeds).

### Code seams (integration points)
- `libs/tracing/client.go` — `WrapTransport(base http.RoundTripper)`; today only injects traceparent and is used only by `services/gateway/internal/service/proxy.go`. The recorder hooks here.
- `libs/tracing/middleware.go` — inbound middleware; where baggage `{origin, user_id, operation}` is seeded (greenfield — baggage used nowhere yet).
- `libs/videoutils/proxy.go` — HLS proxy; `ProxyStream` (`io.Copy` @164) + `ProxyWithReferer` (@591), `isDomainAllowed`/allowlist, existing HLS URL-rewrite logic (where the session token is injected).
- Retrofit clients: `libs/kodikextract/extract.go`, `services/scraper` `BaseHTTPClient` (e.g. `services/scraper/internal/providers/gogoanime/client.go`, `internal/embeds/megacloud.go`), `services/catalog/internal/parser/opensubtitles/client.go`, `libs/idmapping/client.go`.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `tracing.WrapTransport` / `tracing.NewClient` — the single shared outbound seam; extend its RoundTripper to record the egress effect. One place covers every client that adopts it.
- Phase-1 `EventStore` (`OpenClickHouse`/`NewClickHouseStore`/`InsertBatch`, async batcher) — the recorder's sink; reuse, don't rebuild.
- HLS URL-rewrite in `libs/videoutils/proxy.go` — already mutates `.m3u8` segment URLs; the session-token injection (D-04) rides this existing rewrite.
- Proxy allowlist with provenance host-families (kwik.cx, fast4speed.rsvp, am.vidstream.vip, ultracloud.cc, my.1anime.site, jimaku.cc, cdnlibs.org, hanime CDNs…) — useful as a host inventory, but per D-01 NOT turned into a host→provider map.

### Established Patterns
- Async + batched + drop-on-full ingestion (Phase 1) — the recorder must follow it; never block/fail the outbound request.
- `WrapTransport` composition pattern: `t.Transport = tracing.WrapTransport(t.Transport)` — the retrofit shape for the 4 clients (D-08).
- Adding a new `libs/` module triggers the 13-Dockerfile COPY checklist (memory) — only relevant if a new shared lib is introduced; prefer extending `libs/tracing`.

### Integration Points
- Inbound middleware (`libs/tracing/middleware.go`) → seeds baggage → rides context → read at `WrapTransport` recorder → builds egress row → Phase-1 `EventStore.InsertBatch`.
- Streaming path: scraper picks a stream-provider → outbound to 3rd-party CDN host → recorder records host + the explicit stream-provider tag (D-02/D-09); HLS segments aggregate per (session-token, host) in the streaming process (D-03/D-04).

</code_context>

<specifics>
## Specific Ideas

- User's exact framing on provider attribution: *"Don't derive them — outbound host is more important for most requests; for streaming add an option to group by Stream Provider (nineanime/allanime etc.) as they may use 3rd-party domains."* → D-01 / D-02.
- "One row per watch" intent for HLS (idle-timeout end-of-session, exact session token in the rewritten manifest) → D-03 / D-04.

</specifics>

<deferred>
## Deferred Ideas

- **Auto operation discovery via `runtime.Callers` stack-frame attribution** → Phase 3 (kept out of P2 by D-07).
- **AggregatingMergeTree / SummingMergeTree pre-aggregation rollups** for dashboards → v2 (AR-V2-01), already deferred in Phase 1.
- Host→provider enrichment for non-streaming egress → intentionally NOT built (D-01); revisit only if a future report needs it.

None of the above expand Phase 2 scope — discussion stayed within the egress boundary.

</deferred>

---

*Phase: 02-be-egress-recorder*
*Context gathered: 2026-06-05*
