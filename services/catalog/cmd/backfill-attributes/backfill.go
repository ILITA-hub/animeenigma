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
// Idempotency contract:
//   - ShikimoriHalf walks animes WHERE (kind='' OR rating='' OR
//     material_source='' OR no anime_studios row exists). Re-runs only
//     touch rows still missing one of those — already-populated rows are
//     skipped at the SQL level (no Shikimori fetch).
//   - AnilistHalf walks animes WHERE no anime_tags row exists. An anime
//     that already has any anime_tags row is skipped — including ranks,
//     so v2.1 rank-weighted TF-IDF will need a separate force-refresh
//     pass. Documented in CONTEXT.md §Backfill Strategy.
//
// Failure semantics: per-anime errors are logged + counted but never
// abort the run. Each anime is its own transaction so a partial DB write
// rolls back cleanly. The job is re-runnable: re-running picks up rows
// that failed last time.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	loggerlib "github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist"
)

// defaultRetryWaits is the production exponential-backoff schedule for
// HTTP 429 / "Too Many Requests" responses from Shikimori or AniList:
// 5s -> 15s -> 60s. Up to 3 retries per fetch (4 attempts total). On the
// 4th failure the row is counted as failed and the loop continues —
// matches the plan's "continue on individual failures" contract.
//
// Decision driver (Plan 12-02 deviation, Option A): the first production
// run populated ~1080 rows before Shikimori started returning 429 in
// bursts. The operator chose backoff over reducing RPS because most
// minutes the 3 RPS budget is fine; only short bursts trip the limiter.
var defaultRetryWaits = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	60 * time.Second,
}

// is429Error reports whether err looks like a rate-limit response from
// Shikimori or AniList. Both surface the limit through different layers:
//   - Shikimori GraphQL — wraps an error with body "Too Many Requests"
//     or "Retry later" — no HTTP status code in the message.
//   - HTTP 429 — explicit numeric status surfaced when the GraphQL layer
//     decodes the response body or fails before that.
//
// We accept a substring match so a wrapped errors.ExternalAPI cause is
// caught regardless of how many layers wrap the original.
func is429Error(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "retry later")
}

// Consumer-side interfaces — kept small so tests can mock them.
type shikimoriFetcher interface {
	GetAnimeByID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
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
	Limit         int // 0 = no limit
	SkipShikimori bool
	SkipTags      bool
	LogEvery      int // default 100
	// RetryWaits overrides the default 429 backoff schedule. Empty/nil
	// uses defaultRetryWaits (5s/15s/60s). Tests pass compressed values
	// (e.g. 1ms/3ms/5ms) to keep the suite fast.
	RetryWaits []time.Duration
}

// Result types — kept separate so the operator log can break out the two
// halves' progress independently in the final summary line.
type ShikimoriHalfResult struct {
	Succeeded, Skipped, Failed int
}

type AnilistHalfResult struct {
	Succeeded, SkippedNoAnilist, SkippedAlreadyDone, Failed int
}

// BackfillRunner orchestrates the two-phase Phase-12 schema backfill.
type BackfillRunner struct {
	db        *gorm.DB
	shikimori shikimoriFetcher
	anilist   anilistFetcher
	arm       armResolver
	log       *loggerlib.Logger
	cfg       Config
}

func NewBackfillRunner(db *gorm.DB, sh shikimoriFetcher, al anilistFetcher, arm armResolver, log *loggerlib.Logger, cfg Config) *BackfillRunner {
	if cfg.LogEvery <= 0 {
		cfg.LogEvery = 100
	}
	if len(cfg.RetryWaits) == 0 {
		cfg.RetryWaits = defaultRetryWaits
	}
	return &BackfillRunner{db: db, shikimori: sh, anilist: al, arm: arm, log: log, cfg: cfg}
}

