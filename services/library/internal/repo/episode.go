package repo

import (
	"context"
	stderrors "errors"
	"strings"

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
