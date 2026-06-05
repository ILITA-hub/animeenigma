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
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/config"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/parser/animethemes"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/service"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/transport"
)

func main() {
	// Initialize logger
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "themes")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize database
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}

	// Start DB pool metrics collector
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(
		&domain.AnimeTheme{},
		&domain.ThemeRating{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Redis client (added v4.0 Phase 3 — themes had none before). The db_read
	// P95 ReadGate refresher needs a Redis handle to snapshot read_thresholds.
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects themes requests.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EFFECT-01 (D-15): GORM db_write/db_read effect callbacks + the daily-P95
	// ReadGate refresher (D-03), using the Redis client just constructed.
	dbEffectsCtx, dbEffectsCancel := context.WithCancel(context.Background())
	defer dbEffectsCancel()
	dbEffectsStop, err := gormtrace.WireDBEffects(dbEffectsCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

	// Initialize AnimeThemes API client
	atClient := animethemes.NewClient(log)

	// Initialize repositories
	themeRepo := repo.NewThemeRepository(db.DB)
	ratingRepo := repo.NewRatingRepository(db.DB)

	// Initialize services
	syncService := service.NewSyncService(themeRepo, atClient, log)
	themeService := service.NewThemeService(themeRepo, ratingRepo, log)
	ratingService := service.NewRatingService(ratingRepo, themeRepo, log)

	// Initialize handlers
	themeHandler := handler.NewThemeHandler(themeService, log)
	ratingHandler := handler.NewRatingHandler(ratingService, log)
	adminHandler := handler.NewAdminHandler(syncService, log)
	videoProxyHandler := handler.NewVideoProxyHandler(log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("themes")

	// Initialize router
	router := transport.NewRouter(
		themeHandler,
		ratingHandler,
		adminHandler,
		videoProxyHandler,
		cfg.JWT,
		log,
		metricsCollector,
	)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("themes")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // Large for video proxy
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		log.Infow("starting themes service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	// Graceful shutdown
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
