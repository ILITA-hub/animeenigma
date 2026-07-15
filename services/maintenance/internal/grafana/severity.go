package grafana

import "github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"

// Severity levels carried by the `severity` label that every rule in
// docker/grafana/provisioning/alerting/rules.yml sets. The rule's own label is
// the single source of truth for how loud an alert is — this package reads it
// rather than keeping a second opinion in Go. (Before 2026-07-15 a hardcoded
// CriticalAlerts name→P0 map lived here; it drifted out of sync with the rule
// titles it mirrored — "Player Unavailable" vs the actual "Kodik Player
// Unavailable" — and silently downgraded that rule to P1.)
const (
	// SeverityDiagnostic — dashboard-only. Panels and state history keep
	// working; the alert never pages. Grafana's notification policy routes
	// these through the always-on mute timing, and Pages() is the belt-and-
	// braces guard on this side of the webhook.
	SeverityDiagnostic = "diagnostic"
	// SeverityCritical — P0 page.
	SeverityCritical = "critical"
	// SeverityWarning — P1 page. Also the default for a rule with no label.
	SeverityWarning = "warning"
)

// Severity returns an alert's severity label, defaulting to warning for a rule
// that does not set one (fail loud rather than silently swallowing an alert).
func Severity(labels map[string]string) string {
	if s := labels["severity"]; s != "" {
		return s
	}
	return SeverityWarning
}

// Pages reports whether an alert of this severity should reach Telegram.
func Pages(severity string) bool {
	return severity != SeverityDiagnostic
}

// PriorityFor maps a severity to the issue priority used for triage.
func PriorityFor(severity string) domain.Priority {
	if severity == SeverityCritical {
		return domain.P0
	}
	return domain.P1
}
