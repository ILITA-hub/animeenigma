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

// GraceWindow is how long the previous refresh-token hash stays accepted after
// a successful rotation. It MUST exceed the access-token TTL (JWT_ACCESS_TTL,
// 15m by default): the browser only refreshes when its access token nears
// expiry, so consecutive refreshes are ~15m apart. A window shorter than that
// lapses before the next refresh and provides no benefit. At 20m it outlasts
// the 15m cadence (with margin), so an active browser that lost a rotation's
// Set-Cookie (e.g. a transient gateway/proxy 5xx) self-heals on its next
// refresh by riding the grace path instead of getting a hard 401 → logout. The
// window SLIDES on each grace hit (see Rotate), but never past MaxGraceLifetime
// from when it first opened — bounding how long a stolen previous-token can be
// replayed. Keep GraceWindow > JWT_ACCESS_TTL if either is retuned.
const GraceWindow = 20 * time.Minute

// MaxGraceLifetime caps how long a single grace window can be kept alive by
// sliding, measured from the rotation that opened it. Past this the previous
// hash is refused (the window is allowed to lapse) so a desynced client must
// re-authenticate once — this is the hard bound on previous-token replay.
const MaxGraceLifetime = 30 * time.Minute

// slideGraceUntil computes the next grace_until for a grace-path hit: now +
// GraceWindow, clamped to graceOpenedAt + MaxGraceLifetime. Returns ok=false
// when the absolute lifetime cap is reached, or when graceOpenedAt is unknown
// (legacy rows rotated before the column existed) — in both cases the caller
// leaves grace_until untouched and lets the window lapse naturally.
func slideGraceUntil(graceOpenedAt *time.Time, now time.Time) (time.Time, bool) {
	if graceOpenedAt == nil {
		return time.Time{}, false
	}
	lifetimeCap := graceOpenedAt.Add(MaxGraceLifetime)
	if !now.Before(lifetimeCap) {
		return time.Time{}, false
	}
	candidate := now.Add(GraceWindow)
	if candidate.After(lifetimeCap) {
		candidate = lifetimeCap
	}
	return candidate, true
}

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
			"grace_opened_at":             now,
			"last_seen_at":                now,
			"expires_at":                  extendUntil,
			"ip":                          ip,
		})
	if res.Error != nil {
		return RotateResult{}, fmt.Errorf("rotate session: %w", res.Error)
	}

	if res.RowsAffected == 1 {
		// Re-read to return current state. Note: under heavy concurrency,
		// a third rotation could already have swapped refresh_token_hash
		// again before this SELECT lands. The caller MUST use its own
		// locally-generated newRT for the Set-Cookie value, not
		// result.Session.RefreshTokenHash, since that field reflects
		// whatever the latest rotation wrote — possibly newer than newHash.
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

	// Best-effort touch of last_seen + ip; ignore conflicts. Also SLIDE the
	// grace window forward so an active browser stuck on the previous token
	// (a lost-rotation desync) keeps riding the grace path instead of lapsing
	// between its ~15-min refreshes — bounded by MaxGraceLifetime from when
	// the window first opened so a stolen previous-token can't be replayed
	// indefinitely.
	touch := map[string]any{"last_seen_at": now, "ip": ip}
	if newGrace, ok := slideGraceUntil(s.GraceOpenedAt, now); ok {
		touch["grace_until"] = newGrace
	}
	_ = r.db.WithContext(ctx).
		Model(&s).
		Where("id = ?", s.ID).
		Updates(touch).Error

	return RotateResult{Session: &s, Rotated: false}, nil
}

// PreviousHashExists reports whether ANY session currently holds `hash` as its
// previous_refresh_token_hash — i.e. the token was valid within living memory
// but its grace window has since lapsed. Used only for observability, to tell a
// lost-rotation desync apart from a genuinely unknown/garbage refresh token.
func (r *SessionRepository) PreviousHashExists(ctx context.Context, hash string) bool {
	var count int64
	r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("previous_refresh_token_hash = ?", hash).
		Count(&count)
	return count > 0
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
