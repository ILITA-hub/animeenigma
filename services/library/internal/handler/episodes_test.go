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

	recentRet      []domain.Episode
	recentErr      error
	gotRecentLimit int

	gotShikimoriID   string
	gotEpisodeNumber int
	gotStorage       string
	gotListShikimori string
}

func (s *stubEpisodeReader) ListRecentDistinct(_ context.Context, limit int) ([]domain.Episode, error) {
	s.gotRecentLimit = limit
	return s.recentRet, s.recentErr
}

func (s *stubEpisodeReader) GetByShikimoriEpisode(_ context.Context, id string, n int, storage string) (*domain.Episode, error) {
	s.gotShikimoriID = id
	s.gotEpisodeNumber = n
	s.gotStorage = storage
	return s.ret, s.err
}

func (s *stubEpisodeReader) List(_ context.Context, id string) ([]domain.Episode, error) {
	s.gotListShikimori = id
	return s.listRet, s.listErr
}

type stubURL struct {
	last string
}

func (s *stubURL) URLFor(_ context.Context, _ string, path string) (string, error) {
	s.last = path
	return "http://stub.example/" + path, nil
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

// TestEpisodes_Get_StoragePreference is the storage-service Task-4 anchor: the
// GET threads an explicit ?storage= to the repo (backend pin) and defaults to ""
// (repo minio-preference) when absent, and the resolved backend surfaces in the
// "storage" field of the response.
func TestEpisodes_Get_StoragePreference(t *testing.T) {
	// Explicit ?storage=s3 → passed through; response carries the row's storage.
	repo := &stubEpisodeReader{ret: &domain.Episode{
		ShikimoriID: "12345", EpisodeNumber: 3, MinioPath: "aeProvider/12345/RAW/3/", Storage: "s3",
	}}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/episodes/12345/3?storage=s3", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shikimori_id", "12345")
	rctx.URLParams.Add("episode", "3")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if repo.gotStorage != "s3" {
		t.Errorf("repo storage arg = %q, want s3 (from ?storage=)", repo.gotStorage)
	}
	var parsed struct {
		Data struct {
			Storage string `json:"storage"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Data.Storage != "s3" {
		t.Errorf("response storage = %q, want s3", parsed.Data.Storage)
	}

	// No ?storage= → default "" (repo applies minio-preference).
	repo2 := &stubEpisodeReader{ret: &domain.Episode{
		ShikimoriID: "12345", EpisodeNumber: 3, MinioPath: "aeProvider/12345/RAW/3/", Storage: "minio",
	}}
	h2 := NewEpisodesHandler(repo2, &stubURL{}, nil)
	r2, w2 := newReq(t, "12345", "3")
	h2.Get(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}
	if repo2.gotStorage != "" {
		t.Errorf("repo storage arg = %q, want empty (no ?storage=)", repo2.gotStorage)
	}
}

func TestEpisodes_Get_HasStoryboard_True(t *testing.T) {
	repo := &stubEpisodeReader{ret: &domain.Episode{
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		MinioPath:     "12345/3/",
		HasStoryboard: true,
	}}
	url := &stubURL{}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newReq(t, "12345", "3")
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data field missing or wrong shape: %v", raw)
	}
	minioURL, _ := data["minio_url"].(string)
	storyboardURL, ok := data["storyboard_url"].(string)
	if !ok {
		t.Fatalf("storyboard_url missing when HasStoryboard=true; body=%s", w.Body.String())
	}
	wantStoryboard := strings.Replace(minioURL, "playlist.m3u8", "storyboard.vtt", 1)
	if storyboardURL != wantStoryboard {
		t.Errorf("storyboard_url = %q, want %q (derived from minio_url %q)", storyboardURL, wantStoryboard, minioURL)
	}
	if storyboardURL != "http://stub.example/12345/3/storyboard.vtt" {
		t.Errorf("storyboard_url = %q, want %q", storyboardURL, "http://stub.example/12345/3/storyboard.vtt")
	}
}

func TestEpisodes_Get_HasStoryboard_False_KeyAbsent(t *testing.T) {
	repo := &stubEpisodeReader{ret: &domain.Episode{
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		MinioPath:     "12345/3/",
		HasStoryboard: false,
	}}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r, w := newReq(t, "12345", "3")
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data field missing or wrong shape: %v", raw)
	}
	if _, present := data["storyboard_url"]; present {
		t.Errorf("storyboard_url present when HasStoryboard=false; body=%s", w.Body.String())
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

// erroringURLBuilder lets tests script URLFor failures per-call — either for
// every call (allErr) or for specific paths (errPaths) — to exercise the
// per-row-skip vs total-outage branches of EpisodesHandler.List.
type erroringURLBuilder struct {
	allErr   bool
	errPaths map[string]bool
}

func (s *erroringURLBuilder) URLFor(_ context.Context, _ string, path string) (string, error) {
	if s.allErr || s.errPaths[path] {
		return "", errors.New("urlFor failed")
	}
	return "http://stub.example/" + path, nil
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

func TestEpisodes_List_HasStoryboard(t *testing.T) {
	repo := &stubEpisodeReader{listRet: []domain.Episode{
		{ShikimoriID: "54974", EpisodeNumber: 1, MinioPath: "54974/1/", HasStoryboard: true},
		{ShikimoriID: "54974", EpisodeNumber: 2, MinioPath: "54974/2/", HasStoryboard: false},
	}}
	url := &stubURL{}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	data, ok := raw["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data field missing or wrong shape: %v", raw)
	}
	episodes, ok := data["episodes"].([]interface{})
	if !ok || len(episodes) != 2 {
		t.Fatalf("episodes = %v, want 2 entries", data["episodes"])
	}
	ep0, ok := episodes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("ep0 wrong shape: %v", episodes[0])
	}
	minioURL, _ := ep0["minio_url"].(string)
	storyboardURL, present := ep0["storyboard_url"].(string)
	if !present {
		t.Fatalf("ep0 storyboard_url missing when HasStoryboard=true; body=%s", w.Body.String())
	}
	wantStoryboard := strings.Replace(minioURL, "playlist.m3u8", "storyboard.vtt", 1)
	if storyboardURL != wantStoryboard {
		t.Errorf("ep0 storyboard_url = %q, want %q (derived from minio_url %q)", storyboardURL, wantStoryboard, minioURL)
	}
	ep1, ok := episodes[1].(map[string]interface{})
	if !ok {
		t.Fatalf("ep1 wrong shape: %v", episodes[1])
	}
	if _, present := ep1["storyboard_url"]; present {
		t.Errorf("ep1 storyboard_url present when HasStoryboard=false; body=%s", w.Body.String())
	}
}

func TestEpisodes_RecentEpisodes_HappyPath(t *testing.T) {
	repo := &stubEpisodeReader{recentRet: []domain.Episode{
		{ShikimoriID: "100", EpisodeNumber: 28},
		{ShikimoriID: "200", EpisodeNumber: 12},
	}}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/internal/library/recent-episodes?limit=3", nil)
	w := httptest.NewRecorder()
	h.RecentEpisodes(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if repo.gotRecentLimit != 3 {
		t.Errorf("limit passed = %d, want 3", repo.gotRecentLimit)
	}
	var parsed struct {
		Data struct {
			Episodes []struct {
				ShikimoriID   string `json:"shikimori_id"`
				EpisodeNumber int    `json:"episode_number"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(parsed.Data.Episodes) != 2 ||
		parsed.Data.Episodes[0].ShikimoriID != "100" || parsed.Data.Episodes[0].EpisodeNumber != 28 {
		t.Fatalf("episodes = %+v", parsed.Data.Episodes)
	}
}

func TestEpisodes_RecentEpisodes_DefaultLimit(t *testing.T) {
	repo := &stubEpisodeReader{recentRet: nil}
	h := NewEpisodesHandler(repo, &stubURL{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/internal/library/recent-episodes", nil)
	w := httptest.NewRecorder()
	h.RecentEpisodes(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if repo.gotRecentLimit != 3 {
		t.Errorf("default limit = %d, want 3", repo.gotRecentLimit)
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

// TestEpisodes_List_AllURLForFailuresSurface5xx — the repo returned rows, but
// EVERY row's URLFor failed (storage service down / BaseURLs unavailable).
// This must NOT be the ordinary empty-array 200 (catalog caches that 10min as
// "ae has no content"): it must surface as a 5xx.
func TestEpisodes_List_AllURLForFailuresSurface5xx(t *testing.T) {
	repo := &stubEpisodeReader{listRet: []domain.Episode{
		{ShikimoriID: "54974", EpisodeNumber: 1, MinioPath: "54974/1/"},
		{ShikimoriID: "54974", EpisodeNumber: 2, MinioPath: "54974/2/"},
	}}
	url := &erroringURLBuilder{allErr: true}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code < 500 {
		t.Fatalf("status = %d, want 5xx when every row's URLFor fails; body=%s", w.Code, w.Body.String())
	}
}

// TestEpisodes_List_PartialURLForFailureReturns200WithRemaining — only one of
// two rows fails URLFor: single-row failures still just skip that row, so the
// response stays 200 with the surviving item.
func TestEpisodes_List_PartialURLForFailureReturns200WithRemaining(t *testing.T) {
	repo := &stubEpisodeReader{listRet: []domain.Episode{
		{ShikimoriID: "54974", EpisodeNumber: 1, MinioPath: "54974/1/"},
		{ShikimoriID: "54974", EpisodeNumber: 2, MinioPath: "54974/2/"},
	}}
	url := &erroringURLBuilder{errPaths: map[string]bool{"54974/1/playlist.m3u8": true}}
	h := NewEpisodesHandler(repo, url, nil)
	r, w := newListReq(t, "54974")
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when only one row's URLFor fails; body=%s", w.Code, w.Body.String())
	}
	var parsed struct {
		Data struct {
			Episodes []struct {
				EpisodeNumber int `json:"episode_number"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(parsed.Data.Episodes) != 1 || parsed.Data.Episodes[0].EpisodeNumber != 2 {
		t.Fatalf("episodes = %+v, want exactly episode 2", parsed.Data.Episodes)
	}
}
