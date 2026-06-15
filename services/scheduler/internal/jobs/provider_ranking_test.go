package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

func TestProviderRankingJob_PostsRecompute(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	j := NewProviderRankingJob(&config.JobsConfig{AnalyticsInternalURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/internal/player-ranking/recompute" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestProviderRankingJob_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j := NewProviderRankingJob(&config.JobsConfig{AnalyticsInternalURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("want error on 500, got nil")
	}
}
