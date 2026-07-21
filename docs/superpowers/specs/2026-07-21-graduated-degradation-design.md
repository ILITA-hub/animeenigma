# Graduated Degradation — score-driven consumer curves (2026-07-21)

**Status:** design approved by owner (2026-07-21), pre-implementation.
**Extends:** `2026-07-10-graceful-degradation-design.md` (Phases 1–3, all LIVE). This spec does NOT
replace the integer level machine — it adds a continuous track alongside it.

## Problem

The shipped system is coarse: three levels (0 Normal / 1 Elevated / 2 Critical), and consumers
step down in one or two hard jumps (content-verify sheds probe loops one-per-level; Camoufox stops
warming at L1, refuses new work at L2). The owner wants proportional shedding — worker/instance
counts that slide with pressure and demand instead of cliff-dropping, e.g.:

- content-verify: low pressure → up to 6 probes (demand-dependent), mid → 5–2, high → 2–0.
- Camoufox: low → up to 6 instances (demand-dependent), mid → 5–2, high → 2–1.

## Owner decisions (Q&A, 2026-07-21)

1. **Drive signal = continuous PSI score**, not raw utilization %. The governor normalizes its
   existing PSI breach ratios to a 0.00–1.00 score. Rationale: this box idles at ~90% swap with a
   high busy baseline; raw-% triggers would false-positive forever (the founding Phase-1 insight,
   re-proven by the 2026-07-14 Elevated recalibration). "% system usage" in the owner's bands is
   interpreted as **score×100**.
2. **Demand model = joint pressure × demand.** `count = f(score, demand)`. Deep backlog at low
   pressure ramps toward the max; shallow backlog stays low even on an idle box. Each consumer owns
   an explicit local demand signal.
3. **Curve owner = consumer-side.** Governor publishes only the score; each consumer combines it
   with local demand via its own env-tunable curve (the existing `DegradationWatcher` pattern).
   Governor stays storage-free and consumer-agnostic.
4. **Camoufox top end = floor 1 until hard-critical, then refuse.** The curve bottoms out at 1
   warm instance through the high band; the existing sustained-L2 `DegradedShed` 503 backstop
   survives above it. Breaker/park behavior in the scraper is unchanged.
5. **Camoufox scale-down = 3-stage escalation** (owner-specified): lazy below the kill threshold;
   graceful drain + stream migration when `current > ceil(pool_target/2)+1` (**relative to the
   current curve target**, owner-confirmed — NOT the static max); forceful kill + honest
   "high load" 503 when graceful is impossible or RAM stays over the hard budget.

## Reality constraints (bind before the curve does)

- **Camoufox is RAM-bound:** pool capacity is governed by combined RSS (soft 2 GiB / hard 3 GiB,
  `services/stealth-scraper/app/config.py`); `STEALTH_POOL_SIZE=4` is only a fail-safe ceiling. At
  ~1 GiB per warm Firefox, 6 instances ≈ 6 GiB — double the hard budget. Effective capacity =
  `min(curve(score), demand, ram_capacity, STEALTH_POOL_SIZE)`. Reaching 6 requires the owner to
  raise RAM budgets (not recommended on this box); the curve design is identical either way.
- **content-verify workers are clamped 1..4** (`clampWorkers`) and the container `mem_limit: 2g`
  is sized for 2 concurrent whisper runs. 6 probes needs the clamp raised to 6 AND a mem_limit
  decision (~1 GiB per concurrent whisper run). Same framing: curve = pressure ceiling, RAM = an
  independent binding cap.

## 1. Pressure score pipeline

**Recording rules** (`docker/prometheus/rules/degradation.yml`, dir-mounted ⇒ deploy = edit +
`POST /prometheus/-/reload`, no recreate — threshold changes stay rule-only, as with the 07-14
recalibration):

- Per-signal normalized score, piecewise-linear anchored to the EXISTING thresholds:
  `0` at half the elevated threshold → `0.5` at elevated → `1.0` at critical (clamped 0–1).
  Signals: `psi_cpu_some`, `psi_io_full`, `psi_mem_full`, `mem_available` (inverted).
