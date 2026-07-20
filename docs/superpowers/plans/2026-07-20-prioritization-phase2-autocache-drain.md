# Unified Prioritization — Phase 2: Autocache Weighted Drain — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the autocache demand Planner's single-class FIFO drain with a weighted drain that reserves a configurable share of each batch for hot-reason demand (`next_ep`, `ongoing`) vs `backfill`, so re-asserted ongoing demand can't be starved by a backlog of static backfill and vice-versa — the WR-01 anti-starvation guarantee becomes an explicit weight instead of a FIFO side effect.

**Architecture:** A new `DemandRepository.DrainWeighted(hotN, coldN)` does two `requested_at ASC` class queries and merges them (short class fills the other's remainder). The Planner computes `hotN`/`coldN` from `AUTOCACHE_HOT_SHARE` (default 0.70) and falls back to the existing `Drain` on error. Wire contract, producers (scheduler Logic A / player Logic B / catalog backfill), and eviction ranking are untouched.

**Tech Stack:** Go (library :8089, separate `library` Postgres DB). Tests: in-memory SQLite with explicit DDL for repo; existing `fakeDrainer` harness in `planner_test.go` for the Planner.

## Global Constraints

- **library is on a SEPARATE Postgres** and cannot join watch tables — the band signal is the existing `autocache_demand.reason`, already on every row. No cross-DB join, no new endpoint call.
- **WR-01 preserved:** `Record` still never bumps `requested_at` on re-assert; within-class ordering stays FIFO by first-seen time.
- **Planner-package-local change only** — do NOT touch `NewPlanner`'s signature (every test call site depends on it), the wire contract, or eviction.
- **Never run `gofmt -w` / `make fmt`** — fix `gofmt -l` findings by hand.
- **Co-authors** on every commit (Claude Code / 0neymik0 / NANDIorg).

---

## File Structure

- `services/library/internal/repo/demand.go` — add `DrainWeighted` (after `Drain`, ~line 105).
- `services/library/internal/repo/demand_weighted_test.go` — **new** SQLite repo test.
- `services/library/internal/autocache/planner.go` — add `demandDrainer.DrainWeighted` to the interface (:25), add `hotShare` package var + `envHotShare()`, use `DrainWeighted` in `runOnce` (:237) with `Drain` fallback.
- `services/library/internal/autocache/planner_test.go` — add `DrainWeighted` to `fakeDrainer` (:23-47), add a split test.
- `docs/environment-variables.md` — document `AUTOCACHE_HOT_SHARE`.

Reference facts (verified in code):
- `domain.DemandReasonBackfill = "backfill"`, `domain.DemandReasonNextEp = "next_ep"`, `domain.DemandReasonOngoing = "ongoing"` (`services/library/internal/domain/autocache_demand.go`).
- `autocache_demand` columns: `mal_id`, `episode` (composite PK), `reason`, `requested_at`, `titles`.
- `drainBatchLimit = 50` (`planner.go:94`); `Drain` called at `planner.go:237`.

---

## Task 1: `DrainWeighted` repo method

**Files:**
- Modify: `services/library/internal/repo/demand.go`
- Test: `services/library/internal/repo/demand_weighted_test.go` (create)

**Interfaces:**
- Produces: `func (r *DemandRepository) DrainWeighted(ctx context.Context, hotN, coldN int) ([]domain.AutocacheDemand, error)` — up to `hotN` rows with `reason IN ('next_ep','ongoing')` and up to `coldN` with `reason='backfill'`, each `requested_at ASC`; if one class is short, the remainder is filled from the other so the batch is never under-filled below `hotN+coldN` when rows exist. Non-positive total → nil.

- [ ] **Step 1: Write the failing test**

Create `services/library/internal/repo/demand_weighted_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDemandTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE autocache_demand (
		mal_id TEXT, episode INTEGER, reason TEXT, requested_at DATETIME, titles TEXT,
		PRIMARY KEY (mal_id, episode)
	)`).Error)
	return db
}

