// Package main is the analytics clickstream service entrypoint (port 8092).
//
// Boot: logger → config → database.New (auto-creates DB) → AutoMigrateAll +
// EnsureView → batcher.Start → purge cron → router → HTTP server with
// graceful shutdown (batcher drains on stop).
package main

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/config"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/job"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/probe"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/roster"
	svc "github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/transport"
	"github.com/robfig/cron/v3"
)

// countingSink, repoEraser, and repoPurger are defined in adapters.go.

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load failed", "error", err)
	}

	if cfg.IPSalt == "" || cfg.IPSalt == "change-me-in-production" || cfg.IPSalt == "dev-salt-change-in-production" {
		log.Warnw("ANALYTICS_IP_SALT is unset or a placeholder — IP hashes are guessable; set a strong secret in docker/.env")
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect failed", "error", err)
	}
	defer db.Close()

	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := repo.AutoMigrateAll(db.DB); err != nil {
		log.Fatalw("migrate failed", "error", err)
	}
	if err := repo.EnsureView(db.DB); err != nil {
		log.Fatalw("view create failed", "error", err)
	}

	// Postgres store is ALWAYS constructed — it is the dual-write primary /
	// source of truth and the reversible-rollback path. The PG tables/view are
	// migrated above regardless of backend.
	pgStore := repo.NewPostgresStore(db.DB)

	// Select the EventStore on ANALYTICS_STORE_BACKEND (default "postgres" keeps
	// the system exactly as today). "clickhouse"/"dualwrite" open the native CH
	// connection + run EnsureSchema. On CH failure: clickhouse → Fatal (explicit
	// CH-only must not silently fall back); dualwrite → WARN + degrade to PG-only
	// (a CH outage must never take analytics down — T-01-07 reversibility).
	log.Infow("analytics store backend", "backend", cfg.StoreBackend)
	var store domain.EventStore = pgStore
	var chConn driver.Conn
	if cfg.StoreBackend == "clickhouse" || cfg.StoreBackend == "dualwrite" {
		conn, err := repo.OpenClickHouse(repo.CHConfig{
			Addr:     cfg.ClickHouse.Addr(),
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.User,
			Password: cfg.ClickHouse.Password,
		})
		if err == nil {
			err = repo.EnsureSchema(context.Background(), conn)
		}
		if err != nil {
			if cfg.StoreBackend == "clickhouse" {
				log.Fatalw("clickhouse backend selected but unavailable", "error", err)
			}
			log.Warnw("dualwrite: clickhouse unavailable at boot — degrading to postgres-only", "error", err)
		} else {
			chConn = conn
			defer chConn.Close()
			chStore := repo.NewClickHouseStore(chConn)
			if cfg.StoreBackend == "clickhouse" {
				store = chStore
			} else {
				store = repo.NewDualWriteStore(pgStore, chStore, log)
			}
		}
	}

	batcher := ingest.New(store, ingest.Config{
		MaxBatch:      cfg.MaxBatch,
		FlushInterval: cfg.FlushInterval,
		BufferSize:    cfg.BufferSize,
	}).WithLogger(log).WithDropHook(func() { observ.EventsDropped.Inc() })
	batcher.Start()

	// AUTO-608: the DB stream_providers roster (catalog /internal/scraper/providers)
	// is the single source of truth for provider EXISTENCE — it feeds the
	// player-telemetry whitelist, the playability roster filter, and (below,
	// when ClickHouse is wired) probe-target membership. 60s TTL, last-good
	// fallback, embedded cold-start snapshot; safe to construct before catalog
	// is reachable.
	rosterClient := roster.New(cfg.CatalogURL, time.Minute)

	collectHandler := handler.NewCollectHandler(countingSink{batcher}, cfg.IPSalt)
	// Log-only FE error sink — one structured Warnw line per accepted frontend
	// error (→ ClickHouse otel_logs via the OTel filelog receiver) + a
	// kind-labelled Prometheus counter. No DB write, no event-store enqueue.
	clientErrorHandler := handler.NewClientErrorHandler(handler.NewLoggingClientErrorSink(log), cfg.IPSalt)
	// Player telemetry handler shares the same batcher Sink — player resolve/stall
	// rows flow through the identical InsertBatch write path as clickstream rows,
	// stored under effect_kind "player_resolve" / "player_stall".
	playerTelemetryHandler := handler.NewPlayerTelemetryHandler(countingSink{batcher}, rosterClient)
	// The effects handler shares the same batcher Sink — effect rows flow
	// through the identical InsertBatch write path as clickstream rows.
	effectsHandler := handler.NewEffectsHandler(countingSink{batcher})
	adminHandler := handler.NewAdminHandler(repoEraser{db: db, chConn: chConn})

	// Redis publisher for the daily db_read P95 recompute (D-03). The hash is
	// the fan-out channel the 7 GORM services snapshot into their ReadGate. A
	// Redis outage at boot is NON-fatal: the recompute endpoint then degrades to
	// a no-op (nil writer) and the GORM services keep their last good 48h hash —
	// analytics' core ingestion must never depend on Redis being up.
	var readThresholdHandler *handler.ReadThresholdHandler
	var playerRankingHandler *handler.PlayerRankingHandler
	var probeHandler *handler.ProbeHandler
	redisCache, rerr := cache.New(cfg.Redis)
	if rerr != nil {
		log.Warnw("redis unavailable — read-threshold recompute disabled this boot", "error", rerr)
	} else {
		defer redisCache.Close()
		readThresholdSvc := svc.NewReadThresholdService(chConn, redisThresholdWriter{cache: redisCache})
		readThresholdHandler = handler.NewReadThresholdHandler(readThresholdSvc)
		// Stage 2b daily provider-reliability recompute + player_ranking Redis
		// publish. Needs ClickHouse for the telemetry aggregates, so it is only
		// wired when the CH backend is active (chConn != nil); the service's own
		// Recompute also nil-guards conn/redis defensively.
		if chConn != nil {
			playerRankingSvc := svc.NewPlayerRankingService(chConn, redisRankingWriter{cache: redisCache})
			playerRankingHandler = handler.NewPlayerRankingHandler(playerRankingSvc)

			// Playback probe engine — only wired when ClickHouse is available so
			// probe.PromReporter can persist probe_runs rows. The scheduler triggers
			// /internal/probe/run daily; operators may also call it ad-hoc.
			chStore := repo.NewClickHouseStore(chConn)
			validator := probe.NewHTTPValidator(cfg.StreamingURL, nil, probe.NewFFprobe(cfg.FFprobePath))

			// Shared instances for the EN scraper chain: one spotlight anime-set
			// and one scraper resolver (catalog /scraper/...).
			spotlight := probe.NewHTTPAnimeSet(cfg.CatalogURL, cfg.ProbeAnchorUUID, nil, rand.New(rand.NewSource(time.Now().UnixNano()))) //nolint:gosec
			scraperRes := probe.NewHTTPResolver(cfg.CatalogURL, nil)
			// One AnimeJoy resolver serves both legs (it keys on the provider name);
			// generous timeout for cold live discovery (search → news_id → playlist).
			animejoyRes := probe.NewAnimejoyResolver(cfg.CatalogURL, &http.Client{Timeout: 45 * time.Second})

			// Providers with custom rules. ae uses the "3 latest distinct-anime
			// library uploads" set + the ae stream resolver; kodik-noads reuses the
			// spotlight set with the scraped ad-free Kodik resolver.
			build := map[string]func() probe.ProbeTarget{
				"ae": func() probe.ProbeTarget {
					return probe.ProbeTarget{Provider: "ae", AnimeSet: probe.NewAeAnimeSet(cfg.CatalogURL, 3, nil), Resolver: probe.NewAeResolver(cfg.CatalogURL, nil)}
				},
				"kodik-noads": func() probe.ProbeTarget {
					return probe.ProbeTarget{Provider: "kodik-noads", AnimeSet: spotlight, Resolver: probe.NewKodikNoadsResolver(cfg.CatalogURL, nil)}
				},
				// animepahe is browser-engine (Camoufox): a cold Cloudflare-Turnstile
				// solve on an expired cf_clearance can take 30–60s, far over the
				// shared 15s scraper resolver timeout (a 15s cap would read a working
				// animepahe as a false "down" whenever the clearance lapsed between
				// 6h runs). Give it the shared spotlight set but a dedicated
				// long-timeout scraper resolver. Warm-cookie resolves are ~2s, and
				// the catalog plan probes it fail_fast with sample_size=1, so the
				// worst case is a single cold solve per tick.
				"animepahe": func() probe.ProbeTarget {
					return probe.ProbeTarget{Provider: "animepahe", AnimeSet: spotlight, Resolver: probe.NewHTTPResolver(cfg.CatalogURL, &http.Client{Timeout: 90 * time.Second})}
				},
				// AnimeJoy RU-sub legs: shared spotlight set + the catalog-side animejoy
				// resolver. Progressive mp4 (no HLS), so the validator ffprobes the file
				// head directly. Both legs share one resolver instance (keyed on name).
				"animejoy-sibnet": func() probe.ProbeTarget {
					return probe.ProbeTarget{Provider: "animejoy-sibnet", AnimeSet: spotlight, Resolver: animejoyRes}
				},
				"animejoy-allvideo": func() probe.ProbeTarget {
					return probe.ProbeTarget{Provider: "animejoy-allvideo", AnimeSet: spotlight, Resolver: animejoyRes}
				},
			}

			// AUTO-608: probe-target membership comes from the DB roster (the
			// catalog probe-plan already gates WHEN each row is probed; this
			// gates WHAT this binary can probe). PROBE_PROVIDERS is now an
			// OPTIONAL comma-separated filter (unset ⇒ every wirable row).
			var filter map[string]bool
			if cfg.ProbeProviders != "" {
				filter = map[string]bool{}
				for _, n := range strings.Split(cfg.ProbeProviders, ",") {
					if n = strings.TrimSpace(n); n != "" {
						filter[n] = true
					}
				}
			}
			var targets []probe.ProbeTarget
			for _, row := range rosterClient.Rows(context.Background()) {
				if filter != nil && !filter[row.Name] {
					continue
				}
				if b, ok := build[row.Name]; ok {
					targets = append(targets, b())
					continue
				}
				if row.ScraperOperated && row.Group == "en" {
					// Default: EN scraper provider — shared spotlight set + scraper resolver.
					targets = append(targets, probe.ProbeTarget{Provider: row.Name, AnimeSet: spotlight, Resolver: scraperRes})
					continue
				}
				// Intentionally unprobeable (kodik-iframe: no direct stream; hanime/
				// animelib/18anime: no probe resolver built). Logged, not gauged —
				// see plan deviation #2.
				log.Infow("probe target skipped — no resolver for roster row", "provider", row.Name, "group", row.Group)
			}

			pool := probe.NewHTTPPopularPool(cfg.CatalogURL, nil)
			planClient := probe.NewHTTPPlanClient(cfg.CatalogURL, &http.Client{Timeout: 15 * time.Second})
			engine := probe.NewEngine(
				targets,
				validator,
				probe.NewPromReporter(chStore),
				pool, rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
				func() int64 { return time.Now().Unix() },
				log,
				planClient,
			)
			probeHandler = handler.NewProbeHandler(engine)
		}
	}

	purgeJob := job.NewPurgeJob(repoPurger{db: db}, cfg.RetentionDays, log)
	c := cron.New()
	_, _ = c.AddFunc(cfg.PurgeCron, func() {
		_, _ = purgeJob.RunOnce(context.Background())
	})
	c.Start()
	defer c.Stop()

	// GPU telemetry handler (CD-15): only wired when ClickHouse is available so
	// the insert target exists. When nil, the route is not mounted (nil-guard in
	// NewRouter), and the upscaler's fire-and-forget producer gets 404 which it
	// swallows as a non-fatal outage.
	var upscaleTelemetryHandler *handler.UpscaleTelemetryHandler
	if chConn != nil {
		upscaleTelemetryHandler = handler.NewUpscaleTelemetryHandler(repo.NewClickHouseStore(chConn))
	}

	// Playability scores handler (Phase B Source-panel index): only wired when
	// ClickHouse is available, since both repo queries it drives (watch decay +
	// probe-up decay) read from CH tables.
	var playabilityHandler *handler.PlayabilityHandler
	if chConn != nil {
		playabilityHandler = handler.NewPlayabilityHandler(chConn, rosterClient)
	}

	// Degradation-transition handler (graceful-degradation Phase 2): only wired
	// when ClickHouse is available so the insert target exists. When nil the
	// route is not mounted and the governor's best-effort sink logs the 404.
	var degradationTransitionHandler *handler.DegradationTransitionHandler
	if chConn != nil {
		degradationTransitionHandler = handler.NewDegradationTransitionHandler(repo.NewClickHouseStore(chConn))
	}

	collector := metrics.NewCollector("analytics")
	router := transport.NewRouter(collectHandler, clientErrorHandler, playerTelemetryHandler, effectsHandler, adminHandler, readThresholdHandler, playerRankingHandler, probeHandler, playabilityHandler, upscaleTelemetryHandler, degradationTransitionHandler, log, collector)

	srv := &http.Server{Addr: cfg.Server.Address(), Handler: router}
	go func() {
		log.Infow("analytics service listening", "addr", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	batcher.Stop()
}
