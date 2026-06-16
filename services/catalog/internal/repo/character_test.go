package repo

// Unit tests for CharacterRepository. Uses an in-memory SQLite DB seeded
// by raw SQL (matching the production schema's columns minus Postgres-only
// types like `uuid DEFAULT gen_random_uuid()` and `CASE … THEN 0` ordering
// which SQLite also supports). Mirrors the established pattern from
// collection_test.go (no build tag, plain `go test ./...`).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func setupCharacterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// SQLite-portable DDL. Production uses Postgres `uuid DEFAULT gen_random_uuid()`;
	// SQLite stores UUIDs as TEXT. Tests supply explicit IDs where needed and
	// rely on GORM's BeforeCreate hook to populate the UUID default for models
	// that define one (Character uses `default:gen_random_uuid()` tag which
	// GORM replaces via its own UUID hook on create).
	stmts := []string{
		`CREATE TABLE animes (
			id   TEXT PRIMARY KEY,
			name TEXT
		)`,
		`CREATE TABLE characters (
			id           TEXT PRIMARY KEY,
			shikimori_id TEXT NOT NULL,
			mal_id       TEXT,
			name         TEXT,
			name_ru      TEXT,
			name_jp      TEXT,
			synonyms     TEXT,
			poster_url   TEXT,
			description  TEXT,
			url          TEXT,
			created_at   DATETIME,
			updated_at   DATETIME,
			deleted_at   DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_characters_shikimori_id ON characters (shikimori_id)`,
		`CREATE INDEX idx_characters_deleted_at ON characters (deleted_at)`,
		`CREATE TABLE anime_characters (
			anime_id     TEXT NOT NULL,
			character_id TEXT NOT NULL,
			role         TEXT,
			position     INTEGER DEFAULT 0,
			created_at   DATETIME,
			PRIMARY KEY (anime_id, character_id)
		)`,
		`CREATE INDEX idx_anime_characters_role ON anime_characters (role)`,
	}
	for _, ddl := range stmts {
		require.NoError(t, db.Exec(ddl).Error)
	}
	return db
}

