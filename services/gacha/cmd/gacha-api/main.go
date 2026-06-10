// Package main is the gacha service entrypoint (port 8093) — owns the
// «Энигмы» economy: wallets + an idempotent ledger. Cards/banners/pulls
// arrive in later phases. Mirrors services/notifications boot sequence.
package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "gacha")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := db.AutoMigrate(
		&domain.Wallet{}, &domain.LedgerEntry{},
		&domain.Card{}, &domain.Group{}, &domain.CardGroup{},
		&domain.Banner{}, &domain.BannerCard{},
		&domain.CollectionEntry{}, &domain.PityCounter{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Partial unique index for credit idempotency — GORM can't express the
	// WHERE clause, so create it explicitly. IF NOT EXISTS = no-op on reboot.
	if err := db.DB.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_gacha_ledger_dedup
		 ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	).Error; err != nil {
		log.Fatalw("failed to create ledger dedup index", "error", err)
	}

	// MinIO storage for gacha card/banner art.
	storage, err := videoutils.NewStorage(cfg.Storage)
	if err != nil {
		log.Fatalw("failed to init minio storage", "error", err)
	}
	if err := storage.EnsureBucket(context.Background()); err != nil {
		log.Fatalw("failed to ensure gacha-cards bucket", "error", err)
	}
	log.Infow("gacha-cards bucket ensured", "bucket", cfg.Storage.BucketName)

	walletRepo := repo.NewWalletRepository(db.DB)
	walletSvc := service.NewWalletService(walletRepo, cfg.Economy.StarterBonus, cfg.Enabled, log)

	contentRepo := repo.NewContentRepository(db.DB)
	bannerRepo := repo.NewBannerRepository(db.DB)
	pullRepo := repo.NewPullRepository(db.DB)
	contentSvc := service.NewContentService(contentRepo, bannerRepo)
	imageSvc := service.NewImageService(&storageAdapter{storage})
	pullSvc := service.NewPullService(pullRepo, bannerRepo, contentRepo, cfg.Economy, service.NewSecureRand(), log)

	walletHandler := handler.NewWalletHandler(walletSvc, log)
	internalHandler := handler.NewInternalHandler(walletSvc, log)
	adminHandler := handler.NewAdminHandler(contentSvc, imageSvc, log)
	imagesHandler := handler.NewImagesHandler(storage, log)
	pullHandler := handler.NewPullHandler(pullSvc, log)

	metricsCollector := metrics.NewCollector("gacha")
	router := transport.NewRouter(walletHandler, internalHandler, adminHandler, imagesHandler, pullHandler, cfg.JWT, log, metricsCollector)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("gacha")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting gacha service",
			"address", cfg.Server.Address(),
			"db_name", cfg.Database.Database,
			"enabled", cfg.Enabled,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down gacha service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("gacha service stopped")
}

// storageAdapter adapts *videoutils.Storage to the service.objectStore interface.
// The real Upload returns (*VideoFile, error); the interface only needs error.
type storageAdapter struct{ s *videoutils.Storage }

func (a *storageAdapter) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := a.s.Upload(ctx, key, r, size, contentType)
	return err
}
