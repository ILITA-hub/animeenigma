# Telegram Deep Link + QR Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Telegram Login Widget (browser iframe) with a deep link + QR code flow that opens the native Telegram app for authentication.

**Architecture:** Backend generates a short-lived token stored in Redis. Frontend shows a QR code / deep link button. User opens the bot in Telegram, confirms login. Telegram webhook fires, backend stores user data in Redis. Frontend polls for confirmation and completes login.

**Tech Stack:** Go (auth service), Vue 3 + TypeScript (frontend), Redis (token storage), Telegram Bot API (webhook, sendMessage, answerCallbackQuery), `qrcode` npm package (QR rendering)

**Spec:** `docs/superpowers/specs/2026-03-22-telegram-deeplink-auth-design.md`

---

## File Structure

### New Files
- `services/auth/internal/handler/telegram_bot.go` — Telegram Bot API HTTP helpers + webhook handler
- (no other new files)

### Modified Files
- `libs/cache/ttl.go` — Add `PrefixTelegramAuth` and `TTLTelegramAuth`
- `services/auth/internal/config/config.go` — Add `WebhookSecret`, `WebhookURL` to `TelegramConfig`
- `services/auth/internal/domain/user.go` — Replace `TelegramLoginRequest` with `TelegramWebhookUser`, add deeplink types
- `services/auth/internal/service/auth.go` — Add `CreateDeepLinkToken`, `CheckDeepLinkToken`, `HandleTelegramStart`, `HandleTelegramCallback`; remove `verifyTelegramAuth`
- `services/auth/internal/handler/auth.go` — Add `DeepLink`, `CheckDeepLink` handlers; remove `TelegramLogin`, `GetTelegramConfig`
- `services/auth/internal/transport/router.go` — Update routes
- `services/auth/cmd/auth-api/main.go` — Call `setWebhook` on startup
- `docker/.env` — Add `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_WEBHOOK_URL`
- `docker/docker-compose.yml` — Pass new env vars to auth service
- `frontend/web/src/stores/auth.ts` — Replace `loginWithTelegram` with `requestDeepLink` + `checkDeepLink`
- `frontend/web/src/views/Auth.vue` — QR + deep link + polling UI
- `frontend/web/src/locales/en.json` — New auth i18n keys
- `frontend/web/src/locales/ru.json` — New auth i18n keys
- `frontend/web/src/locales/ja.json` — New auth i18n keys

---

## Task 1: Add Redis constants for Telegram auth tokens

**Files:**
- Modify: `libs/cache/ttl.go`

- [ ] **Step 1: Add TTL and prefix constants**

In `libs/cache/ttl.go`, add to the `var` block after `TTLUserOnline`:

```go
TTLTelegramAuth = 5 * time.Minute
```

Add to the `const` block after `PrefixRoom`:

```go
PrefixTelegramAuth = "tgauth:"
```

Add key builder after `KeyRoom`:

```go
func KeyTelegramAuth(token string) string {
	return PrefixTelegramAuth + token
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /data/animeenigma && go build ./libs/cache/...`
Expected: Success, no errors.

- [ ] **Step 3: Commit**

```bash
git add libs/cache/ttl.go
git commit -m "feat(cache): add Telegram auth token prefix and TTL constants"
```

---

## Task 2: Extend auth config with webhook settings

**Files:**
- Modify: `services/auth/internal/config/config.go`

- [ ] **Step 1: Add webhook fields to TelegramConfig**

Change the `TelegramConfig` struct at line 23-26 to:

```go
type TelegramConfig struct {
	BotToken       string
	BotName        string
	WebhookSecret  string
	WebhookURL     string
}
```

- [ ] **Step 2: Load new env vars in `Load()`**

In the `Load()` function, update the `Telegram` block (line 78-81) to:

```go
Telegram: TelegramConfig{
	BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
	BotName:       getEnv("TELEGRAM_BOT_NAME", ""),
	WebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
	WebhookURL:    getEnv("TELEGRAM_WEBHOOK_URL", ""),
},
```

- [ ] **Step 3: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Success.

- [ ] **Step 4: Commit**

```bash
git add services/auth/internal/config/config.go
git commit -m "feat(auth): add webhook secret and URL to Telegram config"
```

---

## Task 3: Add domain types for deep link auth

**Files:**
- Modify: `services/auth/internal/domain/user.go`

- [ ] **Step 1: Replace TelegramLoginRequest and add new types**

Replace the `TelegramLoginRequest` struct (lines 55-64) with:

