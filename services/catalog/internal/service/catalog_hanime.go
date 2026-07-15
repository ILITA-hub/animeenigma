package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/hanime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// GetHanimeEpisodes searches Hanime for an anime and returns its franchise episodes.
func (s *CatalogService) GetHanimeEpisodes(ctx context.Context, animeID string) (_ []domain.HanimeEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hanime", "get_episodes", start, &retErr)

	if !s.hanimeClient.IsConfigured() {
		return nil, errors.NotFound("hanime provider is not configured")
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("hanime:episodes:%s", animeID)
	var cached []domain.HanimeEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	searchNames := []string{anime.Name}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	var hits []hanime.SearchHit
	for _, name := range searchNames {
		hits, err = s.hanimeClient.Search(name)
		if err == nil && len(hits) > 0 {
			break
		}
	}

	if len(hits) == 0 {
		_ = s.cache.Set(ctx, cacheKey, []domain.HanimeEpisode{}, 30*time.Minute)
		return []domain.HanimeEpisode{}, nil
	}

	video, err := s.hanimeClient.GetVideo(hits[0].Slug)
	if err != nil {
		s.log.Warnw("failed to get hanime video", "slug", hits[0].Slug, "error", err)
		return []domain.HanimeEpisode{}, nil
	}

	episodes := make([]domain.HanimeEpisode, len(video.FranchiseVideos))
	for i, ep := range video.FranchiseVideos {
		episodes[i] = domain.HanimeEpisode{
			Name: ep.Name,
			Slug: ep.Slug,
		}
	}

	if len(episodes) == 0 {
		episodes = []domain.HanimeEpisode{{
			Name: video.Video.Name,
			Slug: video.Video.Slug,
		}}
	}

	_ = s.cache.Set(ctx, cacheKey, episodes, time.Hour)
	return episodes, nil
}

// GetHanimeStream fetches stream URLs for a specific Hanime episode slug.
func (s *CatalogService) GetHanimeStream(ctx context.Context, animeID string, slug string) (_ *domain.HanimeStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("hanime", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("hanime").Inc()

	if !s.hanimeClient.IsConfigured() {
		return nil, errors.NotFound("hanime provider is not configured")
	}

	cacheKey := fmt.Sprintf("hanime:stream:%s:%s", animeID, slug)
	var cached domain.HanimeStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		signHanimeSources(cached.Sources)
		return &cached, nil
	}

	video, err := s.hanimeClient.GetVideo(slug)
	if err != nil {
		return nil, errors.NotFound(fmt.Sprintf("Stream unavailable: %s", err.Error()))
	}

	var sources []domain.HanimeSource
	for _, srv := range video.Servers {
		for _, stream := range srv.Streams {
			if stream.URL == "" {
				continue
			}
			sources = append(sources, domain.HanimeSource{
				URL:    stream.URL,
				Height: stream.Height,
				Width:  stream.Width,
				SizeMB: stream.FilesizeMbs,
			})
		}
	}

	if len(sources) == 0 {
		return nil, errors.NotFound("no stream sources available")
	}

	result := &domain.HanimeStream{
		Sources: sources,
	}

	// Cached BEFORE signing so the Redis body never persists a signature;
	// signatures are minted at response time (here and on the cache-hit path).
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)
	signHanimeSources(result.Sources)
	return result, nil
}

// signHanimeSources mints provenance signatures + Track A masked forms for
// every Hanime CDN source URL (hanime.tv / htv-* / hydaelyn-* / zodiark-*
// families), authorizing them through the HLS proxy without static allowlist
// entries. Hanime needs no Referer (verified at smoke time). Called at
// RESPONSE time only — never persisted in the cache.
func signHanimeSources(sources []domain.HanimeSource) {
	for i := range sources {
		src := &sources[i]
		// Progressive MP4 unless the URL is an HLS manifest — mirrors the FE
		// adapter's ".m3u8 ⇒ hls" rule so the masked token selects the proxy's
		// range-passthrough path only for MP4s.
		streamType := "mp4"
		if strings.Contains(src.URL, ".m3u8") {
			streamType = ""
		}
		src.Exp, src.Sig, src.MaskedURL = streamsign.Stamp(src.URL, "", streamType)
	}
}
