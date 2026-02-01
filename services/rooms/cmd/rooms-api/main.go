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
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/config"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/transport"
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
	ctx := context.Background()
	db, err := database.New(ctx, cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Initialize services
	roomService := service.NewRoomService(redisCache, log)
	wsService := service.NewWebSocketService(log)
	leaderboardService := service.NewLeaderboardService(log)

	// Initialize handlers
	roomHandler := handler.NewRoomHandler(roomService, log)
	wsHandler := handler.NewWebSocketHandler(wsService, log)
	leaderboardHandler := handler.NewLeaderboardHandler(leaderboardService, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("rooms")

	// Initialize router
	router := transport.NewRouter(roomHandler, wsHandler, leaderboardHandler, cfg.JWT, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server
	go func() {
		log.Infow("starting rooms service", "address", cfg.Server.Address())
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
