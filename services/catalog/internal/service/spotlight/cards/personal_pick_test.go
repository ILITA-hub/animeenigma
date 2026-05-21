package cards

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// --- fakes --------------------------------------------------------------

type fakeTrending struct {
	items []*domain.Anime
	total int64
	err   error
	calls int32
	mu    sync.Mutex
	last  struct {
		page, pageSize int
	}
}

func (f *fakeTrending) GetTrendingAnime(_ context.Context, page, pageSize int) ([]*domain.Anime, int64, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.last.page = page
	f.last.pageSize = pageSize
	f.mu.Unlock()
	return f.items, f.total, f.err
}

type fakePlayerRecs struct {
	recs        []client.UserRec
	err         error
	calls       int32
	capturedJWT string
	mu          sync.Mutex
}

func (f *fakePlayerRecs) FetchUserRecs(_ context.Context, jwt string) ([]client.UserRec, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.capturedJWT = jwt
	f.mu.Unlock()
	return f.recs, f.err
}

// helper: build a domain.Anime as a json.RawMessage for the recs envelope.
func recFromAnime(a domain.Anime, score float64) client.UserRec {
	raw, _ := json.Marshal(a)
	return client.UserRec{Anime: raw, Score: score}
}

// seededRng returns a deterministic *rand.Rand for tests.
func seededRng(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

// --- tests --------------------------------------------------------------

func TestPersonalPick_Type(t *testing.T) {
	r := &PersonalPickResolver{}
	if got := r.Type(); got != "personal_pick" {
		t.Errorf("Type() = %q, want personal_pick", got)
	}
}

func TestPersonalPick_AnonHappy_PicksThreeFromTen(t *testing.T) {
	tr := &fakeTrending{items: makeAnimes(10), total: 10}
	recs := &fakePlayerRecs{}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, recs, c, seededRng(42), testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data, ok := card.Data.(spotlight.PersonalPickData)
	if !ok {
		t.Fatalf("Card.Data not PersonalPickData: %T", card.Data)
	}
	if len(data.Items) != 3 {
		t.Errorf("expected 3 items (AdaptiveSlice of N=10 == 3), got %d", len(data.Items))
	}
	if data.Source != "trending" {
		t.Errorf("Source = %q; want trending", data.Source)
	}
	if recs.calls != 0 {
		t.Errorf("expected 0 player rec calls in anon path, got %d", recs.calls)
	}
}

func TestPersonalPick_AnonEmpty_ReturnsNilNil(t *testing.T) {
	tr := &fakeTrending{items: nil}
	recs := &fakePlayerRecs{}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, recs, c, seededRng(1), testLogger())

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

func TestPersonalPick_AnonOne_ReturnsOne(t *testing.T) {
	tr := &fakeTrending{items: makeAnimes(1)}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, &fakePlayerRecs{}, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.PersonalPickData)
	if len(data.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(data.Items))
	}
}

func TestPersonalPick_AnonTwo_ReturnsRandomOne(t *testing.T) {
	tr := &fakeTrending{items: makeAnimes(2)}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, &fakePlayerRecs{}, c, seededRng(99), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.PersonalPickData)
	if len(data.Items) != 1 {
		t.Errorf("AdaptiveSlice(N=2) should return 1 item, got %d", len(data.Items))
	}
}

func TestPersonalPick_LoginHappy_CallsFetchUserRecs_WithJWT(t *testing.T) {
	tr := &fakeTrending{}
	animes := makeAnimes(5)
	recList := make([]client.UserRec, 0, 5)
	for _, a := range animes {
		recList = append(recList, recFromAnime(*a, 0.9))
	}
	recs := &fakePlayerRecs{recs: recList}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, recs, c, seededRng(42), testLogger())

	uid := "user1"
	ctx := ContextWithJWT(context.Background(), "testjwt")
	card, err := r.Resolve(ctx, &uid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if recs.capturedJWT != "testjwt" {
		t.Errorf("capturedJWT = %q; want testjwt", recs.capturedJWT)
	}
	data := card.Data.(spotlight.PersonalPickData)
	if data.Source != "personal" {
		t.Errorf("Source = %q; want personal", data.Source)
	}
	if len(data.Items) != 3 {
		t.Errorf("AdaptiveSlice(N=5) should return 3 items, got %d", len(data.Items))
	}
	if tr.calls != 0 {
		t.Errorf("expected 0 trending calls on login happy path, got %d", tr.calls)
	}
}

func TestPersonalPick_LoginEmpty_ReturnsNilNil(t *testing.T) {
	tr := &fakeTrending{}
	recs := &fakePlayerRecs{recs: nil}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, recs, c, seededRng(1), testLogger())

	uid := "user1"
	ctx := ContextWithJWT(context.Background(), "tj")
	card, err := r.Resolve(ctx, &uid)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card on empty, got %+v", card)
	}
}

