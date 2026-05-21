package domain

import "time"

// ParserEpisodeSnapshot is the per-combo "last seen episode count" cache the
// Phase 2 detector reads/writes. Stable composite key
// (anime_id, player, language, watch_type, translation_id) is enforced by
// the uk_combo unique index — GORM expresses it via the
// "uniqueIndex:uk_combo,priority:N" tag syntax.
//
// Phase 1 ships only the table + the GORM type; no methods yet (Phase 2's
// detector is what fills it). The table just needs to exist so AutoMigrate
// is correct when the detector ships.
type ParserEpisodeSnapshot struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID       string    `gorm:"type:uuid;not null;uniqueIndex:uk_combo,priority:1" json:"anime_id"`
	Player        string    `gorm:"size:20;not null;uniqueIndex:uk_combo,priority:2" json:"player"`
	Language      string    `gorm:"size:5;not null;uniqueIndex:uk_combo,priority:3" json:"language"`
	WatchType     string    `gorm:"size:5;not null;uniqueIndex:uk_combo,priority:4" json:"watch_type"`
	TranslationID string    `gorm:"size:50;not null;uniqueIndex:uk_combo,priority:5" json:"translation_id"`
	LatestEpisode int       `gorm:"not null" json:"latest_episode"`
	CheckedAt     time.Time `json:"checked_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName pins the physical table.
func (ParserEpisodeSnapshot) TableName() string { return "parser_episode_snapshots" }
