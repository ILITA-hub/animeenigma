// Package handler — admin_reports.go: the admin-only feedback browser.
//
// Surfaces the user feedback / error reports that SubmitReport (report.go)
// persists to disk under reportsDir as `{ts}_{user}_{type}.json`. The footer
// "обратная связь" button is the primary producer (player_type="feedback");
// legacy per-player buttons share the same store.
//
//   - GET    /api/admin/reports                 — paginated list (light rows)
//   - GET    /api/admin/reports/{id}            — full report incl. diagnostics
//   - PATCH  /api/admin/reports/{id}/status     — set new|in_progress|resolved
//
// Telegram delivery is unchanged: SubmitReport still ships every report to the
// maintenance bot / Telegram immediately. This handler is a read+triage layer
// on top of the same on-disk archive — no DB, no schema migration. Statuses
// live in a sidecar `_status.json` (id → {status, updated_at, updated_by})
// in reportsDir, guarded by a mutex (player is single-instance).
//
// Mounted under the admin-role-gated group — see
// services/player/internal/transport/router.go + admin.go. The gateway applies
// the same JWT + admin gates (defense-in-depth).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

const (
	statusFileName  = "_status.json"
	descSnippetMax  = 280
	defaultPageSize = 50
	maxPageSize     = 200
)

// validFeedbackStatus is the allowlist of triage states. "not_relevant" marks
// reports/ideas that won't be acted on (obsolete, out of scope, won't-do).
// "ai_done" is a transparent automation state: an AI agent believes the item is
// done, pending human verification before it's promoted to "resolved".
var validFeedbackStatus = map[string]bool{"new": true, "in_progress": true, "ai_done": true, "resolved": true, "not_relevant": true}

// reportIDRe guards against path traversal. IDs are derived from on-disk
// filenames (timestamp_username_playertype, already sanitized at write time),
// so only these chars are valid — no '/', '\', '.', or '..'.
var reportIDRe = regexp.MustCompile(`^[0-9A-Za-z:_\-]+$`)

// AdminReportsHandler serves the admin feedback browser. It owns the same
// reportsDir as ReportHandler and a mutex protecting the sidecar status file.
// notifier (optional) drives the feedback triage notification loop
// (AUTO-417): status transitions notify the report's author.
type AdminReportsHandler struct {
	log        *logger.Logger
	reportsDir string
	notifier   *service.FeedbackNotifier
	mu         sync.Mutex
}

func NewAdminReportsHandler(log *logger.Logger, reportsDir string, notifier *service.FeedbackNotifier) *AdminReportsHandler {
	return &AdminReportsHandler{log: log, reportsDir: reportsDir, notifier: notifier}
}

