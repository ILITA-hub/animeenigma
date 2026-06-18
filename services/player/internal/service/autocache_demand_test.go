package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// TestDemandProducer_PostsDemand verifies the outbound body carries the
// mal_id/episode/reason and hits the library autocache demand endpoint.
func TestDemandProducer_PostsDemand(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/library/autocache/demand" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewDemandProducer(srv.URL, true, logger.Default())
	p.Start()
	p.Want("57466", 13, "next_ep", nil, nil)
	p.Stop() // Stop drains the channel before returning

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d demands, want 1: %v", len(got), got)
	}
	if got[0]["mal_id"] != "57466" {
		t.Errorf("mal_id = %v, want 57466", got[0]["mal_id"])
	}
	// JSON numbers decode to float64
	if ep, ok := got[0]["episode"].(float64); !ok || int(ep) != 13 {
		t.Errorf("episode = %v, want 13", got[0]["episode"])
	}
	if got[0]["reason"] != "next_ep" {
		t.Errorf("reason = %v, want next_ep", got[0]["reason"])
	}
}

// TestDemandProducer_NilAndDisabledAreNoops verifies nil/!enabled producers
// no-op without panicking and never POST.
func TestDemandProducer_NilAndDisabledAreNoops(t *testing.T) {
	var posted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var p *DemandProducer
	p.Want("57466", 13, "next_ep", nil, nil) // nil receiver must not panic
	p.Start()
	p.Stop()

	p2 := NewDemandProducer(srv.URL, false, logger.Default())
	p2.Start()
	p2.Want("57466", 13, "next_ep", nil, nil)
	p2.Stop()

	if posted {
		t.Fatalf("disabled/nil producer must not POST")
	}
}

// TestDemandProducer_DropOnFull verifies a full channel drops without panicking.
func TestDemandProducer_DropOnFull(t *testing.T) {
	// Construct but do NOT Start() — no worker drains the channel, so after
	// demandChanCap sends the buffer is full and further Want calls drop.
	p := NewDemandProducer("http://library:8089", true, logger.Default())
	for i := 0; i < demandChanCap+50; i++ {
		p.Want("57466", i, "next_ep", nil, nil) // must never block or panic
	}
}

// TestDemandProducer_Non2xxDoesNotError verifies a non-2xx upstream does not
// crash the worker (the caller is fire-and-forget; send() swallows the error).
func TestDemandProducer_Non2xxDoesNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewDemandProducer(srv.URL, true, logger.Default())
	p.Start()
	p.Want("57466", 13, "next_ep", nil, nil)
	p.Stop() // must drain and return cleanly despite the 500
}