```go
// TelegramWebhookUser represents user data received from Telegram webhook.
// Unlike the old TelegramLoginRequest, this has no AuthDate/Hash fields
// because webhook data is authenticated via the secret token header.
type TelegramWebhookUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// DeepLinkResponse is returned when creating a new deep link auth session.
type DeepLinkResponse struct {
	Token       string `json:"token"`
	DeepLinkURL string `json:"deeplink_url"`
	ExpiresIn   int    `json:"expires_in"`
}

// DeepLinkCheckResponse is returned when polling for deep link auth status.
type DeepLinkCheckResponse struct {
	Status      string              `json:"status"` // "pending", "confirmed", "expired"
	AccessToken string              `json:"access_token,omitempty"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	User        *User               `json:"user,omitempty"`
}

// TelegramAuthSession represents the data stored in Redis for a pending/confirmed auth session.
type TelegramAuthSession struct {
	Status     string `json:"status"` // "", "started", "confirmed"
	TelegramID int64  `json:"telegram_id,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Username   string `json:"username,omitempty"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Compilation errors in `service/auth.go` referencing old `TelegramLoginRequest` — expected, will be fixed in Task 4.

- [ ] **Step 3: Commit**

```bash
git add services/auth/internal/domain/user.go
git commit -m "feat(auth): add domain types for deep link Telegram auth"
```

---

## Task 4: Implement deep link service methods

**Files:**
- Modify: `services/auth/internal/service/auth.go`

- [ ] **Step 1: Update imports**

Add `"github.com/google/uuid"` to imports. Remove `"crypto/hmac"`, `"crypto/subtle"`, `"sort"` (only used by `verifyTelegramAuth`). Keep `"crypto/sha256"` (still used by API key methods). The full import block should be:

```go
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
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
```

- [ ] **Step 2: Remove `verifyTelegramAuth` method**

Delete the entire `verifyTelegramAuth` method (lines 194-232).

- [ ] **Step 3: Rewrite `LoginWithTelegram` to accept `TelegramWebhookUser`**

Replace the `LoginWithTelegram` method (lines 135-192) with:

```go
// LoginWithTelegram finds or creates a user by Telegram ID.
// Used by the webhook flow — data is trusted (authenticated by webhook secret).
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
```

- [ ] **Step 4: Add `CreateDeepLinkToken` method**

Add after `LoginWithTelegram`:

```go
// CreateDeepLinkToken generates a UUID token for deep link auth and stores it in Redis.
func (s *AuthService) CreateDeepLinkToken(ctx context.Context, botName string) (*domain.DeepLinkResponse, error) {
	token := uuid.New().String()
	key := cache.KeyTelegramAuth(token)

	// Store empty session (pending state)
	session := &domain.TelegramAuthSession{Status: ""}
	if err := s.cache.Set(ctx, key, session, cache.TTLTelegramAuth); err != nil {
		return nil, fmt.Errorf("store deep link token: %w", err)
	}

	deepLinkURL := fmt.Sprintf("https://t.me/%s?start=%s", botName, token)

	return &domain.DeepLinkResponse{
		Token:       token,
		DeepLinkURL: deepLinkURL,
		ExpiresIn:   int(cache.TTLTelegramAuth.Seconds()),
	}, nil
}
```

- [ ] **Step 5: Add `CheckDeepLinkToken` method**

Add after `CreateDeepLinkToken`. Returns both the check response (for the HTTP response body) AND the full auth response (so the handler can set cookies with the refresh token):

```go
// CheckDeepLinkToken checks the status of a deep link auth token.
// Returns both the public check response and the full auth response (for cookie setting).
func (s *AuthService) CheckDeepLinkToken(ctx context.Context, token string) (*domain.DeepLinkCheckResponse, *domain.AuthResponse, error) {
	key := cache.KeyTelegramAuth(token)

	var session domain.TelegramAuthSession
	if err := s.cache.Get(ctx, key, &session); err != nil {
		return &domain.DeepLinkCheckResponse{Status: "expired"}, nil, nil
	}

	if session.Status != "confirmed" {
		return &domain.DeepLinkCheckResponse{Status: "pending"}, nil, nil
	}

	// Token confirmed — login the user
	tgUser := &domain.TelegramWebhookUser{
		ID:        session.TelegramID,
		FirstName: session.FirstName,
		LastName:  session.LastName,
		Username:  session.Username,
	}

	authResp, err := s.LoginWithTelegram(ctx, tgUser)
	if err != nil {
		return nil, nil, err
	}

	// Delete the token (single-use)
	_ = s.cache.Delete(ctx, key)

	checkResp := &domain.DeepLinkCheckResponse{
		Status:      "confirmed",
		AccessToken: authResp.AccessToken,
		ExpiresAt:   &authResp.ExpiresAt,
		User:        authResp.User,
	}

	return checkResp, authResp, nil
}
```

- [ ] **Step 6: Add `HandleTelegramStart` method**

Add after `CheckDeepLinkToken`:

```go
// HandleTelegramStart processes a /start command from the Telegram bot.
// Stores the sender's Telegram ID in the session and returns the chat ID for the bot to message.
func (s *AuthService) HandleTelegramStart(ctx context.Context, token string, tgUser *domain.TelegramWebhookUser) error {
	key := cache.KeyTelegramAuth(token)

	var session domain.TelegramAuthSession
	if err := s.cache.Get(ctx, key, &session); err != nil {
		return errors.NotFound("auth token expired or not found")
	}

	// Store the Telegram user ID to prevent QR phishing
	session.Status = "started"
	session.TelegramID = tgUser.ID

	if err := s.cache.Set(ctx, key, &session, cache.TTLTelegramAuth); err != nil {
		return fmt.Errorf("update auth session: %w", err)
	}

	return nil
}
```

- [ ] **Step 7: Add `HandleTelegramCallback` method**

Add after `HandleTelegramStart`:

```go
// HandleTelegramCallback processes a confirm callback from the Telegram bot.
// Verifies the sender matches the /start user, then stores full user data.
func (s *AuthService) HandleTelegramCallback(ctx context.Context, token string, tgUser *domain.TelegramWebhookUser) error {
	key := cache.KeyTelegramAuth(token)

	var session domain.TelegramAuthSession
	if err := s.cache.Get(ctx, key, &session); err != nil {
		return errors.NotFound("auth token expired or not found")
	}

	// Verify sender matches the user who started the flow
	if session.TelegramID != 0 && session.TelegramID != tgUser.ID {
		return errors.Forbidden("this login session belongs to a different user")
	}

	// Store confirmed session with full user data
	session.Status = "confirmed"
	session.TelegramID = tgUser.ID
	session.FirstName = tgUser.FirstName
	session.LastName = tgUser.LastName
	session.Username = tgUser.Username

	if err := s.cache.Set(ctx, key, &session, cache.TTLTelegramAuth); err != nil {
		return fmt.Errorf("confirm auth session: %w", err)
	}

	return nil
}
```

- [ ] **Step 8: Add `uuid` dependency**

Run: `cd /data/animeenigma/services/auth && go get github.com/google/uuid`

- [ ] **Step 9: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Compilation errors in `handler/auth.go` referencing old `TelegramLogin`/`GetTelegramConfig` — expected, fixed in Task 5.

- [ ] **Step 10: Commit**

```bash
git add services/auth/internal/service/auth.go services/auth/go.mod services/auth/go.sum
git commit -m "feat(auth): add deep link token service methods, remove widget verification"
```

---

## Task 5: Create Telegram bot webhook handler

**Files:**
- Create: `services/auth/internal/handler/telegram_bot.go`

- [ ] **Step 1: Create the file**

Create `services/auth/internal/handler/telegram_bot.go` with Telegram Bot API helpers and the webhook HTTP handler:

```go
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// TelegramBotHandler handles Telegram webhook updates and bot API calls.
type TelegramBotHandler struct {
	authService   *service.AuthService
	botToken      string
	webhookSecret string
	log           *logger.Logger
}

func NewTelegramBotHandler(authService *service.AuthService, botToken, webhookSecret string, log *logger.Logger) *TelegramBotHandler {
	return &TelegramBotHandler{
		authService:   authService,
		botToken:      botToken,
		webhookSecret: webhookSecret,
		log:           log,
	}
}

// --- Telegram Bot API types ---

type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message,omitempty"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query,omitempty"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`
}

