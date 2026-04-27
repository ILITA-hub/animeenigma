# Requirements: Smart Watch Picker Overhaul

**Defined:** 2026-04-27
**Core Value:** When a logged-in user opens an anime, the player loads on the correct episode in the combo (language + dub/sub + team + player) they actually want — without the user touching anything — and we can prove it with a single metric (auto-pick override rate).

## v1 Requirements

### A. Resume CTA Correctness

- [ ] **A-01**: At the existing 20-min auto-mark threshold, set `watch_progress.completed = true` (in addition to bumping `anime_list.episodes`) so resume logic and watchlist counter agree.
- [ ] **A-02**: Manual "mark watched" action also sets `watch_progress.completed = true` — both paths converge on a single source of truth.
- [ ] **A-03**: Pre-player episode selection follows a state machine:
  - **Watching** (last < total): start ep N+1, breadcrumb "You finished ep N"
  - **Finished series** (last == total): "You finished this" surface with Rewatch / Mark complete in list / Find similar
  - **Ongoing, next not yet aired**: "Episode N+1 — not yet available" with ETA from Schedule data; if ETA passed, show "currently airing — usually available within hours"
- [ ] **A-04**: All four players (Kodik, AnimeLib, HiAnime, Consumet) honor the same episode pre-selection logic on mount.

### B. Smarter Tier 2 Inference

- [ ] **B-01**: Tier 2 query weights aggregation by `WatchHistory.duration_watched` (column exists, currently unused in aggregation).
- [ ] **B-02**: Tier 2 applies exponential time decay (target half-life 30 days) so abandoned old habits do not outrank current ones.
- [ ] **B-03**: Tier 2 emits two signals — coarse `(language, watch_type)` for the lock decision, fine `(translation_title)` for the team pick within the lock — replacing the single team-bound rank.
- [ ] **B-04**: Min-confidence threshold — if total weighted history is below a tunable floor, fall through to Tier 3 (community) instead of locking from a thin signal.
- [ ] **B-05**: Profile > Advanced Settings panel exposes power-user knobs: view current resolved tier per anime, override default lock, force a combo, view raw Tier 2 weights, reset learned preferences.

### C. Analytics Audit + Gap-Fill

- [ ] **C-01**: Audit every column captured in `watch_history`, `watch_progress`, `anime_list` and document what each is used for vs. unused.
- [ ] **C-02**: Identify gaps for smart episode selection: drop-off / abandon point, rewatch detection, completion-percentage trajectory, session length, intro/outro skip patterns.
- [ ] **C-03**: Add the gap-fill columns / events that score highest on (value-for-this-project × low-risk-to-add). Minimum required: distinguish "session start" from "session resume" so Tier 2 weighting can ignore brief checks.
- [ ] **C-04**: Document downstream readiness — what additional capture would unlock a future recommendations engine; do not build that engine here.

### D. Cross-Cutting

- [ ] **D-01**: Anonymous (logged-out) users get a localStorage-backed preference (language + watch_type + last-used team), with the same state-machine resume CTA from `localStorage` watch progress.
- [ ] **D-02**: Single source of truth for "episode is watched" — `watch_progress.completed` is canonical; `anime_list.episodes` derives from it; episode-list checkmarks read from the same source.
- [ ] **D-03**: Cross-device freshness — invalidate the 24h composable cache on auth-state change and on a server-side combo-changed signal (e.g., bump a `prefs_version` cookie/header on save); shorten TTL if needed.

### M. Instrumentation (Success Metric)

- [ ] **M-01**: Emit a `combo_override` event when a user changes language / player / team / episode within 30s of player load.
- [ ] **M-02**: Grafana dashboard tile for override-rate, segmented by tier, language, anonymous/auth, and player. Baseline current state before B/D land; target < 10% override after the overhaul.

## v2 Requirements

Deferred — not in this roadmap.

### Recommendations Adjacent

- **REC-01**: "Because you watched X" surface on Home / Anime detail (requires C-04 readiness work first)
- **REC-02**: "Similar to X" related-anime suggestions

### Onboarding & Personalization

- **ONB-01**: First-visit prompt for explicit subs/dubs/language preference (only if M-02 metrics show inferred default fails for cold-start users)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Recommendations engine | Phase C audits and fills analytics gaps so a future project can build recs cheaply; building "because you watched X" surfaces is deferred to a separate milestone. |
| Replacing the 4-player split | Keeping Kodik / AnimeLib / HiAnime / Consumet separate. Unifying the player abstraction is a different overhaul. |
| Furigana, JP subtitles, or any subtitle-rendering change | Orthogonal to combo selection. |
| Geo-based defaults for anonymous users | Discussed and rejected in favor of localStorage; geo adds infrastructure cost (IP lookup) without clearly better UX. |
| Onboarding "subs or dubs?" prompt | User explicitly chose inferred-only with an Advanced Settings escape hatch over an explicit profile setting. |
| AI / ML ranking models for Tier 2 | Weighted-decayed counting is the chosen approach; ML would be premature. |

## Traceability

Phase mapping is filled by the roadmapper.

| Requirement | Phase | Status |
|-------------|-------|--------|
| A-01 | TBD | Pending |
| A-02 | TBD | Pending |
| A-03 | TBD | Pending |
| A-04 | TBD | Pending |
| B-01 | TBD | Pending |
| B-02 | TBD | Pending |
| B-03 | TBD | Pending |
| B-04 | TBD | Pending |
| B-05 | TBD | Pending |
| C-01 | TBD | Pending |
| C-02 | TBD | Pending |
| C-03 | TBD | Pending |
| C-04 | TBD | Pending |
| D-01 | TBD | Pending |
| D-02 | TBD | Pending |
| D-03 | TBD | Pending |
| M-01 | TBD | Pending |
| M-02 | TBD | Pending |
