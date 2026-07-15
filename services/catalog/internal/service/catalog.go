package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/aniboom"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animejoy"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/hanime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jikan"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/scraper"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type CatalogService struct {
	animeRepo       *repo.AnimeRepository
	genreRepo       *repo.GenreRepository
	videoRepo       *repo.VideoRepository
	shikimoriClient *shikimori.Client
	aniboomClient   *aniboom.Client
	kodikClient     *kodik.Client
	jikanClient     *jikan.Client
	jimakuClient    *jimaku.Client
	animelibClient  *animelib.Client
	hanimeClient    *hanime.Client
	animejoyClient  *animejoy.Client
	idMappingClient *idmapping.Client
	// aniListAiring resolves AniList's broadcaster airing schedule for the
	// calendar reconciler. Defaults to the same idmapping client; an interface
	// so tests inject a fake.
	aniListAiring AniListAiringFetcher
	// aniListReconcilePacing throttles per-anime AniList calls during calendar
	// reconciliation (~2 req/s in prod; 0 in tests to skip sleeps).
	aniListReconcilePacing time.Duration
	scraperClient          *scraper.Client
	cache                  *cache.RedisCache
	log                    *logger.Logger

	// kodikExtractWrap wraps the per-call Kodik extractor client's transport
	// for egress recording (AR-EGRESS-03). nil → default (no recording). Built
	// from CatalogServiceOptions.EgressTransportWrap so kodikextract stays a
	// dependency-free leaf module (it never imports the tracing module).
	kodikExtractWrap func(base http.RoundTripper) http.RoundTripper
}

// CatalogServiceOptions contains optional configuration for CatalogService
type CatalogServiceOptions struct {
	AniwatchAPIURL string
	JimakuAPIKey   string
	AnimeLibToken  string
	HanimeEmail    string
	HanimePassword string
	// Phase 15 (plan 04) — scraper microservice thin-client wiring.
	// ScraperAPIURL defaults to http://scraper:8088 when empty.
	// ScraperTimeout defaults to 15s when zero.
	ScraperAPIURL  string
	ScraperTimeout time.Duration

	// EgressTransportWrap, when set, wraps an http.RoundTripper to add egress
	// effect recording (AR-EGRESS-03, host-only per D-08). catalog passes
	// tracing.WrapTransport here so the internal idmapping client and the Kodik
	// extractor emit one egress effect per outbound request — WITHOUT this
	// service or the dependency-free leaf modules (idmapping/kodikextract)
	// importing the tracing module. When nil, the default transports are used.
	EgressTransportWrap func(base http.RoundTripper) http.RoundTripper
}

func NewCatalogService(
	animeRepo *repo.AnimeRepository,
	genreRepo *repo.GenreRepository,
	videoRepo *repo.VideoRepository,
	shikimoriClient *shikimori.Client,
	cache *cache.RedisCache,
	log *logger.Logger,
	opts ...CatalogServiceOptions,
) *CatalogService {
	// Initialize Kodik client (log warning if fails, don't block service startup)
	var kodikClient *kodik.Client
	var err error
	kodikClient, err = kodik.NewClient()
	if err != nil {
		log.Warnw("failed to initialize kodik client, kodik features will be unavailable", "error", err)
	}

	// Get API URLs from options
	var jimakuAPIKey, animelibToken, hanimeEmail, hanimePassword string
	var scraperAPIURL string
	var scraperTimeout time.Duration
	var egressWrap func(base http.RoundTripper) http.RoundTripper
	if len(opts) > 0 {
		jimakuAPIKey = opts[0].JimakuAPIKey
		animelibToken = opts[0].AnimeLibToken
		hanimeEmail = opts[0].HanimeEmail
		hanimePassword = opts[0].HanimePassword
		scraperAPIURL = opts[0].ScraperAPIURL
		scraperTimeout = opts[0].ScraperTimeout
		egressWrap = opts[0].EgressTransportWrap
	}
	if scraperAPIURL == "" {
		// Match the docker-compose / config.go default so unit-test
		// callers that don't populate options still get a sensible client.
		scraperAPIURL = "http://scraper:8088"
	}

	jimakuClient := jimaku.NewClient(jimakuAPIKey)
	if jimakuClient.IsConfigured() {
		log.Infow("jimaku client initialized")
	} else {
		log.Warnw("jimaku client not configured, japanese subtitle features will be unavailable")
	}

	animelibClient := animelib.NewClient(animelibToken)
	if animelibToken != "" {
		log.Infow("animelib client initialized with token")
	} else {
		log.Infow("animelib client initialized without token")
	}

	hanimeClient := hanime.NewClient(hanimeEmail, hanimePassword)
	if hanimeClient.IsConfigured() {
		log.Infow("hanime client initialized")
	} else {
		log.Infow("hanime client not configured, hanime features will be unavailable")
	}

	// AR-EGRESS-03 (D-08, host-only): wrap the internal idmapping client's
	// IPv4-forced transport for egress recording when a wrap func is provided.
	var idMapOpts []idmapping.Option
	if egressWrap != nil {
		idMapOpts = append(idMapOpts,
			idmapping.WithTransport(egressWrap(idmapping.NewIPv4Transport())))
	}

	idMapClient := idmapping.NewClient(idMapOpts...)
	return &CatalogService{
		animeRepo:              animeRepo,
		genreRepo:              genreRepo,
		videoRepo:              videoRepo,
		shikimoriClient:        shikimoriClient,
		aniboomClient:          aniboom.NewClient(),
		kodikClient:            kodikClient,
		jikanClient:            jikan.NewClient(),
		jimakuClient:           jimakuClient,
		animelibClient:         animelibClient,
		hanimeClient:           hanimeClient,
		animejoyClient:         animejoy.NewClient(),
		idMappingClient:        idMapClient,
		aniListAiring:          idMapClient,
		aniListReconcilePacing: 500 * time.Millisecond,
		scraperClient:          scraper.NewClient(scraperAPIURL, scraperTimeout),
		cache:                  cache,
		log:                    log,
		kodikExtractWrap:       egressWrap,
	}
}

