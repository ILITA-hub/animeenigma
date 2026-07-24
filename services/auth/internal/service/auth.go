package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

// sessionHashFinder is satisfied by *repo.SessionRepository and by in-package
// test fakes — allows MintMagicToken to be unit-tested without a live DB.
type sessionHashFinder interface {
	FindAliveByHash(ctx context.Context, hash string) (*domain.UserSession, error)
	Create(ctx context.Context, s *domain.UserSession) error
}

// userByIDGetter is satisfied by *repo.UserRepository and by in-package
// test fakes — allows ConsumeMagicToken to be unit-tested without a live DB.
type userByIDGetter interface {
	GetByID(ctx context.Context, id string) (*domain.User, error)
}

type AuthService struct {
	userRepo         *repo.UserRepository
	sessionRepo      *repo.SessionRepository
	cache            cache.Cache
	jwtManager       *authz.JWTManager
	telegramBotToken string
	guestTokenTTL    time.Duration
	log              *logger.Logger

	// Login brute-force throttle (audit medium #6). Defaults set in
	// NewAuthService; overridable in tests.
	loginMaxFails   int
	loginFailWindow time.Duration

	// magicSessionFinder and magicUserGetter are set to the concrete repos by
	// NewAuthService but can be replaced by in-package test fakes to allow
	// MintMagicToken / ConsumeMagicToken unit tests without a live DB.
	magicSessionFinder sessionHashFinder
	magicUserGetter    userByIDGetter
}

func NewAuthService(
	userRepo *repo.UserRepository,
	sessionRepo *repo.SessionRepository,
	c cache.Cache,
	jwtConfig authz.JWTConfig,
	telegramBotToken string,
	guestTokenTTL time.Duration,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:           userRepo,
		sessionRepo:        sessionRepo,
		cache:              c,
		jwtManager:         authz.NewJWTManager(jwtConfig),
		telegramBotToken:   telegramBotToken,
		guestTokenTTL:      guestTokenTTL,
		log:                log,
		magicSessionFinder: sessionRepo,
		magicUserGetter:    userRepo,
		loginMaxFails:      10,
		loginFailWindow:    15 * time.Minute,
	}
}

// GuestSession mints an ephemeral, login-less guest identity used ONLY to
// JOIN a Watch Together room via invite link. It creates no DB user row and
// no refresh token — the returned access-only JWT carries authz.RoleGuest,
// which the gateway rejects on every protected route except the Watch
// Together routes (defense-in-depth; see gateway BlockGuestRoleMiddleware).
//
// The identity is throwaway: uid = "guest_" + uuid, username = "Guest-NNNN".
// A re-mint (when the token nears expiry) produces a NEW identity — acceptable
// for the MVP since the 6h default TTL makes mid-session churn rare.
func (s *AuthService) GuestSession(ctx context.Context) (*domain.PublicAuthResponse, error) {
	uid := "guest_" + uuid.NewString()

	// 4-digit display suffix → "Guest-1234". crypto/rand keeps it
	// collision-resistant across concurrent joins without seeding.
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("guest suffix: %w", err)
	}
	suffix := (int(b[0])<<8|int(b[1]))%9000 + 1000 // 1000..9999
	username := fmt.Sprintf("Guest-%d", suffix)

	token, err := s.jwtManager.GenerateGuestToken(uid, username, s.guestTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("generate guest token: %w", err)
	}

	return &domain.PublicAuthResponse{
		AccessToken: token,
		ExpiresAt:   time.Now().Add(s.guestTokenTTL),
		User: &domain.User{
			ID:       uid,
			Username: username,
			Role:     authz.RoleGuest,
		},
	}, nil
}

// SessionContext carries per-request context the service needs to create a
// session row. Login/Register/Telegram-confirm all populate this from
// the HTTP layer.
type SessionContext struct {
	UserAgent string
	IP        string
}

// SessionExpirySentinel makes a session effectively non-expiring. We keep the
// expires_at column (so no schema migration) but set it ~100 years out and
// never let it lapse for an active session. A session ends only on revoke.
const SessionExpirySentinel = 100 * 365 * 24 * time.Hour

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
	hashedPassword, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := &domain.User{
		Username:     req.Username,
		PasswordHash: hashedPassword,
		Role:         authz.RoleUser,
	}
	// Browser-detected zone from the sign-up request; invalid → leave empty
	// (frontend backfills on first login) rather than failing registration.
	if IsValidTimezone(req.Timezone) {
		user.Timezone = req.Timezone
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.createSessionAndAuthResponse(ctx, user, sc)
}

