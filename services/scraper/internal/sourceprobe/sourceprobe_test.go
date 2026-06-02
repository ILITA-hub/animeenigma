package sourceprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func testClient(t *testing.T) *domain.BaseHTTPClient {
	t.Helper()
	return domain.NewBaseHTTPClient(logger.Default())
}

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
		want        Kind
	}{
		{"hls content-type", "application/vnd.apple.mpegurl", "#EXTM3U\n#EXT-X-VERSION:3\n", 200, Stream},
		{"hls body as text/plain", "text/plain", "#EXTM3U\n#EXTINF:5,\nseg.ts\n", 200, Stream},
		{"hls body with BOM", "application/octet-stream", "\xEF\xBB\xBF#EXTM3U\n", 200, Stream},
		{"embed text/html", "text/html; charset=utf-8", "<!doctype html><html><body>player</body></html>", 200, Embed},
		{"embed html body, octet-stream ct", "application/octet-stream", "<html><head></head></html>", 200, Embed},
		{"direct mp4 video/*", "video/mp4", "\x00\x00\x00\x18ftypmp42binarygarbage", 200, Stream},
		{"direct mp4 octet-stream", "application/octet-stream", "\x00\x00\x00\x18ftypisombinary", 200, Stream},
		{"error status", "text/html", "nope", 404, Unknown},
		{"ambiguous content", "application/json", `{"err":"x"}`, 200, Unknown},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got := Classify(context.Background(), testClient(t), srv.URL, "https://ref.example/")
			if got != tc.want {
				t.Errorf("Classify(%s) = %v; want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestClassify_NilClientOrEmptyURL(t *testing.T) {
	t.Parallel()
	if got := Classify(context.Background(), nil, "https://x/", ""); got != Unknown {
		t.Errorf("nil client = %v; want Unknown", got)
	}
	if got := Classify(context.Background(), testClient(t), "   ", ""); got != Unknown {
		t.Errorf("empty url = %v; want Unknown", got)
	}
}

func TestClassify_ForwardsReferer(t *testing.T) {
	t.Parallel()
	var gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U"))
	}))
	defer srv.Close()

	Classify(context.Background(), testClient(t), srv.URL, "https://allmanga.to/")
	if gotRef != "https://allmanga.to/" {
		t.Errorf("Referer forwarded = %q; want https://allmanga.to/", gotRef)
	}
}
