package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

type fakeTransitionStore struct {
	rows []repo.DegradationTransitionRow
	err  error
}

func (f *fakeTransitionStore) InsertDegradationTransition(_ context.Context, row repo.DegradationTransitionRow) error {
	if f.err != nil {
		return f.err
	}
	f.rows = append(f.rows, row)
	return nil
}

func postTransition(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/degradation/transition", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDegradationTransition_InsertsRow(t *testing.T) {
	store := &fakeTransitionStore{}
	h := NewDegradationTransitionHandler(store)

	rec := postTransition(t, h, `{
	  "ts": "2026-07-10T12:00:00Z",
	  "from_level": 0,
	  "to_level": 2,
	  "reasons": ["psi_cpu_some:critical"],
	  "signal_values": {"ae:host_psi_cpu_some:ratio": 0.61}
	}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, store.rows, 1)
	row := store.rows[0]
	assert.Equal(t, uint8(0), row.FromLevel)
	assert.Equal(t, uint8(2), row.ToLevel)
	assert.Equal(t, []string{"psi_cpu_some:critical"}, row.Reasons)
	assert.Equal(t, 0.61, row.SignalValues["ae:host_psi_cpu_some:ratio"])
	assert.Equal(t, time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC), row.TS.UTC())
}

func TestDegradationTransition_DefaultsMissingTimestamp(t *testing.T) {
	store := &fakeTransitionStore{}
	h := NewDegradationTransitionHandler(store)

	rec := postTransition(t, h, `{"from_level": 1, "to_level": 0, "reasons": []}`)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, store.rows, 1)
	assert.WithinDuration(t, time.Now().UTC(), store.rows[0].TS, time.Minute)
}

func TestDegradationTransition_Rejects(t *testing.T) {
	t.Run("bad json", func(t *testing.T) {
		rec := postTransition(t, NewDegradationTransitionHandler(&fakeTransitionStore{}), `{nope`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("level out of range", func(t *testing.T) {
		rec := postTransition(t, NewDegradationTransitionHandler(&fakeTransitionStore{}), `{"from_level":0,"to_level":7}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
	t.Run("store failure -> 500", func(t *testing.T) {
		rec := postTransition(t, NewDegradationTransitionHandler(&fakeTransitionStore{err: errors.New("ch down")}), `{"from_level":0,"to_level":1}`)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
