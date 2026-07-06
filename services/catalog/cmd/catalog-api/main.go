package main

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/jimaku"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/prometheus"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/telegram"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/scraperprovider"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/cards"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight/client"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/subprobe"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the service down.
	var tracer *tracing.Tracer
	tracer, err = tracing.InitFromEnv(context.Background(), "catalog")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	// Initialize database (auto-creates DB if not exists)
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}

	// Start DB pool metrics collector
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	// One-time migration: rename scraper_providers → stream_providers (roster
	// unification 2026-06-17). MUST run before AutoMigrate so the new
	// scraper_operated column lands on the renamed, data-carrying table rather
	// than a fresh empty one. Guarded + idempotent (no-op on fresh DBs).
	if err := scraperprovider.RenameScraperProvidersTable(db.DB); err != nil {
		log.Fatalw("rename scraper_providers -> stream_providers failed", "error", err)
	}

	// Auto-migrate schema. GORM AutoMigrate is the AUTHORITATIVE schema source
	// for this service — the services/catalog/migrations/*.sql files were dead
	// code (no golang-migrate runner ever existed; they targeted a singular
	// `anime` table that AutoMigrate never owns) and were removed. (audit L389)
	if err := db.AutoMigrate(
		&domain.Anime{},
		&domain.Genre{},
		&domain.Video{},
		&domain.PinnedTranslation{},
		// Phase 12 (REC-SIG-05) — S5 attribute schema additions per Decision §A1.
		// AnimeTag is the explicit join model for the anime <-> tag m2m so
		// AnimeTag.Rank is preserved (Decision §A4 — v2.1 rank-weighted TF-IDF).
		&domain.Studio{},
		&domain.Tag{},
		&domain.AnimeTag{},
		// Phase 17 (UX-33) — admin-curated editorial collections.
		&domain.Collection{},
		&domain.CollectionItem{},
		// Scraper provider config + capability traits (spec 2026-06-15).
		&domain.ScraperProvider{},
		&domain.Character{},
		&domain.AnimeCharacter{},
		// Secret-feature roulette admin on/off overrides (seed for future RBAC).
		&domain.SecretFeatureFlag{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Phase 12 — the libs/database wrapper's AutoMigrate ADDs missing
	// columns on existing tables but never creates m2m join tables for
	// relations added to a pre-existing struct (anime_studios was missed
	// on the first Phase-12 redeploy because Anime already existed). Fall
	// through to GORM's native AutoMigrate on Anime to register the new
	// Studios m2m. Idempotent — does nothing on subsequent boots.
	if err := db.DB.AutoMigrate(&domain.Anime{}); err != nil {
		log.Fatalw("failed to migrate Anime m2m relations", "error", err)
	}

	// Phase 12 — register the explicit AnimeTag join model so GORM
	// associations preserve Rank. Required AFTER AutoMigrate so the
	// underlying table already exists.
	if err := db.SetupJoinTable(&domain.Anime{}, "Tags", &domain.AnimeTag{}); err != nil {
		log.Fatalw("failed to register AnimeTag join model", "error", err)
	}

	// Punctuation-insensitive Search (handler → repo.Search) filters on
	// regexp_replace(lower(name), '[^[:alnum:]]+', '', 'g') LIKE '%q%' across
	// name/name_ru/name_jp, executed twice per request (Count + Find). A plain
	// b-tree can't serve that expression, so the query seq-scans the whole
	// `animes` table. GORM AutoMigrate cannot express GIN/expression indexes, so
	// create pg_trgm GIN expression indexes here — idempotent, Postgres-only, and
	// non-fatal (search still works unindexed if creation fails). (audit L382)
	if db.DB.Dialector.Name() == "postgres" {
		searchIdxStmts := []string{
			`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
			`CREATE INDEX IF NOT EXISTS idx_animes_name_trgm ON animes USING gin (regexp_replace(lower(name), '[^[:alnum:]]+', '', 'g') gin_trgm_ops)`,
			`CREATE INDEX IF NOT EXISTS idx_animes_name_ru_trgm ON animes USING gin (regexp_replace(lower(name_ru), '[^[:alnum:]]+', '', 'g') gin_trgm_ops)`,
			`CREATE INDEX IF NOT EXISTS idx_animes_name_jp_trgm ON animes USING gin (regexp_replace(lower(name_jp), '[^[:alnum:]]+', '', 'g') gin_trgm_ops)`,
		}
		for _, stmt := range searchIdxStmts {
			if err := db.DB.Exec(stmt).Error; err != nil {
				log.Warnw("failed to create search trigram index (non-fatal)", "error", err, "stmt", stmt)
			}
		}
	}

	// One-time migration: scraper_providers.enabled (bool) → status enum
	// (enabled|degraded|disabled), AUTO-484. AutoMigrate above added `status` with
	// default 'enabled', so existing disabled rows (enabled=false) must be
	// backfilled BEFORE the old column is dropped. Guarded by column presence so
	// it is a no-op on fresh DBs and on every boot after the column is gone.
	if db.DB.Migrator().HasColumn(&domain.ScraperProvider{}, "enabled") {
		if err := db.DB.Exec(
			`UPDATE stream_providers SET status = CASE WHEN enabled THEN 'enabled' ELSE 'disabled' END`,
		).Error; err != nil {
			log.Fatalw("backfill stream_providers.status from enabled failed", "error", err)
		}
		if err := db.DB.Migrator().DropColumn(&domain.ScraperProvider{}, "enabled"); err != nil {
			log.Warnw("drop legacy scraper_providers.enabled column failed (non-fatal)", "error", err)
		} else {
			log.Infow("migrated stream_providers.enabled bool → status enum")
		}
	}

	// Split the legacy "kodik" row into "kodik-iframe" (un-probeable embed) +
	// "kodik-noads" (scraped HLS). MUST run BEFORE SeedDefaults so the rename
	// wins before the insert-if-absent seed could add a duplicate kodik-iframe.
	if err := scraperprovider.SplitKodik(db.DB); err != nil {
		log.Errorw("split-kodik migration failed (continuing)", "error", err)
	}

	// Bootstrap stream_providers from the Go-embedded default roster
	// (insert-if-absent; the DB is the SINGLE source of truth — the YAML was
	// retired 2026-06-17, AUTO-484). Idempotent: operator edits are never
	// overwritten.
	if err := scraperprovider.SeedDefaults(db.DB); err != nil {
		log.Errorw("scraper provider seed failed (continuing)", "error", err)
	} else {
		log.Infow("scraper provider defaults seeded")
	}

	// One-time enablement: route the EXISTING gogoanime row through the Camoufox
	// stealth-scraper sidecar (engine=browser + gogoanimes.fi base). AutoMigrate
	// added `engine` with default 'http', so a pre-existing gogoanime row needs
	// flipping; fresh DBs already get engine=browser from the seed above, so the
	// filter matches nothing there. Idempotent — once engine='browser' it is a
	// no-op. Uses the model (TableName-aware) so it targets the roster table
	// regardless of the scraper_providers↔stream_providers rename. Non-fatal so a
	// migration hiccup never blocks catalog boot.
	if res := db.DB.Model(&domain.ScraperProvider{}).
		Where("name = ? AND engine <> ?", "gogoanime", "browser").
		Updates(map[string]any{"engine": "browser", "base_url": "https://gogoanimes.fi"}); res.Error != nil {
		log.Warnw("gogoanime engine→browser migration failed (non-fatal)", "error", res.Error)
	} else if res.RowsAffected > 0 {
		log.Infow("migrated gogoanime provider to engine=browser (stealth-scraper)")
	}

	// Backfill the intrinsic scraper_operated flag on every roster row (idempotent;
	// the flag mirrors Group — intrinsic, not operator-editable). Ensures pre-
	// existing rows (which AutoMigrate created the column on as default-false) and
	// any operator-touched rows reflect the canonical scraper-operated set.
	if err := scraperprovider.BackfillScraperOperated(db.DB); err != nil {
		log.Errorw("backfill scraper_operated failed (continuing)", "error", err)
	}

	// One-time (guarded) retirement of the hanime + animelib roster rows (Plan B:
	// those player surfaces are retired, content dropped). Run-once via the
	// catalog_migration_guards ledger table, so a later operator re-enable in
	// the DB is never clobbered.
	if err := scraperprovider.RetireHanimeAnimelib(db.DB); err != nil {
		log.Errorw("retire hanime+animelib failed (continuing)", "error", err)
	}

	// Forward-only: hanime was retired in Plan B (2026-06-18) but restored as an
	// in-aePlayer 18+ source (2026-06-19). MUST run after RetireHanimeAnimelib so
	// it wins the final status on fresh DBs. Run-once guarded; an operator who
	// later re-disables it is never clobbered.
	if err := scraperprovider.ReEnableHanime(db.DB); err != nil {
		log.Errorw("re-enable hanime failed (continuing)", "error", err)
	}

	// One-time (guarded) flip of miruro to supports_sub=false: its upstream stopped
	// serving sub streams (only English dub plays), so it must not advertise/auto-
	// select for SUB (original-Japanese-audio) playback. Run-once via the ledger;
	// a later operator re-enable in the DB is never clobbered.
	if err := scraperprovider.MiruroDubOnly(db.DB); err != nil {
		log.Errorw("miruro dub-only migration failed (continuing)", "error", err)
	}

	// One-time (guarded) refresh of the miruro roster description: 2026-07-02
	// daily-recovery finding that www.miruro.tv now sits behind a Cloudflare WAF
	// block on every path (T-28-04-01 reappeared). Description-only — reason is
	// probe-managed and status/policy/health stay with the self-healing machine.
	if err := scraperprovider.MiruroCloudflareBlock(db.DB); err != nil {
		log.Errorw("miruro cloudflare-block migration failed (continuing)", "error", err)
	}

	// One-time (guarded) revival of miruro through the Camoufox stealth-scraper
	// roster after the 2026-07-02 Cloudflare block: engine=browser, seeded DEGRADED
	// (policy=manual/health=down → WireStatus=degraded). A Phase-0 spike proved the
	// homepage-Turnstile solve unblocks the /api/secure/pipe WORLD-A path. Mirrors
	// AnimepaheBrowserRevival; never clobbers a later operator change.
	if err := scraperprovider.MiruroBrowserRevival(db.DB); err != nil {
		log.Errorw("miruro browser-revival migration failed (continuing)", "error", err)
	}

	// Remove unverified "Region-walled" / egress-IP-class claims from animefever
	// description (AUTO-484 follow-up). Run-once via the ledger.
	if err := scraperprovider.AnimefeverDeclaim(db.DB); err != nil {
		log.Errorw("animefever declaim migration failed (continuing)", "error", err)
	}

	// One-time (guarded) flip of allanime to status=degraded: its stream leg is
	// dead (sources decode to /apivtwo/clock.json behind a Cloudflare Turnstile,
	// unsolvable from our egress); the clock-free ok.ru sources are served by the
	// new 'okru' provider. The seed ships degraded for fresh DBs only; this carries
	// the flip to existing live DBs. Run-once via the ledger; a later operator
	// re-enable in the DB is never clobbered.
	if err := scraperprovider.AllAnimeDegrade(db.DB); err != nil {
		log.Errorw("allanime degrade migration failed (continuing)", "error", err)
	}

	// One-time (guarded) refresh of the animepahe roster row: its dedicated
	// animepahe-resolver stealth-Chromium sidecar was retired 2026-06-24. animepahe
	// stays OFF (disabled) but is KEPT in the roster + candidateProviders so it can be
	// revived later; this only updates the reason/description so the row stops citing a
	// sidecar that no longer exists. Run-once via the ledger; a later operator re-enable
	// in the DB is never clobbered.
	if err := scraperprovider.AnimepaheSidecarRetired(db.DB); err != nil {
		log.Errorw("animepahe sidecar-retired migration failed (continuing)", "error", err)
	}

	// One-time (guarded) durable DISABLE of animefever (dead for everyone —
	// residential A/B proved the content is gone, not egress-fixable). The seed
	// was degraded and AnimefeverDeclaim only refreshed reason/description, so
	// this flips any DB still on degraded → disabled. Provider code removed from
	// the scraper binary; the tombstone row is kept. Operator re-enable never clobbered.
	if err := scraperprovider.AnimefeverDisable(db.DB); err != nil {
		log.Errorw("animefever disable migration failed (continuing)", "error", err)
	}

	// One-time (guarded) REVIVAL of animepahe onto the Camoufox stealth-scraper
	// sidecar (engine=browser, base_url=https://animepahe.pw, disabled→degraded).
	// animepahe.pw's Cloudflare managed challenge is solvable from our own IP via
	// the warm /fetch Turnstile solver — superseding the "0% solve" retirement
	// above. MUST run AFTER AnimepaheSidecarRetired so its degraded/browser flip
	// wins on a fresh DB. Run-once via the ledger; a later operator revert is
	// never clobbered.
	if err := scraperprovider.AnimepaheBrowserRevival(db.DB); err != nil {
		log.Errorw("animepahe browser revival migration failed (continuing)", "error", err)
	}

	// One-time (guarded) flip of nineanime to engine=browser + base_url:
	// 9anime.me.uk is DDoS-Guard/JS-gated and its megaplay player resolves the
	// stream id + rotating CDN at runtime, so it must be scraped via the Camoufox
	// stealth-scraper sidecar. The seed ships engine=browser for fresh DBs only;
	// this carries the flip to existing live DBs. Run-once via the ledger; a later
	// operator revert in the DB is never clobbered.
	if err := scraperprovider.NineanimeBrowser(db.DB); err != nil {
		log.Errorw("nineanime browser migration failed (continuing)", "error", err)
	}

	// One-time (guarded) INSERT of the two catalog-side animejoy RU-sub provider
	// rows (animejoy-sibnet + animejoy-allvideo) IF ABSENT. animejoy itself is the
	// shared discovery base, not a row. The seed covers fresh DBs (insert-if-absent);
	// this carries the rows to existing live DBs (server IS prod). The insert sets
	// group='ru' + scraper_operated=false EXPLICITLY (intrinsicGroup stamping does
	// not run for a raw migration), keeping these catalog-operated RU rows out of
	// the EN scraper-failover chain. Seeded DEGRADED (soak). Run-once via the ledger;
	// a later operator edit/delete is never clobbered. Rows stay dormant until the
	// capability family + FE adapter ship in a later phase.
	if err := scraperprovider.AddAnimejoyProviders(db.DB); err != nil {
		log.Errorw("add-animejoy-providers migration failed (continuing)", "error", err)
	}

	// Hard-delete the retired standalone "raw" JP provider row (removed 2026-06-30).
	// The seed no longer creates it, but insert-if-absent never deletes the existing
	// prod row, and the Grafana roster reads stream_providers directly — so the stale
	// row must be removed explicitly. Run-once via the ledger; idempotent.
	if err := scraperprovider.RemoveRawProvider(db.DB); err != nil {
		log.Errorw("remove-raw-provider migration failed (continuing)", "error", err)
	}

	// One-time (guarded) back-fill of the legacy status tri-state onto the new
	// (policy, health) dimensions: enabled→auto/up, degraded→manual/down,
	// disabled→disabled/down. Run-once via the catalog_migration_guards ledger;
	// later machine/operator writes to policy or health are never clobbered.
	if err := scraperprovider.BackfillPolicyHealth(db.DB); err != nil {
		log.Errorw("policy/health backfill failed (continuing)", "error", err)
	}

	// Reflect the catalog-owned provider rows (scraper_operated=false) into the
	// provider_info/provider_enabled management metrics. Runs after the roster is
	// fully migrated/seeded/backfilled so names + flags are authoritative. The
	// scraper emits its own (scraper_operated=true) rows — the two partition the
	// roster with no duplicate series.
	if err := scraperprovider.EmitCatalogSideRoster(db.DB); err != nil {
		log.Errorw("emit catalog-side provider roster metrics failed (continuing)", "error", err)
	}

	// Seed the provider_state gauge for the FULL roster (every provider's derived
	// (policy, health) lifecycle state) so the dashboard's "Provider State History"
	// timeline has a baseline band for never-probed rows. The probe-result handler
	// re-sets each provider's gauge live on every transition. Catalog is the sole
	// emitter of provider_state.
	if err := scraperprovider.EmitProviderStates(db.DB); err != nil {
		log.Errorw("emit provider state metrics failed (continuing)", "error", err)
	}

	// Initialize cache
	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	// Egress effect producer (AR-EGRESS-01/03). The producer ships recorded
	// outbound effects to analytics /internal/effects; it is non-blocking +
	// drop-on-full so an analytics outage never affects catalog requests.
	// SetGlobalSink lights up tracing.WrapTransport's recording composition
	// process-wide, so the OpenSubtitles / idmapping / Kodik retrofits (02-02)
	// begin emitting effect rows with no further per-client wiring.
	analyticsURL := os.Getenv("ANALYTICS_INTERNAL_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics:8092"
	}
	effectProducer := tracing.NewProducer(tracing.ProducerConfig{AnalyticsURL: analyticsURL})
	effectProducer.Start()
	defer effectProducer.Stop()
	tracing.SetGlobalSink(effectProducer)

	// AR-EFFECT-01 (D-15): wire the GORM db_write (always) + db_read (P95-gated)
	// effect callbacks + the daily-P95 ReadGate refresher (D-03). The sink is the
	// Producer just installed; the existing redisCache doubles as the HashReader
	// for the read_thresholds snapshot. dbEffectsStop halts the refresher on
	// shutdown.
	dbEffectsCtx, dbEffectsCancel := context.WithCancel(context.Background())
	defer dbEffectsCancel()
	dbEffectsStop, err := gormtrace.WireDBEffects(dbEffectsCtx, db.DB, tracing.GlobalSink(), redisCache)
	if err != nil {
		log.Warnw("db-effect callbacks disabled", "error", err)
	}
	defer dbEffectsStop()

	// AR-EFFECT-02 (D-17): catalog is the cache-effect service. Attach the cache
	// hit/miss aggregator to the shared RedisCache so Get/Set outcomes flush as
	// summed `cache` effect rows keyed by key-class. The aggregator MUST flush
	// BEFORE the producer drains so the final summed rows are shipped (mirror
	// streaming's HLSSessions ordering). defers run LIFO, so registering this
	// Stop AFTER effectProducer.Stop() above guarantees cacheAggregator.Stop()
	// (flush) runs first, then the producer drains. gateway is intentionally N/A
	// (its rate-limit cache uses a raw redis.Client, not libs/cache.RedisCache —
	// the aggregator's only hook seam); rooms/watch-together are skipped too.
	cacheAggregator := cache.NewCacheAggregator(tracing.GlobalSink(), 0, 0)
	redisCache.WithAggregator(cacheAggregator)
	cacheAggregator.Start()
	defer cacheAggregator.Stop()

	// Initialize external parsers
	shikimoriClient := shikimori.NewClient(cfg.Shikimori, log)

	// Initialize repositories
	animeRepo := repo.NewAnimeRepository(db.DB)
	genreRepo := repo.NewGenreRepository(db.DB)
	videoRepo := repo.NewVideoRepository(db.DB)
	characterRepo := repo.NewCharacterRepository(db.DB)
	// Phase 17 (UX-33) — editorial collections repo.
	collectionRepo := repo.NewCollectionRepository(db.DB)
	// Secret-feature roulette admin config store.
	secretFeatureRepo := repo.NewSecretFeatureRepository(db.DB)

	// Initialize services
	catalogService := service.NewCatalogService(
		animeRepo,
		genreRepo,
		videoRepo,
		shikimoriClient,
		redisCache,
		log,
		service.CatalogServiceOptions{
			JimakuAPIKey:   cfg.Jimaku.APIKey,
			AnimeLibToken:  cfg.AnimeLib.Token,
			HanimeEmail:    cfg.Hanime.Email,
			HanimePassword: cfg.Hanime.Password,
			ScraperAPIURL:  cfg.Scraper.APIURL,
			ScraperTimeout: cfg.Scraper.Timeout,
			// AR-EGRESS-03 (D-08): the internal idmapping client + Kodik
			// extractor record egress via the shared recording transport.
			EgressTransportWrap: tracing.WrapTransport,
		},
	)

	characterService := service.NewCharacterService(animeRepo, characterRepo, shikimoriClient, redisCache, log)

	// Initialize external clients
	telegramClient := telegram.NewClient(cfg.Telegram.NewsChannel)

	// Initialize handlers
	catalogHandler := handler.NewCatalogHandler(catalogService, log)
	characterHandler := handler.NewCharacterHandler(characterService, log)
	adminHandler := handler.NewAdminHandler(catalogService, log)
	newsHandler := handler.NewNewsHandler(telegramClient, redisCache, log)
	// Phase 17 (UX-33) — editorial collections service + handler.
	collectionService := service.NewCollectionService(collectionRepo, log)
	collectionHandler := handler.NewCollectionHandler(collectionService, log)
	// Secret-feature roulette management (admin) + public state.
	secretFeatureService := service.NewSecretFeatureService(secretFeatureRepo, log)
	secretFeatureHandler := handler.NewSecretFeatureHandler(secretFeatureService, log)
	// Phase 18 (UX-34) — skip-intro/skip-outro proxy of aniskip.com.
	// Stateless handler with embedded http.Client + shared redis cache.
	skipTimesHandler := handler.NewSkipTimesHandler(redisCache, log)

	// First-party ("ae") provider — serves JP/EN/RU strictly from the self-hosted
	// library (MinIO HLS ladder); nil-safe — an unreachable / unconfigured library
	// client makes it report no episodes. The handler mounts
	// /api/anime/{id}/ae/{episodes,stream}. (The standalone JP "raw" provider this
	// resolver also fronted was removed 2026-06-30 — AllAnime + ok.ru cover
	// JP-original now.)
	libraryClient := library.NewClient(library.Config{
		APIURL:  cfg.Library.APIURL,
		Timeout: cfg.Library.Timeout,
	})
	libraryResolver := service.NewRawResolver(libraryClient, animeRepo, redisCache, log)
	aeHandler := handler.NewAeHandler(libraryResolver, log)

	// Start Kodik + ae library liveness probes (reports via shared provider-health
	// metrics). Constructed AFTER libraryClient so the ae probe can be wired in.
	healthChecker := service.NewPlayerHealthChecker(
		catalogService.KodikClient(),
		cfg.HealthCheck.Interval,
		log,
		libraryClient,
	)
	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	go healthChecker.Start(healthCtx)

	// Workstream raw-jp, Phase 06 — internal cache-invalidation
	// endpoint POSTed by the library encoder after every successful
	// encode. Mounted OUTSIDE /api (no AuthMiddleware) — reachable
	// only from within the docker network because nginx/gateway
	// does not proxy /internal/*.
	internalCacheHandler := handler.NewInternalCacheHandler(redisCache, animeRepo, log)

	// Workstream notifications, Phase 2 — internal latest-episode
	// lookup endpoint consumed by the notifications detector. Same
	// gateway-non-routing security model as InvalidateRaw above.
	// Wraps kodik/animelib parser adapters behind a 5-min Redis cache
	// keyed by the literal notifications:episodes:{shikimori}:{player}:
	// {translation_id}:{watch_type} pattern (NOTIF-DET-01).
	episodesLookupService := service.NewEpisodesLookupService(
		redisCache,
		catalogService.KodikClient(),
		catalogService.AnimeLibClient(),
		animeRepo,
		catalogService,
		libraryResolver,
		log,
	)
	internalEpisodesHandler := handler.NewInternalEpisodesHandler(episodesLookupService, log)

	// Watch-Together workstream, Phase 04 — WT-STATE-02. Sibling of the
	// notifications-detector episodes endpoint above. Validates a
	// (player, episode_id, translation_id) tuple for the WT service's
	// state:change_{episode,player,translation} inbound handlers. The
	// kodik/animelib path reuses episodesLookupService (D-04 "smallest
	// change"); ourenglish/hanime/raw/aeplayer ship permissive v1.0 validation.
	episodesValidateService := service.NewEpisodesValidateService(episodesLookupService, animeRepo, log)
	internalEpisodesValidateHandler := handler.NewInternalEpisodesValidateHandler(episodesValidateService, log)

	// Scraper provider config + capability traits (spec 2026-06-15).
	// Serves GET /internal/scraper/providers — consumed by the scraper
	// microservice at boot + on a refresh interval. Same gateway-non-routing
	// security model as the other /internal/* endpoints above.
	internalScraperProvidersHandler := handler.NewInternalScraperProvidersHandler(db.DB, log)

	// Probe-result endpoint (Task 6 / self-healing Phase 3). Receives a
	// pass/fail verdict from the scheduler/scraper probe and drives the
	// provider policy state machine (ApplyVerdict), persisting the result.
	// Same gateway-non-routing security model as peers above.
	internalProviderPolicyHandler := handler.NewInternalProviderPolicyHandler(db.DB, cfg.ProviderPolicy, log)
	// ae playback-probe target set: newest distinct-anime library uploads mapped
	// to catalog UUIDs (library client + anime repo).
	internalProbeHandler := handler.NewInternalProbeHandler(libraryClient, animeRepo, log)

	// Anidle guess-game pool (spec 2026-06-15) — Docker-network only.
	// Serves GET /internal/guessgame/pool for the anidle guessing-game service.
	guessPoolService := service.NewGuessPoolService(animeRepo, shikimoriClient, log)
	internalGuessPoolHandler := handler.NewInternalGuessPoolHandler(guessPoolService, log)

	// Workstream raw-jp, Phase 02 — multi-provider subtitle aggregator.
	// Fans out to Jimaku (JP) + OpenSubtitles (everything else, keyed by
	// IMDb/TMDB) and merges results. Mounts /api/anime/{id}/subtitles[/all].
	jimakuClient := jimaku.NewClient(cfg.Jimaku.APIKey)
	// AR-EGRESS-03 (D-08, host-only): route OpenSubtitles + idmapping outbound
	// through the shared recording transport. tracing.WrapTransport composes the
	// recording RoundTripper only when a process-global sink is installed
	// (SetGlobalSink at BE boot, wired in the general-egress plan); until then
	// it is a nil-safe pass-through, so this is safe to wire now.
	openSubsClient := opensubtitles.NewClient(opensubtitles.Config{
		APIKey:    cfg.OpenSubtitles.APIKey,
		UserAgent: cfg.OpenSubtitles.UserAgent,
		Timeout:   cfg.OpenSubtitles.Timeout,
		Logger:    log,
		Transport: tracing.WrapTransport(nil),
	})
	// anime365 (smotret-anime) — Russian fansubs, no key. Spec 2026-06-24.
	anime365Client := anime365.NewClient(anime365.Config{
		BaseURL:   cfg.Anime365.BaseURL,
		Enabled:   cfg.Anime365.Enabled,
		Timeout:   8 * time.Second,
		Transport: tracing.WrapTransport(nil),
	})
	// Wrap idmapping's IPv4-forced transport (preserve the dialer; add recording).
	idMapClient := idmapping.NewClient(
		idmapping.WithTransport(tracing.WrapTransport(idmapping.NewIPv4Transport())),
	)
	// Active subtitle-provider probe (subprobe): pings the configured providers
	// on a scheduler-fired cron and records up/degraded/down verdicts the
	// aggregator overlays as provider_health. staleAfter = 15m (3× the 5-min
	// cron) so a missed run downgrades to "unknown" rather than a stale "up".
	subHealthStore := subprobe.NewStore(15 * time.Minute)
	subPingers := map[string]subprobe.Pinger{}
	if jimakuClient.IsConfigured() {
		subPingers["jimaku"] = jimakuClient
	}
	if openSubsClient.IsConfigured() {
		subPingers["opensubtitles"] = openSubsClient
	}
	if anime365Client.IsConfigured() {
		subPingers["anime365"] = anime365Client
	}
	subtitleProbe := subprobe.New(subHealthStore, subPingers, 2*time.Second, 8*time.Second, log)
	internalSubtitleProbeHandler := handler.NewInternalSubtitleProbeHandler(subtitleProbe, log)
	subsAggregator := service.NewSubsAggregator(jimakuClient, openSubsClient, anime365Client, idMapClient, animeRepo, redisCache, subHealthStore, log)
	subtitlesHandler := handler.NewSubtitlesHandler(subsAggregator, log)

	// Workstream hero-spotlight, v1.0 Phase 3 (Plan 03-04) — hero spotlight
	// aggregator wired with all 9 resolvers:
	//
	//   Static cards (Phase 1):
	//     1. featured            — curated-pin or daily pick (cached 24h)
	//     2. random_tail         — random anime from the bottom of the catalog
	//     3. latest_news         — newest changelog entries via web client
	//     4. platform_stats      — global counters from postgres
	//
	//   Dynamic cards (Phase 3 — Plan 03-03):
	//     5. personal_pick       — trending (anon) or recs (login) via player
	//     6. telegram_news       — channel posts via telegram parser
	//     7. now_watching        — live watch_progress snapshot via postgres
	//     8. not_time_yet        — login-only; planned/postponed list w/ aired eps
	//     9. continue_watching_new — login-only; new aired ep since last view
	//
	// HSB-BE-03 per-card 800ms / HSB-BE-04 overall 2s deadlines applied
	// inside the aggregator. Endpoint mounted at /api/home/spotlight behind
	// OptionalAuthMiddleware (Plan 03-04 Task 2 — see router.go); flag-gated
	// by cfg.SpotlightEnabled (HSB-BE-07).
	//
	// Shared *rand.Rand drives the HSB-BE-30 N=2 random-pick branch of
	// AdaptiveSlice across 4 resolvers (personal_pick / latest_news /
	// now_watching / telegram_news). Time-seeded so the daily rotation is
	// non-deterministic across reboots without being predictable.
	spotlightWebClient := client.NewWebClient("", nil)
	spotlightPlayerClient := client.NewPlayerClient("", nil, log)
	spotlightRng := rand.New(rand.NewSource(time.Now().UnixNano()))
	prometheusClient := prometheus.NewClient(cfg.Prometheus.URL)
	// random_tail kept in a named var — the handler's reroll route
	// (v4 B-1 «Ещё разок») needs direct access beside the aggregator.
	randomTailResolver := cards.NewRandomTailResolver(animeRepo, redisCache, log)
	spotlightResolvers := []spotlight.Resolver{
		// Static cards (Phase 1)
		cards.NewFeaturedResolver(animeRepo, redisCache, log),
		randomTailResolver,
		cards.NewLatestNewsResolver(spotlightWebClient, redisCache, spotlightRng, log),
		cards.NewPlatformStatsResolver(prometheusClient, redisCache, log),
		// Dynamic cards (Phase 3 — Plan 03-03)
		cards.NewPersonalPickResolver(catalogService, spotlightPlayerClient, redisCache, spotlightRng, log),
		cards.NewTelegramNewsResolver(telegramClient, redisCache, spotlightRng, log),
		cards.NewNowWatchingResolver(cards.NewGormNowWatchingAdapter(db.DB), redisCache, spotlightRng, log),
		cards.NewNotTimeYetResolver(spotlightPlayerClient, redisCache, spotlightRng, log),
		cards.NewContinueWatchingNewResolver(spotlightPlayerClient, redisCache, spotlightRng, log),
	}
	spotlightAggregator := spotlight.NewAggregator(redisCache, log, spotlightResolvers)
	spotlightHandler := handler.NewSpotlightHandler(spotlightAggregator, cfg.SpotlightEnabled, log)
	spotlightHandler.SetReroller(randomTailResolver)

	// Ranked capability report (spec 2026-06-15 P4).
	// ScraperHealth adapts catalogService.GetScraperHealth (which forwards to
	// the scraper microservice at cfg.Scraper.APIURL) as a HealthSource.
	// aeLibraryAdapter backs the first-party `ae` family's library-presence
	// lookup via the raw resolver's library episode index.
	capSvc := capability.NewService(db.DB, capability.NewScraperHealth(catalogService), catalogService, redisCache, log, aeLibraryAdapter{r: libraryResolver})
	capabilitiesHandler := handler.NewCapabilitiesHandler(capSvc, log)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("catalog")

	// Initialize router
	router := transport.NewRouter(catalogHandler, characterHandler, adminHandler, newsHandler, collectionHandler, skipTimesHandler, aeHandler, subtitlesHandler, internalCacheHandler, internalEpisodesHandler, internalEpisodesValidateHandler, internalScraperProvidersHandler, internalProbeHandler, internalSubtitleProbeHandler, spotlightHandler, internalGuessPoolHandler, capabilitiesHandler, internalProviderPolicyHandler, secretFeatureHandler, cfg, log, metricsCollector)

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("catalog")(router),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting catalog service", "address", cfg.Server.Address())
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

// aeLibraryAdapter satisfies capability.LibrarySource: the first-party `ae`
// family is `active` (selectable) only when the title is self-hosted in the
// library. Presence = the raw resolver's library episode index reports
// available episodes for the anime.
type aeLibraryAdapter struct{ r *service.RawResolver }

func (a aeLibraryAdapter) HasLibraryTitle(ctx context.Context, animeID string) (bool, error) {
	resp, err := a.r.GetLibraryEpisodes(ctx, animeID)
	if err != nil {
		return false, err
	}
	return resp.Available && len(resp.Episodes) > 0, nil
}
