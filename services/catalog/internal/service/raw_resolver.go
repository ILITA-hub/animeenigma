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
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// Cache key prefixes. Kept as exported constants so the
// invalidation endpoint (handler/internal_cache.go) can build the
// SCAN patterns from a single source of truth.
const (
	// CacheKeySourceDecision is the per-(animeID, episode) memo of
	// which backend served the last stream resolve — "library" or
	// "allanime". 1h TTL; busted by the library service's
	// post-encode webhook.
	CacheKeySourceDecision = "raw:source-decision"
	// CacheKeyStream is the per-(animeID, episode, quality) cached
	// RawStream for the AllAnime path. 1h TTL.
	CacheKeyStream = "raw:stream"
	// CacheKeyEpisodes is the per-animeID cached raw-episode list.
	CacheKeyEpisodes = "raw:episodes"
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
	library   *library.Client // optional — when nil, the library-first
	// branch is skipped entirely (defensive for environments without
	// LIBRARY_API_URL). Phase 06 (workstream raw-jp / v0.2).
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

// NewRawResolver constructs the resolver. Pass a configured AllAnime
// client from the main entry point. libraryClient is optional — when
// nil, the Phase-06 library-first branch is skipped entirely and the
// resolver behaves identically to v0.1 (AllAnime only). This is the
// defensive path for deployments without LIBRARY_API_URL set.
func NewRawResolver(client *allanime.Client, libraryClient *library.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver {
	return &RawResolver{
		client:    client,
		library:   libraryClient,
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
//
// Phase 06 (workstream raw-jp / v0.2) added the Source field — its
// value is "library" when served from the self-hosted MinIO HLS
// ladder, or "allanime" when served from the v0.1 AllAnime path.
// Existing cached entries from before the field existed deserialize
// with Source == "" — the resolver normalizes that to "allanime" on
// read for backward compatibility.
// Exp/Sig (added 2026-06-13, workstream first-party / ae provider) carry the
// HLS-proxy provenance signature for a self-hosted MinIO URL. MinIO is on the
// internal docker network and is NOT in the proxy allowlist, so a library/ae
// master-playlist request must arrive pre-signed; the proxy then mints child
// segment tokens during m3u8 rewrite. Empty for AllAnime URLs (those pass via
// the static allowlist / their own provenance seed).
type RawStream struct {
	URL       string        `json:"url"`
	Type      string        `json:"type"`
	Quality   string        `json:"quality,omitempty"`
	Subtitles []RawSubtitle `json:"subtitles,omitempty"`
	ExpiresAt time.Time     `json:"expires_at"`
	Source    string        `json:"source"`
	Exp       string        `json:"exp,omitempty"`
	Sig       string        `json:"sig,omitempty"`
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

	cacheKey := fmt.Sprintf("%s:%s", CacheKeyEpisodes, animeID)
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

// GetStream resolves a playable HLS stream for an episode number on
// an anime.
//
// Phase 06 (workstream raw-jp / v0.2) inserts a library-first branch
// in front of the existing AllAnime path:
//
//  1. Read the per-(animeID, episode) source-decision cache. If
//     "allanime", skip the library lookup entirely.
//  2. Otherwise (cache empty or "library") and when r.library is
//     non-nil and anime.ShikimoriID is set, call library.GetEpisode.
//     200 → cache "library", return MinIO URL with Source="library".
//     404 → cache "allanime" for 1h, fall through to AllAnime.
//     5xx / timeout / transport error → do NOT cache (transient),
//     fall through to AllAnime.
//  3. AllAnime path runs unchanged; the returned RawStream now sets
//     Source="allanime".
//
// Existing raw:stream:* cache entries from before the Source field
// existed deserialize with Source == "" — we normalize that to
// "allanime" on read.
func (r *RawResolver) GetStream(ctx context.Context, animeID string, episodeNumber int, quality string) (_ *RawStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("allanime", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("raw").Inc()

	cacheKey := fmt.Sprintf("%s:%s:%d:%s", CacheKeyStream, animeID, episodeNumber, quality)
	var cached RawStream
	if err := r.cache.Get(ctx, cacheKey, &cached); err == nil {
		// Backward-compat: older cached entries lack the Source
		// field — they all came from the AllAnime path.
		if cached.Source == "" {
			cached.Source = "allanime"
		}
		return &cached, nil
	}

	anime, err := r.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}

	// Library-first branch (Phase 06).
	sourceCacheKey := fmt.Sprintf("%s:%s:%d", CacheKeySourceDecision, animeID, episodeNumber)
	var sourceDecision string
	_ = r.cache.Get(ctx, sourceCacheKey, &sourceDecision)

	if r.library != nil && anime.ShikimoriID != "" && sourceDecision != "allanime" {
		resp, lerr := r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber)
		switch {
		case lerr == nil && resp != nil:
			// Library hit. Cache the decision (1h) and return the
			// MinIO URL. We do NOT write raw:stream:* on the library
			// path — MinIO URLs derive from a stable path and the
			// webhook invalidation handles structural changes.
			_ = r.cache.Set(ctx, sourceCacheKey, "library", time.Hour)
			out := newLibraryStream(resp.MinIOURL, quality)
			if !anime.HasRaw {
				_ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
			}
			return out, nil
		case lerr == nil && resp == nil:
			// Library 404. Cache "allanime" for 1h and fall through.
			_ = r.cache.Set(ctx, sourceCacheKey, "allanime", time.Hour)
		default:
			// Library 5xx / timeout / transport error. Do NOT cache
			// (transient). Fall through to AllAnime.
			if r.log != nil {
				r.log.Warnw("raw: library lookup failed; falling back to allanime",
					"anime_id", animeID,
					"shikimori_id", anime.ShikimoriID,
					"episode", episodeNumber,
					"error", lerr,
				)
			}
		}
	}

	// AllAnime path (v0.1 — unchanged behavior; Source field added).
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
		Source:    "allanime",
	}

	// 1-hour cache — stream URLs typically expire upstream.
	_ = r.cache.Set(ctx, cacheKey, out, time.Hour)

	if !anime.HasRaw {
		_ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
	}
	return out, nil
}

// newLibraryStream builds a RawStream for a self-hosted MinIO HLS URL,
// signing it with the HLS-proxy provenance HMAC so the (un-allowlisted)
// minio host is trusted on the master-playlist request. Shared by the
// auto raw path and the first-party ("ae") path.
func newLibraryStream(minioURL, quality string) *RawStream {
	exp, sig := streamsign.Sign(minioURL)
	return &RawStream{
		URL:       minioURL,
		Type:      "hls",
		Quality:   quality,
		Subtitles: nil,
		ExpiresAt: time.Now().Add(time.Hour),
		Source:    "library",
		Exp:       exp,
		Sig:       sig,
	}
}

// GetLibraryEpisodes lists the episodes for an anime that are present in
// the self-hosted library (MinIO). This is the first-party ("ae")
// provider's episode source — it never touches AllAnime, so it works for
// titles AllAnime has never heard of (e.g. Chinese donghua). Returns an
// empty list (Available=false) when the library is unconfigured, the
// anime has no shikimori_id, or nothing is encoded yet.
func (r *RawResolver) GetLibraryEpisodes(ctx context.Context, animeID string) (*EpisodesResponse, error) {
	empty := &EpisodesResponse{Episodes: []RawEpisode{}, Available: false, Source: "library"}
	if r.library == nil {
		return empty, nil
	}
	anime, err := r.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}
	if anime.ShikimoriID == "" {
		return empty, nil
	}

	items, err := r.library.ListEpisodes(ctx, anime.ShikimoriID)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "library unavailable")
	}
	if len(items) == 0 {
		return empty, nil
	}
	out := make([]RawEpisode, 0, len(items))
	for _, it := range items {
		out = append(out, RawEpisode{
			ID:     fmt.Sprintf("%d", it.EpisodeNumber),
			Number: it.EpisodeNumber,
			Title:  "",
		})
	}
	return &EpisodesResponse{Episodes: out, Available: true, Source: "library"}, nil
}

