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
//  9. service.NewCatalogClient → NewInboundRouter (catalog back-channel
//     for WT-STATE-02 validation, Plan 04.2 + 04.3).
// 10. service.NewGraceManager (Plan 05.1) — per-room reconnect-window
//     timer registry. Constructed AFTER wsHub (needs HubFanout for the
//     fire-time broadcast) and BEFORE the handlers (both Cancel via this).
// 11. handler.NewRoomHandler — chi handlers for POST/GET/DELETE /rooms.
//     DELETE now (05.1) broadcasts room:closed via wsHub and cancels any
//     pending grace timer before the explicit DeleteRoom.
// 12. handler.NewWebSocketHandler — /ws upgrade entry point (01.5).
//     Upgrade Cancels any pending grace timer; OnClose Starts a new one
//     when MemberCount drops to 0 (Plan 05.1).
// 13. transport.NewRouter — mounts /health + /metrics + /rooms + /ws.
// 14. http.Server with graceful shutdown on SIGINT/SIGTERM.
//
// On SIGTERM: hub.Close() runs FIRST so live WS connections drain cleanly.
// graceMgr.Close() runs AFTER hub.Close — the OnClose cascade from
// hub.Close would otherwise schedule a flurry of grace timers
// immediately before teardown; graceMgr's `closed` atomic flag
// short-circuits those Start calls so SIGTERM teardown stays quiet.
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

	// Repo → Service. The service is the single mutation surface for room
	// lifecycle (Plan 01.4); the WS upgrader (01.5) shares the same
	// *RoomService for snapshot generation.
	roomRepo := repo.NewRoomRepo(redisClient, cfg.RoomTTL, log)
	roomService := service.NewRoomService(roomRepo, log)

	// Hub (01.3) — in-process WebSocket connection registry. instanceID
	// tags every pubsub publish so the hub drops its own echoes
	// (single-instance v1.0 default; identifies foreign instances for v2).
	instanceID := uuid.NewString()
	wsHub := hub.NewHub(roomRepo, log, instanceID)

	// Inbound message router (01.6) — dispatches every WS envelope to the
	// correct handler, drives the drift engine, enforces per-user rate
	// limits, and (Plan 04.3) validates state-change inbounds against the
	// catalog before broadcasting. Wired into the WS handler so
	// Connection.OnMessage = router.Dispatch + Connection.OnClose chains
	// to router.OnDisconnect for drift/rate-limit cleanup.
	driftEngine := service.NewDriftEngine(log)
	rateLimiter := service.NewRateLimiter()
	catalogClient := service.NewCatalogClient(cfg.CatalogURL, log)
	inboundRouter := service.NewInboundRouter(roomRepo, wsHub, driftEngine, rateLimiter, catalogClient, log)

	// Grace manager (Plan 05.1, WT-POLISH-02) — per-room reconnect-window
	// timer registry. Cancel-on-rejoin / Start-on-last-disconnect / fire-after
	// cfg.GracePeriod broadcasts room:closed + DeleteRoom. Construction order:
	// AFTER wsHub (needs HubFanout for the fire-time broadcast) and BEFORE
	// the WS + room handlers (both Cancel via this manager).
	graceMgr := service.NewGraceManager(roomRepo, wsHub, cfg.GracePeriod, log)

	// Room handler — POST/GET/DELETE /rooms. The DELETE path (Plan 05.1)
	// now broadcasts room:closed via wsHub and cancels any pending grace
	// timer before the explicit DeleteRoom.
	roomHandler := handler.NewRoomHandler(roomService, wsHub, graceMgr, cfg, log)

	// WS upgrade handler (01.5) — JWT validation, capacity gate, snapshot
	// on connect, member:joined/left lifecycle. Mounted at /api/watch-together/ws
	// in transport.NewRouter, OUTSIDE the AuthMiddleware-wrapped subgroup.
	wsHandler := handler.NewWebSocketHandler(wsHub, roomRepo, roomService, inboundRouter, graceMgr, cfg, log)

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

	// Grace manager next — flips the closed flag so any OnClose-driven
	// graceMgr.Start cascade from the hub.Close above is immediately a
	// no-op, then Stops every still-pending timer. Order matters: must run
	// AFTER hub.Close (so the OnClose cascade is suppressed) and BEFORE
	// redisClient.Close (so a racing fire goroutine doesn't hit a closed
	// client).
	graceMgr.Close()

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
