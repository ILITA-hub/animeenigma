package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// TestProxyToStreamingBody_DropsUpstreamSetCookie is the regression test for
// F29 (CWE-113): the public streaming body routes (/api/streaming/hls-proxy,
// /m/*, /stream/*) proxy an untrusted upstream media host through the
// first-party origin. proxyStream must NOT relay the upstream's Set-Cookie/
// Set-Cookie2 — otherwise a hostile/compromised upstream could plant or
// overwrite cookies (e.g. refresh_token/access_token) on animeenigma.org.
func TestProxyToStreamingBody_DropsUpstreamSetCookie(t *testing.T) {
	t.Parallel()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "refresh_token=attacker; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "access_token=attacker")
		w.Header().Set("Set-Cookie2", "legacy=attacker")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#EXTM3U\n"))
	}))
	defer backend.Close()

	proxySvc := service.NewProxyService(config.ServiceURLs{
		StreamingService: backend.URL,
	}, logger.Default())
	h := NewProxyHandler(proxySvc, logger.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/streaming/hls-proxy", nil)
	h.ProxyToStreamingBody(rec, req)

	res := rec.Result()
	if got := res.Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("Set-Cookie relayed to client on streaming path = %v; want none", got)
	}
	if got := res.Header.Values("Set-Cookie2"); len(got) != 0 {
		t.Errorf("Set-Cookie2 relayed to client on streaming path = %v; want none", got)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "#EXTM3U\n" {
		t.Errorf("streamed body/status corrupted: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestStripUpstreamSetCookie_ScopedToNonAuth locks the auth exemption: the
// strip must remove Set-Cookie/Set-Cookie2 for the streaming media services but
// leave them intact for auth, whose login/refresh responses legitimately set
// refresh_token/access_token cookies the browser must receive.
func TestStripUpstreamSetCookie_ScopedToNonAuth(t *testing.T) {
	t.Parallel()

	newResp := func() *http.Response {
		h := http.Header{}
		h.Set("Set-Cookie", "refresh_token=v; Path=/; HttpOnly")
		h.Set("Set-Cookie2", "legacy=v")
		return &http.Response{Header: h}
	}

	// Non-auth (streaming): cookie headers are stripped.
	streamingResp := newResp()
	stripUpstreamSetCookie(streamingResp, "streaming")
	if got := streamingResp.Header.Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("streaming Set-Cookie = %v; want stripped", got)
	}
	if got := streamingResp.Header.Values("Set-Cookie2"); len(got) != 0 {
		t.Errorf("streaming Set-Cookie2 = %v; want stripped", got)
	}

	// auth: cookie headers survive so login/refresh still set session cookies.
	authResp := newResp()
	stripUpstreamSetCookie(authResp, "auth")
	if got := authResp.Header.Get("Set-Cookie"); got != "refresh_token=v; Path=/; HttpOnly" {
		t.Errorf("auth Set-Cookie = %q; want preserved", got)
	}
	if got := authResp.Header.Get("Set-Cookie2"); got != "legacy=v" {
		t.Errorf("auth Set-Cookie2 = %q; want preserved", got)
	}
}
