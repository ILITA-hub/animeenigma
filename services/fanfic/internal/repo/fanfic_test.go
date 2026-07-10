package repo

import (
	"context"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB returns an in-memory SQLite *gorm.DB with the fanfics table
// AutoMigrate'd from the domain model. The universal repo-test convention in
// this repo is SQLite in-memory (see services/gacha/internal/repo/wallet_test.go),
// not testcontainers-Postgres. The Postgres-only `default:gen_random_uuid()`
// column default was removed from the model in favor of a portable
// BeforeCreate hook (see domain/fanfic.go), so AutoMigrate + Create both work
// unmodified on SQLite.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Fanfic{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	db := newTestDB(t)
	return NewRepository(db)
}

func TestCreateAndGet(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	f := &domain.Fanfic{
		UserID:     "11111111-1111-1111-1111-111111111111",
		AnimeTitle: "Frieren", Rating: "mature", Language: "ru",
		Characters: datatypes.JSON([]byte(`[{"name":"Frieren"}]`)),
		Tags:       datatypes.JSON([]byte(`["angst"]`)),
		Status:     domain.StatusGenerating,
	}
	if err := r.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ID == "" {
		t.Fatal("expected generated ID")
	}
	got, err := r.Get(ctx, f.UserID, f.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AnimeTitle != "Frieren" {
		t.Errorf("AnimeTitle = %q", got.AnimeTitle)
	}
}

func TestGet_WrongOwnerNotFound(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	owner := "55555555-5555-5555-5555-555555555555"
	other := "66666666-6666-6666-6666-666666666666"
	f := &domain.Fanfic{UserID: owner, AnimeTitle: "A", Status: domain.StatusGenerating}
	if err := r.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := r.Get(ctx, other, f.ID); err == nil {
		t.Error("expected not-found for non-owner Get")
	}
}

func TestUpdateResultAndList(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	user := "22222222-2222-2222-2222-222222222222"
	f := &domain.Fanfic{UserID: user, AnimeTitle: "A", Status: domain.StatusGenerating}
	_ = r.Create(ctx, f)
	if err := r.UpdateResult(ctx, f.ID, "My Title", "the story", 123); err != nil {
		t.Fatalf("UpdateResult: %v", err)
	}
	items, total, err := r.List(ctx, user, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	if items[0].Title != "My Title" || items[0].Status != domain.StatusComplete {
		t.Errorf("row not updated: %+v", items[0])
	}
	if items[0].TokenUsage != 123 {
		t.Errorf("TokenUsage = %d; want 123", items[0].TokenUsage)
	}
}

func TestMarkFailed(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	user := "77777777-7777-7777-7777-777777777777"
	f := &domain.Fanfic{UserID: user, AnimeTitle: "A", Status: domain.StatusGenerating}
	_ = r.Create(ctx, f)
	if err := r.MarkFailed(ctx, f.ID, "groq timeout"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	got, err := r.Get(ctx, user, f.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.StatusFailed || got.ErrorMsg != "groq timeout" {
		t.Errorf("row not marked failed: %+v", got)
	}
}

func TestList_OrderedNewestFirstAndPaginated(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	user := "88888888-8888-8888-8888-888888888888"
	var ids []string
	for i := 0; i < 3; i++ {
		f := &domain.Fanfic{UserID: user, AnimeTitle: "A", Status: domain.StatusGenerating}
		if err := r.Create(ctx, f); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		ids = append(ids, f.ID)
	}
	items, total, err := r.List(ctx, user, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}
	// Newest-first: last created (ids[2]) should come first.
	if items[0].ID != ids[2] || items[2].ID != ids[0] {
		t.Errorf("not newest-first: got order %v; want reverse of %v", []string{items[0].ID, items[1].ID, items[2].ID}, ids)
	}

	// Pagination: limit=1 offset=1 returns the middle item, total still 3.
	page, total2, err := r.List(ctx, user, 1, 1)
	if err != nil {
		t.Fatalf("List page: %v", err)
	}
	if total2 != 3 || len(page) != 1 {
		t.Fatalf("total2=%d len=%d", total2, len(page))
	}
	if page[0].ID != items[1].ID {
		t.Errorf("paginated item = %s; want %s", page[0].ID, items[1].ID)
	}
}

func TestAppendPart(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	f := &domain.Fanfic{
		UserID: "u1", AnimeTitle: "Frieren", Status: domain.StatusComplete,
		Content: "первая часть", TokenUsage: 100, PartCount: 1,
	}
	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.AppendPart(ctx, "u1", f.ID, "\n\n---\n\n## Часть 2\n\nвторая часть", 55, 2); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := repo.Get(ctx, "u1", f.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PartCount != 2 {
		t.Errorf("part_count = %d, want 2", got.PartCount)
	}
	if got.TokenUsage != 155 {
		t.Errorf("token_usage = %d, want 155", got.TokenUsage)
	}
	if !strings.Contains(got.Content, "первая часть") || !strings.Contains(got.Content, "вторая часть") {
		t.Errorf("content missing a part: %q", got.Content)
	}

	// Non-owner append affects zero rows -> NotFound.
	if err := repo.AppendPart(ctx, "someone-else", f.ID, "x", 1, 3); err == nil {
		t.Error("expected NotFound for non-owner append")
	}
}

func TestSoftDeleteScopedToOwner(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	owner := "33333333-3333-3333-3333-333333333333"
	other := "44444444-4444-4444-4444-444444444444"
	f := &domain.Fanfic{UserID: owner, AnimeTitle: "A", Status: domain.StatusComplete}
	_ = r.Create(ctx, f)
	// Other user cannot delete it.
	if err := r.SoftDelete(ctx, other, f.ID); err == nil {
		t.Error("expected not-found for non-owner delete")
	}
	if err := r.SoftDelete(ctx, owner, f.ID); err != nil {
		t.Errorf("owner delete failed: %v", err)
	}
	if _, err := r.Get(ctx, owner, f.ID); err == nil {
		t.Error("expected not-found after soft delete")
	}
}
