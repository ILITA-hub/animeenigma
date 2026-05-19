package handler

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	allowed := []string{"https://animeenigma.ru", "http://localhost:5173"}
	check := buildOriginCheck(allowed)

	cases := []struct {
		origin string
		want   bool
	}{
		{"https://animeenigma.ru", true},
		{"http://localhost:5173", true},
		{"https://evil.com", false},
		{"", false},
		{"https://animeenigma.ru.evil.com", false},
	}
	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			got := check(r)
			if got != c.want {
				t.Errorf("origin %q: got %v, want %v", c.origin, got, c.want)
			}
		})
	}

	// Empty allowlist must reject everything (fail-closed).
	deny := buildOriginCheck(nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://animeenigma.ru")
	if deny(r) {
		t.Fatal("empty allowlist must reject")
	}
}
