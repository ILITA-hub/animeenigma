package grafana

import "testing"

func TestExtractService_GroupFallback(t *testing.T) {
	// Fleet alert: only a `group` label, no service/provider → return the group.
	if got := ExtractService(map[string]string{"group": "en"}, map[string]string{}); got != "en" {
		t.Errorf("group-only labels: got %q, want %q", got, "en")
	}
	if got := ExtractService(map[string]string{"group": "ru"}, map[string]string{}); got != "ru" {
		t.Errorf("group-only labels: got %q, want %q", got, "ru")
	}
	// `service` still wins over `group` when both present.
	if got := ExtractService(map[string]string{"service": "streaming", "group": "en"}, map[string]string{}); got != "streaming" {
		t.Errorf("service must win over group: got %q, want %q", got, "streaming")
	}
	// No recognizable labels at all → unknown (unchanged behavior).
	if got := ExtractService(map[string]string{}, map[string]string{}); got != "unknown" {
		t.Errorf("empty: got %q, want %q", got, "unknown")
	}
}
