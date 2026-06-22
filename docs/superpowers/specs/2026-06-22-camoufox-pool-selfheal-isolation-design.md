# Camoufox Pool Self-Heal + Isolation + Provider Auto-Resurrection — Design Spec

**Date:** 2026-06-22
**Status:** Approved (design); pending implementation plan
**Owner:** @0neymik0
**Drives:** Closes maintenance-bot escalation **AUTO-527** ("Nineanime Camoufox pool exhaustion — collateral gogoanime stream outage") and retires the restart band-aid (AUTO-519/520/521/524).

---

## Goal

Make the stealth-scraper Camoufox sidecar **self-healing and fault-isolated** so a flaky browser provider (nineanime) can no longer crash or starve a healthy one (gogoanime), the pool revives crashed slots without a container restart, capacity is governed by a **RAM budget** instead of a fixed instance count, per-user consumption is bounded, and a recovered provider is **automatically resurrected** (degraded → enabled) without a migration, SQL edit, or restart.

## Background — the live failure (code-level)

The sidecar (`services/stealth-scraper/`) fronts one `CamoufoxEngine` (`app/engine.py:129`) owning a **single shared, provider-blind** `ProfileManager` pool (prod `STEALTH_POOL_SIZE=4`; `app/config.py:43,109`, `app/profiles.py:37-48`). Three paths compete for it: `resolve()` (retains a `Session`), `browser_fetch()` (warm session keyed `fetch::{provider}::{origin}`, `engine.py:549-552`), and `proxy_fetch()` (`/hls`).

The AUTO-527 cascade has **two compounding causes**:

1. **Warm-session poison loop (root).** nineanime reuses the long-lived `fetch::nineanime::https://9anime.me.uk` session. When `9anime.me.uk` hangs, the 20 s in-page `fetch` timeout throws `Target closed` (`engine.py:613-614,638-645`), poisoning that session's page. The next lease **re-navigates the dead page** (`engine.py:615-629`) → re-hits `Target closed` → **burns one pool slot per attempt**.
2. **Shared-pool starvation.** Wedged sessions hold profiles during the stall + teardown; `_acquire_profile` spins 50 × 0.1 s then returns `None` → `PoolExhausted` → 503 (`engine.py:377-383`). gogoanime, sharing the pool with **no quota or partition**, gets the **same** 503 even though its upstream is healthy → the whole EN failover chain 502/503-storms.

Two reasons it never recovers on its own:

- **`/healthz` always returns HTTP 200** even when `free==0` (body says `degraded`, status stays 200; `app/main.py:81-84`, `engine.py:173-189`) → Docker liveness never fires; the maintenance bot restarts the container by hand.
- **Provider status is frozen at boot.** The orchestrator reads `enabled|degraded|disabled` once into a map (`services/scraper/cmd/scraper-api/main.go:242-255`); `StartProvidersRefresher` swaps engine/base-URL routing but **never re-gates failover** (`services/scraper/internal/config/providers_refresh.go:19-49`); catalog exposes only a **read** endpoint (`services/catalog/internal/handler/internal_scraper_providers.go:33`) with **no status-write path**; the analytics probe writes verdicts to Prometheus + ClickHouse, never to `stream_providers.status`. So degraded → enabled "resurrection" is 100 % manual today.

## Locked decisions

