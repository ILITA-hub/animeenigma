package roster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const payload = `{"success":true,"data":{"providers":[
	{"name":"gogoanime","group":"en","status":"enabled","scraper_operated":true},
	{"name":"kodik-noads","group":"ru","status":"enabled","scraper_operated":false},
	{"name":"animefever","group":"en","status":"disabled","scraper_operated":true}
]}}`

func TestClient_FetchKnownAndTTL(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Minute)
	if !c.Known("gogoanime") || !c.Known("KODIK-NOADS") { // case-insensitive
		t.Fatal("roster rows must be Known")
	}
	if !c.Known("animefever") {
		t.Fatal("disabled tombstone rows stay Known (legacy events keep recording)")
	}
	if c.Known("nosuch") {
		t.Fatal("unknown name must not be Known")
	}
	c.Known("gogoanime")
	if hits != 1 {
		t.Fatalf("TTL cache must serve repeat lookups from memory, got %d fetches", hits)
	}
	if got := len(c.Rows(context.Background())); got != 3 {
		t.Fatalf("Rows() = %d rows, want 3", got)
	}
}

func TestClient_FallbackSnapshotWhenCatalogDown(t *testing.T) {
	c := New("http://127.0.0.1:1", time.Minute) // unreachable
	// Cold-start fallback: the embedded snapshot must cover the live roster.
	for _, name := range []string{"gogoanime", "kodik-noads", "ae", "hanime", "animejoy-sibnet"} {
		if !c.Known(name) {
			t.Fatalf("embedded fallback snapshot missing %q", name)
		}
	}
}

func TestClient_LastGoodSurvivesOutage(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(payload))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Millisecond) // force refetch
	if !c.Known("gogoanime") {
		t.Fatal("initial fetch")
	}
	up = false
	time.Sleep(5 * time.Millisecond)
	if !c.Known("gogoanime") {
		t.Fatal("last-good roster must survive a catalog outage")
	}
}