type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    telegramUser     `json:"from"`
	Data    string           `json:"data"`
	Message *telegramMessage `json:"message,omitempty"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramInlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type telegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]telegramInlineKeyboardButton `json:"inline_keyboard"`
}

// --- Webhook handler ---

// HandleWebhook processes incoming Telegram webhook updates.
func (h *TelegramBotHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verify webhook secret
	if h.webhookSecret != "" {
		secretHeader := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if secretHeader != h.webhookSecret {
			h.log.Warnw("telegram webhook: invalid secret token")
			w.WriteHeader(http.StatusOK) // Always 200 to prevent retries
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Errorw("telegram webhook: failed to read body", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	var update telegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		h.log.Errorw("telegram webhook: failed to parse update", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/start ") {
		h.handleStart(r, update.Message)
	} else if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "confirm_login:") {
		h.handleCallbackConfirm(r, update.CallbackQuery)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *TelegramBotHandler) handleStart(r *http.Request, msg *telegramMessage) {
	token := strings.TrimPrefix(msg.Text, "/start ")
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	tgUser := &domain.TelegramWebhookUser{
		ID:        msg.From.ID,
		FirstName: msg.From.FirstName,
		LastName:  msg.From.LastName,
		Username:  msg.From.Username,
	}

	if err := h.authService.HandleTelegramStart(r.Context(), token, tgUser); err != nil {
		// Token expired or not found — inform user
		h.sendMessage(msg.Chat.ID, "This login link has expired. Please request a new one from the website.")
		return
	}

	// Send confirmation prompt
	h.sendMessageWithConfirmButton(msg.Chat.ID, token)
}

func (h *TelegramBotHandler) handleCallbackConfirm(r *http.Request, cb *telegramCallbackQuery) {
	token := strings.TrimPrefix(cb.Data, "confirm_login:")
	if token == "" {
		return
	}

	tgUser := &domain.TelegramWebhookUser{
		ID:        cb.From.ID,
		FirstName: cb.From.FirstName,
		LastName:  cb.From.LastName,
		Username:  cb.From.Username,
	}

	if err := h.authService.HandleTelegramCallback(r.Context(), token, tgUser); err != nil {
		// Answer callback with error
		h.answerCallbackQuery(cb.ID, err.Error())
		// Edit the message to show error
		if cb.Message != nil {
			h.editMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Session expired. Please request a new login link from the website.")
		}
		return
	}

	// Answer callback
	h.answerCallbackQuery(cb.ID, "Login confirmed!")

	// Edit message to show success
	if cb.Message != nil {
		h.editMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Login confirmed! You can now close this chat.")
	}
}

// --- Telegram Bot API calls ---

func (h *TelegramBotHandler) sendMessage(chatID int64, text string) {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	h.callBotAPI("sendMessage", payload)
}

func (h *TelegramBotHandler) sendMessageWithConfirmButton(chatID int64, token string) {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    "You are logging into AnimeEnigma.\n\nClick the button below to confirm:",
		"reply_markup": telegramInlineKeyboardMarkup{
			InlineKeyboard: [][]telegramInlineKeyboardButton{
				{
					{Text: "Confirm login", CallbackData: "confirm_login:" + token},
				},
			},
		},
	}
	h.callBotAPI("sendMessage", payload)
}

func (h *TelegramBotHandler) answerCallbackQuery(callbackQueryID string, text string) {
	payload := map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
	}
	h.callBotAPI("answerCallbackQuery", payload)
}

func (h *TelegramBotHandler) editMessageText(chatID int64, messageID int64, text string) {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	h.callBotAPI("editMessageText", payload)
}

func (h *TelegramBotHandler) callBotAPI(method string, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Errorw("telegram bot api: marshal error", "method", method, "error", err)
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", h.botToken, method)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		h.log.Errorw("telegram bot api: request error", "method", method, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		h.log.Warnw("telegram bot api: non-200 response",
			"method", method,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
	}
}

// SetWebhook registers the webhook URL with Telegram. Called on startup.
func (h *TelegramBotHandler) SetWebhook(webhookURL string) error {
	payload := map[string]interface{}{
		"url":            webhookURL,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if h.webhookSecret != "" {
		payload["secret_token"] = h.webhookSecret
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setWebhook payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", h.botToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("setWebhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setWebhook failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Still fails due to handler/auth.go referencing old methods. Expected.

- [ ] **Step 3: Commit**

```bash
git add services/auth/internal/handler/telegram_bot.go
git commit -m "feat(auth): add Telegram bot webhook handler and Bot API helpers"
```

---

## Task 6: Update auth handler with deep link endpoints

**Files:**
- Modify: `services/auth/internal/handler/auth.go`

- [ ] **Step 1: Remove `TelegramLogin` handler**

Delete the `TelegramLogin` method (lines 220-245).

- [ ] **Step 2: Remove `GetTelegramConfig` handler**

Delete the `GetTelegramConfig` method (lines 324-338, after step 1 the line numbers will have shifted).

- [ ] **Step 3: Add `DeepLink` and `CheckDeepLink` handlers**

Add after `Logout`:

```go
// DeepLink generates a new deep link auth token for Telegram login.
func (h *AuthHandler) DeepLink(w http.ResponseWriter, r *http.Request) {
	resp, err := h.authService.CreateDeepLinkToken(r.Context(), h.telegramConfig.BotName)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, resp)
}

