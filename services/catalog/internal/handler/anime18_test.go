package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/go-chi/chi/v5"
)

// fakeAnime18Service implements anime18ServiceAPI for handler tests.
type fakeAnime18Service struct {
	episodes   []domain.Anime18Episode
	episodesErr error
	stream     *domain.Anime18Stream
	streamErr  error

	gotAnimeID string
	gotEp      string
}

func (f *fakeAnime18Service) Get18AnimeEpisodes(ctx context.Context, animeID string) ([]domain.Anime18Episode, error) {
	f.gotAnimeID = animeID
	return f.episodes, f.episodesErr
}

func (f *fakeAnime18Service) Get18AnimeStream(ctx context.Context, animeID, episodeSlug string) (*domain.Anime18Stream, error) {
	f.gotAnimeID = animeID
	f.gotEp = episodeSlug
	return f.stream, f.streamErr
}

func newAnime18Handler(svc anime18ServiceAPI) *Anime18EndpointsHandler {
	h := &Anime18EndpointsHandler{}
	WireAnime18Endpoints(h, svc, nil)
	return h
}

// serveWithAnimeID routes a request through chi so chi.URLParam("animeId") resolves.
func serveWithAnimeID(h http.HandlerFunc, method, target string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Method(method, "/{animeId}/x", h)
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestGetAnime18Episodes_OK(t *testing.T) {
	svc := &fakeAnime18Service{episodes: []domain.Anime18Episode{
		{Slug: "1164-foo-episode-1", URL: "https://18anime.me/hentai/1164-foo-episode-1.html", Number: 1},
		{Slug: "1165-foo-episode-2", URL: "https://18anime.me/hentai/1165-foo-episode-2.html", Number: 2},
	}}
	h := newAnime18Handler(svc)
	rec := serveWithAnimeID(h.GetAnime18Episodes, http.MethodGet, "/uuid-123/x")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if svc.gotAnimeID != "uuid-123" {
		t.Errorf("service got animeID %q, want uuid-123", svc.gotAnimeID)
	}
	var resp struct {
		Success bool                   `json:"success"`
		Data    []domain.Anime18Episode `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Data) != 2 || resp.Data[0].Number != 1 {
		t.Fatalf("unexpected episodes: %+v", resp.Data)
	}
}

func TestGetAnime18Stream_OK(t *testing.T) {
	svc := &fakeAnime18Service{stream: &domain.Anime18Stream{
		URL: "https://a4.mp4upload.com:183/d/tok/video.mp4", Referer: "https://www.mp4upload.com/", IsHLS: false, Quality: "FullHD",
	}}
	h := newAnime18Handler(svc)
	rec := serveWithAnimeID(h.GetAnime18Stream, http.MethodGet, "/uuid-9/x?ep=1167-foo-episode-2")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if svc.gotEp != "1167-foo-episode-2" {
		t.Errorf("service got ep %q, want 1167-foo-episode-2", svc.gotEp)
	}
	var resp struct {
		Data domain.Anime18Stream `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.URL == "" || resp.Data.Referer != "https://www.mp4upload.com/" {
		t.Fatalf("stream not passed through: %+v", resp.Data)
	}
}

func TestGetAnime18Stream_MissingEp(t *testing.T) {
	h := newAnime18Handler(&fakeAnime18Service{})
	rec := serveWithAnimeID(h.GetAnime18Stream, http.MethodGet, "/uuid-9/x")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// The core resilience contract: a fully-unavailable source must surface an
// explicit 503, never a silent empty-200 (spec §7).
func TestGetAnime18Stream_UnavailableIs503NotEmpty200(t *testing.T) {
	svc := &fakeAnime18Service{streamErr: liberrors.ServiceUnavailable("all mirrors failed")}
	h := newAnime18Handler(svc)
	rec := serveWithAnimeID(h.GetAnime18Stream, http.MethodGet, "/uuid-9/x?ep=1167-foo-episode-2")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}
