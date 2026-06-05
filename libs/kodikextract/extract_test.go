package kodikextract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// stubRoundTripper records the hosts it intercepts and delegates to inner.
// It stands in for tracing.WrapRecording's recording transport.
type stubRoundTripper struct {
	inner http.RoundTripper
	hosts []string
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.hosts = append(s.hosts, req.URL.Host)
	return s.inner.RoundTrip(req)
}

// TestKodikExtractTransport — ResolveWithClient routes the outbound embed GET
// through the injected (recording) transport. We point the embed at an
// httptest server and assert the stub transport saw the request; the resolve
// itself fails later (no real /ftor stream) but the recording seam fired,
// which is the behavior under test. Resolve() remains a thin back-compat
// wrapper using newClient() (asserted by the package's other tests + build).
func TestKodikExtractTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleEmbed))
	}))
	defer srv.Close()

	stub := &stubRoundTripper{inner: http.DefaultTransport}
	client := NewRecordingClient(func(base http.RoundTripper) http.RoundTripper {
		return stub // ignore base; route everything through the stub for the test
	})

	// The embed GET succeeds (server returns sampleEmbed); the /ftor POST then
	// fails to decode a real stream. Either way the transport must be hit.
	_, _ = ResolveWithClient(context.Background(), srv.URL, client)

	if len(stub.hosts) == 0 {
		t.Fatalf("injected transport never invoked — egress recording would never fire")
	}
	wantHost := strings.TrimPrefix(srv.URL, "http://")
	if stub.hosts[0] != wantHost {
		t.Errorf("recorder saw host %q, want %q", stub.hosts[0], wantHost)
	}
}

const sampleEmbed = `
<script>
  videoInfo.type = 'seria';
  videoInfo.hash = '71dcc2d2bb2459ae1ae89f58e17cabff';
  videoInfo.id = '782423';
  var domain = "kodikplayer.com";
  var d_sign = "c0167a7b33be40af";
  var pd_sign = "c0167a7b33be40af";
  var ref = "https://kodikplayer.com/";
  var ref_sign = "a525bb4353fafa27";
</script>`

func TestParseEmbedParams(t *testing.T) {
	p, err := parseEmbedParams(sampleEmbed)
	if err != nil {
		t.Fatalf("parseEmbedParams err: %v", err)
	}
	if p.Type != "seria" || p.ID != "782423" {
		t.Fatalf("type/id wrong: %+v", p)
	}
	if p.Ref != "https://kodikplayer.com/" {
		t.Fatalf("ref wrong: %q (must not match href= attributes)", p.Ref)
	}
	if p.Domain != "kodikplayer.com" || p.DSign == "" || p.RefSign == "" {
		t.Fatalf("signed params missing: %+v", p)
	}
}

func TestParseEmbedParamsMissing(t *testing.T) {
	if _, err := parseEmbedParams("<html>nope</html>"); err == nil {
		t.Fatal("expected error for embed with no params")
	}
}

// TestBuildStreamsFixture tests the /ftor decode path offline using a captured fixture.
func TestBuildStreamsFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/ftor.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var fr ftorResponse
	if err := json.Unmarshal(data, &fr); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	res, err := buildStreams(fr, "https://kodikplayer.com/")
	if err != nil {
		t.Fatalf("buildStreams err: %v", err)
	}

	if len(res.Streams) != 1 {
		t.Fatalf("want 1 stream, got %d: %+v", len(res.Streams), res.Streams)
	}
	s := res.Streams[0]
	if s.Quality != 720 {
		t.Errorf("want Quality=720, got %d", s.Quality)
	}
	if !strings.HasPrefix(s.M3U8URL, "https://cloud.solodcdn.com") {
		t.Errorf("M3U8URL should start with https://cloud.solodcdn.com, got %q", s.M3U8URL)
	}
	if !strings.Contains(s.M3U8URL, "mp4:hls:manifest.m3u8") {
		t.Errorf("M3U8URL should contain mp4:hls:manifest.m3u8, got %q", s.M3U8URL)
	}
	if res.Referer != "https://kodikplayer.com/" {
		t.Errorf("Referer want %q, got %q", "https://kodikplayer.com/", res.Referer)
	}
}

// TestPickQuality verifies quality selection logic across boundary cases.
func TestPickQuality(t *testing.T) {
	streams := []Stream{
		{Quality: 360, M3U8URL: "https://cdn.example.com/360.m3u8"},
		{Quality: 480, M3U8URL: "https://cdn.example.com/480.m3u8"},
		{Quality: 720, M3U8URL: "https://cdn.example.com/720.m3u8"},
	}
	r := &Result{Default: 360, Streams: streams}

	tests := []struct {
		name  string
		want  int
		wantQ int
	}{
		{"want=0 returns default (360)", 0, 360},
		{"want=720 exact match", 720, 720},
		{"want=500 returns highest<=500 (480)", 500, 480},
		{"want=1080 no>=1080 returns highest (720)", 1080, 720},
		{"want=240 no<=240 returns highest (720)", 240, 720},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.PickQuality(tc.want)
			if got.Quality != tc.wantQ {
				t.Errorf("PickQuality(%d): got Quality=%d, want %d", tc.want, got.Quality, tc.wantQ)
			}
		})
	}
}

// TestPickQualityEmpty verifies that PickQuality on an empty Result returns zero Stream without panic.
func TestPickQualityEmpty(t *testing.T) {
	r := &Result{}
	got := r.PickQuality(720)
	if got.Quality != 0 {
		t.Errorf("empty PickQuality: want Quality=0, got %d", got.Quality)
	}
}
