package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newBannerTestDB creates an in-memory sqlite DB with all 5 gacha tables
// needed for banner tests.
func newBannerTestDB(t *testing.T) *gorm.DB {
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
			id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			art_path    TEXT NOT NULL DEFAULT '',
			is_standard INTEGER NOT NULL DEFAULT 0,
			enabled     INTEGER NOT NULL DEFAULT 0,
			active_from DATETIME,
			active_to   DATETIME,
			sort_order  INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME,
			updated_at  DATETIME,
			deleted_at  DATETIME
		)`,
		`CREATE INDEX idx_gacha_banners_enabled ON gacha_banners(enabled)`,
		`CREATE INDEX idx_gacha_banners_deleted_at ON gacha_banners(deleted_at)`,
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

func seedCard(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	c := &domain.Card{Name: name, Rarity: domain.RarityN, ImagePath: "p"}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("seed card %s: %v", name, err)
	}
	return c.ID
}

func TestBannerCRUD_AndCardSet(t *testing.T) {
	db := newBannerTestDB(t)
	cr := NewContentRepository(db)
	br := NewBannerRepository(db)
	ctx := context.Background()

	c1 := seedCard(t, cr.db, "C1")
	c2 := seedCard(t, cr.db, "C2")
	c3 := seedCard(t, cr.db, "C3")

	// Create banner
	b := &domain.Banner{Name: "Event A", Enabled: true}
	if err := br.CreateBanner(ctx, b); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}
	if b.ID == "" {
		t.Fatal("banner ID must be set after create")
	}

	// GetBanner
	got, err := br.GetBanner(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetBanner: %v", err)
	}
	if got.Name != "Event A" {
		t.Errorf("unexpected name: %s", got.Name)
	}

	// SetCards [c1, c2, c3] — whole set
	if err := br.SetCards(ctx, b.ID, []string{c1, c2, c3}); err != nil {
		t.Fatalf("SetCards: %v", err)
	}
	ids, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids) != 3 {
		t.Errorf("expected 3 cards after SetCards, got %d", len(ids))
	}

	// SetCards [c2] — replaces the whole set
	if err := br.SetCards(ctx, b.ID, []string{c2}); err != nil {
		t.Fatalf("SetCards replace: %v", err)
	}
	ids2, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids2) != 1 || ids2[0] != c2 {
		t.Errorf("expected only c2 after SetCards, got %v", ids2)
	}

	// AddCards [c1] — appends
	if err := br.AddCards(ctx, b.ID, []string{c1}); err != nil {
		t.Fatalf("AddCards: %v", err)
	}
	ids3, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids3) != 2 {
		t.Errorf("expected 2 cards after AddCards, got %d", len(ids3))
	}

	// Duplicate add is a no-op
	if err := br.AddCards(ctx, b.ID, []string{c1}); err != nil {
		t.Fatalf("AddCards duplicate: %v", err)
	}
	ids4, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids4) != 2 {
		t.Errorf("duplicate add must be no-op, got %d cards", len(ids4))
	}

	// UpdateBanner
	got.Description = "updated"
	if err := br.UpdateBanner(ctx, got); err != nil {
		t.Fatalf("UpdateBanner: %v", err)
	}

	// DeleteBanner (soft)
	if err := br.DeleteBanner(ctx, b.ID); err != nil {
		t.Fatalf("DeleteBanner: %v", err)
	}
	_, err = br.GetBanner(ctx, b.ID)
	if err == nil {
		t.Fatal("expected NotFound after soft delete")
	}
}

func TestBannerAddGroupCards(t *testing.T) {
	db := newBannerTestDB(t)
	cr := NewContentRepository(db)
	br := NewBannerRepository(db)
	ctx := context.Background()

	c1 := seedCard(t, cr.db, "C1")
	c2 := seedCard(t, cr.db, "C2")

	// Create group with 2 cards
	g := &domain.Group{Name: "GroupA"}
	if err := cr.CreateGroup(ctx, g); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := cr.AddCardsToGroup(ctx, g.ID, []string{c1, c2}); err != nil {
		t.Fatalf("AddCardsToGroup: %v", err)
	}

	// Create banner
	b := &domain.Banner{Name: "Banner B", Enabled: true}
	if err := br.CreateBanner(ctx, b); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}

	// AddGroupCards pulls both cards in
	if err := br.AddGroupCards(ctx, b.ID, g.ID); err != nil {
		t.Fatalf("AddGroupCards: %v", err)
	}
	ids, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids) != 2 {
		t.Errorf("expected 2 cards after AddGroupCards, got %d", len(ids))
	}

	// Calling again is a no-op (still 2)
	if err := br.AddGroupCards(ctx, b.ID, g.ID); err != nil {
		t.Fatalf("AddGroupCards second call: %v", err)
	}
	ids2, _ := br.BannerCardIDs(ctx, b.ID)
	if len(ids2) != 2 {
		t.Errorf("duplicate AddGroupCards must be no-op, got %d", len(ids2))
	}
}

// TestBannerCardIDs_ExcludesSoftDeletedCards asserts that BannerCardIDs does
// not return IDs for soft-deleted cards.
func TestBannerCardIDs_ExcludesSoftDeletedCards(t *testing.T) {
	db := newBannerTestDB(t)
	cr := NewContentRepository(db)
	br := NewBannerRepository(db)
	ctx := context.Background()

	c1 := seedCard(t, cr.db, "Visible")
	c2 := seedCard(t, cr.db, "Deleted")

	b := &domain.Banner{Name: "SoftDelBanner", Enabled: true}
	if err := br.CreateBanner(ctx, b); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}
	if err := br.SetCards(ctx, b.ID, []string{c1, c2}); err != nil {
		t.Fatalf("SetCards: %v", err)
	}

	// Soft-delete c2.
	if err := cr.DeleteCard(ctx, c2); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}

	ids, err := br.BannerCardIDs(ctx, b.ID)
	if err != nil {
		t.Fatalf("BannerCardIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 card after soft-delete, got %d: %v", len(ids), ids)
	}
	if len(ids) == 1 && ids[0] != c1 {
		t.Errorf("expected c1 (%s), got %s", c1, ids[0])
	}
}

// TestAddGroupCards_ExcludesSoftDeletedCards asserts that AddGroupCards does
// not import soft-deleted group members into the banner pool.
func TestAddGroupCards_ExcludesSoftDeletedCards(t *testing.T) {
	db := newBannerTestDB(t)
	cr := NewContentRepository(db)
	br := NewBannerRepository(db)
	ctx := context.Background()

	c1 := seedCard(t, cr.db, "Keep")
	c2 := seedCard(t, cr.db, "ToDelete")

	g := &domain.Group{Name: "GroupWithDeleted"}
	if err := cr.CreateGroup(ctx, g); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := cr.AddCardsToGroup(ctx, g.ID, []string{c1, c2}); err != nil {
		t.Fatalf("AddCardsToGroup: %v", err)
	}

	// Soft-delete c2 before importing group into banner.
	if err := cr.DeleteCard(ctx, c2); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}

	b := &domain.Banner{Name: "BannerGroupDel", Enabled: true}
	if err := br.CreateBanner(ctx, b); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}
	if err := br.AddGroupCards(ctx, b.ID, g.ID); err != nil {
		t.Fatalf("AddGroupCards: %v", err)
	}

	ids, err := br.BannerCardIDs(ctx, b.ID)
	if err != nil {
		t.Fatalf("BannerCardIDs: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("AddGroupCards must skip soft-deleted card, got %d ids: %v", len(ids), ids)
	}
}

// TestSetCards_DedupesInputIDs asserts that SetCards with a duplicate card ID
// in the input list stores exactly one row per unique card ID and returns no error.
func TestSetCards_DedupesInputIDs(t *testing.T) {
	db := newBannerTestDB(t)
	cr := NewContentRepository(db)
	br := NewBannerRepository(db)
	ctx := context.Background()

	c1 := seedCard(t, cr.db, "C1")
	c2 := seedCard(t, cr.db, "C2")

	b := &domain.Banner{Name: "DedupBanner", Enabled: true}
	if err := br.CreateBanner(ctx, b); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}

	// Pass [c1, c1, c2] — c1 is duplicated.
	if err := br.SetCards(ctx, b.ID, []string{c1, c1, c2}); err != nil {
		t.Fatalf("SetCards with duplicate IDs must not error, got: %v", err)
	}

	ids, err := br.BannerCardIDs(ctx, b.ID)
	if err != nil {
		t.Fatalf("BannerCardIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 unique cards after SetCards([c1,c1,c2]), got %d: %v", len(ids), ids)
	}
}

func TestBannerActiveNow_WindowAndFlags(t *testing.T) {
	db := newBannerTestDB(t)
	br := NewBannerRepository(db)
	ctx := context.Background()

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	wayPast := now.Add(-48 * time.Hour)

	// enabled + no window → ACTIVE
	b1 := &domain.Banner{Name: "B1 no window", Enabled: true, SortOrder: 2}
	if err := br.CreateBanner(ctx, b1); err != nil {
		t.Fatalf("CreateBanner b1: %v", err)
	}

	// enabled + window around now → ACTIVE
	b2 := &domain.Banner{Name: "B2 window active", Enabled: true, ActiveFrom: &past, ActiveTo: &future, SortOrder: 1}
	if err := br.CreateBanner(ctx, b2); err != nil {
		t.Fatalf("CreateBanner b2: %v", err)
	}

	// enabled + window in past → NOT active
	b3 := &domain.Banner{Name: "B3 window past", Enabled: true, ActiveFrom: &wayPast, ActiveTo: &past, SortOrder: 0}
	if err := br.CreateBanner(ctx, b3); err != nil {
		t.Fatalf("CreateBanner b3: %v", err)
	}

	// disabled + no window → NOT active
	b4 := &domain.Banner{Name: "B4 disabled", Enabled: false, SortOrder: 0}
	if err := br.CreateBanner(ctx, b4); err != nil {
		t.Fatalf("CreateBanner b4: %v", err)
	}

	// standard + enabled → ACTIVE, returned FIRST
	b5 := &domain.Banner{Name: "B5 standard", Enabled: true, IsStandard: true, SortOrder: 99}
	if err := br.CreateBanner(ctx, b5); err != nil {
		t.Fatalf("CreateBanner b5: %v", err)
	}

	active, err := br.ActiveNow(ctx, now)
	if err != nil {
		t.Fatalf("ActiveNow: %v", err)
	}
	if len(active) != 3 {
		names := make([]string, len(active))
		for i, a := range active {
			names[i] = a.Name
		}
		t.Fatalf("expected 3 active banners, got %d: %v", len(active), names)
	}

	// First must be the standard one
	if !active[0].IsStandard {
		t.Errorf("first result must be the standard banner, got %s", active[0].Name)
	}

	// Then the two non-standard ones ordered by sort_order ASC: b2 (1) then b1 (2)
	if active[1].Name != "B2 window active" {
		t.Errorf("expected B2 at index 1, got %s", active[1].Name)
	}
	if active[2].Name != "B1 no window" {
		t.Errorf("expected B1 at index 2, got %s", active[2].Name)
	}
}
