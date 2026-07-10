package videoutils

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusServer is a tiny httptest upstream that always answers `code` with
// `body`, registered for cleanup. Used to build real *http.Response values (with
// live bodies over keep-alive conns) for the edge-rotation unit tests.
func statusServer(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

// TestMaybeRotateSolodcdnEdge_SuccessOnSibling: a 500 from the p12 edge rotates
// to the first healthy sibling (p13) and serves it — the AUTO-562 happy path.
func TestMaybeRotateSolodcdnEdge_SuccessOnSibling(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	good := statusServer(t, http.StatusOK, "#EXTM3U\n")

	var hooks [][3]string
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  []string{"p12", "p13", "p14"},
		OnEdgeRotation: func(from, to, outcome string) { hooks = append(hooks, [3]string{from, to, outcome}) },
	})

	first, err := http.Get(bad.URL + "/seg-1.ts") // the failed p12 response
	require.NoError(t, err)

	var calls []string
	out := p.maybeRotateSolodcdnEdge(first, "https://p12.solodcdn.com/seg-1.ts", func(siblingURL string) (*http.Response, error) {
		calls = append(calls, siblingURL)
		return http.Get(good.URL + "/seg-1.ts")
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusOK, out.StatusCode)
	assert.Equal(t, []string{"https://p13.solodcdn.com/seg-1.ts"}, calls, "stops at the first healthy sibling; never tries p14")
	assert.Equal(t, [][3]string{{"p12", "p13", "success"}}, hooks)
}

// TestMaybeRotateSolodcdnEdge_Never4xxRetry: a 4xx first response must NOT
// trigger any rotation (only >=500 enters the loop).
func TestMaybeRotateSolodcdnEdge_Never4xxRetry(t *testing.T) {
	notfound := statusServer(t, http.StatusNotFound, "nope")
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  []string{"p12", "p13", "p14"},
		OnEdgeRotation: func(_, _, _ string) { t.Fatal("hook must not fire for a 4xx first response") },
	})

	first, err := http.Get(notfound.URL + "/x.ts")
	require.NoError(t, err)

	called := false
	out := p.maybeRotateSolodcdnEdge(first, "https://p12.solodcdn.com/x.ts", func(string) (*http.Response, error) {
		called = true
		return nil, nil
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusNotFound, out.StatusCode)
	assert.False(t, called, "4xx must not trigger any sibling fetch")
}

// TestMaybeRotateSolodcdnEdge_CappedAtTwoAlternates: with a large edge pool and
// every sibling also failing 5xx, exactly maxSolodcdnRotations (2) siblings are
// tried — the failed edge is skipped and the cap holds independent of pool size.
func TestMaybeRotateSolodcdnEdge_CappedAtTwoAlternates(t *testing.T) {
	bad := statusServer(t, http.StatusServiceUnavailable, "err")

	var hooks [][3]string
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  []string{"p12", "p13", "p14", "p15", "p16"},
		OnEdgeRotation: func(from, to, outcome string) { hooks = append(hooks, [3]string{from, to, outcome}) },
	})

	first, err := http.Get(bad.URL + "/x.ts")
	require.NoError(t, err)

	var calls []string
	out := p.maybeRotateSolodcdnEdge(first, "https://p12.solodcdn.com/x.ts", func(siblingURL string) (*http.Response, error) {
		calls = append(calls, siblingURL)
		return http.Get(bad.URL + "/x.ts") // every sibling also 5xx
	})
	defer out.Body.Close()

	assert.Equal(t, []string{"https://p13.solodcdn.com/x.ts", "https://p14.solodcdn.com/x.ts"}, calls,
		"capped at 2 alternates: p13, p14; p15/p16 never reached")
	assert.Equal(t, http.StatusServiceUnavailable, out.StatusCode, "falls back to the last (still-500) response for the caller's 502")
	assert.Equal(t, [][3]string{{"p12", "p13", "fail"}, {"p12", "p14", "fail"}}, hooks)
}