// KodikClient returns the Kodik parser client (may be nil if init failed).
func (s *CatalogService) KodikClient() *kodik.Client {
	return s.kodikClient
}

// AnimeLibClient returns the AnimeLib parser client.
func (s *CatalogService) AnimeLibClient() *animelib.Client {
	return s.animelibClient
}

// ResolveAnimeLibID exposes the otherwise-unexported findAnimeLibID helper so
// the Phase 2 notifications detector (via services/catalog/internal/service/
// episodes_lookup.go) can map a catalog *domain.Anime → AnimeLib's internal
// numeric ID without duplicating the search-then-pick logic. The 24h
// `animelib:mapping:{anime.ID}` Redis cache inside findAnimeLibID already
// absorbs the cost of repeated lookups across detector runs.
func (s *CatalogService) ResolveAnimeLibID(ctx context.Context, anime *domain.Anime) (int, error) {
	return s.findAnimeLibID(ctx, anime)
}

// HanimeClient returns the Hanime parser client.
func (s *CatalogService) HanimeClient() *hanime.Client {
	return s.hanimeClient
}

// upsertAnimeFromExternal stores or updates anime from external source
func (s *CatalogService) upsertAnimeFromExternal(ctx context.Context, anime *domain.Anime) error {
	// Fall back to MAL poster if Shikimori has none
	s.fetchMALPosterIfMissing(ctx, anime)

	// Check if exists
	existing, err := s.animeRepo.GetByShikimoriID(ctx, anime.ShikimoriID)
	if err != nil {
		return err
	}

	if existing != nil {
		// Update existing — preserve MAL poster if Shikimori still has none
		if anime.PosterURL == "" && existing.PosterURL != "" {
			anime.PosterURL = existing.PosterURL
		}
		anime.ID = existing.ID
		anime.HasVideo = existing.HasVideo
		anime.CreatedAt = existing.CreatedAt
		if err := s.animeRepo.Update(ctx, anime); err != nil {
			return err
		}
		// Invalidate the per-anime cache so fresh Shikimori data (episodes_aired,
		// next_episode_at, …) is visible immediately. This path is reached from
		// search / trending / seasonal / ResolveMALAnime; without this the 6h
		// anime:{id} cache served stale airing data to the watch page.
		_ = s.cache.Delete(ctx, cache.KeyAnime(existing.ID))
		return nil
	}

	// Create new
	if err := s.animeRepo.Create(ctx, anime); err != nil {
		return err
	}

	// Upsert + link genres
	s.persistAnimeGenres(ctx, anime)

	return nil
}

// fetchMALPosterIfMissing tries to get a poster from MAL/Jikan when Shikimori has none.
// Shikimori IDs equal MAL IDs, so we use ShikimoriID directly.
func (s *CatalogService) fetchMALPosterIfMissing(ctx context.Context, anime *domain.Anime) {
	if anime.PosterURL != "" || anime.ShikimoriID == "" {
		return
	}

	malInfo, err := s.jikanClient.GetAnimeByID(ctx, anime.ShikimoriID)
	if err != nil {
		s.log.Debugw("MAL poster fallback failed", "shikimori_id", anime.ShikimoriID, "error", err)
		return
	}

	if poster := malInfo.PosterURL(); poster != "" {
		anime.PosterURL = poster
		s.log.Infow("resolved poster from MAL", "shikimori_id", anime.ShikimoriID, "poster_url", poster)
	}
}

