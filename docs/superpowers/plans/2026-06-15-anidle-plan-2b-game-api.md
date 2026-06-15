# Anidle Plan 2b — Game API (daily/endless, persistence, stats, leaderboard)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On top of the live Plan-2a skeleton (pool store + comparison engine), add the playable game: Postgres persistence, a deterministic daily puzzle, guess/give-up/resume, autocomplete search, endless rounds, per-user stats/streak, and a daily leaderboard — exposed under `/api/anidle/*` (already gateway-routed, optional JWT).

**Architecture:** Guests play statelessly (server compares; the client tracks progress) — the **no-cheat** rule means the secret never reaches the client until solved/given-up, so all comparison is server-side via the Plan-2a `Compare` engine against a frozen `answer_snapshot`. Logged-in users get server-persisted game results + stats/streak + leaderboard. Daily secret is chosen deterministically from the cached pool, excluding recent answers, and frozen in `daily_puzzle`. Endless secrets live in short-lived Redis keys keyed by a round token.

**Tech Stack:** Go 1.25, chi/v5, GORM (Postgres; sqlite in-memory for tests), `libs/{authz,cache,httputil,logger,errors}`, `redis/go-redis/v9` (sorted-set leaderboard via `cache.Client()`), testify.

**Reference spec:** `docs/superpowers/specs/2026-06-15-anidle-anime-guessing-game-design.md` §2.4, §4.1, §5.2–5.4. **Builds on Plan 2a** (`services/anidle/internal/service`: `Compare`, `PoolStore.All/Lookup/Search`, `domain.PoolAnime`).

---

## File Structure

```
services/anidle/internal/
├── domain/game.go            # DailyPuzzle, UserGameResult, UserStats models + Snapshot type
├── repo/game.go              # GORM repo (puzzle, results, stats) + sqlite-tested
├── service/daily.go          # DailyService: GetOrCreateToday, Guess, GiveUp, Resume
├── service/endless.go        # EndlessService: NewRound, Guess (redis token)
├── service/stats.go          # streak/stat aggregation
├── service/leaderboard.go    # LeaderboardService (redis sorted set)
├── service/clock.go          # Clock interface (testable "today")
├── handler/anidle.go         # all game HTTP handlers
├── transport/middleware.go   # OptionalAuthMiddleware (mirror recs)
└── transport/router.go       # (modify) mount middleware + game routes
cmd/anidle-api/main.go        # (modify) AutoMigrate + wire repo/services/handlers
```

---

## Task 1: Domain models + repo + migration

**Files:**
- Create `services/anidle/internal/domain/game.go`
- Create `services/anidle/internal/repo/game.go`
- Create `services/anidle/internal/repo/game_test.go`
- Modify `services/anidle/cmd/anidle-api/main.go` (AutoMigrate + repo construction)

- [ ] **Step 1: Domain models**

Create `services/anidle/internal/domain/game.go`:
```go
package domain

import "time"

// Snapshot is the frozen answer (the 8 attributes + display fields) stored as
// JSONB on a daily puzzle. It is a PoolAnime — reuse the same shape so Compare
// works directly.
type Snapshot = PoolAnime

// DailyPuzzle is the secret for one calendar day (UTC). Immutable once created.
type DailyPuzzle struct {
	Date           string    `gorm:"primaryKey;size:10" json:"date"` // "2006-01-02"
	AnimeID        string    `gorm:"size:64;index" json:"anime_id"`
	AnswerSnapshot Snapshot  `gorm:"serializer:json" json:"answer_snapshot"`
	CreatedAt      time.Time `json:"created_at"`
}

func (DailyPuzzle) TableName() string { return "anidle_daily_puzzle" }

// UserGameResult is one user's game for a day+mode (logged-in only).
type UserGameResult struct {
	ID         string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     string     `gorm:"size:64;index:idx_anidle_result_user_date_mode,unique,priority:1" json:"user_id"`
	PuzzleDate string     `gorm:"size:10;index:idx_anidle_result_user_date_mode,unique,priority:2" json:"puzzle_date"`
	Mode       string     `gorm:"size:16;index:idx_anidle_result_user_date_mode,unique,priority:3" json:"mode"` // "daily"
	Solved     bool       `json:"solved"`
	Attempts   int        `json:"attempts"`
	Guesses    []string   `gorm:"serializer:json" json:"guesses"` // ordered anime_ids
	SolvedAt   *time.Time `json:"solved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (UserGameResult) TableName() string { return "anidle_user_game_result" }

// UserStats is the per-user aggregate.
type UserStats struct {
	UserID            string         `gorm:"size:64;primaryKey" json:"user_id"`
	GamesPlayed       int            `json:"games_played"`
	GamesWon          int            `json:"games_won"`
	CurrentStreak     int            `json:"current_streak"`
	MaxStreak         int            `json:"max_streak"`
	GuessDistribution map[string]int `gorm:"serializer:json" json:"guess_distribution"` // attempts -> count
	LastPlayedDate    string         `gorm:"size:10" json:"last_played_date"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (UserStats) TableName() string { return "anidle_user_stats" }
```

- [ ] **Step 2: Write the failing repo test**

Create `services/anidle/internal/repo/game_test.go`:
```go
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

func newTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_daily_puzzle (
		date TEXT PRIMARY KEY, anime_id TEXT, answer_snapshot TEXT, created_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_user_game_result (
		id TEXT PRIMARY KEY, user_id TEXT, puzzle_date TEXT, mode TEXT, solved INTEGER,
		attempts INTEGER, guesses TEXT, solved_at DATETIME, created_at DATETIME, updated_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anidle_user_stats (
		user_id TEXT PRIMARY KEY, games_played INTEGER, games_won INTEGER, current_streak INTEGER,
		max_streak INTEGER, guess_distribution TEXT, last_played_date TEXT, updated_at DATETIME)`).Error)
	return db
}

func TestGameRepo_DailyPuzzleRoundTrip(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()

	_, err := r.GetDailyPuzzle(ctx, "2026-06-15")
	require.ErrorIs(t, err, ErrNotFound)

	p := &domain.DailyPuzzle{Date: "2026-06-15", AnimeID: "frieren",
		AnswerSnapshot: domain.Snapshot{ID: "frieren", NameRU: "Фрирен", Year: 2023}}
	require.NoError(t, r.CreateDailyPuzzle(ctx, p))

	got, err := r.GetDailyPuzzle(ctx, "2026-06-15")
	require.NoError(t, err)
	assert.Equal(t, "frieren", got.AnimeID)
	assert.Equal(t, 2023, got.AnswerSnapshot.Year)
}

func TestGameRepo_RecentAnswerIDs(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()
	for _, d := range []struct{ date, id string }{{"2026-06-13", "a"}, {"2026-06-14", "b"}, {"2026-06-15", "c"}} {
		require.NoError(t, r.CreateDailyPuzzle(ctx, &domain.DailyPuzzle{Date: d.date, AnimeID: d.id}))
	}
	ids, err := r.RecentAnswerIDs(ctx, 2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"c", "b"}, ids) // most recent 2 dates
}