| # | Decision | Choice |
|---|----------|--------|
| D1 | Capacity model | **RAM-budgeted**, not instance-based: combined Camoufox RSS **soft 4 GB / hard 6 GB**. `STEALTH_POOL_SIZE` retires as a hard cap (kept as a high fail-safe only). |
| D2 | Fairness axis | **Per-user quota: ≤ 2 concurrent sessions per `user_key`** (replaces per-provider quota). |
| D3 | Provider isolation | Achieved by **fast self-heal + RAM headroom + Go circuit breaker**, not a static per-provider wall. |
| D4 | Resurrection scope | **Full** — in-sidecar self-heal **and** durable probe-driven provider auto re-enable (catalog status-write + orchestrator runtime re-gate). |
| D5 | Re-enable authority | **Automatic enabled ↔ degraded** after N healthy signals; **`disabled` stays human-only** (mirrors the "only `resolved` is human-only" feedback rule). |
| D6 | nineanime placement | **Keep on the shared pool**, protected by the new self-heal + RAM headroom + Go breaker. No separate sidecar. |
| D7 | Phase 1 focus | **Self-heal** — the single most urgent slice; fixes the live incident on its own. |
| D8 | `/readyz` semantics | New `/readyz` is **observability only** (metrics / Go health poller). It does **not** drive a Docker/k8s restart — the sidecar must stay live so in-flight streams keep playing; recovery is in-process self-heal. |

## Architecture

Five components across four services, delivered in five independently deployable phases. **Phase 1 alone closes AUTO-527's collateral damage.**

### Component 1 — Sidecar self-heal *(Phase 1, FOCUS)* — `services/stealth-scraper/`

- **Poison-fence the warm session.** `Session` gains `crash_count` + `last_error` (`engine.py:79-102`). Before reusing a `fetch::{provider}::{origin}` session (`engine.py:549-555`), run a cheap liveness probe `await page.evaluate('()=>1')`; on failure, **evict + recreate** instead of inheriting a poisoned page. In `_in_page_fetch` (`engine.py:602-636`), on `Target closed` / `context was destroyed` do **not** nav-retry the same page — increment `crash_count`; once `crash_count >= POISON_MAX` (default **2**), `aclose_session` + `_teardown` that profile and raise so the caller fails over. Add a short grace-wait after `handle.close()` (`engine.py:120-126,233-242`) so the relaunch does not inherit a half-dead WebSocket.
- **Profile health + reaper resurrection.** `Profile` (`profiles.py:21-35`) gains `status` (`healthy|crashed|warming`) + `consecutive_fail` + `last_crash`. `_teardown(reason='crash')` marks the slot `crashed` (with metadata) rather than silently clearing. The 30 s reaper (`engine.py:680-708`) gains `_resurrect_crashed_slot`: for `crashed`, **not-in-use** slots past a per-slot backoff, attempt a cold `_ensure_browser` relaunch (`engine.py:192-231`); success → `healthy`, after **3** consecutive failed resurrects → retire + `_rm_dir`. Backoff is exponential per slot: 1 → 2 → 4 → 8 → 16 → 30 s (cap).
- **Real readiness signal.** Keep `/healthz` at **200** (process liveness). Add **`/readyz`** in `main.py` returning **503** when the pool is saturated for a sustained window (`free==0` for ≥ **15 s**), else 200. Expand `engine.health()` to a per-provider **and** per-user breakdown: `{global:{free,crashed,warming,ram_bytes}, providers:{<name>:{held,crashed,last_error}}, users:{<key>:{held}}}`.
- **`kind` in error bodies.** `/resolve` and `/fetch` 503 bodies carry a machine-readable `kind`: `provider_wedged` (poison/crash), `pool_exhausted`. (`capacity` + `user_quota` arrive in Phase 2.)
- **Self-heal metrics** (`app/metrics.py`): `stealth_pool_free`, `stealth_pool_crashed`, `stealth_slot_resurrect_total{result}`, plus the existing `stealth_browser_relaunch_total{reason}`.

### Component 2 — RAM-budgeted capacity + per-user quota *(Phase 2)* — `services/stealth-scraper/` (+ thin catalog/scraper plumbing)

