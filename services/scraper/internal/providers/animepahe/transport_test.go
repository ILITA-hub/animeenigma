package animepahe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// transportProvider builds a Provider whose upstream transport (Search /
// Release / Play) is the methods on *Provider in transport.go. It is wired to
// the supplied Deps overrides on top of the package's shared fakes — the
// caller passes BaseURL (an httptest server) and, for the browser-path test,
// UseBrowser + BrowserFetch.
//
// Retries are disabled on the BaseHTTPClient so error-mapping assertions don't
// see retry amplification (mirrors the now-deleted newTestResolverClient).
func transportProvider(t *testing.T, d Deps) *Provider {
	t.Helper()
	log := newTestLogger(t)
	if d.HTTP == nil {
		d.HTTP = domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	}
	if d.Embeds == nil {
		reg := domain.NewRegistry()
		reg.Register(&fakeKwikExtractor{streams: map[string]*domain.Stream{}})
		d.Embeds = reg
	}
	if d.MalSync == nil {
		d.MalSync = &fakeMalSync{mappings: map[string]string{}, misses: map[string]bool{}}
	}
	if d.Cache == nil {
		d.Cache = newFakeCache()
	}
	if d.Log == nil {
		d.Log = log
	}
	p, err := New(d)
	if err != nil {
		t.Fatalf("New(Deps{...}) = err %v; want nil", err)
	}
	return p
}

// animepaheMux stands up a fake animepahe.pw that routes the THREE real
// upstream shapes the transport builds:
//
//   - GET /api?m=search&q=<q>                               → searchBody
//   - GET /api?m=release&id=<s>&sort=episode_asc&page=<n>   → releaseBody
//   - GET /play/<animeSession>/<episodeSession>             → playBody (HTML)
//
// Any handler may be nil → the route 404s (lets a test assert one endpoint in
// isolation). captured.* records the parsed request shape for assertions.
type capturedReq struct {
	searchQ     string
	releaseID   string
	releaseSort string
	releasePage string
	playAnime   string
	playEpisode string
}

func newAnimepaheMux(t *testing.T, cap *capturedReq, searchBody, releaseBody, playBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api" && r.URL.Query().Get("m") == "search":
			cap.searchQ = r.URL.Query().Get("q")
			if searchBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(searchBody))
		case r.URL.Path == "/api" && r.URL.Query().Get("m") == "release":
			cap.releaseID = r.URL.Query().Get("id")
			cap.releaseSort = r.URL.Query().Get("sort")
			cap.releasePage = r.URL.Query().Get("page")
			if releaseBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(releaseBody))
		case strings.HasPrefix(r.URL.Path, "/play/"):
			// /play/<anime>/<ep> — split the two trailing segments.
			rest := strings.TrimPrefix(r.URL.Path, "/play/")
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) == 2 {
				cap.playAnime = parts[0]
				cap.playEpisode = parts[1]
			}
			if playBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(playBody))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

// TestTransport_Search_ParsesResults: the plain-HTTP fallback path (no
// UseBrowser/BrowserFetch wired) GETs /api?m=search&q=<q> and decodes the
// searchResponse. Also pins that the query is URL-escaped, not raw-
// interpolated (the &/#/?/= smuggling guard the deleted resolver test held).
func TestTransport_Search_ParsesResults(t *testing.T) {
	t.Parallel()
	cap := &capturedReq{}
	const body = `{"total":2,"per_page":8,"current_page":1,"last_page":1,"data":[` +
		`{"id":4,"session":"anime-session-naruto-001","title":"Naruto","type":"TV","year":2002,"episodes":220},` +
		`{"id":1735,"session":"anime-session-narutoshippuden-001","title":"Naruto: Shippuuden","type":"TV","year":2007,"episodes":500}` +
		`]}`
	srv := newAnimepaheMux(t, cap, body, "", "")
	defer srv.Close()

	p := transportProvider(t, Deps{BaseURL: srv.URL})
	sr, err := p.Search(context.Background(), "a&b#c?d=e/f")
	if err != nil {
		t.Fatalf("Search err = %v; want nil", err)
	}
	if len(sr.Data) != 2 {
		t.Fatalf("len(sr.Data) = %d; want 2", len(sr.Data))
	}
	if sr.Data[0].Session != "anime-session-naruto-001" || sr.Data[0].Title != "Naruto" {
		t.Errorf("sr.Data[0] = %+v; want session/title naruto", sr.Data[0])
	}
	// Query round-trips intact only if url.QueryEscape was used.
	if cap.searchQ != "a&b#c?d=e/f" {
		t.Errorf("q param round-trip = %q; want \"a&b#c?d=e/f\"", cap.searchQ)
	}
}

