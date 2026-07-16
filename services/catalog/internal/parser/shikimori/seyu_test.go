package shikimori

import "testing"

func TestAbsImageURL(t *testing.T) {
	const base = "https://shikimori.io"
	cases := map[string]string{
		"/system/people/original/47918.jpg": "https://shikimori.io/system/people/original/47918.jpg",
		"https://cdn.example/x.jpg":         "https://cdn.example/x.jpg", // already absolute
		"":                                  "",                          // empty stays empty
	}
	for in, want := range cases {
		if got := absImageURL(base, in); got != want {
			t.Fatalf("absImageURL(%q) = %q, want %q", in, got, want)
		}
	}
}