func TestGameRepo_UserResultUpsertAndStats(t *testing.T) {
	r := NewGameRepo(newTestDB(t))
	ctx := context.Background()

	res, err := r.GetUserResult(ctx, "u1", "2026-06-15", "daily")
	require.NoError(t, err)
	assert.Nil(t, res)

	require.NoError(t, r.SaveUserResult(ctx, &domain.UserGameResult{
		UserID: "u1", PuzzleDate: "2026-06-15", Mode: "daily", Attempts: 1, Guesses: []string{"x"}}))
	require.NoError(t, r.SaveUserResult(ctx, &domain.UserGameResult{
		UserID: "u1", PuzzleDate: "2026-06-15", Mode: "daily", Attempts: 2, Guesses: []string{"x", "y"}, Solved: true}))

	res, err = r.GetUserResult(ctx, "u1", "2026-06-15", "daily")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Solved)
	assert.Equal(t, 2, res.Attempts)
	assert.Equal(t, []string{"x", "y"}, res.Guesses)

	require.NoError(t, r.SaveUserStats(ctx, &domain.UserStats{UserID: "u1", GamesPlayed: 1, CurrentStreak: 1}))
	st, err := r.GetUserStats(ctx, "u1")
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, 1, st.CurrentStreak)
}
```

- [ ] **Step 2b: Run (fails)** — `cd /data/animeenigma/services/anidle && go test ./internal/repo/ -v` → FAIL (package missing).

- [ ] **Step 3: Implement the repo**

Create `services/anidle/internal/repo/game.go`:
```go
package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("anidle: not found")

type GameRepo struct{ db *gorm.DB }

func NewGameRepo(db *gorm.DB) *GameRepo { return &GameRepo{db: db} }

func (r *GameRepo) GetDailyPuzzle(ctx context.Context, date string) (*domain.DailyPuzzle, error) {
	var p domain.DailyPuzzle
	err := r.db.WithContext(ctx).First(&p, "date = ?", date).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get daily puzzle: %w", err)
	}
	return &p, nil
}

func (r *GameRepo) CreateDailyPuzzle(ctx context.Context, p *domain.DailyPuzzle) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// RecentAnswerIDs returns the anime_ids of the most recent `days` puzzles.
func (r *GameRepo) RecentAnswerIDs(ctx context.Context, days int) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&domain.DailyPuzzle{}).
		Order("date DESC").Limit(days).Pluck("anime_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("recent answer ids: %w", err)
	}
	return ids, nil
}

func (r *GameRepo) GetUserResult(ctx context.Context, userID, date, mode string) (*domain.UserGameResult, error) {
	var res domain.UserGameResult
	err := r.db.WithContext(ctx).First(&res, "user_id = ? AND puzzle_date = ? AND mode = ?", userID, date, mode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // absence is not an error for resume
	}
	if err != nil {
		return nil, fmt.Errorf("get user result: %w", err)
	}
	return &res, nil
}

// SaveUserResult upserts on (user_id, puzzle_date, mode).
func (r *GameRepo) SaveUserResult(ctx context.Context, res *domain.UserGameResult) error {
	existing, err := r.GetUserResult(ctx, res.UserID, res.PuzzleDate, res.Mode)
	if err != nil {
		return err
	}
	if existing != nil {
		res.ID = existing.ID
		res.CreatedAt = existing.CreatedAt
		return r.db.WithContext(ctx).Save(res).Error
	}
	return r.db.WithContext(ctx).Create(res).Error
}

func (r *GameRepo) GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error) {
	var st domain.UserStats
	err := r.db.WithContext(ctx).First(&st, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user stats: %w", err)
	}
	return &st, nil
}

func (r *GameRepo) SaveUserStats(ctx context.Context, st *domain.UserStats) error {
	return r.db.WithContext(ctx).Save(st).Error
}
```

- [ ] **Step 4: Run (pass)** — `cd /data/animeenigma/services/anidle && go test ./internal/repo/ -v` → PASS.

- [ ] **Step 5: AutoMigrate + repo wiring in main.go**

In `services/anidle/cmd/anidle-api/main.go`, after `database.New` and before the router, add the AutoMigrate (replace the `// Plan 2b adds db.AutoMigrate(...)` comment) and construct the repo:
```go
	if err := db.AutoMigrate(
		&domain.DailyPuzzle{},
		&domain.UserGameResult{},
		&domain.UserStats{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	gameRepo := repo.NewGameRepo(db.DB)
	_ = gameRepo // wired into services in Task 7
```
Add imports `"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"` and `".../internal/repo"`.

- [ ] **Step 6: Build + commit**

Run: `cd /data/animeenigma/services/anidle && go build ./... && go test ./... -count=1`
Expected: PASS.
```bash
git add services/anidle/internal/domain/game.go services/anidle/internal/repo/game.go services/anidle/internal/repo/game_test.go services/anidle/cmd/anidle-api/main.go
git commit -m "feat(anidle): game persistence models + repo + automigrate"
```

---

## Task 2: Clock + DailyService.GetOrCreateToday (deterministic pick)

**Files:**
- Create `services/anidle/internal/service/clock.go`
- Create `services/anidle/internal/service/daily.go`
- Create `services/anidle/internal/service/daily_test.go`

- [ ] **Step 1: Clock**

Create `services/anidle/internal/service/clock.go`:
```go
package service

import "time"

// Clock yields the current UTC date string; injectable for tests.
type Clock interface {
	Today() string // "2006-01-02" in UTC
}

type realClock struct{}

func (realClock) Today() string { return time.Now().UTC().Format("2006-01-02") }

// fixedClock is used by tests.
type fixedClock struct{ date string }

func (f fixedClock) Today() string { return f.date }
```

- [ ] **Step 2: Write the failing test**

Create `services/anidle/internal/service/daily_test.go`:
```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// fakeGameRepo implements dailyRepo.
type fakeGameRepo struct {
	puzzles map[string]*domain.DailyPuzzle
	recent  []string
	created []*domain.DailyPuzzle
}

func newFakeGameRepo() *fakeGameRepo { return &fakeGameRepo{puzzles: map[string]*domain.DailyPuzzle{}} }

func (f *fakeGameRepo) GetDailyPuzzle(_ context.Context, date string) (*domain.DailyPuzzle, error) {
	p, ok := f.puzzles[date]
	if !ok {
		return nil, errRepoNotFound
	}
	return p, nil
}
func (f *fakeGameRepo) CreateDailyPuzzle(_ context.Context, p *domain.DailyPuzzle) error {
	f.created = append(f.created, p)
	f.puzzles[p.Date] = p
	return nil
}
func (f *fakeGameRepo) RecentAnswerIDs(_ context.Context, _ int) ([]string, error) {
	return f.recent, nil
}

// fakePool implements poolReader.
type fakePool struct{ pool []domain.PoolAnime }

func (f *fakePool) All(_ context.Context) ([]domain.PoolAnime, error) { return f.pool, nil }
func (f *fakePool) Lookup(id string) (domain.PoolAnime, bool) {
	for _, a := range f.pool {
		if a.ID == id {
			return a, true
		}
	}
	return domain.PoolAnime{}, false
}

func samplePool() []domain.PoolAnime {
	return []domain.PoolAnime{
		{ID: "a", NameRU: "A"}, {ID: "b", NameRU: "B"}, {ID: "c", NameRU: "C"},
	}
}

func TestDaily_GetOrCreateToday_Deterministic(t *testing.T) {
	repo := newFakeGameRepo()
	svc := NewDailyService(repo, &fakePool{pool: samplePool()}, fixedClock{"2026-06-15"}, nil, nil)

	p1, err := svc.GetOrCreateToday(context.Background())
	require.NoError(t, err)
	require.Len(t, repo.created, 1)

	// second call returns the SAME stored puzzle, does not create again
	p2, err := svc.GetOrCreateToday(context.Background())
	require.NoError(t, err)
	assert.Equal(t, p1.AnimeID, p2.AnimeID)
	assert.Len(t, repo.created, 1)

	// snapshot was frozen from the pool entry
	assert.Equal(t, p1.AnimeID, p1.AnswerSnapshot.ID)
}

func TestDaily_GetOrCreateToday_ExcludesRecent(t *testing.T) {
	repo := newFakeGameRepo()
	// force the deterministic index to land on a recent answer and verify it's skipped
	pool := samplePool()
	svc := NewDailyService(repo, &fakePool{pool: pool}, fixedClock{"2026-06-15"}, nil, nil)
	// mark everything except "b" as recent
	repo.recent = []string{"a", "c"}

	p, err := svc.GetOrCreateToday(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "b", p.AnimeID, "must pick the only non-recent anime")
}
```

