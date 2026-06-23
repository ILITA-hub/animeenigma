# Provider Policy/Health Self-Healing Loop + Churn Suppression — Design Spec

**Date:** 2026-06-23
**Status:** Design approved (brainstorm), pending implementation plan
**Owner services:** `services/catalog` (owns `stream_providers` + the new policy/health state machine) · `services/analytics` (the prober) · `services/scheduler` (the clock) · `services/scraper` (eligibility consumer) · `services/maintenance` (churn suppression) · `frontend/web` (Source-picker pill) · `docker/grafana/dashboards/playback-health.json`

---

## 1. Summary

Scraper EN-provider degraded↔enabled flips are **manual today** (`AllAnimeDegrade`/`AnimefeverDeclaim`-style SQL migrations against `stream_providers.status`), and the playability probe's observations live only in ClickHouse (`probe_runs`) + Prometheus — **they never write back to the DB**. Consequences measured 2026-06-23:

- A freshly-broken provider lingers in the auto-failover chain until a human notices and flips it (users hit dead providers in the meantime).
- The maintenance bot re-files the **same** probe failure as a new escalation every run — **~16 duplicate `allanime stream_segment` issues** out of a **97-item escalated backlog**.

This change closes the loop: the probe becomes **authoritative and DB-persisted**, a **policy/health state machine** with time-hysteresis auto-demotes broken providers out of failover and auto-promotes recovered ones back, probe cadence is **tiered by state** (spend probes where they matter), and the maintenance bot **stops escalating already-managed providers**.

### Goals
- A broken provider leaves the auto-failover chain on the **first** failed probe (≤6h), without a human in the loop.
- A recovered provider auto-returns, but only after proving stability (hysteresis — no flapping).
- The `stream_providers` DB row is the **single source of truth** for provider state; Grafana reads it; the probe writes it.
- Stop the duplicate-escalation churn for already-degraded providers.
- All thresholds/cadences are tunable via env (no code change to retune).

### Non-goals
- **Reviving allanime / animefever** (the CF-Turnstile-clock and AUTO-484 ad-substitution root causes) — tracked separately; this spec only manages provider *state*, it does not fix any upstream provider.
- Routing providers through the Camoufox sidecar (separate effort).
- Changing the failover *ordering* among eligible providers (`preference_weight` logic unchanged).
- A clickable/editable Grafana panel — admin edits remain SQL / a small admin endpoint (Grafana stays a read view).
- Per-title capability discovery (the existing `supports_*` columns are untouched).

---

## 2. Current state (measured 2026-06-23)

| Aspect | Today | Problem |
|--------|-------|---------|
| Provider state | single `stream_providers.status ∈ {enabled, degraded, disabled}` | `degraded` already = "manual-only, out of auto-failover, sorted last, pill". No intermediate/recovering state, no timestamps. |
| State transitions | **manual** SQL migrations (`AllAnimeDegrade`, `AnimefeverDeclaim`) | A human must notice + flip. Broken providers linger in failover. |
| Probe | heavy **daily** canary `PLAYBACK_PROBE_CRON="0 3 * * *"` → analytics `/internal/probe/run` → ClickHouse `probe_runs` + Prometheus gauges | Daily granularity (up to 24h to notice a break). Results **never persisted to `stream_providers`** — DB and probe disagree. |
| Failover gate | `services/scraper` consumes `status==enabled` (`IsEnabled`) via `/internal/scraper/providers` | Works, but keyed on a state nothing auto-updates. |
| Dashboard | "Provider Roster & Playability" panel reads `status AS "Policy"` from `stream_providers` (postgres) **and** a separate ClickHouse `probe_runs` playability column | The two columns are **unreconciled** — Policy (DB) and Playability (probe) can silently disagree. |
| Escalations | maintenance bot re-files every probe failure as a new issue | ~16 duplicate `allanime stream_segment` issues; 97-item escalated backlog. |

Live roster at design time: `gogoanime`, `miruro`, `nineanime`, `okru` = enabled/up; `allanime` (CF Turnstile clock) + `animefever` (AUTO-484) = degraded.

---

## 3. State model

`stream_providers` gains two machine-managed dimensions plus bookkeeping. The legacy `status` column is migrated and retired (see §8).

| Field | Values | Writer |
|---|---|---|
| `policy` | `auto` · `manual` · `disabled` | machine (for `auto`↔`manual`) · **admin only for `disabled`** |
| `health` | `up` · `recovering` · `down` | machine (probe-derived) |
| `health_since` | timestamp | machine — drives the >1-day hysteresis |
| `policy_since` | timestamp | machine — audit / dashboard |
| `last_probed_at` | timestamp | prober — drives tiered cadence |

**Failover eligibility = `policy == auto && health == up`.** Anything else is out of the auto chain. Out-of-chain providers (except `disabled`) remain manually selectable via `prefer=`/hacker-mode, sorted last.

