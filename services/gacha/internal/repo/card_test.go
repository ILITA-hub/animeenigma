package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newContentTestDB creates an in-memory sqlite DB with gacha_cards,
// gacha_groups, and gacha_card_groups tables (raw DDL — avoids Postgres-only
// gen_random_uuid()).
func newContentTestDB(t *testing.T) *gorm.DB {
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
			rarity      TEXT NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME,
			updated_at  DATETIME,
			deleted_at  DATETIME
		)`,
		`CREATE INDEX idx_gacha_cards_rarity ON gacha_cards(rarity)`,
		`CREATE INDEX idx_gacha_cards_enabled ON gacha_cards(enabled)`,
		`CREATE INDEX idx_gacha_cards_deleted_at ON gacha_cards(deleted_at)`,
		`CREATE TABLE gacha_groups (
			id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name       TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_gacha_groups_name ON gacha_groups(name)`,
		`CREATE TABLE gacha_card_groups (
			group_id TEXT NOT NULL,
			card_id  TEXT NOT NULL,
			UNIQUE(group_id, card_id)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup DDL: %v", err)
		}
	}
	return db
}

func TestCardCRUD_CreateGetUpdateSoftDelete(t *testing.T) {
	db := newContentTestDB(t)
	r := NewContentRepository(db)
	ctx := context.Background()

	// Create
	c := &domain.Card{
		Name:      "Rem",
		Rarity:    domain.RaritySR,
		ImagePath: "cards/rem.webp",
		Enabled:   false,
	}
	if err := r.CreateCard(ctx, c); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if c.ID == "" {
		t.Fatal("ID must be set after create")
	}

	// Get
	got, err := r.GetCard(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if got.Name != "Rem" || got.Rarity != domain.RaritySR || got.Enabled {
		t.Errorf("unexpected card: %+v", got)
	}

	// Update rarity + enabled
	got.Rarity = domain.RaritySSR
	got.Enabled = true
	if err := r.UpdateCard(ctx, got); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	got2, err := r.GetCard(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetCard after update: %v", err)
	}
	if got2.Rarity != domain.RaritySSR || !got2.Enabled {
		t.Errorf("unexpected card after update: %+v", got2)
	}

	// Soft delete → invisible to Get
	if err := r.DeleteCard(ctx, c.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	_, err = r.GetCard(ctx, c.ID)
	if err == nil {
		t.Fatal("expected NotFound after soft delete")
	}
	var appErr *apperrors.AppError
	if ok := isNotFound(err); !ok {
		_ = appErr
		t.Errorf("expected NotFound error, got: %v", err)
	}

	// Soft deleted card must not appear in List
	cards, err := r.ListCards(ctx, CardFilter{})
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if len(cards) != 0 {
		t.Errorf("soft-deleted card must be invisible to List, got %d", len(cards))
	}
}

func TestCardList_FiltersByRarityEnabledAndGroup(t *testing.T) {
	db := newContentTestDB(t)
	r := NewContentRepository(db)
	ctx := context.Background()

	cards := []*domain.Card{
		{Name: "C1", Rarity: domain.RarityN, ImagePath: "p1", Enabled: false},
		{Name: "C2", Rarity: domain.RarityR, ImagePath: "p2", Enabled: true},
		{Name: "C3", Rarity: domain.RaritySR, ImagePath: "p3", Enabled: true},
		{Name: "C4", Rarity: domain.RaritySSR, ImagePath: "p4", Enabled: false},
	}
	for _, c := range cards {
		if err := r.CreateCard(ctx, c); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}

	// Create a group containing C1 and C3
	g := &domain.Group{Name: "TestGroup"}
	if err := r.CreateGroup(ctx, g); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := r.AddCardsToGroup(ctx, g.ID, []string{cards[0].ID, cards[2].ID}); err != nil {
		t.Fatalf("AddCardsToGroup: %v", err)
	}

	// Filter by rarity SR → 1
	bySR, err := r.ListCards(ctx, CardFilter{Rarity: domain.RaritySR})
	if err != nil {
		t.Fatalf("ListCards SR: %v", err)
	}
	if len(bySR) != 1 {
		t.Errorf("expected 1 SR card, got %d", len(bySR))
	}

	// Filter by enabled → 2
	trueVal := true
	byEnabled, err := r.ListCards(ctx, CardFilter{Enabled: &trueVal})
	if err != nil {
		t.Fatalf("ListCards enabled: %v", err)
	}
	if len(byEnabled) != 2 {
		t.Errorf("expected 2 enabled cards, got %d", len(byEnabled))
	}

	// Filter by group → 2
	byGroup, err := r.ListCards(ctx, CardFilter{GroupID: g.ID})
	if err != nil {
		t.Fatalf("ListCards group: %v", err)
	}
	if len(byGroup) != 2 {
		t.Errorf("expected 2 cards in group, got %d", len(byGroup))
	}

	// Empty filter → 4
	all, err := r.ListCards(ctx, CardFilter{})
	if err != nil {
		t.Fatalf("ListCards all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 cards, got %d", len(all))
	}
}

func TestGroupCRUD_AndMembership(t *testing.T) {
	db := newContentTestDB(t)
	r := NewContentRepository(db)
	ctx := context.Background()

	// Create two cards
	c1 := &domain.Card{Name: "C1", Rarity: domain.RarityN, ImagePath: "p1"}
	c2 := &domain.Card{Name: "C2", Rarity: domain.RarityR, ImagePath: "p2"}
	for _, c := range []*domain.Card{c1, c2} {
		if err := r.CreateCard(ctx, c); err != nil {
			t.Fatalf("CreateCard: %v", err)
		}
	}

	// Create group
	g := &domain.Group{Name: "Alpha"}
	if err := r.CreateGroup(ctx, g); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	// Rename
	if err := r.RenameGroup(ctx, g.ID, "Beta"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	groups, _ := r.ListGroups(ctx)
	if len(groups) != 1 || groups[0].Name != "Beta" {
		t.Errorf("rename failed: %+v", groups)
	}

	// AddCards [c1, c2]
	if err := r.AddCardsToGroup(ctx, g.ID, []string{c1.ID, c2.ID}); err != nil {
		t.Fatalf("AddCardsToGroup: %v", err)
	}
	ids, err := r.GroupCardIDs(ctx, g.ID)
	if err != nil {
		t.Fatalf("GroupCardIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 cards in group, got %d", len(ids))
	}

	// Add c1 again — idempotent (ON CONFLICT DO NOTHING), count stays 2
	if err := r.AddCardsToGroup(ctx, g.ID, []string{c1.ID}); err != nil {
		t.Fatalf("AddCardsToGroup duplicate: %v", err)
	}
	ids2, _ := r.GroupCardIDs(ctx, g.ID)
	if len(ids2) != 2 {
		t.Errorf("duplicate add must be no-op, got %d", len(ids2))
	}

	// Remove c1 → 1 left
	if err := r.RemoveCardFromGroup(ctx, g.ID, c1.ID); err != nil {
		t.Fatalf("RemoveCardFromGroup: %v", err)
	}
	ids3, _ := r.GroupCardIDs(ctx, g.ID)
	if len(ids3) != 1 || ids3[0] != c2.ID {
		t.Errorf("expected only c2 after remove, got %v", ids3)
	}

	// DeleteGroup removes group AND its join rows
	if err := r.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	groups2, _ := r.ListGroups(ctx)
	if len(groups2) != 0 {
		t.Errorf("group must be deleted, got %v", groups2)
	}
	// Verify join rows gone
	ids4, _ := r.GroupCardIDs(ctx, g.ID)
	if len(ids4) != 0 {
		t.Errorf("orphan join rows after group delete: %v", ids4)
	}
}

// isNotFound checks if an error is a NotFound app error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	appErr, ok := err.(*apperrors.AppError)
	return ok && appErr.Code == apperrors.CodeNotFound
}
