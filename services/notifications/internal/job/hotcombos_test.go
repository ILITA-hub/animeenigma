package job

import (
	"context"
	"fmt"
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
	// AUTO-608: anime-level eligibility now derives from the stream_providers
	// roster (player_key/anime_level/status), not a literal player-name list.
	// Seed the roster rows the old literal list used to hardcode — same
	// players, same expected admission.
	seedProvider(t, db, "gogoanime", "english", true, "enabled")
	seedProvider(t, db, "ae-firstparty", "ae", true, "enabled")
	seedProvider(t, db, "kodik-noads", "kodik", true, "enabled")
	seedProvider(t, db, "animelib", "animelib", true, "enabled")
	seedProvider(t, db, "animejoy-sibnet", "animejoy-sibnet", true, "enabled")
	seedProvider(t, db, "animejoy-allvideo", "animejoy-allvideo", true, "enabled")
	seedProvider(t, db, "hanime", "hanime", false, "enabled") // not anime-level → stays excluded

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

// seedWatchRowSeq gives each seedWatchRow call its own anime so combos in
// the same test don't collide on (user, anime) uniqueness.
var seedWatchRowSeq int

// seedWatchRow builds the full watch_history/anime_list/animes fixture
// (single-user, ongoing anime, watching status) for one (player,
// translation_id) combo — the minimal row shape the hotcombos collector
// joins across. Mirrors seedAnime/seedList/seedWatch, just pre-wired for
// tests that only care about player+translation_id admission.
func seedWatchRow(t *testing.T, db *gorm.DB, player, translationID string) {
	t.Helper()
	seedWatchRowSeq++
	id := fmt.Sprintf("wr-%d", seedWatchRowSeq)
	seedAnime(t, db, id, id)
	seedList(t, db, "u1", id, "watching")
	seedWatch(t, db, "u1", id, player, "en", "sub", translationID, 1)
}

// TestHotCombos_RosterDrivenAnimeLevelPlayers pins the AUTO-608 behavior
// change: anime-level eligibility for empty-translation_id combos comes from
// the stream_providers roster (player_key + anime_level + status), not a
// literal player-name list. A disabled roster row's player_key must drop out
// of the eligible set (deviation #6).
func TestHotCombos_RosterDrivenAnimeLevelPlayers(t *testing.T) {
	db := testDB(t)
	seedProvider(t, db, "gogoanime", "english", true /*anime_level*/, "enabled")
	seedProvider(t, db, "hanime", "hanime", false, "enabled")
	seedProvider(t, db, "animelib", "animelib", true, "disabled") // disabled → excluded

	// combo with empty translation_id and player 'english' → collected
	seedWatchRow(t, db, "english", "")
	// empty translation_id + player 'hanime' (not anime-level) → NOT collected
	seedWatchRow(t, db, "hanime", "")
	// empty translation_id + player 'animelib' (row disabled) → NOT collected (deviation #6)
	seedWatchRow(t, db, "animelib", "")
	// non-empty translation_id always collected regardless of player
	seedWatchRow(t, db, "hanime", "tr-1")

	combos, err := NewHotCombosCollector(db, logger.Default()).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]bool{}
	for _, c := range combos {
		got[c.Player+"|"+c.TranslationID] = true
	}
	if !got["english|"] || !got["hanime|tr-1"] {
		t.Fatalf("expected english| and hanime|tr-1 collected, got %v", got)
	}
	if got["hanime|"] || got["animelib|"] {
		t.Fatalf("non-anime-level / disabled players with empty translation must be excluded, got %v", got)
	}
}
