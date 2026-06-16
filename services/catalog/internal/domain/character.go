package domain

import (
	"time"

	"gorm.io/gorm"
)

// Character is an anime character sourced from Shikimori GraphQL.
// Stored durably in Postgres; the catalog service hot-caches it in Redis.
// Synonyms are stored as a single " / "-joined string to avoid a pq array
// dependency (the frontend never splits them — display only).
type Character struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ShikimoriID string         `gorm:"size:50;uniqueIndex" json:"shikimori_id"`
	MalID       string         `gorm:"size:50;index" json:"mal_id,omitempty"`
	Name        string         `gorm:"size:500;index" json:"name"`
	NameRU      string         `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP      string         `gorm:"size:500" json:"name_jp,omitempty"`
	Synonyms    string         `gorm:"size:1000" json:"synonyms,omitempty"`
	PosterURL   string         `gorm:"size:1000" json:"poster_url,omitempty"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	URL         string         `gorm:"size:1000" json:"url,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// AnimeCharacter is the explicit join row for the anime <-> character
// relation. Managed directly by CharacterRepository (NOT a GORM m2m
// association on Anime) so we control ordering (Main before Supporting,
// then Position). Composite PK (AnimeID, CharacterID) prevents dup joins.
type AnimeCharacter struct {
	AnimeID     string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	CharacterID string    `gorm:"type:uuid;primaryKey" json:"character_id"`
	Role        string    `gorm:"size:20;index" json:"role"`
	Position    int       `gorm:"default:0" json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

// AnimeCharacterView is the flattened read model returned by
// CharacterRepository.GetByAnimeID — a Character plus its per-anime
// role/position. Populated via a raw JOIN scan.
type AnimeCharacterView struct {
	Character
	Role     string `json:"role" gorm:"column:role"`
	Position int    `json:"position" gorm:"column:position"`
}
