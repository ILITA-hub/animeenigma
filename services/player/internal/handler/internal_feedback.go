// Package handler — internal_feedback.go: the Docker-network/host-local write
// API the maintenance bot uses to mirror Telegram conversations into the same
// on-disk feedback store the /admin/feedback browser reads.
//
//   - POST  /internal/feedback                      — create a feedback entry
//   - POST  /internal/feedback/{id}/attachments     — attach a file (multipart)
//   - PATCH /internal/feedback/{id}/status          — set triage status
//   - GET   /api/admin/reports/{id}/attachments/{name} — serve an attachment (admin)
//
// The /internal/* routes are mounted OUTSIDE /api with no auth — nginx/gateway
// does not proxy /internal/*, and the player port is published only on
// 127.0.0.1, so callers are the Docker network + host-local processes (the
// maintenance bot systemd unit). Precedent: /internal/users/{id}/list.
//
// All sidecar-status and report-file mutations go through AdminReportsHandler's
// mutex so the admin UI and the bot never race on _status.json or a report file.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/go-chi/chi/v5"
)

const (
	attachmentsDirName    = "_attachments"
	maxAttachmentSize     = 25 * 1024 * 1024 // Telegram bot API caps getFile at 20MB
	maxInternalBodySize   = 1 * 1024 * 1024
	maxAttachmentsPerItem = 20
)

// internalPlayerTypes is the allowlist for internally-created entries. Kept
// separate from report.go's allowedPlayerTypes so public /api/users/report
// submissions cannot impersonate bot-sourced entries.
var internalPlayerTypes = map[string]bool{"telegram": true}

// attachmentNameRe matches a sanitized attachment filename.
var attachmentNameRe = regexp.MustCompile(`^[0-9A-Za-z._\-]+$`)

type internalFeedbackCreateRequest struct {
	Username    string                 `json:"username"`
	UserID      string                 `json:"user_id"`
	PlayerType  string                 `json:"player_type"`
	Category    string                 `json:"category,omitempty"` // bug | issue | feature
	Description string                 `json:"description"`
	URL         string                 `json:"url,omitempty"`
	Source      string                 `json:"source,omitempty"`
	TelegramMeta map[string]interface{} `json:"telegram_meta,omitempty"`
}

// CreateInternal writes a new feedback entry into reportsDir using the same
// `{ts}_{user}_{type}.json` naming the public report path uses, so the admin
// feedback browser picks it up with zero changes to its listing logic.
func (h *AdminReportsHandler) CreateInternal(w http.ResponseWriter, r *http.Request) {
	if h.reportsDir == "" {
		httputil.Error(w, errors.Internal("reports dir not configured"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxInternalBodySize)

	var req internalFeedbackCreateRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	if !internalPlayerTypes[req.PlayerType] {
		httputil.BadRequest(w, "invalid player_type")
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		httputil.BadRequest(w, "description is required")
		return
	}

	username := req.Username
	if username == "" {
		username = "unknown"
	}
	username = sanitizeForFilename(username)

	entry := map[string]interface{}{
		"user_id":     req.UserID,
		"username":    req.Username,
		"player_type": req.PlayerType,
		"category":    req.Category,
		"description": req.Description,
		"url":         req.URL,
		"source":      req.Source,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"attachments": []string{},
	}
	if len(req.TelegramMeta) > 0 {
		entry["telegram_meta"] = req.TelegramMeta
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		httputil.Error(w, errors.Internal("failed to marshal entry"))
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Same-second collisions get a -2, -3, … suffix (media-group splits,
	// rapid-fire messages).
	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	base := fmt.Sprintf("%s_%s_%s", ts, username, req.PlayerType)
	id := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(h.reportsDir, id+".json")); os.IsNotExist(err) {
			break
		}
		if i > 50 {
			httputil.Error(w, errors.Internal("could not allocate entry id"))
			return
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}

	if err := os.WriteFile(filepath.Join(h.reportsDir, id+".json"), data, 0600); err != nil {
		h.log.Errorw("internal feedback write failed", "id", id, "error", err)
		httputil.Error(w, errors.Internal("failed to persist entry"))
		return
	}

	h.log.Infow("internal feedback entry created", "id", id, "source", req.Source, "username", req.Username)
	httputil.OK(w, map[string]string{"id": id, "status": "new"})
}

// UploadAttachmentInternal stores one multipart file (field "file") under
// reportsDir/_attachments/{id}/ and appends its name to the entry's
// attachments array. The `_` prefix keeps the dir invisible to List().
func (h *AdminReportsHandler) UploadAttachmentInternal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reportPath, ok := h.safeReportPath(id)
	if !ok {
		httputil.BadRequest(w, "invalid report id")
		return
	}
	if _, err := os.Stat(reportPath); err != nil {
		httputil.NotFound(w, "report")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentSize+64*1024)
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.BadRequest(w, "missing multipart field 'file'")
		return
	}
	defer file.Close()

	name := sanitizeAttachmentName(header.Filename)
	if name == "" {
		httputil.BadRequest(w, "invalid filename")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	dir := filepath.Join(h.reportsDir, attachmentsDirName, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		httputil.Error(w, errors.Internal("failed to create attachments dir"))
		return
	}

	// De-dupe stored name if it already exists for this entry.
	stored := name
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, stored)); os.IsNotExist(err) {
			break
		}
		if i > 50 {
			httputil.Error(w, errors.Internal("could not allocate attachment name"))
			return
		}
		ext := filepath.Ext(name)
		stored = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), i, ext)
	}

	dst, err := os.OpenFile(filepath.Join(dir, stored), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		httputil.Error(w, errors.Internal("failed to create attachment file"))
		return
	}
	written, copyErr := io.Copy(dst, io.LimitReader(file, maxAttachmentSize))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(filepath.Join(dir, stored))
		httputil.Error(w, errors.Internal("failed to write attachment"))
		return
	}

	// Append to the entry's attachments array (bounded).
	if err := h.appendAttachmentLocked(reportPath, stored); err != nil {
		h.log.Errorw("failed to record attachment on entry", "id", id, "name", stored, "error", err)
		os.Remove(filepath.Join(dir, stored))
		httputil.Error(w, errors.Internal("failed to record attachment"))
		return
	}

	h.log.Infow("attachment stored", "id", id, "name", stored, "bytes", written)
	httputil.OK(w, map[string]interface{}{"id": id, "name": stored, "bytes": written})
}

