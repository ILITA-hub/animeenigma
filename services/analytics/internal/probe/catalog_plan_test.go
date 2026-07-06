package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/providers/probe-plan" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"plan":[{"provider":"gogoanime","sample_size":3,"fail_fast":false}]}}`))
	}))
	defer srv.Close()

	entries, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("FetchPlan returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Provider != "gogoanime" {
		t.Errorf("expected provider=gogoanime, got %q", e.Provider)
	}
	if e.SampleSize != 3 {
		t.Errorf("expected sample_size=3, got %d", e.SampleSize)
	}
	if e.FailFast != false {
		t.Errorf("expected fail_fast=false, got %v", e.FailFast)
	}
}

func TestFetchPlanEmptyPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"plan":[]}}`))
	}))
	defer srv.Close()

	entries, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("FetchPlan returned error on empty plan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestFetchPlanNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestFetchPlanDecodesEngine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"plan":[
			{"provider":"miruro","sample_size":1,"fail_fast":true,"engine":"browser"},
			{"provider":"allanime","sample_size":3,"fail_fast":false,"engine":"http"}]}}`))
	}))
	defer srv.Close()

	entries, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("FetchPlan: %v", err)
	}
	if len(entries) != 2 || entries[0].Engine != "browser" || entries[1].Engine != "http" {
		t.Fatalf("engine not decoded: %+v", entries)
	}
}

func TestPostVerdict(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/providers/probe-result" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"provider":"gogoanime","policy":"active","health":"healthy"}}`))
	}))
	defer srv.Close()

	err := PostVerdict(context.Background(), srv.URL, srv.Client(), "gogoanime", false, "status_403", nil)
	if err != nil {
		t.Fatalf("PostVerdict returned error: %v", err)
	}

	var posted map[string]any
	if err := json.Unmarshal(capturedBody, &posted); err != nil {
		t.Fatalf("captured body is not valid JSON: %v", err)
	}
	if posted["provider"] != "gogoanime" {
		t.Errorf("expected provider=gogoanime in body, got %v", posted["provider"])
	}
	if posted["pass"] != false {
		t.Errorf("expected pass=false in body, got %v", posted["pass"])
	}
	if posted["reason"] != "status_403" {
		t.Errorf("expected reason=status_403 in body, got %v", posted["reason"])
	}
	if _, ok := posted["metrics"]; ok {
		t.Errorf("expected metrics to be omitted when nil, got %v", posted["metrics"])
	}
}

func TestPostVerdictNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	err := PostVerdict(context.Background(), srv.URL, srv.Client(), "gogoanime", true, "", nil)
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestPostVerdictIncludesMetrics(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tm := &TickMetrics{At: "2026-07-06T00:00:00Z", Pass: true, ProviderUsed: "miruro", WarmupMs: 9800, ResolveMs: 1900, ThroughputKbps: 5400, CDNHost: "kwik.cx"}
	if err := PostVerdict(context.Background(), srv.URL, srv.Client(), "miruro", true, "", tm); err != nil {
		t.Fatalf("PostVerdict: %v", err)
	}
	m, ok := gotBody["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics missing from body: %+v", gotBody)
	}
	if m["cdn_host"] != "kwik.cx" || m["warmup_ms"].(float64) != 9800 {
		t.Fatalf("bad metrics payload: %+v", m)
	}
}
