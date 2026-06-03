package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/transport"
	"github.com/redis/go-redis/v9"
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
	// fatal, so an unreachable collector cannot take the gateway down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "gateway")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			// Bounded so a SIGTERM during a collector outage can't hang the
			// graceful shutdown on a blocked OTLP flush.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize services
	proxyService := service.NewProxyService(cfg.Services, log)

	// Initialize handlers
	proxyHandler := handler.NewProxyHandler(proxyService, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("gateway")

	// WV3-T3: wire Redis for the per-user GCRA rate limiter. RedisAddr was
	// already in config (host:port form, e.g. "redis:6379") but had no
	// consumer before this task. We use a raw *redis.Client because the
	// redis_rate/v10 limiter accepts the client directly — libs/cache is a
	// JSON-marshaling cache helper, not a thin client factory, and would
	// add an extra layer with no upside for this use case. We ping at
	// startup so an unreachable Redis surfaces in the logs immediately;
	// the middleware itself fails open (logs WARN, lets the request
	// through), so a Redis outage degrades to "per-IP only" rather than
	// 500ing every authenticated request.
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.Services.RedisAddr})
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		// Don't fatal — fail open: gateway still serves traffic, just
		// without the per-user limiter until Redis comes back. The per-IP
		// limiter higher in the chain remains in force.
		log.Warnw("gateway redis ping failed; per-user rate limiter will fail-open until reachable",
			"addr", cfg.Services.RedisAddr,
			"error", err,
		)
	} else {
		log.Infow("gateway redis connection established",
			"addr", cfg.Services.RedisAddr,
			"user_rate_per_minute", cfg.RateLimit.UserPerMinute,
			"user_rate_burst", cfg.RateLimit.UserBurst,
		)
	}
	pingCancel()
	defer func() { _ = redisClient.Close() }()

	// Initialize router (wires the user rate limiter into every protected group)
	router, _ := transport.NewRouterWithCleanup(proxyHandler, cfg, log, metricsCollector, redisClient)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("gateway")(router),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server
	go func() {
		log.Infow("starting gateway service", "address", cfg.Server.Address())
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
