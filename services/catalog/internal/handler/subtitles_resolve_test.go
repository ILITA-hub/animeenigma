package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGetOpenSubtitlesFile_BadID(t *testing.T) {
	h := &SubtitlesHandler{} // aggregator unused on the bad-id path
	r := chi.NewRouter()
	r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", h.GetOpenSubtitlesFile)

	req := httptest.NewRequest(http.MethodGet, "/x/subtitles/opensubtitles/file/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
