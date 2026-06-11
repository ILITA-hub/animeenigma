package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupS5TestDB creates an in-memory SQLite DB with the full Phase-12 schema
// S5 needs: animes (with kind / rating / material_source columns added by
// Phase 12 Wave 1), anime_genres + anime_studios + anime_tags (m2m join
// tables), watch_history (the time-weighted history input), rec_user_signals
// (where Precompute persists s5_affinity).
func setupS5TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY,
		kind TEXT DEFAULT '',
		rating TEXT DEFAULT '',
		material_source TEXT DEFAULT '',
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE studios (
		id TEXT PRIMARY KEY,
		name TEXT
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT NOT NULL,
		studio_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, studio_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE tags (
		id TEXT PRIMARY KEY,
		name TEXT,
		source TEXT
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_tags (
		anime_id TEXT NOT NULL,
		tag_id TEXT NOT NULL,
		rank INTEGER DEFAULT 0,
		PRIMARY KEY (anime_id, tag_id)
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE watch_history (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		episode_number INTEGER NOT NULL DEFAULT 0,
		player TEXT NOT NULL,
		duration_watched INTEGER NOT NULL DEFAULT 0,
		watched_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE rec_user_signals (
		user_id TEXT PRIMARY KEY,
		s1_vector TEXT NOT NULL DEFAULT '{}',
		s5_affinity TEXT NOT NULL DEFAULT '{}',
		s6_seed_anime_id TEXT,
		s6_seed_completed_at DATETIME,
		s6_seed_score INTEGER,
		last_computed DATETIME NOT NULL
	)`).Error)
	return db
}

func seedS5Anime(t *testing.T, db *gorm.DB, id, kind, rating, source string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, kind, rating, material_source, hidden) VALUES (?, ?, ?, ?, 0)`,
		id, kind, rating, source,
	).Error)
}

func seedS5Genre(t *testing.T, db *gorm.DB, animeID, genreID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)`, animeID, genreID,
	).Error)
}

func seedS5Studio(t *testing.T, db *gorm.DB, animeID, studioID string) {
	t.Helper()
	// Insert into studios first (idempotent) then into anime_studios.
	_ = db.Exec(`INSERT OR IGNORE INTO studios (id, name) VALUES (?, ?)`, studioID, studioID).Error
	require.NoError(t, db.Exec(
		`INSERT INTO anime_studios (anime_id, studio_id) VALUES (?, ?)`, animeID, studioID,
	).Error)
}

func seedS5Tag(t *testing.T, db *gorm.DB, animeID, tagID string) {
	t.Helper()
	_ = db.Exec(`INSERT OR IGNORE INTO tags (id, name, source) VALUES (?, ?, 'anilist')`, tagID, tagID).Error
	require.NoError(t, db.Exec(
		`INSERT INTO anime_tags (anime_id, tag_id, rank) VALUES (?, ?, 0)`, animeID, tagID,
	).Error)
}

func seedS5History(t *testing.T, db *gorm.DB, rowID, userID, animeID, player string, durationWatched int) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, episode_number, player, duration_watched) VALUES (?, ?, ?, 1, ?, ?)`,
		rowID, userID, animeID, player, durationWatched,
	).Error)
}

func TestS5Attribute_ID(t *testing.T) {
	db := setupS5TestDB(t)
	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	assert.Equal(t, recs.SignalID("s5"), s5.ID())
}

func TestS5Attribute_ColdStart_NoHistory(t *testing.T) {
	db := setupS5TestDB(t)
	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)

	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row, "Precompute must persist a row even on cold-start")
	assert.Equal(t, "{}", row.S5Affinity, "cold-start must persist empty JSON object")

	got, err := s5.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A"})
	require.NoError(t, err)
	assert.Empty(t, got, "cold-start Score returns empty map (not nil, not NaN)")
}

