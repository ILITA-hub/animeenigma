package allanime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newMockClient builds a Client whose only configured domain points at the
// given httptest server. The mock server URL is "http://127.0.0.1:PORT" but
// our client constructs URLs as "https://api.{domain}/api", so we need to
// rewrite the transport to round-trip everything to the mock.
func newMockClient(t *testing.T, mock *httptest.Server) *Client {
	t.Helper()

	c := NewClient(Config{
		Domains:          []string{"mock.test"},
		QuerySearchSHA:   "test-search-sha",
		QueryEpisodesSHA: "test-episodes-sha",
		QuerySourcesSHA:  "test-sources-sha",
		HTTPTimeout:      2 * time.Second,
		Referer:          "https://test/",
		UserAgent:        "test-agent",
	})

	mockURL, _ := url.Parse(mock.URL)
	c.httpClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: &rewriteTransport{
			to:   mockURL.Host,
			base: http.DefaultTransport,
		},
	}
	return c
}

// rewriteTransport rewrites every request's URL host to the mock server.
type rewriteTransport struct {
	to   string
	base http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = rt.to
	return rt.base.RoundTrip(req)
}

func TestSearch_ReturnsParsedResults(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return one show.
		fmt.Fprint(w, `{
			"data": {
				"shows": {
					"edges": [{
						"_id": "abc123",
						"name": "Bocchi the Rock!",
						"englishName": "Bocchi the Rock!",
						"nativeName": "ぼっち・ざ・ろっく！",
						"thumbnail": "/poster.jpg",
						"availableEpisodes": {"raw": 12}
					}]
				}
			}
		}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	got, err := c.Search(context.Background(), "bocchi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 result, got %d", len(got))
	}
	if got[0].ID != "abc123" {
		t.Errorf("ID = %q, want abc123", got[0].ID)
	}
	if got[0].Episodes != 12 {
		t.Errorf("Episodes = %d, want 12", got[0].Episodes)
	}
	if got[0].JName != "ぼっち・ざ・ろっく！" {
		t.Errorf("JName = %q, want native", got[0].JName)
	}
}

func TestSearch_EmptyQueryReturnsError(t *testing.T) {
	c := NewClient(Config{Domains: []string{"x"}})
	_, err := c.Search(context.Background(), "   ")
	if err == nil {
		t.Fatal("want error on empty query")
	}
}

func TestSearch_NoMatchReturnsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data": {"shows": {"edges": []}}}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, err := c.Search(context.Background(), "nonsense")
	if err == nil || !strings.Contains(err.Error(), "no match") {
		t.Fatalf("want 'no match' error, got %v", err)
	}
}

func TestSearch_GraphQLErrorWrapped(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data": null, "errors": [{"message":"PersistedQueryNotFound"}]}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, err := c.Search(context.Background(), "bocchi")
	if err == nil || !strings.Contains(err.Error(), "stale SHA") {
		t.Fatalf("want stale-SHA error, got %v", err)
	}
}

func TestSearch_4xxNotMarkedAsDomainFailure(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"errors": [{"message":"bad"}]}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, _ = c.Search(context.Background(), "x")

	// The domain should still be cached as active — 4xx is not a transport failure.
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.failedAt.IsZero() {
		t.Error("domain marked failed on 4xx; should only mark on transport/5xx")
	}
}

func TestSearch_5xxMarksDomainFailed(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, _ = c.Search(context.Background(), "x")

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.failedAt.IsZero() {
		t.Error("domain not marked failed on 5xx")
	}
}

func TestEpisodesByID_ReturnsSortedEpisodes(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"data": {
				"show": {
					"_id": "abc123",
					"availableEpisodesDetail": {
						"raw": ["3", "1", "12", "2"]
					}
				}
			}
		}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	eps, err := c.EpisodesByID(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eps) != 4 {
		t.Fatalf("want 4 episodes, got %d", len(eps))
	}
	// Should be sorted 1, 2, 3, 12.
	wantNums := []int{1, 2, 3, 12}
	for i, w := range wantNums {
		if eps[i].Number != w {
			t.Errorf("eps[%d].Number = %d, want %d", i, eps[i].Number, w)
		}
	}
	// IDs are composite "showID/epStr".
	if !strings.HasPrefix(eps[0].ID, "abc123/") {
		t.Errorf("eps[0].ID = %q, want prefix abc123/", eps[0].ID)
	}
}

func TestEpisodesByID_EmptyRawListReturnsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"show":{"_id":"x","availableEpisodesDetail":{"raw":[]}}}}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, err := c.EpisodesByID(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "no raw episodes") {
		t.Fatalf("want 'no raw episodes' error, got %v", err)
	}
}

func TestRawStream_PicksHighestPriorityHTTPSource(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"data": {
				"episode": {
					"sourceUrls": [
						{"sourceUrl": "internal:obfuscated", "priority": 9, "type": "iframe"},
						{"sourceUrl": "https://stream.example/playlist.m3u8", "priority": 5, "type": "hls", "subtitles": [{"src":"https://subs/jp.vtt","lang":"ja","label":"Japanese"}]},
						{"sourceUrl": "https://cdn.example/v.mp4", "priority": 3, "type": "mp4"}
					]
				}
			}
		}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	s, err := c.RawStream(context.Background(), "abc123/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.URL != "https://stream.example/playlist.m3u8" {
		t.Errorf("URL = %q, want playlist.m3u8 (highest-priority http source)", s.URL)
	}
	if s.Type != "hls" {
		t.Errorf("Type = %q, want hls", s.Type)
	}
	if len(s.Subtitles) != 1 || s.Subtitles[0].Lang != "ja" {
		t.Errorf("subtitles not parsed: %+v", s.Subtitles)
	}
}

func TestRawStream_InvalidEpisodeID(t *testing.T) {
	c := NewClient(Config{Domains: []string{"x"}})
	_, err := c.RawStream(context.Background(), "no-slash")
	if err == nil || !strings.Contains(err.Error(), "invalid episode ID") {
		t.Fatalf("want invalid-episode-ID error, got %v", err)
	}
}

func TestRawStream_NoSourcesReturnsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"episode":{"sourceUrls":[]}}}`)
	}))
	defer mock.Close()

	c := newMockClient(t, mock)
	_, err := c.RawStream(context.Background(), "abc/1")
	if err == nil || !strings.Contains(err.Error(), "no sources") {
		t.Fatalf("want 'no sources' error, got %v", err)
	}
}

