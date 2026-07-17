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
	r := NewUpcomingForYouResolver(&fakeUpcomingFetcher{}, newFakeCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if card != nil || err != nil {
		t.Fatalf("anon must be (nil,nil), got card=%v err=%v", card, err)
	}
}

func TestUpcomingForYou_NoJWTNilNil(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger())
	// userID set but NO JWT on ctx — defensive path, no fetch.
	card, err := r.Resolve(context.Background(), &uid)
	if card != nil || err != nil || f.calls != 0 {
		t.Fatalf("no-jwt must be (nil,nil) with no fetch, got card=%v err=%v calls=%d", card, err, f.calls)
	}
}

func TestUpcomingForYou_EmptyItemsIneligible(t *testing.T) {
	uid := "u1"
	f := &fakeUpcomingFetcher{items: []client.UpcomingWireItem{}}
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger())
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
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger())
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
	r := NewUpcomingForYouResolver(f, newFakeCache(), testLogger())
	ctx := ContextWithJWT(context.Background(), "jwt-1")
	card, err := r.Resolve(ctx, &uid)
	if err == nil || card != nil {
		t.Fatalf("fetch error must propagate, got card=%v err=%v", card, err)
	}
}
