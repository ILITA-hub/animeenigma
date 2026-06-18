package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakeLookup struct{}

func (fakeLookup) LatestAvailable(_ context.Context, _, _, _, _, _ string) (service.EpisodesLookupResult, error) {
	return service.EpisodesLookupResult{LatestAvailableEpisode: 12}, nil
}

func doReq(t *testing.T, h *InternalEpisodesHandler, target string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shikimoriId", "57466")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.GetLatestEpisode(rec, req)
	return rec.Code
}

func TestInternalEpisodes_AnimeLevelPlayersNoTranslationID(t *testing.T) {
	h := NewInternalEpisodesHandler(fakeLookup{}, nil)
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=english&watch_type=sub&language=en"); c != 200 {
		t.Errorf("english sub no-id = %d, want 200", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=ae"); c != 200 {
		t.Errorf("ae no-id = %d, want 200", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=kodik"); c != 200 {
		t.Errorf("kodik no-id = %d, want 200 (Phase 3 any-team)", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=animelib"); c != 200 {
		t.Errorf("animelib no-id = %d, want 200 (Phase 3 any-team)", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=hanime&translation_id=x"); c != 400 {
		t.Errorf("hanime = %d, want 400", c)
	}
}