// TestMaybeRotateSolodcdnEdge_NonSolodcdnNoop: a 500 from any non-solodcdn host
// is returned untouched — the retry is fenced to the p<N>.solodcdn.com family.
func TestMaybeRotateSolodcdnEdge_NonSolodcdnNoop(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	p := NewVideoProxy(ProxyConfig{
		OnEdgeRotation: func(_, _, _ string) { t.Fatal("hook must not fire for a non-solodcdn host") },
	})

	first, err := http.Get(bad.URL + "/x.ts")
	require.NoError(t, err)

	called := false
	out := p.maybeRotateSolodcdnEdge(first, "https://cdn.example.com/x.ts", func(string) (*http.Response, error) {
		called = true
		return nil, nil
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, out.StatusCode)
	assert.False(t, called, "non-solodcdn host must not rotate")
}

// TestMaybeRotateSolodcdnEdge_TransportErrorThenSuccess: a transport error on
// the first sibling keeps the original response's body live, then the second
// sibling succeeds — the error outcome is recorded and the cap still counts it.
func TestMaybeRotateSolodcdnEdge_TransportErrorThenSuccess(t *testing.T) {
	bad := statusServer(t, http.StatusBadGateway, "boom")
	good := statusServer(t, http.StatusOK, "ok")

	var hooks [][3]string
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  []string{"p12", "p13", "p14"},
		OnEdgeRotation: func(from, to, outcome string) { hooks = append(hooks, [3]string{from, to, outcome}) },
	})

	first, err := http.Get(bad.URL + "/x.ts")
	require.NoError(t, err)

	attempt := 0
	out := p.maybeRotateSolodcdnEdge(first, "https://p12.solodcdn.com/x.ts", func(siblingURL string) (*http.Response, error) {
		attempt++
		if attempt == 1 {
			return nil, fmt.Errorf("dial p13: connection refused")
		}
		return http.Get(good.URL + "/x.ts")
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusOK, out.StatusCode)
	assert.Equal(t, [][3]string{{"p12", "p13", "error"}, {"p12", "p14", "success"}}, hooks)
}

// TestMaybeRotateSolodcdnEdge_Sibling4xxStopsWithFail: a sibling that answers
// 4xx is authoritative — rotation stops (never retry 4xx) and the outcome is
// recorded as fail (it didn't heal playback).
func TestMaybeRotateSolodcdnEdge_Sibling4xxStopsWithFail(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	notfound := statusServer(t, http.StatusNotFound, "nf")

	var hooks [][3]string
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  []string{"p12", "p13", "p14"},
		OnEdgeRotation: func(from, to, outcome string) { hooks = append(hooks, [3]string{from, to, outcome}) },
	})

	first, err := http.Get(bad.URL + "/x.ts")
	require.NoError(t, err)

	calls := 0
	out := p.maybeRotateSolodcdnEdge(first, "https://p12.solodcdn.com/x.ts", func(string) (*http.Response, error) {
		calls++
		return http.Get(notfound.URL + "/x.ts")
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusNotFound, out.StatusCode)
	assert.Equal(t, 1, calls, "a sibling 4xx stops rotation immediately")
	assert.Equal(t, [][3]string{{"p12", "p13", "fail"}}, hooks)
}

// TestMaybeRotateSolodcdnEdge_DefaultEdgePool: an empty SolodcdnEdges falls back
// to the built-in default (p12,p13,p14).
func TestMaybeRotateSolodcdnEdge_DefaultEdgePool(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	good := statusServer(t, http.StatusOK, "ok")

	p := NewVideoProxy(ProxyConfig{}) // no SolodcdnEdges configured

	first, err := http.Get(bad.URL + "/x.ts")
	require.NoError(t, err)

	var calls []string
	out := p.maybeRotateSolodcdnEdge(first, "https://p13.solodcdn.com/x.ts", func(siblingURL string) (*http.Response, error) {
		calls = append(calls, siblingURL)
		return http.Get(good.URL + "/x.ts")
	})
	defer out.Body.Close()

	assert.Equal(t, http.StatusOK, out.StatusCode)
	// From-edge p13 is skipped; default pool p12,p13,p14 => first sibling is p12.
	assert.Equal(t, []string{"https://p12.solodcdn.com/x.ts"}, calls)
}
