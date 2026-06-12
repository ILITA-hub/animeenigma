// megaplay_test.go — offline fixture tests for MegaplayExtractor.
//
// Uses the rewriteToSrv RoundTripper (packed_common_test.go) so Matches()
// stays strict against the real 1anime.site / megaplay.buzz hosts while the
// underlying TCP socket points at a local httptest server that serves the
// captured fixtures by path.
package embeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// megaplaySrv serves the full three-hop chain by path:
//   - /megaplay/stream/s-2/94554/sub      → 1anime.site wrapper
//   - /stream/s-2/94554/sub               → megaplay.buzz player (data-id)
//   - /stream/getSources                  → getSources JSON
func megaplaySrv(t *testing.T) *httptest.Server {
	t.Helper()
	wrapper := mustRead(t, "megaplay_wrapper_1anime.html")
	player := mustRead(t, "megaplay_player.html")
	sources := mustRead(t, "megaplay_getsources.json")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/megaplay/stream/"):
			_, _ = w.Write(wrapper)
		case r.URL.Path == "/stream/getSources":
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(sources)
		case strings.HasPrefix(r.URL.Path, "/stream/s-2/"):
			_, _ = w.Write(player)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func newTestMegaplay(srvURL string) *MegaplayExtractor {
	e := NewMegaplayExtractor()
	e.http = &http.Client{Transport: &rewriteToSrv{srvURL: srvURL}}
	return e
}

// vidwishWrapperHTML nests a vidwish.live player iframe — the API-identical
// sister player the gogoanime/9anime chain migrated to in 2026-06 (AUTO-446).
const vidwishWrapperHTML = `<!DOCTYPE html><html><body>` +
	`<iframe src="https://vidwish.live/stream/s-2/94554/sub" frameborder="0"></iframe>` +
	`</body></html>`

// vidwishSrv serves the same three-hop chain as megaplaySrv but with a wrapper
// page that nests a vidwish.live iframe (player + getSources fixtures reused —
// the endpoints are byte-identical to megaplay.buzz).
func vidwishSrv(t *testing.T) *httptest.Server {
	t.Helper()
	player := mustRead(t, "megaplay_player.html")
	sources := mustRead(t, "megaplay_getsources.json")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/megaplay/stream/"):
			_, _ = w.Write([]byte(vidwishWrapperHTML))
		case r.URL.Path == "/stream/getSources":
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(sources)
		case strings.HasPrefix(r.URL.Path, "/stream/s-2/"):
			_, _ = w.Write(player)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func TestMegaplay_Name(t *testing.T) {
	if got := NewMegaplayExtractor().Name(); got != "megaplay" {
		t.Fatalf("Name() = %q; want megaplay", got)
	}
}

// markerTransport is a sentinel RoundTripper used to prove the wrap func was
// applied to the extractor's http.Client transport (WR-07).
type markerTransport struct{ inner http.RoundTripper }

func (m *markerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return m.inner.RoundTrip(r)
}

// TestMegaplay_RecordingTransportWrapped proves the recording-wrap seam is
// applied to the extractor's http.Client transport (WR-07) — mirrors the
// kodikextract NewRecordingClient test. The default zero-arg constructor leaves
// the transport unwrapped (back-compat).
func TestMegaplay_RecordingTransportWrapped(t *testing.T) {
	var wrapCalled bool
	e := NewRecordingMegaplayExtractor(func(base http.RoundTripper) http.RoundTripper {
		wrapCalled = true
		return &markerTransport{inner: base}
	})
	if !wrapCalled {
		t.Fatal("wrap func was never invoked — recording seam not applied")
	}
	if _, ok := e.http.Transport.(*markerTransport); !ok {
		t.Fatalf("http.Client.Transport = %T; want *markerTransport (recording-wrapped)", e.http.Transport)
	}

	// Back-compat: the zero-arg constructor must NOT wrap (no recording).
	plain := NewMegaplayExtractor()
	if _, ok := plain.http.Transport.(*markerTransport); ok {
		t.Fatal("NewMegaplayExtractor() unexpectedly wrapped its transport")
	}
}

func TestMegaplay_Matches(t *testing.T) {
	t.Parallel()
	e := NewMegaplayExtractor()
	cases := []struct {
		url  string
		want bool
	}{
		{"https://1anime.site/megaplay/stream/s-2/94554/sub", true},
		{"https://megaplay.buzz/stream/s-2/94554/sub", true},
		{"https://cdn.megaplay.buzz/x", true},     // strict subdomain
		{"https://vidwish.live/stream/s-2/94554/sub", true}, // AUTO-446 sister player
		{"https://cdn.vidwish.live/x", true},                // strict subdomain
		{"https://evilvidwish.live/x", false},               // substring impostor
		{"https://vidwish.live.evil.com/x", false},          // suffix impostor
		{"https://gogoanime.me.uk/newplayer.php?id=frieren-20409?ep=168580&type=hd-1&category=dub", true}, // gogoanime megaplay wrapper
		{"https://evil1anime.site/x", false},      // substring impostor
		{"https://1anime.site.evil.com/x", false}, // suffix impostor
		{"https://my.1anime.site/index.php", false},
		{"https://gogoanime.me.uk.evil.com/x", false}, // suffix impostor (exact-host gate)
		{"https://sub.gogoanime.me.uk/x", false},      // exact-host only, no subdomains
		{"https://youtube.com/embed/x", false},
		{"ftp://megaplay.buzz/x", false},
	}
	for _, c := range cases {
		if got := e.Matches(c.url); got != c.want {
			t.Errorf("Matches(%q) = %v; want %v", c.url, got, c.want)
		}
	}
}

// TestMegaplay_Extract_FullChain drives wrapper → player → getSources and
// asserts the resolved HLS source, subtitle track, intro marker, and Referer.
func TestMegaplay_Extract_FullChain(t *testing.T) {
	srv := megaplaySrv(t)
	defer srv.Close()
	e := newTestMegaplay(srv.URL)

	stream, err := e.Extract(context.Background(), "https://1anime.site/megaplay/stream/s-2/94554/sub", nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(stream.Sources) != 1 {
		t.Fatalf("Sources len = %d; want 1", len(stream.Sources))
	}
	src := stream.Sources[0]
	if !strings.HasSuffix(src.URL, "master.m3u8") {
		t.Errorf("Sources[0].URL = %q; want master.m3u8 suffix", src.URL)
	}
	if !strings.Contains(src.URL, "cdn.mewstream.buzz") {
		t.Errorf("Sources[0].URL = %q; want cdn.mewstream.buzz host", src.URL)
	}
	if src.Type != "hls" {
		t.Errorf("Sources[0].Type = %q; want hls", src.Type)
	}
	if stream.Headers["Referer"] != megaplayReferer {
		t.Errorf("Headers[Referer] = %q; want %q", stream.Headers["Referer"], megaplayReferer)
	}
	if len(stream.Tracks) != 1 {
		t.Fatalf("Tracks len = %d; want 1", len(stream.Tracks))
	}
	if !strings.HasSuffix(stream.Tracks[0].File, ".vtt") || !stream.Tracks[0].Default {
		t.Errorf("Track[0] = %+v; want default .vtt", stream.Tracks[0])
	}
	if stream.Intro == nil || stream.Intro.End != 130 {
		t.Errorf("Intro = %+v; want End=130", stream.Intro)
	}
	if stream.Outro != nil {
		t.Errorf("Outro = %+v; want nil (End=0)", stream.Outro)
	}
}

// TestMegaplay_Extract_DirectMegaplayURL skips the wrapper hop when given a
// megaplay.buzz URL directly.
func TestMegaplay_Extract_DirectMegaplayURL(t *testing.T) {
	srv := megaplaySrv(t)
	defer srv.Close()
	e := newTestMegaplay(srv.URL)

	stream, err := e.Extract(context.Background(), "https://megaplay.buzz/stream/s-2/94554/sub", nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(stream.Sources) != 1 || !strings.HasSuffix(stream.Sources[0].URL, "master.m3u8") {
		t.Fatalf("Sources = %+v; want one master.m3u8", stream.Sources)
	}
}

// TestMegaplay_Extract_VidwishWrapper proves the extractor resolves a
// vidwish.live player nested in a wrapper page (AUTO-446) and derives the
// vidwish.live Referer from the active player origin — NOT the hardcoded
// megaplay.buzz value, which the vidwish CDN would 403.
func TestMegaplay_Extract_VidwishWrapper(t *testing.T) {
	srv := vidwishSrv(t)
	defer srv.Close()
	e := newTestMegaplay(srv.URL)

	stream, err := e.Extract(context.Background(), "https://1anime.site/megaplay/stream/s-2/94554/sub", nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(stream.Sources) != 1 || !strings.HasSuffix(stream.Sources[0].URL, "master.m3u8") {
		t.Fatalf("Sources = %+v; want one master.m3u8", stream.Sources)
	}
	if got := stream.Headers["Referer"]; got != "https://vidwish.live/" {
		t.Errorf("Headers[Referer] = %q; want https://vidwish.live/ (active domain, not megaplay.buzz)", got)
	}
}

func TestMegaplay_Extract_RejectsNonAllowlistedHost(t *testing.T) {
	e := NewMegaplayExtractor()
	_, err := e.Extract(context.Background(), "https://youtube.com/embed/x", nil)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("err = %v; want ErrExtractFailed", err)
	}
}

func TestMegaplay_Extract_GetSourcesRequiresXHR(t *testing.T) {
	// Server that 404s getSources unless X-Requested-With is set — proves the
	// extractor sends it. (The shared megaplaySrv already enforces this; this
	// test fails loudly if the XHR header were ever dropped.)
	srv := megaplaySrv(t)
	defer srv.Close()
	e := newTestMegaplay(srv.URL)
	if _, err := e.Extract(context.Background(), "https://megaplay.buzz/stream/s-2/94554/sub", nil); err != nil {
		t.Fatalf("Extract with XHR should succeed: %v", err)
	}
}
