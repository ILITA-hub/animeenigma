# Phase 2: Analytics Audit - Context

**Gathered:** 2026-04-28
**Status:** Complete (read-only audit; deliverable promoted to `docs/`)

<domain>
## Phase Boundary

Produce a written, cite-able inventory of every column we currently capture in
`watch_history`, `watch_progress`, `anime_list` (plus `reviews` for Phase 8 recs
readiness) and a prioritized gap list scored on (value × inverse-risk), with the
top candidates LOCKED as Phase 5's input shopping list.

**Strictly in scope:** read-only investigation against the live production
database, code reads of `services/player/internal/{handler,service,repo,domain}/`
and `frontend/web/src/`, output as a single markdown doc under `docs/`.

**Strictly out of scope:**
- Any code or schema change (Phases 3, 5, 6 own those)
- Any per-event capture beyond what already exists (Phase 5 locked above)
- Any decision about *building* the recommendations engine (Phase 8 documents readiness; engine is a separate milestone)

</domain>

<decisions>
## Implementation Decisions

### Workflow

- **D-01:** A 315-line draft (`02-DRAFT-AUDIT.md`) was written before /gsd-discuss-phase ran. The user directed: "promote draft to docs/" — treat the draft as the deliverable, polish + relocate, do not re-audit.
- **D-02:** Final deliverable lives at `docs/analytics-audit-2026-04-28.md` (top-level under `docs/`, dated). Not under `docs/issues/` (those are UI audits / incident reports) and not under `docs/audits/` (no existing audit subdir convention to extend).

### Coverage scope

- **D-03:** ROADMAP C-01 names 3 tables; the audit also briefly inventories `reviews` (because it is the strongest user-stated affinity signal for Phase 8) and references `activity_events` as a free trajectory log Phase 8 should not overlook. Other player-domain tables (`anime_preferences`, `pinned_translations`) are deliberately NOT inventoried — they are configuration tables, not user-event capture.

### Phase 5 candidate lock

- **D-04:** Top 3 gaps LOCKED as Phase 5 input shopping list — listed in priority order in the audit doc's "Phase 5 Candidate Lock" section:
  1. **G-02 Per-episode rewatch detection** — `watch_progress.watch_count INT NOT NULL DEFAULT 0`, depends on Phase 3
  2. **G-04-lite Session-start vs session-resume bit** — `watch_progress.session_id UUID NULLABLE` + same on `watch_history`, plus client-side session-id helper
  3. **G-01 Drop-off / abandon point** — final-flush beacon on player unload (extend `KodikPlayer.vue:689` pattern to all 4 players); abandoned derivation `progress/duration < 0.5 AND no follow-up`
- **D-05:** G-03 (full trajectory) and G-05 (intro/outro skip) explicitly DEFERRED with rationale recorded in the audit. G-03 because cheap derivation captures 90 % of value; G-05 because Kodik (~84 % of rows) is iframe-only and we lack upstream chapter metadata.
- **D-06:** Phase 5 may downgrade any of the 3 locked items but must record the justification in its CONTEXT.md.

### Hygiene items disposition

- **D-07:** Hygiene/cleanup items (ghost columns, read-orphan endpoints, index drift, write-only fields) are documented in the audit's "Cleanup / Hygiene Items" section but explicitly OUT OF SCOPE for Phases 5-8. They are recommended for milestone backlog consideration. NO janitorial phase is being added to this milestone roadmap.
- **D-08:** Two hygiene items have a natural home in existing phases and are flagged inline in the audit:
  - `GET /progress/{animeId}` orphan → Phase 7 (cross-device freshness already in scope)
  - `is_rewatching` removal → after Phase 5 G-02 ships, the list-level boolean is strictly weaker

### Empirical snapshot freshness

- **D-09:** Row counts and combo-distribution stats in the audit are stamped 2026-04-28. Phase 5 should re-run the SELECTs at the start of its CONTEXT pass — if magnitudes have shifted materially (>2x in any cohort) Phase 5 may need to re-rank candidates. The audit's *qualitative* findings (which columns are write-only, which endpoints are orphan) do not need re-validation; only the live populations.

### Loki retention

- **D-10:** PROJECT.md has the canonical Loki retention figure: **168h / 7 days** (per `docker/loki/loki-config.yml:27-28`). Phase 1 CONTEXT.md D-06 incorrectly stated ~31 days; do NOT propagate that number. Per-event analytics windows beyond 7 days require a Phase 5 schema-add (per-event DB table), not Loki tuning. The audit references this constraint in its cross-references section.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents (Phase 5/6/8 planners and researchers) MUST read these.**

### Audit deliverable
- `docs/analytics-audit-2026-04-28.md` — the deliverable. Phase 5 reads "Phase 5 Candidate Lock"; Phase 6 reads the column inventory + `duration_watched` notes; Phase 8 reads "Notes for Phase 8".

### Project planning
- `.planning/PROJECT.md` — overall project context, Key Decisions, "Loki retention constraint" section
- `.planning/REQUIREMENTS.md` — C-01, C-02 closed by this phase; C-03 (gap-fill) is Phase 5
- `.planning/ROADMAP.md` §"Phase 2" — success criteria
- `.planning/phases/01-instrumentation-baseline/01-CONTEXT.md` — instrumentation already shipped (override-rate observable)

### Codebase locations cited extensively in the audit
- `services/player/internal/domain/watch.go` — canonical column declarations
- `services/player/internal/{handler,service,repo}/` — write/read paths
- `frontend/web/src/components/player/{Kodik,AnimeLib,HiAnime,Consumet}Player.vue` — progress save call sites
- `frontend/web/src/views/Anime.vue:745` — localStorage resume read (root cause for the cross-device staleness item)
- `frontend/web/src/api/client.ts:213` — orphan watchHistory API stub

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (input to Phase 5)
- `frontend/web/src/components/player/KodikPlayer.vue:689` — existing `navigator.sendBeacon` on unload; pattern to extend to other 3 players for G-01 drop-off capture.
- `services/player/internal/repo/progress.go:21–39` — existing upsert pattern; G-02 watch_count increment hooks here.
- `services/player/internal/service/list.go:218–222` — existing snapshot of `watch_progress.progress` into `watch_history.duration_watched` at mark-watched time; Phase 6 weighting consumes this snapshot.

### Patterns to Preserve
- `WatchHistory.duration_watched` (column) is reserved for Phase 6 weighting — it is currently write-only by design, NOT dead code. Don't remove.
- `localStorage[watch_progress:<animeId>]` is the current resume source — Phase 7 will likely move resume reads to the server endpoint, but Phase 3 should NOT touch this.

</code_context>

<deferred>
## Deferred Ideas

- **Janitorial / hygiene phase** — discussed during scoping. Not added to this milestone roadmap. Recommended for milestone v1.1 or backlog. Concrete items listed in audit § "Cleanup / Hygiene Items".
- **`anime_preferences` and `pinned_translations` table inventory** — configuration tables, not event capture. Not in audit scope. If a future phase needs them, easy to add.
- **Empirical re-validation cadence** — left to Phase 5 entry. No automated re-run mechanism added.

</deferred>

---

*Phase 2 — Analytics Audit*
*Context gathered: 2026-04-28*
*Closed inline (no separate research/planning rounds — draft was the deliverable)*
