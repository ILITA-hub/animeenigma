# Pre-probe Camoufox warmup + "Last tick metrics" provider-roster column

- **Date:** 2026-07-06
- **Status:** Design (approved forks: back-to-back warmup; Grafana display)
- **Branch:** `feat/probe-warmup-last-tick-metrics`
- **Touches:** `services/analytics` (probe engine, streamprobe, plan client), `services/catalog` (probe-plan, probe-result handler, `stream_providers` domain + column, admin health feed), `docker/grafana/dashboards/playback-health.json`

## Problem

`engine=browser` EN providers (animepahe, miruro, gogoanime, nineanime) resolve
through the Camoufox stealth-scraper, which clears a Cloudflare **interactive
Turnstile** on the first (cold) nav in ~10s and then reuses that `cf_clearance`
session for the rest of the resolve. The session TTL is **600s**
(`STEALTH_SESSION_TTL_SECONDS`); the automated health probe runs on a **6h
(up) / 24h (manual+down)** per-provider cadence. So the warm session is **always
expired when the probe fires** → the probe **always** eats the cold Turnstile
solve → under probe timing/contention that cold path is slow/flaky → the
provider gets pinned `health=down` even though real warm traffic plays fine.

Observed live (2026-07-06): animepahe resolves a fully playable kwik.cx/uwucdn
HLS stream end-to-end in **0.24s warm**, yet the probe has had it `down` since
07-04. Same structural trap that kept miruro down. Confirmed on this server's own
datacenter IP the cold solve **succeeds** in ~10s (it is not an IP block) — it is
purely a cold-vs-warm timing artifact of the probe.

Separately, the probe records **no timing at all** (a `Verdict` is only
`{Stage, Reason}`), so the operator has no visibility into how long a resolve
took, how fast the resulting CDN is, or how long the cold solve cost.

## Goals

1. Make the probe measure the **warm** path a real viewer hits, by warming the
   browser session immediately before the measured probe (browser providers only).
2. Capture per-tick timing/quality — **warmup time, resolve latency, connection
   speed/latency, and metadata** — and persist it as a **"Last tick metrics"**
   summary on the provider roster, surfaced in the Grafana playback-health
   dashboard.

## Non-goals

- No perpetual "always-warm" keepalive loop (explicitly rejected — a single
  back-to-back warmup within the 600s TTL is sufficient).
- No change to the state-machine cadences/thresholds, policy/health model, or the
  provider registration/roster derivation.
- No change to HTTP-provider probing (they have no cold-solve cost; they skip
  warmup and report `warmup_ms` absent).

## Design

### Part 1 — Back-to-back warmup (browser providers only)

In the analytics probe engine (`services/analytics/internal/probe/engine.go`),
`probeProvider` gains a warmup pre-step gated on **`engine == browser`**:

1. **Warm:** one best-effort, **unscored** `Resolver.Resolve(...)` for the first
   ref. This eats the cold Turnstile solve and writes `cf_clearance` into the
   leased pool profile's persistent `user_data_dir`. Timed → `warmup_ms`.
   - Failure is swallowed (warmup never produces a verdict, never marks the
     provider down). If warmup fails, the measured probe still runs and reports
     honestly — so a genuinely broken provider is still caught.
2. **Measure:** the existing probe loop runs next, against the now-warm session
   (`lease()` prefers already-launched, fewest-uses profiles, and `cf_clearance`
   persists per-profile — so the warm profile carries over). Timed → `resolve_ms`.

