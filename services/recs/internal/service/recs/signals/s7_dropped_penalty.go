package signals

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S7DroppedPenalty is the negative "more like what you dropped" signal
// (spec 2026-06-11 Phase 3). It mirrors S2's stateless request-time pattern,
// inverted: seeds are the user's DROPPED anime, similarity is max Jaccard
// over the namespaced union of genre IDs + tag IDs, and the ENSEMBLE applies
// it with a negative weight (−0.15) so high similarity demotes a candidate.
//
// Per the SignalModule contract the signal itself returns POSITIVE raw
// scores in [0, 1] — the minus sign lives in the ensemble weight, never here.
//
// Score nullability: anime_list.score is nullable in Postgres (DEFAULT 0 but
// accepts NULL when the user drops without rating). In SQL, NULL < 7 evaluates
// to NULL (not TRUE), so a plain "score < 7" predicate would exclude unscored
// drops. We use "(score IS NULL OR score < 7)" so unscored drops are correctly
// treated as genuine dislike evidence.
//
// Guards (dropping is a noisy signal — demote, never bury):
//   - Drops the user scored >= 7 are excluded ("liked but life happened").
//   - Fewer than 2 eligible seeds => silent (empty map, cold-start).
type S7DroppedPenalty struct {
	db *gorm.DB
}

const (
	// s7LikedDropThreshold: dropped rows with score >= this value are NOT
	// dislike evidence and are excluded from the seed set.
	// "I dropped it but I liked it" — life interrupted viewing.
	s7LikedDropThreshold = 7

	// s7MinSeeds: below this many eligible dropped seeds the signal stays
	// silent — one drop is mood, two is a pattern.
	s7MinSeeds = 2
)

// NewS7DroppedPenalty wires S7 with a DB handle.
func NewS7DroppedPenalty(db *gorm.DB) *S7DroppedPenalty {
	return &S7DroppedPenalty{db: db}
}

// ID returns the stable signal identifier "s7".
func (s *S7DroppedPenalty) ID() recs.SignalID { return recs.SignalID("s7") }

// Precompute is a no-op — S7 is request-time only, like S2.
func (s *S7DroppedPenalty) Precompute(_ context.Context, _ recs.UserID) error { return nil }

// Score returns max-Jaccard similarity between each candidate's (genre+tag)
// attribute set and the user's eligible dropped-anime seeds. Candidates with
// no overlap are omitted (normalizer downstream treats absent entries as zero).
//
// Return values are in [0, 1]. The ensemble applies a negative weight to this
// signal; the signal itself always returns non-negative, non-NaN, non-Inf values.
func (s *S7DroppedPenalty) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	// 1. Load eligible dropped seeds: status='dropped' AND (score IS NULL OR score < 7).
	var seeds []string
	if err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("anime_id").
		Where("user_id = ? AND status = ? AND (score IS NULL OR score < ?)", userID, "dropped", s7LikedDropThreshold).
		Pluck("anime_id", &seeds).Error; err != nil {
		return nil, fmt.Errorf("s7: load dropped seeds: %w", err)
	}
	if len(seeds) < s7MinSeeds {
		// Cold-start: too few reliable dislike signals — stay silent.
		return out, nil
	}

	// 2. Load namespaced attribute sets for seeds and candidates.
	seedAttrs, err := s.loadAttrSets(ctx, seeds)
	if err != nil {
		return nil, fmt.Errorf("s7: load seed attrs: %w", err)
	}
	candAttrs, err := s.loadAttrSets(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("s7: load candidate attrs: %w", err)
	}

	// 3. For each candidate, max Jaccard across all seeds.
	for _, candidateID := range candidates {
		cset := candAttrs[candidateID]
		if len(cset) == 0 {
			continue
		}
		var best float64
		for _, sset := range seedAttrs {
			if j := jaccard(sset, cset); j > best {
				best = j
			}
		}
		if best > 0 {
			out[candidateID] = recs.RawScore(best)
		}
	}
	return out, nil
}

// loadAttrSets builds per-anime namespaced attribute sets from anime_genres
// ("genre:{id}") and anime_tags ("tag:{id}") in two batched queries.
// The namespaced key format ("genre:X" vs "tag:Y") ensures genre IDs and
// tag IDs in the same namespace never accidentally collide.
func (s *S7DroppedPenalty) loadAttrSets(ctx context.Context, animeIDs []recs.AnimeID) (map[recs.AnimeID]map[string]struct{}, error) {
	out := make(map[recs.AnimeID]map[string]struct{}, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}

	add := func(animeID recs.AnimeID, key string) {
		set, ok := out[animeID]
		if !ok {
			set = make(map[string]struct{})
			out[animeID] = set
		}
		set[key] = struct{}{}
	}

	// Genre rows — reuse the s2GenreRow type defined in s2_metadata.go.
	var genreRows []s2GenreRow
	if err := s.db.WithContext(ctx).
		Table("anime_genres").
		Select("anime_id, genre_id").
		Where("anime_id IN ?", animeIDs).
		Scan(&genreRows).Error; err != nil {
		return nil, fmt.Errorf("anime_genres query: %w", err)
	}
	for _, r := range genreRows {
		add(r.AnimeID, "genre:"+r.GenreID)
	}

	// Tag rows — inline struct (tag_id confirmed from \d anime_tags above).
	var tagRows []struct {
		AnimeID string
		TagID   string
	}
	if err := s.db.WithContext(ctx).
		Table("anime_tags").
		Select("anime_id, tag_id").
		Where("anime_id IN ?", animeIDs).
		Scan(&tagRows).Error; err != nil {
		return nil, fmt.Errorf("anime_tags query: %w", err)
	}
	for _, r := range tagRows {
		add(r.AnimeID, "tag:"+r.TagID)
	}

	return out, nil
}
