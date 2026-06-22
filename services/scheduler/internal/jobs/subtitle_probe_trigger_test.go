package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

func TestSubtitleProbeTrigger_PostsCatalog(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	j := NewSubtitleProbeTriggerJob(&config.JobsConfig{CatalogServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotPath != "/internal/subtitle-probe/run" {
		t.Fatalf("path = %q; want /internal/subtitle-probe/run", gotPath)
	}
}

func TestSubtitleProbeTrigger_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j := NewSubtitleProbeTriggerJob(&config.JobsConfig{CatalogServiceURL: srv.URL}, nil)
	if err := j.Run(context.Background()); err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
