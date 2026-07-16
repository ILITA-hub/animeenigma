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
	ID          string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ShikimoriID string `gorm:"size:50;uniqueIndex" json:"shikimori_id"`
	MalID       string `gorm:"size:50;index" json:"mal_id,omitempty"`
	Name        string `gorm:"size:500;index" json:"name"`
	NameRU      string `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP      string `gorm:"size:500" json:"name_jp,omitempty"`
	Synonyms    string `gorm:"size:1000" json:"synonyms,omitempty"`
	PosterURL   string `gorm:"size:1000" json:"poster_url,omitempty"`
	Description string `gorm:"type:text" json:"description,omitempty"`
	URL         string `gorm:"size:1000" json:"url,omitempty"`
	// Seyu — the character's voice cast, stored inline as JSON (serializer:json,
	// portable across Postgres + the SQLite test DB). Populated by the
	// per-character REST fetch in GetCharacterByID.
	Seyu      []CharacterSeyu `gorm:"serializer:json" json:"seyu,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `gorm:"index" json:"-"`
}

// CharacterSeyu is one voice actor for a character. Stored inline on Character
// (owner directive: wire the cast onto existing characters, no separate seiyu
// table). Sourced from Shikimori REST /api/characters/{id} → seyu[]. The list
// mixes JP seiyu and localized dub actors with no language flag from Shikimori.
type CharacterSeyu struct {
	ShikimoriID string `json:"shikimori_id"`
	Name        string `json:"name"`
	NameRU      string `json:"name_ru,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	URL         string `json:"url,omitempty"`
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
