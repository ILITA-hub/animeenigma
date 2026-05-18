package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeJobStore is the handler test stub. Thread-safe to support the
// concurrent path the underlying JobsHandler hits via r.Context().
type fakeStoreH struct {
	mu       sync.Mutex
	created  []*domain.Job
	byID     map[string]*domain.Job
	listResp []domain.Job
	getErr   error
	createErr error
	listFilter repo.JobFilter
	listCalls  int
}

func newFakeStoreH() *fakeStoreH {
	return &fakeStoreH{byID: map[string]*domain.Job{}}
}

func (s *fakeStoreH) Create(_ context.Context, j *domain.Job) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.ID == "" {
		j.ID = "fake-id-" + j.Title
	}
	s.created = append(s.created, j)
	cp := *j
	s.byID[j.ID] = &cp
	return nil
}
func (s *fakeStoreH) GetByID(_ context.Context, id string) (*domain.Job, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[id]
	if !ok {
		return nil, liberrors.NotFound("job")
	}
	cp := *j
	return &cp, nil
}
func (s *fakeStoreH) List(_ context.Context, f repo.JobFilter) ([]domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	s.listFilter = f
	return s.listResp, nil
}

type fakeDiskGuard struct {
	allowed bool
	freePct int
	err     error
	called  bool
}

func (d *fakeDiskGuard) Allow(min int) (bool, int, error) {
	d.called = true
	return d.allowed, d.freePct, d.err
}

type fakeCanceller struct {
	called bool
	id     string
	err    error
}

func (c *fakeCanceller) CancelJob(_ context.Context, id string) error {
	c.called = true
	c.id = id
	return c.err
}

func newTestHandler(t *testing.T) (*JobsHandler, *fakeStoreH, *fakeDiskGuard, *fakeCanceller, *prometheus.Registry, *metrics.LibraryMetrics) {
	t.Helper()
	store := newFakeStoreH()
	dg := &fakeDiskGuard{allowed: true, freePct: 50}
	cc := &fakeCanceller{}
	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)
	h := NewJobsHandler(store, dg, cc, m, 20, nil)
	return h, store, dg, cc, reg, m
}

func validMagnet() string {
	return "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=test"
}

func TestJobsHandler_Create_Valid(t *testing.T) {
	h, store, _, _, _, m := newTestHandler(t)

	body := map[string]any{
		"magnet": validMagnet(),
		"title":  "Test Title",
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(store.created))
	}
	if store.created[0].Status != domain.JobStatusQueued {
		t.Fatalf("created status = %q, want queued", store.created[0].Status)
	}

	// library_jobs_total{status="queued"} bumped.
	v := testutil.ToFloat64(m.GetJobsTotalForTest("queued"))
	if v != 1 {
		t.Fatalf("library_jobs_total{queued} = %v, want 1", v)
	}
}

func TestJobsHandler_Create_InvalidMagnet(t *testing.T) {
	h, store, _, _, _, m := newTestHandler(t)

	body := map[string]any{
		"magnet": "definitely-not-a-magnet",
		"title":  "x",
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(store.created) != 0 {
		t.Fatalf("expected 0 created on bad magnet, got %d", len(store.created))
	}
	// library_enqueue_rejected_total{reason="invalid_magnet"} bumped.
	if v := testutil.ToFloat64(m.GetEnqueueRejectedForTest("invalid_magnet")); v != 1 {
		t.Fatalf("invalid_magnet count = %v, want 1", v)
	}
}

func TestJobsHandler_Create_DiskFull(t *testing.T) {
	h, store, dg, _, _, m := newTestHandler(t)
	dg.allowed = false
	dg.freePct = 5

	body := map[string]any{
		"magnet": validMagnet(),
		"title":  "x",
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusInsufficientStorage {
		t.Fatalf("status = %d, want 507", w.Code)
	}
	bodyStr := w.Body.String()
	if !contains(bodyStr, `"disk_full"`) {
		t.Fatalf("body = %q, want to contain disk_full marker", bodyStr)
	}
	if len(store.created) != 0 {
		t.Fatalf("expected 0 created on disk full, got %d", len(store.created))
	}
	if v := testutil.ToFloat64(m.GetEnqueueRejectedForTest("disk_full")); v != 1 {
		t.Fatalf("disk_full count = %v, want 1", v)
	}
}

func TestJobsHandler_Create_MissingTitle(t *testing.T) {
	h, _, _, _, _, _ := newTestHandler(t)
	body := map[string]any{
		"magnet": validMagnet(),
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestJobsHandler_Create_UnknownSource(t *testing.T) {
	h, _, _, _, _, _ := newTestHandler(t)
	body := map[string]any{
		"magnet": validMagnet(),
		"title":  "x",
		"source": "rutorrent",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestJobsHandler_List_StatusFilter(t *testing.T) {
	h, store, _, _, _, _ := newTestHandler(t)
	store.listResp = []domain.Job{{ID: "1"}, {ID: "2"}}

	r := httptest.NewRequest(http.MethodGet, "/api/library/jobs?status=queued,downloading&limit=10", nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if store.listCalls != 1 {
		t.Fatalf("List calls = %d, want 1", store.listCalls)
	}
	if len(store.listFilter.Statuses) != 2 {
		t.Fatalf("filter statuses = %v, want 2", store.listFilter.Statuses)
	}
	if store.listFilter.Limit != 10 {
		t.Fatalf("filter limit = %d, want 10", store.listFilter.Limit)
	}
}

func TestJobsHandler_List_UnknownStatus(t *testing.T) {
	h, _, _, _, _, _ := newTestHandler(t)
	r := httptest.NewRequest(http.MethodGet, "/api/library/jobs?status=galactic", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestJobsHandler_Get_NotFound(t *testing.T) {
	h, _, _, _, _, _ := newTestHandler(t)
	r := newChiRequest(http.MethodGet, "/api/library/jobs/missing", "missing")
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestJobsHandler_Get_Found(t *testing.T) {
	h, store, _, _, _, _ := newTestHandler(t)
	store.byID["abc"] = &domain.Job{ID: "abc", Title: "x"}

	r := newChiRequest(http.MethodGet, "/api/library/jobs/abc", "abc")
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestJobsHandler_Delete_Success(t *testing.T) {
	h, _, _, cc, _, _ := newTestHandler(t)
	r := newChiRequest(http.MethodDelete, "/api/library/jobs/xyz", "xyz")
	w := httptest.NewRecorder()
	h.Delete(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if !cc.called || cc.id != "xyz" {
		t.Fatalf("canceller called=%v id=%q, want true 'xyz'", cc.called, cc.id)
	}
}

func TestJobsHandler_Delete_NotFound(t *testing.T) {
	h, _, _, cc, _, _ := newTestHandler(t)
	cc.err = liberrors.NotFound("job")
	r := newChiRequest(http.MethodDelete, "/api/library/jobs/missing", "missing")
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// newChiRequest wires up a chi URL param so chi.URLParam(r, "id")
// resolves inside the handler under test.
func newChiRequest(method, target, id string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	r = r.WithContext(contextWithRouteContext(r.Context(), rctx))
	return r
}

func contextWithRouteContext(parent context.Context, rctx *chi.Context) context.Context {
	return context.WithValue(parent, chi.RouteCtxKey, rctx)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) != -1)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
