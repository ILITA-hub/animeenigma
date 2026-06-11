package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// fakeHintDeps records calls; handwritten fake per house style.
type fakeHintDeps struct {
	triggered   []string
	seedUpdates []string // "userID/animeID"
	cacheDels   []string
	listEntry   *hintListEntry // what LookupCompletion returns
}

func (f *fakeHintDeps) TriggerForUser(_ context.Context, userID string) error {
	f.triggered = append(f.triggered, userID)
	return nil
}
func (f *fakeHintDeps) LookupCompletion(_ context.Context, userID, animeID string) (*hintListEntry, error) {
	return f.listEntry, nil
}
func (f *fakeHintDeps) UpdateS6Seed(_ context.Context, userID, animeID string, _ time.Time, _ int) error {
	f.seedUpdates = append(f.seedUpdates, userID+"/"+animeID)
	return nil
}
func (f *fakeHintDeps) DeleteCache(_ context.Context, keys ...string) error {
	f.cacheDels = append(f.cacheDels, keys...)
	return nil
}

func postHint(t *testing.T, h *InternalHintHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/recs/recompute-hint", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.PostRecomputeHint(w, req)
	return w
}

func TestHint_TriggersDebounceAndSkipsSeedWhenNotQualifying(t *testing.T) {
	now := time.Now()
	f := &fakeHintDeps{listEntry: &hintListEntry{Status: "watching", Score: 0, CompletedAt: &now}}
	h := NewInternalHintHandler(f, logger.Default())

	w := postHint(t, h, `{"user_id":"u1","anime_id":"a1"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(f.triggered) != 1 || f.triggered[0] != "u1" {
		t.Fatalf("triggered = %v, want [u1]", f.triggered)
	}
	if len(f.seedUpdates) != 0 {
		t.Fatalf("seedUpdates = %v, want empty (status != completed)", f.seedUpdates)
	}
}

func TestHint_QualifyingCompletionUpdatesSeedAndBustsCache(t *testing.T) {
	now := time.Now()
	f := &fakeHintDeps{listEntry: &hintListEntry{Status: "completed", Score: 8, CompletedAt: &now}}
	h := NewInternalHintHandler(f, logger.Default())

	postHint(t, h, `{"user_id":"u1","anime_id":"a1"}`)

	if len(f.seedUpdates) != 1 || f.seedUpdates[0] != "u1/a1" {
		t.Fatalf("seedUpdates = %v, want [u1/a1]", f.seedUpdates)
	}
	if len(f.cacheDels) != 1 {
		t.Fatalf("cacheDels = %v, want one key", f.cacheDels)
	}
}

func TestHint_BadBodyIs400(t *testing.T) {
	f := &fakeHintDeps{}
	h := NewInternalHintHandler(f, logger.Default())
	if w := postHint(t, h, `{`); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if w := postHint(t, h, `{"anime_id":"a1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing user_id: status = %d, want 400", w.Code)
	}
}

// TestHint_LowScoreCompletionSkipsSeed verifies the score>=7 gate: a
// status='completed' entry with score=5 must NOT trigger the S6 seed update.
// Mirrors the gate from services/player/internal/service/list.go (Phase 13).
func TestHint_LowScoreCompletionSkipsSeed(t *testing.T) {
	now := time.Now()
	f := &fakeHintDeps{listEntry: &hintListEntry{Status: "completed", Score: 5, CompletedAt: &now}}
	h := NewInternalHintHandler(f, logger.Default())

	w := postHint(t, h, `{"user_id":"u1","anime_id":"a1"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(f.seedUpdates) != 0 {
		t.Fatalf("seedUpdates = %v, want empty (score < 7)", f.seedUpdates)
	}
	if len(f.cacheDels) != 0 {
		t.Fatalf("cacheDels = %v, want empty (no seed update)", f.cacheDels)
	}
	// Debounce must still fire regardless of seed qualification.
	if len(f.triggered) != 1 || f.triggered[0] != "u1" {
		t.Fatalf("triggered = %v, want [u1]", f.triggered)
	}
}
