package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// animeRepoForSubs is a narrow interface for the repo methods used by SubsAggregator.
// *repo.AnimeRepository satisfies it; tests can inject a handwritten fake.
type animeRepoForSubs interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
	UpdateExternalIDs(ctx context.Context, animeID string, imdb, tmdb *string) error
	UpdateAniListID(ctx context.Context, animeID string, anilistID string) error
}

// errProviderUnconfigured marks a provider that is intentionally off (no key),
// so metrics classify it as "unconfigured" rather than "down".
var errProviderUnconfigured = errors.New("subtitle provider not configured")

// SubsAggregator merges subtitle tracks from Jimaku (JP-only) and
// OpenSubtitles (everything else, keyed by IMDb/TMDB). Workstream raw-jp,
// Phase 02.
//
// The aggregator fails soft: a provider's outage or revoked key reduces
// the result set but does not abort the request. The handler surfaces
// `X-Subtitle-Providers-Down: <csv>` so the UI / monitoring can see what
// was missing.
type SubsAggregator struct {
	jimaku    *jimaku.Client
	opensubs  *opensubtitles.Client
	idmap     *idmapping.Client
	animeRepo animeRepoForSubs
	cache     *cache.RedisCache
	log       *logger.Logger
}

// NewSubsAggregator wires the dependencies.
func NewSubsAggregator(
	jimakuClient *jimaku.Client,
	openSubsClient *opensubtitles.Client,
	idMapClient *idmapping.Client,
	animeRepo *repo.AnimeRepository,
	redisCache *cache.RedisCache,
	log *logger.Logger,
) *SubsAggregator {
	return &SubsAggregator{
		jimaku:    jimakuClient,
		opensubs:  openSubsClient,
		idmap:     idMapClient,
		animeRepo: animeRepo,
		cache:     redisCache,
		log:       log,
	}
}

// SubtitleTrack is one subtitle file in the aggregated response.
type SubtitleTrack struct {
	URL      string `json:"url"`
	Lang     string `json:"lang"`
	Label    string `json:"label"`
	Format   string `json:"format,omitempty"`
	Provider string `json:"provider"` // "jimaku" or "opensubtitles"
	Release  string `json:"release,omitempty"`
}

// AggregateResponse is the handler payload.
type AggregateResponse struct {
	Languages     map[string][]SubtitleTrack `json:"languages"`
	Episode       int                        `json:"episode"`
	ProvidersDown []string                   `json:"providers_down,omitempty"`
}

// FetchAll runs both providers in parallel and returns the merged result.
// langs filters the response; pass nil/empty for "everything".
//
// Errors returned here are caller-fatal (DB lookup failed, etc). Per-provider
// failures are absorbed into ProvidersDown.
func (s *SubsAggregator) FetchAll(ctx context.Context, animeID string, episode int, langs []string) (*AggregateResponse, error) {
	cacheKey := s.cacheKey(animeID, episode, langs)
	var cached AggregateResponse
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	if anime == nil {
		return nil, fmt.Errorf("anime not found: %s", animeID)
	}

	// Lazy IMDb/TMDB backfill — required before OpenSubtitles can do
	// anything useful. Failures are non-fatal.
	if (anime.IMDbID == nil || anime.TMDBID == nil) && anime.ShikimoriID != "" {
		s.ensureExternalIDs(ctx, anime)
	}

	// Lazy AniList ID backfill — required before Jimaku can search.
	// fetchJimaku silently returns nil when AniListID == "", so without
	// this Jimaku is permanently empty for any anime where the AniList
	// ID was never resolved (Dorohedoro S2 hit this in prod 2026-05-19).
	if anime.AniListID == "" && anime.ShikimoriID != "" {
		s.ensureAniListID(ctx, anime)
	}

	type providerResult struct {
		name   string
		tracks []SubtitleTrack
		err    error
	}
	resultsCh := make(chan providerResult, 2)
	var wg sync.WaitGroup

	start := time.Now()

	// Jimaku — JP only, keyed by AniList ID.
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracks, err := s.fetchJimaku(ctx, anime, episode)
		resultsCh <- providerResult{name: "jimaku", tracks: tracks, err: err}
	}()

	// OpenSubtitles — multi-language, keyed by IMDb/TMDB.
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracks, err := s.fetchOpenSubtitles(ctx, anime, episode, langs)
		resultsCh <- providerResult{name: "opensubtitles", tracks: tracks, err: err}
	}()

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	resp := &AggregateResponse{
		Languages: map[string][]SubtitleTrack{},
		Episode:   episode,
	}

	outcomes := make([]metrics.SubtitleProviderOutcome, 0, 2)
	for r := range resultsCh {
		if r.err != nil {
			if errors.Is(r.err, errProviderUnconfigured) {
				outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: "unconfigured"})
				continue // not "down", not in ProvidersDown
			}
			s.log.Warnw("subs aggregator: provider failed",
				"provider", r.name, "anime_id", animeID, "episode", episode, "error", r.err)
			resp.ProvidersDown = append(resp.ProvidersDown, r.name)
			outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: "down"})
			continue
		}
		kept := 0
		for _, t := range r.tracks {
			if len(langs) > 0 && !containsLang(langs, t.Lang) {
				continue
			}
			resp.Languages[t.Lang] = append(resp.Languages[t.Lang], t)
			kept++
		}
		status := "ok"
		if kept == 0 {
			status = "empty"
		}
		outcomes = append(outcomes, metrics.SubtitleProviderOutcome{Provider: r.name, Status: status, Tracks: kept})
	}

	metrics.RecordSubtitleResolve(time.Since(start).Seconds(), outcomes)

	dedupe(resp.Languages)
	_ = s.cache.Set(ctx, cacheKey, resp, subsCacheTTL(resp))
	return resp, nil
}

