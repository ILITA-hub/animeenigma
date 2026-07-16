package repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ContentVerification{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpsertUnitAndReadBack(t *testing.T) {
	s := NewStore(testDB(t))
	ctx := context.Background()
	v := domain.UnitVerdict{Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 12,
		Status: domain.StatusVerified, Audio: &domain.AudioVerdict{Lang: "en", Confidence: 0.97, Verified: true}}
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v); err != nil {
		t.Fatal(err)
	}
	// Same key again → replace, not append.
	v.Audio.Lang = "ru"
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v); err != nil {
		t.Fatal(err)
	}
	// Different key → append.
	v2 := domain.UnitVerdict{Key: domain.UnitKey{Server: "hd-2", Category: "dub"}, Status: domain.StatusInconclusive}
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v2); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ByAnime(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Units) != 2 {
		t.Fatalf("rows=%d units=%v", len(rows), rows)
	}
	if rows[0].Units[0].Audio.Lang != "ru" {
		t.Fatalf("replace failed: %+v", rows[0].Units[0])
	}
	got, err := s.Get(ctx, "a-1", "gogoanime")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if miss, err := s.Get(ctx, "a-1", "nineanime"); err != nil || miss != nil {
		t.Fatalf("miss must be nil,nil: %v %v", miss, err)
	}
}
