package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
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

func TestSyncRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	job := &domain.SyncJob{
		ID:             "job-1",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		StartedAt:      time.Now(),
	}

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	// Verify retrievable
	got, err := repo.GetByID(ctx, "job-1")
	require.NoError(t, err)
	assert.Equal(t, "job-1", got.ID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "mal", got.Source)
	assert.Equal(t, "testuser", got.SourceUsername)
	assert.Equal(t, "processing", got.Status)
	assert.Equal(t, 100, got.Total)
}

func TestSyncRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	got, err := repo.GetByID(ctx, "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSyncRepository_GetActiveByUserAndSource(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	// Create a processing job
	processingJob := &domain.SyncJob{
		ID:             "job-active",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, processingJob))

	// Create a completed job (should be ignored)
	completedJob := &domain.SyncJob{
		ID:             "job-done",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "completed",
		Total:          50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, completedJob))

	// GetActive should return only the processing job
	got, err := repo.GetActiveByUserAndSource(ctx, "user-1", "mal")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "job-active", got.ID)
	assert.Equal(t, "processing", got.Status)
}

func TestSyncRepository_GetActiveByUserAndSource_NoneActive(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	// Create only completed jobs
	completedJob := &domain.SyncJob{
		ID:             "job-done",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "completed",
		Total:          50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, completedJob))

	got, err := repo.GetActiveByUserAndSource(ctx, "user-1", "mal")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSyncRepository_GetLatestByUserAndSource(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	earlier := time.Now().Add(-2 * time.Hour)
	later := time.Now().Add(-1 * time.Hour)

	// Create an older completed job
	oldJob := &domain.SyncJob{
		ID:             "job-old",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "completed",
		Total:          50,
		Imported:       40,
		Skipped:        10,
		StartedAt:      earlier.Add(-time.Minute),
		CompletedAt:    &earlier,
	}
	require.NoError(t, repo.Create(ctx, oldJob))

	// Create a newer completed job
	newJob := &domain.SyncJob{
		ID:             "job-new",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "completed",
		Total:          80,
		Imported:       70,
		Skipped:        10,
		StartedAt:      later.Add(-time.Minute),
		CompletedAt:    &later,
	}
	require.NoError(t, repo.Create(ctx, newJob))

	// Should return the newest
	got, err := repo.GetLatestByUserAndSource(ctx, "user-1", "mal")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "job-new", got.ID)
	assert.Equal(t, 70, got.Imported)
}

func TestSyncRepository_GetLatestByUserAndSource_NoneCompleted(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	// Only a processing job exists
	processingJob := &domain.SyncJob{
		ID:             "job-active",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, processingJob))

	got, err := repo.GetLatestByUserAndSource(ctx, "user-1", "mal")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestSyncRepository_UpdateProgress(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	job := &domain.SyncJob{
		ID:             "job-1",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, job))

	err := repo.UpdateProgress(ctx, "job-1", 25, 5)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, "job-1")
	require.NoError(t, err)
	assert.Equal(t, 25, got.Imported)
	assert.Equal(t, 5, got.Skipped)
	assert.Equal(t, "processing", got.Status) // status unchanged
}

func TestSyncRepository_Complete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	job := &domain.SyncJob{
		ID:             "job-1",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, job))

	err := repo.Complete(ctx, "job-1", "completed", "", 80, 20)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, "job-1")
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.Equal(t, 80, got.Imported)
	assert.Equal(t, 20, got.Skipped)
	assert.NotNil(t, got.CompletedAt)
	assert.Empty(t, got.ErrorMessage)

	// Test with error message
	job2 := &domain.SyncJob{
		ID:             "job-2",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          50,
		StartedAt:      time.Now(),
	}
	require.NoError(t, repo.Create(ctx, job2))

	err = repo.Complete(ctx, "job-2", "failed", "connection timeout", 10, 5)
	require.NoError(t, err)

	got2, err := repo.GetByID(ctx, "job-2")
	require.NoError(t, err)
	assert.Equal(t, "failed", got2.Status)
	assert.Equal(t, "connection timeout", got2.ErrorMessage)
	assert.NotNil(t, got2.CompletedAt)
}

func TestSyncRepository_MarkStaleJobsFailed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSyncRepository(db)
	ctx := context.Background()

	// Create an old processing job (started 2 hours ago)
	oldJob := &domain.SyncJob{
		ID:             "job-stale",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          100,
		StartedAt:      time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, repo.Create(ctx, oldJob))

	// Create a recent processing job (started 5 minutes ago)
	recentJob := &domain.SyncJob{
		ID:             "job-recent",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          50,
		StartedAt:      time.Now().Add(-5 * time.Minute),
	}
	require.NoError(t, repo.Create(ctx, recentJob))

	// Mark stale jobs (older than 1 hour)
	err := repo.MarkStaleJobsFailed(ctx, 1*time.Hour)
	require.NoError(t, err)

	// Old job should be failed
	got, err := repo.GetByID(ctx, "job-stale")
	require.NoError(t, err)
	assert.Equal(t, "failed", got.Status)
	assert.Equal(t, "stale job cleaned up on startup", got.ErrorMessage)
	assert.NotNil(t, got.CompletedAt)

	// Recent job should still be processing
	got2, err := repo.GetByID(ctx, "job-recent")
	require.NoError(t, err)
	assert.Equal(t, "processing", got2.Status)
}
