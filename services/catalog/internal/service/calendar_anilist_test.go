package service

import (
	"testing"
	"time"
)

func TestLaterWins(t *testing.T) {
	early := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		shikimori    *time.Time
		anilist      *time.Time
		wantChosen   *time.Time
		wantFromAni  bool
	}{
		{"anilist later wins", &early, &late, &late, true},
		{"anilist earlier loses", &late, &early, &late, false},
		{"equal keeps shikimori", &early, &early, &early, false},
		{"anilist nil keeps shikimori", &early, nil, &early, false},
		{"both nil stays nil", nil, nil, nil, false},
		{"shikimori nil adopts anilist", nil, &late, &late, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chosen, fromAni := laterWins(tc.shikimori, tc.anilist)
			if fromAni != tc.wantFromAni {
				t.Errorf("fromAniList: want %v, got %v", tc.wantFromAni, fromAni)
			}
			switch {
			case tc.wantChosen == nil && chosen != nil:
				t.Errorf("chosen: want nil, got %v", chosen)
			case tc.wantChosen != nil && (chosen == nil || !chosen.Equal(*tc.wantChosen)):
				t.Errorf("chosen: want %v, got %v", tc.wantChosen, chosen)
			}
		})
	}
}
