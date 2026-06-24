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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
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
