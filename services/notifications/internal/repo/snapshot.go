package repo

import "gorm.io/gorm"

// SnapshotRepository will host the cache-update + bulk-load operations for
// parser_episode_snapshots once Phase 2's detector ships.
//
// Phase 1 only needs the table to exist — created by db.AutoMigrate against
// domain.ParserEpisodeSnapshot in cmd/notifications-api/main.go. The
// methods are deliberately deferred so the Phase 1 review surface stays
// minimal and Phase 2's diff is "add detector logic", not "wire repo".
//
// TODO(phase 2 detector):
//   - BulkLoad(combos []ComboKey) (map[ComboKey]int, error)
//   - BulkUpsert(snaps []domain.ParserEpisodeSnapshot) error
//   - Cleanup(olderThan time.Duration) (int64, error)
//
// See services/notifications/internal/job/ for the cron jobs that will call
// the above.
type SnapshotRepository struct {
	db *gorm.DB
}

// NewSnapshotRepository constructs the (currently empty) repo. Wired into
// the boot graph in cmd/notifications-api/main.go so Phase 2 only needs to
// add methods, not restructure boot.
func NewSnapshotRepository(db *gorm.DB) *SnapshotRepository {
	return &SnapshotRepository{db: db}
}
