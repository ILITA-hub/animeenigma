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
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/config"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "streaming")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	ctx := context.Background()

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Degradation-aware image-resize lever (graceful-degradation Phase 3
	// consumer): under Critical platform degradation (level >= 2) the image
	// proxy sheds the CPU-heavy on-the-fly poster downscale and serves the
	// original bytes it already has in hand. Fail-open — a down/undeployed
	// governor (missing Redis key) reads level 0, i.e. normal resize. The poll
	// loop runs for the process lifetime off the background ctx.
	imgDegWatcher := cache.NewDegradationWatcher(redisCache, 5*time.Second)
	imgDegWatcher.Start(ctx)

	// Initialize MinIO storage
	storage, err := videoutils.NewStorage(cfg.Storage)
	if err != nil {
		log.Fatalw("failed to connect to storage", "error", err)
	}

	// Ensure bucket exists
	if err := storage.EnsureBucket(ctx); err != nil {
		log.Warnw("failed to ensure bucket exists", "error", err)
	}

	// Optional second storage backend: external S3-compatible host (library
	// episodes may live here alongside local MinIO). Nil when S3_ENDPOINT is
	// unset — the HLS proxy then presigns only against MinIO, unchanged from
	// before this existed. EnsureBucket is deliberately NOT called here:
	// unlike MinIO, this bucket is provisioned out-of-band on the external
	// host, and streaming only ever reads from it (never uploads).
	var s3Storage *videoutils.Storage
	if cfg.S3Storage != nil {
		s3Storage, err = videoutils.NewStorage(*cfg.S3Storage)
		if err != nil {
			log.Fatalw("failed to connect to S3 storage", "error", err)
		}
	}

	// Initialize video proxy
	proxy := videoutils.NewVideoProxy(cfg.Proxy)

	// Initialize services
	streamingService := service.NewStreamingService(storage, proxy, redisCache, cfg, log)

	// Egress effect producer + HLS session aggregator (AR-EGRESS-04/05).
	// The producer ships aggregated egress rows to analytics /internal/effects;
	// it is non-blocking + drop-on-full so an analytics outage never affects
	// playback. The aggregator folds a watch's many segment GETs (correlated by
	// the per-manifest ?sess= token) into ONE row per (session, host) on idle
	// flush — never one row per ~6s segment.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	hlsSessions := service.NewHLSSessions(effectProducer, 0, 0) // defaults: 45s idle, 10k cap
	hlsSessions.Start()
	// Flush all open sessions BEFORE the producer drains so the final rows are
	// shipped (D-06 graceful flush).
	defer hlsSessions.Stop()

	// Initialize handlers
	streamHandler := handler.NewStreamHandlerWithSessions(streamingService, s3Storage, hlsSessions, log)
	uploadHandler := handler.NewUploadHandler(streamingService, log)

	// Initialize image proxy
	imageProxyService := service.NewImageProxyService(storage, imgDegWatcher, log, cfg.GachaInternalURL)
	imageProxyHandler := handler.NewImageProxyHandler(imageProxyService, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("streaming")

	// Initialize router
	router := transport.NewRouter(streamHandler, uploadHandler, imageProxyHandler, cfg, log, metricsCollector)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("streaming")(router),
		ReadTimeout:  5 * time.Minute, // Long timeout for video uploads
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting streaming service", "address", cfg.Server.Address())
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