`disabled` is the **only** hard admin lock (per approved decision): the machine never probes a `disabled` provider and never changes its policy. `auto`↔`manual` are fully machine-driven (admin SQL edits to them are transient — the machine re-derives next tick).

### Derived display (FE Source-picker pill + Grafana "Policy" column)

The single user-facing "Policy" label is **computed** from `(policy, health)`:

| (policy, health) | Pill label | Color |
|---|---|---|
| `auto`, `up` | **UP** | green |
| `auto`/`manual`, `recovering` | **Recovering** | yellow-green |
| `auto`, `down` | **Failing** | amber (transient, pre-demote window) |
| `manual`, `down` | **Manual-only** | red |
| `disabled`, — | **Disabled** | hidden / grey |

> The yellow-green state is named **Recovering** (approved). `(manual, up)` is not a stable state — a manual provider that reaches `up` is promoted to `auto` in the same step (§4), so it never persists.

---

## 4. Transition rules (the state machine)

The machine lives in `catalog` (new `service/providerpolicy`). On each probe verdict for a non-`disabled` provider it updates `health` first, then evaluates `policy`. All timers read off `health_since`/`policy_since` so they survive restarts.

### Health (probe-driven)

```
down       --probe PASS-->        recovering        (health_since reset)
recovering --PASS sustained >1d--> up               (→ triggers promote, §below)
recovering --any probe FAIL-->     down             (recovering clock reset)
up         --probe FAIL-->         down             (health_since reset; eligibility drops immediately)
up         --PASS-->               up               (stay)
```

### Policy (machine, non-`disabled` only)

```
auto --health==down sustained >1d--> manual          (DEMOTE)
manual --(recovering→up fires)-->     auto + up       (PROMOTE; same step as health reaching up)
```

### Behavior this produces

- **Fast exclusion, slow demotion.** A break drops `health` to `down` on the first failed probe → the provider leaves the failover chain *immediately* (eligibility needs `up`). But it isn't demoted to the cheap once-daily `manual` cadence until it's been `down` a continuous day (hysteresis — a transient blip keeps probing at the fast `auto` cadence and can recover without a demotion).
- **Strict, gradual recovery.** A `manual`/`down` provider's daily probe must pass to enter `recovering`; it must then stay clean for >1 day at the 12h cadence before it is promoted back to `auto`/`up`. Any single failure resets the clock to `down`.
- **Structurally-dead providers settle correctly.** allanime (CF Turnstile clock) demotes to `manual`, its daily top-title probe keeps failing → it stays `manual-only` indefinitely with one cheap probe/day and **no escalations** (§7).

Tunables (env, catalog): `PROVIDER_DEMOTE_AFTER` (default `24h`), `PROVIDER_PROMOTE_AFTER` (default `24h`).

---

## 5. Probe engine (tiered cadence + fail-fast + popularity order)

Reuse the existing heavy canary (analytics `/internal/probe/run`). Raise the base tick to **every 6h** and gate each provider by `now − last_probed_at ≥ cadence(state)`:

| State | Cadence env (default) | Sample | Fail-fast |
|---|---|---|---|
| UP (`health=up`) | `PROBE_CADENCE_UP` (`6h`) | full 5 titles | **no** — run all, record playability % for the dashboard |
| Recovering (`health=recovering`) | `PROBE_CADENCE_RECOVERING` (`12h`) | top-K (`PROBE_RECOVERING_SAMPLE`, default `3`) | **yes** |
| Failing (`auto`,`down`) | `PROBE_CADENCE_UP` (`6h`) | top title → up to full | **yes** |
| Manual-only (`manual`,`down`) | `PROBE_CADENCE_MANUAL` (`24h`) | **top title only** | **yes** |
| Disabled | — | — | not probed |

### Rules

- **Most-popular-first ordering (global).** The canary's title list is sorted by popularity descending for *every* run, so the highest-value content is validated first and is what trips fail-fast.
- **Fail-fast (recovering / down states).** The first failed title aborts the run: provider verdict = `FAILED`, the failed title recorded with its reason, remaining titles recorded `not_tried` in `probe_runs`. This is your "don't burn the full 5 on a still-flaky provider."
- **UP runs the full sample** (no fail-fast) so the dashboard keeps a true playability % (e.g. "4/5 playable"). An UP provider's verdict is `FAILED` when its **most-popular** title fails (→ `health: up→down`); partial failures below the top title are recorded for the dashboard but do not by themselves flip health (tunable later if needed).

### Verdict → state

The prober posts each provider's run verdict (`PASS`/`FAILED` + per-title rows) to catalog `POST /internal/providers/probe-result`. Catalog's `providerpolicy` applies §4, writes `health`/`policy`/`*_since`/`last_probed_at`, and (as today) the run rows land in ClickHouse `probe_runs` for the dashboard.

