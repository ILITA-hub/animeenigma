package handler

// admin_test.go — table-driven tests for AdminHandler.
//
// Uses the handlerSQLiteOnce / openHandlerTestDB harness already established
// in segments_test.go (same package). A small upscale_workers table DDL is
// added here so ListWorkers is testable without pulling in the shared-cache
// test DB used by repo tests.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ── fixtures ──────────────────────────────────────────────────────────────────

// openAdminTestDB is like openHandlerTestDB but also creates the upscale_workers
// table so AdminHandler.ListWorkers has something to query. The workers table is
// intentionally absent from the segment handler harness (not needed there), so we
// add it here without modifying the shared helper.
func openAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openHandlerTestDB(t)
	workersDDL := `CREATE TABLE IF NOT EXISTS upscale_workers (
		worker_id          TEXT NOT NULL PRIMARY KEY,
		gpu_info           TEXT,
		image_version      TEXT,
		models_available   TEXT,
		status             TEXT NOT NULL DEFAULT 'idle',
		current_job_id     TEXT,
		current_segment    INTEGER,
		session_expires_at DATETIME,
		last_heartbeat_at  DATETIME,
		created_at         DATETIME
	)`
	if err := db.Exec(workersDDL).Error; err != nil {
		t.Skipf("create workers table: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM upscale_workers")
	})
	return db
}

type adminFixture struct {
	db      *gorm.DB
	jobs    *repo.JobRepository
	workers *repo.WorkerRepository
	handler *AdminHandler
	router  chi.Router
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	db := openAdminTestDB(t)
	jobs := repo.NewJobRepository(db)
	workers := repo.NewWorkerRepository(db)
	h := NewAdminHandler(jobs, workers, 2, "realesrgan-x4plus-anime", logger.Default())

	r := chi.NewRouter()
	r.Post("/api/upscale/jobs", h.CreateJob)
	r.Get("/api/upscale/jobs", h.ListJobs)
	r.Get("/api/upscale/jobs/{id}", h.GetJob)
	r.Post("/api/upscale/jobs/{id}/cancel", h.CancelJob)
	r.Post("/api/upscale/jobs/{id}/retry", h.RetryJob)
	r.Get("/api/upscale/workers", h.ListWorkers)

	return &adminFixture{db: db, jobs: jobs, workers: workers, handler: h, router: r}
}

// doRequest is a helper that fires an HTTP request through the router and
// returns the recorder.
func doRequest(t *testing.T, r chi.Router, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// responseData unmarshals the { "success": bool, "data": <T> } envelope.
func responseData(t *testing.T, body []byte, dst interface{}) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v (body: %s)", err, body)
	}
	if dst == nil {
		return
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("unmarshal data: %v (data: %s)", err, env.Data)
	}
}

// ── POST /api/upscale/jobs ────────────────────────────────────────────────────

func TestAdminHandler_CreateJob_HappyPath(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	body, _ := json.Marshal(map[string]interface{}{
		"shikimori_id": "12345",
		"episode":      3,
		"model":        "realesrgan-x4plus-anime",
		"scale":        4,
	})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("CreateJob status = %d; want 201 (body: %s)", rr.Code, rr.Body)
	}

	var got domain.UpscaleJob
	responseData(t, rr.Body.Bytes(), &got)

	if got.ID == "" {
		t.Error("CreateJob: job ID is empty")
	}
	if got.Status != domain.JobQueued {
		t.Errorf("CreateJob: status = %q; want %q", got.Status, domain.JobQueued)
	}
	if got.ShikimoriID != "12345" {
		t.Errorf("CreateJob: shikimori_id = %q; want 12345", got.ShikimoriID)
	}
	if got.Episode != 3 {
		t.Errorf("CreateJob: episode = %d; want 3", got.Episode)
	}
	if got.Scale != 4 {
		t.Errorf("CreateJob: scale = %d; want 4", got.Scale)
	}

	// Verify persisted to DB.
	persisted, err := f.jobs.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("Get persisted job: %v", err)
	}
	if persisted.Status != domain.JobQueued {
		t.Errorf("persisted status = %q; want queued", persisted.Status)
	}
}