func TestS5Attribute_ColdStart_NoRow(t *testing.T) {
	db := setupS5TestDB(t)
	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)

	got, err := s5.Score(context.Background(), "user-no-row", []recs.AnimeID{"anime-A"})
	require.NoError(t, err)
	assert.Empty(t, got, "no rec_user_signals row -> empty map")
}

func TestS5Attribute_KodikFallback(t *testing.T) {
	db := setupS5TestDB(t)

	// 2 anime, both Madhouse. anime-A watched 3x via Kodik (duration_watched=0
	// because Kodik writes 0). anime-B watched once via Kodik, 1500s = 25min.
	// Expected: total_units = 3 (Kodik integer-episode) + 25 (Kodik min) = 28.
	// studio:Madhouse contributes from BOTH anime → tf = 28/28 = 1.0.
	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Anime(t, db, "anime-B", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-A", "Madhouse")
	seedS5Studio(t, db, "anime-B", "Madhouse")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 0)
	seedS5History(t, db, "wh-2", "user-1", "anime-A", "kodik", 0)
	seedS5History(t, db, "wh-3", "user-1", "anime-A", "kodik", 0)
	seedS5History(t, db, "wh-4", "user-1", "anime-B", "kodik", 1500)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row)

	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))

	// Kodik fallback: each row = 1 unit. With 1 user, IDF = log(1/(1+1)) =
	// -0.693, so per-attribute affinity is NEGATIVE (legitimate — universal
	// attributes have negative IDF, and Score's normalizer downstream treats
	// negative-raw entries as zero contribution). The test asserts non-zero
	// magnitude — what matters is that the Kodik branch contributed units.
	assert.NotZero(t, aff["studio:Madhouse"], "studio:Madhouse must have non-zero affinity")
	assert.Greater(t, math.Abs(aff["kind:tv"]), 0.0, "kind:tv populated from both Kodik+Kodik anime")
}

func TestS5Attribute_KodikFallback_DistinguishedFromZero(t *testing.T) {
	db := setupS5TestDB(t)

	// Direct Kodik-vs-zero-duration distinction: one Kodik row with
	// duration_watched=0 must produce 1 unit (NOT 0).
	// Setup: user-1 has ONLY one watch_history row, Kodik, duration=0.
	// If Kodik branch is correct, total_units=1 and the studio gets non-zero
	// affinity. If Kodik branch falls through to max(0/60,1)=1 fortuitously,
	// the same. But if a regression sets unit=duration/60 (no floor), then
	// total_units=0 and Precompute writes empty {}.
	// Distinguishing test: a Kodik-only fixture must produce a non-empty
	// affinity vector.
	seedS5Anime(t, db, "anime-K", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-K", "MAPPA")
	seedS5History(t, db, "wh-1", "user-1", "anime-K", "kodik", 0)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row)

	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))
	assert.NotEmpty(t, aff, "Kodik-only history must produce a non-empty affinity vector (unit fallback to 1)")
	_, hasStudio := aff["studio:MAPPA"]
	assert.True(t, hasStudio, "studio:MAPPA must be present from the Kodik-only history")
}

func TestS5Attribute_DurationFloor(t *testing.T) {
	db := setupS5TestDB(t)

	// Kodik with duration_watched=30s = 0.5 min. Floor → 1 min unit.
	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-A", "Madhouse")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 30)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))
	assert.NotEmpty(t, aff, "duration floor must give the row at least 1 unit -> non-empty vector")
}

