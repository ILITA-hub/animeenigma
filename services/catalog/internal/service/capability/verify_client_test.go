package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// TestVerifyClient_SummariesDecodes asserts a healthy 200 response decodes
// into a provider->VerifySummary map keyed by the wire "provider" field.
func TestVerifyClient_SummariesDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/verify/verdicts" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("anime_id"); got != "anime-1" {
			t.Fatalf("anime_id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"anime_id":"anime-1","providers":[
			{"provider":"gogoanime","summary":{"status":"verified","raw":true,"dub_langs":["en"],"hardsub_langs":["en"]},"units":[]},
			{"provider":"kodik","summary":{"status":"partial","raw":false,"dub_langs":[],"hardsub_langs":[]},"units":[]}
		]}}`))
	}))
	defer srv.Close()

	c := NewVerifyClient(srv.URL, true)
	sums := c.Summaries(context.Background(), "anime-1")
	if len(sums) != 2 {
		t.Fatalf("got %d summaries, want 2: %+v", len(sums), sums)
	}
	got, ok := sums["gogoanime"]
	if !ok {
		t.Fatalf("missing gogoanime summary: %+v", sums)
	}
	want := domain.VerifySummary{Status: "verified", Raw: true, DubLangs: []string{"en"}, HardsubLangs: []string{"en"}}
	if got.Status != want.Status || got.Raw != want.Raw || len(got.DubLangs) != 1 || got.DubLangs[0] != "en" {
		t.Fatalf("gogoanime summary = %+v, want %+v", got, want)
	}
	if k := sums["kodik"]; k.Status != "partial" {
		t.Fatalf("kodik summary = %+v", k)
	}
}

// TestVerifyClient_Summaries_ServerError asserts a 500 upstream degrades to a
// nil map (best-effort, never blocks capability assembly).
func TestVerifyClient_Summaries_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewVerifyClient(srv.URL, true)
	if sums := c.Summaries(context.Background(), "anime-1"); sums != nil {
		t.Fatalf("expected nil map on 500, got %+v", sums)
	}
}

// TestVerifyClient_Summaries_Disabled asserts the kill switch (enabled=false)
// short-circuits to nil without making a request.
func TestVerifyClient_Summaries_Disabled(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewVerifyClient(srv.URL, false)
	if sums := c.Summaries(context.Background(), "anime-1"); sums != nil {
		t.Fatalf("expected nil map when disabled, got %+v", sums)
	}
	if called {
		t.Fatal("disabled client must not hit the server")
	}
}

// TestVerifyClient_RawVerdicts_Passthrough asserts RawVerdicts returns the
// exact "data" payload bytes verbatim (the public handler passes them
// through unmodified).
func TestVerifyClient_RawVerdicts_Passthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"anime_id":"a1","providers":[]}}`))
	}))
	defer srv.Close()

	c := NewVerifyClient(srv.URL, true)
	raw, err := c.RawVerdicts(context.Background(), "a1")
	if err != nil {
		t.Fatalf("RawVerdicts error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("raw not valid json: %v (%s)", err, raw)
	}
	if decoded["anime_id"] != "a1" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

// TestVerifyClient_Hint_Posts asserts Hint fires an async POST with the
// expected body; the test server signals completion over a channel since
// Hint is fire-and-forget (returns before the request completes).
func TestVerifyClient_Hint_Posts(t *testing.T) {
	done := make(chan hintPayload, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/verify/hint" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var p hintPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		w.WriteHeader(http.StatusNoContent)
		done <- p
	}))
	defer srv.Close()

	c := NewVerifyClient(srv.URL, true)
	c.Hint("anime-1", "ip:abc123", "visit")

	select {
	case p := <-done:
		if p.AnimeID != "anime-1" || p.Visitor != "ip:abc123" || p.Source != "visit" {
			t.Fatalf("hint payload = %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for hint POST")
	}
}

// TestVerifyClient_Hint_DisabledOrEmptyNoop asserts Hint is a no-op (no
// request fired) when disabled or the required fields are blank.
func TestVerifyClient_Hint_DisabledOrEmptyNoop(t *testing.T) {
	hit := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
	}))
	defer srv.Close()

	NewVerifyClient(srv.URL, false).Hint("anime-1", "ip:abc", "visit")
	NewVerifyClient(srv.URL, true).Hint("", "ip:abc", "visit")
	NewVerifyClient(srv.URL, true).Hint("anime-1", "", "visit")

	select {
	case <-hit:
		t.Fatal("expected no request for disabled/empty-field Hint calls")
	case <-time.After(300 * time.Millisecond):
		// expected: nothing arrived
	}
}

type hintPayload struct {
	AnimeID string `json:"anime_id"`
	Visitor string `json:"visitor"`
	Source  string `json:"source"`
}
