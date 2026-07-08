# Provider Probe: Health Hysteresis + Globally-Paced Scheduler — Design

- **Date:** 2026-07-08
- **Status:** Approved (design), pending spec review → implementation plan
- **Branch:** `feat/provider-probe-hysteresis-pacing`
- **Scope:** two phases — (1) health state-machine hysteresis, (2) globally-paced probe scheduler

---

## 1. Problem

On the `playback-health` Grafana dashboard, provider **Miruro** transitioned **UP → Down** on a *single* failed probe. The intended behavior (per the operator "reglament") is **UP → Degraded → Down**: a first failed probe should land a healthy provider in an intermediate **Degraded** band, and only a **second consecutive** failed probe should confirm **Down**.

### Evidence (live, 2026-07-08)

`stream_providers` (Postgres) + `probe_runs` (ClickHouse):

| Time (UTC) | Probe result | Effect |
|---|---|---|
| 2026-07-07 12:00 | `playable` | health `up` |
| 2026-07-08 00:00:40 | `cdn_unreachable` (one probe) | health flipped straight to `down`; `policy` stayed `auto` → dashboard shows **Down** |

Miruro is a browser/Turnstile provider and was `playable` 12h earlier, so the `cdn_unreachable` is almost certainly a **canary false-negative** (cold Camoufox/Turnstile solve) — exactly the class of transient failure the reglament exists to absorb.

### Root cause

`services/catalog/internal/service/providerpolicy/engine.go` — `ApplyHealth`:

```go
if !pass {
    p.Health = domain.HealthDown   // ANY single fail → Down, from ANY state
}
```

There is **no consecutive-failure counter**. The only debounces are time-gated and on the *other* edges (24h to recover up, 24h to demote `auto→manual`). A healthy `auto+up` provider that fails one probe goes straight to derived **Down**.

### Semantic collision (why this isn't a one-line change)

Today "Degraded" (dashboard `StateCode` 2) means `policy = manual` — a provider **demoted out of auto-failover after 24h of sustained Down** (via `ApplyPolicy`). So in the current model the severity order is actually `UP → Down → (24h) → Degraded` — Degraded comes *after* Down, the **opposite** of a mild pre-Down warning. Reclaiming "Degraded" as the transient rung requires re-homing the manual case.

**Operator decision (2026-07-08):** `manual` demotion should render as **"Disabled"**, freeing **"Degraded"** entirely for the transient (one-fail) state; and an `auto` provider's lifecycle should be **`UP → Degraded → Down`** with **no additional post-Down steps** (the 24h `auto→manual` demotion is removed).

---

## 2. Goals / Non-goals

**Goals**
- A single failed probe drops a healthy provider to **Degraded**, not **Down** (absorbs transient false-negatives).
- Two consecutive failed probes confirm **Down**. A pass from Degraded restores **UP**.
- Reclaim the dashboard labels: **Degraded** = transient one-fail; **Disabled** = admin/manual lock.
- Probes are **evenly paced in time** (no thundering-herd batch), via a central token-bucket pacer with headroom (utilization ρ < 1).

**Non-goals**
- No change to the analytics per-run tri-state rollup (`scorer.go`) or the boolean `pass` wire between analytics and catalog (Phase 1 rides the existing boolean).
- No change to runtime failover *resolution* logic (scraper orchestrator) beyond what `WireStatus` already drives.
- No revival of dead providers (allanime/animefever CF/AUTO-484 — out of scope).

---

## 3. Phase 1 — Health state-machine hysteresis

Self-contained, catalog + Grafana only, independently deployable. Fixes the Miruro bug.

### 3.1 Health enum

Add a fourth value to `domain.ProviderHealth` (`services/catalog/internal/domain/scraper_provider.go`):

```go
HealthUp         = "up"
HealthDegraded   = "degraded"   // NEW — one failed probe, pending confirmation
HealthRecovering = "recovering"
HealthDown       = "down"
```

