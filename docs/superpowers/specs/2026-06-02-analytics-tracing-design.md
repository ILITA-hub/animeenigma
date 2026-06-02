# Spec: Analytics & Distributed Tracing (AnimeEnigma-shaped)

**Date:** 2026-06-02
**Status:** Approved design — ready for implementation planning
**Author:** brainstormed with project owner

## 0. Context & Goals

AnimeEnigma has a **mature ops-metrics stack** (Prometheus + Loki + Grafana) but two real gaps:

1. **No end-to-end tracing.** Each service mints its own `X-Request-ID` and the gateway does **not** propagate it downstream — so a single user action cannot be followed across services, and a frontend click cannot be joined to the backend work it triggered. `libs/tracing/tracing.go` already contains the full OpenTelemetry SDK but is **dormant**: no service calls `tracing.New()` and no collector exists. `libs/logger.WithContext()` already injects `trace_id`/`span_id` when a span is present (lines 84–93) — but the span is never valid because tracing is never initialized.
2. **No product analytics.** User behavior lives in scattered, normalized Postgres tables (`activity_events`, `rec_events`, `watch_progress`) read by DB-polling Go collectors. The frontend is a behavioral black box — the only client-side tracking is the single drop-off beacon in `useWatchSession.ts`. There is no way to answer: which buttons get clicked and how often, retention, time-on-page, or anonymous-vs-identified comparison.

This spec closes both gaps with the **least new infrastructure that fits our stack** (Go, Postgres, the existing Grafana). It deliberately diverges from the reference spec "Self-Hosted Analytics & Observability Stack" in three ways:

| Reference spec | This design | Why |
|---|---|---|
| Node 20 + Fastify ingestion/consumer | **Go** `services/analytics/` | We are a 13-service Go shop; the only Node service is the Puppeteer sidecar (a deliberate exception). |
| ClickHouse for everything | **Postgres** clickstream + keep Prometheus/Loki/Tempo | At <500 users ClickHouse is overkill and the heaviest box on the host; consolidating working systems into it is a big migration with zero user payoff. |
| OTel → ClickHouse for traces/metrics/logs | **OTel → Tempo** for traces only | Metrics (Prometheus) and logs (Loki) already work and are wired into Grafana. Tempo is the trace-native sibling, ~256 MB, native Grafana correlation. |

### Locked decisions (from brainstorming, 2026-06-02)

- **Scale target:** small group, <500 users → Postgres-first, no ClickHouse, no Redis-Streams buffer.
- **Click capture:** autocapture everything (delegated listener + PII-stripping).
- **Tracing depth:** full distributed tracing via OTel Collector + Grafana Tempo.
- **Privacy:** collect-by-default + hygiene (no blocking consent banner). Audience is Russian (152-ФЗ), not primarily GDPR.

### Non-goals (explicitly deferred)

- **ClickHouse consolidation** — parked as a future "analytics refactor" TODO. The `EventStore` interface is the swap seam. Revisit when event volume justifies columnar storage and native `windowFunnel()`/`retention()` aggregates.
- **GlitchTip / error tracking** — separate decision; we have none today.
- **OTel browser RUM (in-browser spans)** — v1 trace roots at the gateway; click↔trace linked via `trace_id` stamped on events.
- **Redis-Streams ingestion buffer** — only needed at higher volume / with ClickHouse.

## 1. Architecture

```
TRACING (new)
  Browser ──traceparent header──► Gateway ──(otelhttp, propagates)──► 13 Go services
                                                    │ OTLP
                                                    ▼
                                            OTel Collector ──OTLP──► Tempo (traces; storage = existing MinIO)
                                                                       │
PRODUCT ANALYTICS (new)                                                │
  Browser snippet ──POST /collect (sendBeacon)──► services/analytics/  │
        (autocapture clicks, pageviews, heartbeat)      │ batched      │
                                                        ▼ INSERT       │
                                                   Postgres            │
                                                 (analytics_events,    │
                                                  analytics_identities)│
EXISTING (untouched): Prometheus (metrics), Loki + promtail (logs) ────┤
                                                                       ▼
                                                              Grafana (one UI:
                                                              metrics + logs + traces + clickstream)
```

