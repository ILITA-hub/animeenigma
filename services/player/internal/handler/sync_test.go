package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create table manually — SQLite doesn't support gen_random_uuid()
	err = db.Exec(`CREATE TABLE sync_jobs (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		source TEXT,
		source_username TEXT,
		status TEXT DEFAULT 'processing',
		total INTEGER DEFAULT 0,
		imported INTEGER DEFAULT 0,
		skipped INTEGER DEFAULT 0,
		error_message TEXT,
		started_at DATETIME,
		completed_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	return db
}

func TestSyncHandler_GetJobStatus_Unauthorized(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	r := chi.NewRouter()
	r.Get("/api/users/import/{jobId}", handler.GetJobStatus)

	req := httptest.NewRequest("GET", "/api/users/import/job-123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSyncHandler_GetJobStatus_NotFound(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	r := chi.NewRouter()
	r.Get("/api/users/import/{jobId}", handler.GetJobStatus)

	req := httptest.NewRequest("GET", "/api/users/import/nonexistent", nil)
	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSyncHandler_GetJobStatus_WrongUser(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	// Create a job belonging to user-A
	job := &domain.SyncJob{
		ID:             "job-owned-by-a",
		UserID:         "user-A",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		StartedAt:      time.Now(),
	}
	require.NoError(t, syncRepo.Create(context.Background(), job))

	r := chi.NewRouter()
	r.Get("/api/users/import/{jobId}", handler.GetJobStatus)

	// Request as user-B
	req := httptest.NewRequest("GET", "/api/users/import/job-owned-by-a", nil)
	claims := &authz.Claims{UserID: "user-B"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSyncHandler_GetJobStatus_Success(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	job := &domain.SyncJob{
		ID:             "job-success",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		Imported:       30,
		Skipped:        5,
		StartedAt:      time.Now(),
	}
	require.NoError(t, syncRepo.Create(context.Background(), job))

	r := chi.NewRouter()
	r.Get("/api/users/import/{jobId}", handler.GetJobStatus)

	req := httptest.NewRequest("GET", "/api/users/import/job-success", nil)
	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "job-success", data["id"])
	assert.Equal(t, "processing", data["status"])
	assert.Equal(t, float64(100), data["total"])
	assert.Equal(t, float64(30), data["imported"])
	assert.Equal(t, float64(5), data["skipped"])
}

func TestSyncHandler_GetSyncStatus_Unauthorized(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	req := httptest.NewRequest("GET", "/api/users/sync/status", nil)
	w := httptest.NewRecorder()

	handler.GetSyncStatus(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSyncHandler_GetSyncStatus_Empty(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	req := httptest.NewRequest("GET", "/api/users/sync/status", nil)
	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetSyncStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)

	// Both sources should be present
	malStatus, ok := data["mal"].(map[string]interface{})
	require.True(t, ok)
	assert.Nil(t, malStatus["active"])
	assert.Nil(t, malStatus["last_sync"])

	shikiStatus, ok := data["shikimori"].(map[string]interface{})
	require.True(t, ok)
	assert.Nil(t, shikiStatus["active"])
	assert.Nil(t, shikiStatus["last_sync"])
}

func TestSyncHandler_GetSyncStatus_WithJobs(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewSyncHandler(syncRepo, log)

	// Create an active MAL job
	malJob := &domain.SyncJob{
		ID:             "mal-active",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          200,
		Imported:       50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, syncRepo.Create(context.Background(), malJob))

	// Create a completed Shikimori job
	completedAt := time.Now().Add(-1 * time.Hour)
	shikiJob := &domain.SyncJob{
		ID:             "shiki-done",
		UserID:         "user-1",
		Source:         "shikimori",
		SourceUsername: "testuser",
		Status:         "completed",
		Total:          150,
		Imported:       140,
		Skipped:        10,
		StartedAt:      time.Now().Add(-2 * time.Hour),
		CompletedAt:    &completedAt,
	}
	require.NoError(t, syncRepo.Create(context.Background(), shikiJob))

	req := httptest.NewRequest("GET", "/api/users/sync/status", nil)
	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetSyncStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)

	// MAL should have active, no last_sync
	malStatus := data["mal"].(map[string]interface{})
	assert.NotNil(t, malStatus["active"])
	malActive := malStatus["active"].(map[string]interface{})
	assert.Equal(t, "mal-active", malActive["id"])

	// Shikimori should have no active, but has last_sync
	shikiStatus := data["shikimori"].(map[string]interface{})
	assert.Nil(t, shikiStatus["active"])
	assert.NotNil(t, shikiStatus["last_sync"])
	shikiLastSync := shikiStatus["last_sync"].(map[string]interface{})
	assert.Equal(t, "shiki-done", shikiLastSync["id"])
}
