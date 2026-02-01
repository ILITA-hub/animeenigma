package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
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
	userRepo         *repo.UserRepository
	cache            *cache.RedisCache
	jwtManager       *authz.JWTManager
	telegramBotToken string
	log              *logger.Logger
}

func NewAuthService(
	userRepo *repo.UserRepository,
	cache *cache.RedisCache,
	jwtConfig authz.JWTConfig,
	telegramBotToken string,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		cache:            cache,
		jwtManager:       authz.NewJWTManager(jwtConfig),
		telegramBotToken: telegramBotToken,
		log:              log,
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

func (s *AuthService) LoginWithTelegram(ctx context.Context, req *domain.TelegramLoginRequest) (*domain.AuthResponse, error) {
	// Verify Telegram hash
	if !s.verifyTelegramAuth(req) {
		return nil, errors.Unauthorized("invalid telegram auth data")
	}

	// Check if auth_date is not too old (allow 1 day)
	if time.Now().Unix()-req.AuthDate > 86400 {
		return nil, errors.Unauthorized("telegram auth data expired")
	}

	// Try to find existing user by Telegram ID
	user, err := s.userRepo.GetByTelegramID(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}

	if user == nil {
		// Create new user
		username := req.Username
		if username == "" {
			username = fmt.Sprintf("tg_%d", req.ID)
		}

		// Check if username exists, append numbers if needed
		baseUsername := username
		for i := 1; ; i++ {
			exists, err := s.userRepo.ExistsByUsername(ctx, username)
			if err != nil {
				return nil, fmt.Errorf("check username: %w", err)
			}
			if !exists {
				break
			}
			username = fmt.Sprintf("%s_%d", baseUsername, i)
		}

		telegramID := req.ID
		user = &domain.User{
			Username:     username,
			PasswordHash: "", // No password for Telegram users
			TelegramID:   &telegramID,
			Role:         authz.RoleUser,
		}

		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}

		s.log.Infow("created new user via telegram",
			"user_id", user.ID,
			"telegram_id", req.ID,
			"username", username,
		)
	}

	return s.generateAuthResponse(user)
}

func (s *AuthService) verifyTelegramAuth(req *domain.TelegramLoginRequest) bool {
	if s.telegramBotToken == "" {
		return false
	}

	// Build data-check-string
	var params []string
	if req.AuthDate != 0 {
		params = append(params, fmt.Sprintf("auth_date=%d", req.AuthDate))
	}
	if req.FirstName != "" {
		params = append(params, fmt.Sprintf("first_name=%s", req.FirstName))
	}
	if req.ID != 0 {
		params = append(params, fmt.Sprintf("id=%d", req.ID))
	}
	if req.LastName != "" {
		params = append(params, fmt.Sprintf("last_name=%s", req.LastName))
	}
	if req.PhotoURL != "" {
		params = append(params, fmt.Sprintf("photo_url=%s", req.PhotoURL))
	}
	if req.Username != "" {
		params = append(params, fmt.Sprintf("username=%s", req.Username))
	}

	sort.Strings(params)
	dataCheckString := strings.Join(params, "\n")

	// Calculate secret key: SHA256(bot_token)
	secretKey := sha256.Sum256([]byte(s.telegramBotToken))

	// Calculate HMAC-SHA256
	h := hmac.New(sha256.New, secretKey[:])
	h.Write([]byte(dataCheckString))
	calculatedHash := hex.EncodeToString(h.Sum(nil))

	return calculatedHash == req.Hash
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
