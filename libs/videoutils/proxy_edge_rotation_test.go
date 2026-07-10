package videoutils

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timeoutErr is a net.Error whose Timeout() reports true — models the
// transport's ResponseHeaderTimeout firing (classifyEdgeOutcome => "timeout").
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout awaiting response headers" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// statusServer is a tiny httptest upstream that always answers `code` with
// `body`, registered for cleanup. Used to build real *http.Response values (with
// live bodies over keep-alive conns) for the edge-failover unit tests.
func statusServer(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s
}

// route helpers: each returns a do()-style thunk for one edge's behaviour.
func srv(s *httptest.Server) func() (*http.Response, error) {
	return func() (*http.Response, error) { return http.Get(s.URL) }
}
func dialErr() func() (*http.Response, error) {
	return func() (*http.Response, error) { return nil, fmt.Errorf("dial: connection refused") }
}
func hdrTimeout() func() (*http.Response, error) {
	return func() (*http.Response, error) { return nil, timeoutErr{} }
}

// edgeRoute builds a do(fetchURL) that routes by solodcdn edge label ("p12") to
// the matching per-edge behaviour. A "" key routes non-solodcdn fetches.
func edgeRoute(routes map[string]func() (*http.Response, error)) func(string) (*http.Response, error) {
	return func(fetchURL string) (*http.Response, error) {
		edge := ""
		if u, err := url.Parse(fetchURL); err == nil {
			edge = solodcdnEdgeOf(u.Host)
		}
		if r, ok := routes[edge]; ok {
			return r()
		}
		return nil, fmt.Errorf("test: no route for edge %q (%s)", edge, fetchURL)
	}
}

// recorder captures the OnEdgeRotation and OnEdgeAttempt hook streams.
type recorder struct {
	rotations [][3]string // {from, to, outcome}
	attempts  []string    // "edge:outcome" in attempt order (mirrors the trail)
}

func recordingProxy(edges []string) (*VideoProxy, *recorder) {
	rec := &recorder{}
	p := NewVideoProxy(ProxyConfig{
		SolodcdnEdges:  edges,
		OnEdgeRotation: func(from, to, outcome string) { rec.rotations = append(rec.rotations, [3]string{from, to, outcome}) },
		OnEdgeAttempt:  func(edge, outcome string, _ int64) { rec.attempts = append(rec.attempts, edge+":"+outcome) },
	})
	return p, rec
}

// trailPairs renders an edgeFailover trail as "edge:outcome" (dropping the
// timing, which varies) so tests can assert order+outcome deterministically.
func trailPairs(ef edgeFailover) []string {
	out := make([]string, 0, len(ef.trail))
	for _, a := range ef.trail {
		out = append(out, a.edge+":"+a.outcome)
	}
	return out
}

const p12URL = "https://p12.solodcdn.com/s/m/seg-1.ts"

// --- Nominal serves directly (no rotation) --------------------------------

func TestFetchWithEdgeFailover_NominalOK(t *testing.T) {
	ok := statusServer(t, http.StatusOK, "#EXTM3U\n")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){"p12": srv(ok)}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p12", ef.served)
	assert.Equal(t, []string{"p12:ok"}, trailPairs(ef))
	assert.Empty(t, rec.rotations, "a healthy nominal edge never rotates")
	assert.Equal(t, []string{"p12:ok"}, rec.attempts)
}

func TestFetchWithEdgeFailover_Nominal4xxNoRotation(t *testing.T) {
	nf := statusServer(t, http.StatusNotFound, "nope")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){"p12": srv(nf)}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "a 4xx is authoritative — serve it")
	assert.Equal(t, "p12", ef.served)
	assert.Empty(t, rec.rotations, "4xx must not trigger any sibling fetch")
	assert.Equal(t, []string{"p12:http4xx"}, rec.attempts)
}

// --- >=500 rotation (existing AUTO-562 behaviour, preserved) ---------------

func TestFetchWithEdgeFailover_Nominal5xxSiblingOK(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	good := statusServer(t, http.StatusOK, "#EXTM3U\n")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": srv(bad), "p13": srv(good),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p13", ef.served, "stops at the first healthy sibling; never tries p14")
	assert.Equal(t, []string{"p12:http5xx", "p13:ok"}, trailPairs(ef))
	assert.Equal(t, [][3]string{{"p12", "p13", "success"}}, rec.rotations)
}

