package domain

import "time"

// Snapshot is the frozen answer (the 8 attributes + display fields) stored as
// JSONB on a daily puzzle. It is a PoolAnime — reuse the same shape so Compare
// works directly.
type Snapshot = PoolAnime

// DailyPuzzle is the secret for one calendar day (UTC). Immutable once created.
type DailyPuzzle struct {
	Date           string    `gorm:"primaryKey;size:10" json:"date"` // "2006-01-02"
	AnimeID        string    `gorm:"size:64;index" json:"anime_id"`
	AnswerSnapshot Snapshot  `gorm:"serializer:json" json:"answer_snapshot"`
	CreatedAt      time.Time `json:"created_at"`
}

func (DailyPuzzle) TableName() string { return "anidle_daily_puzzle" }

// UserGameResult is one user's game for a day+mode (logged-in only).
type UserGameResult struct {
	ID         string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     string     `gorm:"size:64;index:idx_anidle_result_user_date_mode,unique,priority:1" json:"user_id"`
	PuzzleDate string     `gorm:"size:10;index:idx_anidle_result_user_date_mode,unique,priority:2" json:"puzzle_date"`
	Mode       string     `gorm:"size:16;index:idx_anidle_result_user_date_mode,unique,priority:3" json:"mode"` // "daily"
	Solved     bool       `json:"solved"`
	GaveUp     bool       `json:"gave_up"` // finished-but-lost sentinel (distinct from "still playing")
	Attempts   int        `json:"attempts"`
	Guesses    []string   `gorm:"serializer:json" json:"guesses"` // ordered anime_ids
	SolvedAt   *time.Time `json:"solved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (UserGameResult) TableName() string { return "anidle_user_game_result" }

// UserStats is the per-user aggregate.
type UserStats struct {
	UserID            string         `gorm:"size:64;primaryKey" json:"user_id"`
	GamesPlayed       int            `json:"games_played"`
	GamesWon          int            `json:"games_won"`
	CurrentStreak     int            `json:"current_streak"`
	MaxStreak         int            `json:"max_streak"`
	GuessDistribution map[string]int `gorm:"serializer:json" json:"guess_distribution"` // attempts -> count
	LastPlayedDate    string         `gorm:"size:10" json:"last_played_date"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (UserStats) TableName() string { return "anidle_user_stats" }
