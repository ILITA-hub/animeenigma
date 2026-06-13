package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/go-chi/chi/v5"
)

type stubEpisodeReader struct {
	ret *domain.Episode
	err error

	listRet []domain.Episode
	listErr error

	gotShikimoriID   string
	gotEpisodeNumber int
	gotListShikimori string
}

func (s *stubEpisodeReader) GetByShikimoriEpisode(_ context.Context, id string, n int) (*domain.Episode, error) {
	s.gotShikimoriID = id
	s.gotEpisodeNumber = n
	return s.ret, s.err
}

func (s *stubEpisodeReader) List(_ context.Context, id string) ([]domain.Episode, error) {
	s.gotListShikimori = id
	return s.listRet, s.listErr
}

type stubURL struct {
	last string
}

func (s *stubURL) URLFor(path string) string {
	s.last = path
	return "http://stub.example/" + path
}

func newReq(t *testing.T, shikimoriID, episode string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/episodes/"+shikimoriID+"/"+episode, nil)
	// Inject chi URLParams.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shikimori_id", shikimoriID)
	rctx.URLParams.Add("episode", episode)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r, httptest.NewRecorder()
}

func TestEpisodes_Get_HappyPath(t *testing.T) {
	dur := 1450
	size := int64(123456)
	jobID := "abc-def"
	repo := &stubEpisodeReader{ret: &domain.Episode{
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		JobID:         &jobID,
		MinioPath:     "12345/3/",
		DurationSec:   &dur,
		SizeBytes:     &size,
	}}
	url := &stubURL{}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newReq(t, "12345", "3")
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if url.last != "12345/3/playlist.m3u8" {
		t.Errorf("URLFor called with %q, want %q", url.last, "12345/3/playlist.m3u8")
	}
	var parsed struct {
		Data struct {
			MinioURL    string `json:"minio_url"`
			DurationSec int    `json:"duration_sec"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if !strings.HasSuffix(parsed.Data.MinioURL, "/12345/3/playlist.m3u8") {
		t.Errorf("MinioURL = %q, want suffix /12345/3/playlist.m3u8", parsed.Data.MinioURL)
	}
	if parsed.Data.DurationSec != 1450 {
		t.Errorf("DurationSec = %d, want 1450", parsed.Data.DurationSec)
	}
	if parsed.Data.SizeBytes != 123456 {
		t.Errorf("SizeBytes = %d, want 123456", parsed.Data.SizeBytes)
	}
}

func TestEpisodes_Get_NotFound(t *testing.T) {
	repo := &stubEpisodeReader{err: liberrors.NotFound("episode")}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "999")
	h.Get(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestEpisodes_Get_BadEpisodeNonNumeric(t *testing.T) {
	repo := &stubEpisodeReader{}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "notanumber")
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestEpisodes_Get_BadEpisodeZero(t *testing.T) {
	repo := &stubEpisodeReader{}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "0")
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestEpisodes_Get_BadEpisodeNegative(t *testing.T) {
	repo := &stubEpisodeReader{}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "-1")
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestEpisodes_Get_MissingShikimoriID(t *testing.T) {
	repo := &stubEpisodeReader{}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "", "1")
	h.Get(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when shikimori_id missing", w.Code)
	}
}

func TestEpisodes_Get_InternalError(t *testing.T) {
	repo := &stubEpisodeReader{err: errors.New("simulated DB failure")}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "1")
	h.Get(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func newListReq(t *testing.T, shikimoriID string) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/episodes/"+shikimoriID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shikimori_id", shikimoriID)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r, httptest.NewRecorder()
}

func TestEpisodes_List_HappyPath(t *testing.T) {
	dur := 1450
	repo := &stubEpisodeReader{listRet: []domain.Episode{
		{ShikimoriID: "54974", EpisodeNumber: 1, MinioPath: "54974/1/", DurationSec: &dur},
		{ShikimoriID: "54974", EpisodeNumber: 2, MinioPath: "54974/2/"},
	}}
	url := &stubURL{}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if repo.gotListShikimori != "54974" {
		t.Errorf("List called with %q, want 54974", repo.gotListShikimori)
	}
	var parsed struct {
		Data struct {
			Episodes []struct {
				EpisodeNumber int    `json:"episode_number"`
				MinioURL      string `json:"minio_url"`
				DurationSec   int    `json:"duration_sec"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(parsed.Data.Episodes) != 2 {
		t.Fatalf("episodes len = %d, want 2", len(parsed.Data.Episodes))
	}
	if parsed.Data.Episodes[0].EpisodeNumber != 1 ||
		parsed.Data.Episodes[0].MinioURL != "http://stub.example/54974/1/playlist.m3u8" {
		t.Errorf("ep0 = %+v", parsed.Data.Episodes[0])
	}
	if parsed.Data.Episodes[0].DurationSec != 1450 {
		t.Errorf("ep0 duration = %d, want 1450", parsed.Data.Episodes[0].DurationSec)
	}
}

func TestEpisodes_List_EmptyIsNotFoundFree(t *testing.T) {
	// Nothing encoded yet → 200 with an empty array, NOT 404.
	repo := &stubEpisodeReader{listRet: nil}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for empty list", w.Code)
	}
	if !strings.Contains(w.Body.String(), "\"episodes\":[]") {
		t.Errorf("body = %s, want empty episodes array", w.Body.String())
	}
}

func TestEpisodes_List_MissingShikimoriID(t *testing.T) {
	repo := &stubEpisodeReader{}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newListReq(t, "")
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when shikimori_id missing", w.Code)
	}
}

func TestEpisodes_List_InternalError(t *testing.T) {
	repo := &stubEpisodeReader{listErr: errors.New("simulated DB failure")}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
