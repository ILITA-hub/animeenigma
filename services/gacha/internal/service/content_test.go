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

func isInvalidInput(err error) bool {
	if err == nil {
		return false
	}
	appErr, ok := err.(*apperrors.AppError)
	return ok && appErr.Code == apperrors.CodeInvalidInput
}