func TestAdminHandler_CreateJob_DefaultScaleAndModel(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	// Omit model and scale — they should be filled from handler defaults.
	body, _ := json.Marshal(map[string]interface{}{
		"shikimori_id": "99999",
		"episode":      1,
	})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("CreateJob (defaults) status = %d; want 201 (body: %s)", rr.Code, rr.Body)
	}

	var got domain.UpscaleJob
	responseData(t, rr.Body.Bytes(), &got)
	if got.Scale != 2 {
		t.Errorf("scale = %d; want 2 (default)", got.Scale)
	}
	if got.Model != "realesrgan-x4plus-anime" {
		t.Errorf("model = %q; want realesrgan-x4plus-anime (default)", got.Model)
	}
}

func TestAdminHandler_CreateJob_ValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing shikimori_id", map[string]interface{}{"episode": 1}},
		{"empty shikimori_id", map[string]interface{}{"shikimori_id": "", "episode": 1}},
		{"episode zero", map[string]interface{}{"shikimori_id": "123", "episode": 0}},
		{"episode negative", map[string]interface{}{"shikimori_id": "123", "episode": -1}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			body, _ := json.Marshal(c.body)
			rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs", body)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("CreateJob(%s) status = %d; want 400", c.name, rr.Code)
			}
		})
	}
}

// ── GET /api/upscale/jobs/{id} ────────────────────────────────────────────────

func TestAdminHandler_GetJob_Found(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "42",
		Episode:     7,
		Model:       "realesrgan-x4plus-anime",
		Scale:       2,
		Status:      domain.JobQueued,
	}
	if err := f.jobs.Create(context.Background(), job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs/"+job.ID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GetJob status = %d; want 200", rr.Code)
	}
	var got domain.UpscaleJob
	responseData(t, rr.Body.Bytes(), &got)
	if got.ID != job.ID {
		t.Errorf("GetJob: id = %q; want %q", got.ID, job.ID)
	}
	if got.Episode != 7 {
		t.Errorf("GetJob: episode = %d; want 7", got.Episode)
	}
}

func TestAdminHandler_GetJob_NotFound(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs/"+uuid.New().String(), nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GetJob(missing) status = %d; want 404", rr.Code)
	}
}

// ── GET /api/upscale/jobs ─────────────────────────────────────────────────────

func TestAdminHandler_ListJobs_FilterByStatus(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx := context.Background()

	seed := []domain.UpscaleJob{
		{ID: uuid.New().String(), ShikimoriID: "1", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
		{ID: uuid.New().String(), ShikimoriID: "2", Episode: 1, Model: "m", Scale: 2, Status: domain.JobFailed},
		{ID: uuid.New().String(), ShikimoriID: "3", Episode: 1, Model: "m", Scale: 2, Status: domain.JobQueued},
	}
	for i := range seed {
		if err := f.jobs.Create(ctx, &seed[i]); err != nil {
			t.Fatalf("seed Create: %v", err)
		}
	}

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs?status=failed", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListJobs(failed) status = %d; want 200", rr.Code)
	}
	var got []domain.UpscaleJob
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("ListJobs(failed) len = %d; want 1", len(got))
	}
	if got[0].Status != domain.JobFailed {
		t.Errorf("ListJobs(failed)[0].status = %q; want failed", got[0].Status)
	}
}

func TestAdminHandler_ListJobs_InvalidStatus(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs?status=bogus", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("ListJobs(status=bogus) status = %d; want 400 (body: %s)", rr.Code, rr.Body)
	}
}

func TestAdminHandler_ListJobs_NoFilter(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		job := &domain.UpscaleJob{
			ID:          uuid.New().String(),
			ShikimoriID: fmt.Sprintf("s%d", i),
			Episode:     i + 1,
			Model:       "m",
			Scale:       2,
			Status:      domain.JobQueued,
		}
		if err := f.jobs.Create(ctx, job); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListJobs status = %d; want 200", rr.Code)
	}
	var got []domain.UpscaleJob
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 3 {
		t.Errorf("ListJobs (no filter) len = %d; want 3", len(got))
	}
}

