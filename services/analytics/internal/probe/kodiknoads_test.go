package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// kodikStub serves translations + stream. translationsBody and streamBody let
// each test shape the responses.
func kodikStub(t *testing.T, translationsBody, streamBody string, streamStatus int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/kodik/translations"):
			w.Write([]byte(translationsBody))
		case strings.Contains(r.URL.Path, "/kodik/stream"):
			if streamStatus != 0 && streamStatus != 200 {
				w.WriteHeader(streamStatus)
			}
			w.Write([]byte(streamBody))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestKodikNoads_HappyPath_PrefersPinned(t *testing.T) {
	translations := `{"success":true,"data":[{"id":111,"pinned":false},{"id":222,"pinned":true}]}`
	stream := `{"success":true,"data":{"stream_url":"https://cloud.solodcdn.com/m.m3u8","referer":"https://kodikplayer.com/"}}`
	srv := kodikStub(t, translations, stream, 200)
	defer srv.Close()

	r := NewKodikNoadsResolver(srv.URL, srv.Client())
	streams, stage, err := r.Resolve(context.Background(), "u1", "Frieren", 0, SlotAnchor, "kodik-noads")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stage != StageStream || len(streams) != 1 {
		t.Fatalf("stage=%v n=%d", stage, len(streams))
	}
	s := streams[0]
	if s.MasterURL != "https://cloud.solodcdn.com/m.m3u8" || s.Referer != "https://kodikplayer.com/" {
		t.Fatalf("bad stream: %+v", s)
	}
	// pinned translation 222 must be chosen → server label reflects it.
	if s.Server != "kodik-222" {
		t.Fatalf("server = %q, want kodik-222 (pinned)", s.Server)
	}
	if s.Provider != "kodik-noads" || s.AnimeName != "Frieren" {
		t.Fatalf("bad meta: %+v", s)
	}
}

func TestKodikNoads_FirstWhenNoPinned(t *testing.T) {
	translations := `{"success":true,"data":[{"id":111,"pinned":false},{"id":222,"pinned":false}]}`
	stream := `{"success":true,"data":{"stream_url":"https://cloud.solodcdn.com/m.m3u8","referer":"r"}}`
	srv := kodikStub(t, translations, stream, 200)
	defer srv.Close()
	r := NewKodikNoadsResolver(srv.URL, srv.Client())
	streams, _, err := r.Resolve(context.Background(), "u1", "X", 0, SlotAnchor, "kodik-noads")
	if err != nil || len(streams) != 1 || streams[0].Server != "kodik-111" {
		t.Fatalf("want first translation 111; got streams=%+v err=%v", streams, err)
	}
}

func TestKodikNoads_NoTranslations(t *testing.T) {
	srv := kodikStub(t, `{"success":true,"data":[]}`, ``, 200)
	defer srv.Close()
	r := NewKodikNoadsResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "u1", "X", 0, SlotAnchor, "kodik-noads")
	if err == nil || stage != StageServers {
		t.Fatalf("want servers-stage error on no translations; stage=%v err=%v", stage, err)
	}
}

func TestKodikNoads_EmptyStream(t *testing.T) {
	translations := `{"success":true,"data":[{"id":111,"pinned":true}]}`
	srv := kodikStub(t, translations, `{"success":true,"data":{"stream_url":""}}`, 200)
	defer srv.Close()
	r := NewKodikNoadsResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "u1", "X", 0, SlotAnchor, "kodik-noads")
	if err == nil || stage != StageStream {
		t.Fatalf("want stream-stage error on empty stream_url; stage=%v err=%v", stage, err)
	}
}
