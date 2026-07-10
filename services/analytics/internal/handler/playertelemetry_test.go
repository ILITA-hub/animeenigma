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

// TestPlayerTelemetry_PerEventAnimeID mirrors the REAL frontend wire shape:
// playerTelemetry.ts flushes { events: [...] } with NO top-level envelope —
// anime_id/episode/audio/lang ride on each event. This is the regression test
// for the bug where every analytics.events row had a NULL anime_id (and
// episode=0) because the handler only read the (never-sent) envelope fields.
func TestPlayerTelemetry_PerEventAnimeID(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	// No envelope-level anime_id/episode — exactly what the FE sends. A single
	// batch can also span two different episodes (buffered across a switch).
	body := `{"events":[
		{"kind":"resolve","provider":"kodik","anime_id":"uuid-1","episode":7,"audio":"sub","lang":"ru","latency_ms":1200,"outcome":"ok","reached_playback":true},
		{"kind":"stall","provider":"allanime","anime_id":"uuid-2","episode":8,"stall_ms":400}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if sink.count() != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", sink.count())
	}

	row0 := sink.at(0)
	if row0.AnimeID != "uuid-1" {
		t.Errorf("row0 AnimeID = %q, want uuid-1 (per-event, no envelope)", row0.AnimeID)
	}
	var p0 map[string]any
	if err := json.Unmarshal([]byte(row0.Properties), &p0); err != nil {
		t.Fatalf("row0 Properties invalid JSON: %v", err)
	}
	if p0["episode"] != float64(7) {
		t.Errorf("row0 Properties episode = %v, want 7", p0["episode"])
	}
	if p0["audio"] != "sub" || p0["lang"] != "ru" {
		t.Errorf("row0 audio/lang = %v/%v, want sub/ru", p0["audio"], p0["lang"])
	}

	row1 := sink.at(1)
	if row1.AnimeID != "uuid-2" {
		t.Errorf("row1 AnimeID = %q, want uuid-2 (distinct per-event anime in same batch)", row1.AnimeID)
	}
	var p1 map[string]any
	if err := json.Unmarshal([]byte(row1.Properties), &p1); err != nil {
		t.Fatalf("row1 Properties invalid JSON: %v", err)
	}
	if p1["episode"] != float64(8) {
		t.Errorf("row1 Properties episode = %v, want 8", p1["episode"])
	}
}

// TestPlayerTelemetry_EnvelopeFallback verifies the envelope still works when an
// event omits anime_id/episode (smoke-test / older-sender compatibility).
func TestPlayerTelemetry_EnvelopeFallback(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	body := `{"anime_id":"env-anime","episode":5,"audio":"dub","lang":"en","events":[
		{"kind":"resolve","provider":"miruro","latency_ms":800,"outcome":"ok","reached_playback":true}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	row0 := sink.at(0)
	if row0.AnimeID != "env-anime" {
		t.Errorf("row0 AnimeID = %q, want env-anime (envelope fallback)", row0.AnimeID)
	}
	var p0 map[string]any
	_ = json.Unmarshal([]byte(row0.Properties), &p0)
	if p0["episode"] != float64(5) {
		t.Errorf("row0 episode = %v, want 5 (envelope fallback)", p0["episode"])
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

// TestPlayerTelemetry_PlaybackStartRejected: the browser-veto kind maps to its
// own effect_kind and carries the DOMException name in properties.error_kind.
func TestPlayerTelemetry_PlaybackStartRejected(t *testing.T) {
	sink := &effectSink{}
	h := NewPlayerTelemetryHandler(sink)

	body := `{"events":[{"kind":"playback_start_rejected","provider":"kodik","anime_id":"a1","episode":6,"error_kind":"NotAllowedError","audio":"sub","lang":"ru"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 event enqueued, got %d", sink.count())
	}
	row := sink.events[0]
	if row.EffectKind != "player_playback_start_rejected" {
		t.Errorf("EffectKind = %q, want player_playback_start_rejected", row.EffectKind)
	}
	if row.Target != "kodik" {
		t.Errorf("Target = %q, want kodik", row.Target)
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(row.Properties), &props); err != nil {
		t.Fatalf("bad properties JSON: %v", err)
	}
	if props["error_kind"] != "NotAllowedError" {
		t.Errorf("properties.error_kind = %v, want NotAllowedError", props["error_kind"])
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