- `ae:pressure_score:preview` = **max** across the per-signal scores.

**Governor** (already ingests `{__name__=~"ae:.+"}` every tick — zero query changes):

- Applies asymmetric smoothing mirroring the level machine's hysteresis: rise fast (~60 s to track
  a genuine ramp), decay slow (~5 min), flap-pinned. This damps the self-feedback loop
  (probes → pressure → fewer probes → less pressure → …): counts step down and stay down rather
  than oscillating.
- Publishes: Redis `ae:degradation:score` (string like `"0.42"`, TTL 60 s = fail-open) alongside
  the existing `level`/`reasons` keys; gauge `ae_degradation_score`.
- **The integer-level machine is untouched.** Level 0/1/2, override, CH transitions, and every
  binary consumer (library gates, scheduler crons, catalog backfill) work unchanged.
- **Manual override:** `override set 1` additionally pins score to 0.5; `set 2` → 1.0; `set 0` →
  0.0; `clear` returns both level and score to computed values next tick.
- Governor HTTP `/api/degradation/status` response gains a `score` field (Camoufox's poll path).

**Band translation** (score×100 ≡ owner's "% usage"; Elevated = 0.5, Critical = 1.0 nest inside):

| score | content-verify cap | Camoufox pool_target |
|---|---|---|
| < 0.40 | 6 | 6 |
| 0.40–0.60 | 5 → 2 (linear) | 5 → 2 (linear) |
| 0.60–0.80 | 2 → 0 (linear) | 2 → 1 (linear) |
| ≥ 0.80 | 0 | 1 (+ sustained-L2 `DegradedShed` backstop) |

## 2. content-verify — graduated probe loops

- `libs/cache` `DegradationWatcher` gains `Score() float64` (nil-safe; missing key / error ⇒ 0.0).
- `clampWorkers` raised to 1..6; `CV_WORKERS` loops spawn as today.
- Each tick, loop `i` computes `cap = min(curveCV(score), demandCap)` and sits out when `i >= cap`
  — the existing static `shedMin` integer generalizes into the same one-at-a-time shedding, now
  continuous. Note this is deliberately LESS aggressive than today at mid-pressure: at score 0.5
  (= today's L1) the curve still allows ~3 loops where the old shedMin dropped to 1 — that
  softening is the point of graduation, not an accident. Caps are `floor()`-rounded from the
  linear interpolation.
- **Demand:** `demandCap = ceil(pending_units / CV_DEMAND_PER_WORKER)` from the unit queue the
  worker already enumerates — a shallow queue doesn't spin 6 whisper-capable loops on an idle box.
- Curve breakpoints env-tunable (`CV_CURVE`), defaults per the band table.
- Metrics: `cv_worker_cap{source="pressure"|"demand"}` (which constraint binds) + existing
  `cv_ticks_skipped_total{reason="degraded"}`.
- ⚠ Register new metrics in the service-local cvmetrics package, NOT libs/metrics
  (auto-registration trap — plain promauto in libs exports permanent-0 impostors from every
  importer).

## 3. Camoufox — graduated pool with 3-stage scale-down

`pool_target = clamp(curveCFX(score), 1, min(STEALTH_POOL_SIZE, ram_capacity))`.
Scale-UP stays demand-driven (warming only on real resolve traffic) and gated by
`_warming_allowed()`, which changes from the binary `level >= 1` to `warm_count < pool_target`
(RAM soft-budget check retained).

**Scale-DOWN escalation ladder** (owner-specified). Kill threshold = `ceil(pool_target/2) + 1`,
relative to the CURRENT target:

| score | pool_target | kill threshold | at current=5 |
|---|---|---|---|
| 0.35 | 6 | 4 | 5 > 4 → drain |
| 0.50 | 4 | 3 | 5 > 3 → drain |
| 0.70 | 2 | 2 | 5 > 2 → drain |
| 0.85 | 1 | 2 | 5 > 2 → drain; floor 1 survivor |

- **Stage 0 — lazy** (`current ≤ threshold`): stop warming above target, LRU-evict idle sessions
  only. No kills, streams untouched.
- **Stage 1 — graceful drain + stream migration** (`current > threshold`):
  - Victim selection: LRU order, preferring browsers with zero active streams, then fewest
    (per-session `in_use` refcount already exists).
  - Victim marked **draining**: admits no new streams or resolves.
  - Active streams on a victim are **migrated**: re-resolve the same `{provider, episode, server}`
    on a surviving browser and atomically swap the session→browser mapping the `/hls` proxy uses.
    (A stream cannot be literally handed over — `/hls` fetches ride the session's own cookies/TLS
    fingerprint context — so "redirect" = fresh resolve on the survivor; the player never sees a
    URL change.)
  - Victim closes cleanly once its `in_use` reaches 0 (migrated or naturally ended).
- **Stage 2 — forceful kill + honest 503** when graceful isn't enough:
  - Triggers: no survivor capacity for the migration · provider needs a distinct profile ·
    migration re-resolve fails · combined RSS still over the HARD budget after draining.
  - Victim killed outright. Its in-flight `/hls` fetches and subsequent segment requests return
    **503 `kind="degraded"` ("high load")** — the consumer-facing signal. The scraper already
    parks on `"degraded"`; the player's self-heal / connection-badge path surfaces it honestly.
  - A RAM hard-budget breach may skip Stage 1 (memory emergencies don't wait for drains).

**Explicit revision of a Phase-3 guarantee:** Phase 3 promised in-flight sessions and `/hls` are
never gated. Under sustained over-capacity that guarantee now bends: active streams are
migrate-then-shed rather than sacred. Mitigation: redirect before kill; forced kills are the last
rung and are honestly signaled. Session reuse and `/hls` on NON-victim sessions remain ungated.

**Backstop unchanged:** sustained L2 still refuses all new `resolve`/`browser_fetch` via
`DegradedShed` 503; scraper breaker/park behavior unchanged.

Metrics: `stealth_pool_target`, `stealth_pool_kills_total{mode="graceful"|"forced"}`,
`stealth_stream_migrations_total{result="ok"|"failed"}` (+ existing
`stealth_degradation_level_seen`).

## 4. Observability + safety

- Degradation-overview dashboard: score timeline beside the level timeline (state row);
  `cv_worker_cap` + `stealth_pool_target` panels in the heavy-actors row. Content-verify's own
  dashboard gets the cap panel too.
- Fail-open everywhere: missing score key / governor unreachable / Redis down ⇒ score 0.0 ⇒ full
  speed — identical philosophy to the level track.
- Binary consumers (library, scheduler, catalog backfill) intentionally stay on the level track;
  migrating them to curves is out of scope.

## 5. Testing

- Governor: table-driven tests for per-signal normalization + asymmetric smoothing (pure funcs).
- content-verify: extend worker tests — fake ShedChecker returning a score; assert per-loop
  sit-out at each band boundary and the pressure-vs-demand `min()` interplay.
- Camoufox: unit tests for `pool_target` math, kill-threshold arithmetic, victim ordering, and
  Stage-1→Stage-2 trigger conditions (mock sessions with `in_use` refcounts).
- Live E2E: override drill (`bin/degradation-override.sh set 1` ⇒ score 0.5 ⇒ cv cap ≈ 3, pool
  target ≈ 4; `set 2` ⇒ caps 0/1 + backstop) mirroring the Phase-3 verification, plus a
  drain-migration drill with an active stream on a victim browser.

## Scores

- **UXΔ = +1 (Better)** — smoother shedding under moderate load means fewer full stops of source
  resolution and probing; forced kills are rarer than today's blanket L2 refusal and are honestly
  surfaced when they happen.
- **CDI = 0.04 * 21** — five touch points (rules, governor, libs/cache, content-verify,
  stealth-scraper), every change additive alongside the untouched level machine; the Camoufox
  migration machinery is the dominant effort driver.
- **MVQ = Griffin 80%/85%** — graduated vigilance; sheds one feather at a time, talons out only at
  the hard budget.
