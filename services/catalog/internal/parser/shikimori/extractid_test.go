package shikimori

import "testing"

func TestExtractShikimoriID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"5114", "5114"},
		{"  5114  ", "5114"},
		{"z5114", "5114"},
		{"5114-cowboy-bebop", "5114"},
		{"z5114-cowboy-bebop", "5114"},
		{"https://shikimori.one/animes/z5114", "5114"},
		{"https://shikimori.one/animes/z5114-cowboy-bebop", "5114"},
		{"https://shikimori.me/animes/23273-shigatsu-wa-kimi-no-uso", "23273"},
		{"http://shikimori.io/animes/1", "1"},
		{"shikimori.one/animes/5114", "5114"},
		// No id present — returned unchanged.
		{"https://shikimori.one/animes", "https://shikimori.one/animes"},
		{"cowboy-bebop", "cowboy-bebop"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ExtractShikimoriID(c.in); got != c.want {
			t.Errorf("ExtractShikimoriID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
