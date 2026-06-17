package repo

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// EpisodeRepository handles persistence for library_episodes rows.
// Mirrors the Phase-3 JobRepository style: context-first, wrap GORM
// errors via liberrors so the HTTP layer can map cleanly to status codes.
type EpisodeRepository struct {
	db *gorm.DB
}

// NewEpisodeRepository constructs an EpisodeRepository over the
// provided *gorm.DB.
func NewEpisodeRepository(db *gorm.DB) *EpisodeRepository {
	return &EpisodeRepository{db: db}
}

// Create inserts a new episode row. On UNIQUE constraint violation
// (duplicate shikimori_id + episode_number) returns
// liberrors.AlreadyExists. Other errors wrap Internal.
func (r *EpisodeRepository) Create(ctx context.Context, ep *domain.Episode) error {
	if ep == nil {
		return liberrors.InvalidInput("episode is nil")
	}
	err := r.db.WithContext(ctx).Create(ep).Error
	if err == nil {
		return nil
	}
	// pgx surfaces the unique-violation as a string we can match on.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "library_episodes_shikimori_ep_uniq") {
		return liberrors.AlreadyExists("episode")
	}
	return liberrors.Wrap(err, liberrors.CodeInternal, "create episode")
}

// GetByShikimoriEpisode returns the episode row matching the
// (shikimori_id, episode_number) pair, or liberrors.NotFound if no
// such row exists.
func (r *EpisodeRepository) GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int) (*domain.Episode, error) {
	var ep domain.Episode
	err := r.db.WithContext(ctx).
		Where("shikimori_id = ? AND episode_number = ?", shikimoriID, episodeNumber).
		First(&ep).Error
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, liberrors.NotFound("episode")
	}
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "fetch episode")
	}
	return &ep, nil
}

// List returns every episode for a given shikimori_id ordered by
// episode_number ASC so the admin UI / hybrid resolver can render a
// linear episode list.
func (r *EpisodeRepository) List(ctx context.Context, shikimoriID string) ([]domain.Episode, error) {
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("shikimori_id = ?", shikimoriID).
		Order("episode_number ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list episodes")
	}
	return eps, nil
}

// UpdateMinioPath repoints a single episode row's minio_path prefix. Used by
// the Phase-7 one-time admin-content migrator (autocache.Migrator) AFTER the
// MinIO objects have been server-side-Moved into the new aeProvider/<mal>/RAW/
// layout — so the row is only repointed once the copy has already succeeded.
// Scoped to the given id (a single-column Updates), returns nil on success.
func (r *EpisodeRepository) UpdateMinioPath(ctx context.Context, id string, path string) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("id = ?", id).
		Update("minio_path", path).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "update episode minio_path")
	}
	return nil
}

// BumpFetch sets last_fetch_at=now() and increments fetch_count for the
// (shikimori_id, episode_number) row — the "viewed by any user" freshness +
// popularity signal the ae serve HIT path fires (SERVE-02). The increment uses
// gorm.Expr so it is a single atomic SQL statement (no read-modify-write race).
//
// It is a NO-OP (nil error) when no row matches: an absent row is a legitimate
// empty state, and fetch_count is a popularity counter (not money) so
// at-least-once / a zero RowsAffected is acceptable (CONTEXT line 59/101). It
// therefore does NOT use First/NotFound — a bare scoped Updates is intentional.
func (r *EpisodeRepository) BumpFetch(ctx context.Context, malID string, episode int) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("shikimori_id = ? AND episode_number = ?", malID, episode).
		Updates(map[string]any{
			"last_fetch_at": gorm.Expr("now()"),
			"fetch_count":   gorm.Expr("fetch_count + 1"),
		}).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "bump episode fetch")
	}
	return nil
}

// ListAdminLegacyPath returns the episode rows still on the legacy
// "{shikimori_id}/{ep}/" layout — i.e. whose minio_path does NOT start with
// the unified-pool "aeProvider/" prefix. Rows already migrated are excluded,
// which is what makes the Phase-7 migrator idempotent + restart-safe: a re-run
// (or a reboot mid-migration) simply re-lists whatever has not yet been
// repointed. Ordered by created_at ASC for deterministic processing. The LIKE
// pattern is a fixed literal (no user input), so it is GORM-safe inline.
func (r *EpisodeRepository) ListAdminLegacyPath(ctx context.Context) ([]domain.Episode, error) {
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("minio_path NOT LIKE 'aeProvider/%'").
		Order("created_at ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list admin legacy-path episodes")
	}
	return eps, nil
}

// SumPoolBytes returns Σ size_bytes over the unified first-party aeProvider/
// pool (admin + autocache, since both live under the same prefix). This is the
// numerator of the EVICT-01 budget check (Σ ≤ autocache_config.budget_bytes).
//
// An empty pool — or a pool whose rows all have NULL size_bytes — returns 0
// (not an error): SUM over zero rows is SQL NULL, which COALESCE folds to 0 so
// the budget math never has to special-case "no episodes yet". The LIKE pattern
// is a fixed literal (no user input), so it is GORM-safe inline.
func (r *EpisodeRepository) SumPoolBytes(ctx context.Context) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("minio_path LIKE 'aeProvider/%'").
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&total).Error; err != nil {
		return 0, liberrors.Wrap(err, liberrors.CodeInternal, "sum pool bytes")
	}
	return total, nil
}

