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
