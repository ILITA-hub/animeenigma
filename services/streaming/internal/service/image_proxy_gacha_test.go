package service

import "testing"

// TestRewriteGachaURL proves rewriteGachaURL is the sole gate for relative
// gacha image URLs: it accepts only /api/gacha/images/{cards,banners}/<key>
// (strict key charset, no traversal), rewrites onto the configured internal
// gacha base, and rejects everything else (other prefixes, absolute URLs,
// unrelated relative paths).
func TestRewriteGachaURL(t *testing.T) {
	s := &ImageProxyService{gachaBaseURL: "http://gacha:8093"}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"/api/gacha/images/cards/ab-1.png", "http://gacha:8093/api/gacha/images/cards/ab-1.png", true},
		{"/api/gacha/images/banners/x.webp", "http://gacha:8093/api/gacha/images/banners/x.webp", true},
		{"/api/gacha/images/cards/../secret", "", false},
		{"/api/gacha/images/other/x.png", "", false},
		{"https://shikimori.one/x.png", "", false},
		{"/api/streaming/whatever", "", false},
	}
	for _, c := range cases {
		got, ok := s.rewriteGachaURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("rewriteGachaURL(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
