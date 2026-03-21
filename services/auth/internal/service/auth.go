package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

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

func (s *AuthService) LoginWithTelegram(ctx context.Context, tgUser *domain.TelegramWebhookUser) (*domain.AuthResponse, error) {
	// Try to find existing user by Telegram ID
	user, err := s.userRepo.GetByTelegramID(ctx, tgUser.ID)
	if err != nil {
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}

	if user == nil {
		// Create new user
		username := tgUser.Username
		if username == "" {
			username = fmt.Sprintf("tg_%d", tgUser.ID)
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

		telegramID := tgUser.ID
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
			"telegram_id", tgUser.ID,
			"username", username,
		)
	}

	return s.generateAuthResponse(user)
}

// CreateDeepLinkToken generates a unique token and stores an empty auth session in Redis.
// The returned deep link URL opens the Telegram bot with /start <token>.
func (s *AuthService) CreateDeepLinkToken(ctx context.Context, botName string) (*domain.DeepLinkResponse, error) {
	token := uuid.New().String()

	session := &domain.TelegramAuthSession{
		Status: "pending",
	}

	if err := s.cache.Set(ctx, cache.KeyTelegramAuth(token), session, cache.TTLTelegramAuth); err != nil {
		return nil, fmt.Errorf("store telegram auth session: %w", err)
	}

	deepLinkURL := fmt.Sprintf("https://t.me/%s?start=%s", botName, token)

	return &domain.DeepLinkResponse{
		Token:       token,
		DeepLinkURL: deepLinkURL,
		ExpiresIn:   int(cache.TTLTelegramAuth.Seconds()),
	}, nil
}

// CheckDeepLinkToken polls the status of a deep link auth session.
// Returns the session status and, if confirmed, completes login and returns auth tokens.
func (s *AuthService) CheckDeepLinkToken(ctx context.Context, token string) (*domain.DeepLinkCheckResponse, *domain.AuthResponse, error) {
	var session domain.TelegramAuthSession
	err := s.cache.Get(ctx, cache.KeyTelegramAuth(token), &session)
	if err != nil {
		return nil, nil, errors.NotFound("token not found or expired")
	}

	if session.Status != "confirmed" {
		return &domain.DeepLinkCheckResponse{
			Status: session.Status,
		}, nil, nil
	}

	// Session is confirmed — complete login
	tgUser := &domain.TelegramWebhookUser{
		ID:        session.TelegramID,
		FirstName: session.FirstName,
		LastName:  session.LastName,
		Username:  session.Username,
	}

	authResp, err := s.LoginWithTelegram(ctx, tgUser)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram login: %w", err)
	}

	// Delete the used token
	_ = s.cache.Delete(ctx, cache.KeyTelegramAuth(token))

	expiresAt := authResp.ExpiresAt
	checkResp := &domain.DeepLinkCheckResponse{
		Status:      "confirmed",
		AccessToken: authResp.AccessToken,
		ExpiresAt:   &expiresAt,
		User:        authResp.User,
	}

	return checkResp, authResp, nil
}

// HandleTelegramStart is called when a user sends /start <token> to the bot.
// It validates the token exists in Redis and records the Telegram user who started the flow.
func (s *AuthService) HandleTelegramStart(ctx context.Context, token string, tgUser *domain.TelegramWebhookUser) error {
	var session domain.TelegramAuthSession
	err := s.cache.Get(ctx, cache.KeyTelegramAuth(token), &session)
	if err != nil {
		return errors.NotFound("token not found or expired")
	}

	if session.Status != "pending" {
		return errors.InvalidInput("token already used")
	}

	session.Status = "started"
	session.TelegramID = tgUser.ID

	if err := s.cache.Set(ctx, cache.KeyTelegramAuth(token), &session, cache.TTLTelegramAuth); err != nil {
		return fmt.Errorf("update telegram auth session: %w", err)
	}

	return nil
}

// HandleTelegramCallback is called when the user clicks the "Confirm" button in the bot.
// It verifies the sender matches the user who started the flow and stores the confirmed session.
func (s *AuthService) HandleTelegramCallback(ctx context.Context, token string, tgUser *domain.TelegramWebhookUser) error {
	var session domain.TelegramAuthSession
	err := s.cache.Get(ctx, cache.KeyTelegramAuth(token), &session)
	if err != nil {
		return errors.NotFound("token not found or expired")
	}

	if session.Status != "started" {
		return errors.InvalidInput("token not in started state")
	}

	if session.TelegramID != tgUser.ID {
		return errors.Unauthorized("telegram user mismatch")
	}

	session.Status = "confirmed"
	session.FirstName = tgUser.FirstName
	session.LastName = tgUser.LastName
	session.Username = tgUser.Username

	if err := s.cache.Set(ctx, cache.KeyTelegramAuth(token), &session, cache.TTLTelegramAuth); err != nil {
		return fmt.Errorf("update telegram auth session: %w", err)
	}

	return nil
}

// GenerateApiKey generates a new API key for the user, replacing any existing one.
func (s *AuthService) GenerateApiKey(ctx context.Context, userID string) (string, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	rawKey := "ak_" + hex.EncodeToString(b)

	// SHA-256 hash for storage
	hash := sha256.Sum256([]byte(rawKey))
	hashStr := hex.EncodeToString(hash[:])

	if err := s.userRepo.UpdateApiKeyHash(ctx, userID, &hashStr); err != nil {
		return "", err
	}

	s.log.Infow("generated api key", "user_id", userID)
	return rawKey, nil
}

// RevokeApiKey removes the user's API key.
func (s *AuthService) RevokeApiKey(ctx context.Context, userID string) error {
	if err := s.userRepo.UpdateApiKeyHash(ctx, userID, nil); err != nil {
		return err
	}
	s.log.Infow("revoked api key", "user_id", userID)
	return nil
}

// ResolveApiKey validates an API key and returns claims for the associated user.
func (s *AuthService) ResolveApiKey(ctx context.Context, apiKey string) (*authz.Claims, error) {
	hash := sha256.Sum256([]byte(apiKey))
	hashStr := hex.EncodeToString(hash[:])

	user, err := s.userRepo.GetByApiKeyHash(ctx, hashStr)
	if err != nil {
		return nil, errors.Unauthorized("invalid api key")
	}

	return &authz.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, nil
}

// HasApiKey checks if the user has an API key configured.
func (s *AuthService) HasApiKey(ctx context.Context, userID string) (bool, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user.ApiKeyHash != nil, nil
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
