package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeRecsRepoForList records UpdateS6Seed calls so the synchronous-seed
// test cases can assert on the count + arguments. Implements the narrow
// recsRepoForListService interface defined alongside ListService.
type fakeRecsRepoForList struct {
	mu          sync.Mutex
	callCount   int
	lastUserID  string
	lastAnimeID string
	lastAt      time.Time
	lastScore   int
	returnErr   error
}

func (f *fakeRecsRepoForList) UpdateS6Seed(_ context.Context, userID, animeID string, completedAt time.Time, score int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.lastUserID = userID
	f.lastAnimeID = animeID
	f.lastAt = completedAt
	f.lastScore = score
	return f.returnErr
}

func (f *fakeRecsRepoForList) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// fakeListCache records Delete calls. Implements listServiceCache.
type fakeListCache struct {
	mu       sync.Mutex
	deletes  []string
	returnErr error
}

func (f *fakeListCache) Delete(_ context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, keys...)
	return f.returnErr
}

func (f *fakeListCache) DeletedKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.deletes))
	copy(out, f.deletes)
	return out
}

// setupListServiceWithRecsTestDB builds the same fixture as
// setupListServiceTestDB but constructs the service with a fake recsRepo
// + fake cache so the Phase-13 seed-update path can be exercised.
func setupListServiceWithRecsTestDB(t *testing.T, recsRepo recsRepoForListService, cache listServiceCache) (*ListService, *gorm.DB) {
	_, db := setupListServiceTestDB(t)
	listRepo := repo.NewListRepository(db)
	activityRepo := repo.NewActivityRepository(db)
	prefRepo := repo.NewPreferenceRepository(db)
	progressRepo := repo.NewProgressRepository(db)
	log, err := logger.New(logger.Config{Level: "error", Development: false})
	require.NoError(t, err)
	svc := NewListService(listRepo, activityRepo, prefRepo, progressRepo, nil, recsRepo, cache, log)
	return svc, db
}

// seedListEntry inserts an anime_list row matching production shape.
func seedListEntry(t *testing.T, db *gorm.DB, id, userID, animeID, status string, score, episodes int, completedAt *time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, episodes_count, deleted_at) VALUES (?, ?, ?, NULL)`,
		animeID, "Test Anime", 24).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_list (id, user_id, anime_id, status, score, episodes, completed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, animeID, status, score, episodes, completedAt, time.Now(), time.Now()).Error)
}

func TestMarkEpisodeWatched_SeedUpdate_FiresWhenStatusCompletedAndScoreSeven(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	cache := &fakeListCache{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, cache)
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-1", "u1", "anime-1", "completed", 8, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u1", "anime-1", req)
	require.NoError(t, err)

	assert.Equal(t, 1, fakeRecs.Calls(), "qualifying completion (status='completed', score=8) must invoke UpdateS6Seed")
	assert.Equal(t, "u1", fakeRecs.lastUserID)
	assert.Equal(t, "anime-1", fakeRecs.lastAnimeID)
	assert.Equal(t, 8, fakeRecs.lastScore)
}

func TestMarkEpisodeWatched_SeedUpdate_SkipsWhenScoreBelow7(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, &fakeListCache{})
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-2", "u2", "anime-2", "completed", 6, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u2", "anime-2", req)
	require.NoError(t, err)

	assert.Equal(t, 0, fakeRecs.Calls(), "score=6 must NOT trigger seed update")
}

func TestMarkEpisodeWatched_SeedUpdate_SkipsWhenStatusNotCompleted(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, &fakeListCache{})
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-3", "u3", "anime-3", "watching", 8, 5, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 6, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u3", "anime-3", req)
	require.NoError(t, err)

	assert.Equal(t, 0, fakeRecs.Calls(), "status='watching' must NOT trigger seed update even with score>=7")
}

func TestMarkEpisodeWatched_SeedUpdate_SkipsWhenScoreZero(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, &fakeListCache{})
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-4", "u4", "anime-4", "completed", 0, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u4", "anime-4", req)
	require.NoError(t, err)

	assert.Equal(t, 0, fakeRecs.Calls(), "score=0 (unset) must NOT trigger seed update")
}

func TestMarkEpisodeWatched_CacheBustOnSeedUpdate(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	cache := &fakeListCache{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, cache)
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-5", "u5", "anime-5", "completed", 9, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u5", "anime-5", req)
	require.NoError(t, err)

	// Cache invalidation is fire-and-forget; allow the goroutine a moment.
	assert.Eventually(t, func() bool {
		for _, k := range cache.DeletedKeys() {
			if k == "recs:user:u5:topN" {
				return true
			}
		}
		return false
	}, 200*time.Millisecond, 10*time.Millisecond, "cache.Delete must be invoked with the user's topN key after a successful seed update")
}

func TestMarkEpisodeWatched_SeedUpdateFailureDoesNotFailRequest(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{returnErr: assertSentinelErr}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, &fakeListCache{})
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-6", "u6", "anime-6", "completed", 8, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	entry, err := svc.MarkEpisodeWatched(ctx, "u6", "anime-6", req)
	require.NoError(t, err, "UpdateS6Seed failure must NOT fail the request — same contract as CreateWatchHistory")
	require.NotNil(t, entry)
}

func TestMarkEpisodeWatched_NilRecsRepo_SkipsCleanly(t *testing.T) {
	svc, db := setupListServiceWithRecsTestDB(t, nil, &fakeListCache{})
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-7", "u7", "anime-7", "completed", 8, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u7", "anime-7", req)
	require.NoError(t, err, "nil recsRepo must not panic")
}

func TestMarkEpisodeWatched_NilCache_SkipsCleanly(t *testing.T) {
	fakeRecs := &fakeRecsRepoForList{}
	svc, db := setupListServiceWithRecsTestDB(t, fakeRecs, nil)
	ctx := context.Background()

	completed := time.Now().UTC().Add(-1 * time.Hour)
	seedListEntry(t, db, "al-8", "u8", "anime-8", "completed", 8, 12, &completed)

	req := &domain.MarkEpisodeWatchedRequest{Episode: 12, Player: "kodik", Language: "ru", WatchType: "dub"}
	_, err := svc.MarkEpisodeWatched(ctx, "u8", "anime-8", req)
	require.NoError(t, err, "nil cache must not panic — seed update still ran")
	assert.Equal(t, 1, fakeRecs.Calls(), "seed update still fires when cache is nil")
}

// assertSentinelErr is a deliberately-non-nil error used to drive the
// failure-doesn't-fail-request test branch.
var assertSentinelErr = sentinelError("seed-update-failed")

type sentinelError string

func (s sentinelError) Error() string { return string(s) }
