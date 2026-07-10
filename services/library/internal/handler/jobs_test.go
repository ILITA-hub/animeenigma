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
	"github.com/ILITA-hub/animeenigma/libs/storageclient"
	"github.com/ILITA-hub/animeenigma/services/library/internal/autocache"
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
	mu         sync.Mutex
	created    []*domain.Job
	byID       map[string]*domain.Job
	listResp   []domain.Job
	getErr     error
	createErr  error
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

func (s *fakeStoreH) UpdateShikimoriID(_ context.Context, id, shikimoriID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.byID[id]
	if !ok {
		return liberrors.NotFound("job")
	}
	j.ShikimoriID = shikimoriID
	return nil
}

func (s *fakeStoreH) Retry(_ context.Context, oldID string) (*domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.byID[oldID]
	if !ok {
		return nil, liberrors.NotFound("job")
	}
	if old.Status != domain.JobStatusFailed {
		return nil, liberrors.InvalidInput("only failed jobs can be retried")
	}
	fresh := &domain.Job{
		ID:          "retry-" + oldID,
		Source:      old.Source,
		Magnet:      old.Magnet,
		Title:       old.Title,
		Uploader:    old.Uploader,
		Quality:     old.Quality,
		SizeBytes:   old.SizeBytes,
		ShikimoriID: old.ShikimoriID,
		Status:      domain.JobStatusQueued,
		ErrorText:   "retry of " + oldID,
	}
	s.byID[fresh.ID] = fresh
	return fresh, nil
}

// fakeMover stubs the storage-service List / Move for the Link handler tests.
type fakeMover struct {
	keys      []string
	listErr   error
	moveErr   error
	listCalls []struct{ storage, prefix string }
	moveCalls []struct{ storage, src, dst string }
}

func (m *fakeMover) List(_ context.Context, storage, prefix string) ([]storageclient.Object, error) {
	m.listCalls = append(m.listCalls, struct{ storage, prefix string }{storage, prefix})
	if m.listErr != nil {
		return nil, m.listErr
	}
	objs := make([]storageclient.Object, 0, len(m.keys))
	for _, k := range m.keys {
		objs = append(objs, storageclient.Object{Key: k})
	}
	return objs, nil
}

func (m *fakeMover) Move(_ context.Context, storage, src, dst string) error {
	m.moveCalls = append(m.moveCalls, struct{ storage, src, dst string }{storage, src, dst})
	return m.moveErr
}

// fakeEpisodeStore stubs the EpisodeStore.Create call.
type fakeEpisodeStore struct {
	created []*domain.Episode
	err     error
}

