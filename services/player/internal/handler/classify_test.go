package handler

import "testing"

func TestNormalizeSource(t *testing.T) {
	cases := []struct {
		raw, playerType, want string
	}{
		{"feedback_form", "feedback", "feedback_form"}, // canonical passes through
		{"manual", "feedback", "manual"},
		{"api", "feedback", "api"},
		{"telegram", "telegram", "telegram"},
		{"", "telegram", "telegram"},      // legacy telegram entry, no source field
		{"", "feedback", "feedback_form"}, // legacy user report, no source field
		{"owner-todo", "feedback", "api"}, // legacy AI/owner ledger
		{"repo-todo", "feedback", "api"},
	}
	for _, c := range cases {
		if got := normalizeSource(c.raw, c.playerType); got != c.want {
			t.Errorf("normalizeSource(%q,%q) = %q, want %q", c.raw, c.playerType, got, c.want)
		}
	}
}

func TestDeriveKind(t *testing.T) {
	cases := []struct {
		rawKind, source, want string
	}{
		{"idea", "manual", "idea"},        // explicit wins
		{"todo", "api", "todo"},           // explicit wins
		{"", "feedback_form", "feedback"}, // user channel → feedback
		{"", "telegram", "feedback"},
		{"", "api", "todo"}, // internal channel → todo
		{"", "manual", "todo"},
		{"bogus", "feedback_form", "feedback"}, // invalid kind ignored
	}
	for _, c := range cases {
		if got := deriveKind(c.rawKind, c.source); got != c.want {
			t.Errorf("deriveKind(%q,%q) = %q, want %q", c.rawKind, c.source, got, c.want)
		}
	}
}
