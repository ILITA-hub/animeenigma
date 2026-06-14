package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// capturingErrSink records accepted FE errors for assertions.
type capturingErrSink struct {
	mu   sync.Mutex
	recs []WireClientError
	uas  []string
	ips  []string
}

func (c *capturingErrSink) Record(e WireClientError, ua, ipHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recs = append(c.recs, e)
	c.uas = append(c.uas, ua)
	c.ips = append(c.ips, ipHash)
}
func (c *capturingErrSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recs)
}

func postClientErrors(h *ClientErrorHandler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/client-errors", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.RemoteAddr = "203.0.113.9:4444"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestClientErrors_AcceptsBatch(t *testing.T) {
	sink := &capturingErrSink{}
	h := NewClientErrorHandler(sink, "test-salt")

	body := `{"errors":[
	  {"kind":"http","message":"Request failed","url":"/api/anime/x/ae/stream","method":"GET","status":404,"provider":"ae","anime_id":"x"},
	  {"kind":"player","message":"Stream unavailable","provider":"ae","anime_id":"x"}
	],"ctx":{"user_agent":"UA"}}`

	rec := postClientErrors(h, body)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 2 {
		t.Fatalf("expected 2 errors recorded, got %d", sink.count())
	}
	sink.mu.Lock()
	first, ua, ip := sink.recs[0], sink.uas[0], sink.ips[0]
	sink.mu.Unlock()
	if first.Kind != "http" || first.Status != 404 || first.Provider != "ae" {
		t.Fatalf("fields not applied: %+v", first)
	}
	if ua != "UA" {
		t.Fatalf("ua not applied: %q", ua)
	}
	if ip == "" || strings.Contains(ip, "203.0.113.9") {
		t.Fatalf("ip must be hashed, got %q", ip)
	}
}

func TestClientErrors_RejectsBadJSON(t *testing.T) {
	sink := &capturingErrSink{}
	h := NewClientErrorHandler(sink, "salt")
	rec := postClientErrors(h, `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected 0 recorded on bad json, got %d", sink.count())
	}
}

func TestClientErrors_ClampsBatchSize(t *testing.T) {
	sink := &capturingErrSink{}
	h := NewClientErrorHandler(sink, "salt")

	var b strings.Builder
	b.WriteString(`{"errors":[`)
	for i := 0; i < maxClientErrorsPerBatch+25; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"kind":"js","message":"boom"}`)
	}
	b.WriteString(`],"ctx":{}}`)

	rec := postClientErrors(h, b.String())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != maxClientErrorsPerBatch {
		t.Fatalf("expected clamp to %d, got %d", maxClientErrorsPerBatch, sink.count())
	}
}

func TestClientErrors_SkipsEmptyAndTruncates(t *testing.T) {
	sink := &capturingErrSink{}
	h := NewClientErrorHandler(sink, "salt")

	long := strings.Repeat("z", maxMessageLen+200)
	body := `{"errors":[
	  {"kind":"js"},
	  {"kind":"js","message":"` + long + `"}
	],"ctx":{}}`

	rec := postClientErrors(h, body)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	// The empty (no message, no url) error is dropped; only the long one stays.
	if sink.count() != 1 {
		t.Fatalf("expected 1 recorded (empty dropped), got %d", sink.count())
	}
	sink.mu.Lock()
	got := sink.recs[0].Message
	sink.mu.Unlock()
	if len([]rune(got)) != maxMessageLen {
		t.Fatalf("message not truncated to %d runes, got %d", maxMessageLen, len([]rune(got)))
	}
}

func TestKindLabel_Whitelist(t *testing.T) {
	for _, k := range []string{"js", "unhandledrejection", "vue", "http", "player", "suppressed", "cap"} {
		if kindLabel(k) != k {
			t.Fatalf("known kind %q remapped to %q", k, kindLabel(k))
		}
	}
	if kindLabel("forged-kind-xyz") != "other" {
		t.Fatalf("unknown kind must map to 'other', got %q", kindLabel("forged-kind-xyz"))
	}
}
