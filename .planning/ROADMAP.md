# Roadmap: Smart Watch Picker Overhaul

## Overview

The journey: ship instrumentation FIRST so we can baseline the override-rate metric, then audit existing analytics tables to expose gaps, then fix the most embarrassing correctness bug ("episode is watched" has two disagreeing sources of truth), then wire the resume state machine into all four players, then add the schema/event columns the audit identified, then rewrite the Tier 2 inference query (weighted by `duration_watched`, exponentially decayed, two-signal coarse-vs-fine, with min-confidence fallback), then surface power-user controls + anonymous UX + cross-device freshness, and finally document the analytics surface a future recommendations engine would need. Each phase is independently deployable via `make redeploy-<service>` and preserves the strict no-cross-language and no-cross-dub/sub boundary established in the existing resolver.

## Phases

**Phase Numbering:**
- Integer phases (1-8): Planned milestone work
- Decimal phases (e.g., 2.1): Reserved for urgent insertions if scoping reveals one

- [ ] **Phase 1: Instrumentation Baseline** - Emit `combo_override` events and stand up the Grafana tile so every later phase can be measured against a real baseline
- [ ] **Phase 2: Analytics Audit** - Read-only inventory of `watch_history` / `watch_progress` / `anime_list` columns and a written gap analysis for smart episode selection
- [ ] **Phase 3: Single Source of Truth for "Watched"** - Both auto-mark (20 min) and manual-mark paths set `watch_progress.completed = true`, and `anime_list.episodes` + episode-list checkmarks derive from it
- [ ] **Phase 4: Resume State Machine in All Four Players** - Pre-player episode selection follows the watching / finished / not-yet-aired state machine across Kodik, AnimeLib, HiAnime, Consumet
- [ ] **Phase 5: Analytics Gap-Fill** - Add the highest-value low-risk columns/events identified in Phase 2; at minimum distinguish session-start vs session-resume
- [ ] **Phase 6: Tier 2 Inference Rewrite** - Weighted by `duration_watched`, exponentially decayed (30-day half-life), two-signal coarse/fine, with a min-confidence fallback to Tier 3
- [ ] **Phase 7: Advanced Settings, Anonymous UX, Cross-Device Freshness** - Profile > Advanced Settings panel, localStorage-backed anonymous preference, and cache-invalidation on auth-change + `prefs_version` signal
- [ ] **Phase 8: Recommendations Readiness Documentation** - Document what additional capture would unlock a future recommendations engine; no engine built here

## Phase Details

### Phase 1: Instrumentation Baseline
**Goal**: Make the project's success metric (auto-pick override rate) observable in Grafana before any behavior changes ship, so phases 3-7 can be evaluated against a real pre-overhaul baseline.
**Depends on**: Nothing (first phase)
**Requirements**: M-01, M-02
**Success Criteria** (what must be TRUE):
  1. A `combo_override` event is emitted whenever a logged-in or anonymous user changes language, player, team, or episode within 30 seconds of player load, on any of the four players
  2. A Grafana dashboard tile shows override rate segmented by tier, language, anonymous-vs-auth, and player, refreshing within one minute of new events
  3. A baseline override-rate snapshot (≥ 24 hours of real traffic) is captured and recorded in PROJECT.md before Phase 6 starts
  4. The instrumentation is deployed via `make redeploy-player` (or whichever service emits) and verified live on production
**Plans**: 7 plans
- [ ] 01-01-PLAN.md — Wave 0: write RED test scaffolds (Go handler/middleware/service tests + Playwright spec stub)
- [ ] 01-02-PLAN.md — Wave 1: add ComboOverrideTotal/ComboResolveTotal CounterVecs + create OverrideHandler
- [ ] 01-03-PLAN.md — Wave 2: OptionalAuthMiddleware + wire override route + anon-friendly resolve + gateway proxy
- [ ] 01-04-PLAN.md — Wave 3: anonId util + axios X-Anon-ID interceptor + useOverrideTracker composable
- [ ] 01-05-PLAN.md — Wave 4: wire useOverrideTracker into 4 players + Anime.vue + unbreak Playwright E2E specs
- [ ] 01-06-PLAN.md — Wave 5: add Auto-Pick Override Rate row + 5 panels to preference-resolution.json
- [ ] 01-07-PLAN.md — Wave 6: deploy via make redeploy + smoke tests + Grafana visual + animeenigma-after-update
**UI hint**: yes

