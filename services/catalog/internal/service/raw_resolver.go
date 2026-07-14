package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
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

// RawResolver backs the first-party ("ae") self-hosted provider, resolving
// STRICTLY from the self-hosted library (MinIO HLS). The standalone JP-original
// "raw" provider that this resolver also fronted was removed 2026-06-30 (AllAnime
// + ok.ru cover JP-original now); the library/ae path below is unchanged.
// libraryClient is optional; when nil, it reports no episodes / NotFound
// (defensive for environments without LIBRARY_API_URL).
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
// of domain.RawEpisode for client-side parity. Track/AudioLang/Quality
// (Phase C source-panel truth) carry the self-hosted encoder's actual
// per-episode audio facts through from the library, feeding AeTitleInfo's
// per-title aggregation below.
type RawEpisode struct {
	ID        string `json:"id"`
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Track     string `json:"track,omitempty"`
	AudioLang string `json:"audio_lang,omitempty"`
	Quality   string `json:"quality,omitempty"`
}

// AeInfo aggregates the self-hosted ("ae") audio facts for a title: whether
// any episode is a localized dub (and which language) vs original audio,
// plus a representative quality. Present=false means the library has
// nothing self-hosted for this title yet (a legitimate empty state, not an
// error) — callers should treat a zero AeInfo as "no ae data".
type AeInfo struct {
	Present   bool
	AudioLang string
	Track     string
	Quality   string
	// CoversFirstEpisode is true when the self-hosted library holds episode 1.
	// ae is auto-cached and frequently PARTIAL — it often holds only a late
	// episode (e.g. Frieren ep 27 of 28). A fresh/first-time open with no
	// requested episode must land on episode 1, so the capability feed carries
	// this so the FE keeps a late-only ae library OUT of the smart default
	// (it stays manually selectable). A complete library covers ep 1 and stays
	// the preferred default.
	CoversFirstEpisode bool
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
	// Track A opaque path-token form of URL (spec 2026-07-10 §3); preferred
	// by the FE over URL+exp/sig when present.
	MaskedURL  string         `json:"masked_url,omitempty"`
	Storyboard *RawStoryboard `json:"storyboard,omitempty"`
	// Servers lists the storage backends this episode is available on, ONLY
	// when it exists on BOTH (dual-storage). Absent (nil) for the common
	// single-copy case — the FE source panel only renders a Local/Cloud
	// picker when there's an actual choice to make.
	Servers []RawServer `json:"servers,omitempty"`
}

