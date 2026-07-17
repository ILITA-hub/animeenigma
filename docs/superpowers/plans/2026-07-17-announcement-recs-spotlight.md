# Announcement-Aware Recs + "Upcoming for you" Spotlight Card — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** S8 franchise signal in the recs engine, an announcement-matching endpoint (`GET /api/users/recs/upcoming` + dismiss), catalog announcement discovery from Shikimori, and a login-only `upcoming_for_you` spotlight card with Add-to-Plan / Dismiss CTAs.

**Architecture:** Recs owns matching (scores announced titles per user with S8+S5+S2 raw-gated ensemble, caches per user); catalog owns discovery (daily Shikimori `status:anons` sync with inline franchise enrichment) and the spotlight resolver (JWT-forwarded HTTP to recs, `personal_pick` pattern); frontend adds the 10th spotlight card via the 5-anchor recipe. Also fixes the stale spotlight→player recs URL (personalized personal_pick has silently 404'd → trending fallback since the 2026-06-11 recs extraction).

**Tech Stack:** Go (chi, GORM, sqlite in-mem tests), Vue 3 + TS (bun, vitest), shared Postgres `animeenigma`, Redis.

**Spec:** `docs/superpowers/specs/2026-07-17-announcement-recs-spotlight-design.md`
**Metrics:** UXΔ = +3 (Better) · CDI = 0.06 * 21 · MVQ = Griffin 85%/80%

## Global Constraints

- Worktree root: `/data/animeenigma/.claude/worktrees/announcement-recs-spotlight` — ALL file paths below are relative to it. NEVER touch `/data/animeenigma` base-tree files directly (Edit/Write absolute paths must stay under the worktree root).
- Commit per task, pathspec-only (`git add <files>` / `git commit -- <files>`). NEVER `git add -A`. Co-author trailer on every commit:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>` + `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>` + `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>` — "Claude Code", no model/version.
- DB status literal for announcements is `'announced'` (Shikimori `anons` is mapped at the parser; never write `'anons'` in SQL).
- S7 must remain LAST in every signal registry (established invariant — it must never win top_contributor ties).
- Signal raw scores: non-negative, non-NaN, non-Inf; omit zero-contribution candidates from the returned map.
- Frontend: `bun`/`bunx` only (never npm/npx); lucide icons via NAMED imports; bind semantic DS tokens (no raw hex/palette classes — DS-lint gates the build); i18n keys must land in en+ru+ja (locale-parity test enforces).
- Go tests for recs run on in-memory SQLite — SQL must stay portable (no `::int` casts, no `NOW()`).
- Cache-version discipline: any ranking-behavior change to the logged-in row bumps `UserTopNKeySuffix` (this plan: v4→v5, Task 2). Public trending key stays `v2`.

---

### Task 1: S8 franchise signal (recs)

**Files:**
- Create: `services/recs/internal/service/recs/signals/s8_franchise.go`
- Test: `services/recs/internal/service/recs/signals/s8_franchise_test.go`

**Interfaces:**
- Consumes: `recs.SignalModule` contract (`ID() recs.SignalID`, `Precompute(ctx, userID) error`, `Score(ctx, userID, candidates) (map[recs.AnimeID]recs.RawScore, error)`), tables `anime_list` (user_id, anime_id, score) and `animes` (id, franchise).
- Produces: `signals.NewS8Franchise(db *gorm.DB) *S8Franchise` with `ID() == recs.SignalID("s8")` — Tasks 2 and 4 register it in ensembles.

- [ ] **Step 1: Write the failing test**

`services/recs/internal/service/recs/signals/s8_franchise_test.go` — NOTE: package `signals` already has a `newTestDB` (s7 test); this file must use a distinct fixture name `newS8TestDB`:

```go
package signals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newS8TestDB creates an in-memory SQLite DB with the minimal schema S8
// needs: animes (id + franchise) and anime_list (user scores). Distinct
// name from s7's newTestDB — same package.
func newS8TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id        TEXT PRIMARY KEY,
		franchise TEXT NOT NULL DEFAULT ''
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		id       TEXT PRIMARY KEY,
		user_id  TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status   TEXT,
		score    INTEGER -- nullable: unscored rows stored as NULL
	)`).Error)
	return db
}

func s8Seed(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()
	require.NoError(t, db.Exec(sql, args...).Error)
}

func TestS8_ID(t *testing.T) {
	s := NewS8Franchise(newS8TestDB(t))
	assert.Equal(t, "s8", string(s.ID()))
}

func TestS8_FranchiseMatchScaledByUserScore(t *testing.T) {
	db := newS8TestDB(t)
	// Seed: user rated "frieren" franchise entry 9 → candidate in same
	// franchise scores (9-5)/5 = 0.8.
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'frieren'), ('cand-1', 'frieren'), ('cand-2', 'other')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`)

	got, err := NewS8Franchise(db).Score(context.Background(), "u1",
		[]string{"cand-1", "cand-2"})
	require.NoError(t, err)
	assert.InDelta(t, 0.8, float64(got["cand-1"]), 0.0001)
	_, hasOther := got["cand-2"]
	assert.False(t, hasOther, "unrelated franchise must be omitted")
}

func TestS8_Score10ClampsToOne(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-1', 'f')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 10)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, float64(got["cand-1"]), 0.0001)
}

func TestS8_BestScoreWinsAcrossFranchiseEntries(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-lo', 'f'), ('seed-hi', 'f'), ('cand-1', 'f')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
		('l1', 'u1', 'seed-lo', 'dropped', 6),
		('l2', 'u1', 'seed-hi', 'completed', 8)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	// MAX(6, 8) = 8 → (8-5)/5 = 0.6
	assert.InDelta(t, 0.6, float64(got["cand-1"]), 0.0001)
}

