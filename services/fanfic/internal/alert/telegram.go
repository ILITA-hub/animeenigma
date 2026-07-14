// Package alert sends operational alerts to the maintenance Telegram chat via
// the ALERTS bot. The auth bot token is NOT usable here (not a chat member).
package alert

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Alerter interface {
	Send(ctx context.Context, text string) error
}

type noop struct{}

func NewNoop() Alerter                          { return noop{} }
func (noop) Send(context.Context, string) error { return nil }

type Telegram struct {
	token   string
	chatID  string
	baseURL string
	http    *http.Client
}

func NewTelegram(botToken, chatID, baseURL string, hc *http.Client) *Telegram {
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Telegram{token: botToken, chatID: chatID, baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.token)
	form := url.Values{"chat_id": {t.chatID}, "text": {text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram send: build request: %s", t.redact(err.Error()))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		// net/http returns a *url.Error whose Error() embeds the full request
		// URL (token included) — redact before wrapping so the token can
		// never end up in a log line via err.Error().
		return fmt.Errorf("telegram send: %s", t.redact(err.Error()))
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send: status %d", resp.StatusCode)
	}
	return nil
}

// redact strips the bot token out of an arbitrary error string so it can
// never leak into logs even if the underlying error embeds the request URL.
func (t *Telegram) redact(msg string) string {
	if t.token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, t.token, "REDACTED")
}
