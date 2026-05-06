package domain

import "time"

// RecUserSignals stores per-user precomputed signals. See spec §4.1.
//
// JSONB fields hold sparse maps (typed as string in Go for raw JSON storage —
// Phase 11 (S1) and Phase 12 (S5) marshal/unmarshal at the service layer):
//   - S1Vector:   {anime_id: predicted_score}    (k-NN output)
//   - S5Affinity: {attr_id: affinity_value}       (TF-IDF output, keyed by
//                                                  "studio:Madhouse", "tag:slice-of-life", etc.)
//
// S6Seed* track the most recent qualifying completion (score >= 7,
// completed within last 7 days) for the "Because you finished X" pin.
//
// FK references (added via raw SQL in cmd/player-api/main.go because GORM
// AutoMigrate does not infer FKs from struct tags alone):
//   - user_id          → users(id)  ON DELETE CASCADE
//   - s6_seed_anime_id → animes(id) ON DELETE SET NULL
type RecUserSignals struct {
	UserID            string     `gorm:"type:uuid;primaryKey" json:"user_id"`
	S1Vector          string     `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"s1_vector"`
	S5Affinity        string     `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"s5_affinity"`
	S6SeedAnimeID     *string    `gorm:"type:uuid" json:"s6_seed_anime_id,omitempty"`
	S6SeedCompletedAt *time.Time `json:"s6_seed_completed_at,omitempty"`
	S6SeedScore       *int       `json:"s6_seed_score,omitempty"`
	LastComputed      time.Time  `gorm:"not null;default:now();index" json:"last_computed"`
}

func (RecUserSignals) TableName() string { return "rec_user_signals" }

// RecPopulationSignals stores population-wide signals (shared across users).
// See spec §4.1.
//
// FK references:
//   - anime_id → animes(id) ON DELETE CASCADE
type RecPopulationSignals struct {
	AnimeID         string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	S3TrendingScore float32   `gorm:"not null;default:0" json:"s3_trending_score"`
	S4RecencyScore  float32   `gorm:"not null;default:0" json:"s4_recency_score"`
	LastComputed    time.Time `gorm:"not null;default:now()" json:"last_computed"`
}

func (RecPopulationSignals) TableName() string { return "rec_population_signals" }

// RecCompletionCoOccurrence is the materialized seed -> candidate co-occurrence
// matrix for S6 local lookups. CoCount counts users who completed both seed
// and candidate with score >= 7. See spec §4.1.
//
// Composite primary key (seed_anime_id, candidate_anime_id).
//
// FK references:
//   - seed_anime_id      → animes(id) ON DELETE CASCADE
//   - candidate_anime_id → animes(id) ON DELETE CASCADE
type RecCompletionCoOccurrence struct {
	SeedAnimeID      string    `gorm:"type:uuid;primaryKey" json:"seed_anime_id"`
	CandidateAnimeID string    `gorm:"type:uuid;primaryKey" json:"candidate_anime_id"`
	CoCount          int       `gorm:"not null" json:"co_count"`
	LastComputed     time.Time `gorm:"not null;default:now()" json:"last_computed"`
}

func (RecCompletionCoOccurrence) TableName() string { return "rec_completion_co_occurrence" }
