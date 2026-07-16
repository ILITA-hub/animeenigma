package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestProxiedURL(t *testing.T) {
	got := ProxiedURL("https://gw.example/", "https://cdn.example/video.m3u8", "123", "sig1", "https://ref.example/")
	want := "https://gw.example" + publicProxyPath + "?" + url.Values{
		"exp":     {"123"},
		"referer": {"https://ref.example/"},
		"sig":     {"sig1"},
		"url":     {"https://cdn.example/video.m3u8"},
	}.Encode()
	if got != want {
		t.Fatalf("raw CDN url:\n got  %s\n want %s", got, want)
	}

	// Already-proxied /api/v1/ path is only re-based onto the gateway.
	got2 := ProxiedURL("https://gw.example", "/api/v1/hls-proxy?url=abc&sig=xyz", "", "", "")
	want2 := "https://gw.example/api/streaming/hls-proxy?url=abc&sig=xyz"
	if got2 != want2 {
		t.Fatalf("already-proxied passthrough:\n got  %s\n want %s", got2, want2)
	}

	// Already-proxied /api/streaming/ path passes through unmodified (module the base).
	got3 := ProxiedURL("https://gw.example", "/api/streaming/hls-proxy?url=abc", "", "", "")
	want3 := "https://gw.example/api/streaming/hls-proxy?url=abc"
	if got3 != want3 {
		t.Fatalf("already gateway-shaped passthrough:\n got  %s\n want %s", got3, want3)
	}
}

func TestLocalizeHLS(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\nv.m3u8\n"))
	})
	mux.HandleFunc("/v.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Join([]string{
			"#EXTM3U",
			// root-absolute /api/v1/ URI: LocalizeHLS must remap it onto
			// /api/streaming/ on the gateway, same as ProxiedURL does.
			`#EXT-X-KEY:METHOD=AES-128,URI="/api/v1/hls-proxy?url=key"`,
			"#EXTINF:6.0,",
			"/api/streaming/hls-proxy?url=seg1",
			"#EXTINF:6.0,",
			"seg2.ts",
			"#EXT-X-ENDLIST",
			"",
		}, "\n")))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	gatewayBase := "https://gw.example"
	local, dur, err := LocalizeHLS(context.Background(), srv.Client(), gatewayBase, srv.URL+"/master.m3u8", dir)
	if err != nil {
		t.Fatalf("LocalizeHLS: %v", err)
	}
	if dur != 12.0 {
		t.Fatalf("duration: got %f want 12.0", dur)
	}
	b, err := os.ReadFile(local)
	if err != nil {
		t.Fatalf("read local playlist: %v", err)
	}
	content := string(b)

	if !strings.Contains(content, `URI="`+gatewayBase+`/api/streaming/hls-proxy?url=key"`) {
		t.Fatalf("EXT-X-KEY URI not absolutized+remapped (/api/v1/ -> /api/streaming/) onto gateway:\n%s", content)
	}
	if !strings.Contains(content, gatewayBase+"/api/streaming/hls-proxy?url=seg1") {
		t.Fatalf("root-absolute segment not gateway-prefixed:\n%s", content)
	}
	if !strings.Contains(content, srv.URL+"/seg2.ts") {
		t.Fatalf("relative segment not server-prefixed:\n%s", content)
	}
	// No line should be left un-absolutized (bare "seg2.ts" or "/api/streaming"
	// without a scheme).
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#EXTM3U") || strings.HasPrefix(trimmed, "#EXTINF") || trimmed == "#EXT-X-ENDLIST" {
			continue
		}
		if strings.HasPrefix(trimmed, "#EXT-X-KEY") {
			continue
		}
		if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
			t.Fatalf("segment line not absolute: %q", trimmed)
		}
	}
}
