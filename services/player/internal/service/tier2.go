package service

import (
	"math"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// AggregateTier2 walks the user's watch_history rows and produces two weighted
// signals used by the Phase 6 Tier 2 resolver rewrite:
//
//   - coarse: per (language, watch_type) — drives the lock decision
//   - fine:   per (language, watch_type, player, translation_id, translation_title)
//     — picks the team within the lock
//
// Each row's contribution is `max(duration_watched, durationFloor) * exp(-ln(2) * age_seconds / (halfLifeDays * 86400))`.
// The duration floor exists so legacy rows with `duration_watched=0` (pre-Phase 5)
// still vote — without a floor they would silently disappear after the Phase 5
// migration, biasing the signal toward only post-2026-05-03 history.
//
// Returns the two signals plus the total weight (sum of all coarse weights),
// used by ChooseTier2Lock to apply the min-confidence floor. Pure function:
// deterministic given identical (rows, halfLifeDays, now, durationFloor).
func AggregateTier2(rows []domain.WatchHistory, halfLifeDays float64, now time.Time, durationFloor int) (coarse []domain.WeightedCoarse, fine []domain.WeightedFine, total float64) {
	if len(rows) == 0 || halfLifeDays <= 0 {
		return nil, nil, 0
	}

	type coarseKey struct {
		Language, WatchType string
	}
	type fineKey struct {
		Language, WatchType, Player, TranslationID, TranslationTitle string
	}

	coarseMap := map[coarseKey]float64{}
	fineMap := map[fineKey]float64{}

	decayRate := math.Ln2 / (halfLifeDays * 86400.0)

	for _, h := range rows {
		// Skip rows with undefined language/watch_type — they would pollute
		// the coarse signal with empty-string buckets that the resolver can't
		// match against any available combo anyway.
		if h.Language == "" || h.WatchType == "" {
			continue
		}

		ageSec := now.Sub(h.WatchedAt).Seconds()
		if ageSec < 0 {
			ageSec = 0 // clock-skew safety: a future-dated row gets full weight
		}

		dur := h.DurationWatched
		if dur < durationFloor {
			dur = durationFloor
		}

		weight := float64(dur) * math.Exp(-decayRate*ageSec)

		ck := coarseKey{Language: h.Language, WatchType: h.WatchType}
		coarseMap[ck] += weight
		total += weight

		fk := fineKey{
			Language:         h.Language,
			WatchType:        h.WatchType,
			Player:           h.Player,
			TranslationID:    h.TranslationID,
			TranslationTitle: h.TranslationTitle,
		}
		fineMap[fk] += weight
	}

	for k, w := range coarseMap {
		coarse = append(coarse, domain.WeightedCoarse{
			Language:  k.Language,
			WatchType: k.WatchType,
			Weight:    w,
		})
	}
	sort.Slice(coarse, func(i, j int) bool {
		return coarse[i].Weight > coarse[j].Weight
	})

	for k, w := range fineMap {
		fine = append(fine, domain.WeightedFine{
			Language:         k.Language,
			WatchType:        k.WatchType,
			Player:           k.Player,
			TranslationID:    k.TranslationID,
			TranslationTitle: k.TranslationTitle,
			Weight:           w,
		})
	}
	sort.Slice(fine, func(i, j int) bool {
		return fine[i].Weight > fine[j].Weight
	})

	return coarse, fine, total
}

// ChooseTier2Lock applies the min-confidence floor and picks the lock from
// the top coarse + top-within-lock fine signal. Returns nil when:
//
//   - total weighted history is below minConfidence (thin-signal skip)
//   - coarse is empty (no usable history)
//
// nil is the signal to the resolver to fall through to Tier 3. Phase 6.
func ChooseTier2Lock(coarse []domain.WeightedCoarse, fine []domain.WeightedFine, total float64, minConfidence float64) *domain.Tier2Lock {
	if len(coarse) == 0 || total < minConfidence {
		return nil
	}

	// coarse is sorted descending by weight (AggregateTier2 invariant).
	top := coarse[0]

	// Find the heaviest fine entry within the locked (language, watch_type).
	var topTitle string
	var bestFineWeight float64
	for _, f := range fine {
		if f.Language != top.Language || f.WatchType != top.WatchType {
			continue
		}
		if f.Weight > bestFineWeight {
			bestFineWeight = f.Weight
			topTitle = f.TranslationTitle
		}
	}

	return &domain.Tier2Lock{
		Language:            top.Language,
		WatchType:           top.WatchType,
		TopTranslationTitle: topTitle,
		Confidence:          total,
	}
}
