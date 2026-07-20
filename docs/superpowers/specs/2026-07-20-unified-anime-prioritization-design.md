# Unified Anime Prioritization Model — Design

> **For agentic workers:** this is a design spec. The implementation plan follows via `superpowers:writing-plans`.

**Goal:** Replace the flat, visitor-dominated content-verify priority score with a **banded interest model** focused on ongoings, and express notifications + autocache under the **same signal vocabulary** — so "what the platform cares about right now" is computed once and projected into each subsystem's own execution discipline. When the hot set (ongoing / top-100 / watched) is exhausted, content-verify keeps working down user watchlists and top-200/300/… windows by round-robin instead of idling.

**Architecture (one sentence):** A shared **interest-signal layer** in catalog (bands + raw signals per anime) is consumed by three **policy projections** — content-verify (weighted band claim + idle backfill), notifications (next-episode-proximity cadence tiers), autocache (weighted drain by reason-class) — each keeping its own correctness guarantees.

**Tech stack:** Go (catalog / content-verify / notifications / scheduler / library), PostgreSQL (catalog `animeenigma` DB; library separate `library` DB), Redis (signals, cursors, cooldowns).

## Global Constraints

- **content-verify k8s replicas MUST stay 1** — probe leases are in-process only; a second replica double-probes. Nothing here changes that.
- **library is on a SEPARATE Postgres** (`library` DB) and CANNOT join `watch_history` / `anime_list` / `animes`. Autocache's band signal must ride the existing `autocache_demand.reason` wire field — no new cross-DB join.
- **No time-effort units** anywhere (UXΔ/CDI/MVQ only, per `.planning/CONVENTIONS.md`).
- **Fail-open everywhere:** any Redis / catalog-endpoint error degrades to "treat as lowest actionable signal", never blocks a probe/notify/drain tick. Current code already fails visitor-signal to 0 (`signals.go:55-65`) — preserve that discipline.
- **Notifications delivery floor is inviolable:** every watched ongoing combo MUST be checked at least once per `NOTIF_TIER_FLOOR` (default 6h). Tiering may delay a cold title's check, never drop it.
- **Weights/proportions are env-configurable** — the current hard-coded consts (`queue.go:12-23`) become config-backed. No behavior-affecting magic numbers left compile-time.
- **Co-authors** on every commit (Claude Code / 0neymik0 / NANDIorg).

---

## 1. The shared model: bands + raw signals

Every anime is assigned, from catalog data, a **band** (priority class) and carries a small set of **raw signals**. Consumers read whichever they need.

### Bands (highest priority first)

| Band | Definition | Who uses it |
|------|-----------|-------------|
| **P — Pinned** | operator pin (`CV_PIN_ANIME`) | content-verify only |
| **1 — Hot ongoing** | `status='ongoing'` AND visible | content-verify, notifications (sub-tiered), probe (opt) |
| **2 — Watched + Top** | non-ongoing AND (`visitors > 0` OR in browse-order top-100) | content-verify |
| **3 — Idle backfill** | everything else visible, surfaced round-robin | content-verify only |

### Raw signals (per anime)

- `visitors` — unique visitors over 7d (existing `cv:visit:*` cardinality, `signals.go:53-59`). **Unchanged source.**
- `ongoing` — `status='ongoing'`.
- `top_rank` — 1-based rank in browse order (`sort_priority DESC, score DESC`), or 0 if outside the top window.
- `score` — MAL/community score (existing `animes.score`).
- `episodes_aired`, `next_episode_at` — existing catalog columns; drive notifications' proximity tier and cv freshness.
- `planners` — count of `anime_list.status='plan_to_watch'` rows (idle sub-source ordering). Prod today: 466 rows.

The band ladder is **content-verify's** interpretation. Notifications and autocache read the raw signals (or, for autocache, an equivalent reason-class already on the demand row) and apply their own policy — they do NOT execute the Band-1/2/3 claim ladder, because their candidate sets are structurally different (notifications = ongoing-only; autocache = demand-driven on a foreign DB). This is the "unify the model, not the queue" principle.

### Where computed

Catalog owns all of it (it owns the `animes` table + browse ranking). New endpoint (§2). content-verify's visitor signal stays where it is (Redis, written by catalog/player hints — `verify.go:86-96`); the endpoint supplies only the DB-derived terms so the two signal sources compose exactly as today.

---

## 2. Catalog endpoint: `/internal/interest/bands`

Evolve the existing `GET /internal/verify/membership` (`internal_verify.go`, `ListVerifyMembership` `anime.go:635-650`) into a superset. Keep the old route as a thin alias for one release (no big-bang cutover).

