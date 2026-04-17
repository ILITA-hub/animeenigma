package classifier

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

var (
	issueKeywords = regexp.MustCompile(`(?i)(not working|broken|error|bug|can't watch|loading|lag|down|crash|stuck|не работает|сломал|ошибка|баг|лагает|не грузит|не воспроизвод)`)
	serviceNames  = regexp.MustCompile(`(?i)\b(gateway|auth|catalog|streaming|player|rooms|scheduler|themes|kodik|animelib|hianime|consumet|aniwatch)\b`)
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
	msg.Text = m.Text
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

	if m.From != nil && isAdmin(m.From.Username, adminUsernames) {
		msg.Type = domain.MessageAdminMessage
		msg.Priority = domain.P1
		return msg
	}

	if issueKeywords.MatchString(m.Text) {
		msg.Type = domain.MessageUserIssue
		msg.Priority = domain.P2
		return msg
	}

	return msg
}

// ClassifyBatch groups classified messages into categories.
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
			batch.Relevant = append(batch.Relevant, classified)
		default:
			batch.Ignored++
		}
	}

	return batch
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

func isAdmin(username string, admins []string) bool {
	for _, admin := range admins {
		if strings.EqualFold(username, admin) {
			return true
		}
	}
	return false
}
