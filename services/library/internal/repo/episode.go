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
