package domain

import "time"

// CollectionEntry is one row per (user, card) — the player's owned cards.
// Duplicate pulls bump Count rather than inserting a new row (spec §4.6,
// decision #7 "вариант D"). FirstObtainedAt is set on the first ever obtain
// and never moves on subsequent dupes.
type CollectionEntry struct {
	UserID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_card,priority:1" json:"user_id"`
	CardID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_card,priority:2" json:"card_id"`
	Count           int       `gorm:"not null;default:1" json:"count"`
	FirstObtainedAt time.Time `json:"first_obtained_at"`
}

func (CollectionEntry) TableName() string { return "gacha_collection" }

// PityCounter is the per-(user, banner) hard-pity counter (spec §4.7,
// decision #11). PullsSinceSSR increments on every roll for that banner and
// resets to 0 on any SSR (natural or pity-forced). Counters are isolated per
// banner — pulls on banner A never move banner B's counter.
type PityCounter struct {
	UserID        string `gorm:"type:uuid;not null;uniqueIndex:idx_user_banner,priority:1" json:"user_id"`
	BannerID      string `gorm:"type:uuid;not null;uniqueIndex:idx_user_banner,priority:2" json:"banner_id"`
	PullsSinceSSR int    `gorm:"not null;default:0" json:"pulls_since_ssr"`
}

func (PityCounter) TableName() string { return "gacha_pity" }
