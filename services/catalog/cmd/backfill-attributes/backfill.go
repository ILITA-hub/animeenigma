// Package main implements the Phase-12 (REC-SIG-05) attribute backfill —
// a one-shot CLI that re-fetches Shikimori metadata for every anime row
// missing the Wave-1 schema additions and fetches AniList tags via the
// libs/idmapping ARM resolver.
//
// Plan-12-02 location deviation (Rule 3 — blocking): the plan placed
// this binary at services/maintenance/cmd/backfill-attributes/, but Go's
// internal-package visibility rule blocks any module outside
// services/catalog/ from importing services/catalog/internal/{domain,
// parser/...}. Relocating into services/catalog/cmd/backfill-attributes/
// is the canonical Go layout for tools that consume a service's internal
// packages — it mirrors services/catalog/cmd/catalog-api/.
//
// Stub: implementation lands in TDD GREEN.
package main

import (
	"context"

	"gorm.io/gorm"

	loggerlib "github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	catalogdomain "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist"
)

// Consumer-side interfaces — kept small so tests can mock them.
type shikimoriFetcher interface {
	GetAnimeByID(ctx context.Context, shikimoriID string) (*catalogdomain.Anime, error)
}

type anilistFetcher interface {
	FetchTags(ctx context.Context, anilistID int) ([]anilist.Tag, error)
}

type armResolver interface {
	ResolveByShikimoriID(id string) (*idmapping.MappingResult, error)
}

// Config carries flags forwarded from main.go's CLI parser into the runner.
type Config struct {
	DryRun        bool
	Limit         int
	SkipShikimori bool
	SkipTags      bool
	LogEvery      int
}

// Result types — kept separate so the operator log can break out the two
// halves' progress independently in the final summary line.
type ShikimoriHalfResult struct {
	Succeeded, Skipped, Failed int
}

type AnilistHalfResult struct {
	Succeeded, SkippedNoAnilist, SkippedAlreadyDone, Failed int
}

// BackfillRunner — STUB. Real implementation in GREEN.
type BackfillRunner struct {
	db        *gorm.DB
	shikimori shikimoriFetcher
	anilist   anilistFetcher
	arm       armResolver
	log       *loggerlib.Logger
	cfg       Config
}

func NewBackfillRunner(db *gorm.DB, sh shikimoriFetcher, al anilistFetcher, arm armResolver, log *loggerlib.Logger, cfg Config) *BackfillRunner {
	return &BackfillRunner{db: db, shikimori: sh, anilist: al, arm: arm, log: log, cfg: cfg}
}

func (r *BackfillRunner) ShikimoriHalf(_ context.Context) (ShikimoriHalfResult, error) {
	return ShikimoriHalfResult{}, nil // STUB — fails tests
}

func (r *BackfillRunner) AnilistHalf(_ context.Context) (AnilistHalfResult, error) {
	return AnilistHalfResult{}, nil // STUB — fails tests
}