// RawServer is one dual-storage playback option surfaced to the frontend
// (`?server=minio|s3` on the ae stream endpoint selects between them).
type RawServer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// RawStoryboard points at the episode's WebVTT thumbnail track (signed for
// the HLS proxy, same trust path as the playlist URL). Present only when the
// library's episode row has a storyboard (best-effort ffmpeg pass; absent for
// episodes encoded before the pass shipped, or when it failed).
type RawStoryboard struct {
	URL string `json:"url"`
	Exp string `json:"exp,omitempty"`
	Sig string `json:"sig,omitempty"`
	// Track A opaque path-token form of URL (spec 2026-07-10 §3); preferred
	// by the FE over URL+exp/sig when present.
	MaskedURL string `json:"masked_url,omitempty"`
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

// newLibraryStream builds a RawStream for a self-hosted MinIO HLS URL, signing
// it with the HLS-proxy provenance HMAC so the (un-allowlisted) minio host is
// trusted on the master-playlist request. Shared by raw + the first-party
// ("ae") path. storyboardURL is optional (empty when the episode has no
// storyboard yet); when present it is signed the same way and attached as
// RawStream.Storyboard.
func newLibraryStream(minioURL, quality, storyboardURL string) *RawStream {
	exp, sig, masked := streamsign.Stamp(minioURL, "", "")
	s := &RawStream{
		URL:       minioURL,
		Type:      "hls",
		Quality:   quality,
		Subtitles: nil,
		ExpiresAt: time.Now().Add(time.Hour),
		Source:    "library",
		Exp:       exp,
		Sig:       sig,
		MaskedURL: masked,
	}
	if storyboardURL != "" {
		sbExp, sbSig, sbMasked := streamsign.Stamp(storyboardURL, "", "")
		s.Storyboard = &RawStoryboard{
			URL:       storyboardURL,
			Exp:       sbExp,
			Sig:       sbSig,
			MaskedURL: sbMasked,
		}
	}
	return s
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
	for _, it := range dedupeLibraryEpisodes(items) {
		out = append(out, RawEpisode{
			ID:        fmt.Sprintf("%d", it.EpisodeNumber),
			Number:    it.EpisodeNumber,
			Title:     "",
			Track:     it.Track,
			AudioLang: it.AudioLang,
			Quality:   it.Quality,
		})
	}
	return &EpisodesResponse{Episodes: out, Available: true, Source: "library"}, nil
}

// dedupeLibraryEpisodes collapses ListEpisodes' union of minio+s3 rows down
// to one entry per episode_number (dual storage presence is a playback-time
// choice, not a second episode — every ae aggregate downstream of this
// list, capabilities/AeTitleInfo/partial_library included, must not
// double-count a dual-present episode). Preserves first-seen order; when an
// episode number appears on both storages, the minio row wins (matches
// GetLibraryStream's own minio-first default preference).
func dedupeLibraryEpisodes(items []library.EpisodeListItem) []library.EpisodeListItem {
	out := make([]library.EpisodeListItem, 0, len(items))
	index := make(map[int]int, len(items)) // episode_number -> index in out
	for _, it := range items {
		if i, ok := index[it.EpisodeNumber]; ok {
			if out[i].Storage != "minio" && it.Storage == "minio" {
				out[i] = it
			}
			continue
		}
		index[it.EpisodeNumber] = len(out)
		out = append(out, it)
	}
	return out
}

// aeDubTrack is the library's episode_track value for a localized dub
// (mirrors services/library/internal/domain.EpisodeTrackDub — that package
// is unexported outside the library service, so the literal is repeated
// here rather than imported).
const aeDubTrack = "dub"

// AeTitleInfo aggregates the self-hosted audio facts for a title: whether
// ANY episode is a localized dub (recording its audio_lang) vs original,
// plus a representative (first non-empty) quality. Best-effort — an
// empty/absent library yields Present=false, not an error.
func (r *RawResolver) AeTitleInfo(ctx context.Context, animeID string) (AeInfo, error) {
	resp, err := r.GetLibraryEpisodes(ctx, animeID)
	if err != nil || resp == nil || !resp.Available || len(resp.Episodes) == 0 {
		return AeInfo{}, err
	}
	info := AeInfo{Present: true}
	for _, ep := range resp.Episodes {
		if ep.Track == aeDubTrack && info.AudioLang == "" {
			info.AudioLang, info.Track = ep.AudioLang, ep.Track
		}
		if info.Quality == "" && ep.Quality != "" {
			info.Quality = ep.Quality
		}
		if ep.Number == 1 {
			info.CoversFirstEpisode = true
		}
	}
	if info.Track == "" {
		info.Track = "raw" // original/sub when no dub episode was found
	}
	return info, nil
}

// validLibraryServers is the set of accepted ?server= values for
// GetLibraryStream: "" (auto — prefer minio, else s3), "minio", "s3".
var validLibraryServers = map[string]bool{"": true, "minio": true, "s3": true}

// GetLibraryStream resolves an episode's playable HLS stream STRICTLY from the
// self-hosted library — no AllAnime fallback. Backs the first-party ("ae")
// provider AND the raw provider (both reflect on-prem availability only).
//
// server pins which storage copy to serve when the episode is dual-present:
// "" auto-prefers the local minio copy over s3, "minio"/"s3" force that
// specific backend. An invalid server value returns errors.InvalidInput
// (400 via httputil.Error). Returns errors.NotFound when the episode isn't
// encoded on the requested (or any, for server="") storage.
//
// Storage failures are ISOLATED: only the storage the stream is actually
// served from can hard-fail the resolution (CodeUnavailable). The other
// storage's lookup is a best-effort dual-presence probe — on error it is
// logged and the Servers list is omitted, but the stream still serves. For
// server="" the resolution hard-fails only when NO storage resolved.
func (r *RawResolver) GetLibraryStream(ctx context.Context, animeID string, episodeNumber int, quality string, server string) (*RawStream, error) {
	if !validLibraryServers[server] {
		return nil, errors.InvalidInput(`server must be one of: "", "minio", "s3"`)
	}
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

	// Union-fetch both storages concurrently: this both resolves the
	// requested copy AND tells us whether the episode is dual-present
	// (Servers list). Cheap — the library is on the same docker network with
	// a 2s-max timeout. Errors are captured PER STORAGE and isolated below:
	// a failing lookup on the storage we don't actually need to serve from
	// must never fail the resolution (it only costs the Servers list), or an
	// s3-side outage would break every minio-served ae stream and vice versa.
	var (
		minioResp, s3Resp *library.EpisodeResponse
		minioErr, s3Err   error
		wg                sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		minioResp, minioErr = r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber, "minio")
	}()
	go func() {
		defer wg.Done()
		s3Resp, s3Err = r.library.GetEpisode(ctx, anime.ShikimoriID, episodeNumber, "s3")
	}()
	wg.Wait()

	// logProbeErr surfaces a best-effort other-storage probe failure without
	// failing the resolution (the only casualty is the Servers list).
	logProbeErr := func(storage string, probeErr error) {
		if probeErr != nil && r.log != nil {
			r.log.Warnw("ae dual-storage presence probe failed; omitting servers list",
				"anime_id", animeID, "episode", episodeNumber, "storage", storage, "error", probeErr)
		}
	}

	var resp *library.EpisodeResponse
	switch server {
	case "minio":
		// Only the explicitly-requested storage's lookup may hard-fail; the
		// other side is a presence probe for the Servers list only.
		if minioErr != nil {
			return nil, errors.Wrap(minioErr, errors.CodeUnavailable, "library unavailable")
		}
		logProbeErr("s3", s3Err)
		resp = minioResp
	case "s3":
		if s3Err != nil {
			return nil, errors.Wrap(s3Err, errors.CodeUnavailable, "library unavailable")
		}
		logProbeErr("minio", minioErr)
		resp = s3Resp
	default: // "" → prefer minio, else s3; hard-fail only when NEITHER resolved
		logProbeErr("minio", minioErr)
		logProbeErr("s3", s3Err)
		switch {
		case minioResp != nil:
			resp = minioResp
		case s3Resp != nil:
			resp = s3Resp
		case minioErr != nil:
			return nil, errors.Wrap(minioErr, errors.CodeUnavailable, "library unavailable")
		case s3Err != nil:
			return nil, errors.Wrap(s3Err, errors.CodeUnavailable, "library unavailable")
		}
	}

	if resp == nil {
		// A request pinned to a specific storage that just doesn't have THIS
		// copy (while the other storage does) is NOT a pool miss — the
		// episode is encoded, only the requested copy is absent. Only fire
		// the best-effort backfill-demand signal on a genuine full miss:
		// both storages CONFIRMED absent (nil result, nil error). An errored
		// probe leaves that storage's state unknown, so no demand either.
		if minioResp == nil && s3Resp == nil && minioErr == nil && s3Err == nil {
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
		}
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

	stream := newLibraryStream(resp.MinIOURL, quality, resp.StoryboardURL)
	if minioResp != nil && s3Resp != nil {
		stream.Servers = []RawServer{{ID: "minio", Label: "Local"}, {ID: "s3", Label: "Cloud"}}
	}
	return stream, nil
}