func TestS5Attribute_TFNormalization(t *testing.T) {
	db := setupS5TestDB(t)

	// 3 anime: A, B share studio "Shared"; C has studio "Other".
	// All Kodik, 60s = 1 min each → 3 history rows, 3 total units.
	// tf(studio:Shared) = (1+1)/3 = 0.667
	// tf(studio:Other)  = 1/3 = 0.333
	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Anime(t, db, "anime-B", "tv", "pg_13", "manga")
	seedS5Anime(t, db, "anime-C", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-A", "Shared")
	seedS5Studio(t, db, "anime-B", "Shared")
	seedS5Studio(t, db, "anime-C", "Other")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 60)
	seedS5History(t, db, "wh-2", "user-1", "anime-B", "kodik", 60)
	seedS5History(t, db, "wh-3", "user-1", "anime-C", "kodik", 60)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))

	// With 1 user in watch_history, IDF = log(1/(1+1)) = log(0.5) ≈ -0.693.
	// Shared affinity = (2/3) * -0.693 ≈ -0.4621
	// Other  affinity = (1/3) * -0.693 ≈ -0.2310
	// Ratio Shared/Other = 2.0 (the tf ratio carries through).
	assert.NotZero(t, aff["studio:Shared"])
	assert.NotZero(t, aff["studio:Other"])
	ratio := aff["studio:Shared"] / aff["studio:Other"]
	assert.InDelta(t, 2.0, ratio, 0.01, "tf ratio = 2:1 (Shared appears twice, Other once)")
}

func TestS5Attribute_IDFAcrossUsers(t *testing.T) {
	db := setupS5TestDB(t)

	// 5 users, multiple anime, mixed studios.
	// "Madhouse" appears in everyone's history → high user_count → low IDF (closer to 0).
	// "RareStudio" appears only in user-1's history → low user_count → high IDF.
	// Expected: rare studio has higher (more negative or less negative) IDF magnitude.
	seedS5Anime(t, db, "anime-Mad", "tv", "pg_13", "manga")
	seedS5Anime(t, db, "anime-Rare", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-Mad", "Madhouse")
	seedS5Studio(t, db, "anime-Rare", "RareStudio")

	for i, u := range []string{"user-1", "user-2", "user-3", "user-4", "user-5"} {
		// Everyone watched Madhouse anime
		seedS5History(t, db, "wh-mad-"+u, u, "anime-Mad", "kodik", 60)
		// Only user-1 watched the rare studio's anime.
		if i == 0 {
			seedS5History(t, db, "wh-rare-"+u, u, "anime-Rare", "kodik", 60)
		}
	}

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))

	// IDF(Madhouse) = log(5/(1+5)) = log(0.833) ≈ -0.182
	// IDF(RareStudio) = log(5/(1+1)) = log(2.5) ≈ 0.916
	// Rare studio has POSITIVE IDF (more discriminative); common one is NEGATIVE.
	// |aff(rare)| > |aff(common)| since user-1's tf for both is 0.5 (each is half their units).
	assert.Greater(t, aff["studio:RareStudio"], aff["studio:Madhouse"],
		"rare studio gets higher (more positive) IDF than the universally-popular one")
}

func TestS5Attribute_AllSixDimensions(t *testing.T) {
	db := setupS5TestDB(t)

	// Single anime with all 6 dimensions populated.
	seedS5Anime(t, db, "anime-Full", "tv", "pg_13", "manga")
	seedS5Genre(t, db, "anime-Full", "action")
	seedS5Studio(t, db, "anime-Full", "Madhouse")
	seedS5Tag(t, db, "anime-Full", "shounen")
	seedS5History(t, db, "wh-1", "user-1", "anime-Full", "kodik", 1500)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))

	prefixes := map[string]bool{"tag:": false, "studio:": false, "genre:": false, "rating:": false, "source:": false, "kind:": false}
	for k := range aff {
		for p := range prefixes {
			if strings.HasPrefix(k, p) {
				prefixes[p] = true
			}
		}
	}
	for p, present := range prefixes {
		assert.True(t, present, "expected at least one key with prefix %q in affinity vector", p)
	}
}

