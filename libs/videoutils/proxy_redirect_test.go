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
)

// extractProxiedChildTargets decodes every rewritten child URL in a proxied
// manifest body back to its absolute upstream target. Handles both rewrite
// forms: the Track A masked path (/api/streaming/m/<token>/<leaf>) and the
// legacy signed query form (/api/streaming/hls-proxy?url=...&exp=...&sig=...).
func extractProxiedChildTargets(t *testing.T, manifest string) []string {
	t.Helper()
	var targets []string
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "/api/streaming/m/"):
			tok := strings.TrimPrefix(line, "/api/streaming/m/")
			if i := strings.Index(tok, "/"); i != -1 {
				tok = tok[:i]
			}
			p, err := DecodeStreamToken(tok, time.Now())
			if assert.NoError(t, err, "rewritten child's masked token must decode") {
				targets = append(targets, p.URL)
			}
		case strings.HasPrefix(line, "/api/streaming/hls-proxy?"):
			q, err := url.ParseQuery(strings.TrimPrefix(line, "/api/streaming/hls-proxy?"))
			if assert.NoError(t, err) {
				assert.NotEmpty(t, q.Get("exp"), "legacy-form child must carry exp")
				assert.NotEmpty(t, q.Get("sig"), "legacy-form child must carry sig")
				targets = append(targets, q.Get("url"))
			}
		}
	}
	return targets
}

// TestManifestRewrite_RedirectedMasterBasesChildrenOnFinalURL is the S1
// regression test for AUTO-517: when the master playlist request 302s to a
// DIFFERENT host (e.g. a signed ultracloud.cc master redirecting to its
// inner-embed CDN), the manifest's relative children must be rebased onto the
// POST-redirect final URL (the RFC-correct HLS base URI) and carry a
// freshly-minted masked token / provenance signature for that host — not
// resolve against the pre-redirect source host, which re-gates them bare.
func TestManifestRewrite_RedirectedMasterBasesChildrenOnFinalURL(t *testing.T) {
	// Final host: serves the real master with a relative child.
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/real/master.m3u8" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\nseg-1.ts\n"))
	}))
	defer final.Close()

	// Origin host: 302s the master request to the final host.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/real/master.m3u8", http.StatusFound)
	}))
	defer origin.Close()

	sourceURL := origin.URL + "/redir/master.m3u8"
	exp, sig := signProvenance(sourceURL, time.Now())

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err)

	body := rec.Body.String()
	targets := extractProxiedChildTargets(t, body)
	if assert.Len(t, targets, 1, "the single relative child must be rewritten to a proxied URL") {
		assert.Equal(t, final.URL+"/real/seg-1.ts", targets[0],
			"redirected master's relative child must resolve against the FINAL (post-redirect) URL")
	}
	assert.NotContains(t, body, origin.URL,
		"no child may be rebased onto the pre-redirect origin host")
}

// TestManifestRewrite_NoRedirectKeepsSourceBase pins the non-redirect
// behavior: with no upstream redirect, children keep resolving against the
// original source URL (rewriteBase == sourceURL).
func TestManifestRewrite_NoRedirectKeepsSourceBase(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:6.0,\nseg-1.ts\n"))
	}))
	defer upstream.Close()

	sourceURL := upstream.URL + "/path/master.m3u8"
	exp, sig := signProvenance(sourceURL, time.Now())

	proxy := NewVideoProxy(ProxyConfig{UserAgent: "test-agent"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/hls-proxy?url="+url.QueryEscape(sourceURL)+"&exp="+exp+"&sig="+sig, nil)

	_, _, err := proxy.ProxyWithRefererCounted(context.Background(), sourceURL, "", rec, req)
	assert.NoError(t, err)

	targets := extractProxiedChildTargets(t, rec.Body.String())
	if assert.Len(t, targets, 1) {
		assert.Equal(t, upstream.URL+"/path/seg-1.ts", targets[0],
			"non-redirected master's child must keep the source URL base")
	}
}
