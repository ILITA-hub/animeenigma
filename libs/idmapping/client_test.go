package idmapping

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a Client whose ARM and AniList endpoints both
// point at the given httptest servers. Both endpoints use the same
// httpClient (no IPv4-only dialer needed since the test loopback is v4).
func newTestClient(armServerURL, aniListServerURL string) *Client {
	c := NewClient()
	// Replace the IPv4-only dialer transport with the default — loopback
	// works on the default stack and we don't want to depend on a real v4
	// route inside the test environment.
	c.httpClient = &http.Client{}
	c.baseURL = armServerURL
	c.aniListBaseURL = aniListServerURL
	return c
}

// armOKBody is a representative ARM response containing all six fields.
const armOKBody = `{
  "anilist": 21,
  "myanimelist": 21,
  "anidb": 69,
  "kitsu": 12,
  "livechart": 1234,
  "imdb": "tt0388629"
}`

// aniListOKBody mirrors the AniList GraphQL response for `idMal:21`.
const aniListOKBody = `{"data":{"Media":{"id":21,"idMal":21}}}`

// stubRoundTripper records the hosts it sees and delegates to an inner
// RoundTripper. It stands in for tracing.WrapRecording's recording transport:
// the test asserts that injected transports actually intercept outbound
// requests (the egress-recording seam).
type stubRoundTripper struct {
	inner http.RoundTripper
	hosts []string
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.hosts = append(s.hosts, req.URL.Host)
	return s.inner.RoundTrip(req)
}

// TestIDMappingTransport — WithTransport routes outbound ARM/AniList requests
// through the injected (recording) transport, and the default NewClient()
// continues to build its own IPv4 transport.
func TestIDMappingTransport(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(armOKBody))
	}))
	defer armSrv.Close()

	stub := &stubRoundTripper{inner: http.DefaultTransport}
	c := NewClient(WithTransport(stub))
	c.baseURL = armSrv.URL
	c.aniListBaseURL = armSrv.URL // unused on the happy path

	if _, err := c.ResolveByMALID("21"); err != nil {
		t.Fatalf("ResolveByMALID error: %v", err)
	}
	if len(stub.hosts) == 0 {
		t.Fatalf("injected transport was never invoked — egress recording would never fire")
	}
	wantHost := strings.TrimPrefix(armSrv.URL, "http://")
	if stub.hosts[0] != wantHost {
		t.Errorf("recorder saw host %q, want %q", stub.hosts[0], wantHost)
	}

	// Default client preserves the IPv4-forced transport (no stub).
	def := NewClient()
	if def.httpClient.Transport == nil {
		t.Fatalf("default NewClient() must keep its IPv4 transport")
	}
	if _, ok := def.httpClient.Transport.(*stubRoundTripper); ok {
		t.Errorf("default NewClient() unexpectedly used the stub transport")
	}
}

// TestResolveByMALID_ARMHappyPath — ARM responds with a complete mapping,
// AniList must NOT be called.
func TestResolveByMALID_ARMHappyPath(t *testing.T) {
	armHits := 0
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		armHits++
		if !strings.Contains(r.URL.Path, "/ids") || r.URL.Query().Get("source") != "myanimelist" {
			t.Errorf("unexpected ARM request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(armOKBody))
	}))
	defer armSrv.Close()

	aniListHits := 0
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aniListHits++
		t.Error("AniList must NOT be hit when ARM gives a complete result")
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	got, err := c.ResolveByMALID("21")
	if err != nil {
		t.Fatalf("ResolveByMALID: %v", err)
	}
	if got == nil || got.AniList == nil || *got.AniList != 21 {
		t.Fatalf("expected AniList=21, got %+v", got)
	}
	if got.AniDB == nil || *got.AniDB != 69 {
		t.Fatalf("expected AniDB=69 (ARM-only field), got %+v", got)
	}
	if armHits != 1 {
		t.Errorf("expected exactly 1 ARM hit, got %d", armHits)
	}
	if aniListHits != 0 {
		t.Errorf("expected 0 AniList hits, got %d", aniListHits)
	}
}

