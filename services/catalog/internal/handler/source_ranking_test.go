package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/sourceranking"
)

type fakeRankReader struct{ out sourceranking.Ranking }

func (f fakeRankReader) Read(context.Context, string) sourceranking.Ranking { return f.out }

type fakeFixWriter struct {
	err      error
	animeID  string
	provider string
	calls    int
}

func (f *fakeFixWriter) SetFix(_ context.Context, animeID, provider string) error {
	f.animeID, f.provider = animeID, provider
	f.calls++
	return f.err
}

func TestSourceRankingHandler_OK(t *testing.T) {
	h := NewSourceRankingHandler(fakeRankReader{out: sourceranking.Ranking{
		Global:   []sourceranking.Record{{Provider: "kodik", Score: 0.9}},
		PerAnime: []sourceranking.Record{},
	}}, &fakeFixWriter{}, nil)

	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/source-ranking", h.Get)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/anime/uuid-1/source-ranking", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	// catalog wraps responses in the httputil {success,data} envelope.
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Global   []sourceranking.Record `json:"global"`
			PerAnime []sourceranking.Record `json:"perAnime"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if !body.Success {
		t.Errorf("success = false")
	}
	if len(body.Data.Global) != 1 || body.Data.Global[0].Provider != "kodik" {
		t.Errorf("global = %+v", body.Data.Global)
	}
}

func TestSourceFixHandler_OK(t *testing.T) {
	fw := &fakeFixWriter{}
	h := NewSourceRankingHandler(fakeRankReader{}, fw, nil)

	r := chi.NewRouter()
	r.Post("/api/anime/{animeId}/source-fix", h.Post)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/anime/uuid-1/source-fix", strings.NewReader(`{"provider":"allanime"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if fw.calls != 1 || fw.animeID != "uuid-1" || fw.provider != "allanime" {
		t.Errorf("writer called with (%q, %q) x%d", fw.animeID, fw.provider, fw.calls)
	}
}

func TestSourceFixHandler_BadProvider(t *testing.T) {
	fw := &fakeFixWriter{err: stderrors.New("unknown provider")}
	h := NewSourceRankingHandler(fakeRankReader{}, fw, nil)

	r := chi.NewRouter()
	r.Post("/api/anime/{animeId}/source-fix", h.Post)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/anime/uuid-1/source-fix", strings.NewReader(`{"provider":"bogus"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}
