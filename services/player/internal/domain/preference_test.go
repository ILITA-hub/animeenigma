package domain

import "testing"

func TestValidateCombo_NewEnumValues(t *testing.T) {
	cases := []struct {
		player, language, watchType string
		want                        bool
	}{
		{"ae", "en", "sub", true},
		{"ae", "ru", "dub", true},
		{"raw", "ja", "sub", true},
		{"kodik", "ru", "dub", true}, // existing still valid
		{"bogus", "en", "sub", false},
		{"ae", "klingon", "sub", false},
		{"", "", "", true}, // empty = no combo, valid
		// EN scraper (unified player posts player='english')
		{"english", "en", "sub", true},
		{"english", "en", "dub", true},
		// 18+ / adult player
		{"hanime", "ru", "dub", true},
		// AnimeJoy RU-sub legs (Sibnet/AllVideo)
		{"animejoy-sibnet", "ru", "sub", true},
		{"animejoy-allvideo", "ru", "sub", true},
	}
	for _, c := range cases {
		if got := ValidateCombo(c.player, c.language, c.watchType); got != c.want {
			t.Errorf("ValidateCombo(%q,%q,%q)=%v want %v", c.player, c.language, c.watchType, got, c.want)
		}
	}
}
