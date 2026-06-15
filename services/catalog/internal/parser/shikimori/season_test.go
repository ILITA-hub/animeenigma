package shikimori

import "testing"

func TestDetectSeason(t *testing.T) {
	cases := []struct {
		month int
		want  string
	}{
		{1, "winter"}, {2, "winter"}, {3, "winter"},
		{4, "spring"}, {5, "spring"}, {6, "spring"},
		{7, "summer"}, {8, "summer"}, {9, "summer"},
		{10, "fall"}, {11, "fall"}, {12, "fall"},
		{0, ""},  // missing month must NOT map to fall
		{13, ""}, // out of range
		{-1, ""}, // out of range
	}
	for _, c := range cases {
		if got := detectSeason(c.month); got != c.want {
			t.Errorf("detectSeason(%d) = %q, want %q", c.month, got, c.want)
		}
	}
}
