package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
)

// buildExternalHandlerWithStub builds an ExternalAPIHandler whose upstream is
// a stub httptest.Server. Returns the handler, the stub server (caller closes
// it), and a channel that receives the path+headers seen by the stub.
type stubCapture struct {
	path    string
	hasCookie bool
	hasSetCookie bool
	gatewayInternal string
}

func buildExternalHandlerWithStub(t *testing.T) (*handler.ExternalAPIHandler, *httptest.Server, chan stubCapture) {
	t.Helper()
	captured := make(chan stubCapture, 4)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- stubCapture{
			path:            r.URL.Path,
			hasCookie:       r.Header.Get("Cookie") != "",
			hasSetCookie:    r.Header.Get("Set-Cookie") != "",
			gatewayInternal: r.Header.Get("X-Gateway-Internal"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	h, err := handler.NewExternalAPIHandler(stub.URL, logger.Default())
	if err != nil {
		stub.Close()
		t.Fatalf("NewExternalAPIHandler: %v", err)
	}
	return h, stub, captured
}

// TestExternalAPIHandler_SegmentsForwardedToUpstream — /worker/segments/x with
// a valid key reaches the upstream stub.
func TestExternalAPIHandler_SegmentsForwardedToUpstream(t *testing.T) {
	h, stub, captured := buildExternalHandlerWithStub(t)
	defer stub.Close()

	req := httptest.NewRequest(http.MethodGet, "/worker/segments/abc123.ts", nil)
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case c := <-captured:
		if c.path != "/worker/segments/abc123.ts" {
			t.Errorf("stub received path %q; want /worker/segments/abc123.ts", c.path)
		}
	default:
		t.Fatal("stub never received the request")
	}
}

// TestExternalAPIHandler_NoCookieForwarded — Cookie header is stripped before
// forwarding to the upscaler. Workers authenticate via X-API-Key + session
// tokens, not browser cookies.
func TestExternalAPIHandler_NoCookieForwarded(t *testing.T) {
	h, stub, captured := buildExternalHandlerWithStub(t)
	defer stub.Close()

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	req.Header.Set("Cookie", "access_token=supersecret; refresh_token=also-secret")
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	select {
	case c := <-captured:
		if c.hasCookie {
			t.Error("Cookie header was forwarded to the upscaler — must be stripped")
		}
	default:
		t.Fatal("stub never received the request")
	}
}

// TestExternalAPIHandler_NoSetCookieForwarded — Set-Cookie is stripped from
// the inbound request (defence-in-depth).
func TestExternalAPIHandler_NoSetCookieForwarded(t *testing.T) {
	h, stub, captured := buildExternalHandlerWithStub(t)
	defer stub.Close()

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	req.Header.Set("Set-Cookie", "session=abc")
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	select {
	case c := <-captured:
		if c.hasSetCookie {
			t.Error("Set-Cookie header was forwarded to the upscaler — must be stripped")
		}
	default:
		t.Fatal("stub never received the request")
	}
}

// TestExternalAPIHandler_StripsGatewayInternal — a client-supplied
// X-Gateway-Internal on the internet-facing /worker/* edge must be stripped
// before forwarding (defence-in-depth): an external worker must never be able to
// spoof the gateway-internal admin marker (cheap-minor 3).
func TestExternalAPIHandler_StripsGatewayInternal(t *testing.T) {
	h, stub, captured := buildExternalHandlerWithStub(t)
	defer stub.Close()

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	req.Header.Set("X-Gateway-Internal", "1") // attacker tries to forge the marker
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	select {
	case c := <-captured:
		if c.gatewayInternal != "" {
			t.Errorf("X-Gateway-Internal = %q forwarded to upstream; must be stripped on the public edge", c.gatewayInternal)
		}
	default:
		t.Fatal("stub never received the request")
	}
}

// TestExternalAPIHandler_NoSetCookieInResponse — Set-Cookie from the upstream
// response is stripped before reaching the GPU worker.
func TestExternalAPIHandler_NoSetCookieInResponse(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "backend-session=secret")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer stub.Close()

	h, err := handler.NewExternalAPIHandler(stub.URL, logger.Default())
	if err != nil {
		t.Fatalf("NewExternalAPIHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/worker/enroll", nil)
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("Set-Cookie leaked in response: %q", rec.Header().Get("Set-Cookie"))
	}
}

// TestExternalAPIHandler_BadGatewayOnDialFailure — when the upstream is down
// the response must be a generic 502 with no internal topology detail.
func TestExternalAPIHandler_BadGatewayOnDialFailure(t *testing.T) {
	// Use a stub that's already closed so dial fails.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	stubURL := stub.URL
	stub.Close() // close immediately

	h, err := handler.NewExternalAPIHandler(stubURL, logger.Default())
	if err != nil {
		t.Fatalf("NewExternalAPIHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/worker/segments/x.ts", nil)
	rec := httptest.NewRecorder()
	h.ProxyWorker(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("dial failure: status = %d; want 502", rec.Code)
	}
	body := rec.Body.String()
	// Generic body — no internal host/path/bucket/infohash detail.
	for _, leak := range []string{"upscaler", "/data/torrents", "raw-library", "bucket", "dial"} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Errorf("response body leaks internal detail %q: %s", leak, body)
		}
	}
}
