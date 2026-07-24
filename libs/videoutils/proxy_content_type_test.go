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

// TestProxy_RewriteBranchDerivesContentType is the regression test for the
// CWE-79 finding: on the M3U8/VTT rewrite branch the proxy used to copy the
// upstream Content-Type verbatim, so a hostile segment/subtitle host could
// serve Content-Type: text/html with a script body and — because the response
// also carries Access-Control-Allow-Origin: * and renders on the app origin —
// get script execution in animeenigma.org (stored/reflected XSS). The gateway's
// global X-Content-Type-Options: nosniff does not help, because the type is
// declared, not sniffed.
//
// The fix skips upstream Content-Type in writeRewrittenText's copy loop (as the
// non-rewrite branch already does) and sets it explicitly from the branch that
// ran: application/vnd.apple.mpegurl for the manifest, text/vtt for the cue
// sheet. This test drives a hostile upstream that declares text/html on both
// branches and asserts the client never sees text/html.
func TestProxy_RewriteBranchDerivesContentType(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		body     string
		wantType string
	}{
		{
			name:     "m3u8 branch",
			path:     "/playlist.m3u8",
			body:     "#EXTM3U\n#EXTINF:6.0,\nseg-0.ts\n",
			wantType: "application/vnd.apple.mpegurl",
		},
		{
			name:     "vtt branch",
			path:     "/storyboard.vtt",
			body:     "WEBVTT\n\n00:00.000 --> 00:05.000\nsprite.jpg#xywh=0,0,160,90\n",
			wantType: "text/vtt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// Hostile upstream: declares text/html (and a Set-Cookie) on a
				// manifest/cue-sheet path so, if relayed, the browser would render
				// the body as HTML/script in the app origin.
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Set-Cookie", "sid=attacker; Path=/")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			sourceURL := upstream.URL + tc.path
			// Authorize the otherwise-unlisted httptest host via a provenance token.
			exp, sig := signProvenance(sourceURL, time.Now())

			proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

			_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
			assert.NoError(t, err)

			ct := rec.Result().Header.Get("Content-Type")
			assert.NotContains(t, ct, "text/html",
				"rewrite branch must never relay the upstream text/html Content-Type (CWE-79)")
			assert.Equal(t, tc.wantType, ct,
				"rewrite branch must set the derived Content-Type so hls.js / SubtitleOverlay accept it")

			// Set-Cookie from a rewritten manifest/cue sheet must not reach the client.
			assert.Empty(t, rec.Result().Header.Values("Set-Cookie"),
				"rewrite branch must not relay upstream Set-Cookie")
		})
	}
}
