package domain

import "time"

// AnimeAiringOccurrence is one provider-confirmed episode airing. The
// (anime_id, episode) primary key lets later provider syncs correct a timestamp
// without duplicating the episode.
type AnimeAiringOccurrence struct {
	AnimeID string    `gorm:"type:uuid;primaryKey;index:idx_airing_occurrences_range,priority:2" json:"anime_id"`
	Episode int       `gorm:"primaryKey" json:"episode"`
	AiredAt time.Time `gorm:"not null;index:idx_airing_occurrences_range,priority:1" json:"aired_at"`
	Source  string    `gorm:"size:16;not null" json:"source"`

	Anime *Anime `gorm:"foreignKey:AnimeID;references:ID" json:"anime,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
