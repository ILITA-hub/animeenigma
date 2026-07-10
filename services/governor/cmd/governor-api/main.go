// Package main is the governor entrypoint (port 8100).
//
// governor — the graceful-degradation authority (Phase 2 of
// docs/superpowers/specs/2026-07-10-graceful-degradation-design.md). Every
// tick it polls the ae:* Prometheus recording rules (Phase-1 awareness layer),
// smooths the instantaneous pressure target through an enter-fast/exit-slow
// hysteresis machine, applies the owner override, and publishes the level:
// Redis (ae:degradation:level, TTL'd so consumers fail open), Prometheus
// gauges (ae_degradation_level, ae_degradation_reason_active), and a durable
// ClickHouse transition log via analytics /internal/degradation/transition.
// Boot: logger -> config -> redis -> governor loop -> router -> serve.
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
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/config"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/promquery"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/service"
	"github.com/ILITA-hub/animeenigma/services/governor/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "governor")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Redis is the publication channel. Fatal at boot: a governor that can
	// never publish is useless, and compose orders us after redis-healthy.
	// (At RUNTIME Redis failures are non-fatal — consumers fail open on the
	// expired key and the loop keeps retrying.)
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("redis unavailable at boot", "error", err)
	}
	defer redisCache.Close()

	collector := metrics.NewCollector("governor")

	gov := service.New(
		promquery.NewClient(cfg.PrometheusURL),
		repo.NewRedisStore(redisCache.Client()),
		repo.NewAnalyticsSink(cfg.AnalyticsURL, log),
		log,
		cfg.Tick, cfg.LevelTTL, cfg.EnterTicks, cfg.ExitTicks, cfg.PromFailTicks,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gov.Start(ctx)
	log.Infow("governor loop started",
		"tick", cfg.Tick, "enter_ticks", cfg.EnterTicks, "exit_ticks", cfg.ExitTicks,
		"level_ttl", cfg.LevelTTL, "prometheus", cfg.PrometheusURL)

	router := transport.NewRouter(handler.NewStatusHandler(gov), log, collector)
	srv := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Infow("governor listening", "addr", cfg.Server.Address())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalw("server error", "error", err)
	}
}