// ── POST /api/upscale/jobs/{id}/cancel ────────────────────────────────────────

func TestAdminHandler_CancelJob_NonTerminal(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx := context.Background()

	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "10",
		Episode:     1,
		Model:       "m",
		Scale:       2,
		Status:      domain.JobUpscaling, // non-terminal
	}
	if err := f.jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/cancel", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("CancelJob status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}

	updated, err := f.jobs.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get after cancel: %v", err)
	}
	if updated.Status != domain.JobCancelled {
		t.Errorf("status after cancel = %q; want cancelled", updated.Status)
	}
}

func TestAdminHandler_CancelJob_AlreadyTerminal(t *testing.T) {
	t.Parallel()
	terminalStatuses := []domain.JobStatus{
		domain.JobDone,
		domain.JobFailed,
		domain.JobCancelled,
	}
	for _, s := range terminalStatuses {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			ctx := context.Background()
			job := &domain.UpscaleJob{
				ID:          uuid.New().String(),
				ShikimoriID: "20",
				Episode:     1,
				Model:       "m",
				Scale:       2,
				Status:      s,
			}
			if err := f.jobs.Create(ctx, job); err != nil {
				t.Fatalf("Create: %v", err)
			}
			rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/cancel", nil)
			if rr.Code != http.StatusConflict {
				t.Errorf("CancelJob(%s) status = %d; want 409", s, rr.Code)
			}
		})
	}
}

func TestAdminHandler_CancelJob_NotFound(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+uuid.New().String()+"/cancel", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("CancelJob(missing) status = %d; want 404", rr.Code)
	}
}

// TestAdminHandler_CancelJob_DeliversCancelToWorker verifies that cancelling a
// job that has a connected worker ALSO issues a `cancel` command to that worker
// (Deliverable A — beyond the DB-only 12a cancel).
func TestAdminHandler_CancelJob_DeliversCancelToWorker(t *testing.T) {
	t.Parallel()
	cmd := &fakeCommander{}
	f := newAdminFixtureWithExtras(t, cmd, nil)
	ctx := context.Background()

	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "55",
		Episode:     1,
		Model:       "m",
		Scale:       2,
		Status:      domain.JobUpscaling,
	}
	if err := f.jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	// Seed a worker currently bound to the job.
	now := time.Now()
	wk := &domain.UpscaleWorker{
		WorkerID:        "worker-running",
		Status:          "busy",
		CurrentJobID:    job.ID,
		LastHeartbeatAt: &now,
	}
	if err := f.workers.Upsert(ctx, wk); err != nil {
		t.Fatalf("Upsert worker: %v", err)
	}

	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/cancel", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("CancelJob status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}

	// DB status flipped.
	updated, err := f.jobs.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get after cancel: %v", err)
	}
	if updated.Status != domain.JobCancelled {
		t.Errorf("status after cancel = %q; want cancelled", updated.Status)
	}

	// Worker received a cancel command.
	cmd.mu.Lock()
	defer cmd.mu.Unlock()
	if len(cmd.calls) != 1 {
		t.Fatalf("expected 1 cancel command delivered; got %d", len(cmd.calls))
	}
	if cmd.calls[0].workerID != "worker-running" {
		t.Errorf("cancel delivered to %q; want worker-running", cmd.calls[0].workerID)
	}
	if cmd.calls[0].cmd != "cancel" {
		t.Errorf("delivered cmd = %q; want cancel", cmd.calls[0].cmd)
	}
}