func seedAnimeForCharTest(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name) VALUES (?, ?)`, id, name).Error)
}

func TestCharacterRepo_UpsertAndGetByAnime_OrdersMainFirst(t *testing.T) {
	db := setupCharacterTestDB(t)
	r := NewCharacterRepository(db)
	ctx := context.Background()

	seedAnimeForCharTest(t, db, "anime-1", "Frieren")

	c0, err := r.UpsertCharacter(ctx, &domain.Character{
		ShikimoriID: "1",
		Name:        "Stark",
		NameRU:      "Штарк",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, c0.ID)
	assert.Equal(t, "Stark", c0.Name)

	c1, err := r.UpsertCharacter(ctx, &domain.Character{
		ShikimoriID: "2",
		Name:        "Frieren",
		NameRU:      "Фрирен",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, c1.ID)

	rows := []domain.AnimeCharacter{
		{AnimeID: "anime-1", CharacterID: c0.ID, Role: "supporting", Position: 0},
		{AnimeID: "anime-1", CharacterID: c1.ID, Role: "main", Position: 1},
	}
	require.NoError(t, r.ReplaceAnimeCharacters(ctx, "anime-1", rows, nil))

	got, err := r.GetByAnimeID(ctx, "anime-1")
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Main character must sort first regardless of insertion order.
	assert.Equal(t, "Frieren", got[0].Name, "first result should be the main character")
	assert.Equal(t, "main", got[0].Role)
	assert.Equal(t, "Stark", got[1].Name, "supporting character should be second")
}

func TestCharacterRepo_UpsertCharacter_IsIdempotent(t *testing.T) {
	db := setupCharacterTestDB(t)
	r := NewCharacterRepository(db)
	ctx := context.Background()

	first, err := r.UpsertCharacter(ctx, &domain.Character{
		ShikimoriID: "99",
		Name:        "Original Name",
		NameRU:      "Оригинал",
	})
	require.NoError(t, err)
	assert.Equal(t, "Original Name", first.Name)

	// Second upsert with same shikimori_id must update fields, not duplicate.
	updated, err := r.UpsertCharacter(ctx, &domain.Character{
		ShikimoriID: "99",
		Name:        "Updated Name",
		NameRU:      "Обновлено",
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, first.ID, updated.ID, "UUID must remain stable across upserts")

	var count int64
	require.NoError(t, db.Model(&domain.Character{}).Where("shikimori_id = ?", "99").Count(&count).Error)
	assert.Equal(t, int64(1), count, "upsert must not create duplicate rows")
}

func TestCharacterRepo_GetByShikimoriID_NotFound(t *testing.T) {
	db := setupCharacterTestDB(t)
	r := NewCharacterRepository(db)
	ctx := context.Background()

	_, err := r.GetByShikimoriID(ctx, "nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestCharacterRepo_ReplaceAnimeCharacters_ClearsOldRows(t *testing.T) {
	db := setupCharacterTestDB(t)
	r := NewCharacterRepository(db)
	ctx := context.Background()

	seedAnimeForCharTest(t, db, "anime-2", "Attack on Titan")

	c0, err := r.UpsertCharacter(ctx, &domain.Character{ShikimoriID: "10", Name: "Eren"})
	require.NoError(t, err)
	c1, err := r.UpsertCharacter(ctx, &domain.Character{ShikimoriID: "11", Name: "Mikasa"})
	require.NoError(t, err)

	// Initial set: two characters.
	rows := []domain.AnimeCharacter{
		{AnimeID: "anime-2", CharacterID: c0.ID, Role: "main", Position: 0},
		{AnimeID: "anime-2", CharacterID: c1.ID, Role: "supporting", Position: 1},
	}
	require.NoError(t, r.ReplaceAnimeCharacters(ctx, "anime-2", rows, nil))

	got, err := r.GetByAnimeID(ctx, "anime-2")
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Replace with only one character — old rows must be cleared.
	reduced := []domain.AnimeCharacter{
		{AnimeID: "anime-2", CharacterID: c0.ID, Role: "main", Position: 0},
	}
	require.NoError(t, r.ReplaceAnimeCharacters(ctx, "anime-2", reduced, nil))

	got2, err := r.GetByAnimeID(ctx, "anime-2")
	require.NoError(t, err)
	require.Len(t, got2, 1, "replace must delete old join rows before inserting new ones")
	assert.Equal(t, "Eren", got2[0].Name)
}

func TestCharacterRepo_ReplaceAnimeCharacters_UpsertsBulkChars(t *testing.T) {
	db := setupCharacterTestDB(t)
	r := NewCharacterRepository(db)
	ctx := context.Background()

	seedAnimeForCharTest(t, db, "anime-3", "Naruto")

	chars := []domain.Character{
		{ShikimoriID: "20", Name: "Naruto"},
		{ShikimoriID: "21", Name: "Sasuke"},
	}
	rows := []domain.AnimeCharacter{
		{AnimeID: "anime-3", CharacterID: "", Role: "main", Position: 0},
		{AnimeID: "anime-3", CharacterID: "", Role: "supporting", Position: 1},
	}

	// ReplaceAnimeCharacters upserts the chars slice first, then joins.
	// We pass the chars to be upserted; rows CharacterIDs will be populated
	// by the caller in real use. For this test we wire manually after upsert.
	require.NoError(t, r.ReplaceAnimeCharacters(ctx, "anime-3", nil, chars))

	// Confirm characters landed in the DB.
	c0, err := r.GetByShikimoriID(ctx, "20")
	require.NoError(t, err)
	assert.Equal(t, "Naruto", c0.Name)

	c1, err := r.GetByShikimoriID(ctx, "21")
	require.NoError(t, err)
	assert.Equal(t, "Sasuke", c1.Name)

	// Now wire the rows with real IDs and replace.
	rows[0].CharacterID = c0.ID
	rows[1].CharacterID = c1.ID
	require.NoError(t, r.ReplaceAnimeCharacters(ctx, "anime-3", rows, nil))

	got, err := r.GetByAnimeID(ctx, "anime-3")
	require.NoError(t, err)
	require.Len(t, got, 2)
	// Naruto is main → first.
	assert.Equal(t, "Naruto", got[0].Name)
	assert.Equal(t, "Sasuke", got[1].Name)
}
