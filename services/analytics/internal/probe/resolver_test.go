package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPResolver_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/scraper/episodes"):
			w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep1","number":1}]}}`))
		case strings.Contains(r.URL.Path, "/scraper/servers"):
			w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"srvA","name":"HD-1","type":"sub"}]}}`))
		case strings.Contains(r.URL.Path, "/scraper/stream"):
			w.Write([]byte(`{"success":true,"data":{"stream":{"headers":{"Referer":"https://ref/"},"sources":[{"url":"https://cdn/m.m3u8","exp":"99","sig":"ab","type":"hls"}]}}}`))
		}
	}))
	defer srv.Close()

	r := NewHTTPResolver(srv.URL, srv.Client())
	streams, stage, err := r.Resolve(context.Background(), "uuid1", "Frieren", 0, SlotAnchor, "gogoanime")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stage != StageStream || len(streams) != 1 {
		t.Fatalf("stage=%v n=%d", stage, len(streams))
	}
	s := streams[0]
	if s.MasterURL != "https://cdn/m.m3u8" || s.Exp != "99" || s.Sig != "ab" || s.Referer != "https://ref/" {
		t.Fatalf("bad resolved stream: %+v", s)
	}
	if s.AnimeName != "Frieren" {
		t.Fatalf("AnimeName not carried through: got %q", s.AnimeName)
	}
}

func TestHTTPResolver_NoEpisodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer srv.Close()
	r := NewHTTPResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "uuid1", "Frieren", 0, SlotAnchor, "gogoanime")
	if err == nil || stage != StageEpisodes {
		t.Fatalf("want episodes-stage error, got stage=%v err=%v", stage, err)
	}
}

func TestHTTPResolver_NotFound404_ReturnsSearchSentinel(t *testing.T) {
	var sawExclusive bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/scraper/episodes") {
			if r.URL.Query().Get("exclusive") == "true" {
				sawExclusive = true
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}))
	defer srv.Close()

	res := NewHTTPResolver(srv.URL, srv.Client())
	_, stage, err := res.Resolve(context.Background(), "uuid1", "Frieren", 0, SlotAnchor, "gogoanime")
	if !errors.Is(err, ErrProbeNotFound) {
		t.Fatalf("want ErrProbeNotFound, got %v", err)
	}
	if stage != StageSearch {
		t.Fatalf("want StageSearch, got %v", stage)
	}
	if !sawExclusive {
		t.Fatal("expected request to carry exclusive=true query param")
	}
}