// TestTransport_Release_ParsesPaginationAndEpisodes: /api?m=release&id=<s>&
// sort=episode_asc&page=<n> decodes the releaseResponse including pagination
// meta, and the request carries the canonical sort + page params.
func TestTransport_Release_ParsesPaginationAndEpisodes(t *testing.T) {
	t.Parallel()
	cap := &capturedReq{}
	const body = `{"total":4,"per_page":2,"current_page":2,"last_page":3,"from":3,"to":4,"data":[` +
		`{"session":"ep-s3","episode":3,"filler":1,"title":"Three"},` +
		`{"session":"ep-s4","episode":4,"filler":0,"title":"Four"}` +
		`]}`
	srv := newAnimepaheMux(t, cap, "", body, "")
	defer srv.Close()

	p := transportProvider(t, Deps{BaseURL: srv.URL})
	rr, err := p.Release(context.Background(), "uuid-anime-session", 2)
	if err != nil {
		t.Fatalf("Release err = %v; want nil", err)
	}
	if rr.CurrentPage != 2 || rr.LastPage != 3 {
		t.Errorf("pagination = (%d,%d); want (2,3)", rr.CurrentPage, rr.LastPage)
	}
	if len(rr.Data) != 2 {
		t.Fatalf("len(rr.Data) = %d; want 2", len(rr.Data))
	}
	if rr.Data[0].Session != "ep-s3" || rr.Data[0].Filler != 1 {
		t.Errorf("rr.Data[0] = %+v; want session ep-s3 filler 1", rr.Data[0])
	}
	// Request shape: id escaped, sort=episode_asc, page=2.
	if cap.releaseID != "uuid-anime-session" {
		t.Errorf("release id = %q; want uuid-anime-session", cap.releaseID)
	}
	if cap.releaseSort != "episode_asc" {
		t.Errorf("release sort = %q; want episode_asc", cap.releaseSort)
	}
	if cap.releasePage != "2" {
		t.Errorf("release page = %q; want 2", cap.releasePage)
	}
}

// TestTransport_Play_ReturnsHTML: /play/<anime>/<ep> returns the raw HTML body
// (not parsed JSON) so ListServers can feed it straight into goquery. Also
// pins that the anime/episode sessions are PATH segments (path-escaped), not
// query params (the resolver shape).
func TestTransport_Play_ReturnsHTML(t *testing.T) {
	t.Parallel()
	cap := &capturedReq{}
	const playHTML = `<html><body><button data-src="https://kwik.cx/e/abc-720p" data-audio="jpn">720p · JP</button></body></html>`
	srv := newAnimepaheMux(t, cap, "", "", playHTML)
	defer srv.Close()

	p := transportProvider(t, Deps{BaseURL: srv.URL})
	got, err := p.Play(context.Background(), "anime-sess", "ep-sess")
	if err != nil {
		t.Fatalf("Play err = %v; want nil", err)
	}
	if got != playHTML {
		t.Errorf("Play body = %q; want %q", got, playHTML)
	}
	if cap.playAnime != "anime-sess" || cap.playEpisode != "ep-sess" {
		t.Errorf("play segments = (%q,%q); want (anime-sess, ep-sess)", cap.playAnime, cap.playEpisode)
	}
}

// TestTransport_ErrorMapping pins the 200/404/403/503/other → domain.Err*
// contract across all three transport methods (Search / Release / Play),
// exercising mapStatus through the plain-HTTP fallback path. Table-driven so a
// new status row is one line.
func TestTransport_ErrorMapping(t *testing.T) {
	t.Parallel()

	type stub struct {
		op   string
		call func(ctx context.Context, p *Provider) error
	}
	stubs := []stub{
		{"search", func(ctx context.Context, p *Provider) error {
			_, err := p.Search(ctx, "Frieren")
			return err
		}},
		{"release", func(ctx context.Context, p *Provider) error {
			_, err := p.Release(ctx, "abc-uuid", 1)
			return err
		}},
		{"play", func(ctx context.Context, p *Provider) error {
			_, err := p.Play(ctx, "abc", "def")
			return err
		}},
	}

	cases := []struct {
		name     string
		status   int
		body     string
		sentinel error // nil for the 200-OK case
	}{
		{"200 OK", http.StatusOK, `{"data":[{"id":1,"session":"abc","title":"Frieren"}]}`, nil},
		{"404 maps to ErrNotFound", http.StatusNotFound, ``, domain.ErrNotFound},
		{"403 challenge maps to ErrProviderDown", http.StatusForbidden, ``, domain.ErrProviderDown},
		{"503 blocked maps to ErrProviderDown", http.StatusServiceUnavailable, ``, domain.ErrProviderDown},
		{"502 maps to ErrProviderDown", http.StatusBadGateway, ``, domain.ErrProviderDown},
		{"500 maps to ErrProviderDown", http.StatusInternalServerError, ``, domain.ErrProviderDown},
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

				p := transportProvider(t, Deps{BaseURL: srv.URL})
				err := s.call(context.Background(), p)

				if tc.sentinel == nil {
					if err != nil {
						t.Fatalf("%s 200 path err = %v; want nil", s.op, err)
					}
					return
				}
				if err == nil {
					t.Fatalf("%s status %d: err = nil; want %v", s.op, tc.status, tc.sentinel)
				}
				if !errors.Is(err, tc.sentinel) {
					t.Errorf("%s status %d: errors.Is(err, %v) = false; err = %v",
						s.op, tc.status, tc.sentinel, err)
				}
			})
		}
	}
}

