package repo

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	user.PublicID = generatePublicID()
	user.PublicStatuses = pq.StringArray{"watching", "completed", "plan_to_watch"}

	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUniqueViolation(err) {
			return liberrors.AlreadyExists("username")
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func generatePublicID() string {
	return fmt.Sprintf("user%d", rand.Intn(9000000)+1000000)
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "telegram_id = ?", telegramID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Not found is not an error for this use case
		}
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "username = ?", username).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	result := r.db.WithContext(ctx).Save(user)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return liberrors.AlreadyExists("username or public_id")
		}
		return fmt.Errorf("update user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}

func (r *UserRepository) GetByPublicID(ctx context.Context, publicID string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "public_id = ?", publicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by public_id: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) UpdatePublicID(ctx context.Context, userID, publicID string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("public_id", publicID)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			return liberrors.AlreadyExists("public_id")
		}
		return fmt.Errorf("update public_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}

func (r *UserRepository) UpdatePublicStatuses(ctx context.Context, userID string, statuses []string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("public_statuses", pq.StringArray(statuses))
	if result.Error != nil {
		return fmt.Errorf("update public_statuses: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}

func (r *UserRepository) ExistsByPublicID(ctx context.Context, publicID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.User{}).Where("public_id = ?", publicID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check public_id exists: %w", err)
	}
	return count > 0, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&domain.User{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}

func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check username exists: %w", err)
	}
	return count > 0, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "unique constraint") || contains(errStr, "duplicate key")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
