package cards

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// fakeNowWatchingDB implements nowWatchingDB without a real *gorm.DB.
type fakeNowWatchingDB struct {
	rows  []nowWatchingRow
	err   error
	calls int32
	mu    sync.Mutex
	last  string
}

func (f *fakeNowWatchingDB) RawScan(_ context.Context, dest any, sql string, _ ...any) error {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.last = sql
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	// dest must be *[]nowWatchingRow
	out, ok := dest.(*[]nowWatchingRow)
	if !ok {
		return errors.New("dest must be *[]nowWatchingRow")
	}
	*out = append((*out)[:0], f.rows...)
	return nil
}

func makeNWRows(n int) []nowWatchingRow {
	out := make([]nowWatchingRow, n)
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		out[i] = nowWatchingRow{
			Username:      "user" + id,
			PublicID:      "user-" + id,
			AnimeID:       "anime-" + id,
			AnimeName:     "Anime " + id,
			AnimeNameRU:   "Аниме " + id,
			PosterURL:     "/poster-" + id + ".jpg",
			EpisodeNumber: i + 1,
			UpdatedAt:     "2026-05-21T12:00:0" + id + "Z",
		}
	}
	return out
}

func TestNowWatching_Type(t *testing.T) {
	r := &NowWatchingResolver{}
	if got := r.Type(); got != "now_watching" {
		t.Errorf("Type() = %q; want now_watching", got)
	}
}

func TestNowWatching_Three_ReturnsThree(t *testing.T) {
	db := &fakeNowWatchingDB{rows: makeNWRows(3)}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.NowWatchingData)
	if len(data.Sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(data.Sessions))
	}
}

func TestNowWatching_Five_AdaptiveSliceToThree(t *testing.T) {
	db := &fakeNowWatchingDB{rows: makeNWRows(5)}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.NowWatchingData)
	if len(data.Sessions) != 3 {
		t.Errorf("AdaptiveSlice(N=5) should return 3 sessions, got %d", len(data.Sessions))
	}
}

func TestNowWatching_Two_ReturnsRandomOne(t *testing.T) {
	db := &fakeNowWatchingDB{rows: makeNWRows(2)}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(99), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.NowWatchingData)
	if len(data.Sessions) != 1 {
		t.Errorf("AdaptiveSlice(N=2) should return 1 session, got %d", len(data.Sessions))
	}
}

func TestNowWatching_Empty_ReturnsNilNil(t *testing.T) {
	db := &fakeNowWatchingDB{rows: nil}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card on empty, got %+v", card)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set on empty path, got %d", c.sets)
	}
}

func TestNowWatching_NoPrivateFieldsLeaked(t *testing.T) {
	db := &fakeNowWatchingDB{rows: []nowWatchingRow{
		{
			Username:      "user1",
			PublicID:      "user-1",
			AnimeID:       "anime-1",
			AnimeName:     "A1",
			AnimeNameRU:   "А1",
			PosterURL:     "/p.jpg",
			EpisodeNumber: 5,
			UpdatedAt:     "2026-05-21T12:00:00Z",
		},
	}}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	raw, _ := json.Marshal(card)
	s := string(raw)
	// Allowed public fields:
	if !strings.Contains(s, `"username":"user1"`) {
		t.Errorf("expected username in payload: %s", s)
	}
	if !strings.Contains(s, `"public_id":"user-1"`) {
		t.Errorf("expected public_id in payload: %s", s)
	}
	// Forbidden private fields (HSB-NF-04 privacy gate):
	for _, forbidden := range []string{"email", "password", "api_key", `"user_id"`} {
		if strings.Contains(s, forbidden) {
			t.Errorf("private field %q leaked into now_watching payload: %s", forbidden, s)
		}
	}
}

func TestNowWatching_CacheKey_NotDateKeyed(t *testing.T) {
	db := &fakeNowWatchingDB{rows: makeNWRows(1)}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	if _, err := r.Resolve(context.Background(), nil); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	keys := c.keys()
	if !reflect.DeepEqual(keys, []string{"spotlight:now_watching"}) {
		t.Errorf("expected exactly [\"spotlight:now_watching\"], got %v", keys)
	}
}

func TestNowWatching_DBError_Wraps(t *testing.T) {
	db := &fakeNowWatchingDB{err: errors.New("pg gone")}
	c := newFakeCache()
	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if card != nil {
		t.Errorf("expected nil card on error, got %+v", card)
	}
	if !strings.Contains(err.Error(), "now_watching") {
		t.Errorf("expected wrapped error to mention now_watching, got %v", err)
	}
}

func TestNowWatching_CacheHit_DoesNotCallDB(t *testing.T) {
	db := &fakeNowWatchingDB{rows: makeNWRows(3)}
	c := newFakeCache()

	seeded := spotlight.NowWatchingData{Sessions: []spotlight.NowWatchingSession{
		{Username: "cached", PublicID: "cached-1", AnimeID: "x", EpisodeNumber: 1, UpdatedAt: "2026-05-21T12:00:00Z"},
	}}
	data, _ := json.Marshal(seeded)
	c.store["spotlight:now_watching"] = data

	r := NewNowWatchingResolver(db, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	if db.calls != 0 {
		t.Errorf("expected 0 db calls on cache hit, got %d", db.calls)
	}
	out := card.Data.(spotlight.NowWatchingData)
	if len(out.Sessions) != 1 || out.Sessions[0].Username != "cached" {
		t.Errorf("expected cached payload, got: %+v", out)
	}
}