const (
	// fullSubsCacheTTL caches a complete result (no provider failed) long enough
	// to absorb repeat opens without re-hitting upstreams.
	fullSubsCacheTTL = 6 * time.Hour
	// degradedSubsCacheTTL caches a result where a provider transiently failed
	// for only a short window, so the failed provider is retried soon instead of
	// freezing a "providers_down" panel for hours.
	degradedSubsCacheTTL = 60 * time.Second
)

// subsCacheTTL picks the cache lifetime: full results live long, degraded ones
// (a provider in ProvidersDown) live briefly so transient outages self-heal.
func subsCacheTTL(resp *AggregateResponse) time.Duration {
	if len(resp.ProvidersDown) > 0 {
		return degradedSubsCacheTTL
	}
	return fullSubsCacheTTL
}

func (s *SubsAggregator) fetchJimaku(ctx context.Context, anime *domain.Anime, episode int) ([]SubtitleTrack, error) {
	if s.jimaku == nil || !s.jimaku.IsConfigured() {
		return nil, errors.New("jimaku not configured")
	}
	if anime.AniListID == "" {
		// No AniList ID → Jimaku can't search.
		return nil, nil
	}
	anilistID, err := strconv.Atoi(anime.AniListID)
	if err != nil {
		return nil, fmt.Errorf("invalid anilist id %q: %w", anime.AniListID, err)
	}

	_ = ctx // jimaku client doesn't take ctx today
	entries, err := s.jimaku.SearchByAnilistID(anilistID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	tracks := []SubtitleTrack{}
	ep := episode
	files, err := s.jimaku.GetFiles(entries[0].ID, &ep)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		tracks = append(tracks, SubtitleTrack{
			URL:      f.URL,
			Lang:     "ja",
			Label:    f.Name,
			Format:   formatFromName(f.Name),
			Provider: "jimaku",
		})
	}
	return tracks, nil
}

func (s *SubsAggregator) fetchOpenSubtitles(ctx context.Context, anime *domain.Anime, episode int, langs []string) ([]SubtitleTrack, error) {
	if s.opensubs == nil || !s.opensubs.IsConfigured() {
		return nil, errProviderUnconfigured
	}

	params := opensubtitles.SearchParams{
		Languages: canonicalLangs(langs),
	}
	if anime.IMDbID != nil {
		params.IMDbID = *anime.IMDbID
	}
	if anime.TMDBID != nil {
		params.TMDBID = *anime.TMDBID
	}
	if params.IMDbID == "" && params.TMDBID == "" {
		// Last resort: query by name.
		name := anime.Name
		if anime.NameEN != "" {
			name = anime.NameEN
		}
		params.Query = name
	}
	if !strings.EqualFold(anime.Kind, "movie") {
		params.SeasonNumber = 1
		params.EpisodeNumber = episode
	}

	entries, err := s.opensubs.Search(ctx, params)
	if err != nil {
		return nil, err
	}

	tracks := make([]SubtitleTrack, 0, len(entries))
	for _, e := range entries {
		if e.FileID == 0 {
			continue // can't resolve without a numeric file id
		}
		tracks = append(tracks, SubtitleTrack{
			URL:      fmt.Sprintf("/api/anime/%s/subtitles/opensubtitles/file/%d", anime.ID, e.FileID),
			Lang:     e.Language,
			Label:    e.Release,
			Format:   e.Format,
			Provider: "opensubtitles",
			Release:  e.Release,
		})
	}
	return tracks, nil
}

