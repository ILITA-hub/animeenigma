package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/sourceranking"
)

type fakeRankReader struct{ out sourceranking.Ranking }

func (f fakeRankReader) Read(context.Context, string) sourceranking.Ranking { return f.out }

func TestSourceRankingHandler_OK(t *testing.T) {
	h := NewSourceRankingHandler(fakeRankReader{out: sourceranking.Ranking{
		Global:   []sourceranking.Record{{Provider: "kodik", Score: 0.9}},
		PerAnime: []sourceranking.Record{},
	}}, nil)

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
