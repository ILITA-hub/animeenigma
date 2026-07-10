// Package main is the storage service entrypoint (port 8099).
//
// storage-service — the single authority for user-content object storage
// across two S3-compatible backends (local MinIO + external
// s3.firstvds.ru). Docker-network-internal only, no gateway route. See
// docs/superpowers/specs/2026-07-10-storage-service-design.md.
//
// Boot sequence:
//
//  1. logger.Default()
//  2. config.Load() — STORAGE_* env vars, all with safe defaults; the s3
//     backend is simply absent if STORAGE_S3_ENDPOINT is empty (dev).
//  3. tracing.InitFromEnv — no-op unless TRACING_ENABLED=true.
//  4. metrics.NewCollector("storage").
//  5. service.NewBackends — constructs the minio client (always) and the s3
//     client (only if configured), then EnsureBuckets creates each bucket
//     if absent (idempotent — mirrors services/library's minio writer).
//  6. service.NewPlacement — the placement policy authority, seeded from
//     cfg.Defaults and backends.HasS3() (drives the s3-absent fallback).
//  7. handler.NewStorageHandler + transport.NewRouter.
//  8. http.Server with graceful shutdown on SIGINT/SIGTERM.
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
	"github.com/ILITA-hub/animeenigma/services/storage/internal/config"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/service"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export
	// to OTLP_ENDPOINT (default otel-collector:4317); failures are dropped,
	// never fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "storage")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	metricsCollector := metrics.NewCollector("storage")

	backends, err := service.NewBackends(cfg, log)
	if err != nil {
		log.Fatalw("failed to init storage backends", "error", err)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := backends.EnsureBuckets(bootCtx); err != nil {
		bootCancel()
		log.Fatalw("failed to ensure buckets", "error", err)
	}
	bootCancel()

	placement := service.NewPlacement(cfg.Defaults, !backends.HasS3(), log)
	storageHandler := handler.NewStorageHandler(backends, placement, log)

	router := transport.NewRouter(storageHandler, log, metricsCollector)

	srv := &http.Server{
		Addr:    cfg.Server.Address(),
		Handler: tracing.HTTPMiddleware("storage")(router),
		// WriteTimeout must accommodate the BULK routes: /internal/storage/copy
		// streams a whole episode prefix (potentially multi-GB) cross-backend
		// before it can respond — a 1.8GB prefix blew through the previous 30s
		// during the 2026-07-10 storage migration. This service is Docker-
		// network-only (never internet-facing), so a generous cap is safe; it
		// must stay >= libs/storageclient's longOpTimeout.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 20 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting storage service",
			"address", cfg.Server.Address(),
			"s3_enabled", backends.HasS3(),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down storage service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("storage service stopped")
}
