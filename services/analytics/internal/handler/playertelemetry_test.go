package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPlayerTelemetry verifies the /api/analytics/player-events handler:
// - 204 status on a valid 3-event batch
// - exactly 3 events enqueued
// - row 0 (resolve ok): EffectKind, Target, TargetKind, AnimeID, DurationMS, Source, EventType, Properties
// - row 2 (stall): EffectKind, DurationMS
func TestPlayerTelemetry(t *testing.T) {
	sink := &effectSink{} // reuse from effects_test.go (same package)
	h := NewPlayerTelemetryHandler(sink)

	body := `{
		"anime_id": "a1",
		"episode": 3,
		"audio": "sub",
		"lang": "ru",
		"events": [
			{
				"kind":    "resolve",
				"provider": "kodik",
				"latency_ms": 1200,
				"outcome": "ok",
				"reached_playback": true
			},
			{
				"kind":    "resolve",
				"provider": "animelib",
				"latency_ms": 900,
				"outcome": "fail",
				"error_kind": "timeout",
				"reached_playback": false
			},
			{
				"kind":     "stall",
				"provider": "kodik",
				"stall_ms": 350
			}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if sink.count() != 3 {
		t.Fatalf("expected 3 events enqueued, got %d", sink.count())
	}

	// row 0: resolve ok
	row0 := sink.at(0)
	if row0.EffectKind != "player_resolve" {
		t.Errorf("row0 EffectKind = %q, want player_resolve", row0.EffectKind)
	}
	if row0.Target != "kodik" {
		t.Errorf("row0 Target = %q, want kodik", row0.Target)
	}
	if row0.TargetKind != "provider" {
		t.Errorf("row0 TargetKind = %q, want provider", row0.TargetKind)
	}
	if row0.AnimeID != "a1" {
		t.Errorf("row0 AnimeID = %q, want a1", row0.AnimeID)
	}
	if row0.DurationMS != 1200 {
		t.Errorf("row0 DurationMS = %d, want 1200", row0.DurationMS)
	}
	if row0.Source != "fe" {
		t.Errorf("row0 Source = %q, want fe", row0.Source)
	}
	if string(row0.EventType) != "player" {
		t.Errorf("row0 EventType = %q, want player", row0.EventType)
	}
	if row0.Requests != 1 {
		t.Errorf("row0 Requests = %d, want 1", row0.Requests)
	}

	// Properties must contain outcome and reached_playback
	var props map[string]any
	if err := json.Unmarshal([]byte(row0.Properties), &props); err != nil {
		t.Fatalf("row0 Properties is invalid JSON: %v", err)
	}
	if props["outcome"] != "ok" {
		t.Errorf("row0 Properties outcome = %v, want ok", props["outcome"])
	}
	if props["reached_playback"] != true {
		t.Errorf("row0 Properties reached_playback = %v, want true", props["reached_playback"])
	}

	// row 2: stall
	row2 := sink.at(2)
	if row2.EffectKind != "player_stall" {
		t.Errorf("row2 EffectKind = %q, want player_stall", row2.EffectKind)
	}
	if row2.DurationMS != 350 {
		t.Errorf("row2 DurationMS = %d, want 350", row2.DurationMS)
	}
}

// TestPlayerTelemetry_SkipsEmptyProvider: events with empty provider are dropped.
func TestPlayerTelemetry_SkipsEmptyProvider(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	body := `{"anime_id":"a1","events":[{"kind":"resolve","provider":"","latency_ms":100}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected 0 events (empty provider dropped), got %d", sink.count())
	}
}

// TestPlayerTelemetry_SkipsUnknownKind: events with kind not in {resolve,stall} are dropped.
func TestPlayerTelemetry_SkipsUnknownKind(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	body := `{"anime_id":"a1","events":[{"kind":"unknown","provider":"kodik","latency_ms":100}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 0 {
		t.Fatalf("expected 0 events (unknown kind dropped), got %d", sink.count())
	}
}

// TestPlayerTelemetry_RejectsMalformed: bad JSON → 400.
func TestPlayerTelemetry_RejectsMalformed(t *testing.T) {
	h := NewPlayerTelemetryHandler(&effectSink{})
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestPlayerTelemetry_CapsAtHundred: arrays over 100 entries are capped.
func TestPlayerTelemetry_CapsAtHundred(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	var b strings.Builder
	b.WriteString(`{"anime_id":"a1","events":[`)
	for i := 0; i < 150; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"kind":"resolve","provider":"kodik","latency_ms":100}`)
	}
	b.WriteString(`]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(b.String()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() > 100 {
		t.Fatalf("batch not capped: enqueued %d > 100", sink.count())
	}
}
