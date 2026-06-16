package shikimori

import "testing"

func TestSanitizeDescription(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "A strong mage.", "A strong mage."},
		{"empty", "", ""},
		{
			"character link",
			"Приёмная дочь [character=196826]Хайтера[/character].",
			"Приёмная дочь Хайтера.",
		},
		{
			"multiple tags",
			"[b]Fern[/b] is a pupil of [character=184947]Frieren[/character].",
			"Fern is a pupil of Frieren.",
		},
		{
			"url tag",
			"See [url=https://x.test]source[/url] here.",
			"See source here.",
		},
		{"trim", "  spaced  ", "spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeDescription(tc.in); got != tc.want {
				t.Fatalf("sanitizeDescription(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeRole(t *testing.T) {
	cases := map[string]string{
		"Main":       "main",
		"Supporting": "supporting",
		"":           "supporting",
		"Background": "supporting",
	}
	for in, want := range cases {
		if got := normalizeRole([]string{in}); got != want {
			t.Fatalf("normalizeRole([%q]) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeRole(nil); got != "supporting" {
		t.Fatalf("normalizeRole(nil) = %q, want supporting", got)
	}
}
