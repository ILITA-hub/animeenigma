package service

import (
	"context"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
)

// fakeContinueStore records Get + AppendPart.
type fakeContinueStore struct {
	get       *domain.Fanfic
	appended  string
	addedUse  int
	newPart   int
	appendErr error
}

func (s *fakeContinueStore) Create(context.Context, *domain.Fanfic) error { return nil }
func (s *fakeContinueStore) UpdateResult(context.Context, string, string, string, int) error {
	return nil
}
func (s *fakeContinueStore) MarkFailed(context.Context, string, string) error { return nil }
func (s *fakeContinueStore) Get(ctx context.Context, userID, id string) (*domain.Fanfic, error) {
	return s.get, nil
}
func (s *fakeContinueStore) AppendPart(ctx context.Context, userID, id, appended string, addedUsage, newPart int) error {
	s.appended, s.addedUse, s.newPart = appended, addedUsage, newPart
	return s.appendErr
}

func TestContinue_AppendsSectionedPart(t *testing.T) {
	store := &fakeContinueStore{get: &domain.Fanfic{
		ID: "f1", UserID: "u1", AnimeTitle: "Frieren",
		Length: "oneshot", POV: "third", Rating: "teen", Language: "ru",
		Status: domain.StatusComplete, Content: "первая часть", PartCount: 1,
		Characters: datatypes.JSON([]byte(`[]`)), Tags: datatypes.JSON([]byte(`[]`)),
	}}
	g := NewGenerator(&fakeStreamer{out: "# Ignored\nследующая часть"}, store,
		noopQuota{}, nil, "test-model", 24000, nil)

	var events []string
	emit := func(event string, data any) error { events = append(events, event); return nil }
	if err := g.Continue(context.Background(), "u1", "f1", emit); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if store.newPart != 2 {
		t.Errorf("newPart = %d, want 2", store.newPart)
	}
	if !strings.Contains(store.appended, "## Часть 2") || !strings.Contains(store.appended, "---") {
		t.Errorf("appended not sectioned: %q", store.appended)
	}
	if strings.Contains(store.appended, "# Ignored") {
		t.Errorf("stray title H1 should be stripped: %q", store.appended)
	}
	if store.addedUse != 55 {
		t.Errorf("addedUse = %d, want 55", store.addedUse)
	}
}

func TestContinue_RejectsNonComplete(t *testing.T) {
	store := &fakeContinueStore{get: &domain.Fanfic{
		ID: "f1", UserID: "u1", Status: domain.StatusGenerating,
		Characters: datatypes.JSON([]byte(`[]`)), Tags: datatypes.JSON([]byte(`[]`)),
	}}
	g := NewGenerator(&fakeStreamer{}, store, noopQuota{}, nil, "m", 24000, nil)
	err := g.Continue(context.Background(), "u1", "f1", func(string, any) error { return nil })
	if err == nil {
		t.Fatal("expected error continuing a non-complete fanfic")
	}
}
