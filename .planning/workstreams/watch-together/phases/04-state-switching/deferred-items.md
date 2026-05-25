# Phase 04 — Deferred Items

Items discovered during plan execution that are OUT OF SCOPE for the
phase and have been logged for a separate fix pass per the executor
SCOPE BOUNDARY rule ("Only auto-fix issues DIRECTLY caused by the
current task's changes").

---

## D-04-01 — Pre-existing build failure in `spotlight/cards/platform_stats.go`

**Discovered during:** Plan 04.1, Task 2 verification
**Branch:** `feat/platform-stats-joke-card` (pre-existing at HEAD before this plan)
**Workstream:** hero-spotlight (NOT watch-together)

**Symptom:**
```
# github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards
internal/service/spotlight/cards/platform_stats.go:70:30: undefined: spotlight.StatsMetric
internal/service/spotlight/cards/platform_stats.go:84:19: undefined: spotlight.StatsMetric
internal/service/spotlight/cards/platform_stats.go:126:38: unknown field Metrics in struct literal of type spotlight.PlatformStatsData
internal/service/spotlight/cards/platform_stats.go:147:80: undefined: spotlight.StatsMetric
```

**Root cause:** Commit `b17bbb3` ("feat(spotlight/08): emit previous_value
+ series[7] on StatsMetric (HSB-V11-PS-01)") landed code expecting a
`spotlight.StatsMetric` type and a `PlatformStatsData.Metrics` field
that do NOT exist in `services/catalog/internal/service/spotlight/types.go`
on this branch. The current `PlatformStatsData` exposes `Hero` /
`Tiles` instead.

**Impact on Plan 04.1:**
- `cd services/catalog && go build ./...` cannot complete (transitively
  depends on the broken package).
- `cd services/catalog && go test ./internal/handler/... -count=1 -race`
  cannot run because `internal/handler/spotlight.go` imports
  `internal/service/spotlight/cards`.
- My new files `internal_episodes_validate.go` +
  `internal_episodes_validate_test.go` are syntactically clean (and the
  pure-Go service-layer logic in `internal/service/episodes_validate.go`
  + `episodes_validate_test.go` runs all 16 tests green under `-race`).

**Why not fixed here:** Out of scope per executor SCOPE BOUNDARY. This
is hero-spotlight workstream territory (workstream tag `hero-spotlight`
v1.1-polish Phase 08), not watch-together. Fixing it requires
restoring or porting the `StatsMetric` type + the `PlatformStatsData`
shape — a non-trivial change unrelated to WT-STATE-02.

**Recommended action:** Open separate ticket in `hero-spotlight`
workstream. Likely needs either reverting `b17bbb3` or completing the
type restructure in `spotlight/types.go`. The platform_stats joke card
spec (`docs/superpowers/specs/2026-05-22-platform-stats-v1.1-polish.md`
referenced by `b17bbb3`) should drive the resolution.

**Workaround for Plan 04.1 verification:** Service-layer (`internal/service`)
tests pass cleanly because `service` package does NOT import
`spotlight/cards`. The new handler-layer files compile against the
service interface and have been code-reviewed for the same scenarios.
Handler-level integration verification will land via Plan 04.6's
end-to-end smoke once the spotlight breakage is unblocked elsewhere.
