package cards

import (
	"context"
	"encoding/json"
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

// fakeListedFilter reports the configured `listed` IDs as already in the
// user's list; err simulates a DB failure (the resolver must fail open).
type fakeListedFilter struct {
	listed map[string]struct{}
	err    error
	calls  int
}

func (f *fakeListedFilter) ListedAnimeIDs(_ context.Context, _ string, animeIDs []string) (map[string]struct{}, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]struct{})
	for _, id := range animeIDs {
		if _, ok := f.listed[id]; ok {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

// noListed is the default filter: the user has nothing in their list, so the
// resolver behaves as it did before the already-added rule was added.
func noListed() *fakeListedFilter { return &fakeListedFilter{} }

func TestUpcomingForYou_AnonNilNil(t *testing.T) {
	r := NewUpcomingForYouResolver(&fakeUpcomingFetcher{}, noListed(), newFakeCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if card != nil || err != nil {
		t.Fatalf("anon must be (nil,nil), got card=%v err=%v", card, err)
	}
}

func TestUpcomingForYou_NoJWTNilNil(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{}
	r := NewUpcomingForYouResolver(f, noListed(), newFakeCache(), testLogger())
	// userID set but NO JWT on ctx — defensive path, no fetch.
	card, err := r.Resolve(context.Background(), &uid)
	if card != nil || err != nil || f.calls != 0 {
		t.Fatalf("no-jwt must be (nil,nil) with no fetch, got card=%v err=%v calls=%d", card, err, f.calls)
	}
}

func TestUpcomingForYou_EmptyItemsIneligible(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{}}
	lf := noListed()
	r := NewUpcomingForYouResolver(f, lf, newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if card != nil || err != nil {
		t.Fatalf("empty items must be (nil,nil), got card=%v err=%v", card, err)
	}
	if lf.calls != 0 {
		t.Fatalf("no recs items must short-circuit before the list filter, got calls=%d", lf.calls)
	}
}

func TestUpcomingForYou_ItemsProduceCard(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{
		{Anime: []byte(`{"id":"a1","name":"Frieren S2"}`), MatchScore: 0.61, Reason: []byte(`{"kind":"franchise"}`)},
	}}
	r := NewUpcomingForYouResolver(f, noListed(), newFakeCache(), testLogger())
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
	r := NewUpcomingForYouResolver(f, noListed(), newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err == nil || card != nil {
		t.Fatalf("fetch error must propagate, got card=%v err=%v", card, err)
	}
}

// upcomingItemID decodes the "id" out of one item's verbatim Anime payload.
func upcomingItemID(t *testing.T, it spotlight.UpcomingForYouItem) string {
	t.Helper()
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(it.Anime, &a); err != nil {
		t.Fatalf("decode item id: %v", err)
	}
	return a.ID
}

// A title the user has ALREADY added is dropped; unlisted siblings survive.
func TestUpcomingForYou_AlreadyListedItemHidden(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{
		{Anime: []byte(`{"id":"a1","name":"Already Planned"}`), MatchScore: 0.7, Reason: []byte(`{"kind":"franchise"}`)},
		{Anime: []byte(`{"id":"a2","name":"Fresh Match"}`), MatchScore: 0.6, Reason: []byte(`{"kind":"taste"}`)},
	}}
	lf := &fakeListedFilter{listed: map[string]struct{}{"a1": {}}}
	r := NewUpcomingForYouResolver(f, lf, newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	data, ok := card.Data.(spotlight.UpcomingForYouData)
	if !ok || len(data.Items) != 1 {
		t.Fatalf("expected exactly the unlisted item, got %+v", card.Data)
	}
	if got := upcomingItemID(t, data.Items[0]); got != "a2" {
		t.Fatalf("surviving item must be a2 (unlisted), got %q", got)
	}
}

// When every match is already listed, the card is ineligible (nil, nil).
func TestUpcomingForYou_AllListedIneligible(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{
		{Anime: []byte(`{"id":"a1","name":"Already Planned"}`), MatchScore: 0.7, Reason: []byte(`{"kind":"franchise"}`)},
	}}
	lf := &fakeListedFilter{listed: map[string]struct{}{"a1": {}}}
	r := NewUpcomingForYouResolver(f, lf, newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if card != nil || err != nil {
		t.Fatalf("all-listed must be (nil,nil), got card=%v err=%v", card, err)
	}
}

// A filter DB error must NOT blank the card — fail open, keep the items.
func TestUpcomingForYou_FilterErrorFailsOpen(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{
		{Anime: []byte(`{"id":"a1","name":"Frieren S2"}`), MatchScore: 0.61, Reason: []byte(`{"kind":"franchise"}`)},
	}}
	lf := &fakeListedFilter{err: errors.New("db down")}
	r := NewUpcomingForYouResolver(f, lf, newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	data, ok := card.Data.(spotlight.UpcomingForYouData)
	if !ok || len(data.Items) != 1 {
		t.Fatalf("filter error must keep items (fail open), got %+v", card.Data)
	}
}
