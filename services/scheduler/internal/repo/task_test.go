package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Auto-migrate test tables
	if err := db.AutoMigrate(
		&domain.ExportJob{},
		&domain.AnimeLoadTask{},
		&domain.MALShikimoriMapping{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestTaskRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	task := &domain.AnimeLoadTask{
		ID:          "task-1",
		ExportJobID: "job-1",
		UserID:      "user-1",
		MALID:       12345,
		MALTitle:    "Test Anime",
		Status:      domain.TaskStatusPending,
		Priority:    0,
		MaxAttempts: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := repo.Create(context.Background(), task)
	require.NoError(t, err)

	// Verify it was created
	found, err := repo.GetByID(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Anime", found.MALTitle)
	assert.Equal(t, 12345, found.MALID)
}

func TestTaskRepository_GetNextPending(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	// Create tasks with different priorities and times
	now := time.Now()

	tasks := []*domain.AnimeLoadTask{
		{
			ID:          "task-low",
			ExportJobID: "job-1",
			UserID:      "user-1",
			MALID:       1,
			MALTitle:    "Low Priority",
			Status:      domain.TaskStatusPending,
			Priority:    0,
			MaxAttempts: 3,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "task-high",
			ExportJobID: "job-1",
			UserID:      "user-2",
			MALID:       2,
			MALTitle:    "High Priority",
			Status:      domain.TaskStatusPending,
			Priority:    10,
			MaxAttempts: 3,
			CreatedAt:   now,
			UpdatedAt:   now.Add(time.Second),
		},
		{
			ID:          "task-completed",
			ExportJobID: "job-1",
			UserID:      "user-1",
			MALID:       3,
			MALTitle:    "Completed",
			Status:      domain.TaskStatusCompleted,
			Priority:    10,
			MaxAttempts: 3,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}

	for _, task := range tasks {
		err := repo.Create(context.Background(), task)
		require.NoError(t, err)
	}

	// Get next pending - should be highest priority first
	next, err := repo.GetNextPending(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "task-high", next.ID)
}

func TestTaskRepository_MarkCompleted(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	task := &domain.AnimeLoadTask{
		ID:          "task-1",
		ExportJobID: "job-1",
		UserID:      "user-1",
		MALID:       12345,
		MALTitle:    "Test Anime",
		Status:      domain.TaskStatusPending,
		Priority:    0,
		MaxAttempts: 3,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := repo.Create(context.Background(), task)
	require.NoError(t, err)

	// Mark as completed
	err = repo.MarkCompleted(context.Background(), "task-1", "z12345", "anime-uuid", domain.ResolutionExactJapanese)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCompleted, found.Status)
	assert.Equal(t, "z12345", found.ResolvedShikimoriID)
	assert.Equal(t, "anime-uuid", found.ResolvedAnimeID)
	assert.Equal(t, domain.ResolutionExactJapanese, found.ResolutionMethod)
}

func TestTaskRepository_MarkFailed_WithRetry(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	task := &domain.AnimeLoadTask{
		ID:           "task-1",
		ExportJobID:  "job-1",
		UserID:       "user-1",
		MALID:        12345,
		MALTitle:     "Test Anime",
		Status:       domain.TaskStatusPending,
		Priority:     0,
		AttemptCount: 1,
		MaxAttempts:  3,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := repo.Create(context.Background(), task)
	require.NoError(t, err)

	// Mark as failed with retry
	retryTime := time.Now().Add(30 * time.Second)
	err = repo.MarkFailed(context.Background(), "task-1", "network error", &retryTime)
	require.NoError(t, err)

	// Verify - should be pending again since attempts < max
	found, err := repo.GetByID(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusPending, found.Status)
	assert.Equal(t, "network error", found.LastError)
	assert.NotNil(t, found.NextRetryAt)
}

func TestTaskRepository_MarkFailed_MaxAttempts(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	task := &domain.AnimeLoadTask{
		ID:           "task-1",
		ExportJobID:  "job-1",
		UserID:       "user-1",
		MALID:        12345,
		MALTitle:     "Test Anime",
		Status:       domain.TaskStatusPending,
		Priority:     0,
		AttemptCount: 3, // At max attempts
		MaxAttempts:  3,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := repo.Create(context.Background(), task)
	require.NoError(t, err)

	// Mark as failed - should be permanent since at max attempts
	err = repo.MarkFailed(context.Background(), "task-1", "final failure", nil)
	require.NoError(t, err)

	// Verify - should be failed permanently
	found, err := repo.GetByID(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusFailed, found.Status)
	assert.Equal(t, "final failure", found.LastError)
}

func TestTaskRepository_GetStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	exportJobID := "job-1"
	now := time.Now()

	// Create tasks with various statuses
	tasks := []*domain.AnimeLoadTask{
		{ID: "t1", ExportJobID: exportJobID, UserID: "u1", MALID: 1, MALTitle: "A1", Status: domain.TaskStatusPending, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "t2", ExportJobID: exportJobID, UserID: "u1", MALID: 2, MALTitle: "A2", Status: domain.TaskStatusPending, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "t3", ExportJobID: exportJobID, UserID: "u1", MALID: 3, MALTitle: "A3", Status: domain.TaskStatusProcessing, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "t4", ExportJobID: exportJobID, UserID: "u1", MALID: 4, MALTitle: "A4", Status: domain.TaskStatusCompleted, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "t5", ExportJobID: exportJobID, UserID: "u1", MALID: 5, MALTitle: "A5", Status: domain.TaskStatusFailed, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: "t6", ExportJobID: exportJobID, UserID: "u1", MALID: 6, MALTitle: "A6", Status: domain.TaskStatusSkipped, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
	}

	for _, task := range tasks {
		err := repo.Create(context.Background(), task)
		require.NoError(t, err)
	}

	stats, err := repo.GetStats(context.Background(), exportJobID)
	require.NoError(t, err)

	assert.Equal(t, 6, stats.Total)
	assert.Equal(t, 2, stats.Pending)
	assert.Equal(t, 1, stats.Processing)
	assert.Equal(t, 1, stats.Completed)
	assert.Equal(t, 1, stats.Failed)
	assert.Equal(t, 1, stats.Skipped)
}

func TestTaskRepository_ResetStuckTasks(t *testing.T) {
	db := setupTestDB(t)
	repo := NewTaskRepository(db)

	now := time.Now()
	stuckTime := now.Add(-10 * time.Minute) // 10 minutes ago

	// Create tasks - one stuck, one recent
	tasks := []*domain.AnimeLoadTask{
		{
			ID:          "stuck-task",
			ExportJobID: "job-1",
			UserID:      "u1",
			MALID:       1,
			MALTitle:    "Stuck",
			Status:      domain.TaskStatusProcessing,
			MaxAttempts: 3,
			CreatedAt:   stuckTime,
			UpdatedAt:   stuckTime,
		},
		{
			ID:          "recent-task",
			ExportJobID: "job-1",
			UserID:      "u1",
			MALID:       2,
			MALTitle:    "Recent",
			Status:      domain.TaskStatusProcessing,
			MaxAttempts: 3,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}

	for _, task := range tasks {
		err := repo.Create(context.Background(), task)
		require.NoError(t, err)
	}

	// Reset tasks stuck for more than 5 minutes
	count, err := repo.ResetStuckTasks(context.Background(), 5*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify stuck task is now pending
	stuck, err := repo.GetByID(context.Background(), "stuck-task")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusPending, stuck.Status)

	// Recent task should still be processing
	recent, err := repo.GetByID(context.Background(), "recent-task")
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusProcessing, recent.Status)
}
