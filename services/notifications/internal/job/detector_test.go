package job

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/config"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// stubChecker implements service.EpisodeChecker over a static fixture.
// `title` (optional) is returned as the TranslationTitle for every combo —
// enough to assert the title plumbs through to the payload.
type stubChecker struct {
	byCombo map[domain.Combo]int
	title   string
}

func (s *stubChecker) LatestEpisode(_ context.Context, combo domain.Combo) (service.EpisodeCheckResult, error) {
	if v, ok := s.byCombo[combo]; ok {
		return service.EpisodeCheckResult{Latest: v, TranslationTitle: s.title}, nil
	}
	return service.EpisodeCheckResult{}, nil
}

// testDB spins up an in-memory SQLite DB with the schema both the
// notifications service owns AND the read-only views it consumes
// (watch_history, anime_list, animes). The schema is intentionally
// minimal — just enough columns to satisfy the SQL the detector executes.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Hand-rolled SQLite-friendly DDL (we bypass GORM AutoMigrate +
	// repo.EnsureIndexes because both encode Postgres-specific syntax
	// — `uuid`, `gen_random_uuid()`, partial-index WHERE clauses).
	stmts := []string{
		`CREATE TABLE user_notifications (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			dedupe_key TEXT NOT NULL,
			payload TEXT NOT NULL,
			read_at DATETIME,
			dismissed_at DATETIME,
			invalidated_at DATETIME,
			deleted_at DATETIME,
			clicked_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		// Unconditional unique on (user_id, dedupe_key) is strong enough
		// for the test cases (none of them dismiss-and-reinsert).
		`CREATE UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key)`,
		`CREATE TABLE parser_episode_snapshots (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			anime_id TEXT NOT NULL,
			player TEXT NOT NULL,
			language TEXT NOT NULL,
			watch_type TEXT NOT NULL,
			translation_id TEXT NOT NULL,
			latest_episode INTEGER NOT NULL,
			checked_at DATETIME,
			updated_at DATETIME,
			UNIQUE (anime_id, player, language, watch_type, translation_id)
		)`,
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY,
			shikimori_id TEXT,
			status TEXT,
			name TEXT,
			name_ru TEXT,
			poster_url TEXT,
			next_episode_at DATETIME
		)`,
		`CREATE TABLE anime_list (
			user_id TEXT,
			anime_id TEXT,
			status TEXT,
			PRIMARY KEY (user_id, anime_id)
		)`,
		`CREATE TABLE watch_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT,
			anime_id TEXT,
			episode_number INTEGER,
			player TEXT,
			language TEXT,
			watch_type TEXT,
			translation_id TEXT
		)`,
		// stream_providers mirrors the catalog-owned roster (AUTO-608): the
		// hot-combos collector subselects player_key/anime_level/status from
		// it instead of a literal player-name list. Minimal columns only —
		// just what the collector's WHERE clause reads.
		`CREATE TABLE stream_providers (
			name TEXT PRIMARY KEY,
			player_key TEXT NOT NULL DEFAULT '',
			anime_level INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'enabled'
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}

func seedAnime(t *testing.T, db *gorm.DB, id, shikimoriID string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO animes (id, shikimori_id, status, name, name_ru, poster_url) VALUES (?,?,?,?,?,?)`,
		id, shikimoriID, "ongoing", "Show", "Шоу", "https://example/p.jpg",
	).Error; err != nil {
		t.Fatalf("seed anime: %v", err)
	}
}

// seedAnimeWithNextEpisode is seedAnime plus an explicit next_episode_at —
// used by the cadence-tiering test to drive AiringTimes.
func seedAnimeWithNextEpisode(t *testing.T, db *gorm.DB, id, shikimoriID string, nextEpisodeAt time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO animes (id, shikimori_id, status, name, name_ru, poster_url, next_episode_at) VALUES (?,?,?,?,?,?,?)`,
		id, shikimoriID, "ongoing", "Show", "Шоу", "https://example/p.jpg", nextEpisodeAt,
	).Error; err != nil {
		t.Fatalf("seed anime with next episode: %v", err)
	}
}

func seedList(t *testing.T, db *gorm.DB, userID, animeID, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO anime_list (user_id, anime_id, status) VALUES (?,?,?)`,
		userID, animeID, status,
	).Error; err != nil {
		t.Fatalf("seed list: %v", err)
	}
}

