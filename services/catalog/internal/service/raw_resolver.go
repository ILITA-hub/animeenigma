package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// Cache key prefixes. Kept as exported constants so the invalidation endpoint
// (handler/internal_cache.go) can build the SCAN patterns from a single source
// of truth. The raw provider is now LIBRARY-ONLY (AllAnime backend dropped
// 2026-06-22), so the source-decision / per-stream caches are no longer
// written; the constants remain so the invalidation endpoint keeps compiling
// and any legacy keys are still swept on the next encode webhook.
const (
	// CacheKeySourceDecision is a legacy per-(animeID, episode) memo of which
	// backend served the last stream resolve. No longer written (library-only).
	CacheKeySourceDecision = "raw:source-decision"
	// CacheKeyStream is a legacy per-(animeID, episode, quality) cached
	// RawStream from the old AllAnime path. No longer written (library URLs
	// are stable + signed per-request).
	CacheKeyStream = "raw:stream"
	// CacheKeyEpisodes is the per-animeID cached raw-episode list (library).
	CacheKeyEpisodes = "raw:episodes"
)

// RawResolver resolves the JP-original-audio "raw" provider STRICTLY from the
// self-hosted library (MinIO HLS). RAW = Japanese audio with NO burned-in subs;
// subtitles overlay softly at playback (Jimaku). The legacy AllAnime backend was
// dropped 2026-06-22 — its sources were behind a Cloudflare-Turnstile clock and
// duplicated the scraper's allanime provider — so raw now serves only titles
// present in the library. libraryClient is optional; when nil, raw reports no
// episodes / NotFound (defensive for environments without LIBRARY_API_URL).
type RawResolver struct {
	library   *library.Client
	animeRepo *repo.AnimeRepository
	cache     *cache.RedisCache
	log       *logger.Logger

	// serveSignalSem bounds the in-flight fire-and-forget library serve-signal
	// goroutines (WR-01). Each HIT/MISS resolution spawns a best-effort signal
	// to the library /internal endpoint; without a cap, a request burst against
	// a slow library would accumulate goroutines (each holding an idle TCP
	// conn). The buffered channel is a counting semaphore with DROP-ON-FULL:
	// when saturated, fireSignal skips the signal rather than blocking — the
	// signal must never block or fail the resolution.
	serveSignalSem chan struct{}
}

// serveSignalConcurrency caps concurrent in-flight library serve-signal
// goroutines. Signals are cheap and dropping one is acceptable (popularity /
// observability, not correctness), so a small bound is fine.
const serveSignalConcurrency = 64

// NewRawResolver constructs the library-only raw resolver. libraryClient is
// optional — when nil, raw returns empty episode lists / NotFound streams (the
// defensive path for deployments without LIBRARY_API_URL set).
func NewRawResolver(libraryClient *library.Client, animeRepo *repo.AnimeRepository, redisCache *cache.RedisCache, log *logger.Logger) *RawResolver {
	return &RawResolver{
		library:        libraryClient,
		animeRepo:      animeRepo,
		cache:          redisCache,
		log:            log,
		serveSignalSem: make(chan struct{}, serveSignalConcurrency),
	}
}

