package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/eighteenanime"
)

// Get18AnimeEpisodes resolves the catalog anime's title and returns its
// 18anime.me episode list. Like the Hanime path, a no-match resolves to an
// empty list (cached briefly) rather than an error — the player then shows a
// plain "no episodes" state instead of a hard failure.
func (s *CatalogService) Get18AnimeEpisodes(ctx context.Context, animeID string) (_ []domain.Anime18Episode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("anime18", "get_episodes", start, &retErr)

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("anime18:episodes:%s", animeID)
	var cached []domain.Anime18Episode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	searchNames := []string{anime.Name}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	var eps []eighteenanime.Episode
	for _, name := range searchNames {
		if name == "" {
			continue
		}
		eps, err = s.anime18Client.ListEpisodes(ctx, name)
		if err == nil && len(eps) > 0 {
			break
		}
	}

	if len(eps) == 0 {
		_ = s.cache.Set(ctx, cacheKey, []domain.Anime18Episode{}, 30*time.Minute)
		return []domain.Anime18Episode{}, nil
	}

	out := make([]domain.Anime18Episode, len(eps))
	for i, e := range eps {
		out[i] = domain.Anime18Episode{Slug: e.Slug, URL: e.URL, Number: e.Number}
	}

	_ = s.cache.Set(ctx, cacheKey, out, time.Hour)
	return out, nil
}

// Get18AnimeStream resolves a playable source for an 18anime episode slug.
// On total mirror failure it returns a typed ServiceUnavailable error (→ 503),
// never an empty success — see spec §7. Resolved URLs are signed/short-lived,
// so successes are cached only briefly.
func (s *CatalogService) Get18AnimeStream(ctx context.Context, animeID, episodeSlug string) (_ *domain.Anime18Stream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("anime18", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("anime18").Inc()

	cacheKey := fmt.Sprintf("anime18:stream:%s:%s", animeID, episodeSlug)
	var cached domain.Anime18Stream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	src, err := s.anime18Client.GetStream(ctx, eighteenanime.EpisodeURL(episodeSlug))
	if err != nil {
		return nil, errors.ServiceUnavailable(fmt.Sprintf("18anime source unavailable: %s", err.Error()))
	}
	if src == nil || src.URL == "" {
		return nil, errors.ServiceUnavailable("18anime source unavailable: no playable mirror")
	}

	result := &domain.Anime18Stream{
		URL:     src.URL,
		Referer: src.Referer,
		IsHLS:   src.IsHLS,
		Quality: src.Quality,
	}

	_ = s.cache.Set(ctx, cacheKey, result, 5*time.Minute)
	return result, nil
}
