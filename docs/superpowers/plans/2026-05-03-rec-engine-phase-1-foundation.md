# Recommendations Engine — Phase 1: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lay the structural foundation for the recommendations engine inside `services/player`: pluggable signal interface, shared min-max normalizer, weighted-ensemble aggregator, persistence tables, and a stub precompute orchestrator. No actual signals are wired up; no API or frontend surface ships.

**Architecture:** A new `recs` sub-package under the existing player service. A `SignalModule` interface is the seam every future signal (S1-S6, S7+) implements. The ensemble calls `Score` on each registered module, normalizes per-pool to `[0, 1]`, then computes a weighted sum. Persistence is GORM models auto-migrated alongside existing tables. Phase 2 will register the first concrete signals (S3, S4, S11) against this skeleton.

**Tech Stack:** Go 1.22, GORM v2, Postgres (with JSONB, UUID), stdlib `testing` with table-driven tests.

**Spec reference:** `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §2 (architecture), §3 (signals), §4 (storage), §6 (backend layout).

**Out of scope (deferred to later plans):**
- Any concrete signal implementation (S1-S6) — Phases 2-5 in spec §12.2
- Admin debug page / `/api/users/recs` endpoint — Phase 2-3
- Redis top-N cache — Phase 2 (when first signals start ranking)
- Cron scheduling wiring — Phase 2

---

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `services/player/internal/service/recs/types.go` | Create | Type aliases (`AnimeID`, `UserID`, `SignalID`, `RawScore`, `NormalizedScore`) |
| `services/player/internal/service/recs/signal.go` | Create | `SignalModule` interface declaration |
| `services/player/internal/service/recs/normalize.go` | Create | `MinMaxNormalize` per-pool helper |
| `services/player/internal/service/recs/normalize_test.go` | Create | Table-driven tests for normalizer (incl. edge cases) |
| `services/player/internal/service/recs/ensemble.go` | Create | `Ensemble` aggregator, `WeightedSignal`, `Recommendation` |
| `services/player/internal/service/recs/ensemble_test.go` | Create | Tests using a mock `SignalModule` |
| `services/player/internal/service/recs/precompute.go` | Create | `Orchestrator` stub: iterates registered modules, calls `Precompute` |
| `services/player/internal/service/recs/precompute_test.go` | Create | Tests using mock `SignalModule` |
| `services/player/internal/domain/recs.go` | Create | `RecUserSignals`, `RecPopulationSignals`, `RecCompletionCoOccurrence` GORM models |
| `services/player/internal/repo/recs.go` | Create | `RecsRepository` (thin GORM wrapper) |
| `services/player/cmd/player-api/main.go` | Modify | Register the three new domain models in the `AutoMigrate(...)` call |

No frontend changes. No new env vars. No new dependencies.

---

### Task 1: Package skeleton + types

**Files:**
- Create: `services/player/internal/service/recs/types.go`

- [ ] **Step 1: Create types.go**

```go
// Package recs is the player service's recommendations engine.
// See docs/superpowers/specs/2026-05-03-rec-engine-design.md for the
// full design spec. Phase 1 wires up the interface and aggregator only;
// concrete signals land in Phase 2 and beyond.
package recs

// AnimeID identifies an anime by UUID string. Type alias matches existing
// domain conventions (see services/player/internal/domain/watch.go).
type AnimeID = string

// UserID identifies a user by UUID string.
type UserID = string

// SignalID is a stable identifier for a signal module ("s1", "s5", "s11").
// Used in admin debug breakdowns and the weight registry.
type SignalID string

// RawScore is the unnormalized output of a signal's Score method.
// Pre-normalization values can sit at any scale (e.g. S1 ~[0,10], S5 ~[0,0.05]).
// MinMaxNormalize collapses these to [0, 1] over a candidate pool.
type RawScore float64

// NormalizedScore is the per-pool min-max normalized score in [0, 1].
type NormalizedScore float64
```

- [ ] **Step 2: Verify package compiles**

Run: `cd services/player && go build ./internal/service/recs/...`
Expected: exit 0, no output.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/service/recs/types.go
git commit -m "feat(player/recs): add type aliases for recommendations package"
```

