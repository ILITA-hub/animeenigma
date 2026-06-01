package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, mock *httptest.Server, apiKey string) *Client {
	t.Helper()
	return NewClient(Config{
		APIKey:    apiKey,
		UserAgent: "test/1.0",
		BaseURL:   mock.URL,
	})
}

func TestSearch_RequiresAPIKey(t *testing.T) {
	c := NewClient(Config{}) // no api key
	_, err := c.Search(context.Background(), SearchParams{Query: "x"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestSearch_BuildsExpectedQuery(t *testing.T) {
	var captured string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.RawQuery
		fmt.Fprint(w, `{"data": []}`)
	}))
	defer mock.Close()

	c := newTestClient(t, mock, "test-key")
	_, _ = c.Search(context.Background(), SearchParams{
		IMDbID:        "tt15302498",
		Languages:     []string{"ja", "en"},
		SeasonNumber:  1,
		EpisodeNumber: 3,
	})

	// imdb_id should have "tt" stripped per the OpenSubtitles API contract.
	if !strings.Contains(captured, "imdb_id=15302498") {
		t.Errorf("captured query missing stripped imdb_id: %q", captured)
	}
	if !strings.Contains(captured, "languages=ja%2Cen") {
		t.Errorf("captured query missing languages: %q", captured)
	}
	if !strings.Contains(captured, "episode_number=3") {
		t.Errorf("captured query missing episode_number: %q", captured)
	}
}

func TestSearch_ParsesResults(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"data": [
				{
					"type": "subtitle",
					"id": "9001",
					"attributes": {
						"language": "ja",
						"release": "Bocchi.S01E01.1080p.WEBRip",
						"download_count": 42,
						"file_extension": "srt",
						"files": [{"file_id": 8765, "file_name": "bocchi.s01e01.srt"}],
						"url": "https://example/sub.srt"
					}
				}
			]
		}`)
	}))
	defer mock.Close()

	c := newTestClient(t, mock, "k")
	got, err := c.Search(context.Background(), SearchParams{Query: "bocchi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].ID != "9001" || got[0].FileID != 8765 {
		t.Errorf("entry IDs wrong: %+v", got[0])
	}
	if got[0].Language != "ja" {
		t.Errorf("Language = %q, want ja", got[0].Language)
	}
	if got[0].DownloadCount != 42 {
		t.Errorf("DownloadCount = %d, want 42", got[0].DownloadCount)
	}
	if got[0].Format != "srt" {
		t.Errorf("Format = %q, want srt", got[0].Format)
	}
}

func TestSearch_AuthErrors(t *testing.T) {
	cases := []int{http.StatusUnauthorized, http.StatusForbidden}
	for _, status := range cases {
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		c := newTestClient(t, mock, "bad-key")
		_, err := c.Search(context.Background(), SearchParams{Query: "x"})
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("status %d → want ErrUnauthorized, got %v", status, err)
		}
		mock.Close()
	}
}

func TestSearch_RateLimitedByStatus(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer mock.Close()
	c := newTestClient(t, mock, "k")
	_, err := c.Search(context.Background(), SearchParams{Query: "x"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestSearch_RateLimitedByBody(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errors":["Reached download limit"]}`)
	}))
	defer mock.Close()
	c := newTestClient(t, mock, "k")
	_, err := c.Search(context.Background(), SearchParams{Query: "x"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestSearch_5xxReturnsUpstreamError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock.Close()
	c := newTestClient(t, mock, "k")
	_, err := c.Search(context.Background(), SearchParams{Query: "x"})
	if err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("want upstream error, got %v", err)
	}
}

func TestNormalizeLang(t *testing.T) {
	cases := []struct{ in, want string }{
		{"japanese", "ja"},
		{"jpn", "ja"},
		{"JA", "ja"},
		{"english", "en"},
		{"rus", "ru"},
		{"xx", "xx"},
		{"unknown-long", "unknown-long"},
	}
	for _, tc := range cases {
		if got := normalizeLang(tc.in); got != tc.want {
			t.Errorf("normalizeLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatFromFilename(t *testing.T) {
	if formatFromFilename("x.ass") != "ass" {
		t.Error("ass")
	}
	if formatFromFilename("x.SRT") != "srt" {
		t.Error("srt")
	}
	if formatFromFilename("x") != "" {
		t.Error("empty fallback")
	}
}

func TestClient_Download_Success(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/download":
			if r.Header.Get("Api-Key") != "k" {
				t.Errorf("missing api key header")
			}
			var body struct {
				FileID int `json:"file_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.FileID != 42 {
				t.Errorf("file_id = %d, want 42", body.FileID)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"link":%q,"file_name":"ep.srt","remaining":99}`, srv.URL+"/file")
		case r.URL.Path == "/file":
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{APIKey: "k", BaseURL: srv.URL})
	body, name, err := c.Download(context.Background(), 42)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if name != "ep.srt" {
		t.Errorf("name = %q, want ep.srt", name)
	}
	if !strings.Contains(string(body), "hi") {
		t.Errorf("body = %q, want subtitle text", string(body))
	}
}

func TestClient_Download_QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"You have reached download limit","remaining":0}`))
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "k", BaseURL: srv.URL})
	_, _, err := c.Download(context.Background(), 7)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestClient_Download_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient(Config{APIKey: "bad", BaseURL: srv.URL})
	_, _, err := c.Download(context.Background(), 7)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
