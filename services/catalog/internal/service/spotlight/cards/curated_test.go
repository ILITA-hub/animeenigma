package cards

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// fakeAnimeGetter implements animeGetter with a canned anime + call counter.
type fakeAnimeGetter struct {
	anime *domain.Anime
	err   error
	calls int
}

func (f *fakeAnimeGetter) GetByShikimoriID(_ context.Context, _ string) (*domain.Anime, error) {
	f.calls++
	return f.anime, f.err
}

func TestCuratedResolver_Type(t *testing.T) {
	r := &CuratedResolver{}
	if got := r.Type(); got != "curated" {
		t.Errorf("Type() = %q, want curated", got)
	}
}

func TestCuratedResolver_OngoingAnime_ReturnsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{ID: "u1", Name: "Yani Neko", Status: domain.StatusOngoing}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected a card for an ongoing anime")
	}
	if card.Type != "curated" {
		t.Errorf("Card.Type = %q, want curated", card.Type)
	}
	if card.Priority != 1.5 {
		t.Errorf("Card.Priority = %v, want 1.5", card.Priority)
	}
	if _, ok := card.Data.(spotlight.CuratedData); !ok {
		t.Fatalf("Card.Data is not CuratedData: %T", card.Data)
	}
}

func TestCuratedResolver_ReleasedAnime_DropsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{ID: "u1", Status: domain.StatusReleased}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) for released anime, got card=%v err=%v", card, err)
	}
}

func TestCuratedResolver_EmptyID_Disabled(t *testing.T) {
	repo := &fakeAnimeGetter{anime: &domain.Anime{Status: domain.StatusOngoing}}
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) when disabled, got card=%v err=%v", card, err)
	}
	if repo.calls != 0 {
		t.Errorf("repo should not be queried when disabled, got %d calls", repo.calls)
	}
}

func TestCuratedResolver_NotFound_DropsCard(t *testing.T) {
	repo := &fakeAnimeGetter{anime: nil} // GetByShikimoriID returns (nil,nil) on not-found
	r := NewCuratedResolver(repo, newFakeCache(), testLogger(), "63403")

	card, err := r.Resolve(context.Background(), nil)
	if err != nil || card != nil {
		t.Fatalf("expected (nil,nil) for missing anime, got card=%v err=%v", card, err)
	}
}