// TestAdminHandler_CancelJob_NoWorker_StillCancels verifies that DB-cancel still
// succeeds when no worker is bound to the job (no command is delivered).
func TestAdminHandler_CancelJob_NoWorker_StillCancels(t *testing.T) {
	t.Parallel()
	cmd := &fakeCommander{}
	f := newAdminFixtureWithExtras(t, cmd, nil)
	ctx := context.Background()

	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "56",
		Episode:     1,
		Model:       "m",
		Scale:       2,
		Status:      domain.JobUpscaling,
	}
	if err := f.jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create job: %v", err)
	}

	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/cancel", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("CancelJob status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}
	updated, err := f.jobs.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get after cancel: %v", err)
	}
	if updated.Status != domain.JobCancelled {
		t.Errorf("status after cancel = %q; want cancelled", updated.Status)
	}
	cmd.mu.Lock()
	defer cmd.mu.Unlock()
	if len(cmd.calls) != 0 {
		t.Errorf("expected 0 commands delivered (no worker bound); got %d", len(cmd.calls))
	}
}

// ── POST /api/upscale/jobs/{id}/retry ─────────────────────────────────────────

func TestAdminHandler_RetryJob_FailedBecomesQueued(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx := context.Background()

	job := &domain.UpscaleJob{
		ID:          uuid.New().String(),
		ShikimoriID: "30",
		Episode:     1,
		Model:       "m",
		Scale:       2,
		Status:      domain.JobFailed,
		ErrorText:   "something broke",
	}
	if err := f.jobs.Create(ctx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/retry", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("RetryJob status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}

	updated, err := f.jobs.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get after retry: %v", err)
	}
	if updated.Status != domain.JobQueued {
		t.Errorf("status after retry = %q; want queued", updated.Status)
	}
	// error_text must be cleared in the DB.
	if updated.ErrorText != "" {
		t.Errorf("error_text after retry = %q; want empty", updated.ErrorText)
	}
}

func TestAdminHandler_RetryJob_NonFailedRejected(t *testing.T) {
	t.Parallel()
	nonFailedStatuses := []domain.JobStatus{
		domain.JobQueued,
		domain.JobSegmenting,
		domain.JobUpscaling,
		domain.JobFinalizing,
		domain.JobDone,
		domain.JobCancelled,
	}
	for _, s := range nonFailedStatuses {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			ctx := context.Background()
			job := &domain.UpscaleJob{
				ID:          uuid.New().String(),
				ShikimoriID: "40",
				Episode:     1,
				Model:       "m",
				Scale:       2,
				Status:      s,
			}
			if err := f.jobs.Create(ctx, job); err != nil {
				t.Fatalf("Create: %v", err)
			}
			rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+job.ID+"/retry", nil)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("RetryJob(%s) status = %d; want 400", s, rr.Code)
			}
		})
	}
}

func TestAdminHandler_RetryJob_NotFound(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/jobs/"+uuid.New().String()+"/retry", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("RetryJob(missing) status = %d; want 404", rr.Code)
	}
}

// ── GET /api/upscale/workers ──────────────────────────────────────────────────

func TestAdminHandler_ListWorkers(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	ctx := context.Background()

	now := time.Now()
	// Two active workers (recent heartbeat) and one gone (old heartbeat).
	active1 := &domain.UpscaleWorker{
		WorkerID:        "worker-a",
		Status:          "idle",
		LastHeartbeatAt: &now,
	}
	active2 := &domain.UpscaleWorker{
		WorkerID:        "worker-b",
		Status:          "busy",
		LastHeartbeatAt: &now,
	}
	old := time.Now().Add(-10 * time.Minute)
	stale := &domain.UpscaleWorker{
		WorkerID:        "worker-stale",
		Status:          "idle",
		LastHeartbeatAt: &old,
	}
	for _, w := range []*domain.UpscaleWorker{active1, active2, stale} {
		if err := f.workers.Upsert(ctx, w); err != nil {
			t.Fatalf("Upsert worker: %v", err)
		}
	}

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/workers", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListWorkers status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}
	var got []domain.UpscaleWorker
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Errorf("ListWorkers: got %d workers; want 2 active", len(got))
	}
	for _, w := range got {
		if w.WorkerID == "worker-stale" {
			t.Errorf("ListWorkers: stale worker should not appear")
		}
	}
}

