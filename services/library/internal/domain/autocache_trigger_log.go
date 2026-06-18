package domain

import "time"

// AutocacheTriggerLog is one append-only cause→effect record: a user-driven
// autocache trigger (player Logic B next_ep, or catalog backfill serve-miss) and
// the TARGET episode it caused the autocache to fetch. It mirrors
// migrations/012_autocache_trigger_log.sql 1:1.
//
// The CAUSE is (UserID/Username watched WatchedEpisode of MalID via the
// Player/Language/WatchType combo, at CreatedAt). The EFFECT is the download of
// (MalID, TargetEpisode) — the dashboard joins those two columns to
// library_episodes/library_jobs to render the resulting file + its status next to
// the trigger.
//
// TargetEpisode = WatchedEpisode+1 for Logic B (next-episode pull), = the watched
// episode for backfill. The aggregate scheduler Logic A push is intentionally NOT
// logged here (no single user) — it stays in the by-trigger metrics.
type AutocacheTriggerLog struct {
	// ID is server-filled via gen_random_uuid() (migration default); the repo also
	// sets it explicitly so SQLite-backed unit tests (no gen_random_uuid) work.
	ID             string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid();column:id" json:"id"`
	MalID          string    `gorm:"not null;column:mal_id" json:"mal_id"`
	TargetEpisode  int       `gorm:"not null;column:target_episode" json:"target_episode"`
	Reason         string    `gorm:"not null;column:reason" json:"reason"`
	UserID         string    `gorm:"not null;default:'';column:user_id" json:"user_id"`
	Username       string    `gorm:"not null;default:'';column:username" json:"username"`
	WatchPlayer    string    `gorm:"not null;default:'';column:watch_player" json:"watch_player"`
	WatchLanguage  string    `gorm:"not null;default:'';column:watch_language" json:"watch_language"`
	WatchType      string    `gorm:"not null;default:'';column:watch_type" json:"watch_type"`
	WatchedEpisode int       `gorm:"not null;default:0;column:watched_episode" json:"watched_episode"`
	CreatedAt      time.Time `gorm:"not null;default:now();column:created_at" json:"created_at"`
}

// TableName pins the table name (GORM would otherwise pluralize).
func (AutocacheTriggerLog) TableName() string { return "autocache_trigger_log" }