> The test references `errRepoNotFound` — define it in the service package as the sentinel the service treats as "no puzzle yet" (it must match what the real `repo.GetDailyPuzzle` returns). The service depends on a `dailyRepo` interface whose `GetDailyPuzzle` returns `repo.ErrNotFound`; in tests the fake returns `errRepoNotFound`. To keep one sentinel, the service checks via the injected repo's error using `errors.Is(err, repo.ErrNotFound)`. So set `var errRepoNotFound = repo.ErrNotFound` in the test file (import repo).

Adjust the test header import + sentinel:
```go
import (
	// ...
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/repo"
)

var errRepoNotFound = repo.ErrNotFound
```

- [ ] **Step 2b: Run (fails)** — `go test ./internal/service/ -run TestDaily -v` → FAIL (`NewDailyService` undefined).

- [ ] **Step 3: Implement DailyService.GetOrCreateToday**

Create `services/anidle/internal/service/daily.go`:
```go
package service

import (
	"context"
	"errors"
	"hash/fnv"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/repo"
)

const recentExclusionDays = 30

type dailyRepo interface {
	GetDailyPuzzle(ctx context.Context, date string) (*domain.DailyPuzzle, error)
	CreateDailyPuzzle(ctx context.Context, p *domain.DailyPuzzle) error
	RecentAnswerIDs(ctx context.Context, days int) ([]string, error)
}

type poolReader interface {
	All(ctx context.Context) ([]domain.PoolAnime, error)
	Lookup(id string) (domain.PoolAnime, bool)
}

// resultStore + stats are used by Guess/Resume (Task 3); declared here, used there.
type DailyService struct {
	repo  dailyRepo
	pool  poolReader
	clock Clock
	rs    resultStore // nil-safe: guest-only deployments still work
	stats statsUpdater
	log   *logger.Logger
}

func NewDailyService(r dailyRepo, p poolReader, clock Clock, rs resultStore, stats statsUpdater) *DailyService {
	if clock == nil {
		clock = realClock{}
	}
	return &DailyService{repo: r, pool: p, clock: clock, rs: rs, stats: stats}
}

// GetOrCreateToday returns today's puzzle, creating it deterministically on first call.
func (s *DailyService) GetOrCreateToday(ctx context.Context) (*domain.DailyPuzzle, error) {
	date := s.clock.Today()
	if p, err := s.repo.GetDailyPuzzle(ctx, date); err == nil {
		return p, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		return nil, err
	}

	pool, err := s.pool.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, errors.New("anidle: empty pool")
	}

	recent, err := s.repo.RecentAnswerIDs(ctx, recentExclusionDays)
	if err != nil {
		return nil, err
	}
	recentSet := make(map[string]struct{}, len(recent))
	for _, id := range recent {
		recentSet[id] = struct{}{}
	}
	eligible := make([]domain.PoolAnime, 0, len(pool))
	for _, a := range pool {
		if _, bad := recentSet[a.ID]; !bad {
			eligible = append(eligible, a)
		}
	}
	if len(eligible) == 0 {
		eligible = pool // everything used recently — fall back to full pool
	}

	idx := int(hashDate(date) % uint32(len(eligible)))
	secret := eligible[idx]

	p := &domain.DailyPuzzle{Date: date, AnimeID: secret.ID, AnswerSnapshot: secret}
	if err := s.repo.CreateDailyPuzzle(ctx, p); err != nil {
		// lost a create race — re-read the winner
		if existing, gerr := s.repo.GetDailyPuzzle(ctx, date); gerr == nil {
			return existing, nil
		}
		return nil, err
	}
	return p, nil
}

func hashDate(date string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(date))
	return h.Sum32()
}
```

> Task 3 adds `resultStore` and `statsUpdater` interface definitions + the Guess/GiveUp/Resume methods to this file. For Task 2 to compile, add minimal stubs at the bottom of `daily.go`:
> ```go
> type resultStore interface{}  // expanded in Task 3
> type statsUpdater interface{} // expanded in Task 3
> ```
> Task 3 replaces these two stub lines with the real interfaces.

- [ ] **Step 4: Run (pass)** — `go test ./internal/service/ -run TestDaily -v` → PASS (both).

- [ ] **Step 5: Commit**
```bash
git add services/anidle/internal/service/clock.go services/anidle/internal/service/daily.go services/anidle/internal/service/daily_test.go
git commit -m "feat(anidle): deterministic daily puzzle selection (recent-exclusion)"
```

---

## Task 3: Daily guess / give-up / resume

**Files:**
- Modify `services/anidle/internal/service/daily.go`
- Modify `services/anidle/internal/service/daily_test.go`

Defines the guess result DTO and the logged-in persistence path. Guests get comparison only; logged-in get persistence + (on solve) a stats + leaderboard hook.

- [ ] **Step 1: Write the failing test** (append to `daily_test.go`)
```go
type fakeResultStore struct {
	results map[string]*domain.UserGameResult
}

func newFakeResultStore() *fakeResultStore {
	return &fakeResultStore{results: map[string]*domain.UserGameResult{}}
}
func key(u, d, m string) string { return u + "|" + d + "|" + m }
func (f *fakeResultStore) GetUserResult(_ context.Context, u, d, m string) (*domain.UserGameResult, error) {
	return f.results[key(u, d, m)], nil
}
func (f *fakeResultStore) SaveUserResult(_ context.Context, r *domain.UserGameResult) error {
	f.results[key(r.UserID, r.PuzzleDate, r.Mode)] = r
	return nil
}

type fakeStats struct{ solves []string }

func (f *fakeStats) RecordDailyResult(_ context.Context, userID, date string, won bool, attempts int) error {
	if won {
		f.solves = append(f.solves, userID)
	}
	return nil
}

func dailySvcWithStores(date string) (*DailyService, *fakeResultStore, *fakeStats) {
	rs := newFakeResultStore()
	st := &fakeStats{}
	svc := NewDailyService(newFakeGameRepo(), &fakePool{pool: samplePool()}, fixedClock{date}, rs, st)
	return svc, rs, st
}

func TestDaily_Guess_WrongThenRight_PersistsForLoggedIn(t *testing.T) {
	svc, rs, st := dailySvcWithStores("2026-06-15")
	ctx := context.Background()
	p, err := svc.GetOrCreateToday(ctx)
	require.NoError(t, err)
	secretID := p.AnimeID
	wrongID := "a"
	if secretID == "a" {
		wrongID = "b"
	}

	// wrong guess
	out, err := svc.Guess(ctx, "u1", wrongID)
	require.NoError(t, err)
	assert.False(t, out.Solved)
	assert.Nil(t, out.Answer)
	assert.Equal(t, 1, out.Attempt)

	// correct guess
	out, err = svc.Guess(ctx, "u1", secretID)
	require.NoError(t, err)
	assert.True(t, out.Solved)
	require.NotNil(t, out.Answer)
	assert.Equal(t, secretID, out.Answer.ID)
	assert.Equal(t, 2, out.Attempt)

	res := rs.results[key("u1", "2026-06-15", "daily")]
	require.NotNil(t, res)
	assert.True(t, res.Solved)
	assert.Equal(t, []string{wrongID, secretID}, res.Guesses)
	assert.Equal(t, []string{"u1"}, st.solves)
}

func TestDaily_Guess_Guest_NoPersistButCompares(t *testing.T) {
	svc, rs, _ := dailySvcWithStores("2026-06-15")
	ctx := context.Background()
	p, _ := svc.GetOrCreateToday(ctx)
	out, err := svc.Guess(ctx, "", p.AnimeID) // empty userID = guest
	require.NoError(t, err)
	assert.True(t, out.Solved)
	assert.Empty(t, rs.results) // nothing persisted for guests
}

func TestDaily_Guess_UnknownAnime_Errors(t *testing.T) {
	svc, _, _ := dailySvcWithStores("2026-06-15")
	_, _ = svc.GetOrCreateToday(context.Background())
	_, err := svc.Guess(context.Background(), "u1", "does-not-exist")
	require.Error(t, err)
}

func TestDaily_Resume_ReplaysStoredGuesses(t *testing.T) {
	svc, rs, _ := dailySvcWithStores("2026-06-15")
	ctx := context.Background()
	p, _ := svc.GetOrCreateToday(ctx)
	_, _ = svc.Guess(ctx, "u1", "a")
	_, _ = svc.Guess(ctx, "u1", "b")

	state, err := svc.Resume(ctx, "u1")
	require.NoError(t, err)
	assert.Len(t, state.Guesses, 2)
	assert.Equal(t, "2026-06-15", state.Date)
	// secret not leaked unless solved
	solvedNow := rs.results[key("u1", "2026-06-15", "daily")].Solved
	if !solvedNow {
		assert.Nil(t, state.Answer)
	}
}
```

