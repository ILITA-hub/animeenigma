package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRecomputer struct {
	calls int
	err   error
}

func (f *fakeRecomputer) Recompute(_ context.Context) error {
	f.calls++
	return f.err
}

func TestReadThresholdHandler(t *testing.T) {
	t.Run("success returns 204 and triggers one recompute", func(t *testing.T) {
		fr := &fakeRecomputer{}
		h := NewReadThresholdHandler(fr)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/internal/read-thresholds/recompute", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("want 204, got %d", rec.Code)
		}
		if fr.calls != 1 {
			t.Fatalf("want 1 recompute, got %d", fr.calls)
		}
	})

	t.Run("recompute error returns 500", func(t *testing.T) {
		fr := &fakeRecomputer{err: errors.New("clickhouse timeout")}
		h := NewReadThresholdHandler(fr)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/internal/read-thresholds/recompute", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("want 500, got %d", rec.Code)
		}
	})
}
