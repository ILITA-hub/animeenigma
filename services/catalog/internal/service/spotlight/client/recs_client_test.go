// Workstream hero-spotlight v1.0 Phase 3 — Task 9 (2026-07-17).
//
// RecsClient tests use httptest.NewServer as a fake recs service.
//   - FetchUserRecs → /api/users/recs with JWT forwarded (T-03-05: never
//     log it). Moved here verbatim from player_client_test.go — the routes
//     migrated player→recs on 2026-06-11 and the spotlight client kept
//     calling player:8083, silently degrading personal_pick to trending.
//   - FetchUpcoming → /api/users/recs/upcoming with JWT forwarded (Task 10
//     consumer).
//
// noopLogger / observingLogger are defined in player_client_test.go (same
// package) and reused here.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecsClient_FetchUserRecs_HappyPath_ForwardsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/recs" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer testjwt" {
			t.Errorf("expected Authorization=Bearer testjwt, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// recs wraps via httputil.OK → {success:true, data: RecsEnvelope}.
		_, _ = w.Write([]byte(`{"success":true,"data":{"recs":[{"anime":{"id":"a1"}},{"anime":{"id":"a2"}}],"row_label_key":"recs.upNext","total":2,"cache_hit":false,"generated_at":"2026-05-21T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c := NewRecsClient(srv.URL, srv.Client(), noopLogger())
	got, err := c.FetchUserRecs(context.Background(), "testjwt")
	if err != nil {
		t.Fatalf("FetchUserRecs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 recs, got %d", len(got))
	}
	// Decode embedded anime.id to confirm RawMessage pass-through.
	var anime1 struct{ ID string }
	if err := json.Unmarshal(got[0].Anime, &anime1); err != nil {
		t.Fatalf("decode rec[0].anime: %v", err)
	}
	if anime1.ID != "a1" {
		t.Errorf("expected rec[0].anime.id=a1, got %q", anime1.ID)
	}
}

func TestRecsClient_FetchUserRecs_AnonNoJWT_OmitsAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected NO Authorization header for anon, got %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"recs":[],"row_label_key":"recs.trending","total":0,"cache_hit":false,"generated_at":"2026-05-21T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c := NewRecsClient(srv.URL, srv.Client(), noopLogger())
	got, err := c.FetchUserRecs(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchUserRecs anon: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty recs, got %d items", len(got))
	}
}

func TestRecsClient_FetchUserRecs_5xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewRecsClient(srv.URL, srv.Client(), noopLogger())
	_, err := c.FetchUserRecs(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Errorf("expected 'status=500' in error, got: %v", err)
	}
}

func TestRecsClient_FetchUserRecs_NeverLogsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	log, recorded := observingLogger()
	c := NewRecsClient(srv.URL, srv.Client(), log)
	_, _ = c.FetchUserRecs(context.Background(), "supersecretjwttoken-abc123")

	// Every recorded entry's message + fields combined MUST NOT contain the
	// raw token. Concatenate the full structured-log payload as the test surface.
	for _, e := range recorded.All() {
		full := e.Message
		for _, f := range e.Context {
			full += " " + f.String + " "
			if f.Interface != nil {
				// fmt-style string fallback for non-string fields.
				full += " "
			}
		}
		if strings.Contains(full, "supersecretjwttoken-abc123") {
			t.Fatalf("secret JWT leaked into log line: %q", full)
		}
	}
}

func TestRecsClient_ContextCancellation_Honored(t *testing.T) {
	// Server hangs forever; client ctx is cancelled immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewRecsClient(srv.URL, srv.Client(), noopLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.FetchUserRecs(ctx, "tok")
	if err == nil {
		t.Fatal("expected ctx-cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled in err chain, got: %v", err)
	}
}

func TestRecsClient_DefaultBaseURL(t *testing.T) {
	c := NewRecsClient("", nil, noopLogger())
	if c.BaseURL() != "http://recs:8094" {
		t.Fatalf("default base URL: got %s", c.BaseURL())
	}
}

func TestRecsClient_FetchUpcoming_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/recs/upcoming" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-1" {
			t.Errorf("jwt not forwarded: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"items":[
			{"anime":{"id":"a1","name":"Frieren S2"},"match_score":0.61,
			 "reason":{"kind":"franchise","seed_anime_id":"s1","seed_anime_name":"Frieren","user_score":9}}
		]}}`))
	}))
	defer srv.Close()

	items, err := NewRecsClient(srv.URL, nil, noopLogger()).FetchUpcoming(context.Background(), "jwt-1")
	if err != nil {
		t.Fatalf("FetchUpcoming: %v", err)
	}
	if len(items) != 1 || items[0].MatchScore != 0.61 {
		t.Fatalf("unexpected items: %+v", items)
	}
	if !strings.Contains(string(items[0].Anime), `"id":"a1"`) {
		t.Fatalf("anime payload not forwarded verbatim: %s", items[0].Anime)
	}
}

func TestRecsClient_FetchUpcoming_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := NewRecsClient(srv.URL, nil, noopLogger()).FetchUpcoming(context.Background(), "jwt-1")
	if err == nil {
		t.Fatal("expected error on 502")
	}
}