func TestAdminHandler_ListWorkers_Empty(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/workers", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListWorkers(empty) status = %d; want 200", rr.Code)
	}
	var got []domain.UpscaleWorker
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Errorf("ListWorkers(empty) len = %d; want 0", len(got))
	}
}

// ── Fakes for commander and logBuffer ─────────────────────────────────────────

// fakeCommander records Issue calls and optionally returns an error.
type fakeCommander struct {
	mu    sync.Mutex
	calls []struct{ workerID, cmd string }
	err   error
}

func (f *fakeCommander) Issue(workerID, cmd string, _ json.RawMessage) error {
	f.mu.Lock()
	f.calls = append(f.calls, struct{ workerID, cmd string }{workerID, cmd})
	f.mu.Unlock()
	return f.err
}

// fakeLogBuffer is an in-memory adminLogBuffer for tests.
type fakeLogBuffer struct {
	lines []service.LogLine
}

func (f *fakeLogBuffer) Tail(_ string, n int) []service.LogLine {
	if len(f.lines) == 0 {
		return nil
	}
	if n >= len(f.lines) {
		return f.lines
	}
	return f.lines[len(f.lines)-n:]
}

func (f *fakeLogBuffer) Subscribe(_ string) (<-chan service.LogLine, func()) {
	ch := make(chan service.LogLine, 4)
	// Send all buffered lines into the channel then close it.
	go func() {
		for _, l := range f.lines {
			ch <- l
		}
		close(ch)
	}()
	return ch, func() {}
}

// newAdminFixtureWithExtras constructs an adminFixture with the commander and
// logBuffer wired, plus extra routes for the new endpoints.
func newAdminFixtureWithExtras(t *testing.T, cmd commander, lb adminLogBuffer) *adminFixture {
	t.Helper()
	db := openAdminTestDB(t)
	jobs := repo.NewJobRepository(db)
	workers := repo.NewWorkerRepository(db)
	h := NewAdminHandler(jobs, workers, 2, "realesrgan-x4plus-anime", logger.Default())
	if cmd != nil {
		h.WithCommander(cmd)
	}
	if lb != nil {
		h.WithLogBuffer(lb)
	}

	r := chi.NewRouter()
	r.Post("/api/upscale/jobs", h.CreateJob)
	r.Get("/api/upscale/jobs", h.ListJobs)
	r.Get("/api/upscale/jobs/{id}", h.GetJob)
	r.Post("/api/upscale/jobs/{id}/cancel", h.CancelJob)
	r.Post("/api/upscale/jobs/{id}/retry", h.RetryJob)
	r.Get("/api/upscale/workers", h.ListWorkers)
	r.Post("/api/upscale/workers/{id}/commands", h.PostWorkerCommand)
	r.Get("/api/upscale/jobs/{id}/logs", h.GetJobLogs)
	r.Get("/api/upscale/jobs/{id}/logs/stream", h.StreamJobLogs)

	return &adminFixture{db: db, jobs: jobs, workers: workers, handler: h, router: r}
}

// ── POST /api/upscale/workers/{id}/commands ───────────────────────────────────

func TestAdminHandler_PostWorkerCommand_ValidCommand(t *testing.T) {
	t.Parallel()
	cmd := &fakeCommander{}
	f := newAdminFixtureWithExtras(t, cmd, nil)

	body, _ := json.Marshal(map[string]interface{}{"cmd": "cancel"})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/workers/worker-1/commands", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("PostWorkerCommand(cancel) status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}
	cmd.mu.Lock()
	defer cmd.mu.Unlock()
	if len(cmd.calls) != 1 {
		t.Fatalf("expected 1 commander call; got %d", len(cmd.calls))
	}
	if cmd.calls[0].workerID != "worker-1" {
		t.Errorf("workerID = %q; want worker-1", cmd.calls[0].workerID)
	}
	if cmd.calls[0].cmd != "cancel" {
		t.Errorf("cmd = %q; want cancel", cmd.calls[0].cmd)
	}
}