// CheckDeepLink polls for deep link auth status.
func (h *AuthHandler) CheckDeepLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		httputil.Error(w, errors.InvalidInput("token is required"))
		return
	}

	checkResp, authResp, err := h.authService.CheckDeepLinkToken(r.Context(), token)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// If confirmed, set cookies (same as login flow)
	if checkResp.Status == "confirmed" && authResp != nil {
		metrics.AuthEventsTotal.WithLabelValues("telegram_deeplink", "success").Inc()
		h.setRefreshTokenCookie(w, authResp.RefreshToken)
		h.setAccessTokenCookie(w, authResp.AccessToken, authResp.ExpiresAt)
	}

	httputil.OK(w, checkResp)
}
```

- [ ] **Step 4: Remove unused imports**

Remove `"strings"` from imports in auth.go (was used by `GetTelegramConfig`). Keep all other imports.

- [ ] **Step 5: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Compilation errors in router.go referencing old methods. Expected.

- [ ] **Step 6: Commit**

```bash
git add services/auth/internal/handler/auth.go services/auth/internal/service/auth.go
git commit -m "feat(auth): add deep link and check handlers, remove old widget handlers"
```

---

## Task 7: Update router

**Files:**
- Modify: `services/auth/internal/transport/router.go`

- [ ] **Step 1: Add TelegramBotHandler parameter to NewRouter**

Update `NewRouter` signature to accept the new handler:

```go
func NewRouter(
	authHandler *handler.AuthHandler,
	userHandler *handler.UserHandler,
	telegramBotHandler *handler.TelegramBotHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
```

- [ ] **Step 2: Replace old Telegram routes with new ones**

Replace lines 54-55:

```go
r.Post("/telegram", authHandler.TelegramLogin)
r.Get("/telegram/config", authHandler.GetTelegramConfig)
```

With:

```go
r.Post("/telegram/deeplink", authHandler.DeepLink)
r.Get("/telegram/check", authHandler.CheckDeepLink)
r.Post("/telegram/webhook", telegramBotHandler.HandleWebhook)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Compilation error in main.go due to NewRouter signature change. Expected.

- [ ] **Step 4: Commit**

```bash
git add services/auth/internal/transport/router.go
git commit -m "feat(auth): update routes for deep link Telegram auth"
```

---

## Task 8: Update main.go — wire up and set webhook

**Files:**
- Modify: `services/auth/cmd/auth-api/main.go`

- [ ] **Step 1: Create TelegramBotHandler and pass to router**

After the `authHandler` initialization (line 67), add:

```go
telegramBotHandler := handler.NewTelegramBotHandler(authService, cfg.Telegram.BotToken, cfg.Telegram.WebhookSecret, log)
```

Update the `transport.NewRouter` call (line 74) to include the new handler:

```go
router := transport.NewRouter(authHandler, userHandler, telegramBotHandler, cfg.JWT, log, metricsCollector)
```

- [ ] **Step 2: Set webhook on startup**

After the router initialization and before the server creation (before line 77), add:

```go
// Register Telegram webhook if configured
if cfg.Telegram.WebhookURL != "" && cfg.Telegram.BotToken != "" {
	if err := telegramBotHandler.SetWebhook(cfg.Telegram.WebhookURL); err != nil {
		log.Warnw("failed to set telegram webhook, deep link login will not work", "error", err)
	} else {
		log.Infow("telegram webhook registered", "url", cfg.Telegram.WebhookURL)
	}
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: Success — all Go code compiles.

- [ ] **Step 4: Commit**

```bash
git add services/auth/cmd/auth-api/main.go
git commit -m "feat(auth): wire up Telegram bot handler and set webhook on startup"
```

---

## Task 9: Update Docker config

**Files:**
- Modify: `docker/.env`
- Modify: `docker/docker-compose.yml`

- [ ] **Step 1: Add new env vars to docker/.env**

Append to `docker/.env`:

```
TELEGRAM_WEBHOOK_SECRET=animeenigma_webhook_secret_changeme
TELEGRAM_WEBHOOK_URL=https://animeenigma.ru/api/auth/telegram/webhook
```

- [ ] **Step 2: Pass new env vars in docker-compose.yml**

In the `auth` service environment block (after line 298 `TELEGRAM_BOT_NAME`), add:

```yaml
      TELEGRAM_WEBHOOK_SECRET: ${TELEGRAM_WEBHOOK_SECRET:-}
      TELEGRAM_WEBHOOK_URL: ${TELEGRAM_WEBHOOK_URL:-}
```

- [ ] **Step 3: Commit**

```bash
git add docker/.env docker/docker-compose.yml
git commit -m "feat(docker): add Telegram webhook env vars for auth service"
```

---

## Task 10: Update frontend auth store

**Files:**
- Modify: `frontend/web/src/stores/auth.ts`

- [ ] **Step 1: Remove `TelegramAuthData` interface and `loginWithTelegram` method**

Remove the `TelegramAuthData` interface (lines 27-35).

Remove the `loginWithTelegram` method (lines 135-152).

Remove `loginWithTelegram` from the return object (line 209).

- [ ] **Step 2: Add deep link methods**

Add these two new methods before `logout`:

```typescript
  const requestDeepLink = async (): Promise<{ token: string; deeplink_url: string; expires_in: number } | null> => {
    error.value = null
    try {
      const response = await apiClient.post('/auth/telegram/deeplink')
      return response.data?.data || response.data
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
      error.value = e.response?.data?.error?.message || e.response?.data?.message || i18n.global.t('auth.telegramLoginError')
      return null
    }
  }

  const checkDeepLink = async (token: string): Promise<{ status: string; access_token?: string; user?: User } | null> => {
    try {
      const response = await apiClient.get(`/auth/telegram/check?token=${token}`)
      const data = response.data?.data || response.data
      if (data.status === 'confirmed' && data.access_token) {
        setToken(data.access_token)
        setUser(data.user)
      }
      return data
    } catch {
      return { status: 'expired' }
    }
  }
```

- [ ] **Step 3: Update return object**

Replace `loginWithTelegram,` in the return object with:

```typescript
    requestDeepLink,
    checkDeepLink,
```

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/stores/auth.ts
git commit -m "feat(frontend): replace Telegram widget auth with deep link polling in auth store"
```

---

## Task 11: Install QR code package

**Files:**
- Modify: `frontend/web/package.json`

- [ ] **Step 1: Install qrcode**

Run: `cd /data/animeenigma/frontend/web && bun add qrcode @types/qrcode`

- [ ] **Step 2: Commit**

```bash
git add frontend/web/package.json frontend/web/bun.lock
git commit -m "feat(frontend): add qrcode dependency for deep link auth"
```

---

## Task 12: Rewrite Auth.vue with QR + deep link + polling

**Files:**
- Modify: `frontend/web/src/views/Auth.vue`

- [ ] **Step 1: Replace the entire file**

Replace `frontend/web/src/views/Auth.vue` with:

```vue
<template>
  <div class="min-h-screen flex items-center justify-center px-4 py-12">
    <!-- Background -->
    <div class="fixed inset-0 -z-10">
      <div class="absolute inset-0 bg-gradient-to-br from-base via-surface to-base" />
      <div class="absolute top-1/4 left-1/4 w-96 h-96 bg-cyan-500/10 rounded-full blur-3xl" />
      <div class="absolute bottom-1/4 right-1/4 w-96 h-96 bg-pink-500/10 rounded-full blur-3xl" />
    </div>

    <div class="w-full max-w-md">
      <!-- Logo -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center gap-2 text-2xl font-bold">
          <span class="text-cyan-400">Anime</span>
          <span class="text-white">Enigma</span>
        </router-link>
      </div>

      <!-- Auth Card -->
      <div class="glass-card p-6 md:p-8">
        <h2 class="text-center text-white text-lg font-medium mb-6">{{ $t('auth.telegramLogin') }}</h2>

        <!-- Error -->
        <div v-if="authStore.error" class="mb-4 p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
          {{ authStore.error }}
        </div>

        <!-- Loading state -->
        <div v-if="state === 'loading'" class="flex flex-col items-center gap-3">
          <div class="animate-spin h-8 w-8 border-2 border-cyan-400 border-t-transparent rounded-full" />
          <span class="text-white/40 text-sm">{{ $t('auth.loading') }}</span>
        </div>

        <!-- QR + Deep link -->
        <div v-else-if="state === 'ready'" class="flex flex-col items-center gap-5">
          <!-- QR Code -->
          <div class="bg-white rounded-xl p-3">
            <canvas ref="qrCanvas" />
          </div>

          <!-- Open in Telegram button -->
          <a
            :href="deeplinkUrl"
            class="inline-flex items-center gap-2 px-6 py-3 bg-[#54a9eb] hover:bg-[#4a96d2] text-white font-medium rounded-lg transition-colors w-full justify-center"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
            </svg>
            {{ $t('auth.openInTelegram') }}
          </a>

          <!-- Timer -->
          <p class="text-white/30 text-xs">
            {{ $t('auth.expiresIn', { seconds: remainingSeconds }) }}
          </p>

          <!-- Troubleshooting hint (shows after 30 seconds) -->
          <p v-if="showTroubleshootingHint" class="text-white/30 text-xs text-center">
            {{ $t('auth.troubleshootingHint') }}
          </p>
        </div>

        <!-- Expired state -->
        <div v-else-if="state === 'expired'" class="flex flex-col items-center gap-4">
          <p class="text-white/50 text-sm">{{ $t('auth.sessionExpired') }}</p>
          <button
            class="px-6 py-3 bg-cyan-500 hover:bg-cyan-600 text-white font-medium rounded-lg transition-colors"
            @click="startAuth"
          >
            {{ $t('auth.tryAgain') }}
          </button>
        </div>
      </div>

      <!-- Back to home -->
      <p class="text-center mt-6 text-white/40 text-sm">
        <router-link to="/" class="hover:text-white transition-colors">
          {{ '\u2190 ' + $t('auth.backHome') }}
        </router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import QRCode from 'qrcode'

useI18n()

const router = useRouter()
const authStore = useAuthStore()
const qrCanvas = ref<HTMLCanvasElement | null>(null)

const state = ref<'loading' | 'ready' | 'expired'>('loading')
const deeplinkUrl = ref('')
const authToken = ref('')
const remainingSeconds = ref(300)
const showTroubleshootingHint = ref(false)

let pollInterval: ReturnType<typeof setInterval> | null = null
let countdownInterval: ReturnType<typeof setInterval> | null = null
let troubleshootingTimeout: ReturnType<typeof setTimeout> | null = null
let pollCount = 0
const MAX_POLLS = 150

async function startAuth() {
  state.value = 'loading'
  showTroubleshootingHint.value = false
  pollCount = 0
  cleanup()

  const result = await authStore.requestDeepLink()
  if (!result) {
    state.value = 'expired'
    return
  }

  authToken.value = result.token
  deeplinkUrl.value = result.deeplink_url
  remainingSeconds.value = result.expires_in
  state.value = 'ready'

  // Render QR code after DOM update
  await nextTick()
  if (qrCanvas.value) {
    QRCode.toCanvas(qrCanvas.value, result.deeplink_url, {
      width: 200,
      margin: 0,
      color: { dark: '#000000', light: '#ffffff' },
    })
  }

  // Start polling
  pollInterval = setInterval(pollForAuth, 2000)

  // Start countdown
  countdownInterval = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      state.value = 'expired'
      cleanup()
    }
  }, 1000)

  // Show troubleshooting hint after 30 seconds
  troubleshootingTimeout = setTimeout(() => {
    showTroubleshootingHint.value = true
  }, 30000)
}

async function pollForAuth() {
  if (!authToken.value || pollCount >= MAX_POLLS) {
    state.value = 'expired'
    cleanup()
    return
  }

  pollCount++
  const result = await authStore.checkDeepLink(authToken.value)

  if (result?.status === 'confirmed') {
    cleanup()
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.push(returnUrl || '/')
  } else if (result?.status === 'expired') {
    state.value = 'expired'
    cleanup()
  }
}

function cleanup() {
  if (pollInterval) { clearInterval(pollInterval); pollInterval = null }
  if (countdownInterval) { clearInterval(countdownInterval); countdownInterval = null }
  if (troubleshootingTimeout) { clearTimeout(troubleshootingTimeout); troubleshootingTimeout = null }
}

onMounted(() => {
  startAuth()
})

onUnmounted(() => {
  cleanup()
})
</script>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/web/src/views/Auth.vue
git commit -m "feat(frontend): replace Telegram widget with QR code + deep link auth UI"
```

---

## Task 13: Update i18n locale files

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

- [ ] **Step 1: Update English translations**

Replace the `"auth"` block in `en.json` with:

```json
"auth": {
  "telegramLogin": "Login via Telegram",
  "loading": "Loading...",
  "backHome": "Back to Home",
  "loginError": "Login error",
  "registerError": "Registration error",
  "telegramLoginError": "Telegram login error",
  "openInTelegram": "Open in Telegram",
  "expiresIn": "Expires in {seconds}s",
  "sessionExpired": "Session expired",
  "tryAgain": "Try again",
  "troubleshootingHint": "Having trouble? Make sure you haven't blocked @animeenigma_auth_bot in Telegram."
}
```

- [ ] **Step 2: Update Russian translations**

Replace the `"auth"` block in `ru.json` with:

```json
"auth": {
  "telegramLogin": "Войти через Telegram",
  "loading": "Загрузка...",
  "backHome": "Вернуться на главную",
  "loginError": "Ошибка входа",
  "registerError": "Ошибка регистрации",
  "telegramLoginError": "Ошибка входа через Telegram",
  "openInTelegram": "Открыть в Telegram",
  "expiresIn": "Истекает через {seconds} сек",
  "sessionExpired": "Сессия истекла",
  "tryAgain": "Попробовать снова",
  "troubleshootingHint": "Не получается? Убедитесь, что вы не заблокировали @animeenigma_auth_bot в Telegram."
}
```

- [ ] **Step 3: Update Japanese translations**

Replace the `"auth"` block in `ja.json` with:

```json
"auth": {
  "telegramLogin": "Telegramでログイン",
  "loading": "読み込み中...",
  "backHome": "ホームに戻る",
  "loginError": "ログインエラー",
  "registerError": "登録エラー",
  "telegramLoginError": "Telegramログインエラー",
  "openInTelegram": "Telegramで開く",
  "expiresIn": "{seconds}秒後に期限切れ",
  "sessionExpired": "セッションが期限切れです",
  "tryAgain": "もう一度試す",
  "troubleshootingHint": "問題がありますか？Telegramで @animeenigma_auth_bot をブロックしていないか確認してください。"
}
```

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(i18n): add translations for deep link Telegram auth"
```

---

## Task 14: Clean up old Telegram widget references

**Files:**
- Check: `frontend/web/src/stores/auth.ts`
- Modify: `frontend/web/Dockerfile` — remove `VITE_TELEGRAM_BOT_NAME` ARG/ENV
- Modify: `docker/docker-compose.yml` — remove `VITE_TELEGRAM_BOT_NAME` build arg from web service
- Modify: `frontend/web/.env.example` — remove `VITE_TELEGRAM_BOT_NAME` line

- [ ] **Step 1: Verify no remaining references to old types**

Search for `TelegramAuthData` and `loginWithTelegram` across the frontend codebase. There should be no remaining references.

Run: `cd /data/animeenigma && grep -r "TelegramAuthData\|loginWithTelegram" frontend/web/src/`
Expected: No matches.

- [ ] **Step 2: Remove `VITE_TELEGRAM_BOT_NAME` from frontend Dockerfile**

In `frontend/web/Dockerfile`, remove the lines:
```
ARG VITE_TELEGRAM_BOT_NAME=""
ENV VITE_TELEGRAM_BOT_NAME=$VITE_TELEGRAM_BOT_NAME
```

- [ ] **Step 3: Remove `VITE_TELEGRAM_BOT_NAME` from docker-compose.yml web service**

In `docker/docker-compose.yml`, remove the line from the web service build args:
```
VITE_TELEGRAM_BOT_NAME: ${TELEGRAM_BOT_NAME:-}
```

- [ ] **Step 4: Remove from .env.example**

In `frontend/web/.env.example`, remove the `VITE_TELEGRAM_BOT_NAME=` line.

- [ ] **Step 5: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/ frontend/web/Dockerfile frontend/web/.env.example docker/docker-compose.yml
git commit -m "chore: clean up old Telegram widget auth references and unused VITE_TELEGRAM_BOT_NAME"
```

---

## Task 15: Generate a proper webhook secret

**Files:**
- Modify: `docker/.env`

- [ ] **Step 1: Generate and set a random webhook secret**

Run: `openssl rand -hex 32`

Replace the placeholder `TELEGRAM_WEBHOOK_SECRET` value in `docker/.env` with the generated value.

- [ ] **Step 2: Commit**

```bash
git add docker/.env
git commit -m "chore(docker): set random Telegram webhook secret"
```

---

## Task 16: Deploy and verify

- [ ] **Step 1: Redeploy auth service**

Run: `make redeploy-auth`

- [ ] **Step 2: Check logs for webhook registration**

Run: `make logs-auth` and verify the log line:
```
telegram webhook registered  url=https://animeenigma.ru/api/auth/telegram/webhook
```

If instead you see `failed to set telegram webhook`, check the bot token and webhook URL.

- [ ] **Step 3: Redeploy frontend**

Run: `make redeploy-web`

- [ ] **Step 4: Health check**

Run: `make health`
Expected: All services healthy.

- [ ] **Step 5: Manual test**

1. Open the site in browser → click Login
2. Should see QR code + "Open in Telegram" button + countdown timer
3. Click "Open in Telegram" → Telegram app should open with the bot
4. Bot should say "You are logging into AnimeEnigma" with a "Confirm login" button
5. Click "Confirm login" → bot says "Login confirmed!"
6. Browser should automatically log in and redirect to home

- [ ] **Step 6: Test QR from phone**

1. Open the site on desktop → Login page with QR
2. Scan QR code with phone camera → should open Telegram app on phone
3. Confirm login in Telegram on phone
4. Desktop browser should log in

- [ ] **Step 7: Test expiration**

Wait 5 minutes without confirming → should show "Session expired" with "Try again" button.
