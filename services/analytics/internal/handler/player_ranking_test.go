package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRankRecomputer struct{ err error }

func (f fakeRankRecomputer) Recompute(context.Context) error { return f.err }

func TestPlayerRankingRecompute_OK(t *testing.T) {
	h := NewPlayerRankingHandler(fakeRankRecomputer{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/player-ranking/recompute", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}

func TestPlayerRankingRecompute_Err(t *testing.T) {
	h := NewPlayerRankingHandler(fakeRankRecomputer{err: errors.New("boom")})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/player-ranking/recompute", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}
