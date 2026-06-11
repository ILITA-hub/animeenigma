package signals

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB creates an in-memory SQLite DB with the minimal schema S7 needs:
// animes (id only), anime_list (dropped-status history), anime_genres (m2m),
// anime_tags (m2m), and the supporting tags table (FK target).
//
// Schema note: anime_list.score is nullable (DEFAULT 0 in Postgres, but NULL
// when a user drops without scoring). The seed query must use
// (score IS NULL OR score < threshold) so unscored drops are treated as
// genuine dislikes, not excluded via SQL NULL semantics.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("newTestDB: open: %v", err)
	}

	mustExec(t, db, `CREATE TABLE animes (
		id   TEXT PRIMARY KEY,
		hidden INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`)
	mustExec(t, db, `CREATE TABLE anime_list (
		id       TEXT PRIMARY KEY,
		user_id  TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		status   TEXT,
		score    INTEGER  -- nullable: unscored drops stored as NULL
	)`)
	mustExec(t, db, `CREATE TABLE anime_genres (
		anime_id TEXT NOT NULL,
		genre_id TEXT NOT NULL,
		PRIMARY KEY (anime_id, genre_id)
	)`)
	mustExec(t, db, `CREATE TABLE tags (
		id   TEXT PRIMARY KEY,
		name TEXT
	)`)
	mustExec(t, db, `CREATE TABLE anime_tags (
		anime_id TEXT NOT NULL,
		tag_id   TEXT NOT NULL,
		rank     INTEGER DEFAULT 0,
		PRIMARY KEY (anime_id, tag_id)
	)`)
	return db
}

func mustExec(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	if err := db.Exec(sql).Error; err != nil {
		t.Fatalf("mustExec: %v\nSQL: %s", err, sql)
	}
}

