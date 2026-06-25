package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type fakeAiringFetcher struct {
	byID map[string]*idmapping.AniListAiring
	err  map[string]error
}

func (f *fakeAiringFetcher) AniListAiringByMALID(_ context.Context, malID string) (*idmapping.AniListAiring, error) {
	if e, ok := f.err[malID]; ok {
		return nil, e
	}
	return f.byID[malID], nil
}

func TestReconcileCalendarWithAniList(t *testing.T) {
	shiki := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	aniLater := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	aniEarlier := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	seen := map[string]*calendarInfo{
		"100": {shikimoriID: "100", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist later → override
		"200": {shikimoriID: "200", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist earlier → keep
		"300": {shikimoriID: "300", nextEpisodeAt: &shiki, source: sourceShikimori}, // anilist nil airing → keep
		"400": {shikimoriID: "400", nextEpisodeAt: &shiki, source: sourceShikimori}, // fetch error → keep
		"500": {shikimoriID: "500", nextEpisodeAt: nil, source: sourceShikimori},    // shiki nil, anilist set → adopt
	}
	fake := &fakeAiringFetcher{
		byID: map[string]*idmapping.AniListAiring{
			"100": {AniListID: 1, Status: "RELEASING", NextEpisode: 12, NextAiringAt: &aniLater},
			"200": {AniListID: 2, Status: "RELEASING", NextEpisode: 5, NextAiringAt: &aniEarlier},
			"300": {AniListID: 3, Status: "FINISHED", NextEpisode: 0, NextAiringAt: nil},
			"500": {AniListID: 5, Status: "RELEASING", NextEpisode: 1, NextAiringAt: &aniLater},
		},
		err: map[string]error{"400": errors.New("anilist down")},
	}

	s := &CatalogService{aniListAiring: fake, log: logger.Default()} // aniListReconcilePacing defaults to 0 → no sleeps
	s.reconcileCalendarWithAniList(context.Background(), seen)

	assert := func(id string, wantAt *time.Time, wantSrc string) {
		t.Helper()
		got := seen[id]
		if wantAt == nil && got.nextEpisodeAt != nil {
			t.Errorf("%s: want nil date, got %v", id, got.nextEpisodeAt)
		}
		if wantAt != nil && (got.nextEpisodeAt == nil || !got.nextEpisodeAt.Equal(*wantAt)) {
			t.Errorf("%s: want date %v, got %v", id, wantAt, got.nextEpisodeAt)
		}
		if got.source != wantSrc {
			t.Errorf("%s: want source %q, got %q", id, wantSrc, got.source)
		}
	}
	assert("100", &aniLater, sourceAniList)
	assert("200", &shiki, sourceShikimori)
	assert("300", &shiki, sourceShikimori)
	assert("400", &shiki, sourceShikimori)
	assert("500", &aniLater, sourceAniList)
}

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
