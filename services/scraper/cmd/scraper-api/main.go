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
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animefever"
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
		domain.WithProvider("animepahe"),
		domain.WithTransport(egressTransport),
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
	// ISS-022: bound per-provider failover time so one hung provider (e.g.
	// animepahe when animepahe.pw is down) cannot consume the whole request
	// budget and starve the chain before failover reaches a healthy provider.
	orchestrator.SetProviderTimeout(cfg.ProviderTimeout)
	log.Infow("per-provider failover budget configured", "timeout", cfg.ProviderTimeout.String())

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
	registerByStatus := func(p domain.Provider) {
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
		return stealthClient.ResolveEmbed(ctx, "gogoanime", embedURL, category, cfg.Providers.BaseURLOf("gogoanime"))
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
	// order is gogoanime → animepahe → allanime → okru → animefever → …
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

	// Phase 28 (SCRAPER-HEAL-36) — AnimeFever as the FOURTH live EN provider.
	// Failover slot 4 per CONTEXT.md D5. Always-on; operator disables/degrades it
	// via the catalog `scraper_providers` DB table if upstream breaks.
	// HTML-scrape + AJAX-POST data path; embed extraction delegated to the
	// vidstream_vip extractor registered in Plan 28-03 (sibling Wave 1 plan).
	animeFeverBaseHTTP := domain.NewBaseHTTPClient(log,
		domain.WithPerHostRPS("animefever.cc", 1.0, 2),
		domain.WithPerHostRPS("am.vidstream.vip", 1.0, 2),
		domain.WithPerHostRPS("static-cdn-ca1.mofl.pro", 2.0, 4),
		domain.WithProvider("animefever"),
		domain.WithTransport(egressTransport),
	)
	animeFeverProvider, err := animefever.New(animefever.Deps{
		BaseURL: cfg.AnimeFever.BaseURL,
		HTTP:    animeFeverBaseHTTP,
		Embeds:  registry,
		Cache:   redisCache,
		Log:     log,
	})
	if err != nil {
		log.Fatalw("failed to construct AnimeFever provider", "error", err)
	}
	registerByStatus(animeFeverProvider)

	// Phase 28 Plan 28-04 (SCRAPER-HEAL-37) — Miruro provider. Failover
	// slot 5 (between allanime/animefever and 9anime). Uses the pure-Go
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
	miruroProvider, err := miruro.New(miruro.Deps{
		BaseURL:     cfg.Miruro.BaseURL,
		ProxyURL:    cfg.Miruro.ProxyURL,
		ProxyURLAlt: cfg.Miruro.ProxyURLAlt,
		HTTP:        miruroBaseHTTP,
		Cache:       redisCache,
		IDMapping:   armClient,
		Log:         log,
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
		return stealthClient.ResolveEmbed(ctx, "nineanime", embedURL, category, cfg.Providers.BaseURLOf("nineanime"))
	}
	nineBrowserFetch := func(ctx context.Context, provider, url string) (int, []byte, error) {
		return stealthClient.Fetch(ctx, provider, url)
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
	// credentials/sidecar — a plain HTTP client + the real 18anime.me base.
	adultOrch := service.NewOrchestrator(log, registry, cache)
	adultOrch.SetProviderTimeout(cfg.ProviderTimeout)
	anime18Provider := eighteenanime.New(eighteenanime.Deps{Log: log})
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

	// Hot-reload provider config from catalog (enable/disable without restart).
	config.StartProvidersRefresher(context.Background(), &cfg.Providers, cfg.CatalogURL, cfg.ProvidersRefresh, log)

	router := transport.NewRouter(scraperHandler, anime18Handler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("scraper")(router),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Phase 19 wiring invariant — adapts the Phase 18 invariant to the
	// flag-conditional shape. Candidates are: gogoanime + animepahe
	// (unconditional) and animekai (gated by SCRAPER_ANIMEKAI_ENABLED). Each
	// candidate the DB marks `disabled` is intentionally not registered, so the
	// expected count counts only the registered (enabled OR degraded) ones. A
	// future maintainer dropping any Register() call surfaces the regression at
	// boot via this fatal.
	// Phase 28: order per CONTEXT.md D5 register order — gogoanime → animepahe
	// → allanime → animefever (28-02) → miruro (28-04) → nineanime (28-05).
	candidateProviders := []string{"gogoanime", "animepahe", "allanime", "animefever", "miruro", "nineanime"}
	if cfg.AnimeKai.Enabled {
		candidateProviders = append(candidateProviders, "animekai")
	}
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
	// reason/description. Disabled providers are not Register()-ed, so without
	// this they would vanish from all metrics. Includes the adult group so
	// 18anime appears in the provider-management dashboard.
	for _, row := range cfg.Providers.Rows(append(append([]string{}, candidateProviders...), adultCandidates...)) {
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
			"animepahe_resolver_url", cfg.AnimePahe.ResolverURL,
			"gogoanime_base_url", cfg.Gogoanime.BaseURL,
			"animekai_enabled", cfg.AnimeKai.Enabled,
			"animekai_base_url", cfg.AnimeKai.BaseURL,
			"animefever_base_url", cfg.AnimeFever.BaseURL,
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

