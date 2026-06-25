package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

type fakeAiringFetcher struct {
	byID   map[string]*idmapping.AniListAiring
	err    map[string]error
	called map[string]bool
}

func (f *fakeAiringFetcher) AniListAiringByMALID(_ context.Context, malID string) (*idmapping.AniListAiring, error) {
	if f.called == nil {
		f.called = map[string]bool{}
	}
	f.called[malID] = true
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
		"100": {shikimoriID: "100", nextEpisodeAt: &shiki, source: sourceShikimori, status: "ongoing"}, // anilist later → override
		"200": {shikimoriID: "200", nextEpisodeAt: &shiki, source: sourceShikimori, status: "ongoing"}, // anilist earlier → keep
		"300": {shikimoriID: "300", nextEpisodeAt: &shiki, source: sourceShikimori, status: "ongoing"}, // anilist nil airing → keep
		"400": {shikimoriID: "400", nextEpisodeAt: &shiki, source: sourceShikimori, status: "ongoing"}, // fetch error → keep
		"500": {shikimoriID: "500", nextEpisodeAt: nil, source: sourceShikimori, status: "ongoing"},    // shiki nil, anilist set → adopt
		"600": {shikimoriID: "600", nextEpisodeAt: &shiki, source: sourceShikimori, status: "anons"},   // NOT ongoing → skipped, never fetched
	}
	fake := &fakeAiringFetcher{
		byID: map[string]*idmapping.AniListAiring{
			"100": {AniListID: 1, Status: "RELEASING", NextEpisode: 12, NextAiringAt: &aniLater},
			"200": {AniListID: 2, Status: "RELEASING", NextEpisode: 5, NextAiringAt: &aniEarlier},
			"300": {AniListID: 3, Status: "FINISHED", NextEpisode: 0, NextAiringAt: nil},
			"500": {AniListID: 5, Status: "RELEASING", NextEpisode: 1, NextAiringAt: &aniLater},
			"600": {AniListID: 6, Status: "NOT_YET_RELEASED", NextEpisode: 1, NextAiringAt: &aniLater},
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
	assert("600", &shiki, sourceShikimori) // non-ongoing kept on Shikimori
	if fake.called["600"] {
		t.Errorf("600: non-ongoing anime must not be fetched from AniList")
	}
}

func TestDefendAniListNextEpisode(t *testing.T) {
	ani := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	shikiEarlier := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	shikiLater := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("defends anilist date against earlier shikimori refresh", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiEarlier}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(ani) || fresh.NextEpisodeSource != sourceAniList {
			t.Errorf("want defended anilist date, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("shikimori even-later date wins and source reverts", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiLater}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(shikiLater) || fresh.NextEpisodeSource != sourceShikimori {
			t.Errorf("want shikimori-later win, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("non-anilist existing is not defended; default source set", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: &shikiEarlier}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceShikimori}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(shikiEarlier) || fresh.NextEpisodeSource != sourceShikimori {
			t.Errorf("want shikimori kept, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})

	t.Run("defends when shikimori refresh has no date", func(t *testing.T) {
		fresh := &domain.Anime{NextEpisodeAt: nil}
		existing := &domain.Anime{NextEpisodeAt: &ani, NextEpisodeSource: sourceAniList}
		defendAniListNextEpisode(fresh, existing)
		if fresh.NextEpisodeAt == nil || !fresh.NextEpisodeAt.Equal(ani) || fresh.NextEpisodeSource != sourceAniList {
			t.Errorf("want defended anilist date when fresh nil, got %v src=%s", fresh.NextEpisodeAt, fresh.NextEpisodeSource)
		}
	})
}

func TestLaterWins(t *testing.T) {
	early := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		shikimori   *time.Time
		anilist     *time.Time
		wantChosen  *time.Time
		wantFromAni bool
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