// videoSourcesFromVideos collapses videos into a deduplicated (type, quality,
// language) source summary, preserving first-seen order. Shared by enrichAnime
// (single) and enrichAll (batch) so the dedup logic lives in one place.
func videoSourcesFromVideos(videos []*domain.Video) []domain.VideoSource {
	// domain.VideoSource carries a []string Subtitles field (not comparable), so
	// dedup on a small comparable key rather than the value itself.
	type key struct {
		typ      domain.SourceType
		quality  string
		language string
	}
	seen := make(map[key]struct{}, len(videos))
	sources := make([]domain.VideoSource, 0, len(videos))
	for _, v := range videos {
		k := key{v.SourceType, v.Quality, v.Language}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		sources = append(sources, domain.VideoSource{
			Type:     v.SourceType,
			Quality:  v.Quality,
			Language: v.Language,
		})
	}
	return sources
}

// enrichAnime adds genres and video sources to anime
func (s *CatalogService) enrichAnime(ctx context.Context, anime *domain.Anime) {
	if anime == nil {
		return
	}

	// Load genres if not already loaded
	if len(anime.Genres) == 0 {
		genres, err := s.genreRepo.GetForAnime(ctx, anime.ID)
		if err == nil {
			anime.Genres = genres
		}
	}

	// Load video sources summary
	videos, err := s.videoRepo.GetForAnime(ctx, anime.ID, "")
	if err == nil && len(videos) > 0 {
		anime.VideoSources = append(anime.VideoSources, videoSourcesFromVideos(videos)...)
	}
}

// enrichAll loads genres and video sources for a whole slice of anime in bulk:
// two queries (one genre join, one video IN-list) regardless of slice size,
// instead of the per-anime 4-query path (which N+1'd list endpoints — a popular
// page once issued ~400 queries). Falls back to per-anime enrichAnime only if a
// batch query errors.
func (s *CatalogService) enrichAll(ctx context.Context, animes []*domain.Anime) {
	if len(animes) == 0 {
		return
	}

	ids := make([]string, len(animes))
	for i, a := range animes {
		ids[i] = a.ID
	}

	genresMap, err := s.genreRepo.GetForAnimes(ctx, ids)
	if err != nil {
		s.log.Warnw("batch genre load failed, falling back to individual", "error", err)
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return
	}

	videosMap, err := s.videoRepo.GetForAnimes(ctx, ids)
	if err != nil {
		s.log.Warnw("batch video load failed, falling back to individual", "error", err)
		for _, anime := range animes {
			s.enrichAnime(ctx, anime)
		}
		return
	}

	for _, anime := range animes {
		if len(anime.Genres) == 0 {
			anime.Genres = genresMap[anime.ID]
		}
		videos := videosMap[anime.ID]
		if len(videos) > 0 {
			anime.VideoSources = append(anime.VideoSources, videoSourcesFromVideos(videos)...)
		}
	}
}

// persistAnimeGenres upserts an anime's genres and links them to the anime,
// logging (but not failing on) errors. Mirrors the inline blocks it replaces.
func (s *CatalogService) persistAnimeGenres(ctx context.Context, anime *domain.Anime) {
	for _, genre := range anime.Genres {
		if err := s.genreRepo.Upsert(ctx, &genre); err != nil {
			s.log.Warnw("failed to upsert genre", "error", err)
		}
	}
	genreIDs := make([]string, len(anime.Genres))
	for i, g := range anime.Genres {
		genreIDs[i] = g.ID
	}
	if len(genreIDs) > 0 {
		if err := s.genreRepo.SetAnimeGenres(ctx, anime.ID, genreIDs); err != nil {
			s.log.Warnw("failed to set anime genres", "error", err)
		}
	}
}

// matchesAnime checks if a search result matches an anime using exact and containment matching.
// Extra names (e.g. English title from Jikan) can be passed for additional matching.
func matchesAnime(resultName string, anime *domain.Anime, extraNames ...string) bool {
	resultNorm := normalizeTitle(resultName)

	names := []string{anime.Name, anime.NameRU, anime.NameJP}
	names = append(names, extraNames...)
	for _, name := range names {
		if name == "" {
			continue
		}
		norm := normalizeTitle(name)
		// Exact match
		if norm == resultNorm {
			return true
		}
		// Containment match (bidirectional) — only for names longer than 3 chars to avoid false positives
		if len(norm) > 3 && len(resultNorm) > 3 {
			if strings.Contains(resultNorm, norm) || strings.Contains(norm, resultNorm) {
				return true
			}
		}
	}

	return false
}

// normalizeTitle normalizes a title for comparison
func normalizeTitle(title string) string {
	// Convert to lowercase and remove common variations
	title = strings.ToLower(title)
	title = strings.TrimSpace(title)
	// Remove special characters
	title = strings.ReplaceAll(title, ":", "")
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "  ", " ")
	return title
}
