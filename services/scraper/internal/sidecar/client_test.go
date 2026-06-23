package sidecar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/userkey"
)

const okBody = `{"success":true,"data":{
  "session_id":"abc123",
  "master_url":"https://s2.cinewave2.site/a/b/master.m3u8",
  "playlist_proxy_path":"/hls?sid=abc123&url=enc",
  "referer":"https://megaplay.buzz/",
  "subtitles":[{"url":"https://x/eng.vtt","label":"English","default":true}],
  "intro":{"start":0,"end":130},
  "outro":{"start":1400,"end":1440}
}}`

func TestResolveEmbed_MapsSession(t *testing.T) {
	var gotPath, gotCT string
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		_ = decodeJSON(r, &gotReq)
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	st, err := c.ResolveEmbed(context.Background(), "gogoanime", "https://gogoanime.me.uk/x", domain.CategorySub, "https://gogoanimes.fi")
	if err != nil {
		t.Fatalf("ResolveEmbed: %v", err)
	}
	if gotPath != "/resolve" {
		t.Errorf("path = %q; want /resolve", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotReq["provider"] != "gogoanime" || gotReq["embed_url"] != "https://gogoanime.me.uk/x" {
		t.Errorf("request payload wrong: %v", gotReq)
	}
	// Source is the sidecar's own /hls restream path (baseURL + playlist_proxy_path),
	// NOT the raw CDN master — the CDN is Cloudflare-fingerprint-gated and can only
	// be fetched through the resolving browser, so the streaming proxy fetches this.
	if len(st.Sources) != 1 || st.Sources[0].URL != srv.URL+"/hls?sid=abc123&url=enc" {
		t.Errorf("source url = %v; want sidecar /hls path", st.Sources)
	}
	if st.Sources[0].Type != "hls" {
		t.Errorf("type = %q; want hls", st.Sources[0].Type)
	}
	if len(st.Tracks) != 1 || !st.Tracks[0].Default || st.Tracks[0].Kind != "captions" {
		t.Errorf("tracks = %v", st.Tracks)
	}
	if st.Intro == nil || st.Intro.End != 130 || st.Outro == nil || st.Outro.Start != 1400 {
		t.Errorf("intro/outro = %v %v", st.Intro, st.Outro)
	}
}

func TestResolveEmbed_404_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false,"kind":"not_found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestResolveEmbed_502_ProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false,"kind":"challenge"}`, http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
}

func TestResolveEmbed_SuccessFalse_ProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":"no m3u8","kind":"error"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
}

func TestResolveEmbed_TransportError_ProviderDown(t *testing.T) {
	// A closed listener → connection refused → transport error must map to
	// ErrProviderDown so the orchestrator soft-skips/fails over.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	c := New(addr, 2*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
}

func TestResolveEmbed_EmptyPlaylistPath_ProviderDown(t *testing.T) {
	// success:true but no playlist_proxy_path would otherwise build a Source URL
	// of baseURL+"" (the bare sidecar root) — a broken HLS source. Guarded.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"master_url":"https://x/m.m3u8"}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
}

func TestResolveEmbed_MalformedJSON_ProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, err := c.ResolveEmbed(context.Background(), "gogoanime", "e", domain.CategorySub, "")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("err = %v; want ErrProviderDown", err)
	}
}

func TestFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := base64.StdEncoding.EncodeToString([]byte(`{"ok":1}`))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "status": 200, "content_type": "application/json", "body": body,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	status, body, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if string(body) != `{"ok":1}` {
		t.Fatalf("body = %q", body)
	}
}

func TestFetch_UpstreamStatusPassedThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := base64.StdEncoding.EncodeToString([]byte("not found"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "status": 404, "content_type": "text/html", "body": body,
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	status, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/missing")
	if err != nil {
		t.Fatalf("err: %v", err) // upstream 404 is NOT a sidecar error; Go handles status
	}
	if status != 404 {
		t.Fatalf("status = %d", status)
	}
}

func TestFetch_ChallengeIsProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false, "kind": "challenge", "error": "blocked",
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("want ErrProviderDown, got %v", err)
	}
}

func TestFetch_TransportErrorIsProviderDown(t *testing.T) {
	c := New("http://127.0.0.1:0", 1*time.Second) // unroutable
	_, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("want ErrProviderDown, got %v", err)
	}
}

func TestResolveEmbed_ForwardsUserKeyFromContext(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	ctx := userkey.WithUserKey(context.Background(), "alice")
	if _, err := c.ResolveEmbed(ctx, "gogoanime", "https://x/y", domain.CategorySub, "https://b"); err != nil {
		t.Fatalf("ResolveEmbed: %v", err)
	}
	if gotReq["user_key"] != "alice" {
		t.Errorf("user_key = %v; want alice", gotReq["user_key"])
	}
}

func TestResolveEmbed_OmitsUserKeyWhenAbsent(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	if _, err := c.ResolveEmbed(context.Background(), "gogoanime", "https://x/y", domain.CategorySub, "https://b"); err != nil {
		t.Fatalf("ResolveEmbed: %v", err)
	}
	if _, present := gotReq["user_key"]; present {
		t.Errorf("user_key present on anon request: %v", gotReq)
	}
}

func TestFetch_ForwardsUserKeyFromContext(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		body := base64.StdEncoding.EncodeToString([]byte(`{"ok":1}`))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "status": 200, "body": body})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	ctx := userkey.WithUserKey(context.Background(), "alice")
	if _, _, err := c.Fetch(ctx, "nineanime", "https://9anime.me.uk/x"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotReq["user_key"] != "alice" {
		t.Errorf("user_key = %v; want alice", gotReq["user_key"])
	}
}

func TestFetch_OmitsUserKeyWhenAbsent(t *testing.T) {
	var gotReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = decodeJSON(r, &gotReq)
		body := base64.StdEncoding.EncodeToString([]byte(`{"ok":1}`))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "status": 200, "body": body})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	if _, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if _, present := gotReq["user_key"]; present {
		t.Errorf("user_key present on anon request: %v", gotReq)
	}
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
