package grafana

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

func TestSeverity_ReadsLabel_DefaultsToWarning(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"explicit critical", map[string]string{"severity": "critical"}, SeverityCritical},
		{"explicit warning", map[string]string{"severity": "warning"}, SeverityWarning},
		{"explicit diagnostic", map[string]string{"severity": "diagnostic"}, SeverityDiagnostic},
		{"missing label defaults to warning", map[string]string{"alertname": "X"}, SeverityWarning},
		{"empty label defaults to warning", map[string]string{"severity": ""}, SeverityWarning},
		{"nil labels default to warning", nil, SeverityWarning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Severity(c.labels); got != c.want {
				t.Errorf("Severity(%v) = %q, want %q", c.labels, got, c.want)
			}
		})
	}
}

// TestPages_OnlyDiagnosticIsSilent is the core guard against AUTO-616/618: a
// diagnostic alert must never page, whatever its name or service.
func TestPages_OnlyDiagnosticIsSilent(t *testing.T) {
	if Pages(SeverityDiagnostic) {
		t.Error("diagnostic must not page — it is dashboard-only")
	}
	for _, s := range []string{SeverityCritical, SeverityWarning, "", "anything-else"} {
		if !Pages(s) {
			t.Errorf("severity %q must page (fail loud, never swallow an alert)", s)
		}
	}
}

func TestPriorityFor(t *testing.T) {
	if got := PriorityFor(SeverityCritical); got != domain.P0 {
		t.Errorf("critical → %v, want P0", got)
	}
	for _, s := range []string{SeverityWarning, "", "unknown"} {
		if got := PriorityFor(s); got != domain.P1 {
			t.Errorf("severity %q → %v, want P1", s, got)
		}
	}
}

// TestEverySeverityLabelInRulesIsKnown pins the contract to the actual
// provisioned rules: every `severity:` value in rules.yml must be one of the
// three this package understands. A rule shipping `severity: info` would
// otherwise silently page as a warning.
//
// This is the source-scan guard the deleted CriticalAlerts map never had — that
// map keyed on rule TITLE and drifted (it held "Player Unavailable" while the
// rule was titled "Kodik Player Unavailable"), silently downgrading it to P1.
func TestEverySeverityLabelInRulesIsKnown(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "docker", "grafana", "provisioning", "alerting", "rules.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("rules.yml not readable from this checkout: %v", err)
	}

	known := map[string]bool{
		SeverityCritical:   true,
		SeverityWarning:    true,
		SeverityDiagnostic: true,
	}
	re := regexp.MustCompile(`(?m)^\s*severity:\s*(\S+)\s*$`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("no severity labels found in rules.yml — the regex or the file layout changed")
	}
	for _, m := range matches {
		val := strings.Trim(m[1], `"'`)
		if !known[val] {
			t.Errorf("rules.yml declares unknown severity %q — add it to severity.go or fix the rule", val)
		}
	}
}
