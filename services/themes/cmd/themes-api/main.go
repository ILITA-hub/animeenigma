package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
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

	// Initialize database
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
		&domain.AnimeTheme{},
		&domain.ThemeRating{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

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
		Handler:      router,
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