### Phase 2: Analytics Audit
**Goal**: Produce a written, cite-able inventory of every column we currently capture and a prioritized gap list, so Phase 5 schema additions and Phase 6 Tier 2 design are evidence-based rather than guessed.
**Depends on**: Nothing (parallel-eligible with Phase 1)
**Requirements**: C-01, C-02
**Success Criteria** (what must be TRUE):
  1. A markdown document under `docs/` lists every column in `watch_history`, `watch_progress`, `anime_list` with: data type, who writes it, who reads it, current usage, and "unused" flag where applicable
  2. The same document lists the gap items for smart episode selection (drop-off / abandon point, rewatch detection, completion-percentage trajectory, session length, intro/outro skip patterns) each scored on (value-for-this-project × risk-to-add)
  3. The gap list is ranked and the top 1-3 items explicitly flagged as Phase 5 candidates
  4. No production code or schema changes ship in this phase (read-only investigation)
**Plans**: TBD

### Phase 3: Single Source of Truth for "Watched"
**Goal**: Eliminate the disagreement between `watch_progress.completed` and `anime_list.episodes` so resume CTAs, watchlist counters, and episode-list checkmarks all agree.
**Depends on**: Phase 1 (so the override-rate baseline already exists when this lands)
**Requirements**: A-01, A-02, D-02
**Success Criteria** (what must be TRUE):
  1. The 20-minute auto-mark threshold sets `watch_progress.completed = true` in addition to bumping `anime_list.episodes`
  2. The manual "mark watched" button (preserved in every player) sets `watch_progress.completed = true` via the same code path
  3. `anime_list.episodes` is derived from `watch_progress.completed` rows (read or recomputed) instead of being independently incremented; episode-list checkmarks in the UI read from the same source
  4. A user who has watched 5 episodes sees the same count of completed episodes on the watchlist counter, the episode-list checkmarks, and the resume CTA — verified for `ui_audit_bot`'s seeded data on production
  5. Backfill / migration handles existing rows where `anime_list.episodes > 0` but no `watch_progress.completed = true` exists (or this gap is consciously accepted and documented)
**Plans**: TBD

### Phase 4: Resume State Machine in All Four Players
**Goal**: When a logged-in user opens an anime, the player loads on the correct episode and shows the right contextual CTA — for in-progress series, finished series, and ongoing series whose next episode hasn't aired — consistently across all four player components.
**Depends on**: Phase 3 (state machine reads `watch_progress.completed`)
**Requirements**: A-03, A-04
**Success Criteria** (what must be TRUE):
  1. Last watched < total episodes → player auto-selects ep N+1 with a "You finished ep N" breadcrumb visible above the player
  2. Last watched == total episodes → "You finished this" surface appears with Rewatch / Mark complete in list / Find similar actions, instead of silently re-loading ep N
  3. Last watched < total but next ep not yet aired → "Episode N+1 — not yet available" with ETA from the existing Schedule data; if ETA has passed, the copy switches to "currently airing — usually available within hours"
  4. The same selection logic and CTA copy apply identically across `KodikPlayer.vue`, `AnimeLibPlayer.vue`, `HiAnimePlayer.vue`, `ConsumetPlayer.vue` (no per-player divergence)
  5. All new copy ships in both EN and RU locales
**Plans**: TBD
**UI hint**: yes

### Phase 5: Analytics Gap-Fill
**Goal**: Add the highest-value low-risk columns and events identified in the Phase 2 audit so Phase 6 Tier 2 inference and any future analytics work have the data they need; at minimum, the system can distinguish a session-start from a session-resume.
**Depends on**: Phase 2 (audit output is the input shopping list)
**Requirements**: C-03
**Success Criteria** (what must be TRUE):
  1. New columns / events identified by the Phase 2 audit are added via GORM AutoMigrate and verified to populate on production traffic
  2. The system distinguishes "session start" (fresh open) from "session resume" (reopening an in-progress episode) on every progress save, and Tier 2 aggregation can filter on this distinction
  3. Each added column / event has a one-line description of intended consumer (Tier 2 query, future recs, debugging) recorded in the audit doc from Phase 2
  4. No existing column is dropped or renamed (additive-only schema change, per project compatibility constraint)
**Plans**: TBD

