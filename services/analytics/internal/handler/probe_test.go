package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRunner struct{ err error }

func (f fakeRunner) RunOnce(_ context.Context) error { return f.err }

func TestProbeHandler_OK(t *testing.T) {
	h := NewProbeHandler(fakeRunner{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/probe/run", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rr.Code)
	}
}

func TestProbeHandler_Err(t *testing.T) {
	h := NewProbeHandler(fakeRunner{err: errors.New("x")})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/probe/run", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}
