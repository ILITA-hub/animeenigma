package handler

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// TelegramBotHandler handles Telegram Bot API webhooks for deep link authentication.
type TelegramBotHandler struct {
	authService   *service.AuthService
	botToken      string
	webhookSecret string
	log           *logger.Logger
}

// NewTelegramBotHandler creates a new TelegramBotHandler.
func NewTelegramBotHandler(authService *service.AuthService, botToken, webhookSecret string, log *logger.Logger) *TelegramBotHandler {
	return &TelegramBotHandler{
		authService:   authService,
		botToken:      botToken,
		webhookSecret: webhookSecret,
		log:           log,
	}
}

// Telegram API types

type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message,omitempty"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query,omitempty"`
}

type telegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *telegramUser `json:"from,omitempty"`
	Chat      *telegramChat `json:"chat"`
	Text      string        `json:"text"`
}

type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    *telegramUser    `json:"from"`
	Message *telegramMessage `json:"message,omitempty"`
	Data    string           `json:"data"`
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
	CallbackData string `json:"callback_data,omitempty"`
}

type telegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]telegramInlineKeyboardButton `json:"inline_keyboard"`
}

// HandleWebhook processes incoming Telegram Bot API webhook updates.
//
// The X-Telegram-Bot-Api-Secret-Token header is the ONLY authentication on this
// publicly routed endpoint (the update body alone drives login-session state),
// so the check fails closed: without a configured TELEGRAM_WEBHOOK_SECRET every
// request is rejected instead of trusted.
func (h *TelegramBotHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verify webhook secret header
	if h.webhookSecret == "" {
		h.log.Errorw("telegram webhook rejected: TELEGRAM_WEBHOOK_SECRET is not configured")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	secretHeader := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(secretHeader), []byte(h.webhookSecret)) != 1 {
		h.log.Warnw("telegram webhook secret mismatch")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Errorw("failed to read webhook body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var update telegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		h.log.Errorw("failed to parse webhook update", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Dispatch based on update type
	if update.Message != nil && strings.HasPrefix(update.Message.Text, "/start ") {
		h.handleStart(r, w, &update)
		return
	}

	if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "confirm:") {
		h.handleCallbackConfirm(r, w, &update)
		return
	}

	// Unknown update type — acknowledge silently
	w.WriteHeader(http.StatusOK)
}

// handleStart processes /start <token> messages from users.
func (h *TelegramBotHandler) handleStart(r *http.Request, w http.ResponseWriter, update *telegramUpdate) {
	msg := update.Message
	if msg.From == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract token from "/start <token>"
	parts := strings.SplitN(msg.Text, " ", 2)
	if len(parts) < 2 || parts[1] == "" {
		h.sendMessage(msg.Chat.ID, "Welcome! Use the login button on the website to authenticate.")
		w.WriteHeader(http.StatusOK)
		return
	}

	token := parts[1]

	tgUser := &domain.TelegramWebhookUser{
		ID:        msg.From.ID,
		FirstName: msg.From.FirstName,
		LastName:  msg.From.LastName,
		Username:  msg.From.Username,
	}

	origin, err := h.authService.HandleTelegramStart(r.Context(), token, tgUser)
	if err != nil {
		h.log.Warnw("telegram start failed",
			"token", token,
			"telegram_id", msg.From.ID,
			"error", err,
		)
		h.sendMessage(msg.Chat.ID, "This login link has expired or is invalid. Please request a new one from the website.")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Send confirm button. When we captured where the login was requested from
	// (device + IP at mint time), surface it so a user asked to confirm a login
	// they did not initiate sees an unfamiliar origin and can decline — the only
	// backend-visible signal against an attacker-initiated flow (vector B),
	// where the attacker owns the minting/polling browser.
	confirmText := "Click the button below to confirm login to AnimeEnigma:"
	if origin != "" {
		confirmText = "A login to AnimeEnigma was requested from:\n\n" + origin +
			"\n\nOnly confirm if this was YOU. If you did not start this login, do not confirm — just ignore this message.\n\n" +
			"Tap the button below to confirm login:"
	}
	h.sendMessageWithConfirmButton(msg.Chat.ID, confirmText, token)
	w.WriteHeader(http.StatusOK)
}

// handleCallbackConfirm processes "confirm:<token>" callback queries.
func (h *TelegramBotHandler) handleCallbackConfirm(r *http.Request, w http.ResponseWriter, update *telegramUpdate) {
	cb := update.CallbackQuery

	// Extract token from "confirm:<token>"
	token := strings.TrimPrefix(cb.Data, "confirm:")

	tgUser := &domain.TelegramWebhookUser{
		ID:        cb.From.ID,
		FirstName: cb.From.FirstName,
		LastName:  cb.From.LastName,
		Username:  cb.From.Username,
	}

	if err := h.authService.HandleTelegramCallback(r.Context(), token, tgUser); err != nil {
		h.log.Warnw("telegram callback confirm failed",
			"token", token,
			"telegram_id", cb.From.ID,
			"error", err,
		)
		h.answerCallbackQuery(cb.ID, "Login failed. The link may have expired.")
		w.WriteHeader(http.StatusOK)
		return
	}

	h.answerCallbackQuery(cb.ID, "Login confirmed!")

	if cb.Message != nil {
		h.editMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Login confirmed! You can now close this chat.")
	}

	w.WriteHeader(http.StatusOK)
}

// Bot API helpers

func (h *TelegramBotHandler) sendMessage(chatID int64, text string) {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	h.callBotAPI("sendMessage", payload)
}

func (h *TelegramBotHandler) sendMessageWithConfirmButton(chatID int64, text string, token string) {
	markup := telegramInlineKeyboardMarkup{
		InlineKeyboard: [][]telegramInlineKeyboardButton{
			{
				{
					Text:         "Confirm Login",
					CallbackData: "confirm:" + token,
				},
			},
		},
	}

	payload := map[string]interface{}{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": markup,
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

// redactToken removes the bot token from a string so it can never reach a log
// or a returned/wrapped error. The Bot API URL embeds the token in its
// /bot<token>/ path segment, and Go's *url.Error renders the full request URL
// verbatim, so a transport error (DNS, timeout, TLS, connection refused) would
// otherwise print the production token. Redacting by the actual token value
// (not just the /bot.../ shape) also covers any differently-shaped URL that a
// future caller might construct. Guards against an empty token, for which
// strings.ReplaceAll would otherwise splice the mask between every character.
func (h *TelegramBotHandler) redactToken(s string) string {
	if h.botToken == "" {
		return s
	}
	return strings.ReplaceAll(s, h.botToken, "***")
}

func (h *TelegramBotHandler) callBotAPI(method string, payload interface{}) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", h.botToken, method)

	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Errorw("failed to marshal telegram api payload", "method", method, "error", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		h.log.Errorw("failed to call telegram api", "method", method, "error", h.redactToken(err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		h.log.Warnw("telegram api non-200 response",
			"method", method,
			"status", resp.StatusCode,
			"body", string(respBody),
		)
	}
}

// SetWebhook registers the webhook URL with Telegram Bot API.
//
// Always calls deleteWebhook first to clear any silent-disable state Telegram
// may have entered (e.g., after repeated 5xx or transient TLS issues, Telegram
// stops delivering without setting last_error_date in getWebhookInfo, and a
// plain setWebhook returns "already set" without re-enabling delivery).
//
// Refuses to register without a configured secret: HandleWebhook rejects every
// unauthenticated update, so registering one would only produce deliveries that
// are dropped — surface the misconfiguration at startup instead.
func (h *TelegramBotHandler) SetWebhook(webhookURL string) error {
	if h.webhookSecret == "" {
		return fmt.Errorf("refusing to register telegram webhook: TELEGRAM_WEBHOOK_SECRET is not configured")
	}

	deleteURL := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", h.botToken)
	if delResp, err := http.Post(deleteURL, "application/json", bytes.NewReader([]byte(`{"drop_pending_updates":true}`))); err == nil {
		_ = delResp.Body.Close()
	}

	payload := map[string]interface{}{
		"url":                  webhookURL,
		"allowed_updates":      []string{"message", "callback_query"},
		"drop_pending_updates": true,
		"secret_token":         h.webhookSecret,
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", h.botToken)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setWebhook payload: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		// Do not wrap err with %w: its *url.Error renders the full request URL,
		// which embeds the bot token. Return a token-redacted message instead.
		return fmt.Errorf("call setWebhook: %s", h.redactToken(err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setWebhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
