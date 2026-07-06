package ffmpeg

import "testing"

func TestLangMatches(t *testing.T) {
	cases := []struct {
		tag, want string
		match     bool
	}{
		{"eng", "eng", true},
		{"en", "eng", true},
		{"English", "eng", true},
		{"eng", "en", true},
		{"jpn", "eng", false},
		{"ja", "jpn", true},
		{"japanese", "jpn", true},
		{"rus", "ru", true},
		{"", "eng", false},
		{"eng", "", false},
		{"und", "eng", false},
	}
	for _, c := range cases {
		if got := langMatches(c.tag, c.want); got != c.match {
			t.Errorf("langMatches(%q,%q)=%v want %v", c.tag, c.want, got, c.match)
		}
	}
}

func TestSelectAudioOrdinal(t *testing.T) {
	mk := func(langs ...string) []probedAudioStream {
		out := make([]probedAudioStream, len(langs))
		for i, l := range langs {
			out[i].Tags.Language = l
		}
		return out
	}

	// Judas Black Lagoon layout: JP first, EN second → eng is ordinal 1.
	if ord, ok := selectAudioOrdinal(mk("jpn", "eng"), "eng"); !ok || ord != 1 {
		t.Fatalf("dual-audio jpn,eng: got ord=%d ok=%v want 1,true", ord, ok)
	}
	// English first → ordinal 0.
	if ord, ok := selectAudioOrdinal(mk("eng", "jpn"), "eng"); !ok || ord != 0 {
		t.Fatalf("eng first: got ord=%d ok=%v want 0,true", ord, ok)
	}
	// No English track → fall back (ok=false).
	if _, ok := selectAudioOrdinal(mk("jpn", "rus"), "eng"); ok {
		t.Fatalf("no eng track: expected ok=false")
	}
	// Empty stream list → fall back.
	if _, ok := selectAudioOrdinal(nil, "eng"); ok {
		t.Fatalf("no streams: expected ok=false")
	}
	// 2-letter tag still matches.
	if ord, ok := selectAudioOrdinal(mk("jpn", "en"), "eng"); !ok || ord != 1 {
		t.Fatalf("2-letter en: got ord=%d ok=%v want 1,true", ord, ok)
	}
}
