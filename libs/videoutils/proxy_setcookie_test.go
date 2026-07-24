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

// TestProxy_DropsUpstreamSetCookie is the regression test for F28 (CWE-644):
// the HLS proxy relays an untrusted third-party media host through the
// first-party origin. It must NOT copy the upstream's Set-Cookie/Set-Cookie2
// to the client — otherwise a hostile/compromised CDN could plant or overwrite
// cookies (e.g. refresh_token/access_token) on animeenigma.org (session
// fixation / forced logout). Media segments/manifests never legitimately set
// app cookies.
//
// The non-rewrite (segment passthrough) branch is exercised — that is the copy
// loop this patch hardens. (The M3U8/VTT rewrite branch's Set-Cookie strip is
// owned by writeRewrittenText and a sibling patch.)
func TestProxy_DropsUpstreamSetCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A hostile upstream tries to plant a first-party session cookie.
		w.Header().Set("Set-Cookie", "refresh_token=attacker; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "access_token=attacker")
		w.Header().Set("Set-Cookie2", "legacy=attacker")
		// A benign header that MUST still be relayed, proving the loop still copies.
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("\x47\x40\x00\x10binarysegmentbytes"))
	}))
	defer upstream.Close()

	sourceURL := upstream.URL + "/seg-0.ts"
	// Authorize the otherwise-unlisted httptest host via a provenance token.
	exp, sig := signProvenance(sourceURL, time.Now())

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err)

	res := rec.Result()
	assert.Empty(t, res.Header.Values("Set-Cookie"),
		"upstream Set-Cookie must NOT be relayed to the client on the first-party origin")
	assert.Empty(t, res.Header.Values("Set-Cookie2"),
		"upstream Set-Cookie2 must NOT be relayed to the client on the first-party origin")
	// Sanity: the copy loop still relays legitimate non-media-cookie headers.
	assert.Equal(t, "max-age=60", res.Header.Get("Cache-Control"),
		"benign upstream headers must still be copied through")
}
