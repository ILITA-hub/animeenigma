package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func TestAeAnimeSet_Resolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/internal/probe/ae-targets") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"data":{"targets":[{"uuid":"u1","name":"Frieren","episode":28}]}}`))
	}))
	defer srv.Close()

	as := NewAeAnimeSet(srv.URL, 3, srv.Client())
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(refs))
	}
	r := refs[0]
	if r.UUID != "u1" || r.Name != "Frieren" || r.Episode != 28 || r.Slot != SlotLibraryLatest {
		t.Fatalf("ref = %+v, want {u1, Frieren, 28, library_latest}", r)
	}
}

func TestAeAnimeSet_EmptyOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	as := NewAeAnimeSet(srv.URL, 3, srv.Client())
	refs, err := as.Resolve(context.Background())
	if err != nil || refs != nil {
		t.Fatalf("want nil,nil on 500; got refs=%+v err=%v", refs, err)
	}
}

func TestAeResolver_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/ae/stream") || r.URL.Query().Get("episode") != "28" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"success":true,"data":{"url":"http://minio:9000/raw-library/x.m3u8","exp":"99","sig":"ab"}}`))
	}))
	defer srv.Close()

	r := NewAeResolver(srv.URL, srv.Client())
	streams, stage, err := r.Resolve(context.Background(), "u1", "Frieren", 28, SlotLibraryLatest, "ae")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stage != StageStream || len(streams) != 1 {
		t.Fatalf("stage=%v n=%d", stage, len(streams))
	}
	s := streams[0]
	if s.MasterURL != "http://minio:9000/raw-library/x.m3u8" || s.Exp != "99" || s.Sig != "ab" {
		t.Fatalf("bad stream: %+v", s)
	}
	if s.Provider != "ae" || s.AnimeName != "Frieren" || s.Server != "library" {
		t.Fatalf("bad stream meta: %+v", s)
	}
}

func TestAeResolver_FlatBody(t *testing.T) {
	// Defensive: a flat (un-enveloped) body must still decode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"url":"http://minio/x.m3u8","exp":"1","sig":"z"}`))
	}))
	defer srv.Close()
	r := NewAeResolver(srv.URL, srv.Client())
	streams, _, err := r.Resolve(context.Background(), "u1", "X", 0, SlotLibraryLatest, "ae")
	if err != nil || len(streams) != 1 || streams[0].MasterURL != "http://minio/x.m3u8" {
		t.Fatalf("flat decode failed: streams=%+v err=%v", streams, err)
	}
}

func TestAeResolver_NoStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"url":""}}`))
	}))
	defer srv.Close()
	r := NewAeResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "u1", "X", 1, SlotLibraryLatest, "ae")
	if err == nil || stage != StageStream {
		t.Fatalf("want stream-stage error on empty url; stage=%v err=%v", stage, err)
	}
}

// ensure the synthetic-verdict reason stays a valid streamprobe reason.
func TestAe_ReasonsAreValid(t *testing.T) {
	if streamprobe.ReasonEmptyResponse == "" {
		t.Fatal("ReasonEmptyResponse must be defined")
	}
}
