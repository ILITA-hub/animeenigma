package service

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newServiceTestDB creates an in-memory sqlite DB with all 5 gacha content tables.
func newServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE gacha_cards (
			id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name        TEXT NOT NULL,
			source_title TEXT NOT NULL DEFAULT '',
			image_path  TEXT NOT NULL,
			back_path   TEXT NOT NULL DEFAULT '',
			rarity      TEXT NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME,
			updated_at  DATETIME,
			deleted_at  DATETIME
		)`,
		`CREATE TABLE gacha_groups (
			id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name       TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE gacha_card_groups (
			group_id TEXT NOT NULL,
			card_id  TEXT NOT NULL,
			UNIQUE(group_id, card_id)
		)`,
		`CREATE TABLE gacha_banners (
			id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name          TEXT NOT NULL,
			description   TEXT NOT NULL DEFAULT '',
			art_path      TEXT NOT NULL DEFAULT '',
			backdrop_path TEXT NOT NULL DEFAULT '',
			is_standard   INTEGER NOT NULL DEFAULT 0,
			enabled       INTEGER NOT NULL DEFAULT 0,
			active_from   DATETIME,
			active_to     DATETIME,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			created_at    DATETIME,
			updated_at    DATETIME,
			deleted_at    DATETIME
		)`,
		`CREATE TABLE gacha_banner_cards (
			banner_id TEXT NOT NULL,
			card_id   TEXT NOT NULL,
			UNIQUE(banner_id, card_id)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup DDL: %v", err)
		}
	}
	return db
}

func newContentService(t *testing.T) *ContentService {
	t.Helper()
	db := newServiceTestDB(t)
	cr := repo.NewContentRepository(db)
	br := repo.NewBannerRepository(db)
	return NewContentService(cr, br)
}

func TestCreateCard_ValidatesNameRarityImage(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	// Empty name → InvalidInput
	_, err := svc.CreateCard(ctx, CreateCardRequest{
		Name:      "",
		Rarity:    domain.RaritySR,
		ImagePath: "cards/foo.webp",
	})
	if !isInvalidInput(err) {
		t.Errorf("empty name: want InvalidInput, got %v", err)
	}

	// Bad rarity → InvalidInput
	_, err = svc.CreateCard(ctx, CreateCardRequest{
		Name:      "Emilia",
		Rarity:    "XX",
		ImagePath: "cards/foo.webp",
	})
	if !isInvalidInput(err) {
		t.Errorf("bad rarity: want InvalidInput, got %v", err)
	}

	// Empty image path → InvalidInput
	_, err = svc.CreateCard(ctx, CreateCardRequest{
		Name:      "Emilia",
		Rarity:    domain.RaritySR,
		ImagePath: "",
	})
	if !isInvalidInput(err) {
		t.Errorf("empty image path: want InvalidInput, got %v", err)
	}

	// Valid → persisted via repo
	card, err := svc.CreateCard(ctx, CreateCardRequest{
		Name:      "Emilia",
		Rarity:    domain.RaritySR,
		ImagePath: "cards/emilia.webp",
	})
	if err != nil {
		t.Fatalf("valid CreateCard: %v", err)
	}
	if card.ID == "" {
		t.Error("ID must be set after create")
	}
}

func TestCreateBanner_DefaultsAndValidation(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	// Empty name → InvalidInput
	_, err := svc.CreateBanner(ctx, CreateBannerRequest{Name: ""})
	if !isInvalidInput(err) {
		t.Errorf("empty name: want InvalidInput, got %v", err)
	}

	// ActiveTo before ActiveFrom → InvalidInput
	from := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour) // before from
	_, err = svc.CreateBanner(ctx, CreateBannerRequest{
		Name:       "Test",
		ActiveFrom: &from,
		ActiveTo:   &to,
	})
	if !isInvalidInput(err) {
		t.Errorf("ActiveTo before ActiveFrom: want InvalidInput, got %v", err)
	}

	// Valid → persisted
	banner, err := svc.CreateBanner(ctx, CreateBannerRequest{
		Name: "Summer Event",
	})
	if err != nil {
		t.Fatalf("valid CreateBanner: %v", err)
	}
	if banner.ID == "" {
		t.Error("banner ID must be set after create")
	}
}

