package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S1ScoreCluster predicts each candidate's score for a target user via
// Pearson-correlation k-NN over anime_list.score history.
//
// Algorithm (per CONTEXT.md decisions §S1 + spec §3.3):
//  1. Load the target user's rated anime (score > 0) from anime_list.
//  2. Cold-start gate: if the target has fewer than s1ColdStartThreshold
//     scored anime, persist an empty {} vector and return — the normalizer
//     treats absent entries as zero so the ensemble cleanly degrades to
//     S3+S4-leaning behaviour without NaN propagation (REC-SIG-03).
//  3. For every other user who has rated >= s1MinOverlap of the target's
//     anime, compute Pearson similarity over the OVERLAPPING set only.
//  4. Keep the top-s1NeighborCount neighbours by absolute similarity.
//  5. For every unwatched candidate (animes the target has not yet rated,
//     visible + non-deleted), compute the weighted-average predicted score
//     across neighbours who DID rate that candidate:
//     predicted = sum(sim * neighbour_score) / sum(|sim|)
//     Candidates that no neighbour scored are omitted (the normalizer
//     treats them as zero).
//  6. Persist the {anime_id: predicted_score} map to rec_user_signals.s1_vector
//     as JSONB. Score reads the persisted map at request time.
//
// The persistence-then-read split matches Phase 9's foundation contract
// and keeps per-request work bounded to a single rec_user_signals row read.
type S1ScoreCluster struct {
	db   *gorm.DB
	repo *repo.RecsRepository
}

const (
	// s1NeighborCount is the standard k for collaborative-filtering k-NN.
	// Tunable later if Phase 14 admin breakdowns show weak signal.
	s1NeighborCount = 10
	// s1MinOverlap is the minimum number of co-rated anime required before
	// a peer is considered a neighbour. Without this guard a single shared
	// anime would yield Pearson=±1 spuriously.
	s1MinOverlap = 2
	// s1ColdStartThreshold is the minimum number of scored anime the target
	// user must have before S1 produces non-zero output. Below it the signal
	// returns empty and the ensemble degrades to trending-leaning.
	s1ColdStartThreshold = 3
)

// NewS1ScoreCluster wires S1 with the player DB handle and the recs repository.
func NewS1ScoreCluster(db *gorm.DB, recsRepo *repo.RecsRepository) *S1ScoreCluster {
	return &S1ScoreCluster{db: db, repo: recsRepo}
}

// ID returns the stable signal identifier "s1".
func (s *S1ScoreCluster) ID() recs.SignalID { return recs.SignalID("s1") }

// listRow projects an (anime_id, score) row.
type s1ListRow struct {
	UserID  string
	AnimeID string
	Score   int
}

