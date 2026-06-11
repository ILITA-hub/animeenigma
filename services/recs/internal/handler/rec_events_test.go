// Phase 14 (REC-EVAL-01) — handler-level tests for the public telemetry
// endpoint POST /api/events/rec. Verifies the handler increments the
// Prometheus counters via libs/metrics, persists a rec_events row, and
// gracefully degrades when DB inserts fail (telemetry must NEVER break a
// click).
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRecEventsRepo records inserts so tests can assert persistence shape +
// optionally inject a failure to verify the handler's degrade-gracefully
// behavior.
type fakeRecEventsRepo struct {
	mu       sync.Mutex
	inserted []domain.RecEvent
	insertErr error
}

func (r *fakeRecEventsRepo) Insert(_ context.Context, ev *domain.RecEvent) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = append(r.inserted, *ev)
	return nil
}

func newRecEventsRequest(t *testing.T, body interface{}) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return httptest.NewRequest(http.MethodPost, "/api/events/rec", bytes.NewReader(b))
}

func TestRecEventsHandler_PostRecEvent_LoggedInClickIncrementsCounter(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type":  "rec_click",
		"anime_id":    "anime-1",
		"signal_id":   "s5",
		"pinned":      false,
		"source_route": "/",
		"rank":        3,
	}
	before := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s5", "false"))

	req := newRecEventsRequest(t, body)
	// Logged-in via injected claims.
	ctx := authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "user-1"})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	after := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s5", "false"))
	assert.InDelta(t, before+1, after, 1e-9, "rec_click_total{s5,false} must increment by 1")
	require.Len(t, repo.inserted, 1)
	assert.Equal(t, "rec_click", repo.inserted[0].EventType)
	assert.Equal(t, "s5", repo.inserted[0].SignalID)
	require.NotNil(t, repo.inserted[0].UserID)
	assert.Equal(t, "user-1", *repo.inserted[0].UserID)
	assert.False(t, repo.inserted[0].Pinned)
}

func TestRecEventsHandler_PostRecEvent_AnonymousClickAllowed(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type": "rec_click",
		"anime_id":   "anime-2",
		"signal_id":  "s3",
		"pinned":     false,
	}
	before := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s3", "false"))

	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	after := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s3", "false"))
	assert.InDelta(t, before+1, after, 1e-9)
	require.Len(t, repo.inserted, 1)
	assert.Nil(t, repo.inserted[0].UserID, "anonymous click stored with NULL user_id")
}

func TestRecEventsHandler_PostRecEvent_WatchedEventIncrementsWatchCounter(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type": "rec_watched",
		"anime_id":   "anime-3",
		"signal_id":  "s2",
		"pinned":     false,
	}
	before := testutil.ToFloat64(metrics.RecWatchedTotal.WithLabelValues("s2", "false"))

	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	after := testutil.ToFloat64(metrics.RecWatchedTotal.WithLabelValues("s2", "false"))
	assert.InDelta(t, before+1, after, 1e-9)
	require.Len(t, repo.inserted, 1)
	assert.Equal(t, "rec_watched", repo.inserted[0].EventType)
}

func TestRecEventsHandler_PostRecEvent_PinClickTagsS6Pin(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	pinSource := "shikimori_similar"
	pinSeed := "seed-anime-1"
	body := map[string]interface{}{
		"event_type":         "rec_click",
		"anime_id":           "anime-4",
		"signal_id":          "s6_pin",
		"pinned":             true,
		"pin_source":         pinSource,
		"pin_seed_anime_id":  pinSeed,
	}
	before := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s6_pin", "true"))

	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	after := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s6_pin", "true"))
	assert.InDelta(t, before+1, after, 1e-9, "pin clicks must increment {s6_pin,true}")
	require.Len(t, repo.inserted, 1)
	assert.True(t, repo.inserted[0].Pinned)
	require.NotNil(t, repo.inserted[0].PinSource)
	assert.Equal(t, pinSource, *repo.inserted[0].PinSource)
	require.NotNil(t, repo.inserted[0].PinSeedAnimeID)
	assert.Equal(t, pinSeed, *repo.inserted[0].PinSeedAnimeID)
}

func TestRecEventsHandler_PostRecEvent_InvalidEventTypeReturns400(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type": "rec_skipped", // unknown
		"anime_id":   "anime-x",
		"signal_id":  "s3",
	}
	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, repo.inserted, "unknown event_type must NOT persist")
}

func TestRecEventsHandler_PostRecEvent_MissingFieldsReturns400(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type": "rec_click",
		"anime_id":   "",
		"signal_id":  "s3",
	}
	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, repo.inserted)
}

func TestRecEventsHandler_PostRecEvent_DBErrorDoesNotFailRequest(t *testing.T) {
	// Telemetry must NEVER fail the click. DB insert failure is logged
	// (logged via h.log.Errorw inside the handler) but the HTTP response
	// remains 200 so the calling code (frontend) gets a clean ack.
	repo := &fakeRecEventsRepo{insertErr: errors.New("simulated db down")}
	h := NewRecEventsHandler(repo, logger.Default())

	body := map[string]interface{}{
		"event_type": "rec_click",
		"anime_id":   "anime-y",
		"signal_id":  "s3",
		"pinned":     false,
	}
	before := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s3", "false"))

	req := newRecEventsRequest(t, body)
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusOK, w.Code, "db failure must NOT fail the click")
	after := testutil.ToFloat64(metrics.RecClickTotal.WithLabelValues("s3", "false"))
	assert.InDelta(t, before+1, after, 1e-9, "counter still increments even when DB insert fails")
}

func TestRecEventsHandler_PostRecEvent_MalformedJSONReturns400(t *testing.T) {
	repo := &fakeRecEventsRepo{}
	h := NewRecEventsHandler(repo, logger.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/events/rec", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	h.PostRecEvent(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, repo.inserted)
}
