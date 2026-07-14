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
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-en','111','ongoing'),('a-ae','222','ongoing'),('a-h','444','ongoing'),('a-k','555','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-en','watching'),('u1','a-ae','watching'),('u1','a-h','watching'),('u1','a-k','watching')`)

	seedWatch(t, db, "u1", "a-en", "english", "en", "sub", "", 5)  // empty id, anime-level
	seedWatch(t, db, "u1", "a-ae", "ae", "ja", "sub", "", 3)       // empty id, anime-level
	seedWatch(t, db, "u1", "a-h", "hanime", "ru", "dub", "", 1)    // empty id, NOT admitted
	seedWatch(t, db, "u1", "a-k", "kodik", "ru", "sub", "1291", 7) // legacy kodik with id, admitted

	// aePlayer kodik/animelib with empty translation_id (any-team) — must be admitted
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-ke','666','ongoing'),('a-ale','777','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-ke','watching'),('u1','a-ale','watching')`)
	seedWatch(t, db, "u1", "a-ke", "kodik", "ru", "sub", "", 4)     // aePlayer kodik, empty id → admitted
	seedWatch(t, db, "u1", "a-ale", "animelib", "ru", "sub", "", 2) // aePlayer animelib, empty id → admitted

	// animejoy-sibnet/animejoy-allvideo with empty translation_id (anime-level) — must be admitted
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-ajs','888','ongoing'),('a-ajav','999','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-ajs','watching'),('u1','a-ajav','watching')`)
	seedWatch(t, db, "u1", "a-ajs", "animejoy-sibnet", "ru", "sub", "", 1)    // anime-level, empty id → admitted
	seedWatch(t, db, "u1", "a-ajav", "animejoy-allvideo", "ru", "sub", "", 1) // anime-level, empty id → admitted

	combos, err := NewHotCombosCollector(db, logger.Default()).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]bool{}
	for _, c := range combos {
		got[c.Player] = true
	}
	for _, p := range []string{"english", "ae", "kodik", "animejoy-sibnet", "animejoy-allvideo"} {
		if !got[p] {
			t.Errorf("expected player %q in hot combos, missing", p)
		}
	}
	if got["hanime"] {
		t.Errorf("hanime (empty translation_id) must NOT be admitted")
	}

	// Assert empty-id kodik/animelib admitted, empty-id hanime excluded
	seen := map[string]bool{}
	for _, c := range combos {
		seen[c.Player+"|"+c.TranslationID] = true
	}
	if !seen["kodik|"] {
		t.Errorf("empty-id kodik combo not admitted")
	}
	if !seen["animelib|"] {
		t.Errorf("empty-id animelib combo not admitted")
	}
	if !seen["animejoy-sibnet|"] {
		t.Errorf("empty-id animejoy-sibnet combo not admitted")
	}
	if !seen["animejoy-allvideo|"] {
		t.Errorf("empty-id animejoy-allvideo combo not admitted")
	}
	if seen["hanime|"] {
		t.Errorf("empty-id hanime combo must NOT be admitted")
	}
}
