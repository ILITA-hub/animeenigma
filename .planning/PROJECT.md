# Smart Watch Picker Overhaul

## What This Is

Auto-selection of what a viewer watches — episode, language, player, dub-vs-sub, translation team — when they open an anime on AnimeEnigma. Today the picker has correctness gaps (resume points to already-watched episodes), inference gaps (a single binge can lock the wrong combo for everyone after it), and instrumentation gaps (we cannot tell whether picks are good). This project fixes all three so users land on the right thing without thinking, with an Advanced Settings panel for power users who want to override or debug.

## Core Value

When a logged-in user opens an anime, the player loads on the correct episode in the combo (language + dub/sub + team + player) they actually want — without the user touching anything — and we can prove it with a single metric (auto-pick override rate).

## Requirements

### Validated

<!-- Inferred from existing code as of 2026-04-27. These work today and must continue to work. -->

- ✓ **VAL-01**: 5-tier preference resolution (per-anime → user-global → community → pinned → default) — `services/player/internal/service/resolver.go`
- ✓ **VAL-02**: Strict no-cross-language and no-cross-dub/sub boundary lock once any tier sets it — same file
- ✓ **VAL-03**: Per-anime preference upserted on every progress save — `services/player/internal/service/preference.go`
- ✓ **VAL-04**: 4 video players (Kodik, AnimeLib, HiAnime, Consumet) consume `preferred-combo` prop and self-select on load
- ✓ **VAL-05**: 24h `localStorage` cache of resolved combo per anime — `frontend/web/src/composables/useWatchPreferences.ts`
- ✓ **VAL-06**: Frontend players auto-call `MarkEpisodeWatched` after 20 minutes of playback — bumps `anime_list.episodes` only
- ✓ **VAL-07**: Manual "mark watched" button in every player (accessibility) — must be preserved
- ✓ **VAL-08**: Admin pinned-translations system (Tier 4) — `services/catalog/internal/repo/anime.go`
- ✓ **VAL-09**: Schedule view exists with `next_episode_at` / `episodes_aired` data on `AnimeInfo`

### Active

#### A. Resume CTA correctness

- [ ] **A-01**: At the existing 20-min auto-mark threshold, set `watch_progress.completed = true` (in addition to bumping `anime_list.episodes`) so resume logic and watchlist counter agree
- [ ] **A-02**: Manual "mark watched" must also set `watch_progress.completed = true` — single source of truth
- [ ] **A-03**: Pre-player episode selection follows a state machine:
  - Last watched < total episodes → start ep N+1, breadcrumb "You finished ep N"
  - Last watched == total episodes → "You finished this" surface with Rewatch / Mark complete in list / Find similar actions
  - Last watched < total but next ep not yet aired → "Episode N+1 — not yet available" with ETA from Schedule data; if ETA passed, show "currently airing — usually available within hours" placeholder
- [ ] **A-04**: All four players honor the same episode pre-selection logic on mount

#### B. Smarter Tier 2 inference

- [ ] **B-01**: Tier 2 query weights by `WatchHistory.duration_watched` (already captured, currently unused in aggregation)
- [ ] **B-02**: Tier 2 applies exponential time decay (target half-life 30 days) so abandoned old habits do not outrank current ones
- [ ] **B-03**: Tier 2 emits two signals — coarse `(language, watch_type)` for the lock decision, fine `(translation_title)` for the team pick within the lock — instead of a single team-bound rank
- [ ] **B-04**: Min-confidence threshold — if total weighted history is below a tunable floor, fall through to Tier 3 (community) instead of locking from a thin signal
- [ ] **B-05**: Profile > Advanced Settings panel exposes power-user knobs: view current resolved tier per anime, override default lock, force a combo, view raw Tier 2 weights, reset learned preferences

#### C. Analytics audit + gap-fill

- [ ] **C-01**: Audit every column we currently capture in `watch_history`, `watch_progress`, `anime_list` and document what each is used for vs. unused
- [ ] **C-02**: Identify gaps for smart episode selection: drop-off / abandon point, rewatch detection, completion-percentage trajectory, session length, intro/outro skip patterns
- [ ] **C-03**: Add the gap-fill columns / events that score highest on (value-for-this-project × low-risk-to-add). At minimum: distinguish "session start" from "session resume" so Tier 2 weighting can ignore brief checks
- [ ] **C-04**: Document downstream readiness — what additional capture would unlock a future recommendations engine, but do not build that engine here

#### D. Cross-cutting

- [ ] **D-01**: Anonymous (logged-out) users get a localStorage-backed preference (language + watch_type + last-used team), with the same state-machine resume CTA from `localStorage` watch progress
- [ ] **D-02**: Single source of truth for "episode is watched" — pick one (`watch_progress.completed` recommended) and have `anime_list.episodes` derive from it; episode-list checkmarks read from the same source
- [ ] **D-03**: Cross-device freshness — invalidate the 24h composable cache on auth-state change and on a server-side combo-changed signal (e.g., bump a `prefs_version` cookie/header on save); shorten TTL if needed

#### Instrumentation (success metric)

