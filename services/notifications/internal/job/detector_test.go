package job

import (
	"context"
	"encoding/json"
	"testing"

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
			poster_url TEXT
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

func newDetector(db *gorm.DB, checker service.EpisodeChecker) *NewEpisodeDetectorJob {
	log := logger.Default()
	notifRepo := repo.NewNotificationRepository(db)
	notifService := service.NewNotificationService(notifRepo, log)
	cfg := &config.DetectorConfig{WorkerLimit: 2}
	return NewEpisodeDetectorJobNew(
		NewHotCombosCollector(db, log),
		checker,
		repo.NewSnapshotRepository(db),
		repo.NewMaxWatchedRepository(db),
		repo.NewAnimeViewRepository(db),
		notifService,
		cfg,
		log,
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

	det := newDetector(db, checker)

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
	det := newDetector(db, checker)

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
	want := "new_episode:" + animeID + ":animelib:ru:dub:9999"
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
	det := newDetector(db, checker)

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
	det := newDetector(db, checker)
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
	det := newDetector(db, checker)
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

// Compile-time sanity: tests reference the public service.EpisodeChecker
// surface — keeps the interface in sync with the production HTTP client.
var _ service.EpisodeChecker = (*stubChecker)(nil)
var _ service.EpisodeChecker = (*failingCheckerForB)(nil)

// Sanity: import database to ensure the test binary links the same DB
// driver tree the production binary uses (cross-checks go.mod resolution).
var _ = database.Config{}
