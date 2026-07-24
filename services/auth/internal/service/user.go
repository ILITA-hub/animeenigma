package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

// otherSessionRevoker revokes every session of a user except one. It is
// satisfied by *repo.SessionRepository — the SAME RevokeOthers method the
// "revoke other sessions" settings button uses — and by in-package test fakes.
// UserService depends on the interface, not the concrete repo, so a password
// change can invalidate the user's other sessions and still be unit-tested.
type otherSessionRevoker interface {
	RevokeOthers(ctx context.Context, userID, keepID string) (int64, error)
}

// nilSessionID is a syntactically valid UUID that never matches a real session
// row (gen_random_uuid never yields all-zeros). It is passed as the "keep" id
// when the caller's own session cannot be identified, so RevokeOthers revokes
// ALL of the user's sessions (fail safe — the user simply re-authenticates).
const nilSessionID = "00000000-0000-0000-0000-000000000000"

type UserService struct {
	userRepo    *repo.UserRepository
	sessionRepo otherSessionRevoker
	log         *logger.Logger
}

func NewUserService(userRepo *repo.UserRepository, sessionRepo otherSessionRevoker, log *logger.Logger) *UserService {
	return &UserService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		log:         log,
	}
}

func (s *UserService) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *UserService) GetPublicProfile(ctx context.Context, id string) (*domain.PublicUser, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	pub := user.ToPublic()
	pub.ShowcaseState = s.userRepo.GetShowcaseState(ctx, user.ID)
	return pub, nil
}

// Update applies a profile update. currentSessionID is the id of the caller's
// own session (the JWT `sid` claim, forwarded by the handler); it is spared
// when a password change forces revocation of the user's OTHER sessions so the
// user stays logged in on the device they just used.
func (s *UserService) Update(ctx context.Context, userID, currentSessionID string, req *domain.UpdateUserRequest) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update username if provided
	if req.Username != nil && *req.Username != user.Username {
		// Enforce the same charset/length policy the registration path applies
		// (mirrors the domain.ValidatePassword call below). Without this the
		// profile-update path accepted homoglyphs, whitespace-padded or empty
		// usernames and control characters that registration forbids.
		if err := domain.ValidateUsername(*req.Username); err != nil {
			return nil, err
		}

		exists, err := s.userRepo.ExistsByUsername(ctx, *req.Username)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.AlreadyExists("username")
		}
		user.Username = *req.Username
	}

	// Update password if provided
	passwordChanged := false
	if req.NewPassword != nil {
		if req.CurrentPassword == nil {
			return nil, errors.InvalidInput("current password is required to change password")
		}

		// Verify current password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(*req.CurrentPassword)); err != nil {
			return nil, errors.InvalidInput("current password is incorrect")
		}

		// Enforce the password policy before hashing (8–72 bytes). Without the
		// upper bound a >72-byte password makes bcrypt return an opaque error
		// that surfaced as a 500 instead of a clean 400.
		if err := domain.ValidatePassword(*req.NewPassword); err != nil {
			return nil, err
		}

		// Hash new password
		hashedPassword, err := HashPassword(*req.NewPassword)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		user.PasswordHash = hashedPassword
		passwordChanged = true
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	// A changed credential must invalidate every OTHER session so a refresh
	// token captured under the old password cannot outlive it (CWE-613).
	// Sessions never time-expire (100-year sentinel) and are revoke-only, so
	// this is the only thing that kills the attacker's stolen token; without
	// it the "change your password" remediation silently does nothing.
	//
	// Ordering is deliberate: the new password is persisted FIRST (above) and
	// is durable no matter what happens here — a revocation failure never rolls
	// the password back. The failure IS surfaced (not swallowed) so the caller
	// learns the containment step did not fully complete rather than believing
	// they are safe; they can retry via the /auth/sessions/revoke-others
	// control. A username-only (or any non-password) update revokes nothing.
	if passwordChanged {
		keepID := currentSessionID
		if keepID == "" {
			// The caller's own session id is unknown (e.g. a legacy access
			// token minted without a `sid` claim). Fail safe: revoke ALL of the
			// user's sessions rather than none — forcing a re-login is strictly
			// safer than leaving a possibly-stolen token alive.
			keepID = nilSessionID
			s.log.Warnw("password changed but current session id unknown; revoking all sessions", "user_id", userID)
		}
		n, err := s.sessionRepo.RevokeOthers(ctx, userID, keepID)
		if err != nil {
			return nil, fmt.Errorf("revoke other sessions after password change: %w", err)
		}
		metrics.AuthEventsTotal.WithLabelValues("session_revoked", "password_change").Add(float64(n))
		s.log.Infow("revoked other sessions after password change", "user_id", userID, "revoked", n)
	}

	return user, nil
}

func (s *UserService) Delete(ctx context.Context, userID string) error {
	return s.userRepo.Delete(ctx, userID)
}

func (s *UserService) GetPublicProfileByPublicID(ctx context.Context, publicID string) (*domain.PublicUser, error) {
	user, err := s.userRepo.GetByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	pub := user.ToPublic()
	pub.ShowcaseState = s.userRepo.GetShowcaseState(ctx, user.ID)
	return pub, nil
}

func (s *UserService) UpdatePublicID(ctx context.Context, userID, publicID string) error {
	// Check if public_id is already taken
	exists, err := s.userRepo.ExistsByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	if exists {
		// Check if it's the same user
		user, err := s.userRepo.GetByPublicID(ctx, publicID)
		if err != nil {
			return err
		}
		if user.ID != userID {
			return errors.AlreadyExists("public_id")
		}
		// Same user, no change needed
		return nil
	}

	return s.userRepo.UpdatePublicID(ctx, userID, publicID)
}

func (s *UserService) UpdateAvatar(ctx context.Context, userID, avatar string) error {
	if !strings.HasPrefix(avatar, "data:image/") {
		return errors.InvalidInput("avatar must be a data:image/* URL")
	}
	// Max ~500KB base64 payload
	if len(avatar) > 500*1024 {
		return errors.InvalidInput("avatar is too large (max 500KB)")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	user.Avatar = avatar
	return s.userRepo.Update(ctx, user)
}

// IsValidTimezone reports whether tz is a concrete IANA zone the runtime
// image can resolve (alpine ships tzdata). "Local" is rejected — a stored
// zone must mean the same instant on every machine that reads it.
func IsValidTimezone(tz string) bool {
	if tz == "" || tz == "Local" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

func (s *UserService) UpdateTimezone(ctx context.Context, userID, tz string) error {
	if !IsValidTimezone(tz) {
		return errors.InvalidInput("invalid IANA timezone: " + tz)
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	user.Timezone = tz
	return s.userRepo.Update(ctx, user)
}

func (s *UserService) UpdatePublicStatuses(ctx context.Context, userID string, statuses []string) error {
	// Validate statuses
	validStatuses := map[string]bool{
		"watching":      true,
		"completed":     true,
		"plan_to_watch": true,
		"on_hold":       true,
		"dropped":       true,
	}

	for _, status := range statuses {
		if !validStatuses[status] {
			return errors.InvalidInput("invalid status: " + status)
		}
	}

	return s.userRepo.UpdatePublicStatuses(ctx, userID, statuses)
}

func (s *UserService) UpdateActivityVisibility(ctx context.Context, userID, visibility string) error {
	if !domain.ValidActivityVisibility(visibility) {
		return errors.InvalidInput("invalid activity_visibility: " + visibility)
	}
	return s.userRepo.UpdateActivityVisibility(ctx, userID, visibility)
}
