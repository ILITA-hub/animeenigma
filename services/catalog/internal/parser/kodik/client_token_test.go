package kodik

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// closeTrackingBody wraps an io.ReadCloser and records whether Close was called.
type closeTrackingBody struct {
	io.Reader
	mu     sync.Mutex
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}

func (b *closeTrackingBody) wasClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// trackingRoundTripper returns a 500 response whose body is a closeTrackingBody
// for the primary token URL, and a 404 (no token) for the fallback. It records
// the primary body so the test can assert it was closed by getToken.
type trackingRoundTripper struct {
	primaryURL  string
	fallbackURL string
	primaryBody *closeTrackingBody
}

func (rt *trackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.String() {
	case rt.primaryURL:
		body := &closeTrackingBody{Reader: strings.NewReader("boom: kodik token source unavailable")}
		rt.primaryBody = body
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       body,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	default:
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
}

// TestGetToken_NonOKBodyClosed asserts that when the primary token source
// returns a non-200 status, getToken still closes the response body before
// falling through to the fallback source. Regression guard for the leak where
// resp.Body.Close() lived only inside the StatusCode==200 branch, so a non-200
// (err==nil) response leaked the connection out of the keep-alive pool.
func TestGetToken_NonOKBodyClosed(t *testing.T) {
	const primaryURL = "https://primary.token.test/online_mod.js"
	const fallbackURL = "https://fallback.token.test/add-players.min.js"

	rt := &trackingRoundTripper{primaryURL: primaryURL, fallbackURL: fallbackURL}
	c := &Client{
		httpClient:       &http.Client{Transport: rt, Timeout: 5 * time.Second},
		tokenSourceURL:   primaryURL,
		fallbackTokenURL: fallbackURL,
	}

	// getToken returns an error here (no valid token from either source), which
	// is fine — we only assert the primary body was closed.
	_, _ = c.getToken()

	if rt.primaryBody == nil {
		t.Fatal("primary token source was never requested")
	}
	if !rt.primaryBody.wasClosed() {
		t.Fatal("getToken leaked the non-200 primary response body (Close never called)")
	}
}
