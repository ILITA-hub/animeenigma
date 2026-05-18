package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/allanime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// RawResolver resolves "raw JP" provider video sources via the AllAnime
// parser. Workstream raw-jp, Phase 01. Wraps the parser with:
//   - shikimori-id → anime → AllAnime show-id lookup (cached 6h, negative
//     cache 10m to dampen repeat lookups for absent titles).
//   - episode listing cache (6h) and stream URL cache (1h, matching the
//     existing per-provider TTL convention).
//   - error wrapping into libs/errors.AppError so the handler maps to 503
//     rather than 500 on upstream/transport failures.
type RawResolver struct {
	client    *allanime.Client
	animeRepo *repo.AnimeRepository
	cache     *cache.RedisCache
	log       *logger.Logger

	// Per-anime lookup deduplication for in-flight AllAnime queries.
	lookups sync.Map // map[string]*rawLookup
}

type rawLookup struct {
	done chan struct{}
	id   string
	err  error
}

// NewRawResolver constructs the resolver. Pass a configured AllAnime client
// from the main entry point.
func NewRawResolver(client *allanime.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver {
	return &RawResolver{
		client:    client,
		animeRepo: animeRepo,
		cache:     redisCache,
		log:       log,
	}
}

// RawEpisode is what the handler returns to the frontend. Mirrors the shape
// of domain.RawEpisode for client-side parity.
type RawEpisode struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// RawStream is the resolved playable stream + subtitle tracks.
type RawStream struct {
	URL       string         `json:"url"`
	Type      string         `json:"type"`
	Quality   string         `json:"quality,omitempty"`
	Subtitles []RawSubtitle  `json:"subtitles,omitempty"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// RawSubtitle is an embedded subtitle track from the AllAnime source.
type RawSubtitle struct {
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Label string `json:"label"`
}

// EpisodesResponse is the JSON envelope returned by /raw/episodes.
type EpisodesResponse struct {
	Episodes  []RawEpisode `json:"episodes"`
	Available bool         `json:"available"`
	Source    string       `json:"source"`
}

// GetEpisodes returns the raw-translation-type episode list for an anime.
// On upstream failure, returns an empty available=false envelope when the
// anime simply isn't on AllAnime, or an errors.ServiceUnavailable AppError
// when the API is unreachable.
func (r *RawResolver) GetEpisodes(ctx context.Context, animeID string) (_ *EpisodesResponse, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("allanime", "get_episodes", start, &retErr)

	cacheKey := fmt.Sprintf("raw:episodes:%s", animeID)
	var cached EpisodesResponse
	if err := r.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	anime, err := r.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}

	showID, err := r.resolveShowID(ctx, anime)
	if err != nil {
		// Anime not on AllAnime — return available=false rather than
		// surfacing this as a 503. The frontend should hide the chip.
		r.log.Infow("raw: anime not on allanime",
			"anime_id", animeID, "name", anime.Name, "error", err)
		resp := &EpisodesResponse{Episodes: []RawEpisode{}, Available: false, Source: "allanime"}
		_ = r.cache.Set(ctx, cacheKey, resp, 10*time.Minute)
		return resp, nil
	}

	eps, err := r.client.EpisodesByID(ctx, showID)
	if err != nil {
		// Distinguish "no episodes" (return empty) from "API unreachable" (503).
		if isUpstreamFailure(err) {
			return nil, errors.Wrap(err, errors.CodeUnavailable, "raw provider unavailable")
		}
		r.log.Warnw("raw: no episodes for show",
			"anime_id", animeID, "show_id", showID, "error", err)
		resp := &EpisodesResponse{Episodes: []RawEpisode{}, Available: false, Source: "allanime"}
		_ = r.cache.Set(ctx, cacheKey, resp, 10*time.Minute)
		return resp, nil
	}

	out := make([]RawEpisode, 0, len(eps))
	for _, e := range eps {
		out = append(out, RawEpisode{ID: e.ID, Number: e.Number, Title: e.Title})
	}
	resp := &EpisodesResponse{Episodes: out, Available: true, Source: "allanime"}
	_ = r.cache.Set(ctx, cacheKey, resp, 6*time.Hour)

	// Best-effort lazy backfill of has_raw column.
	if !anime.HasRaw {
		if uerr := r.animeRepo.SetHasRaw(ctx, anime.ID, true); uerr == nil {
			r.log.Infow("raw: backfilled has_raw", "anime_id", anime.ID)
		}
	}
	return resp, nil
}

// GetStream resolves a playable HLS stream for an episode number on an anime.
func (r *RawResolver) GetStream(ctx context.Context, animeID string, episodeNumber int, quality string) (_ *RawStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("allanime", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("raw").Inc()

	cacheKey := fmt.Sprintf("raw:stream:%s:%d:%s", animeID, episodeNumber, quality)
	var cached RawStream
	if err := r.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	anime, err := r.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}

	showID, err := r.resolveShowID(ctx, anime)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "raw provider unavailable")
	}

	// Episode ID is the composite "showID/episodeString" — episodes use a
	// string representation (e.g. "1", "2.5"), so format from int directly.
	episodeID := fmt.Sprintf("%s/%d", showID, episodeNumber)
	stream, err := r.client.RawStream(ctx, episodeID)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "raw provider unavailable")
	}

	subs := make([]RawSubtitle, 0, len(stream.Subtitles))
	for _, s := range stream.Subtitles {
		subs = append(subs, RawSubtitle{URL: s.URL, Lang: s.Lang, Label: s.Label})
	}

	out := &RawStream{
		URL:       stream.URL,
		Type:      stream.Type,
		Quality:   stream.Quality,
		Subtitles: subs,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// 1-hour cache — stream URLs typically expire upstream.
	_ = r.cache.Set(ctx, cacheKey, out, time.Hour)

	if !anime.HasRaw {
		_ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
	}
	return out, nil
}

// resolveShowID maps an anime to its AllAnime show ID, with a singleflight
// to dedupe concurrent lookups for the same anime.
func (r *RawResolver) resolveShowID(ctx context.Context, anime *domain.Anime) (string, error) {
	cacheKey := fmt.Sprintf("raw:mapping:%s", anime.ID)
	var cached string
	if err := r.cache.Get(ctx, cacheKey, &cached); err == nil {
		if cached == "" {
			return "", fmt.Errorf("allanime: anime not found (cached)")
		}
		return cached, nil
	}

	flight := &rawLookup{done: make(chan struct{})}
	if existing, loaded := r.lookups.LoadOrStore(anime.ID, flight); loaded {
		ex := existing.(*rawLookup)
		<-ex.done
		return ex.id, ex.err
	}
	defer func() {
		close(flight.done)
		r.lookups.Delete(anime.ID)
	}()

	id, err := r.doSearch(ctx, anime, cacheKey)
	flight.id = id
	flight.err = err
	return id, err
}

func (r *RawResolver) doSearch(ctx context.Context, anime *domain.Anime, cacheKey string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try names in order of likelihood — English/romanized first (matches
	// the catalog index AllAnime exposes), then native Japanese.
	candidates := []string{}
	for _, n := range []string{anime.Name, anime.NameEN, anime.NameJP, anime.NameRU} {
		n = strings.TrimSpace(n)
		if n != "" {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		_ = r.cache.Set(ctx, cacheKey, "", 10*time.Minute)
		return "", fmt.Errorf("allanime: no name candidates for anime %s", anime.ID)
	}

	var lastErr error
	for _, name := range candidates {
		results, err := r.client.Search(ctx, name)
		if err != nil {
			lastErr = err
			if isUpstreamFailure(err) {
				// Bubble up upstream failures so the caller wraps as 503.
				return "", err
			}
			continue
		}
		if len(results) > 0 {
			id := results[0].ID
			_ = r.cache.Set(ctx, cacheKey, id, 6*time.Hour)
			return id, nil
		}
	}

	_ = r.cache.Set(ctx, cacheKey, "", 10*time.Minute)
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("allanime: no match for %s", anime.Name)
}

// isUpstreamFailure returns true for transport-level / server-side errors
// (5xx, all-domains-unreachable) — these warrant a 503. 4xx-from-graphql
// errors (no match, stale SHA on a single query) do NOT — they're per-request
// outcomes and the caller decides how to surface them.
func isUpstreamFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "all domains unreachable") ||
		strings.Contains(msg, "upstream 5") ||
		strings.Contains(msg, "http ")
}