func TestFetchWithEdgeFailover_All5xxCappedAtTwo(t *testing.T) {
	bad := statusServer(t, http.StatusServiceUnavailable, "err")
	p, rec := recordingProxy([]string{"p12", "p13", "p14", "p15", "p16"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": srv(bad), "p13": srv(bad), "p14": srv(bad), "p15": srv(bad), "p16": srv(bad),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "falls back to the last 5xx for the caller's 502")
	assert.Equal(t, "p14", ef.served)
	assert.Equal(t, []string{"p12:http5xx", "p13:http5xx", "p14:http5xx"}, trailPairs(ef), "nominal + exactly 2 siblings; p15/p16 unreached")
	assert.Equal(t, [][3]string{{"p12", "p13", "fail"}, {"p12", "p14", "fail"}}, rec.rotations)
}

func TestFetchWithEdgeFailover_Sibling4xxStopsWithFail(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	nf := statusServer(t, http.StatusNotFound, "nf")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": srv(bad), "p13": srv(nf),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "p13", ef.served)
	assert.Equal(t, [][3]string{{"p12", "p13", "fail"}}, rec.rotations, "a sibling 4xx stops rotation immediately")
	assert.Equal(t, []string{"p12:http5xx", "p13:http4xx"}, trailPairs(ef))
}

// --- NEW: primary transport-error / timeout rotates "straight ahead" -------

func TestFetchWithEdgeFailover_NominalDialErrorSiblingOK(t *testing.T) {
	good := statusServer(t, http.StatusOK, "#EXTM3U\n")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	// The report's failure mode: the nominal edge hangs/resets (browser status:0),
	// which is a PRIMARY transport error — AUTO-562 never rotated on this. Now it
	// fails over to a sibling straight ahead.
	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": dialErr(), "p13": srv(good),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p13", ef.served)
	assert.Equal(t, []string{"p12:dial_error", "p13:ok"}, trailPairs(ef))
	assert.Equal(t, [][3]string{{"p12", "p13", "success"}}, rec.rotations)
}

func TestFetchWithEdgeFailover_NominalTimeoutSiblingOK(t *testing.T) {
	good := statusServer(t, http.StatusOK, "#EXTM3U\n")
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	// A cold edge that accepts TCP but blows the (generous) response-header window
	// surfaces as a timeout — also rotates now.
	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": hdrTimeout(), "p13": srv(good),
	}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p13", ef.served)
	assert.Equal(t, []string{"p12:timeout", "p13:ok"}, trailPairs(ef))
	assert.Equal(t, [][3]string{{"p12", "p13", "success"}}, rec.rotations)
}

func TestFetchWithEdgeFailover_AllDialErrorReturnsError(t *testing.T) {
	p, rec := recordingProxy([]string{"p12", "p13", "p14"})

	resp, ef, err := p.fetchWithEdgeFailover(p12URL, edgeRoute(map[string]func() (*http.Response, error){
		"p12": dialErr(), "p13": dialErr(), "p14": dialErr(),
	}))

	require.Error(t, err, "every edge failed at the transport layer → surface the error (caller 502)")
	assert.Nil(t, resp)
	assert.Equal(t, "", ef.served)
	assert.Equal(t, []string{"p12:dial_error", "p13:dial_error", "p14:dial_error"}, trailPairs(ef))
	assert.Equal(t, [][3]string{{"p12", "p13", "error"}, {"p12", "p14", "error"}}, rec.rotations)
}

// --- Fencing + defaults ----------------------------------------------------

func TestFetchWithEdgeFailover_NonSolodcdnSingleAttempt(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	p, rec := recordingProxy(nil)

	// A non-solodcdn host takes the fast path: exactly one plain attempt, returned
	// verbatim — no rotation, no trail, no OnEdgeAttempt metric (the failover
	// machinery must never touch the universal per-segment path).
	resp, ef, err := p.fetchWithEdgeFailover("https://cdn.example.com/x.ts",
		edgeRoute(map[string]func() (*http.Response, error){"": srv(bad)}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "", ef.served)
	assert.Empty(t, ef.trail, "fast path records no attempt trail")
	assert.Empty(t, rec.rotations, "non-solodcdn host must never rotate")
	assert.Empty(t, rec.attempts, "no OnEdgeAttempt fired for non-solodcdn traffic")
}

func TestFetchWithEdgeFailover_NonSolodcdnErrorPropagates(t *testing.T) {
	p, _ := recordingProxy(nil)

	resp, ef, err := p.fetchWithEdgeFailover("https://cdn.example.com/x.ts",
		edgeRoute(map[string]func() (*http.Response, error){"": dialErr()}))

	require.Error(t, err, "a non-solodcdn transport error is returned unchanged, not rotated")
	assert.Nil(t, resp)
	assert.Equal(t, "", ef.served)
	assert.Empty(t, ef.trail, "fast path records no attempt trail")
}

func TestFetchWithEdgeFailover_DefaultEdgePool(t *testing.T) {
	bad := statusServer(t, http.StatusInternalServerError, "boom")
	good := statusServer(t, http.StatusOK, "ok")
	p, rec := recordingProxy(nil) // empty pool => built-in default p12,p13,p14

	// Nominal is p13; default pool skips it and the first sibling is p12.
	resp, ef, err := p.fetchWithEdgeFailover("https://p13.solodcdn.com/x.ts",
		edgeRoute(map[string]func() (*http.Response, error){"p13": srv(bad), "p12": srv(good)}))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p12", ef.served)
	assert.Equal(t, [][3]string{{"p13", "p12", "success"}}, rec.rotations)
}

// --- trailString rendering -------------------------------------------------

func TestEdgeFailover_TrailString(t *testing.T) {
	ef := edgeFailover{trail: []edgeAttempt{
		{edge: "p13", outcome: "timeout", ms: 45003},
		{edge: "p12", outcome: "ok", ms: 210},
	}}
	assert.Equal(t, "p13:timeout:45003,p12:ok:210", ef.trailString())
}

func TestClassifyEdgeOutcome(t *testing.T) {
	assert.Equal(t, "timeout", classifyEdgeOutcome(nil, timeoutErr{}))
	assert.Equal(t, "dial_error", classifyEdgeOutcome(nil, fmt.Errorf("connection refused")))
	assert.Equal(t, "http5xx", classifyEdgeOutcome(&http.Response{StatusCode: 503}, nil))
	assert.Equal(t, "http4xx", classifyEdgeOutcome(&http.Response{StatusCode: 404}, nil))
	assert.Equal(t, "ok", classifyEdgeOutcome(&http.Response{StatusCode: 200}, nil))
}