No DB migration: it's a string column with GORM default `'up'`; existing rows are unaffected and no row currently holds `degraded`.

### 3.2 `ApplyHealth` transition table

Driven by the boolean `pass` catalog already receives:

| Current health | `pass` | `fail` |
|---|---|---|
| `up` | `up` | **`degraded`** ← *the fix (was `down`)* |
| `degraded` *(new)* | `up` | `down` |
| `recovering` | `up` if `≥ PromoteAfter` else `recovering` | `down` |
| `down` | `recovering` | `down` |
| *unseeded* | `recovering` | `down` |

`HealthSince` continues to reset only on a real state change. Recovery (up-direction) is unchanged: `Down → Recovering → UP`, gated by `PromoteAfter` (kept).

### 3.3 Remove `ApplyPolicy`

Delete `ApplyPolicy` entirely — **both** the 24h `auto→manual` demotion **and** the `manual→auto` auto-promotion. Policy (`auto | manual | disabled`) becomes **admin-controlled only** (SQL/admin endpoint). `ApplyVerdict` drops its `demoteAfter` parameter:

```go
func ApplyVerdict(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration) {
    ApplyHealth(p, pass, now, promoteAfter)
    p.LastProbedAt = now
}
```

Reverses the *auto* half of the 2026-06-23 self-healing loop (intentional, per operator). `PROVIDER_PROMOTE_AFTER` stays (health `recovering→up`); `PROVIDER_DEMOTE_AFTER` is dropped.

### 3.4 Display band — `DerivedState` / `StateCode`

Numeric codes are **unchanged** (4/3/2/1/0), so the Grafana gauge value-mappings and colors stay put. Only the *derivation* changes:

| `(policy, health)` | Band | Code |
|---|---|---|
| `auto` + `up` | UP | 4 |
| `auto` + `recovering` | Recovering | 3 |
| `auto` + **`degraded`** | **Degraded** | 2 |
| `auto` + `down` | Down | 1 |
| **`manual`** or `disabled` | **Disabled** | 0 |

(Was: `manual → Degraded`.) The roster table's Postgres `CASE` in `docker/grafana/dashboards/playback-health.json` mirrors this one-for-one and must be updated to match; the `provider_state` gauge help text in `libs/metrics/provider.go` gets its "Degraded" description refreshed (code meanings unchanged).

### 3.5 Wire status + eligibility (keeps the player working)

