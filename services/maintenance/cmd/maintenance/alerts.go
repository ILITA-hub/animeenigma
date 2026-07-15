package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// checkResolvedAlerts detects alerts that were active but are no longer firing.
func (s *service) checkResolvedAlerts(currentAlerts []domain.ClassifiedMessage) {
	// Build set of currently firing alert keys
	currentKeys := make(map[string]bool)
	for _, a := range currentAlerts {
		if len(a.Alerts) > 0 {
			key := a.Alerts[0].Name + ":" + a.Alerts[0].Service
			currentKeys[key] = true
		}
	}

	// Check each active alert — if no longer in Grafana, it resolved
	st := s.state.State()
	for key, active := range st.ActiveAlerts {
		if !currentKeys[key] {
			log.Infow("grafana alert resolved", "alert_key", key)
			s.state.UpdateIssue(active.IssueID, func(issue *domain.Issue) {
				issue.Status = domain.StatusResolved
				issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
				issue.Resolution = "Alert resolved (no longer firing in Grafana)"
			})
			s.state.RemoveActiveAlert(key)

			// Notify in Telegram
			duration := "unknown"
			if firstSeen, err := time.Parse(time.RFC3339, active.FirstSeen); err == nil {
				duration = time.Since(firstSeen).Truncate(time.Second).String()
			}
			s.tg.SendMessage(fmt.Sprintf(
				"*✅ Alert Resolved*\n*Alert:* %s (%s)\n*Duration:* %s\n*Issue:* %s",
				active.AlertUID, active.Service, duration, active.IssueID,
			))
		}
	}
	s.state.Save()
}

// resolveAlertFromWebhook handles a resolve event pushed by the Grafana webhook.
// Invariant: state.RemoveActiveAlert MUST happen before tg.SendMessage so the
// reconciliation poller cannot re-emit the same resolve.
func (s *service) resolveAlertFromWebhook(key string, wa domain.GrafanaWebhookAlert) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := s.state.GetActiveAlert(key)
	if active == nil {
		log.Warnw("webhook resolve for unknown alert — skipping", "alert_key", key)
		return
	}

	log.Infow("webhook alert resolved", "alert_key", key)
	s.state.UpdateIssue(active.IssueID, func(issue *domain.Issue) {
		issue.Status = domain.StatusResolved
		issue.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
		issue.Resolution = "Alert resolved (webhook notification from Grafana)"
	})
	s.state.RemoveActiveAlert(key)

	duration := "unknown"
	if firstSeen, err := time.Parse(time.RFC3339, active.FirstSeen); err == nil {
		duration = time.Since(firstSeen).Truncate(time.Second).String()
	}
	s.tg.SendMessage(fmt.Sprintf(
		"*✅ Alert Resolved*\n*Alert:* %s (%s)\n*Duration:* %s\n*Issue:* %s",
		active.AlertUID, active.Service, duration, active.IssueID,
	))
	s.state.Save()
}

// scraperProviderRow is the subset of the catalog /internal/scraper/providers
// response used to summarise provider faults in scraper firing alerts.
type scraperProviderRow struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Status string `json:"status"` // enabled | degraded | disabled
	Health string `json:"health"` // up | recovering | down
	Reason string `json:"reason"`
}

// isScraperAlert reports whether a firing alert concerns the scraper / EN
// failover chain, so the provider fault summary is appended only where it's
// relevant (not on web/catalog/db alerts).
func isScraperAlert(alert domain.AlertInfo) bool {
	if strings.EqualFold(alert.Service, "scraper") {
		return true
	}
	name := strings.ToLower(alert.Name)
	return strings.Contains(name, "scraper") || strings.Contains(name, "parser")
}

