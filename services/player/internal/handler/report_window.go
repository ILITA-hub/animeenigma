// Package handler — report_window.go: shared submitted-at window filtering
// for the feedback listings (admin List + user ListMine).
//
// The frontend sends ?from=&to= as RFC3339 instants (local-day boundaries
// computed client-side, so "one day" means the USER's calendar day, not the
// server's). Either bound may be absent.
package handler

import (
	"net/url"
	"time"
)

// parseReportWindow extracts the optional [from, to] bounds. Invalid values
// are ignored (treated as absent) — a malformed filter must not 500 a listing.
func parseReportWindow(q url.Values) (from, to *time.Time) {
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}
	return from, to
}

// reportInWindow reports whether a row falls inside [from, to] (inclusive).
// The row's RFC3339 timestamp field is preferred; when missing/corrupt it
// falls back to the id's leading filename segment (2006-01-02T15-04-05,
// written in UTC by SubmitReport). Rows with no parseable time are kept only
// when no window is requested — silently dropping them would make reports
// "disappear" from an unfiltered-looking view.
func reportInWindow(timestamp, id string, from, to *time.Time) bool {
	if from == nil && to == nil {
		return true
	}
	t, ok := reportTime(timestamp, id)
	if !ok {
		return false
	}
	if from != nil && t.Before(*from) {
		return false
	}
	if to != nil && t.After(*to) {
		return false
	}
	return true
}

func reportTime(timestamp, id string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return t, true
	}
	const idTsLen = len("2006-01-02T15-04-05")
	if len(id) >= idTsLen {
		if t, err := time.Parse("2006-01-02T15-04-05", id[:idTsLen]); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
