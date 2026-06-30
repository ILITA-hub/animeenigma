package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnimejoyResolver_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/animejoy-sibnet/episodes"):
			w.Write([]byte(`{"success":true,"data":{"episodes":[1,2,3],"teams":[{"id":"0","name":""}]}}`))
		case strings.HasSuffix(r.URL.Path, "/animejoy-sibnet/stream"):
			if r.URL.Query().Get("episode") != "1" {
				t.Errorf("stream episode = %q, want 1", r.URL.Query().Get("episode"))
			}
			w.Write([]byte(`{"success":true,"data":{"url":"https://video.sibnet.ru/v/abc/5.mp4","referer":"https://video.sibnet.ru/","exp":"99","sig":"ab","type":"mp4"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	r := NewAnimejoyResolver(srv.URL, srv.Client())
	streams, stage, err := r.Resolve(context.Background(), "u1", "Frieren", 0, SlotFeatured, "animejoy-sibnet")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stage != StageStream || len(streams) != 1 {
		t.Fatalf("stage=%v n=%d", stage, len(streams))
	}
	s := streams[0]
	if s.MasterURL != "https://video.sibnet.ru/v/abc/5.mp4" || s.Exp != "99" || s.Sig != "ab" || s.Referer != "https://video.sibnet.ru/" {
		t.Fatalf("bad stream: %+v", s)
	}
	if s.Provider != "animejoy-sibnet" || s.AnimeName != "Frieren" || s.Server != "animejoy-sibnet" {
		t.Fatalf("bad stream meta: %+v", s)
	}
}

func TestAnimejoyResolver_EpisodeOverridePicksRequested(t *testing.T) {
	var gotEp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/episodes") {
			w.Write([]byte(`{"success":true,"data":{"episodes":[1,2,3]}}`))
			return
		}
		gotEp = r.URL.Query().Get("episode")
		w.Write([]byte(`{"success":true,"data":{"url":"https://x/1.mp4"}}`))
	}))
	defer srv.Close()
	r := NewAnimejoyResolver(srv.URL, srv.Client())
	if _, _, err := r.Resolve(context.Background(), "u1", "X", 2, SlotFeatured, "animejoy-allvideo"); err != nil {
		t.Fatal(err)
	}
	if gotEp != "2" {
		t.Fatalf("requested episode not honored: got %q, want 2", gotEp)
	}
}

func TestAnimejoyResolver_NoMatchIsReRoll(t *testing.T) {
	// Empty episodes ⇒ AnimeJoy lacks this title ⇒ ErrProbeNotFound (re-roll),
	// NOT a down verdict.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"episodes":[],"teams":[]}}`))
	}))
	defer srv.Close()
	r := NewAnimejoyResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "u1", "Obscure", 0, SlotFeatured, "animejoy-sibnet")
	if !errors.Is(err, ErrProbeNotFound) {
		t.Fatalf("want ErrProbeNotFound, got %v", err)
	}
	if stage != StageSearch {
		t.Fatalf("want StageSearch, got %v", stage)
	}
}

func TestAnimejoyResolver_EmptyStreamURLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/episodes") {
			w.Write([]byte(`{"success":true,"data":{"episodes":[1]}}`))
			return
		}
		w.Write([]byte(`{"success":true,"data":{"url":""}}`))
	}))
	defer srv.Close()
	r := NewAnimejoyResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "u1", "X", 0, SlotFeatured, "animejoy-sibnet")
	if err == nil || stage != StageStream {
		t.Fatalf("want StageStream error, got stage=%v err=%v", stage, err)
	}
}
