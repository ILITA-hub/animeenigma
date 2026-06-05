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

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session.
func (r *SessionRepository) Create(ctx context.Context, s *domain.UserSession) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindAliveByHash returns the alive (not revoked) session whose current hash
// equals `hash`. ExpiresAt is a far-future sentinel and only acts as a cheap
// tripwire. Returns NotFound if none.
func (r *SessionRepository) FindAliveByHash(ctx context.Context, hash string) (*domain.UserSession, error) {
	now := time.Now()
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("revoked_at IS NULL AND expires_at > ? AND refresh_token_hash = ?", now, hash).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("session")
		}
		return nil, fmt.Errorf("find session by hash: %w", err)
	}
	return &s, nil
}

// Touch records activity on a refresh: bumps last_seen_at + ip and pushes
// expires_at to the caller-supplied (far-future) sentinel. No-op on a revoked
// row. This replaces the old rotate-on-refresh: the refresh token itself is
// stable, so the only per-refresh write is this activity stamp.
func (r *SessionRepository) Touch(ctx context.Context, sessionID, ip string, lastSeen, expiresAt time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Updates(map[string]any{
			"last_seen_at": lastSeen,
			"ip":           ip,
			"expires_at":   expiresAt,
		})
	if res.Error != nil {
		return fmt.Errorf("touch session: %w", res.Error)
	}
	return nil
}

// Revoke marks one session revoked iff it belongs to userID and is alive.
// Returns ErrRecordNotFound-equivalent if the row is missing or not the user's.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID, userID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, now).
		Update("revoked_at", now)
	if res.Error != nil {
		return fmt.Errorf("revoke session: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("session")
	}
	return nil
}

// RevokeOthers revokes every alive session for `userID` except `keepID`.
// Returns the count revoked.
func (r *SessionRepository) RevokeOthers(ctx context.Context, userID, keepID string) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, keepID).
		Update("revoked_at", now)
	if res.Error != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ListAlive returns all alive sessions for a user, newest last_seen first.
func (r *SessionRepository) ListAlive(ctx context.Context, userID string) ([]*domain.UserSession, error) {
	now := time.Now()
	var sessions []*domain.UserSession
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
		Order("last_seen_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list alive sessions: %w", err)
	}
	return sessions, nil
}

// Cleanup removes sessions revoked more than 7 days ago. Non-rotating sessions
// never time-expire, so there is no expiry-based deletion.
func (r *SessionRepository) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	res := r.db.WithContext(ctx).
		Where("revoked_at IS NOT NULL AND revoked_at < ?", cutoff).
		Delete(&domain.UserSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