// fireSignal runs fn in a bounded best-effort goroutine for the library
// serve-signal path (WR-01). It acquires a slot from serveSignalSem; if the
// semaphore is saturated it DROPS the signal (returns false) rather than
// blocking or spawning an unbounded goroutine. The signal must NEVER block or
// fail the playback resolution, so a drop is the correct degradation. Returns
// true when the goroutine was spawned. A nil semaphore (e.g. a zero-value
// resolver in a test) also drops. The caller is responsible for using
// context.WithoutCancel inside fn so a client disconnect can't cancel the
// in-flight signal.
func (r *RawResolver) fireSignal(fn func()) bool {
	if r.serveSignalSem == nil {
		return false
	}
	select {
	case r.serveSignalSem <- struct{}{}:
		go func() {
			defer func() { <-r.serveSignalSem }()
			fn()
		}()
		return true
	default:
		// Saturated → drop (best-effort, matches the SERVE-03 contract).
		return false
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
// Source is "library" — raw is served exclusively from the self-hosted MinIO
// HLS ladder. Exp/Sig carry the HLS-proxy provenance signature for the
// self-hosted MinIO URL: MinIO is on the internal docker network and is NOT in
// the proxy allowlist, so a library master-playlist request must arrive
// pre-signed; the proxy then mints child segment tokens during m3u8 rewrite.
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

// RawSubtitle is an embedded subtitle track.
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

// GetEpisodes returns the raw episode list, served from the library, with a
// cache (6h on a hit, 10m on an empty result). Returns an empty
// available=false envelope when the library is unconfigured, the anime has no
// shikimori_id, or nothing is encoded yet. An errors.ServiceUnavailable
// AppError surfaces when the library API is unreachable.
func (r *RawResolver) GetEpisodes(ctx context.Context, animeID string) (_ *EpisodesResponse, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("library", "get_episodes", start, &retErr)

	cacheKey := fmt.Sprintf("%s:%s", CacheKeyEpisodes, animeID)
	var cached EpisodesResponse
	if err := r.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	resp, err := r.GetLibraryEpisodes(ctx, animeID)
	if err != nil {
		return nil, err
	}
	ttl := 10 * time.Minute
	if resp.Available {
		ttl = 6 * time.Hour
	}
	_ = r.cache.Set(ctx, cacheKey, resp, ttl)
	return resp, nil
}

// GetStream resolves a playable raw HLS stream from the library only. A miss
// returns errors.NotFound (after firing a best-effort autocache backfill
// demand). Delegates to the shared library-stream path — raw and the
// first-party "ae" provider both serve the self-hosted JP pool.
func (r *RawResolver) GetStream(ctx context.Context, animeID string, episodeNumber int, quality string) (_ *RawStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("library", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("raw").Inc()
	return r.GetLibraryStream(ctx, animeID, episodeNumber, quality)
}

// newLibraryStream builds a RawStream for a self-hosted MinIO HLS URL, signing
// it with the HLS-proxy provenance HMAC so the (un-allowlisted) minio host is
// trusted on the master-playlist request. Shared by raw + the first-party
// ("ae") path.
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

// GetLibraryEpisodes lists the episodes for an anime that are present in the
// self-hosted library (MinIO). This is the first-party ("ae") provider's
// episode source AND (since 2026-06-22) the raw provider's source — it never
// touches AllAnime, so it works for titles AllAnime has never heard of (e.g.
// Chinese donghua). Returns an empty list (Available=false) when the library is
// unconfigured, the anime has no shikimori_id, or nothing is encoded yet.
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

// GetLibraryStream resolves an episode's playable HLS stream STRICTLY from the
// self-hosted library — no AllAnime fallback. Backs the first-party ("ae")
// provider AND the raw provider (both reflect on-prem availability only).
// Returns errors.NotFound when the episode is not encoded locally.
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
		// MISS: pool does not have this episode. Fire a non-blocking
		// best-effort backfill demand so it's cached next time, then
		// return NotFound. context.WithoutCancel so a client disconnect
		// can't cancel the in-flight signal; the error is dropped
		// (best-effort). Bounded + drop-on-full via fireSignal (WR-01).
		mal, ep := anime.ShikimoriID, episodeNumber
		// Ordered fallback titles (name_jp → romaji → name_en) so the library
		// Planner can search trackers by title; empties are dropped server-side.
		titles := []string{anime.NameJP, anime.Name, anime.NameEN}
		trigger := &library.DemandTrigger{Player: "ae", WatchedEpisode: episodeNumber}
		if claims, ok := authz.ClaimsFromContext(ctx); ok && claims != nil {
			trigger.UserID = claims.UserID
			trigger.Username = claims.Username
		}
		sigCtx := context.WithoutCancel(ctx)
		r.fireSignal(func() {
			_ = r.library.RecordDemand(sigCtx, mal, ep, "backfill", titles, trigger)
		})
		return nil, errors.NotFound("episode not in library")
	}
	if !anime.HasRaw {
		_ = r.animeRepo.SetHasRaw(ctx, anime.ID, true)
	}
	// HIT: serving from the pool. Fire a non-blocking best-effort fetch signal
	// (bumps last_fetch_at/fetch_count + serve_total{hit} on the library side)
	// BEFORE returning the stream. drop-on-failure; the resolution never fails
	// because this side effect errored. Bounded + drop-on-full via fireSignal.
	mal, ep := anime.ShikimoriID, episodeNumber
	sigCtx := context.WithoutCancel(ctx)
	r.fireSignal(func() {
		_ = r.library.RecordFetch(sigCtx, mal, ep)
	})
	return newLibraryStream(resp.MinIOURL, quality), nil
}