// scraperProviderFaultLine fetches the live provider roster from the catalog
// and returns a one-line Telegram-Markdown summary of the EN failover providers
// that are currently unhealthy, each with its reason — e.g.
//
//	⚠️ Unhealthy: allanime (cdn_unreachable), okru (cdn_unreachable)
//
// Returns "" when the catalog is unreachable or no EN provider is faulted
// (fail-open: a catalog blip must never block the firing alert).
func (s *service) scraperProviderFaultLine() string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.CatalogURL+"/internal/scraper/providers", nil)
	if err != nil {
		return ""
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return "" // fail-open: catalog blip must not strip the alert
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Data struct {
			Providers []scraperProviderRow `json:"providers"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return formatProviderFaultLine(body.Data.Providers)
}

// formatProviderFaultLine builds the one-line unhealthy-provider summary from a
// provider roster. Pure (no I/O) for testability. Considers only the "en"
// failover group; lists providers that are degraded OR whose health is not
// "up" (so an in-chain provider with a pending one-fail warning or confirmed
// down still shows), excluding admin-disabled ones (intentionally off, not a
// fault).
// Returns "" when nothing is faulted.
func formatProviderFaultLine(providers []scraperProviderRow) string {
	var parts []string
	for _, p := range providers {
		if !strings.EqualFold(p.Group, "en") {
			continue
		}
		if strings.EqualFold(p.Status, "disabled") {
			continue
		}
		degraded := strings.EqualFold(p.Status, "degraded")
		if !degraded && strings.EqualFold(p.Health, "up") {
			continue
		}
		label := escTelegram(p.Name)
		if r := shortReason(p.Reason); r != "" {
			label += " (" + escTelegram(r) + ")"
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return ""
	}
	return "⚠️ Unhealthy: " + strings.Join(parts, ", ")
}

// shortReason trims a provider reason down to its leading machine code,
// dropping a trailing " on <host>" qualifier and capping length, so the fault
// line stays compact: "empty_response on tserver" → "empty_response",
// "cdn_unreachable on " → "cdn_unreachable".
func shortReason(reason string) string {
	r := strings.TrimSpace(reason)
	if i := strings.Index(r, " on"); i > 0 {
		if rest := r[i+3:]; rest == "" || strings.HasPrefix(rest, " ") {
			r = r[:i]
		}
	}
	r = strings.TrimSpace(r)
	const maxLen = 48
	if len([]rune(r)) > maxLen {
		r = string([]rune(r)[:maxLen-1]) + "…"
	}
	return r
}

func (s *service) escalateBatch(batch domain.ClassifiedBatch) {
	var alertNames []string
	for _, msg := range batch.Relevant {
		for _, a := range msg.Alerts {
			alertNames = append(alertNames, a.Name)
		}
	}

	text := fmt.Sprintf(
		"*⚠️ Multi-Service Outage Detected*\n\n"+
			"*Affected alerts:* %s\n"+
			"*Count:* 3+ services\n\n"+
			"Automated fixes disabled — likely infrastructure issue.\n"+
			"Manual investigation required.",
		strings.Join(alertNames, ", "),
	)

	if len(batch.Relevant) > 0 {
		s.tg.SendReply(batch.Relevant[0].MessageID, text)
	} else {
		s.tg.SendMessage(text)
	}
}

func (s *service) isSuppressed(alertKey string) bool {
	for _, suppressed := range s.cfg.SuppressedAlerts {
		if strings.EqualFold(alertKey, suppressed) {
			return true
		}
	}
	return false
}

// dropSuppressedAlerts removes firing alerts whose alertName:service key is in
// SUPPRESSED_ALERTS, so deferred alerts never reach the multi-service triage,
// escalateBatch, dedup, or analysis. Non-alert messages pass through unchanged.
func (s *service) dropSuppressedAlerts(msgs []domain.ClassifiedMessage) []domain.ClassifiedMessage {
	out := msgs[:0:0] // new backing array; do not alias
	for _, m := range msgs {
		if m.Type == domain.MessageAlertFiring && len(m.Alerts) > 0 &&
			s.isSuppressed(m.Alerts[0].Name+":"+m.Alerts[0].Service) {
			log.Infow("deferred alert (suppressed)", "alert", m.Alerts[0].Name, "service", m.Alerts[0].Service)
			continue
		}
		out = append(out, m)
	}
	return out
}