- **RAM sampler + admission controller.** A background task samples **combined Camoufox RSS** every few seconds by summing the RSS of the Camoufox/Firefox process trees via `/proc/<pid>/statm` (dependency-free; no new package). Thresholds:
  - **Soft 4 GB** — stop warming new profiles; proactively evict idle/expired not-in-use sessions (back-pressure). Existing leases untouched.
  - **Hard 6 GB** — refuse a new browser launch → `503 {kind:"capacity"}`; evict the LRU not-in-use session to reclaim. Never exceed.
  - Instance count is **derived** from RAM, not capped at 4. `STEALTH_POOL_SIZE` becomes a high fail-safe ceiling used only if the RSS read fails.
  - Config: `STEALTH_RAM_SOFT_BYTES` (default `4294967296`), `STEALTH_RAM_HARD_BYTES` (default `6442450944`), `STEALTH_RAM_SAMPLE_SECONDS` (default `5`).
- **Per-user quota.** `ResolveRequest`/`FetchRequest` (`main.py:33-56`) gain an optional `user_key: str`. The engine enforces **≤ `STEALTH_USER_QUOTA` (default 2)** concurrent held sessions per `user_key`; over-quota → `503 {kind:"user_quota"}`. `user_key` is threaded end-to-end:
  - **catalog** `GetScraperStream` extracts the authenticated user ID (already available at the gateway) and passes it to the scraper via an `X-AE-User` request header.
  - **scraper** stream handler forwards it to `sidecar.Client`; `sidecar.Client.resolve/fetch` set `user_key` on the request body.
  - **Fallback:** when no authenticated user is present, `user_key` = a salted hash of the client IP (so anonymous traffic is still bounded, never globally shared).
- **Compose.** `services/stealth-scraper` `mem_limit: 3500m → 7g` (6 GB hard + Xvfb/python overhead). **Planning gate:** confirm host RAM headroom before raising. Document `STEALTH_RAM_*` + `STEALTH_USER_QUOTA` in `docker/docker-compose.yml` and `docker/.env.example`.

### Component 3 — Go scraper: kind surfacing + circuit breaker + runtime re-gate *(Phase 3)* — `services/scraper/`

- **Surface `kind`.** `sidecar.Client` parses `kind` then collapses it into `ErrProviderDown` (`internal/sidecar/client.go:143-161,197-217`). Wrap it in a typed `ProviderWedgedError{Kind}` that still satisfies the failover `ErrProviderDown` classification (`internal/service/orchestrator.go:174-189`) but lets the breaker inspect the cause.
- **Circuit breaker** (wires the existing, in-prod-unused `InMemoryHealthCache`, `internal/health/cache.go`). On **≥ 3** wedged-kind errors (`provider_wedged|pool_exhausted|capacity|user_quota`) within **60 s** for a provider, `cache.Update(provider, Up=false)`. The orchestrator already skips `!cache.IsHealthy(name)` (`orchestrator.go:317,536`) → the wedged provider drops out of failover **per-request** (protects gogoanime in real time, not in 15 min). Recovery: **half-open after 120 s** — let one trial request through; success → clear. (The existing fail-open `cacheStaleTTL` of 30 min remains the backstop.)
- **Orchestrator runtime re-gate.** On every `StartProvidersRefresher` poll (`providers_refresh.go:19-49`), re-evaluate each provider's catalog `status` and move it **in/out** of the orchestrator's `degraded` failover map — so a catalog status change (from the probe, Component 5) takes effect **without a scraper restart**. (Today `registerByStatus` reads status once at boot and never revisits it.)

### Component 4 — Catalog status-write endpoint *(Phase 4)* — `services/catalog/`

- **`POST /internal/scraper/providers/{name}/status`**, sibling of the read-only `List` (`internal/handler/internal_scraper_providers.go:33`). Body `{status, reason}`. Constraints:
  - **Docker-network-only** — not gateway-proxied (the gateway exposes no `/internal/*`), matching every other `/internal/*` producer route.
  - Idempotent `UPDATE stream_providers SET status, reason, updated_at`.
  - **Refuses to set `disabled`** and **refuses to change an already-`disabled` provider** → `409 Conflict` (human-only gate; mirrors the `resolved` feedback-status rule). Only `enabled ↔ degraded` are writable.
  - **Audited**: structured log of `caller, provider, old→new status, reason`.
  - Returns `200` on apply, `409` on a forbidden transition, `404` on unknown provider.