func TestDomainRotation_FallsBackOnFirstDomainFailure(t *testing.T) {
	// Two mock servers; the rewrite transport only points at one host.
	// To simulate rotation we instead seed two domains in the cfg and use
	// the probe path: we mark the first failed by setting cooldown to 0,
	// have probeDomain accept "ok.test", and assert the active domain
	// becomes "ok.test".

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mock.Close()

	c := NewClient(Config{
		Domains: []string{"unreachable.test", "ok.test"},
	})

	// Repoint the http client at the mock for the probe.
	mockURL, _ := url.Parse(mock.URL)
	c.httpClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: &selectiveTransport{
			match: "ok.test",
			to:    mockURL.Host,
			base:  http.DefaultTransport,
		},
	}

	got, err := c.pickDomain(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok.test" {
		t.Errorf("pickDomain = %q, want ok.test (rotation fallback)", got)
	}
}

// selectiveTransport rewrites only URLs whose host contains `match` to the
// mock; everything else is returned as a connection error to simulate an
// unreachable domain.
type selectiveTransport struct {
	match string
	to    string
	base  http.RoundTripper
}

func (st *selectiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, st.match) {
		req.URL.Scheme = "http"
		req.URL.Host = st.to
		return st.base.RoundTrip(req)
	}
	return nil, fmt.Errorf("simulated unreachable: %s", req.URL.Host)
}

func TestDecodeSourceURL_PassthroughForHTTPS(t *testing.T) {
	got := decodeSourceURL("https://example.com/x.m3u8")
	if got != "https://example.com/x.m3u8" {
		t.Errorf("decodeSourceURL = %q, want passthrough", got)
	}
}

func TestStreamTypeFromURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://x/y.m3u8", "hls"},
		{"https://x/y.mp4", "mp4"},
		{"https://x/y", "hls"},
	}
	for _, tc := range cases {
		if got := streamTypeFromURL(tc.in); got != tc.want {
			t.Errorf("streamTypeFromURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
