package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo *repo.UserRepository
	log      *logger.Logger
}

func NewUserService(userRepo *repo.UserRepository, log *logger.Logger) *UserService {
	return &UserService{
		userRepo: userRepo,
		log:      log,
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
	return user.ToPublic(), nil
}

func (s *UserService) Update(ctx context.Context, userID string, req *domain.UpdateUserRequest) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update username if provided
	if req.Username != nil && *req.Username != user.Username {
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
	if req.NewPassword != nil {
		if req.CurrentPassword == nil {
			return nil, errors.InvalidInput("current password is required to change password")
		}

		// Verify current password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(*req.CurrentPassword)); err != nil {
			return nil, errors.InvalidInput("current password is incorrect")
		}

		// Hash new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		user.PasswordHash = string(hashedPassword)
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
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
	return user.ToPublic(), nil
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
