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

// GraceWindow is how long the previous refresh-token hash stays accepted
// after a successful rotation. Long enough to absorb cross-tab races,
// short enough that a stolen previous-token can't be reused indefinitely.
const GraceWindow = 30 * time.Second

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

// FindAliveByHash returns the alive session whose current OR previous
// (within grace) hash equals `hash`. Returns NotFound if none.
func (r *SessionRepository) FindAliveByHash(ctx context.Context, hash string) (*domain.UserSession, error) {
	now := time.Now()
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("revoked_at IS NULL AND expires_at > ?", now).
		Where("refresh_token_hash = ? OR (previous_refresh_token_hash = ? AND grace_until > ?)", hash, hash, now).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("session")
		}
		return nil, fmt.Errorf("find session by hash: %w", err)
	}
	return &s, nil
}

// RotateResult tells the caller which path the rotation took so it knows
// whether to set a new refresh-token cookie.
type RotateResult struct {
	Session *domain.UserSession
	Rotated bool // true: caller should Set-Cookie with new RT. false: grace-path, leave cookie alone.
}

// Rotate performs the CAS rotation. If `oldHash` matches `refresh_token_hash`,
// it swaps in `newHash` (also moves the old hash into previous + sets grace).
// If `oldHash` matches `previous_refresh_token_hash` (within grace), it does
// NOT rotate — caller mints a fresh access token and reuses the existing RT.
//
// `extendUntil` is the new expires_at the rotating writer should set.
// `ip` is recorded for audit.
func (r *SessionRepository) Rotate(
	ctx context.Context,
	sessionID, oldHash, newHash, ip string,
	extendUntil time.Time,
) (RotateResult, error) {
	now := time.Now()
	graceUntil := now.Add(GraceWindow)

	// Try CAS swap first.
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, oldHash, now).
		Updates(map[string]any{
			"previous_refresh_token_hash": oldHash,
			"refresh_token_hash":          newHash,
			"grace_until":                 graceUntil,
			"last_seen_at":                now,
			"expires_at":                  extendUntil,
			"ip":                          ip,
		})
	if res.Error != nil {
		return RotateResult{}, fmt.Errorf("rotate session: %w", res.Error)
	}

	if res.RowsAffected == 1 {
		// Re-read to return current state.
		var s domain.UserSession
		if err := r.db.WithContext(ctx).First(&s, "id = ?", sessionID).Error; err != nil {
			return RotateResult{}, fmt.Errorf("re-read rotated session: %w", err)
		}
		return RotateResult{Session: &s, Rotated: true}, nil
	}

	// CAS missed — concurrent rotation already happened. Confirm we're on
	// the grace path: the row's previous_hash should equal oldHash within
	// grace_until. Bump last_seen but DO NOT mint a new RT.
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("id = ? AND previous_refresh_token_hash = ? AND grace_until > ? AND revoked_at IS NULL AND expires_at > ?",
			sessionID, oldHash, now, now).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RotateResult{}, liberrors.NotFound("session")
		}
		return RotateResult{}, fmt.Errorf("read grace session: %w", err)
	}

	// Best-effort touch of last_seen + ip; ignore conflicts.
	_ = r.db.WithContext(ctx).
		Model(&s).
		Where("id = ?", s.ID).
		Updates(map[string]any{"last_seen_at": now, "ip": ip}).Error

	return RotateResult{Session: &s, Rotated: false}, nil
}

// Revoke marks one session revoked iff it belongs to userID and is alive.
// Returns ErrRecordNotFound-equivalent if the row is missing or not the user's.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID, userID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
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

// Cleanup removes sessions revoked >7d ago or expired >7d ago.
// Returns the count deleted.
func (r *SessionRepository) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	res := r.db.WithContext(ctx).
		Where("(revoked_at IS NOT NULL AND revoked_at < ?) OR expires_at < ?", cutoff, cutoff).
		Delete(&domain.UserSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
