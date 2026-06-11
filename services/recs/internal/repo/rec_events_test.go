package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupRecEventsTestDB builds an in-memory sqlite DB with the rec_events
// table created manually (sqlite does not support Postgres
// gen_random_uuid() in column DEFAULTs). The Insert path therefore must
// supply ID explicitly OR rely on a sqlite-side default — we do the
// former in tests by setting ID before the call (or letting GORM omit
// the column entirely; sqlite then errors on NOT NULL PK; therefore we
// drop the DEFAULT in the test fixture and assign explicit IDs).
//
// Phase 14 (REC-EVAL-01).
func setupRecEventsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE rec_events (
		id TEXT PRIMARY KEY,
		event_type TEXT NOT NULL,
		user_id TEXT,
		anime_id TEXT NOT NULL,
		signal_id TEXT NOT NULL,
		pinned INTEGER NOT NULL DEFAULT 0,
		pin_source TEXT,
		pin_seed_anime_id TEXT,
		source_route TEXT,
		rank INTEGER,
		created_at DATETIME NOT NULL,
		deleted_at DATETIME
	)`).Error)
	return db
}

// nextTestEventID returns a deterministic UUID-shaped ID for sqlite tests
// where gen_random_uuid() is unavailable.
var testEventCounter int

func nextTestEventID() string {
	testEventCounter++
	// Pad-to-12 with zeros so the format mimics a real UUID's last segment.
	return "00000000-0000-0000-0000-" + leftPad(testEventCounter)
}

func leftPad(i int) string {
	const z = "000000000000"
	s := ""
	for n := i; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	if len(s) >= 12 {
		return s
	}
	return z[:12-len(s)] + s
}

func TestRecEventsRepository_Insert_FullLoggedInPinClick(t *testing.T) {
	db := setupRecEventsTestDB(t)
	r := NewRecEventsRepository(db)

	uid := "11111111-1111-1111-1111-111111111111"
	pinSource := "shikimori_similar"
	pinSeed := "22222222-2222-2222-2222-222222222222"
	srcRoute := "/"
	rank := 1

	ev := &domain.RecEvent{
		ID:             nextTestEventID(),
		EventType:      "rec_click",
		UserID:         &uid,
		AnimeID:        "33333333-3333-3333-3333-333333333333",
		SignalID:       "s6_pin",
		Pinned:         true,
		PinSource:      &pinSource,
		PinSeedAnimeID: &pinSeed,
		SourceRoute:    &srcRoute,
		Rank:           &rank,
	}
	require.NoError(t, r.Insert(context.Background(), ev))
	assert.NotEmpty(t, ev.ID, "ID provided by caller (gen_random_uuid in production)")
	assert.False(t, ev.CreatedAt.IsZero(), "CreatedAt auto-set when zero")

	var fetched domain.RecEvent
	require.NoError(t, db.Where("anime_id = ?", ev.AnimeID).First(&fetched).Error)
	assert.Equal(t, "rec_click", fetched.EventType)
	assert.Equal(t, "s6_pin", fetched.SignalID)
	assert.True(t, fetched.Pinned)
	require.NotNil(t, fetched.UserID)
	assert.Equal(t, uid, *fetched.UserID)
	require.NotNil(t, fetched.PinSource)
	assert.Equal(t, pinSource, *fetched.PinSource)
	require.NotNil(t, fetched.PinSeedAnimeID)
	assert.Equal(t, pinSeed, *fetched.PinSeedAnimeID)
}

func TestRecEventsRepository_Insert_AnonymousClick(t *testing.T) {
	db := setupRecEventsTestDB(t)
	r := NewRecEventsRepository(db)

	ev := &domain.RecEvent{
		ID:        nextTestEventID(),
		EventType: "rec_click",
		AnimeID:   "44444444-4444-4444-4444-444444444444",
		SignalID:  "s3",
		Pinned:    false,
		// UserID nil (anonymous click on the trending row)
	}
	require.NoError(t, r.Insert(context.Background(), ev))

	var fetched domain.RecEvent
	require.NoError(t, db.Where("anime_id = ?", ev.AnimeID).First(&fetched).Error)
	assert.Nil(t, fetched.UserID, "anonymous click stored with NULL user_id")
	assert.False(t, fetched.Pinned)
	assert.Nil(t, fetched.PinSource)
	assert.Nil(t, fetched.PinSeedAnimeID)
}

func TestRecEventsRepository_Insert_RecWatchedEvent(t *testing.T) {
	db := setupRecEventsTestDB(t)
	r := NewRecEventsRepository(db)

	uid := "55555555-5555-5555-5555-555555555555"
	ev := &domain.RecEvent{
		ID:        nextTestEventID(),
		EventType: "rec_watched",
		UserID:    &uid,
		AnimeID:   "66666666-6666-6666-6666-666666666666",
		SignalID:  "s5",
		Pinned:    false,
	}
	require.NoError(t, r.Insert(context.Background(), ev))

	var fetched domain.RecEvent
	require.NoError(t, db.Where("event_type = ?", "rec_watched").First(&fetched).Error)
	assert.Equal(t, "rec_watched", fetched.EventType)
}

func TestRecEventsRepository_Insert_NonUniqueRowsAllowed(t *testing.T) {
	// The same user clicking the same card twice produces two rows. There is
	// no unique constraint on (user_id, anime_id, signal_id) — events are
	// purely additive per CONTEXT.md §C4 ("append-only").
	db := setupRecEventsTestDB(t)
	r := NewRecEventsRepository(db)

	uid := "77777777-7777-7777-7777-777777777777"
	for i := 0; i < 3; i++ {
		ev := &domain.RecEvent{
			ID:        nextTestEventID(),
			EventType: "rec_click",
			UserID:    &uid,
			AnimeID:   "88888888-8888-8888-8888-888888888888",
			SignalID:  "s3",
		}
		require.NoError(t, r.Insert(context.Background(), ev))
	}
	var count int64
	require.NoError(t, db.Model(&domain.RecEvent{}).Where("user_id = ?", uid).Count(&count).Error)
	assert.EqualValues(t, 3, count)
}

func TestRecEventsRepository_Insert_PreservesProvidedTimestamp(t *testing.T) {
	db := setupRecEventsTestDB(t)
	r := NewRecEventsRepository(db)

	custom := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	ev := &domain.RecEvent{
		ID:        nextTestEventID(),
		EventType: "rec_click",
		AnimeID:   "99999999-9999-9999-9999-999999999999",
		SignalID:  "s4",
		CreatedAt: custom,
	}
	require.NoError(t, r.Insert(context.Background(), ev))

	var fetched domain.RecEvent
	require.NoError(t, db.Where("anime_id = ?", ev.AnimeID).First(&fetched).Error)
	// sqlite truncates time precision; check second-level equality
	assert.WithinDuration(t, custom, fetched.CreatedAt, 1*time.Second)
}
