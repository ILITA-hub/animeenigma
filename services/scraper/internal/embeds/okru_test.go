package embeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realistic ok.ru /videoembed page: a <div data-options="{...escaped json...}">
// whose flashvars.metadata is a JSON-encoded STRING carrying hlsManifestUrl +
// videos[]. This mirrors the live shape captured 2026-06-22.
const okruEmbedHTML = `<!DOCTYPE html><html><body>` +
	`<div data-module="OKVideo" data-options="{&quot;flashvars&quot;:{&quot;metadata&quot;:&quot;{\&quot;hlsManifestUrl\&quot;:\&quot;https://vd1.okcdn.ru/video.m3u8?x=1\&quot;,\&quot;videos\&quot;:[{\&quot;name\&quot;:\&quot;hd\&quot;,\&quot;url\&quot;:\&quot;https://vd1.okcdn.ru/hd.mp4\&quot;}]}&quot;}}"></div>` +
	`</body></html>`

func TestOkru_Matches(t *testing.T) {
	e := NewOkruExtractor()
	for _, u := range []string{"https://ok.ru/videoembed/123", "https://m.ok.ru/videoembed/9"} {
		if !e.Matches(u) {
			t.Errorf("Matches(%q) = false, want true", u)
		}
	}
	for _, u := range []string{"https://evil.com/ok.ru", "https://notok.ru/x", "ftp://ok.ru/x"} {
		if e.Matches(u) {
			t.Errorf("Matches(%q) = true, want false", u)
		}
	}
}

func TestOkru_Extract_HLSAndMP4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, okruEmbedHTML)
	}))
	defer srv.Close()
	e := NewOkruExtractor()
	// point Matches-bypass: Extract calls Matches first, so use a host it accepts
	// by overriding the fetch URL via the test server while keeping ok.ru host.
	// Simplest: temporarily allow the test server host.
	e.allowTestHost(strings.TrimPrefix(srv.URL, "http://"))
	st, err := e.Extract(context.Background(), srv.URL+"/videoembed/1", nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(st.Sources) == 0 || st.Sources[0].Type != "hls" {
		t.Fatalf("want HLS first source, got %+v", st.Sources)
	}
	if st.Sources[0].URL != "https://vd1.okcdn.ru/video.m3u8?x=1" {
		t.Errorf("hls url = %q", st.Sources[0].URL)
	}
	if st.Headers["Referer"] != "https://ok.ru/" {
		t.Errorf("referer = %q", st.Headers["Referer"])
	}
}
