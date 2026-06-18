package job

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// mustExec executes raw SQL against the test DB and fatals on error.
func mustExec(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	if err := db.Exec(sql).Error; err != nil {
		t.Fatalf("mustExec: %v", err)
	}
}

func TestHotCombos_AdmitsAnimeLevelPlayers(t *testing.T) {
	db := testDB(t)
	// anime rows (ongoing) + list rows (watching)
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-en','111','ongoing'),('a-ae','222','ongoing'),('a-raw','333','ongoing'),('a-h','444','ongoing'),('a-k','555','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-en','watching'),('u1','a-ae','watching'),('u1','a-raw','watching'),('u1','a-h','watching'),('u1','a-k','watching')`)

	seedWatch(t, db, "u1", "a-en", "english", "en", "sub", "", 5)  // empty id, anime-level
	seedWatch(t, db, "u1", "a-ae", "ae", "ja", "sub", "", 3)       // empty id, anime-level
	seedWatch(t, db, "u1", "a-raw", "raw", "ja", "sub", "", 2)     // empty id, anime-level
	seedWatch(t, db, "u1", "a-h", "hanime", "ru", "dub", "", 1)    // empty id, NOT admitted
	seedWatch(t, db, "u1", "a-k", "kodik", "ru", "sub", "1291", 7) // legacy, admitted

	combos, err := NewHotCombosCollector(db, logger.Default()).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]bool{}
	for _, c := range combos {
		got[c.Player] = true
	}
	for _, p := range []string{"english", "ae", "raw", "kodik"} {
		if !got[p] {
			t.Errorf("expected player %q in hot combos, missing", p)
		}
	}
	if got["hanime"] {
		t.Errorf("hanime (empty translation_id) must NOT be admitted")
	}
}
