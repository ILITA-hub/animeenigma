package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/repo"
)

var errRepoNotFound = repo.ErrNotFound

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

func dailySamplePool() []domain.PoolAnime {
	return []domain.PoolAnime{
		{ID: "a", NameRU: "A"}, {ID: "b", NameRU: "B"}, {ID: "c", NameRU: "C"},
	}
}

func TestDaily_GetOrCreateToday_Deterministic(t *testing.T) {
	repo := newFakeGameRepo()
	svc := NewDailyService(repo, &fakePool{pool: dailySamplePool()}, fixedClock{"2026-06-15"}, nil, nil)

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
	pool := dailySamplePool()
	svc := NewDailyService(repo, &fakePool{pool: pool}, fixedClock{"2026-06-15"}, nil, nil)
	// mark everything except "b" as recent
	repo.recent = []string{"a", "c"}

	p, err := svc.GetOrCreateToday(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "b", p.AnimeID, "must pick the only non-recent anime")
}

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

type fakeStats struct {
	solves []string
	losses int
}

func (f *fakeStats) RecordDailyResult(_ context.Context, userID, date string, won bool, attempts int) error {
	if won {
		f.solves = append(f.solves, userID)
	} else {
		f.losses++
	}
	return nil
}

func dailySvcWithStores(date string) (*DailyService, *fakeResultStore, *fakeStats) {
	rs := newFakeResultStore()
	st := &fakeStats{}
	svc := NewDailyService(newFakeGameRepo(), &fakePool{pool: dailySamplePool()}, fixedClock{date}, rs, st)
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
	_ = p
}

func TestDaily_ResubmitAfterSolve_NoFreshSolve(t *testing.T) {
	svc, _, st := dailySvcWithStores("2026-06-15")
	ctx := context.Background()
	p, _ := svc.GetOrCreateToday(ctx)

	first, err := svc.Guess(ctx, "u1", p.AnimeID)
	require.NoError(t, err)
	assert.True(t, first.Solved)
	assert.True(t, first.FreshSolve, "first solving guess must be FreshSolve")

	again, err := svc.Guess(ctx, "u1", p.AnimeID)
	require.NoError(t, err)
	assert.True(t, again.Solved)
	assert.False(t, again.FreshSolve, "re-submitting the correct answer must NOT be a fresh solve")
	assert.Equal(t, []string{"u1"}, st.solves, "stats win recorded exactly once")
}

func TestDaily_DoubleGiveUp_RecordsLossOnce(t *testing.T) {
	svc, _, st := dailySvcWithStores("2026-06-15")
	ctx := context.Background()
	_, _ = svc.GetOrCreateToday(ctx)

	_, err := svc.GiveUp(ctx, "u1")
	require.NoError(t, err)
	_, err = svc.GiveUp(ctx, "u1")
	require.NoError(t, err)

	assert.Equal(t, 1, st.losses, "a second give-up must not re-record the loss")
}
