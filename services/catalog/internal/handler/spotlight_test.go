package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
	"go.uber.org/zap"
)

// testLogger returns a no-op *logger.Logger so tests don't pollute output.
// Pattern mirrors services/catalog/internal/service/spotlight/cards/fakes_test.go.
func testLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// fakeAggregator implements the `aggregator` interface declared inside
// spotlight.go. Returns whatever (resp, err) tuple the test configures.
type fakeAggregator struct {
	resp *spotlight.Response
	err  error
}

func (f *fakeAggregator) Resolve(_ context.Context, _ *string) (*spotlight.Response, error) {
	return f.resp, f.err
}

// TestSpotlightHandler_Get_Envelope verifies the success path emits the
// bare {cards, generated_at} envelope — NOT the libs/httputil
// {success, data} wrapper. This is DELIBERATE DIVERGENCE 3.
func TestSpotlightHandler_Get_Envelope(t *testing.T) {
	fake := &fakeAggregator{
		resp: &spotlight.Response{
			Cards: []spotlight.Card{
				{Type: "anime_of_day", Data: nil},
				{Type: "random_tail", Data: nil},
			},
			GeneratedAt: "2026-05-21T00:00:00Z",
		},
	}
	h := NewSpotlightHandler(fake, true, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	bodyBytes, _ := io.ReadAll(w.Body)
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, string(bodyBytes))
	}

	// Must contain top-level "cards" and "generated_at".
	if _, ok := body["cards"]; !ok {
		t.Errorf("body missing top-level 'cards' key; got keys=%v", topKeys(body))
	}
	if _, ok := body["generated_at"]; !ok {
		t.Errorf("body missing top-level 'generated_at' key; got keys=%v", topKeys(body))
	}
	// Must NOT contain libs/httputil envelope keys.
	if _, ok := body["success"]; ok {
		t.Errorf("body has 'success' key — handler must NOT use httputil.OK envelope")
	}
	if _, ok := body["data"]; ok {
		t.Errorf("body has 'data' key — handler must NOT use httputil.OK envelope")
	}

	// Cards length sanity check.
	cards, ok := body["cards"].([]any)
	if !ok {
		t.Fatalf("'cards' is not an array; got %T", body["cards"])
	}
	if len(cards) != 2 {
		t.Errorf("len(cards) = %d, want 2", len(cards))
	}
}

// TestSpotlightHandler_Get_FlagOff_Returns404NoBody verifies HSB-BE-07:
// when SpotlightEnabled=false the handler short-circuits to a bare 404
// with no body. NOT httputil.NotFound (which emits {success:false,error:{...}}).
func TestSpotlightHandler_Get_FlagOff_Returns404NoBody(t *testing.T) {
	fake := &fakeAggregator{
		resp: &spotlight.Response{Cards: []spotlight.Card{}, GeneratedAt: "x"},
	}
	h := NewSpotlightHandler(fake, false, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body len = %d, want 0 (bare 404, no envelope); body=%q", w.Body.Len(), w.Body.String())
	}
}

// TestSpotlightHandler_Get_OptionalAuth_DoesNot401 verifies the Phase 1
// endpoint tolerates an Authorization header — it must NOT 401, since
// Phase 1 is public (no auth middleware mounted).
func TestSpotlightHandler_Get_OptionalAuth_DoesNot401(t *testing.T) {
	fake := &fakeAggregator{
		resp: &spotlight.Response{
			Cards:       []spotlight.Card{{Type: "anime_of_day", Data: nil}},
			GeneratedAt: "2026-05-21T00:00:00Z",
		},
	}
	h := NewSpotlightHandler(fake, true, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	req.Header.Set("Authorization", "Bearer fake-token-should-be-ignored")
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("status = 401 — handler must NOT validate Authorization header in Phase 1")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestSpotlightHandler_Get_AggregatorError_Returns500EmptyCards verifies
// that a catastrophic aggregator error still emits the bare envelope
// shape (NOT httputil.Error's {success:false,error:{...}}).
func TestSpotlightHandler_Get_AggregatorError_Returns500EmptyCards(t *testing.T) {
	fake := &fakeAggregator{err: errors.New("aggregator down")}
	h := NewSpotlightHandler(fake, true, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}

	bodyBytes, _ := io.ReadAll(w.Body)
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, string(bodyBytes))
	}

	// Bare envelope on 500 path too — no httputil error wrapper.
	if _, ok := body["success"]; ok {
		t.Errorf("500-path body has 'success' key — handler must NOT use httputil envelope")
	}
	if _, ok := body["error"]; ok {
		t.Errorf("500-path body has 'error' key — handler must NOT use httputil envelope")
	}

	// Cards MUST be present as an empty array, not null.
	rawCards, ok := body["cards"]
	if !ok {
		t.Fatalf("500-path body missing 'cards' key")
	}
	if rawCards == nil {
		t.Errorf("500-path 'cards' is null — must marshal as [] not null")
	}
	cards, ok := rawCards.([]any)
	if !ok {
		t.Fatalf("500-path 'cards' is not an array; got %T", rawCards)
	}
	if len(cards) != 0 {
		t.Errorf("500-path len(cards) = %d, want 0", len(cards))
	}

	if _, ok := body["generated_at"]; !ok {
		t.Errorf("500-path body missing 'generated_at'")
	}
}

// TestSpotlightHandler_Get_NoEnvelopeWrapper is the explicit regression
// guard for re-introducing httputil.OK. The body MUST NOT contain the
// literal substring `"success":` anywhere — even nested.
func TestSpotlightHandler_Get_NoEnvelopeWrapper(t *testing.T) {
	fake := &fakeAggregator{
		resp: &spotlight.Response{
			Cards:       []spotlight.Card{{Type: "anime_of_day", Data: map[string]string{"k": "v"}}},
			GeneratedAt: "2026-05-21T00:00:00Z",
		},
	}
	h := NewSpotlightHandler(fake, true, testLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/home/spotlight", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	body := w.Body.String()
	if strings.Contains(body, `"success":`) {
		t.Errorf("response body contains `\"success\":` — handler must NOT use httputil envelope; body=%s", body)
	}
}

// topKeys is a tiny helper that returns a sorted-ish key list for error
// messages. Used in TestSpotlightHandler_Get_Envelope failure prints.
func topKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
