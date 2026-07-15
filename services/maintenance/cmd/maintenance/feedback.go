package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/classifier"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
)

// mirrorToFeedback creates the /admin/feedback entry for a human Telegram
// message — the FIRST step of handling, before any analysis. It downloads the
// message's attachments from Telegram to the host (so the Claude dispatcher
// can Read them) and uploads each into the entry. All failures are
// WARN-and-continue: the feedback store must never block message handling.
func (s *service) mirrorToFeedback(msg *domain.ClassifiedMessage) {
	username := msg.From.Username
	if username == "" {
		username = fmt.Sprintf("id%d", msg.From.ID)
	}

	meta := map[string]interface{}{
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
	}
	if msg.ForwardedFrom != "" {
		meta["forwarded_from"] = msg.ForwardedFrom
	}
	if msg.ReplyToText != "" {
		meta["reply_to"] = msg.ReplyToText
	}
	if s.isAdminMessage(*msg) {
		meta["from_admin"] = true
	}

	description := msg.Text
	if strings.TrimSpace(description) == "" {
		description = "<no text — see attachments>"
	}

	id, err := s.fb.Create(feedback.CreateRequest{
		Username:     username,
		UserID:       fmt.Sprintf("tg:%d", msg.From.ID),
		PlayerType:   "telegram",
		Category:     classifier.GuessCategory(msg.Text),
		Description:  description,
		Source:       "telegram",
		TelegramMeta: meta,
	})
	if err != nil {
		log.Warnw("feedback entry creation failed — continuing without mirror",
			"message_id", msg.MessageID,
			"error", err,
		)
		return
	}
	msg.FeedbackID = id
	log.Infow("feedback entry created", "feedback_id", id, "message_id", msg.MessageID, "attachments", len(msg.Attachments))

	s.downloadAndAttach(msg)
}

// downloadAndAttach pulls each Telegram attachment to the host attachments
// dir (for Claude) and uploads a copy into the feedback entry (for the admin
// UI). Per-file failures are logged and skipped.
func (s *service) downloadAndAttach(msg *domain.ClassifiedMessage) {
	if len(msg.Attachments) == 0 {
		return
	}
	baseDir := resolveProjectPath(s.cfg.Claude.ProjectRoot, s.cfg.AttachmentsDir)
	dir := filepath.Join(baseDir, msg.FeedbackID)
	if msg.FeedbackID == "" {
		dir = filepath.Join(baseDir, fmt.Sprintf("msg-%d", msg.MessageID))
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Warnw("attachments dir creation failed", "dir", dir, "error", err)
		return
	}

	for i := range msg.Attachments {
		a := &msg.Attachments[i]
		f, err := s.tg.GetFile(a.FileID)
		if err != nil {
			log.Warnw("telegram getFile failed", "file_id", a.FileID, "name", a.FileName, "error", err)
			continue
		}
		data, err := s.tg.DownloadFile(f.FilePath)
		if err != nil {
			log.Warnw("telegram download failed", "file_id", a.FileID, "name", a.FileName, "error", err)
			continue
		}

		name := sanitizeLocalFilename(a.FileName)
		local := filepath.Join(dir, name)
		if err := os.WriteFile(local, data, 0600); err != nil {
			log.Warnw("attachment local write failed", "path", local, "error", err)
		} else {
			a.LocalPath = local
		}

		if msg.FeedbackID != "" {
			stored, err := s.fb.UploadAttachment(msg.FeedbackID, name, data)
			if err != nil {
				log.Warnw("attachment upload to feedback store failed",
					"feedback_id", msg.FeedbackID, "name", name, "error", err)
			} else {
				a.StoredName = stored
			}
		}
		log.Infow("attachment saved",
			"feedback_id", msg.FeedbackID,
			"name", name,
			"kind", a.Kind,
			"bytes", len(data),
			"local_path", a.LocalPath,
		)
	}
}

// sanitizeLocalFilename keeps a filesystem-safe basename, preserving the extension.
func sanitizeLocalFilename(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
	s = strings.Trim(s, "._")
	if s == "" {
		s = "attachment"
	}
	if len(s) > 128 {
		s = s[len(s)-128:]
	}
	return s
}

// feedbackStatusFor maps an analysis outcome onto the feedback-store
// lifecycle. The bot is an AI, so it NEVER sets "resolved" — that is
// human-only (CLAUDE.md): a person promotes ai_done → resolved after
// verifying. The terminal the bot may set is "ai_done".
//
// Special case: an admin "add to todo / capture for later" request (e.g.
// «Добавь в туду: …») only RECORDS future work — it is NOT done. Such a
// captured backlog item stays a single OPEN ("new") task on the board; the
// bot must not split it into a "done" acknowledgement plus the task itself.
func feedbackStatusFor(result *domain.AnalysisResult) string {
	switch result.Tier {
	case domain.TierAutoFix:
		// Fix applied — AI believes done, human verifies.
		return feedback.StatusAIDone
	case domain.TierInfoOnly, domain.TierResolved:
		if isCapturedTodo(result.Issue.Status) {
			// Recorded-but-not-done future work → one open task.
			return feedback.StatusNew
		}
		// Answered/confirmed — AI believes done, human promotes to resolved.
		return feedback.StatusAIDone
	default: // button_fix (pending), escalate, unknown
		return feedback.StatusInProgress
	}
}

// captureMarkers are the substrings that signal the LLM recorded the request
// as future work to do, rather than a completed action. They are kept tight
// (capture-intent words only) so legitimate completion/lifecycle statuses like
// "auto_fixed", "open", "resolved", "escalated" are never mis-captured.
var captureMarkers = []string{"captured", "backlog", "todo", "to do"}

// isCapturedTodo reports whether the LLM recorded the request as a backlog /
// todo item to be worked on later (issue status the analyzer emits for
// "captured this for the future" rather than a completed action).
//
// The Claude CLI's --json-schema enum is best-effort, not a hard validator, so
// the analyzer may drift to a longer phrase ("captured for later", "backlog
// this item", "TODO: investigate"). We therefore match any capture marker as a
// substring of the normalized status, not just the bare tokens.
func isCapturedTodo(issueStatus string) bool {
	norm := strings.ToLower(strings.TrimSpace(issueStatus))
	for _, m := range captureMarkers {
		if strings.Contains(norm, m) {
			return true
		}
	}
	return false
}
