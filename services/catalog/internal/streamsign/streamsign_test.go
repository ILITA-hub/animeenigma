package streamsign

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

func TestMain(m *testing.M) {
	if os.Getenv("STREAM_TOKEN_SECRET") == "" {
		os.Setenv("STREAM_TOKEN_SECRET", "test-streamsign-secret-0123456789")
	}
	os.Exit(m.Run())
}

func TestIsExternal(t *testing.T) {
	cases := map[string]bool{
		"https://cdn.example.com/x.m3u8":         true,
		"http://cdn.example.com/x.m3u8":          true,
		"/api/anime/x/subtitles/opensubtitles/1": false,
		"//cdn.example.com/x.m3u8":               false, // protocol-relative not signed (proxy gets absolute)
		"":                                       false,
	}
	for u, want := range cases {
		if got := IsExternal(u); got != want {
			t.Errorf("IsExternal(%q) = %v; want %v", u, got, want)
		}
	}
}

func TestSignScraperStreamBody_SignsExternalSourcesAndTracks(t *testing.T) {
	body := []byte(`{"success":true,"data":{"stream":{` +
		`"sources":[{"url":"https://cdn.example.com/master.m3u8","type":"hls"}],` +
		`"tracks":[{"file":"https://subs.example.com/en.vtt","kind":"subtitles"},` +
		`{"file":"/api/anime/x/subtitles/jimaku/1","kind":"subtitles"}],` +
		`"headers":{"Referer":"https://allmanga.to"},"intro":{"start":0,"end":90}},` +
		`"meta":{"tried":["allanime"],"provider":"allanime"}}}`)

	out := SignScraperStreamBody(http.StatusOK, body)

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal rewritten body: %v", err)
	}
	stream := env["data"].(map[string]any)["stream"].(map[string]any)

	src := stream["sources"].([]any)[0].(map[string]any)
	if src["exp"] == nil || src["sig"] == nil {
		t.Error("external source url was not signed (missing exp/sig)")
	}

	tracks := stream["tracks"].([]any)
	ext := tracks[0].(map[string]any)
	if ext["exp"] == nil || ext["sig"] == nil {
		t.Error("external track was not signed")
	}
	sameOrigin := tracks[1].(map[string]any)
	if sameOrigin["exp"] != nil || sameOrigin["sig"] != nil {
		t.Error("same-origin track must NOT be signed")
	}

	// Forward-compat / passthrough fields preserved.
	if stream["headers"] == nil || stream["intro"] == nil {
		t.Error("headers/intro fields dropped during rewrite")
	}
	if env["data"].(map[string]any)["meta"] == nil {
		t.Error("meta dropped during rewrite")
	}
}

func TestSignScraperStreamBody_LeavesNon200AndErrorBodies(t *testing.T) {
	// non-200 untouched
	errBody := []byte(`{"success":false,"error":{"code":"INTERNAL","message":"x"}}`)
	if got := SignScraperStreamBody(http.StatusBadGateway, errBody); string(got) != string(errBody) {
		t.Error("non-200 body was modified")
	}
	// 200 but success:false untouched
	if got := SignScraperStreamBody(http.StatusOK, errBody); string(got) != string(errBody) {
		t.Error("success:false body was modified")
	}
	// malformed JSON untouched
	bad := []byte(`not json`)
	if got := SignScraperStreamBody(http.StatusOK, bad); string(got) != string(bad) {
		t.Error("malformed body was modified")
	}
}

func TestSignScraperStreamBody_StampsMaskedURL(t *testing.T) {
	body := []byte(`{"success":true,"data":{"stream":{` +
		`"sources":[{"url":"https://cdn.example.com/master.m3u8","type":"hls"},` +
		`{"url":"https://mp4.example.com/ep.mp4","type":"mp4"}],` +
		`"tracks":[{"file":"https://subs.example.com/en.vtt","kind":"subtitles"}],` +
		`"headers":{"Referer":"https://allmanga.to"}},` +
		`"meta":{"provider":"allanime"}}}`)

	out := SignScraperStreamBody(http.StatusOK, body)

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stream := env["data"].(map[string]any)["stream"].(map[string]any)
	sources := stream["sources"].([]any)

	hls := sources[0].(map[string]any)
	masked, _ := hls["masked_url"].(string)
	if !strings.HasPrefix(masked, "/api/streaming/m/") {
		t.Fatalf("hls source masked_url = %q", masked)
	}
	if strings.Contains(masked, "cdn.example.com") {
		t.Fatal("masked_url leaks upstream hostname")
	}

	mp4 := sources[1].(map[string]any)
	if m, _ := mp4["masked_url"].(string); !strings.HasPrefix(m, "/api/streaming/m/") {
		t.Fatalf("mp4 source masked_url = %q", m)
	}

	track := stream["tracks"].([]any)[0].(map[string]any)
	if m, _ := track["masked_url"].(string); !strings.HasPrefix(m, "/api/streaming/m/") {
		t.Fatalf("track masked_url = %q", m)
	}
	// exp/sig legacy pair still stamped (dual-accept).
	if hls["exp"] == nil || hls["sig"] == nil {
		t.Fatal("legacy exp/sig missing — dual-accept broken")
	}
}

// AUTO-627: megaplay-family CDNs Referer-gate subtitle .vtt files exactly like
// their video playlists — a masked track token minted WITHOUT the stream's
// Referer makes the proxy fetch 403 upstream and the player show blank subs.
// Both sources[] and tracks[] tokens must carry the stream headers' Referer.
func TestSignScraperStreamBody_MaskedTokensCarryReferer(t *testing.T) {
	const referer = "https://megaplay.buzz/"
	body := []byte(`{"success":true,"data":{"stream":{` +
		`"sources":[{"url":"https://cdn.example.com/master.m3u8","type":"hls"}],` +
		`"tracks":[{"file":"https://subs.example.com/track_0_eng.vtt","kind":"captions"}],` +
		`"headers":{"Referer":"` + referer + `"}},` +
		`"meta":{"provider":"gogoanime"}}}`)

	out := SignScraperStreamBody(http.StatusOK, body)

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stream := env["data"].(map[string]any)["stream"].(map[string]any)

	decode := func(field string, m map[string]any) *videoutils.StreamTokenPayload {
		t.Helper()
		masked, _ := m["masked_url"].(string)
		tok := strings.TrimPrefix(masked, "/api/streaming/m/")
		if i := strings.IndexByte(tok, '/'); i >= 0 {
			tok = tok[:i]
		}
		p, err := videoutils.DecodeStreamToken(tok, time.Now())
		if err != nil {
			t.Fatalf("%s: decode masked token: %v", field, err)
		}
		return p
	}

	src := decode("source", stream["sources"].([]any)[0].(map[string]any))
	if src.Referer != referer {
		t.Errorf("source token referer = %q; want %q", src.Referer, referer)
	}
	trk := decode("track", stream["tracks"].([]any)[0].(map[string]any))
	if trk.Referer != referer {
		t.Errorf("track token referer = %q; want %q", trk.Referer, referer)
	}
}
