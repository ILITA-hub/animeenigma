package main

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/feedback"
)

// feedbackStatusFor must NEVER yield "resolved" — that status is human-only
// (CLAUDE.md). It must also keep an admin "add to todo" capture as a single
// open ("new") task rather than marking it done.
func TestFeedbackStatusFor(t *testing.T) {
	res := func(tier domain.FixTier, issueStatus string) *domain.AnalysisResult {
		return &domain.AnalysisResult{Tier: tier, Issue: domain.IssueInfo{Status: issueStatus}}
	}

	cases := []struct {
		name   string
		result *domain.AnalysisResult
		want   string
	}{
		{"applied fix → ai_done", res(domain.TierAutoFix, "auto_fixed"), feedback.StatusAIDone},
		{"answered info request → ai_done (never resolved)", res(domain.TierInfoOnly, "open"), feedback.StatusAIDone},
		{"alert-resolved tier → ai_done (never resolved)", res(domain.TierResolved, "resolved"), feedback.StatusAIDone},
		{"captured todo stays a single open task", res(domain.TierInfoOnly, "captured"), feedback.StatusNew},
		{"backlogged todo stays open", res(domain.TierInfoOnly, "backlog"), feedback.StatusNew},
		{"captured is case/space-insensitive", res(domain.TierInfoOnly, "  Captured "), feedback.StatusNew},
		// Phrasing drift: the Claude CLI's --json-schema is best-effort, so the LLM
		// may emit a longer capture phrase than the bare token. These must still map
		// to a single open task, not leak to ai_done.
		{"captured-for-later phrase stays new", res(domain.TierInfoOnly, "captured for later"), feedback.StatusNew},
		{"backlog phrase stays new", res(domain.TierInfoOnly, "backlog this item"), feedback.StatusNew},
		{"to do (spaced) stays new", res(domain.TierInfoOnly, "to do"), feedback.StatusNew},
		{"TODO: prefix on resolved tier stays new", res(domain.TierResolved, "TODO: investigate"), feedback.StatusNew},
		// Guard: legitimate completion/lifecycle statuses must NOT be mis-captured as
		// todo (they carry no capture marker), so they stay ai_done.
		{"auto_fixed is not a captured todo", res(domain.TierInfoOnly, "auto_fixed"), feedback.StatusAIDone},
		{"open is not a captured todo", res(domain.TierInfoOnly, "open"), feedback.StatusAIDone},
		{"resolved-word on resolved tier stays ai_done", res(domain.TierResolved, "resolved"), feedback.StatusAIDone},
		{"button fix pending → in_progress", res(domain.TierButtonFix, "open"), feedback.StatusInProgress},
		{"escalation → in_progress", res(domain.TierEscalate, "escalated"), feedback.StatusInProgress},
		// A captured backlog item only matters for the info/resolved tiers; an
		// escalation that happens to be "captured" still needs human work.
		{"escalation is not downgraded to new", res(domain.TierEscalate, "captured"), feedback.StatusInProgress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := feedbackStatusFor(tc.result)
			if got == feedback.StatusResolved {
				t.Fatalf("feedbackStatusFor returned human-only 'resolved' for %q", tc.name)
			}
			if got != tc.want {
				t.Fatalf("feedbackStatusFor = %q, want %q", got, tc.want)
			}
		})
	}
}
