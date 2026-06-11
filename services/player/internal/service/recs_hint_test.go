package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestRecsHintProducer_PostsHint(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/recs/recompute-hint" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewRecsHintProducer(srv.URL, true, logger.Default())
	p.Start()
	p.Hint("u1", "a1")
	p.Stop() // Stop drains the channel before returning

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0]["user_id"] != "u1" || got[0]["anime_id"] != "a1" {
		t.Fatalf("got %v, want one hint u1/a1", got)
	}
}

func TestRecsHintProducer_NilAndDisabledAreNoops(t *testing.T) {
	var p *RecsHintProducer
	p.Hint("u1", "a1") // nil receiver must not panic
	p.Start()
	p.Stop()
	p2 := NewRecsHintProducer("http://recs:8094", false, logger.Default())
	p2.Start()
	p2.Hint("u1", "a1")
	p2.Stop()
}