// Precompute computes the S1 prediction vector for a user and persists it.
// Idempotent — repeated calls upsert the same vector with a fresh
// LastComputed stamp.
func (s *S1ScoreCluster) Precompute(ctx context.Context, userID recs.UserID) error {
	now := time.Now().UTC()

	// 1. Load target's scored anime.
	var targetRows []s1ListRow
	if err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("anime_id, score").
		Where("user_id = ? AND score > 0", userID).
		Scan(&targetRows).Error; err != nil {
		return fmt.Errorf("s1: load target scores: %w", err)
	}

	// 2. Cold-start gate. Persist an empty vector so Score returns cleanly
	// and the user_orchestrator's "did this row get touched?" check works.
	if len(targetRows) < s1ColdStartThreshold {
		return s.persistVector(ctx, userID, "{}", now)
	}

	targetScores := make(map[string]int, len(targetRows))
	targetAnimeIDs := make([]string, 0, len(targetRows))
	for _, r := range targetRows {
		targetScores[r.AnimeID] = r.Score
		targetAnimeIDs = append(targetAnimeIDs, r.AnimeID)
	}

	// 3. Load every OTHER user's scores on the target's anime — used to
	// compute Pearson over the overlapping anime set.
	var overlapRows []s1ListRow
	if err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("user_id, anime_id, score").
		Where("user_id != ? AND score > 0 AND anime_id IN ?", userID, targetAnimeIDs).
		Scan(&overlapRows).Error; err != nil {
		return fmt.Errorf("s1: load neighbour overlap: %w", err)
	}

	overlapByUser := make(map[string]map[string]int)
	for _, r := range overlapRows {
		m, ok := overlapByUser[r.UserID]
		if !ok {
			m = make(map[string]int)
			overlapByUser[r.UserID] = m
		}
		m[r.AnimeID] = r.Score
	}

	// 4. Compute Pearson similarity for each candidate neighbour. Skip
	// neighbours whose overlap is below the minimum.
	type neighbour struct {
		userID string
		sim    float64
	}
	var neighbours []neighbour
	for nbrID, scores := range overlapByUser {
		if len(scores) < s1MinOverlap {
			continue
		}
		// Build overlapping vectors in deterministic order.
		ax := make([]float64, 0, len(scores))
		bx := make([]float64, 0, len(scores))
		for animeID, nbrScore := range scores {
			ax = append(ax, float64(targetScores[animeID]))
			bx = append(bx, float64(nbrScore))
		}
		sim := pearson(ax, bx)
		if sim == 0 {
			continue
		}
		neighbours = append(neighbours, neighbour{userID: nbrID, sim: sim})
	}

	// Top-k by absolute similarity (negative correlations carry signal too —
	// the predicted score formula handles sign correctly).
	sort.Slice(neighbours, func(i, j int) bool {
		return math.Abs(neighbours[i].sim) > math.Abs(neighbours[j].sim)
	})
	if len(neighbours) > s1NeighborCount {
		neighbours = neighbours[:s1NeighborCount]
	}

	if len(neighbours) == 0 {
		return s.persistVector(ctx, userID, "{}", now)
	}

	// 5. Load all of the chosen neighbours' scores so we can predict
	// candidates beyond the target's overlap set.
	nbrIDs := make([]string, len(neighbours))
	for i, n := range neighbours {
		nbrIDs[i] = n.userID
	}
	var nbrScores []s1ListRow
	if err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("user_id, anime_id, score").
		Where("user_id IN ? AND score > 0", nbrIDs).
		Scan(&nbrScores).Error; err != nil {
		return fmt.Errorf("s1: load neighbour score sets: %w", err)
	}
	scoresByUser := make(map[string]map[string]int, len(neighbours))
	for _, r := range nbrScores {
		m, ok := scoresByUser[r.UserID]
		if !ok {
			m = make(map[string]int)
			scoresByUser[r.UserID] = m
		}
		m[r.AnimeID] = r.Score
	}

	// Load the candidate universe — every visible non-deleted anime the
	// target has NOT already scored. Same shape as S11.CandidatePool but
	// without the user-specific completed/dropped exclusion (S11 layer
	// handles that at request time).
	var candidates []string
	if err := s.db.WithContext(ctx).
		Table("animes").
		Select("id").
		Where("hidden = ? AND deleted_at IS NULL", false).
		Where("id NOT IN ?", targetAnimeIDs).
		Pluck("id", &candidates).Error; err != nil {
		return fmt.Errorf("s1: load candidate universe: %w", err)
	}

	// 6. For each candidate, weighted-average predicted score across
	// neighbours who scored it.
	predictions := make(map[string]float64, len(candidates))
	for _, candidate := range candidates {
		var num, den float64
		for _, n := range neighbours {
			score, ok := scoresByUser[n.userID][candidate]
			if !ok {
				continue
			}
			num += n.sim * float64(score)
			den += math.Abs(n.sim)
		}
		if den < 1e-9 {
			continue // no neighbour rated this candidate -> omit (= zero)
		}
		predictions[candidate] = num / den
	}

	encoded, err := json.Marshal(predictions)
	if err != nil {
		return fmt.Errorf("s1: marshal vector: %w", err)
	}
	return s.persistVector(ctx, userID, string(encoded), now)
}

// persistVector upserts the s1_vector for a user, preserving any existing
// s5_affinity value the orchestrator may already have written.
func (s *S1ScoreCluster) persistVector(ctx context.Context, userID recs.UserID, vector string, when time.Time) error {
	existing, err := s.repo.GetUserSignals(ctx, userID)
	if err != nil {
		return fmt.Errorf("s1: load existing user signals: %w", err)
	}
	row := &domain.RecUserSignals{
		UserID:       userID,
		S1Vector:     vector,
		S5Affinity:   "{}",
		LastComputed: when,
	}
	if existing != nil {
		// Don't clobber S5 / S6 fields the orchestrator may have populated.
		if existing.S5Affinity != "" {
			row.S5Affinity = existing.S5Affinity
		}
		row.S6SeedAnimeID = existing.S6SeedAnimeID
		row.S6SeedCompletedAt = existing.S6SeedCompletedAt
		row.S6SeedScore = existing.S6SeedScore
	}
	return s.repo.UpsertUserSignals(ctx, row)
}

// Score reads the persisted s1_vector for the user and returns the subset
// matching the candidate slice. Candidates absent from the vector are
// omitted — MinMaxNormalize treats them as zero, the correct cold-start
// behaviour.
func (s *S1ScoreCluster) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}
	row, err := s.repo.GetUserSignals(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row == nil || row.S1Vector == "" {
		return out, nil
	}

	parsed := make(map[string]float64)
	if err := json.Unmarshal([]byte(row.S1Vector), &parsed); err != nil {
		return nil, fmt.Errorf("s1: unmarshal vector: %w", err)
	}
	if len(parsed) == 0 {
		return out, nil
	}

	for _, c := range candidates {
		if v, ok := parsed[c]; ok {
			out[c] = recs.RawScore(v)
		}
	}
	return out, nil
}

// pearson returns the Pearson correlation coefficient over two equal-length
// score vectors. Returns 0 when the vectors are empty / mismatched / one of
// them has zero variance (avoids division by zero — those neighbours don't
// carry signal anyway).
func pearson(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	n := float64(len(a))
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= n
	mb /= n

	var num, da, db float64
	for i := range a {
		dx := a[i] - ma
		dy := b[i] - mb
		num += dx * dy
		da += dx * dx
		db += dy * dy
	}
	den := math.Sqrt(da * db)
	if den < 1e-9 {
		return 0
	}
	return num / den
}
