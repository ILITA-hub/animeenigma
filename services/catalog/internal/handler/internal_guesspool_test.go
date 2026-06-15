package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakePoolBuilder struct {
	entries []service.GuessPoolEntry
}

func (f *fakePoolBuilder) BuildPool(_ context.Context) ([]service.GuessPoolEntry, error) {
	return f.entries, nil
}

func TestInternalGuessPool_GetPool(t *testing.T) {
	builder := &fakePoolBuilder{entries: []service.GuessPoolEntry{
		{ID: "frieren", NameRU: "Фрирен", Score: 9.3},
	}}
	h := NewInternalGuessPoolHandler(builder, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/guessgame/pool", nil)
	h.GetPool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Success bool                   `json:"success"`
		Data    []service.GuessPoolEntry `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success || len(resp.Data) != 1 || resp.Data[0].ID != "frieren" {
		t.Fatalf("unexpected body: %+v", resp)
	}
}