// ListStaleEvictionCandidates returns ONLY the Stale rows in the aeProvider/
// pool, ordered in the locked EVICT-03 4-tier eviction order (oldest-first
// within each tier). The Evictor (Plan 02) deletes from the top of this list
// until enough room is freed; Fresh rows are excluded here so they are NEVER
// eligible for eviction.
//
// Freshness (evaluated at `now`) is source-branched (CONTEXT §decisions):
//   - autocache Fresh ⟺ downloaded_at > now-auto_fresh_download_days
//     OR last_fetch_at > now-auto_fresh_fetch_days
//   - admin     Fresh ⟺ downloaded_at > now-admin_fresh_days
//     OR last_fetch_at > now-admin_fresh_days
//
// A row is Stale ⟺ NOT Fresh. NULL downloaded_at is treated as "very old"
// (the `downloaded_at > cutoff` term is simply false for NULL in SQL, so the
// row classifies by last_fetch_at only); NULL last_fetch_at = never-fetched
// (its term is likewise false). The day-window cutoffs are computed in Go and
// passed as BOUND `?` params — never string-interpolated into the SQL (T-10-01).
//
// Ordering tiers (CASE):
//  1. autocache · never-fetched (last_fetch_at IS NULL)
//  2. autocache · fetched
//  3. admin · never-fetched
//  4. admin · fetched
//
// within a tier: COALESCE(last_fetch_at, downloaded_at, created_at) ASC
// (oldest-touched first; created_at is the always-present final fallback).
func (r *EpisodeRepository) ListStaleEvictionCandidates(ctx context.Context, cfg *domain.AutocacheConfig, now time.Time) ([]domain.Episode, error) {
	if cfg == nil {
		return nil, liberrors.InvalidInput("autocache config is nil")
	}

	autoDownloadCutoff := now.AddDate(0, 0, -cfg.AutoFreshDownloadDays)
	autoFetchCutoff := now.AddDate(0, 0, -cfg.AutoFreshFetchDays)
	adminCutoff := now.AddDate(0, 0, -cfg.AdminFreshDays)

	// NOT-Fresh predicate, source-branched. Each Fresh disjunct is negated:
	// a row is a candidate when it is NOT fresh under its source's windows.
	// Bound params (?) carry the cutoffs — no interpolation of cfg.* ints.
	const notFresh = `
		(
			source = 'autocache' AND NOT (
				(downloaded_at IS NOT NULL AND downloaded_at > ?) OR
				(last_fetch_at IS NOT NULL AND last_fetch_at > ?)
			)
		) OR (
			source = 'admin' AND NOT (
				(downloaded_at IS NOT NULL AND downloaded_at > ?) OR
				(last_fetch_at IS NOT NULL AND last_fetch_at > ?)
			)
		)`

	const tierOrder = `CASE ` +
		`WHEN source = 'autocache' AND last_fetch_at IS NULL THEN 1 ` +
		`WHEN source = 'autocache' THEN 2 ` +
		`WHEN source = 'admin' AND last_fetch_at IS NULL THEN 3 ` +
		`ELSE 4 END ASC`

	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("minio_path LIKE 'aeProvider/%'").
		Where(notFresh, autoDownloadCutoff, autoFetchCutoff, adminCutoff, adminCutoff).
		Order(tierOrder).
		Order("COALESCE(last_fetch_at, downloaded_at, created_at) ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list stale eviction candidates")
	}
	return eps, nil
}

// DeleteByID hard-deletes the single library_episodes row identified by id. The
// Evictor (Plan 02) calls this AFTER the row's MinIO objects are gone, so a row
// never points at deleted data (we tolerate orphaned objects over orphaned
// rows). Deleting a non-existent id is a nil no-op (zero RowsAffected is a
// legitimate "already gone" state, not a NotFound) — the evictor may race the
// periodic sweep, and a double-delete must be harmless.
func (r *EpisodeRepository) DeleteByID(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&domain.Episode{}).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "delete episode by id")
	}
	return nil
}

// ListPool returns every row in the unified first-party aeProvider/ pool (admin +
// autocache). The Evictor's periodic Accountant sweep (Plan 02) lists the pool once
// and Classify-buckets each row in Go to publish the per-(source,freshness)
// bytes_used / episodes gauges — so the freshness math stays in ONE pure Go helper
// (Classify) rather than being duplicated in SQL. The LIKE pattern is a fixed literal
// (no user input), so it is GORM-safe inline. Ordered by created_at ASC for
// deterministic processing.
func (r *EpisodeRepository) ListPool(ctx context.Context) ([]domain.Episode, error) {
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("minio_path LIKE 'aeProvider/%'").
		Order("created_at ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list pool episodes")
	}
	return eps, nil
}
