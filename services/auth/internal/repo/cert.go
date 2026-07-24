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

type CertRepository struct {
	db *gorm.DB
}

func NewCertRepository(db *gorm.DB) *CertRepository {
	return &CertRepository{db: db}
}

// GetCA returns the single CA row, or NotFound when none exists yet.
func (r *CertRepository) GetCA(ctx context.Context) (*domain.AuthCA, error) {
	var ca domain.AuthCA
	if err := r.db.WithContext(ctx).First(&ca, "id = 1").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("auth ca")
		}
		return nil, fmt.Errorf("get ca: %w", err)
	}
	return &ca, nil
}

// SaveCA inserts the CA row. A concurrent duplicate insert (two replicas
// booting at once) fails on the primary key — callers treat that as "someone
// else won, re-read".
func (r *CertRepository) SaveCA(ctx context.Context, ca *domain.AuthCA) error {
	ca.ID = 1
	if err := r.db.WithContext(ctx).Create(ca).Error; err != nil {
		return fmt.Errorf("save ca: %w", err)
	}
	return nil
}

func (r *CertRepository) CreateUserCert(ctx context.Context, c *domain.UserCertificate) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("create user cert: %w", err)
	}
	return nil
}

func (r *CertRepository) ListUserCerts(ctx context.Context, userID string) ([]domain.UserCertificate, error) {
	var out []domain.UserCertificate
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list user certs: %w", err)
	}
	return out, nil
}

// GetByFingerprint returns the non-revoked cert row for a fingerprint.
func (r *CertRepository) GetByFingerprint(ctx context.Context, fp string) (*domain.UserCertificate, error) {
	var c domain.UserCertificate
	err := r.db.WithContext(ctx).
		Where("fingerprint_sha256 = ? AND revoked_at IS NULL", fp).
		First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("certificate")
		}
		return nil, fmt.Errorf("get cert by fingerprint: %w", err)
	}
	return &c, nil
}

// RevokeUserCert marks the user's cert revoked. NotFound when the row is not
// the caller's or already revoked (idempotent from the UI's point of view).
func (r *CertRepository) RevokeUserCert(ctx context.Context, id, userID string) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserCertificate{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", time.Now())
	if res.Error != nil {
		return fmt.Errorf("revoke cert: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("certificate")
	}
	return nil
}

func (r *CertRepository) TouchUserCert(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domain.UserCertificate{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now()).Error
}
