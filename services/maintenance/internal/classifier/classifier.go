package classifier

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

var (
	// English keywords are word-bounded so substrings don't false-positive
	// ("lag" in "feature flags", "down" in "download"). Cyrillic keywords stay
	// unbounded: RE2's \b is ASCII-only and never matches next to Cyrillic letters.
	issueKeywords = regexp.MustCompile(`(?i)\b(not working|broken|error|bug|can't watch|loading|lag(s|ging|gy)?|down|crash|stuck)\b|(?i)(не работает|сломал|ошибка|баг|лагает|не грузит|не воспроизвод)`)
	// Explicit feature-request framing beats breakage keywords in GuessCategory.
	featureKeywords = regexp.MustCompile(`(?i)\bfeature request\b|(?i)(фича|фичу|предложени|предлага|хотелось бы|было бы (круто|здорово|удобно))`)
	serviceNames    = regexp.MustCompile(`(?i)\b(gateway|auth|catalog|streaming|player|rooms|scheduler|themes|kodik|animelib)\b`)
)

// Classify determines the type and priority of a Telegram update.
func Classify(update telegram.Update, adminUsernames []string) domain.ClassifiedMessage {
	raw, _ := json.Marshal(update)

	msg := domain.ClassifiedMessage{
		UpdateID: update.UpdateID,
		RawJSON:  string(raw),
		Type:     domain.MessageIgnore,
		Priority: domain.P3,
	}

	if update.CallbackQuery != nil {
		cb := update.CallbackQuery
		msg.Type = domain.MessageButtonClick
		msg.CallbackData = cb.Data
		msg.CallbackID = cb.ID
		if cb.From != nil {
			msg.From = domain.User{
				ID:       cb.From.ID,
				Username: cb.From.Username,
				IsBot:    cb.From.IsBot,
			}
		}
		if cb.Message != nil {
			msg.MessageID = cb.Message.MessageID
			msg.ChatID = cb.Message.Chat.ID
		}
		return msg
	}

	if update.Message == nil {
		return msg
	}

	m := update.Message
	msg.MessageID = m.MessageID
	if m.Chat != nil {
		msg.ChatID = m.Chat.ID
	}
	if m.From != nil {
		msg.From = domain.User{
			ID:       m.From.ID,
			Username: m.From.Username,
			IsBot:    m.From.IsBot,
		}
	}

	// Bot messages are ignored — alert delivery is now via webhook, not Telegram
	if m.From != nil && m.From.IsBot {
		return msg
	}

	// Text falls back to the media caption; reply/forward context is folded in
	// so downstream consumers (Claude, the feedback entry) see the whole thread.
	msg.MediaGroupID = m.MediaGroupID
	msg.Attachments = extractAttachments(m)
	msg.ForwardedFrom = m.ForwardOrigin.Label()
	msg.ReplyToText = replyContext(m.ReplyTo)
	msg.Text = composeText(m, msg.ForwardedFrom, msg.ReplyToText)

	if m.From != nil && isAdmin(m.From.Username, adminUsernames) {
		msg.Type = domain.MessageAdminMessage
		msg.Priority = domain.P1
		return msg
	}

	if issueKeywords.MatchString(msg.Text) {
		msg.Type = domain.MessageUserIssue
		msg.Priority = domain.P2
		return msg
	}

	// A human message carrying media (screenshot, recording, log file) or a
	// forwarded report is relevant even without issue keywords.
	if len(msg.Attachments) > 0 || msg.ForwardedFrom != "" {
		msg.Type = domain.MessageUserIssue
		msg.Priority = domain.P2
		return msg
	}

	return msg
}

// extractAttachments collects the media carried by one message. A photo
// message lists multiple sizes of the same image — only the largest (last)
// is taken.
func extractAttachments(m *telegram.Message) []domain.Attachment {
	var out []domain.Attachment
	if len(m.Photo) > 0 {
		p := m.Photo[len(m.Photo)-1]
		out = append(out, domain.Attachment{
			FileID:   p.FileID,
			FileName: fmt.Sprintf("photo_%d.jpg", m.MessageID),
			MimeType: "image/jpeg",
			Kind:     "photo",
			Size:     p.FileSize,
		})
	}
	if m.Document != nil {
		name := m.Document.FileName
		if name == "" {
			name = fmt.Sprintf("document_%d", m.MessageID)
		}
		out = append(out, domain.Attachment{
			FileID:   m.Document.FileID,
			FileName: name,
			MimeType: m.Document.MimeType,
			Kind:     "document",
			Size:     m.Document.FileSize,
		})
	}
	if m.Video != nil {
		name := m.Video.FileName
		if name == "" {
			name = fmt.Sprintf("video_%d.mp4", m.MessageID)
		}
		out = append(out, domain.Attachment{
			FileID:   m.Video.FileID,
			FileName: name,
			MimeType: m.Video.MimeType,
			Kind:     "video",
			Size:     m.Video.FileSize,
		})
	}
	if m.Audio != nil {
		name := m.Audio.FileName
		if name == "" {
			name = fmt.Sprintf("audio_%d", m.MessageID)
		}
		out = append(out, domain.Attachment{
			FileID:   m.Audio.FileID,
			FileName: name,
			MimeType: m.Audio.MimeType,
			Kind:     "audio",
			Size:     m.Audio.FileSize,
		})
	}
	if m.Voice != nil {
		out = append(out, domain.Attachment{
			FileID:   m.Voice.FileID,
			FileName: fmt.Sprintf("voice_%d.ogg", m.MessageID),
			MimeType: m.Voice.MimeType,
			Kind:     "voice",
			Size:     m.Voice.FileSize,
		})
	}
	return out
}

