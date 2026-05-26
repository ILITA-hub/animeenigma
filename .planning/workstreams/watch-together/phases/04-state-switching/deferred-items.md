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

---

## D-04-02 — Pre-existing genproto ambiguous-import in catalog module

**Discovered during:** Plan 04.3 cross-service build sanity
**Branch:** `feat/platform-stats-joke-card` (pre-existing)
**Workstream:** infra / hero-spotlight (NOT watch-together)

**Symptom:**
```
/root/go/pkg/mod/google.golang.org/grpc@v1.77.0/status/status.go:35:2:
ambiguous import: found package google.golang.org/genproto/googleapis/rpc/status
in multiple modules:
  google.golang.org/genproto v0.0.0-20190425155659-357c62f0e4bb
  google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8
```

**Root cause:** Both the legacy monorepo `genproto` and the split
`genproto/googleapis/rpc` ship a `status` subpackage; grpc v1.77.0
imports via the new path, and the workspace pulls in both because some
other dep still requires the old one. Needs a `go mod tidy` pass on
catalog's go.mod (likely adding an explicit `google.golang.org/genproto`
exclude or upgrading the offending transitive dep).

**Impact on Plan 04.3:** Cross-service build (`cd services/catalog &&
go build ./...`) cannot complete under workspace mode. The catalog
module brings in both genproto roots transitively. Watch-together
builds + tests cleanly under `GOWORK=off` (module-only mode); under
workspace mode it inherits the catalog conflict through go.work.

**Workaround verified for Plan 04.3:** All Plan 04.3 verification
commands pass under module mode:

```
cd services/watch-together
GOWORK=off go build ./...        # clean
GOWORK=off go vet ./...          # clean
GOWORK=off go test ./... -count=1 -race  # all packages OK
```

The runtime artifact (when produced from the per-service Dockerfile)
uses module mode by default, so production deploys are unaffected.

**Why not fixed here:** Out of scope (catalog go.mod hygiene, not
watch-together state-change wiring). Plan 04.1 already shipped the
catalog `/internal/anime/{id}/episodes/validate` endpoint and verified
its service-layer tests in isolation; runtime integration will be
validated in Plan 04.6's docker-compose smoke once the workstream's
catalog build is restored.

**Recommended action:** Separate infra ticket. `go mod tidy` +
`go mod why google.golang.org/genproto` in `services/catalog/` to
identify the offending transitive importer, then upgrade or exclude.