**Correlation glue:** one shared `trace_id` appears in Tempo (spans), Loki (logs — already injected by `libs/logger`), and Postgres (`analytics_events.trace_id`). One id joins click → backend trace → logs.

**Net new infra:** 2 containers (`otel-collector`, `tempo`) + 1 Go service (`analytics`). Tempo uses the existing MinIO as its S3-compatible object store — no new storage volume. Prometheus, Loki, promtail, Grafana, Redis, Postgres all untouched.

## 2. Tracing Pipeline

Most of this is enabling dormant wiring.

- **Service instrumentation:** add `tracing.New()` to each service `main.go`, config-gated by `TRACING_ENABLED` (default off until rolled out). Add `otelhttp` middleware for incoming-span continuation; instrument outbound HTTP and GORM DB calls.
- **Gateway propagation:** the gateway extracts the inbound `traceparent` and injects it into downstream requests (today it propagates nothing — this is the core FE→BE fix).
- **OTel Collector** (`infra/otel/collector-config.yaml`): OTLP receivers gRPC (4317) + HTTP (4318); processors `memory_limiter` + `batch` + **tail sampling** (keep 100% of error/slow traces, sample ~10–20% of the rest to keep Tempo small); exporter `otlp` → Tempo.
- **Tempo** (`infra/tempo/tempo.yaml`): single-binary, OTLP receiver, object storage backed by existing **MinIO** (S3-compatible). Trace retention ~7–14 days (configurable; traces are debugging data, not long-term analytics).
- **Grafana:** add Tempo datasource + trace→logs correlation by `trace_id` (Loki already carries it via `libs/logger.WithContext`).
- **Browser:** an **axios request interceptor** generates a W3C `traceparent` per API call so the backend trace roots with a known `trace_id`. The analytics snippet stamps that same `trace_id` on the click event that triggered the call.
  - **v1 honesty:** precise click↔call association is best-effort (correlate the click with the next API call within a short window on the same path). Tightening this is a later refinement; full in-browser RUM spans are out of scope for v1.

## 3. Clickstream Pipeline — `services/analytics/`

Standard `services/{name}/` Go layout (cmd / internal/{config,domain,handler,service,repo,transport}).

### 3.1 Ingestion

- **`POST /collect`** — accepts a batched JSON array parsed from a `text/plain` beacon body. Validates shape, rejects malformed, drops events with timestamps too far in the future/past. Hashes IP immediately (`ip_hash = sha256(ip + daily_salt)`), never stores raw IP. Pushes events into an **in-process bounded buffer** and returns `204` fast — never blocks on the DB beyond a short timeout.
- **No Redis Streams.** At our volume (thousands of events/day) an in-process batcher is sufficient: flush on **500 rows or 1s**, whichever first. Durability is best-effort (analytics is not billing data) — on DB failure, retry a few times then drop. Redis Streams is the documented upgrade path alongside ClickHouse.
- **CORS** for the app origin(s); per-IP rate limit; max body size; `GET /healthz`.
- Gateway route family: `POST /api/analytics/collect` (public — anonymous users tracked), plus internal query endpoints behind admin auth.

### 3.2 Store interface (the ClickHouse seam)

```go
type EventStore interface {
    InsertBatch(ctx context.Context, events []domain.Event) error
    UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error
}
```

Postgres implementation now (`repo/postgres_store.go`). A future `repo/clickhouse_store.go` is the only thing the ClickHouse migration needs to touch on the write path.

### 3.3 Postgres schema