func TestS8_NeutralAndLowScoresSilent(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-1', 'f')`)
	// score 5 → (5-5)/5 = 0 → omitted. Also NULL score → excluded by SQL.
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
		('l1', 'u1', 'seed-1', 'completed', 5),
		('l2', 'u1', 'seed-1', 'watching', NULL)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-1"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS8_CandidateWithoutFranchiseOmitted(t *testing.T) {
	db := newS8TestDB(t)
	s8Seed(t, db, `INSERT INTO animes (id, franchise) VALUES
		('seed-1', 'f'), ('cand-nofr', '')`)
	s8Seed(t, db, `INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`)
	got, err := NewS8Franchise(db).Score(context.Background(), "u1", []string{"cand-nofr"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestS8_AnonymousAndEmptyPoolSilent(t *testing.T) {
	db := newS8TestDB(t)
	got, err := NewS8Franchise(db).Score(context.Background(), "", []string{"x"})
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = NewS8Franchise(db).Score(context.Background(), "u1", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/announcement-recs-spotlight && go test ./services/recs/internal/service/recs/signals/ -run TestS8 -v`
Expected: FAIL — `undefined: NewS8Franchise`

- [ ] **Step 3: Write the implementation**

`services/recs/internal/service/recs/signals/s8_franchise.go`:

```go
package signals

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S8Franchise is the franchise/sequel-proximity signal (spec 2026-07-17):
// "a new entry in a franchise you scored highly". Seeds are every anime in
// the user's anime_list with score > 5 and a non-empty animes.franchise;
// affinity per franchise is the user's BEST score across its entries. A
// candidate in franchise F scores clamp((best-5)/5, 0, 1) — so a 9/10
// franchise yields 0.8 and a 10/10 yields 1.0.
//
// Positive-only by design: low/dropped franchises contribute 0 here —
// negative pressure is S7's job (no double-penalty). Stateless request-time
// signal, mirrors S2/S7's pattern.
//
// Score nullability: `al.score > 5` in SQL naturally excludes NULL scores
// (NULL comparisons are never TRUE) — unscored list rows are not affinity
// evidence.
type S8Franchise struct {
	db *gorm.DB
}

const (
	// s8NeutralScore is the score at/below which a franchise entry carries
	// no positive affinity (5/10 = neutral).
	s8NeutralScore = 5.0
	// s8ScoreSpan maps (score - neutral) onto [0, 1]: span of 5 means a
	// 10/10 hits exactly 1.0.
	s8ScoreSpan = 5.0
)

// NewS8Franchise wires S8 with a DB handle.
func NewS8Franchise(db *gorm.DB) *S8Franchise {
	return &S8Franchise{db: db}
}

// ID returns the stable signal identifier "s8".
func (s *S8Franchise) ID() recs.SignalID { return recs.SignalID("s8") }

// Precompute is a no-op — S8 is request-time only, like S2/S7.
func (s *S8Franchise) Precompute(_ context.Context, _ recs.UserID) error { return nil }

// Score returns clamp((best_franchise_score-5)/5, 0, 1) for each candidate
// whose franchise the user has scored > 5. Candidates without a franchise,
// with an unknown franchise, or for anonymous callers are omitted (the
// normalizer treats absent entries as zero).
func (s *S8Franchise) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 || userID == "" {
		return out, nil
	}

	// 1. Franchise affinity: best user score per franchise.
	type affRow struct {
		Franchise string
		Best      float64
	}
	var affRows []affRow
	if err := s.db.WithContext(ctx).
		Table("anime_list AS al").
		Select("a.franchise AS franchise, MAX(al.score) AS best").
		Joins("JOIN animes a ON a.id = al.anime_id").
		Where("al.user_id = ? AND al.score > ? AND a.franchise <> ''", userID, s8NeutralScore).
		Group("a.franchise").
		Scan(&affRows).Error; err != nil {
		return nil, fmt.Errorf("s8: load franchise affinity: %w", err)
	}
	if len(affRows) == 0 {
		return out, nil
	}
	affinity := make(map[string]float64, len(affRows))
	for _, r := range affRows {
		affinity[r.Franchise] = r.Best
	}

	// 2. Candidate → franchise map (only rows with a franchise).
	type candRow struct {
		ID        string
		Franchise string
	}
	var candRows []candRow
	if err := s.db.WithContext(ctx).
		Table("animes").
		Select("id, franchise").
		Where("id IN ? AND franchise <> ''", candidates).
		Scan(&candRows).Error; err != nil {
		return nil, fmt.Errorf("s8: load candidate franchises: %w", err)
	}

	// 3. clamp((best-5)/5, 0, 1); omit zero contributions.
	for _, c := range candRows {
		best, ok := affinity[c.Franchise]
		if !ok {
			continue
		}
		v := (best - s8NeutralScore) / s8ScoreSpan
		if v <= 0 {
			continue
		}
		if v > 1 {
			v = 1
		}
		out[c.ID] = recs.RawScore(v)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/recs/internal/service/recs/signals/ -run TestS8 -v`
Expected: PASS (all 7 tests)

- [ ] **Step 5: Run the whole signals package (no regressions)**

Run: `go test ./services/recs/internal/service/recs/signals/`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add services/recs/internal/service/recs/signals/s8_franchise.go services/recs/internal/service/recs/signals/s8_franchise_test.go
git commit -m "feat(recs): S8 franchise/sequel-proximity signal" -m "Best-user-score-per-franchise affinity, clamp((best-5)/5,0,1). Positive-only; S7 keeps the negative role.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/recs/internal/service/recs/signals/s8_franchise.go services/recs/internal/service/recs/signals/s8_franchise_test.go
```

---

### Task 2: Wire S8 into the logged-in ensemble + cache key v5

**Files:**
- Modify: `services/recs/internal/handler/recs.go` (struct field, constructor, `computeFreshForUser` ensemble + `upNextWeights`, package doc comment)
- Modify: `services/recs/internal/service/recs/user_orchestrator.go:31` (`UserTopNKeySuffix`)
- Modify (literal bump `topN:v4` → `topN:v5`): `services/recs/internal/service/recs/user_orchestrator_test.go` (6 occurrences), `services/recs/internal/handler/admin_recs_test.go:496`, `services/recs/internal/handler/recs_test.go:483,503`
- Check-and-update if weight-asserting: `services/recs/internal/handler/admin_recs.go` (admin breakdown builds its own weight registry — S8 must be added there too, before S7)

**Interfaces:**
- Consumes: `signals.NewS8Franchise(db)` from Task 1.
- Produces: logged-in ensemble `{S1:0.27, S2:0.17, S3:0.17, S4:0.09, S5:0.17, S8:0.13, S7:−0.15}` (S7 LAST), key suffix `":topN:v5"`.

- [ ] **Step 1: Bump the cache-key version**

In `services/recs/internal/service/recs/user_orchestrator.go` change:

```go
	UserTopNKeySuffix = ":topN:v4"
```
to:
```go
	// :v5 — S8 franchise signal joined the logged-in ensemble (2026-07-17);
	// v4 was S12-without-S8.
	UserTopNKeySuffix = ":topN:v5"
```
(Replace the old `:v4 — S12 diversification…` comment line above the const block accordingly.)

- [ ] **Step 2: Add the S8 field + constructor wiring in `handler/recs.go`**

In the `RecsHandler` struct, after the `s7` field:
```go
	s8  *signals.S8Franchise      // spec 2026-07-17 — franchise/sequel proximity
```
In `NewRecsHandler`, after the `s7:` line:
```go
		s8:          signals.NewS8Franchise(db),
```

- [ ] **Step 3: Rebalance `computeFreshForUser` ensemble (S7 stays LAST)**

Replace the ensemble literal in `computeFreshForUser`:
```go
	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s1, Weight: 0.27},
		{Module: h.s2, Weight: 0.17},
		{Module: h.s3, Weight: 0.17},
		{Module: h.s4, Weight: 0.09},
		{Module: h.s5, Weight: 0.17},  // Phase 12 (REC-SIG-05)
		{Module: h.s8, Weight: 0.13},  // spec 2026-07-17 — franchise proximity
		{Module: h.s7, Weight: -0.15}, // S7 dropped-penalty: demotes, never buries — MUST stay last
	})
```
and the matching `upNextWeights` literal later in the same function with the identical seven entries. Update the package doc comment (top of file) and the `computeFreshForUser` doc comment formula to `0.27·S1 + 0.17·S2 + 0.17·S3 + 0.09·S4 + 0.17·S5 + 0.13·S8 − 0.15·S7`.

- [ ] **Step 4: Add S8 to the admin breakdown registry**

Open `services/recs/internal/handler/admin_recs.go`, find its weight registry (mirrors upNextWeights), insert `{Module: <s8 field/constructor>, Weight: 0.13}` BEFORE the S7 entry, updating the handler struct/constructor the same way as Step 2 if it owns separate signal fields. Keep S7 last.

- [ ] **Step 5: Bump the 9 test literals**

```bash
cd /data/animeenigma/.claude/worktrees/announcement-recs-spotlight
grep -rl 'topN:v4' services/recs | xargs sed -i 's/topN:v4/topN:v5/g'
grep -rn 'topN:v4' services/recs   # expect: no output
```

- [ ] **Step 6: Run the recs test suite**

Run: `go test ./services/recs/...`
Expected: `ok` for every package. If a handler test asserts specific weights/breakdowns, update it to the Task-2 weights (keep assertions meaningful, don't delete them).

- [ ] **Step 7: Commit**

```bash
git add services/recs/internal/handler/recs.go services/recs/internal/handler/admin_recs.go services/recs/internal/service/recs/user_orchestrator.go services/recs/internal/service/recs/user_orchestrator_test.go services/recs/internal/handler/admin_recs_test.go services/recs/internal/handler/recs_test.go
git commit -m "feat(recs): S8 joins the logged-in ensemble at 0.13; topN cache key v5" -m "Rebalanced {S1 .27, S2 .17, S3 .17, S4 .09, S5 .17, S8 .13, S7 -.15}; S7 stays last.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/recs
```

---

### Task 3: Announcement dismissals (domain + repo + migration)

**Files:**
- Create: `services/recs/internal/domain/rec_announcement_dismissal.go`
- Create: `services/recs/internal/repo/announcement_dismissals.go`
- Test: `services/recs/internal/repo/announcement_dismissals_test.go`
- Modify: `services/recs/cmd/recs-api/main.go` (AutoMigrate list + FK stmt)

**Interfaces:**
- Produces: `domain.RecAnnouncementDismissal` (table `rec_announcement_dismissals`), `repo.NewAnnouncementDismissalsRepository(db) *AnnouncementDismissalsRepository` with methods `Insert(ctx, userID, animeID string) error` (idempotent — ON CONFLICT DO NOTHING) and `ListAnimeIDs(ctx, userID string) ([]string, error)`. Task 4 consumes both.

- [ ] **Step 1: Write the failing repo test**

`services/recs/internal/repo/announcement_dismissals_test.go`:

```go
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
)

func newDismissalsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.RecAnnouncementDismissal{}))
	return db
}

func TestAnnouncementDismissals_InsertAndList(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Insert(ctx, "u1", "a1"))
	require.NoError(t, r.Insert(ctx, "u1", "a2"))
	require.NoError(t, r.Insert(ctx, "u2", "a3"))

	ids, err := r.ListAnimeIDs(ctx, "u1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a1", "a2"}, ids)
}

func TestAnnouncementDismissals_InsertIdempotent(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Insert(ctx, "u1", "a1"))
	require.NoError(t, r.Insert(ctx, "u1", "a1")) // duplicate — must not error

	ids, err := r.ListAnimeIDs(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"a1"}, ids)
}

func TestAnnouncementDismissals_ListEmptyForUnknownUser(t *testing.T) {
	db := newDismissalsTestDB(t)
	r := NewAnnouncementDismissalsRepository(db)
	ids, err := r.ListAnimeIDs(context.Background(), "nobody")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
```

NOTE (SQLite portability): the domain struct's `default:gen_random_uuid()` does not exist in SQLite — the repo's `Insert` must generate the UUID in Go (`github.com/google/uuid`, already an indirect dep of the tree; verify with `go list -m github.com/google/uuid` and `go get` it into `services/recs` if absent).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/recs/internal/repo/ -run TestAnnouncementDismissals -v`
Expected: FAIL — `undefined: domain.RecAnnouncementDismissal`

- [ ] **Step 3: Write domain model + repo**

`services/recs/internal/domain/rec_announcement_dismissal.go`:

```go
package domain

import "time"

// RecAnnouncementDismissal records a user's permanent "don't show this
// announced title again" action from the upcoming_for_you spotlight card
// (spec 2026-07-17). One row per (user, anime); inserts are idempotent.
//
// Owned by the recs service (AutoMigrate in cmd/recs-api/main.go). Also
// reusable later as a mild negative signal and by the future announcement
// notification producer.
type RecAnnouncementDismissal struct {
	ID        string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:uk_rec_ann_dismiss_user_anime,priority:1" json:"user_id"`
	AnimeID   string    `gorm:"type:uuid;not null;uniqueIndex:uk_rec_ann_dismiss_user_anime,priority:2" json:"anime_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName pins the table name explicitly.
func (RecAnnouncementDismissal) TableName() string { return "rec_announcement_dismissals" }
```

`services/recs/internal/repo/announcement_dismissals.go`:

```go
package repo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
)

// AnnouncementDismissalsRepository persists upcoming_for_you dismiss actions
// (spec 2026-07-17).
type AnnouncementDismissalsRepository struct {
	db *gorm.DB
}

// NewAnnouncementDismissalsRepository wires the repository.
func NewAnnouncementDismissalsRepository(db *gorm.DB) *AnnouncementDismissalsRepository {
	return &AnnouncementDismissalsRepository{db: db}
}

// Insert records a dismissal. Idempotent: a duplicate (user, anime) pair is
// a silent no-op via ON CONFLICT DO NOTHING on the unique index. The UUID is
// generated in Go so the same code runs on Postgres and the SQLite test DB.
func (r *AnnouncementDismissalsRepository) Insert(ctx context.Context, userID, animeID string) error {
	row := domain.RecAnnouncementDismissal{
		ID:      uuid.NewString(),
		UserID:  userID,
		AnimeID: animeID,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
			DoNothing: true,
		}).
		Create(&row).Error
}

// ListAnimeIDs returns every anime the user has dismissed, for candidate-pool
// exclusion. Ordered by anime_id for deterministic tests.
func (r *AnnouncementDismissalsRepository) ListAnimeIDs(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&domain.RecAnnouncementDismissal{}).
		Where("user_id = ?", userID).
		Order("anime_id ASC").
		Pluck("anime_id", &ids).Error
	return ids, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./services/recs/internal/repo/ -run TestAnnouncementDismissals -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Register the migration + FKs in main.go**

In `services/recs/cmd/recs-api/main.go`:
1. Add `&domain.RecAnnouncementDismissal{},` to the `db.AutoMigrate(...)` list (after `&domain.RecEvent{},`) and extend the step-6 comment's table list.
2. Append two idempotent FK statements to the `stmts` slice (mirror the existing DO-block pattern):

```go
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_ann_dismiss_user_fkey') THEN
					ALTER TABLE rec_announcement_dismissals
						ADD CONSTRAINT rec_ann_dismiss_user_fkey
						FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
			`DO $$ BEGIN
				IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rec_ann_dismiss_anime_fkey') THEN
					ALTER TABLE rec_announcement_dismissals
						ADD CONSTRAINT rec_ann_dismiss_anime_fkey
						FOREIGN KEY (anime_id) REFERENCES animes(id) ON DELETE CASCADE;
				END IF;
			END $$;`,
```

- [ ] **Step 6: Build + full recs tests**

Run: `go build ./services/recs/... && go test ./services/recs/...`
Expected: build ok, tests `ok`.

- [ ] **Step 7: Commit**

```bash
git add services/recs/internal/domain/rec_announcement_dismissal.go services/recs/internal/repo/announcement_dismissals.go services/recs/internal/repo/announcement_dismissals_test.go services/recs/cmd/recs-api/main.go services/recs/go.mod services/recs/go.sum
git commit -m "feat(recs): rec_announcement_dismissals table + idempotent repo" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/recs
```

---

### Task 4: `GET /api/users/recs/upcoming` + dismiss endpoint (recs)

**Files:**
- Create: `services/recs/internal/handler/upcoming.go`
- Test: `services/recs/internal/handler/upcoming_test.go`
- Modify: `services/recs/internal/config/config.go` (3 env knobs)
- Modify: `services/recs/internal/transport/router.go` (2 routes + constructor param)
- Modify: `services/recs/cmd/recs-api/main.go` (DI)

**Interfaces:**
- Consumes: `signals.NewS8Franchise/NewS5Attribute/NewS2Metadata`, `repo.AnnouncementDismissalsRepository` (Task 3), `recs.Ensemble.RankWithBreakdown`, `authz.ClaimsFromContext`, `httputil.OK/BadRequest/Unauthorized`.
- Produces (wire contract, consumed by the catalog resolver in Task 10 and the FE card):

```
GET /api/users/recs/upcoming  (JWT required; gateway proxies — Task 5)
→ 200 {"success":true,"data":{"items":[UpcomingItem…],"generated_at":RFC3339,"cache_hit":bool}}
UpcomingItem = {
  "anime": {"id","name","name_ru","name_jp","poster_url","score","status","year","season","kind","franchise"},
  "match_score": float,
  "reason": {"kind":"franchise","seed_anime_id","seed_anime_name","seed_anime_name_ru","user_score"} |
            {"kind":"taste"}
}
POST /api/users/recs/upcoming/dismiss  (JWT required) body {"anime_id":"<uuid>"}
→ 200 {"success":true,"data":{"dismissed":true}}
```

- Cache: `recs:user:<uid>:upcoming:v1`, TTL 6h; dismiss busts it.
- Config (env → default): `RECS_UPCOMING_TOPK` → 3, `RECS_UPCOMING_MIN_S8` → 0.2, `RECS_UPCOMING_MIN_S2` → 0.3.
- Eligibility gate runs on RAW scores (per-pool min-max would inflate junk in weak pools): eligible iff `raw_s8 ≥ MIN_S8` OR `raw_s2 ≥ MIN_S2`. Ordering by ensemble Final `{S8:0.5, S5:0.3, S2:0.2}`. Reason = franchise iff `raw_s8 ≥ MIN_S8`, else taste.

- [ ] **Step 1: Write the failing handler test**

`services/recs/internal/handler/upcoming_test.go` — same in-memory-SQLite + fake-cache conventions as `recs_test.go` (reuse its `fakeCache` if exported within the package; it is package `handler`, so reuse directly):

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
)

// newUpcomingTestDB builds the minimal shared-schema slice the upcoming
// endpoint touches: animes (+franchise/status), anime_list, anime_genres,
// anime_tags/tags (S5/S2/S7 attr loaders), watch_history (S5), and the
// service-owned dismissals table.
func newUpcomingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, ddl := range []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY, name TEXT DEFAULT '', name_ru TEXT DEFAULT '',
			name_jp TEXT DEFAULT '', poster_url TEXT DEFAULT '',
			score REAL DEFAULT 0, episodes_count INTEGER DEFAULT 0,
			status TEXT DEFAULT 'released', year INTEGER DEFAULT 0,
			season TEXT DEFAULT '', kind TEXT DEFAULT '',
			franchise TEXT DEFAULT '', hidden INTEGER DEFAULT 0,
			deleted_at DATETIME
		)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY, user_id TEXT, anime_id TEXT,
			status TEXT, score INTEGER
		)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE tags (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE anime_tags (anime_id TEXT, tag_id TEXT, rank INTEGER DEFAULT 0)`,
		`CREATE TABLE anime_studios (anime_id TEXT, studio_id TEXT)`, // S5 loadM2M touches it
		`CREATE TABLE watch_history (
			id TEXT PRIMARY KEY, user_id TEXT, anime_id TEXT,
			episode_number INTEGER, watched_at DATETIME
		)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	// S5.Score reads rec_user_signals via repo.GetUserSignals (nil row = OK,
	// missing TABLE = SQL error) — migrate the real model so the signal stays
	// silent instead of erroring the whole ensemble.
	require.NoError(t, db.AutoMigrate(&domain.RecUserSignals{}, &domain.RecAnnouncementDismissal{}))
	return db
}

func upcomingRequest(t *testing.T, h *UpcomingHandler, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/users/recs/upcoming", nil)
	if userID != "" {
		ctx := authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: userID})
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.GetUpcoming(rec, req)
	return rec
}

func seedUpcoming(t *testing.T, db *gorm.DB) {
	t.Helper()
	// User u1 loved franchise "frieren" (score 9 on seed-1).
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, name_ru, franchise, status) VALUES
		('seed-1', 'Frieren', 'Фрирен', 'frieren', 'released'),
		('ann-franchise', 'Frieren S2', 'Фрирен 2', 'frieren', 'announced'),
		('ann-unrelated', 'Blob', 'Блоб', '', 'announced')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l1', 'u1', 'seed-1', 'completed', 9)`).Error)
}

func TestUpcoming_Unauthorized(t *testing.T) {
	db := newUpcomingTestDB(t)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUpcoming_FranchiseMatchReturnsItemWithReason(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data struct {
			Items []UpcomingItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data.Items, 1, "only the franchise-matched announcement passes the gate")
	it := env.Data.Items[0]
	assert.Equal(t, "ann-franchise", it.Anime.ID)
	assert.Equal(t, "franchise", it.Reason.Kind)
	assert.Equal(t, "seed-1", it.Reason.SeedAnimeID)
	assert.Equal(t, "Frieren", it.Reason.SeedAnimeName)
	assert.Equal(t, 9, it.Reason.UserScore)
	assert.Greater(t, it.MatchScore, 0.0)
}

func TestUpcoming_DismissedAndListedExcluded(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	dismissals := repo.NewAnnouncementDismissalsRepository(db)
	h := NewUpcomingHandler(db, dismissals, newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	// Dismiss the only eligible announcement → empty items.
	require.NoError(t, dismissals.Insert(context.Background(), "u1", "ann-franchise"))
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcoming_ListedAnnouncementExcluded(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	// u1 already planned the announced title → excluded from the pool.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score)
		VALUES ('l2', 'u1', 'ann-franchise', 'plan_to_watch', NULL)`).Error)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcoming_WeakMatchesGatedOut(t *testing.T) {
	db := newUpcomingTestDB(t)
	// Announced title exists but user has NO affinity signals at all.
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, franchise, status) VALUES
		('ann-1', 'Nobody Cares', '', 'announced')`).Error)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"items":[]`)
}

func TestUpcomingDismiss_PersistsAndBustsCache(t *testing.T) {
	db := newUpcomingTestDB(t)
	seedUpcoming(t, db)
	dismissals := repo.NewAnnouncementDismissalsRepository(db)
	cache := newFakeCache()
	h := NewUpcomingHandler(db, dismissals, cache, testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})

	// Warm the cache.
	rec := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec.Code)

	// Dismiss via handler.
	req := httptest.NewRequest(http.MethodPost, "/api/users/recs/upcoming/dismiss",
		strings.NewReader(`{"anime_id":"ann-franchise"}`))
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u1"}))
	drec := httptest.NewRecorder()
	h.PostDismiss(drec, req)
	require.Equal(t, http.StatusOK, drec.Code)

	// Persisted + next GET recomputes without the dismissed title.
	ids, err := dismissals.ListAnimeIDs(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"ann-franchise"}, ids)

	rec2 := upcomingRequest(t, h, "u1")
	require.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), `"items":[]`)
}

func TestUpcomingDismiss_BadBody(t *testing.T) {
	db := newUpcomingTestDB(t)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeCache(), testLogger(t), UpcomingConfig{TopK: 3, MinS8: 0.2, MinS2: 0.3})
	req := httptest.NewRequest(http.MethodPost, "/api/users/recs/upcoming/dismiss",
		strings.NewReader(`{}`))
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u1"}))
	rec := httptest.NewRecorder()
	h.PostDismiss(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
```

IMPORTANT adaptation note for the implementer: `newFakeCache()` and `testLogger(t)` refer to the package-local test helpers used by `recs_test.go` — open that file first and reuse ITS exact fake-cache constructor + logger helper names (rename the calls in this test to match; if no logger helper exists, use `logger.Default()`). Do NOT introduce a second fake cache implementation.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/recs/internal/handler/ -run TestUpcoming -v`
Expected: FAIL — `undefined: NewUpcomingHandler`

- [ ] **Step 3: Write the handler**

`services/recs/internal/handler/upcoming.go`:

```go
// Package handler — upcoming.go: GET /api/users/recs/upcoming +
// POST /api/users/recs/upcoming/dismiss (spec 2026-07-17).
//
// "Announce recs": scores status='announced' titles for a logged-in user
// with the signals that work for unaired content — S8 franchise (dominant),
// S5 attribute affinity, S2 genre similarity. Behavioral signals (S1/S3/S4)
// are structurally ~0 for unaired titles and are not consulted.
//
// Eligibility gate runs on RAW scores, not normalized ones: per-pool min-max
// normalization inflates the best of a garbage pool to 1.0, so a normalized
// floor would pass junk. Raw gates are absolute: raw_s8 >= MinS8 (user
// scored a franchise entry above neutral) OR raw_s2 >= MinS2 (genre Jaccard
// vs a loved seed). S5 raw operates on a tiny scale (~0..0.05) and is used
// for ORDERING only, never gating.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// UpcomingKeyPrefix/Suffix build the per-user upcoming cache key:
// recs:user:<uid>:upcoming:v1. Bump the suffix on any ranking/gate change.
const (
	UpcomingKeyPrefix = "recs:user:"
	UpcomingKeySuffix = ":upcoming:v1"
	upcomingTTL       = 6 * time.Hour
)

// UpcomingConfig carries the env-tunable knobs (config.Load wires them).
type UpcomingConfig struct {
	TopK  int     // RECS_UPCOMING_TOPK, default 3
	MinS8 float64 // RECS_UPCOMING_MIN_S8, default 0.2
	MinS2 float64 // RECS_UPCOMING_MIN_S2, default 0.3
}

// UpcomingAnimePayload is the hydrated anime shape for one upcoming item.
// Superset of RecAnimePayload with the announcement-relevant fields.
type UpcomingAnimePayload struct {
	ID        string  `json:"id"`
	Name      string  `json:"name,omitempty"`
	NameRU    string  `json:"name_ru,omitempty"`
	NameJP    string  `json:"name_jp,omitempty"`
	PosterURL string  `json:"poster_url,omitempty"`
	Score     float64 `json:"score,omitempty"`
	Status    string  `json:"status,omitempty"`
	Year      int     `json:"year,omitempty"`
	Season    string  `json:"season,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	Franchise string  `json:"franchise,omitempty"`
}

// UpcomingReason explains WHY a title matched. Kind is "franchise" (seed
// fields populated) or "taste" (genre/attribute similarity, no seed).
type UpcomingReason struct {
	Kind            string `json:"kind"`
	SeedAnimeID     string `json:"seed_anime_id,omitempty"`
	SeedAnimeName   string `json:"seed_anime_name,omitempty"`
	SeedAnimeNameRU string `json:"seed_anime_name_ru,omitempty"`
	UserScore       int    `json:"user_score,omitempty"`
}

// UpcomingItem is one matched announcement.
type UpcomingItem struct {
	Anime      UpcomingAnimePayload `json:"anime"`
	MatchScore float64              `json:"match_score"`
	Reason     UpcomingReason       `json:"reason"`
}

// UpcomingEnvelope is the data field of the response.
type UpcomingEnvelope struct {
	Items       []UpcomingItem `json:"items"`
	GeneratedAt string         `json:"generated_at"`
	CacheHit    bool           `json:"cache_hit"`
}

// UpcomingHandler serves the two upcoming endpoints.
type UpcomingHandler struct {
	db         *gorm.DB
	dismissals *repo.AnnouncementDismissalsRepository
	cache      recsCache
	log        *logger.Logger
	cfg        UpcomingConfig
	sf         singleflight.Group

	s2 *signals.S2Metadata
	s5 *signals.S5Attribute
	s8 *signals.S8Franchise
}

// NewUpcomingHandler wires the handler. Signals are constructed here (cheap
// struct literals over the shared DB handle). NOTE: S5 needs the recs repo —
// mirror NewRecsHandler's construction exactly.
func NewUpcomingHandler(db *gorm.DB, dismissals *repo.AnnouncementDismissalsRepository, cache recsCache, log *logger.Logger, cfg UpcomingConfig) *UpcomingHandler {
	if cfg.TopK <= 0 {
		cfg.TopK = 3
	}
	return &UpcomingHandler{
		db:         db,
		dismissals: dismissals,
		cache:      cache,
		log:        log,
		cfg:        cfg,
		s2:         signals.NewS2Metadata(db),
		s5:         signals.NewS5Attribute(db, repo.NewRecsRepository(db)),
		s8:         signals.NewS8Franchise(db),
	}
}

// upcomingKey returns the per-user cache key.
func upcomingKey(userID string) string {
	return UpcomingKeyPrefix + userID + UpcomingKeySuffix
}

// GetUpcoming serves the personalized announced-title matches. JWT required
// (router mounts AuthMiddleware, but the handler double-checks claims so a
// direct call without middleware still 401s).
func (h *UpcomingHandler) GetUpcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	userID := claims.UserID

	var cached UpcomingEnvelope
	if err := h.cache.Get(ctx, upcomingKey(userID), &cached); err == nil {
		cached.CacheHit = true
		httputil.OK(w, cached)
		return
	} else if !isCacheMiss(err) {
		h.log.Warnw("upcoming cache read failed; recomputing", "user_id", userID, "error", err)
	}

	key := upcomingKey(userID)
	v, err, _ := h.sf.Do(key, func() (interface{}, error) {
		var warm UpcomingEnvelope
		if cerr := h.cache.Get(ctx, key, &warm); cerr == nil {
			return warm, nil
		}
		env, cerr := h.computeUpcoming(ctx, userID)
		if cerr != nil {
			return UpcomingEnvelope{}, cerr
		}
		if setErr := h.cache.Set(ctx, key, env, upcomingTTL); setErr != nil {
			h.log.Warnw("upcoming cache write failed", "user_id", userID, "error", setErr)
		}
		return env, nil
	})
	if err != nil {
		h.log.Errorw("upcoming compute failed", "user_id", userID, "error", err)
		httputil.OK(w, UpcomingEnvelope{
			Items:       []UpcomingItem{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	httputil.OK(w, v.(UpcomingEnvelope))
}

// dismissBody is the POST /upcoming/dismiss request shape.
type dismissBody struct {
	AnimeID string `json:"anime_id"`
}

// PostDismiss persists a permanent per-user dismissal and busts the upcoming
// cache so the card advances on the next resolve.
func (h *UpcomingHandler) PostDismiss(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	var body dismissBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AnimeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}
	if err := h.dismissals.Insert(ctx, claims.UserID, body.AnimeID); err != nil {
		h.log.Errorw("upcoming dismiss insert failed", "user_id", claims.UserID, "anime_id", body.AnimeID, "error", err)
		httputil.InternalError(w, "failed to persist dismissal")
		return
	}
	if err := h.cache.Delete(ctx, upcomingKey(claims.UserID)); err != nil {
		h.log.Warnw("upcoming cache bust failed", "user_id", claims.UserID, "error", err)
	}
	httputil.OK(w, map[string]bool{"dismissed": true})
}

// computeUpcoming builds the pool, scores it, gates on raw scores, and
// hydrates the top-K.
func (h *UpcomingHandler) computeUpcoming(ctx context.Context, userID string) (UpcomingEnvelope, error) {
	env := UpcomingEnvelope{
		Items:       []UpcomingItem{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// 1. Pool: announced, visible, not listed by the user, not dismissed.
	var pool []recs.AnimeID
	if err := h.db.WithContext(ctx).
		Table("animes AS a").
		Select("a.id").
		Joins("LEFT JOIN anime_list al ON al.anime_id = a.id AND al.user_id = ?", userID).
		Joins("LEFT JOIN rec_announcement_dismissals d ON d.anime_id = a.id AND d.user_id = ?", userID).
		Where("a.status = ?", "announced").
		Where("a.hidden = ?", false).
		Where("a.deleted_at IS NULL").
		Where("al.status IS NULL").
		Where("d.id IS NULL").
		Pluck("a.id", &pool).Error; err != nil {
		return env, err
	}
	if len(pool) == 0 {
		return env, nil
	}

	// 2. Score with the announcement ensemble. RankWithBreakdown so the raw
	//    per-signal scores are available for gating + reason derivation.
	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s8, Weight: 0.50},
		{Module: h.s5, Weight: 0.30},
		{Module: h.s2, Weight: 0.20},
	})
	ranked, err := ensemble.RankWithBreakdown(ctx, recs.UserID(userID), pool)
	if err != nil {
		return env, err
	}

	// 3. Raw-score gate + top-K.
	type pick struct {
		id        string
		final     float64
		rawS8     float64
		franchise bool
	}
	picks := make([]pick, 0, h.cfg.TopK)
	for _, r := range ranked {
		rawS8 := float64(r.Raw[recs.SignalID("s8")])
		rawS2 := float64(r.Raw[recs.SignalID("s2")])
		byFranchise := rawS8 >= h.cfg.MinS8
		if !byFranchise && rawS2 < h.cfg.MinS2 {
			continue
		}
		picks = append(picks, pick{id: r.AnimeID, final: r.Final, rawS8: rawS8, franchise: byFranchise})
		if len(picks) == h.cfg.TopK {
			break
		}
	}
	if len(picks) == 0 {
		return env, nil
	}

	// 4. Hydrate.
	ids := make([]string, len(picks))
	for i, p := range picks {
		ids[i] = p.id
	}
	hydrated, err := h.hydrateUpcoming(ctx, ids)
	if err != nil {
		return env, err
	}

	for _, p := range picks {
		anime, ok := hydrated[p.id]
		if !ok {
			continue
		}
		item := UpcomingItem{Anime: anime, MatchScore: p.final, Reason: UpcomingReason{Kind: "taste"}}
		if p.franchise && anime.Franchise != "" {
			if seed, serr := h.franchiseSeed(ctx, userID, anime.Franchise); serr != nil {
				h.log.Warnw("upcoming franchise seed lookup failed; falling back to taste reason",
					"user_id", userID, "franchise", anime.Franchise, "error", serr)
			} else if seed != nil {
				item.Reason = *seed
			}
		}
		env.Items = append(env.Items, item)
	}
	return env, nil
}

// hydrateUpcoming fetches the announcement card fields in one SELECT.
func (h *UpcomingHandler) hydrateUpcoming(ctx context.Context, ids []string) (map[string]UpcomingAnimePayload, error) {
	type row struct {
		ID        string
		Name      string
		NameRU    string
		NameJP    string
		PosterURL string
		Score     float64
		Status    string
		Year      int
		Season    string
		Kind      string
		Franchise string
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, name, name_ru, name_jp, poster_url, score, status, year, season, kind, franchise").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]UpcomingAnimePayload, len(rows))
	for _, r := range rows {
		out[r.ID] = UpcomingAnimePayload{
			ID: r.ID, Name: r.Name, NameRU: r.NameRU, NameJP: r.NameJP,
			PosterURL: r.PosterURL, Score: r.Score, Status: r.Status,
			Year: r.Year, Season: r.Season, Kind: r.Kind, Franchise: r.Franchise,
		}
	}
	return out, nil
}

// franchiseSeed finds the user's best-scored anime in the given franchise —
// the "you rated X 9/10" half of the why-line. Returns nil (no error) when
// the user has no scored entry in the franchise (shouldn't happen when the
// S8 gate passed, but the data can shift between scoring and hydration).
func (h *UpcomingHandler) franchiseSeed(ctx context.Context, userID, franchise string) (*UpcomingReason, error) {
	type row struct {
		AnimeID string
		Name    string
		NameRU  string
		Score   int
	}
	var rows []row
	if err := h.db.WithContext(ctx).
		Table("anime_list AS al").
		Select("al.anime_id AS anime_id, a.name AS name, a.name_ru AS name_ru, al.score AS score").
		Joins("JOIN animes a ON a.id = al.anime_id").
		Where("al.user_id = ? AND a.franchise = ? AND al.score > 5", userID, franchise).
		Order("al.score DESC").
		Limit(1).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &UpcomingReason{
		Kind:            "franchise",
		SeedAnimeID:     rows[0].AnimeID,
		SeedAnimeName:   rows[0].Name,
		SeedAnimeNameRU: rows[0].NameRU,
		UserScore:       rows[0].Score,
	}, nil
}
```

Adaptation notes for the implementer:
- `recsCache` (Get/Set) is defined in `recs.go` with only Get+Set — `PostDismiss` needs `Delete`. Widen the local interface: add `Delete(ctx context.Context, keys ...string) error` to `recsCache` in `recs.go` (the production `*cache.RedisCache` already has it; update the package-local fake cache in tests to implement it).
- If `httputil.InternalError` doesn't exist, use the package's actual 500 helper (check `libs/httputil` — e.g. `httputil.Error(w, http.StatusInternalServerError, …)`); mirror what other handlers in this repo use.
- `authz.Claims` field name: confirm `UserID` (used by `GetRecs`) — reuse exactly.

- [ ] **Step 4: Run tests**

Run: `go test ./services/recs/internal/handler/ -run TestUpcoming -v`
Expected: PASS (7 tests). Then `go test ./services/recs/...` — all `ok` (the widened `recsCache` interface may require adding `Delete` to the existing fake cache).

- [ ] **Step 5: Config knobs**

In `services/recs/internal/config/config.go` add to `Config`:
```go
	// Upcoming — announcement-matching knobs (spec 2026-07-17).
	UpcomingTopK  int
	UpcomingMinS8 float64
	UpcomingMinS2 float64
```
and in `Load()`:
```go
		UpcomingTopK:  getEnvInt("RECS_UPCOMING_TOPK", 3),
		UpcomingMinS8: getEnvFloat("RECS_UPCOMING_MIN_S8", 0.2),
		UpcomingMinS2: getEnvFloat("RECS_UPCOMING_MIN_S2", 0.3),
```
If `getEnvFloat` doesn't exist yet, add it beside `getEnvInt`:
```go
func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
```

- [ ] **Step 6: Router + DI**

`services/recs/internal/transport/router.go` — add `upcomingHandler *handler.UpcomingHandler` parameter to `NewRouter` (after `recsHandler`), and inside the `/users/recs` route group:
```go
		r.Route("/users/recs", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/", recsHandler.GetRecs)

			// Upcoming (spec 2026-07-17): JWT REQUIRED — announced-title
			// matching is personal by definition.
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(jwtConfig))
				r.Get("/upcoming", upcomingHandler.GetUpcoming)
				r.Post("/upcoming/dismiss", upcomingHandler.PostDismiss)
			})
		})
```
NOTE: chi does not allow `r.Use` after routes are registered in the same group — the nested `r.Group` avoids that. But ALSO: chi panics on duplicate middleware ordering only; mounting `AuthMiddleware` inside a group already wrapped by `OptionalAuthMiddleware` is fine (Optional decodes, Auth enforces).

`services/recs/cmd/recs-api/main.go` — in the Handlers block:
```go
	dismissalsRepo := repo.NewAnnouncementDismissalsRepository(db.DB)
	upcomingHandler := handler.NewUpcomingHandler(db.DB, dismissalsRepo, redisCache, log, handler.UpcomingConfig{
		TopK:  cfg.UpcomingTopK,
		MinS8: cfg.UpcomingMinS8,
		MinS2: cfg.UpcomingMinS2,
	})
```
and pass `upcomingHandler` into `transport.NewRouter(...)` (update the call).

- [ ] **Step 7: Build + full suite**

Run: `go build ./services/recs/... && go test ./services/recs/...`
Expected: ok.

- [ ] **Step 8: Commit**

```bash
git add services/recs
git commit -m "feat(recs): GET /api/users/recs/upcoming + dismiss — announcement matching" -m "Raw-gated (S8>=0.2 OR S2>=0.3) announcement ensemble {S8 .5, S5 .3, S2 .2}, per-user 6h cache recs:user:<uid>:upcoming:v1, permanent dismissals.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/recs
```

---

### Task 5: Gateway subpath proxy for /users/recs/*

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (~line 568, the recs proxy group)

**Interfaces:**
- Produces: `/api/users/recs/upcoming` and `/api/users/recs/upcoming/dismiss` reachable through the gateway (OptionalJWTValidationMiddleware resolves ak_ keys / validates JWTs; recs enforces auth itself).

- [ ] **Step 1: Add the wildcard route**

In the recs proxy group (`r.HandleFunc("/users/recs", …)` / `r.HandleFunc("/users/recs/", …)`), add:

```go
			r.HandleFunc("/users/recs/*", proxyHandler.ProxyToRecs)
```

(chi's `/users/recs/` matches ONLY the exact trailing-slash path; subpaths need the `/*` pattern. Keep all three lines.)

- [ ] **Step 2: Build + gateway tests**

Run: `go build ./services/gateway/... && go test ./services/gateway/...`
Expected: ok.

- [ ] **Step 3: Commit**

```bash
git add services/gateway/internal/transport/router.go
git commit -m "feat(gateway): proxy /api/users/recs/* subpaths to recs" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/gateway/internal/transport/router.go
```

---

### Task 6: Shikimori parser — GetAnnouncedAnime

**Files:**
- Modify: `services/catalog/internal/parser/shikimori/client.go` (new method beside `GetPopularAnime`, ~line 422)
- Test: `services/catalog/internal/parser/shikimori/announced_test.go`

**Interfaces:**
- Consumes: `executeRawQuery` (private helper, same file), `mapStatus` (maps Shikimori `anons` → `domain.StatusAnnounced`).
- Produces: `(c *Client) GetAnnouncedAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error)` — Task 7 consumes.

- [ ] **Step 1: Write the failing test**

`services/catalog/internal/parser/shikimori/announced_test.go` — reuses `newTestClient` from `franchise_test.go` (same package; it sets `GraphQLURL: srvURL + "/api/graphql"`):

```go
package shikimori

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestGetAnnouncedAnime_QueryShapeAndMapping(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/graphql" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		gotQuery = req.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"animes":[{
			"id":"60001","name":"Frieren 2","russian":"Фрирен 2","japanese":"葬送のフリーレン2",
			"status":"anons","score":0,
			"poster":{"originalUrl":"https://x/p.jpg"},
			"genres":[{"id":"8","name":"Drama","russian":"Драма"}],
			"studios":[{"id":"11","name":"Madhouse"}]
		}]}}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnnouncedAnime(context.Background(), 1, 30)
	if err != nil {
		t.Fatalf("GetAnnouncedAnime: %v", err)
	}
	if !strings.Contains(gotQuery, `status: "anons"`) {
		t.Errorf("query must filter status anons, got: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "order: popularity") {
		t.Errorf("query must order by popularity, got: %s", gotQuery)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 anime, got %d", len(got))
	}
	a := got[0]
	if a.Status != domain.StatusAnnounced {
		t.Errorf("status: want %q got %q", domain.StatusAnnounced, a.Status)
	}
	if a.ShikimoriID != "60001" || len(a.Genres) != 1 || len(a.Studios) != 1 {
		t.Errorf("mapping incomplete: %+v", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/parser/shikimori/ -run TestGetAnnouncedAnime -v`
Expected: FAIL — `undefined` method.

- [ ] **Step 3: Implement**

Insert after `GetPopularAnime` in `client.go`:

```go
// GetAnnouncedAnime fetches announced (anons) titles ordered by community
// popularity — the discovery source for announcement matching (spec
// 2026-07-17). Popularity ordering IS the "featured" gate: only titles the
// Shikimori community already anticipates surface here.
func (c *Client) GetAnnouncedAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	gqlQuery := fmt.Sprintf(`{
		animes(limit: %d, page: %d, order: popularity, status: "anons") {
			id name english russian japanese description score status kind rating origin episodes episodesAired duration
			airedOn { year month day }
			releasedOn { year month day }
			nextEpisodeAt
			malId
			poster { originalUrl }
			genres { id name russian }
			studios { id name }
		}
	}`, limit, page)

	return c.executeRawQuery(ctx, gqlQuery)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./services/catalog/internal/parser/shikimori/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/shikimori/client.go services/catalog/internal/parser/shikimori/announced_test.go
git commit -m "feat(catalog): Shikimori GetAnnouncedAnime (anons by popularity)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/catalog/internal/parser/shikimori
```

---

### Task 7: Catalog SyncAnnouncements (service + repo + handler + route)

**Files:**
- Modify: `services/catalog/internal/service/catalog_sync.go` (new method at end)
- Modify: `services/catalog/internal/repo/anime.go` (new method beside `SetFranchise`, ~line 595)
- Modify: `services/catalog/internal/handler/catalog.go` (new handler beside `SyncCalendar`, ~line 357)
- Modify: `services/catalog/internal/transport/router.go` (~line 160, beside `calendar-sync`)

**Interfaces:**
- Consumes: `GetAnnouncedAnime` (Task 6), `s.animeRepo.GetByShikimoriID`, `s.upsertAnimeFromExternal` (persists genres on create), `s.shikimoriClient.GetAnimeFranchise`, `s.animeRepo.SetFranchise`.
- Produces: `POST /api/anime/announcements-sync?limit=30&seed_backfill=40` → `{"success":true,"data":{"imported":n,"refreshed":n,"enriched":n,"failed":n}}`. Task 8's scheduler job consumes. NOTE: no app-level auth — same posture as `calendar-sync`/`batch-refresh` (docker-network + not exposed via nginx to that path family externally… it IS under /api/anime which the gateway proxies, matching calendar-sync's existing posture; do not add new auth here, mirror the sibling).

- [ ] **Step 1: Repo method — seed-side backfill pool**

Append to `services/catalog/internal/repo/anime.go`:

```go
// ListFranchiseUncheckedListed returns anime that appear in at least one
// user's anime_list but were never franchise-checked — the S8 seed-side
// franchise backfill pool (spec 2026-07-17). Bounded by limit; oldest rows
// first so the backfill converges deterministically across daily runs.
func (r *AnimeRepository) ListFranchiseUncheckedListed(ctx context.Context, limit int) ([]*domain.Anime, error) {
	var out []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("franchise_checked = ? AND shikimori_id <> ''", false).
		Where("EXISTS (SELECT 1 FROM anime_list al WHERE al.anime_id = animes.id)").
		Order("created_at ASC").
		Limit(limit).
		Find(&out).Error
	return out, err
}
```

- [ ] **Step 2: Service method**

Append to `services/catalog/internal/service/catalog_sync.go`:

```go
// SyncAnnouncements discovers featured announced (anons) titles from
// Shikimori and prepares them for S8/announcement matching (spec 2026-07-17):
//
//  1. Fetch top-`limit` anons titles by community popularity (the implicit
//     "featured" gate) and upsert them — new titles are imported with
//     genres; existing rows get status/metadata refreshed (this also
//     persists announced→ongoing transitions between batch-refresh runs).
//  2. Franchise-enrich the announced titles (S8 candidate side).
//  3. Franchise-enrich up to `seedBackfillLimit` list-referenced anime that
//     were never franchise-checked (S8 seed side) — converges the sparse
//     franchise coverage (425/4942 rows as of 2026-07-17) where it matters.
//
// Per-title failures are logged and counted, never fatal. Mirrors
// SyncCalendar's structure; called by the scheduler daily.
func (s *CatalogService) SyncAnnouncements(ctx context.Context, limit, seedBackfillLimit int) (imported, refreshed, enriched, failed int, err error) {
	s.log.Infow("starting announcements sync from Shikimori", "limit", limit, "seed_backfill", seedBackfillLimit)

	announced, err := s.shikimoriClient.GetAnnouncedAnime(ctx, 1, limit)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("fetch announced: %w", err)
	}

	// 1. Upsert (import missing / refresh existing).
	for _, anime := range announced {
		select {
		case <-ctx.Done():
			return imported, refreshed, enriched, failed, ctx.Err()
		default:
		}
		existing, gerr := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
		if gerr != nil {
			s.log.Warnw("announcements sync: existence check failed", "shikimori_id", anime.ShikimoriID, "error", gerr)
			failed++
			continue
		}
		if uerr := s.upsertAnimeFromExternal(ctx, anime); uerr != nil {
			s.log.Warnw("announcements sync: upsert failed", "shikimori_id", anime.ShikimoriID, "error", uerr)
			failed++
			continue
		}
		if existing == nil {
			imported++
		} else {
			refreshed++
		}
	}

	// 2+3. Franchise enrichment: announced candidates + list-referenced seeds.
	enrichPool := make([]*domain.Anime, 0, limit+seedBackfillLimit)
	for _, anime := range announced {
		row, gerr := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
		if gerr != nil || row == nil {
			continue
		}
		enrichPool = append(enrichPool, row)
	}
	if seedBackfillLimit > 0 {
		seeds, serr := s.animeRepo.ListFranchiseUncheckedListed(ctx, seedBackfillLimit)
		if serr != nil {
			s.log.Warnw("announcements sync: seed backfill pool query failed", "error", serr)
		} else {
			enrichPool = append(enrichPool, seeds...)
		}
	}
	for _, a := range enrichPool {
		select {
		case <-ctx.Done():
			return imported, refreshed, enriched, failed, ctx.Err()
		default:
		}
		if a.FranchiseChecked || a.Franchise != "" || a.ShikimoriID == "" {
			continue
		}
		fr, ferr := s.shikimoriClient.GetAnimeFranchise(ctx, a.ShikimoriID)
		if ferr != nil {
			// Not marked checked — retried on the next daily run.
			s.log.Debugw("announcements sync: franchise fetch failed", "anime_id", a.ID, "shikimori_id", a.ShikimoriID, "error", ferr)
			failed++
			continue
		}
		if serr := s.animeRepo.SetFranchise(ctx, a.ID, fr); serr != nil {
			s.log.Warnw("announcements sync: persist franchise failed", "anime_id", a.ID, "error", serr)
			failed++
			continue
		}
		enriched++
	}

	s.log.Infow("announcements sync completed",
		"imported", imported, "refreshed", refreshed, "enriched", enriched, "failed", failed)
	return imported, refreshed, enriched, failed, nil
}
```

Adaptation note: `s.shikimoriClient` is the field `SyncCalendar` uses — if it's an interface type, add `GetAnnouncedAnime` + `GetAnimeFranchise` to it (GetAnimeFranchise may already be on the concrete `*shikimori.Client` only — check the field's declared type in `catalog.go` and widen where declared).

- [ ] **Step 3: Handler + route**

Append to `services/catalog/internal/handler/catalog.go` (mirror `SyncCalendar`; parse query ints inline the way `BatchRefreshAnime` parses its params — check its helper style and reuse):

```go
// SyncAnnouncements triggers announcement discovery from Shikimori (called
// by scheduler; spec 2026-07-17). Query knobs: ?limit= (default 30, max 100)
// and ?seed_backfill= (default 40, max 200).
func (h *CatalogHandler) SyncAnnouncements(w http.ResponseWriter, r *http.Request) {
	limit := parseQueryInt(r, "limit", 30, 100)
	seedBackfill := parseQueryInt(r, "seed_backfill", 40, 200)

	imported, refreshed, enriched, failed, err := h.catalogService.SyncAnnouncements(r.Context(), limit, seedBackfill)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]interface{}{
		"imported":  imported,
		"refreshed": refreshed,
		"enriched":  enriched,
		"failed":    failed,
	})
}

// parseQueryInt reads an int query param with default + upper bound.
func parseQueryInt(r *http.Request, key string, def, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
```
(If a same-named helper already exists in the handler package, reuse it instead of redefining.)

Route in `services/catalog/internal/transport/router.go`, directly under the `calendar-sync` line:
```go
		r.Post("/announcements-sync", catalogHandler.SyncAnnouncements)
```

- [ ] **Step 4: Build + tests**

Run: `go build ./services/catalog/... && go test ./services/catalog/...`
Expected: ok (no dedicated service test — the sync path has no existing test harness; coverage = Task 6's parser test + the live verify at the end of the plan).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/catalog_sync.go services/catalog/internal/repo/anime.go services/catalog/internal/handler/catalog.go services/catalog/internal/transport/router.go
git commit -m "feat(catalog): announcements-sync — anons discovery + franchise enrichment" -m "POST /api/anime/announcements-sync: top-popularity anons import/refresh, franchise backfill for candidates + listed seeds.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/catalog
```

---

### Task 8: Scheduler job — daily announcements sync

**Files:**
- Create: `services/scheduler/internal/jobs/announcements.go`
- Modify: `services/scheduler/internal/config/config.go` (cron field + default)
- Modify: `services/scheduler/internal/service/` JobService (thread the new job — positional args; open the file and mirror how `calendarJob` flows)
- Modify: `services/scheduler/cmd/scheduler-api/main.go` (construct + register)

**Interfaces:**
- Consumes: `POST {CatalogServiceURL}/api/anime/announcements-sync` (Task 7).
- Produces: `AnnouncementsSyncCron` env `ANNOUNCEMENTS_SYNC_CRON`, default `"23 5 * * *"` (daily 05:23 — off the :00/:30 marks per fleet convention).

- [ ] **Step 1: Job file** — `services/scheduler/internal/jobs/announcements.go` (mirror of `calendar.go`):

```go
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

// AnnouncementsSyncJob triggers catalog's announcement discovery
// (spec 2026-07-17): top-popularity anons import + franchise enrichment.
type AnnouncementsSyncJob struct {
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger
}

type announcementsSyncResponse struct {
	Data struct {
		Imported  int `json:"imported"`
		Refreshed int `json:"refreshed"`
		Enriched  int `json:"enriched"`
		Failed    int `json:"failed"`
	} `json:"data"`
}

func NewAnnouncementsSyncJob(config *config.JobsConfig, log *logger.Logger) *AnnouncementsSyncJob {
	return &AnnouncementsSyncJob{
		config: config,
		client: &http.Client{
			// Rate-limited Shikimori fan-out (franchise REST calls) can be slow.
			Timeout: 600 * time.Second,
		},
		log: log,
	}
}

// Run calls the catalog announcements-sync endpoint.
func (j *AnnouncementsSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting announcements sync job")

	url := fmt.Sprintf("%s/api/anime/announcements-sync", j.config.CatalogServiceURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("announcements sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("announcements-sync returned status %d: %s", resp.StatusCode, string(body))
	}

	var result announcementsSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	j.log.Infow("announcements sync completed",
		"imported", result.Data.Imported,
		"refreshed", result.Data.Refreshed,
		"enriched", result.Data.Enriched,
		"failed", result.Data.Failed,
	)
	return nil
}
```

- [ ] **Step 2: Config** — in `services/scheduler/internal/config/config.go` add struct field `AnnouncementsSyncCron string` beside `CalendarSyncCron` and in Load():

```go
		AnnouncementsSyncCron: getEnv("ANNOUNCEMENTS_SYNC_CRON", "23 5 * * *"), // Daily at 05:23
```

- [ ] **Step 3: Thread through JobService + main**

Open `services/scheduler/internal/service/` (the JobService file). `NewJobService(...)` and `Start(...)` take POSITIONAL args — add `announcementsJob` / `cfg.Jobs.AnnouncementsSyncCron` params mirroring exactly how `calendarJob` / `CalendarSyncCron` flow (constructor param + field + Start cron param + cron registration inside Start). ⚠ Scheduler FP-alert memory: if JobService seeds a `KnownJobs`/job_successes metric list, ADD the new job name there too (missing seed ⇒ false "job never succeeded" alerts).

In `services/scheduler/cmd/scheduler-api/main.go`:
```go
	announcementsJob := jobs.NewAnnouncementsSyncJob(&cfg.Jobs, log)
```
…and add it to the `service.NewJobService(...)` call + `cfg.Jobs.AnnouncementsSyncCron` to `jobService.Start(...)` in the SAME positional slot order you added them to the signatures.

- [ ] **Step 4: Build + tests**

Run: `go build ./services/scheduler/... && go test ./services/scheduler/...`
Expected: ok.

- [ ] **Step 5: Commit**

```bash
git add services/scheduler
git commit -m "feat(scheduler): daily announcements-sync job (05:23)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/scheduler
```

---

### Task 9: Spotlight RecsClient (+ fix the dead personal_pick recs URL)

**Files:**
- Create: `services/catalog/internal/service/spotlight/client/recs_client.go`
- Test: `services/catalog/internal/service/spotlight/client/recs_client_test.go`
- Modify: `services/catalog/internal/service/spotlight/client/player_client.go` (REMOVE `FetchUserRecs`, `UserRec`, `userRecsEnvelope` — clean break, they move to the recs client)
- Modify: `services/catalog/internal/service/spotlight/client/player_client_test.go` (move the FetchUserRecs tests to recs_client_test.go, retargeted)
- Modify: `services/catalog/internal/service/spotlight/cards/personal_pick.go` (only if it names `*client.PlayerClient` concretely — it consumes a narrow fetcher interface, so likely NO change; verify)
- Modify: `services/catalog/cmd/catalog-api/main.go` (~line 665: construct RecsClient; pass it to `NewPersonalPickResolver` instead of the player client)

**Interfaces:**
- Produces: `client.NewRecsClient(baseURL string, hc *http.Client, log *logger.Logger) *RecsClient` (empty baseURL → `http://recs:8094`), methods:
  - `FetchUserRecs(ctx, jwt string) ([]UserRec, error)` — MOVED verbatim from PlayerClient (log event names renamed `player_client.user_recs.*` → `recs_client.user_recs.*`). This FIXES the bug where personal_pick still called `player:8083/api/users/recs` — a route that moved to recs:8094 on 2026-06-11, silently degrading personal_pick to its trending fallback.
  - `FetchUpcoming(ctx, jwt string) ([]UpcomingWireItem, error)` — Task 10 consumes.

- [ ] **Step 1: Write the failing test**

`recs_client_test.go` — model directly on the existing `player_client_test.go` FetchUserRecs tests (same fake-server + zaptest-observer style), PLUS:

```go
func TestRecsClient_DefaultBaseURL(t *testing.T) {
	c := NewRecsClient("", nil, testLog(t))
	if c.BaseURL() != "http://recs:8094" {
		t.Fatalf("default base URL: got %s", c.BaseURL())
	}
}

func TestRecsClient_FetchUpcoming_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/recs/upcoming" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
			t.Errorf("jwt not forwarded: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[
			{"anime":{"id":"a1","name":"Frieren S2"},"match_score":0.61,
			 "reason":{"kind":"franchise","seed_anime_id":"s1","seed_anime_name":"Frieren","user_score":9}}
		]}}`))
	}))
	defer srv.Close()

	items, err := NewRecsClient(srv.URL, nil, testLog(t)).FetchUpcoming(context.Background(), "jwt-1")
	if err != nil {
		t.Fatalf("FetchUpcoming: %v", err)
	}
	if len(items) != 1 || items[0].MatchScore != 0.61 {
		t.Fatalf("unexpected items: %+v", items)
	}
	if !strings.Contains(string(items[0].Anime), `"id":"a1"`) {
		t.Fatalf("anime payload not forwarded verbatim: %s", items[0].Anime)
	}
}

func TestRecsClient_FetchUpcoming_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := NewRecsClient(srv.URL, nil, testLog(t)).FetchUpcoming(context.Background(), "jwt-1")
	if err == nil {
		t.Fatal("expected error on 502")
	}
}
```
(`testLog(t)` = whatever logger helper `player_client_test.go` uses — reuse its exact construction.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./services/catalog/internal/service/spotlight/client/ -run TestRecsClient -v`
Expected: FAIL — `undefined: NewRecsClient`

- [ ] **Step 3: Implement `recs_client.go`**

```go
// RecsClient is the catalog → recs HTTP fan-out for spotlight resolvers.
//
// FetchUserRecs moved here from PlayerClient on 2026-07-17: the
// /api/users/recs routes migrated player→recs on 2026-06-11 (extraction),
// but the spotlight client kept calling player:8083 — the personalized
// personal_pick path 404'd behind auth and silently fell back to trending.
// Pointing the client at recs:8094 restores personalization.
//
// T-03-05 carry-over: JWT values MUST NEVER appear in log lines.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// defaultRecsBaseURL is the docker-network DNS name + port of the recs service.
const defaultRecsBaseURL = "http://recs:8094"

// defaultRecsTimeout mirrors defaultPlayerTimeout: tighter than the
// aggregator's 800ms per-card budget so transport failures surface first.
const defaultRecsTimeout = 700 * time.Millisecond

// UpcomingWireItem is one announced-title match from
// GET /api/users/recs/upcoming. Anime and Reason are json.RawMessage so the
// resolver forwards recs' payloads verbatim into the spotlight Card without
// re-shaping (same pattern as UserRec.Anime).
type UpcomingWireItem struct {
	Anime      json.RawMessage `json:"anime"`
	MatchScore float64         `json:"match_score"`
	Reason     json.RawMessage `json:"reason"`
}

// upcomingEnvelope is recs' wire format ({success, data:{items}}).
type upcomingEnvelope struct {
	Data struct {
		Items []UpcomingWireItem `json:"items"`
	} `json:"data"`
}

// RecsClient fans out HTTP calls to the recs service.
type RecsClient struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger
}

// NewRecsClient constructs a RecsClient. Empty baseURL → "http://recs:8094".
func NewRecsClient(baseURL string, hc *http.Client, log *logger.Logger) *RecsClient {
	if baseURL == "" {
		baseURL = defaultRecsBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultRecsTimeout}
	}
	return &RecsClient{baseURL: baseURL, http: hc, log: log}
}

// BaseURL returns the configured base URL — exported solely for tests.
func (c *RecsClient) BaseURL() string { return c.baseURL }

// FetchUpcoming calls GET {baseURL}/api/users/recs/upcoming with the
// caller's JWT (required — the endpoint 401s anonymous callers; resolvers
// must not call this without a JWT).
func (c *RecsClient) FetchUpcoming(ctx context.Context, jwt string) ([]UpcomingWireItem, error) {
	endpoint := c.baseURL + "/api/users/recs/upcoming"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("recs_client.upcoming: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.transport_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.upcoming: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.bad_status", "url", endpoint, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("recs_client.upcoming: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env upcomingEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.decode_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.upcoming: decode: %w", err)
	}
	return env.Data.Items, nil
}
```

Then MOVE `FetchUserRecs` + `UserRec` + `userRecsEnvelope` from `player_client.go` into `recs_client.go` as a `RecsClient` method (verbatim body; rename receiver + log event prefixes to `recs_client.user_recs.*`; update the doc comments). Delete them from `player_client.go` and trim its header comment to the list-only surface. Move the corresponding tests.

- [ ] **Step 4: Re-point DI**

In `services/catalog/cmd/catalog-api/main.go` (~line 665):
```go
	spotlightRecsClient := client.NewRecsClient("", nil, log)
```
and change the personal_pick line to:
```go
		cards.NewPersonalPickResolver(catalogService, spotlightRecsClient, redisCache, spotlightRng, log),
```
`personal_pick.go` consumes a narrow fetcher interface — if it compiles unchanged, no resolver edit is needed; if the interface lives in `personal_pick.go` naming `FetchUserRecs`, it is satisfied by `*RecsClient` automatically.

- [ ] **Step 5: Build + tests + commit**

Run: `go build ./services/catalog/... && go test ./services/catalog/internal/service/spotlight/...`
Expected: ok.

```bash
git add services/catalog/internal/service/spotlight/client services/catalog/cmd/catalog-api/main.go services/catalog/internal/service/spotlight/cards
git commit -m "fix(catalog): spotlight recs calls target recs:8094 — restores personal_pick personalization" -m "FetchUserRecs moved PlayerClient->RecsClient (routes migrated 2026-06-11); + FetchUpcoming for the upcoming_for_you card.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/catalog
```

---

### Task 10: `upcoming_for_you` spotlight type + resolver + DI

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go` (new Data structs + `encoding/json` import)
- Create: `services/catalog/internal/service/spotlight/cards/upcoming_for_you.go`
- Test: `services/catalog/internal/service/spotlight/cards/upcoming_for_you_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (append resolver to `spotlightResolvers`)

**Interfaces:**
- Consumes: `client.RecsClient.FetchUpcoming` (Task 9), `cards.JWTFromContext`, `spotlight.Card`/`Resolver` contract, `libs/cache`.
- Produces: `Card{Type:"upcoming_for_you", Data: UpcomingForYouData{Items:[…]}}` — the FE union (Task 11) mirrors this shape.

- [ ] **Step 1: Types** — append to `types.go` (add `"encoding/json"` to imports):

```go
// UpcomingForYouItem is one matched announced title. Anime and Reason are
// forwarded VERBATIM from the recs service wire format (spec 2026-07-17) —
// catalog does not re-shape recs payloads (personal_pick precedent).
type UpcomingForYouItem struct {
	Anime      json.RawMessage `json:"anime"`
	MatchScore float64         `json:"match_score"`
	Reason     json.RawMessage `json:"reason"`
}

// UpcomingForYouData is the payload for `Card{Type: "upcoming_for_you"}` —
// login-only announcement matches. Items MUST be initialized as
// `[]UpcomingForYouItem{}` (never nil) so it marshals as `[]` not `null`.
type UpcomingForYouData struct {
	Items []UpcomingForYouItem `json:"items"`
}
```

- [ ] **Step 2: Failing resolver test** — `upcoming_for_you_test.go`, modeled on `not_time_yet_test.go` (reuse the package's existing fake cache from `fakes_test.go` — open it first and use its constructor):

```go
package cards

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

type fakeUpcomingFetcher struct {
	items []client.UpcomingWireItem
	err   error
	calls int
}

func (f *fakeUpcomingFetcher) FetchUpcoming(_ context.Context, _ string) ([]client.UpcomingWireItem, error) {
	f.calls++
	return f.items, f.err
}

func TestUpcomingForYou_AnonNilNil(t *testing.T) {
	r := NewUpcomingForYouResolver(&fakeUpcomingFetcher{}, newFakeCache(), testLogger(t))
	card, err := r.Resolve(context.Background(), nil)
	if card != nil || err != nil {
		t.Fatalf("anon must be (nil,nil), got card=%v err=%v", card, err)
	}
}

func TestUpcomingForYou_NoJWTNilNil(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger(t))
	// userID set but NO JWT on ctx — defensive path, no fetch.
	card, err := r.Resolve(context.Background(), &uid)
	if card != nil || err != nil || f.calls != 0 {
		t.Fatalf("no-jwt must be (nil,nil) with no fetch, got card=%v err=%v calls=%d", card, err, f.calls)
	}
}

func TestUpcomingForYou_EmptyItemsIneligible(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{}}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger(t))
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if card != nil || err != nil {
		t.Fatalf("empty items must be (nil,nil), got card=%v err=%v", card, err)
	}
}

func TestUpcomingForYou_ItemsProduceCard(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{
		{Anime: []byte(`{"id":"a1","name":"Frieren S2"}`), MatchScore: 0.61, Reason: []byte(`{"kind":"franchise"}`)},
	}}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger(t))
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil || card.Type != "upcoming_for_you" {
		t.Fatalf("expected upcoming_for_you card, got %+v", card)
	}
	data, ok := card.Data.(spotlight.UpcomingForYouData)
	if !ok || len(data.Items) != 1 {
		t.Fatalf("unexpected data: %+v", card.Data)
	}
}

func TestUpcomingForYou_FetchErrorPropagates(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{err: errors.New("boom")}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger(t))
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err == nil || card != nil {
		t.Fatalf("fetch error must propagate, got card=%v err=%v", card, err)
	}
}
```
(`newFakeCache()` / `testLogger(t)` = the exact helper names in `fakes_test.go` — verify and adjust the calls, do NOT reimplement.)

- [ ] **Step 3: Implement the resolver** — `upcoming_for_you.go`:

```go
// UpcomingForYouResolver implements spotlight.Resolver for the
// `upcoming_for_you` card (spec 2026-07-17). Login-only — anon callers get
// (nil, nil) BEFORE any fetch. Calls recs GET /api/users/recs/upcoming with
// the caller's JWT (jwt_context.go pattern); recs owns matching + its own
// 6h per-user cache, so the resolver adds only a short 60s cache to absorb
// refresh-mashing. Empty matches → ineligible (nil, nil) → the slide simply
// doesn't render, which keeps the surface rare by construction.

package cards

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// upcomingFetcher is the minimal surface this resolver needs; the
// production *client.RecsClient satisfies it implicitly.
type upcomingFetcher interface {
	FetchUpcoming(ctx context.Context, jwt string) ([]client.UpcomingWireItem, error)
}

const (
	upcomingForYouKeyPrefix = "spotlight:upcoming_for_you:"
	upcomingForYouTTL       = 60 * time.Second
)

// UpcomingForYouResolver resolves the upcoming_for_you card.
type UpcomingForYouResolver struct {
	recs  upcomingFetcher
	cache cache.Cache
	log   *logger.Logger
}

// NewUpcomingForYouResolver constructs the resolver.
func NewUpcomingForYouResolver(recs upcomingFetcher, c cache.Cache, log *logger.Logger) *UpcomingForYouResolver {
	return &UpcomingForYouResolver{recs: recs, cache: c, log: log}
}

// Type returns the card discriminator string.
func (r *UpcomingForYouResolver) Type() string { return "upcoming_for_you" }

// Resolve produces the card, or (nil, nil) when the user is anonymous, the
// JWT is missing (defensive), or there are no matches.
func (r *UpcomingForYouResolver) Resolve(ctx context.Context, userID *string) (*spotlight.Card, error) {
	if userID == nil || *userID == "" {
		return nil, nil
	}
	jwt, ok := JWTFromContext(ctx)
	if !ok {
		return nil, nil
	}
	key := upcomingForYouKeyPrefix + *userID

	var cached spotlight.UpcomingForYouData
	if err := r.cache.Get(ctx, key, &cached); err == nil {
		if len(cached.Items) == 0 {
			return nil, nil
		}
		return &spotlight.Card{Type: r.Type(), Data: cached}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", key, "error", err)
	}

	wire, err := r.recs.FetchUpcoming(ctx, jwt)
	if err != nil {
		return nil, fmt.Errorf("upcoming_for_you: recs fetch: %w", err)
	}

	data := spotlight.UpcomingForYouData{Items: []spotlight.UpcomingForYouItem{}}
	for _, it := range wire {
		data.Items = append(data.Items, spotlight.UpcomingForYouItem{
			Anime:      it.Anime,
			MatchScore: it.MatchScore,
			Reason:     it.Reason,
		})
	}
	// Cache the empty result too — absorbs refresh-mashing for users with
	// no matches (the common case).
	if err := r.cache.Set(ctx, key, data, upcomingForYouTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
	}
	if len(data.Items) == 0 {
		return nil, nil
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
```

- [ ] **Step 4: DI** — in `main.go`, append inside `spotlightResolvers`:

```go
		// Upcoming-for-you — announced titles matched to the user's taste
		// via recs (spec 2026-07-17). Login-only.
		cards.NewUpcomingForYouResolver(spotlightRecsClient, redisCache, log),
```

- [ ] **Step 5: Tests + build + commit**

Run: `go test ./services/catalog/internal/service/spotlight/... && go build ./services/catalog/...`
Expected: ok. (Additive card type — old cached spotlight snapshots simply lack the card; no cache-shape flush needed.)

```bash
git add services/catalog/internal/service/spotlight/types.go services/catalog/internal/service/spotlight/cards/upcoming_for_you.go services/catalog/internal/service/spotlight/cards/upcoming_for_you_test.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): upcoming_for_you spotlight resolver (login-only, recs-backed)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- services/catalog
```

---

### Task 11: FE types + tokens + HeroSpotlightBlock dispatch

**Files:**
- Modify: `frontend/web/src/types/spotlight.ts` (union + interfaces)
- Modify: `frontend/web/src/components/home/spotlight/tokens.ts` (`cardTokens` entry)
- Modify: `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (import + dispatch branch + `cardTitle` case + `cardImageUrls` case)

**Interfaces:**
- Consumes: BE wire shape from Task 4/10.
- Produces: `UpcomingForYouData`/`UpcomingForYouItem`/`UpcomingReasonFE` types + the `'upcoming_for_you'` union variant that Task 12's SFC props consume.

- [ ] **Step 1: types** — in `types/spotlight.ts` add above the union:

```ts
/** Why an announced title matched (spec 2026-07-17). snake_case = Go JSON. */
export interface UpcomingReasonFE {
  kind: 'franchise' | 'taste'
  seed_anime_id?: string
  seed_anime_name?: string
  seed_anime_name_ru?: string
  user_score?: number
}

export interface UpcomingForYouItem {
  anime: SpotlightAnime
  match_score: number
  reason: UpcomingReasonFE
}

/** Login-only announcement matches — `upcoming_for_you` card. */
export interface UpcomingForYouData {
  items: UpcomingForYouItem[]
}
```
and add to the `SpotlightCard` union:
```ts
  | { type: 'upcoming_for_you'; data: UpcomingForYouData }
```

- [ ] **Step 2: tokens** — in `tokens.ts` `cardTokens` add (cyan/clock is unique — not_time_yet is violet/clock):

```ts
  upcoming_for_you:      { accent: 'cyan',   kickerKey: 'spotlight.upcomingForYou.title',      icon: 'clock'    },
```

- [ ] **Step 3: HeroSpotlightBlock** — four edits:
1. Import: `import UpcomingForYouCard from './cards/UpcomingForYouCard.vue'`
2. Dispatch branch (inside the `<transition>` chain, before the closing):
```vue
          <UpcomingForYouCard
            v-else-if="active.type === 'upcoming_for_you'"
            :key="`upcoming_for_you:${currentIndex}`"
            :data="active.data"
          />
```
3. `cardTitle()` case:
```ts
    case 'upcoming_for_you': {
      const first = card.data.items[0]
      return first
        ? getLocalizedTitle(first.anime.name, first.anime.name_ru, first.anime.name_jp)
        : t('spotlight.upcomingForYou.title')
    }
```
4. `cardImageUrls()` case (256 for every item — the card advances through items locally, so each must be prefetched at its render bucket):
```ts
    case 'upcoming_for_you':
      return (card.data.items ?? [])
        .filter((it) => it.anime.poster_url)
        .map((it) => cardPosterUrl(it.anime.poster_url!, 256))
```

- [ ] **Step 4: Type-check** (card SFC doesn't exist yet — expect ONLY the missing-module error):

Run: `cd frontend/web && bunx vue-tsc --noEmit 2>&1 | head -20`
Expected: only `Cannot find module './cards/UpcomingForYouCard.vue'` — any OTHER new error must be fixed now. Commit lands with Task 12 (the SFC) so the tree never has a broken build commit.

---

### Task 12: UpcomingForYouCard.vue + i18n + card test

**Files:**
- Create: `frontend/web/src/components/home/spotlight/cards/UpcomingForYouCard.vue`
- Test: `frontend/web/src/components/home/spotlight/cards/UpcomingForYouCard.spec.ts`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (spotlight.upcomingForYou group)

**Interfaces:**
- Consumes: `UpcomingForYouData` (Task 11), `useWatchlistStore().setStatusOptimistic(animeId, 'plan_to_watch')`, `apiClient.post('/users/recs/upcoming/dismiss', {anime_id})`, `SpotlightCardShell`/`SpotlightPoster`, `buttonVariants`.
- Produces: the rendered card with two CTAs; advances locally through `items` after add/dismiss; renders a "done" body when exhausted (single-root invariant — the shell always renders).

- [ ] **Step 1: i18n keys** — add to `en.json` inside the `spotlight` object (after `continueWatchingNew`):

```json
    "upcomingForYou": {
      "title": "Upcoming for you",
      "reasonFranchise": "New in the {name} franchise — you rated it {score}/10",
      "reasonTaste": "Matches your taste",
      "announcedBadge": "Announced",
      "addCta": "Plan to watch",
      "dismissCta": "Not interested",
      "done": "All caught up — new announcements will appear here"
    }
```
`ru.json`:
```json
    "upcomingForYou": {
      "title": "Скоро для вас",
      "reasonFranchise": "Новое во франшизе {name} — вы поставили {score}/10",
      "reasonTaste": "Совпадает с вашим вкусом",
      "announcedBadge": "Анонс",
      "addCta": "Буду смотреть",
      "dismissCta": "Не интересно",
      "done": "Пока всё — новые анонсы появятся здесь"
    }
```
`ja.json`:
```json
    "upcomingForYou": {
      "title": "あなたへの新作予告",
      "reasonFranchise": "「{name}」シリーズの新作 — あなたの評価 {score}/10",
      "reasonTaste": "あなたの好みにマッチ",
      "announcedBadge": "発表",
      "addCta": "視聴予定に追加",
      "dismissCta": "興味なし",
      "done": "以上です — 新しい発表はここに表示されます"
    }
```

- [ ] **Step 2: The SFC** — `UpcomingForYouCard.vue` (modeled on ContinueWatchingNewCard; accent cyan; CTAs are `<button>`s, not links):

```vue
<template>
  <SpotlightCardShell
    accent="cyan"
    icon="clock"
    :kicker="t('spotlight.upcomingForYou.title')"
    backdrop="poster-blur"
    :poster-url="current?.anime.poster_url || ''"
  >
    <!-- Cyan wash — content-core accent (spec 2026-07-17). -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-cyan-500/20 via-transparent to-transparent"
      />
    </template>

    <div
      v-if="current"
      class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-8 md:items-center"
    >
      <router-link :to="animeUrl" class="flex-shrink-0 self-center group">
        <SpotlightPoster
          :poster-url="current.anime.poster_url"
          :alt="title"
          width-class="w-24 md:w-40"
          glow="cyan"
          :proxy-width="256"
          img-class="group-hover:scale-105 transition-transform duration-300"
        />
      </router-link>

      <div class="flex-1 min-w-0 max-w-[600px]">
        <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>

        <div class="mt-2 flex flex-wrap items-center gap-2">
          <Badge variant="success" size="sm" overlay>
            {{ t('spotlight.upcomingForYou.announcedBadge') }}
          </Badge>
          <span
            v-if="current.anime.year"
            class="text-[13px] text-muted-foreground font-medium"
          >
            {{ current.anime.year }}
          </span>
          <span
            v-if="current.anime.kind"
            class="text-[13px] text-muted-foreground font-medium uppercase"
          >
            {{ current.anime.kind }}
          </span>
        </div>

        <p class="mt-2.5 text-[13px] leading-relaxed text-white/70 line-clamp-2" data-testid="ufy-reason">
          {{ reasonLine }}
        </p>
      </div>
    </div>

    <!-- Exhausted state — user acted on every match. -->
    <div v-else class="flex-1 min-h-0 flex items-center">
      <p class="text-[14px] text-white/60" data-testid="ufy-done">
        {{ t('spotlight.upcomingForYou.done') }}
      </p>
    </div>

    <template #cta>
      <div v-if="current" class="flex items-center gap-2">
        <button
          type="button"
          :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
          :disabled="busy"
          data-testid="ufy-add"
          @click="addToPlan"
        >
          <BookmarkPlus class="w-4 h-4" aria-hidden="true" />
          {{ t('spotlight.upcomingForYou.addCta') }}
        </button>
        <button
          type="button"
          :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
          :disabled="busy"
          data-testid="ufy-dismiss"
          @click="dismiss"
        >
          <X class="w-4 h-4" aria-hidden="true" />
          {{ t('spotlight.upcomingForYou.dismissCta') }}
        </button>
      </div>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookmarkPlus, X } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import { getLocalizedTitle } from '@/utils/title'
import { apiClient } from '@/api/client'
import { useWatchlistStore } from '@/stores/watchlist'
import type { UpcomingForYouData } from '@/types/spotlight'

const props = defineProps<{ data: UpcomingForYouData }>()
const { t, locale: i18nLocale } = useI18n()
const watchlist = useWatchlistStore()

const idx = ref(0)
const busy = ref(false)

const current = computed(() => props.data.items[idx.value] ?? null)

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const title = computed<string>(() =>
  current.value
    ? getLocalizedTitle(
        current.value.anime.name,
        current.value.anime.name_ru,
        current.value.anime.name_jp,
      )
    : '',
)

const reasonLine = computed<string>(() => {
  const c = current.value
  if (!c) return ''
  if (c.reason.kind === 'franchise' && c.reason.seed_anime_name) {
    const seed =
      locale.value === 'ru'
        ? c.reason.seed_anime_name_ru || c.reason.seed_anime_name
        : c.reason.seed_anime_name
    return t('spotlight.upcomingForYou.reasonFranchise', {
      name: seed,
      score: c.reason.user_score ?? '?',
    })
  }
  return t('spotlight.upcomingForYou.reasonTaste')
})

const animeUrl = computed<string>(() =>
  current.value ? `/anime/${current.value.anime.id}` : '/',
)

function advance(): void {
  idx.value += 1
}

async function addToPlan(): Promise<void> {
  const c = current.value
  if (!c || busy.value) return
  busy.value = true
  try {
    await watchlist.setStatusOptimistic(c.anime.id, 'plan_to_watch')
    advance()
  } catch (e) {
    // Optimistic store already rolled back; keep the item visible.
    console.warn('[spotlight] upcoming add-to-plan failed', e)
  } finally {
    busy.value = false
  }
}

async function dismiss(): Promise<void> {
  const c = current.value
  if (!c || busy.value) return
  busy.value = true
  try {
    await apiClient.post('/users/recs/upcoming/dismiss', { anime_id: c.anime.id })
    advance()
  } catch (e) {
    console.warn('[spotlight] upcoming dismiss failed', e)
  } finally {
    busy.value = false
  }
}
</script>
```

DS notes: `cyan` gradient class + `text-white/*` + `text-muted-foreground` all appear in existing cards (brand-hue exemption covers cyan); if DS-lint flags anything, mirror the exact classes of ContinueWatchingNewCard instead of allowlisting.

- [ ] **Step 3: Card test** — `UpcomingForYouCard.spec.ts` (conventions of `ContinueWatchingNewCard.spec.ts`: vue-i18n key-echo mock, RouterLinkStub; plus api/store mocks):

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}::${JSON.stringify(params)}` : key,
    locale: { value: 'en' },
  }),
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: (name?: string, nameRu?: string, nameJp?: string) =>
    name || nameRu || nameJp || '',
}))

const postMock = vi.fn().mockResolvedValue({ data: {} })
vi.mock('@/api/client', () => ({
  apiClient: { post: (...args: unknown[]) => postMock(...args) },
}))

const setStatusMock = vi.fn().mockResolvedValue(undefined)
vi.mock('@/stores/watchlist', () => ({
  useWatchlistStore: () => ({ setStatusOptimistic: setStatusMock }),
}))

import UpcomingForYouCard from './UpcomingForYouCard.vue'

function mountCard(items: unknown[]) {
  return mount(UpcomingForYouCard, {
    props: { data: { items } } as unknown as InstanceType<
      typeof UpcomingForYouCard
    >['$props'],
    global: { stubs: { 'router-link': RouterLinkStub } },
  })
}

const franchiseItem = {
  anime: { id: 'a-1', name: 'Frieren S2', poster_url: '/p.jpg', year: 2027, kind: 'tv' },
  match_score: 0.61,
  reason: {
    kind: 'franchise',
    seed_anime_id: 's-1',
    seed_anime_name: 'Frieren',
    user_score: 9,
  },
}
const tasteItem = {
  anime: { id: 'a-2', name: 'Other Show', poster_url: '/q.jpg' },
  match_score: 0.4,
  reason: { kind: 'taste' },
}

beforeEach(() => {
  postMock.mockClear()
  setStatusMock.mockClear()
})

describe('UpcomingForYouCard', () => {
  it('renders a single <article> root', () => {
    expect(mountCard([franchiseItem]).element.tagName).toBe('ARTICLE')
  })

  it('renders the franchise reason with seed name and score', () => {
    const w = mountCard([franchiseItem])
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('reasonFranchise')
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('Frieren')
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('9')
  })

  it('add-to-plan calls the watchlist store and advances', async () => {
    const w = mountCard([franchiseItem, tasteItem])
    await w.find('[data-testid="ufy-add"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(setStatusMock).toHaveBeenCalledWith('a-1', 'plan_to_watch')
    expect(w.text()).toContain('Other Show')
  })

  it('dismiss posts to the recs endpoint and advances to done state', async () => {
    const w = mountCard([franchiseItem])
    await w.find('[data-testid="ufy-dismiss"]').trigger('click')
    await new Promise((r) => setTimeout(r))
    expect(postMock).toHaveBeenCalledWith('/users/recs/upcoming/dismiss', {
      anime_id: 'a-1',
    })
    expect(w.find('[data-testid="ufy-done"]').exists()).toBe(true)
  })

  it('taste reason renders the taste key', () => {
    const w = mountCard([tasteItem])
    expect(w.find('[data-testid="ufy-reason"]').text()).toContain('reasonTaste')
  })
})
```

- [ ] **Step 4: Run FE tests**

Run: `cd frontend/web && bunx vitest run src/components/home/spotlight --reporter=basic`
Expected: PASS, including `tokens.spec.ts` (auto-covers the new union entry) and `HeroSpotlightBlock.spec.ts`. Then locale parity: `bunx vitest run src/locales --reporter=basic` — PASS.

- [ ] **Step 5: Commit (Tasks 11+12 together — the tree builds only with both)**

```bash
cd /data/animeenigma/.claude/worktrees/announcement-recs-spotlight
git add frontend/web/src/types/spotlight.ts frontend/web/src/components/home/spotlight/tokens.ts frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue frontend/web/src/components/home/spotlight/cards/UpcomingForYouCard.vue frontend/web/src/components/home/spotlight/cards/UpcomingForYouCard.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(web): upcoming_for_you spotlight card — plan-to-watch + dismiss CTAs" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- frontend/web
```

---

### Task 13: Frontend verification gate

- [ ] **Step 1:** Run `bunx vue-tsc --noEmit` in `frontend/web` — zero errors (memory: vue-tsc false-pass caveat — also run the real build).
- [ ] **Step 2:** Run `bun run build` in `frontend/web` — must succeed (DS-lint is build-enforced).
- [ ] **Step 3:** Invoke the **`/frontend-verify`** skill (DS gate + i18n parity + build traps). Fix anything it flags before proceeding. No Chrome smoke (opt-in only; small-to-medium visual change — offer to the owner at the end instead).

---

### Task 14: Docs + spec reconciliation

**Files:**
- Modify: `docs/environment-variables.md` (recs: `RECS_UPCOMING_TOPK`/`RECS_UPCOMING_MIN_S8`/`RECS_UPCOMING_MIN_S2`; scheduler: `ANNOUNCEMENTS_SYNC_CRON`)
- Modify: `CLAUDE.md` — the Spotlight recipe line says "9-card rotating carousel; a 10th touches 5 anchors": correct to the actual count (12 card types after this feature; keep the 5-anchor recipe wording)
- Modify: `docs/superpowers/specs/2026-05-03-rec-engine-design.md` — annotate §S11: implementation intentionally excludes candidates with ANY anime_list row (not just completed/dropped); superseded-by-implementation note dated 2026-07-17
- Modify: `docs/superpowers/specs/2026-07-17-announcement-recs-spotlight-design.md` — reconcile deltas: raw-score gate `MIN_S8`/`MIN_S2` replaces the combined `RECS_UPCOMING_MIN_SCORE`; sync limits are query params (`?limit&seed_backfill`) not env; `SPOTLIGHT_RECS_URL` dropped (client uses the in-cluster default like every other spotlight client); S8 ensemble weight landed at 0.13

- [ ] **Step 1:** Make the four edits above.
- [ ] **Step 2:** Commit:

```bash
git add docs/environment-variables.md CLAUDE.md docs/superpowers/specs/2026-05-03-rec-engine-design.md docs/superpowers/specs/2026-07-17-announcement-recs-spotlight-design.md
git commit -m "docs: announcement-recs env knobs, spotlight card count, S11 annotation" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- docs CLAUDE.md
```

---

## Final phase (orchestrator, NOT a subagent task)

1. **Full test sweep** in the worktree: `go test ./services/recs/... ./services/catalog/... ./services/gateway/... ./services/scheduler/...` + FE suites.
2. **Land on main**: `git fetch origin && git rebase origin/main && git push origin HEAD:main` (from the worktree branch; resolve conflicts by path, never `git add -A`).
3. **Live verify with REAL data** (after deploy): trigger `curl -X POST http://localhost:8081/api/anime/announcements-sync` once manually; then as ui_audit_bot check `GET /api/users/recs/upcoming` returns sane items and the spotlight feed includes `upcoming_for_you` for a user with franchise affinity; verify a dismiss round-trip.
4. **`/animeenigma-after-update`** — redeploys (recs, catalog, gateway, scheduler, web), health checks, Trump-mode changelog, final push. Deploy order note: recs and gateway can go in any order relative to catalog — the resolver degrades to a missing slide on 404s.
5. Offer the owner an opt-in Chrome smoke of the new card.
