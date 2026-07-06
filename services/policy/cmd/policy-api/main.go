// Package main is the policy-service entrypoint (port 8098).
//
// policy-service — runtime feature-access authority (RBAC + roulette). Owns the
// feature_flags table; resolves (userID,role)->access for the gateway (ruleset
// feed) and the SPA (/features/mine). Enforcement middleware lives in the
// gateway (Phase 2). Boot: logger -> config -> db -> redis -> AutoMigrate ->
// SeedDefaults -> router -> serve.
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
	"github.com/ILITA-hub/animeenigma/services/policy/internal/config"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "policy")
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

	if err := db.AutoMigrate(&domain.FeatureFlag{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Redis is reserved for the Phase-2 ruleset invalidate pub/sub; connect
	// non-fatally so policy-service boots even if Redis is briefly down.
	if redisCache, err := cache.New(cfg.Redis); err != nil {
		log.Warnw("redis unavailable at boot (ruleset pub/sub disabled)", "error", err)
	} else {
		defer redisCache.Close()
	}

	policySvc := service.NewPolicyService(repo.NewFeatureFlagRepository(db.DB), log)
	if err := policySvc.SeedDefaults(context.Background()); err != nil {
		log.Fatalw("failed to seed default flags", "error", err)
	}

	adminH := handler.NewAdminFlagsHandler(policySvc, log)
	publicH := handler.NewPublicFlagsHandler(policySvc, log)
	internalH := handler.NewInternalRulesetHandler(policySvc, log)

	router := transport.NewRouter(adminH, publicH, internalH, cfg.JWT, log, metrics.NewCollector("policy"))

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("policy")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting policy service", "address", cfg.Server.Address(), "db_name", cfg.Database.Database)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down policy service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("policy service stopped")
}