// appendAttachmentLocked rewrites the report JSON with the new attachment name
// appended. Caller holds h.mu.
func (h *AdminReportsHandler) appendAttachmentLocked(reportPath, name string) error {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return err
	}
	var full map[string]interface{}
	if err := json.Unmarshal(data, &full); err != nil {
		return err
	}
	var list []interface{}
	if existing, ok := full["attachments"].([]interface{}); ok {
		list = existing
	}
	if len(list) >= maxAttachmentsPerItem {
		return fmt.Errorf("attachment limit reached (%d)", maxAttachmentsPerItem)
	}
	full["attachments"] = append(list, name)
	out, err := json.MarshalIndent(full, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(reportPath, out, 0600)
}

type internalSetStatusRequest struct {
	Status    string `json:"status"`
	UpdatedBy string `json:"updated_by,omitempty"`
}

// SetStatusInternal is the unauthenticated (internal-only) twin of SetStatus —
// the maintenance bot drives the entry lifecycle (new → in_progress → ai_done/
// resolved/not_relevant) as it analyzes and fixes.
func (h *AdminReportsHandler) SetStatusInternal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	path, ok := h.safeReportPath(id)
	if !ok {
		httputil.BadRequest(w, "invalid report id")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxInternalBodySize)
	var req internalSetStatusRequest
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
	updatedBy := req.UpdatedBy
	if updatedBy == "" {
		updatedBy = "maintenance-bot"
	}

	h.mu.Lock()
	statuses := h.loadStatuses()
	statuses[id] = feedbackStatusEntry{
		Status:    req.Status,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedBy: updatedBy,
	}
	saveErr := h.saveStatuses(statuses)
	h.mu.Unlock()

	if saveErr != nil {
		h.log.Errorw("failed to persist internal feedback status", "id", id, "error", saveErr)
		httputil.Error(w, errors.Internal("failed to persist status"))
		return
	}

	h.log.Infow("feedback status updated (internal)", "id", id, "status", req.Status, "by", updatedBy)
	httputil.OK(w, map[string]interface{}{"id": id, "status": req.Status})
}

// GetAttachment serves a stored attachment to the admin UI. Mounted inside the
// admin-role-gated group (gateway re-applies the same gates).
func (h *AdminReportsHandler) GetAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.safeReportPath(id); !ok {
		httputil.BadRequest(w, "invalid report id")
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" || !attachmentNameRe.MatchString(name) || strings.Contains(name, "..") {
		httputil.BadRequest(w, "invalid attachment name")
		return
	}
	path := filepath.Join(h.reportsDir, attachmentsDirName, id, name)
	if _, err := os.Stat(path); err != nil {
		httputil.NotFound(w, "attachment")
		return
	}
	// http.ServeFile sets Content-Type from the extension and handles ranges.
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", name))
	http.ServeFile(w, r, path)
}

// sanitizeForFilename mirrors report.go's username sanitization.
func sanitizeForFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}

// sanitizeAttachmentName keeps a safe basename (letters, digits, dot, dash,
// underscore), preserving the extension so Content-Type detection works.
func sanitizeAttachmentName(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
	s = strings.Trim(s, "._")
	if s == "" || len(s) > 128 || !attachmentNameRe.MatchString(s) {
		return ""
	}
	return s
}
