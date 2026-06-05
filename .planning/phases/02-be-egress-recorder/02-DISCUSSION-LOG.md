# Phase 2: BE Egress Recorder - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-05
**Phase:** 02-be-egress-recorder
**Areas discussed:** Provider attribution, HLS session aggregation (flush + key), operation scope boundary, retrofit invasiveness

---

## Provider attribution

| Option | Description | Selected |
|--------|-------------|----------|
| Host→provider map (reuse proxy allowlist families) | Derive a logical provider from the outbound host | |
| Raw host only | Record host as the egress dimension; no derivation | ✓ (general) |
| Explicit per-client call-site tag | Each client passes provider explicitly | ✓ (streaming only) |

**User's choice (free-text):** *"Don't derive them — outbound host is more important there for most requests; for streaming add an option to group by Stream Provider (nineanime/allanime etc) as they may use 3rd party domains."*
**Notes:** General egress → record host, no mapping table. Streaming → also tag the logical stream-provider because scraper providers route through arbitrary 3rd-party CDN hosts. → CONTEXT D-01/D-02, and shaped the retrofit answer (D-09).

---

## HLS session aggregation — flush trigger

| Option | Description | Selected |
|--------|-------------|----------|
| Idle-timeout = session end | In-memory totals; reaper flushes one row after ~30–60s idle | ✓ |
| Periodic / incremental | Flush partial rows every N segments / T seconds | |
| Hybrid (periodic + idle close) | Checkpoints during long sessions + final idle close | |

**User's choice:** Idle-timeout = session end (Recommended)
**Notes:** One clean row per watch, accuracy-first, matches awareness goal. → CONTEXT D-03.

---

## HLS session aggregation — session key

| Option | Description | Selected |
|--------|-------------|----------|
| Session token in rewritten .m3u8 | Inject per-manifest token via existing URL-rewrite; segments carry it back | ✓ |
| Heuristic (client-IP + stream + user_id) | Derive key from request attributes, no URL rewrite | |

**User's choice:** Session token in rewritten .m3u8 (Recommended)
**Notes:** Exact grouping even for concurrent same-IP / shared-NAT viewers; rides the existing proxy rewrite seam. → CONTEXT D-04.

---

## `operation` dimension — Phase 2 scope boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Baggage/route now, stack-frame in P3 | Coarse operation from inbound middleware; auto stack-frame attribution stays Phase 3 | ✓ |
| Pull stack-frame attribution into P2 | Bring runtime.Callers service-frame discovery forward now | |

**User's choice:** Baggage/route now, stack-frame in P3 (Recommended)
**Notes:** Non-empty operation now without absorbing Phase 3 scope. → CONTEXT D-07.

---

## Retrofit invasiveness (4 uninstrumented clients)

| Option | Description | Selected |
|--------|-------------|----------|
| Host-only swap + stream-provider tag on streaming | WrapTransport for all 4; explicit provider tag only on scraper/streaming path | ✓ |
| Explicit provider+operation tags everywhere | Tag at every client construction | |

**User's choice:** Host-only swap + stream-provider tag on streaming (Recommended)
**Notes:** Minimal edits where host suffices (idmapping, OpenSubtitles, Kodik); explicit tag only where the CDN host hides the provider. Consistent with the provider-attribution decision. → CONTEXT D-08/D-09.

---

## Claude's Discretion

- Exact idle-timeout value (default ~45s, 30–60s range), session-token param name/format, counting-wrapper implementation, in-memory session-map location, and context-key-vs-baggage threading of the stream-provider tag — planner/researcher, consistent with locked decisions.
- Graceful-shutdown flush of open sessions; accept open-session loss on hard crash (awareness register, not billing) — CONTEXT D-06.

## Deferred Ideas

- Auto operation discovery via `runtime.Callers` stack-frame attribution → Phase 3.
- AggregatingMergeTree/SummingMergeTree pre-aggregation rollups → v2 (AR-V2-01).
- Host→provider enrichment for non-streaming egress → intentionally not built (revisit only if a report needs it).
