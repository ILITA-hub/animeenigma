package signals

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"gorm.io/gorm"
)

// S3Trending is the population trending signal. The raw score is the count of
// DISTINCT users who started watching an anime within the last 30 days
// (watch_history.watched_at >= NOW() - 30d).
//
// Architectural split:
//   - Precompute runs once per population cron tick. A single GROUP BY pulls
//     the count for every anime in one pass and upserts each row into
//     rec_population_signals via RecsRepository.UpsertPopulationSignal.
//   - Score reads back the persisted s3_trending_score for the candidate set.
//     This is intentional — re-counting per request would be expensive at
//     production scale. The 60-minute cron cadence is sufficient freshness
//     for a "trending" surface.
//
// Time math is parameterized in Go (time.Now().Add(-30*24*time.Hour))
// instead of using Postgres INTERVAL syntax so the same code path runs
// against in-memory SQLite test fixtures and the Postgres production DB.
type S3Trending struct {
	db   *gorm.DB
	repo *repo.RecsRepository
}

// NewS3Trending wires S3 with the player DB handle and the recs repository.
func NewS3Trending(db *gorm.DB, recsRepo *repo.RecsRepository) *S3Trending {
	return &S3Trending{db: db, repo: recsRepo}
}

// ID returns the stable signal identifier "s3".
func (s *S3Trending) ID() recs.SignalID { return recs.SignalID("s3") }

// trendingRow is the GROUP BY projection used by Precompute.
type trendingRow struct {
	AnimeID string
	Cnt     int
}

// Precompute runs a single GROUP BY query over watch_history rows from the
// last 30 days, counting DISTINCT users per anime, and upserts the result
// into rec_population_signals for each anime with at least one start.
//
// Animes with zero starts in the window are NOT upserted here — their rows
// (if any) keep their previous score and will be re-evaluated either when
// they receive a start, or when an admin/maintenance pass rewrites them.
// This keeps the cron cheap (only writes rows that actually changed).
func (s *S3Trending) Precompute(ctx context.Context, _ recs.UserID) error {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)

	var rows []trendingRow
	err := s.db.WithContext(ctx).
		Table("watch_history").
		Select("anime_id AS anime_id, COUNT(DISTINCT user_id) AS cnt").
		Where("watched_at >= ?", cutoff).
		Group("anime_id").
		Scan(&rows).Error
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, r := range rows {
		row := &domain.RecPopulationSignals{
			AnimeID:         r.AnimeID,
			S3TrendingScore: float32(r.Cnt),
			LastComputed:    now,
		}
		// UpsertPopulationSignal writes s3_trending_score AND s4_recency_score
		// AND last_computed in its DoUpdates clause. We must NOT clobber an
		// already-persisted s4 score with zero. The orchestrator (Phase 10
		// Task 4) is the canonical writer for s4 — but in case S3 runs first
		// we read the existing row and preserve s4. Cheap: this is bounded by
		// "anime that received a start in the last 30 days" which is small.
		// Read existing s4_recency_score (if any) so we don't clobber it.
		// Use a scalar Pluck to avoid gorm's default-logger noise on NotFound,
		// which would log every cold-start anime as "record not found" warnings.
		var existingS4 []float32
		if findErr := s.db.WithContext(ctx).
			Table("rec_population_signals").
			Where("anime_id = ?", r.AnimeID).
			Pluck("s4_recency_score", &existingS4).Error; findErr != nil {
			return findErr
		}
		if len(existingS4) > 0 {
			row.S4RecencyScore = existingS4[0]
		}
		// else: brand-new row; s4 stays 0 until the next S4 orchestrator pass.
		if err := s.repo.UpsertPopulationSignal(ctx, row); err != nil {
			return err
		}
	}

	return nil
}

// scoreRow is the projection used by Score.
type scoreRow struct {
	AnimeID         string
	S3TrendingScore float32
}

// Score reads the persisted s3_trending_score for each candidate from
// rec_population_signals. Candidates with no row are omitted (the normalizer
// treats absent entries as zero, which is the correct cold-start behavior).
func (s *S3Trending) Score(ctx context.Context, _ recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	var rows []scoreRow
	err := s.db.WithContext(ctx).
		Table("rec_population_signals").
		Select("anime_id, s3_trending_score").
		Where("anime_id IN ?", candidates).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		out[r.AnimeID] = recs.RawScore(r.S3TrendingScore)
	}
	return out, nil
}
