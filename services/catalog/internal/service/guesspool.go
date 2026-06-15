package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// collapseByFranchise keeps one entry per non-empty franchise (the first one
// seen — callers pass earliest-aired first), and keeps every standalone
// (empty-franchise) anime individually.
func collapseByFranchise(animes []*domain.Anime) []*domain.Anime {
	seen := make(map[string]bool)
	out := make([]*domain.Anime, 0, len(animes))
	for _, a := range animes {
		if a.Franchise == "" {
			out = append(out, a)
			continue
		}
		if seen[a.Franchise] {
			continue
		}
		seen[a.Franchise] = true
		out = append(out, a)
	}
	return out
}

// --- placeholders filled in Task 4b (DTO, service) live below ---
var _ = context.Background
var _ logger.Logger