```sql
CREATE TABLE analytics_events (
  event_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type    text NOT NULL,          -- pageview|click|heartbeat|identify|custom
  event_name    text DEFAULT '',
  anonymous_id  text NOT NULL,          -- ALWAYS present
  user_id       text,                   -- present only when known
  session_id    text NOT NULL,
  timestamp     timestamptz NOT NULL,
  received_at   timestamptz NOT NULL DEFAULT now(),
  url text, path text, referrer text DEFAULT '', title text DEFAULT '',
  el_selector text, el_text text, el_tag text, el_attrs jsonb DEFAULT '{}',
  active_ms     integer,                -- heartbeat foreground time
  user_agent text DEFAULT '', device_type text DEFAULT '',
  screen_w int DEFAULT 0, screen_h int DEFAULT 0,
  ip_hash text DEFAULT '',              -- hashed, never raw
  trace_id text,                        -- links click → backend trace
  properties jsonb DEFAULT '{}'
);
CREATE INDEX ON analytics_events (anonymous_id, timestamp);
CREATE INDEX ON analytics_events (session_id);
CREATE INDEX ON analytics_events (timestamp);   -- for 90-day purge

CREATE TABLE analytics_identities (
  anonymous_id text, user_id text, timestamp timestamptz NOT NULL
);
CREATE INDEX ON analytics_identities (anonymous_id, timestamp DESC);
```

GORM `AutoMigrate` on service startup, per project convention. (Native monthly partitioning is an optional later optimization; at our scale a plain table + indexed timestamp purge is simpler.)

### 3.4 Identity resolution (anonymous ↔ identified)

```sql
CREATE VIEW analytics_events_resolved AS
SELECT e.*,
  COALESCE(e.user_id, i.user_id)                 AS resolved_user_id,
  COALESCE(e.user_id, i.user_id, e.anonymous_id) AS person_id
FROM analytics_events e
LEFT JOIN LATERAL (
  SELECT user_id FROM analytics_identities a
  WHERE a.anonymous_id = e.anonymous_id ORDER BY timestamp DESC LIMIT 1
) i ON true;
```

`person_id` is the canonical identity — identified user if known, else anonymous. Every analytics query groups by `person_id`, so anonymous sessions retroactively stitch to a user after login. Cross-device works because multiple `anonymous_id`s can point at one `user_id`.

> **Postgres caveat:** no native `windowFunnel()`/`retention()` aggregates — funnels and retention are hand-written with window functions. Acceptable at our scale; this is exactly what a future ClickHouse swap would simplify.

## 4. Frontend Snippet (`frontend/web`, vanilla TS)

Single small module, framework-agnostic, with a Vue Router adapter. Reuses the existing `useWatchSession.ts` sendBeacon pattern.

1. **anonymous_id** — UUID v4 in `localStorage` (cookie fallback), reused on every event.
2. **session_id** — UUID, regenerated after 30 min inactivity or on a new day.
3. **pageviews** — initial load + Vue Router `afterEach`.
4. **autocapture clicks** — one delegated `document` listener: stable CSS selector path (tag + id + classes, capped depth), trimmed `innerText` (≤200 chars, PII-stripped), tag name, `data-*` attributes. `data-no-track` opt-out.
5. **heartbeat** — every 15s while foregrounded (`visibilitychange` pause/resume) → `active_ms`. Source of truth for time-on-page; survives unclean tab close (not `beforeunload`).
6. **batching + delivery** — buffer; flush on size ≥ 20 / ~5s / `visibilitychange:hidden` / `pagehide`; via `navigator.sendBeacon` (`fetch` keepalive fallback); split batches over ~64 KB.
7. **identify / reset** — `identify(userId)` on login/signup sets `user_id` going forward and emits an `identify` event → `analytics_identities`. `reset()` on logout (new anonymous_id, clear user_id).
8. **trace_id** — stamped on click events from the axios interceptor (§2).

Public API: `analytics.init({ endpoint, heartbeatMs, flushMs })`, `analytics.page(props?)`, `analytics.track(name, props?)`, `analytics.identify(userId, traits?)`, `analytics.reset()`.

Gated behind `VITE_ANALYTICS_ENABLED` (default on; allows dark-ship override).

## 5. Privacy (collect-by-default + hygiene)

No blocking consent banner (Russian 152-ФЗ, small known-user group). Built in from day 1:

- **Hash IP** (`sha256(ip + daily_salt)`), never store raw.
- **Strip PII** from autocaptured `el_text`; never capture input field values; `data-no-track` masks sensitive elements.
- **90-day retention** on `analytics_events` via a daily purge job in the existing `scheduler` service (`DELETE WHERE timestamp < now() - interval '90 days'`).
- **Right to erasure:** `DELETE FROM analytics_events WHERE … person-resolves-to $id` plus `analytics_identities` cleanup, runnable by `user_id` or `anonymous_id`.
- Privacy-policy note documenting what is collected and the retention window.

