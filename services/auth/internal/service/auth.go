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
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo         *repo.UserRepository
	sessionRepo      *repo.SessionRepository
	cache            *cache.RedisCache
	jwtManager       *authz.JWTManager
	telegramBotToken string
	log              *logger.Logger
}

func NewAuthService(
	userRepo *repo.UserRepository,
	sessionRepo *repo.SessionRepository,
	cache *cache.RedisCache,
	jwtConfig authz.JWTConfig,
	telegramBotToken string,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		sessionRepo:      sessionRepo,
		cache:            cache,
		jwtManager:       authz.NewJWTManager(jwtConfig),
		telegramBotToken: telegramBotToken,
		log:              log,
	}
}

// SessionContext carries per-request context the service needs to create a
// session row. Login/Register/Telegram-confirm all populate this from
// the HTTP layer.
type SessionContext struct {
	UserAgent string
	IP        string
}

// SessionTTL is the sliding-window length. Every refresh extends a session
// to now+SessionTTL. 30 days = "user opens the site at least once a month".
const SessionTTL = 30 * 24 * time.Hour

func (s *AuthService) Register(ctx context.Context, req *domain.RegisterRequest, sc SessionContext) (*domain.AuthResponse, error) {
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

	return s.createSessionAndAuthResponse(ctx, user, sc)
}

func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest, sc SessionContext) (*domain.AuthResponse, error) {
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

	return s.createSessionAndAuthResponse(ctx, user, sc)
}

func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *domain.RefreshRequest,
	sc SessionContext,
) (*domain.AuthResponse, bool, error) {
	rt := req.RefreshToken
	hash := hashRefreshToken(rt)

	// 1) Try persistent-session path.
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err == nil {
		newRT, err := generateRefreshToken()
		if err != nil {
			return nil, false, err
		}
		newHash := hashRefreshToken(newRT)

		result, err := s.sessionRepo.Rotate(ctx, session.ID, hash, newHash, sc.IP, time.Now().Add(SessionTTL))
		if err != nil {
			// Either CAS missed AND grace window expired, or session was
			// revoked between FindAliveByHash and Rotate. Either way, treat
			// as a generic auth failure — don't leak that the token was
			// once valid.
			return nil, false, errors.Unauthorized("invalid refresh token")
		}

		// Re-load user (cheap; could cache later)
		user, err := s.userRepo.GetByID(ctx, result.Session.UserID)
		if err != nil {
			return nil, false, err
		}

		pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, result.Session.ID)
		if err != nil {
			return nil, false, fmt.Errorf("generate tokens: %w", err)
		}

		resp := &domain.AuthResponse{
			AccessToken: pair.AccessToken,
			ExpiresAt:   pair.ExpiresAt,
			User:        user,
		}
		if result.Rotated {
			resp.RefreshToken = newRT
			metrics.AuthEventsTotal.WithLabelValues("refresh_token", "success").Inc()
		} else {
			// Grace path — caller must NOT issue a Set-Cookie for refresh.
			metrics.AuthEventsTotal.WithLabelValues("refresh_cas_miss", "grace_hit").Inc()
		}
		return resp, result.Rotated, nil
	}

	// 2) Legacy JWT path (transition window).
	userID, jwtErr := s.jwtManager.ValidateRefreshToken(rt)
	if jwtErr == nil {
		// Check legacy blacklist
		blacklistKey := cache.PrefixSession + "blacklist:" + rt
		if exists, _ := s.cache.Exists(ctx, blacklistKey); exists {
			return nil, false, errors.Unauthorized("token has been revoked")
		}
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, false, err
		}
		// Blacklist the legacy RT so it can't be reused.
		_ = s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)

		// Upgrade — mint a real session.
		resp, err := s.createSessionAndAuthResponse(ctx, user, sc)
		if err != nil {
			return nil, false, err
		}
		metrics.AuthEventsTotal.WithLabelValues("session_legacy_upgraded", "success").Inc()
		s.log.Infow("upgraded legacy refresh JWT to persistent session", "user_id", user.ID)
		return resp, true, nil
	}

	// 3) Both paths failed.
	return nil, false, errors.Unauthorized("invalid refresh token")
}

