package cards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

type fakeTelegram struct {
	items []telegram.NewsItem
	err   error
	calls int32
}

func (f *fakeTelegram) FetchNews(_ context.Context) ([]telegram.NewsItem, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.items, f.err
}

func tgItems(n int) []telegram.NewsItem {
	out := make([]telegram.NewsItem, n)
	for i := 0; i < n; i++ {
		id := string(rune('1' + i))
		out[i] = telegram.NewsItem{
			ID:    id,
			Text:  "post text " + id,
			Date:  "2026-05-21T12:00:0" + id + "Z",
			Link:  "https://t.me/x/" + id,
			Views: "10",
		}
	}
	return out
}

func TestTelegramNews_Type(t *testing.T) {
	r := &TelegramNewsResolver{}
	if got := r.Type(); got != "telegram_news" {
		t.Errorf("Type() = %q; want telegram_news", got)
	}
}

func TestTelegramNews_HappyThree(t *testing.T) {
	tg := &fakeTelegram{items: tgItems(3)}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data := card.Data.(spotlight.TelegramNewsData)
	if len(data.Posts) != 3 {
		t.Errorf("expected 3 posts, got %d", len(data.Posts))
	}
}

func TestTelegramNews_HappyFive_AdaptiveSliceToThree(t *testing.T) {
	tg := &fakeTelegram{items: tgItems(5)}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.TelegramNewsData)
	if len(data.Posts) != 3 {
		t.Errorf("AdaptiveSlice(N=5) should return 3, got %d", len(data.Posts))
	}
}

func TestTelegramNews_Empty_ReturnsNilNil(t *testing.T) {
	tg := &fakeTelegram{items: nil}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card, got %+v", card)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set on empty, got %d", c.sets)
	}
}

func TestTelegramNews_CacheHit_ReusesExistingNewsKey(t *testing.T) {
	tg := &fakeTelegram{items: tgItems(3)}
	c := newFakeCache()

	// Seed cache at the existing news:telegram key with raw []telegram.NewsItem
	// — that's the shape the news handler writes.
	seeded := tgItems(2)
	data, _ := json.Marshal(seeded)
	c.store["news:telegram"] = data

	r := NewTelegramNewsResolver(tg, c, seededRng(99), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	if tg.calls != 0 {
		t.Errorf("expected 0 telegram calls on cache hit, got %d", tg.calls)
	}
	out := card.Data.(spotlight.TelegramNewsData)
	if len(out.Posts) != 1 {
		// N=2 in cache → AdaptiveSlice picks 1
		t.Errorf("expected 1 post (AdaptiveSlice of N=2), got %d", len(out.Posts))
	}
}

func TestTelegramNews_FetchError_Wraps(t *testing.T) {
	tg := &fakeTelegram{err: errors.New("telegram 500")}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if card != nil {
		t.Errorf("expected nil card on error, got %+v", card)
	}
	if !strings.Contains(err.Error(), "telegram_news") {
		t.Errorf("expected wrapped error to mention telegram_news, got %v", err)
	}
}

// TestTelegramNews_ImageURL_FlowsThrough asserts the ImageURL added in
// v1.1-polish Phase 06 (HSB-V11-TG-01) survives the parser → resolver →
// spotlight.TelegramPost pipeline. Fixture mirrors the live cache shape
// confirmed via `redis-cli GET news:telegram`: about 30% of @anime_enigma
// posts carry `image_url`, the rest are text-only (empty string).
func TestTelegramNews_ImageURL_FlowsThrough(t *testing.T) {
	items := []telegram.NewsItem{
		{
			ID:       "100",
			Text:     "post with image",
			Date:     "2026-05-23T17:03:13+00:00",
			Link:     "https://t.me/animeenigma/100",
			Views:    "42",
			ImageURL: "https://cdn4.telesco.pe/file/abcdef.jpg",
		},
		{
			ID:    "101",
			Text:  "text-only post",
			Date:  "2026-05-23T17:04:00+00:00",
			Link:  "https://t.me/animeenigma/101",
			Views: "10",
		},
		{
			ID:       "102",
			Text:     "another image post",
			Date:     "2026-05-23T17:05:00+00:00",
			Link:     "https://t.me/animeenigma/102",
			Views:    "5",
			ImageURL: "https://cdn4.telesco.pe/file/zzz.jpg",
		},
	}
	tg := &fakeTelegram{items: items}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data := card.Data.(spotlight.TelegramNewsData)
	if len(data.Posts) != 3 {
		t.Fatalf("expected AdaptiveSlice(3)=3, got %d posts", len(data.Posts))
	}

	// Build a quick lookup by Link so we don't rely on AdaptiveSlice order.
	byLink := map[string]spotlight.TelegramPost{}
	for _, p := range data.Posts {
		byLink[p.Link] = p
	}

	if got, want := byLink["https://t.me/animeenigma/100"].ImageURL,
		"https://cdn4.telesco.pe/file/abcdef.jpg"; got != want {
		t.Errorf("post 100 ImageURL = %q; want %q", got, want)
	}
	if got := byLink["https://t.me/animeenigma/101"].ImageURL; got != "" {
		t.Errorf("post 101 (text-only) ImageURL = %q; want empty string", got)
	}
	if got, want := byLink["https://t.me/animeenigma/102"].ImageURL,
		"https://cdn4.telesco.pe/file/zzz.jpg"; got != want {
		t.Errorf("post 102 ImageURL = %q; want %q", got, want)
	}
}

// TestTelegramNews_ImageURL_CacheRoundTrip seeds the cache directly with the
// raw []telegram.NewsItem shape (same as handler/news.go writes) and confirms
// ImageURL survives the JSON unmarshal in the cache-hit path. Guards against
// a future cache-shape refactor accidentally dropping the field.
func TestTelegramNews_ImageURL_CacheRoundTrip(t *testing.T) {
	tg := &fakeTelegram{items: tgItems(3)} // should not be called

	c := newFakeCache()
	seeded := []telegram.NewsItem{
		{
			ID:       "200",
			Text:     "cached image post",
			Date:     "2026-05-23T18:00:00+00:00",
			Link:     "https://t.me/animeenigma/200",
			Views:    "99",
			ImageURL: "https://cdn4.telesco.pe/file/cached.jpg",
		},
	}
	data, _ := json.Marshal(seeded)
	c.store["news:telegram"] = data

	r := NewTelegramNewsResolver(tg, c, seededRng(7), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	if tg.calls != 0 {
		t.Fatalf("expected 0 telegram calls on cache hit, got %d", tg.calls)
	}
	out := card.Data.(spotlight.TelegramNewsData)
	if len(out.Posts) != 1 {
		t.Fatalf("AdaptiveSlice(N=1) should pick 1, got %d", len(out.Posts))
	}
	if got, want := out.Posts[0].ImageURL,
		"https://cdn4.telesco.pe/file/cached.jpg"; got != want {
		t.Errorf("cached ImageURL = %q; want %q", got, want)
	}
}

func TestTelegramNews_UsesExistingKey(t *testing.T) {
	tg := &fakeTelegram{items: tgItems(3)}
	c := newFakeCache()
	r := NewTelegramNewsResolver(tg, c, seededRng(1), testLogger())
	if _, err := r.Resolve(context.Background(), nil); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	keys := c.keys()
	if len(keys) != 1 || keys[0] != "news:telegram" {
		t.Errorf("expected single cache key news:telegram, got %v", keys)
	}
}