---

### Task 2: SignalModule interface

**Files:**
- Create: `services/player/internal/service/recs/signal.go`

- [ ] **Step 1: Create signal.go**

```go
package recs

import "context"

// SignalModule is the pluggable contract every ranking signal implements.
// Concrete implementations live in services/player/internal/service/recs/signals/.
// See spec §6.1.
//
// Contract:
//   - Score MUST never return NaN, Inf, or negative values.
//   - Score MUST return 0 (or omit the entry) for missing data; emptiness
//     is normal during cold-start, not an error.
//   - Precompute is a no-op for stateless signals (e.g. trending, recency).
type SignalModule interface {
	// ID returns the stable identifier (e.g. SignalID("s1")). Used for
	// logging, admin debug page columns, and weight registry keys.
	ID() SignalID

	// Precompute runs the heavy per-user step. Called from cron jobs and
	// the on-write debouncer in later phases. May be a no-op.
	Precompute(ctx context.Context, userID UserID) error

	// Score returns raw scores for each candidate in pool. Candidates with
	// no signal contribution may be omitted from the returned map; the
	// normalizer treats missing entries as zero.
	Score(ctx context.Context, userID UserID, candidates []AnimeID) (map[AnimeID]RawScore, error)
}
```

- [ ] **Step 2: Verify package compiles**

Run: `cd services/player && go build ./internal/service/recs/...`
Expected: exit 0, no output.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/service/recs/signal.go
git commit -m "feat(player/recs): define SignalModule interface"
```

---

### Task 3: MinMaxNormalize helper (TDD)

**Files:**
- Create: `services/player/internal/service/recs/normalize_test.go`
- Create: `services/player/internal/service/recs/normalize.go`

- [ ] **Step 1: Write the failing tests**

Create `services/player/internal/service/recs/normalize_test.go`:

```go
package recs

import (
	"math"
	"testing"
)

func TestMinMaxNormalize(t *testing.T) {
	const eps = 1e-6

	tests := []struct {
		name string
		raw  map[AnimeID]RawScore
		pool []AnimeID
		// expect[id] is the expected NormalizedScore for that id.
		expect map[AnimeID]NormalizedScore
	}{
		{
			name:   "empty pool returns empty map",
			raw:    map[AnimeID]RawScore{},
			pool:   []AnimeID{},
			expect: map[AnimeID]NormalizedScore{},
		},
		{
			name: "single-element pool collapses to zero (degenerate)",
			raw:  map[AnimeID]RawScore{"a": 5},
			pool: []AnimeID{"a"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
			},
		},
		{
			name: "all-equal pool collapses to zero (degenerate)",
			raw:  map[AnimeID]RawScore{"a": 7, "b": 7, "c": 7},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0, "b": 0, "c": 0,
			},
		},
		{
			name: "normal pool: min->0, max->1",
			raw:  map[AnimeID]RawScore{"a": 0, "b": 5, "c": 10},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0.5,
				"c": 1,
			},
		},
		{
			name: "missing candidate defaults to zero",
			raw:  map[AnimeID]RawScore{"a": 0, "c": 10},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0,
				"c": 1,
			},
		},
		{
			name: "scale-invariance: small absolute values normalize the same as large",
			raw:  map[AnimeID]RawScore{"a": 0.001, "b": 0.002, "c": 0.003},
			pool: []AnimeID{"a", "b", "c"},
			expect: map[AnimeID]NormalizedScore{
				"a": 0,
				"b": 0.5,
				"c": 1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MinMaxNormalize(tc.raw, tc.pool)
			if len(got) != len(tc.expect) {
				t.Fatalf("len(got)=%d want %d; got=%v", len(got), len(tc.expect), got)
			}
			for id, want := range tc.expect {
				gotV := got[id]
				if math.Abs(float64(gotV-want)) > eps {
					t.Errorf("got[%q]=%v want %v", id, gotV, want)
				}
			}
		})
	}
}

