// Package feedback is the maintenance bot's client for the player service's
// internal feedback API (services/player/internal/handler/internal_feedback.go).
// It mirrors every Telegram conversation the bot handles into the same
// on-disk store the /admin/feedback browser reads, and drives entry status as
// analysis/fixes progress.
//
// The bot runs as a host-side systemd unit; the player container publishes
// its port on 127.0.0.1:8083, so the default base URL is host-local. Every
// method is best-effort from the caller's perspective — a feedback-store
// outage must never block alert/message handling.
package feedback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Entry statuses understood by the feedback store (admin_reports.go's
// validFeedbackStatus).
const (
	StatusNew         = "new"
	StatusInProgress  = "in_progress"
	StatusAIDone      = "ai_done"
	StatusResolved    = "resolved"
	StatusNotRelevant = "not_relevant"
)

type Client struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger
}

func NewClient(baseURL string, log *logger.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
		log:     log,
	}
}

// CreateRequest mirrors player's internalFeedbackCreateRequest.
type CreateRequest struct {
	Username     string                 `json:"username"`
	UserID       string                 `json:"user_id"`
	PlayerType   string                 `json:"player_type"`
	Category     string                 `json:"category,omitempty"`
	Description  string                 `json:"description"`
	URL          string                 `json:"url,omitempty"`
	Source       string                 `json:"source,omitempty"`
	TelegramMeta map[string]interface{} `json:"telegram_meta,omitempty"`
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// Create writes a new feedback entry and returns its id.
func (c *Client) Create(req CreateRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	resp, err := c.http.Post(c.baseURL+"/internal/feedback", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("parse id from %s", truncate(string(data), 300))
	}
	return out.ID, nil
}

// SetStatus updates the triage status of an entry. updatedBy defaults to
// "maintenance-bot" server-side when empty.
//
// "resolved" is HUMAN-ONLY (see CLAUDE.md feedback-triage section): only a
// person promotes an entry to resolved after verifying it. The bot is an AI,
// so this guard hard-downgrades any resolved write to "ai_done" — the
// AI-allowed "I believe this is done, awaiting human verification" terminal.
// This enforces the invariant at the lowest layer, regardless of caller (the
// player service's internal route does NOT refuse resolved server-side).
func (c *Client) SetStatus(id, status, updatedBy string) error {
	if status == StatusResolved {
		c.log.Warnw("refusing to auto-set human-only 'resolved'; downgrading to ai_done",
			"feedback_id", id,
		)
		status = StatusAIDone
	}
	body, _ := json.Marshal(map[string]string{"status": status, "updated_by": updatedBy})
	req, err := http.NewRequest(http.MethodPatch,
		c.baseURL+"/internal/feedback/"+id+"/status", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("patch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	return nil
}

// UploadAttachment ships a downloaded file into the entry's attachment store.
// Returns the stored (possibly de-duplicated) filename.
func (c *Client) UploadAttachment(id, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	resp, err := c.http.Post(
		c.baseURL+"/internal/feedback/"+id+"/attachments",
		mw.FormDataContentType(), &buf)
	if err != nil {
		return "", fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var out struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil || out.Name == "" {
		return "", fmt.Errorf("parse name from %s", truncate(string(body), 300))
	}
	return out.Name, nil
}

// TrySetStatus is SetStatus with WARN-and-continue semantics — the standard
// call shape inside the processing loop where the feedback store must never
// block message handling.
func (c *Client) TrySetStatus(id, status string) {
	if id == "" {
		return
	}
	if err := c.SetStatus(id, status, "maintenance-bot"); err != nil {
		c.log.Warnw("feedback status update failed",
			"feedback_id", id,
			"status", status,
			"error", err,
		)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