func TestBulkUpdateCards_ValidationAndApply(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	c1, err := svc.CreateCard(ctx, CreateCardRequest{Name: "A", Rarity: domain.RarityN, ImagePath: "cards/a.webp"})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}
	c2, err := svc.CreateCard(ctx, CreateCardRequest{Name: "B", Rarity: domain.RarityN, ImagePath: "cards/b.webp"})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	// Empty ids → InvalidInput
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{}); !isInvalidInput(err) {
		t.Errorf("empty ids: want InvalidInput, got %v", err)
	}

	// Empty set → InvalidInput
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{IDs: []string{c1.ID}}); !isInvalidInput(err) {
		t.Errorf("empty set: want InvalidInput, got %v", err)
	}

	// Empty-string name → InvalidInput (source_title MAY be blanked, name may not)
	empty := ""
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{Name: &empty},
	}); !isInvalidInput(err) {
		t.Errorf("empty name: want InvalidInput, got %v", err)
	}

	// Bad rarity → InvalidInput
	bad := domain.Rarity("XX")
	if _, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID}, Set: BulkCardSet{Rarity: &bad},
	}); !isInvalidInput(err) {
		t.Errorf("bad rarity: want InvalidInput, got %v", err)
	}

	// Valid partial update: rarity+enabled+blank source on both cards; a
	// nonexistent id is silently skipped (affected count reports reality).
	sr := domain.RaritySR
	on := true
	blank := ""
	n, err := svc.BulkUpdateCards(ctx, BulkUpdateCardsRequest{
		IDs: []string{c1.ID, c2.ID, "00000000-0000-0000-0000-000000000000"},
		Set: BulkCardSet{Rarity: &sr, Enabled: &on, SourceTitle: &blank},
	})
	if err != nil {
		t.Fatalf("bulk update: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 rows affected (missing id skipped), got %d", n)
	}

	got, err := svc.GetCard(ctx, c1.ID)
	if err != nil {
		t.Fatalf("get c1: %v", err)
	}
	if got.Rarity != domain.RaritySR || !got.Enabled {
		t.Errorf("c1 not updated: %+v", got)
	}
	if got.Name != "A" {
		t.Errorf("untouched field must survive: name = %q, want A", got.Name)
	}
}

func TestBulkDeleteCards(t *testing.T) {
	svc := newContentService(t)
	ctx := context.Background()

	c1, err := svc.CreateCard(ctx, CreateCardRequest{Name: "A", Rarity: domain.RarityN, ImagePath: "cards/a.webp"})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}
	c2, err := svc.CreateCard(ctx, CreateCardRequest{Name: "B", Rarity: domain.RarityN, ImagePath: "cards/b.webp"})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	if _, err := svc.BulkDeleteCards(ctx, nil); !isInvalidInput(err) {
		t.Errorf("empty ids: want InvalidInput, got %v", err)
	}

	n, err := svc.BulkDeleteCards(ctx, []string{c1.ID, c2.ID})
	if err != nil {
		t.Fatalf("bulk delete: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2 deleted, got %d", n)
	}

	if _, err := svc.GetCard(ctx, c1.ID); err == nil {
		t.Error("c1 must be soft-deleted (GetCard should return NotFound)")
	}
	list, err := svc.ListCards(ctx, repo.CardFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("want empty list after bulk delete, got %d", len(list))
	}
}

func isInvalidInput(err error) bool {
	if err == nil {
		return false
	}
	appErr, ok := err.(*apperrors.AppError)
	return ok && appErr.Code == apperrors.CodeInvalidInput
}
