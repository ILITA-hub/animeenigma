// vidstream_vip_test.go — tests for VidstreamVipExtractor.
//
// Phase 28 SCRAPER-HEAL-38. The extractor is a plain-regex puller against
// the inline `sources: [{"file":"...m3u8"}]` literal in am.vidstream.vip
// embed pages (AnimeFever upstream). Tests use the package-private
// rewriteToSrv RoundTripper (defined in packed_common_test.go) so
// Matches() stays strict against the real allowlisted host while the
// underlying TCP socket points at a local httptest server.
package embeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// TestVidstreamVip_Name pins the stable identifier emitted in logs / metrics.
func TestVidstreamVip_Name(t *testing.T) {
	t.Parallel()
	if got := NewVidstreamVipExtractor().Name(); got != "vidstream_vip" {
		t.Errorf("Name() = %q; want %q", got, "vidstream_vip")
	}
}

// TestVidstreamVip_Matches_PositiveAndNegative covers Matches T-28-03-05:
// host equality OR strict subdomain only — no substring / suffix attack.
func TestVidstreamVip_Matches_PositiveAndNegative(t *testing.T) {
	t.Parallel()
	e := NewVidstreamVipExtractor()
	cases := []struct {
		url  string
		want bool
	}{
		// Positive — exact + strict subdomain + case insensitive.
		{"https://am.vidstream.vip/?x=y", true},
		{"https://vidstream.vip/foo", true},
		{"HTTPS://AM.VIDSTREAM.VIP/?z=1", true},
		{"http://am.vidstream.vip/x", true},
		{"https://edge.am.vidstream.vip/x", true},
		// Negative — unrelated hosts.
		{"https://kwik.cx/e/123", false},
		{"https://otakuhg.site/foo", false},
		{"https://animefever.cc/watch/xyz", false},
		// Negative — suffix attack guards.
		{"https://am.vidstream.vip.evil.com/x", false},
		{"https://vidstream.vip.evil.com/x", false},
		{"https://evilvidstream.vip/x", false},
		// Negative — wrong scheme / malformed.
		{"ftp://am.vidstream.vip/x", false},
		{"file:///am.vidstream.vip/passwd", false},
		{"https:///x", false},
		{"", false},
		{"://no-scheme", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.url, func(t *testing.T) {
			t.Parallel()
			if got := e.Matches(c.url); got != c.want {
				t.Errorf("Matches(%q) = %v; want %v", c.url, got, c.want)
			}
		})
	}
}

// vidstreamVipGolden resolves the Frieren ep28 fixture committed alongside
// the extractor in services/scraper/internal/embeds/testdata/.
func vidstreamVipGolden(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "vidstream_vip_frieren.html")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("golden missing: %v", err)
	}
	return p
}

// TestVidstreamVip_Extract_Success drives Extract against the captured
// offline fixture and asserts shape: one HLS Source + Referer header.
func TestVidstreamVip_Extract_Success(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(vidstreamVipGolden(t))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	hdr := http.Header{}
	hdr.Set("Referer", "https://animefever.cc/")
	stream, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/?embed=frieren-ep28",
		hdr,
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if stream == nil || len(stream.Sources) != 1 {
		t.Fatalf("Extract returned unexpected stream: %+v", stream)
	}
	got := stream.Sources[0]
	if !strings.HasSuffix(got.URL, ".m3u8") {
		t.Errorf("Sources[0].URL = %q; want .m3u8 suffix", got.URL)
	}
	if !strings.Contains(got.URL, "static-cdn-ca1.mofl.pro") {
		t.Errorf("Sources[0].URL = %q; want host static-cdn-ca1.mofl.pro", got.URL)
	}
	if got.Type != "hls" {
		t.Errorf("Sources[0].Type = %q; want hls", got.Type)
	}
	if got.Quality != "HD" {
		t.Errorf("Sources[0].Quality = %q; want HD", got.Quality)
	}
	if stream.Headers["Referer"] != "https://am.vidstream.vip/" {
		t.Errorf("Headers[Referer] = %q; want https://am.vidstream.vip/",
			stream.Headers["Referer"])
	}
}

// TestVidstreamVip_Extract_NoSourcesLiteral — body returned 200 but has no
// `sources: [...]` literal. Must wrap ErrExtractFailed (not ErrProviderDown
// — the server answered).
func TestVidstreamVip_Extract_NoSourcesLiteral(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html>nothing useful here</html>`))
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/embed-missing",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is ErrExtractFailed", err)
	}
}

// TestVidstreamVip_Extract_Upstream5xx — upstream 503 must wrap ErrProviderDown.
func TestVidstreamVip_Extract_Upstream5xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/embed-down",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrProviderDown")
	}
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Errorf("err = %v; want errors.Is ErrProviderDown", err)
	}
}

// TestVidstreamVip_Extract_Upstream4xx — upstream 404 must wrap ErrExtractFailed
// (the server answered with a non-2xx that isn't a transport failure).
func TestVidstreamVip_Extract_Upstream4xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/embed-notfound",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is ErrExtractFailed", err)
	}
}

// TestVidstreamVip_Extract_MalformedJSON — regex matches but the captured
// `{...}` body fails json.Unmarshal → ErrExtractFailed.
func TestVidstreamVip_Extract_MalformedJSON(t *testing.T) {
	t.Parallel()
	body := []byte(`<html><script>
sources: [{not_valid_json: true, missing_quotes}]
</script></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/embed-malformed",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is ErrExtractFailed", err)
	}
}

// TestVidstreamVip_Extract_NonAbsoluteURL — regex + JSON ok, but `file`
// doesn't start with http(s). Must wrap ErrExtractFailed.
func TestVidstreamVip_Extract_NonAbsoluteURL(t *testing.T) {
	t.Parallel()
	body := []byte(`<html><script>
sources: [{"file":"/relative/master.m3u8","type":"mp4","label":"HD"}]
</script></html>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	e := NewVidstreamVipExtractor()
	e.http = &http.Client{
		Transport: &rewriteToSrv{srvURL: srv.URL},
		Timeout:   5 * time.Second,
	}
	_, err := e.Extract(
		context.Background(),
		"https://am.vidstream.vip/embed-relative",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract: error = nil; want wrapped ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is ErrExtractFailed", err)
	}
}

// TestVidstreamVip_Extract_HostGate — Matches() refuses the URL up front;
// Extract MUST fail closed (the SSRF gate is a hard precondition).
func TestVidstreamVip_Extract_HostGate(t *testing.T) {
	t.Parallel()
	e := NewVidstreamVipExtractor()
	_, err := e.Extract(
		context.Background(),
		"https://kwik.cx/e/123",
		http.Header{},
	)
	if err == nil {
		t.Fatal("Extract on non-matching host returned nil error; want ErrExtractFailed")
	}
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Errorf("err = %v; want errors.Is ErrExtractFailed", err)
	}
}

// Compile-time assertion: VidstreamVipExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*VidstreamVipExtractor)(nil)
