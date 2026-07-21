package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/maintenancegate"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/config"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/state"
)

func newTestService(t *testing.T, admins []string) *service {
	t.Helper()
	dir := t.TempDir()
	m := state.NewManager(filepath.Join(dir, "state.json"), filepath.Join(dir, "issues.json"))
	if err := m.Load(); err != nil {
		t.Fatalf("state load: %v", err)
	}
	return &service{
		state: m,
		cfg:   &config.Config{Admins: admins},
	}
}

// newGateServer stands up a fake policy-service /internal/maintenance/routines/{id}
// endpoint returning the given enabled flag + auto_apply_max_risk setting, in the
// envelope shape maintenancegate.Client expects.
func newGateServer(t *testing.T, enabled bool, maxRisk string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings := map[string]any{}
		if maxRisk != "" {
			settings["auto_apply_max_risk"] = maxRisk
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"enabled":  enabled,
				"settings": settings,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func buttonResult(risk domain.FixRisk, category, target string) *domain.AnalysisResult {
	return &domain.AnalysisResult{
		Tier: domain.TierButtonFix,
		Risk: risk,
		Issue: domain.IssueInfo{
			Title: "t", Category: category, Priority: "P2", Status: "open",
		},
		FixPlan: &domain.FixPlan{Type: domain.FixCodeFix, Target: target, Description: "d"},
	}
}

func msgFrom(username string) domain.ClassifiedMessage {
	return domain.ClassifiedMessage{Type: domain.MessageErrorReport, From: domain.User{Username: username}}
}

func TestIsRealBug(t *testing.T) {
	real := []string{"bug", "outage", "regression", "stability", "content-quality", "Degradation", "PARSER_FAILURE", "crash", "data-integrity"}
	for _, c := range real {
		if !isRealBug(c) {
			t.Errorf("isRealBug(%q) = false, want true", c)
		}
	}
	notReal := []string{"feature", "latency", "capacity", "alert_misconfiguration", "false_positive_alert", ""}
	for _, c := range notReal {
		if isRealBug(c) {
			t.Errorf("isRealBug(%q) = true, want false", c)
		}
	}
}

func TestDecideAutoApply(t *testing.T) {
	const admin = "0neymik0"

	tests := []struct {
		name      string
		risk      domain.FixRisk
		category  string
		sender    string // "" = grafana/non-admin, admin = admin
		wantApply bool
	}{
		{"low risk auto-applies for anyone", domain.RiskLow, "bug", "", true},
		{"low risk auto-applies for feedback latency too", domain.RiskLow, "latency", "", true},
		// audit #5: a medium-risk bug from an END-USER report must NOT auto-apply
		// (write+deploy+push) — it needs the admin button.
		{"medium risk real bug from end-user needs button", domain.RiskMedium, "bug", "", false},
		{"medium risk content-quality from end-user needs button", domain.RiskMedium, "content-quality", "", false},
		{"medium risk real bug from admin auto-applies", domain.RiskMedium, "bug", admin, true},
		{"medium risk non-bug from admin auto-applies", domain.RiskMedium, "latency", admin, true},
		{"medium risk non-bug from non-admin needs button", domain.RiskMedium, "latency", "", false},
		{"high risk never auto-applies", domain.RiskHigh, "bug", admin, false},
		{"unset risk never auto-applies", domain.FixRisk(""), "bug", admin, false},
		{"feature never auto-implemented even at low risk", domain.RiskLow, "feature", admin, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestService(t, []string{admin})
			// Unique target per case so the loop guard never interferes.
			res := buttonResult(tt.risk, tt.category, "svc-"+tt.name)
			apply, label, reason := s.decideAutoApply(msgFrom(tt.sender), res)
			if apply != tt.wantApply {
				t.Fatalf("apply=%v want=%v (label=%q reason=%q)", apply, tt.wantApply, label, reason)
			}
			if apply && label == "" {
				t.Errorf("auto-apply returned empty label")
			}
			if !apply && reason == "" {
				t.Errorf("button path returned empty reason")
			}
		})
	}
}

// TestDecideAutoApply_GrafanaAlertBugAutoApplies confirms a Grafana alert (a
// trusted internal source) still auto-applies a medium-risk bug fix (audit #5).
func TestDecideAutoApply_GrafanaAlertBugAutoApplies(t *testing.T) {
	s := newTestService(t, []string{"0neymik0"})
	res := buttonResult(domain.RiskMedium, "bug", "svc-grafana")
	grafanaMsg := domain.ClassifiedMessage{
		Type: domain.MessageAlertFiring,
		From: domain.User{Username: "grafana", IsBot: true},
	}
	apply, _, reason := s.decideAutoApply(grafanaMsg, res)
	if !apply {
		t.Fatalf("Grafana-sourced medium bug must auto-apply; got apply=false reason=%q", reason)
	}
}

// TestDecideAutoApply_GateDisabled: when the maintenance_bot routine is
// disabled via /admin/policy, decideAutoApply must refuse to auto-apply even
// a low-risk fix — the daemon falls back to the admin button path.
func TestDecideAutoApply_GateDisabled(t *testing.T) {
	srv := newGateServer(t, false, "")
	s := newTestService(t, nil)
	s.maint = maintenancegate.New(srv.URL, time.Second)

	res := buttonResult(domain.RiskLow, "bug", "svc-gate-disabled")
	apply, _, reason := s.decideAutoApply(msgFrom(""), res)
	if apply {
		t.Fatalf("gate disabled: apply=true, want false (reason=%q)", reason)
	}
	if reason == "" {
		t.Error("gate-disabled button path returned empty reason")
	}
}

func TestMaintenanceEnabled(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		srv := newGateServer(t, false, "")
		s := newTestService(t, nil)
		s.maint = maintenancegate.New(srv.URL, time.Second)
		if s.maintenanceEnabled(t.Context()) {
			t.Fatal("maintenanceEnabled = true; want false")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		srv := newGateServer(t, true, "")
		s := newTestService(t, nil)
		s.maint = maintenancegate.New(srv.URL, time.Second)
		if !s.maintenanceEnabled(t.Context()) {
			t.Fatal("maintenanceEnabled = false; want true")
		}
	})

	t.Run("unreachable fails open", func(t *testing.T) {
		s := newTestService(t, nil)
		s.maint = maintenancegate.New("http://127.0.0.1:1/", 200*time.Millisecond)
		if !s.maintenanceEnabled(t.Context()) {
			t.Fatal("maintenanceEnabled = false; unreachable policy must fail open")
		}
	})
}

