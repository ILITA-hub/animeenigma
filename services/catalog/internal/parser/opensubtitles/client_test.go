package opensubtitles

import (
	"context"
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