**How analytics learns "browser":** the catalog probe-plan entry
(`GET /internal/providers/probe-plan`, `PlanEntry`) gains an **`engine`** string
field (catalog already knows each row's engine from the `stream_providers`
roster — drift-proof, mirrors the scraper's `BrowserEngineNames()` DB-derivation).
Analytics warms iff `entry.Engine == "browser"`. No hardcoded provider list.

> **Planning-time nuance (not a blocker):** with `pool_size=2`, if both profiles
> are launched the measured probe *could* lease the other (possibly-cold)
> profile. Clearance persists per-profile and `lease()` prefers warm, so in
> practice the warm profile is reused; the plan should verify this and, if
> needed, either warm the pool such that the preferred lease is warm, or accept
> the (still-improved) outcome. Verify empirically on animepahe before shipping.

### Part 2 — "Last tick metrics"

#### Instrumentation (analytics + streamprobe)

The probe wraps resolve/validate with timers and reads bytes/host from the
resolved stream. `streamprobe.Validate` is extended to return manifest-fetch
latency and a throughput sample:

- `validate_ms` — HLS manifest GET latency (already fetched; just timed).
- `throughput_kbps` — from **one first-segment fetch** (bytes ÷ elapsed).
  Justified: probe cadence is 6–24h per provider, so one extra segment fetch per
  tick is negligible, and a speed derived from the ~20KB manifest alone would be
  noise. If `Validate` already fetches a segment, reuse those bytes.
- `cdn_host` — host of the resolved master/media URL.
- `quality` — resolution/label when the resolver exposes it (else omitted).

#### Metrics summary shape (persisted JSON)

```json
{
  "at": "2026-07-06T05:48:37Z",
  "pass": true,
  "reason": "",
  "provider_used": "animepahe",
  "anime": "Frieren: Beyond Journey's End",
  "slot": "sub",
  "sample_size": 1,
  "warmup_ms": 9800,
  "resolve_ms": 1900,
  "validate_ms": 320,
  "throughput_kbps": 5400,
  "cdn_host": "vault-08.uwucdn.top",
  "quality": "1080p"
}
```

- `warmup_ms` is **omitted** (or `null`) for HTTP providers.
- `reason` mirrors the existing dominant-failure classification ("" when up).
- The blob is a *summary of the last tick only* — history stays in ClickHouse /
  `probe_runs_total`; this is the at-a-glance "what did the last probe see".

#### Transport (analytics → catalog)

`PostVerdict` body extends from `{provider, pass, reason}` to add an optional
`metrics` object (backward-compatible — catalog treats it as optional):

```json
{ "provider": "animepahe", "pass": true, "reason": "",
  "metrics": { ...the shape above... } }
```

#### Storage (catalog)

- New column on `ScraperProvider` (`stream_providers`):
  `LastTickMetrics datatypes.JSON `gorm:"column:last_tick_metrics;type:jsonb"``.
  GORM `AutoMigrate` adds it on startup (additive — safe).
- `InternalProviderPolicyHandler.ProbeResult` decodes the optional `metrics`
  object and writes it to `last_tick_metrics` in the **same** update that
  persists the state transition / `last_probed_at`. Absent metrics ⇒ column
  left unchanged (never overwrite a good summary with null on a legacy caller).

#### Surface

1. **Admin health feed** (`/scraper/health/admin`, gateway
   `/api/admin/scraper/health`): the enriched DTO gains a `last_tick_metrics`
   field passed through from the row. Always populated regardless of dashboard.
2. **Grafana** — the **"Provider Roster & Playability"** table panel in
   `playback-health.json` is a **PostgreSQL-datasource** panel already selecting
   from `stream_providers`. Extend its SQL to project the jsonb fields as
   columns — **no new Prometheus gauges required**:

   ```sql
   SELECT name AS provider,
          "group" AS "Group",
          CASE policy WHEN 'auto' THEN 'yes (auto)' WHEN 'manual' THEN 'no (manual)' END AS "Policy",
          round((last_tick_metrics->>'warmup_ms')::numeric  / 1000, 1) AS "Warmup s",
          round((last_tick_metrics->>'resolve_ms')::numeric / 1000, 2) AS "Resolve s",
          round((last_tick_metrics->>'throughput_kbps')::numeric / 1000, 1) AS "Speed Mbps",
          last_tick_metrics->>'cdn_host' AS "CDN"
   FROM stream_providers
   ...
   ```

   Column formatting (units, null → "—" for HTTP warmup) via SQL `round`/`coalesce`
   + Grafana field overrides. The panel already merges a ClickHouse playability
   target on the `provider` key; the new columns ride the existing Postgres target.

## Testing

- **analytics:** unit-test the warmup gate (browser ⇒ warm-then-measure; http ⇒
  no warm), warmup-failure isolation (warmup error ⇒ measured probe still runs,
  no verdict from warmup), and metrics assembly (durations, `cdn_host`,
  `warmup_ms` absent for http). Fakes only (no testify/mock — repo convention).
- **streamprobe:** unit-test `validate_ms`/`throughput_kbps` from a fake HTTP
  server serving a manifest + one segment.
- **catalog:** `ProbeResult` writes `last_tick_metrics` when present, leaves it
  untouched when absent; `PlanEntry.Engine` populated from the roster.
- **Manual E2E:** force an animepahe probe (backdate + `POST
  analytics:8092/internal/probe/run`), assert `stream_providers.last_tick_metrics`
  is populated and the Grafana panel renders the columns.

## Effort metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — indirect for viewers (browser providers that work warm
  stop being falsely pinned `down`, so more EN sources stay selectable); direct
  observability win for the operator.
- **CDI = 0.05 * 13** — Spread×Shift ≈ 0.05 (a handful of packages across
  analytics + catalog + one dashboard; mostly **additive** — new column, new
  warmup pre-step, backward-compatible payload). Effort 13 (multi-service
  instrumentation + DB column + dashboard + tests).
- **MVQ = Kraken 85%/80%** — reaches across services with coordinated tentacles
  (analytics probe → catalog persistence → Grafana), each well-bounded and
  independently testable.

## Rollout / risk

- All changes additive & backward-compatible; a legacy analytics still POSTs the
  3-field body and catalog still works (metrics simply not updated).
- Warmup adds ~10s per browser-provider tick; `RunOnce` has an 8-min budget and
  browser providers are few — comfortably within budget.
- Extra pool `uses` (2 resolves/tick vs 1) is trivial against `max_uses=50` at
  6–24h cadence.
- If warmup does **not** reliably rescue animepahe/miruro in the E2E check, that
  falsifies the cold-vs-warm hypothesis for that provider (points to a real CDN
  block or sidecar wedge) — surface it rather than masking with retries.
