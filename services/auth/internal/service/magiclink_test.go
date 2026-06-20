package service

import "testing"

func TestSanitizeOldURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/anime/abc", "/anime/abc"},
		{"/anime/abc?x=1&y=2", "/anime/abc?x=1&y=2"},
		{"", "/"},
		{"//evil.com", "/"},
		{"/\\evil.com", "/"},
		{"https://evil.com", "/"},
		{"http://evil.com/x", "/"},
		{"javascript:alert(1)", "/"},
		{"/path with space", "/path with space"},
		{"relative/no/leading/slash", "/"},
		{"/\t/control", "/"},
	}
	for _, c := range cases {
		if got := SanitizeOldURL(c.in); got != c.want {
			t.Errorf("SanitizeOldURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