// TestTransport_NewTrimsTrailingSlash: New() strips a trailing slash from
// BaseURL so the URL builders don't emit `//api`. Replaces the deleted
// TestResolverClient_NewTrimsTrailingSlash at the provider-construction layer.
func TestTransport_NewTrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	p := transportProvider(t, Deps{BaseURL: "https://animepahe.pw/"})
	if p.baseURL != "https://animepahe.pw" {
		t.Errorf("baseURL = %q; want trailing slash trimmed", p.baseURL)
	}
}

// TestTransport_EmptyBaseURLDefaults: an empty BaseURL falls back to
// defaultBaseURL (https://animepahe.pw) — pins the New() default branch.
func TestTransport_EmptyBaseURLDefaults(t *testing.T) {
	t.Parallel()
	p := transportProvider(t, Deps{BaseURL: ""})
	if p.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q; want %q (defaultBaseURL)", p.baseURL, defaultBaseURL)
	}
}

// TestTransport_BrowserPath: when UseBrowser()==true and BrowserFetch is wired,
// the transport routes the GET through the sidecar closure instead of plain
// HTTP. Asserts (a) the parsed result, (b) the closure received the canonical
// /api?m=search... URL, and (c) the provider string passed to the closure is
// "animepahe". A live BaseURL is supplied but the closure short-circuits it, so
// the httptest server is never hit on this path.
func TestTransport_BrowserPath(t *testing.T) {
	t.Parallel()

	// The httptest server MUST NOT be reached on the browser path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("plain HTTP must not be called on the browser path; got %s", r.URL)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	const body = `{"total":1,"data":[{"id":7,"session":"browser-session-001","title":"Frieren"}]}`
	var gotProvider, gotURL string
	var fetchCalls int
	fetch := func(ctx context.Context, provider, urlStr string) (int, []byte, error) {
		fetchCalls++
		gotProvider = provider
		gotURL = urlStr
		return http.StatusOK, []byte(body), nil
	}

	p := transportProvider(t, Deps{
		BaseURL:      srv.URL,
		UseBrowser:   func() bool { return true },
		BrowserFetch: fetch,
	})

	sr, err := p.Search(context.Background(), "Frieren")
	if err != nil {
		t.Fatalf("Search (browser) err = %v; want nil", err)
	}
	if len(sr.Data) != 1 || sr.Data[0].Session != "browser-session-001" {
		t.Fatalf("sr.Data = %+v; want one entry session browser-session-001", sr.Data)
	}
	if fetchCalls != 1 {
		t.Errorf("BrowserFetch calls = %d; want 1", fetchCalls)
	}
	if gotProvider != providerName {
		t.Errorf("BrowserFetch provider = %q; want %q", gotProvider, providerName)
	}
	// URL must be the canonical search shape against the live BaseURL.
	wantURL := srv.URL + "/api?m=search&q=Frieren"
	if gotURL != wantURL {
		t.Errorf("BrowserFetch url = %q; want %q", gotURL, wantURL)
	}
	if !strings.Contains(gotURL, "/api?m=search") {
		t.Errorf("BrowserFetch url = %q; want it to contain /api?m=search", gotURL)
	}
}

// TestTransport_BrowserPath_StatusMapped: a non-200 status RETURNED by the
// browser closure (the body still came back, e.g. a 403 challenge page) is run
// through mapStatus so the orchestrator fails over. Pins that the browser path
// maps status the same way the plain-HTTP path does.
func TestTransport_BrowserPath_StatusMapped(t *testing.T) {
	t.Parallel()
	fetch := func(ctx context.Context, provider, urlStr string) (int, []byte, error) {
		return http.StatusForbidden, []byte(`<html>Just a moment...</html>`), nil
	}
	p := transportProvider(t, Deps{
		BaseURL:      "https://animepahe.pw",
		UseBrowser:   func() bool { return true },
		BrowserFetch: fetch,
	})
	_, err := p.Release(context.Background(), "abc-uuid", 1)
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("browser 403 err = %v; want ErrProviderDown", err)
	}
}

// TestTransport_BrowserEnabledGate: browserEnabled() requires BOTH the live
// gate AND the fetch closure. A partial wiring degrades to the plain-HTTP
// fallback rather than panicking on a nil closure.
func TestTransport_BrowserEnabledGate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		useBrowser func() bool
		fetch      BrowserFetchFunc
		want       bool
	}{
		{"both wired, gate true", func() bool { return true }, func(context.Context, string, string) (int, []byte, error) { return 200, nil, nil }, true},
		{"both wired, gate false", func() bool { return false }, func(context.Context, string, string) (int, []byte, error) { return 200, nil, nil }, false},
		{"gate nil", nil, func(context.Context, string, string) (int, []byte, error) { return 200, nil, nil }, false},
		{"fetch nil", func() bool { return true }, nil, false},
		{"both nil", nil, nil, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := transportProvider(t, Deps{
				BaseURL:      "https://animepahe.pw",
				UseBrowser:   tc.useBrowser,
				BrowserFetch: tc.fetch,
			})
			if got := p.browserEnabled(); got != tc.want {
				t.Errorf("browserEnabled() = %v; want %v", got, tc.want)
			}
		})
	}
}
