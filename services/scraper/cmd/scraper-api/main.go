package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animekai"
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

	// Phase 28 (SCRAPER-HEAL-38) — vidstream_vip extractor for AnimeFever.
	// Plain regex against inline `sources: [{"file":"...m3u8"}]` literal —
	// NOT a Dean-Edwards-packer (CONTEXT.md D4 + RESEARCH.md Discretion).
	// MUST be registered BEFORE the AnimeFever provider construction (Plan
	// 28-02) so the embed Registry.Find lookup at GetStream time succeeds.
	vidstreamVipExtractor := embeds.NewVidstreamVipExtractor()
	registry.Register(vidstreamVipExtractor)
	log.Infow("registered embed extractor", "name", vidstreamVipExtractor.Name())

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

	// Build the shared HTTP client for AnimePahe.
	//
	// Phase 27 SCRAPER-HEAL-30: the per-host rate limits for upstream
	// animepahe.{ru,com,si} are GONE — the resolver sidecar owns
	// upstream rate-pacing (its single Chromium worker naturally
	// serializes). The internal `animepahe-resolver` host gets a higher
	// RPS (5/sec, burst 5) since it lives on the docker network and
	// rate-limiting twice (orchestrator + sidecar) would add latency
	// without reducing load on upstream. kwik.cx / kwik.si /
	// api.malsync.moe limits preserved (still hit directly from this
	// process).
	animePaheBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("animepahe-resolver", 5.0, 5),
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
		ResolverURL: cfg.AnimePahe.ResolverURL,
		HTTP:        animePaheBaseHTTP,
		Embeds:      registry,
		MalSync:     malSyncClient,
		Cache:       redisCache,
		Log:         log,
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

	// Gogoanime/Anitaku — PRIMARY EN provider (Phase 18 + 2026-05-13 reorder).
	// Pivoted from "9anime" since the entire 9anime mirror chain is dead per
	// 2026-05-12 research (.planning/phases/18-9anime/18-RESEARCH.md).
	// Backend slug is "gogoanime"; the user-facing display label is "Anitaku".
	// Registration ORDER is the failover ORDER (CONTEXT.md D5) — gogoanime is
	// tried first; animepahe is the second-chance provider when gogoanime is
	// unhealthy or returns ErrNotFound.
	gogoanimeBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("anitaku.to", 1.0, 2),
		domain.WithPerHostRPS("vibeplayer.site", 1.0, 2),
		domain.WithPerHostRPS("otakuhg.site", 1.0, 2),
		domain.WithPerHostRPS("otakuvid.online", 1.0, 2),
		domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
	)
	gogoanimeMalsync := gogoanime.NewMalSyncClient(redisCache)

	// Phase 21 SCRAPER-HEAL-03: build the host→extractor-name map +
	// validate SCRAPER_SERVER_PRIORITY against the known-extractor set
	// BEFORE constructing the gogoanime.Provider. Fail-fast on typos so
	// boot logs surface the typo'd name (CONTEXT.md §risks: "Server-
	// priority env typo silently demotes a real server").
	gogoanimeExtractors := []embeds.HostingExtractor{
		vibeplayerExtractor, streamhgExtractor, earnvidsExtractor,
	}
	hostExtractor := map[string]string{}
	knownExtractorNames := make([]string, 0, len(gogoanimeExtractors))
	for _, ext := range gogoanimeExtractors {
		knownExtractorNames = append(knownExtractorNames, ext.Name())
		for _, h := range ext.Hosts() {
			hostExtractor[strings.ToLower(h)] = ext.Name()
		}
	}
	if err := gogoanime.ValidatePriorityList(cfg.Gogoanime.ServerPriority, knownExtractorNames); err != nil {
		log.Fatalw("invalid SCRAPER_SERVER_PRIORITY", "error", err,
			"configured", cfg.Gogoanime.ServerPriority,
			"known", knownExtractorNames)
	}
	log.Infow("gogoanime server priority configured",
		"priority", cfg.Gogoanime.ServerPriority,
		"known_extractors", knownExtractorNames)

	gogoanimeProvider, err := gogoanime.New(gogoanime.Deps{
		BaseURL:        cfg.Gogoanime.BaseURL,
		HTTP:           gogoanimeBaseHTTP,
		Embeds:         registry,
		MalSync:        gogoanimeMalsync,
		Cache:          redisCache,
		Log:            log,
		ServerPriority: cfg.Gogoanime.ServerPriority,
		HostExtractor:  hostExtractor,
		// Probe nil → New() defaults to libs/streamprobe.Probe.
		Probe: nil,
	})
	if err != nil {
		log.Fatalw("failed to construct Gogoanime provider", "error", err)
	}
	if cfg.DegradedProviders.IsDegraded(gogoanimeProvider.Name()) {
		log.Warnw("provider SKIPPED (degraded via SCRAPER_DEGRADED_PROVIDERS)",
			"name", gogoanimeProvider.Name(),
			"reason", "global kill-switch — upstream confirmed unreachable or platform-rebranded")
	} else {
		orchestrator.Register(gogoanimeProvider)
		log.Infow("registered provider", "name", gogoanimeProvider.Name())
	}

	// AnimePahe — SECOND-CHANCE EN provider. Demoted from primary on
	// 2026-05-13 after gogoanime/Anitaku proved more reliable in real traffic
	// (AnimePahe upstream observed timing out at ~65s on /api?m=release).
	// Kept as the failover so a single-provider outage on Anitaku doesn't
	// blank the English tab.
	if cfg.DegradedProviders.IsDegraded(animePaheProvider.Name()) {
		log.Warnw("provider SKIPPED (degraded via SCRAPER_DEGRADED_PROVIDERS)",
			"name", animePaheProvider.Name(),
			"reason", "global kill-switch — upstream confirmed unreachable or platform-rebranded")
	} else {
		orchestrator.Register(animePaheProvider)
		log.Infow("registered provider", "name", animePaheProvider.Name())
	}

	// Phase 26 (SCRAPER-HEAL-25) — AllAnime as the THIRD live EN provider.
	// Lifted from services/catalog/internal/parser/allanime/ (copy-with-
	// adaptation per CONTEXT.md D1). Ships ALWAYS-ON — no SCRAPER_ALLANIME_
	// ENABLED gate. Operator can disable via SCRAPER_DEGRADED_PROVIDERS=
	// allanime if upstream goes hard down. Failover chain order:
	// gogoanime → animepahe → allanime → [animekai gated].
	allAnimeBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("api.allanime.day", 1.0, 2),
		domain.WithPerHostRPS("allmanga.to", 1.0, 2),
	)
	allAnimeProvider, err := allanime.New(allanime.Deps{
		BaseURL: cfg.AllAnime.BaseURL,
		HTTP:    allAnimeBaseHTTP,
		Cache:   redisCache,
		Log:     log,
	})
	if err != nil {
		log.Fatalw("failed to construct AllAnime provider", "error", err)
	}
	if cfg.DegradedProviders.IsDegraded(allAnimeProvider.Name()) {
		log.Warnw("provider SKIPPED (degraded via SCRAPER_DEGRADED_PROVIDERS)",
			"name", allAnimeProvider.Name(),
			"reason", "global kill-switch")
	} else {
		orchestrator.Register(allAnimeProvider)
		log.Infow("registered provider", "name", allAnimeProvider.Name())
	}

	// Phase 19 — AnimeKai (gated, ESCAPE-HATCH path). Default FALSE in prod.
	// SCRAPER-KAI-05: env-flag toggle; SCRAPER-KAI-06: stub provider returns
	// ErrProviderDown so failover lands on the previous two providers.
	// The provider's Go methods are stubs (escape-hatch); the sidecar route
	// POST /animekai-token returns HTTP 501. SCRAPER-KAI-01..04 + KAI-07 are
	// carried to v3.1 — see .planning/REQUIREMENTS.md and
	// .planning/phases/19-animekai-gated/19-RESEARCH.md for rationale.
	if cfg.AnimeKai.Enabled {
		animeKaiBaseHTTP := domain.NewBaseHTTPClient(log,
			domain.WithPerHostRPS("anikai.to", 1.0, 2),
			domain.WithPerHostRPS("megaup.cc", 1.0, 2),
			domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
		)
		animeKaiProvider, err := animekai.New(animekai.Deps{
			BaseURL: cfg.AnimeKai.BaseURL,
			HTTP:    animeKaiBaseHTTP,
			Embeds:  registry,
			// WR-01: a non-nil malsync client is REQUIRED by animekai.New().
			// The stub never calls Lookup on the success path; the noop here
			// closes the v3.1 fill-in nil-pointer footgun. Replace with
			// animekai.NewMalSyncClient(redisCache) when the fill-in lands.
			MalSync: animekai.NewNoopMalSync(),
			Cache:   redisCache,
			Log:     log,
		})
		if err != nil {
			log.Fatalw("failed to construct AnimeKai provider", "error", err)
		}
		if cfg.DegradedProviders.IsDegraded(animeKaiProvider.Name()) {
			log.Warnw("provider SKIPPED (degraded via SCRAPER_DEGRADED_PROVIDERS)",
				"name", animeKaiProvider.Name(),
				"reason", "global kill-switch")
		} else {
			orchestrator.Register(animeKaiProvider)
			log.Infow("registered provider", "name", animeKaiProvider.Name(),
				"flag", "SCRAPER_ANIMEKAI_ENABLED=true",
				"mode", "escape-hatch-stub")
		}
	} else {
		log.Infow("AnimeKai provider SKIPPED (flag off)",
			"flag", "SCRAPER_ANIMEKAI_ENABLED=false")
	}

	// Phase 17 Plan 01: seed the provider_health_up gauge family with zero-
	// initialized children for every (provider, stage) pair so the HELP/TYPE
	// lines appear in /metrics from boot. The probe runner (Plan 17-02) will
	// overwrite these once it ticks. Without this seed, Prometheus exposition
	// suppresses the metric family until the first observation lands.
	//
	// For most providers we initialize Up=1 (the optimistic default) so the
	// orchestrator's nil-cache backcompat path observes a Grafana panel
	// reading 1, matching the "no probe yet, assume healthy" semantic.
	// Plan 17-02 flips this based on real probe data.
	//
	// Phase 19 (CR-01): escape-hatch providers like AnimeKai MUST be seeded
	// with Up=0 so Grafana NEVER shows a green panel during the ~15 min
	// before the first probe tick fires when the flag is on. This preserves
	// the documented invariant in animekai/client.go and ensures the
	// SCRAPER-OBS-04 alert ("stream_segment=0 for 15 min") can fire during
	// the escape-hatch window — see bootHealthSeedValue() below.
	//
	// REVIEW.md WR-01: take ONE snapshot of the registered-providers list
	// and reuse it for both the metric-seed loop and the probe-spawn loop.
	// Taking two snapshots invites a bug where a future maintainer inserts
	// a Register() between the two and the seeded metric set diverges from
	// the spawned probe set.
	providers := orchestrator.RegisteredProviders()
	for _, p := range providers {
		seedValue := bootHealthSeedValue(p.Name())
		for _, stage := range health.AllStages {
			metrics.ProviderHealthUp.WithLabelValues(p.Name(), stage).Set(seedValue)
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

	// Phase 19 wiring invariant — adapts the Phase 18 invariant to the
	// flag-conditional shape. Candidates are: gogoanime + animepahe
	// (unconditional) and animekai (gated by SCRAPER_ANIMEKAI_ENABLED). Each
	// candidate that is in SCRAPER_DEGRADED_PROVIDERS is intentionally not
	// Register()-ed, so the expected count subtracts those. A future
	// maintainer dropping any Register() call surfaces the regression at boot
	// via this fatal.
	candidateProviders := []string{"gogoanime", "animepahe", "allanime"}
	if cfg.AnimeKai.Enabled {
		candidateProviders = append(candidateProviders, "animekai")
	}
	expectedProviders := 0
	for _, name := range candidateProviders {
		if !cfg.DegradedProviders.IsDegraded(name) {
			expectedProviders++
		}
	}
	registered := orchestrator.RegisteredProviders()
	if got := len(registered); got != expectedProviders {
		// WR-05: include the actual registered provider names so an on-call
		// hitting this fatal sees WHICH providers landed in the orchestrator
		// without having to read source. Mirrors the "error" + context shape
		// of the other Fatalw call sites in this file (e.g. line 116).
		names := make([]string, 0, len(registered))
		for _, p := range registered {
			names = append(names, p.Name())
		}
		degradedNames := make([]string, 0, len(cfg.DegradedProviders.Names))
		for n := range cfg.DegradedProviders.Names {
			degradedNames = append(degradedNames, n)
		}
		log.Fatalw("Phase 19 wiring invariant broken",
			"got", got, "want", expectedProviders,
			"flag", cfg.AnimeKai.Enabled,
			"degraded", degradedNames,
			"registered", names)
	}

	go func() {
		log.Infow("scraper service ready",
			"port", cfg.Server.Port,
			"address", cfg.Server.Address(),
			"providers", len(orchestrator.HealthSnapshot(context.Background())),
			"embed_extractors", len(registry.Names()),
			"megacloud_url", cfg.MegacloudExtractor.URL,
			"animepahe_resolver_url", cfg.AnimePahe.ResolverURL,
			"gogoanime_base_url", cfg.Gogoanime.BaseURL,
			"animekai_enabled", cfg.AnimeKai.Enabled,
			"animekai_base_url", cfg.AnimeKai.BaseURL,
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

// bootHealthSeedValue returns the initial value for the provider_health_up
// gauge children seeded at startup, given the provider's stable name.
//
// Most providers seed Up=1 ("no probe yet, assume healthy"); escape-hatch
// providers must seed Up=0 so Grafana never shows a green panel during the
// ~15 min before the first probe tick fires. AnimeKai is registered as an
// escape-hatch stub in Phase 19 (see animekai/client.go and
// .planning/phases/19-animekai-gated/19-REVIEW.md CR-01).
//
// A name-based decision is an intentional pragmatic shortcut for Phase 19;
// the longer-term refactor is to expose this as an optional method on the
// domain.Provider interface so each provider declares its own boot state.
// When that lands, drop the name switch here and call the interface method.
func bootHealthSeedValue(name string) float64 {
	if name == "animekai" {
		return 0
	}
	return 1
}