// seedAnimeS7 inserts an anime row plus its genre and tag rows.
func seedAnimeS7(t *testing.T, db *gorm.DB, id string, genres []string, tags []string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO animes (id, hidden) VALUES ('`+id+`', 0)`)
	for _, g := range genres {
		if err := db.Exec(
			`INSERT INTO anime_genres (anime_id, genre_id) VALUES (?, ?)`, id, g,
		).Error; err != nil {
			t.Fatalf("seedAnimeS7 genre %s: %v", g, err)
		}
	}
	for _, tag := range tags {
		// Insert into tags table first (idempotent).
		_ = db.Exec(`INSERT OR IGNORE INTO tags (id, name) VALUES (?, ?)`, tag, tag).Error
		if err := db.Exec(
			`INSERT INTO anime_tags (anime_id, tag_id, rank) VALUES (?, ?, 0)`, id, tag,
		).Error; err != nil {
			t.Fatalf("seedAnimeS7 tag %s: %v", tag, err)
		}
	}
}

// seedDropS7 inserts a dropped anime_list entry for a user.
// score=0 means "no score given" (stored as nullable 0 / may be NULL).
// Pass score < 0 to insert a NULL score (truly unscored).
func seedDropS7(t *testing.T, db *gorm.DB, rowID, userID, animeID string, score int) {
	t.Helper()
	if score < 0 {
		// NULL score: unscored drop.
		if err := db.Exec(
			`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, 'dropped', NULL)`,
			rowID, userID, animeID,
		).Error; err != nil {
			t.Fatalf("seedDropS7 null-score: %v", err)
		}
	} else {
		if err := db.Exec(
			`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES (?, ?, ?, 'dropped', ?)`,
			rowID, userID, animeID, score,
		).Error; err != nil {
			t.Fatalf("seedDropS7 score %d: %v", score, err)
		}
	}
}

// seedS7Fixtures builds the canonical fixture set described in the test spec:
//
//	user "u1" dropped:
//	  - "d1" (genres g1, g2; score 0)         — eligible seed
//	  - "d2" (genre g1, tag t1; score 4)       — eligible seed
//	  - "d3" (genre g9, tag t9; score 8)       — LIKED drop, score>=7, excluded
//	Candidates:
//	  - "c1" (genres g1, g2)  — overlaps d1 strongly (Jaccard 1.0 vs d1)
//	  - "c2" (genre g5)       — no overlap with any eligible seed
//	  - "c3" (genre g9, tag t9) — overlaps only the liked drop d3 (must be excluded)
func seedS7Fixtures(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Seeds (dropped anime for u1)
	seedAnimeS7(t, db, "d1", []string{"g1", "g2"}, nil)
	seedAnimeS7(t, db, "d2", []string{"g1"}, []string{"t1"})
	seedAnimeS7(t, db, "d3", []string{"g9"}, []string{"t9"})
	seedDropS7(t, db, "al-d1", "u1", "d1", 0)
	seedDropS7(t, db, "al-d2", "u1", "d2", 4)
	seedDropS7(t, db, "al-d3", "u1", "d3", 8) // liked drop — must be excluded

	// Candidates
	seedAnimeS7(t, db, "c1", []string{"g1", "g2"}, nil)
	seedAnimeS7(t, db, "c2", []string{"g5"}, nil)
	seedAnimeS7(t, db, "c3", []string{"g9"}, []string{"t9"})
}

// seedOneDrop provides exactly ONE eligible dropped seed for u1 (cold-start guard).
func seedOneDrop(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedAnimeS7(t, db, "d1", []string{"g1"}, nil)
	seedDropS7(t, db, "al-d1", "u1", "d1", 3)
	seedAnimeS7(t, db, "c1", []string{"g1"}, nil)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestS7_ScoresSimilarityToDroppedSeeds — c1 overlaps two eligible seeds;
// must receive a positive score. c2 has no overlap; must be omitted.
func TestS7_ScoresSimilarityToDroppedSeeds(t *testing.T) {
	db := newTestDB(t)
	seedS7Fixtures(t, db)
	s7 := NewS7DroppedPenalty(db)

	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c1", "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if got["c1"] == 0 {
		t.Fatalf("c1 overlaps dropped seeds; want > 0, got %v", got["c1"])
	}
	if _, ok := got["c2"]; ok {
		t.Fatalf("c2 has no overlap; must be omitted, got %v", got["c2"])
	}
}

// TestS7_ColdStartUnderTwoSeeds — exactly one eligible dropped seed →
// signal is silent (empty map returned, no error).
func TestS7_ColdStartUnderTwoSeeds(t *testing.T) {
	db := newTestDB(t)
	seedOneDrop(t, db)
	s7 := NewS7DroppedPenalty(db)
	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cold-start (<2 seeds) must return empty map, got %v", got)
	}
}

// TestS7_LikedDropsExcluded — c3 only overlaps d3 (score 8, liked drop);
// d3 must be excluded from seeds → c3 must be omitted.
func TestS7_LikedDropsExcluded(t *testing.T) {
	db := newTestDB(t)
	seedS7Fixtures(t, db)
	s7 := NewS7DroppedPenalty(db)
	got, _ := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c3"})
	if _, ok := got["c3"]; ok {
		t.Fatalf("c3 only matches a liked drop (score>=7); must be omitted, got %v", got["c3"])
	}
}

// TestS7_IDAndPrecompute — signal ID must be "s7"; Precompute must be a no-op.
func TestS7_IDAndPrecompute(t *testing.T) {
	s7 := NewS7DroppedPenalty(nil)
	if s7.ID() != recs.SignalID("s7") {
		t.Fatalf("ID = %q, want s7", s7.ID())
	}
	if err := s7.Precompute(context.Background(), "u1"); err != nil {
		t.Fatalf("Precompute must be a no-op, got %v", err)
	}
}

// TestS7_TagOverlapCounts — candidate sharing ONLY a tag (not a genre) with
// a dropped seed must score > 0. Proves the namespaced union includes tags.
func TestS7_TagOverlapCounts(t *testing.T) {
	db := newTestDB(t)

	// Two eligible seeds: "ds1" (genre g1, tag t1) and "ds2" (genre g2, tag t2).
	// Candidate "ct" has ONLY tag t1, NO genres in common with any seed.
	seedAnimeS7(t, db, "ds1", []string{"g1"}, []string{"t1"})
	seedAnimeS7(t, db, "ds2", []string{"g2"}, []string{"t2"})
	seedDropS7(t, db, "al-ds1", "u1", "ds1", 0)
	seedDropS7(t, db, "al-ds2", "u1", "ds2", 3)

	// Candidate with tag t1 only (no genre overlap with ds1 or ds2).
	seedAnimeS7(t, db, "ct", nil, []string{"t1"})

	s7 := NewS7DroppedPenalty(db)
	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"ct"})
	if err != nil {
		t.Fatal(err)
	}
	if got["ct"] == 0 {
		t.Fatalf("ct shares tag t1 with seed ds1; namespaced union must give score > 0, got %v", got["ct"])
	}
}

// TestS7_NullScoreDropIncluded — a dropped row with NULL score (no rating
// given) must be counted as an eligible seed (score<7 guard via IS NULL OR).
func TestS7_NullScoreDropIncluded(t *testing.T) {
	db := newTestDB(t)

	// Two drops: "dn1" NULL score, "dn2" score 2 — both eligible.
	seedAnimeS7(t, db, "dn1", []string{"g1", "g2"}, nil)
	seedAnimeS7(t, db, "dn2", []string{"g1"}, nil)
	seedDropS7(t, db, "al-dn1", "u1", "dn1", -1) // -1 sentinel → NULL
	seedDropS7(t, db, "al-dn2", "u1", "dn2", 2)

	// Candidate with matching genres.
	seedAnimeS7(t, db, "cn", []string{"g1", "g2"}, nil)

	s7 := NewS7DroppedPenalty(db)
	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"cn"})
	if err != nil {
		t.Fatal(err)
	}
	if got["cn"] == 0 {
		t.Fatalf("cn overlaps dn1 (NULL-score drop, must be included); want > 0, got %v", got["cn"])
	}
}
