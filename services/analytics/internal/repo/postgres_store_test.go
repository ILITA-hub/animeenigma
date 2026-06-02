package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := EnsureView(db); err != nil {
		t.Fatalf("view: %v", err)
	}
	return db
}

func TestInsertBatch(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	events := []domain.Event{
		{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now()},
		{EventID: "e2", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now(), ElSelector: "button#buy"},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	var count int64
	db.Model(&Event{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestInsertBatch_Empty(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	if err := store.InsertBatch(context.Background(), nil); err != nil {
		t.Fatalf("empty batch must be a no-op, got %v", err)
	}
}

func TestUpsertIdentity(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	if err := store.UpsertIdentity(ctx, "a1", "u1", time.Now()); err != nil {
		t.Fatalf("UpsertIdentity: %v", err)
	}
	var count int64
	db.Model(&Identity{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 identity row, got %d", count)
	}
}
