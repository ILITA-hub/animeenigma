package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// resolveAniListID tries to resolve the AniList ID for an anime using ARM.
// It tries ShikimoriID first (source of truth), then MALID as fallback.
// If resolved, persists the AniList ID to the database and caches it.
func (s *CatalogService) resolveAniListID(ctx context.Context, anime *domain.Anime) string {
	// Check cache first
	cacheKey := fmt.Sprintf("idmap:anilist:%s", anime.ID)
	var cachedID string
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil && cachedID != "" {
		return cachedID
	}

	var anilistID string

	// Try Shikimori ID first (source of truth)
	if anime.ShikimoriID != "" {
		armStart := time.Now()
		result, err := s.idMappingClient.ResolveByShikimoriIDContext(ctx, anime.ShikimoriID)
		metrics.ExternalAPIDuration.WithLabelValues("arm").Observe(time.Since(armStart).Seconds())
		if err != nil {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "error").Inc()
			s.log.Warnw("ARM lookup by shikimori_id failed", "shikimori_id", anime.ShikimoriID, "error", err)
		} else {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "success").Inc()
			if result != nil && result.AniList != nil {
				anilistID = strconv.Itoa(*result.AniList)
			}
		}
	}

	// Fallback to MAL ID
	if anilistID == "" && anime.MALID != "" {
		armStart := time.Now()
		result, err := s.idMappingClient.ResolveByMALIDContext(ctx, anime.MALID)
		metrics.ExternalAPIDuration.WithLabelValues("arm").Observe(time.Since(armStart).Seconds())
		if err != nil {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "error").Inc()
			s.log.Warnw("ARM lookup by mal_id failed", "mal_id", anime.MALID, "error", err)
		} else {
			metrics.ExternalAPIRequestsTotal.WithLabelValues("arm", "success").Inc()
			if result != nil && result.AniList != nil {
				anilistID = strconv.Itoa(*result.AniList)
			}
		}
	}

	if anilistID == "" {
		return ""
	}

	// Persist to database
	if err := s.animeRepo.UpdateAniListID(ctx, anime.ID, anilistID); err != nil {
		s.log.Warnw("failed to persist resolved AniList ID", "anime_id", anime.ID, "anilist_id", anilistID, "error", err)
	} else {
		s.log.Infow("resolved and saved AniList ID via ARM", "anime_id", anime.ID, "anilist_id", anilistID)
	}

	// Cache for 24 hours
	_ = s.cache.Set(ctx, cacheKey, anilistID, 24*time.Hour)

	return anilistID
}

// GetJimakuSubtitles fetches Japanese subtitles from Jimaku for an anime episode
func (s *CatalogService) GetJimakuSubtitles(ctx context.Context, animeID string, episode int) (*domain.JimakuSubtitleResponse, error) {
	if !s.jimakuClient.IsConfigured() {
		return nil, errors.NotFound("jimaku subtitles not configured")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("jimaku:subs:%s:%d", animeID, episode)
	var cached domain.JimakuSubtitleResponse
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		signJimakuSubtitles(&cached)
		return &cached, nil
	}

	// Look up the anime to get AniList ID
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, errors.NotFound("anime not found")
	}

	// Resolve AniList ID if missing, using ARM
	if anime.AniListID == "" {
		resolved := s.resolveAniListID(ctx, anime)
		if resolved == "" {
			return nil, errors.NotFound("no AniList ID available for this anime")
		}
		anime.AniListID = resolved
	}

	anilistID, err := strconv.Atoi(anime.AniListID)
	if err != nil {
		return nil, errors.NotFound("invalid AniList ID format")
	}

	// Query Jimaku API
	jimakuStart := time.Now()
	files, entryName, err := s.jimakuClient.GetSubtitlesForEpisode(anilistID, episode)
	metrics.ExternalAPIDuration.WithLabelValues("jimaku").Observe(time.Since(jimakuStart).Seconds())
	if err != nil {
		metrics.ExternalAPIRequestsTotal.WithLabelValues("jimaku", "error").Inc()
		metrics.SubtitleRequestsTotal.WithLabelValues("jimaku", "error").Inc()
		s.log.Warnw("failed to get jimaku subtitles",
			"anime_id", animeID,
			"anilist_id", anilistID,
			"episode", episode,
			"error", err)
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to fetch jimaku subtitles")
	}
	metrics.ExternalAPIRequestsTotal.WithLabelValues("jimaku", "success").Inc()
	metrics.SubtitleRequestsTotal.WithLabelValues("jimaku", "success").Inc()

	// Map to domain types
	subtitles := make([]domain.JimakuSubtitle, 0, len(files))
	for _, f := range files {
		subtitles = append(subtitles, domain.JimakuSubtitle{
			URL:      f.URL,
			FileName: f.Name,
			Lang:     "Japanese",
			Format:   jimaku.FileFormat(f.Name),
		})
	}

	result := &domain.JimakuSubtitleResponse{
		Subtitles: subtitles,
		EntryName: entryName,
	}

	// Cache for 1 hour. Cached BEFORE signing so the Redis body never persists
	// a signature; signatures are minted at response time (here and on the
	// cache-hit path).
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)
	signJimakuSubtitles(result)
	return result, nil
}

// signJimakuSubtitles mints provenance signatures for the external jimaku.cc
// download URLs, authorizing them through the HLS proxy without a static
// allowlist entry. Called at RESPONSE time only — never persisted in the cache.
func signJimakuSubtitles(resp *domain.JimakuSubtitleResponse) {
	for i := range resp.Subtitles {
		sub := &resp.Subtitles[i]
		sub.Exp, sub.Sig = streamsign.Sign(sub.URL)
	}
}
