package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animepahe"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	if cfg.MegacloudExtractor.URL == "" {
		log.Warnw("MEGACLOUD_EXTRACTOR_URL is empty; megacloud-backed extraction will fail when invoked")
	}

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("scraper")

	// Build the embed extractor registry. Order matters — registry.Find
	// returns the first matching extractor, so the cheapest Matches() check
	// goes first.
	registry := domain.NewRegistry()

	// Kwik first — Matches() is plain host equality on 2 hosts.
	kwikExtractor := embeds.NewKwikExtractor()
	registry.Register(kwikExtractor)
	log.Infow("registered embed extractor", "name", kwikExtractor.Name())

	// Megacloud — kept registered for Phase 18+ providers (9anime, AnimeKai).
	mcClient := embeds.NewMegacloudClient(cfg.MegacloudExtractor.URL, cfg.MegacloudExtractor.Timeout)
	registry.Register(mcClient)
	log.Infow("registered embed extractor", "name", mcClient.Name())

	// Redis cache — shared by malsync, episodes, stream TTLs. We fatal here
	// instead of degrading gracefully because every AnimePahe response is
	// cached on the way back; without Redis the upstream rate limits and
	// DDoS-Guard handshake would hit the wall on every request.
	redisCache, err := cache.New(cache.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer func() { _ = redisCache.Close() }()
	log.Infow("redis connected", "host", cfg.Redis.Host, "port", cfg.Redis.Port)

	// Build the shared HTTP client for AnimePahe. Per-host rate limits keep
	// us polite and let DDoS-Guard's cookie jar warm up before sustained
	// traffic. The jar is exposed via *BaseHTTPClient.Jar() so the
	// ensureDDoSCookie helper can pre-populate `__ddg2_` cookies.
	animePaheBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("animepahe.ru", 1.0, 2),
		domain.WithPerHostRPS("animepahe.com", 1.0, 2),
		domain.WithPerHostRPS("kwik.cx", 1.0, 2),
		domain.WithPerHostRPS("kwik.si", 1.0, 2),
		domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
	)

	// MalSync client (24h positive + 24h negative cache).
	malSyncClient := animepahe.NewMalSyncClient(redisCache)

	// AnimePahe provider — first live Provider in v3.0.
	// WR-11: New now validates required dependencies eagerly and returns an
	// error so a misconfigured Deps surfaces at boot, not via a runtime
	// 502 (nil pointer dereference) minutes after deploy.
	animePaheProvider, err := animepahe.New(animepahe.Deps{
		BaseURL: cfg.AnimePahe.BaseURL,
		HTTP:    animePaheBaseHTTP,
		Embeds:  registry,
		MalSync: malSyncClient,
		Cache:   redisCache,
		Log:     log,
	})
	if err != nil {
		log.Fatalw("failed to construct AnimePahe provider", "error", err)
	}

	// Build the orchestrator and register the provider before HTTP starts.
	orchestrator := service.NewOrchestrator(log, registry)
	orchestrator.Register(animePaheProvider)
	log.Infow("registered provider", "name", animePaheProvider.Name())

	// HTTP handler + router wiring.
	scraperHandler := handler.NewScraperHandler(orchestrator, log)
	router := transport.NewRouter(scraperHandler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("scraper service ready",
			"port", cfg.Server.Port,
			"address", cfg.Server.Address(),
			"providers", len(orchestrator.HealthSnapshot(context.Background())),
			"embed_extractors", len(registry.Names()),
			"megacloud_url", cfg.MegacloudExtractor.URL,
			"animepahe_base_url", cfg.AnimePahe.BaseURL,
		)
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