func TestS5Attribute_ScorePerAttributeWeights(t *testing.T) {
	db := setupS5TestDB(t)

	// Build a fixture where Score produces a known final value on a single
	// candidate. The user's affinity vector is hand-crafted via a direct
	// rec_user_signals INSERT — this isolates the Score path's per-attribute
	// weight math from the Precompute path's TF-IDF math.
	seedS5Anime(t, db, "cand-1", "tv", "pg_13", "manga")
	seedS5Genre(t, db, "cand-1", "g1")
	seedS5Studio(t, db, "cand-1", "s1")
	seedS5Tag(t, db, "cand-1", "t1")

	// Hand-crafted affinity: all six attributes the candidate has → 1.0 each.
	aff := map[string]float64{
		"tag:t1":      1.0,
		"studio:s1":   1.0,
		"genre:g1":    1.0,
		"rating:pg_13": 1.0,
		"source:manga": 1.0,
		"kind:tv":     1.0,
	}
	affJSON, err := json.Marshal(aff)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", "{}", string(affJSON),
	).Error)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	got, err := s5.Score(context.Background(), "user-1", []recs.AnimeID{"cand-1"})
	require.NoError(t, err)

	// Sum of locked weights (Decision §A2):
	// tag 0.30 + studio 0.25 + genre 0.15 + demographic 0.10 + source 0.10 + kind 0.10 = 1.00
	assert.InDelta(t, 1.00, float64(got["cand-1"]), 0.0001,
		"with all 6 attributes at affinity 1.0 each, S5_raw must equal sum of weights = 1.00")
}

func TestS5Attribute_MissingAttributeContributesZero(t *testing.T) {
	db := setupS5TestDB(t)

	// Candidate has a tag but no studios / no genres / no rating / no source / no kind.
	// Score must contribute the tag weight only, with no NaN and no error.
	seedS5Anime(t, db, "cand-thin", "", "", "") // empty single-value attrs
	seedS5Tag(t, db, "cand-thin", "t1")

	aff := map[string]float64{
		"tag:t1": 1.0,
	}
	affJSON, _ := json.Marshal(aff)
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", "{}", string(affJSON),
	).Error)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	got, err := s5.Score(context.Background(), "user-1", []recs.AnimeID{"cand-thin"})
	require.NoError(t, err)
	// Expect 0.30 * 1.0 = 0.30 (tag dimension only).
	assert.InDelta(t, 0.30, float64(got["cand-thin"]), 0.0001,
		"missing attribute contributes 0 — only the tag dimension fires")
}

func TestS5Attribute_KeyFormatColonSeparated(t *testing.T) {
	db := setupS5TestDB(t)

	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Genre(t, db, "anime-A", "g1")
	seedS5Studio(t, db, "anime-A", "s1")
	seedS5Tag(t, db, "anime-A", "t1")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 60)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	var aff map[string]float64
	require.NoError(t, json.Unmarshal([]byte(row.S5Affinity), &aff))

	// All keys must match ^(tag|studio|genre|rating|source|kind):.+$
	allowedPrefixes := []string{"tag:", "studio:", "genre:", "rating:", "source:", "kind:"}
	for k := range aff {
		matched := false
		for _, p := range allowedPrefixes {
			if strings.HasPrefix(k, p) && len(k) > len(p) {
				matched = true
				break
			}
		}
		assert.True(t, matched, "key %q must use the locked {dim}:{attr_id} colon-separated format", k)
	}
}

