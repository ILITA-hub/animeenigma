package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetKageFile_BadSrtIDIs400(t *testing.T) {
	h := &SubtitlesHandler{} // aggregator not needed: bad id rejected before use
	req := httptest.NewRequest(http.MethodGet, "/api/anime/x/subtitles/kage/file/abc", nil)
	// chi URL param not set → chi.URLParam returns "" → Atoi fails → 400.
	rec := httptest.NewRecorder()
	h.GetKageFile(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
