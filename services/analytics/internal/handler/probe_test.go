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

// ctxErrRunner returns the context's error — so if the handler passed a
// cancelled context, RunOnce would fail.
type ctxErrRunner struct{}

func (ctxErrRunner) RunOnce(ctx context.Context) error { return ctx.Err() }

// TestProbeHandler_DetachesFromRequestContext is the regression test for the
// deploy bug: the multi-minute sweep must NOT inherit the request context, or a
// client disconnect aborts the final ClickHouse write. An already-cancelled
// request context must still produce a successful (204) run.
func TestProbeHandler_DetachesFromRequestContext(t *testing.T) {
	h := NewProbeHandler(ctxErrRunner{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // client already disconnected
	req := httptest.NewRequest(http.MethodPost, "/internal/probe/run", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("handler must detach from a cancelled request ctx; want 204, got %d", rr.Code)
	}
}
