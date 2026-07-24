package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// PasskeyRepository persists enrolled WebAuthn credentials.
type PasskeyRepository struct {
	db *gorm.DB
}

func NewPasskeyRepository(db *gorm.DB) *PasskeyRepository {
	return &PasskeyRepository{db: db}
}

func (r *PasskeyRepository) Create(ctx context.Context, c *domain.WebAuthnCredential) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("create passkey: %w", err)
	}
	return nil
}

func (r *PasskeyRepository) ListByUser(ctx context.Context, userID string) ([]domain.WebAuthnCredential, error) {
	var out []domain.WebAuthnCredential
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list passkeys: %w", err)
	}
	return out, nil
}

// GetByCredentialID looks up a credential by its base64url credential id
// (the value webauthn.js reports and toLibraryCredential decodes back).
func (r *PasskeyRepository) GetByCredentialID(ctx context.Context, credID string) (*domain.WebAuthnCredential, error) {
	var c domain.WebAuthnCredential
	err := r.db.WithContext(ctx).
		Where("credential_id = ?", credID).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("passkey")
		}
		return nil, fmt.Errorf("get passkey by credential id: %w", err)
	}
	return &c, nil
}

// UpdateSignCount bumps the stored signature counter and last-used timestamp
// after a successful login, so a future clone-detection check has a baseline.
func (r *PasskeyRepository) UpdateSignCount(ctx context.Context, id string, count uint32, lastUsed time.Time) error {
	if err := r.db.WithContext(ctx).
		Model(&domain.WebAuthnCredential{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"sign_count":   count,
			"last_used_at": lastUsed,
		}).Error; err != nil {
		return fmt.Errorf("update passkey sign count: %w", err)
	}
	return nil
}

// Delete hard-deletes the user's own passkey row. NotFound when the row
// doesn't exist or belongs to a different user.
func (r *PasskeyRepository) Delete(ctx context.Context, id, userID string) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&domain.WebAuthnCredential{})
	if res.Error != nil {
		return fmt.Errorf("delete passkey: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("passkey")
	}
	return nil
}
