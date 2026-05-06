package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAttributesTestDB creates an in-memory SQLite DB and runs AutoMigrate
// for the full Phase-12 attribute schema (Anime + Genre + Studio + Tag + AnimeTag).
//
// SetupJoinTable is called AFTER AutoMigrate so the explicit AnimeTag join
// model is registered for GORM associations (preserves AnimeTag.Rank — Decision §A4).
//
// Note: sqlite stores the Anime.ID PK as TEXT (the postgres-only
// `default:gen_random_uuid()` clause is ignored), so each test inserts an
// Anime with an explicit ID.
func setupAttributesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&Anime{},
		&Genre{},
		&Studio{},
		&Tag{},
		&AnimeTag{},
	))

	require.NoError(t, db.SetupJoinTable(&Anime{}, "Tags", &AnimeTag{}))
	return db
}

// TestAnimeAttributesSchema_AutoMigrate confirms the Phase-12 attribute
// migrations succeed and the three new string columns (kind / rating /
// material_source) round-trip on insert + select.
func TestAnimeAttributesSchema_AutoMigrate(t *testing.T) {
	db := setupAttributesTestDB(t)

	a := Anime{
		ID:             "anime-1",
		Name:           "Frieren",
		Kind:           "tv",
		Rating:         "pg_13",
		MaterialSource: "manga",
	}
	require.NoError(t, db.Create(&a).Error)

	var got Anime
	require.NoError(t, db.First(&got, "id = ?", "anime-1").Error)
	assert.Equal(t, "tv", got.Kind)
	assert.Equal(t, "pg_13", got.Rating)
	assert.Equal(t, "manga", got.MaterialSource)
}

// TestAnimeAttributesSchema_StudioAssociation confirms the new anime_studios
// m2m join works end-to-end via GORM associations.
func TestAnimeAttributesSchema_StudioAssociation(t *testing.T) {
	db := setupAttributesTestDB(t)

	a := Anime{ID: "anime-2", Name: "Hunter x Hunter"}
	require.NoError(t, db.Create(&a).Error)

	studio := Studio{ID: "1", Name: "Madhouse"}
	require.NoError(t, db.Create(&studio).Error)

	require.NoError(t, db.Model(&a).Association("Studios").Append(&studio))

	var loaded Anime
	require.NoError(t, db.Preload("Studios").First(&loaded, "id = ?", "anime-2").Error)
	require.Len(t, loaded.Studios, 1)
	assert.Equal(t, "Madhouse", loaded.Studios[0].Name)
}

// TestAnimeAttributesSchema_TagAssociationWithRank confirms the explicit
// AnimeTag join model preserves the Rank column for v2.1 use (Decision §A4).
func TestAnimeAttributesSchema_TagAssociationWithRank(t *testing.T) {
	db := setupAttributesTestDB(t)

	a := Anime{ID: "anime-3", Name: "Spice and Wolf"}
	require.NoError(t, db.Create(&a).Error)

	tag := Tag{ID: "slice_of_life", Name: "Slice of Life", Source: "anilist"}
	require.NoError(t, db.Create(&tag).Error)

	// Insert the join row directly via the AnimeTag model so we can set Rank.
	join := AnimeTag{AnimeID: "anime-3", TagID: "slice_of_life", Rank: 85}
	require.NoError(t, db.Create(&join).Error)

	var loaded Anime
	require.NoError(t, db.Preload("Tags").First(&loaded, "id = ?", "anime-3").Error)
	require.Len(t, loaded.Tags, 1)
	assert.Equal(t, "Slice of Life", loaded.Tags[0].Name)

	// Confirm Rank persisted on the join row (v2.1 will use this).
	var rank int
	require.NoError(t, db.Raw(
		`SELECT rank FROM anime_tags WHERE anime_id = ? AND tag_id = ?`,
		"anime-3", "slice_of_life",
	).Scan(&rank).Error)
	assert.Equal(t, 85, rank)
}

// TestAnimeAttributesSchema_AnimeTagCompositeKey confirms the composite PK
// (AnimeID, TagID) prevents duplicate joins but allows two distinct tags
// for the same anime.
func TestAnimeAttributesSchema_AnimeTagCompositeKey(t *testing.T) {
	db := setupAttributesTestDB(t)

	require.NoError(t, db.Create(&Anime{ID: "anime-4", Name: "Steins;Gate"}).Error)
	require.NoError(t, db.Create(&Tag{ID: "time_travel", Name: "Time Travel"}).Error)
	require.NoError(t, db.Create(&Tag{ID: "psychological", Name: "Psychological"}).Error)

	require.NoError(t, db.Create(&AnimeTag{AnimeID: "anime-4", TagID: "time_travel", Rank: 90}).Error)

	// Duplicate (anime_id, tag_id) must fail the composite primary key.
	err := db.Create(&AnimeTag{AnimeID: "anime-4", TagID: "time_travel", Rank: 99}).Error
	assert.Error(t, err, "duplicate (anime_id, tag_id) must violate the composite PK")

	// Same anime, different tag — must succeed.
	require.NoError(t, db.Create(&AnimeTag{AnimeID: "anime-4", TagID: "psychological", Rank: 80}).Error)
}

// TestAnimeAttributesSchema_HasTables confirms AutoMigrate created the
// expected join tables — the schema half of Phase-12 SC#3.
func TestAnimeAttributesSchema_HasTables(t *testing.T) {
	db := setupAttributesTestDB(t)
	mig := db.Migrator()

	assert.True(t, mig.HasTable("studios"), "studios table must exist")
	assert.True(t, mig.HasTable("tags"), "tags table must exist")
	assert.True(t, mig.HasTable("anime_studios"), "anime_studios join table must exist")
	assert.True(t, mig.HasTable("anime_tags"), "anime_tags join table must exist")
}
