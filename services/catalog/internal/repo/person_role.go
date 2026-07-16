package repo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// PersonRoleRepository persists the flat anime staff/crew credits.
type PersonRoleRepository struct {
	db *gorm.DB
}

func NewPersonRoleRepository(db *gorm.DB) *PersonRoleRepository {
	return &PersonRoleRepository{db: db}
}

// ReplaceAnimeStaff deletes the anime's existing staff rows and inserts the
// given set, in one transaction. The flat table has no join to resolve, so
// this is a straight delete-then-insert (mirrors the ReplaceAnimeCharacters
// resilience contract without the id-remap dance). IDs are assigned Go-side
// when blank — Postgres's gen_random_uuid() default works in prod, but
// generating here keeps the repo portable to the SQLite test DB (same reason
// CharacterRepository does it).
func (r *PersonRoleRepository) ReplaceAnimeStaff(ctx context.Context, animeID string, rows []domain.AnimePersonRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("anime_id = ?", animeID).Delete(&domain.AnimePersonRole{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			if rows[i].ID == "" {
				rows[i].ID = uuid.NewString()
			}
		}
		return tx.Create(&rows).Error
	})
}

// GetStaffByAnimeID returns the anime's staff ordered by whitelist rank then name.
func (r *PersonRoleRepository) GetStaffByAnimeID(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error) {
	var rows []domain.AnimePersonRole
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Order("position ASC, name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
