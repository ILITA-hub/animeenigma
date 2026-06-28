package transport

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMaxBodySizeMiddleware_ExemptsWorkerSegments verifies that the global body
// cap does NOT apply to the worker segment data-plane (/worker/segments/*), where
// uploads are multi-MB video segments, while it still caps every other path. A
// capped path truncates the body and the upstream handler 502s on the short read;
// the exemption is what lets segment uploads succeed end-to-end.
func TestMaxBodySizeMiddleware_ExemptsWorkerSegments(t *testing.T) {
	const capBytes = 10              // tiny cap so a 100-byte body trips it
	body := strings.Repeat("x", 100) // 100 bytes > capBytes

	var readN int
	var readErr error
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		readN, readErr = len(b), err
		w.WriteHeader(http.StatusOK)
	})
	mw := MaxBodySizeMiddleware(capBytes, "/worker/segments/")(h)

	t.Run("exempt worker-segment path reads the full body", func(t *testing.T) {
		readN, readErr = 0, nil
		req := httptest.NewRequest(http.MethodPut, "/worker/segments/job-1/0", bytes.NewReader([]byte(body)))
		mw.ServeHTTP(httptest.NewRecorder(), req)
		if readErr != nil {
			t.Fatalf("exempt path: body read errored (was capped): %v", readErr)
		}
		if readN != len(body) {
			t.Fatalf("exempt path: read %d bytes, want %d (body was truncated)", readN, len(body))
		}
	})

	t.Run("non-exempt path is still capped", func(t *testing.T) {
		readN, readErr = 0, nil
		req := httptest.NewRequest(http.MethodPost, "/api/anime/search", bytes.NewReader([]byte(body)))
		mw.ServeHTTP(httptest.NewRecorder(), req)
		if readErr == nil {
			t.Fatalf("non-exempt path: expected the body read to be capped, but read %d bytes with no error", readN)
		}
	})
}

// TestLargeTransferDeadlineMiddleware_PassesThrough verifies the deadline-extension
// middleware is transparent: it serves the request even when the ResponseWriter
// does not support ResponseController deadlines (httptest.Recorder returns
// ErrNotSupported, which the middleware must ignore rather than failing the route).
func TestLargeTransferDeadlineMiddleware_PassesThrough(t *testing.T) {
	called := false
	h := largeTransferDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/worker/segments/j/0", strings.NewReader("body")))
	if !called {
		t.Fatal("next handler not called")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}
}
