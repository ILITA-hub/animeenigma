package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

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
	user.ActivityVisibility = domain.ActivityVisibilityAll

	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUniqueViolation(err) {
			return liberrors.AlreadyExists("username")
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func generatePublicID() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(9000000))
	return fmt.Sprintf("user%d", n.Int64()+1000000)
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

// UpdatePasswordHash writes only the password_hash column for the user.
// Used by the opportunistic-rehash path on successful login.
func (r *UserRepository) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("password_hash", hash)
	if result.Error != nil {
		return fmt.Errorf("update password hash: %w", result.Error)
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

func (r *UserRepository) UpdateActivityVisibility(ctx context.Context, userID, visibility string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("activity_visibility", visibility)
	if result.Error != nil {
		return fmt.Errorf("update activity_visibility: %w", result.Error)
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

func (r *UserRepository) GetByApiKeyHash(ctx context.Context, hash string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "api_key_hash = ?", hash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by api key hash: %w", err)
	}
	return &user, nil
}

func (r *UserRepository) UpdateApiKeyHash(ctx context.Context, userID string, hash *string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("api_key_hash", hash)
	if result.Error != nil {
		return fmt.Errorf("update api key hash: %w", result.Error)
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

// GetShowcaseState returns the profile-showcase visibility signal for a user.
//
// This is a read-only denormalization of the player-owned profile_showcases
// table, co-located in the shared "animeenigma" Postgres DB (auth, player,
// catalog, … all share it). Auth does NOT own or AutoMigrate this table, so we
// read it via a raw query — never a GORM model. It is an in-DB read, not a
// cross-service HTTP call.
//
// Mapping: missing table/row or empty blocks → "none"; non-empty + enabled →
// "visible"; non-empty + not enabled → "hidden". Any error (e.g. before the
// player has migrated the table) defensively yields "none" — the profile still
// loads, just without a showcase tab, and self-heals once the table exists.
func (r *UserRepository) GetShowcaseState(ctx context.Context, userID string) string {
	var row struct {
		Enabled bool
		Blocks  string
	}
	err := r.db.WithContext(ctx).
		Raw(`SELECT enabled, blocks FROM profile_showcases WHERE user_id = ?`, userID).
		Scan(&row).Error
	if err != nil {
		return domain.ShowcaseStateNone // table/row absent or error → treat as none
	}
	hasContent := row.Blocks != "" && row.Blocks != "[]"
	if !hasContent {
		return domain.ShowcaseStateNone
	}
	if row.Enabled {
		return domain.ShowcaseStateVisible
	}
	return domain.ShowcaseStateHidden
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