### Phase 6: Tier 2 Inference Rewrite
**Goal**: Replace the naive `COUNT(*) GROUP BY` Tier 2 query with weighted, time-decayed, two-signal inference that respects a min-confidence floor — so a single mega-binge no longer locks the wrong combo for everyone after it, and thin signals fall through to community popularity instead of locking from noise.
**Depends on**: Phase 1 (override-rate baseline must exist), Phase 5 (any new columns the rewrite consumes), and prior tier behavior preserved
**Requirements**: B-01, B-02, B-03, B-04
**Success Criteria** (what must be TRUE):
  1. Tier 2 aggregation weights every history row by `WatchHistory.duration_watched` instead of treating every row as 1 vote
  2. Tier 2 applies exponential time decay with a tunable half-life (default 30 days) so habits older than ~3 half-lives have negligible influence
  3. Tier 2 emits two distinct signals — coarse `(language, watch_type)` for the lock decision, fine `(translation_title)` for the team pick within the lock — and the resolver consumes both
  4. When total weighted history is below the configured floor, Tier 2 declines to lock and the resolver falls through to Tier 3 (community); the `combo_override` event downstream confirms this thin-signal case is no longer overriding at the previous rate
  5. The strict no-cross-language and no-cross-dub/sub boundary (VAL-02) is preserved — verified by tests that try to cross the boundary and assert the lock holds
  6. Resolver p95 latency stays under 50 ms (per PROJECT.md performance constraint), achieved with a materialized view or cached aggregate if the naive query exceeds budget
**Plans**: TBD

### Phase 7: Advanced Settings, Anonymous UX, and Cross-Device Freshness
**Goal**: Surface the new resolver behavior to power users (Advanced Settings panel), give anonymous users a comparable picker experience via localStorage, and stop stale 24h client cache from masking the resolver improvements across devices.
**Depends on**: Phase 4 (anonymous resume needs the same state machine), Phase 6 (Advanced Settings exposes Tier 2 weights and the resolved tier per anime)
**Requirements**: B-05, D-01, D-03
**Success Criteria** (what must be TRUE):
  1. A logged-in user can open Profile > Advanced Settings and see the current resolved tier per anime, override the default lock, force a specific combo, view raw Tier 2 weights for any anime, and reset learned preferences
  2. An anonymous (logged-out) user opens an anime and the player auto-selects language + watch_type + last-used team from localStorage, with the same state-machine resume CTA driven by localStorage watch progress
  3. Logging in or logging out invalidates the 24h composable cache immediately, and a server-side `prefs_version` cookie/header bumps on every preference save so cross-device users see the new combo without waiting 24h
  4. All new Advanced Settings copy ships in both EN and RU locales
  5. The override-rate Grafana tile (Phase 1) shows a measurable drop after this phase deploys versus the Phase 1 baseline; target is < 10% but the success criterion is "drops in the right direction"
**Plans**: TBD
**UI hint**: yes

### Phase 8: Recommendations Readiness Documentation
**Goal**: Capture what we learned and what additional analytics capture would unlock a future "because you watched X" recommendations engine, so the next milestone starts with a clear shopping list instead of repeating the audit.
**Depends on**: Phase 5 (the gap-fill columns are the starting point), Phase 6 (the rewrite revealed which signals matter)
**Requirements**: C-04
**Success Criteria** (what must be TRUE):
  1. A markdown document under `docs/` describes the additional capture (events, columns, derived signals) that would be needed to ship REC-01 / REC-02 from the v2 backlog
  2. The document explicitly states that no recommendations engine is built in this phase
  3. Each proposed addition is annotated with rough cost (schema risk, query cost, frontend wiring) so the next milestone can prioritize
  4. The document is committed and the override-rate baseline + post-overhaul number from Phase 7 are recorded for posterity
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8

Phases 1 and 2 are independent and may execute in parallel if `parallelization=true` (config.json) is honored downstream.

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Instrumentation Baseline | 0/7 | Not started | - |
| 2. Analytics Audit | 0/TBD | Not started | - |
| 3. Single Source of Truth for "Watched" | 0/TBD | Not started | - |
| 4. Resume State Machine in All Four Players | 0/TBD | Not started | - |
| 5. Analytics Gap-Fill | 0/TBD | Not started | - |
| 6. Tier 2 Inference Rewrite | 0/TBD | Not started | - |
| 7. Advanced Settings, Anonymous UX, Cross-Device Freshness | 0/TBD | Not started | - |
| 8. Recommendations Readiness Documentation | 0/TBD | Not started | - |