// GetLibraryStream resolves an episode's playable HLS stream STRICTLY from
// the self-hosted library — no AllAnime fallback. This backs the
// first-party ("ae") provider, which must reflect on-prem availability
// only (that's the whole point of the latency/load comparison). Returns
// errors.NotFound when the episode is not encoded locally.
func (r *RawResolver) GetLibraryStream(ctx context.Context, animeID string, episodeNumber int, quality string) (*RawStream, error) {
	if r.library == nil {
		return nil, errors.NotFound("library not configured")
	}
	anime, err := r.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, errors.NotFound("anime")
	}
	if anime.ShikimoriID == "" {
		// No MALID → no serve-signal can be fired (the library keys on
		// mal_id). Intentionally un-instrumented: this early NotFound is
		// NOT a genuine ae-pool MISS, so firing a backfill demand here
		// would record a row with no usable mal_id. (Phase 08-03.)
		return nil, errors.NotFound("episode not in library")
	}

	resp, err := r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "library unavailable")
	}
	if resp == nil {
		// MISS: ae pool does not have this episode. Fire a non-blocking
		// best-effort backfill demand so it's cached next time, then
		// return the existing NotFound UNCHANGED — the caller's
		// AllAnime-raw failover is untouched (SERVE-03 no regression).
		// context.WithoutCancel so a client disconnect can't cancel the
		// in-flight signal; the error is dropped (best-effort).
		if r.library != nil {
			go func(mal string, ep int) {
				_ = r.library.RecordDemand(context.WithoutCancel(ctx), mal, ep, "backfill")
			}(anime.ShikimoriID, episodeNumber)
		}
		return nil, errors.NotFound("episode not in library")
	}
	if !anime.HasRaw {
		_ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
	}
	// HIT: serving from the ae pool. Fire a non-blocking best-effort
	// fetch signal (bumps last_fetch_at/fetch_count + serve_total{hit}
	// on the library side) BEFORE returning the stream. drop-on-failure;
	// the resolution never fails because this side effect errored.
	if r.library != nil {
		go func(mal string, ep int) {
			_ = r.library.RecordFetch(context.WithoutCancel(ctx), mal, ep)
		}(anime.ShikimoriID, episodeNumber)
	}
	return newLibraryStream(resp.MinIOURL, quality), nil
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
