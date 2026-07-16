package repo

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func newPersonRoleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// SQLite-portable DDL, not db.AutoMigrate(&domain.AnimePersonRole{}):
	// production runs on Postgres with `uuid DEFAULT gen_random_uuid()`, but
	// SQLite's CREATE TABLE grammar rejects a bare function-call DEFAULT
	// (`near "(": syntax error`) — the same GORM/SQLite incompatibility
	// documented in character_test.go, collection_test.go and
	// anime_update_test.go in this package. ReplaceAnimeStaff assigns IDs
	// Go-side when blank, so the missing DEFAULT is irrelevant to the test.
	stmts := []string{
		`CREATE TABLE anime_person_roles (
			id                  TEXT PRIMARY KEY,
			anime_id            TEXT,
			shikimori_person_id TEXT,
			name                TEXT,
			name_ru             TEXT,
			name_jp             TEXT,
			poster_url          TEXT,
			role                TEXT,
			role_ru             TEXT,
			is_producer         BOOLEAN,
			is_mangaka          BOOLEAN,
			position            INTEGER DEFAULT 0,
			created_at          DATETIME,
			updated_at          DATETIME
		)`,
		`CREATE INDEX idx_person_roles_anime ON anime_person_roles (anime_id)`,
		`CREATE INDEX idx_anime_person_roles_shikimori_person_id ON anime_person_roles (shikimori_person_id)`,
		`CREATE INDEX idx_anime_person_roles_role ON anime_person_roles (role)`,
	}
	for _, ddl := range stmts {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	return db
}

func TestReplaceAnimeStaff_ReplacesAndOrders(t *testing.T) {
	db := newPersonRoleDB(t)
	r := NewPersonRoleRepository(db)
	ctx := context.Background()
	const animeID = "11111111-1111-1111-1111-111111111111"

	// First write: two rows.
	rows := []domain.AnimePersonRole{
		{AnimeID: animeID, ShikimoriPersonID: "1", Name: "B Person", Role: "Script", Position: 3},
		{AnimeID: animeID, ShikimoriPersonID: "2", Name: "A Person", Role: "Director", Position: 0},
	}
	if err := r.ReplaceAnimeStaff(ctx, animeID, rows); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := r.GetStaffByAnimeID(ctx, animeID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	// Ordered by position ASC → Director (0) before Script (3).
	if got[0].Role != "Director" || got[1].Role != "Script" {
		t.Fatalf("bad order: %s, %s", got[0].Role, got[1].Role)
	}

	// Second write REPLACES (not appends): one row.
	if err := r.ReplaceAnimeStaff(ctx, animeID, []domain.AnimePersonRole{
		{AnimeID: animeID, ShikimoriPersonID: "9", Name: "Solo", Role: "Music", Position: 0},
	}); err != nil {
		t.Fatalf("replace2: %v", err)
	}
	got, _ = r.GetStaffByAnimeID(ctx, animeID)
	if len(got) != 1 || got[0].Role != "Music" {
		t.Fatalf("replace did not overwrite: %+v", got)
	}
}
