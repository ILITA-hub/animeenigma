package cards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

type fakeDailyReviewDB struct {
	rows     []dailyReviewRow
	err      error
	calls    int32
	lastSQL  string
	lastArgs []any
}

func (f *fakeDailyReviewDB) RawScan(_ context.Context, dest any, sql string, args ...any) error {
	atomic.AddInt32(&f.calls, 1)
	f.lastSQL = sql
	f.lastArgs = append([]any(nil), args...)
	if f.err != nil {
		return f.err
	}
	out, ok := dest.(*[]dailyReviewRow)
	if !ok {
		return errors.New("dest must be *[]dailyReviewRow")
	}
	*out = append((*out)[:0], f.rows...)
	return nil
}

func reviewRow() dailyReviewRow {
	return dailyReviewRow{
		ReviewID:    "review-1",
		Score:       9,
		ReviewText:  strings.Repeat("x", dailyReviewMinChars),
		CreatedAt:   "2026-07-20T12:00:00Z",
		Username:    "alice",
		PublicID:    "alice-public",
		Avatar:      "/avatar.png",
		AnimeID:     "anime-1",
		AnimeName:   "The Anime",
		AnimeNameRU: "Аниме",
		AnimeNameJP: "アニメ",
		PosterURL:   "/poster.jpg",
	}
}

func TestDailyReviewResolver_Type(t *testing.T) {
	if got := (&DailyReviewResolver{}).Type(); got != "daily_review" {
		t.Fatalf("Type() = %q, want daily_review", got)
	}
}

func TestDailyReviewMinimumLengthBoundaryCountsUnicodeCharacters(t *testing.T) {
	if dailyReviewMeetsMinimum(strings.Repeat("界", dailyReviewMinChars-1)) {
		t.Fatal("99-character review must not be eligible")
	}
	if !dailyReviewMeetsMinimum(strings.Repeat("界", dailyReviewMinChars)) {
		t.Fatal("100-character review must be eligible")
	}
}

func TestDailyReviewResolver_CacheMissReturnsPublicReviewAndCaches(t *testing.T) {
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{reviewRow()}}
	c := newFakeCache()
	r := NewDailyReviewResolver(db, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	if card.Type != "daily_review" {
		t.Fatalf("card.Type = %q", card.Type)
	}
	data, ok := card.Data.(spotlight.DailyReviewData)
	if !ok {
		t.Fatalf("card.Data = %T, want DailyReviewData", card.Data)
	}
	if len(data.ReviewText) != dailyReviewMinChars || data.Score != 9 {
		t.Fatalf("review fields lost: %+v", data)
	}
	if data.Author.Username != "alice" || data.Author.PublicID != "alice-public" {
		t.Fatalf("author fields lost: %+v", data.Author)
	}
	if data.Anime.ID != "anime-1" || data.Anime.PosterURL != "/poster.jpg" {
		t.Fatalf("anime fields lost: %+v", data.Anime)
	}
	if c.sets != 1 {
		t.Fatalf("cache.Set calls = %d, want 1", c.sets)
	}
	keys := c.keys()
	if len(keys) != 1 || !strings.HasPrefix(keys[0], dailyReviewCachePrefix) {
		t.Fatalf("cache keys = %v", keys)
	}
}

func TestDailyReviewResolver_QueryRequiresAtLeast100CharactersAndUsesDailySeed(t *testing.T) {
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{reviewRow()}}
	r := NewDailyReviewResolver(db, newFakeCache(), testLogger())

	if _, err := r.Resolve(context.Background(), nil); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, required := range []string{
		"CHAR_LENGTH(BTRIM(al.review_text)) >= ?",
		"COALESCE(a.hidden, false) = false",
		"ORDER BY md5(al.id::text || ?)",
		"LIMIT 1",
	} {
		if !strings.Contains(db.lastSQL, required) {
			t.Errorf("query missing %q: %s", required, db.lastSQL)
		}
	}
	if len(db.lastArgs) != 2 {
		t.Fatalf("query args = %v, want minimum length and date key", db.lastArgs)
	}
	minChars, ok := db.lastArgs[0].(int)
	if !ok || minChars != 100 {
		t.Fatalf("minimum length = %#v, want 100", db.lastArgs[0])
	}
	dateKey, ok := db.lastArgs[1].(string)
	if !ok || len(dateKey) != len("2026-07-21") {
		t.Fatalf("date seed = %#v, want YYYY-MM-DD", db.lastArgs[1])
	}
}

