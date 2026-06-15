package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/service"
)

type fakeDaily struct {
	guessOut *service.GuessOutcome
}

func (f *fakeDaily) GetOrCreateToday(_ context.Context) (*domain.DailyPuzzle, error) {
	return &domain.DailyPuzzle{Date: "2026-06-15"}, nil
}
func (f *fakeDaily) Guess(_ context.Context, _, _ string) (*service.GuessOutcome, error) {
	return f.guessOut, nil
}
func (f *fakeDaily) GiveUp(_ context.Context, _ string) (*service.VisibleAnime, error) {
	return &service.VisibleAnime{ID: "frieren"}, nil
}
func (f *fakeDaily) Resume(_ context.Context, _ string) (*service.DailyState, error) {
	return &service.DailyState{Date: "2026-06-15", Guesses: []service.GuessOutcome{}}, nil
}

type fakeSearch struct{}

func (fakeSearch) Search(_ context.Context, q string, _ int) []domain.PoolAnime {
	if strings.HasPrefix(q, "fr") {
		return []domain.PoolAnime{{ID: "frieren", NameRU: "Фрирен"}}
	}
	return nil
}

func TestHandler_DailyGuess(t *testing.T) {
	h := NewAnidleHandler(&fakeDaily{guessOut: &service.GuessOutcome{Solved: true, Attempt: 2}}, nil, nil, nil, fakeSearch{})
	body := strings.NewReader(`{"anime_id":"frieren"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/anidle/daily/guess", body)
	rec := httptest.NewRecorder()
	h.DailyGuess(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool                 `json:"success"`
		Data    service.GuessOutcome `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.Data.Solved)
}

func TestHandler_Search(t *testing.T) {
	h := NewAnidleHandler(&fakeDaily{}, nil, nil, nil, fakeSearch{})
	req := httptest.NewRequest(http.MethodGet, "/api/anidle/search?q=fr", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "frieren")
}
