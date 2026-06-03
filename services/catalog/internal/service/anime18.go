package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// 18anime (18+) is served by the scraper microservice's SEPARATE adult
// orchestrator on /anime18/* (never the EN failover chain). The catalog
// resolves the anime title and forwards; these methods map the scraper's JSON
// envelopes back to the frontend's domain.Anime18Episode / Anime18Stream shapes
// so the Anime18Player contract is unchanged.

// anime18MalID returns a non-empty mal_id for the scraper handler's shape gate.
// The 18anime provider matches by title and ignores this value; we pass the
// catalog's ShikimoriID (== MAL id) when present, else a "0" sentinel.
func anime18MalID(a *domain.Anime) string {
	if a.ShikimoriID != "" {
		return a.ShikimoriID
	}
	if a.MALID != "" {
		return a.MALID
	}
	return "0"
}

// anime18Titles picks a primary title + alternate forms for the title search.
func anime18Titles(a *domain.Anime) (string, []string) {
	title := a.NameEN
	if title == "" {
		title = a.Name
	}
	var alts []string
	seen := map[string]bool{title: true, "": true}
	for _, t := range []string{a.Name, a.NameJP, a.NameEN} {
		if !seen[t] {
			seen[t] = true
			alts = append(alts, t)
		}
	}
	return title, alts
}

// Get18AnimeEpisodes forwards the title to the scraper's /anime18/episodes. A
// no-match / unavailable upstream resolves to an empty list (cached briefly) so
// the player shows a plain "no episodes" state instead of a hard error.
func (s *CatalogService) Get18AnimeEpisodes(ctx context.Context, animeID string) (_ []domain.Anime18Episode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("anime18", "get_episodes", start, &retErr)

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}

	cacheKey := fmt.Sprintf("anime18:episodes:%s", animeID)
	var cached []domain.Anime18Episode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	title, altTitles := anime18Titles(anime)
	status, body, err := s.scraperClient.GetAnime18Episodes(ctx, anime18MalID(anime), title, altTitles)
	if err != nil || status != 200 {
		_ = s.cache.Set(ctx, cacheKey, []domain.Anime18Episode{}, 30*time.Minute)
		return []domain.Anime18Episode{}, nil
	}

	var resp struct {
		Data struct {
			Episodes []struct {
				ID     string `json:"id"`
				Number int    `json:"number"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data.Episodes) == 0 {
		_ = s.cache.Set(ctx, cacheKey, []domain.Anime18Episode{}, 30*time.Minute)
		return []domain.Anime18Episode{}, nil
	}

	out := make([]domain.Anime18Episode, 0, len(resp.Data.Episodes))
	for _, e := range resp.Data.Episodes {
		out = append(out, domain.Anime18Episode{
			Slug:   e.ID,
			URL:    "https://18anime.me/hentai/" + e.ID + ".html",
			Number: e.Number,
		})
	}

	_ = s.cache.Set(ctx, cacheKey, out, time.Hour)
	return out, nil
}

// Get18AnimeStream forwards to the scraper's /anime18/stream. On total failure
// it returns a typed ServiceUnavailable (-> 503), never an empty success.
func (s *CatalogService) Get18AnimeStream(ctx context.Context, animeID, episodeSlug string) (_ *domain.Anime18Stream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("anime18", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("anime18").Inc()

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}

	cacheKey := fmt.Sprintf("anime18:stream:%s:%s", animeID, episodeSlug)
	var cached domain.Anime18Stream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	title, altTitles := anime18Titles(anime)
	status, body, err := s.scraperClient.GetAnime18Stream(ctx, anime18MalID(anime), title, altTitles, episodeSlug, "")
	if err != nil || status != 200 {
		return nil, errors.ServiceUnavailable("18anime source unavailable")
	}

	var resp struct {
		Data struct {
			Stream struct {
				Sources []struct {
					URL     string `json:"url"`
					Type    string `json:"type"`
					Quality string `json:"quality"`
				} `json:"sources"`
				Headers map[string]string `json:"headers"`
			} `json:"stream"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data.Stream.Sources) == 0 {
		return nil, errors.ServiceUnavailable("18anime source unavailable: no playable mirror")
	}

	src := resp.Data.Stream.Sources[0]
	quality := src.Quality
	if quality == "" {
		quality = "FullHD"
	}
	result := &domain.Anime18Stream{
		URL:     src.URL,
		Referer: resp.Data.Stream.Headers["Referer"],
		IsHLS:   src.Type == "hls",
		Quality: quality,
	}

	_ = s.cache.Set(ctx, cacheKey, result, 5*time.Minute)
	return result, nil
}
