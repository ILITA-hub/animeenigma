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
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animekai"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animepahe"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/eighteenanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/miruro"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/nineanime"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/okru"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/sidecar"
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

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "scraper")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
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

	// NOTE: the Megaplay extractor is constructed AFTER SetGlobalSink (below) so
	// its recording transport composes the egress recorder (WR-07). It is still
	// registered before the gogoanime.Provider constructor (line ~248), which is
	// the only ordering constraint the registry dispatch requires.

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

	// Egress effect producer (AR-EGRESS-01/03). Ships recorded outbound effects
	// to analytics /internal/effects; non-blocking + drop-on-full so an analytics
	// outage never affects scraper resolves. SetGlobalSink MUST run BEFORE the
	// egressTransport is built below, because tracing.WrapTransport snapshots the
	// process-global sink at wrap time to decide whether to compose the recording
	// RoundTripper.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EGRESS-03: the shared recording transport that every per-provider
	// BaseHTTPClient routes its upstream traffic through. tracing.WrapTransport
	// composes the recording RoundTripper because the process-global effect sink
	// (the producer above) is now installed. The per-provider tag
	// (domain.WithProvider) rides each request's context so the recorder can pivot
	// streaming egress by provider+host (D-02/D-09); general (non-streaming)
	// upstream calls still carry their provider since each BaseHTTPClient is
	// single-provider — the streaming-vs-general distinction is made downstream by
	// EffectKind, not here.
	egressTransport := tracing.WrapTransport(nil)

	// megaplayWrap wraps a leaf extractor's transport with the egress recorder
	// (WR-07). Built here, after SetGlobalSink, so tracing.WrapTransport composes
	// the recording RoundTripper. The leaf module stays tracing-free (it takes a
	// func(base) wrapper, mirroring kodikextract.NewRecordingClient).
	megaplayWrap := func(base http.RoundTripper) http.RoundTripper {
		return tracing.WrapTransport(base)
	}

	// Megaplay — the gogoanimes.fi mirror's newplayer.php embed (host
	// gogoanime.me.uk) nests the megaplay.buzz HLS player, so gogoanime resolves
	// streams through this extractor via the shared registry (2026-06-05 revival).
	// nineanime constructs its own instance below; this shared one serves
	// gogoanime. Both are recording-wrapped so the megaplay.buzz / 1anime.site /
	// getSources hops emit egress effects (WR-07).
	megaplayExtractor := embeds.NewRecordingMegaplayExtractor(megaplayWrap)
	registry.Register(megaplayExtractor)
	log.Infow("registered embed extractor", "name", megaplayExtractor.Name())

	// Build the shared HTTP client for AnimePahe.
	//
	// Camoufox revival (2026-06-26): the retired animepahe-resolver sidecar is
	// gone — discovery now hits animepahe.pw through the Camoufox stealth-scraper
	// /fetch route (browser engine). This client backs the engine=http fallback
	// (a curl-class GET of animepahe.pw — challenge-blocked, degraded only) plus
	// the kwik.cx stream-leg + malsync calls still made directly from this
	// process. animepahe.pw is paced low; kwik.cx/kwik.si/api.malsync.moe
	// limits preserved.
	animePaheBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("animepahe.pw", 1.0, 2),
		domain.WithPerHostRPS("kwik.cx", 1.0, 2),
		domain.WithPerHostRPS("kwik.si", 1.0, 2),
		domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
		domain.WithProvider("animepahe"),
		domain.WithTransport(egressTransport),
	)

	// MalSync client (24h positive + 24h negative cache).
	malSyncClient := animepahe.NewMalSyncClient(redisCache)

	// NOTE: the AnimePahe provider is constructed LATER (just before its
	// registration), because its browser-fetch wiring needs `stealthClient` and
	// `breaker`, which are built further down. See animepahe.New(...) below.

	// Build the orchestrator and register the provider before HTTP starts.
	// Phase 17 Plan 02 wires the real health cache that Plan 01 introduced;
	// the orchestrator gates dispatch on cache.IsHealthy and the probe
	// runner (spawned below) populates the cache on each tick.
	cache := health.NewInMemoryHealthCache()

	// Phase 3 (Camoufox self-heal) — per-provider circuit breaker that drives the
	// same health cache the orchestrator skip-gates on. The sidecar-call closures
	// below feed it; >=3 wedged-kind sidecar errors/60s force the provider DOWN so
	// the orchestrator skips it per-request (protecting the healthy provider in
	// real time, not in 15 min).
	breaker := health.NewBreaker(cache)

	orchestrator := service.NewOrchestrator(log, registry, cache)
	// ISS-022: bound per-provider failover time so one hung provider (e.g.
	// animepahe when animepahe.pw is down) cannot consume the whole request
	// budget and starve the chain before failover reaches a healthy provider.
	orchestrator.SetProviderTimeout(cfg.ProviderTimeout)
	log.Infow("per-provider failover budget configured", "timeout", cfg.ProviderTimeout.String())
	// animepahe's stealth-scraper warm session cold-solves a Cloudflare
	// Turnstile challenge (up to 30s) and idle-expires every 600s — far shorter
	// than the 6h health-probe cadence — so it is ALWAYS cold at probe time.
	// The shared 8s ProviderTimeout kills that cold solve before it finishes,
	// marking animepahe perpetually down regardless of real health (see
	// docs/issues/provider-recovery-log.md 2026-07-04). Give it its own budget.
	orchestrator.SetProviderTimeoutOverride("animepahe", cfg.AnimepaheProviderTimeout)
	log.Infow("animepahe per-provider timeout override configured", "timeout", cfg.AnimepaheProviderTimeout.String())

	// Prefer DB-backed provider config from catalog (the runtime source of truth);
	// fall back to the all-enabled offline default already in cfg if catalog is
	// unreachable at boot. cfg.Providers (which gates provider registration below)
	// is authoritative at boot.
	// NOTE: this only re-gates registration at BOOT. Runtime hot enable/disable of the
	// failover roster would require orchestrator re-registration (future work); the
	// periodic refresher currently hot-updates the /health display only.
	if cfg.CatalogURL != "" {
		if pc, err := config.LoadProvidersRemote(context.Background(), cfg.CatalogURL, nil, 5*time.Second); err != nil {
			log.Warnw("remote provider config unavailable; using all-enabled offline fallback", "error", err, "catalog_url", cfg.CatalogURL)
		} else {
			cfg.Providers = pc
			log.Infow("loaded provider config from catalog",
				"source", pc.Source, "disabled", pc.DisabledNames(), "degraded", pc.DegradedNames())
		}
	}

	// registerByStatus registers p into the EN orchestrator according to its DB
	// status (AUTO-484): enabled → auto-failover chain; degraded → registered but
	// EXCLUDED from auto-failover (reachable only via an explicit `prefer` /
	// hacker-mode pin, sorted last in the player); disabled → not registered.
	// candidateProviders is the ordered EN roster of providers THIS BINARY
	// constructs. It is NOT a hand-maintained literal — registerByStatus appends
	// each provider's name as it is constructed+registered below, so the slice is
	// DERIVED from the actual wiring and can never drift from it. It drives the
	// runtime re-gate and the wiring-invariant fatal further down (both run AFTER
	// every registerByStatus call, so the slice is fully populated by then).
	// Registration order == failover order (CONTEXT.md D5). A provider the DB marks
	// disabled is still constructed + passed to registerByStatus (which records the
	// name but skips the orchestrator registration), so it appears here. A CODELESS
	// tombstone like animefever (disabled DB row, no provider code) is NOT
	// constructed, so it is absent here — the Prometheus reflection keys off
	// config.KnownProviders instead so the tombstone still shows on the dashboard.
	var candidateProviders []string

	// registerByStatus registers p into the EN orchestrator according to its DB
	// status (AUTO-484): enabled → auto-failover chain; degraded → registered but
	// EXCLUDED from auto-failover (reachable only via an explicit `prefer` /
	// hacker-mode pin, sorted last in the player); disabled → not registered. In
	// every case p's name joins candidateProviders (the derived EN roster above).
	registerByStatus := func(p domain.Provider) {
		candidateProviders = append(candidateProviders, p.Name())
		meta := cfg.Providers.Meta(p.Name())
		switch cfg.Providers.Status(p.Name()) {
		case config.StatusDisabled:
			log.Warnw("provider SKIPPED (disabled in DB)", "name", p.Name(), "reason", meta.Reason)
		case config.StatusDegraded:
			orchestrator.RegisterDegraded(p)
			log.Infow("registered provider (DEGRADED — manual-only, excluded from auto-failover)",
				"name", p.Name(), "reason", meta.Reason)
		default:
			orchestrator.Register(p)
			log.Infow("registered provider", "name", p.Name())
		}
	}

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
		domain.WithProvider("gogoanime"),
		domain.WithTransport(egressTransport),
	)
	gogoanimeMalsync := gogoanime.NewMalSyncClient(redisCache)

	// Phase 21 SCRAPER-HEAL-03: build the host→extractor-name map +
	// validate SCRAPER_SERVER_PRIORITY against the known-extractor set
	// BEFORE constructing the gogoanime.Provider. Fail-fast on typos so
	// boot logs surface the typo'd name (CONTEXT.md §risks: "Server-
	// priority env typo silently demotes a real server").
	gogoanimeExtractors := []embeds.HostingExtractor{
		vibeplayerExtractor, streamhgExtractor, earnvidsExtractor, megaplayExtractor,
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

	// Camoufox stealth-scraper sidecar — providers whose DB `engine` column is
	// "browser" (e.g. gogoanime → megaplay: JS-runtime stream id + rotating CDN)
	// delegate stream extraction here. engine + base_url are read LIVE from the
	// atomic provider roster (cfg.Providers) so a roster refresh flips behaviour
	// without a restart.
	stealthClient := sidecar.New(cfg.StealthScraperURL, 90*time.Second)
	gogoUseBrowser := func() bool {
		return cfg.Providers.EngineOf("gogoanime") == config.EngineBrowser
	}
	gogoBrowserResolve := func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
		st, err := stealthClient.ResolveEmbed(ctx, "gogoanime", embedURL, category, cfg.Providers.BaseURLOf("gogoanime"))
		// Phase 3 — feed the breaker. IsWedged(nil) returns ("", false), so a
		// success records wedged=false and clears any tripped breaker; a
		// non-wedged error also records false (a challenge is not pool-wedge
		// evidence). Only a wedged-kind sidecar error counts toward the trip.
		_, wedged := sidecar.IsWedged(err)
		breaker.Record("gogoanime", wedged)
		return st, err
	}

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
		Probe:          nil,
		UseBrowser:     gogoUseBrowser,
		BrowserResolve: gogoBrowserResolve,
	})
	if err != nil {
		log.Fatalw("failed to construct Gogoanime provider", "error", err)
	}
	registerByStatus(gogoanimeProvider)

	// AnimePahe — SECOND-CHANCE EN provider. Demoted from primary on
	// 2026-05-13 after gogoanime/Anitaku proved more reliable in real traffic
	// (AnimePahe upstream observed timing out at ~65s on /api?m=release).
	// Kept as the failover so a single-provider outage on Anitaku doesn't
	// blank the English tab.
	//
	// Camoufox revival (2026-06-26): discovery routes through the stealth-scraper
	// /fetch warm session, which solves animepahe.pw's Cloudflare managed
	// (Turnstile) challenge when the DB engine is "browser". Mirrors the
	// nineanime wiring; constructed here (not earlier) so stealthClient + breaker
	// are in scope. engine=http is a degraded fallback only (curl-class GETs of
	// animepahe.pw are challenge-blocked).
	paheUseBrowser := func() bool {
		return cfg.Providers.EngineOf("animepahe") == config.EngineBrowser
	}
	paheBrowserFetch := func(ctx context.Context, provider, url string) (int, []byte, error) {
		status, body, err := stealthClient.Fetch(ctx, provider, url)
		_, wedged := sidecar.IsWedged(err)
		breaker.Record(provider, wedged)
		return status, body, err
	}
	animePaheProvider, err := animepahe.New(animepahe.Deps{
		BaseURL:      cfg.Providers.BaseURLOf("animepahe"),
		HTTP:         animePaheBaseHTTP,
		Embeds:       registry,
		MalSync:      malSyncClient,
		Cache:        redisCache,
		Log:          log,
		UseBrowser:   paheUseBrowser,
		BrowserFetch: paheBrowserFetch,
	})
	if err != nil {
		log.Fatalw("failed to construct AnimePahe provider", "error", err)
	}
	registerByStatus(animePaheProvider)

	// Phase 26 (SCRAPER-HEAL-25) — AllAnime as the THIRD live EN provider.
	// Lifted from services/catalog/internal/parser/allanime/ (copy-with-
	// adaptation per CONTEXT.md D1). Ships ALWAYS-ON — no SCRAPER_ALLANIME_
	// ENABLED gate. Operator can disable/degrade it via the catalog
	// `scraper_providers` DB table if upstream goes hard down. Failover chain order:
	// gogoanime → animepahe → allanime → [animekai gated].
	allAnimeBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("api.allanime.day", 1.0, 2),
		domain.WithPerHostRPS("allmanga.to", 1.0, 2),
		domain.WithProvider("allanime"),
		domain.WithTransport(egressTransport),
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
	registerByStatus(allAnimeProvider)

	// okru — serves AllAnime's Ok (ok.ru) sources clock-free (no api.allanime.day
	// clock endpoint). Registered immediately AFTER allanime so the EN failover
	// order is gogoanime → animepahe → allanime → okru → miruro → …
	// This client backs okru's DISCOVERY only (it wraps an internal allanime
	// provider hitting api.allanime.day). The ok.ru page fetch happens in the
	// embeds.OkruExtractor's own http.Client (mirrors vibeplayer), so a
	// per-host RPS for ok.ru here would be dead config.
	okruBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("api.allanime.day", 1.0, 2),
		domain.WithProvider("okru"),
		domain.WithTransport(egressTransport),
	)
	okruProvider, err := okru.New(okru.Deps{HTTP: okruBaseHTTP, Cache: redisCache, Log: log})
	if err != nil {
		log.Fatalw("failed to construct okru provider", "error", err)
	}
	registerByStatus(okruProvider)

	// animefever (was EN failover slot 4) was REMOVED from the binary 2026-07-05:
	// its upstream is dead for EVERYONE — 100% of HLS segments 302 to a ByteDance
	// ad CDN that 403s, and a residential external A/B proved the content is gone
	// regardless of egress (not IP-class-fixable; falsifies AUTO-484). It survives
	// only as a disabled `scraper_operated` tombstone row in the catalog DB (kept
	// in KnownProviders so the remote-config loader still validates). See the
	// AnimefeverDisable migration + docs/scraper-framework.md.

	// Phase 28 Plan 28-04 (SCRAPER-HEAL-37) — Miruro provider. Failover
	// slot (between allanime/okru and 9anime; animefever removed 2026-07-05).
	// Uses the pure-Go
	// secure-pipe transform from Plan 28-00's obfuscation.go. FindID
	// resolves Shikimori/MAL → AniList via libs/idmapping ARM. Pipe
	// endpoint lives at www.miruro.tv/api/secure/pipe; pro.ultracloud.cc
	// + pru.ultracloud.cc are env2.js VITE_PROXY_A/B fallback hosts we
	// configure for D7 allowlist parity + future failover.
	miruroBaseHTTP := domain.NewBaseHTTPClient(log,
		// T-28-04-07: Cloudflare-fronted host gets half the standard
		// per-host RPS (0.5 / burst 2). Fallback proxies same pacing.
		domain.WithPerHostRPS("www.miruro.tv", 0.5, 2),
		domain.WithPerHostRPS("pro.ultracloud.cc", 0.5, 2),
		domain.WithPerHostRPS("pru.ultracloud.cc", 0.5, 2),
		domain.WithPerHostRPS("arm.haglund.dev", 1.0, 2),
		domain.WithProvider("miruro"),
		domain.WithTransport(egressTransport),
	)
	// AR-EGRESS-03 (host-only, D-08): the miruro ARM/AniList ID-mapping client
	// records egress via the shared recording transport, preserving its
	// IPv4-forced dialer.
	armClient := idmapping.NewClient(
		idmapping.WithTransport(tracing.WrapTransport(idmapping.NewIPv4Transport())),
	)
	// Camoufox migration (2026-07-02): when the DB roster sets miruro engine=
	// "browser", the secure-pipe GET routes through the stealth-scraper /fetch
	// warm session (solves www.miruro.tv's Cloudflare Turnstile; the in-page fetch
	// to /api/secure/pipe then rides cf_clearance). FetchWithHeaders surfaces the
	// x-obfuscated RESPONSE header the Go decoder needs. Mirrors the animepahe
	// wiring; engine=http is a degraded fallback (curl-class GETs are CF-blocked).
	miruroUseBrowser := func() bool {
		return cfg.Providers.EngineOf("miruro") == config.EngineBrowser
	}
	miruroBrowserFetch := func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error) {
		status, headers, body, err := stealthClient.FetchWithHeaders(ctx, provider, url)
		_, wedged := sidecar.IsWedged(err)
		breaker.Record(provider, wedged)
		return status, headers, body, err
	}
	miruroProvider, err := miruro.New(miruro.Deps{
		BaseURL:      cfg.Miruro.BaseURL,
		ProxyURL:     cfg.Miruro.ProxyURL,
		ProxyURLAlt:  cfg.Miruro.ProxyURLAlt,
		HTTP:         miruroBaseHTTP,
		Cache:        redisCache,
		IDMapping:    armClient,
		Log:          log,
		UseBrowser:   miruroUseBrowser,
		BrowserFetch: miruroBrowserFetch,
	})
	if err != nil {
		log.Fatalw("failed to construct Miruro provider", "error", err)
	}
	registerByStatus(miruroProvider)

	// Phase 28 Plan 28-05 (SCRAPER-HEAL-39) — 9anime.me.uk as the LAST-RESORT
	// EN provider (failover slot 6 per CONTEXT.md D5).
	//
	// Per CONTEXT.md D2 — explicitly accepted as low-quality, last-resort.
	// Operator policy "as many providers as possible" overrides the natural
	// "not-worth" verdict. ~6-month half-life expected. Operator kills/degrades it
	// via the catalog `scraper_providers` DB table when upstream rebrands/DMCAs.
	//
	// The legacy my.1anime.site direct-MP4 extraction still happens inline
	// via regex (long-tail catalog). The popular catalog migrated to the
	// megaplay.buzz HLS player (1anime.site wrapper), resolved via the
	// MegaplayExtractor passed below. Both paths coexist in GetStream's host
	// router; rotating megaplay segment CDNs are handled by the streaming
	// proxy's HMAC provenance token, not a per-host allowlist.
	nineAnimeBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("9anime.me.uk", 1.0, 2),
		domain.WithPerHostRPS("my.1anime.site", 1.0, 2),
		domain.WithPerHostRPS("1anime.site", 1.0, 2),
		domain.WithPerHostRPS("megaplay.buzz", 1.0, 2),
		domain.WithProvider("nineanime"),
		domain.WithTransport(egressTransport),
	)
	nineUseBrowser := func() bool {
		return cfg.Providers.EngineOf("nineanime") == config.EngineBrowser
	}
	nineBrowserResolve := func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
		st, err := stealthClient.ResolveEmbed(ctx, "nineanime", embedURL, category, cfg.Providers.BaseURLOf("nineanime"))
		_, wedged := sidecar.IsWedged(err)
		breaker.Record("nineanime", wedged)
		return st, err
	}
	nineBrowserFetch := func(ctx context.Context, provider, url string) (int, []byte, error) {
		status, body, err := stealthClient.Fetch(ctx, provider, url)
		_, wedged := sidecar.IsWedged(err)
		breaker.Record(provider, wedged)
		return status, body, err
	}
	nineAnimeProvider, err := nineanime.New(nineanime.Deps{
		BaseURL:        cfg.NineAnime.BaseURL,
		HTTP:           nineAnimeBaseHTTP,
		Cache:          redisCache,
		Log:            log,
		Megaplay:       embeds.NewRecordingMegaplayExtractor(megaplayWrap), // WR-07: record megaplay egress; nineanime tags provider via ctx
		UseBrowser:     nineUseBrowser,
		BrowserResolve: nineBrowserResolve,
		BrowserFetch:   nineBrowserFetch,
	})
	if err != nil {
		log.Fatalw("failed to construct NineAnime provider", "error", err)
	}
	registerByStatus(nineAnimeProvider)

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
			domain.WithProvider("animekai"),
			domain.WithTransport(egressTransport),
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
		registerByStatus(animeKaiProvider)
	} else {
		log.Infow("AnimeKai provider SKIPPED (flag off)",
			"flag", "SCRAPER_ANIMEKAI_ENABLED=false")
	}

	// 18+ group — a SEPARATE orchestrator (adultOrch) that is NEVER given any
	// EN provider, so 18+ content cannot leak into the OurEnglish failover.
	// Served on the /anime18/* route family. The 18anime provider needs no
	// credentials/sidecar — just the real 18anime.me base.
	adultOrch := service.NewOrchestrator(log, registry, cache)
	adultOrch.SetProviderTimeout(cfg.ProviderTimeout)
	// 18anime uses the shared BaseHTTPClient like every other provider (finding
	// L690): per-host RPS limiter + retry backoff + cookie jar + the egress
	// recording transport, tagged WithProvider so its upstream traffic emits
	// provider-pivotable egress-effect rows (it was previously the sole provider
	// on a bare http.Client emitting ZERO egress rows).
	anime18BaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithProvider("18anime"),
		domain.WithTransport(egressTransport),
	)
	anime18Provider := eighteenanime.New(eighteenanime.Deps{HTTP: anime18BaseHTTP, Cache: redisCache, Log: log})
	if cfg.Providers.IsEnabled(anime18Provider.Name()) {
		adultOrch.Register(anime18Provider)
		log.Infow("registered provider (adult group)", "name", anime18Provider.Name())
	} else {
		log.Warnw("provider SKIPPED (disabled in scraper-providers.yaml)",
			"name", anime18Provider.Name(), "group", config.GroupAdult)
	}

	// Seed parser_zero_match_total with a no-op Add(0) for every registered
	// provider (EN + adult groups) so the HELP/TYPE lines appear in /metrics
	// from boot. The label `selector="_seeded"` is a sentinel that means "no
	// real zero-match event yet" — once parsers start incrementing real
	// selectors (Phase 18+), this sentinel becomes background noise on a
	// single low-value child.
	for _, p := range append(append([]domain.Provider{}, orchestrator.RegisteredProviders()...), adultOrch.RegisteredProviders()...) {
		metrics.ParserZeroMatchTotal.WithLabelValues(p.Name(), "_seeded").Add(0)
	}

	// HTTP handler + router wiring.
	// Phase 17 Plan 03: thread the same in-memory health cache the probe
	// runner writes to so /scraper/health/admin can read the enriched
	// per-stage snapshot.
	// Task 3 (unified player plan): attach the operator ProvidersConfig so
	// GetHealth can surface enabled/reason/description per provider.

	scraperHandler := handler.NewScraperHandler(orchestrator, cache, log)
	scraperHandler.WithProvidersConfig(&cfg.Providers)
	anime18Handler := handler.NewScraperHandler(adultOrch, cache, log)
	anime18Handler.WithProvidersConfig(&cfg.Providers)

	// candidateProviders is now fully populated: registerByStatus appended every
	// constructed EN provider's name in failover order above (gogoanime → animepahe
	// → allanime → okru → miruro → nineanime → animekai-if-enabled). It drives the
	// Phase-3 runtime re-gate, the wiring-invariant fatal, and the Prometheus
	// reflection below. No hand-maintained literal → it cannot drift from the
	// actual registrations. (animefever removed from the binary 2026-07-05.)

	// Phase 3 — runtime re-gate: each catalog refresh moves providers in/out of
	// the orchestrator's degraded failover map without a restart. Only EN-group
	// candidate providers that are REGISTERED (enabled or degraded at boot) are
	// re-gated; disabled providers were never registered and stay restart-gated
	// (D5). The adult orchestrator is intentionally NOT re-gated (single fixed
	// 18+ provider).
	regate := func() {
		statuses := make(map[string]string, len(candidateProviders))
		for _, name := range candidateProviders {
			switch cfg.Providers.Status(name) {
			case config.StatusEnabled:
				statuses[name] = "enabled"
			case config.StatusDegraded:
				statuses[name] = "degraded"
				// StatusDisabled: omitted — never re-gated at runtime (D5).
			}
		}
		orchestrator.ApplyStatuses(statuses)
		log.Infow("orchestrator re-gated from catalog status",
			"degraded", cfg.Providers.DegradedNames(),
			"disabled", cfg.Providers.DisabledNames())
	}

	// Hot-reload provider config from catalog (enable/disable without restart) and
	// re-gate the failover roster after each successful refresh.
	config.StartProvidersRefresher(context.Background(), &cfg.Providers, cfg.CatalogURL, cfg.ProvidersRefresh, log, regate)

	router := transport.NewRouter(scraperHandler, anime18Handler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("scraper")(router),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Phase 19 wiring invariant (narrowed by the accumulator refactor). Since
	// candidateProviders is now derived from the registerByStatus calls, the old
	// literal-vs-registration drift is structurally impossible; this residual check
	// only fires if a provider reached the orchestrator WITHOUT going through
	// registerByStatus (a direct orchestrator.Register bypass → got > want). Cheap
	// boot-time defense-in-depth; kept, not load-bearing.
	expectedProviders := 0
	for _, name := range candidateProviders {
		if cfg.Providers.IsRegistered(name) {
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
		log.Fatalw("Phase 19 wiring invariant broken",
			"got", got, "want", expectedProviders,
			"flag", cfg.AnimeKai.Enabled,
			"disabled", cfg.Providers.DisabledNames(),
			"registered", names)
	}

	// Adult-group wiring invariant — the adult orchestrator holds exactly the
	// enabled adult providers (1 when 18anime is enabled, 0 when disabled).
	adultCandidates := config.KnownProvidersInGroup(config.GroupAdult)
	expectedAdult := 0
	for _, name := range adultCandidates {
		if cfg.Providers.IsEnabled(name) {
			expectedAdult++
		}
	}
	if got := len(adultOrch.RegisteredProviders()); got != expectedAdult {
		log.Fatalw("adult-group wiring invariant broken", "got", got, "want", expectedAdult)
	}

	// ISS-023: reflect the provider-management config into Prometheus so the
	// Grafana dashboard shows EVERY provider (enabled and disabled) with its
	// reason/description. Reflect the full KNOWN roster (config.KnownProviders =
	// every scraper_operated name, incl. the adult group), NOT candidateProviders
	// (which is derived from what this binary actually constructs). That gap
	// matters for a codeless tombstone like animefever: it has a disabled DB row
	// but no provider code, so it never joins candidateProviders — keying the
	// reflection off KnownProviders keeps its row on the dashboard as intended.
	for _, row := range cfg.Providers.Rows(config.KnownProviders) {
		enabled := 0.0
		if row.Enabled {
			enabled = 1.0
		}
		metrics.ProviderEnabled.WithLabelValues(row.Name).Set(enabled)
		metrics.ProviderInfo.WithLabelValues(row.Name, string(row.Status), row.Reason, row.Description).Set(1)
	}
	log.Infow("provider management config loaded",
		"source", cfg.Providers.Source,
		"disabled", cfg.Providers.DisabledNames(),
	)

	go func() {
		log.Infow("scraper service ready",
			"port", cfg.Server.Port,
			"address", cfg.Server.Address(),
			"providers", len(orchestrator.HealthSnapshot(context.Background())),
			"embed_extractors", len(registry.Names()),
			"megacloud_url", cfg.MegacloudExtractor.URL,
			"animepahe_base_url", cfg.Providers.BaseURLOf("animepahe"),
			"animepahe_engine", cfg.Providers.EngineOf("animepahe"),
			"gogoanime_base_url", cfg.Gogoanime.BaseURL,
			"animekai_enabled", cfg.AnimeKai.Enabled,
			"animekai_base_url", cfg.AnimeKai.BaseURL,
			"nineanime_base_url", cfg.NineAnime.BaseURL,
		)
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
