---
phase: 01-backend-aggregator
plan: 01
subsystem: services/catalog/internal/service/spotlight
workstream: hero-spotlight
milestone: v1.0
tags: [scaffold, types, contracts, tdd, aggregator-skeleton, backend]
requirements: [HSB-BE-02]
dependency_graph:
  requires: []
  provides:
    - spotlight.Card, spotlight.Response (JSON discriminated-union envelope)
    - spotlight.Resolver interface (Plan 01-02 resolvers implement)
    - spotlight.NewAggregator constructor (Plan 01-03 wires real fan-out)
    - spotlight.DateSeedUTC, DateKeyUTC, SnapshotKey helpers
  affects:
    - Plan 01-02 (4 resolvers under spotlight/cards/) — implements Resolver
    - Plan 01-03 (concurrent fan-out + snapshot fallback) — replaces Aggregator.Resolve stub
    - Plan 01-04 (handler + router + gateway wiring) — consumes Response envelope
tech_stack:
  added: []
  patterns:
    - Discriminated-union JSON via typed structs + `any` Data field (PATTERNS.md §Pattern 5)
    - UTC-source-of-truth date seeding for TZ-invariant per-day pickers (RESEARCH.md §Pitfall 2)
    - Interface-first task ordering — contracts ship before implementations
key_files:
  created:
    - services/catalog/internal/service/spotlight/types.go
    - services/catalog/internal/service/spotlight/seed.go
    - services/catalog/internal/service/spotlight/aggregator.go
    - services/catalog/internal/service/spotlight/types_test.go
  modified: []
decisions:
  - "Card.Data typed as `any` (not json.RawMessage) — compile-time safety for the 4 known card variants; resolvers return their concrete typed struct"
  - "Cards slice MUST be initialized to []Card{} (NOT var declaration) — frontend treats JSON null as parse failure; doc comment + TestTypes_EmptyCardsMarshalArray guard"
  - "Resolver returns (nil, nil) for ineligible — distinct from (nil, err) which logs spotlight.card_failed; aggregator drops both, only the latter is observed"
  - "Aggregator.Resolve is a stub returning empty Cards — no premature goroutine/channel code; Plan 01-03 owns the fan-out implementation"
  - "Aggregator.resolvers stays unexported; exposed via Resolvers() getter for test introspection only"
metrics:
  duration_minutes: 8
  tasks_completed: 2
  files_created: 4
  files_modified: 0
  commits: 3
  tests_added: 7
  completed_date: 2026-05-21
---

# Phase 01 Plan 01-01: Scaffold spotlight package + Aggregator skeleton Summary

Established the contract surface for the hero-spotlight backend aggregator: 9 exported types, 3 date-seed helpers, the `Resolver` interface that Plan 01-02 will implement, and an `Aggregator` skeleton ready for Plan 01-03's concurrent fan-out — JSON shape locked to design doc §4.1 by 7 passing tests.

## What Was Built

### Files Created (4)

| File | Lines | Role |
|------|-------|------|
| `services/catalog/internal/service/spotlight/types.go` | 99 | Card discriminated-union envelope, per-card payload structs, Resolver interface, Response envelope |
| `services/catalog/internal/service/spotlight/seed.go` | 36 | DateSeedUTC, DateKeyUTC, SnapshotKey — UTC-locked date helpers |
| `services/catalog/internal/service/spotlight/aggregator.go` | 51 | Aggregator struct, NewAggregator constructor, Resolvers() getter, Resolve() stub |
| `services/catalog/internal/service/spotlight/types_test.go` | 269 | 7 test funcs locking JSON shape + seed math + constructor contract |

### Exported Type Set

- `Card` — `{Type string, Data any}` discriminated union
- `Response` — `{Cards []Card, GeneratedAt string}` top-level envelope
- `Resolver` — interface `{Type() string; Resolve(ctx, *userID) (*Card, error)}`
- `AnimeOfDayData` — `{Anime domain.Anime, ReasonI18nKey string}` (reason omitempty)
- `RandomTailData` — `{Anime domain.Anime}`
- `LatestNewsData` — `{Entries []ChangelogEntry}`
- `PlatformStatsData` — `{Metrics []StatsMetric}`
- `StatsMetric` — `{Key string, Value int64, Delta *int64}` (delta omitempty)
- `ChangelogEntry` — `{Date, Type, Message string}` (type omitempty)
- `Aggregator` — concrete struct (unexported fields)

### Public Helper Signatures

```go
func DateSeedUTC(t time.Time) int                                // YYYY*100*32 + MM*32 + DD
func DateKeyUTC(t time.Time) string                              // "YYYY-MM-DD" UTC
func SnapshotKey(userID *string) string                          // "spotlight:snapshot:<anon|uid>:<date>"
func NewAggregator(c cache.Cache, log *logger.Logger, resolvers []Resolver) *Aggregator
func (a *Aggregator) Resolvers() []Resolver                      // test-only getter
func (a *Aggregator) Resolve(ctx context.Context, userID *string) (*Response, error)  // STUB
```

### Tests Added (7)