// feedbackStatusEntry is one row of the sidecar status map.
type feedbackStatusEntry struct {
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

// reportMeta is the light list-row shape. The heavy diagnostic fields
// (page_html / console_logs / network_logs) are intentionally omitted so they
// are not retained in memory while listing. Status is injected post-decode.
type reportMeta struct {
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	Username      string `json:"username"`
	UserID        string `json:"user_id"`
	PlayerType    string `json:"player_type"`
	Category      string `json:"category"`
	AnimeName     string `json:"anime_name"`
	EpisodeNumber *int   `json:"episode_number,omitempty"`
	URL           string `json:"url"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	Source        string   `json:"source,omitempty"`
	Attachments   []string `json:"attachments,omitempty"`
}

func (h *AdminReportsHandler) statusPath() string {
	return filepath.Join(h.reportsDir, statusFileName)
}

// loadStatuses reads the sidecar status map. A missing/corrupt file yields an
// empty map (every report then defaults to "new"). Caller holds h.mu.
func (h *AdminReportsHandler) loadStatuses() map[string]feedbackStatusEntry {
	out := map[string]feedbackStatusEntry{}
	data, err := os.ReadFile(h.statusPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// saveStatuses persists the sidecar status map. Caller holds h.mu.
func (h *AdminReportsHandler) saveStatuses(m map[string]feedbackStatusEntry) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.statusPath(), data, 0600)
}

// safeReportPath resolves an id to an absolute file path inside reportsDir,
// rejecting anything that could traverse out (path-traversal guard mirrors
// report.go's sanitization). Returns (path, true) only for a valid id.
func (h *AdminReportsHandler) safeReportPath(id string) (string, bool) {
	if id == "" || h.reportsDir == "" {
		return "", false
	}
	if strings.HasPrefix(id, "_") || strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return "", false
	}
	if !reportIDRe.MatchString(id) {
		return "", false
	}
	clean := filepath.Clean(filepath.Join(h.reportsDir, id+".json"))
	if clean != filepath.Join(h.reportsDir, id+".json") {
		return "", false
	}
	return clean, true
}

// List returns a paginated, optionally filtered slice of feedback rows,
// newest first. Query params: category, status, type, page, page_size.
func (h *AdminReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fCategory := q.Get("category")
	fStatus := q.Get("status")
	fType := q.Get("type")

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	empty := map[string]interface{}{"items": []reportMeta{}, "total": 0, "page": page, "page_size": pageSize}
	if h.reportsDir == "" {
		httputil.OK(w, empty)
		return
	}

	entries, err := os.ReadDir(h.reportsDir)
	if err != nil {
		h.log.Warnw("failed to read reports dir", "path", h.reportsDir, "error", err)
		httputil.OK(w, empty)
		return
	}

	// Filenames begin with an ISO timestamp, so a reverse name sort == newest first.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "_") || !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, n)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	h.mu.Lock()
	statuses := h.loadStatuses()
	h.mu.Unlock()

	all := make([]reportMeta, 0, len(names))
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(h.reportsDir, n))
		if err != nil {
			continue
		}
		var m reportMeta
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		m.ID = strings.TrimSuffix(n, ".json")
		m.Status = "new"
		if st, ok := statuses[m.ID]; ok && st.Status != "" {
			m.Status = st.Status
		}
		m.Description = truncateRunes(m.Description, descSnippetMax)

		if fCategory != "" && m.Category != fCategory {
			continue
		}
		if fType != "" && m.PlayerType != fType {
			continue
		}
		if fStatus != "" && m.Status != fStatus {
			continue
		}
		all = append(all, m)
	}

	total := len(all)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	httputil.OK(w, map[string]interface{}{
		"items":     all[start:end],
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Get returns the full report (including diagnostics) plus its current status.
func (h *AdminReportsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	path, ok := h.safeReportPath(id)
	if !ok {
		httputil.BadRequest(w, "invalid report id")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		httputil.NotFound(w, "report")
		return
	}
	var full map[string]interface{}
	if err := json.Unmarshal(data, &full); err != nil {
		h.log.Errorw("failed to parse report file", "id", id, "error", err)
		httputil.Error(w, errors.Internal("failed to parse report"))
		return
	}
	full["id"] = id

	h.mu.Lock()
	statuses := h.loadStatuses()
	h.mu.Unlock()
	st := "new"
	if e, ok := statuses[id]; ok && e.Status != "" {
		st = e.Status
		full["status_updated_at"] = e.UpdatedAt
		full["status_updated_by"] = e.UpdatedBy
	}
	full["status"] = st

	httputil.OK(w, full)
}

type setStatusRequest struct {
	Status string `json:"status"`
}

// SetStatus upserts the triage status for a report into the sidecar file.
// Admin route — actor comes from JWT claims; all five statuses allowed.
// (The unauthenticated internal twin lives in internal_feedback.go.)
// Status transitions also drive the AUTO-417 author notification loop via
// dispatchStatusNotification.
func (h *AdminReportsHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	path, ok := h.safeReportPath(id)
	if !ok {
		httputil.BadRequest(w, "invalid report id")
		return
	}

	var req setStatusRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if !validFeedbackStatus[req.Status] {
		httputil.BadRequest(w, "invalid status (expected new|in_progress|ai_done|resolved|not_relevant)")
		return
	}
	if _, err := os.Stat(path); err != nil {
		httputil.NotFound(w, "report")
		return
	}

	var updatedBy string
	if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
		updatedBy = claims.Username
	}

	h.mu.Lock()
	statuses := h.loadStatuses()
	prev := "new"
	if e, ok := statuses[id]; ok && e.Status != "" {
		prev = e.Status
	}
	statuses[id] = feedbackStatusEntry{
		Status:    req.Status,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedBy: updatedBy,
	}
	saveErr := h.saveStatuses(statuses)
	h.mu.Unlock()

	if saveErr != nil {
		h.log.Errorw("failed to persist feedback status", "id", id, "error", saveErr)
		httputil.Error(w, errors.Internal("failed to persist status"))
		return
	}

	h.log.Infow("feedback status updated", "id", id, "status", req.Status, "prev", prev, "by", updatedBy)
	h.dispatchStatusNotification(id, prev, req.Status)
	httputil.OK(w, map[string]interface{}{"id": id, "status": req.Status, "previous_status": prev})
}

// dispatchStatusNotification implements the AUTO-417 triage loop on top of a
// status transition: in_progress and ai_done notify the report's author
// (superseding the previous stage's unread notification); not_relevant
// silently invalidates pending stage notifications; new/resolved and no-op
// repeats dispatch nothing. Author identity comes from the report JSON.
func (h *AdminReportsHandler) dispatchStatusNotification(id, prev, status string) {
	if h.notifier == nil || status == prev {
		return
	}
	path, ok := h.safeReportPath(id)
	if !ok {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		h.log.Warnw("feedback notify: cannot read report", "id", id, "error", err)
		return
	}
	var report struct {
		UserID      string `json:"user_id"`
		Category    string `json:"category"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		h.log.Warnw("feedback notify: cannot parse report", "id", id, "error", err)
		return
	}

	switch status {
	case "in_progress", "ai_done":
		go h.notifier.NotifyStage(context.Background(),
			id, report.UserID, report.Category, report.Description, status)
	case "not_relevant":
		go h.notifier.InvalidateAll(context.Background(), id, report.UserID)
	}
}

// truncateRunes shortens s to at most max runes (UTF-8 safe — Russian text),
// appending an ellipsis when truncated.
func truncateRunes(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…"
}
