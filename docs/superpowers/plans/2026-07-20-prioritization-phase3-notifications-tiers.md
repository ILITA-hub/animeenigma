# Unified Prioritization — Phase 3: Notifications Cadence Tiers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the detector from parser-checking every ongoing combo every hour. Tier the per-run candidate set by `next_episode_at` proximity (imminent/just-aired = hot = every run; otherwise warm = slower cadence), with a hard delivery floor so no watched combo is ever checked less often than `NOTIF_TIER_FLOOR` (default 6h). Cuts external parser traffic and speeds hot titles, without weakening the new-episode delivery guarantee.

**Architecture:** All detector candidates are ongoing (Band 1), so the tier is a per-anime sub-division on `next_episode_at`. A per-anime last-checked timestamp in Redis (`notif:checked:<animeID>`, the client the service already builds) plus airing times from the notifications DB drive a pure `tierFilter`. The filter runs right after combo collection; only selected combos flow through the existing snapshot/parser/diff/notify pipeline unchanged. Fail-open: any airing/Redis error includes everything (today's behavior).

**Tech Stack:** Go (notifications :8090). Tests: in-memory SQLite (airing query), miniredis (checked store), table tests (tier logic).

## Global Constraints

- **Delivery floor is inviolable:** every watched ongoing combo MUST be checked at least once per `NOTIF_TIER_FLOOR`. A never-seen combo (no Redis entry) is always checked. Tiering only DELAYS a cold check, never drops it.
- **Fail-open:** airing-times query error or Redis error → include ALL combos (current behavior). Never let the tier filter drop a combo on infrastructure failure.
- **Backward-compatible first run:** with an empty `notif:checked:*` keyspace, every anime is "never checked" → all combos included → identical to today. Existing detector tests must stay green.
- **Do not touch** the snapshot diff, bootstrap protection, never-lower invariant, or per-user fan-out. Only the candidate-set filter in front of the pipeline and a post-run mark are added.
- **Never run `gofmt -w` / `make fmt`** — fix `gofmt -l` by hand.
- **Co-authors** on every commit (Claude Code / 0neymik0 / NANDIorg).

---

## File Structure

- `services/notifications/internal/job/airing.go` — **new** `AiringTimes` query (animeID → next_episode_at) on the collector's DB.
- `services/notifications/internal/job/airing_test.go` — **new** SQLite test.
- `services/notifications/internal/job/checked.go` — **new** `CheckedStore` (Redis MGET/MSET of `notif:checked:<animeID>`).
- `services/notifications/internal/job/checked_test.go` — **new** miniredis test.
- `services/notifications/internal/job/tier.go` — **new** pure `tierFilter` + `tierDue`.
- `services/notifications/internal/job/tier_test.go` — **new** table test.
- `services/notifications/internal/job/detector.go` — thread `checked`/`airing`/tier config; filter after Step 1; mark after Step 5.
- `services/notifications/internal/job/detector_test.go` — update constructor call; add a tier-integration assertion.
- `services/notifications/internal/config/config.go` — `HotWindow`/`WarmEvery`/`TierFloor` on `DetectorConfig`.
- `services/notifications/cmd/notifications-api/main.go` — build `CheckedStore` from `redisCache.Client()`, pass into the detector.
- `docs/environment-variables.md` — document `NOTIF_HOT_WINDOW`/`NOTIF_WARM_EVERY`/`NOTIF_TIER_FLOOR`.

Reference facts (verified in code):
- `HotCombosCollector{db *gorm.DB, log *logger.Logger}`, `Collect(ctx) ([]domain.Combo, error)` (`hotcombos.go`).
- `domain.Combo{AnimeID, ShikimoriID, Player, Language, WatchType, TranslationID}` (comparable, used as map key — do NOT add fields to it).
- Detector `Run` steps: 1 Collect → 2 BulkLoad → 3 checkCombos → 4 diff → 5 BulkUpsert → 6 notify (`detector.go:103-160`).
- `NewNewEpisodeDetectorJob(...)` constructor at `detector.go:66`; deps struct fields `hotCombos`, `snapshots`, `cfg`, `log` (:48-56).
- Redis client available in `main.go` via `redisCache.Client()` (`cache.New(cfg.Redis)` already built).

---

## Task 1: `AiringTimes` query

**Files:**
- Create: `services/notifications/internal/job/airing.go`
- Test: `services/notifications/internal/job/airing_test.go`

**Interfaces:**
- Produces: `func (c *HotCombosCollector) AiringTimes(ctx context.Context, animeIDs []string) (map[string]*time.Time, error)` — `SELECT id, next_episode_at FROM animes WHERE id IN (...)`; missing/NULL → absent from the map (caller treats absent as "unknown → hot").

- [ ] **Step 1: Write the failing test**

Create `services/notifications/internal/job/airing_test.go`:

```go
package job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAiringTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (id TEXT PRIMARY KEY, next_episode_at DATETIME)`).Error)
	return db
}

