package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fakeRunner struct{ runs int32 }

func (f *fakeRunner) RunOnce(ctx context.Context) { atomic.AddInt32(&f.runs, 1) }

func TestInternalSubtitleProbe_Run204(t *testing.T) {
	r := &fakeRunner{}
	h := NewInternalSubtitleProbeHandler(r, nil)
	rec := httptest.NewRecorder()
	h.Run(rec, httptest.NewRequest(http.MethodPost, "/internal/subtitle-probe/run", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", rec.Code)
	}
	if atomic.LoadInt32(&r.runs) != 1 {
		t.Fatalf("RunOnce called %d times; want 1", r.runs)
	}
}