// Logout revokes the session that owns this refresh token. If the cookie
// is a legacy JWT, fall back to the old blacklist behavior.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	hash := hashRefreshToken(refreshToken)
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err == nil {
		if rerr := s.sessionRepo.Revoke(ctx, session.ID, session.UserID); rerr != nil {
			return rerr
		}
		metrics.AuthEventsTotal.WithLabelValues("session_revoked", "logout").Inc()
		return nil
	}
	// Legacy: blacklist the JWT.
	blacklistKey := cache.PrefixSession + "blacklist:" + refreshToken
	return s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)
}

func (s *AuthService) ValidateToken(ctx context.Context, token string) (*authz.Claims, error) {
	return s.jwtManager.ValidateAccessToken(token)
}

func (s *AuthService) LoginWithTelegram(ctx context.Context, tgUser *domain.TelegramWebhookUser, sc SessionContext) (*domain.AuthResponse, error) {
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

	return s.createSessionAndAuthResponse(ctx, user, sc)
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
func (s *AuthService) CheckDeepLinkToken(ctx context.Context, token string, sc SessionContext) (*domain.DeepLinkCheckResponse, *domain.AuthResponse, error) {
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

	authResp, err := s.LoginWithTelegram(ctx, tgUser, sc)
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

// SessionListItem is the public-facing shape returned to /api/auth/sessions.
type SessionListItem struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsCurrent  bool      `json:"is_current"`
}

func (s *AuthService) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionListItem, error) {
	rows, err := s.sessionRepo.ListAlive(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionListItem{
			ID:         r.ID,
			UserAgent:  r.UserAgent,
			IP:         r.IP,
			CreatedAt:  r.CreatedAt,
			LastSeenAt: r.LastSeenAt,
			ExpiresAt:  r.ExpiresAt,
			IsCurrent:  r.ID == currentSessionID,
		})
	}
	return out, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, userID, sessionID string) error {
	if err := s.sessionRepo.Revoke(ctx, sessionID, userID); err != nil {
		return err
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "user_action").Inc()
	return nil
}

func (s *AuthService) RevokeOtherSessions(ctx context.Context, userID, currentSessionID string) (int64, error) {
	n, err := s.sessionRepo.RevokeOthers(ctx, userID, currentSessionID)
	if err != nil {
		return 0, err
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "revoke_others").Add(float64(n))
	return n, nil
}

// CleanupExpiredSessions is called from a goroutine in main.go.
func (s *AuthService) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.sessionRepo.Cleanup(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		metrics.AuthEventsTotal.WithLabelValues("session_expired", "cleanup").Add(float64(n))
	}
	return n, nil
}

// --- helpers ---

const refreshTokenPrefix = "rt_"

// generateRefreshToken returns a fresh opaque refresh token like "rt_<64-hex>".
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return refreshTokenPrefix + hex.EncodeToString(b), nil
}

// hashRefreshToken returns the sha256-hex of a refresh token. Used as the
// row's refresh_token_hash. Never store the raw token.
func hashRefreshToken(rt string) string {
	sum := sha256.Sum256([]byte(rt))
	return hex.EncodeToString(sum[:])
}

// createSessionAndAuthResponse mints a fresh session row + tokens.
// Used by Login, Register, telegram-confirm — anywhere that's NOT a refresh.
func (s *AuthService) createSessionAndAuthResponse(
	ctx context.Context,
	user *domain.User,
	sc SessionContext,
) (*domain.AuthResponse, error) {
	rt, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	session := &domain.UserSession{
		UserID:           user.ID,
		RefreshTokenHash: hashRefreshToken(rt),
		UserAgent:        truncateUA(sc.UserAgent),
		IP:               sc.IP,
		LastSeenAt:       now,
		ExpiresAt:        now.Add(SessionTTL),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, session.ID)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	s.log.Infow("session created", "user_id", user.ID, "session_id", session.ID)
	metrics.AuthEventsTotal.WithLabelValues("session_created", "success").Inc()

	return &domain.AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: rt,
		ExpiresAt:    pair.ExpiresAt,
		User:         user,
	}, nil
}

// truncateUA caps user-agent length at 1024 to avoid pathological headers
// blowing up DB rows.
func truncateUA(ua string) string {
	const maxUA = 1024
	if len(ua) > maxUA {
		return ua[:maxUA]
	}
	return ua
}