- [ ] **Step 2: Replace the Task-2 stubs + add the methods** in `daily.go`.

Replace the two stub lines
```go
type resultStore interface{}
type statsUpdater interface{}
```
with:
```go
type resultStore interface {
	GetUserResult(ctx context.Context, userID, date, mode string) (*domain.UserGameResult, error)
	SaveUserResult(ctx context.Context, r *domain.UserGameResult) error
}

type statsUpdater interface {
	RecordDailyResult(ctx context.Context, userID, date string, won bool, attempts int) error
}
```

Add result types + methods (also add `"time"` to imports):
```go
const modeDaily = "daily"

// VisibleAnime is the guessed anime's public fields echoed back to the client.
type VisibleAnime struct {
	ID        string `json:"id"`
	NameRU    string `json:"name_ru"`
	NameEN    string `json:"name_en"`
	PosterURL string `json:"poster_url"`
}

// GuessOutcome is the per-guess response (no secret unless solved).
type GuessOutcome struct {
	Anime   VisibleAnime           `json:"anime"`
	Result  domain.GuessComparison `json:"result"`
	Solved  bool                   `json:"solved"`
	Attempt int                    `json:"attempt"`
	Answer  *VisibleAnime          `json:"answer,omitempty"`
}

// DailyState is the resume payload (GET /daily for logged-in).
type DailyState struct {
	Date    string         `json:"date"`
	Solved  bool           `json:"solved"`
	Guesses []GuessOutcome `json:"guesses"`
	Answer  *VisibleAnime  `json:"answer,omitempty"`
}

func visible(a domain.PoolAnime) VisibleAnime {
	return VisibleAnime{ID: a.ID, NameRU: a.NameRU, NameEN: a.NameEN, PosterURL: a.PosterURL}
}

// Guess scores one guess. userID == "" means an anonymous guest (no persistence).
func (s *DailyService) Guess(ctx context.Context, userID, animeID string) (*GuessOutcome, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	guess, ok := s.pool.Lookup(animeID)
	if !ok {
		return nil, errors.New("anidle: unknown anime")
	}
	secret := puzzle.AnswerSnapshot
	solved := animeID == puzzle.AnimeID

	out := &GuessOutcome{
		Anime:  visible(guess),
		Result: Compare(secret, guess),
		Solved: solved,
	}

	if userID == "" || s.rs == nil { // guest path: compare only
		out.Attempt = 0
		if solved {
			a := visible(secret)
			out.Answer = &a
		}
		return out, nil
	}

	res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = &domain.UserGameResult{UserID: userID, PuzzleDate: puzzle.Date, Mode: modeDaily}
	}
	if !res.Solved { // ignore extra guesses after a solve
		res.Guesses = append(res.Guesses, animeID)
		res.Attempts = len(res.Guesses)
		if solved {
			res.Solved = true
			now := timeNow()
			res.SolvedAt = &now
		}
		if err := s.rs.SaveUserResult(ctx, res); err != nil {
			return nil, err
		}
		if solved && s.stats != nil {
			if serr := s.stats.RecordDailyResult(ctx, userID, puzzle.Date, true, res.Attempts); serr != nil && s.log != nil {
				s.log.Warnw("record daily stats failed", "user", userID, "error", serr)
			}
		}
	}
	out.Attempt = res.Attempts
	if res.Solved {
		a := visible(secret)
		out.Answer = &a
	}
	return out, nil
}

// GiveUp marks the day lost for a logged-in user and reveals the answer.
func (s *DailyService) GiveUp(ctx context.Context, userID string) (*VisibleAnime, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	if userID != "" && s.rs != nil {
		res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
		if err != nil {
			return nil, err
		}
		if res == nil {
			res = &domain.UserGameResult{UserID: userID, PuzzleDate: puzzle.Date, Mode: modeDaily}
		}
		if !res.Solved {
			res.Attempts = len(res.Guesses)
			if err := s.rs.SaveUserResult(ctx, res); err != nil {
				return nil, err
			}
			if s.stats != nil {
				_ = s.stats.RecordDailyResult(ctx, userID, puzzle.Date, false, res.Attempts)
			}
		}
	}
	a := visible(puzzle.AnswerSnapshot)
	return &a, nil
}

// Resume rebuilds a logged-in user's progress for today (no secret unless solved).
func (s *DailyService) Resume(ctx context.Context, userID string) (*DailyState, error) {
	puzzle, err := s.GetOrCreateToday(ctx)
	if err != nil {
		return nil, err
	}
	state := &DailyState{Date: puzzle.Date, Guesses: []GuessOutcome{}}
	if userID == "" || s.rs == nil {
		return state, nil
	}
	res, err := s.rs.GetUserResult(ctx, userID, puzzle.Date, modeDaily)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return state, nil
	}
	state.Solved = res.Solved
	for _, gid := range res.Guesses {
		g, ok := s.pool.Lookup(gid)
		if !ok {
			continue
		}
		state.Guesses = append(state.Guesses, GuessOutcome{
			Anime:  visible(g),
			Result: Compare(puzzle.AnswerSnapshot, g),
			Solved: gid == puzzle.AnimeID,
		})
	}
	if res.Solved {
		a := visible(puzzle.AnswerSnapshot)
		state.Answer = &a
	}
	return state, nil
}
```

Add a tiny injectable now() at the bottom of `daily.go` (avoids `time.Now()` sprinkled in logic, keeps tests deterministic enough):
```go
var timeNow = func() time.Time { return time.Now().UTC() }
```

