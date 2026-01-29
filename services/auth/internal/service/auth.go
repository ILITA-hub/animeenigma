package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo   *repo.UserRepository
	cache      *cache.RedisCache
	jwtManager *authz.JWTManager
	log        *logger.Logger
}

func NewAuthService(
	userRepo *repo.UserRepository,
	cache *cache.RedisCache,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		cache:      cache,
		jwtManager: authz.NewJWTManager(jwtConfig),
		log:        log,
	}
}

func (s *AuthService) Register(ctx context.Context, req *domain.RegisterRequest) (*domain.AuthResponse, error) {
	// Check if username exists
	exists, err := s.userRepo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("check username: %w", err)
	}
	if exists {
		return nil, errors.AlreadyExists("username")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := &domain.User{
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		Role:         authz.RoleUser,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	// Generate tokens
	return s.generateAuthResponse(user)
}

func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest) (*domain.AuthResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok && appErr.Code == errors.CodeNotFound {
			return nil, errors.Unauthorized("invalid credentials")
		}
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.Unauthorized("invalid credentials")
	}

	return s.generateAuthResponse(user)
}

func (s *AuthService) RefreshToken(ctx context.Context, req *domain.RefreshRequest) (*domain.AuthResponse, error) {
	// Validate refresh token
	userID, err := s.jwtManager.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		if err == authz.ErrTokenExpired {
			return nil, errors.Unauthorized("refresh token expired")
		}
		return nil, errors.Unauthorized("invalid refresh token")
	}

	// Check if token is blacklisted
	blacklistKey := cache.PrefixSession + "blacklist:" + req.RefreshToken
	exists, _ := s.cache.Exists(ctx, blacklistKey)
	if exists {
		return nil, errors.Unauthorized("token has been revoked")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Blacklist old refresh token
	_ = s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)

	return s.generateAuthResponse(user)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	// Blacklist the refresh token
	blacklistKey := cache.PrefixSession + "blacklist:" + refreshToken
	return s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)
}

func (s *AuthService) ValidateToken(ctx context.Context, token string) (*authz.Claims, error) {
	return s.jwtManager.ValidateAccessToken(token)
}

func (s *AuthService) generateAuthResponse(user *domain.User) (*domain.AuthResponse, error) {
	tokenPair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	return &domain.AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
		User:         user,
	}, nil
}