func TestAiringTimes(t *testing.T) {
	db := setupAiringTestDB(t)
	soon := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec(`INSERT INTO animes (id,next_episode_at) VALUES ('a1',?),('a2',NULL)`, soon).Error)

	c := NewHotCombosCollector(db, nil)
	got, err := c.AiringTimes(context.Background(), []string{"a1", "a2", "a3"})
	require.NoError(t, err)
	require.NotNil(t, got["a1"])
	require.True(t, got["a1"].Equal(soon))
	// a2 has NULL, a3 absent → both omitted from the map.
	require.Nil(t, got["a2"])
	require.NotContains(t, got, "a3")
}

func TestAiringTimesEmptyInput(t *testing.T) {
	db := setupAiringTestDB(t)
	c := NewHotCombosCollector(db, nil)
	got, err := c.AiringTimes(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/content-verify-probing && go test ./services/notifications/internal/job/ -run TestAiringTimes`
Expected: FAIL — `AiringTimes` undefined.

- [ ] **Step 3: Implement**

Create `services/notifications/internal/job/airing.go`:

```go
package job

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
)

// AiringTimes returns next_episode_at per anime id, for the tier decision.
// Rows with a NULL next_episode_at (and ids not present) are omitted — the
// caller treats an absent value as "airing unknown → hot".
func (c *HotCombosCollector) AiringTimes(ctx context.Context, animeIDs []string) (map[string]*time.Time, error) {
	out := map[string]*time.Time{}
	if len(animeIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID            string
		NextEpisodeAt *time.Time
	}
	if err := c.db.WithContext(ctx).
		Table("animes").
		Select("id, next_episode_at").
		Where("id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "airing times")
	}
	for _, r := range rows {
		if r.NextEpisodeAt != nil {
			out[r.ID] = r.NextEpisodeAt
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/notifications/internal/job/ -run TestAiringTimes`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/job/airing.go services/notifications/internal/job/airing_test.go
git commit -m "feat(notifications): AiringTimes query for cadence tiering"
```

---

## Task 2: `CheckedStore` (Redis last-checked)

**Files:**
- Create: `services/notifications/internal/job/checked.go`
- Test: `services/notifications/internal/job/checked_test.go`

**Interfaces:**
- Produces: `type CheckedStore struct{ rdb *redis.Client; now func() time.Time }`; `NewCheckedStore(rdb *redis.Client) *CheckedStore`; `func (s *CheckedStore) LastChecked(ctx, animeIDs []string) map[string]time.Time` (absent = not-yet-checked; fail-open → empty map on error); `func (s *CheckedStore) MarkChecked(ctx, animeIDs []string)` (SET each `notif:checked:<id>` = unix now, TTL `checkedTTL=48h`; best-effort).

- [ ] **Step 1: Write the failing test**

Create `services/notifications/internal/job/checked_test.go`:

```go
package job

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestChecked(t *testing.T) (*CheckedStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return NewCheckedStore(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestCheckedMarkAndRead(t *testing.T) {
	s, _ := newTestChecked(t)
	ctx := context.Background()

	// Nothing checked yet → empty map.
	require.Empty(t, s.LastChecked(ctx, []string{"a1", "a2"}))

	s.MarkChecked(ctx, []string{"a1"})
	got := s.LastChecked(ctx, []string{"a1", "a2"})
	_, ok := got["a1"]
	require.True(t, ok, "a1 must be marked checked")
	_, ok = got["a2"]
	require.False(t, ok, "a2 was never checked")
}

func TestCheckedEmptyInput(t *testing.T) {
	s, _ := newTestChecked(t)
	require.Empty(t, s.LastChecked(context.Background(), nil))
	s.MarkChecked(context.Background(), nil) // no panic
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/notifications/internal/job/ -run TestChecked`
Expected: FAIL — `CheckedStore` undefined.

- [ ] **Step 3: Implement**

Create `services/notifications/internal/job/checked.go`:

```go
package job

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// checkedTTL bounds notif:checked growth and lets a title that stops being
// scanned age back to "never checked" (fail-safe re-check). Comfortably above
// any tier floor.
const checkedTTL = 48 * time.Hour

func checkedKey(animeID string) string { return "notif:checked:" + animeID }

// CheckedStore records the last time each anime's combos were parser-checked,
// so the detector can tier its per-run candidate set and still guarantee a
// delivery floor. All reads fail open (error → empty map = "check them").
type CheckedStore struct {
	rdb *redis.Client
	now func() time.Time
}

// NewCheckedStore constructs the store over the service's Redis client.
func NewCheckedStore(rdb *redis.Client) *CheckedStore {
	return &CheckedStore{rdb: rdb, now: time.Now}
}

// LastChecked returns the last-checked time per anime id that has one. Ids
// absent from the result were never checked (or expired). Fail-open: any
// Redis error yields an empty map, so the caller checks everything.
func (s *CheckedStore) LastChecked(ctx context.Context, animeIDs []string) map[string]time.Time {
	out := map[string]time.Time{}
	if len(animeIDs) == 0 {
		return out
	}
	keys := make([]string, len(animeIDs))
	for i, id := range animeIDs {
		keys[i] = checkedKey(id)
	}
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		s, ok := v.(string)
		if !ok {
			continue
		}
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		out[animeIDs[i]] = time.Unix(sec, 0)
	}
	return out
}

// MarkChecked stamps each anime id as checked now (TTL checkedTTL). Best-effort.
func (s *CheckedStore) MarkChecked(ctx context.Context, animeIDs []string) {
	if len(animeIDs) == 0 {
		return
	}
	now := strconv.FormatInt(s.now().Unix(), 10)
	pipe := s.rdb.Pipeline()
	for _, id := range animeIDs {
		pipe.Set(ctx, checkedKey(id), now, checkedTTL)
	}
	_, _ = pipe.Exec(ctx)
}
```

Note the receiver-name shadow: the local `s, ok := v.(string)` shadows the `*CheckedStore` receiver `s` inside `LastChecked`. Rename the local to `str` to avoid confusion:

```go
	for i, v := range vals {
		str, ok := v.(string)
		if !ok {
			continue
		}
		sec, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			continue
		}
		out[animeIDs[i]] = time.Unix(sec, 0)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/notifications/internal/job/ -run TestChecked`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/job/checked.go services/notifications/internal/job/checked_test.go
git commit -m "feat(notifications): CheckedStore — per-anime last-checked in Redis"
```

---

## Task 3: `tierFilter` pure logic

**Files:**
- Create: `services/notifications/internal/job/tier.go`
- Test: `services/notifications/internal/job/tier_test.go`

**Interfaces:**
- Produces: `type TierWindows struct{ Hot, Warm, Floor time.Duration }`; `func tierDue(nextEp *time.Time, lastChecked time.Time, checkedKnown bool, now time.Time, w TierWindows) bool`; `func tierFilter(combos []domain.Combo, airing map[string]*time.Time, lastChecked map[string]time.Time, now time.Time, w TierWindows) []domain.Combo` — includes ALL combos of any anime that is due.

- [ ] **Step 1: Write the failing table test**

Create `services/notifications/internal/job/tier_test.go`:

```go
package job

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTierDue(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	w := TierWindows{Hot: 36 * time.Hour, Warm: 3 * time.Hour, Floor: 6 * time.Hour}
	soon := now.Add(10 * time.Hour)  // within hot window
	far := now.Add(10 * 24 * time.Hour)

	// Never checked → due.
	require.True(t, tierDue(&far, time.Time{}, false, now, w))
	// Hot (imminent) + checked 1h ago → due (every run).
	require.True(t, tierDue(&soon, now.Add(-1*time.Hour), true, now, w))
	// Warm + checked 1h ago → NOT due (warm cadence 3h not elapsed).
	require.False(t, tierDue(&far, now.Add(-1*time.Hour), true, now, w))
	// Warm + checked 4h ago → due (warm cadence elapsed).
	require.True(t, tierDue(&far, now.Add(-4*time.Hour), true, now, w))
	// Warm + checked 2h ago but floor 6h... not yet; still warm-not-due.
	require.False(t, tierDue(&far, now.Add(-2*time.Hour), true, now, w))
	// Any tier + checked 7h ago → floor forces due.
	require.True(t, tierDue(&far, now.Add(-7*time.Hour), true, now, w))
	// Unknown airing (nil) → treated hot → due.
	require.True(t, tierDue(nil, now.Add(-1*time.Hour), true, now, w))
}

func TestTierFilterIncludesAllCombosOfDueAnime(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	w := TierWindows{Hot: 36 * time.Hour, Warm: 3 * time.Hour, Floor: 6 * time.Hour}
	far := now.Add(10 * 24 * time.Hour)
	combos := []domain.Combo{
		{AnimeID: "hot", Player: "kodik"},
		{AnimeID: "hot", Player: "english"},   // second combo of the same anime
		{AnimeID: "cold", Player: "kodik"},
	}
	airing := map[string]*time.Time{"hot": timePtr(now.Add(5 * time.Hour)), "cold": &far}
	lastChecked := map[string]time.Time{
		"hot":  now.Add(-1 * time.Hour), // hot → due
		"cold": now.Add(-1 * time.Hour), // warm, checked 1h ago → not due
	}
	got := tierFilter(combos, airing, lastChecked, now, w)
	require.Len(t, got, 2) // both "hot" combos, no "cold"
	for _, c := range got {
		require.Equal(t, "hot", c.AnimeID)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/notifications/internal/job/ -run 'TestTierDue|TestTierFilter'`
Expected: FAIL — `TierWindows`/`tierDue`/`tierFilter` undefined.

- [ ] **Step 3: Implement**

Create `services/notifications/internal/job/tier.go`:

```go
package job

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
)

// TierWindows are the cadence-tier durations (spec §4).
type TierWindows struct {
	Hot   time.Duration // ± window on next_episode_at that counts as "imminent/just-aired"
	Warm  time.Duration // minimum spacing between checks for non-hot titles
	Floor time.Duration // hard delivery floor: no combo checked less often than this
}

// within reports whether t is within ±w of now.
func within(t, now time.Time, w time.Duration) bool {
	d := t.Sub(now)
	if d < 0 {
		d = -d
	}
	return d <= w
}

// tierDue decides whether an anime should be checked this run.
//   - never checked → always (bootstrap / fail-safe).
//   - checked older than Floor → always (delivery guarantee).
//   - hot (next episode within ±Hot, or airing unknown) → every run.
//   - warm → only when Warm has elapsed since the last check.
func tierDue(nextEp *time.Time, lastChecked time.Time, checkedKnown bool, now time.Time, w TierWindows) bool {
	if !checkedKnown {
		return true
	}
	if now.Sub(lastChecked) >= w.Floor {
		return true
	}
	hot := nextEp == nil || within(*nextEp, now, w.Hot)
	if hot {
		return true
	}
	return now.Sub(lastChecked) >= w.Warm
}

// tierFilter returns the combos to check this run: every combo whose anime is
// due. Grouping by anime keeps the delivery floor per-combo (including an
// anime includes all its combos).
func tierFilter(combos []domain.Combo, airing map[string]*time.Time, lastChecked map[string]time.Time, now time.Time, w TierWindows) []domain.Combo {
	decision := map[string]bool{}
	out := make([]domain.Combo, 0, len(combos))
	for _, c := range combos {
		due, done := decision[c.AnimeID]
		if !done {
			lc, known := lastChecked[c.AnimeID]
			due = tierDue(airing[c.AnimeID], lc, known, now, w)
			decision[c.AnimeID] = due
		}
		if due {
			out = append(out, c)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/notifications/internal/job/ -run 'TestTierDue|TestTierFilter'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/job/tier.go services/notifications/internal/job/tier_test.go
git commit -m "feat(notifications): tierFilter — next-episode-proximity cadence with delivery floor"
```

---

## Task 4: Wire tiering into the detector + config + main

**Files:**
- Modify: `services/notifications/internal/config/config.go` (DetectorConfig fields + env)
- Modify: `services/notifications/internal/job/detector.go` (deps + filter + mark)
- Modify: `services/notifications/internal/job/detector_test.go` (constructor + assertion)
- Modify: `services/notifications/cmd/notifications-api/main.go` (build store, pass deps)
- Modify: `docs/environment-variables.md`

**Interfaces:**
- Consumes: `CheckedStore` (Task 2), `AiringTimes` (Task 1), `tierFilter`/`TierWindows` (Task 3).
- Produces: `DetectorConfig.HotWindow/WarmEvery/TierFloor time.Duration`; detector constructor gains `checked *CheckedStore` param.

- [ ] **Step 1: Add config fields**

In `services/notifications/internal/config/config.go`, add to `DetectorConfig`:

```go
	// Cadence tiering (spec §4). HotWindow is the ± window on next_episode_at
	// that keeps an ongoing on the hourly (every-run) tier; WarmEvery is the
	// spacing for non-hot titles; TierFloor is the hard delivery floor —
	// no combo is checked less often than this.
	HotWindow time.Duration
	WarmEvery time.Duration
	TierFloor time.Duration
```

In the `Load()` block that fills `DetectorConfig` (grep `NOTIFICATIONS_DETECTOR_CRON`), add:

```go
		HotWindow: getEnvDuration("NOTIF_HOT_WINDOW", 36*time.Hour),
		WarmEvery: getEnvDuration("NOTIF_WARM_EVERY", 3*time.Hour),
		TierFloor: getEnvDuration("NOTIF_TIER_FLOOR", 6*time.Hour),
```

Use the config file's existing duration-env helper (grep for how `ParserTimeout` is parsed — reuse that helper name, e.g. `getEnvDuration`).

- [ ] **Step 2: Write the failing detector integration assertion**

In `services/notifications/internal/job/detector_test.go`, the existing tests build a `NewEpisodeDetectorJob`. After Task 4 the constructor takes a `*CheckedStore`. First run the suite to see it fail to compile:

Run: `go test ./services/notifications/internal/job/ -run TestNewEpisodeDetector 2>&1 | head`
Expected: build error — constructor arity mismatch after Step 3 (once detector.go changes). If detector.go hasn't changed yet, this compiles; make the constructor change in Step 3 first, then update the test.

Add a focused test that a warm, recently-checked anime is skipped:

```go
func TestDetectorTierSkipsWarmRecentlyChecked(t *testing.T) {
	// Harness: reuse the existing detector test builder. Seed:
	//  - a fake hotCombos returning one combo for anime "warm1"
	//  - AiringTimes: "warm1" next episode 10 days out (not hot)
	//  - CheckedStore pre-marked "warm1" as checked 1h ago (miniredis)
	// Expect: the parser checker is NOT called (0 combos scanned after filter),
	// report.CombosScanned reflects the pre-filter count but the parser fan-out
	// sees 0. Assert via the fake episode-checker's call count == 0.
	// (Wire through the same fakes the neighbouring detector tests use.)
}
```

The implementer completes this against the existing detector-test fakes. Minimum assertion: with a warm + recently-checked anime, the fake parser/episode-checker receives zero combos; with the same anime marked checked 7h ago (past floor), it receives the combo.

- [ ] **Step 3: Thread deps + filter + mark into detector.go**

Add fields to `NewEpisodeDetectorJob` (:48-56):

```go
	checked *CheckedStore
	airing  *HotCombosCollector // reuse the collector for AiringTimes (same DB)
```

`airing` is the same `*HotCombosCollector` as `hotCombos` — pass it once and reference it for both. (No second collector; just call `j.hotCombos.AiringTimes(...)`.) So only add `checked *CheckedStore` as a new field/param.

Update the constructor `NewNewEpisodeDetectorJob` (:66) to accept `checked *CheckedStore` and set `j.checked = checked`.

In `Run`, right after Step 1 collects combos and sets `report.CombosScanned` (:108-109), insert the tier filter:

```go
	// Cadence tiering (spec §4): only check combos whose anime is due this run.
	// Fail-open — an airing or Redis error includes everything (today's cadence).
	animeIDs := distinctAnimeIDs(combos)
	airing, err := j.hotCombos.AiringTimes(ctx, animeIDs)
	if err != nil {
		if j.log != nil {
			j.log.Warnw("detector airing-times fetch failed; skipping tier filter", "error", err)
		}
		airing = nil // fail-open: nil map → all treated hot
	}
	lastChecked := j.checked.LastChecked(ctx, animeIDs)
	w := TierWindows{Hot: j.cfg.HotWindow, Warm: j.cfg.WarmEvery, Floor: j.cfg.TierFloor}
	combos = tierFilter(combos, airing, lastChecked, time.Now(), w)
	report.CombosSelected = len(combos)
	if len(combos) == 0 {
		j.recordOutcome("success", &report)
		j.logCompleted(report)
		return report, nil
	}
```

Add the helper (in detector.go or tier.go):

```go
func distinctAnimeIDs(combos []domain.Combo) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(combos))
	for _, c := range combos {
		if _, ok := seen[c.AnimeID]; !ok {
			seen[c.AnimeID] = struct{}{}
			out = append(out, c.AnimeID)
		}
	}
	return out
}
```

Add `CombosSelected int` to `RunReport` (next to `CombosScanned`).

After Step 5's `BulkUpsert` succeeds (the combos WERE checked this run), mark them:

```go
	j.checked.MarkChecked(ctx, distinctAnimeIDs(combos))
```

Place this right after the successful `BulkUpsert` (:136-138), before the affected-count branch, so every checked anime is stamped even when there are no diffs.

- [ ] **Step 4: Build notifications + update main**

In `services/notifications/cmd/notifications-api/main.go`, after `redisCache` is built (~:115) and before the detector job is constructed, add:

```go
	checkedStore := job.NewCheckedStore(redisCache.Client())
```

Pass `checkedStore` into `job.NewNewEpisodeDetectorJob(...)` in the argument position matching the new `checked` param.

- [ ] **Step 5: Update the detector test constructor calls + run**

Update every `NewNewEpisodeDetectorJob(...)` in `detector_test.go` to pass a `*CheckedStore` (build one over a miniredis, e.g. a `newTestChecked(t)` helper — reuse the one from `checked_test.go` in the same package). For tests that must preserve today's "check all" behavior, an empty miniredis makes every anime "never checked" → all included (backward-compatible). Set the tier config on the `DetectorConfig` the tests build (e.g. `HotWindow: 36h, WarmEvery: 3h, TierFloor: 6h`); with all combos unchecked, they're all included regardless.

Run: `go test ./services/notifications/internal/job/ ./services/notifications/internal/config/`
Expected: PASS (new tier tests + existing detector tests green — empty checked store preserves "check all").

- [ ] **Step 6: Build whole service**

Run: `go build ./services/notifications/...`
Expected: no errors.

- [ ] **Step 7: Document env**

In `docs/environment-variables.md`, notifications section:

```
- `NOTIF_HOT_WINDOW` (default `36h`) — ± window on `next_episode_at` within which an ongoing stays on the every-run (hourly) tier; airing-unknown titles are also treated hot.
- `NOTIF_WARM_EVERY` (default `3h`) — minimum spacing between checks for non-hot ongoings.
- `NOTIF_TIER_FLOOR` (default `6h`) — hard delivery floor: no watched combo is checked less often than this, regardless of tier.
```

- [ ] **Step 8: Commit**

```bash
git add services/notifications/internal/config/config.go services/notifications/internal/job/detector.go services/notifications/internal/job/detector_test.go services/notifications/cmd/notifications-api/main.go docs/environment-variables.md
git commit -m "feat(notifications): tiered detector cadence by next-episode proximity + delivery floor"
```

---

## Post-plan verification

```bash
cd /data/animeenigma/.claude/worktrees/content-verify-probing
go build ./services/notifications/...
go test ./services/notifications/...
gofmt -l services/notifications/internal/job/airing.go services/notifications/internal/job/checked.go services/notifications/internal/job/tier.go services/notifications/internal/job/detector.go services/notifications/internal/config/config.go
```

Expected: builds clean, tests pass, `gofmt -l` silent (fix by hand if flagged). Then `/animeenigma-after-update` (redeploys `notifications`, changelog, push). Live check: `docker logs animeenigma-notifications --since 2h 2>&1 | grep -iE "combos_selected|combos_scanned"` — after two runs, `combos_selected < combos_scanned` for a catalog with non-imminent ongoings, and every anime still re-checked within 6h (watch `notif:checked:*` TTLs). Verify a NEW-episode notification still fires for an imminent title (the delivery guarantee).

---

## Self-Review notes (author)

- **Spec coverage:** §4 proximity tiers → Task 3 `tierDue` (hot/warm); delivery floor → `tierDue` floor branch + Task 2 `CheckedStore`; `next_episode_at` on candidates → Task 1 `AiringTimes` (separate map, NOT added to the comparable `Combo` key); fail-open → detector nil-airing path + `LastChecked` empty-on-error; pipeline untouched → filter sits before Step 2, mark after Step 5.
- **Spec deviation (intentional, documented):** spec §4 phrased WARM as "every 3 ticks"; this plan implements it as a **duration** (`NOTIF_WARM_EVERY=3h`) so the cadence is independent of the (configurable) cron interval. Same effect at the default hourly cron. Flag for owner at review.
- **Type consistency:** `TierWindows{Hot,Warm,Floor}` built from `DetectorConfig.HotWindow/WarmEvery/TierFloor`; `CheckedStore.LastChecked` returns `map[string]time.Time` consumed by `tierFilter`; `AiringTimes` returns `map[string]*time.Time` consumed by `tierFilter`. Constructor arity change reconciled in `main.go` + `detector_test.go`.
- **Backward-compat:** empty `notif:checked` → all animes "never checked" → `tierFilter` includes everything → existing detector tests stay green.