// Property: every output MUST be in [0, 1] regardless of input scale.
func TestMinMaxNormalize_OutputInZeroOneRange(t *testing.T) {
	raw := map[AnimeID]RawScore{
		"a": -100, "b": -50, "c": 0, "d": 50, "e": 100,
	}
	pool := []AnimeID{"a", "b", "c", "d", "e"}
	got := MinMaxNormalize(raw, pool)
	for id, v := range got {
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("got[%q]=%v: must not be NaN/Inf", id, v)
		}
		if f < 0 || f > 1+1e-9 {
			t.Errorf("got[%q]=%v: out of [0,1] range", id, v)
		}
	}
}

// Property: monotonicity. If raw(a) > raw(b), then normalized(a) >= normalized(b).
func TestMinMaxNormalize_Monotonicity(t *testing.T) {
	raw := map[AnimeID]RawScore{"low": 1, "mid": 5, "high": 9}
	pool := []AnimeID{"low", "mid", "high"}
	got := MinMaxNormalize(raw, pool)
	if !(got["low"] <= got["mid"] && got["mid"] <= got["high"]) {
		t.Errorf("monotonicity violated: low=%v mid=%v high=%v", got["low"], got["mid"], got["high"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: build failure — `undefined: MinMaxNormalize`.

- [ ] **Step 3: Write the implementation**

Create `services/player/internal/service/recs/normalize.go`:

```go
package recs

// epsilon prevents division-by-zero in degenerate pools where max == min.
const epsilon = 1e-9

// MinMaxNormalize maps raw scores to NormalizedScore in [0, 1] over the
// candidate pool. Degenerate pools (max-min < epsilon, including empty,
// single-element, and all-equal pools) return all zeros — never NaN.
// Candidates absent from the raw map default to NormalizedScore(0).
//
// See spec §2.3 for why this is per-signal-pool rather than global.
func MinMaxNormalize(raw map[AnimeID]RawScore, pool []AnimeID) map[AnimeID]NormalizedScore {
	out := make(map[AnimeID]NormalizedScore, len(pool))
	if len(pool) == 0 {
		return out
	}

	min, max, ok := findMinMax(raw, pool)
	if !ok || (max-min) < epsilon {
		for _, id := range pool {
			out[id] = 0
		}
		return out
	}

	span := max - min
	for _, id := range pool {
		v, present := raw[id]
		if !present {
			out[id] = 0
			continue
		}
		out[id] = NormalizedScore((float64(v) - min) / span)
	}
	return out
}

// findMinMax inspects raw entries indexed by pool. Returns ok=false when
// the pool has no entries that exist in raw (treated as fully degenerate).
func findMinMax(raw map[AnimeID]RawScore, pool []AnimeID) (min, max float64, ok bool) {
	for _, id := range pool {
		v, present := raw[id]
		if !present {
			continue
		}
		f := float64(v)
		if !ok {
			min, max = f, f
			ok = true
			continue
		}
		if f < min {
			min = f
		}
		if f > max {
			max = f
		}
	}
	return min, max, ok
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: PASS, all subtests green.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/service/recs/normalize.go services/player/internal/service/recs/normalize_test.go
git commit -m "feat(player/recs): add per-pool min-max normalizer with degenerate-pool guards"
```

---

### Task 4: Ensemble aggregator (TDD)

**Files:**
- Create: `services/player/internal/service/recs/ensemble_test.go`
- Create: `services/player/internal/service/recs/ensemble.go`

- [ ] **Step 1: Write the failing tests**

Create `services/player/internal/service/recs/ensemble_test.go`:

```go
package recs

import (
	"context"
	"errors"
	"math"
	"testing"
)

// fakeSignal is a deterministic SignalModule for unit tests.
type fakeSignal struct {
	id     SignalID
	scores map[AnimeID]RawScore
	err    error
}

func (f *fakeSignal) ID() SignalID { return f.id }
func (f *fakeSignal) Precompute(_ context.Context, _ UserID) error { return nil }
func (f *fakeSignal) Score(_ context.Context, _ UserID, candidates []AnimeID) (map[AnimeID]RawScore, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[AnimeID]RawScore, len(candidates))
	for _, id := range candidates {
		if v, ok := f.scores[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func TestEnsemble_RankSingleSignal(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 5, "c": 10}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got)=%d want 3", len(got))
	}
	// After normalization: a=0, b=0.5, c=1. Weight 1.0 → final equals normalized.
	if got[0].AnimeID != "c" || got[1].AnimeID != "b" || got[2].AnimeID != "a" {
		t.Errorf("unexpected sort order: %v", got)
	}
	if math.Abs(got[0].Final-1.0) > 1e-6 {
		t.Errorf("got[0].Final=%v want 1.0", got[0].Final)
	}
	if math.Abs(got[1].Final-0.5) > 1e-6 {
		t.Errorf("got[1].Final=%v want 0.5", got[1].Final)
	}
}

func TestEnsemble_RankWeightedSum(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 10}}
	s2 := &fakeSignal{id: "s2", scores: map[AnimeID]RawScore{"a": 10, "b": 0}}
	e := NewEnsemble([]WeightedSignal{
		{Module: s1, Weight: 0.7},
		{Module: s2, Weight: 0.3},
	})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// After normalization s1: a=0, b=1. s2: a=1, b=0.
	// Weighted: a = 0.7*0 + 0.3*1 = 0.3. b = 0.7*1 + 0.3*0 = 0.7.
	// Expect b first, a second.
	if got[0].AnimeID != "b" || got[1].AnimeID != "a" {
		t.Fatalf("unexpected order: %v", got)
	}
	if math.Abs(got[0].Final-0.7) > 1e-6 {
		t.Errorf("got[0].Final=%v want 0.7", got[0].Final)
	}
	if math.Abs(got[1].Final-0.3) > 1e-6 {
		t.Errorf("got[1].Final=%v want 0.3", got[1].Final)
	}
	// Breakdown must include both signal contributions per anime.
	if _, ok := got[0].Breakdown["s1"]; !ok {
		t.Errorf("got[0].Breakdown missing s1")
	}
	if _, ok := got[0].Breakdown["s2"]; !ok {
		t.Errorf("got[0].Breakdown missing s2")
	}
}

func TestEnsemble_AllSignalsZero(t *testing.T) {
	// Cold-start: signal returns no entries for any candidate.
	cold := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{}}
	e := NewEnsemble([]WeightedSignal{{Module: cold, Weight: 1.0}})
	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, r := range got {
		if r.Final != 0 {
			t.Errorf("got %v: cold-start must produce zero, got %v", r.AnimeID, r.Final)
		}
	}
}

func TestEnsemble_RankEmptyCandidates(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 1}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d want 0", len(got))
	}
}

func TestEnsemble_PropagatesSignalError(t *testing.T) {
	want := errors.New("boom")
	bad := &fakeSignal{id: "s1", err: want}
	e := NewEnsemble([]WeightedSignal{{Module: bad, Weight: 1.0}})

	_, err := e.Rank(context.Background(), "user-1", []AnimeID{"a"})
	if !errors.Is(err, want) {
		t.Errorf("err=%v want %v", err, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: build failure — `undefined: NewEnsemble`, `undefined: WeightedSignal`, etc.

- [ ] **Step 3: Write the implementation**

Create `services/player/internal/service/recs/ensemble.go`:

```go
package recs

import (
	"context"
	"fmt"
	"sort"
)

// WeightedSignal pairs a signal with its weight in the final sum.
// Weights are NOT renormalized when individual signals emit zero — total
// scores sit lower honestly. See spec §2.2.
type WeightedSignal struct {
	Module SignalModule
	Weight float64
}

// Recommendation is the per-anime ensemble output for a single user.
type Recommendation struct {
	AnimeID   AnimeID
	Final     float64
	Breakdown map[SignalID]NormalizedScore
}

// Ensemble aggregates SignalModule outputs by per-pool min-max normalization
// followed by a weighted sum. Filter (S11) and pin (S6) layers wrap the
// ensemble at the call site; this struct is concerned only with scoring.
type Ensemble struct {
	signals []WeightedSignal
}

// NewEnsemble constructs an ensemble. The order of signals does not affect
// the math but is preserved for reproducible breakdown ordering.
func NewEnsemble(signals []WeightedSignal) *Ensemble {
	return &Ensemble{signals: signals}
}

// Rank computes the weighted-sum score for each candidate and returns
// recommendations sorted by Final descending. Empty pool returns nil.
// Any signal error short-circuits and is returned wrapped.
func (e *Ensemble) Rank(ctx context.Context, userID UserID, candidates []AnimeID) ([]Recommendation, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	normalized := make(map[SignalID]map[AnimeID]NormalizedScore, len(e.signals))

	for _, ws := range e.signals {
		raw, err := ws.Module.Score(ctx, userID, candidates)
		if err != nil {
			return nil, fmt.Errorf("recs: signal %q score: %w", ws.Module.ID(), err)
		}
		normalized[ws.Module.ID()] = MinMaxNormalize(raw, candidates)
	}

	out := make([]Recommendation, 0, len(candidates))
	for _, id := range candidates {
		breakdown := make(map[SignalID]NormalizedScore, len(e.signals))
		var final float64
		for _, ws := range e.signals {
			score := normalized[ws.Module.ID()][id]
			breakdown[ws.Module.ID()] = score
			final += ws.Weight * float64(score)
		}
		out = append(out, Recommendation{
			AnimeID:   id,
			Final:     final,
			Breakdown: breakdown,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Final > out[j].Final
	})

	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: PASS, all five tests green.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/service/recs/ensemble.go services/player/internal/service/recs/ensemble_test.go
git commit -m "feat(player/recs): add weighted-ensemble aggregator with per-pool normalization"
```

---

### Task 5: Storage domain models

**Files:**
- Create: `services/player/internal/domain/recs.go`

- [ ] **Step 1: Create the domain models file**

Create `services/player/internal/domain/recs.go`:

```go
package domain

import "time"

// RecUserSignals stores per-user precomputed signals. See spec §4.1.
//
// JSONB fields hold sparse maps:
//   - S1Vector: {anime_id: predicted_score}    (k-NN output)
//   - S5Affinity: {attr_id: affinity_value}     (TF-IDF output, keyed by
//                                                "studio:Madhouse", "tag:slice-of-life", etc.)
//
// S6Seed* track the most recent qualifying completion (score >= 7,
// completed within last 7 days) for the "Because you finished X" pin.
type RecUserSignals struct {
	UserID            string     `gorm:"type:uuid;primaryKey" json:"user_id"`
	S1Vector          string     `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"s1_vector"`
	S5Affinity        string     `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"s5_affinity"`
	S6SeedAnimeID     *string    `gorm:"type:uuid" json:"s6_seed_anime_id,omitempty"`
	S6SeedCompletedAt *time.Time `json:"s6_seed_completed_at,omitempty"`
	S6SeedScore       *int       `json:"s6_seed_score,omitempty"`
	LastComputed      time.Time  `gorm:"not null;default:now();index" json:"last_computed"`
}

func (RecUserSignals) TableName() string { return "rec_user_signals" }

// RecPopulationSignals stores population-wide signals (shared across users).
// See spec §4.1.
type RecPopulationSignals struct {
	AnimeID         string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	S3TrendingScore float32   `gorm:"not null;default:0" json:"s3_trending_score"`
	S4RecencyScore  float32   `gorm:"not null;default:0" json:"s4_recency_score"`
	LastComputed    time.Time `gorm:"not null;default:now()" json:"last_computed"`
}

func (RecPopulationSignals) TableName() string { return "rec_population_signals" }

// RecCompletionCoOccurrence is the materialized seed -> candidate co-occurrence
// matrix for S6 local lookups. CoCount counts users who completed both seed
// and candidate with score >= 7. See spec §4.1.
type RecCompletionCoOccurrence struct {
	SeedAnimeID      string    `gorm:"type:uuid;primaryKey" json:"seed_anime_id"`
	CandidateAnimeID string    `gorm:"type:uuid;primaryKey" json:"candidate_anime_id"`
	CoCount          int       `gorm:"not null" json:"co_count"`
	LastComputed     time.Time `gorm:"not null;default:now()" json:"last_computed"`
}

func (RecCompletionCoOccurrence) TableName() string { return "rec_completion_co_occurrence" }
```

- [ ] **Step 2: Verify domain package compiles**

Run: `cd services/player && go build ./internal/domain/...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/domain/recs.go
git commit -m "feat(player): add rec engine storage domain models (user signals, population signals, co-occurrence)"
```

---

### Task 6: Auto-migrate the new tables on startup

**Files:**
- Modify: `services/player/cmd/player-api/main.go` (the `db.AutoMigrate(...)` block, currently lines 47-57)

- [ ] **Step 1: Add the three new models to the AutoMigrate call**

Edit `services/player/cmd/player-api/main.go`. Replace the existing block:

```go
	if err := db.AutoMigrate(
		&domain.WatchProgress{},
		&domain.AnimeListEntry{},
		&domain.WatchHistory{},
		&domain.UserAnimePreference{},
		&domain.Review{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}
```

with:

```go
	if err := db.AutoMigrate(
		&domain.WatchProgress{},
		&domain.AnimeListEntry{},
		&domain.WatchHistory{},
		&domain.UserAnimePreference{},
		&domain.Review{},
		&domain.SyncJob{},
		&domain.ActivityEvent{},
		&domain.RecUserSignals{},
		&domain.RecPopulationSignals{},
		&domain.RecCompletionCoOccurrence{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}
```

- [ ] **Step 2: Verify the player binary compiles**

Run: `cd services/player && go build ./cmd/player-api/...`
Expected: exit 0.

- [ ] **Step 3: Redeploy the player service and check the migration**

Run: `make redeploy-player`
Expected: container starts; logs show GORM migration running for the three new tables.

Verify the tables exist:

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "\dt rec_*"
```

Expected: three rows — `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence`.

- [ ] **Step 4: Commit**

```bash
git add services/player/cmd/player-api/main.go
git commit -m "feat(player): auto-migrate rec engine tables on startup"
```

---

### Task 7: RecsRepository (thin GORM wrapper)

**Files:**
- Create: `services/player/internal/repo/recs.go`

This is a thin wrapper following the existing `PreferenceRepository` pattern. No unit tests at this stage — there is no business logic, just GORM pass-throughs. Phase 2 will exercise these methods via end-to-end tests when the first signals start writing.

- [ ] **Step 1: Create the repository**

Create `services/player/internal/repo/recs.go`:

```go
package repo

import (
	"context"
	"errors"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecsRepository provides access to rec_user_signals, rec_population_signals,
// and rec_completion_co_occurrence. Phase 1 ships only the methods the
// orchestrator stub needs; later phases extend.
type RecsRepository struct {
	db *gorm.DB
}

func NewRecsRepository(db *gorm.DB) *RecsRepository {
	return &RecsRepository{db: db}
}

// GetUserSignals returns the row for a user, or (nil, nil) if no row exists.
// Callers treat a nil result as "no precomputed signals yet" — that's normal
// for new users until the first precompute pass.
func (r *RecsRepository) GetUserSignals(ctx context.Context, userID string) (*domain.RecUserSignals, error) {
	var row domain.RecUserSignals
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpsertUserSignals inserts or updates the row for a user, keyed by user_id.
// Caller is responsible for setting LastComputed before the call.
func (r *RecsRepository) UpsertUserSignals(ctx context.Context, row *domain.RecUserSignals) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"s1_vector", "s5_affinity",
				"s6_seed_anime_id", "s6_seed_completed_at", "s6_seed_score",
				"last_computed",
			}),
		}).
		Create(row).Error
}

// ListPopulationSignals returns every row in rec_population_signals.
// Population is small (~few thousand rows) — full-scan is acceptable.
func (r *RecsRepository) ListPopulationSignals(ctx context.Context) ([]domain.RecPopulationSignals, error) {
	var rows []domain.RecPopulationSignals
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertPopulationSignal upserts a single anime_id row.
func (r *RecsRepository) UpsertPopulationSignal(ctx context.Context, row *domain.RecPopulationSignals) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "anime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"s3_trending_score", "s4_recency_score", "last_computed",
			}),
		}).
		Create(row).Error
}
```

- [ ] **Step 2: Verify the package compiles**

Run: `cd services/player && go build ./internal/repo/...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/repo/recs.go
git commit -m "feat(player): add RecsRepository with user/population signal upserts"
```

---

### Task 8: Precompute orchestrator stub (TDD)

**Files:**
- Create: `services/player/internal/service/recs/precompute_test.go`
- Create: `services/player/internal/service/recs/precompute.go`

The orchestrator's only Phase 1 responsibility: iterate registered modules, call `Precompute` on each, surface the first error. Phase 2 wires it into a cron and the on-write debouncer.

- [ ] **Step 1: Write the failing tests**

Create `services/player/internal/service/recs/precompute_test.go`:

```go
package recs

import (
	"context"
	"errors"
	"testing"
)

// trackingSignal records every Precompute call for assertion.
type trackingSignal struct {
	id    SignalID
	calls []UserID
	err   error
}

func (t *trackingSignal) ID() SignalID { return t.id }
func (t *trackingSignal) Precompute(_ context.Context, userID UserID) error {
	t.calls = append(t.calls, userID)
	return t.err
}
func (t *trackingSignal) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

func TestOrchestrator_RunForUserCallsAllModules(t *testing.T) {
	a := &trackingSignal{id: "s1"}
	b := &trackingSignal{id: "s2"}
	o := NewOrchestrator([]SignalModule{a, b})

	if err := o.RunForUser(context.Background(), "user-1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(a.calls) != 1 || a.calls[0] != "user-1" {
		t.Errorf("a.calls=%v want [user-1]", a.calls)
	}
	if len(b.calls) != 1 || b.calls[0] != "user-1" {
		t.Errorf("b.calls=%v want [user-1]", b.calls)
	}
}

func TestOrchestrator_RunForUserPropagatesError(t *testing.T) {
	want := errors.New("boom")
	bad := &trackingSignal{id: "s1", err: want}
	good := &trackingSignal{id: "s2"}
	o := NewOrchestrator([]SignalModule{bad, good})

	err := o.RunForUser(context.Background(), "user-1")
	if !errors.Is(err, want) {
		t.Errorf("err=%v want wraps %v", err, want)
	}
	// Even on error, the second module should still have been called —
	// orchestrator collects errors rather than short-circuiting, so a slow
	// signal can't block fresh data for others.
	if len(good.calls) != 1 {
		t.Errorf("good.calls=%v want 1 call (orchestrator must not short-circuit)", good.calls)
	}
}

func TestOrchestrator_RunForUserNoModules(t *testing.T) {
	o := NewOrchestrator(nil)
	if err := o.RunForUser(context.Background(), "user-1"); err != nil {
		t.Errorf("empty registry must not error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: build failure — `undefined: NewOrchestrator`.

- [ ] **Step 3: Write the implementation**

Create `services/player/internal/service/recs/precompute.go`:

```go
package recs

import (
	"context"
	"errors"
	"fmt"
)

// Orchestrator runs Precompute across the registered signal modules.
// Phase 1 ships RunForUser only; Phase 2 will add RunPopulation and the
// cron + on-write entry points. See spec §5.
type Orchestrator struct {
	modules []SignalModule
}

// NewOrchestrator wires the orchestrator with the given modules. Order
// determines invocation order, but errors do not short-circuit — every
// module is given a chance to run, and errors are joined and returned.
func NewOrchestrator(modules []SignalModule) *Orchestrator {
	return &Orchestrator{modules: modules}
}

// RunForUser invokes Precompute on every registered module for the given
// user. Errors from individual modules are collected and returned as a
// joined error. If no module errors, returns nil.
func (o *Orchestrator) RunForUser(ctx context.Context, userID UserID) error {
	var errs []error
	for _, m := range o.modules {
		if err := m.Precompute(ctx, userID); err != nil {
			errs = append(errs, fmt.Errorf("recs: precompute %q for user %q: %w", m.ID(), userID, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/player && go test ./internal/service/recs/...`
Expected: PASS, all three tests green.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/service/recs/precompute.go services/player/internal/service/recs/precompute_test.go
git commit -m "feat(player/recs): add precompute orchestrator stub with non-short-circuiting error collection"
```

---

### Task 9: Final verification + after-update

**Files:**
- None. This is a verification + deploy task.

- [ ] **Step 1: Run the full player test suite**

Run: `cd services/player && go test ./...`
Expected: PASS. No new failures vs. baseline. The `recs` package shows green for all eight test functions.

- [ ] **Step 2: Run go vet on the new package**

Run: `cd services/player && go vet ./internal/service/recs/... ./internal/domain/... ./internal/repo/...`
Expected: no output (clean).

- [ ] **Step 3: Confirm migration ran in production**

Run: `make redeploy-player && sleep 5 && make health`
Expected: player service `200 OK`, no migration errors in logs.

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "\d rec_user_signals" \
  -c "\d rec_population_signals" \
  -c "\d rec_completion_co_occurrence"
```

Expected: all three tables show their columns with correct types (`uuid` PKs, `jsonb` for signal payloads, `timestamp` for `last_computed`).

- [ ] **Step 4: Invoke the after-update skill**

Per CLAUDE.md, every implementation must end with `/animeenigma-after-update` to lint, redeploy, update changelog, commit, and push. The earlier per-task commits already covered the redeploys, but the changelog entry and final push happen here.

Run skill: `animeenigma-after-update` with summary: *"Phase 1 of recommendations engine — pluggable SignalModule interface, weighted-ensemble aggregator, and storage tables landed. No user-visible changes yet; the foundation supports v2.0 Phases 2-5 in the milestone roadmap."*

---

## Self-review checklist

**Spec coverage (cross-checked against design spec §1-§14):**

| Spec section | Covered by |
|---|---|
| §2.1 weighted ensemble pattern | Task 4 (Ensemble) |
| §2.2 final score formula | Task 4 — formula matches `final += weight × normalized` |
| §2.3 per-pool min-max normalization | Task 3 (MinMaxNormalize) — degenerate-pool guard included |
| §3 SignalModule contract | Task 2 (interface) — ID, Precompute, Score signatures |
| §4.1 storage tables | Tasks 5 + 6 — three GORM models, AutoMigrate registered |
| §4.2 Redis keys | **Deferred to Phase 2** (spec'd; nothing to wire until first signal exists) |
| §4.3 storage budget | No code action — sized for current scale |
| §5 refresh strategy | Task 8 — orchestrator stub only; cron wiring is Phase 2 |
| §6.1 SignalModule interface (Go signature) | Task 2 |
| §6 backend layout | Tasks 1, 3, 4, 8 — all under `services/player/internal/service/recs/` |
| §11.1 unit tests per module | Tasks 3, 4, 8 — table-driven tests + property tests |
| §13 locked decisions | Spec is the source of truth; this plan does not relitigate |

**Out-of-scope items confirmed deferred:**
- API endpoints (§7) → Phase 2/3 plan
- Admin debug page (§9) → Phase 2 plan
- Cold-start surface labels (§8) → Phase 3 plan (frontend)
- Concrete signals S1-S6 (§3) → Phases 2-5 plans

**Placeholder scan:** searched plan for "TBD", "TODO", "implement later", "appropriate error handling", "similar to". Only legitimate matches are inside spec quotes (which are commentary, not action items). No placeholders in task code.

**Type consistency:** `AnimeID`, `UserID`, `SignalID`, `RawScore`, `NormalizedScore`, `WeightedSignal`, `Recommendation`, `SignalModule`, `Ensemble`, `Orchestrator`, `RecUserSignals`, `RecPopulationSignals`, `RecCompletionCoOccurrence`, `RecsRepository` — names used identically across all tasks. Method signatures match spec §6.1 exactly.

**Test discipline:** every code-bearing component (normalize, ensemble, orchestrator) is preceded by a failing-test step. Trivial type aliases (Task 1), interface declaration (Task 2), GORM domain structs (Task 5), AutoMigrate wiring (Task 6), and the thin GORM repo (Task 7) skip tests intentionally — they have no behavior to test, only structure that the compiler verifies.

---

**End of Phase 1 plan.** Phases 2-6 (concrete signals, API, frontend, etc.) get their own plans built on this foundation.
