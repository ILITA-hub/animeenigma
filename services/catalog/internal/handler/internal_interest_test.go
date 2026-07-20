package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type stubInterest struct {
	bands                                            repo.InterestBands
	err                                              error
	gotOngoing, gotTop, gotIdleWindow, gotIdleOffset int
}

func (s *stubInterest) ListInterestBands(_ context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (repo.InterestBands, error) {
	s.gotOngoing, s.gotTop, s.gotIdleWindow, s.gotIdleOffset = ongoingLimit, topLimit, idleWindow, idleOffset
	return s.bands, s.err
}

func TestInterestBands(t *testing.T) {
	s := &stubInterest{bands: repo.InterestBands{
		Ongoing:    []repo.InterestRow{{ID: "o1", EpisodesAired: 12}},
		Top:        []repo.InterestRow{{ID: "t1", TopRank: 1}},
		Planned:    []repo.InterestRow{{ID: "p1", Planners: 3}},
		IdleWindow: []repo.InterestRow{{ID: "i1", TopRank: 101}},
		IdleTotal:  4824,
	}}
	h := NewInternalInterestHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Bands(rec, httptest.NewRequest("GET", "/internal/interest/bands?idle_offset=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if s.gotIdleOffset != 100 {
		t.Fatalf("idle_offset not threaded: %d", s.gotIdleOffset)
	}
	var env struct {
		Data repo.InterestBands `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.IdleTotal != 4824 || len(env.Data.Planned) != 1 || env.Data.IdleWindow[0].TopRank != 101 {
		t.Fatalf("body: %s", rec.Body.String())
	}
}
