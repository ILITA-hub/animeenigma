package cards

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
)

// fakeListByStatuses is shared across not_time_yet_test.go and
// continue_watching_new_test.go (same package).
type fakeListByStatuses struct {
	items            []client.InternalListItem
	err              error
	calls            int32
	capturedUserID   string
	capturedStatuses []string
	mu               sync.Mutex
}

func (f *fakeListByStatuses) FetchListByStatuses(_ context.Context, userID string, statuses []string) ([]client.InternalListItem, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.capturedUserID = userID
	f.capturedStatuses = append([]string(nil), statuses...)
	f.mu.Unlock()
	return f.items, f.err
}

func ptr(s string) *string { return &s }

func TestNotTimeYet_Type(t *testing.T) {
	r := &NotTimeYetResolver{}
	if got := r.Type(); got != "not_time_yet" {
		t.Errorf("Type() = %q; want not_time_yet", got)
	}
}

func TestNotTimeYet_AnonUserID_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
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

func TestNotTimeYet_EmptyUserIDString_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
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

func TestNotTimeYet_NoItems_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{items: nil}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card, got %+v", card)
	}
}

func TestNotTimeYet_NoAiring_ReturnsNilNil(t *testing.T) {
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "1", Name: "A", Status: "planned", EpisodesAired: 0, EpisodesCount: 12},
		{AnimeID: "2", Name: "B", Status: "postponed", EpisodesAired: 0, EpisodesCount: 24},
		{AnimeID: "3", Name: "C", Status: "planned", EpisodesAired: 0, EpisodesCount: 12},
	}}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card when no airing items, got %+v", card)
	}
}

func TestNotTimeYet_AiringHappy_RandomPick(t *testing.T) {
	airingA := client.InternalListItem{AnimeID: "airing-1", Name: "AiringA", Status: "planned", EpisodesAired: 3, EpisodesCount: 12}
	airingB := client.InternalListItem{AnimeID: "airing-2", Name: "AiringB", Status: "postponed", EpisodesAired: 5, EpisodesCount: 24}
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "1", Name: "Not", Status: "planned", EpisodesAired: 0, EpisodesCount: 12},
		airingA,
		{AnimeID: "2", Name: "Not2", Status: "planned", EpisodesAired: 0, EpisodesCount: 12},
		airingB,
	}}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(42), testLogger())

	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data := card.Data.(spotlight.NotTimeYetData)
	if data.Anime.ID != airingA.AnimeID && data.Anime.ID != airingB.AnimeID {
		t.Errorf("picked anime %q is neither airing candidate %q nor %q", data.Anime.ID, airingA.AnimeID, airingB.AnimeID)
	}
	if data.Status != "planned" && data.Status != "postponed" {
		t.Errorf("Status should be planned or postponed, got %q", data.Status)
	}
}

func TestNotTimeYet_AddedAt_PopulatedFromUpdatedAt(t *testing.T) {
	// Single airing candidate so the random pick is deterministic; its
	// updated_at must flow through to NotTimeYetData.AddedAt (HSB-V11-NT-01).
	updated := "2026-05-10T08:30:00Z"
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "airing-1", Name: "AiringA", Status: "planned", EpisodesAired: 3, EpisodesCount: 12, UpdatedAt: updated},
	}}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())

	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	data := card.Data.(spotlight.NotTimeYetData)
	if data.AddedAt == nil {
		t.Fatal("expected AddedAt to be populated from a non-zero UpdatedAt, got nil")
	}
	want, _ := time.Parse(time.RFC3339, updated)
	if !data.AddedAt.Equal(want) {
		t.Errorf("AddedAt = %v; want %v", data.AddedAt, want)
	}
}

func TestNotTimeYet_AddedAt_NilWhenUpdatedAtEmptyOrInvalid(t *testing.T) {
	cases := map[string]string{
		"empty":   "",
		"garbage": "not-a-timestamp",
		"zero":    "0001-01-01T00:00:00Z",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeListByStatuses{items: []client.InternalListItem{
				{AnimeID: "airing-1", Name: "AiringA", Status: "postponed", EpisodesAired: 5, EpisodesCount: 24, UpdatedAt: raw},
			}}
			c := newFakeCache()
			r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())

			card, err := r.Resolve(context.Background(), ptr("u1"))
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if card == nil {
				t.Fatal("expected non-nil card")
			}
			data := card.Data.(spotlight.NotTimeYetData)
			if data.AddedAt != nil {
				t.Errorf("expected nil AddedAt for %q, got %v", raw, data.AddedAt)
			}
		})
	}
}

func TestNotTimeYet_StatusFilterForwarded(t *testing.T) {
	f := &fakeListByStatuses{items: nil}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
	_, _ = r.Resolve(context.Background(), ptr("u1"))
	if !reflect.DeepEqual(f.capturedStatuses, []string{"planned", "postponed"}) {
		t.Errorf("expected statuses [planned postponed], got %v", f.capturedStatuses)
	}
	if f.capturedUserID != "u1" {
		t.Errorf("expected userID u1, got %q", f.capturedUserID)
	}
}

func TestNotTimeYet_CacheHit_DoesNotCallPlayer(t *testing.T) {
	f := &fakeListByStatuses{items: []client.InternalListItem{
		{AnimeID: "fresh", Name: "F", Status: "planned", EpisodesAired: 5},
	}}
	c := newFakeCache()

	// Seed cache for u1.
	seeded := spotlight.NotTimeYetData{
		Anime:  fakeAnimeWithID("cached"),
		Status: "planned",
	}
	if err := c.Set(context.Background(), "spotlight:not_time_yet:u1", seeded, 0); err != nil {
		t.Fatal(err)
	}
	c.sets = 0 // reset for assertion

	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
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
	data := card.Data.(spotlight.NotTimeYetData)
	if data.Anime.ID != "cached" {
		t.Errorf("expected cached payload, got: %+v", data)
	}
}

func TestNotTimeYet_PlayerError_Wraps(t *testing.T) {
	f := &fakeListByStatuses{err: errors.New("player 500")}
	c := newFakeCache()
	r := NewNotTimeYetResolver(f, c, seededRng(1), testLogger())
	card, err := r.Resolve(context.Background(), ptr("u1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if card != nil {
		t.Errorf("expected nil card on error, got %+v", card)
	}
	if !strings.Contains(err.Error(), "not_time_yet") {
		t.Errorf("expected wrapped error to mention not_time_yet, got %v", err)
	}
}
