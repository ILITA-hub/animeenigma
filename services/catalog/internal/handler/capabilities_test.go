package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

type fakeCapSvc struct {
	rep domain.CapabilityReport
	err error
}

func (f fakeCapSvc) Report(_ context.Context, animeID string) (domain.CapabilityReport, error) {
	rep := f.rep
	rep.AnimeID = animeID
	return rep, f.err
}

func TestCapabilitiesHandler_OK(t *testing.T) {
	rep := domain.CapabilityReport{
		Families: []domain.SourceFamily{
			{Family: "ourenglish", Providers: []domain.ProviderCap{{Provider: "allanime"}}},
		},
	}
	h := handler.NewCapabilitiesHandler(fakeCapSvc{rep: rep}, nil)
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data domain.CapabilityReport `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AnimeID != "abc" {
		t.Errorf("expected AnimeID=abc, got %q", body.Data.AnimeID)
	}
	if len(body.Data.Families) != 1 {
		t.Errorf("expected 1 family, got %d", len(body.Data.Families))
	}
	if body.Data.Families[0].Family != "ourenglish" {
		t.Errorf("expected family=ourenglish, got %q", body.Data.Families[0].Family)
	}
	if len(body.Data.Families[0].Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(body.Data.Families[0].Providers))
	}
}

func TestCapabilitiesHandler_ServiceError(t *testing.T) {
	h := handler.NewCapabilitiesHandler(fakeCapSvc{err: context.DeadlineExceeded}, nil)
	r := chi.NewRouter()
	r.Get("/api/anime/{animeId}/capabilities", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/anime/abc/capabilities", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 on error, got 200 body=%s", rec.Body.String())
	}
}
