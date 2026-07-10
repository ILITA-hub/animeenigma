// Package main is the fanfic service entrypoint (port 8097) — an admin-only
// AI fanfiction generator backed by Groq. Mirrors the gacha boot sequence.
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
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/catalog"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/config"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "fanfic")
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
	if err := db.AutoMigrate(&domain.Fanfic{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	redis, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redis.Close()

	groqClient := groq.New(cfg.Groq.APIKey, cfg.Groq.BaseURL, cfg.Groq.Model, cfg.Groq.Timeout)
	fanficRepo := repo.NewRepository(db.DB)
	quota := service.NewQuota(newRedisQuotaStore(redis), cfg.DailyCap, time.Now)
	catalogClient := catalog.NewClient(cfg.CatalogURL, cfg.CatalogTimeout, log)
	generator := service.NewGenerator(groqClient, fanficRepo, quota, catalogClient, cfg.Groq.Model, cfg.ContinueContextRunes, log)
	h := handler.NewHandler(generator, fanficRepo, log)

	mc := metrics.NewCollector("fanfic")
	router := transport.NewRouter(h, cfg.JWT, log, mc)

	srv := &http.Server{
		Addr:        cfg.Server.Address(),
		Handler:     tracing.HTTPMiddleware("fanfic")(router),
		ReadTimeout: 15 * time.Second,
		// WriteTimeout MUST be 0 — SSE responses are long-lived and a non-zero
		// write deadline would truncate the stream.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting fanfic service", "address", cfg.Server.Address(), "model", cfg.Groq.Model)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Infow("fanfic service stopped")
}

// redisQuotaStore adapts *cache.RedisCache to the service.quotaStore interface.
type redisQuotaStore struct{ rc *cache.RedisCache }

func newRedisQuotaStore(rc *cache.RedisCache) *redisQuotaStore { return &redisQuotaStore{rc: rc} }

func (s *redisQuotaStore) Incr(ctx context.Context, key string) (int64, error) {
	return s.rc.Client().Incr(ctx, key).Result()
}
func (s *redisQuotaStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.rc.Client().Expire(ctx, key, ttl).Err()
}
func (s *redisQuotaStore) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.rc.Client().SetNX(ctx, key, "1", ttl).Result()
}
func (s *redisQuotaStore) Del(ctx context.Context, key string) error {
	return s.rc.Client().Del(ctx, key).Err()
}
