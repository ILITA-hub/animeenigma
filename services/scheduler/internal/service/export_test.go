package service

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExportService(t *testing.T) (*ExportService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&domain.ExportJob{},
		&domain.AnimeLoadTask{},
	))

	svc := NewExportService(
		repo.NewExportJobRepository(db),
		repo.NewTaskRepository(db),
		logger.Default(),
	)
	return svc, db
}

func seedJobWithTasks(t *testing.T, db *gorm.DB, jobID, userID string, status domain.ExportJobStatus) {
	now := time.Now()
	require.NoError(t, db.Create(&domain.ExportJob{
		ID:          jobID,
		UserID:      userID,
		MALUsername: "testuser",
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error)

	tasks := []domain.AnimeLoadTask{
		{ID: jobID + "-t1", ExportJobID: jobID, UserID: userID, MALID: 1, MALTitle: "A", Status: domain.TaskStatusPending, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: jobID + "-t2", ExportJobID: jobID, UserID: userID, MALID: 2, MALTitle: "B", Status: domain.TaskStatusPending, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
		{ID: jobID + "-t3", ExportJobID: jobID, UserID: userID, MALID: 3, MALTitle: "C", Status: domain.TaskStatusCompleted, MaxAttempts: 3, CreatedAt: now, UpdatedAt: now},
	}
	for i := range tasks {
		require.NoError(t, db.Create(&tasks[i]).Error)
	}
}

func TestExportService_CancelExport_CancelsJobAndDrainsPendingTasks(t *testing.T) {
	svc, db := setupExportService(t)
	seedJobWithTasks(t, db, "job-1", "user-1", domain.ExportStatusProcessing)

	err := svc.CancelExport(context.Background(), "user-1", "job-1")
	require.NoError(t, err)

	var job domain.ExportJob
	require.NoError(t, db.First(&job, "id = ?", "job-1").Error)
	assert.Equal(t, domain.ExportStatusCancelled, job.Status)
	assert.NotNil(t, job.CompletedAt)

	// Pending tasks removed so the worker can't pick them up; completed kept.
	var remaining []domain.AnimeLoadTask
	require.NoError(t, db.Find(&remaining, "export_job_id = ?", "job-1").Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, domain.TaskStatusCompleted, remaining[0].Status)
}

func TestExportService_CancelExport_WrongUserReturnsNotFound(t *testing.T) {
	svc, db := setupExportService(t)
	seedJobWithTasks(t, db, "job-2", "user-1", domain.ExportStatusProcessing)

	err := svc.CancelExport(context.Background(), "user-other", "job-2")
	require.Error(t, err)
	appErr, ok := errors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeNotFound, appErr.Code)

	// Job untouched
	var job domain.ExportJob
	require.NoError(t, db.First(&job, "id = ?", "job-2").Error)
	assert.Equal(t, domain.ExportStatusProcessing, job.Status)
}

func TestExportService_CancelExport_MissingJobReturnsNotFound(t *testing.T) {
	svc, _ := setupExportService(t)

	err := svc.CancelExport(context.Background(), "user-1", "no-such-job")
	require.Error(t, err)
	appErr, ok := errors.IsAppError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeNotFound, appErr.Code)
}

func TestExportService_CancelExport_InactiveJobIsNoop(t *testing.T) {
	svc, db := setupExportService(t)
	seedJobWithTasks(t, db, "job-3", "user-1", domain.ExportStatusCompleted)

	err := svc.CancelExport(context.Background(), "user-1", "job-3")
	require.NoError(t, err)

	var job domain.ExportJob
	require.NoError(t, db.First(&job, "id = ?", "job-3").Error)
	assert.Equal(t, domain.ExportStatusCompleted, job.Status)

	// Tasks untouched (no drain on inactive jobs)
	var count int64
	require.NoError(t, db.Model(&domain.AnimeLoadTask{}).Where("export_job_id = ?", "job-3").Count(&count).Error)
	assert.Equal(t, int64(3), count)
}

func TestExportService_GetUserExports(t *testing.T) {
	svc, db := setupExportService(t)
	seedJobWithTasks(t, db, "job-4", "user-1", domain.ExportStatusCompleted)
	seedJobWithTasks(t, db, "job-5", "user-1", domain.ExportStatusProcessing)
	seedJobWithTasks(t, db, "job-6", "user-2", domain.ExportStatusProcessing)

	jobs, err := svc.GetUserExports(context.Background(), "user-1")
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	for _, j := range jobs {
		assert.Equal(t, "user-1", j.UserID)
	}
}
