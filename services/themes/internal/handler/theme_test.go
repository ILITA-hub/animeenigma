package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseListParams covers the limit/offset clamping logic added for finding
// L739 (GET /api/themes must be bounded — no unpaginated full-table grouped scan).
func TestParseListParams(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{name: "no limit defaults to 100", query: "", wantLimit: 100, wantOffset: 0},
		{name: "explicit limit honored", query: "limit=50", wantLimit: 50, wantOffset: 0},
		{name: "limit above hard max clamped to 500", query: "limit=9999", wantLimit: 500, wantOffset: 0},
		{name: "limit equal to hard max passes", query: "limit=500", wantLimit: 500, wantOffset: 0},
		{name: "zero limit falls back to default", query: "limit=0", wantLimit: 100, wantOffset: 0},
		{name: "negative limit falls back to default", query: "limit=-5", wantLimit: 100, wantOffset: 0},
		{name: "non-numeric limit falls back to default", query: "limit=abc", wantLimit: 100, wantOffset: 0},
		{name: "offset honored", query: "offset=20", wantLimit: 100, wantOffset: 20},
		{name: "negative offset clamped to zero", query: "offset=-3", wantLimit: 100, wantOffset: 0},
		{name: "non-numeric offset clamped to zero", query: "offset=xyz", wantLimit: 100, wantOffset: 0},
		{name: "limit and offset together", query: "limit=25&offset=10", wantLimit: 25, wantOffset: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/themes?"+tc.query, nil)
			params := parseListParams(r)
			if params.Limit != tc.wantLimit {
				t.Errorf("Limit = %d, want %d", params.Limit, tc.wantLimit)
			}
			if params.Offset != tc.wantOffset {
				t.Errorf("Offset = %d, want %d", params.Offset, tc.wantOffset)
			}
		})
	}
}

// TestParseListParamsPassesFilters verifies the existing season/type/sort/year
// parsing is preserved by the extracted helper.
func TestParseListParamsPassesFilters(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/themes?season=winter&type=op&sort=name&year=2024", nil)
	params := parseListParams(r)

	if params.Season != "winter" {
		t.Errorf("Season = %q, want %q", params.Season, "winter")
	}
	if params.Type != "op" {
		t.Errorf("Type = %q, want %q", params.Type, "op")
	}
	if params.Sort != "name" {
		t.Errorf("Sort = %q, want %q", params.Sort, "name")
	}
	if params.Year != 2024 {
		t.Errorf("Year = %d, want %d", params.Year, 2024)
	}
}

// TestParseListParamsInvalidYear ensures a non-numeric year is ignored (left zero).
func TestParseListParamsInvalidYear(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/themes?year=notayear", nil)
	params := parseListParams(r)
	if params.Year != 0 {
		t.Errorf("Year = %d, want 0", params.Year)
	}
}