| Test | What it pins |
|------|--------------|
| `TestTypes_JSONShape` | 4-case table — each Card variant's top-level `{type,data}` keys + per-type inner key (anime / entries / metrics); also asserts `reason_i18n_key` is absent when empty |
| `TestTypes_EmptyCardsMarshalArray` | `Response{Cards: []Card{}}` marshals as `"cards":[]` (regression guard against `null`) |
| `TestTypes_StatsMetric_DeltaOmittedWhenNil` | `delta` absent from JSON when `*int64` Delta is nil |
| `TestDateSeedUTC` | 4-case table — formula correctness + timezone-invariance (MSK May 22 00:30 == UTC May 21 21:30 → seed for May 21) |
| `TestDateKeyUTC` | Same UTC-invariance for the YYYY-MM-DD key form |
| `TestSnapshotKey` | nil → `spotlight:snapshot:anon:<date>`; `&"abc"` → `spotlight:snapshot:abc:<date>`; date suffix matches `^\d{4}-\d{2}-\d{2}$` |
| `TestNewAggregator_ConstructsEmpty` | Resolvers() returns non-nil empty slice (NPE guard); stub Resolve returns empty Cards + populated GeneratedAt |

## Commits

| Hash | Type | Message |
|------|------|---------|
| `f8368d7` | feat | define spotlight types + date-seed helpers (Task 1) |
| `fe24491` | test | pin JSON shape + date-seed + Aggregator constructor contract (Task 2 RED) |
| `52ccf94` | feat | add Aggregator skeleton (constructor + Resolve stub) (Task 2 GREEN) |

## TDD Gate Compliance

Task 2 followed the RED → GREEN cycle as required by `tdd="true"`:

- **RED commit (`fe24491`):** test file added; `go test` failed with `undefined: NewAggregator` (build failure). Confirmed before commit.
- **GREEN commit (`52ccf94`):** aggregator.go added; all 7 tests pass; `go vet` clean.
- **REFACTOR:** none needed (greenfield code, no cleanup to commit).

Task 1 was effectively types-only with no behaviour to TDD; its `types.go`/`seed.go` were exercised by the Task 2 test suite.

## Verification

```
$ cd services/catalog && go build ./internal/service/spotlight/...
(exit 0)

$ cd services/catalog && go test ./internal/service/spotlight/ -count=1
ok  	github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight	0.009s

$ cd services/catalog && go vet ./internal/service/spotlight/...
(exit 0)

$ grep -c "^type " services/catalog/internal/service/spotlight/types.go
9   (≥ 9 required)

$ grep -c "^func " services/catalog/internal/service/spotlight/seed.go
3   (= 3 required)

$ grep -c "^func Test" services/catalog/internal/service/spotlight/types_test.go
7   (≥ 7 required)

$ grep -q "STUB: Plan 03 replaces this body" services/catalog/internal/service/spotlight/aggregator.go
(exit 0 — STUB marker present)

$ grep -v '^//' services/catalog/internal/service/spotlight/aggregator.go | grep -c "sync\."
0   (= 0 — no premature concurrent code shipped)
```

## Deviations from Plan

None — plan executed exactly as written. All `<must_haves>` truths satisfied, all acceptance-criteria `grep -q` gates pass.

## Confirmation: No Concurrent Code

`aggregator.go` ships:
- Zero `sync.WaitGroup`, `sync.Mutex`, `sync.Once`
- Zero `go ` goroutine launches
- Zero `chan ` channel declarations
- Zero `context.WithTimeout` calls

These all belong to Plan 01-03's concurrent fan-out body. The current `Resolve` method is a 4-line stub returning `&Response{Cards: []Card{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}`.

## Threat Flags

None — types-only scaffold, no network endpoints, no auth paths, no file access, no schema changes. No new trust-boundary surface.

## What Unblocks Next

- **Plan 01-02 (4 resolvers under `spotlight/cards/`):** can `import "...service/spotlight"` and implement `spotlight.Resolver` directly. Card discriminator strings ("anime_of_day", "random_tail", "latest_news", "platform_stats") match the JSON-shape test expectations verbatim.
- **Plan 01-03 (concurrent aggregator body):** replaces the 4-line `Resolve` stub with the fan-out described in PATTERNS.md §"aggregator.go" — adds `sync.WaitGroup`, buffered `result` channel, per-card `context.WithTimeout(ctx, 800ms)`, snapshot-fallback path keyed via `SnapshotKey(userID)`. The constructor signature, struct fields, and `Resolvers()` getter stay stable.
- **Plan 01-04 (handler + router + gateway wiring):** consumes `*spotlight.Response` via `agg.Resolve(ctx, nil)`; JSON-encodes directly (no `httputil.OK` wrapper — bare `{cards, generated_at}` envelope per design doc §4.1).

## Self-Check: PASSED

Files exist:
- `services/catalog/internal/service/spotlight/types.go` — FOUND
- `services/catalog/internal/service/spotlight/seed.go` — FOUND
- `services/catalog/internal/service/spotlight/aggregator.go` — FOUND
- `services/catalog/internal/service/spotlight/types_test.go` — FOUND

Commits exist:
- `f8368d7` — FOUND
- `fe24491` — FOUND
- `52ccf94` — FOUND

Build + tests + vet — all clean (see Verification block above).
