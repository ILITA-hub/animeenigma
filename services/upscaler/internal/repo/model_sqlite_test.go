package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"gorm.io/gorm"
)

// TestModelRepository_List_Empty verifies that List returns an empty (non-nil)
// slice when no models have been inserted.
func TestModelRepository_List_Empty(t *testing.T) {
	db := openTestDB(t)
	r := NewModelRepository(db)
	ctx := context.Background()

	models, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("List (empty): got %d rows; want 0", len(models))
	}
}

// TestModelRepository_UpsertAndList verifies that upserted models are returned
// by List, ordered by name then version.
func TestModelRepository_UpsertAndList(t *testing.T) {
	db := openTestDB(t)
	r := NewModelRepository(db)
	ctx := context.Background()

	models := []domain.UpscaleModel{
		{Name: "z-model", Version: "1", Checksum: "aaa", ObjectPath: "models/z-model/1.tar"},
		{Name: "a-model", Version: "2", Checksum: "bbb", ObjectPath: "models/a-model/2.tar"},
		{Name: "a-model", Version: "1", Checksum: "ccc", ObjectPath: "models/a-model/1.tar"},
	}
	for i := range models {
		if err := r.Upsert(ctx, &models[i]); err != nil {
			t.Fatalf("Upsert[%d]: %v", i, err)
		}
	}

	got, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List: got %d rows; want 3", len(got))
	}
	// Expect order: a-model/1, a-model/2, z-model/1.
	wantOrder := [][2]string{
		{"a-model", "1"},
		{"a-model", "2"},
		{"z-model", "1"},
	}
	for i, w := range wantOrder {
		if got[i].Name != w[0] || got[i].Version != w[1] {
			t.Errorf("List[%d] = {%q, %q}; want {%q, %q}",
				i, got[i].Name, got[i].Version, w[0], w[1])
		}
	}
}

// TestModelRepository_Upsert_UpdatesExisting verifies the on-conflict update:
// upserting a (name, version) that already exists refreshes checksum + object_path.
func TestModelRepository_Upsert_UpdatesExisting(t *testing.T) {
	db := openTestDB(t)
	r := NewModelRepository(db)
	ctx := context.Background()

	orig := &domain.UpscaleModel{
		Name: "my-model", Version: "1",
		Checksum: "old-checksum", ObjectPath: "models/my-model/1.tar",
	}
	if err := r.Upsert(ctx, orig); err != nil {
		t.Fatalf("Upsert (original): %v", err)
	}

	updated := &domain.UpscaleModel{
		Name: "my-model", Version: "1",
		Checksum: "new-checksum", ObjectPath: "models/my-model/1.tar",
	}
	if err := r.Upsert(ctx, updated); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	got, err := r.Get(ctx, "my-model", "1")
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got.Checksum != "new-checksum" {
		t.Errorf("checksum after upsert = %q; want new-checksum", got.Checksum)
	}

	// List should still contain exactly one row.
	all, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List after upsert: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List after upsert: got %d rows; want 1", len(all))
	}
}

// TestModelRepository_Get_NotFound verifies gorm.ErrRecordNotFound is returned
// for a (name, version) that does not exist.
func TestModelRepository_Get_NotFound(t *testing.T) {
	db := openTestDB(t)
	r := NewModelRepository(db)
	ctx := context.Background()

	_, err := r.Get(ctx, "nonexistent", "1")
	if err == nil {
		t.Fatal("Get(nonexistent): want error, got nil")
	}
	if err != gorm.ErrRecordNotFound {
		t.Errorf("Get(nonexistent): err = %v; want gorm.ErrRecordNotFound", err)
	}
}
