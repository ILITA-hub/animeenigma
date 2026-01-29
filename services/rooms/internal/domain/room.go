package domain

import "time"

type Room struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	CreatorID   string    `json:"creator_id" db:"creator_id"`
	MaxPlayers  int       `json:"max_players" db:"max_players"`
	Status      string    `json:"status" db:"status"` // waiting, playing, finished
	CurrentRound int      `json:"current_round" db:"current_round"`
	TotalRounds int       `json:"total_rounds" db:"total_rounds"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Player struct {
	ID       string `json:"id" db:"id"`
	RoomID   string `json:"room_id" db:"room_id"`
	UserID   string `json:"user_id" db:"user_id"`
	Username string `json:"username" db:"username"`
	Score    int    `json:"score" db:"score"`
	IsReady  bool   `json:"is_ready" db:"is_ready"`
}

type GameRound struct {
	ID          string    `json:"id" db:"id"`
	RoomID      string    `json:"room_id" db:"room_id"`
	RoundNumber int       `json:"round_number" db:"round_number"`
	AnimeID     string    `json:"anime_id" db:"anime_id"`
	OpeningURL  string    `json:"opening_url" db:"opening_url"`
	StartTime   time.Time `json:"start_time" db:"start_time"`
	EndTime     time.Time `json:"end_time" db:"end_time"`
}

type LeaderboardEntry struct {
	UserID   string `json:"user_id" db:"user_id"`
	Username string `json:"username" db:"username"`
	TotalScore int  `json:"total_score" db:"total_score"`
	GamesPlayed int `json:"games_played" db:"games_played"`
	GamesWon   int  `json:"games_won" db:"games_won"`
}

// WebSocket message types
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type CreateRoomRequest struct {
	Name       string `json:"name"`
	MaxPlayers int    `json:"max_players"`
}

type JoinRoomRequest struct {
	RoomID string `json:"room_id"`
}

type SubmitAnswerRequest struct {
	RoomID    string `json:"room_id"`
	RoundID   string `json:"round_id"`
	AnimeID   string `json:"anime_id"`
	TimeTaken int    `json:"time_taken"` // in seconds
}