- [ ] **Step 3: Run (pass)** — `go test ./internal/service/ -run TestDaily -v` → all PASS.

- [ ] **Step 4: Commit**
```bash
git add services/anidle/internal/service/daily.go services/anidle/internal/service/daily_test.go
git commit -m "feat(anidle): daily guess/give-up/resume (no-cheat, logged-in persistence)"
```

---

## Task 4: Stats / streak service

**Files:**
- Create `services/anidle/internal/service/stats.go`
- Create `services/anidle/internal/service/stats_test.go`

Implements `statsUpdater.RecordDailyResult` + a `Get` for the stats endpoint. Streak: solving consecutive days increments; a gap or a loss resets.

- [ ] **Step 1: Write the failing test**

Create `services/anidle/internal/service/stats_test.go`:
```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

type fakeStatsStore struct{ stats map[string]*domain.UserStats }

func newFakeStatsStore() *fakeStatsStore { return &fakeStatsStore{stats: map[string]*domain.UserStats{}} }
func (f *fakeStatsStore) GetUserStats(_ context.Context, u string) (*domain.UserStats, error) {
	return f.stats[u], nil
}
func (f *fakeStatsStore) SaveUserStats(_ context.Context, st *domain.UserStats) error {
	f.stats[st.UserID] = st
	return nil
}

func TestStats_StreakIncrementsOnConsecutiveDays(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()

	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-14", true, 3))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", true, 2))

	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 2, st.GamesWon)
	assert.Equal(t, 2, st.CurrentStreak)
	assert.Equal(t, 2, st.MaxStreak)
	assert.Equal(t, 1, st.GuessDistribution["2"])
	assert.Equal(t, 1, st.GuessDistribution["3"])
}

func TestStats_StreakResetsAfterGap(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-10", true, 1))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", true, 1)) // 5-day gap
	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 1, st.CurrentStreak)
	assert.Equal(t, 1, st.MaxStreak)
}

func TestStats_LossBreaksStreak(t *testing.T) {
	store := newFakeStatsStore()
	svc := NewStatsService(store)
	ctx := context.Background()
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-14", true, 1))
	require.NoError(t, svc.RecordDailyResult(ctx, "u1", "2026-06-15", false, 0)) // gave up
	st, _ := store.GetUserStats(ctx, "u1")
	assert.Equal(t, 0, st.CurrentStreak)
	assert.Equal(t, 1, st.GamesWon)
	assert.Equal(t, 2, st.GamesPlayed)
}
```

- [ ] **Step 1b: Run (fails)** — `go test ./internal/service/ -run TestStats -v` → FAIL.

- [ ] **Step 2: Implement**

Create `services/anidle/internal/service/stats.go`:
```go
package service

import (
	"context"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

type statsStore interface {
	GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error)
	SaveUserStats(ctx context.Context, st *domain.UserStats) error
}

type StatsService struct{ store statsStore }

func NewStatsService(s statsStore) *StatsService { return &StatsService{store: s} }

func (s *StatsService) Get(ctx context.Context, userID string) (*domain.UserStats, error) {
	st, err := s.store.GetUserStats(ctx, userID)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return &domain.UserStats{UserID: userID, GuessDistribution: map[string]int{}}, nil
	}
	return st, nil
}

// RecordDailyResult updates aggregates + streak for one finished daily game.
func (s *StatsService) RecordDailyResult(ctx context.Context, userID, date string, won bool, attempts int) error {
	st, err := s.store.GetUserStats(ctx, userID)
	if err != nil {
		return err
	}
	if st == nil {
		st = &domain.UserStats{UserID: userID, GuessDistribution: map[string]int{}}
	}
	if st.GuessDistribution == nil {
		st.GuessDistribution = map[string]int{}
	}

	st.GamesPlayed++
	if won {
		st.GamesWon++
		st.GuessDistribution[strconv.Itoa(attempts)]++
		if st.LastPlayedDate != "" && isYesterday(st.LastPlayedDate, date) {
			st.CurrentStreak++
		} else {
			st.CurrentStreak = 1
		}
		if st.CurrentStreak > st.MaxStreak {
			st.MaxStreak = st.CurrentStreak
		}
	} else {
		st.CurrentStreak = 0
	}
	st.LastPlayedDate = date
	st.UpdatedAt = time.Now().UTC()
	return s.store.SaveUserStats(ctx, st)
}

// isYesterday reports whether `prev` is exactly one day before `cur` (both "2006-01-02").
func isYesterday(prev, cur string) bool {
	p, err1 := time.Parse("2006-01-02", prev)
	c, err2 := time.Parse("2006-01-02", cur)
	if err1 != nil || err2 != nil {
		return false
	}
	return c.Sub(p) == 24*time.Hour
}
```

- [ ] **Step 3: Run (pass)** — `go test ./internal/service/ -run TestStats -v` → PASS.

- [ ] **Step 4: Commit**
```bash
git add services/anidle/internal/service/stats.go services/anidle/internal/service/stats_test.go
git commit -m "feat(anidle): stats + streak aggregation"
```

---

## Task 5: Endless rounds (Redis token) + Leaderboard (Redis sorted set)

**Files:**
- Create `services/anidle/internal/service/endless.go`
- Create `services/anidle/internal/service/endless_test.go`
- Create `services/anidle/internal/service/leaderboard.go`
- Create `services/anidle/internal/service/leaderboard_test.go`

Both wrap Redis behind narrow interfaces so they're unit-testable with fakes.

- [ ] **Step 1: Endless test**

Create `services/anidle/internal/service/endless_test.go`:
```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTokenStore struct{ m map[string]string }

func newFakeTokenStore() *fakeTokenStore { return &fakeTokenStore{m: map[string]string{}} }
func (f *fakeTokenStore) PutToken(_ context.Context, token, animeID string) error {
	f.m[token] = animeID
	return nil
}
func (f *fakeTokenStore) GetToken(_ context.Context, token string) (string, bool, error) {
	v, ok := f.m[token]
	return v, ok, nil
}

func TestEndless_NewRoundThenGuess(t *testing.T) {
	ts := newFakeTokenStore()
	n := 0
	pick := func(pool []PoolAnimeRef) PoolAnimeRef { n++; return pool[n%len(pool)] }
	svc := NewEndlessService(&fakePool{pool: samplePool()}, ts, pick)
	ctx := context.Background()

	round, err := svc.NewRound(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, round.RoundToken)

	secretID, ok, _ := ts.GetToken(ctx, round.RoundToken)
	require.True(t, ok)

	out, err := svc.Guess(ctx, round.RoundToken, secretID)
	require.NoError(t, err)
	assert.True(t, out.Solved)
	require.NotNil(t, out.Answer)
}

func TestEndless_Guess_BadToken(t *testing.T) {
	svc := NewEndlessService(&fakePool{pool: samplePool()}, newFakeTokenStore(),
		func(p []PoolAnimeRef) PoolAnimeRef { return p[0] })
	_, err := svc.Guess(context.Background(), "nope", "a")
	require.Error(t, err)
}
```

- [ ] **Step 2: Implement endless**