### Component 5 — Analytics probe writeback *(Phase 5)* — `services/analytics/`

- Off the per-slot pass-percentage **Rollup** scorer (shipped 2026-06-22, `internal/probe/scorer.go`). After each probe run, for every scraper-operated provider:
  - provider currently `enabled` **and** scored **DOWN (0 % slots)** → call catalog status-write `enabled → degraded`.
  - provider currently `degraded` **and** scored **UP (> 50 %)** for **N = 2 consecutive runs** (anti-thrash) → `degraded → enabled` (**resurrection**).
  - Never touches `disabled`.
- Gated by `PROBE_AUTOGATE_ENABLED` (default **on**; dark-shippable). Consecutive-UP state is tracked in the probe's existing per-provider result aggregation (no schema change). Metric `probe_autogate_transitions_total{from,to,provider}`.

## Data flow — the new path during a nineanime storm

1. `9anime.me.uk` hangs → poison-fence kills nineanime's poisoned warm session after 2 strikes + tears down the profile; **gogoanime's profiles are untouched** and keep streaming. RAM headroom (≈ up to 6 GB) leaves room for new gogoanime launches.
2. Sidecar 503s nineanime requests with `kind=provider_wedged`.
3. Go breaker trips nineanime after 3 strikes → orchestrator skips it → **no retry storm**; gogoanime served.
4. Reaper resurrects nineanime's crashed slots in the background — **no container restart**.
5. Probe run scores nineanime DOWN → catalog `enabled → degraded` → dashboard shows honest `degraded`.
6. nineanime upstream recovers → 2 consecutive UP runs → catalog `degraded → enabled` → orchestrator runtime re-gate brings it back into failover. **No human, no restart, no migration.** AUTO-527 closed.

## Error handling

- Every sidecar refusal carries a `kind` so the Go side can distinguish "this provider is wedged / over budget" from "this stream genuinely failed." All wedged kinds remain retryable in the failover classifier (so failover still advances), but also feed the breaker.
- Self-heal must be **fail-safe**: if the RSS read fails, fall back to the `STEALTH_POOL_SIZE` ceiling; if a resurrect attempt throws, count it as a failed resurrect (toward the retire-after-3 rule), never crash the reaper loop.
- The catalog status-write is **fail-closed on authority**: any attempt to set/clear `disabled` is rejected, never silently coerced.
- The probe autogate is **fail-open**: a catalog status-write error is logged and retried next run; it never blocks or fails the probe run itself.

## Testing strategy

- **Phase 1 (Python, `pytest`):** poison-fence evicts+recreates a dead warm session (no nav-retry); `crash_count >= POISON_MAX` tears down the profile; reaper resurrects a `crashed` slot and retires after 3 failures; `/readyz` 503 on sustained `free==0` while `/healthz` stays 200; `health()` per-provider/per-user shape; `kind` present in 503 bodies. Extend `tests/test_engine_lifecycle.py`, `tests/test_engine_fetch.py`.
- **Phase 2 (Python):** RAM sampler reads `/proc` RSS; soft-limit stops warming + evicts idle; hard-limit refuses launch (`kind:capacity`) + evicts LRU; per-user quota rejects the 3rd concurrent session (`kind:user_quota`); fallback `user_key` from IP hash. (catalog/scraper plumbing covered by Go tests below.)
- **Phase 3 (Go):** `kind` surfaces through `sidecar.Client` as `ProviderWedgedError`; breaker trips after 3/60 s, half-opens after 120 s, clears on success; orchestrator skips a cache-down provider and re-gates when refresher status changes. Mock the sidecar + catalog — **no live API**.
- **Phase 4 (Go):** status-write happy path (`enabled↔degraded`); `409` on set-`disabled` and on changing an already-`disabled` provider; audit log emitted; route is **not** gateway-registered.
- **Phase 5 (Go):** degrade-on-DOWN; re-enable only after 2 consecutive UP; `PROBE_AUTOGATE_ENABLED=false` no-ops; never touches `disabled`; transition metric increments.
- **E2E / manual validation:** point `SCRAPER_NINEANIME` (DB `base_url`) at a black-hole host to force the wedge; confirm gogoanime keeps streaming, the breaker trips nineanime, the reaper revives slots without a restart, the probe auto-degrades then (on restore) auto-re-enables.

