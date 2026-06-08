package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type capturingSink struct {
	mu     sync.Mutex
	events []domain.Event
}

func (c *capturingSink) Enqueue(e domain.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return true
}
func (c *capturingSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestCollect_AcceptsBatch(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "test-salt")

	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"pageview","path":"/pricing"},
	  {"event_type":"click","path":"/pricing","el_selector":"button#buy"}
	],"ctx":{"user_agent":"UA","screen_w":1920,"screen_h":1080}}`

	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", sink.count())
	}
	sink.mu.Lock()
	first := sink.events[0]
	sink.mu.Unlock()
	if first.AnonymousID != "a1" || first.SessionID != "s1" {
		t.Fatalf("envelope fields not applied: %+v", first)
	}
	if first.IPHash == "" || strings.Contains(first.IPHash, "203.0.113.7") {
		t.Fatalf("ip must be hashed, got %q", first.IPHash)
	}
	if first.UserAgent != "UA" || first.ScreenW != 1920 {
		t.Fatalf("ctx not applied: %+v", first)
	}
	if first.EventID == "" {
		t.Fatal("event_id must be assigned")
	}
}

// TestCollectMapsFERegisterFields proves AR-FE-01: an FE-originated beacon
// row lands in the register with its own source/operation/target populated,
// not silently mis-tagged as a backend (source='be') clickstream row.
func TestCollectMapsFERegisterFields(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "test-salt")

	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"custom","event_name":"fe_call","source":"fe",
	   "operation":"catalog GET /api/anime/{id}","target":"/api/anime/123",
	   "target_kind":"route"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	sink.mu.Lock()
	ev := sink.events[0]
	sink.mu.Unlock()

	if ev.Source != "fe" {
		t.Fatalf("Source not mapped: want %q, got %q", "fe", ev.Source)
	}
	if ev.Operation == "" {
		t.Fatalf("Operation must be populated, got empty")
	}
	if ev.Target == "" {
		t.Fatalf("Target must be populated, got empty")
	}
	if ev.TargetKind != "route" {
		t.Fatalf("TargetKind not mapped: got %q", ev.TargetKind)
	}
}

// TestFERUMRowCarriesZeroBytes proves AR-FE-03: an FE RUM row carries its
// requests+duration_ms, is tagged accuracy='approximate', and is structurally
// byte-poor (BytesIn==0 && BytesOut==0) so it can never contaminate
// authoritative byte aggregations.
func TestFERUMRowCarriesZeroBytes(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "test-salt")

	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"custom","event_name":"fe_rum","source":"fe_rum",
	   "target":"cdn.example.com","target_kind":"host",
	   "requests":4,"duration_ms":320}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	sink.mu.Lock()
	ev := sink.events[0]
	sink.mu.Unlock()

	if ev.Source != "fe_rum" {
		t.Fatalf("Source: want %q, got %q", "fe_rum", ev.Source)
	}
	if ev.Accuracy != "approximate" {
		t.Fatalf("Accuracy: want %q, got %q", "approximate", ev.Accuracy)
	}
	if ev.Requests != 4 {
		t.Fatalf("Requests: want 4, got %d", ev.Requests)
	}
	if ev.DurationMS != 320 {
		t.Fatalf("DurationMS: want 320, got %d", ev.DurationMS)
	}
	// CRITICAL byte-poverty invariant: an fe_rum row must never carry bytes.
	if ev.BytesIn != 0 || ev.BytesOut != 0 {
		t.Fatalf("fe_rum row must be byte-poor: BytesIn=%d BytesOut=%d", ev.BytesIn, ev.BytesOut)
	}
}

// TestCollectRejectsForgedSource proves T-04-01: a forged beacon source that
// is not in the {fe, fe_rum} whitelist is normalized to empty so it is never
// written as-is (and the store's source='be' default keeps BE rows
// authoritative).
func TestCollectRejectsForgedSource(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "test-salt")

	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"custom","event_name":"forged","source":"evil",
	   "operation":"x","target":"y"},
	  {"event_type":"custom","event_name":"forged_be","source":"be"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, ev := range sink.events {
		if ev.Source == "evil" || ev.Source == "be" {
			t.Fatalf("forged source must be normalized to empty, got %q", ev.Source)
		}
		if ev.Source != "" {
			t.Fatalf("non-whitelisted source must normalize to empty, got %q", ev.Source)
		}
	}
}

func TestCollect_RejectsMalformed(t *testing.T) {
	h := NewCollectHandler(&capturingSink{}, "salt")
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCollect_SkipsInvalidEventsButAcceptsRest(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "salt")
	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"pageview","path":"/"},
	  {"event_type":"bogus","path":"/"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 valid event, got %d", sink.count())
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/anime/3b9f1c2d-4e5a-6b7c-8d9e-0f1a2b3c4d5e": "/anime/{id}",
		"/anime/12345":                "/anime/{id}",
		"/anime/12345/episode/7":      "/anime/{id}/episode/{id}",
		"/home":                       "/home",
		"/anime/12345?tab=info#top":   "/anime/{id}",
		"":                            "",
		"/":                           "/",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClickstreamOperation(t *testing.T) {
	cases := []struct {
		etype, path, elTag, name string
		want                     string
	}{
		{"pageview", "/anime/12345", "", "", "pageview /anime/{id}"},
		{"heartbeat", "/home", "", "", "heartbeat /home"},
		{"click", "", "BUTTON", "", "click button"},
		{"click", "", "", "", "click"},
		{"identify", "", "", "", "identify"},
		{"custom", "", "", "video_play", "custom video_play"},
		{"custom", "", "", "", "custom"},
		{"", "/x", "", "", ""},
		{"weird", "", "", "", "weird"},
	}
	for _, c := range cases {
		if got := clickstreamOperation(c.etype, c.path, c.elTag, c.name); got != c.want {
			t.Errorf("clickstreamOperation(%q,%q,%q,%q) = %q, want %q", c.etype, c.path, c.elTag, c.name, got, c.want)
		}
	}
}

// TestCollect_DerivesClickstreamOperation: an autocapture pageview with no
// explicit register operation gets a meaningful derived dimension (never empty).
func TestCollect_DerivesClickstreamOperation(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "salt")
	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"pageview","path":"/anime/3b9f1c2d-4e5a-6b7c-8d9e-0f1a2b3c4d5e"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	if got := sink.events[0].Operation; got != "pageview /anime/{id}" {
		t.Fatalf("derived Operation = %q, want %q", got, "pageview /anime/{id}")
	}
}

// TestCollect_ExplicitOperationWins: an event carrying an explicit operation is
// not overridden by the derivation.
func TestCollect_ExplicitOperationWins(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "salt")
	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"custom","event_name":"x","source":"fe","operation":"explicit op"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	if got := sink.events[0].Operation; got != "explicit op" {
		t.Fatalf("Operation = %q, want explicit op", got)
	}
}