- [ ] **M-01**: Emit a `combo_override` event when a user changes language / player / team / episode within 30s of player load
- [ ] **M-02**: Dashboard tile in Grafana for override-rate, segmented by tier, language, anonymous/auth, and player. Baseline current state before B/D land; target < 10% override after the overhaul

### Out of Scope

- **Recommendations engine** — Phase C audits and fills analytics gaps so a future project can build recs; building "because you watched X" surfaces is deferred to a separate milestone.
- **Replacing the 4-player split** — keeping Kodik / AnimeLib / HiAnime / Consumet separate. Unifying the player abstraction is a different overhaul.
- **Furigana, JP subtitles, or any subtitle-rendering change** — orthogonal to combo selection.
- **Geo-based defaults for anonymous users** — discussed and rejected in favor of localStorage; geo adds infrastructure cost (IP lookup) without clearly better UX.
- **Onboarding "subs or dubs?" prompt** — user explicitly chose inferred-only with an Advanced Settings escape hatch over an explicit profile setting.
- **AI / ML ranking models for Tier 2** — weighted-decayed counting is the chosen approach; ML would be premature.

## Context

**Codebase state (verified 2026-04-27):**
- Backend: Go 1.22, Chi router, GORM, Postgres, Redis. Player service at `services/player/`. Catalog service at `services/catalog/`.
- Frontend: Vue 3 + TypeScript + Bun + Tailwind. Player composable at `frontend/web/src/composables/useWatchPreferences.ts`. Players under `frontend/web/src/components/player/`.
- Shared libs: `libs/metrics`, `libs/logger`, `libs/database`, `libs/cache`, `libs/idmapping` (ARM client for AniList ID resolution).

**Current resolver shape (file: `services/player/internal/service/resolver.go`):**
- Tier 1 — exact match on saved per-anime preference; sets `(language, type)` lock regardless of available match
- Tier 2 — naive `COUNT(*) GROUP BY player, language, watch_type, translation_title` over all watch_history (the inference problem)
- Tier 3 — community popularity, filtered by lock
- Tier 4 — admin-pinned translations, hardcoded RU+Kodik only
- Tier 5 — default `kodik+ru+sub` (cultural/historical default; works because RU is the larger user base today)

**Smoking-gun bugs surfaced during scoping:**
1. `WatchProgress.Completed` is hardcoded `false` with comment "User marks manually" (`services/player/internal/service/progress.go:34`) despite a 20-min auto-mark already firing in every player — the auto-mark only updates `anime_list.episodes`, never `watch_progress.completed`. Two parallel sources of truth, both partially correct.
2. Tier 2 query in `repo/preference.go:42` ignores `duration_watched`, never decays, treats one mega-binge of a dub as a permanent global preference, and only emits team-granular ranks (no coarse subs-vs-dub signal).

**Prior commitments (from feedback memory `feedback_watch_preferences.md`):**
- Strict fallback rules apply — never cross language or sub/dub boundary; gradual graph-like fallback. Any new logic must preserve these invariants.

**Test data available:** `ui_audit_bot` user has 8 anime_list entries (mixed statuses), 3 watch_history rows, 3 theme_ratings — usable for resolver tests, may need expansion for Tier 2 weighting/decay scenarios.

## Constraints

- **Tech stack**: Go 1.22 backend, Vue 3 / TypeScript / Bun frontend — no new languages or frameworks. Reuse `libs/metrics` for the override-rate metric.
- **Compatibility**: Cannot break the existing `/api/users/preferences/resolve` contract — it is consumed by all 4 players. Additive changes only (new fields ok, no removed fields).
- **Accessibility**: The manual "mark watched" button stays in every player.
- **Performance**: Resolver runs on every anime page load — Tier 2 weighted-decay query must complete in < 50ms p95. Use materialized view or cached aggregate if naive query exceeds budget.
- **Multi-language UX**: All new copy (CTA labels, advanced settings) must be added to both EN and RU locales.
- **Deployment**: This server IS production. Each phase that touches prod-affecting code must be redeployed via `make redeploy-<service>` and verified before marking done.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Single project covering A+B+C+D, recs deferred | All four threads share one root cause (we can't tell what users want, so we guess wrong) — fixing them together avoids three round-trips of analytics/inference work. Recs isolated as future milestone. | — Pending |
| Inferred preferences (no explicit "dub or sub" setting) + Advanced Settings escape hatch | User explicitly chose this over an onboarding prompt. Goal: zero-friction default for normals; full control for power users. | — Pending |
| Override rate < 10% as success metric | Single observable behavior that measures the project's actual goal. Avoids self-grading on per-tier accuracy without ground truth. | — Pending |
| Auto-mark threshold stays at 20 minutes (existing) | Already shipped, already feels right per user memory. Save scope by reusing the threshold; just wire it to the right table. | — Pending |
| `watch_progress.completed` becomes the single source of truth for "ep watched" | Avoids the `anime_list.episodes` vs `watch_progress.completed` disagreement. `anime_list.episodes` derives from it. | — Pending |
| Recommendations engine out of scope | Building recs without the analytics work below is guessing twice. Phase C makes the *next* project (recs) cheaper. | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-27 after initialization*
