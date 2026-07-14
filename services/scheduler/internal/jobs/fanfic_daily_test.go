package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

func TestFanficDailyJob_PostsEnsureDaily(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	j := NewFanficDailyJob(&config.JobsConfig{FanficServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/internal/fanfic/ensure-daily" {
		t.Errorf("path = %q; want /internal/fanfic/ensure-daily", gotPath)
	}
}

func TestFanficDailyJob_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j := NewFanficDailyJob(&config.JobsConfig{FanficServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("want error on 500, got nil")
	}
}
