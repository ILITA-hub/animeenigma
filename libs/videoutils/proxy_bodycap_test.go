package videoutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProxy_TextBodyCap locks the maxTextBodyBytes bound on the two branches
// that buffer a whole upstream body (M3U8 rewrite, VTT rewrite). The branch is
// chosen by the UPSTREAM's Content-Type, so without the cap one hostile CDN
// response could allocate arbitrary heap in the streaming service.
//
// A body at/below the cap must still be proxied and rewritten exactly as
// before; a body above it must fail the request rather than be buffered.
func TestProxy_TextBodyCap(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		leaf        string
		body        string
		wantErr     bool
	}{
		{
			name:        "m3u8 under cap is rewritten",
			contentType: "application/vnd.apple.mpegurl",
			leaf:        "/playlist.m3u8",
			body:        "#EXTM3U\n#EXTINF:6.0,\nseg-0.ts\n",
		},
		{
			name:        "vtt under cap is rewritten",
			contentType: "text/vtt",
			leaf:        "/storyboard.vtt",
			body:        "WEBVTT\n\n00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90\n",
		},
		{
			name:        "m3u8 over cap is rejected",
			contentType: "application/vnd.apple.mpegurl",
			leaf:        "/playlist.m3u8",
			body:        "#EXTM3U\n" + repeatPast("#EXTINF:6.0,\nseg-0.ts\n", maxTextBodyBytes),
			wantErr:     true,
		},
		{
			name:        "vtt over cap is rejected",
			contentType: "text/vtt",
			leaf:        "/storyboard.vtt",
			body:        "WEBVTT\n" + repeatPast("\n00:00:00.000 --> 00:00:05.000\nstoryboard_001.jpg#xywh=0,0,160,90\n", maxTextBodyBytes),
			wantErr:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr {
				require.Greater(t, len(tc.body), maxTextBodyBytes, "fixture must exceed the cap")
			} else {
				require.LessOrEqual(t, len(tc.body), maxTextBodyBytes, "fixture must fit under the cap")
			}

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			sourceURL := upstream.URL + tc.leaf
			// Authorize the otherwise-unlisted httptest host via a provenance token.
			exp, sig := signProvenance(sourceURL, time.Now())

			proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

			bytesIn, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
			if tc.wantErr {
				require.Error(t, err, "an oversized text body must fail the request, not be buffered")
				assert.Contains(t, err.Error(), "cap")
				assert.Zero(t, bytesIn)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, uint64(len(tc.body)), bytesIn)
			assert.Contains(t, rec.Body.String(), "/api/streaming/",
				"children of an under-cap body must still be rewritten through the proxy")
		})
	}
}

// repeatPast repeats unit until the result is strictly longer than n bytes.
func repeatPast(unit string, n int) string {
	return strings.Repeat(unit, n/len(unit)+2)
}