// replyContext renders the message being replied to as quotable context.
func replyContext(r *telegram.Message) string {
	if r == nil {
		return ""
	}
	text := r.Text
	if text == "" {
		text = r.Caption
	}
	if text == "" && (len(r.Photo) > 0 || r.Document != nil || r.Video != nil) {
		text = "<media message>"
	}
	if len([]rune(text)) > 500 {
		text = string([]rune(text)[:500]) + "…"
	}
	who := ""
	if r.From != nil {
		who = r.From.Username
		if who == "" {
			who = r.From.FirstName
		}
		if r.From.IsBot {
			who += " (bot)"
		}
	}
	if who != "" {
		return "@" + who + ": " + text
	}
	return text
}

// composeText folds caption/forward/reply context into one human-readable text
// block so Claude and the feedback entry see the full conversational context.
func composeText(m *telegram.Message, forwardedFrom, replyTo string) string {
	text := m.Text
	if text == "" {
		text = m.Caption
	}

	var sb strings.Builder
	if forwardedFrom != "" {
		sb.WriteString("[Forwarded from " + forwardedFrom + "]\n")
	}
	sb.WriteString(text)
	if replyTo != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("[In reply to " + replyTo + "]")
	}
	return sb.String()
}

// ClassifyBatch groups classified messages into categories. Messages that
// share a media_group_id (a Telegram album) are merged into ONE relevant
// message carrying all the album's attachments, so a multi-screenshot report
// produces a single feedback entry + a single analysis.
func ClassifyBatch(updates []telegram.Update, adminUsernames []string) domain.ClassifiedBatch {
	var batch domain.ClassifiedBatch

	for _, u := range updates {
		classified := Classify(u, adminUsernames)

		switch classified.Type {
		case domain.MessageButtonClick:
			batch.ButtonClicks = append(batch.ButtonClicks, classified)
		case domain.MessageAlertResolved:
			batch.Resolved = append(batch.Resolved, classified)
		case domain.MessageAlertFiring, domain.MessageErrorReport,
			domain.MessageAdminMessage, domain.MessageUserIssue:
			if merged := tryMergeAlbum(batch.Relevant, classified); merged {
				continue
			}
			batch.Relevant = append(batch.Relevant, classified)
		default:
			// Album tails without their own caption/keywords can classify as
			// ignore — still merge their attachments into the album head.
			if classified.MediaGroupID != "" && tryMergeAlbum(batch.Relevant, classified) {
				continue
			}
			batch.Ignored++
		}
	}

	return batch
}

// tryMergeAlbum folds an album member into an already-collected message with
// the same media_group_id. Returns true when merged.
func tryMergeAlbum(relevant []domain.ClassifiedMessage, msg domain.ClassifiedMessage) bool {
	if msg.MediaGroupID == "" {
		return false
	}
	for i := range relevant {
		if relevant[i].MediaGroupID == msg.MediaGroupID && relevant[i].From.ID == msg.From.ID {
			relevant[i].Attachments = append(relevant[i].Attachments, msg.Attachments...)
			// Albums carry the caption on one member only — keep whichever has text.
			if strings.TrimSpace(relevant[i].Text) == "" {
				relevant[i].Text = msg.Text
			}
			return true
		}
	}
	return false
}

// CountAffectedServices returns the number of distinct services with firing alerts in the batch.
func CountAffectedServices(batch domain.ClassifiedBatch) int {
	services := make(map[string]bool)
	for _, msg := range batch.Relevant {
		if msg.Type == domain.MessageAlertFiring {
			for _, alert := range msg.Alerts {
				if alert.Service != "" {
					services[alert.Service] = true
				}
			}
		}
	}
	return len(services)
}

// GuessCategory gives the initial feedback-store category for a raw message,
// before Claude has classified it: explicit feature framing → "feature",
// keyword-matched breakage → "bug", everything else → "issue". Claude's later
// classification lands on the linked issue; the feedback entry keeps the
// at-intake guess.
func GuessCategory(text string) string {
	if featureKeywords.MatchString(text) {
		return "feature"
	}
	if issueKeywords.MatchString(text) {
		return "bug"
	}
	return "issue"
}

func isAdmin(username string, admins []string) bool {
	for _, admin := range admins {
		if strings.EqualFold(username, admin) {
			return true
		}
	}
	return false
}
