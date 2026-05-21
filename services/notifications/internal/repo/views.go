// READ-ONLY VIEWS — the structs in this file project the existing physical
// tables owned by other services. They are intentionally MINIMAL — only the
// fields Phase 2's detector reads. They MUST NOT appear in db.AutoMigrate
// in cmd/notifications-api/main.go. Letting GORM "auto-migrate" a view
// struct against the source table can silently add columns or override
// defaults belonging to the owning service.
//
// Source of truth:
//   - WatchHistoryView  → services/player/internal/domain/watch.go::WatchHistory
//   - AnimeListView     → services/player/internal/domain/watch.go::AnimeListEntry
//   - AnimeView         → services/catalog/internal/domain/anime.go::Anime
//
// v1.0 Notifications Engine — workstream notifications, Phase 1 (D-01 single
// shared DB allows the same *gorm.DB handle to read across services with no
// extra connection plumbing).
package repo

// WatchHistoryView is a read-only projection of player.watch_history. Used
// by Phase 2's detector to compute (a) which combos a user has ever watched
// and (b) the user's highest watched episode per combo.
type WatchHistoryView struct {
	UserID        string `gorm:"column:user_id"`
	AnimeID       string `gorm:"column:anime_id"`
	EpisodeNumber int    `gorm:"column:episode_number"`
	Player        string `gorm:"column:player"`
	Language      string `gorm:"column:language"`
	WatchType     string `gorm:"column:watch_type"`
	TranslationID string `gorm:"column:translation_id"`
}

// TableName binds the projection to the existing physical table.
func (WatchHistoryView) TableName() string { return "watch_history" }

// AnimeListView is a read-only projection of player.anime_list. Phase 2's
// detector uses status ∈ {watching, plan_to_watch} as a hint when no
// watch_history rows exist yet (e.g. a user adds an anime to plan_to_watch
// before episode 1 airs — the first new_episode then fires).
type AnimeListView struct {
	UserID  string `gorm:"column:user_id"`
	AnimeID string `gorm:"column:anime_id"`
	Status  string `gorm:"column:status"`
}

// TableName binds the projection to the existing physical table.
func (AnimeListView) TableName() string { return "anime_list" }

// AnimeView is a read-only projection of catalog.animes. Phase 2's detector
// reads name + poster for the notification payload; future phases may pull
// more fields for richer payloads.
type AnimeView struct {
	ID          string `gorm:"column:id"`
	ShikimoriID string `gorm:"column:shikimori_id"`
	Status      string `gorm:"column:status"`
	Name        string `gorm:"column:name"`
	NameRU      string `gorm:"column:name_ru"`
	PosterURL   string `gorm:"column:poster_url"`
}

// TableName binds the projection to the existing physical table.
func (AnimeView) TableName() string { return "animes" }
