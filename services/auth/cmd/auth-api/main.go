package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/transport"
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
	tracer, err = tracing.InitFromEnv(context.Background(), "auth")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize database (auto-creates DB if not exists)
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
		metrics.StartUserMetricsCollector(sqlDB, 60*time.Second)
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&domain.User{}, &domain.UserSession{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Activity-register effect producer (AR-EFFECT-01). Non-blocking +
	// drop-on-full so an analytics outage never affects auth requests.
	// SetGlobalSink also lights up tracing.WrapTransport recording for any
	// shared HTTP client this service uses.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EFFECT-01 (D-15): GORM db_write/db_read effect callbacks + the daily-P95
	// ReadGate refresher (D-03), reusing the existing Redis client as HashReader.
	dbEffectsCtx, dbEffectsCancel := context.WithCancel(context.Background())
	defer dbEffectsCancel()
	dbEffectsStop, err := gormtrace.WireDBEffects(dbEffectsCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

	// Initialize repositories
	userRepo := repo.NewUserRepository(db.DB)
	sessionRepo := repo.NewSessionRepository(db.DB)

	// Initialize services
	authService := service.NewAuthService(userRepo, sessionRepo, redisCache, cfg.JWT, cfg.GuestTokenTTL, log)
	userService := service.NewUserService(userRepo, log)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService, cfg.Cookie, log)
	telegramOIDC := service.NewTelegramOIDC(cfg.TelegramOIDC, redisCache, log)
	telegramOIDCHandler := handler.NewTelegramOIDCHandler(telegramOIDC, authService, authHandler, log)
	userHandler := handler.NewUserHandler(userService, log)
	sessionsHandler := handler.NewSessionsHandler(authService, log)
	magicLinkHandler := handler.NewMagicLinkHandler(authService, authHandler, cfg.MagicLinkTargetBase, log)
	userResolveHandler := handler.NewUserResolveHandler(userRepo, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("auth")

	// Initialize router
	router := transport.NewRouter(authHandler, telegramOIDCHandler, userHandler, sessionsHandler, magicLinkHandler, userResolveHandler, cfg.JWT, log, metricsCollector)

	// One-time webhook teardown: the deep-link login flow is gone, so tell
	// Telegram to stop POSTing /start updates at the removed route.
	// Idempotent; warn-only — login no longer depends on the bot.
	if cfg.Telegram.BotToken != "" {
		go deleteTelegramWebhook(cfg.Telegram.BotToken, log)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("auth")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown channel — registered before starting goroutines so
	// they can listen for the same signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// shutdown is closed when SIGINT is received — long-lived goroutines
	// (session cleanup) listen for it.
	shutdown := make(chan struct{})

	// Periodic session cleanup — drops rows revoked or expired >7 days ago.
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-shutdown:
				return
			case <-t.C:
				n, err := authService.CleanupExpiredSessions(context.Background())
				if err != nil {
					log.Warnw("session cleanup failed", "error", err)
					continue
				}
				if n > 0 {
					log.Infow("session cleanup", "deleted", n)
				}
			}
		}
	}()

	// Start server
	go func() {
		log.Infow("starting auth service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	<-quit
	close(shutdown)

	log.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}

// redactWebhookErr strips the request URL from transport errors before they
// reach logs — the deleteWebhook URL embeds the bot token.
func redactWebhookErr(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

// deleteTelegramWebhook tears down the legacy bot-webhook registration.
func deleteTelegramWebhook(botToken string, log *logger.Logger) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://api.telegram.org/bot"+botToken+"/deleteWebhook", "application/json", nil)
	if err != nil {
		log.Warnw("telegram deleteWebhook failed", "error", redactWebhookErr(err))
		return
	}
	defer resp.Body.Close()
	log.Infow("telegram webhook deleted", "status", resp.StatusCode)
}