Create `services/anidle/internal/service/endless.go`:
```go
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// PoolAnimeRef is a thin alias so the picker signature reads clearly.
type PoolAnimeRef = domain.PoolAnime

type tokenStore interface {
	PutToken(ctx context.Context, token, animeID string) error
	GetToken(ctx context.Context, token string) (animeID string, ok bool, err error)
}

// Picker chooses the secret for a new endless round (injectable for tests).
type Picker func(pool []PoolAnimeRef) PoolAnimeRef

type EndlessService struct {
	pool   poolReader
	tokens tokenStore
	pick   Picker
}

func NewEndlessService(p poolReader, ts tokenStore, pick Picker) *EndlessService {
	if pick == nil {
		pick = randomPicker
	}
	return &EndlessService{pool: p, tokens: ts, pick: pick}
}

type EndlessRound struct {
	RoundToken string `json:"round_token"`
}

func (s *EndlessService) NewRound(ctx context.Context) (*EndlessRound, error) {
	pool, err := s.pool.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, errors.New("anidle: empty pool")
	}
	secret := s.pick(pool)
	token := newToken()
	if err := s.tokens.PutToken(ctx, token, secret.ID); err != nil {
		return nil, err
	}
	return &EndlessRound{RoundToken: token}, nil
}

func (s *EndlessService) Guess(ctx context.Context, token, animeID string) (*GuessOutcome, error) {
	secretID, ok, err := s.tokens.GetToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("anidle: round expired or not found")
	}
	guess, gok := s.pool.Lookup(animeID)
	if !gok {
		return nil, errors.New("anidle: unknown anime")
	}
	secret, sok := s.pool.Lookup(secretID)
	if !sok {
		return nil, errors.New("anidle: secret missing from pool")
	}
	out := &GuessOutcome{Anime: visible(guess), Result: Compare(secret, guess), Solved: animeID == secretID}
	if out.Solved {
		a := visible(secret)
		out.Answer = &a
	}
	return out, nil
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomPicker(pool []PoolAnimeRef) PoolAnimeRef {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := (uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
	return pool[int(n%uint32(len(pool)))]
}
```

- [ ] **Step 3: Leaderboard test**

Create `services/anidle/internal/service/leaderboard_test.go`:
```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeZSet struct{ members map[string]map[string]float64 }

func newFakeZSet() *fakeZSet { return &fakeZSet{members: map[string]map[string]float64{}} }
func (f *fakeZSet) ZAdd(_ context.Context, key, member string, score float64) error {
	if f.members[key] == nil {
		f.members[key] = map[string]float64{}
	}
	f.members[key][member] = score
	return nil
}
func (f *fakeZSet) ZRangeAsc(_ context.Context, key string, n int) ([]ZEntry, error) {
	type kv struct {
		m string
		s float64
	}
	var all []kv
	for m, s := range f.members[key] {
		all = append(all, kv{m, s})
	}
	// simple insertion sort ascending by score
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].s < all[j-1].s; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := []ZEntry{}
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, ZEntry{Member: all[i].m, Score: all[i].s})
	}
	return out, nil
}

func TestLeaderboard_RanksByFewerAttemptsThenEarlier(t *testing.T) {
	z := newFakeZSet()
	lb := NewLeaderboardService(z)
	ctx := context.Background()

	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "alice", 4, 1000))
	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "bob", 2, 2000))
	require.NoError(t, lb.RecordSolve(ctx, "2026-06-15", "carol", 2, 1500))

	top, err := lb.Top(ctx, "2026-06-15", 10)
	require.NoError(t, err)
	require.Len(t, top, 3)
	// 2 attempts beats 4; among 2-attempt solvers, earlier solve (1500) beats later (2000)
	assert.Equal(t, "carol", top[0].Username)
	assert.Equal(t, "bob", top[1].Username)
	assert.Equal(t, "alice", top[2].Username)
	assert.Equal(t, 2, top[0].Attempts)
}
```

- [ ] **Step 4: Implement leaderboard**

Create `services/anidle/internal/service/leaderboard.go`:
```go
package service

import (
	"context"
	"fmt"
)

// ZEntry is one sorted-set member+score.
type ZEntry struct {
	Member string
	Score  float64
}

type zsetStore interface {
	ZAdd(ctx context.Context, key, member string, score float64) error
	ZRangeAsc(ctx context.Context, key string, n int) ([]ZEntry, error)
}

type LeaderboardService struct{ z zsetStore }

func NewLeaderboardService(z zsetStore) *LeaderboardService { return &LeaderboardService{z: z} }

// LeaderEntry is one row of the daily leaderboard.
type LeaderEntry struct {
	Username string `json:"username"`
	Attempts int    `json:"attempts"`
}

const attemptsWeight = 1e10 // attempts dominate; solve-time breaks ties

func lbKey(date string) string { return "anidle:leaderboard:" + date }

// RecordSolve adds a solver. Score packs (attempts, solveUnix) so ascending
// ZRange = fewest attempts first, earliest solve first within a tie.
func (s *LeaderboardService) RecordSolve(ctx context.Context, date, username string, attempts int, solveUnix int64) error {
	score := float64(attempts)*attemptsWeight + float64(solveUnix)
	return s.z.ZAdd(ctx, lbKey(date), username, score)
}

func (s *LeaderboardService) Top(ctx context.Context, date string, n int) ([]LeaderEntry, error) {
	entries, err := s.z.ZRangeAsc(ctx, lbKey(date), n)
	if err != nil {
		return nil, fmt.Errorf("leaderboard top: %w", err)
	}
	out := make([]LeaderEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, LeaderEntry{
			Username: e.Member,
			Attempts: int(e.Score / attemptsWeight),
		})
	}
	return out, nil
}
```

- [ ] **Step 5: Run + commit**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run 'TestEndless|TestLeaderboard' -v` → PASS.
```bash
git add services/anidle/internal/service/endless.go services/anidle/internal/service/endless_test.go services/anidle/internal/service/leaderboard.go services/anidle/internal/service/leaderboard_test.go
git commit -m "feat(anidle): endless rounds (token) + daily leaderboard (sorted set)"
```

---

## Task 6: Redis adapters (token store + zset) wired to libs/cache

**Files:**
- Create `services/anidle/internal/service/redisadapters.go`

These connect the `tokenStore`/`zsetStore` interfaces to the real Redis client (`cache.Client()`). They are thin and exercised by the deploy smoke (Task 8), not unit-tested.

- [ ] **Step 1: Implement adapters**

Create `services/anidle/internal/service/redisadapters.go`:
```go
package service

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisTokenStore implements tokenStore over go-redis (endless secrets, TTL).
type RedisTokenStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisTokenStore(client *redis.Client, ttl time.Duration) *RedisTokenStore {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &RedisTokenStore{client: client, ttl: ttl}
}

func (s *RedisTokenStore) PutToken(ctx context.Context, token, animeID string) error {
	return s.client.Set(ctx, "anidle:endless:"+token, animeID, s.ttl).Err()
}

