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

func (f fakeScraper) GetScraperEpisodes(_ context.Context, _, _ string) (int, []byte, error) {
	return f.status, f.body, f.err
}

type fakeRaw struct {
	lib  *EpisodesResponse
	raw  *EpisodesResponse
	err  error
}

func (f fakeRaw) GetLibraryEpisodes(_ context.Context, _ string) (*EpisodesResponse, error) {
	return f.lib, f.err
}
func (f fakeRaw) GetEpisodes(_ context.Context, _ string) (*EpisodesResponse, error) {
	return f.raw, f.err
}

func newResolver(fnd animeFinder, scr scraperEpisodeLister, raw rawEpisodeLister) *animeLevelResolver {
	return &animeLevelResolver{finder: fnd, scraper: scr, raw: raw}
}

func TestIsAnimeLevelPlayer(t *testing.T) {
	for _, p := range []string{"english", "ae", "raw"} {
		if !isAnimeLevelPlayer(p) {
			t.Errorf("isAnimeLevelPlayer(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"kodik", "animelib", "hanime", ""} {
		if isAnimeLevelPlayer(p) {
			t.Errorf("isAnimeLevelPlayer(%q) = true, want false", p)
		}
	}
}

func TestAnimeLevel_EnglishSub_MaxEpisode(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1},{"number":12},{"number":7}]}}`)},
		fakeRaw{},
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
	)
	_, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("english dub (no dub eps) err = %v; want NotFound-like", err)
	}
}

func TestAnimeLevel_EnglishSub_EmptyIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[]}}`)}, fakeRaw{})
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
	)
	latest, _, err := r.Latest(context.Background(), "57466", "ae", "sub")
	if err != nil || latest != 9 {
		t.Fatalf("ae latest = %d, err = %v, want 9, nil", latest, err)
	}
}

func TestAnimeLevel_Raw_MaxFromAllAnime(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{raw: &EpisodesResponse{Available: true, Episodes: []RawEpisode{{Number: 24}}}},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "raw", "sub")
	if err != nil || latest != 24 {
		t.Fatalf("raw latest = %d, err = %v, want 24, nil", latest, err)
	}
}

func TestAnimeLevel_Raw_UnavailableIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{}, fakeRaw{raw: &EpisodesResponse{Available: false}})
	_, _, err := r.Latest(context.Background(), "57466", "raw", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("unavailable raw err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_ScraperError_Propagates(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{err: errors.New("connection refused")}, fakeRaw{})
	_, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err == nil || isNotFoundLike(err) {
		t.Fatalf("scraper-down err = %v, want a non-NotFound (infra) error", err)
	}
}
