package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type fixedDegradationLevel int

func (l fixedDegradationLevel) Level() int { return int(l) }

func TestZundamonStatusFiltersExactSpeaker(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = w.Write([]byte(`"0.25.2"`))
		case "/speakers":
			_, _ = w.Write([]byte(`[
				{"name":"四国めたん","styles":[{"id":2,"name":"ノーマル"}]},
				{"name":"ずんだもん","styles":[{"id":3,"name":"ノーマル"},{"id":1,"name":"あまあま"}]}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	h, err := NewZundamonHandler(upstream.URL, fixedDegradationLevel(0), logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.Status(rec, httptest.NewRequest(http.MethodGet, "/api/zundamon/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Version string          `json:"version"`
		Styles  []VoicevoxStyle `json:"styles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != "0.25.2" || len(body.Styles) != 2 || body.Styles[0].ID != 3 {
		t.Fatalf("unexpected filtered status: %+v", body)
	}
}

func TestZundamonSynthesisUsesValidatedStyleAndVoiceSettings(t *testing.T) {
	querySeen := make(chan map[string]any, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/speakers":
			_, _ = w.Write([]byte(`[{"name":"ずんだもん","styles":[{"id":3,"name":"ノーマル"}]}]`))
		case "/audio_query":
			if r.URL.Query().Get("speaker") != "3" || r.URL.Query().Get("text") != "こんにちはなのだ" {
				t.Errorf("unexpected audio_query params: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"speedScale":1,"pitchScale":0,"kana":"test"}`))
		case "/synthesis":
			var query map[string]any
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				t.Error(err)
			}
			querySeen <- query
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("RIFF-zundamon"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	h, err := NewZundamonHandler(upstream.URL, fixedDegradationLevel(0), logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	body := `{"text":"こんにちはなのだ","styleId":3,"speedScale":1.2,"pitchScale":0.05}`
	rec := httptest.NewRecorder()
	h.Synthesize(rec, httptest.NewRequest(http.MethodPost, "/api/zundamon/synthesis", strings.NewReader(body)))
	if rec.Code != http.StatusOK || rec.Body.String() != "RIFF-zundamon" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	query := <-querySeen
	if query["speedScale"] != 1.2 || query["pitchScale"] != 0.05 {
		t.Fatalf("voice settings not applied: %+v", query)
	}
}

func TestZundamonShedsAtElevatedWithoutCallingEngine(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h, err := NewZundamonHandler(upstream.URL, fixedDegradationLevel(1), logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.Synthesize(rec, httptest.NewRequest(http.MethodPost, "/api/zundamon/synthesis", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503", rec.Code)
	}
	if called {
		t.Fatal("engine was called despite elevated governor level")
	}
}

func TestZundamonRejectsAnotherSpeakersStyle(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"ずんだもん","styles":[{"id":3,"name":"ノーマル"}]}]`))
	}))
	defer upstream.Close()

	h, err := NewZundamonHandler(upstream.URL, fixedDegradationLevel(0), logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	body := `{"text":"test","styleId":2,"speedScale":1,"pitchScale":0}`
	h.Synthesize(rec, httptest.NewRequest(http.MethodPost, "/api/zundamon/synthesis", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_style") {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}