**Known limitation (document in README):** clearing localStorage/cookies yields a fresh `anonymous_id` — a user looks new until they log in again. Unavoidable in this scheme.

## 6. Grafana Dashboards

**Backend observability** (Tempo): trace latency p50/p95/p99, error rate, slow-trace explorer, service dependency view.

**Product analytics** (from `analytics_events_resolved`):
- **Top clicked elements** — `GROUP BY el_selector ORDER BY count() DESC`.
- **Click frequency over time** — bucketed by hour.
- **Retention** — cohort by first-seen day (hand-written SQL).
- **Time on page** — summed `active_ms` (active) vs `max(timestamp) - min(timestamp)` (wall-clock) per `(person_id, session_id, path)`.
- **Session duration** — per `session_id`, split on >30 min gap.
- **Anonymous vs identified** — every panel `GROUP BY (resolved_user_id IS NULL)`.
- **Unique users** — `COUNT(DISTINCT person_id)`.

Dashboards as code under `infra/grafana/dashboards/`.

## 7. Resource Budget

| Component | RAM | New? |
|---|---|---|
| Tempo | ~256 MB (traces in MinIO) | new |
| OTel Collector | 256 MB – 1 GB | new |
| analytics service | ~256 MB | new |
| **Total added** | **~<1.5 GB** | vs reference spec's ~6 GB (ClickHouse + GlitchTip) |

## 8. Build Order (milestones)

Each milestone independently runnable/demoable.

1. **Clickstream backbone** — `services/analytics/` with `POST /collect` → in-process batcher → Postgres `analytics_events`. Prove end-to-end with curl. Gateway route `/api/analytics/collect`.
2. **Snippet v1** — anonymous_id, session, pageview, autocapture clicks, batching, sendBeacon. `VITE_ANALYTICS_ENABLED`.
3. **Heartbeat + time-on-page**, then identity stitching (`identify`, `analytics_identities`, `analytics_events_resolved`).
4. **Tracing** — wire `libs/tracing` in services, `otelhttp` middleware, gateway propagation, OTel Collector + Tempo containers, Grafana Tempo datasource + trace→logs correlation.
5. **Browser trace_id linking** — axios interceptor `traceparent` + stamp on click events.
6. **Analytics queries + Grafana dashboards** (funnels, retention, top clicks, session length, anon-vs-identified).
7. **Privacy** — IP hashing verified, PII stripping, 90-day purge job in scheduler, erasure path.

## 9. Acceptance Criteria

- A click in the browser appears in `analytics_events` within seconds, carrying `anonymous_id`, `session_id`, `el_selector`, `path`.
- After login, prior anonymous events resolve to the `user_id` via `analytics_events_resolved.person_id`.
- Funnel and retention queries return correct numbers on seeded test data.
- Time-on-page derives from heartbeats and survives an unclean tab close.
- A user action produces a single Tempo trace spanning gateway → downstream services, and the triggering click event carries the same `trace_id`.
- Grafana shows trace → logs jump by `trace_id`.
- Nothing stores a raw IP; `analytics_events` older than 90 days is purged; erasure-by-user works.
- `TRACING_ENABLED=false` and `VITE_ANALYTICS_ENABLED=false` cleanly disable each pipeline.

## 10. Effort Metrics (per `.planning/CONVENTIONS.md` — no days/hours)

- **UXΔ = +2 (Better)** — no direct end-user UI change, but unlocks data-driven UX improvement and faster incident diagnosis (FE→BE traces). Indirect, real.
- **CDI = 0.04 * 21** — moderate spread (new service + frontend snippet + 13-service tracing wiring + infra), low shift (additive, existing systems untouched), Effort_Fib 21.
- **MVQ = Griffin 85%/80%** — composite build (tracing + clickstream + dashboards) bridging known patterns; high slop-resistance because most tracing is enabling existing wiring and the store sits behind an interface.
