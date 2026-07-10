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
	if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "library_episodes_shikimori_ep_storage_uniq") {
		return liberrors.AlreadyExists("episode")
	}
	return liberrors.Wrap(err, liberrors.CodeInternal, "create episode")
}

// GetByShikimoriEpisode returns the episode row matching the (shikimori_id,
// episode_number) pair, or liberrors.NotFound if no such row exists.
//
// Since migration 017 the same (shikimori_id, episode_number) can have TWO rows
// (a local 'minio' copy AND an external 's3' copy), so the lookup takes an
// explicit storage-preference argument to stay DETERMINISTIC:
//   - storage != "" → filter to exactly that backend (the caller wants the row
//     on a specific store — e.g. batchingest checking presence in its ingest
//     destination, or the episodes GET honoring an explicit ?storage=).
//   - storage == "" → no filter; prefer the local 'minio' row when both exist
//     (ORDER BY storage='minio' DESC), else return whichever single row is
//     present. This is the "present in any backend, minio-first" gate the
//     autocache Planner and the default episodes GET use.
func (r *EpisodeRepository) GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int, storage string) (*domain.Episode, error) {
	q := r.db.WithContext(ctx).
		Where("shikimori_id = ? AND episode_number = ?", shikimoriID, episodeNumber)
	if storage != "" {
		q = q.Where("storage = ?", storage)
	} else {
		// Deterministic minio-first preference. The boolean expression evaluates
		// to 1/0 on both Postgres and the sqlite test harness; DESC puts the
		// local minio row first.
		q = q.Order("(storage = 'minio') DESC")
	}
	var ep domain.Episode
	err := q.First(&ep).Error
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

// ListRecentDistinct returns the newest episode of each distinct anime, newest
// upload first, capped at limit. Used by the playback probe's ae target set
// ("3 latest uploaded library episodes", deduped to distinct anime). Portable:
// loads rows newest-first and dedupes in Go (no Postgres-only DISTINCT ON), so
// it works on both the Postgres prod DB and the sqlite test harness.
func (r *EpisodeRepository) ListRecentDistinct(ctx context.Context, limit int) ([]domain.Episode, error) {
	if limit <= 0 || limit > 20 {
		limit = 3
	}
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Order("created_at DESC, episode_number DESC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list recent distinct episodes")
	}
	return dedupeNewestPerAnime(eps, limit), nil
}

// dedupeNewestPerAnime keeps the first row per shikimori_id from an already
// newest-first slice and caps the result at limit. Factored out so the dedupe
// rule is unit-testable without a database.
func dedupeNewestPerAnime(eps []domain.Episode, limit int) []domain.Episode {
	seen := make(map[string]struct{}, len(eps))
	out := make([]domain.Episode, 0, limit)
	for _, ep := range eps {
		if _, ok := seen[ep.ShikimoriID]; ok {
			continue
		}
		seen[ep.ShikimoriID] = struct{}{}
		out = append(out, ep)
		if len(out) >= limit {
			break
		}
	}
	return out
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

// ListByStorageSource returns every episode row currently on backend `storage`
// with the given `source` (e.g. storage='minio', source='autocache' — the
// storage-migrate operator's selection set), ordered by created_at ASC for
// deterministic, resumable processing. Both filters are BOUND `?` params.
//
// Selection scoped to storage='minio' + already-flipped rows never reselected is
// what makes the migration idempotent: a row that has already been copied to s3
// and flipped (storage='s3') is invisible to this query, so a re-run only ever
// picks up rows still awaiting migration.
func (r *EpisodeRepository) ListByStorageSource(ctx context.Context, storage string, source domain.EpisodeSource) ([]domain.Episode, error) {
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("storage = ? AND source = ?", storage, source).
		Order("created_at ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list episodes by storage+source")
	}
	return eps, nil
}

// UpdateStorage repoints a single episode row's storage backend (e.g. 'minio' →
// 's3') after its objects have been copied AND re-List-verified on the target.
// The storage-migrate operator flips the row BEFORE deleting the local prefix,
// so a crash between flip and delete leaves a benign dual-presence state (the s3
// row is authoritative; the stale minio objects are cleaned on the next re-run).
// Scoped to the given id (single-column Updates); returns nil on success.
func (r *EpisodeRepository) UpdateStorage(ctx context.Context, id string, storage string) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("id = ?", id).
		Update("storage", storage).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "update episode storage")
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
// Scoped to storage='minio' — the Evictor's budget frees LOCAL disk, so an s3
// row (which consumes no local disk) must never inflate this total.
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
		Where("storage = 'minio'").
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
//
// Scoped to storage='minio': the Evictor deletes MinIO objects then the row
// (evictOne), which is only correct for a row that actually lives on local
// disk — an s3 row must never surface here.
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
		Where("storage = 'minio'").
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

// ListWithoutStoryboard returns up to `limit` episodes that still lack a
// storyboard (has_storyboard = false), oldest first (created_at ASC). It skips
// rows younger than 10 minutes for two reasons: (1) the encoder's ingest-time
// storyboard pass already covers freshly-encoded episodes, so re-processing
// them here would race that pass; (2) an episode that keeps failing must not be
// re-hammered every cycle — it re-surfaces only once it has aged out and the
// backfill loop re-queries. The 10-minute cutoff is computed in Go and passed
// as a BOUND `?` param (portable across the Postgres prod DB and the sqlite
// test harness — no Postgres-only `now() - interval`), mirroring
// ListStaleEvictionCandidates.
func (r *EpisodeRepository) ListWithoutStoryboard(ctx context.Context, limit int) ([]domain.Episode, error) {
	if limit <= 0 {
		limit = 1
	}
	cutoff := time.Now().Add(-10 * time.Minute)
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("has_storyboard = ? AND created_at < ?", false, cutoff).
		Order("created_at ASC").
		Limit(limit).
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list episodes without storyboard")
	}
	return eps, nil
}

// SetHasStoryboard flips has_storyboard = true for the single episode row
// identified by id — the storyboard backfill worker's success step, run only
// AFTER the sprite sheets + VTT upload succeeds. A scoped single-column
// Update; a non-matching id is a nil no-op (the row may have been evicted
// between the list and the set, which is a legitimate "already gone" state,
// not a NotFound — same tolerance as DeleteByID).
func (r *EpisodeRepository) SetHasStoryboard(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.Episode{}).
		Where("id = ?", id).
		Update("has_storyboard", true).Error; err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "set episode has_storyboard")
	}
	return nil
}

// ListPool returns every row in the unified first-party aeProvider/ pool (admin +
// autocache). The Evictor's periodic Accountant sweep (Plan 02) lists the pool once
// and Classify-buckets each row in Go to publish the per-(source,freshness)
// bytes_used / episodes gauges — so the freshness math stays in ONE pure Go helper
// (Classify) rather than being duplicated in SQL. The LIKE pattern is a fixed literal
// (no user input), so it is GORM-safe inline. Ordered by created_at ASC for
// deterministic processing. Scoped to storage='minio' — these gauges report LOCAL
// disk usage, so an s3 row must never be counted.
func (r *EpisodeRepository) ListPool(ctx context.Context) ([]domain.Episode, error) {
	var eps []domain.Episode
	if err := r.db.WithContext(ctx).
		Where("minio_path LIKE 'aeProvider/%'").
		Where("storage = 'minio'").
		Order("created_at ASC").
		Find(&eps).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "list pool episodes")
	}
	return eps, nil
}