// ensureExternalIDs runs the Kitsu mapping lookup and persists results.
// Best-effort: all failures are logged at debug level (these endpoints are
// expected to miss for newer titles).
func (s *SubsAggregator) ensureExternalIDs(ctx context.Context, anime *domain.Anime) {
	if s.idmap == nil {
		return
	}
	mapping, err := s.idmap.ResolveByShikimoriIDContext(ctx, anime.ShikimoriID)
	if err != nil || mapping == nil || mapping.Kitsu == nil {
		return
	}
	extra, err := s.idmap.KitsuMappings(ctx, *mapping.Kitsu)
	if err != nil {
		s.log.Debugw("subs aggregator: kitsu mappings lookup failed",
			"anime_id", anime.ID, "kitsu_id", *mapping.Kitsu, "error", err)
		return
	}
	if extra == nil {
		return
	}
	if updateErr := s.animeRepo.UpdateExternalIDs(ctx, anime.ID, extra.IMDbID, extra.TMDBID); updateErr != nil {
		s.log.Warnw("subs aggregator: persist external IDs failed",
			"anime_id", anime.ID, "error", updateErr)
		return
	}
	if extra.IMDbID != nil {
		anime.IMDbID = extra.IMDbID
	}
	if extra.TMDBID != nil {
		anime.TMDBID = extra.TMDBID
	}
}

// ensureAniListID resolves the anime's AniList ID via ARM and persists it
// to the database when missing. Mutates the in-memory anime so the same
// request can use the resolved ID immediately.
//
// Best-effort: failures are logged at debug level. Jimaku is a single
// provider; missing AniList just means an empty result set.
func (s *SubsAggregator) ensureAniListID(ctx context.Context, anime *domain.Anime) {
	if s.idmap == nil {
		return
	}
	mapping, err := s.idmap.ResolveByShikimoriIDContext(ctx, anime.ShikimoriID)
	if err != nil || mapping == nil || mapping.AniList == nil {
		if err != nil {
			s.log.Debugw("subs aggregator: arm anilist lookup failed",
				"anime_id", anime.ID, "shikimori_id", anime.ShikimoriID, "error", err)
		}
		return
	}
	anilistID := strconv.Itoa(*mapping.AniList)
	if err := s.animeRepo.UpdateAniListID(ctx, anime.ID, anilistID); err != nil {
		s.log.Warnw("subs aggregator: persist anilist id failed",
			"anime_id", anime.ID, "anilist_id", anilistID, "error", err)
		return
	}
	anime.AniListID = anilistID
	s.log.Infow("subs aggregator: backfilled anilist id via ARM",
		"anime_id", anime.ID, "anilist_id", anilistID)
}

func (s *SubsAggregator) cacheKey(animeID string, episode int, langs []string) string {
	c := canonicalLangs(langs)
	return fmt.Sprintf("subs:%s:%d:%s", animeID, episode, strings.Join(c, ","))
}

func canonicalLangs(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, l := range in {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func containsLang(set []string, l string) bool {
	for _, x := range set {
		if strings.EqualFold(x, l) {
			return true
		}
	}
	return false
}

func dedupe(groups map[string][]SubtitleTrack) {
	for lang, tracks := range groups {
		seen := map[string]bool{}
		out := tracks[:0]
		for _, t := range tracks {
			key := lang + "|" + t.URL
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
		groups[lang] = out
	}
}

func formatFromName(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.HasSuffix(low, ".srt"):
		return "srt"
	case strings.HasSuffix(low, ".ass"):
		return "ass"
	case strings.HasSuffix(low, ".vtt"):
		return "vtt"
	default:
		return ""
	}
}

// cachedSubFile is the Redis-stored resolved subtitle. []byte marshals to
// base64 in JSON, so non-UTF-8 subtitle bytes survive the round-trip.
type cachedSubFile struct {
	Body   []byte `json:"body"`
	Format string `json:"format"`
}

// ResolveOpenSubtitlesFile turns a numeric OpenSubtitles file_id into the
// actual subtitle bytes. It spends one download quota unit on a cache miss,
// then caches the result for 24h so re-watches cost nothing (RAW-NF-01).
func (s *SubsAggregator) ResolveOpenSubtitlesFile(ctx context.Context, fileID int) ([]byte, string, error) {
	if s.opensubs == nil || !s.opensubs.IsConfigured() {
		// Sentinel so the handler maps "no key" to a clean 503, not a 500.
		return nil, "", opensubtitles.ErrUnauthorized
	}
	cacheKey := fmt.Sprintf("subsfile:opensubtitles:%d", fileID)

	var hit cachedSubFile
	if err := s.cache.Get(ctx, cacheKey, &hit); err == nil && len(hit.Body) > 0 {
		return hit.Body, hit.Format, nil
	}

	body, filename, err := s.opensubs.Download(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	format := formatFromName(filename)
	_ = s.cache.Set(ctx, cacheKey, cachedSubFile{Body: body, Format: format}, 24*time.Hour)
	return body, format, nil
}