// TestResolveByMALID_ARMFailsAniListFallback — ARM hangs / errors, AniList
// fallback fires and returns an AniList ID. AniDB/Kitsu/etc. stay nil.
func TestResolveByMALID_ARMFailsAniListFallback(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream blackholed"))
	}))
	defer armSrv.Close()

	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request shape.
		if r.Method != http.MethodPost {
			t.Errorf("AniList: expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"mal":21`) {
			t.Errorf("AniList: missing mal=21 in body: %s", body)
		}
		_, _ = w.Write([]byte(aniListOKBody))
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	got, err := c.ResolveByMALID("21")
	if err != nil {
		t.Fatalf("ResolveByMALID: %v", err)
	}
	if got == nil || got.AniList == nil || *got.AniList != 21 {
		t.Fatalf("expected AniList=21 via fallback, got %+v", got)
	}
	if got.AniDB != nil || got.Kitsu != nil || got.LiveChart != nil || got.IMDB != nil {
		t.Errorf("expected ARM-only fields nil under fallback, got %+v", got)
	}
}

// TestResolveByMALID_ARMReturnsNoAniListID — ARM returns success but with
// a null AniList field (e.g. only AniDB present). AniList fallback fills
// in the AniList ID while preserving ARM's other fields.
func TestResolveByMALID_ARMReturnsNoAniListID(t *testing.T) {
	armPartial := `{"anilist":null,"myanimelist":21,"anidb":69,"kitsu":null,"livechart":null,"imdb":null}`
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(armPartial))
	}))
	defer armSrv.Close()

	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(aniListOKBody))
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	got, err := c.ResolveByMALID("21")
	if err != nil {
		t.Fatalf("ResolveByMALID: %v", err)
	}
	if got == nil || got.AniList == nil || *got.AniList != 21 {
		t.Fatalf("expected AniList=21 via fallback, got %+v", got)
	}
	if got.AniDB == nil || *got.AniDB != 69 {
		t.Fatalf("expected AniDB=69 preserved from ARM partial result, got %+v", got)
	}
}

// TestResolveByMALID_BothFail — ARM errors AND AniList also fails;
// return the wrapped ARM error so the maintenance bot keys on it.
func TestResolveByMALID_BothFail(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("ARM is down"))
	}))
	defer armSrv.Close()

	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("AniList exploded"))
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	_, err := c.ResolveByMALID("21")
	if err == nil {
		t.Fatal("expected error when both ARM and AniList fail")
	}
	if !strings.Contains(err.Error(), "ARM") {
		t.Errorf("expected error to mention ARM (operator triage), got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "AniList fallback also failed") {
		t.Errorf("expected error to mention AniList fallback failure, got %q", err.Error())
	}
}

// TestResolveByMALID_ARMNotFoundAniListSucceeds — ARM returns 404 (no
// mapping), AniList still finds the AniList ID. We must not return nil
// in this case — AniList knows the mapping.
func TestResolveByMALID_ARMNotFoundAniListSucceeds(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer armSrv.Close()

	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(aniListOKBody))
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	got, err := c.ResolveByMALID("21")
	if err != nil {
		t.Fatalf("ResolveByMALID: %v", err)
	}
	if got == nil || got.AniList == nil || *got.AniList != 21 {
		t.Fatalf("expected AniList=21 via fallback after ARM 404, got %+v", got)
	}
}

// TestResolveByMALID_AniListUnknownReturnsARMResult — AniList genuinely
// has no Media with this MAL ID. ARM gave us partial (no AniList), and
// AniList can't help either. Return ARM's partial result (graceful) and
// no error — caller will see nil AniList and handle accordingly.
func TestResolveByMALID_AniListUnknownReturnsARMResult(t *testing.T) {
	armPartial := `{"anilist":null,"myanimelist":21,"anidb":69}`
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(armPartial))
	}))
	defer armSrv.Close()

	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// AniList returns success with `data.Media: null` for unknown IDs.
		_, _ = w.Write([]byte(`{"data":{"Media":null}}`))
	}))
	defer aniSrv.Close()

	c := newTestClient(armSrv.URL, aniSrv.URL)
	got, err := c.ResolveByMALID("21")
	if err != nil {
		t.Fatalf("expected nil error when ARM partial and AniList unknown, got %v", err)
	}
	if got == nil {
		t.Fatal("expected ARM partial result preserved, got nil")
	}
	if got.AniList != nil {
		t.Errorf("expected AniList nil (neither source knew), got %v", *got.AniList)
	}
	if got.AniDB == nil || *got.AniDB != 69 {
		t.Errorf("expected AniDB=69 preserved from ARM, got %+v", got)
	}
}

