// Phase-12 (REC-SIG-05) attribute backfill CLI — host-native one-shot
// binary. Reads DB credentials from the standard env vars used by every
// other service (DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME) and runs
// both halves of the BackfillRunner against production data.
//
// See the package doc on backfill.go for idempotency / failure semantics.
//
// Flags:
//
//	--dry-run         log what would be written without writing
//	--limit N         cap rows per half (0 = no limit)
//	--skip-shikimori  run only the AniList half
//	--skip-tags       run only the Shikimori half
//	--log-every N     progress log frequency (default 100)
//	--shikimori-rps N Shikimori rate limit override (default 3)
//
// Exit codes:
//
//	0  — graceful completion, even with per-anime fetch failures (counts in summary)
//	1  — startup failure (DB connect, flag parse)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	loggerlib "github.com/ILITA-hub/animeenigma/libs/logger"
	catalogconfig "github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
)

type cliConfig struct {
	DryRun        bool
	Limit         int
	SkipShikimori bool
	SkipTags      bool
	LogEvery      int
	ShikimoriRPS  int
}

func parseFlags() cliConfig {
	var c cliConfig
	flag.BoolVar(&c.DryRun, "dry-run", false, "log what would be written without writing")
	flag.IntVar(&c.Limit, "limit", 0, "maximum rows to process per half (0 = no limit)")
	flag.BoolVar(&c.SkipShikimori, "skip-shikimori", false, "skip the Shikimori half (kind/rating/material_source/studios)")
	flag.BoolVar(&c.SkipTags, "skip-tags", false, "skip the AniList tag-fetch half")
	flag.IntVar(&c.LogEvery, "log-every", 100, "log progress every N rows")
	flag.IntVar(&c.ShikimoriRPS, "shikimori-rps", 3, "Shikimori rate limit in req/sec")
	flag.Parse()
	return c
}

func main() {
	cli := parseFlags()

	log, err := loggerlib.New(loggerlib.Config{
		Level:    getenv("LOG_LEVEL", "info"),
		Encoding: "console",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	// DB env vars — match the env shape used by every other service.
	dbCfg := database.Config{
		Host:     getenv("DB_HOST", "localhost"),
		Port:     getenvInt("DB_PORT", 5432),
		User:     getenv("DB_USER", "postgres"),
		Password: getenv("DB_PASSWORD", "postgres"),
		Database: getenv("DB_NAME", "animeenigma"),
		SSLMode:  getenv("DB_SSLMODE", "disable"),
	}
	db, err := database.New(dbCfg)
	if err != nil {
		log.Fatalw("failed to connect to db",
			"error", err, "host", dbCfg.Host, "port", dbCfg.Port, "db", dbCfg.Database)
	}
	defer db.Close()

	shikimoriCfg := catalogconfig.ShikimoriConfig{
		BaseURL:    getenv("SHIKIMORI_BASE_URL", "https://shikimori.io"),
		GraphQLURL: getenv("SHIKIMORI_GRAPHQL_URL", "https://shikimori.io/api/graphql"),
		UserAgent:  getenv("SHIKIMORI_USER_AGENT", "AnimeEnigma/1.0 (https://animeenigma.ru)"),
		Timeout:    10 * time.Second,
		RateLimit:  cli.ShikimoriRPS,
	}
	shikimoriClient := shikimori.NewClient(shikimoriCfg, log)
	anilistClient := anilist.NewClient(log)
	armClient := idmapping.NewClient()

	runner := NewBackfillRunner(db.DB, shikimoriClient, anilistClient, armClient, log, Config{
		DryRun:        cli.DryRun,
		Limit:         cli.Limit,
		SkipShikimori: cli.SkipShikimori,
		SkipTags:      cli.SkipTags,
		LogEvery:      cli.LogEvery,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful Ctrl-C: cancel ctx so the in-flight per-anime loop can
	// finish its current iteration and stop cleanly between rows.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Warnw("shutdown signal received; cancelling backfill")
		cancel()
	}()

	started := time.Now()

	var shResult ShikimoriHalfResult
	if !cli.SkipShikimori {
		log.Infow("starting Shikimori half (kind/rating/material_source/studios)")
		r, err := runner.ShikimoriHalf(ctx)
		if err != nil {
			log.Errorw("Shikimori half returned error (run continues)", "error", err)
		}
		shResult = r
	} else {
		log.Infow("skipping Shikimori half (--skip-shikimori)")
	}

	var alResult AnilistHalfResult
	if !cli.SkipTags {
		log.Infow("starting AniList half (tags)")
		r, err := runner.AnilistHalf(ctx)
		if err != nil {
			log.Errorw("AniList half returned error (run continues)", "error", err)
		}
		alResult = r
	} else {
		log.Infow("skipping AniList half (--skip-tags)")
	}

	duration := time.Since(started)

	log.Infow("backfill complete",
		"duration_seconds", int(duration.Seconds()),
		"shikimori_succeeded", shResult.Succeeded,
		"shikimori_skipped", shResult.Skipped,
		"shikimori_failed", shResult.Failed,
		"anilist_succeeded", alResult.Succeeded,
		"anilist_skipped_no_anilist", alResult.SkippedNoAnilist,
		"anilist_skipped_already_done", alResult.SkippedAlreadyDone,
		"anilist_failed", alResult.Failed,
		"dry_run", cli.DryRun,
	)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
