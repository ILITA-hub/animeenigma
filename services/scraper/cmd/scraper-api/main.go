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
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animepahe"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	if cfg.MegacloudExtractor.URL == "" {
		log.Warnw("MEGACLOUD_EXTRACTOR_URL is empty; megacloud-backed extraction will fail when invoked")
	}

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("scraper")

	// Build the embed extractor registry. Order matters — registry.Find
	// returns the first matching extractor, so the cheapest Matches() check
	// goes first.
	registry := domain.NewRegistry()

	// Kwik first — Matches() is plain host equality on 2 hosts.
	kwikExtractor := embeds.NewKwikExtractor()
	registry.Register(kwikExtractor)
	log.Infow("registered embed extractor", "name", kwikExtractor.Name())

	// Megacloud — kept registered for Phase 18+ providers (9anime, AnimeKai).
	mcClient := embeds.NewMegacloudClient(cfg.MegacloudExtractor.URL, cfg.MegacloudExtractor.Timeout)
	registry.Register(mcClient)
	log.Infow("registered embed extractor", "name", mcClient.Name())

	// Phase 18 — Gogoanime/Anitaku embed wrappers. These MUST be registered
	// BEFORE the gogoanime.Provider constructor below so the provider's
	// embeds.Find dispatch can hit them at GetStream time.
	vibeplayerExtractor := embeds.NewVibePlayerExtractor()
	registry.Register(vibeplayerExtractor)
	log.Infow("registered embed extractor", "name", vibeplayerExtractor.Name())

	streamhgExtractor := embeds.NewStreamHGExtractor()
	registry.Register(streamhgExtractor)
	log.Infow("registered embed extractor", "name", streamhgExtractor.Name())

	earnvidsExtractor := embeds.NewEarnvidsExtractor()
	registry.Register(earnvidsExtractor)
	log.Infow("registered embed extractor", "name", earnvidsExtractor.Name())

	// Redis cache — shared by malsync, episodes, stream TTLs. We fatal here
	// instead of degrading gracefully because every AnimePahe response is
	// cached on the way back; without Redis the upstream rate limits and
	// DDoS-Guard handshake would hit the wall on every request.
	redisCache, err := cache.New(cache.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer func() { _ = redisCache.Close() }()
	log.Infow("redis connected", "host", cfg.Redis.Host, "port", cfg.Redis.Port)

	// Build the shared HTTP client for AnimePahe. Per-host rate limits keep
	// us polite and let DDoS-Guard's cookie jar warm up before sustained
	// traffic. The jar is exposed via *BaseHTTPClient.Jar() so the
	// ensureDDoSCookie helper can pre-populate `__ddg2_` cookies.
	animePaheBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("animepahe.ru", 1.0, 2),
		domain.WithPerHostRPS("animepahe.com", 1.0, 2),
		domain.WithPerHostRPS("kwik.cx", 1.0, 2),
		domain.WithPerHostRPS("kwik.si", 1.0, 2),
		domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
	)

	// MalSync client (24h positive + 24h negative cache).
	malSyncClient := animepahe.NewMalSyncClient(redisCache)

	// AnimePahe provider — first live Provider in v3.0.
	// WR-11: New now validates required dependencies eagerly and returns an
	// error so a misconfigured Deps surfaces at boot, not via a runtime
	// 502 (nil pointer dereference) minutes after deploy.
	animePaheProvider, err := animepahe.New(animepahe.Deps{
		BaseURL: cfg.AnimePahe.BaseURL,
		HTTP:    animePaheBaseHTTP,
		Embeds:  registry,
		MalSync: malSyncClient,
		Cache:   redisCache,
		Log:     log,
	})
	if err != nil {
		log.Fatalw("failed to construct AnimePahe provider", "error", err)
	}

	// Build the orchestrator and register the provider before HTTP starts.
	// Phase 17 Plan 02 wires the real health cache that Plan 01 introduced;
	// the orchestrator gates dispatch on cache.IsHealthy and the probe
	// runner (spawned below) populates the cache on each tick.
	cache := health.NewInMemoryHealthCache()
	orchestrator := service.NewOrchestrator(log, registry, cache)
	orchestrator.Register(animePaheProvider)
	log.Infow("registered provider", "name", animePaheProvider.Name())

	// Gogoanime/Anitaku — second EN provider (Phase 18).
	// Pivoted from "9anime" since the entire 9anime mirror chain is dead per
	// 2026-05-12 research (.planning/phases/18-9anime/18-RESEARCH.md).
	// Backend slug is "gogoanime"; the user-facing display label is "Anitaku".
	// Registration ORDER is the failover ORDER (CONTEXT.md D5) — animepahe
	// is tried first; gogoanime is the second-chance provider when animepahe
	// is unhealthy or returns ErrNotFound.
	gogoanimeBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("anitaku.to", 1.0, 2),
		domain.WithPerHostRPS("vibeplayer.site", 1.0, 2),
		domain.WithPerHostRPS("otakuhg.site", 1.0, 2),
		domain.WithPerHostRPS("otakuvid.online", 1.0, 2),
		domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
	)
	gogoanimeMalsync := gogoanime.NewMalSyncClient(redisCache)
	gogoanimeProvider, err := gogoanime.New(gogoanime.Deps{
		BaseURL: cfg.Gogoanime.BaseURL,
		HTTP:    gogoanimeBaseHTTP,
		Embeds:  registry,
		MalSync: gogoanimeMalsync,
		Cache:   redisCache,
		Log:     log,
	})
	if err != nil {
		log.Fatalw("failed to construct Gogoanime provider", "error", err)
	}
	orchestrator.Register(gogoanimeProvider)
	log.Infow("registered provider", "name", gogoanimeProvider.Name())

	// Phase 17 Plan 01: seed the provider_health_up gauge family with zero-
	// initialized children for every (provider, stage) pair so the HELP/TYPE
	// lines appear in /metrics from boot. The probe runner (Plan 17-02) will
	// overwrite these once it ticks. Without this seed, Prometheus exposition
	// suppresses the metric family until the first observation lands.
	//
	// We initialize Up=1 (the optimistic default) so the orchestrator's
	// nil-cache backcompat path observes a Grafana panel reading 1, matching
	// the "no probe yet, assume healthy" semantic. Plan 17-02 flips this
	// based on real probe data.
	//
	// REVIEW.md WR-01: take ONE snapshot of the registered-providers list
	// and reuse it for both the metric-seed loop and the probe-spawn loop.
	// Taking two snapshots invites a bug where a future maintainer inserts
	// a Register() between the two and the seeded metric set diverges from
	// the spawned probe set.
	providers := orchestrator.RegisteredProviders()
	for _, p := range providers {
		for _, stage := range health.AllStages {
			metrics.ProviderHealthUp.WithLabelValues(p.Name(), stage).Set(1)
		}
		// Heartbeat starts at 0 (never ticked) — the probe sets it on each tick.
		metrics.ProviderProbeLastTick.WithLabelValues(p.Name()).Set(0)
		// Seed parser_zero_match_total with a no-op Add(0) so the HELP/TYPE
		// lines appear in /metrics from boot. The label `selector="_seeded"`
		// is a sentinel that means "no real zero-match event yet" — once
		// parsers start incrementing real selectors (Phase 18+), this sentinel
		// becomes background noise on a single low-value child.
		metrics.ParserZeroMatchTotal.WithLabelValues(p.Name(), "_seeded").Add(0)
	}

	// Phase 17 Plan 02: spawn one liveness probe goroutine per registered
	// provider. IMPORTANT ordering (RESEARCH "Wiring the probe into main.go"):
	//   1. All providers must be Register()-ed first; orchestrator.RegisteredProviders()
	//      enumerates the closed set.
	//   2. Spawn probes BEFORE ListenAndServe so the probe path runs in the
	//      same process as the user-serving path (cookie-jar + HTTP-client
	//      identity guarantee from Phase 15 SCRAPER-FOUND-06).
	//   3. On SIGTERM, probeCancel() is called BEFORE srv.Shutdown(ctx) so
	//      probes drain cleanly without racing the orchestrator teardown.
	//
	// The probe goroutines self-recover from provider panics (defer recover
	// in ProbeRunner.Start). They emit provider_health_up{provider, stage}
	// gauges + provider_probe_last_tick_timestamp{provider} heartbeat on
	// every tick (15 min ± 20% jitter).
	probeCtx, probeCancel := context.WithCancel(context.Background())
	for _, p := range providers {
		runner := health.NewProbeRunner(p, health.DefaultGoldenPool, cache, log)
		go runner.Start(probeCtx)
		log.Infow("scraper.probe: spawned", "provider", p.Name())
	}

	// HTTP handler + router wiring.
	// Phase 17 Plan 03: thread the same in-memory health cache the probe
	// runner writes to so /scraper/health/admin can read the enriched
	// per-stage snapshot.
	scraperHandler := handler.NewScraperHandler(orchestrator, cache, log)
	router := transport.NewRouter(scraperHandler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Phase 18 wiring invariant — fatal if the provider chain didn't grow
	// to 2 (animepahe + gogoanime). A future maintainer dropping one of the
	// orchestrator.Register calls would silently degrade the failover chain
	// to a single provider; this guard surfaces the regression at boot.
	if got, want := len(orchestrator.RegisteredProviders()), 2; got != want {
		log.Fatalw("Phase 18 wiring invariant broken: expected 2 providers (animepahe + gogoanime)",
			"got", got, "want", want)
	}

	go func() {
		log.Infow("scraper service ready",
			"port", cfg.Server.Port,
			"address", cfg.Server.Address(),
			"providers", len(orchestrator.HealthSnapshot(context.Background())),
			"embed_extractors", len(registry.Names()),
			"megacloud_url", cfg.MegacloudExtractor.URL,
			"animepahe_base_url", cfg.AnimePahe.BaseURL,
			"gogoanime_base_url", cfg.Gogoanime.BaseURL,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	// Cancel probe goroutines BEFORE srv.Shutdown so probes drain cleanly
	// without racing the orchestrator teardown (RESEARCH P-07).
	probeCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}

	log.Info("server stopped")
}
