package repo

// Phase 17 (UX-33) unit tests for CollectionRepository. Uses an in-memory
// SQLite DB seeded by raw SQL (matching the production schema's columns
// minus Postgres-only types like `uuid` — slug uniqueness is enforced via
// a UNIQUE index, soft-delete is modelled via a deleted_at column the
// repo's WHERE clauses ignore implicitly through GORM's soft-delete).

import (
	"context"
	"testing"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCollectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// SQLite-portable DDL — production runs on Postgres with
	// `uuid DEFAULT gen_random_uuid()`, but SQLite doesn't know that
	// function. The repo never relies on Postgres-only types at the SQL
	// level (it stores UUIDs as TEXT). Tests pass IDs explicitly so the
	// missing default is irrelevant here.
	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			name TEXT,
			name_ru TEXT,
			name_jp TEXT,
			poster_url TEXT,
			episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0,
			deleted_at DATETIME
		)`,
		`CREATE TABLE collections (
			id TEXT PRIMARY KEY,
			slug TEXT,
			title TEXT,
			title_ru TEXT,
			title_jp TEXT,
			description TEXT,
			description_ru TEXT,
			description_jp TEXT,
			cover_image_url TEXT,
			published INTEGER DEFAULT 0,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_collections_slug ON collections (slug)`,
		`CREATE INDEX idx_collections_deleted_at ON collections (deleted_at)`,
		`CREATE INDEX idx_collections_published ON collections (published)`,
		`CREATE TABLE collection_items (
			id TEXT PRIMARY KEY,
			collection_id TEXT,
			anime_id TEXT,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME
		)`,
		`CREATE INDEX idx_collection_items_collection_id ON collection_items (collection_id)`,
		`CREATE INDEX idx_collection_items_anime_id ON collection_items (anime_id)`,
	}
	for _, ddl := range stmts {
		require.NoError(t, db.Exec(ddl).Error)
	}

	return db
}

func seedAnime(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, episodes_count, episodes_aired) VALUES (?, ?, 12, 12)`,
		id, name,
	).Error)
}

func TestCollectionRepository_CreateAndGetByID(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	c := &domain.Collection{
		ID:            "col-1",
		Slug:          "summer-feels",
		Title:         "Summer Feels",
		TitleRU:       "Летнее настроение",
		Description:   "Sun-drenched ones",
		CoverImageURL: "https://cdn.example/cover.jpg",
		Published:     true,
		CreatedBy:     "admin-uuid",
	}
	require.NoError(t, r.Create(ctx, c))

	got, err := r.GetByID(ctx, "col-1")
	require.NoError(t, err)
	assert.Equal(t, "summer-feels", got.Slug)
	assert.Equal(t, "Summer Feels", got.Title)
	assert.Equal(t, "Летнее настроение", got.TitleRU)
	assert.Equal(t, "https://cdn.example/cover.jpg", got.CoverImageURL)
	assert.True(t, got.Published)
	assert.Equal(t, 0, got.ItemCount)
}

func TestCollectionRepository_ListPublishedAndListAdmin(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	// Seed: 1 published, 1 draft.
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "pub", Slug: "pub", Title: "Pub", Published: true,
	}))
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "draft", Slug: "draft", Title: "Draft", Published: false,
	}))

	pub, err := r.ListPublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pub, 1)
	assert.Equal(t, "pub", pub[0].ID)

	all, err := r.ListAdmin(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestCollectionRepository_GetBySlug(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "pub", Slug: "pub-slug", Title: "Pub", Published: true,
	}))
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "draft", Slug: "draft-slug", Title: "Draft", Published: false,
	}))

	got, err := r.GetBySlug(ctx, "pub-slug")
	require.NoError(t, err)
	assert.Equal(t, "pub", got.ID)

	_, err = r.GetBySlug(ctx, "draft-slug")
	require.Error(t, err)
	appErr, ok := liberrors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, liberrors.CodeNotFound, appErr.Code)

	_, err = r.GetBySlug(ctx, "missing")
	require.Error(t, err)
}

func TestCollectionRepository_AddItemIdempotent(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "anime-a", "Anime A")
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "col", Slug: "col", Title: "Col", Published: true,
	}))

	// First add — fresh insert.
	first := &domain.CollectionItem{CollectionID: "col", AnimeID: "anime-a", SortOrder: 1}
	require.NoError(t, r.AddItem(ctx, first))

	// Second add — same (collection_id, anime_id) — should upsert sort_order.
	second := &domain.CollectionItem{CollectionID: "col", AnimeID: "anime-a", SortOrder: 5}
	require.NoError(t, r.AddItem(ctx, second))

	var count int64
	require.NoError(t, db.Model(&domain.CollectionItem{}).
		Where("collection_id = ? AND anime_id = ?", "col", "anime-a").
		Count(&count).Error)
	assert.Equal(t, int64(1), count, "AddItem must be idempotent on (collection_id, anime_id)")

	// Confirm sort_order was updated.
	var got domain.CollectionItem
	require.NoError(t, db.Where("collection_id = ? AND anime_id = ?", "col", "anime-a").First(&got).Error)
	assert.Equal(t, 5, got.SortOrder)
}

func TestCollectionRepository_RemoveItem(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "anime-a", "Anime A")
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "col", Slug: "col", Title: "Col", Published: true,
	}))
	require.NoError(t, r.AddItem(ctx, &domain.CollectionItem{
		CollectionID: "col", AnimeID: "anime-a", SortOrder: 0,
	}))

	require.NoError(t, r.RemoveItem(ctx, "col", "anime-a"))

	err := r.RemoveItem(ctx, "col", "anime-a")
	require.Error(t, err)
	appErr, ok := liberrors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, liberrors.CodeNotFound, appErr.Code)
}

func TestCollectionRepository_Delete(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "col", Slug: "col-slug", Title: "Col", Published: true,
	}))

	require.NoError(t, r.Delete(ctx, "col"))

	_, err := r.GetByID(ctx, "col")
	require.Error(t, err)
	appErr, ok := liberrors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, liberrors.CodeNotFound, appErr.Code)

	all, err := r.ListAdmin(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 0)
}

func TestCollectionRepository_ItemCountPopulated(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "anime-a", "Anime A")
	seedAnime(t, db, "anime-b", "Anime B")
	require.NoError(t, r.Create(ctx, &domain.Collection{
		ID: "col", Slug: "col", Title: "Col", Published: true,
	}))
	require.NoError(t, r.AddItem(ctx, &domain.CollectionItem{
		CollectionID: "col", AnimeID: "anime-a", SortOrder: 0,
	}))
	require.NoError(t, r.AddItem(ctx, &domain.CollectionItem{
		CollectionID: "col", AnimeID: "anime-b", SortOrder: 1,
	}))

	pub, err := r.ListPublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pub, 1)
	assert.Equal(t, 2, pub[0].ItemCount)

	admin, err := r.ListAdmin(ctx)
	require.NoError(t, err)
	require.Len(t, admin, 1)
	assert.Equal(t, 2, admin[0].ItemCount)
}

func TestCollectionRepository_UpdatePartial(t *testing.T) {
	db := setupCollectionTestDB(t)
	r := NewCollectionRepository(db)
	ctx := context.Background()

	c := &domain.Collection{
		ID: "col", Slug: "col", Title: "Original", Published: false,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, r.Create(ctx, c))

	c.Title = "Renamed"
	c.Published = true
	require.NoError(t, r.Update(ctx, c))

	got, err := r.GetByID(ctx, "col")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Title)
	assert.True(t, got.Published)
}
