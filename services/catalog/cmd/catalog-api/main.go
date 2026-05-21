package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/allanime"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Initialize database (auto-creates DB if not exists)
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	// Start DB pool metrics collector
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(
		&domain.Anime{},
		&domain.Genre{},
		&domain.Video{},
		&domain.PinnedTranslation{},
		// Phase 12 (REC-SIG-05) — S5 attribute schema additions per Decision §A1.
		// AnimeTag is the explicit join model for the anime <-> tag m2m so
		// AnimeTag.Rank is preserved (Decision §A4 — v2.1 rank-weighted TF-IDF).
		&domain.Studio{},
		&domain.Tag{},
		&domain.AnimeTag{},
		// Phase 17 (UX-33) — admin-curated editorial collections.
		&domain.Collection{},
		&domain.CollectionItem{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Phase 12 — the libs/database wrapper's AutoMigrate ADDs missing
	// columns on existing tables but never creates m2m join tables for
	// relations added to a pre-existing struct (anime_studios was missed
	// on the first Phase-12 redeploy because Anime already existed). Fall
	// through to GORM's native AutoMigrate on Anime to register the new
	// Studios m2m. Idempotent — does nothing on subsequent boots.
	if err := db.DB.AutoMigrate(&domain.Anime{}); err != nil {
		log.Fatalw("failed to migrate Anime m2m relations", "error", err)
	}

	// Phase 12 — register the explicit AnimeTag join model so GORM
	// associations preserve Rank. Required AFTER AutoMigrate so the
	// underlying table already exists.
	if err := db.SetupJoinTable(&domain.Anime{}, "Tags", &domain.AnimeTag{}); err != nil {
		log.Fatalw("failed to register AnimeTag join model", "error", err)
	}

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Initialize external parsers
	shikimoriClient := shikimori.NewClient(cfg.Shikimori, log)

	// Initialize repositories
	animeRepo := repo.NewAnimeRepository(db.DB)
	genreRepo := repo.NewGenreRepository(db.DB)
	videoRepo := repo.NewVideoRepository(db.DB)
	// Phase 17 (UX-33) — editorial collections repo.
	collectionRepo := repo.NewCollectionRepository(db.DB)

	// Initialize services
	catalogService := service.NewCatalogService(
		animeRepo,
		genreRepo,
		videoRepo,
		shikimoriClient,
		redisCache,
		log,
		service.CatalogServiceOptions{
			JimakuAPIKey:   cfg.Jimaku.APIKey,
			AnimeLibToken:  cfg.AnimeLib.Token,
			HanimeEmail:    cfg.Hanime.Email,
			HanimePassword: cfg.Hanime.Password,
			ScraperAPIURL:  cfg.Scraper.APIURL,
			ScraperTimeout: cfg.Scraper.Timeout,
		},
	)

	// Start player health checker
	healthChecker := service.NewPlayerHealthChecker(
		catalogService.KodikClient(),
		catalogService.AnimeLibClient(),
		cfg.HealthCheck.Interval,
		log,
	)
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go healthChecker.Start(healthCtx)

	// Initialize external clients
	telegramClient := telegram.NewClient(cfg.Telegram.NewsChannel)

	// Initialize handlers
	catalogHandler := handler.NewCatalogHandler(catalogService, log)
	adminHandler := handler.NewAdminHandler(catalogService, log)
	newsHandler := handler.NewNewsHandler(telegramClient, redisCache, log)
	// Phase 17 (UX-33) — editorial collections service + handler.
	collectionService := service.NewCollectionService(collectionRepo, log)
	collectionHandler := handler.NewCollectionHandler(collectionService, log)
	// Phase 18 (UX-34) — skip-intro/skip-outro proxy of aniskip.com.
	// Stateless handler with embedded http.Client + shared redis cache.
	skipTimesHandler := handler.NewSkipTimesHandler(redisCache, log)

	// Workstream raw-jp, Phase 01 — AllAnime parser exposes a raw-JP video
	// provider via a separate resolver service + handler. The handler
	// mounts /api/anime/{id}/raw/{episodes,stream}.
	allanimeClient := allanime.NewClient(allanime.Config{
		Domains:          cfg.AllAnime.Domains,
		QuerySearchSHA:   cfg.AllAnime.QuerySearchSHA,
		QueryEpisodesSHA: cfg.AllAnime.QueryEpisodesSHA,
		QuerySourcesSHA:  cfg.AllAnime.QuerySourcesSHA,
		HTTPTimeout:      cfg.AllAnime.HTTPTimeout,
		Referer:          cfg.AllAnime.Referer,
		UserAgent:        cfg.AllAnime.UserAgent,
	})
	// Workstream raw-jp, Phase 06 (v0.2) — hybrid resolver consults
	// the self-hosted library service (MinIO HLS ladder) first, then
	// falls back to AllAnime. nil-safe: an unreachable / unconfigured
	// library client makes the resolver behave identically to v0.1.
	libraryClient := library.NewClient(library.Config{
		APIURL:  cfg.Library.APIURL,
		Timeout: cfg.Library.Timeout,
	})
	rawResolver := service.NewRawResolver(allanimeClient, libraryClient, animeRepo, redisCache, log)
	rawHandler := handler.NewRawHandler(rawResolver, log)

	// Workstream raw-jp, Phase 06 — internal cache-invalidation
	// endpoint POSTed by the library encoder after every successful
	// encode. Mounted OUTSIDE /api (no AuthMiddleware) — reachable
	// only from within the docker network because nginx/gateway
	// does not proxy /internal/*.
	internalCacheHandler := handler.NewInternalCacheHandler(redisCache, animeRepo, log)

	// Workstream notifications, Phase 2 — internal latest-episode
	// lookup endpoint consumed by the notifications detector. Same
	// gateway-non-routing security model as InvalidateRaw above.
	// Wraps kodik/animelib parser adapters behind a 5-min Redis cache
	// keyed by the literal notifications:episodes:{shikimori}:{player}:
	// {translation_id}:{watch_type} pattern (NOTIF-DET-01).
	episodesLookupService := service.NewEpisodesLookupService(
		redisCache,
		catalogService.KodikClient(),
		catalogService.AnimeLibClient(),
		animeRepo,
		catalogService,
		log,
	)
	internalEpisodesHandler := handler.NewInternalEpisodesHandler(episodesLookupService, log)

	// Workstream raw-jp, Phase 02 — multi-provider subtitle aggregator.
	// Fans out to Jimaku (JP) + OpenSubtitles (everything else, keyed by
	// IMDb/TMDB) and merges results. Mounts /api/anime/{id}/subtitles[/all].
	jimakuClient := jimaku.NewClient(cfg.Jimaku.APIKey)
	openSubsClient := opensubtitles.NewClient(opensubtitles.Config{
		APIKey:    cfg.OpenSubtitles.APIKey,
		UserAgent: cfg.OpenSubtitles.UserAgent,
		Timeout:   cfg.OpenSubtitles.Timeout,
	})
	idMapClient := idmapping.NewClient()
	subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, idMapClient, animeRepo, redisCache, log)
	subtitlesHandler := handler.NewSubtitlesHandler(subsAggregator, log)

	// Workstream hero-spotlight, v1.0 Phase 1 — hero spotlight aggregator.
	// Wires 4 static-card resolvers (anime_of_day, random_tail, latest_news,
	// platform_stats) behind a concurrent fan-out with per-card 800ms and
	// overall 2s deadlines (HSB-BE-03, HSB-BE-04). Endpoint mounted at
	// /api/home/spotlight; flag-gated by cfg.SpotlightEnabled (HSB-BE-07).
	// Empty baseURL → "http://web:80" docker-network DNS; nil http.Client
	// → 500ms timeout (snug under the 800ms per-card budget).
	spotlightWebClient := client.NewWebClient("", nil)
	spotlightResolvers := []spotlight.Resolver{
		cards.NewAnimeOfDayResolver(animeRepo, redisCache, log),
		cards.NewRandomTailResolver(animeRepo, redisCache, log),
		cards.NewLatestNewsResolver(spotlightWebClient, redisCache, log),
		cards.NewPlatformStatsResolver(db.DB, redisCache, log),
	}
	spotlightAggregator := spotlight.NewAggregator(redisCache, log, spotlightResolvers)
	spotlightHandler := handler.NewSpotlightHandler(spotlightAggregator, cfg.SpotlightEnabled, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("catalog")

	// Initialize router
	router := transport.NewRouter(catalogHandler, adminHandler, newsHandler, collectionHandler, skipTimesHandler, rawHandler, subtitlesHandler, internalCacheHandler, internalEpisodesHandler, spotlightHandler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting catalog service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