## Config / env summary

| Var | Service | Default | Meaning |
|-----|---------|---------|---------|
| `STEALTH_RAM_SOFT_BYTES` | stealth-scraper | `4294967296` | Soft RAM budget (stop warming + evict idle) |
| `STEALTH_RAM_HARD_BYTES` | stealth-scraper | `6442450944` | Hard RAM budget (refuse launch + reclaim) |
| `STEALTH_RAM_SAMPLE_SECONDS` | stealth-scraper | `5` | RAM sampler interval |
| `STEALTH_USER_QUOTA` | stealth-scraper | `2` | Max concurrent sessions per `user_key` |
| `STEALTH_POOL_SIZE` | stealth-scraper | `4` | **Retired as hard cap**; high fail-safe ceiling only |
| `PROBE_AUTOGATE_ENABLED` | analytics | `true` | Enable probe-driven `enabled↔degraded` writeback |
| (compose) `mem_limit` | stealth-scraper | `7g` | Raised from `3500m` to fit the 6 GB hard budget |

## Security & invariants

- All new cross-service endpoints (`/internal/scraper/providers/{name}/status`) are **Docker-network-only**, never gateway-exposed.
- `disabled` is a **human-only** terminal state — no automated path may set or clear it.
- `user_key` is opaque to the sidecar (an ID or a salted IP hash); it is used only for quota accounting, never logged in clear or persisted.
- The breaker and self-heal hold **no durable state** beyond the in-memory pool/cache — safe across restarts; the durable signal of record is `stream_providers.status` in catalog Postgres.

## Phasing (independently deployable + testable)

1. **Phase 1 — Sidecar self-heal** *(FOCUS)*. Deploy `stealth-scraper`. Closes AUTO-527's restart band-aid.
2. **Phase 2 — RAM budget + per-user quota.** Deploy `stealth-scraper` (+ thin catalog/scraper `user_key` plumbing) + compose `mem_limit` bump.
3. **Phase 3 — Go scraper kind + breaker + runtime re-gate.** Deploy `scraper`.
4. **Phase 4 — Catalog status-write endpoint.** Deploy `catalog`.
5. **Phase 5 — Analytics probe writeback.** Deploy `analytics` (flag-gated).

## Scoring (`.planning/CONVENTIONS.md`)

- **UXΔ = +3 (Better)** — EN playback survives a flaky provider; the pool self-heals and recovered providers come back on their own, replacing manual restarts/SQL.
- **CDI = 0.08 * 34** — moderate multi-service spread (4 services + compose), new patterns introduced (RAM-budgeted pool, cross-service `user_key` thread, orchestrator runtime re-gate that supersedes the boot-frozen status, catalog status-write), large multi-phase effort.
- **MVQ = Phoenix 88%/85%** 🔥 — rises-from-ashes: the pool dies and revives itself, and downed providers are resurrected. Strong archetype match; resists slop via per-phase tests + a fail-safe/fail-open posture.

## References

- Maintenance escalation: **AUTO-527** (`docs/issues/issues.json`), Telegram msg 3370.
- nineanime Camoufox migration: `docs/superpowers/specs/2026-06-21-nineanime-camoufox-migration-design.md` (commit `53bae927`).
- Scraper framework: `docs/scraper-framework.md`.
- Honest per-provider availability (the %-scorer this builds on): `docs/superpowers/specs/2026-06-21-honest-per-provider-availability-design.md`.
