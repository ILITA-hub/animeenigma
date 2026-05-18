package repo

import (
	"context"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
)

// FilenamePatternRepository is a thin read-only DAO over
// library_filename_patterns. The filename detector loads every row
// once at startup; there is no runtime mutation path (patterns are
// added via the seed migration or admin SQL).
type FilenamePatternRepository struct {
	db *gorm.DB
}

// NewFilenamePatternRepository constructs the repo.
func NewFilenamePatternRepository(db *gorm.DB) *FilenamePatternRepository {
	return &FilenamePatternRepository{db: db}
}

// LoadAll returns every pattern row ordered by uploader ASC. Called
// once at library service startup by the detector constructor.
func (r *FilenamePatternRepository) LoadAll(ctx context.Context) ([]domain.FilenamePattern, error) {
	var out []domain.FilenamePattern
	if err := r.db.WithContext(ctx).Order("uploader ASC").Find(&out).Error; err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "load filename patterns")
	}
	return out, nil
}
