package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The webhook secret header is the only authentication on the publicly routed
// POST /api/auth/telegram/webhook, and the update body alone drives login-session
// state. These tests pin the fail-closed contract: nothing reaches the auth
// service (nil here — a dispatched update would panic) unless the configured
// secret matched.

const startUpdate = `{"message":{"text":"/start tok","chat":{"id":1},"from":{"id":42,"first_name":"x"}}}`

func webhookRequest(secretHeader string, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/telegram/webhook", strings.NewReader(body))
	if secretHeader != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secretHeader)
	}
	return req
}

func TestHandleWebhook_RejectsWhenSecretNotConfigured(t *testing.T) {
	h := NewTelegramBotHandler(nil, "bot-token", "", testLogger())

	for _, header := range []string{"", "anything"} {
		rec := httptest.NewRecorder()
		h.HandleWebhook(rec, webhookRequest(header, startUpdate))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("empty configured secret, header %q: got status %d, want %d",
				header, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestHandleWebhook_RejectsWrongSecret(t *testing.T) {
	h := NewTelegramBotHandler(nil, "bot-token", "s3cret", testLogger())

	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, webhookRequest("wrong", startUpdate))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleWebhook_AcceptsMatchingSecret(t *testing.T) {
	h := NewTelegramBotHandler(nil, "bot-token", "s3cret", testLogger())

	// Unknown update type: passes the secret gate, then acknowledges silently
	// without dispatching into the (nil) auth service.
	rec := httptest.NewRecorder()
	h.HandleWebhook(rec, webhookRequest("s3cret", `{"update_id":1}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSetWebhook_RefusesWithoutSecret(t *testing.T) {
	h := NewTelegramBotHandler(nil, "bot-token", "", testLogger())

	// Must fail before any Telegram API call is attempted.
	if err := h.SetWebhook("https://example.test/api/auth/telegram/webhook"); err == nil {
		t.Fatal("SetWebhook succeeded with no configured secret, want error")
	}
}
