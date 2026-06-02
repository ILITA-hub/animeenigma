package streamsign

import (
	"encoding/json"
	"net/http"
	"testing"
)

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
