package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

func TestResolvePerson_StitchesAfterIdentify(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	// Anonymous event, then the visitor identifies.
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now()},
	})
	_ = store.UpsertIdentity(ctx, "a1", "u1", time.Now())

	person, err := ResolvePerson(ctx, db, "e1")
	if err != nil {
		t.Fatalf("ResolvePerson: %v", err)
	}
	if person != "u1" {
		t.Fatalf("expected anonymous event to resolve to u1, got %q", person)
	}
}

func TestEraseByUserID(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()
	uid := "u1"
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "e1", EventType: domain.EventTypeClick, AnonymousID: "a1", UserID: uid, SessionID: "s1", Timestamp: time.Now()},
	})
	_ = store.UpsertIdentity(ctx, "a1", uid, time.Now())

	if err := EraseByUserID(ctx, db, uid); err != nil {
		t.Fatalf("EraseByUserID: %v", err)
	}
	var ev, id int64
	db.Model(&Event{}).Count(&ev)
	db.Model(&Identity{}).Count(&id)
	if ev != 0 || id != 0 {
		t.Fatalf("erase left rows: events=%d identities=%d", ev, id)
	}
}

func TestPurgeOlderThan(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now()
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "old", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: old},
		{EventID: "new", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: recent},
	})

	n, err := PurgeOlderThan(ctx, db, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}
	var count int64
	db.Model(&Event{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row remaining, got %d", count)
	}
}
