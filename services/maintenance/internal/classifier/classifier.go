package classifier

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/telegram"
)

var (
	// Issue keywords (EN + RU)
	issueKeywords = regexp.MustCompile(`(?i)(not working|broken|error|bug|can't watch|loading|lag|down|crash|stuck|не работает|сломал|ошибка|баг|лагает|не грузит|не воспроизвод)`)

	// Known service names for extraction from alert summaries
	serviceNames = regexp.MustCompile(`(?i)\b(gateway|auth|catalog|streaming|player|rooms|scheduler|themes|kodik|animelib|hianime|consumet|aniwatch)\b`)

	// Critical alerts (P0)
	criticalAlerts = map[string]bool{
		"Service Unreachable":  true,
		"Scheduler Sync Stale": true,
		"Player Unavailable":   true,
	}
)

// Classify determines the type and priority of a Telegram update.
func Classify(update telegram.Update, alertsBotID int64, adminUsernames []string) domain.ClassifiedMessage {
	raw, _ := json.Marshal(update)

	msg := domain.ClassifiedMessage{
		UpdateID: update.UpdateID,
		RawJSON:  string(raw),
		Type:     domain.MessageIgnore,
		Priority: domain.P3,
	}

	// Handle callback queries (button clicks)
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

	// Handle regular messages
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

	// Check if from our bot (Grafana alerts + error reports)
	// Check if from the alerts bot (Grafana alerts + error reports)
	if m.From != nil && m.From.IsBot && m.From.ID == alertsBotID {
		return classifyBotMessage(msg)
	}

	// Check if from admin
	if m.From != nil && isAdmin(m.From.Username, adminUsernames) {
		msg.Type = domain.MessageAdminMessage
		msg.Priority = domain.P1
		return msg
	}

	// Check for issue keywords
	if issueKeywords.MatchString(m.Text) {
		msg.Type = domain.MessageUserIssue
		msg.Priority = domain.P2
		return msg
	}

	// Ignore everything else
	return msg
}

// ClassifyBatch groups classified messages into categories.
func ClassifyBatch(updates []telegram.Update, alertsBotID int64, adminUsernames []string) domain.ClassifiedBatch {
	var batch domain.ClassifiedBatch

	for _, u := range updates {
		classified := Classify(u, alertsBotID, adminUsernames)

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

func classifyBotMessage(msg domain.ClassifiedMessage) domain.ClassifiedMessage {
	text := msg.Text

	// Error report
	if strings.Contains(text, "🚨") || strings.Contains(text, "Player Error Report") {
		msg.Type = domain.MessageErrorReport
		msg.Priority = domain.P2
		return msg
	}

	// Grafana alert - Firing
	if strings.Contains(text, "🔴") && containsCaseInsensitive(text, "firing") {
		msg.Type = domain.MessageAlertFiring
		msg.Alerts = parseGrafanaAlerts(text)
		// Priority based on alert severity
		msg.Priority = domain.P1
		for _, a := range msg.Alerts {
			if a.Severity == "critical" {
				msg.Priority = domain.P0
				break
			}
		}
		return msg
	}

	// Grafana alert - Resolved
	if strings.Contains(text, "🟢") && containsCaseInsensitive(text, "resolved") {
		msg.Type = domain.MessageAlertResolved
		msg.Alerts = parseGrafanaAlerts(text)
		return msg
	}

	return msg
}

// parseGrafanaAlerts extracts alert info from a Grafana notification.
// Template: 🔴 Firing\n\n{alertname}\n{summary}\n{description}
// Grafana can batch multiple alerts in one message.
func parseGrafanaAlerts(text string) []domain.AlertInfo {
	var alerts []domain.AlertInfo

	// Split by double newline to separate header from alert blocks
	parts := strings.Split(text, "\n\n")
	if len(parts) < 2 {
		// Try single newline separation
		return parseAlertFromLines(strings.Split(text, "\n"))
	}

	// First part is the status header (🔴 Firing or 🟢 Resolved)
	// Remaining parts are alert blocks
	for _, block := range parts[1:] {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		blockAlerts := parseAlertFromLines(lines)
		alerts = append(alerts, blockAlerts...)
	}

	return alerts
}

func parseAlertFromLines(lines []string) []domain.AlertInfo {
	var alerts []domain.AlertInfo

	if len(lines) == 0 {
		return alerts
	}

	alert := domain.AlertInfo{}

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if i == 0 && !strings.Contains(line, "🔴") && !strings.Contains(line, "🟢") {
			// First non-status line is the alert name
			alert.Name = line
			alert.Severity = "warning"
			if criticalAlerts[line] {
				alert.Severity = "critical"
			}
		} else if alert.Name != "" && alert.Summary == "" {
			alert.Summary = line
			// Extract service name from summary
			if matches := serviceNames.FindStringSubmatch(line); len(matches) > 1 {
				alert.Service = strings.ToLower(matches[1])
			}
		} else if alert.Name != "" && alert.Description == "" {
			alert.Description = line
			// Also try to extract service from description if not found in summary
			if alert.Service == "" {
				if matches := serviceNames.FindStringSubmatch(line); len(matches) > 1 {
					alert.Service = strings.ToLower(matches[1])
				}
			}
		}
	}

	if alert.Name != "" {
		alerts = append(alerts, alert)
	}

	return alerts
}

func isAdmin(username string, admins []string) bool {
	for _, admin := range admins {
		if strings.EqualFold(username, admin) {
			return true
		}
	}
	return false
}

func containsCaseInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
