package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// fakeNotifications records POSTs to the notifications internal endpoints.
type fakeNotifications struct {
	mu      sync.Mutex
	upserts []map[string]interface{}
	invals  []map[string]interface{}
}

func (f *fakeNotifications) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		_ = json.Unmarshal(body, &m)
		f.mu.Lock()
		switch r.URL.Path {
		case "/internal/notifications":
			f.upserts = append(f.upserts, m)
		case "/internal/notifications/invalidate":
			f.invals = append(f.invals, m)
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
}

// waitFor polls until cond() or the deadline — the notifier dispatches from
// a goroutine, so the test must wait for the async POST.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within deadline")
}

const testUserUUID = "0b54f8a3-9a1c-4c93-8a8e-2f1c3d4e5f60"

func postInternalStatus(t *testing.T, h *AdminReportsHandler, id, status string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/reports/"+id+"/status",
		strings.NewReader(`{"status":"`+status+`","updated_by":"test-bot"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.SetStatusInternal(w, req)
	return w
}

func TestSetStatusInternal_DispatchesStageNotification(t *testing.T) {
	fake := &fakeNotifications{}
	srv := fake.server(t)
	defer srv.Close()

	dir := t.TempDir()
	notifier := service.NewFeedbackNotifier(srv.URL, true, logger.Default())
	h := NewAdminReportsHandler(logger.Default(), dir, notifier)

	id := writeReport(t, dir, "2026-06-10T05-00-00", "alice", "feedback", map[string]interface{}{
		"user_id":     testUserUUID,
		"category":    "bug",
		"description": "плеер не работает",
	})

	w := postInternalStatus(t, h, id, "in_progress")
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", w.Code, w.Body.String())
	}

	waitFor(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.upserts) == 1
	})

	fake.mu.Lock()
	up := fake.upserts[0]
	fake.mu.Unlock()
	if up["type"] != "feedback_in_progress" {
		t.Errorf("type = %v, want feedback_in_progress", up["type"])
	}
	if up["user_id"] != testUserUUID {
		t.Errorf("user_id = %v", up["user_id"])
	}
	wantKey := "feedback:" + id + ":in_progress"
	if up["dedupe_key"] != wantKey {
		t.Errorf("dedupe_key = %v, want %s", up["dedupe_key"], wantKey)
	}
	inval, _ := up["invalidate_dedupe_keys"].([]interface{})
	if len(inval) != 2 {
		t.Errorf("invalidate_dedupe_keys = %v, want 2 sibling keys", inval)
	}
}

func TestSetStatusInternal_RefusesResolved(t *testing.T) {
	dir := t.TempDir()
	h := NewAdminReportsHandler(logger.Default(), dir, nil)
	id := writeReport(t, dir, "2026-06-10T05-00-00", "alice", "feedback", map[string]interface{}{
		"user_id": testUserUUID,
	})

	w := postInternalStatus(t, h, id, "resolved")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("resolved via internal route: code = %d, want 400", w.Code)
	}
}

func TestSetStatusInternal_NotRelevantInvalidates(t *testing.T) {
	fake := &fakeNotifications{}
	srv := fake.server(t)
	defer srv.Close()

	dir := t.TempDir()
	notifier := service.NewFeedbackNotifier(srv.URL, true, logger.Default())
	h := NewAdminReportsHandler(logger.Default(), dir, notifier)
	id := writeReport(t, dir, "2026-06-10T05-00-00", "alice", "feedback", map[string]interface{}{
		"user_id": testUserUUID,
	})

	w := postInternalStatus(t, h, id, "not_relevant")
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d", w.Code)
	}

	waitFor(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.invals) == 1
	})
	fake.mu.Lock()
	defer fake.mu.Unlock()
	keys, _ := fake.invals[0]["dedupe_keys"].([]interface{})
	if len(keys) != 3 {
		t.Errorf("dedupe_keys = %v, want all 3 stage keys", keys)
	}
	if len(fake.upserts) != 0 {
		t.Errorf("not_relevant must not create a notification, got %v", fake.upserts)
	}
}

func TestFeedbackNotifier_SkipsNonUUIDUser(t *testing.T) {
	fake := &fakeNotifications{}
	srv := fake.server(t)
	defer srv.Close()

	dir := t.TempDir()
	notifier := service.NewFeedbackNotifier(srv.URL, true, logger.Default())
	h := NewAdminReportsHandler(logger.Default(), dir, notifier)
	id := writeReport(t, dir, "2026-06-10T05-00-00", "tNeymik", "feedback", map[string]interface{}{
		"user_id": "tg:898912046", // telegram-ingested report — no site account
	})

	w := postInternalStatus(t, h, id, "in_progress")
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d", w.Code)
	}

	// Give the (would-be) goroutine a moment, then assert NOTHING arrived.
	time.Sleep(150 * time.Millisecond)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.upserts) != 0 || len(fake.invals) != 0 {
		t.Errorf("non-UUID user must be skipped, got upserts=%v invals=%v", fake.upserts, fake.invals)
	}
}

func TestSetStatusInternal_NoOpRepeatDoesNotNotify(t *testing.T) {
	fake := &fakeNotifications{}
	srv := fake.server(t)
	defer srv.Close()

	dir := t.TempDir()
	notifier := service.NewFeedbackNotifier(srv.URL, true, logger.Default())
	h := NewAdminReportsHandler(logger.Default(), dir, notifier)
	id := writeReport(t, dir, "2026-06-10T05-00-00", "alice", "feedback", map[string]interface{}{
		"user_id": testUserUUID,
	})

	if w := postInternalStatus(t, h, id, "in_progress"); w.Code != http.StatusOK {
		t.Fatalf("first set: %d", w.Code)
	}
	waitFor(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.upserts) == 1
	})

	// Repeat the same status — prev == status ⇒ no second dispatch.
	if w := postInternalStatus(t, h, id, "in_progress"); w.Code != http.StatusOK {
		t.Fatalf("repeat set: %d", w.Code)
	}
	time.Sleep(150 * time.Millisecond)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.upserts) != 1 {
		t.Errorf("repeat status dispatched again: %d upserts", len(fake.upserts))
	}
}