func TestS5Attribute_NoNaN_NoInf_NoNegative(t *testing.T) {
	db := setupS5TestDB(t)

	// 50-anime / 5-user fixture. Build a cross-product so the IDF and TF
	// values are non-trivial.
	for i := 0; i < 50; i++ {
		id := s5RandomID("anime", i)
		kind := []string{"tv", "movie", "ova", "ona", "special"}[i%5]
		rating := []string{"g", "pg", "pg_13", "r", "r_plus"}[i%5]
		source := []string{"manga", "novel", "original", "light_novel", "game"}[i%5]
		seedS5Anime(t, db, id, kind, rating, source)
		seedS5Genre(t, db, id, "genre-"+s5RandomID("g", i%7))
		seedS5Studio(t, db, id, "studio-"+s5RandomID("s", i%4))
		seedS5Tag(t, db, id, "tag-"+s5RandomID("t", i%9))
	}
	for u := 0; u < 5; u++ {
		userID := s5RandomID("user", u)
		for k := 0; k < 8; k++ {
			animeID := s5RandomID("anime", (u*8+k)%50)
			seedS5History(t, db, s5RandomID("wh", u*100+k), userID, animeID, "kodik", 60+(k*30))
		}
	}

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), s5RandomID("user", 0)))

	candidates := make([]recs.AnimeID, 50)
	for i := 0; i < 50; i++ {
		candidates[i] = recs.AnimeID(s5RandomID("anime", i))
	}
	got, err := s5.Score(context.Background(), s5RandomID("user", 0), candidates)
	require.NoError(t, err)

	for id, v := range got {
		f := float64(v)
		assert.False(t, math.IsNaN(f), "candidate %s produced NaN", id)
		assert.False(t, math.IsInf(f, 0), "candidate %s produced Inf", id)
		// Score may produce negative values (negative IDF on universally-watched
		// attributes) but those are filtered out before they're returned: Score
		// only emits candidates with raw > 0. Verify all returned values are > 0.
		assert.Greater(t, f, 0.0, "candidate %s produced non-positive value %f (Score should omit these)", id, f)
	}
}

func TestS5Attribute_Idempotent(t *testing.T) {
	db := setupS5TestDB(t)

	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-A", "Madhouse")
	seedS5Genre(t, db, "anime-A", "action")
	seedS5Tag(t, db, "anime-A", "shounen")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 1500)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))
	first, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)

	require.NoError(t, s5.Precompute(context.Background(), "user-1"))
	second, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)

	assert.Equal(t, first.S5Affinity, second.S5Affinity, "running Precompute twice must produce identical affinity vector")
}

func TestS5Attribute_Score_MalformedJSONB(t *testing.T) {
	db := setupS5TestDB(t)
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, last_computed)
		 VALUES (?, ?, ?, datetime('now'))`,
		"user-1", "{}", "{not_json}",
	).Error)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	_, err := s5.Score(context.Background(), "user-1", []recs.AnimeID{"anime-A"})
	require.Error(t, err, "malformed s5_affinity must surface as an error")
	assert.Contains(t, err.Error(), "s5", "error must be wrapped with the s5 prefix")
}

func TestS5Attribute_PreservesS1AndS6(t *testing.T) {
	db := setupS5TestDB(t)

	// Pre-existing row with S1Vector + S6Seed* populated.
	require.NoError(t, db.Exec(
		`INSERT INTO rec_user_signals (user_id, s1_vector, s5_affinity, s6_seed_anime_id, s6_seed_score, last_computed)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		"user-1", `{"anime-X":1.0}`, "{}", "anime-seed", 9,
	).Error)

	seedS5Anime(t, db, "anime-A", "tv", "pg_13", "manga")
	seedS5Studio(t, db, "anime-A", "Madhouse")
	seedS5History(t, db, "wh-1", "user-1", "anime-A", "kodik", 1500)

	r := repo.NewRecsRepository(db)
	s5 := NewS5Attribute(db, r)
	require.NoError(t, s5.Precompute(context.Background(), "user-1"))

	row, err := r.GetUserSignals(context.Background(), "user-1")
	require.NoError(t, err)
	require.NotNil(t, row)

	assert.Equal(t, `{"anime-X":1.0}`, row.S1Vector, "S1Vector must NOT be clobbered")
	require.NotNil(t, row.S6SeedAnimeID, "S6SeedAnimeID must be preserved")
	assert.Equal(t, "anime-seed", *row.S6SeedAnimeID)
	require.NotNil(t, row.S6SeedScore, "S6SeedScore must be preserved")
	assert.Equal(t, 9, *row.S6SeedScore)

	// And of course S5Affinity must be updated.
	assert.NotEqual(t, "{}", row.S5Affinity, "S5Affinity must be populated by Precompute")
}

// s5RandomID is a deterministic ID builder for the property test.
func s5RandomID(prefix string, n int) string {
	return fmt.Sprintf("%s-%04d", prefix, n)
}
