// Package domain holds the gacha service's persisted models and value types.
package domain

import "time"

// Credit/debit reasons recorded on every LedgerEntry. The (user_id, reason,
// ref) unique index uses these — keep them stable strings.
const (
	ReasonStarter        = "starter"
	ReasonEpisodeWatched = "episode_watched"
	ReasonDaily          = "daily"
	ReasonTitleCompleted = "title_completed"
	ReasonPullX1         = "pull_x1"
	ReasonPullX10        = "pull_x10"
)

// Wallet is one row per user. Balance is denormalized from the ledger and
// updated in the same transaction as each LedgerEntry insert.
type Wallet struct {
	UserID         string     `gorm:"type:uuid;primaryKey" json:"user_id"`
	Balance        int64      `gorm:"not null;default:0" json:"balance"`
	StarterGranted bool       `gorm:"not null;default:false" json:"starter_granted"`
	DailyStreak    int        `gorm:"not null;default:0" json:"daily_streak"`
	LastDailyAt    *time.Time `json:"last_daily_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Wallet) TableName() string { return "gacha_wallets" }

// LedgerEntry is the append-only source of truth for every balance change.
// Delta is positive for credits, negative for debits. Ref is an optional
// idempotency discriminator (e.g. "<anime_id>:<episode>"); when non-empty,
// the (UserID, Reason, Ref) unique index makes a duplicate insert a no-op.
type LedgerEntry struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;index:idx_ledger_user_created" json:"user_id"`
	Delta     int64     `gorm:"not null" json:"delta"`
	Reason    string    `gorm:"size:32;not null" json:"reason"`
	Ref       string    `gorm:"size:128;not null;default:''" json:"ref"`
	CreatedAt time.Time `gorm:"index:idx_ledger_user_created" json:"created_at"`
}

func (LedgerEntry) TableName() string { return "gacha_ledger" }