func seedWatch(t *testing.T, db *gorm.DB, userID, animeID, player, language, watchType, translationID string, ep int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO watch_history (user_id, anime_id, episode_number, player, language, watch_type, translation_id)
		 VALUES (?,?,?,?,?,?,?)`,
		userID, animeID, ep, player, language, watchType, translationID,
	).Error; err != nil {
		t.Fatalf("seed watch: %v", err)
	}
}

// seedProvider inserts a stream_providers roster row (AUTO-608). animeLevel
// mirrors domain.ScraperProvider.AnimeLevel; status is the tri-state
// enabled|degraded|disabled string.
func seedProvider(t *testing.T, db *gorm.DB, name, playerKey string, animeLevel bool, status string) {
	t.Helper()
	al := 0
	if animeLevel {
		al = 1
	}
	if err := db.Exec(
		`INSERT INTO stream_providers (name, player_key, anime_level, status) VALUES (?,?,?,?)`,
		name, playerKey, al, status,
	).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
}

// newDetector builds a detector job wired to a fresh, empty miniredis-backed
// CheckedStore — every anime is "never checked", so tierFilter includes
// everything and existing (pre-tiering) test behavior is preserved exactly.
func newDetector(t *testing.T, db *gorm.DB, checker service.EpisodeChecker) *NewEpisodeDetectorJob {
	t.Helper()
	checked, _ := newTestChecked(t)
	return newDetectorWithChecked(t, db, checker, checked)
}

// newDetectorWithChecked is newDetector but wired to a caller-supplied
// CheckedStore instead of a fresh empty one — lets cadence-tiering tests
// pre-mark last-checked timestamps before exercising Run.
func newDetectorWithChecked(t *testing.T, db *gorm.DB, checker service.EpisodeChecker, checked *CheckedStore) *NewEpisodeDetectorJob {
	t.Helper()
	log := logger.Default()
	notifRepo := repo.NewNotificationRepository(db)
	notifService := service.NewNotificationService(notifRepo, log)
	cfg := &config.DetectorConfig{
		WorkerLimit: 2,
		HotWindow:   36 * time.Hour,
		WarmEvery:   3 * time.Hour,
		TierFloor:   6 * time.Hour,
	}
	return NewEpisodeDetectorJobNew(
		NewHotCombosCollector(db, log),
		checker,
		repo.NewSnapshotRepository(db),
		repo.NewMaxWatchedRepository(db),
		repo.NewAnimeViewRepository(db),
		notifService,
		cfg,
		log,
		checked,
	)
}

// Test_Detector_BootstrapProtection asserts NOTIF-DET-06 + plan must-have
// truth #2: starting with parser_episode_snapshots EMPTY and user_notifications
// EMPTY, the first detector pass MUST insert ZERO user_notifications rows
// (it only seeds the snapshot table).
func Test_Detector_BootstrapProtection(t *testing.T) {
	db := testDB(t)

	animeID := "anime-1"
	shikimoriID := "57466"
	userID := "user-1"

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, "animelib", "ru", "dub", "9999", 5)

	checker := &stubChecker{byCombo: map[domain.Combo]int{
		{
			AnimeID:       animeID,
			ShikimoriID:   shikimoriID,
			Player:        "animelib",
			Language:      "ru",
			WatchType:     "dub",
			TranslationID: "9999",
		}: 6, // parser says ep 6 available
	}}

	det := newDetector(t, db, checker)

	report, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("detector run: %v", err)
	}

	// Snapshot table MUST have one row now.
	var snapCount int64
	if err := db.Model(&domain.ParserEpisodeSnapshot{}).Count(&snapCount).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapCount != 1 {
		t.Fatalf("expected 1 snapshot row after bootstrap, got %d", snapCount)
	}

	// Critical: ZERO notifications must have been inserted.
	var notifCount int64
	if err := db.Model(&domain.UserNotification{}).Count(&notifCount).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifCount != 0 {
		t.Fatalf("BOOTSTRAP PROTECTION VIOLATED: expected 0 notifications, got %d", notifCount)
	}

	if report.AffectedCombos != 0 {
		t.Fatalf("expected affected_combos=0 on bootstrap, got %d", report.AffectedCombos)
	}
	if report.NotificationsUpserted != 0 {
		t.Fatalf("expected notifications_upserted=0 on bootstrap, got %d", report.NotificationsUpserted)
	}
	if report.CombosScanned != 1 {
		t.Fatalf("expected combos_scanned=1, got %d", report.CombosScanned)
	}
}

// Test_Detector_FiresOnDiffAfterBootstrap is the natural follow-up to the
// bootstrap test: same setup, second run with a higher parser value, EXACTLY
// one notification fires with the design-doc payload shape.
func Test_Detector_FiresOnDiffAfterBootstrap(t *testing.T) {
	db := testDB(t)

	animeID := "anime-1"
	shikimoriID := "57466"
	userID := "user-1"
	combo := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "9999",
	}

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, "animelib", "ru", "dub", "9999", 5)

	checker := &stubChecker{byCombo: map[domain.Combo]int{combo: 5}, title: "AniRise"}
	det := newDetector(t, db, checker)

	// First run: bootstrap.
	if _, err := det.Run(context.Background()); err != nil {
		t.Fatalf("bootstrap run: %v", err)
	}

	// Second run: parser now reports ep 6 → must fire.
	checker.byCombo[combo] = 6
	report, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}
	if report.NotificationsUpserted != 1 {
		t.Fatalf("expected exactly 1 notification on diff, got %d", report.NotificationsUpserted)
	}

	var n domain.UserNotification
	if err := db.Where("user_id = ?", userID).Take(&n).Error; err != nil {
		t.Fatalf("fetch notification: %v", err)
	}
	want := "new_episode:" + animeID
	if n.DedupeKey != want {
		t.Fatalf("dedupe key mismatch: want %q, got %q", want, n.DedupeKey)
	}

	// The checker-resolved translation title must ride into the payload.
	var payload domain.NewEpisodePayload
	if err := json.Unmarshal([]byte(n.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TranslationTitle != "AniRise" {
		t.Fatalf("translation_title mismatch: want %q, got %q", "AniRise", payload.TranslationTitle)
	}
}

// Test_Detector_GroupsCombosAndEpisodesPerUserAnime is the AUTO-660
// regression: every watched provider/team combo is still scanned and
// snapshotted, while simultaneous diffs collapse into one notification whose
// range covers every newly available episode.
func Test_Detector_GroupsCombosAndEpisodesPerUserAnime(t *testing.T) {
	db := testDB(t)

	animeID := "anime-grouped"
	shikimoriID := "660"
	userID := "user-1"
	animelib := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "team-a",
	}
	kodik := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "kodik", Language: "ru", WatchType: "dub", TranslationID: "team-b",
	}

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, animelib.Player, animelib.Language, animelib.WatchType, animelib.TranslationID, 5)
	seedWatch(t, db, userID, animeID, kodik.Player, kodik.Language, kodik.WatchType, kodik.TranslationID, 4)

	checker := &stubChecker{byCombo: map[domain.Combo]int{
		animelib: 5,
		kodik:    5,
	}}
	det := newDetector(t, db, checker)

	if _, err := det.Run(context.Background()); err != nil {
		t.Fatalf("bootstrap run: %v", err)
	}

	// Both combos advance in the same detector pass, and one of them jumps
	// across multiple episodes.
	checker.byCombo[animelib] = 7
	checker.byCombo[kodik] = 8
	report, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("diff run: %v", err)
	}
	if report.CombosScanned != 2 || report.CombosSelected != 2 || report.AffectedCombos != 2 {
		t.Fatalf("all combos must still be scanned: report=%+v", report)
	}
	if report.NotificationsUpserted != 1 {
		t.Fatalf("expected one grouped notification, got %+v", report)
	}

	var snapshots []domain.ParserEpisodeSnapshot
	if err := db.Order("player").Find(&snapshots).Error; err != nil {
		t.Fatalf("fetch snapshots: %v", err)
	}
	if len(snapshots) != 2 || snapshots[0].LatestEpisode != 7 || snapshots[1].LatestEpisode != 8 {
		t.Fatalf("both combo snapshots must advance independently, got %+v", snapshots)
	}

	var notifications []domain.UserNotification
	if err := db.Where("user_id = ?", userID).Find(&notifications).Error; err != nil {
		t.Fatalf("fetch notifications: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("expected exactly one row for both combos, got %d", len(notifications))
	}
	if want := service.NewEpisodeDedupeKey(animeID); notifications[0].DedupeKey != want {
		t.Fatalf("dedupe key mismatch: want %q, got %q", want, notifications[0].DedupeKey)
	}
	var payload domain.NewEpisodePayload
	if err := json.Unmarshal([]byte(notifications[0].Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.FirstUnwatchedEpisode != 6 || payload.LatestAvailableEpisode != 8 {
		t.Fatalf("expected grouped episode range 6-8, got %+v", payload)
	}
}

// Test_Detector_SkipsLaggingComboDuplicate verifies the cross-run half of
// AUTO-660. When provider B reports episode 6 one run after provider A, it is
// still scanned and its snapshot advances, but the already-reported episode
// must not refresh/reset the user's per-anime notification. Episode 7 remains
// a genuine new event and updates that same row.
func Test_Detector_SkipsLaggingComboDuplicate(t *testing.T) {
	db := testDB(t)

	animeID := "anime-staggered"
	shikimoriID := "661"
	userID := "user-1"
	first := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "sub", TranslationID: "team-a",
	}
	lagging := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "kodik", Language: "ru", WatchType: "sub", TranslationID: "team-b",
	}

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, first.Player, first.Language, first.WatchType, first.TranslationID, 5)
	seedWatch(t, db, userID, animeID, lagging.Player, lagging.Language, lagging.WatchType, lagging.TranslationID, 5)

	checker := &stubChecker{byCombo: map[domain.Combo]int{first: 5, lagging: 5}}
	det := newDetector(t, db, checker)
	if _, err := det.Run(context.Background()); err != nil {
		t.Fatalf("bootstrap run: %v", err)
	}

	checker.byCombo[first] = 6
	firstReport, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("first provider diff: %v", err)
	}
	if firstReport.NotificationsUpserted != 1 {
		t.Fatalf("first provider must create one notification, got %+v", firstReport)
	}

	readAt := time.Now().UTC()
	if err := db.Model(&domain.UserNotification{}).
		Where("user_id = ?", userID).
		Update("read_at", readAt).Error; err != nil {
		t.Fatalf("mark notification read: %v", err)
	}

	// Same episode arrives on the second provider. Snapshot it, but do not
	// upsert (which would incorrectly reset read_at and show a duplicate).
	checker.byCombo[lagging] = 6
	duplicateReport, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("lagging provider diff: %v", err)
	}
	if duplicateReport.AffectedCombos != 1 || duplicateReport.NotificationsUpserted != 0 {
		t.Fatalf("lagging combo must advance without notifying, got %+v", duplicateReport)
	}
	var notification domain.UserNotification
	if err := db.Where("user_id = ?", userID).Take(&notification).Error; err != nil {
		t.Fatalf("fetch notification after duplicate: %v", err)
	}
	if notification.ReadAt == nil {
		t.Fatal("duplicate provider arrival reset read_at")
	}

	// A truly newer episode on either provider updates the same per-anime row.
	checker.byCombo[lagging] = 7
	newReport, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("newer provider diff: %v", err)
	}
	if newReport.NotificationsUpserted != 1 {
		t.Fatalf("episode 7 must refresh one grouped notification, got %+v", newReport)
	}
	var count int64
	if err := db.Model(&domain.UserNotification{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one stable per-anime row, got %d", count)
	}
	notification = domain.UserNotification{}
	if err := db.Where("user_id = ?", userID).Take(&notification).Error; err != nil {
		t.Fatalf("fetch refreshed notification: %v", err)
	}
	if notification.ReadAt != nil {
		t.Fatal("genuinely newer episode must reset read_at")
	}
	var payload domain.NewEpisodePayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		t.Fatalf("unmarshal refreshed payload: %v", err)
	}
	if payload.FirstUnwatchedEpisode != 6 || payload.LatestAvailableEpisode != 7 {
		t.Fatalf("expected refreshed episode range 6-7, got %+v", payload)
	}
}

// Test_Detector_Idempotency: re-running with unchanged upstream produces
// no new rows (NOTIF-DET-10).
func Test_Detector_Idempotency(t *testing.T) {
	db := testDB(t)

	animeID := "anime-1"
	shikimoriID := "57466"
	userID := "user-1"
	combo := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "9999",
	}

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, "animelib", "ru", "dub", "9999", 5)

	checker := &stubChecker{byCombo: map[domain.Combo]int{combo: 5}}
	det := newDetector(t, db, checker)

	// Bootstrap at 5.
	_, _ = det.Run(context.Background())
	// Bump parser to 6 — first-diff fire.
	checker.byCombo[combo] = 6
	_, _ = det.Run(context.Background())
	// Third run with the parser still at 6: unchanged upstream → no-op.
	r3, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if r3.AffectedCombos != 0 {
		t.Fatalf("expected 0 affected on idempotent run, got %d", r3.AffectedCombos)
	}
	if r3.NotificationsUpserted != 0 {
		t.Fatalf("expected 0 notifications upserted on idempotent run, got %d", r3.NotificationsUpserted)
	}

	var count int64
	if err := db.Model(&domain.UserNotification{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("idempotency violated: expected 1 notification, got %d", count)
	}
}

// Test_Detector_NeverLowersSnapshot: simulating a parser regression
// (returning a lower episode number than the snapshot already holds)
// must NOT decrease the snapshot value (NOTIF-DET-10).
func Test_Detector_NeverLowersSnapshot(t *testing.T) {
	db := testDB(t)

	animeID := "anime-1"
	shikimoriID := "57466"
	userID := "user-1"
	combo := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "9999",
	}

	seedAnime(t, db, animeID, shikimoriID)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, "animelib", "ru", "dub", "9999", 5)

	checker := &stubChecker{byCombo: map[domain.Combo]int{combo: 10}}
	det := newDetector(t, db, checker)
	// Bootstrap at 10.
	_, _ = det.Run(context.Background())
	// Parser regresses to 7. Must not lower snapshot.
	checker.byCombo[combo] = 7
	_, _ = det.Run(context.Background())

	var snap domain.ParserEpisodeSnapshot
	if err := db.Take(&snap).Error; err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}
	if snap.LatestEpisode != 10 {
		t.Fatalf("snapshot lowered: want 10, got %d", snap.LatestEpisode)
	}
}

// Test_Detector_ParserFailureIsolation: a parser failure on one combo
// must not abort the run; other combos still process.
func Test_Detector_ParserFailureIsolation(t *testing.T) {
	db := testDB(t)

	animeA := "anime-A"
	animeB := "anime-B"
	sidA := "111"
	sidB := "222"
	userID := "user-1"
	comboA := domain.Combo{AnimeID: animeA, ShikimoriID: sidA, Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "1"}
	_ = domain.Combo{AnimeID: animeB, ShikimoriID: sidB, Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "2"}

	seedAnime(t, db, animeA, sidA)
	seedAnime(t, db, animeB, sidB)
	seedList(t, db, userID, animeA, "watching")
	seedList(t, db, userID, animeB, "watching")
	seedWatch(t, db, userID, animeA, "animelib", "ru", "dub", "1", 5)
	seedWatch(t, db, userID, animeB, "animelib", "ru", "dub", "2", 3)

	checker := &failingCheckerForB{success: map[domain.Combo]int{comboA: 5}}
	det := newDetector(t, db, checker)
	// First run is bootstrap for both. Combo A bootstraps to 5, combo B
	// fails parser — its snapshot is not recorded, but the run does not abort.
	_, _ = det.Run(context.Background())

	// Now bump A to 6. B still fails. Expect 1 notification fired (A only).
	checker.success[comboA] = 6
	r, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.ParserFailures < 1 {
		t.Fatalf("expected at least 1 parser failure, got %d", r.ParserFailures)
	}
	if r.NotificationsUpserted != 1 {
		t.Fatalf("expected 1 notification (combo A), got %d", r.NotificationsUpserted)
	}
}

// failingCheckerForB returns an error for any combo not in success.
type failingCheckerForB struct {
	success map[domain.Combo]int
}

func (f *failingCheckerForB) LatestEpisode(_ context.Context, combo domain.Combo) (service.EpisodeCheckResult, error) {
	if v, ok := f.success[combo]; ok {
		return service.EpisodeCheckResult{Latest: v}, nil
	}
	return service.EpisodeCheckResult{}, errParser("upstream unavailable")
}

type errParser string

func (e errParser) Error() string { return string(e) }

// countingChecker counts LatestEpisode invocations on top of a static
// fixture — used to assert the tier filter actually kept the parser from
// being asked about a combo, rather than the parser being asked and merely
// returning nothing.
type countingChecker struct {
	byCombo map[domain.Combo]int
	calls   int
}

func (c *countingChecker) LatestEpisode(_ context.Context, combo domain.Combo) (service.EpisodeCheckResult, error) {
	c.calls++
	if v, ok := c.byCombo[combo]; ok {
		return service.EpisodeCheckResult{Latest: v}, nil
	}
	return service.EpisodeCheckResult{}, nil
}

// Test_Detector_TierSkipsWarmRecentlyChecked is the cadence-tiering
// integration assertion (spec §4, plan Task 4 Step 2): a warm anime (next
// episode far outside HotWindow) that was checked recently (inside
// WarmEvery) must be skipped entirely — the parser/episode-checker never
// sees its combo. Once its last-checked timestamp ages past TierFloor, the
// SAME anime must be checked regardless of tier (the delivery-floor
// guarantee — Global Constraints: tiering only DELAYS a cold check, never
// drops it).
func Test_Detector_TierSkipsWarmRecentlyChecked(t *testing.T) {
	db := testDB(t)

	animeID := "warm1"
	shikimoriID := "999"
	userID := "user-1"
	combo := domain.Combo{
		AnimeID: animeID, ShikimoriID: shikimoriID,
		Player: "animelib", Language: "ru", WatchType: "dub", TranslationID: "1",
	}

	// Next episode ~10 days out — well outside the 36h HotWindow, so this
	// anime sits in the warm tier.
	nextEpisode := time.Now().Add(10 * 24 * time.Hour)
	seedAnimeWithNextEpisode(t, db, animeID, shikimoriID, nextEpisode)
	seedList(t, db, userID, animeID, "watching")
	seedWatch(t, db, userID, animeID, "animelib", "ru", "dub", "1", 5)

	checked, _ := newTestChecked(t)

	// Pre-mark "checked 1h ago" by temporarily faking the store's clock —
	// inside WarmEvery(3h), so the warm tier is not yet due.
	checked.now = func() time.Time { return time.Now().Add(-1 * time.Hour) }
	checked.MarkChecked(context.Background(), []string{animeID})
	checked.now = time.Now

	checker := &countingChecker{byCombo: map[domain.Combo]int{combo: 5}}
	det := newDetectorWithChecked(t, db, checker, checked)

	report, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("detector run: %v", err)
	}
	if checker.calls != 0 {
		t.Fatalf("warm + recently-checked combo must be skipped: expected 0 parser calls, got %d", checker.calls)
	}
	if report.CombosScanned != 1 {
		t.Fatalf("expected combos_scanned=1 (pre-filter), got %d", report.CombosScanned)
	}
	if report.CombosSelected != 0 {
		t.Fatalf("expected combos_selected=0 (tier-filtered out), got %d", report.CombosSelected)
	}

	// Push the last-checked timestamp past the 6h delivery floor — the SAME
	// anime must now be checked regardless of its warm tier.
	checked.now = func() time.Time { return time.Now().Add(-7 * time.Hour) }
	checked.MarkChecked(context.Background(), []string{animeID})
	checked.now = time.Now

	report2, err := det.Run(context.Background())
	if err != nil {
		t.Fatalf("detector run 2: %v", err)
	}
	if checker.calls != 1 {
		t.Fatalf("floor-expired combo must be checked: expected 1 parser call, got %d", checker.calls)
	}
	if report2.CombosSelected != 1 {
		t.Fatalf("expected combos_selected=1 (floor forced due), got %d", report2.CombosSelected)
	}
}

// Compile-time sanity: tests reference the public service.EpisodeChecker
// surface — keeps the interface in sync with the production HTTP client.
var _ service.EpisodeChecker = (*stubChecker)(nil)
var _ service.EpisodeChecker = (*failingCheckerForB)(nil)
var _ service.EpisodeChecker = (*countingChecker)(nil)

// Sanity: import database to ensure the test binary links the same DB
// driver tree the production binary uses (cross-checks go.mod resolution).
var _ = database.Config{}