func (s *RedisTokenStore) GetToken(ctx context.Context, token string) (string, bool, error) {
	v, err := s.client.Get(ctx, "anidle:endless:"+token).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// RedisZSet implements zsetStore over go-redis.
type RedisZSet struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisZSet(client *redis.Client, ttl time.Duration) *RedisZSet {
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	return &RedisZSet{client: client, ttl: ttl}
}

func (s *RedisZSet) ZAdd(ctx context.Context, key, member string, score float64) error {
	if err := s.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err(); err != nil {
		return err
	}
	return s.client.Expire(ctx, key, s.ttl).Err()
}

func (s *RedisZSet) ZRangeAsc(ctx context.Context, key string, n int) ([]ZEntry, error) {
	zs, err := s.client.ZRangeWithScores(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]ZEntry, 0, len(zs))
	for _, z := range zs {
		member, _ := z.Member.(string)
		out = append(out, ZEntry{Member: member, Score: z.Score})
	}
	return out, nil
}
```

- [ ] **Step 2: Build + commit**

Run: `cd /data/animeenigma/services/anidle && go build ./...` (verify go-redis Z/ZAdd/ZRangeWithScores signatures match the vendored v9; adjust if the API differs).
```bash
git add services/anidle/internal/service/redisadapters.go
git commit -m "feat(anidle): redis adapters for endless token + leaderboard zset"
```

---

## Task 7: HTTP handlers + auth middleware + router + DI

**Files:**
- Create `services/anidle/internal/transport/middleware.go`
- Create `services/anidle/internal/handler/anidle.go`
- Create `services/anidle/internal/handler/anidle_test.go`
- Modify `services/anidle/internal/transport/router.go`
- Modify `services/anidle/cmd/anidle-api/main.go`

- [ ] **Step 1: Optional-auth middleware** — mirror recs.

Create `services/anidle/internal/transport/middleware.go`:
```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// OptionalAuthMiddleware validates a Bearer token if present and stashes Claims
// in the context; missing/invalid tokens pass through as anonymous.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token != "" {
				if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
					r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

> Verify `authz.NewJWTManager`, `ValidateAccessToken`, `ContextWithClaims`, `httputil.BearerToken`, and `authz.ClaimsFromContext` (used below) against `services/recs/internal/transport/middleware.go` + `libs/authz` — mirror exactly.

- [ ] **Step 2: Handlers** — write the failing test first.

Create `services/anidle/internal/handler/anidle_test.go`:
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

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/service"
)

type fakeDaily struct {
	guessOut *service.GuessOutcome
}

func (f *fakeDaily) GetOrCreateToday(_ context.Context) (*domain.DailyPuzzle, error) {
	return &domain.DailyPuzzle{Date: "2026-06-15"}, nil
}
func (f *fakeDaily) Guess(_ context.Context, _ , _ string) (*service.GuessOutcome, error) {
	return f.guessOut, nil
}
func (f *fakeDaily) GiveUp(_ context.Context, _ string) (*service.VisibleAnime, error) {
	return &service.VisibleAnime{ID: "frieren"}, nil
}
func (f *fakeDaily) Resume(_ context.Context, _ string) (*service.DailyState, error) {
	return &service.DailyState{Date: "2026-06-15", Guesses: []service.GuessOutcome{}}, nil
}

type fakeSearch struct{}

func (fakeSearch) Search(_ context.Context, q string, _ int) []domain.PoolAnime {
	if strings.HasPrefix(q, "fr") {
		return []domain.PoolAnime{{ID: "frieren", NameRU: "Фрирен"}}
	}
	return nil
}

func TestHandler_DailyGuess(t *testing.T) {
	h := NewAnidleHandler(&fakeDaily{guessOut: &service.GuessOutcome{Solved: true, Attempt: 2}}, nil, nil, nil, fakeSearch{})
	body := strings.NewReader(`{"anime_id":"frieren"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/anidle/daily/guess", body)
	rec := httptest.NewRecorder()
	h.DailyGuess(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool                  `json:"success"`
		Data    service.GuessOutcome  `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.Data.Solved)
}

func TestHandler_Search(t *testing.T) {
	h := NewAnidleHandler(&fakeDaily{}, nil, nil, nil, fakeSearch{})
	req := httptest.NewRequest(http.MethodGet, "/api/anidle/search?q=fr", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "frieren")
}
```

- [ ] **Step 2b: Run (fails)** — `go test ./internal/handler/ -v` → FAIL.

- [ ] **Step 3: Implement the handler**

Create `services/anidle/internal/handler/anidle.go`:
```go
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/service"
)

type dailyService interface {
	GetOrCreateToday(ctx context.Context) (*domain.DailyPuzzle, error)
	Guess(ctx context.Context, userID, animeID string) (*service.GuessOutcome, error)
	GiveUp(ctx context.Context, userID string) (*service.VisibleAnime, error)
	Resume(ctx context.Context, userID string) (*service.DailyState, error)
}
type endlessService interface {
	NewRound(ctx context.Context) (*service.EndlessRound, error)
	Guess(ctx context.Context, token, animeID string) (*service.GuessOutcome, error)
}
type statsService interface {
	Get(ctx context.Context, userID string) (*domain.UserStats, error)
}
type leaderboardService interface {
	Top(ctx context.Context, date string, n int) ([]service.LeaderEntry, error)
	RecordSolve(ctx context.Context, date, username string, attempts int, solveUnix int64) error
}
type searchService interface {
	Search(ctx context.Context, q string, limit int) []domain.PoolAnime
}

type AnidleHandler struct {
	daily   dailyService
	endless endlessService
	stats   statsService
	lb      leaderboardService
	search  searchService
	log     *logger.Logger
}

func NewAnidleHandler(d dailyService, e endlessService, st statsService, lb leaderboardService, s searchService) *AnidleHandler {
	return &AnidleHandler{daily: d, endless: e, stats: st, lb: lb, search: s}
}

func userID(r *http.Request) (id, username string) {
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		return claims.UserID, claims.Username
	}
	return "", ""
}

type guessReq struct {
	AnimeID    string `json:"anime_id"`
	RoundToken string `json:"round_token"`
}

func (h *AnidleHandler) DailyMeta(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	state, err := h.daily.Resume(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, state)
}

func (h *AnidleHandler) DailyGuess(w http.ResponseWriter, r *http.Request) {
	var req guessReq
	if err := httputil.Bind(r, &req); err != nil || req.AnimeID == "" {
		httputil.BadRequest(w, "anime_id is required")
		return
	}
	uid, username := userID(r)
	out, err := h.daily.Guess(r.Context(), uid, req.AnimeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	// leaderboard hook on a fresh solve for a logged-in user
	if out.Solved && uid != "" && username != "" && h.lb != nil {
		date := time.Now().UTC().Format("2006-01-02")
		_ = h.lb.RecordSolve(r.Context(), date, username, out.Attempt, time.Now().UTC().Unix())
	}
	httputil.OK(w, out)
}

func (h *AnidleHandler) DailyGiveUp(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	ans, err := h.daily.GiveUp(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]any{"answer": ans})
}

func (h *AnidleHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	res := h.search.Search(r.Context(), q, 10)
	if res == nil {
		res = []domain.PoolAnime{}
	}
	httputil.OK(w, res)
}

func (h *AnidleHandler) EndlessNew(w http.ResponseWriter, r *http.Request) {
	round, err := h.endless.NewRound(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, round)
}

func (h *AnidleHandler) EndlessGuess(w http.ResponseWriter, r *http.Request) {
	var req guessReq
	if err := httputil.Bind(r, &req); err != nil || req.RoundToken == "" || req.AnimeID == "" {
		httputil.BadRequest(w, "round_token and anime_id are required")
		return
	}
	out, err := h.endless.Guess(r.Context(), req.RoundToken, req.AnimeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, out)
}

func (h *AnidleHandler) Stats(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	if uid == "" {
		httputil.NoContent(w)
		return
	}
	st, err := h.stats.Get(r.Context(), uid)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, st)
}

