package videoutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestProxy_DropsDuplicateUpstreamCORSHeader is the regression test for the
// CORS double-header bug: the proxy sets Access-Control-Allow-Origin itself, and
// must NOT also copy an upstream ACAO (the stealth-scraper /hls sidecar always
// sends ACAO:*). Two ACAO values are a CORS failure in browsers and silently
// broke the gogoanime/megaplay browser path under the cross-origin stream.* base.
//
// Both the M3U8-rewrite branch and the segment-passthrough branch copy upstream
// headers, so both are exercised.
func TestProxy_DropsDuplicateUpstreamCORSHeader(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
	}{
		{"m3u8 branch", "application/vnd.apple.mpegurl", "#EXTM3U\n#EXTINF:6.0,\nseg-0.ts\n"},
		{"segment branch", "video/mp2t", "\x47\x40\x00\x10binarysegmentbytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// Upstream (like the sidecar) emits its own permissive CORS header.
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			sourceURL := upstream.URL + "/playlist.m3u8"
			if tc.name == "segment branch" {
				sourceURL = upstream.URL + "/seg-0.ts"
			}
			// Authorize the otherwise-unlisted httptest host via a provenance token.
			exp, sig := signProvenance(sourceURL, time.Now())

			proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

			_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
			assert.NoError(t, err)

			acao := rec.Result().Header.Values("Access-Control-Allow-Origin")
			assert.Equal(t, []string{"*"}, acao,
				"client must see exactly one Access-Control-Allow-Origin (proxy's own, not duplicated from upstream)")
		})
	}
}

// TestIsProxySetCORSHeader locks the small skip-set the copy loops consult.
func TestIsProxySetCORSHeader(t *testing.T) {
	for _, k := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Expose-Headers",
	} {
		assert.True(t, isProxySetCORSHeader(k), k+" must be treated as proxy-managed")
	}
	for _, k := range []string{"Content-Type", "Cache-Control", "Etag", "Accept-Ranges"} {
		assert.False(t, isProxySetCORSHeader(k), k+" must be copied through from upstream")
	}
}
