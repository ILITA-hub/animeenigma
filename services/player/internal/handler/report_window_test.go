package handler

import (
	"net/url"
	"testing"
	"time"
)

func tp(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestParseReportWindow(t *testing.T) {
	q := url.Values{"from": {"2026-06-09T21:00:00Z"}, "to": {"2026-06-10T20:59:59Z"}}
	from, to := parseReportWindow(q)
	if from == nil || to == nil {
		t.Fatal("expected both bounds")
	}
	if !from.Equal(*tp("2026-06-09T21:00:00Z")) || !to.Equal(*tp("2026-06-10T20:59:59Z")) {
		t.Fatalf("got %v / %v", from, to)
	}

	// malformed values are ignored, not fatal
	from, to = parseReportWindow(url.Values{"from": {"yesterday"}, "to": {""}})
	if from != nil || to != nil {
		t.Fatalf("expected nil bounds, got %v / %v", from, to)
	}
}

func TestReportInWindow(t *testing.T) {
	from, to := tp("2026-06-10T00:00:00+03:00"), tp("2026-06-10T23:59:59+03:00")

	cases := []struct {
		name string
		ts   string
		id   string
		want bool
	}{
		{"inside via timestamp", "2026-06-10T12:00:00Z", "x", true},
		{"before window", "2026-06-09T11:00:00Z", "x", false},
		{"after window", "2026-06-11T12:00:00Z", "x", false},
		{"boundary: local midnight = 21:00Z prev day", "2026-06-09T21:00:00Z", "x", true},
		{"fallback to id segment", "garbage", "2026-06-10T14-30-45_user_feedback", true},
		{"fallback outside window", "garbage", "2026-06-12T14-30-45_user_feedback", false},
		{"unparseable row excluded when filtering", "garbage", "short", false},
	}
	for _, c := range cases {
		if got := reportInWindow(c.ts, c.id, from, to); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}

	if !reportInWindow("garbage", "short", nil, nil) {
		t.Error("unparseable row must be kept when no window requested")
	}
}
