# Phase 9: Recs Foundation — Interface, Ensemble, Normalizer, Schema - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure-only phase per smart_discuss detection)

<domain>
## Phase Boundary

Land the architectural seam (`SignalModule` interface), weighted-ensemble aggregator, shared per-pool min-max normalizer, and the three persistence tables. Signals slot into this in subsequent phases without modifying the ensemble, normalizer, or API handler. **Ships as silent infrastructure — no user-facing surface yet.**

In scope:
- `SignalModule` Go interface in a new `services/player/internal/service/recs/` package
- `Ensemble` aggregator with weighted-sum + per-pool min-max normalization
- `MinMaxNormalize` helper with degenerate-pool guard (no NaN, no Inf)
- Persistence tables: `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` (GORM AutoMigrate)
- Thin `RecsRepository` (GORM upserts only — no business logic)
- `Orchestrator` stub (iterates registered modules, calls `Precompute`, joins errors)
- Type aliases (`AnimeID`, `UserID`, `SignalID`, `RawScore`, `NormalizedScore`)

Out of scope (later phases):
- Any concrete signal implementation (S1-S6, S11) → Phases 10-13
- HTTP handler / API endpoint → Phase 10
- Cron scheduler wiring → Phase 10
- Redis cache → Phase 10
- Frontend changes → Phases 10-14

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion

All implementation choices are at Claude's discretion — pure infrastructure phase. Use design spec `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §13 (locked decisions) and §6 (backend layout) as the binding contract. Use existing prior-art plan at `docs/superpowers/plans/2026-05-03-rec-engine-phase-1-foundation.md` as a reference implementation guide (already self-reviewed; same scope as this phase).

### Locked from spec §13 (do not relitigate)

- Backend service: extend existing `services/player/`
- Package path: `services/player/internal/service/recs/`
- Domain models live in `services/player/internal/domain/recs.go`
- Repo lives in `services/player/internal/repo/recs.go`
- Module path follows project convention: `github.com/ILITA-hub/animeenigma/services/player/...`
- Three new tables auto-migrated alongside existing player tables in `cmd/player-api/main.go`
- TDD discipline for normalizer, ensemble, orchestrator (write test first, see fail, implement, see pass, commit)
- Trivial wrappers (types, interface, GORM models, AutoMigrate, repo) skip tests — compiler verifies structure
- One commit per task (atomic)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/repo/preference.go` — pattern for thin GORM repo (gorm.DB wrapped, ctx-aware methods, OnConflict upserts)
- `services/player/internal/domain/watch.go` — pattern for GORM domain models (uuid PK, JSONB columns, TableName())
- `services/player/cmd/player-api/main.go:47-57` — AutoMigrate registration site (already takes a list of models)
- `libs/database` — GORM connection management with auto-DB-create
- `libs/cache` — Redis client (used in Phase 10, not Phase 9)

### Established Patterns
- Go 1.22, no Go 2 features
- GORM v2 with `clause.OnConflict` for upserts
- `errors.Join(...)` for collected errors (Go 1.20+)
- Table-driven tests with `t.Run` subtests
- `context.Context` first arg on every public function touching IO

### Integration Points
- AutoMigrate call in `services/player/cmd/player-api/main.go` — add three new domain types
- No new env vars
- No new dependencies (stdlib + GORM only)

</code_context>

<specifics>
## Specific Ideas

- Design spec `docs/superpowers/specs/2026-05-03-rec-engine-design.md` is the binding contract — §6.1 has the exact `SignalModule` interface signature; §4.1 has the exact storage table schema; §2.3 has the exact normalization formula.
- A prior-art plan exists at `docs/superpowers/plans/2026-05-03-rec-engine-phase-1-foundation.md` (created earlier in this session via `superpowers:writing-plans`). It is NOT the GSD plan — `gsd-plan-phase` will produce its own — but the prior-art plan contains exact code for every component this phase needs and has already been self-reviewed for placeholder-free, type-consistent content. Treat it as a reference implementation when planning.
- The `epsilon` constant in the normalizer is `1e-9` per spec §2.3.
- The Kodik fallback math (S5) is NOT in this phase — comes in Phase 12.

</specifics>

<deferred>
## Deferred Ideas

- Optional Phase-9.5 lightweight ensemble integration test using a registered no-op `SignalModule` to prove the registry mechanism works end-to-end (currently rolled into REC-FOUND-01 success criterion 4 — "adding a second test signal does not require diff in ensemble.go beyond a registry entry"). If the planner decides this needs a dedicated test file, add it; otherwise inline assertion in code review is acceptable.

</deferred>