func TestPersonalPick_LoginNoJWT_FallsBackToAnon(t *testing.T) {
	tr := &fakeTrending{items: makeAnimes(3)}
	recs := &fakePlayerRecs{}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, recs, c, seededRng(7), testLogger())

	uid := "u1"
	// NO JWT on ctx
	card, err := r.Resolve(context.Background(), &uid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card (fallback to anon trending)")
	}
	if recs.capturedJWT != "" {
		t.Errorf("capturedJWT should be empty when no JWT on ctx, got %q", recs.capturedJWT)
	}
	if recs.calls != 0 {
		t.Errorf("expected 0 rec calls when no JWT (fell back to anon), got %d", recs.calls)
	}
	if tr.calls != 1 {
		t.Errorf("expected 1 trending call on anon fallback, got %d", tr.calls)
	}
	data := card.Data.(spotlight.PersonalPickData)
	if data.Source != "trending" {
		t.Errorf("Source = %q; want trending (fell back to anon)", data.Source)
	}
}

func TestPersonalPick_CacheHit_DoesNotCallUpstream(t *testing.T) {
	tr := &fakeTrending{items: makeAnimes(10)}
	recs := &fakePlayerRecs{}
	c := newFakeCache()

	// Seed cache for the anon key (today's UTC date).
	seeded := spotlight.PersonalPickData{
		Items:  []spotlight.PersonalPickItem{{Anime: domain.Anime{ID: "cached"}, ReasonI18nKey: "spotlight.personalPick.reason.trending"}},
		Source: "trending",
	}
	key := "spotlight:trending:" + spotlight.DateKeyUTC(time.Now())
	raw, _ := json.Marshal(seeded)
	c.store[key] = raw

	r := NewPersonalPickResolver(tr, recs, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	if tr.calls != 0 {
		t.Errorf("expected 0 trending calls on cache hit, got %d", tr.calls)
	}
	if recs.calls != 0 {
		t.Errorf("expected 0 rec calls on cache hit, got %d", recs.calls)
	}
	data := card.Data.(spotlight.PersonalPickData)
	if len(data.Items) != 1 || data.Items[0].Anime.ID != "cached" {
		t.Errorf("expected cached payload, got: %+v", data)
	}
}

func TestPersonalPick_CacheKeysAreDateOrUserScoped(t *testing.T) {
	dateRE := regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

	// Anon
	tr := &fakeTrending{items: makeAnimes(3)}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, &fakePlayerRecs{}, c, seededRng(1), testLogger())
	if _, err := r.Resolve(context.Background(), nil); err != nil {
		t.Fatalf("Resolve anon: %v", err)
	}
	keys := c.keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 anon cache key, got %d: %v", len(keys), keys)
	}
	if !strings.HasPrefix(keys[0], "spotlight:trending:") {
		t.Errorf("anon key should start with spotlight:trending:, got %q", keys[0])
	}
	if !dateRE.MatchString(keys[0]) {
		t.Errorf("anon key should contain YYYY-MM-DD, got %q", keys[0])
	}

	// Login
	tr2 := &fakeTrending{}
	recs2 := &fakePlayerRecs{recs: []client.UserRec{recFromAnime(*makeAnimes(1)[0], 0.9)}}
	c2 := newFakeCache()
	r2 := NewPersonalPickResolver(tr2, recs2, c2, seededRng(1), testLogger())
	uid := "user-42"
	ctx := ContextWithJWT(context.Background(), "tj")
	if _, err := r2.Resolve(ctx, &uid); err != nil {
		t.Fatalf("Resolve login: %v", err)
	}
	keys2 := c2.keys()
	if len(keys2) != 1 {
		t.Fatalf("expected 1 login cache key, got %d: %v", len(keys2), keys2)
	}
	if !strings.HasPrefix(keys2[0], "spotlight:personal:user-42:") {
		t.Errorf("login key should start with spotlight:personal:user-42:, got %q", keys2[0])
	}
	if !dateRE.MatchString(keys2[0]) {
		t.Errorf("login key should contain YYYY-MM-DD, got %q", keys2[0])
	}
}

func TestPersonalPick_TrendingError_Wraps(t *testing.T) {
	tr := &fakeTrending{err: errors.New("shikimori down")}
	c := newFakeCache()
	r := NewPersonalPickResolver(tr, &fakePlayerRecs{}, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if card != nil {
		t.Errorf("expected nil card on error, got %+v", card)
	}
	if !strings.Contains(err.Error(), "personal_pick") {
		t.Errorf("expected wrapped error to mention personal_pick, got %v", err)
	}
}
