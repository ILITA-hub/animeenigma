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
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/config"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/transport"
)

func main() {
	log := logger.Default()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	ctx := context.Background()

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Initialize MinIO storage
	storage, err := videoutils.NewStorage(cfg.Storage)
	if err != nil {
		log.Fatalw("failed to connect to storage", "error", err)
	}

	// Ensure bucket exists
	if err := storage.EnsureBucket(ctx); err != nil {
		log.Warnw("failed to ensure bucket exists", "error", err)
	}

	// Initialize video proxy
	proxy := videoutils.NewVideoProxy(cfg.Proxy)

	// Initialize services
	streamingService := service.NewStreamingService(storage, proxy, redisCache, cfg, log)

	// Initialize handlers
	streamHandler := handler.NewStreamHandler(streamingService, log)
	uploadHandler := handler.NewUploadHandler(streamingService, log)

	// Initialize router
	router := transport.NewRouter(streamHandler, uploadHandler, cfg, log)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  5 * time.Minute,  // Long timeout for video uploads
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
