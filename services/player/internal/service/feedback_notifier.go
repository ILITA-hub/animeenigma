package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// FeedbackNotifier is the fire-and-forget producer for the feedback triage
// notification loop (AUTO-417). It POSTs to the notifications service's
// internal endpoints over the Docker network; every method is best-effort —
// a notifications outage must never affect report submission or triage.
//
// Stage model (mirrors the triage statuses in admin_reports.go):
//
//	created     — report just submitted (status "new" implied)
//	in_progress — triage/robot started working
//	ai_done     — robot finished, pending human verification
//
// Each stage's notification invalidates the other stages' unread rows for
// the same report so the bell never shows two contradictory stages.
type FeedbackNotifier struct {
	baseURL string
	enabled bool
	client  *http.Client
	log     *logger.Logger
}

// feedbackStages maps a triage stage to its notification type. Order is not
// meaningful — supersede direction is handled by invalidating ALL sibling
// stage keys on every dispatch.
var feedbackStages = map[string]string{
	"created":     "feedback_created",
	"in_progress": "feedback_in_progress",
	"ai_done":     "feedback_ai_done",
}

// uuidRe guards the user_id: user_notifications.user_id is a Postgres uuid
// column, but Telegram-ingested reports carry ids like "tg:898912046" which
// have no site account to notify. Those are silently skipped.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const feedbackDescriptionMax = 200 // runes kept in the notification payload

// NewFeedbackNotifier constructs the producer. baseURL is the notifications
// service root (http://notifications:8090); enabled=false turns every call
// into a no-op (dark-ship / outage toggle).
func NewFeedbackNotifier(baseURL string, enabled bool, log *logger.Logger) *FeedbackNotifier {
	return &FeedbackNotifier{
		baseURL: baseURL,
		enabled: enabled,
		client:  &http.Client{Timeout: 5 * time.Second},
		log:     log,
	}
}

// feedbackDedupeKey mirrors the notifications service's FeedbackDedupeKey.
func feedbackDedupeKey(reportID, stage string) string {
	return fmt.Sprintf("feedback:%s:%s", reportID, stage)
}

// siblingKeys returns the dedupe keys of every stage EXCEPT the given one.
func siblingKeys(reportID, stage string) []string {
	keys := make([]string, 0, len(feedbackStages)-1)
	for s := range feedbackStages {
		if s != stage {
			keys = append(keys, feedbackDedupeKey(reportID, s))
		}
	}
	return keys
}

// allStageKeys returns the dedupe keys of every stage.
func allStageKeys(reportID string) []string {
	keys := make([]string, 0, len(feedbackStages))
	for s := range feedbackStages {
		keys = append(keys, feedbackDedupeKey(reportID, s))
	}
	return keys
}

// NotifyStage upserts the stage notification for the report's author and
// invalidates the sibling stages' unread rows. Safe to call from a request
// handler — runs synchronously but with a 5s client timeout; callers that
// must not block should wrap it in a goroutine.
func (n *FeedbackNotifier) NotifyStage(ctx context.Context, reportID, userID, category, description, stage string) {
	ntype, ok := feedbackStages[stage]
	if !ok {
		return // statuses like "resolved"/"not_relevant" have no stage notification
	}
	if !n.precheck(reportID, userID) {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"report_id":   reportID,
		"category":    category,
		"description": truncateRunes(description, feedbackDescriptionMax),
		"status":      stage,
	})
	if err != nil {
		n.log.Errorw("feedback notify: marshal payload", "report_id", reportID, "error", err)
		return
	}

	body := map[string]interface{}{
		"user_id":                userID,
		"type":                   ntype,
		"dedupe_key":             feedbackDedupeKey(reportID, stage),
		"payload":                json.RawMessage(payload),
		"invalidate_dedupe_keys": siblingKeys(reportID, stage),
	}
	if err := n.post(ctx, "/internal/notifications", body); err != nil {
		n.log.Warnw("feedback notify failed (non-fatal)",
			"report_id", reportID, "stage", stage, "user_id", userID, "error", err)
		return
	}
	n.log.Infow("feedback stage notification dispatched",
		"report_id", reportID, "stage", stage, "user_id", userID)
}

// InvalidateAll tombstones every unread stage notification for the report —
// used when a report is closed as not_relevant (pending notifications stop
// being actual; nothing new replaces them).
func (n *FeedbackNotifier) InvalidateAll(ctx context.Context, reportID, userID string) {
	if !n.precheck(reportID, userID) {
		return
	}
	body := map[string]interface{}{
		"user_id":     userID,
		"dedupe_keys": allStageKeys(reportID),
	}
	if err := n.post(ctx, "/internal/notifications/invalidate", body); err != nil {
		n.log.Warnw("feedback invalidate failed (non-fatal)",
			"report_id", reportID, "user_id", userID, "error", err)
		return
	}
	n.log.Infow("feedback notifications invalidated", "report_id", reportID, "user_id", userID)
}

// precheck gates every dispatch: producer enabled + user is a site account.
func (n *FeedbackNotifier) precheck(reportID, userID string) bool {
	if n == nil || !n.enabled || n.baseURL == "" {
		return false
	}
	if !uuidRe.MatchString(userID) {
		// Telegram-sourced reports ("tg:..."), empty ids, legacy rows — no
		// site account to notify. Expected, so DEBUG not WARN.
		n.log.Debugw("feedback notify skipped: non-site user id",
			"report_id", reportID, "user_id", userID)
		return false
	}
	return true
}

func (n *FeedbackNotifier) post(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// truncateRunes shortens s to at most max runes (UTF-8 safe — Russian text),
// appending an ellipsis when truncated. Duplicated from handler to avoid an
// import cycle; both copies are trivial.
func truncateRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…"
}
