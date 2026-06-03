package cards

import (
	"context"
	"reflect"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

func TestContinueWatchingNew_Type(t *testing.T) {
	r := &ContinueWatchingNewResolver{}
	if got := r.Type(); got != "continue_watching_new" {
		t.Errorf("Type() = %q; want continue_watching_new", got)
	}
}

func TestContinueWatchingNew_AnonUserID_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card for anon, got %+v", card)
	}
	if f.calls != 0 {
		t.Errorf("expected 0 player calls for anon, got %d", f.calls)
	}
}

func TestContinueWatchingNew_EmptyUserIDString_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr(""))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card for empty userID, got %+v", card)
	}
	if f.calls != 0 {
		t.Errorf("expected 0 player calls, got %d", f.calls)
	}
}

func TestContinueWatchingNew_NoItems_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{items: nil}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card, got %+v", card)
	}
}

func TestContinueWatchingNew_NoNewEpisode_ReturnsNilNil(t *testing.T) {
	// EpisodesAired == LastWatchedEpisode → not strictly greater → ineligible.
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "1", Status: "watching", EpisodesAired: 5, LastWatchedEpisode: 5, UpdatedAt: "2026-05-21T12:00:00Z"},
		{AnimeID: "2", Status: "watching", EpisodesAired: 10, LastWatchedEpisode: 10, UpdatedAt: "2026-05-21T13:00:00Z"},
	}}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card when no new episode, got %+v", card)
	}
}

func TestContinueWatchingNew_FilterRule_StrictGreaterThan(t *testing.T) {
	// EpisodesAired = LastWatchedEpisode + 1 → NOT eligible (spec uses strict >, not >=).
	// EpisodesAired = LastWatchedEpisode + 2 → eligible.
	notNew := client.InternalListItem{AnimeID: "not-new", Status: "watching", EpisodesAired: 6, LastWatchedEpisode: 5, UpdatedAt: "2026-05-21T12:00:00Z"}
	newEp := client.InternalListItem{AnimeID: "is-new", Status: "watching", EpisodesAired: 7, LastWatchedEpisode: 5, UpdatedAt: "2026-05-21T13:00:00Z"}
	f := &fakeListByStatuses{items: []client.InternalListItem{notNew, newEp}}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())

	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.ContinueWatchingNewData)
	if data.Anime.ID != "is-new" {
		t.Errorf("expected the strictly-greater item, got %q", data.Anime.ID)
	}
	// NewEpisodeNumber is the NEXT unwatched episode (LastWatchedEpisode+1),
	// not the latest aired — the CTA must resume the user at ep 6, not skip
	// them forward to ep 7 (AUTO-349).
	if data.LastWatchedEpisode != 5 || data.NewEpisodeNumber != 6 {
		t.Errorf("episode fields wrong: %+v", data)
	}
}

func TestContinueWatchingNew_PicksMostRecentUpdatedAt(t *testing.T) {
	older := client.InternalListItem{AnimeID: "older", Status: "watching", EpisodesAired: 10, LastWatchedEpisode: 5, UpdatedAt: "2026-05-20T12:00:00Z"}
	mid := client.InternalListItem{AnimeID: "mid", Status: "watching", EpisodesAired: 10, LastWatchedEpisode: 5, UpdatedAt: "2026-05-21T08:00:00Z"}
	newest := client.InternalListItem{AnimeID: "newest", Status: "watching", EpisodesAired: 10, LastWatchedEpisode: 5, UpdatedAt: "2026-05-21T18:00:00Z"}
	// Provide in random order; the resolver must sort.
	f := &fakeListByStatuses{items: []client.InternalListItem{mid, older, newest}}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())

	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil || card == nil {
		t.Fatalf("Resolve: card=%v err=%v", card, err)
	}
	data := card.Data.(spotlight.ContinueWatchingNewData)
	if data.Anime.ID != "newest" {
		t.Errorf("expected newest UpdatedAt item, got %q", data.Anime.ID)
	}
}

func TestContinueWatchingNew_StatusFilterForwarded(t *testing.T) {
	f := &fakeListByStatuses{items: nil}
	c := newFakeCache()
	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	_, _ = r.Resolve(context.Background(), ptr("u1"))
	if !reflect.DeepEqual(f.capturedStatuses, []string{"watching"}) {
		t.Errorf("expected statuses [watching], got %v", f.capturedStatuses)
	}
	if f.capturedUserID != "u1" {
		t.Errorf("expected userID u1, got %q", f.capturedUserID)
	}
}

func TestContinueWatchingNew_CacheHit_DoesNotCallPlayer(t *testing.T) {
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "fresh", Status: "watching", EpisodesAired: 10, LastWatchedEpisode: 5},
	}}
	c := newFakeCache()

	seeded := spotlight.ContinueWatchingNewData{
		Anime:              fakeAnimeWithID("cached"),
		LastWatchedEpisode: 3,
		NewEpisodeNumber:   5,
	}
	if err := c.Set(context.Background(), "spotlight:continue_new:u1", seeded, 0); err != nil {
		t.Fatal(err)
	}
	c.sets = 0

	r := NewContinueWatchingNewResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	if f.calls != 0 {
		t.Errorf("expected 0 player calls on cache hit, got %d", f.calls)
	}
	data := card.Data.(spotlight.ContinueWatchingNewData)
	if data.Anime.ID != "cached" {
		t.Errorf("expected cached payload, got: %+v", data)
	}
}
