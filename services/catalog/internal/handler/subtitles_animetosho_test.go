package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAnimeToshoFile_BadAttachIDIs400(t *testing.T) {
	h := &SubtitlesHandler{} // aggregator not needed: bad id rejected before use
	req := httptest.NewRequest(http.MethodGet, "/api/anime/x/subtitles/animetosho/file/abc", nil)
	// chi URL param not set → chi.URLParam returns "" → Atoi fails → 400.
	rec := httptest.NewRecorder()
	h.GetAnimeToshoFile(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
