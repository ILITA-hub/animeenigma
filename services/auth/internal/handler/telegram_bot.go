package handler

import (
	"bytes"
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
func (h *TelegramBotHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Verify webhook secret header
	if h.webhookSecret != "" {
		secretHeader := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if secretHeader != h.webhookSecret {
			h.log.Warnw("telegram webhook secret mismatch")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
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

	if err := h.authService.HandleTelegramStart(r.Context(), token, tgUser); err != nil {
		h.log.Warnw("telegram start failed",
			"token", token,
			"telegram_id", msg.From.ID,
			"error", err,
		)
		h.sendMessage(msg.Chat.ID, "This login link has expired or is invalid. Please request a new one from the website.")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Send confirm button
	h.sendMessageWithConfirmButton(msg.Chat.ID, "Click the button below to confirm login to AnimeEnigma:", token)
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

func (h *TelegramBotHandler) callBotAPI(method string, payload interface{}) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", h.botToken, method)

	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Errorw("failed to marshal telegram api payload", "method", method, "error", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		h.log.Errorw("failed to call telegram api", "method", method, "error", err)
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
func (h *TelegramBotHandler) SetWebhook(webhookURL string) error {
	payload := map[string]interface{}{
		"url": webhookURL,
	}
	if h.webhookSecret != "" {
		payload["secret_token"] = h.webhookSecret
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", h.botToken)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal setWebhook payload: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("call setWebhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setWebhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
