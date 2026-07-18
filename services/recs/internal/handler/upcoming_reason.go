// Package handler — upcoming_reason.go: honest reason lines for the "Upcoming
// for you" card (spec 2026-07-18 §5). Resolution order per matched title:
//
//  1. franchise   — the user scored a prior entry in this franchise (seed).
//  2. attribute   — the strongest shared S5 attribute (studio, then source).
//  3. anticipated — pool-relative MAL popularity is high (S9 near the pool max).
//  4. taste       — generic attribute-affinity fallback.
//
// The attribute resolver is deliberately independent of S5's TF-IDF JSONB: it
// asks the direct, explainable question "which studio/source does the user
// actually watch that this candidate also has", so the surfaced name is exactly
// what the copy claims.
package handler

import (
	"context"
)

const (
	// upcomingAnticipatedFraction is how close to the pool's top MAL-popularity
	// a title must sit to earn the "highly anticipated" reason.
	upcomingAnticipatedFraction = 0.85
	// upcomingSourceMinWatched is how many of the user's watched titles must
	// share a candidate's source material before it's a citeable reason.
	upcomingSourceMinWatched = 3
)

// resolveReason picks the most honest reason for one matched title.
func (h *UpcomingHandler) resolveReason(ctx context.Context, userID, animeID string, anime UpcomingAnimePayload, franchise bool, rawS9, maxS9 float64) UpcomingReason {
	// 1. Franchise seed ("you rated X 9/10").
	if franchise && anime.Franchise != "" {
		if seed, err := h.franchiseSeed(ctx, userID, anime.Franchise); err != nil {
			h.log.Warnw("upcoming franchise seed lookup failed; trying attribute reason",
				"user_id", userID, "franchise", anime.Franchise, "error", err)
		} else if seed != nil {
			return *seed
		}
	}

	// 2. Strongest shared attribute (studio, then source).
	if attr, err := h.attributeReason(ctx, userID, animeID); err != nil {
		h.log.Warnw("upcoming attribute reason lookup failed; falling back",
			"user_id", userID, "anime_id", animeID, "error", err)
	} else if attr != nil {
		return *attr
	}

	// 3. Pool-relative popularity.
	if maxS9 > 0 && rawS9 >= upcomingAnticipatedFraction*maxS9 {
		return UpcomingReason{Kind: "anticipated"}
	}

	// 4. Generic taste.
	return UpcomingReason{Kind: "taste"}
}

// attributeReason returns the single strongest shared attribute driving a
// taste match, or nil when none is citeable. Studio first (S5's heaviest
// non-tag dimension and the one with clean display names), then source.
func (h *UpcomingHandler) attributeReason(ctx context.Context, userID, animeID string) (*UpcomingReason, error) {
	// 1. Most-watched studio the candidate also has.
	type studioRow struct {
		Name string
		Cnt  int
	}
	var studios []studioRow
	if err := h.db.WithContext(ctx).
		Table("watch_history AS wh").
		Select("s.name AS name, COUNT(*) AS cnt").
		Joins("JOIN anime_studios uas ON uas.anime_id = wh.anime_id").
		Joins("JOIN anime_studios cas ON cas.studio_id = uas.studio_id AND cas.anime_id = ?", animeID).
		Joins("JOIN studios s ON s.id = uas.studio_id").
		Where("wh.user_id = ?", userID).
		Group("s.id, s.name").
		Order("cnt DESC").
		Limit(1).
		Scan(&studios).Error; err != nil {
		return nil, err
	}
	if len(studios) > 0 && studios[0].Name != "" {
		return &UpcomingReason{Kind: "attribute", Attribute: "studio", AttributeName: studios[0].Name}, nil
	}

	// 2. Shared source material, when the user's history leans that source.
	var sources []string
	if err := h.db.WithContext(ctx).
		Table("animes").
		Where("id = ?", animeID).
		Pluck("material_source", &sources).Error; err != nil {
		return nil, err
	}
	candSource := ""
	if len(sources) > 0 {
		candSource = sources[0]
	}
	// "original"/"other" are not shareable affinities — skip.
	if candSource != "" && candSource != "original" && candSource != "other" {
		var cnt int64
		if err := h.db.WithContext(ctx).
			Table("watch_history AS wh").
			Joins("JOIN animes a ON a.id = wh.anime_id").
			Where("wh.user_id = ? AND a.material_source = ?", userID, candSource).
			Count(&cnt).Error; err != nil {
			return nil, err
		}
		if cnt >= upcomingSourceMinWatched {
			return &UpcomingReason{Kind: "attribute", Attribute: "source", AttributeName: candSource}, nil
		}
	}

	return nil, nil
}
