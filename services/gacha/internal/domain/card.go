package domain

import (
	"time"

	"gorm.io/gorm"
)

// Rarity is the card tier. Pull odds per tier live in Phase 3 config.
type Rarity string

const (
	RarityN   Rarity = "N"
	RarityR   Rarity = "R"
	RaritySR  Rarity = "SR"
	RaritySSR Rarity = "SSR"
)

// ValidRarity reports whether r is one of the four known tiers.
func ValidRarity(r Rarity) bool {
	switch r {
	case RarityN, RarityR, RaritySR, RaritySSR:
		return true
	}
	return false
}

// Card is an admin-curated collectible character card (spec §4.1). ImagePath
// is the object key inside the gacha-cards MinIO bucket (e.g.
// "cards/<uuid>.webp") — the public URL is derived as /api/gacha/images/<path>.
type Card struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	SourceTitle string         `gorm:"size:256" json:"source_title"`
	ImagePath   string         `gorm:"size:512;not null" json:"image_path"`
	// BackPath is the optional card-back image key; frontend falls back to the
	// branded default when empty.
	BackPath    string         `gorm:"size:512" json:"back_path"`
	Rarity      Rarity         `gorm:"size:8;not null;index" json:"rarity"`
	Enabled     bool           `gorm:"not null;default:false;index" json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Card) TableName() string { return "gacha_cards" }

// Group is an admin-created named collection of cards (spec §4.8) — an
// organizational tool only; it never affects pulls (banners do).
type Group struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"size:128;not null;uniqueIndex" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Group) TableName() string { return "gacha_groups" }

// CardGroup is the M:N card↔group join row.
type CardGroup struct {
	GroupID string `gorm:"type:uuid;not null;uniqueIndex:idx_card_group,priority:1" json:"group_id"`
	CardID  string `gorm:"type:uuid;not null;uniqueIndex:idx_card_group,priority:2" json:"card_id"`
}

func (CardGroup) TableName() string { return "gacha_card_groups" }

// Banner is a gameplay pull pool (spec §4.2): a scheduled, admin-curated
// selection of cards. Exactly one banner should have IsStandard=true (the
// always-on pool); the rest are timed events layered on top.
type Banner struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Description string         `gorm:"size:1024" json:"description"`
	ArtPath     string         `gorm:"size:512" json:"art_path"`
	// BackdropPath is the separately uploaded slider/spin-page background image key.
	BackdropPath string        `gorm:"size:512" json:"backdrop_path"`
	IsStandard  bool           `gorm:"not null;default:false" json:"is_standard"`
	Enabled     bool           `gorm:"not null;default:false;index" json:"enabled"`
	ActiveFrom  *time.Time     `json:"active_from,omitempty"`
	ActiveTo    *time.Time     `json:"active_to,omitempty"`
	SortOrder   int            `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Banner) TableName() string { return "gacha_banners" }

// BannerCard is the M:N banner↔card join row (the pull pool contents).
type BannerCard struct {
	BannerID string `gorm:"type:uuid;not null;uniqueIndex:idx_banner_card,priority:1" json:"banner_id"`
	CardID   string `gorm:"type:uuid;not null;uniqueIndex:idx_banner_card,priority:2" json:"card_id"`
}

func (BannerCard) TableName() string { return "gacha_banner_cards" }
