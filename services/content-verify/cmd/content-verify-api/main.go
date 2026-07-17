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
	"github.com/ILITA-hub/animeenigma/libs/tracing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/config"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/prober"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/service"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load failed", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "content-verify")
	if err != nil {
		log.Warnw("tracing init failed", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("redis connect failed", "error", err)
	}
	defer redisCache.Close()

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect failed", "error", err)
	}
	defer db.Close()

	if err := db.AutoMigrate(&domain.ContentVerification{}, &domain.SkipTiming{}, &domain.SkipFingerprint{}); err != nil {
		log.Fatalw("automigrate failed", "error", err)
	}

	collector := metrics.NewCollector("content-verify")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := repo.NewStore(db.DB)
	sig := signals.New(redisCache.Client())
	catClient := catalogclient.New(cfg.CatalogURL, cfg.GatewayURL, nil)
	engine := queue.NewEngine(catClient, sig, store, cfg.ReprobeTTL, cfg.SkipEnabled, log)
	h := handler.NewVerifyHandler(store, sig, engine, log)

	if cfg.WorkerOn {
		if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
			log.Fatalw("workdir create failed", "error", err)
		}
		shedWatcher := cache.NewDegradationWatcher(redisCache, 5*time.Second)
		shedWatcher.Start(ctx)
		runner := prober.NewExecRunner(cfg.PythonPath, cfg.AnalyzersDir)
		pb := prober.New(catClient, cfg.GatewayURL, cfg.FFmpegPath, cfg.WorkDir, runner, log)
		skipCfg := prober.SkipConfig{
			HeadWindow: cfg.SkipHeadWindow, TailWindow: cfg.SkipTailWindow,
			MinMatch: cfg.SkipMinMatch, MaxMatch: cfg.SkipMaxMatch, SimThreshold: cfg.SkipSimThreshold,
		}
		skipPb := prober.NewSkipProber(catClient, cfg.GatewayURL, cfg.FFmpegPath, cfg.WorkDir, runner, store, skipCfg, log)
		worker := service.NewWorker(cfg.Interval, cfg.UnitBudget, shedWatcher, engine, pb, store, skipPb, store, cfg.SkipBudget, log)
		worker.Start(ctx)
		log.Infow("content-verify worker started", "interval", cfg.Interval, "budget", cfg.UnitBudget)
	}

	router := transport.NewRouter(h, log, collector)
	srv := &http.Server{Addr: cfg.Server.Address(), Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Infow("content-verify listening", "addr", cfg.Server.Address())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalw("server error", "error", err)
	}
}