func TestAdminHandler_PostWorkerCommand_InvalidCommand_Returns400(t *testing.T) {
	t.Parallel()
	cmd := &fakeCommander{err: errors.New("controlplane: command \"exec\" not allowed (whitelist: cancel|drain|shutdown|reconfigure|update)")}
	f := newAdminFixtureWithExtras(t, cmd, nil)

	body, _ := json.Marshal(map[string]interface{}{"cmd": "exec"})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/workers/worker-1/commands", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PostWorkerCommand(exec) status = %d; want 400 (body: %s)", rr.Code, rr.Body)
	}
}

func TestAdminHandler_PostWorkerCommand_WorkerNotConnected_Returns503(t *testing.T) {
	t.Parallel()
	cmd := &fakeCommander{err: errors.New("controlplane: worker not connected")}
	f := newAdminFixtureWithExtras(t, cmd, nil)

	body, _ := json.Marshal(map[string]interface{}{"cmd": "drain"})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/workers/missing-worker/commands", body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("PostWorkerCommand(worker missing) status = %d; want 503 (body: %s)", rr.Code, rr.Body)
	}
}

func TestAdminHandler_PostWorkerCommand_NoCommander_Returns503(t *testing.T) {
	t.Parallel()
	// No commander wired.
	f := newAdminFixtureWithExtras(t, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{"cmd": "cancel"})
	rr := doRequest(t, f.router, http.MethodPost, "/api/upscale/workers/w1/commands", body)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("PostWorkerCommand (no commander) status = %d; want 503", rr.Code)
	}
}

// ── GET /api/upscale/jobs/{id}/logs ──────────────────────────────────────────

func TestAdminHandler_GetJobLogs_ReturnsTail(t *testing.T) {
	t.Parallel()
	lb := &fakeLogBuffer{lines: []service.LogLine{
		{Source: "orchestrator", Level: "info", Msg: "started", TS: time.Now()},
		{Source: "worker", Level: "info", Msg: "segment 0 done", TS: time.Now()},
	}}
	f := newAdminFixtureWithExtras(t, nil, lb)

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs/job-abc/logs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GetJobLogs status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}
	var got []service.LogLine
	responseData(t, rr.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Errorf("GetJobLogs: got %d lines; want 2", len(got))
	}
}

func TestAdminHandler_GetJobLogs_NoLogBuffer_Returns503(t *testing.T) {
	t.Parallel()
	f := newAdminFixtureWithExtras(t, nil, nil)

	rr := doRequest(t, f.router, http.MethodGet, "/api/upscale/jobs/job-abc/logs", nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("GetJobLogs (no buffer) status = %d; want 503", rr.Code)
	}
}

// ── GET /api/upscale/jobs/{id}/logs/stream ───────────────────────────────────

func TestAdminHandler_StreamJobLogs_DeliversSSELines(t *testing.T) {
	t.Parallel()
	lb := &fakeLogBuffer{lines: []service.LogLine{
		{Source: "worker", Level: "info", Msg: "processing", TS: time.Now()},
	}}
	f := newAdminFixtureWithExtras(t, nil, lb)

	req := httptest.NewRequest(http.MethodGet, "/api/upscale/jobs/job-xyz/logs/stream", nil)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)

	// The fake subscribe sends one line then closes the channel, so the handler
	// returns after draining it. We only assert that:
	// 1. Content-Type is text/event-stream
	// 2. Body contains the SSE "data:" prefix
	// 3. Status 200
	if rr.Code != http.StatusOK {
		t.Fatalf("StreamJobLogs status = %d; want 200 (body: %s)", rr.Code, rr.Body)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("StreamJobLogs Content-Type = %q; want text/event-stream", ct)
	}
	body := rr.Body.String()
	if len(body) == 0 {
		t.Error("StreamJobLogs: body is empty; want at least one SSE data line")
	}
	if !containsStr(body, "data:") {
		t.Errorf("StreamJobLogs: body %q does not contain 'data:' SSE prefix", body)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