The real failover gate consumes **`WireStatus()`** (via `internal_scraper_providers.go`), not `Eligible()` (test-only). Only one wire mapping changes — **Degraded stays in the auto-failover chain** (it's a warning, not a confirmed outage; runtime failover already covers a genuine miss):

| `(policy, health)` | `WireStatus` | In auto-failover? |
|---|---|---|
| `auto` + `up` | `enabled` | yes |
| `auto` + **`degraded`** | **`enabled`** | **yes (new)** |
| `auto` + `recovering` | `degraded` | no (selectable) |
| `auto` + `down` | `degraded` | no (selectable) |
| `manual` | `degraded` | no (hacker-selectable) — **unchanged** |
| `disabled` | `disabled` | not registered |

Note the deliberate axis split: `manual` shows as **"Disabled"** on the dashboard (`DerivedState`) but keeps `WireStatus = degraded` so hacker-mode selectability is unchanged. `Eligible()` is updated for consistency to `policy == auto && health ∈ {up, degraded}`.

### 3.6 Probe cadence / sample (Phase-1 interim)

Phase 1 keeps the existing single-value `CadenceConfig` + 6h cron; it only needs to place the new `degraded` health: `ProbeCadence(degraded) = Up (6h)` and `ProbeSample(degraded) = FullSample` (re-probe next cycle to confirm/clear). Phase 2 replaces this whole model.

### 3.7 Phase-1 files

- `services/catalog/internal/domain/scraper_provider.go` — `HealthDegraded`; `DerivedState`, `StateCode`, `WireStatus`, `Eligible`, `ProbeCadence`, `ProbeSample`
- `services/catalog/internal/service/providerpolicy/engine.go` — `ApplyHealth` hysteresis; delete `ApplyPolicy`; `ApplyVerdict` signature
- `services/catalog/internal/config/config.go` — drop `DemoteAfter`
- `services/catalog/internal/handler/internal_provider_policy.go` — updated `ApplyVerdict` call
- `docker/grafana/dashboards/playback-health.json` — roster `CASE` mirror + gauge help text
- `libs/metrics/provider.go` — gauge help text
- Tests: `engine_test.go`, `scraper_provider_test.go`, `internal_provider_policy_test.go`, `migrate_test.go` (WireStatus expectations for `manual` unchanged; band expectations updated where asserted)

### 3.8 Phase-1 rollout

- Redeploy **catalog**; restart **grafana** (provisioned dashboard reloads).
- Existing `manual/down` tombstones (allanime, animefever, animepahe) shift dashboard band **Degraded → Disabled** (more accurate). Their `WireStatus` (hacker-selectability) is unchanged.
- Manual providers no longer auto-promote — set `policy='auto'` by SQL when an admin wants one back in the chain.
- **Miruro one-shot ops:** it is `auto/down` right now from the false-negative. Post-deploy it climbs back `Down→Recovering→UP` (~`PromoteAfter`). Optionally hand-reset: `UPDATE stream_providers SET health='up', health_since=now() WHERE name='miruro';`

---

## 4. Phase 2 — Globally-paced probe scheduler (token bucket, dynamic leak rate)

Replaces the 6h batch (`RunOnce` over a due-set) with a continuous, evenly-paced release. Larger; ships after Phase 1.

### 4.1 Two-cadence model

Demand (individual timers) is sized **slower** than capacity (global leak rate) so the system runs at **ρ < 1** — the ready-queue stays short and a newly-Ready provider is probed promptly.

**Desired cadence `Cᵢ`** — individual timer; provider is **Ready** when `now − last_probed_at ≥ Cᵢ`:

| State | `Cᵢ` (demand) |
|---|---|
| UP | 8h |
| Degraded | 6h |
| Recovering | 6h |
| Down | 30h |
| Disabled | ∞ (excluded) |

**Heartbeat cadence `Cᵢ*`** — used **only** to size the global leak rate:

| State | `Cᵢ*` (capacity) |
|---|---|
| UP | 6h |
| Degraded | 4h |
| Recovering | 4h |
| Down | 20h |
| Disabled | excluded |

Both tables are env-tunable config.

### 4.2 Global heartbeat

$$I_{global} = \frac{1}{\sum_{i\,\notin\,disabled} 1/C_i^{*}}$$

Recomputed whenever any provider's `Cᵢ*` changes (state transition) or the roster changes (enable/disable/add). Guard: no active providers → no probing (skip); enforce a **floor** on `I_global` (min seconds-between-probes) to protect Camoufox from a pathologically small roster.

**Worked example** (5×UP + 2×Down): capacity `∑1/Cᵢ* = 5/6 + 2/20 = 0.933/h` → `I_global ≈ 64 min`; demand `∑1/Cᵢ = 5/8 + 2/30 = 0.692/h` → **ρ ≈ 0.74** (≈26% idle slots).

### 4.3 Pacer (analytics, always-on)

An always-on goroutine in the analytics probe service:

1. **Roster snapshot** — periodically fetch `{name, health, policy, last_probed_at, popularity}` from catalog (extend the existing `/internal/providers/probe-plan`, or a sibling roster endpoint). Map `health → (Cᵢ, Cᵢ*)` via the config tables.
2. **Ready set** — providers with `now − last_probed_at ≥ Cᵢ`.
3. **Priority queue** — **most-overdue-first** (`(now − last_probed)/Cᵢ` desc), tie-break by title **popularity** (preserves today's most-popular-first bias, prevents starvation).
4. **Leak** — release **one** probe every `I_global`; token-bucket capacity **1** (a just-idle system probes the first Ready provider immediately; no post-idle burst). Empty queue at a leak tick → idle.
5. **Probe** — run a **single-provider** probe (existing resolve → validate; `sampleSize`/`failFast` remain as per-provider *title-sample* knobs). POST the boolean verdict to catalog → catalog runs `ApplyVerdict` → health (hence `Cᵢ`, `Cᵢ*`) may change → next snapshot recomputes `I_global`.

`PLAYBACK_PROBE_CRON` (6h batch) is retired. The scheduler keeps its manual-trigger "kick" (now: mark the full roster Ready / enqueue a sweep).

### 4.4 Error handling

- Catalog roster fetch fails → keep last-known snapshot, log WARN, continue pacing (never probe blindly).
- Verdict POST fails → log + drop this tick's result (next `Cᵢ` cycle re-probes); do not block the pacer.
- Provider removed from roster → evicted from queue.
- `I_global` divide-by-zero (empty active roster) → pause pacing until roster non-empty.

### 4.5 Phase-2 files (indicative)

- `services/analytics/internal/probe/` — new pacer (queue + leak loop + `I_global`), single-provider run path; `RunOnce` batch retired/repurposed
- `services/analytics/internal/config/config.go` — `Cᵢ` / `Cᵢ*` tables + `I_global` floor
- `services/catalog/internal/handler/internal_provider_policy.go` (plan/roster endpoint) — return `health` + `last_probed_at` + popularity
- `services/scheduler/internal/config/config.go` + `docker/docker-compose.yml` — retire `PLAYBACK_PROBE_CRON` batch; keep manual trigger
- Tests: pacer unit tests (I_global formula; queue ordering; leak timing with injected clock; synchronized-start desync; roster-change recompute)

### 4.6 Phase-2 rollout

Redeploy **analytics** + **catalog**; update **scheduler** config + compose. No DB change.

---

## 5. Testing strategy

- **Phase 1** — table-driven unit tests for the full `ApplyHealth` transition matrix (incl. `up→degraded→down`, `degraded→up`, and that a lone fail never reaches `down`); `DerivedState`/`StateCode`/`WireStatus`/`Eligible` truth tables; `ApplyVerdict` signature + caller. Validate the edited `playback-health.json` parses and the `CASE` mirrors `DerivedState`.
- **Phase 2** — pacer tests with an **injected clock** (no wall-clock/RNG): `I_global` equals the harmonic reciprocal for a fixed roster; queue pops most-overdue-first; a synchronized cold start desynchronizes within one cycle; ρ<1 keeps queue depth bounded; roster change recomputes `I_global`.
- `go test ./... -race` for catalog + analytics; `go vet`.

## 6. Effort & impact metrics (per `.planning/CONVENTIONS.md`)

- **Phase 1** — UXΔ = **+2 (Better)** (dashboard reflects reality; operators stop chasing false "Down"). CDI = **0.03 × 8** (small spread across provider-state derivation, moderate behavioral shift, Effort 8). MVQ = **Griffin 85%/80%** (methodical hysteresis, guards against flapping).
- **Phase 2** — UXΔ = **+1 (Better)** (smoother, less bursty probe load; kinder to Camoufox). CDI = **0.05 × 21** (new pacer subsystem + cross-service interface, Effort 21). MVQ = **Kraken 80%/75%** (systemic, coordinated pacing).

## 7. Risks / open items

- **Self-healing reversal:** removing `ApplyPolicy` means no automatic `manual→auto` re-promotion; admins re-promote via SQL. Accepted by operator.
- **Existing manual tombstones** flip dashboard band Degraded→Disabled on Phase-1 deploy (cosmetic, more accurate).
- **`I_global` floor** must be chosen so a small roster can't hammer Camoufox; default TBD in the plan (e.g. ≥ a few minutes).
- **Roster/plan endpoint shape** (extend `probe-plan` vs new endpoint) finalized in the Phase-2 plan.
