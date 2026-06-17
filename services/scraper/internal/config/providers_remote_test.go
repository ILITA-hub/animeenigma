package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoadProvidersRemote_ParsesAndBuilds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/scraper/providers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"allanime","enabled":true,"group":"en","scraper_operated":true,"supports_sub":true,"supports_dub":true,"sub_delivery":"hard","quality_ceiling":"1080p","preference_weight":90},
			{"name":"animepahe","enabled":false,"group":"en","scraper_operated":true,"supports_sub":true,"supports_dub":true,"sub_delivery":"hard","preference_weight":30}
		]}}`))

	}))
	defer srv.Close()

	pc, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err != nil {
		t.Fatalf("LoadProvidersRemote: %v", err)
	}
	if pc.Source != "remote" {
		t.Errorf("Source = %q, want remote", pc.Source)
	}
	if !pc.IsEnabled("allanime") || pc.IsEnabled("animepahe") {
		t.Errorf("enabled: allanime=%v animepahe=%v want true/false", pc.IsEnabled("allanime"), pc.IsEnabled("animepahe"))
	}
	all := pc.Meta("allanime")
	if !all.SupportsDub || all.PreferenceWeight != 90 || all.Group != "en" {
		t.Errorf("allanime meta wrong: %+v", all)
	}
}

func TestLoadProvidersRemote_RejectsUnknownProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[{"name":"bogus","status":"enabled","scraper_operated":true}]}}`))
	}))
	defer srv.Close()
	_, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err == nil {
		t.Fatal("expected error for unknown provider name, got nil")
	}
}

func TestLoadProvidersRemote_SkipsNonScraperRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Roster now holds first-party/legacy rows (scraper_operated:false) that
		// are NOT in KnownProviders. They must be silently skipped, not rejected.
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":[
			{"name":"gogoanime","status":"enabled","group":"en","scraper_operated":true,"supports_sub":true},
			{"name":"ae","status":"enabled","group":"firstparty","scraper_operated":false},
			{"name":"kodik","status":"enabled","group":"ru","scraper_operated":false}
		]}}`))
	}))
	defer srv.Close()

	pc, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second)
	if err != nil {
		t.Fatalf("LoadProvidersRemote should skip non-scraper rows, got: %v", err)
	}
	if !pc.IsEnabled("gogoanime") {
		t.Error("gogoanime should be present + enabled")
	}
	if _, ok := pc.load()["ae"]; ok {
		t.Error("ae (scraper_operated=false) must not enter the scraper roster")
	}
	if _, ok := pc.load()["kodik"]; ok {
		t.Error("kodik (scraper_operated=false) must not enter the scraper roster")
	}
}

func TestLoadProvidersRemote_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := LoadProvidersRemote(context.Background(), srv.URL, srv.Client(), 2*time.Second); err == nil {
		t.Fatal("expected error on non-200 response, got nil")
	}
}