---

## 6. Service responsibilities & data flow

```
scheduler (6h tick)
   └─> catalog: select due-set (last_probed_at vs cadence(state)) ──┐
                                                                    v
                                          analytics /internal/probe/run (due-set)
                                          · titles popularity-desc
                                          · fail-fast per state · sample per state
                                          · writes probe_runs (ClickHouse)
                                                                    │ verdicts
                                                                    v
        catalog providerpolicy  <── POST /internal/providers/probe-result
          · apply §4 transitions · persist policy/health/*_since/last_probed_at
                                                                    │
            /internal/scraper/providers (eligibility = policy==auto && health==up)
                                                                    v
                                          scraper orchestrator (auto-failover gate)
```

- **catalog** — single source of truth. Owns `stream_providers`, the `providerpolicy` state machine, the due-set selection, and the eligibility it already exposes via `/internal/scraper/providers` (response shape unchanged; the existing `enabled`/`status` fields are derived from `policy`/`health` for backward compatibility during rollout, then the consumer switches to the new fields).
- **analytics** — the prober. Adds per-provider sample size, fail-fast, and popularity ordering to the existing canary; returns/posts verdicts.
- **scheduler** — 6h tick (replaces the daily `PLAYBACK_PROBE_CRON`; the daily value becomes the manual-only cadence default).
- **scraper** — consumes `eligible = policy==auto && health==up`. The current `status==enabled`/`IsEnabled` gate and `degraded`-keyed branches map to `policy!=auto`.
- **frontend** — Source-picker pill from the derived display (§3).

---

## 7. Churn suppression (#2)

The duplicate-escalation problem is a direct consequence of an unmanaged loop. With the machine authoritative:

- **Suppress auto-escalation for any `policy != auto` provider.** A provider the machine has already moved out of `auto` is *known-degraded and managed* — its probe failures update `health`/`last_probed_at` only, and the maintenance bot does **not** open a new issue. (allanime's daily top-title failure becomes a silent DB touch.)
- **Dedup key `(provider, stage, root_cause_fingerprint)`** for the failures that *do* warrant a human (an `auto` provider newly failing): the bot updates the single existing issue instead of filing N near-duplicates.

Expected effect: clears ~16 of the 97-item backlog immediately and stops regeneration.

---

## 8. Migration

GORM auto-migrate adds the new columns; a one-time back-fill maps the retired `status`:

| old `status` | `policy` | `health` |
|---|---|---|
| `enabled` | `auto` | `up` |
| `degraded` | `manual` | `down` |
| `disabled` | `disabled` | (n/a) |

`*_since` back-filled to `updated_at` (or migration time); `last_probed_at` to zero (forces a probe on first eligible tick). The `status` column is dropped only after the scraper consumer has switched to `policy`/`health` (two-step: add+derive → switch consumer → drop), to avoid a cross-service breaking window.

---

## 9. Testing

- **State machine** (`providerpolicy`): table-driven unit tests for every health and policy transition, both hysteresis boundaries (just-under vs just-over `PROVIDER_DEMOTE_AFTER`/`PROMOTE_AFTER`), `recovering→down` clock reset, and `disabled` immunity (machine never touches it). Handwritten fakes — no testify/mock (house style).
- **Probe engine**: fail-fast aborts at first failure with correct `not_tried` records; titles probed in popularity-descending order; per-state sample size honored; UP records full playability %.
- **Migration**: `status → (policy, health)` back-fill round-trip.
- **Integration**: probe verdict → catalog transition persisted → `/internal/scraper/providers` eligibility flips → scraper orchestrator includes/excludes the provider.
- **Churn suppression**: a `policy!=auto` failure produces no new issue; an `auto` failure dedups to one issue by fingerprint.

---

## 10. Effort & impact metrics

- **UXΔ = +3 (Better)** — broken providers leave the chain in ≤6h instead of lingering up to a day; recovered providers auto-return; fewer dead-provider playback attempts for users.
- **CDI = 0.04 × 21** — touches catalog/analytics/scheduler/scraper/FE/Grafana, but each change is localized to the provider-state seam (one new catalog service + additive probe params + a derived FE pill). Effort 21 (Fibonacci, not pre-multiplied).
- **MVQ = Phoenix 88% / 85%** — a literal self-healing/resurrection loop; high slop-resistance via exhaustive table-driven state tests.

---

## 11. Open sub-choices (confirm at plan time)

- `PROBE_RECOVERING_SAMPLE = 3` (top-K titles for recovering runs).
- UP `health: up→down` trigger = most-popular title fails (partial sub-top failures recorded but non-flipping).
- `PROVIDER_DEMOTE_AFTER = PROVIDER_PROMOTE_AFTER = 24h`.
- Base tick `6h` (= `PROBE_CADENCE_UP`); manual-only `24h`; recovering `12h`.