func (h *AnidleHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	top, err := h.lb.Top(r.Context(), date, 50)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, top)
}
```

> Verify `claims.Username` exists on `authz.Claims` (read `libs/authz/jwt.go`). If the field is named differently, adjust `userID()`.

- [ ] **Step 3b: Run (pass)** — `go test ./internal/handler/ -v` → PASS.

- [ ] **Step 4: Router — mount middleware + routes**

Replace `services/anidle/internal/transport/router.go`'s `NewRouter` signature to accept the game handler + JWT config, and register routes under `OptionalAuthMiddleware`:
```go
func NewRouter(healthHandler *handler.HealthHandler, anidleHandler *handler.AnidleHandler, jwtCfg authz.JWTConfig, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	if mc != nil {
		r.Use(mc.Middleware)
	}

	r.Get("/health", healthHandler.Health)
	r.Handle("/metrics", metrics.Handler())

	r.Route("/api/anidle", func(r chi.Router) {
		r.Use(OptionalAuthMiddleware(jwtCfg))
		r.Get("/daily", anidleHandler.DailyMeta)
		r.Post("/daily/guess", anidleHandler.DailyGuess)
		r.Post("/daily/giveup", anidleHandler.DailyGiveUp)
		r.Get("/search", anidleHandler.Search)
		r.Post("/endless/new", anidleHandler.EndlessNew)
		r.Post("/endless/guess", anidleHandler.EndlessGuess)
		r.Get("/stats", anidleHandler.Stats)
		r.Get("/leaderboard", anidleHandler.Leaderboard)
	})

	return r
}
```
Add the `authz` import. (The gateway forwards `/api/anidle/*` verbatim, so the service mounts the same prefix.)

- [ ] **Step 5: DI in main.go**

In `services/anidle/cmd/anidle-api/main.go`, replace `_ = gameRepo` with full wiring:
```go
	poolClient := service.NewPoolClient(cfg.CatalogURL, 60*time.Second, log)
	poolStore := service.NewPoolStore(redisCache, poolClient, cfg.PoolTTL, log)

	statsSvc := service.NewStatsService(gameRepo)
	lbSvc := service.NewLeaderboardService(service.NewRedisZSet(redisCache.Client(), 48*time.Hour))
	dailySvc := service.NewDailyService(gameRepo, poolStore, nil /*realClock*/, gameRepo, statsSvc)
	endlessSvc := service.NewEndlessService(poolStore, service.NewRedisTokenStore(redisCache.Client(), time.Hour), nil)

	healthHandler := handler.NewHealthHandler()
	anidleHandler := handler.NewAnidleHandler(dailySvc, endlessSvc, statsSvc, lbSvc, poolStore)

	mc := metrics.NewCollector("anidle")
	router := transport.NewRouter(healthHandler, anidleHandler, cfg.JWT, log, mc)
```
Notes:
- `gameRepo` satisfies BOTH `dailyRepo` (puzzle methods) AND `resultStore` (GetUserResult/SaveUserResult) AND `statsStore` (GetUserStats/SaveUserStats) — pass it for each.
- `poolStore` satisfies `poolReader` (All/Lookup) AND `searchService` (Search).
- Add imports for `service`, `handler`, `transport`, `time` if not present.
- Verify `redisCache.Client()` returns `*redis.Client` (it does per libs/cache).

- [ ] **Step 6: Build + full test + commit**

Run: `cd /data/animeenigma/services/anidle && go build ./... && go test ./... -count=1`
Expected: PASS.
```bash
git add services/anidle/internal/handler/anidle.go services/anidle/internal/handler/anidle_test.go services/anidle/internal/transport/middleware.go services/anidle/internal/transport/router.go services/anidle/cmd/anidle-api/main.go
git commit -m "feat(anidle): game HTTP API (daily/endless/stats/leaderboard/search) + routes + DI"
```

---

## Task 8: Deploy + end-to-end smoke (through the gateway)

- [ ] **Step 1: Redeploy**

Run: `make redeploy-anidle && make redeploy-gateway`
Expected: both healthy.

- [ ] **Step 2: Smoke the daily flow through the gateway (anonymous)**

```bash
# meta (no secret)
curl -s http://localhost:8000/api/anidle/daily | head -c 300; echo
# search autocomplete
curl -s "http://localhost:8000/api/anidle/search?q=%D1%84" | head -c 300; echo   # q=ф
# a guess (use an id from the search result)
AID=$(curl -s "http://localhost:8000/api/anidle/search?q=%D1%84" | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(d[0]['id'] if d else '')")
curl -s -X POST http://localhost:8000/api/anidle/daily/guess -H 'Content-Type: application/json' -d "{\"anime_id\":\"$AID\"}" | head -c 500; echo
```
Expected: `/daily` returns `{"success":true,"data":{"date":"…","guesses":[]}}` (no `answer` field). `/search` returns matching pool entries. `/daily/guess` returns `{"success":true,"data":{"anime":{…},"result":{"genres":{"status":…},…},"solved":false|true,…}}`.

- [ ] **Step 3: Smoke endless**

```bash
TOK=$(curl -s -X POST http://localhost:8000/api/anidle/endless/new | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['round_token'])")
curl -s -X POST http://localhost:8000/api/anidle/endless/guess -H 'Content-Type: application/json' -d "{\"round_token\":\"$TOK\",\"anime_id\":\"$AID\"}" | head -c 400; echo
```
Expected: a `round_token`, then a guess returns a comparison `result`.

- [ ] **Step 4: Confirm the secret never leaks**

`curl -s http://localhost:8000/api/anidle/daily` must NOT contain an `answer` with a populated anime until a correct guess/give-up. Eyeball the Step 2 `/daily` output: no `answer`.

- [ ] **Step 5: No commit** (deploy only).

---

## Self-Review

**Spec coverage:**
- ✅ §4.1 tables — `anidle_daily_puzzle` / `anidle_user_game_result` (unique user+date+mode) / `anidle_user_stats` (Task 1).
- ✅ §5.4 daily determinism — fnv(date) mod eligible, recent-30 exclusion, immutable row, race re-read (Task 2).
- ✅ §2.4 / §5.3 guess + resume + no-cheat — server-side Compare vs frozen snapshot; secret only on solve/giveup; guests stateless; logged-in persisted (Task 3).
- ✅ §2.4 streak — consecutive-day increment, gap/loss reset (Task 4).
- ✅ §5.2 endless (Redis token), leaderboard (sorted set, attempts-then-time) — Tasks 5–6.
- ✅ §5.2 endpoints (daily/guess/giveup/search/endless/stats/leaderboard) + optional-JWT routes (Task 7).

**Placeholder scan:** The Task-2 `resultStore`/`statsUpdater` empty-interface stubs are explicitly replaced in Task 3 Step 2 — flagged, not a leftover. No other placeholders; every code step is complete.

**Type consistency:** `domain.PoolAnime`/`Snapshot`, `GuessComparison`, `GuessOutcome`, `VisibleAnime`, `DailyState`, `EndlessRound`, `LeaderEntry`, `ZEntry` are defined once and reused. `gameRepo` implements `dailyRepo`+`resultStore`+`statsStore`; `poolStore` implements `poolReader`+`searchService`; the handler interfaces (`dailyService`, etc.) match the concrete service method sets. `RecordDailyResult(ctx,userID,date,won,attempts)` signature matches between `statsUpdater` (daily.go), `StatsService`, and the `fakeStats` test.

**Known verify-points (checked during execution):** `authz.Claims.Username` field name; `authz.NewJWTManager/ValidateAccessToken/ContextWithClaims/ClaimsFromContext` (mirror recs); `cache.RedisCache.Client() *redis.Client`; go-redis v9 `ZAdd(redis.Z{})`/`ZRangeWithScores` signatures; `metrics.Collector.Middleware`/`metrics.Handler()`; GORM `serializer:json` tag support (used for snapshot/guesses/distribution — confirm the GORM version supports it, else use a `[]byte`+manual-marshal column). Each is validated by a build/test/smoke step.
```