```
GET /internal/interest/bands?ongoing_limit=500&top_limit=100&idle_window=100&idle_offset=<N>
```

Response `data`:
```json
{
  "ongoing":  [ {id,name,episodes_aired,next_episode_at,score} ... ],   // Band 1
  "top":      [ {id,name,episodes_aired,score,top_rank} ... ],          // Band 2 (top slice)
  "planned":  [ {id,name,episodes_aired,score,planners} ... ],          // Band 3 sub-source (a)
  "idle_window": [ {id,name,episodes_aired,score,top_rank} ... ],       // Band 3 sub-source (b): rows [idle_offset, idle_offset+idle_window) of browse order, excluding ongoing+top
  "idle_total": 4824                                                     // count of visible non-ongoing non-top — lets cv wrap the cursor
}
```

- `ongoing` / `top` queries are the current two `ListVerifyMembership` queries plus the extra projected columns (`next_episode_at`, `top_rank`, `planners`). Docker-network-only, no gateway route (`/internal/*` unproxied) — unchanged security model.
- `planned`: `SELECT a.id … , count(al.*) AS planners FROM animes a JOIN anime_list al ON al.anime_id=a.id AND al.status='plan_to_watch' WHERE a.status<>'ongoing' AND (a.hidden IS NOT TRUE) GROUP BY a.id ORDER BY planners DESC LIMIT idle_window`.
- `idle_window`: browse-order rows offset into the tail — `ORDER BY sort_priority DESC, score DESC OFFSET idle_offset LIMIT idle_window`, filtered to non-ongoing and beyond the top slice. content-verify advances `idle_offset` itself (cursor in Redis, §3) and wraps when it passes `idle_total`.
- **Kill the dead `CV_TOP_LIMIT`** (`config.go:76`, never wired): either wire it to the endpoint's `top_limit` or delete it. Plan deletes it and lets the caller pass `top_limit` explicitly.

---

## 3. Projection A — content-verify (the core change)

**File anchors:** weights `queue.go:12-23`, `Score()` `queue.go:35-47`, `BuildCandidates` `queue.go:53-89`, `Rank` `queue.go:91-103`, `CooldownTTL` `queue.go:152-157`, claim loop `engine.go:346-415`, config `config.go`.

### 3.1 Band assignment replaces the flat score

`Candidate` gains a `Band` (P/1/2/3) and keeps its raw signals. `Score()` is replaced by **`BandOf(c)` + `IntraScore(c)`**:

- `BandOf`: Pinned→P; ongoing→1; (visitors>0 OR top_rank>0)→2; else→3.
- `IntraScore` (sort key WITHIN a band, all DESC):
  - **Band 1:** `freshBoost, visitors, score`. `freshBoost` = 1 when `next_episode_at` within ±`CV_FRESH_WINDOW` (default 48h) of now — a just-aired or imminent episode jumps its title to the front of the ongoing band. Else 0.
  - **Band 2:** `visitors, -top_rank, score` (watched-heaviest and highest-ranked first).
  - **Band 3:** sub-source order is set by the round-robin cursor, not a score; within a fetched window, `planners` (planned) or `score` (top-window) DESC.