func (s *fakeEpisodeStore) Create(_ context.Context, ep *domain.Episode) error {
	if s.err != nil {
		return s.err
	}
	s.created = append(s.created, ep)
	return nil
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

// fakeBudgetGuard stubs the Phase-10 EvictorAPI seam: admitted/err returned verbatim,
// calls records each EnsureRoom(estBytes).
type fakeBudgetGuard struct {
	admitted bool
	err      error
	calls    []int64
}

func (b *fakeBudgetGuard) EnsureRoom(_ context.Context, estBytes int64) (bool, error) {
	b.calls = append(b.calls, estBytes)
	return b.admitted, b.err
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

// TestJobsHandler_Create_BudgetAdmitted: both gates pass (DiskGuard + budget) → 201.
// Also asserts the budget estimate passed is body.SizeBytes.
func TestJobsHandler_Create_BudgetAdmitted(t *testing.T) {
	h, store, dg, bg, _ := newBudgetHandler(t)

	body := map[string]any{
		"magnet":     validMagnet(),
		"title":      "x",
		"source":     "manual",
		"size_bytes": 4242,
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if !dg.called {
		t.Fatal("DiskGuard must be checked first (EVICT-05 layering)")
	}
	if len(bg.calls) != 1 {
		t.Fatalf("EnsureRoom must be called once, got %d", len(bg.calls))
	}
	if bg.calls[0] != 4242 {
		t.Fatalf("EnsureRoom estBytes = %d, want 4242 (body.SizeBytes)", bg.calls[0])
	}
	if len(store.created) != 1 {
		t.Fatalf("both-gates-pass must create one job, got %d", len(store.created))
	}
}

// TestJobsHandler_Create_BudgetOmittedSizeUsesFallback (WR-03): an admin upload that
// omits size_bytes (=0) must reserve the AvgRawEpSize fallback the Planner uses — NOT 0
// — so the pre-admit gate reserves a realistic estimate for an unknown-size download.
func TestJobsHandler_Create_BudgetOmittedSizeUsesFallback(t *testing.T) {
	h, store, _, bg, _ := newBudgetHandler(t)

	// size_bytes omitted entirely → body.SizeBytes == 0.
	body := map[string]any{
		"magnet": validMagnet(),
		"title":  "x",
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if len(bg.calls) != 1 {
		t.Fatalf("EnsureRoom must be called once, got %d", len(bg.calls))
	}
	if bg.calls[0] != autocache.AvgRawEpSize {
		t.Fatalf("EnsureRoom estBytes = %d, want AvgRawEpSize fallback %d (omitted size_bytes)",
			bg.calls[0], autocache.AvgRawEpSize)
	}
	if len(store.created) != 1 {
		t.Fatalf("admitted upload must create one job, got %d", len(store.created))
	}
}

// TestJobsHandler_Create_BudgetFull: DiskGuard passes but the budget rejects → 507 +
// rejected_total{budget_full}, no job created (EVICT-04).
func TestJobsHandler_Create_BudgetFull(t *testing.T) {
	h, store, _, bg, m := newBudgetHandler(t)
	bg.admitted = false

	body := map[string]any{
		"magnet":     validMagnet(),
		"title":      "x",
		"source":     "manual",
		"size_bytes": 999999,
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusInsufficientStorage {
		t.Fatalf("status = %d, want 507; body=%s", w.Code, w.Body.String())
	}
	if len(store.created) != 0 {
		t.Fatalf("budget-full must create nothing, got %d", len(store.created))
	}
	if v := testutil.ToFloat64(m.GetRejectedTotalForTest("budget_full")); v != 1 {
		t.Fatalf("rejected_total{budget_full} = %v, want 1", v)
	}
}

// TestJobsHandler_Create_BudgetError: a budget-read error must NOT 507 — it fails open
// and the upload proceeds to 201 (mirror disk-guard fail-open).
func TestJobsHandler_Create_BudgetError(t *testing.T) {
	h, store, _, bg, m := newBudgetHandler(t)
	bg.admitted = false
	bg.err = liberrors.Internal("budget read blip")

	body := map[string]any{
		"magnet": validMagnet(),
		"title":  "x",
		"source": "manual",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (fail-open); body=%s", w.Code, w.Body.String())
	}
	if len(store.created) != 1 {
		t.Fatalf("budget-error must fail open and create, got %d", len(store.created))
	}
	if v := testutil.ToFloat64(m.GetRejectedTotalForTest("budget_full")); v != 0 {
		t.Fatalf("budget-error must NOT increment rejected_total, got %v", v)
	}
}

// TestJobsHandler_Create_DiskFullBeforeBudget: DiskGuard rejects FIRST — the budget
// gate is never reached (EVICT-05 ordering: DiskGuard then budget).
func TestJobsHandler_Create_DiskFullBeforeBudget(t *testing.T) {
	h, store, dg, bg, m := newBudgetHandler(t)
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
	if len(bg.calls) != 0 {
		t.Fatalf("DiskGuard rejection must short-circuit before the budget gate, got %d EnsureRoom calls", len(bg.calls))
	}
	if len(store.created) != 0 {
		t.Fatalf("disk-full must create nothing, got %d", len(store.created))
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

// TestJobsHandler_Create_InvalidStorage is the storage-service Task-3
// validation regression: a storage value outside empty|'minio'|'s3' must be
// rejected 400 and must never reach jobRepo.Create.
func TestJobsHandler_Create_InvalidStorage(t *testing.T) {
	h, store, _, _, _, _ := newTestHandler(t)
	body := map[string]any{
		"magnet":  validMagnet(),
		"title":   "x",
		"source":  "manual",
		"storage": "tape",
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/library/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if len(store.created) != 0 {
		t.Fatalf("expected 0 created on invalid storage, got %d", len(store.created))
	}
}

// TestJobsHandler_Create_StorageOverride_Persisted proves a valid explicit
// override ('minio' or 's3') is trimmed, validated, and persisted onto the
// created job; omitting it defaults to the empty string (class default).
func TestJobsHandler_Create_StorageOverride_Persisted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"omitted", map[string]any{}, ""},
		{"minio", map[string]any{"storage": "minio"}, "minio"},
		{"s3", map[string]any{"storage": "s3"}, "s3"},
		{"padded", map[string]any{"storage": "  s3  "}, "s3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, _, _, _, _ := newTestHandler(t)
			body := map[string]any{
				"magnet": validMagnet(),
				"title":  "x",
				"source": "manual",
			}
			for k, v := range tc.payload {
				body[k] = v
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
			if got := store.created[0].Storage; got != tc.want {
				t.Fatalf("created storage = %q, want %q", got, tc.want)
			}
		})
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

// newLinkHandler wires up the Phase-5 constructor with the fakeStore + fakes. The
// budget guard defaults to admit so the Link/Retry cases (which don't exercise the
// pre-admit gate) stay green.
func newLinkHandler(t *testing.T) (*JobsHandler, *fakeStoreH, *fakeMover, *fakeEpisodeStore, *metrics.LibraryMetrics) {
	t.Helper()
	store := newFakeStoreH()
	dg := &fakeDiskGuard{allowed: true, freePct: 50}
	bg := &fakeBudgetGuard{admitted: true}
	cc := &fakeCanceller{}
	mover := &fakeMover{}
	eps := &fakeEpisodeStore{}
	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)
	h := NewJobsHandlerWithLink(store, dg, bg, cc, mover, eps, m, 20, nil)
	return h, store, mover, eps, m
}

// newBudgetHandler wires the Phase-10 pre-admit handler: NewJobsHandlerWithLink with a
// scriptable DiskGuard + budget guard so the EVICT-05 two-gate cases can drive both.
func newBudgetHandler(t *testing.T) (*JobsHandler, *fakeStoreH, *fakeDiskGuard, *fakeBudgetGuard, *metrics.LibraryMetrics) {
	t.Helper()
	store := newFakeStoreH()
	dg := &fakeDiskGuard{allowed: true, freePct: 50}
	bg := &fakeBudgetGuard{admitted: true}
	cc := &fakeCanceller{}
	reg := prometheus.NewRegistry()
	m := metrics.NewLibraryMetricsWithRegisterer(reg)
	h := NewJobsHandlerWithLink(store, dg, bg, cc, nil, nil, m, 20, nil)
	return h, store, dg, bg, m
}

func TestJobsHandler_Link_HappyPath(t *testing.T) {
	h, store, mover, eps, _ := newLinkHandler(t)
	// Seed a done job with empty shikimori_id.
	store.byID["job-1"] = &domain.Job{ID: "job-1", Title: "x", Status: domain.JobStatusDone}
	mover.keys = []string{"pending/job-1/3/playlist.m3u8", "pending/job-1/3/segment_000.ts"}

	body := map[string]any{"shikimori_id": "57466"}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/job-1", "job-1", b)
	w := httptest.NewRecorder()
	h.Link(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(mover.moveCalls) != 1 {
		t.Fatalf("expected 1 Move call, got %d", len(mover.moveCalls))
	}
	if mover.moveCalls[0].src != "pending/job-1/3/" {
		t.Errorf("Move src = %q, want pending/job-1/3/", mover.moveCalls[0].src)
	}
	if mover.moveCalls[0].dst != "aeProvider/57466/RAW/3/" {
		t.Errorf("Move dst = %q, want aeProvider/57466/RAW/3/", mover.moveCalls[0].dst)
	}
	// A pre-write-back (Storage="") job defaults to the minio backend for both
	// the list and the move.
	if mover.moveCalls[0].storage != "minio" {
		t.Errorf("Move storage = %q, want minio (job.Storage empty → default)", mover.moveCalls[0].storage)
	}
	if len(mover.listCalls) != 1 || mover.listCalls[0].storage != "minio" {
		t.Errorf("List calls = %+v, want one on storage=minio", mover.listCalls)
	}
	if len(eps.created) != 1 {
		t.Fatalf("expected 1 episode insert, got %d", len(eps.created))
	}
	if eps.created[0].EpisodeNumber != 3 {
		t.Errorf("episode num = %d, want 3", eps.created[0].EpisodeNumber)
	}
	if eps.created[0].MinioPath != "aeProvider/57466/RAW/3/" {
		t.Errorf("episode minio_path = %q, want aeProvider/57466/RAW/3/", eps.created[0].MinioPath)
	}
	if eps.created[0].Storage != "minio" {
		t.Errorf("episode storage = %q, want minio", eps.created[0].Storage)
	}
	// Job row's shikimori_id should be flipped.
	if store.byID["job-1"].ShikimoriID != "57466" {
		t.Errorf("job shikimori_id = %q, want 57466", store.byID["job-1"].ShikimoriID)
	}
}

func TestJobsHandler_Link_NotFound(t *testing.T) {
	h, _, _, _, _ := newLinkHandler(t)
	body := map[string]any{"shikimori_id": "57466"}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/missing", "missing", b)
	w := httptest.NewRecorder()
	h.Link(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestJobsHandler_Link_NotDone(t *testing.T) {
	h, store, _, _, _ := newLinkHandler(t)
	store.byID["job-1"] = &domain.Job{ID: "job-1", Status: domain.JobStatusDownloading}
	body := map[string]any{"shikimori_id": "57466"}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/job-1", "job-1", b)
	w := httptest.NewRecorder()
	h.Link(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestJobsHandler_Link_AlreadyLinked(t *testing.T) {
	h, store, _, _, _ := newLinkHandler(t)
	store.byID["job-1"] = &domain.Job{
		ID: "job-1", Status: domain.JobStatusDone, ShikimoriID: "999",
	}
	body := map[string]any{"shikimori_id": "57466"}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/job-1", "job-1", b)
	w := httptest.NewRecorder()
	h.Link(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestJobsHandler_Link_NoMinioObjects(t *testing.T) {
	h, store, mover, _, _ := newLinkHandler(t)
	store.byID["job-1"] = &domain.Job{ID: "job-1", Status: domain.JobStatusDone}
	mover.keys = nil
	body := map[string]any{"shikimori_id": "57466"}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/job-1", "job-1", b)
	w := httptest.NewRecorder()
	h.Link(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

func TestJobsHandler_Link_EmptyShikimoriID(t *testing.T) {
	h, store, mover, _, _ := newLinkHandler(t)
	store.byID["job-1"] = &domain.Job{ID: "job-1", Status: domain.JobStatusDone}
	mover.keys = []string{"pending/job-1/1/playlist.m3u8"}
	body := map[string]any{"shikimori_id": "   "}
	b, _ := json.Marshal(body)
	r := newChiRequestWithBody(http.MethodPatch, "/api/library/jobs/job-1", "job-1", b)
	w := httptest.NewRecorder()
	h.Link(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestJobsHandler_Retry_HappyPath(t *testing.T) {
	h, store, _, _, m := newLinkHandler(t)
	store.byID["failed-1"] = &domain.Job{
		ID: "failed-1", Title: "boom", Status: domain.JobStatusFailed,
		Magnet: "magnet:?xt=urn:btih:aaaa", Source: domain.JobSourceManual,
	}
	r := newChiRequest(http.MethodPost, "/api/library/jobs/failed-1/retry", "failed-1")
	w := httptest.NewRecorder()
	h.Retry(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if v := testutil.ToFloat64(m.GetJobsTotalForTest("queued")); v != 1 {
		t.Fatalf("library_jobs_total{queued} = %v, want 1", v)
	}
	// Original row still failed.
	if store.byID["failed-1"].Status != domain.JobStatusFailed {
		t.Errorf("original status changed: %q", store.byID["failed-1"].Status)
	}
}

func TestJobsHandler_Retry_NotFailed(t *testing.T) {
	h, store, _, _, _ := newLinkHandler(t)
	store.byID["q-1"] = &domain.Job{ID: "q-1", Status: domain.JobStatusQueued}
	r := newChiRequest(http.MethodPost, "/api/library/jobs/q-1/retry", "q-1")
	w := httptest.NewRecorder()
	h.Retry(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestJobsHandler_Retry_NotFound(t *testing.T) {
	h, _, _, _, _ := newLinkHandler(t)
	r := newChiRequest(http.MethodPost, "/api/library/jobs/missing/retry", "missing")
	w := httptest.NewRecorder()
	h.Retry(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestParseEpisodeFromPendingKey(t *testing.T) {
	cases := []struct {
		key, prefix string
		want        int
		wantErr     bool
	}{
		{"pending/abc/3/playlist.m3u8", "pending/abc/", 3, false},
		{"pending/xyz/1/segment_000.ts", "pending/xyz/", 1, false},
		{"pending/abc/12/playlist.m3u8", "pending/abc/", 12, false},
		{"pending/abc/notanumber/foo", "pending/abc/", 0, true},
		{"otherprefix/3/foo", "pending/abc/", 0, true},
		{"pending/abc/", "pending/abc/", 0, true},
		{"pending/abc/0/foo", "pending/abc/", 0, true},
	}
	for _, c := range cases {
		got, err := parseEpisodeFromPendingKey(c.key, c.prefix)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseEpisodeFromPendingKey(%q, %q) expected error", c.key, c.prefix)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseEpisodeFromPendingKey(%q, %q) unexpected error: %v", c.key, c.prefix, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseEpisodeFromPendingKey(%q, %q) = %d, want %d", c.key, c.prefix, got, c.want)
		}
	}
}

// newChiRequestWithBody is a helper for PATCH/POST requests with a body.
func newChiRequestWithBody(method, target, id string, body []byte) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	r = r.WithContext(contextWithRouteContext(r.Context(), rctx))
	return r
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
