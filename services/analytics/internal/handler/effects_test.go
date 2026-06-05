package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type effectSink struct {
	mu     sync.Mutex
	events []domain.Event
}

func (c *effectSink) Enqueue(e domain.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return true
}
func (c *effectSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}
func (c *effectSink) at(i int) domain.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events[i]
}

// TestEffectsIngest: a JSON batch of effect rows is enqueued as domain.Events
// with EffectKind="egress" and host/status/bytes/duration populated.
func TestEffectsIngest(t *testing.T) {
	sink := &effectSink{}
	h := NewEffectsHandler(sink)

	body := `{"effects":[
	  {"origin":"be","operation":"catalog GET /api/anime/{id}","target_kind":"host","target":"shikimori.one","status":200,"bytes_in":1234,"bytes_out":56,"duration_ms":42,"user_id":"u1"},
	  {"origin":"be","operation":"scraper GET stream","target_kind":"host","target":"allanime.day","status":404,"bytes_in":0,"bytes_out":10,"duration_ms":7}
	]}`

	req := httptest.NewRequest(http.MethodPost, "/internal/effects", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 2 {
		t.Fatalf("expected 2 effects enqueued, got %d", sink.count())
	}
	first := sink.at(0)
	if first.EffectKind != "egress" {
		t.Fatalf("EffectKind = %q, want egress", first.EffectKind)
	}
	if first.Target != "shikimori.one" || first.TargetKind != "host" {
		t.Fatalf("target not applied: %+v", first)
	}
	if first.BytesIn != 1234 || first.BytesOut != 56 || first.DurationMS != 42 {
		t.Fatalf("measures not applied: %+v", first)
	}
	if first.Origin != "be" || first.Operation != "catalog GET /api/anime/{id}" {
		t.Fatalf("dimensions not applied: %+v", first)
	}
	if first.UserID != "u1" {
		t.Fatalf("user_id not applied: %+v", first)
	}
	if first.Source != "be" || first.Accuracy != "exact" {
		t.Fatalf("source/accuracy defaults missing: %+v", first)
	}
	if first.EventID == "" || first.ReceivedAt.IsZero() {
		t.Fatalf("event_id/received_at not assigned: %+v", first)
	}
	if first.Requests != 1 {
		t.Fatalf("Requests should default to 1, got %d", first.Requests)
	}
}

// TestEffectsIngest_RejectsMalformed: bad JSON → 400.
func TestEffectsIngest_RejectsMalformed(t *testing.T) {
	h := NewEffectsHandler(&effectSink{})
	req := httptest.NewRequest(http.MethodPost, "/internal/effects", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestEffectsIngest_CapsArrayLength: an over-bound array is capped, never
// enqueuing more than maxEffectBatch rows.
func TestEffectsIngest_CapsArrayLength(t *testing.T) {
	sink := &effectSink{}
	h := NewEffectsHandler(sink)

	var b strings.Builder
	b.WriteString(`{"effects":[`)
	for i := 0; i < maxEffectBatch+50; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"origin":"be","target_kind":"host","target":"h","status":200}`)
	}
	b.WriteString(`]}`)

	req := httptest.NewRequest(http.MethodPost, "/internal/effects", strings.NewReader(b.String()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() > maxEffectBatch {
		t.Fatalf("array length not capped: enqueued %d > %d", sink.count(), maxEffectBatch)
	}
}

// TestEffectsIngest_TruncatesOversizeBody: a body over the 256KB LimitReader
// cap is truncated, so a giant payload cannot be parsed whole → 400 (truncated
// JSON is invalid).
func TestEffectsIngest_TruncatesOversizeBody(t *testing.T) {
	sink := &effectSink{}
	h := NewEffectsHandler(sink)

	// Build a body well over 256KB; truncation will corrupt the trailing JSON.
	var b strings.Builder
	b.WriteString(`{"effects":[`)
	for b.Len() < 300*1024 {
		b.WriteString(`{"origin":"be","target_kind":"host","target":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":200},`)
	}
	b.WriteString(`{"origin":"be","target":"last"}]}`)

	req := httptest.NewRequest(http.MethodPost, "/internal/effects", strings.NewReader(b.String()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Truncated JSON cannot parse → 400. The key assertion is that we did NOT
	// ingest the full 300KB+ payload's worth of rows unbounded.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on truncated oversize body, got %d", rec.Code)
	}
}
