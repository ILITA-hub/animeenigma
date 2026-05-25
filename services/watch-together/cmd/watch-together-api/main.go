// Package main is the watch-together service entrypoint (port 8091).
//
// v1.0 Watch Together — workstream watch-together, Phase 1.
//
// Boot sequence (updated through Plan 01.5):
//
//  1. logger.Default()
//  2. config.Load() — JWT_SECRET required; SERVER_PORT defaults to 8091;
//     MaxMembers / RoomTTL / GracePeriod / PublicBaseURL populated from
//     WATCH_TOGETHER_* env vars.
//  3. metrics.NewCollector("watch-together") — service-labelled Prometheus
//     registration.
//  4. redis.NewClient — connection to the shared Redis instance (host/port
//     from cfg.Redis). Health-checked via PING before the HTTP listener
//     starts; a Redis outage at boot is fatal because every Redis-only
//     path needs the client.
//  5. repo.NewRoomRepo — Redis facade owning every wt:* key.
//  6. service.NewRoomService — single mutation surface for the room
//     lifecycle (REST handler + WS snapshot generation).
//  7. instanceID via uuid.NewString — tags every pubsub publish so the
//     hub can drop its own echoes (forward-compat for v2 multi-instance).
//  8. hub.NewHub — in-process WebSocket connection registry.
//  9. handler.NewRoomHandler — chi handlers for POST/GET/DELETE /rooms.
// 10. handler.NewWebSocketHandler — /ws upgrade entry point (01.5).
// 11. transport.NewRouter — mounts /health + /metrics + /rooms + /ws.
// 12. http.Server with graceful shutdown on SIGINT/SIGTERM.
//
// On SIGTERM: hub.Close() runs BEFORE srv.Shutdown so live WS connections
// drain cleanly (the hub's close path tears down per-room pubsub
// subscribers and closes every connection).
//
// No database — service is Redis-only by design (WT-FOUND-02; persistence
// deferred to v1.2 via the persistent-rooms workstream).
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/hub"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/service"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	metricsCollector := metrics.NewCollector("watch-together")

	// Redis client — every persistent piece of state in this service lives
	// in Redis (WT-FOUND-02). PING up-front so config errors surface at
	// boot, not at first request.
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		pingCancel()
		log.Fatalw("redis ping failed", "addr", fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port), "error", err)
	}
	pingCancel()

	// Repo → Service → Handler. The service is the single mutation surface
	// for room lifecycle (Plan 01.4); the WS upgrader (01.5) shares the
	// same *RoomService for snapshot generation.
	roomRepo := repo.NewRoomRepo(redisClient, cfg.RoomTTL, log)
	roomService := service.NewRoomService(roomRepo, log)
	roomHandler := handler.NewRoomHandler(roomService, cfg, log)

	// Hub (01.3) — in-process WebSocket connection registry. instanceID
	// tags every pubsub publish so the hub drops its own echoes
	// (single-instance v1.0 default; identifies foreign instances for v2).
	instanceID := uuid.NewString()
	wsHub := hub.NewHub(roomRepo, log, instanceID)

	// WS upgrade handler (01.5) — JWT validation, capacity gate, snapshot
	// on connect, member:joined/left lifecycle. Mounted at /api/watch-together/ws
	// in transport.NewRouter, OUTSIDE the AuthMiddleware-wrapped subgroup.
	wsHandler := handler.NewWebSocketHandler(wsHub, roomRepo, roomService, cfg, log)

	router := transport.NewRouter(cfg, roomHandler, wsHandler, log, metricsCollector)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting watch-together service",
			"address", cfg.Server.Address(),
			"instance_id", instanceID,
			"max_members", cfg.MaxMembers,
			"room_ttl", cfg.RoomTTL,
			"grace_period", cfg.GracePeriod,
			"public_base_url", cfg.PublicBaseURL,
			"allow_all_origins", cfg.AllowAllOrigins,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down watch-together service...")

	// Tear down the hub FIRST so live WS connections drain cleanly before
	// the HTTP listener stops accepting. The hub's Close cancels every
	// per-room pubsub subscriber and closes every connection in one shot.
	wsHub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	if err := redisClient.Close(); err != nil {
		log.Warnw("redis client close failed", "error", err)
	}
	log.Info("watch-together service stopped")
}
