package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
)

type fakeRecentLister struct {
	ret      []library.RecentEpisode
	err      error
	gotLimit int
}

func (f *fakeRecentLister) RecentEpisodes(_ context.Context, limit int) ([]library.RecentEpisode, error) {
	f.gotLimit = limit
	return f.ret, f.err
}

type fakeAnimeByShikimori struct {
	byID map[string]*domain.Anime
	err  error
}

func (f *fakeAnimeByShikimori) GetByShikimoriID(_ context.Context, id string) (*domain.Anime, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byID[id], nil // (nil, nil) when absent — mirrors the real repo
}

func TestAeTargets_MapsAndSkipsUnmapped(t *testing.T) {
	lib := &fakeRecentLister{ret: []library.RecentEpisode{
		{ShikimoriID: "100", EpisodeNumber: 28},
		{ShikimoriID: "999", EpisodeNumber: 5}, // not in catalog → skipped
	}}
	repo := &fakeAnimeByShikimori{byID: map[string]*domain.Anime{
		"100": {ID: "uuid-100", Name: "Sousou no Frieren", NameRU: "Фрирен"},
	}}
	h := NewInternalProbeHandler(lib, repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/internal/probe/ae-targets?limit=3", nil)
	w := httptest.NewRecorder()
	h.AeTargets(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if lib.gotLimit != 3 {
		t.Errorf("limit passed to library = %d, want 3", lib.gotLimit)
	}
	var parsed struct {
		Data struct {
			Targets []AeTarget `json:"targets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(parsed.Data.Targets) != 1 {
		t.Fatalf("targets len = %d, want 1 (unmapped skipped); got %+v", len(parsed.Data.Targets), parsed.Data.Targets)
	}
	got := parsed.Data.Targets[0]
	if got.UUID != "uuid-100" || got.Episode != 28 || got.Name != "Фрирен" {
		t.Fatalf("target = %+v, want {uuid-100, Фрирен, 28}", got)
	}
}

func TestAeTargets_NamePreference_ENWhenNoRU(t *testing.T) {
	lib := &fakeRecentLister{ret: []library.RecentEpisode{{ShikimoriID: "1", EpisodeNumber: 1}}}
	repo := &fakeAnimeByShikimori{byID: map[string]*domain.Anime{
		"1": {ID: "u1", Name: "Romaji Title", NameEN: "English Title"},
	}}
	h := NewInternalProbeHandler(lib, repo, nil)
	r := httptest.NewRequest(http.MethodGet, "/internal/probe/ae-targets", nil)
	w := httptest.NewRecorder()
	h.AeTargets(w, r)

	var parsed struct {
		Data struct {
			Targets []AeTarget `json:"targets"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	if len(parsed.Data.Targets) != 1 || parsed.Data.Targets[0].Name != "English Title" {
		t.Fatalf("name = %+v, want English Title (RU absent)", parsed.Data.Targets)
	}
	if lib.gotLimit != 3 {
		t.Errorf("default limit = %d, want 3", lib.gotLimit)
	}
}

func TestAeTargets_LibraryError_500(t *testing.T) {
	lib := &fakeRecentLister{err: errors.New("library down")}
	h := NewInternalProbeHandler(lib, &fakeAnimeByShikimori{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/internal/probe/ae-targets", nil)
	w := httptest.NewRecorder()
	h.AeTargets(w, r)
	if w.Code < 500 {
		t.Fatalf("status = %d, want 5xx on library error", w.Code)
	}
}