// fetchShikimoriWithBackoff calls r.shikimori.GetAnimeByID and retries
// on 429 / "Too Many Requests" / "Retry later" responses with the
// schedule in r.cfg.RetryWaits (default 5s/15s/60s, up to 3 retries).
//
// Non-429 errors short-circuit and are returned to the caller — they
// reflect bugs (DNS, parse, schema mismatch) that retrying won't fix.
//
// Context cancellation aborts the wait between retries.
func (r *BackfillRunner) fetchShikimoriWithBackoff(ctx context.Context, animeID, shikimoriID string) (*domain.Anime, error) {
	var lastErr error
	for attempt := 0; attempt <= len(r.cfg.RetryWaits); attempt++ {
		anime, err := r.shikimori.GetAnimeByID(ctx, shikimoriID)
		if err == nil {
			return anime, nil
		}
		lastErr = err
		if !is429Error(err) {
			// Non-rate-limit error: do not retry.
			return nil, err
		}
		if attempt == len(r.cfg.RetryWaits) {
			// Out of retries; surface the last 429.
			break
		}
		wait := r.cfg.RetryWaits[attempt]
		r.log.Warnw("backfill 429 backoff",
			"source", "shikimori",
			"anime_id", animeID,
			"shikimori_id", shikimoriID,
			"attempt", attempt+1,
			"wait_seconds", int(wait.Seconds()),
			"error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

// fetchAnilistTagsWithBackoff mirrors fetchShikimoriWithBackoff for the
// AniList tag fetch. Same 429-detection rules, same backoff schedule.
func (r *BackfillRunner) fetchAnilistTagsWithBackoff(ctx context.Context, animeID string, anilistID int) ([]anilist.Tag, error) {
	var lastErr error
	for attempt := 0; attempt <= len(r.cfg.RetryWaits); attempt++ {
		tags, err := r.anilist.FetchTags(ctx, anilistID)
		if err == nil {
			return tags, nil
		}
		lastErr = err
		if !is429Error(err) {
			return nil, err
		}
		if attempt == len(r.cfg.RetryWaits) {
			break
		}
		wait := r.cfg.RetryWaits[attempt]
		r.log.Warnw("backfill 429 backoff",
			"source", "anilist",
			"anime_id", animeID,
			"anilist_id", anilistID,
			"attempt", attempt+1,
			"wait_seconds", int(wait.Seconds()),
			"error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

// candidate is the minimal projection needed to drive a fetch.
type candidate struct {
	ID          string
	ShikimoriID string
}

// ShikimoriHalf re-fetches every anime missing kind/rating/material_source
// or its studios m2m and writes the new columns + studio relationships.
//
// Algorithm:
//  1. SELECT id, shikimori_id from animes where any of the four signals is empty.
//  2. For each candidate: fetch via Shikimori, UPDATE columns, upsert studios,
//     each in a per-anime transaction. Errors are logged + counted, never abort.
//  3. Progress logged every cfg.LogEvery rows.
//
// Returns nil error always — per-anime failures are reflected in the result
// counts. A non-nil error is returned only for catastrophic startup issues
// (the candidate query itself failing).
func (r *BackfillRunner) ShikimoriHalf(ctx context.Context) (ShikimoriHalfResult, error) {
	var res ShikimoriHalfResult

	// Count rows that are skipped at the SQL level (already populated). The
	// Skipped counter reflects this so operators see the full denominator.
	var totalEligible int64
	if err := r.db.Raw(`SELECT count(*) FROM animes WHERE deleted_at IS NULL AND shikimori_id != ''`).
		Scan(&totalEligible).Error; err != nil {
		return res, fmt.Errorf("count eligible: %w", err)
	}

	// The WHERE clause is a SQL-level idempotency predicate. Rows where ALL
	// four signals are populated are skipped without a Shikimori fetch.
	whereSQL := `
		deleted_at IS NULL
		AND shikimori_id != ''
		AND (
			kind = '' OR kind IS NULL
			OR rating = '' OR rating IS NULL
			OR material_source = '' OR material_source IS NULL
			OR NOT EXISTS (SELECT 1 FROM anime_studios WHERE anime_studios.anime_id = animes.id)
		)
	`

	q := r.db.Table("animes").Select("id, shikimori_id").Where(whereSQL).Order("id")
	if r.cfg.Limit > 0 {
		q = q.Limit(r.cfg.Limit)
	}

	var candidates []candidate
	if err := q.Scan(&candidates).Error; err != nil {
		return res, fmt.Errorf("query candidates: %w", err)
	}

	res.Skipped = int(totalEligible) - len(candidates)
	if res.Skipped < 0 {
		res.Skipped = 0
	}

	r.log.Infow("shikimori-half: starting", "candidates", len(candidates), "already_populated", res.Skipped, "dry_run", r.cfg.DryRun)

	for i, cand := range candidates {
		if err := ctx.Err(); err != nil {
			r.log.Warnw("shikimori-half: ctx cancelled, stopping", "processed", i)
			return res, nil
		}

		anime, err := r.fetchShikimoriWithBackoff(ctx, cand.ID, cand.ShikimoriID)
		if err != nil {
			r.log.Warnw("shikimori-half: fetch failed, continuing",
				"anime_id", cand.ID, "shikimori_id", cand.ShikimoriID, "error", err)
			res.Failed++
			continue
		}

		if r.cfg.DryRun {
			res.Succeeded++
			if (i+1)%r.cfg.LogEvery == 0 {
				r.log.Infow("shikimori-half: progress (dry-run)",
					"processed", i+1, "succeeded", res.Succeeded, "failed", res.Failed)
			}
			continue
		}

		if err := r.applyShikimoriResult(cand.ID, anime); err != nil {
			r.log.Warnw("shikimori-half: db write failed, continuing",
				"anime_id", cand.ID, "shikimori_id", cand.ShikimoriID, "error", err)
			res.Failed++
			continue
		}
		res.Succeeded++

		if (i+1)%r.cfg.LogEvery == 0 {
			r.log.Infow("shikimori-half: progress",
				"processed", i+1, "succeeded", res.Succeeded, "failed", res.Failed)
		}
	}

	r.log.Infow("shikimori-half: complete",
		"succeeded", res.Succeeded, "skipped", res.Skipped, "failed", res.Failed)
	return res, nil
}

// applyShikimoriResult writes the four new fields and upserts studios in
// a single transaction. Per-anime isolation: a failure rolls back this
// anime only.
func (r *BackfillRunner) applyShikimoriResult(animeID string, fresh *domain.Anime) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// UPDATE the four new fields. We do NOT touch other fields (name,
		// score, episodes, etc.) — the freshly-fetched payload may differ
		// but those are owned by the regular catalog refresh path; this
		// backfill is scoped strictly to the Phase-12 attribute schema.
		if err := tx.Exec(`
			UPDATE animes
			SET kind = ?, rating = ?, material_source = ?, updated_at = ?
			WHERE id = ?
		`, fresh.Kind, fresh.Rating, fresh.MaterialSource, now, animeID).Error; err != nil {
			return fmt.Errorf("update anime %s: %w", animeID, err)
		}

		// Upsert each studio + the join row. ON CONFLICT DO NOTHING for
		// the studio (id is shared across anime); ON CONFLICT DO NOTHING
		// for the join (composite PK already enforces uniqueness).
		for _, st := range fresh.Studios {
			if st.ID == "" {
				continue
			}
			studio := domain.Studio{
				ID: st.ID, Name: st.Name,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).Create(&studio).Error; err != nil {
				return fmt.Errorf("upsert studio %s: %w", st.ID, err)
			}

			if err := tx.Exec(`
				INSERT INTO anime_studios (anime_id, studio_id) VALUES (?, ?)
				ON CONFLICT (anime_id, studio_id) DO NOTHING
			`, animeID, st.ID).Error; err != nil {
				return fmt.Errorf("upsert anime_studios %s/%s: %w", animeID, st.ID, err)
			}
		}
		return nil
	})
}

// AnilistHalf walks every anime missing tag rows, resolves Shikimori ->
// AniList via ARM, fetches tags, and upserts Tag rows + anime_tags join
// entries (with rank).
//
// Anime without an AniList mapping (ARM 404 or AniList==nil) are counted
// as SkippedNoAnilist and remain with empty tags — consistent with the
// missing-attribute-equals-zero contract from spec §3.3. They will be
// re-attempted on every backfill run; the cost is minimal because ARM
// returns quickly and AniList is never called for them.
//
// Empty-tag responses (AniList returns []) are counted as Succeeded with
// zero writes; the row will still satisfy NOT EXISTS on the next run and
// re-fetch. This is rare and acceptable.
func (r *BackfillRunner) AnilistHalf(ctx context.Context) (AnilistHalfResult, error) {
	var res AnilistHalfResult

	whereSQL := `
		deleted_at IS NULL
		AND shikimori_id != ''
		AND NOT EXISTS (SELECT 1 FROM anime_tags WHERE anime_tags.anime_id = animes.id)
	`

	q := r.db.Table("animes").Select("id, shikimori_id").Where(whereSQL).Order("id")
	if r.cfg.Limit > 0 {
		q = q.Limit(r.cfg.Limit)
	}

	var candidates []candidate
	if err := q.Scan(&candidates).Error; err != nil {
		return res, fmt.Errorf("query candidates: %w", err)
	}

	r.log.Infow("anilist-half: starting", "candidates", len(candidates), "dry_run", r.cfg.DryRun)

	for i, cand := range candidates {
		if err := ctx.Err(); err != nil {
			r.log.Warnw("anilist-half: ctx cancelled, stopping", "processed", i)
			return res, nil
		}

		mapping, err := r.arm.ResolveByShikimoriID(cand.ShikimoriID)
		if err != nil {
			r.log.Warnw("anilist-half: ARM resolve failed, continuing",
				"anime_id", cand.ID, "shikimori_id", cand.ShikimoriID, "error", err)
			res.Failed++
			continue
		}
		if mapping == nil || mapping.AniList == nil {
			res.SkippedNoAnilist++
			r.log.Debugw("anilist-half: no AniList mapping, skipping tags",
				"anime_id", cand.ID, "shikimori_id", cand.ShikimoriID)
			continue
		}

		tags, err := r.fetchAnilistTagsWithBackoff(ctx, cand.ID, *mapping.AniList)
		if err != nil {
			r.log.Warnw("anilist-half: fetch tags failed, continuing",
				"anime_id", cand.ID, "anilist_id", *mapping.AniList, "error", err)
			res.Failed++
			continue
		}

		if r.cfg.DryRun {
			res.Succeeded++
			if (i+1)%r.cfg.LogEvery == 0 {
				r.log.Infow("anilist-half: progress (dry-run)",
					"processed", i+1, "succeeded", res.Succeeded,
					"skipped_no_anilist", res.SkippedNoAnilist, "failed", res.Failed)
			}
			continue
		}

		if err := r.applyAnilistTags(cand.ID, tags); err != nil {
			r.log.Warnw("anilist-half: db write failed, continuing",
				"anime_id", cand.ID, "anilist_id", *mapping.AniList, "error", err)
			res.Failed++
			continue
		}
		res.Succeeded++

		if (i+1)%r.cfg.LogEvery == 0 {
			r.log.Infow("anilist-half: progress",
				"processed", i+1, "succeeded", res.Succeeded,
				"skipped_no_anilist", res.SkippedNoAnilist, "failed", res.Failed)
		}
	}

	r.log.Infow("anilist-half: complete",
		"succeeded", res.Succeeded,
		"skipped_no_anilist", res.SkippedNoAnilist,
		"skipped_already_done", res.SkippedAlreadyDone,
		"failed", res.Failed)
	return res, nil
}

// applyAnilistTags upserts Tag rows + anime_tags join entries in a
// per-anime transaction. The composite PK on anime_tags (AnimeID, TagID)
// makes the join row insert idempotent across re-runs (we reach this
// path only when the WHERE NOT EXISTS predicate qualified — but a
// concurrent backfill run could in theory race; the upsert handles it).
func (r *BackfillRunner) applyAnilistTags(animeID string, tags []anilist.Tag) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, t := range tags {
			tagID := anilist.SlugifyTagName(t.Name)
			if tagID == "" {
				continue // empty/whitespace name slugifies to "" — skip
			}

			tag := domain.Tag{
				ID: tagID, Name: t.Name, Source: "anilist",
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoNothing: true,
			}).Create(&tag).Error; err != nil {
				return fmt.Errorf("upsert tag %s: %w", tagID, err)
			}

			// On conflict update rank — lets a manual re-run (with a
			// future --force flag) refresh ranks. Today the WHERE NOT
			// EXISTS in AnilistHalf prevents re-fetching, so this branch
			// effectively never updates; it's defensive against races.
			if err := tx.Exec(`
				INSERT INTO anime_tags (anime_id, tag_id, rank, created_at) VALUES (?, ?, ?, ?)
				ON CONFLICT (anime_id, tag_id) DO UPDATE SET rank = EXCLUDED.rank
			`, animeID, tagID, t.Rank, now).Error; err != nil {
				return fmt.Errorf("upsert anime_tags %s/%s: %w", animeID, tagID, err)
			}
		}
		return nil
	})
}
