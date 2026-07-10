package maintenancegate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnabled_failsOpen(t *testing.T) {
	c := New("http://127.0.0.1:1/", 200*time.Millisecond) // unreachable
	if !c.Enabled(context.Background(), "git_autosync") {
		t.Fatal("unreachable gate must fail open (enabled=true)")
	}
}

func TestEnabled_readsDataEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/maintenance/routines/subtitle_probe" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"enabled":false,"settings":{}}}`))
	}))
	defer srv.Close()
	if New(srv.URL, time.Second).Enabled(context.Background(), "subtitle_probe") {
		t.Fatal("gate enabled=false must be read as false")
	}
}

func TestEnabled_non200_failsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if !New(srv.URL, time.Second).Enabled(context.Background(), "nope") {
		t.Fatal("404 must fail open (enabled=true)")
	}
}

func TestMaxRisk_readsSetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"enabled":true,"settings":{"auto_apply_max_risk":"low"}}}`))
	}))
	defer srv.Close()
	if got := New(srv.URL, time.Second).MaxRisk(context.Background(), "maintenance_bot"); got != "low" {
		t.Fatalf("MaxRisk = %q; want low", got)
	}
}

func TestMaxRisk_non200_empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if got := New(srv.URL, time.Second).MaxRisk(context.Background(), "x"); got != "" {
		t.Fatalf("MaxRisk on 500 = %q; want empty (no cap)", got)
	}
}

func TestPostStatus_sendsBody(t *testing.T) {
	got := make(chan int64, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.ContentLength
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"x"}}`))
	}))
	defer srv.Close()
	New(srv.URL, time.Second).PostStatus(context.Background(), "shikimori_sync", true, "412 updated")
	select {
	case n := <-got:
		if n <= 0 {
			t.Fatal("empty status body")
		}
	case <-time.After(time.Second):
		t.Fatal("status POST never arrived")
	}
}