`Rank` becomes per-band (stable sort within the band's slice). The global flat `Rank` over all candidates is removed.

### 3.2 Weighted band claim + fall-through

Replace "walk one globally-ranked list" with: **pick a band by weighted lottery, take the top *actionable* candidate in it; if that band has none, fall through to the next lower band.** Actionable = not in cooldown (pins bypass) AND has pending verify/skip work (existing `PendingUnits` / skip-lane checks, `engine.go:352-381` — unchanged).

- Proportions: `CV_BAND_WEIGHTS="60,30,10"` (Band1/2/3). Band P is always tried first (pins must head the queue NOW — preserves `weightPinned` intent).
- Lottery is per-claim; over many 10s ticks the mix converges. Empty higher bands never waste a tick (fall-through).
- Band 3 is entered by its 10% slot OR when Bands 1+2 have nothing actionable — so the tail gets steady, bounded attention without starving the hot set, and idle time is never truly idle while catalog tail remains.

### 3.3 Idle round-robin cursor

Band 3 alternates its two sub-sources and advances a window so it sweeps the whole catalog tail over time.

- Cursor in Redis `cv:idle:cursor` = `{source: "planned"|"window", offset:int}`. Each Band-3 claim: consult cursor → request that sub-source from the endpoint (`planned`, or `idle_window` at `offset`) → after draining a window, flip source and/or advance `offset += idle_window`, wrapping at `idle_total`. Fail-open: missing cursor → start at planned, offset 0.
- **Idle cooldown is long:** settled Band-3 titles get `CV_IDLE_COOLDOWN` (default 168h/7d) instead of 24h, so the tail doesn't re-spin before the cursor has swept forward. `CooldownTTL` gains a band arg: Band1 6h (unchanged), Band2 24h (unchanged), Band3 7d.

### 3.4 Config knobs (all new, env-backed)

| Env | Default | Meaning |
|-----|---------|---------|
| `CV_BAND_WEIGHTS` | `60,30,10` | lottery proportions Band1/2/3 |
| `CV_FRESH_WINDOW` | `48h` | ±window on `next_episode_at` for Band-1 freshBoost |
| `CV_IDLE_COOLDOWN` | `168h` | cooldown for settled Band-3 titles |
| `CV_IDLE_WINDOW` | `100` | idle sub-source page size |
| `CV_TOP_LIMIT` | (delete/wire) | currently dead; wire to endpoint `top_limit` or remove |

Weights parse to the `Score()` replacement — no compile-time priority magic remains.

---

## 4. Projection B — notifications (cadence tiers within ongoing)

**File anchors:** hot-combo query `hotcombos.go:56-72`, detector `detector.go:91-183`, scheduler cron `scheduler.go:82-123`, config `config.go:106-115`.

Notifications candidates are **all ongoing** (the query hard-filters `a.status='ongoing'`), so the Band ladder collapses to "everything is Band 1." The meaningful sub-division is **next-episode proximity**: an ongoing whose next episode is imminent needs frequent checks; a long-running ongoing with the next episode days out does not. Today every ongoing combo is parser-checked **every hour regardless** — wasted external traffic and slower hot-title latency.

**Change:** tier the per-tick candidate set by `next_episode_at` proximity, with a hard delivery floor.

- Tier HOT: `next_episode_at` within `NOTIF_HOT_WINDOW` (default 36h) of now, OR `next_episode_at` NULL/unknown (fail-safe: unknown → treat as hot) → checked **every tick** (hourly).
- Tier WARM: else → checked every `NOTIF_WARM_EVERY` (default 3 ticks ≈ 3h).
- **Floor:** any combo not checked within `NOTIF_TIER_FLOOR` (default 6h) is force-included regardless of tier. Implemented via `notif:checked:<animeID>` Redis timestamp; a combo enters the tick if `tierDue(band, lastChecked)` OR `now - lastChecked ≥ floor`.
- Detector orchestration, snapshot diffing, per-user fan-out (`maxwatched.go`) unchanged. Only the candidate-set filter in front of the parser fan-out changes. Delivery guarantee preserved by the floor.

This needs `next_episode_at` on the hot-combo rows — add it to the `hotcombos.go` SELECT (already a column on `animes`).

Config knobs: `NOTIF_HOT_WINDOW=36h`, `NOTIF_WARM_EVERY=3`, `NOTIF_TIER_FLOOR=6h`.

---

## 5. Projection C — autocache (weighted drain by reason-class)

**File anchors:** demand upsert `repo/demand.go:33-79`, drain `demand.go:92-105`, planner `planner.go:202-424`, drain cap `planner.go:94`.

Autocache is demand-driven on a foreign DB; its band signal is **already on each row** as `reason` (`next_ep`/`ongoing` = hot, `backfill` = cold). Today `Drain` is pure FIFO (`Order("requested_at ASC")`), and WR-01 deliberately never bumps `requested_at` so re-asserted ongoing demand can't starve backfill. Under the shared vocabulary this becomes explicit.

**Change:** replace single-class FIFO drain with **weighted class drain** — within a 50-row batch, fill `AUTOCACHE_HOT_SHARE` (default 70%) from `reason IN ('next_ep','ongoing')` and the rest from `reason='backfill'`, each class still `requested_at ASC` (FIFO preserved *within* class). If one class is short, the other fills the remainder (no wasted batch slots).

- Pure Planner-internal change: `Drain(limit)` → `DrainWeighted(hotN, coldN)` doing two ordered `SELECT`s and merging. Wire contract, producers (scheduler Logic A, player Logic B, catalog backfill), eviction ranking — **all untouched**.
- WR-01 anti-starvation is now a property of the weights (cold always gets ≥30% of the batch), not a fragile side effect of never bumping `requested_at`. Keep the no-bump behavior too (belt and suspenders).

Config knob: `AUTOCACHE_HOT_SHARE=0.70`.

---

## 6. Projection D (optional, Phase 4) — playability probe

**File anchor:** `services/analytics/internal/probe/animeset.go:103-193`.

The probe's random slots (`SlotSpotlightRandom`, `SlotRandom`) currently sample the spotlight pool. Point them at the interest endpoint's Band-1 list instead, so provider-health probes preferentially exercise titles users are actually watching. Anchor slots (Frieren / "Кот и дракон") and health-driven cadence stay exactly as-is. Small, isolated, no correctness impact — genuinely optional.

---

## 7. Data flow

```
                         ┌───────────────────────────┐
   catalog animes / anime_list ─▶│ /internal/interest/bands  │  (bands + raw signals, DB-derived)
                         └───────────────────────────┘
                             │            │            │
             ┌───────────────┘            │            └──────────────┐
             ▼                            ▼                           ▼
   content-verify (:8101)        notifications (:8090)        probe (analytics :8092, opt)
   weighted band claim           proximity cadence tiers      Band-1 random slots
   + Redis visitors 7d           + notif:checked floor
   + cv:idle:cursor

   autocache (library :8089): reason-class already on demand row → DrainWeighted (no endpoint call; foreign DB)
```

---

## 8. Error handling / fail-open

- Endpoint unreachable → content-verify uses last cached bands (existing 10m membership cache `engine.go:112-134`); notifications falls back to "all combos hot" (today's behavior — safe, just chattier).
- Redis cursor/checked-timestamp errors → start-of-catalog / treat-as-due (never skip a title silently).
- Weighted lottery with all bands empty of actionable work → same as today (nothing to claim, idle tick).
- Autocache class-drain SELECT error on one class → fall back to single FIFO drain for that batch.

---

## 9. Testing

- **content-verify:** table tests for `BandOf` / `IntraScore` (each band's tie-breaks, freshBoost window edges); weighted-claim distribution test (seeded RNG → proportions hold, fall-through when higher bands empty); idle-cursor advance+wrap test; `CooldownTTL(band)` per-band values.
- **catalog:** endpoint test — ongoing/top/planned/idle_window shapes, `idle_offset` paging, `idle_total` count, non-ongoing/non-top exclusion in idle window. Reuse existing `internal_verify` test harness (DDL for `anime_list`).
- **notifications:** tier-selection test (hot/warm/null-next-ep), floor force-include after `NOTIF_TIER_FLOOR`, snapshot/fan-out untouched (existing tests stay green).
- **autocache:** `DrainWeighted` — hot/cold split honored, short-class backfill fills remainder, within-class FIFO order, single-class fallback on error.
- All: env-knob parsing + clamp tests.

---

## 10. Phasing (each phase ships working software)

1. **Phase 1 — shared endpoint + content-verify bands.** Catalog `/internal/interest/bands` (+ old alias); content-verify band model, weighted claim, idle round-robin, per-band cooldown, env knobs. The core; delivers the owner's headline (ongoing focus + idle tail sweep).
2. **Phase 2 — autocache weighted drain.** Planner-only `DrainWeighted`. Small, isolated.
3. **Phase 3 — notifications cadence tiers.** Proximity tiers + delivery floor. Careful (must not break delivery guarantee).
4. **Phase 4 — probe Band-1 sampling (optional).** Tiny; ship only if desired.

Each phase = its own `/animeenigma-after-update` (deploy + changelog + push).

---

## 11. Non-goals

- No change to the visitor-signal source, cv:visit writers, or the 7d/8d windows.
- No merge of notifications/autocache into content-verify's literal queue (structurally impossible: ongoing-only / foreign DB).
- No AI subtitle work (separate TODO already filed).
- No change to eviction ranking, provider health/cadence state machine, or spotlight logic.
- No new persisted queue table — cv stays a virtual queue.

---

## 12. Open decisions — resolved (owner approved direction 2026-07-20)

1. **Weighted band claim** (not strict top-down hierarchy) — chosen. Keeps "просто смотрят" from starving under an all-ongoing flood; proportions env-tunable.
2. **Band 2 = top-100 + watched merged** (not separate bands) — chosen. Signals compose; a watched top-100 title sorts highest naturally.
3. **Idle tier sweeps the whole catalog tail by windows** (not capped at top-300) — chosen, matching "топ200 – топ300 и так далее". Bounded by a 7d idle cooldown + round-robin cursor so the tail never spins hot.

**Metrics:** UXΔ = +3 (Better) (fresh ongoing episodes verified/notified/prefetched faster; catalog tail no longer never-probed) · CDI = 0.04 * 21 · MVQ = Kraken 87%/85%.