func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest, sc SessionContext) (*domain.AuthResponse, error) {
	// Reject over-long usernames before any DB or cache work. Registration
	// caps usernames at 32 chars, so anything longer can never match a real
	// account — treat it as an unknown user with the same generic error, and
	// never let it reach the throttle store (audit F13).
	if len(req.Username) > maxLoginUsernameLen {
		return nil, errors.Unauthorized("invalid credentials")
	}

	// Brute-force throttle (audit medium #6): reject before doing any work
	// once the per-account failure threshold is hit.
	if s.loginLocked(ctx, req.Username) {
		return nil, errors.New(errors.CodeRateLimited, "too many failed login attempts, please try again later")
	}

	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok && appErr.Code == errors.CodeNotFound {
			s.recordLoginFailure(ctx, req.Username)
			return nil, errors.Unauthorized("invalid credentials")
		}
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.recordLoginFailure(ctx, req.Username)
		return nil, errors.Unauthorized("invalid credentials")
	}

	// Successful authentication — clear the failure counter.
	s.clearLoginFailures(ctx, req.Username)

	// Opportunistic upgrade: if the stored hash uses a weaker cost than
	// the current policy, re-hash with the new cost and persist. Failures
	// here MUST NOT block the login.
	if NeedsRehash(user.PasswordHash) {
		if newHash, err := HashPassword(req.Password); err == nil {
			if updateErr := s.userRepo.UpdatePasswordHash(ctx, user.ID, newHash); updateErr != nil {
				s.log.Warnw("opportunistic rehash failed to persist", "user_id", user.ID, "error", updateErr)
			} else {
				user.PasswordHash = newHash
			}
		}
	}

	return s.createSessionAndAuthResponse(ctx, user, sc)
}

func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *domain.RefreshRequest,
	sc SessionContext,
) (*domain.AuthResponse, error) {
	hash := hashRefreshToken(req.RefreshToken)

	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err != nil {
		// Not alive / unknown / revoked — generic auth failure.
		return nil, errors.Unauthorized("invalid refresh token")
	}

	// Non-rotating: stamp activity, keep the same refresh token.
	now := time.Now()
	if terr := s.sessionRepo.Touch(ctx, session.ID, sc.IP, now, now.Add(SessionExpirySentinel)); terr != nil {
		return nil, terr
	}

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, session.ID)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	metrics.AuthEventsTotal.WithLabelValues("refresh_token", "success").Inc()
	return &domain.AuthResponse{
		AccessToken: pair.AccessToken,
		ExpiresAt:   pair.ExpiresAt,
		User:        user,
		// RefreshToken intentionally empty: the cookie value is unchanged.
	}, nil
}

// Logout revokes the session that owns this refresh token. Unknown tokens are
// a no-op (the cookie is cleared by the handler regardless).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	hash := hashRefreshToken(refreshToken)
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err != nil {
		return nil // unknown/already-dead token: nothing to revoke
	}
	if rerr := s.sessionRepo.Revoke(ctx, session.ID, session.UserID); rerr != nil {
		return rerr
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "logout").Inc()
	return nil
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

	// Persist the user's Telegram display identity on every login so admin
	// search (username / tg name) stays current. Best-effort: a cosmetic write
	// failure must never block login.
	if err := s.userRepo.UpdateTelegramProfile(ctx, user.ID, tgUser.Username, tgUser.FirstName); err != nil {
		s.log.Warnw("failed to persist telegram profile", "user_id", user.ID, "error", err)
	} else {
		if tgUser.Username != "" {
			v := tgUser.Username
			user.TelegramUsername = &v
		}
		if tgUser.FirstName != "" {
			v := tgUser.FirstName
			user.TelegramFirstName = &v
		}
	}

	return s.createSessionAndAuthResponse(ctx, user, sc)
}

// CreateDeepLinkToken generates a unique token and stores a pending auth session in Redis.
// The returned deep link URL opens the Telegram bot with /start <token>.
//
// It also mints a one-time browser-binding nonce: only the nonce's hash is
// stored in the session, and the raw nonce is returned to the caller so the
// HTTP layer can set it as an HttpOnly cookie in the minting browser. A later
// /check poll must present the matching cookie (see CheckDeepLinkToken), which
// stops a leaked token from being redeemed by a different browser (vector A).
//
// sc captures the requesting client (IP + User-Agent) so the bot's
// Confirm-login prompt can show where the login was requested from — letting a
// victim decline an attacker-initiated login they did not start (vector B).
func (s *AuthService) CreateDeepLinkToken(ctx context.Context, botName string, sc SessionContext) (*domain.DeepLinkResponse, string, error) {
	token := uuid.New().String()

	nonce, err := generateBindingNonce()
	if err != nil {
		return nil, "", fmt.Errorf("generate binding nonce: %w", err)
	}

	session := &domain.TelegramAuthSession{
		Status:    "pending",
		NonceHash: hashBindingNonce(nonce),
		RequestIP: sc.IP,
		RequestUA: truncateUA(sc.UserAgent),
	}

	if err := s.cache.Set(ctx, cache.KeyTelegramAuth(token), session, cache.TTLTelegramAuth); err != nil {
		return nil, "", fmt.Errorf("store telegram auth session: %w", err)
	}

	deepLinkURL := fmt.Sprintf("https://t.me/%s?start=%s", botName, token)

	return &domain.DeepLinkResponse{
		Token:       token,
		DeepLinkURL: deepLinkURL,
		ExpiresIn:   int(cache.TTLTelegramAuth.Seconds()),
	}, nonce, nil
}