func TestDailyReviewResolver_DoesNotLeakPrivateUserID(t *testing.T) {
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{reviewRow()}}
	card, err := NewDailyReviewResolver(db, newFakeCache(), testLogger()).Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), `"user_id"`) {
		t.Fatalf("private user_id leaked: %s", raw)
	}
	if !strings.Contains(string(raw), `"public_id":"alice-public"`) {
		t.Fatalf("public_id missing: %s", raw)
	}
}

func TestDailyReviewResolver_EmptyPoolReturnsNilWithoutCaching(t *testing.T) {
	db := &fakeDailyReviewDB{}
	c := newFakeCache()
	card, err := NewDailyReviewResolver(db, c, testLogger()).Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("Resolve empty: card=%v err=%v", card, err)
	}
	if c.sets != 0 {
		t.Fatalf("cache.Set calls = %d, want 0", c.sets)
	}
}

func TestDailyReviewResolver_RejectsShortAdapterRowWithoutCaching(t *testing.T) {
	row := reviewRow()
	row.ReviewText = strings.Repeat("x", dailyReviewMinChars-1)
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{row}}
	c := newFakeCache()
	card, err := NewDailyReviewResolver(db, c, testLogger()).Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("Resolve short review: card=%v err=%v", card, err)
	}
	if c.sets != 0 {
		t.Fatalf("cache.Set calls = %d, want 0", c.sets)
	}
}

func TestDailyReviewResolver_CacheHitSkipsDB(t *testing.T) {
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{reviewRow()}}
	c := newFakeCache()
	key := dailyReviewCachePrefix + spotlight.DateKeyUTC(testNow())
	seeded := spotlight.DailyReviewData{
		ReviewID:   "cached",
		ReviewText: strings.Repeat("x", dailyReviewMinChars),
	}
	raw, _ := json.Marshal(seeded)
	c.store[key] = raw

	card, err := NewDailyReviewResolver(db, c, testLogger()).Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve cache hit: card=%v err=%v", card, err)
	}
	if atomic.LoadInt32(&db.calls) != 0 {
		t.Fatalf("DB calls = %d, want 0", db.calls)
	}
	if got := card.Data.(spotlight.DailyReviewData).ReviewID; got != "cached" {
		t.Fatalf("ReviewID = %q, want cached", got)
	}
}

func TestDailyReviewResolver_ShortCacheEntryFallsBackToDB(t *testing.T) {
	db := &fakeDailyReviewDB{rows: []dailyReviewRow{reviewRow()}}
	c := newFakeCache()
	key := dailyReviewCachePrefix + spotlight.DateKeyUTC(testNow())
	seeded := spotlight.DailyReviewData{ReviewID: "short", ReviewText: "too short"}
	raw, _ := json.Marshal(seeded)
	c.store[key] = raw

	card, err := NewDailyReviewResolver(db, c, testLogger()).Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve short cache entry: card=%v err=%v", card, err)
	}
	if atomic.LoadInt32(&db.calls) != 1 {
		t.Fatalf("DB calls = %d, want 1", db.calls)
	}
	if got := card.Data.(spotlight.DailyReviewData).ReviewID; got != "review-1" {
		t.Fatalf("ReviewID = %q, want DB review", got)
	}
}

func TestDailyReviewResolver_DBErrorIsWrapped(t *testing.T) {
	db := &fakeDailyReviewDB{err: errors.New("postgres down")}
	card, err := NewDailyReviewResolver(db, newFakeCache(), testLogger()).Resolve(context.Background(), nil)
	if err == nil || card != nil {
		t.Fatalf("Resolve DB error: card=%v err=%v", card, err)
	}
	if !strings.Contains(err.Error(), "daily_review") {
		t.Fatalf("error not wrapped: %v", err)
	}
}

// testNow is deliberately evaluated at call time so the cache fixture uses
// the same UTC date as Resolve even when the suite runs around midnight.
func testNow() time.Time { return time.Now() }
