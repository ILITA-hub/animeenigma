package signals

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"gorm.io/gorm"
)

// S2Metadata is the item-item metadata-overlap signal. It ranks each candidate
// by the maximum Jaccard similarity between its attribute set and the
// attribute sets of the user's top-scored anime ("seeds").
//
// Phase 11 scope (per CONTEXT.md decisions §S2 attribute selection — path a):
// S2 ships GENRES-ONLY because tags / studios / demographic / source / type
// / producers do NOT exist on the `animes` schema today. Adding them
// requires Shikimori parser changes + GraphQL changes + backfill, which is
// substantially more than 1 day of work. Phase 12 (S5 inventory + backfill)
// is responsible for adding those attribute dimensions; once they land,
// S2 should be extended to compute Jaccard over the union of the available
// attribute sets — the "max-Jaccard across seeds" rule and the seed-selection
// thresholds stay the same.
//
// Algorithm:
//  1. Seed selection — load anime_list rows for the user with score >= 7
//     (primary threshold). If empty, fall back to score >= 5. If still
//     empty, return an empty map (cold-start).
//  2. Genre lookup — pull `anime_genres` rows for the seed set and the
//     candidate set. Group both into per-anime sets in Go.
//  3. Score — for each candidate, max(Jaccard(seed.genres, candidate.genres))
//     across the seed set. Candidates with max == 0 are omitted (the
//     normalizer treats absent entries as zero).
//
// S2 is computed entirely at request time — there is no persistence column
// (no rec_user_signals.s2_vector). Per-request work is bounded at
// O(seeds * candidates) tiny set ops on small (~5–15 element) genre sets,
// which is sub-millisecond in Go for ~3500-anime catalogs.
type S2Metadata struct {
	db *gorm.DB
}

const (
	// s2ScoreThresholdPrimary is the user-rating cutoff that defines a
	// "top-scored anime" used as a seed for similarity. Mirrors S6's
	// qualifying-completion threshold (spec §13).
	s2ScoreThresholdPrimary = 7
	// s2ScoreThresholdFallback is used when no primary-threshold seeds exist
	// — a soft graceful degradation before declaring cold-start.
	s2ScoreThresholdFallback = 5
)

// NewS2Metadata wires S2 with the player DB handle.
func NewS2Metadata(db *gorm.DB) *S2Metadata {
	return &S2Metadata{db: db}
}

// ID returns the stable signal identifier "s2".
func (s *S2Metadata) ID() recs.SignalID { return recs.SignalID("s2") }

// Precompute is a no-op — S2 is request-time only.
func (s *S2Metadata) Precompute(_ context.Context, _ recs.UserID) error {
	return nil
}

// genreRow projects an (anime_id, genre_id) pair from anime_genres.
type s2GenreRow struct {
	AnimeID string
	GenreID string
}

// Score returns max-Jaccard similarity between each candidate's genre set
// and the user's seed set. Candidates with no overlap are omitted; users
// with no qualifying seeds return an empty map (cold-start, the normalizer
// downstream treats this as all-zero -> ensemble degrades cleanly).
func (s *S2Metadata) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	// 1. Seed selection — primary threshold first, fallback second.
	seeds, err := s.loadSeeds(ctx, userID, s2ScoreThresholdPrimary)
	if err != nil {
		return nil, fmt.Errorf("s2: load primary seeds: %w", err)
	}
	if len(seeds) == 0 {
		seeds, err = s.loadSeeds(ctx, userID, s2ScoreThresholdFallback)
		if err != nil {
			return nil, fmt.Errorf("s2: load fallback seeds: %w", err)
		}
		if len(seeds) == 0 {
			return out, nil
		}
	}

	// 2. Genre lookups for seeds + candidates in two batched queries.
	seedGenres, err := s.loadGenres(ctx, seeds)
	if err != nil {
		return nil, fmt.Errorf("s2: load seed genres: %w", err)
	}
	candidateGenres, err := s.loadGenres(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("s2: load candidate genres: %w", err)
	}

	// 3. For each candidate, max Jaccard across all seeds.
	for _, candidateID := range candidates {
		cgenres := candidateGenres[candidateID]
		if len(cgenres) == 0 {
			continue
		}
		var best float64
		for _, sgenres := range seedGenres {
			if j := jaccard(sgenres, cgenres); j > best {
				best = j
			}
		}
		if best > 0 {
			out[candidateID] = recs.RawScore(best)
		}
	}
	return out, nil
}

func (s *S2Metadata) loadSeeds(ctx context.Context, userID recs.UserID, threshold int) ([]string, error) {
	var seeds []string
	err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("anime_id").
		Where("user_id = ? AND score >= ?", userID, threshold).
		Pluck("anime_id", &seeds).Error
	if err != nil {
		return nil, err
	}
	return seeds, nil
}

func (s *S2Metadata) loadGenres(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{}, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}
	var rows []s2GenreRow
	err := s.db.WithContext(ctx).
		Table("anime_genres").
		Select("anime_id, genre_id").
		Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		set, ok := out[r.AnimeID]
		if !ok {
			set = make(map[string]struct{})
			out[r.AnimeID] = set
		}
		set[r.GenreID] = struct{}{}
	}
	return out, nil
}

// jaccard returns |A∩B| / |A∪B|. Returns 0 when either set is empty.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersect := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}