// CheckDeepLinkToken polls the status of a deep link auth session.
// Returns the session status and, if confirmed, completes login and returns auth tokens.
//
// bindingNonce is the raw one-time nonce presented by the polling browser (read
// from the HttpOnly cookie set at mint time), or "" if absent. The pending
// session is bound to the browser that minted it: unless the presented nonce
// hashes to the stored NonceHash, we refuse to advance the flow — a token
// leaked in a URL/referer cannot be redeemed by a browser that never held the
// cookie (vector A). Sessions minted before browser-binding existed carry an
// empty NonceHash and are treated as unbound (fail-open) so an in-flight login
// survives a rolling deploy.
func (s *AuthService) CheckDeepLinkToken(ctx context.Context, token, bindingNonce string, sc SessionContext) (*domain.DeepLinkCheckResponse, *domain.AuthResponse, error) {
	var session domain.TelegramAuthSession
	err := s.cache.Get(ctx, cache.KeyTelegramAuth(token), &session)
	if err != nil {
		return nil, nil, errors.NotFound("token not found or expired")
	}

	// Browser-binding gate. On mismatch, return the same NotFound as an
	// unknown/expired token so a caller without the cookie cannot even tell the
	// token exists.
	if session.NonceHash != "" && !bindingNonceMatches(bindingNonce, session.NonceHash) {
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
// It validates the token exists in Redis and records the Telegram user who
// started the flow. It returns a human-readable summary of the client that
// requested the login (device + IP captured at mint time), which the bot shows
// in the Confirm-login prompt so a victim asked to confirm a login they did not
// start can spot the unfamiliar origin and decline (vector B). The summary is
// "" when no request context was captured.
func (s *AuthService) HandleTelegramStart(ctx context.Context, token string, tgUser *domain.TelegramWebhookUser) (string, error) {
	var session domain.TelegramAuthSession
	err := s.cache.Get(ctx, cache.KeyTelegramAuth(token), &session)
	if err != nil {
		return "", errors.NotFound("token not found or expired")
	}

	if session.Status != "pending" {
		return "", errors.InvalidInput("token already used")
	}

	session.Status = "started"
	session.TelegramID = tgUser.ID

	if err := s.cache.Set(ctx, cache.KeyTelegramAuth(token), &session, cache.TTLTelegramAuth); err != nil {
		return "", fmt.Errorf("update telegram auth session: %w", err)
	}

	return loginRequestOrigin(session.RequestIP, session.RequestUA), nil
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

// generateBindingNonce returns a fresh opaque one-time nonce (32 random bytes,
// hex-encoded) used to bind a deep-link token to the browser that minted it.
func generateBindingNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashBindingNonce returns the sha256-hex of a binding nonce. Only the hash is
// stored in Redis; the raw nonce lives solely in the browser's HttpOnly cookie.
func hashBindingNonce(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// bindingNonceMatches reports whether the presented raw nonce hashes to the
// stored hash, using a constant-time comparison. An empty presented nonce never
// matches a non-empty stored hash.
func bindingNonceMatches(nonce, storedHash string) bool {
	if nonce == "" || storedHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashBindingNonce(nonce)), []byte(storedHash)) == 1
}

// loginRequestOrigin renders a short, plain-text summary of the client that
// requested a deep-link login, for display in the bot's Confirm-login prompt.
// Returns "" when neither field was captured. The text is sent with no Telegram
// parse_mode, so the (attacker-influenced) User-Agent cannot inject markup.
func loginRequestOrigin(ip, ua string) string {
	if ip == "" && ua == "" {
		return ""
	}
	device := ua
	if device == "" {
		device = "unknown device"
	}
	const maxDisplayUA = 200
	if len(device) > maxDisplayUA {
		device = device[:maxDisplayUA] + "…"
	}
	if ip == "" {
		ip = "unknown"
	}
	return fmt.Sprintf("Device: %s\nIP address: %s", device, ip)
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
		ExpiresAt:        now.Add(SessionExpirySentinel),
	}
	if err := s.magicSessionFinder.Create(ctx, session); err != nil {
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

// GetUserByID exposes user lookup for sibling handlers (passkey/cert flows).
func (s *AuthService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

// SessionForUser mints a session for an already-authenticated user — the
// terminal step of passkey login (external proof happened in PasskeyService).
func (s *AuthService) SessionForUser(ctx context.Context, user *domain.User, sc SessionContext) (*domain.AuthResponse, error) {
	return s.createSessionAndAuthResponse(ctx, user, sc)
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