func seedDemand(t *testing.T, db *gorm.DB, mal string, ep int, reason domain.DemandReason, at time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO autocache_demand (mal_id,episode,reason,requested_at,titles) VALUES (?,?,?,?,?)`,
		mal, ep, string(reason), at, "T").Error)
}

func TestDrainWeightedSplitAndFIFO(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	// 4 hot (2 ongoing + 2 next_ep) and 4 backfill, ascending requested_at.
	seedDemand(t, db, "h", 1, domain.DemandReasonOngoing, base.Add(1*time.Minute))
	seedDemand(t, db, "h", 2, domain.DemandReasonNextEp, base.Add(2*time.Minute))
	seedDemand(t, db, "h", 3, domain.DemandReasonOngoing, base.Add(3*time.Minute))
	seedDemand(t, db, "h", 4, domain.DemandReasonNextEp, base.Add(4*time.Minute))
	seedDemand(t, db, "c", 1, domain.DemandReasonBackfill, base.Add(1*time.Minute))
	seedDemand(t, db, "c", 2, domain.DemandReasonBackfill, base.Add(2*time.Minute))
	seedDemand(t, db, "c", 3, domain.DemandReasonBackfill, base.Add(3*time.Minute))
	seedDemand(t, db, "c", 4, domain.DemandReasonBackfill, base.Add(4*time.Minute))

	rows, err := r.DrainWeighted(context.Background(), 3, 2)
	require.NoError(t, err)
	require.Len(t, rows, 5)
	// First 3 hot (FIFO), next 2 backfill (FIFO).
	require.Equal(t, "h", rows[0].MALID)
	require.Equal(t, 1, rows[0].Episode)
	require.Equal(t, 3, rows[2].Episode)         // hot FIFO up to hotN
	require.Equal(t, domain.DemandReasonBackfill, rows[3].Reason)
	require.Equal(t, 1, rows[3].Episode)         // backfill FIFO
}

func TestDrainWeightedShortClassFillsRemainder(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	// Only 1 hot, plenty of backfill; hotN=3 coldN=2 → 1 hot + 4 backfill = 5.
	seedDemand(t, db, "h", 1, domain.DemandReasonOngoing, base)
	for i := 1; i <= 5; i++ {
		seedDemand(t, db, "c", i, domain.DemandReasonBackfill, base.Add(time.Duration(i)*time.Minute))
	}
	rows, err := r.DrainWeighted(context.Background(), 3, 2)
	require.NoError(t, err)
	require.Len(t, rows, 5) // 1 hot + 4 backfill (remainder filled)
	hot := 0
	for _, x := range rows {
		if x.Reason != domain.DemandReasonBackfill {
			hot++
		}
	}
	require.Equal(t, 1, hot)
}

func TestDrainWeightedNonPositive(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	rows, err := r.DrainWeighted(context.Background(), 0, 0)
	require.NoError(t, err)
	require.Nil(t, rows)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/content-verify-probing && go test ./services/library/internal/repo/ -run TestDrainWeighted`
Expected: FAIL — `DrainWeighted` undefined.

- [ ] **Step 3: Implement**

Add to `services/library/internal/repo/demand.go` after `Drain` (~line 105):

```go
// DrainWeighted returns up to hotN hot-reason rows (next_ep, ongoing) and up to
// coldN backfill rows, each ordered requested_at ASC (FIFO within class). If one
// class has fewer than its quota, the remainder is filled from the other class
// so the batch is never under-filled while rows remain — the weighted analogue
// of Drain that makes WR-01 anti-starvation an explicit share rather than a FIFO
// side effect. Non-positive total returns no rows. Errors wrap CodeInternal.
func (r *DemandRepository) DrainWeighted(ctx context.Context, hotN, coldN int) ([]domain.AutocacheDemand, error) {
	total := hotN + coldN
	if total <= 0 {
		return nil, nil
	}
	drainClass := func(where string, args ...any) ([]domain.AutocacheDemand, error) {
		var rows []domain.AutocacheDemand
		err := r.db.WithContext(ctx).
			Where(where, args...).
			Order("requested_at ASC").
			Limit(total). // fetch up to the whole batch so a short sibling can borrow
			Find(&rows).Error
		return rows, err
	}
	hot, err := drainClass("reason IN ?", []domain.DemandReason{domain.DemandReasonNextEp, domain.DemandReasonOngoing})
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "drain weighted hot")
	}
	cold, err := drainClass("reason = ?", domain.DemandReasonBackfill)
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "drain weighted cold")
	}

	out := make([]domain.AutocacheDemand, 0, total)
	takeHot := min2(hotN, len(hot))
	takeCold := min2(coldN, len(cold))
	out = append(out, hot[:takeHot]...)
	out = append(out, cold[:takeCold]...)
	// Fill the remaining slots from whichever class still has rows (hot first).
	for i := takeHot; i < len(hot) && len(out) < total; i++ {
		out = append(out, hot[i])
	}
	for i := takeCold; i < len(cold) && len(out) < total; i++ {
		out = append(out, cold[i])
	}
	return out, nil
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/library/internal/repo/ -run TestDrainWeighted`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/library/internal/repo/demand.go services/library/internal/repo/demand_weighted_test.go
git commit -m "feat(library): DrainWeighted — hot/backfill share drain for autocache demand"
```

---

## Task 2: Planner uses `DrainWeighted` (env share + fallback)

**Files:**
- Modify: `services/library/internal/autocache/planner.go`
- Test: `services/library/internal/autocache/planner_test.go`
- Modify: `docs/environment-variables.md`

**Interfaces:**
- Consumes: `DemandRepository.DrainWeighted` (Task 1).
- Produces: `hotShare` package var (env `AUTOCACHE_HOT_SHARE`, default 0.70); `demandDrainer` interface gains `DrainWeighted(ctx, hotN, coldN int) ([]domain.AutocacheDemand, error)`.

- [ ] **Step 1: Write the failing planner test**

Add `DrainWeighted` to `fakeDrainer` in `planner_test.go` and a split test. First extend `fakeDrainer` (near :23):

```go
// weightedCalls records the (hotN, coldN) DrainWeighted was called with.
// If drainWeightedErr is set, DrainWeighted returns it (to exercise fallback).
```

Add methods:

```go
func (f *fakeDrainer) DrainWeighted(_ context.Context, hotN, coldN int) ([]domain.AutocacheDemand, error) {
	f.gotHotN, f.gotColdN = hotN, coldN
	if f.drainWeightedErr != nil {
		return nil, f.drainWeightedErr
	}
	// Emulate the real split over f.drained (already class-tagged in the test).
	var hot, cold []domain.AutocacheDemand
	for _, r := range f.drained {
		if r.Reason == domain.DemandReasonBackfill {
			cold = append(cold, r)
		} else {
			hot = append(hot, r)
		}
	}
	out := append([]domain.AutocacheDemand{}, hot[:min2t(hotN, len(hot))]...)
	out = append(out, cold[:min2t(coldN, len(cold))]...)
	return out, nil
}

func min2t(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

Add fields `gotHotN, gotColdN int` and `drainWeightedErr error` to the `fakeDrainer` struct. Add a test:

```go
func TestPlannerUsesWeightedDrainShare(t *testing.T) {
	// drainBatchLimit=50, hotShare default 0.70 → hotN=35, coldN=15.
	d := &fakeDrainer{drained: []domain.AutocacheDemand{
		{MALID: "1", Episode: 5, Reason: domain.DemandReasonOngoing, Titles: "Anime"},
	}}
	p, pr, e, s, c, m := plannerFixtures(t) // reuse whatever fixture builder exists; else inline the fakes
	_ = pr
	_ = e
	_ = s
	_ = c
	_ = m
	_ = p
	// Run one sweep (call the exported/one-shot runOnce the other tests use).
	// After the sweep, assert the weighted split was requested:
	if d.gotHotN != 35 || d.gotColdN != 15 {
		t.Fatalf("weighted split = (%d,%d), want (35,15)", d.gotHotN, d.gotColdN)
	}
}
```

Because the exact single-sweep entry point and fixture builder are already used by neighbouring tests (grep `runOnce`/`newPlannerForTest` in `planner_test.go`), the implementer wires this test through the same harness those tests use, seeding `d` and asserting `d.gotHotN/gotColdN`. Minimum assertions: (1) default share yields (35,15) for batch 50; (2) with `d.drainWeightedErr` set, the planner falls back to `Drain` (assert the sweep still processes `d.drained`, e.g. a delete/enqueue happens as in the existing present/enqueue tests).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/library/internal/autocache/ -run TestPlannerUsesWeightedDrainShare`
Expected: FAIL — `fakeDrainer` has no `DrainWeighted` / assertion fails (planner still calls `Drain`).

- [ ] **Step 3: Implement in planner.go**

Add `DrainWeighted` to the `demandDrainer` interface (:25):

```go
type demandDrainer interface {
	Drain(ctx context.Context, limit int) ([]domain.AutocacheDemand, error)
	DrainWeighted(ctx context.Context, hotN, coldN int) ([]domain.AutocacheDemand, error)
	Delete(ctx context.Context, malID string, episode int) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}
```

Add the share var near the `drainBatchLimit` const (:92-97):

```go
// hotShare is the fraction of each drain batch reserved for hot-reason demand
// (next_ep, ongoing) over backfill (spec §5). Env-overridable; clamped to [0,1].
var hotShare = envHotShare()

func envHotShare() float64 {
	if v := os.Getenv("AUTOCACHE_HOT_SHARE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			return f
		}
	}
	return 0.70
}
```

Add `"os"` and `"strconv"` to the planner imports.

Replace the drain call at :237:

```go
	hotN := int(float64(drainBatchLimit) * hotShare)
	coldN := drainBatchLimit - hotN
	rows, err := p.demand.DrainWeighted(ctx, hotN, coldN)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("autocache planner: weighted drain failed; falling back to FIFO", "error", err)
		}
		rows, err = p.demand.Drain(ctx, drainBatchLimit)
		if err != nil {
			if p.log != nil {
				p.log.Warnw("autocache planner: drain failed", "error", err)
			}
			return cadence
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/library/internal/autocache/`
Expected: PASS (new split test + all existing planner tests — `fakeDrainer` now satisfies the extended interface).

- [ ] **Step 5: Document the env knob**

In `docs/environment-variables.md`, in the library/autocache section, add:

```
- `AUTOCACHE_HOT_SHARE` (default `0.70`, clamped 0..1) — fraction of each autocache demand drain batch reserved for hot-reason demand (`next_ep`, `ongoing`) over `backfill`. The remainder goes to backfill; a short class lends its unused slots to the other.
```

- [ ] **Step 6: Commit**

```bash
git add services/library/internal/autocache/planner.go services/library/internal/autocache/planner_test.go docs/environment-variables.md
git commit -m "feat(library): planner weighted drain by reason-class (AUTOCACHE_HOT_SHARE); FIFO fallback"
```

---

## Post-plan verification

```bash
cd /data/animeenigma/.claude/worktrees/content-verify-probing
go build ./services/library/...
go test ./services/library/internal/repo/ ./services/library/internal/autocache/
gofmt -l services/library/internal/repo/demand.go services/library/internal/autocache/planner.go
```

Expected: builds clean, tests pass, `gofmt -l` silent (fix by hand if flagged). Then `/animeenigma-after-update` (redeploys `library`, changelog, push). No live-signal change is user-visible immediately; verify via `docker logs animeenigma-library --since 30m 2>&1 | grep -i "weighted drain"` shows no fallback warnings and the planner keeps enqueuing.

---

## Self-Review notes (author)

- **Spec coverage:** §5 weighted reason-class drain → Task 1 `DrainWeighted`; 70/30 default + env → Task 2 `hotShare`/`envHotShare`; within-class FIFO preserved → both class queries `ORDER BY requested_at ASC`; short-class remainder fill → Task 1 fill loops; single-class/error fallback → Task 2 `Drain` fallback; producers/wire/eviction untouched → confirmed (only Planner + repo touched).
- **Type consistency:** `demandDrainer` interface gains `DrainWeighted` with the exact signature Task 1 defines; `fakeDrainer` implements it; `hotN+coldN = drainBatchLimit` (50) so no batch inflation.
- **WR-01:** `Record` unchanged — `requested_at` still never bumped on re-assert; FIFO within class intact.