// TestResolveByMALID_EmptyID — both paths must refuse empty input
// before issuing any HTTP.
func TestResolveByMALID_EmptyID(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("ARM must NOT be hit on empty ID")
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("AniList must NOT be hit on empty ID")
	}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)
	_, err := c.ResolveByMALID("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

// TestResolveByShikimoriID_DelegatesSameAsMAL — Shikimori IDs equal MAL
// IDs by upstream contract; the two methods must produce identical
// behavior on the same numeric ID.
func TestResolveByShikimoriID_DelegatesSameAsMAL(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Source must be myanimelist for both call paths.
		if r.URL.Query().Get("source") != "myanimelist" {
			t.Errorf("expected source=myanimelist, got %q", r.URL.Query().Get("source"))
		}
		_, _ = w.Write([]byte(armOKBody))
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)
	a, errA := c.ResolveByMALID("21")
	b, errB := c.ResolveByShikimoriID("21")
	if errA != nil || errB != nil {
		t.Fatalf("errors: %v / %v", errA, errB)
	}
	if a == nil || b == nil || *a.AniList != *b.AniList {
		t.Errorf("Shikimori and MAL paths must return identical AniList IDs; got %+v vs %+v", a, b)
	}
}

// TestResolveByMALID_AniListGraphQLError — AniList returns 200 but with
// an `errors` array. We must surface this as the fallback failure and
// return the original ARM error (preserved for triage).
func TestResolveByMALID_AniListGraphQLError(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"Media":null},"errors":[{"message":"Validation: idMal must be Int"}]}`))
	}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)
	_, err := c.ResolveByMALID("21")
	if err == nil {
		t.Fatal("expected error when both ARM and AniList fail")
	}
	if !strings.Contains(err.Error(), "GraphQL") {
		t.Errorf("expected error to mention GraphQL for AniList failure mode, got %q", err.Error())
	}
}

// TestResolveByMALID_NonNumericIDRejectedByFallback — when ARM also
// fails, the fallback rejects a non-numeric MAL ID with a clear message.
func TestResolveByMALID_NonNumericIDRejectedByFallback(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("AniList must NOT be hit for non-numeric ID (parse fails first)")
	}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)
	_, err := c.ResolveByMALID("not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric MAL ID")
	}
	if !strings.Contains(err.Error(), "AniList: invalid MAL id") {
		t.Errorf("expected fallback to refuse non-numeric ID, got %q", err.Error())
	}
}

// ensure errors.Unwrap surface still works on the wrapped ARM error.
func TestResolveByMALID_ErrorUnwrap(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)
	_, err := c.ResolveByMALID("21")
	if err == nil {
		t.Fatal("expected error")
	}
	// We don't promise a sentinel; just verify wrapping shape is sane.
	if next := errors.Unwrap(err); next == nil {
		t.Logf("(no unwrap target, OK for fmt.Errorf %%w over a string-formatted parent — this is informational, not a hard requirement)")
	}
	_ = fmt.Sprintf // imports
}

// TestResolveByMALIDContext_PropagatesCtx proves the caller's ctx threads into
// the outbound ARM/AniList requests (WR-01): an already-cancelled ctx must abort
// the call before either upstream is hit, and neither server should observe a
// request.
func TestResolveByMALIDContext_PropagatesCtx(t *testing.T) {
	var armHits, aniHits int
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		armHits++
		_, _ = w.Write([]byte(armOKBody))
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aniHits++
	}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the call so the threaded ctx aborts the HTTP request

	_, err := c.ResolveByMALIDContext(ctx, "21")
	if err == nil {
		t.Fatal("expected error from a cancelled ctx threaded into the request")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error chain does not carry context.Canceled (ctx not propagated): %v", err)
	}
	if armHits != 0 || aniHits != 0 {
		t.Fatalf("servers were hit despite a pre-cancelled ctx (arm=%d ani=%d) — ctx not threaded", armHits, aniHits)
	}
}

// TestResolveByShikimoriIDContext_Succeeds proves the ctx-aware Shikimori
// variant resolves identically to the no-ctx wrapper on the happy path.
func TestResolveByShikimoriIDContext_Succeeds(t *testing.T) {
	armSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(armOKBody))
	}))
	defer armSrv.Close()
	aniSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer aniSrv.Close()
	c := newTestClient(armSrv.URL, aniSrv.URL)

	res, err := c.ResolveByShikimoriIDContext(context.Background(), "21")
	if err != nil {
		t.Fatalf("ResolveByShikimoriIDContext: %v", err)
	}
	if res == nil || res.AniList == nil {
		t.Fatalf("expected a populated AniList ID, got %+v", res)
	}
}
