package domain

import "time"

// AnimePersonRole is ONE flat, denormalized staff/crew credit for an anime.
//
// Deliberately NOT normalized (owner directive): there is no separate Person
// entity and no m2m join table. Person identity (name/poster) lives inline and
// repeats per role — one row per (anime, person, role). Role is a scalar
// column so the read model can group/sort by it directly. Sourced from
// Shikimori GraphQL personRoles, filtered to a headline whitelist at parse
// time (see parser/shikimori/staff.go).
type AnimePersonRole struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID           string    `gorm:"type:uuid;index:idx_person_roles_anime" json:"anime_id"`
	ShikimoriPersonID string    `gorm:"size:50;index" json:"shikimori_person_id,omitempty"`
	Name              string    `gorm:"size:500" json:"name"`
	NameRU            string    `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP            string    `gorm:"size:500" json:"name_jp,omitempty"`
	PosterURL         string    `gorm:"size:1000" json:"poster_url,omitempty"`
	Role              string    `gorm:"size:100;index" json:"role"`        // canonical EN, scalar
	RoleRU            string    `gorm:"size:100" json:"role_ru,omitempty"` // Shikimori rolesRu (free)
	IsProducer        bool      `json:"is_producer,omitempty"`
	IsMangaka         bool      `json:"is_mangaka,omitempty"`
	Position          int       `gorm:"default:0" json:"position"` // whitelist rank
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
