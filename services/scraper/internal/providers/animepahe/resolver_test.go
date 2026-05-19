package animepahe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// newTestResolverClient builds a resolverClient pointed at an httptest
// server with retries disabled so error-mapping assertions don't see
// retry amplification.
func newTestResolverClient(t *testing.T, srv *httptest.Server) *resolverClient {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	return newResolverClient(srv.URL, hc)
}

// TestResolverClient_ErrorMapping pins the 200/404/502/other → domain.Err*
// contract for all three resolver methods (Search / Release / Play).
// Table-driven so adding a new status row to the contract is one line.
func TestResolverClient_ErrorMapping(t *testing.T) {
	t.Parallel()

	type stub struct {
		// op is the resolver method label; appears in the wrap message
		op string
		// call wraps the resolverClient invocation so the test loop can
		// drive any of the three endpoints uniformly. Returns the error
		// from the method (nil on the 200 path).
		call func(ctx context.Context, c *resolverClient) error
	}

	stubs := []stub{
		{
			op: "search",
			call: func(ctx context.Context, c *resolverClient) error {
				_, err := c.Search(ctx, "Frieren")
				return err
			},
		},
		{
			op: "release",
			call: func(ctx context.Context, c *resolverClient) error {
				_, err := c.Release(ctx, "abc-uuid", 1)
				return err
			},
		},
		{
			op: "play",
			call: func(ctx context.Context, c *resolverClient) error {
				_, err := c.Play(ctx, "abc", "def")
				return err
			},
		},
	}

	type wantErr struct {
		sentinel error // nil for the 200-OK case
	}
	cases := []struct {
		name   string
		status int
		body   string
		want   wantErr
	}{
		{"200 OK", http.StatusOK, `{"data":[{"id":1,"session":"abc","title":"Frieren"}]}`, wantErr{nil}},
		{"404 maps to ErrNotFound", http.StatusNotFound, ``, wantErr{domain.ErrNotFound}},
		{"502 maps to ErrProviderDown (stealth un-solvable)", http.StatusBadGateway, ``, wantErr{domain.ErrProviderDown}},
		{"500 maps to ErrProviderDown", http.StatusInternalServerError, ``, wantErr{domain.ErrProviderDown}},
	}

	for _, s := range stubs {
		s := s
		for _, tc := range cases {
			tc := tc
			t.Run(s.op+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					if tc.body != "" {
						_, _ = w.Write([]byte(tc.body))
					}
				}))
				defer srv.Close()

				rc := newTestResolverClient(t, srv)
				err := s.call(context.Background(), rc)

				if tc.want.sentinel == nil {
					if err != nil {
						t.Fatalf("%s 200 path err = %v; want nil", s.op, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("%s status %d: err = nil; want %v", s.op, tc.status, tc.want.sentinel)
				}
				if !errors.Is(err, tc.want.sentinel) {
					t.Errorf("%s status %d: errors.Is(err, %v) = false; err = %v",
						s.op, tc.status, tc.want.sentinel, err)
				}
			})
		}
	}
}

// TestResolverClient_PlayReturnsHTML pins the contract that /play returns
// the raw HTML body string (not parsed JSON), so the orchestrator's
// goquery-driven ListServers can pass it directly into NewDocumentFromReader.
func TestResolverClient_PlayReturnsHTML(t *testing.T) {
	t.Parallel()
	const playHTML = `<html><body><button data-src="https://kwik.cx/e/abc-720p" data-audio="jpn">720p · JP</button></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(playHTML))
	}))
	defer srv.Close()

	rc := newTestResolverClient(t, srv)
	got, err := rc.Play(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("Play err = %v; want nil", err)
	}
	if got != playHTML {
		t.Errorf("Play body = %q; want %q", got, playHTML)
	}
}

// TestResolverClient_QueryEncoding pins the contract that arbitrary search
// queries are URL-escaped (not raw-interpolated) so a `&` in the title
// can't smuggle parameters into the resolver URL. Mirrors the pre-Phase-27
// TestProvider_FindID_QuerySafetyEscape behavior, but at the resolver
// transport layer.
func TestResolverClient_QueryEncoding(t *testing.T) {
	t.Parallel()
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	rc := newTestResolverClient(t, srv)
	_, _ = rc.Search(context.Background(), "a&b#c?d=e/f")
	if captured != "a&b#c?d=e/f" {
		t.Errorf("q param round-trip = %q; want \"a&b#c?d=e/f\"", captured)
	}
}

// TestResolverClient_NewTrimsTrailingSlash ensures the constructor strips
// a trailing `/` from baseURL so the URL builders don't emit `//search`.
func TestResolverClient_NewTrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	log, _ := logger.New(logger.Config{Level: "error", Encoding: "console"})
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	rc := newResolverClient("http://animepahe-resolver:3000/", hc)
	if rc.baseURL != "http://animepahe-resolver:3000" {
		t.Errorf("baseURL = %q; want trailing slash trimmed", rc.baseURL)
	}
}