// TestDecideAutoApply_RiskCeiling_LowCapsMedium: auto_apply_max_risk="low"
// must cap a medium-risk admin-sourced bug fix that would otherwise auto-apply.
func TestDecideAutoApply_RiskCeiling_LowCapsMedium(t *testing.T) {
	const admin = "0neymik0"
	srv := newGateServer(t, true, "low")
	s := newTestService(t, []string{admin})
	s.maint = maintenancegate.New(srv.URL, time.Second)

	res := buttonResult(domain.RiskMedium, "bug", "svc-ceiling-low")
	apply, _, reason := s.decideAutoApply(msgFrom(admin), res)
	if apply {
		t.Fatalf("max_risk=low must cap a medium-risk fix: apply=true, want false (reason=%q)", reason)
	}
	if reason == "" {
		t.Error("risk-ceiling button path returned empty reason")
	}
}

// TestDecideAutoApply_RiskCeiling_NoneCapsLow: auto_apply_max_risk="none"
// must cap even a normally-always-auto low-risk fix.
func TestDecideAutoApply_RiskCeiling_NoneCapsLow(t *testing.T) {
	srv := newGateServer(t, true, "none")
	s := newTestService(t, nil)
	s.maint = maintenancegate.New(srv.URL, time.Second)

	res := buttonResult(domain.RiskLow, "bug", "svc-ceiling-none")
	apply, _, reason := s.decideAutoApply(msgFrom(""), res)
	if apply {
		t.Fatalf("max_risk=none must cap a low-risk fix: apply=true, want false (reason=%q)", reason)
	}
	if reason == "" {
		t.Error("risk-ceiling button path returned empty reason")
	}
}

// TestDecideAutoApply_GateUnreachable_FailsOpen: when the policy service is
// unreachable, both Enabled and MaxRisk fail open (enabled=true, no cap), so
// decideAutoApply behaves exactly as it did before the gate existed.
func TestDecideAutoApply_GateUnreachable_FailsOpen(t *testing.T) {
	s := newTestService(t, nil)
	s.maint = maintenancegate.New("http://127.0.0.1:1/", 200*time.Millisecond) // unreachable

	res := buttonResult(domain.RiskLow, "bug", "svc-gate-unreachable")
	apply, label, reason := s.decideAutoApply(msgFrom(""), res)
	if !apply {
		t.Fatalf("unreachable gate must fail open: apply=false, want true (reason=%q)", reason)
	}
	if label == "" {
		t.Error("auto-apply returned empty label")
	}
}

func TestDecideAutoApply_LoopGuard_AttemptCap(t *testing.T) {
	s := newTestService(t, nil)
	res := buttonResult(domain.RiskLow, "bug", "catalog")
	// Two attempts within 30m auto-apply; the third is downgraded to a button.
	for i := 1; i <= 2; i++ {
		if apply, _, reason := s.decideAutoApply(msgFrom(""), res); !apply {
			t.Fatalf("attempt %d: apply=false, want true (reason=%q)", i, reason)
		}
	}
	if apply, _, reason := s.decideAutoApply(msgFrom(""), res); apply {
		t.Fatalf("attempt 3: apply=true, want false (loop guard); reason=%q", reason)
	}
}

func TestDecideAutoApply_LoopGuard_RecentlyFixed(t *testing.T) {
	s := newTestService(t, nil)
	s.state.RecordFix("catalog", "code_fix")
	res := buttonResult(domain.RiskLow, "bug", "catalog")
	if apply, _, reason := s.decideAutoApply(msgFrom(""), res); apply {
		t.Fatalf("apply=true, want false (recently fixed); reason=%q", reason)
	}
}

func TestDecideAutoApply_NonButtonTier(t *testing.T) {
	s := newTestService(t, nil)
	res := &domain.AnalysisResult{Tier: domain.TierInfoOnly, Risk: domain.RiskLow}
	if apply, _, _ := s.decideAutoApply(msgFrom(""), res); apply {
		t.Fatalf("info_only tier should never auto-apply")
	}
}

func TestIsAdminMessage(t *testing.T) {
	s := newTestService(t, []string{"0neymik0", "NANDIorg"})
	if !s.isAdminMessage(msgFrom("0neymik0")) {
		t.Error("expected admin match (exact)")
	}
	if !s.isAdminMessage(msgFrom("0NEYMIK0")) {
		t.Error("expected admin match (case-insensitive)")
	}
	if s.isAdminMessage(msgFrom("Oronemu")) {
		t.Error("non-admin user should not match")
	}
	if s.isAdminMessage(msgFrom("")) {
		t.Error("empty username (grafana alert) should not match")
	}
}
