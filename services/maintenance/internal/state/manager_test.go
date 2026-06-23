package state

import (
	"path/filepath"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

func newManagerWithIssues(t *testing.T, issues []domain.Issue) *Manager {
	t.Helper()
	dir := t.TempDir()
	m := NewManager(filepath.Join(dir, "state.json"), filepath.Join(dir, "issues.json"))
	if err := m.Load(); err != nil {
		t.Fatalf("state load: %v", err)
	}
	for _, iss := range issues {
		// Insert directly so we can control Status/ID without going through CreateIssue numbering
		m.mu.Lock()
		m.issues.Issues = append(m.issues.Issues, iss)
		m.mu.Unlock()
	}
	return m
}

func TestFindOpenIssueByAlert_Dedup(t *testing.T) {
	m := newManagerWithIssues(t, []domain.Issue{
		{ID: "AUTO-100", Status: domain.StatusEscalated, AffectedService: "allanime", Source: "grafana_alert", Title: "allanime stream_segment DOWN"},
	})
	got := m.FindOpenIssueByAlert("Service Unreachable", "allanime")
	if got == nil || got.ID != "AUTO-100" {
		t.Fatalf("dedup miss: %+v", got)
	}
	// resolved issues must NOT match (re-fire after fix should open a new one)
	m2 := newManagerWithIssues(t, []domain.Issue{{ID: "AUTO-1", Status: domain.StatusResolved, AffectedService: "allanime", Title: "some old issue"}})
	if m2.FindOpenIssueByAlert("x", "allanime") != nil {
		t.Fatal("resolved issue must not dedup-match")
	}
	// auto_fixed issues must NOT match (re-fire after auto-fix is a new incident)
	m3 := newManagerWithIssues(t, []domain.Issue{{ID: "AUTO-2", Status: domain.StatusAutoFixed, AffectedService: "allanime", Title: "auto fixed issue"}})
	if m3.FindOpenIssueByAlert("x", "allanime") != nil {
		t.Fatal("auto_fixed issue must not dedup-match")
	}
}
