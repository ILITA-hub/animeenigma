package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

type fakeFinder struct {
	anime *domain.Anime
	err   error
}

func (f fakeFinder) GetByShikimoriID(_ context.Context, _ string) (*domain.Anime, error) {
	return f.anime, f.err
}

type fakeScraper struct {
	status int
	body   []byte
	err    error
}

func (f fakeScraper) GetScraperEpisodes(_ context.Context, _, _ string, _ bool) (int, []byte, error) {
	return f.status, f.body, f.err
}

type fakeRaw struct {
	lib *EpisodesResponse
	err error
}

func (f fakeRaw) GetLibraryEpisodes(_ context.Context, _ string) (*EpisodesResponse, error) {
	return f.lib, f.err
}

type fakeAnimejoy struct {
	info *domain.AnimejoyLegInfo
	err  error
}

func (f fakeAnimejoy) GetAnimejoyLegInfo(_ context.Context, _, _ string) (*domain.AnimejoyLegInfo, error) {
	return f.info, f.err
}

func newResolver(fnd animeFinder, scr scraperEpisodeLister, raw rawEpisodeLister, aj animejoyLegLister) *animeLevelResolver {
	return &animeLevelResolver{finder: fnd, scraper: scr, raw: raw, animejoy: aj}
}

func TestIsAnimeLevelPlayer(t *testing.T) {
	for _, p := range []string{"english", "ae", "animejoy-sibnet", "animejoy-allvideo"} {
		if !IsAnimeLevelPlayer(p) {
			t.Errorf("IsAnimeLevelPlayer(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"kodik", "animelib", "hanime", ""} {
		if IsAnimeLevelPlayer(p) {
			t.Errorf("IsAnimeLevelPlayer(%q) = true, want false", p)
		}
	}
}

func TestAnimeLevel_EnglishSub_MaxEpisode(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1},{"number":12},{"number":7}]}}`)},
		fakeRaw{},
		nil,
	)
	latest, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if latest != 12 {
		t.Errorf("latest = %d, want 12", latest)
	}
}

func TestAnimeLevel_EnglishDub_MaxWhereHasDub(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1,"has_dub":true},{"number":2,"has_dub":true},{"number":3,"has_dub":false}]}}`)},
		fakeRaw{},
		nil,
	)
	latest, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err != nil || latest != 2 {
		t.Fatalf("english dub latest = %d, err = %v; want 2, nil", latest, err)
	}
}

func TestAnimeLevel_EnglishDub_NoneIsNotFound(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2}]}}`)},
		fakeRaw{},
		nil,
	)
	_, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("english dub (no dub eps) err = %v; want NotFound-like", err)
	}
}

func TestAnimeLevel_EnglishSub_EmptyIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[]}}`)}, fakeRaw{}, nil)
	_, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("empty english err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_AE_MaxFromLibrary(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{lib: &EpisodesResponse{Available: true, Episodes: []RawEpisode{{Number: 3}, {Number: 9}, {Number: 5}}}},
		nil,
	)
	latest, _, err := r.Latest(context.Background(), "57466", "ae", "sub")
	if err != nil || latest != 9 {
		t.Fatalf("ae latest = %d, err = %v, want 9, nil", latest, err)
	}
}

func TestAnimeLevel_AnimejoySibnet_MaxEpisode(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{},
		fakeAnimejoy{info: &domain.AnimejoyLegInfo{Episodes: []int{1, 3, 2}}},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "animejoy-sibnet", "sub")
	if err != nil || latest != 3 {
		t.Fatalf("animejoy-sibnet latest = %d, err = %v, want 3, nil", latest, err)
	}
}

func TestAnimeLevel_AnimejoyAllVideo_MaxEpisode(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{},
		fakeAnimejoy{info: &domain.AnimejoyLegInfo{Episodes: []int{1, 2, 5}}},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "animejoy-allvideo", "sub")
	if err != nil || latest != 5 {
		t.Fatalf("animejoy-allvideo latest = %d, err = %v, want 5, nil", latest, err)
	}
}

func TestAnimeLevel_Animejoy_EmptyIsNotFound(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{},
		fakeAnimejoy{info: &domain.AnimejoyLegInfo{Episodes: []int{}}},
	)
	_, _, err := r.Latest(context.Background(), "57466", "animejoy-sibnet", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("empty animejoy-sibnet err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_Animejoy_NilListerIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{}, fakeRaw{}, nil)
	_, _, err := r.Latest(context.Background(), "57466", "animejoy-allvideo", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("nil animejoy lister err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_ScraperError_Propagates(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{err: errors.New("connection refused")}, fakeRaw{}, nil)
	_, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err == nil || isNotFoundLike(err) {
		t.Fatalf("scraper-down err = %v, want a non-NotFound (infra) error", err)
	}
}
