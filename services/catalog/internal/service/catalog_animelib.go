package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// ============================================================================
// AnimeLib API Methods
// ============================================================================

// GetAnimeLibEpisodes gets episodes from AnimeLib for an anime
func (s *CatalogService) GetAnimeLibEpisodes(ctx context.Context, animeID string) (_ []domain.AnimeLibEpisode, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_episodes", start, &retErr)
	// Get anime to search by name
	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	// Check cache first
	cacheKey := fmt.Sprintf("animelib:episodes:%s", animeID)
	var cached []domain.AnimeLibEpisode
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Find anime on AnimeLib
	animelibID, err := s.findAnimeLibID(ctx, anime)
	if err != nil {
		s.log.Warnw("failed to find anime on animelib",
			"anime_id", animeID,
			"name", anime.Name,
			"error", err)
		return []domain.AnimeLibEpisode{}, nil
	}

	// Get episodes
	episodes, err := s.animelibClient.GetEpisodes(ctx, animelibID)
	if err != nil {
		s.log.Warnw("failed to get animelib episodes",
			"anime_id", animeID,
			"animelib_id", animelibID,
			"error", err)
		return []domain.AnimeLibEpisode{}, nil
	}

	result := make([]domain.AnimeLibEpisode, len(episodes))
	for i, ep := range episodes {
		result[i] = domain.AnimeLibEpisode{
			ID:     ep.ID,
			Number: ep.Number,
			Name:   ep.Name,
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetAnimeLibTranslations gets available translations for an episode from AnimeLib
func (s *CatalogService) GetAnimeLibTranslations(ctx context.Context, animeID string, episodeID int) (_ []domain.AnimeLibTranslation, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_translations", start, &retErr)
	// Check cache first
	cacheKey := fmt.Sprintf("animelib:translations:%s:%d", animeID, episodeID)
	var cached []domain.AnimeLibTranslation
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	// Get episode streams (includes player/translation data)
	detail, err := s.animelibClient.GetEpisodeStreams(ctx, episodeID)
	if err != nil {
		s.log.Warnw("failed to get animelib episode streams",
			"anime_id", animeID,
			"episode_id", episodeID,
			"error", err)
		return []domain.AnimeLibTranslation{}, nil
	}

	// Collect unique translation teams from Players.
	// Prefer "Animelib" player (direct video) over "Kodik" (iframe) for same team.
	type teamEntry struct {
		translation domain.AnimeLibTranslation
		playerID    int // the actual player entry ID (for stream lookup)
	}
	teamMap := make(map[int]*teamEntry)

	for _, p := range detail.Players {
		// Skip Kodik-backed entries — AniLib must serve only its native MP4
		// players. Kodik content is reachable via the dedicated Kodik tab.
		if p.Player == "Kodik" {
			continue
		}

		// Map translation type label to internal type
		translationType := "voice"
		if p.TranslationType.ID == 1 || strings.Contains(strings.ToLower(p.TranslationType.Label), "субтитр") {
			translationType = "subtitles"
		}

		if _, exists := teamMap[p.Team.ID]; !exists {
			teamMap[p.Team.ID] = &teamEntry{
				translation: domain.AnimeLibTranslation{
					ID:           p.ID, // use the player entry ID (not team ID) for stream lookup
					TeamName:     p.Team.Name,
					Type:         translationType,
					Player:       p.Player,
					HasSubtitles: len(p.Subtitles) > 0,
				},
				playerID: p.ID,
			}
		}
	}

	var result []domain.AnimeLibTranslation
	for _, entry := range teamMap {
		result = append(result, entry.translation)
	}

	if result == nil {
		result = []domain.AnimeLibTranslation{}
	}

	// Phase 15 (UX-31): lazily backfill animes.has_animelib whenever the
	// catalog reaches at least one non-Kodik translation on the AnimeLib
	// path. The Kodik-iframe-fallback path inside AnimeLib does NOT count
	// (per feedback_animelib_no_kodik_fallback.md — AnimeLib treats
	// Kodik-only translations as empty). Best-effort; idempotent guard.
	if len(result) > 0 {
		if anime, err := s.animeRepo.GetByID(ctx, animeID); err == nil && anime != nil && !anime.HasAnimeLib {
			if updateErr := s.animeRepo.SetHasAnimeLib(ctx, animeID, true); updateErr != nil {
				s.log.Warnw("failed to persist anime.has_animelib",
					"anime_id", animeID,
					"error", updateErr)
			}
		}
	}

	// Cache for 1 hour
	_ = s.cache.Set(ctx, cacheKey, result, time.Hour)

	return result, nil
}

// GetAnimeLibStream gets the stream URL from AnimeLib for a specific episode and translation
func (s *CatalogService) GetAnimeLibStream(ctx context.Context, animeID string, episodeID int, translationID int) (_ *domain.AnimeLibStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animelib", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("animelib").Inc()
	// Check cache first
	cacheKey := fmt.Sprintf("animelib:stream:%s:%d:%d", animeID, episodeID, translationID)
	var cached domain.AnimeLibStream
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		signAnimeLibStream(&cached)
		return &cached, nil
	}

	// Get episode streams
	detail, err := s.animelibClient.GetEpisodeStreams(ctx, episodeID)
	if err != nil {
		s.log.Warnw("failed to get animelib episode streams",
			"anime_id", animeID,
			"episode_id", episodeID,
			"error", err)
		return nil, errors.NotFound("stream not available on animelib")
	}

	// Find the PlayerData matching the translationID (player entry ID)
	var matched *animelib.PlayerData
	for i := range detail.Players {
		if detail.Players[i].ID == translationID {
			matched = &detail.Players[i]
			break
		}
	}

	if matched == nil {
		return nil, errors.NotFound("translation not found on animelib")
	}

	var result *domain.AnimeLibStream

	if matched.Player == "Animelib" && matched.Video != nil && len(matched.Video.Quality) > 0 {
		// Direct video: build MP4 URLs from quality array
		sources := make([]domain.AnimeLibSource, len(matched.Video.Quality))
		for i, q := range matched.Video.Quality {
			sources[i] = domain.AnimeLibSource{
				URL:     animelib.BuildVideoURL(q.Href),
				Quality: q.Quality,
			}
		}
		result = &domain.AnimeLibStream{
			Sources: sources,
		}

		// Add external subtitle files if available
		if len(matched.Subtitles) > 0 {
			subs := make([]domain.AnimeLibSubtitle, len(matched.Subtitles))
			for i, s := range matched.Subtitles {
				subs[i] = domain.AnimeLibSubtitle{
					Format: s.Format,
					URL:    s.Src,
				}
			}
			result.Subtitles = subs
		}
	} else {
		return nil, errors.NotFound("no native AniLib video available")
	}

	// Cache for 30 minutes (stream URLs may expire). Cached BEFORE signing so
	// the Redis body never persists a signature; signatures are minted at
	// response time (here and on the cache-hit path).
	_ = s.cache.Set(ctx, cacheKey, result, 30*time.Minute)
	signAnimeLibStream(result)
	return result, nil
}

// signAnimeLibStream mints provenance signatures + Track A masked forms for
// every AniLib CDN source (progressive MP4 on cdnlibs.org/hentaicdn.org) and
// signatures for its external subtitle files, authorizing them through the
// HLS proxy without static allowlist entries. Dormant path (no FE adapter),
// signed anyway so the S3 gate flip cannot strand it. Called at RESPONSE time
// only — never persisted in the cache.
func signAnimeLibStream(st *domain.AnimeLibStream) {
	for i := range st.Sources {
		src := &st.Sources[i]
		src.Exp, src.Sig, src.MaskedURL = streamsign.Stamp(src.URL, "", "mp4")
	}
	for i := range st.Subtitles {
		sub := &st.Subtitles[i]
		sub.Exp, sub.Sig = streamsign.Sign(sub.URL)
	}
}

// SearchAnimeLib searches for anime on AnimeLib by title
func (s *CatalogService) SearchAnimeLib(ctx context.Context, title string) ([]domain.AnimeLibSearchResult, error) {
	results, err := s.animelibClient.Search(ctx, title)
	if err != nil {
		return nil, err
	}

	searchResults := make([]domain.AnimeLibSearchResult, len(results))
	for i, r := range results {
		var poster string
		if r.Cover != nil {
			poster = r.Cover.Default
		}
		searchResults[i] = domain.AnimeLibSearchResult{
			ID:      r.ID,
			Name:    r.Name,
			RusName: r.RusName,
			Poster:  poster,
			SlugURL: r.SlugURL,
		}
	}

	return searchResults, nil
}

// findAnimeLibID finds the AnimeLib integer ID for an anime by searching name variants in parallel
func (s *CatalogService) findAnimeLibID(ctx context.Context, anime *domain.Anime) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check cache for AnimeLib ID mapping
	cacheKey := fmt.Sprintf("animelib:mapping:%s", anime.ID)
	var cachedID int
	if err := s.cache.Get(ctx, cacheKey, &cachedID); err == nil && cachedID != 0 {
		return cachedID, nil
	}

	// Search by different name variants — prefer NameRU since AnimeLib is Russian
	searchNames := []string{}
	if anime.NameRU != "" {
		searchNames = append(searchNames, anime.NameRU)
	}
	if anime.Name != "" && anime.Name != anime.NameRU {
		searchNames = append(searchNames, anime.Name)
	}
	if anime.NameJP != "" {
		searchNames = append(searchNames, anime.NameJP)
	}

	if len(searchNames) == 0 {
		return 0, fmt.Errorf("anime not found on AnimeLib")
	}

	// Launch all name-variant searches in parallel. fanCtx is cancelled as soon
	// as a match is found so the losing goroutines abort their in-flight AnimeLib
	// requests (Search now threads ctx into the upstream HTTP call) instead of
	// running to the client timeout and wasting egress.
	fanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type searchResult struct{ id int }
	ch := make(chan searchResult, 1)
	var wg sync.WaitGroup

	for _, name := range searchNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			if fanCtx.Err() != nil {
				return
			}
			results, err := s.animelibClient.Search(fanCtx, name)
			if err != nil {
				s.log.Debugw("animelib search failed for variant", "name", name, "error", err)
				return
			}
			for _, r := range results {
				if matchesAnimeLibResult(r, anime) {
					select {
					case ch <- searchResult{id: r.ID}:
					default:
					}
					return
				}
			}
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	if found, ok := <-ch; ok {
		cancel() // stop the remaining in-flight variant searches
		_ = s.cache.Set(ctx, cacheKey, found.id, 24*time.Hour)
		return found.id, nil
	}

	return 0, fmt.Errorf("anime not found on AnimeLib")
}

// matchesAnimeLibResult checks if an AnimeLib search result matches an anime
func matchesAnimeLibResult(result animelib.SearchResult, anime *domain.Anime) bool {
	// Check against all the AnimeLib result names
	resultNames := []string{result.Name, result.RusName, result.EngName}

	for _, resultName := range resultNames {
		if resultName == "" {
			continue
		}
		if matchesAnime(resultName, anime) {
			return true
		}
	}
	return false
}
